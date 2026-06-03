package main

import (
	"time"

	"rainbow-chess/engine"
)

// types.go holds the transport-layer data model: the WebSocket message envelope
// and the server-side notions of a connected User, a pending Challenge, and an
// in-progress chess Game. It is deliberately stripped of all the virusgame
// lobby/bot/neutral-cell concepts — Rainbow Chess is strictly anonymous 1v1.
//
// Positions always travel as FEN and the legal-move list for the side to move is
// always included, so the client never re-implements chess rules. Squares in the
// wire protocol are algebraic ("e2", "e4"); promotions are the engine's piece
// letters ("q", "r", "b", "n").

// Message is the single JSON envelope exchanged in both directions over the
// WebSocket. Only the fields relevant to a given Type are populated; everything
// is omitempty so each message stays compact.
type Message struct {
	Type string `json:"type"`

	// Identity / welcome (server -> client).
	UserID   string   `json:"userId,omitempty"`
	Username string   `json:"username,omitempty"`
	Variants []string `json:"variants,omitempty"` // registered variant names

	// Online-users list (server -> client).
	Users []UserInfo `json:"users,omitempty"`

	// Challenge flow.
	TargetUserID string `json:"targetUserId,omitempty"` // challenge (client -> server)
	ChallengeID  string `json:"challengeId,omitempty"`
	FromUserID   string `json:"fromUserId,omitempty"`
	FromUsername string `json:"fromUsername,omitempty"`
	Variant      string `json:"variant,omitempty"`

	// Game state.
	GameID     string     `json:"gameId,omitempty"`
	Color      string     `json:"color,omitempty"` // recipient's color: "white"/"black"
	FEN        string     `json:"fen,omitempty"`
	SideToMove string     `json:"sideToMove,omitempty"`
	InCheck    bool       `json:"inCheck,omitempty"` // is the side to move currently in check
	LegalMoves []MoveDTO  `json:"legalMoves,omitempty"`
	Move       *MoveDTO   `json:"move,omitempty"`     // move (client -> server)
	LastMove   *MoveDTO   `json:"lastMove,omitempty"` // last move played (server -> client)
	Result     *ResultDTO `json:"result,omitempty"`

	// Error (server -> client).
	Message string `json:"message,omitempty"`

	// TimerSeq is an internal, hub-only field (never serialized): it stamps an
	// auto-resign move_timeout message with the move count at which the timer
	// was armed, so a timer that fires after its turn has already passed can be
	// recognised as stale and ignored.
	TimerSeq int `json:"-"`
}

// UserInfo is a connected user as advertised in the online-users list.
type UserInfo struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	InGame   bool   `json:"inGame"`
}

// MoveDTO is the wire representation of a move: algebraic squares plus an
// optional promotion piece letter.
type MoveDTO struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Promotion string `json:"promotion,omitempty"`
}

// ResultDTO is the wire representation of a finished game's outcome.
type ResultDTO struct {
	Outcome string `json:"outcome"`          // "ongoing"/"white_wins"/"black_wins"/"draw"
	Reason  string `json:"reason,omitempty"` // "checkmate"/"stalemate"/"resignation"/...
}

// User is a connected client's server-side identity. Anonymous and ephemeral:
// created on connect, deleted on disconnect.
type User struct {
	ID       string
	Username string
	Client   *Client
	InGame   bool
	GameID   string // ID of the game the user is currently in, if any
}

// Challenge is a pending 1v1 invitation from one user to another under a chosen
// variant. It expires if not accepted in time (see the hub's expiry ticker).
type Challenge struct {
	ID        string
	FromUser  *User
	ToUser    *User
	Variant   string
	CreatedAt time.Time
}

// Game is an in-progress chess game. The engine.Position is authoritative; the
// hub validates every move against it and broadcasts the resulting FEN. Colors
// are fixed at creation: the challenger plays White, the acceptor plays Black.
type Game struct {
	ID       string
	Variant  string
	White    *User
	Black    *User
	Position *engine.Position
	Result   engine.GameResult
	GameOver bool

	Moves     []string // UCI move history, sufficient to replay the game
	StartTime time.Time
	EndTime   time.Time

	MoveTimer *time.Timer // auto-resign timer for the side to move
}
