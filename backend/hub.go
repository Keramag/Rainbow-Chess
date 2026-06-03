package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"rainbow-chess/engine"

	"github.com/google/uuid"
)

const (
	// defaultChallengeTTL is how long a pending challenge lives before it
	// expires unanswered. defaultExpiryInterval is how often the hub sweeps for
	// expired challenges. Both are fields on Hub so tests can shrink them.
	defaultChallengeTTL   = 30 * time.Second
	defaultExpiryInterval = 1 * time.Second
)

// hub.go is the heart of the server: a single goroutine (run) owns all shared
// state — connected clients, users, pending challenges, and active games — and
// mutates it only in response to channel events. Because everything funnels
// through one goroutine there are no locks on the game state. Adapted from
// virusgame's hub, stripped of all lobby/bot/neutral-cell machinery.
//
// This file currently implements connection lifecycle (connect/disconnect and
// the online-users list). The challenge flow, in-game move protocol, and
// persistence are layered on in later tasks by adding cases to
// handleClientMessage and helpers below.

// MessageWrapper pairs an inbound message with the client that sent it, so the
// hub goroutine knows whom to attribute and reply to.
type MessageWrapper struct {
	client  *Client
	message *Message
}

// Hub maintains the set of active clients and routes all messages.
type Hub struct {
	clients    map[*Client]bool
	users      map[string]*User      // userID -> User
	challenges map[string]*Challenge // challengeID -> Challenge
	games      map[string]*Game      // gameID -> Game

	register      chan *Client
	unregister    chan *Client
	handleMessage chan *MessageWrapper

	// challengeTTL / expiryInterval govern the pending-challenge expiry sweep.
	// Set from newHub's defaults; tests may override them before run() starts.
	challengeTTL   time.Duration
	expiryInterval time.Duration
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		users:      make(map[string]*User),
		challenges: make(map[string]*Challenge),
		games:      make(map[string]*Game),

		register:      make(chan *Client),
		unregister:    make(chan *Client),
		handleMessage: make(chan *MessageWrapper, 256), // buffered: handlers may enqueue internal messages

		challengeTTL:   defaultChallengeTTL,
		expiryInterval: defaultExpiryInterval,
	}
}

// run is the hub's single event loop. It must be started in its own goroutine
// and is the sole mutator of the hub's maps.
func (h *Hub) run() {
	expiry := time.NewTicker(h.expiryInterval)
	defer expiry.Stop()

	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			h.handleConnect(client)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				h.handleDisconnect(client)
				delete(h.clients, client)
				close(client.send)
			}
		case wrapper := <-h.handleMessage:
			h.handleClientMessage(wrapper.client, wrapper.message)
		case now := <-expiry.C:
			h.checkExpiredChallenges(now)
		}
	}
}

// handleConnect assigns the new client an anonymous identity, greets it with a
// welcome message carrying the registered variant list, and refreshes the
// online-users list for everyone.
func (h *Hub) handleConnect(client *Client) {
	username := GenerateRandomName()
	userID := uuid.New().String()

	user := &User{
		ID:       userID,
		Username: username,
		Client:   client,
	}
	client.user = user
	h.users[userID] = user

	h.sendToClient(client, &Message{
		Type:     "welcome",
		UserID:   userID,
		Username: username,
		Variants: engine.List(),
	})

	h.broadcastUserList()
	log.Printf("User connected: %s (%s)", username, userID)
}

// handleDisconnect tears down everything associated with a leaving client:
// pending challenges it was party to, any game it was in (the opponent is
// notified and freed), and the user record itself.
func (h *Hub) handleDisconnect(client *Client) {
	if client.user == nil {
		return
	}
	user := client.user
	log.Printf("User disconnected: %s (%s)", user.Username, user.ID)

	// Drop pending challenges involving this user.
	for id, ch := range h.challenges {
		if ch.FromUser.ID == user.ID || ch.ToUser.ID == user.ID {
			delete(h.challenges, id)
		}
	}

	// Tear down any game this user was in, notifying the opponent.
	for gameID, game := range h.games {
		var opponent *User
		switch {
		case game.White != nil && game.White.ID == user.ID:
			opponent = game.Black
		case game.Black != nil && game.Black.ID == user.ID:
			opponent = game.White
		default:
			continue
		}
		if opponent != nil {
			opponent.InGame = false
			opponent.GameID = ""
			h.sendToUser(opponent, &Message{
				Type:   "opponent_disconnected",
				GameID: gameID,
			})
		}
		delete(h.games, gameID)
	}

	delete(h.users, user.ID)
	h.broadcastUserList()
}

// handleClientMessage dispatches an inbound message to its handler. The
// in-game move/resign handlers are added in later tasks; unknown types are
// logged and ignored.
func (h *Hub) handleClientMessage(client *Client, msg *Message) {
	if client == nil || client.user == nil {
		return
	}
	switch msg.Type {
	case "challenge":
		h.handleChallenge(client.user, msg)
	case "accept_challenge":
		h.handleAcceptChallenge(client.user, msg)
	case "decline_challenge":
		h.handleDeclineChallenge(client.user, msg)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

// handleChallenge creates a pending 1v1 invitation from one user to another
// under a chosen variant. It rejects self-challenges, offline or busy targets,
// unknown variants, and duplicate pending challenges, replying to the sender
// with an error in each case. On success the target receives challenge_received.
func (h *Hub) handleChallenge(from *User, msg *Message) {
	if msg.TargetUserID == from.ID {
		h.sendError(from, "You cannot challenge yourself")
		return
	}
	to, ok := h.users[msg.TargetUserID]
	if !ok {
		h.sendError(from, "That user is not online")
		return
	}
	if from.InGame {
		h.sendError(from, "You are already in a game")
		return
	}
	if to.InGame {
		h.sendError(from, "That user is already in a game")
		return
	}
	// The variant must be registered; this also rejects an empty variant.
	if _, err := engine.Get(msg.Variant); err != nil {
		h.sendError(from, fmt.Sprintf("Unknown variant %q", msg.Variant))
		return
	}
	// Disallow piling up multiple pending challenges to the same target.
	for _, c := range h.challenges {
		if c.FromUser.ID == from.ID && c.ToUser.ID == to.ID {
			h.sendError(from, "You already have a pending challenge to this user")
			return
		}
	}

	challengeID := uuid.New().String()
	h.challenges[challengeID] = &Challenge{
		ID:        challengeID,
		FromUser:  from,
		ToUser:    to,
		Variant:   msg.Variant,
		CreatedAt: time.Now(),
	}

	h.sendToUser(to, &Message{
		Type:         "challenge_received",
		ChallengeID:  challengeID,
		FromUserID:   from.ID,
		FromUsername: from.Username,
		Variant:      msg.Variant,
	})
	log.Printf("Challenge created: %s -> %s (%s)", from.Username, to.Username, msg.Variant)
}

// handleAcceptChallenge turns a pending challenge into a live game. It revalidates
// the variant and that both players are still free (either could have started a
// game since the challenge was issued), creates the initial position via the
// variant, fixes colors (challenger = White, acceptor = Black), marks both users
// in-game, and sends each player a game_start carrying their color, the FEN, and
// the side-to-move's legal moves.
func (h *Hub) handleAcceptChallenge(user *User, msg *Message) {
	challenge, ok := h.challenges[msg.ChallengeID]
	if !ok {
		h.sendError(user, "That challenge no longer exists")
		return
	}
	if challenge.ToUser.ID != user.ID {
		// Not this user's challenge to accept; ignore silently.
		log.Printf("User %s tried to accept a challenge not meant for them", user.Username)
		return
	}
	// Consume the challenge up front so neither a double-accept nor the expiry
	// sweep can act on it again.
	delete(h.challenges, msg.ChallengeID)

	from := challenge.FromUser
	if from.InGame {
		h.sendError(user, "The challenger is already in another game")
		return
	}
	if user.InGame {
		h.sendError(user, "You are already in a game")
		return
	}

	variant, err := engine.Get(challenge.Variant)
	if err != nil {
		h.sendError(user, fmt.Sprintf("Unknown variant %q", challenge.Variant))
		h.sendError(from, fmt.Sprintf("Unknown variant %q", challenge.Variant))
		return
	}

	gameID := uuid.New().String()
	pos := variant.InitialPosition()
	game := &Game{
		ID:        gameID,
		Variant:   challenge.Variant,
		White:     from,
		Black:     user,
		Position:  pos,
		StartTime: time.Now(),
		LastMove:  time.Now(),
	}
	h.games[gameID] = game

	from.InGame, from.GameID = true, gameID
	user.InGame, user.GameID = true, gameID

	fen := pos.FEN()
	legal := movesToDTO(variant.LegalMoves(pos))

	h.sendToUser(from, &Message{
		Type:       "game_start",
		GameID:     gameID,
		Variant:    challenge.Variant,
		Color:      "white",
		FEN:        fen,
		LegalMoves: legal,
	})
	h.sendToUser(user, &Message{
		Type:       "game_start",
		GameID:     gameID,
		Variant:    challenge.Variant,
		Color:      "black",
		FEN:        fen,
		LegalMoves: legal,
	})

	h.broadcastUserList()
	log.Printf("Game started: %s (white) vs %s (black) — %s [%s]", from.Username, user.Username, challenge.Variant, gameID)
}

// handleDeclineChallenge removes a pending challenge at the target's request and
// notifies the challenger. Only the challenge's target may decline it.
func (h *Hub) handleDeclineChallenge(user *User, msg *Message) {
	challenge, ok := h.challenges[msg.ChallengeID]
	if !ok {
		return
	}
	if challenge.ToUser.ID != user.ID {
		return
	}
	delete(h.challenges, msg.ChallengeID)
	h.sendToUser(challenge.FromUser, &Message{
		Type:        "challenge_declined",
		ChallengeID: msg.ChallengeID,
	})
	log.Printf("Challenge declined: %s declined %s", user.Username, challenge.FromUser.Username)
}

// checkExpiredChallenges drops every pending challenge older than challengeTTL,
// notifying both parties with challenge_expired. It is called from the hub's
// single goroutine on each expiry tick, so it mutates h.challenges safely.
func (h *Hub) checkExpiredChallenges(now time.Time) {
	for id, c := range h.challenges {
		if now.Sub(c.CreatedAt) < h.challengeTTL {
			continue
		}
		h.sendToUser(c.FromUser, &Message{Type: "challenge_expired", ChallengeID: id})
		h.sendToUser(c.ToUser, &Message{Type: "challenge_expired", ChallengeID: id})
		delete(h.challenges, id)
		log.Printf("Challenge expired: %s -> %s", c.FromUser.Username, c.ToUser.Username)
	}
}

// broadcast marshals msg once and pushes it to every connected client's send
// channel, dropping any client whose buffer is full (treated as dead).
func (h *Hub) broadcast(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			log.Printf("Failed to broadcast to client, removing from clients map")
			delete(h.clients, client)
		}
	}
}

// broadcastUserList sends the current online-users roster to everyone. This is
// the list the frontend renders with Challenge buttons.
func (h *Hub) broadcastUserList() {
	users := make([]UserInfo, 0, len(h.users))
	for _, user := range h.users {
		users = append(users, UserInfo{
			UserID:   user.ID,
			Username: user.Username,
			InGame:   user.InGame,
		})
	}
	h.broadcast(&Message{
		Type:  "users_update",
		Users: users,
	})
}

// sendToClient marshals and queues msg for a single client, dropping it if its
// buffer is full or it has already disconnected.
func (h *Hub) sendToClient(client *Client, msg *Message) {
	if _, exists := h.clients[client]; !exists {
		return // client already disconnected
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	select {
	case client.send <- data:
	default:
		log.Printf("Failed to send to client, removing from clients map")
		delete(h.clients, client)
	}
}

// sendToUser sends msg to the user's current client, if connected.
func (h *Hub) sendToUser(user *User, msg *Message) {
	if user != nil && user.Client != nil {
		h.sendToClient(user.Client, msg)
	}
}

// sendError sends a one-off error message to a single user.
func (h *Hub) sendError(user *User, message string) {
	h.sendToUser(user, &Message{
		Type:    "error",
		Message: message,
	})
}
