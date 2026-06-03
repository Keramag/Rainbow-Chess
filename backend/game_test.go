package main

import (
	"testing"
	"time"

	"rainbow-chess/engine"
)

// game_test.go exercises the in-game move protocol: legal-move application and
// the game_update broadcast, turn/ownership enforcement, illegal-move rejection,
// and every game-ending path (checkmate, stalemate, resignation, timeout,
// disconnect) plus the persistence hook that fires on game end.

// startGame connects two clients, has the first challenge the second under the
// given variant, accepts it, and returns both clients (white = challenger,
// black = acceptor) and the game id. Both game_start messages are consumed.
func startGame(t *testing.T, h *Hub, variant string) (white, black *Client, gameID string) {
	t.Helper()
	c1, _ := connectClient(t, h)
	c2, id2 := connectClient(t, h)

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: variant})
	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return c1, c2, ""
	}
	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})

	ws := waitForMessage(t, c1, "game_start")
	waitForMessage(t, c2, "game_start")
	if ws == nil {
		return c1, c2, ""
	}
	return c1, c2, ws.GameID
}

// move sends a {from,to} move from mover and returns the game_update mover
// receives. Every legal move broadcasts a game_update to BOTH players, so the
// helper drains one update from each client to keep their channels in lockstep
// — otherwise the non-mover's buffered update would be misread on its next turn.
func move(t *testing.T, h *Hub, white, black, mover *Client, gameID, from, to string) *Message {
	t.Helper()
	send(h, mover, &Message{Type: "move", GameID: gameID, Move: &MoveDTO{From: from, To: to}})
	wu := waitForMessage(t, white, "game_update")
	bu := waitForMessage(t, black, "game_update")
	if mover == white {
		return wu
	}
	return bu
}

// TestMove_LegalApplicationAndBroadcast plays 1.e4 and checks the game_update
// broadcast to both players: advanced FEN, black to move, the move just played,
// black's 20 opening replies, and an ongoing result.
func TestMove_LegalApplicationAndBroadcast(t *testing.T) {
	h := newHub()
	go h.run()

	c1, c2, gameID := startGame(t, h, "standard")

	send(h, c1, &Message{Type: "move", GameID: gameID, Move: &MoveDTO{From: "e2", To: "e4"}})
	up := waitForMessage(t, c1, "game_update")
	op := waitForMessage(t, c2, "game_update") // the opponent's copy
	if up == nil || op == nil {
		return
	}
	if up.FEN == engine.StartingFEN {
		t.Errorf("fen did not change after a move: %q", up.FEN)
	}
	if up.SideToMove != "black" {
		t.Errorf("sideToMove = %q, want black", up.SideToMove)
	}
	if up.LastMove == nil || up.LastMove.From != "e2" || up.LastMove.To != "e4" {
		t.Errorf("lastMove = %+v, want e2->e4", up.LastMove)
	}
	if len(up.LegalMoves) != 20 {
		t.Errorf("legalMoves = %d, want 20 (black's opening replies)", len(up.LegalMoves))
	}
	if up.Result == nil || up.Result.Outcome != "ongoing" {
		t.Errorf("result = %+v, want ongoing", up.Result)
	}

	// The opponent must receive the identical update.
	if op.FEN != up.FEN {
		t.Errorf("opponent fen %q != mover fen %q", op.FEN, up.FEN)
	}
}

// TestMove_OutOfTurn rejects a move from the player who is not on turn, with an
// error to that player only.
func TestMove_OutOfTurn(t *testing.T) {
	h := newHub()
	go h.run()

	_, c2, gameID := startGame(t, h, "standard")

	// Black tries to move first; it is White's turn.
	send(h, c2, &Message{Type: "move", GameID: gameID, Move: &MoveDTO{From: "e7", To: "e5"}})
	if waitForMessage(t, c2, "error") == nil {
		return
	}
}

// TestMove_Illegal rejects a move that is not legal in the position, with an
// error to the sender.
func TestMove_Illegal(t *testing.T) {
	h := newHub()
	go h.run()

	c1, _, gameID := startGame(t, h, "standard")

	// e2-e5 is not a legal pawn move from the start position.
	send(h, c1, &Message{Type: "move", GameID: gameID, Move: &MoveDTO{From: "e2", To: "e5"}})
	if waitForMessage(t, c1, "error") == nil {
		return
	}
}

// TestMove_NotInGame rejects a move from a connected user who is not in a game.
func TestMove_NotInGame(t *testing.T) {
	h := newHub()
	go h.run()

	c, _ := connectClient(t, h)
	send(h, c, &Message{Type: "move", Move: &MoveDTO{From: "e2", To: "e4"}})
	if waitForMessage(t, c, "error") == nil {
		return
	}
}

// TestGameEnd_Checkmate plays fool's mate and checks the final game_update
// declares Black the winner by checkmate and that both players are freed.
func TestGameEnd_Checkmate(t *testing.T) {
	h := newHub()
	go h.run()

	c1, c2, gameID := startGame(t, h, "standard")

	move(t, h, c1, c2, c1, gameID, "f2", "f3")          // 1. f3
	move(t, h, c1, c2, c2, gameID, "e7", "e5")          // 1... e5
	move(t, h, c1, c2, c1, gameID, "g2", "g4")          // 2. g4
	final := move(t, h, c1, c2, c2, gameID, "d8", "h4") // 2... Qh4#
	if final == nil {
		return
	}
	if final.Result == nil || final.Result.Outcome != "black_wins" || final.Result.Reason != "checkmate" {
		t.Errorf("result = %+v, want black_wins/checkmate", final.Result)
	}

	// Both players should be freed (no in-game users on the roster).
	waitForInGameCount(t, c1, 0)
}

// TestGameEnd_Stalemate plays Sam Loyd's 10-move stalemate and checks the final
// game_update declares a draw by stalemate.
func TestGameEnd_Stalemate(t *testing.T) {
	h := newHub()
	go h.run()

	c1, c2, gameID := startGame(t, h, "standard")

	// White plies on even indices (c1), Black plies on odd indices (c2).
	plies := [][2]string{
		{"e2", "e3"}, {"a7", "a5"},
		{"d1", "h5"}, {"a8", "a6"},
		{"h5", "a5"}, {"h7", "h5"},
		{"a5", "c7"}, {"a6", "h6"},
		{"h2", "h4"}, {"f7", "f6"},
		{"c7", "d7"}, {"e8", "f7"},
		{"d7", "b7"}, {"d8", "d3"},
		{"b7", "b8"}, {"d3", "h7"},
		{"b8", "c8"}, {"f7", "g6"},
		{"c8", "e6"},
	}

	var last *Message
	for i, p := range plies {
		mover := c1
		if i%2 == 1 {
			mover = c2
		}
		last = move(t, h, c1, c2, mover, gameID, p[0], p[1])
		if last == nil {
			return
		}
	}
	if last.Result == nil || last.Result.Outcome != "draw" || last.Result.Reason != "stalemate" {
		t.Errorf("result = %+v, want draw/stalemate", last.Result)
	}
}

// TestGameEnd_Resign ends a game by resignation and checks both players receive
// a game_update awarding the win to the opponent.
func TestGameEnd_Resign(t *testing.T) {
	h := newHub()
	go h.run()

	c1, c2, gameID := startGame(t, h, "standard")

	send(h, c1, &Message{Type: "resign", GameID: gameID})

	w := waitForMessage(t, c1, "game_update")
	b := waitForMessage(t, c2, "game_update")
	if w == nil || b == nil {
		return
	}
	for _, m := range []*Message{w, b} {
		if m.Result == nil || m.Result.Outcome != "black_wins" || m.Result.Reason != "resignation" {
			t.Errorf("result = %+v, want black_wins/resignation", m.Result)
		}
	}
	waitForInGameCount(t, c1, 0)
}

// TestGameEnd_Timeout lets White's turn clock run out and checks the game ends
// in Black's favor by timeout.
func TestGameEnd_Timeout(t *testing.T) {
	h := newHub()
	// Tiny move timeout so White's first-turn clock expires almost immediately.
	// Set before run() so the goroutine reads it without a race.
	h.moveTimeout = 20 * time.Millisecond
	go h.run()

	c1, c2, _ := startGame(t, h, "standard")

	w := waitForMessage(t, c1, "game_update")
	b := waitForMessage(t, c2, "game_update")
	if w == nil || b == nil {
		return
	}
	for _, m := range []*Message{w, b} {
		if m.Result == nil || m.Result.Outcome != "black_wins" || m.Result.Reason != "timeout" {
			t.Errorf("result = %+v, want black_wins/timeout", m.Result)
		}
	}
}

// TestGameEnd_Disconnect tears down a game when a player disconnects: the
// opponent is notified and awarded the win.
func TestGameEnd_Disconnect(t *testing.T) {
	h := newHub()
	go h.run()

	c1, c2, _ := startGame(t, h, "standard")

	h.unregister <- c1 // White leaves, so Black (the opponent) wins.

	gone := waitForMessage(t, c2, "opponent_disconnected")
	if gone == nil {
		return
	}
	if gone.Result == nil || gone.Result.Outcome != "black_wins" {
		t.Errorf("disconnect result = %+v, want black_wins", gone.Result)
	}
}

// TestGameEnd_PersistenceHookFires confirms the gameEnded hook (the Task 10
// persistence seam) is invoked exactly once with the finished game and its
// recorded result.
func TestGameEnd_PersistenceHookFires(t *testing.T) {
	h := newHub()
	saved := make(chan *Game, 4)
	h.gameEnded = func(g *Game) { saved <- g }
	go h.run()

	c1, _, gameID := startGame(t, h, "standard")
	send(h, c1, &Message{Type: "resign", GameID: gameID})

	select {
	case g := <-saved:
		if !g.GameOver {
			t.Error("persisted game not marked GameOver")
		}
		if g.Result.Outcome != engine.BlackWins || g.Result.Reason != "resignation" {
			t.Errorf("persisted result = %+v, want black_wins/resignation", g.Result)
		}
		if g.Variant != "standard" {
			t.Errorf("persisted variant = %q, want standard", g.Variant)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gameEnded hook was not called")
	}
}
