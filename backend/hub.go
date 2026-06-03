package main

import (
	"encoding/json"
	"log"

	"rainbow-chess/engine"

	"github.com/google/uuid"
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
	}
}

// run is the hub's single event loop. It must be started in its own goroutine
// and is the sole mutator of the hub's maps.
func (h *Hub) run() {
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
// challenge/move/resign handlers are added in later tasks; unknown types are
// logged and ignored.
func (h *Hub) handleClientMessage(client *Client, msg *Message) {
	if client == nil || client.user == nil {
		return
	}
	switch msg.Type {
	default:
		log.Printf("Unknown message type: %s", msg.Type)
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
