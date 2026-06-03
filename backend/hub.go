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

	// defaultMoveTimeout is how long the side to move has before it is
	// auto-resigned. It is a field on Hub so tests can shrink it.
	defaultMoveTimeout = 120 * time.Second
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

	// moveTimeout is the per-turn auto-resign deadline. Set from newHub's
	// default; tests may shrink it before run() starts.
	moveTimeout time.Duration

	// gameEnded is the persistence hook fired once when a game finishes (with
	// its final result recorded). It is nil by default — Task 10 wires it to the
	// SQLite SaveGame path. It runs on the hub goroutine, so it must not block.
	gameEnded func(*Game)
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
		moveTimeout:    defaultMoveTimeout,
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

	// Tear down any game this user was in: the opponent wins by abandonment.
	// endGame records the result, fires persistence, frees the opponent and
	// removes the game; deleting from h.games while ranging it is safe in Go.
	for _, game := range h.games {
		if game.GameOver {
			continue
		}
		var opponent *User
		var result engine.GameResult
		switch {
		case game.White != nil && game.White.ID == user.ID:
			opponent, result = game.Black, engine.GameResult{Outcome: engine.BlackWins, Reason: "opponent disconnected"}
		case game.Black != nil && game.Black.ID == user.ID:
			opponent, result = game.White, engine.GameResult{Outcome: engine.WhiteWins, Reason: "opponent disconnected"}
		default:
			continue
		}
		if opponent != nil {
			h.sendToUser(opponent, &Message{
				Type:   "opponent_disconnected",
				GameID: game.ID,
				Result: resultToDTO(result),
			})
		}
		h.endGame(game, result)
	}

	delete(h.users, user.ID)
	h.broadcastUserList()
}

// handleClientMessage dispatches an inbound message to its handler. Most
// messages carry the client that sent them, but the auto-resign timer enqueues
// an internal move_timeout message with no client — that is handled first,
// before the client/user guard. Unknown types are logged and ignored.
func (h *Hub) handleClientMessage(client *Client, msg *Message) {
	// Internal, hub-generated messages have no associated client.
	if msg.Type == "move_timeout" {
		h.handleMoveTimeout(msg)
		return
	}
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
	case "move":
		h.handleMove(client.user, msg)
	case "resign":
		h.handleResign(client.user, msg)
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
	}
	h.games[gameID] = game

	from.InGame, from.GameID = true, gameID
	user.InGame, user.GameID = true, gameID

	fen := pos.FEN()
	legal := movesToDTO(variant.LegalMoves(pos))
	inCheck := engine.IsInCheck(pos, pos.SideToMove)

	// game_start carries the opponent's username so the client never has to
	// remember whom it challenged/accepted — which would be ambiguous when a user
	// has several outgoing challenges pending and any of them might accept first.
	h.sendToUser(from, &Message{
		Type:         "game_start",
		GameID:       gameID,
		Variant:      challenge.Variant,
		Color:        "white",
		OpponentName: user.Username,
		FEN:          fen,
		SideToMove:   pos.SideToMove.String(),
		InCheck:      inCheck,
		LegalMoves:   legal,
	})
	h.sendToUser(user, &Message{
		Type:         "game_start",
		GameID:       gameID,
		Variant:      challenge.Variant,
		Color:        "black",
		OpponentName: from.Username,
		FEN:          fen,
		SideToMove:   pos.SideToMove.String(),
		InCheck:      inCheck,
		LegalMoves:   legal,
	})

	h.broadcastUserList()
	// The clock starts on White the moment the game begins.
	h.startMoveTimer(game)
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

// gamePlayerColor reports the color user plays in game, and whether the user is
// in fact one of the two players. It is the single source of truth for "whose
// move is this", used by the move and resign handlers.
func gamePlayerColor(game *Game, user *User) (engine.Color, bool) {
	switch {
	case game.White != nil && game.White.ID == user.ID:
		return engine.White, true
	case game.Black != nil && game.Black.ID == user.ID:
		return engine.Black, true
	default:
		return engine.White, false
	}
}

// handleMove validates and applies an in-game move from user. The server is
// authoritative: it rejects moves from a player not in a game, from a player not
// on turn, and any move the variant deems illegal — surfacing each as an error
// to the sender only. On a legal move it advances the position and broadcasts a
// game_update (fen, side to move, the next player's legal moves, the move just
// played, and the current result) to both players. A move that ends the game is
// finalized via endGame; otherwise the auto-resign clock is re-armed.
func (h *Hub) handleMove(user *User, msg *Message) {
	game, ok := h.games[user.GameID]
	if !ok {
		h.sendError(user, "You are not in a game")
		return
	}
	if game.GameOver {
		h.sendError(user, "That game is already over")
		return
	}
	color, isPlayer := gamePlayerColor(game, user)
	if !isPlayer {
		h.sendError(user, "You are not a player in this game")
		return
	}
	if game.Position.SideToMove != color {
		h.sendError(user, "It is not your turn")
		return
	}
	if msg.Move == nil {
		h.sendError(user, "No move supplied")
		return
	}

	mv, err := dtoToMove(*msg.Move)
	if err != nil {
		h.sendError(user, fmt.Sprintf("Invalid move: %v", err))
		return
	}
	variant, err := engine.Get(game.Variant)
	if err != nil {
		h.sendError(user, fmt.Sprintf("Unknown variant %q", game.Variant))
		return
	}
	next, err := variant.ApplyMove(game.Position, mv)
	if err != nil {
		// Illegal moves are a normal client occurrence; tell only the sender.
		h.sendError(user, "Illegal move")
		return
	}

	game.Position = next
	game.Moves = append(game.Moves, mv.String())

	result := variant.Result(next)
	lastMove := moveToDTO(mv)
	update := &Message{
		Type:       "game_update",
		GameID:     game.ID,
		FEN:        next.FEN(),
		SideToMove: next.SideToMove.String(),
		InCheck:    engine.IsInCheck(next, next.SideToMove),
		LegalMoves: movesToDTO(variant.LegalMoves(next)),
		LastMove:   &lastMove,
		Result:     resultToDTO(result),
	}
	h.sendToUser(game.White, update)
	h.sendToUser(game.Black, update)

	if result.IsOver() {
		// The terminal result already rode out on the game_update above; endGame
		// just records it, frees the players and fires persistence.
		h.endGame(game, result)
		return
	}
	h.startMoveTimer(game)
}

// handleResign ends user's current game in their opponent's favor. Only a player
// in the game may resign it; the opponent is awarded the win by resignation.
func (h *Hub) handleResign(user *User, msg *Message) {
	game, ok := h.games[user.GameID]
	if !ok || game.GameOver {
		return
	}
	color, isPlayer := gamePlayerColor(game, user)
	if !isPlayer {
		h.sendError(user, "You are not a player in this game")
		return
	}
	result := winFor(color.Opposite(), "resignation")
	h.broadcastGameResult(game, result)
	h.endGame(game, result)
	log.Printf("Game %s ended by resignation (%s resigned)", game.ID, user.Username)
}

// handleMoveTimeout auto-resigns the side to move when its turn clock expires.
// The timer fires on its own goroutine and routes here through the hub channel,
// so it may arrive after the turn has already passed: TimerSeq is compared
// against the current move count and a stale timeout is ignored. A timeout that
// is still current ends the game in the waiting player's favor.
func (h *Hub) handleMoveTimeout(msg *Message) {
	game, ok := h.games[msg.GameID]
	if !ok || game.GameOver {
		return
	}
	if msg.TimerSeq != len(game.Moves) {
		return // a move was played after this timer was armed; ignore.
	}
	result := winFor(game.Position.SideToMove.Opposite(), "timeout")
	h.broadcastGameResult(game, result)
	h.endGame(game, result)
	log.Printf("Game %s ended on timeout (%s to move ran out of time)", game.ID, game.Position.SideToMove)
}

// winFor builds a GameResult awarding the win to the given color for reason.
func winFor(c engine.Color, reason string) engine.GameResult {
	if c == engine.White {
		return engine.GameResult{Outcome: engine.WhiteWins, Reason: reason}
	}
	return engine.GameResult{Outcome: engine.BlackWins, Reason: reason}
}

// startMoveTimer arms (or re-arms) the auto-resign clock for the side to move.
// The timer fires on its own goroutine, so it does not touch game state
// directly — it enqueues a move_timeout message stamped with the current move
// count so the hub goroutine can detect and discard a stale firing.
func (h *Hub) startMoveTimer(game *Game) {
	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
		game.MoveTimer = nil
	}
	if game.GameOver {
		return
	}
	gameID := game.ID
	seq := len(game.Moves)
	game.MoveTimer = time.AfterFunc(h.moveTimeout, func() {
		h.handleMessage <- &MessageWrapper{
			client:  nil,
			message: &Message{Type: "move_timeout", GameID: gameID, TimerSeq: seq},
		}
	})
}

// broadcastGameResult sends a game_update carrying the final result (and the
// current FEN) to both players. Used for endings that are not the direct
// consequence of a move — resignation and timeout — where there is no last move
// or legal-move list to ship.
func (h *Hub) broadcastGameResult(game *Game, result engine.GameResult) {
	msg := &Message{
		Type:       "game_update",
		GameID:     game.ID,
		FEN:        game.Position.FEN(),
		SideToMove: game.Position.SideToMove.String(),
		Result:     resultToDTO(result),
	}
	h.sendToUser(game.White, msg)
	h.sendToUser(game.Black, msg)
}

// endGame finalizes an in-progress game exactly once: it records the result,
// stops the move timer, frees both players, fires the persistence hook (Task
// 10), removes the game from the active set and refreshes the online-users
// roster. It must run on the hub goroutine. Callers are responsible for sending
// any player-facing result message first (a move ending carries the result on
// its game_update; resign/timeout use broadcastGameResult).
func (h *Hub) endGame(game *Game, result engine.GameResult) {
	if game.GameOver {
		return
	}
	game.GameOver = true
	game.Result = result
	game.EndTime = time.Now()

	if game.MoveTimer != nil {
		game.MoveTimer.Stop()
		game.MoveTimer = nil
	}

	if game.White != nil {
		game.White.InGame, game.White.GameID = false, ""
	}
	if game.Black != nil {
		game.Black.InGame, game.Black.GameID = false, ""
	}

	if h.gameEnded != nil {
		h.gameEnded(game)
	}

	delete(h.games, game.ID)
	h.broadcastUserList()
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
// channel, evicting any client whose buffer is full (treated as dead). Dead
// clients are collected and torn down after the range so that evictClient's own
// teardown broadcasts do not mutate the map mid-iteration.
func (h *Hub) broadcast(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("Error marshaling broadcast message: %v", err)
		return
	}
	var dead []*Client
	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			dead = append(dead, client)
		}
	}
	for _, client := range dead {
		log.Printf("Failed to broadcast to client, evicting it")
		h.evictClient(client)
	}
}

// evictClient tears down a client the hub can no longer reach — its send buffer
// is full, so it is treated as dead. It runs the same teardown as a normal
// unregister (drop pending challenges, end any game and free the opponent,
// remove the user) exactly once. The client is removed from the map and its send
// channel closed *before* handleDisconnect runs so the teardown's own broadcasts
// skip it and cannot re-enter this path for the same client; the readPump's
// later unregister then no-ops on the missing map entry.
func (h *Hub) evictClient(client *Client) {
	if _, ok := h.clients[client]; !ok {
		return // already torn down
	}
	delete(h.clients, client)
	close(client.send)
	h.handleDisconnect(client)
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
		log.Printf("Failed to send to client, evicting it")
		h.evictClient(client)
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
