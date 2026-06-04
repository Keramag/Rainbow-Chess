package main

import (
	"encoding/json"
	"testing"
	"time"

	"rainbow-chess/engine"
)

// connectClient registers a fresh client and returns it along with the userID
// the hub assigned (read from the welcome message). Used to set up the multiple
// participants a challenge flow needs.
func connectClient(t *testing.T, h *Hub) (*Client, string) {
	t.Helper()
	c := newTestClient(h)
	h.register <- c
	w := waitForMessage(t, c, "welcome")
	if w == nil {
		return c, ""
	}
	return c, w.UserID
}

// send pushes a message from a client into the hub as if it had arrived over the
// wire.
func send(h *Hub, c *Client, msg *Message) {
	h.handleMessage <- &MessageWrapper{client: c, message: msg}
}

// TestChallengeLifecycle_AcceptCreatesGame walks the happy path: a challenge is
// created, the target is notified, and accepting it spins up a game with the
// correct variant, colors, initial FEN, and legal-move list for both players.
// Colour assignment is randomised in production, so this test pins the coin flip
// to its default (challenger = White) to make the per-colour assertions concrete;
// TestChallengeAccept_RandomizesColor covers the swapped assignment.
func TestChallengeLifecycle_AcceptCreatesGame(t *testing.T) {
	h := newHub()
	h.coinFlip = func() bool { return false } // deterministic: challenger plays White
	go h.run()

	c1, id1 := connectClient(t, h)
	c2, id2 := connectClient(t, h)

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "standard"})

	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return
	}
	if recv.FromUserID != id1 {
		t.Errorf("challenge_received fromUserId = %q, want %q", recv.FromUserID, id1)
	}
	if recv.FromUsername == "" {
		t.Error("challenge_received missing fromUsername")
	}
	if recv.Variant != "standard" {
		t.Errorf("challenge_received variant = %q, want standard", recv.Variant)
	}
	if recv.ChallengeID == "" {
		t.Fatal("challenge_received missing challengeId")
	}

	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})

	white := waitForMessage(t, c1, "game_start")
	black := waitForMessage(t, c2, "game_start")
	if white == nil || black == nil {
		return
	}

	if white.Color != "white" {
		t.Errorf("challenger color = %q, want white", white.Color)
	}
	if black.Color != "black" {
		t.Errorf("acceptor color = %q, want black", black.Color)
	}
	if white.GameID == "" || white.GameID != black.GameID {
		t.Errorf("game ids differ or empty: %q vs %q", white.GameID, black.GameID)
	}
	if white.Variant != "standard" || black.Variant != "standard" {
		t.Errorf("game_start variant = %q/%q, want standard", white.Variant, black.Variant)
	}
	if white.FEN != engine.StartingFEN {
		t.Errorf("game_start fen = %q, want %q", white.FEN, engine.StartingFEN)
	}
	if black.FEN != engine.StartingFEN {
		t.Errorf("acceptor game_start fen = %q, want starting FEN", black.FEN)
	}
	// 20 legal moves in the standard opening position; both players receive the
	// side-to-move's (white's) moves so the client can highlight on its turn.
	if len(white.LegalMoves) != 20 {
		t.Errorf("white legalMoves = %d, want 20", len(white.LegalMoves))
	}
	if len(black.LegalMoves) != 20 {
		t.Errorf("black legalMoves = %d, want 20", len(black.LegalMoves))
	}

	// Both players should now show as in-game on the roster.
	roster := waitForInGameCount(t, c1, 2)
	if roster != nil {
		for _, u := range roster.Users {
			if !u.InGame {
				t.Errorf("user %q should be in-game after game_start", u.Username)
			}
		}
	}
}

// TestChallengeAccept_RandomizesColor confirms colour assignment honours the coin
// flip: when it returns true the colours swap, so the acceptor plays White and
// the challenger plays Black. The default path (challenger White) is covered by
// TestChallengeLifecycle_AcceptCreatesGame.
func TestChallengeAccept_RandomizesColor(t *testing.T) {
	h := newHub()
	h.coinFlip = func() bool { return true } // force a swap: acceptor plays White
	go h.run()

	c1, _ := connectClient(t, h)   // challenger
	c2, id2 := connectClient(t, h) // acceptor

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "standard"})
	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return
	}
	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})

	challengerStart := waitForMessage(t, c1, "game_start")
	acceptorStart := waitForMessage(t, c2, "game_start")
	if challengerStart == nil || acceptorStart == nil {
		return
	}
	if challengerStart.Color != "black" {
		t.Errorf("challenger color = %q, want black (colours swapped)", challengerStart.Color)
	}
	if acceptorStart.Color != "white" {
		t.Errorf("acceptor color = %q, want white (colours swapped)", acceptorStart.Color)
	}
	// Each player's opponent name is the other player, regardless of colour.
	if acceptorStart.OpponentName == "" || challengerStart.OpponentName == "" {
		t.Error("game_start missing opponent name")
	}
}

// TestChallengeLifecycle_RainbowVariant confirms a Rainbow challenge produces a
// game tagged with the rainbow variant and a non-empty position/legal-move list
// (the exact FEN is randomized per game, so we only assert it is well-formed).
func TestChallengeLifecycle_RainbowVariant(t *testing.T) {
	h := newHub()
	go h.run()

	c1, _ := connectClient(t, h)
	c2, id2 := connectClient(t, h)

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "rainbow"})
	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return
	}
	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})

	start := waitForMessage(t, c1, "game_start")
	if start == nil {
		return
	}
	if start.Variant != "rainbow" {
		t.Errorf("game_start variant = %q, want rainbow", start.Variant)
	}
	if _, err := engine.ParseFEN(start.FEN); err != nil {
		t.Errorf("rainbow game_start fen %q is not parseable: %v", start.FEN, err)
	}
	if len(start.LegalMoves) == 0 {
		t.Error("rainbow game_start has no legal moves")
	}
}

// TestChallengeAccept_GameStartCarriesOpponentName asserts each game_start names
// the other player: the challenger sees the acceptor, and vice versa. The client
// renders the in-game header from this rather than from a remembered name.
func TestChallengeAccept_GameStartCarriesOpponentName(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := newTestClient(h)
	h.register <- c1
	w1 := waitForMessage(t, c1, "welcome")
	c2 := newTestClient(h)
	h.register <- c2
	w2 := waitForMessage(t, c2, "welcome")
	if w1 == nil || w2 == nil {
		return
	}

	send(h, c1, &Message{Type: "challenge", TargetUserID: w2.UserID, Variant: "standard"})
	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return
	}
	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})

	white := waitForMessage(t, c1, "game_start")
	black := waitForMessage(t, c2, "game_start")
	if white == nil || black == nil {
		return
	}
	if white.OpponentName != w2.Username {
		t.Errorf("challenger game_start opponentName = %q, want %q (the acceptor)", white.OpponentName, w2.Username)
	}
	if black.OpponentName != w1.Username {
		t.Errorf("acceptor game_start opponentName = %q, want %q (the challenger)", black.OpponentName, w1.Username)
	}
}

// TestChallengeAccept_OpponentNameWithMultipleOutgoing reproduces the case the
// client used to get wrong: one user issues challenges to two different targets
// (the server permits this — it only blocks duplicate challenges to the *same*
// target), and whichever accepts first must be named as the opponent. Because
// game_start now carries the opponent name from the server, the first acceptor is
// reported correctly regardless of which challenge was issued last.
func TestChallengeAccept_OpponentNameWithMultipleOutgoing(t *testing.T) {
	h := newHub()
	go h.run()

	c1 := newTestClient(h) // challenger
	h.register <- c1
	w1 := waitForMessage(t, c1, "welcome")
	cA := newTestClient(h) // target A
	h.register <- cA
	wA := waitForMessage(t, cA, "welcome")
	cB := newTestClient(h) // target B
	h.register <- cB
	wB := waitForMessage(t, cB, "welcome")
	if w1 == nil || wA == nil || wB == nil {
		return
	}

	// c1 challenges A, then B; both are pending simultaneously.
	send(h, c1, &Message{Type: "challenge", TargetUserID: wA.UserID, Variant: "standard"})
	recvA := waitForMessage(t, cA, "challenge_received")
	send(h, c1, &Message{Type: "challenge", TargetUserID: wB.UserID, Variant: "standard"})
	recvB := waitForMessage(t, cB, "challenge_received")
	if recvA == nil || recvB == nil {
		return
	}

	// A (the earlier-issued challenge) accepts first. The challenger's game_start
	// must name A, even though B was challenged most recently.
	send(h, cA, &Message{Type: "accept_challenge", ChallengeID: recvA.ChallengeID})
	white := waitForMessage(t, c1, "game_start")
	black := waitForMessage(t, cA, "game_start")
	if white == nil || black == nil {
		return
	}
	if white.OpponentName != wA.Username {
		t.Errorf("challenger game_start opponentName = %q, want %q (A, who accepted)", white.OpponentName, wA.Username)
	}
	if black.OpponentName != w1.Username {
		t.Errorf("acceptor game_start opponentName = %q, want challenger %q", black.OpponentName, w1.Username)
	}
}

// TestChallengeExpiry verifies a challenge left unanswered past the TTL expires
// and both parties are notified.
func TestChallengeExpiry(t *testing.T) {
	h := newHub()
	// Tiny TTL and sweep interval so the test does not wait seconds. Set before
	// run() so the goroutine reads them without a race.
	h.challengeTTL = 20 * time.Millisecond
	h.expiryInterval = 5 * time.Millisecond
	go h.run()

	c1, _ := connectClient(t, h)
	c2, id2 := connectClient(t, h)

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "standard"})
	if waitForMessage(t, c2, "challenge_received") == nil {
		return
	}

	if waitForMessage(t, c1, "challenge_expired") == nil {
		return
	}
	if waitForMessage(t, c2, "challenge_expired") == nil {
		return
	}
}

// TestChallengeDecline verifies declining a challenge notifies the challenger and
// removes the pending challenge (a subsequent accept finds nothing).
func TestChallengeDecline(t *testing.T) {
	h := newHub()
	go h.run()

	c1, _ := connectClient(t, h)
	c2, id2 := connectClient(t, h)

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "standard"})
	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return
	}

	send(h, c2, &Message{Type: "decline_challenge", ChallengeID: recv.ChallengeID})
	declined := waitForMessage(t, c1, "challenge_declined")
	if declined == nil {
		return
	}
	if declined.ChallengeID != recv.ChallengeID {
		t.Errorf("challenge_declined id = %q, want %q", declined.ChallengeID, recv.ChallengeID)
	}

	// Accepting the now-declined challenge must fail rather than start a game.
	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})
	if waitForMessage(t, c2, "error") == nil {
		return
	}
}

// TestChallengeInvalid_Self rejects a user challenging themselves.
func TestChallengeInvalid_Self(t *testing.T) {
	h := newHub()
	go h.run()

	c1, id1 := connectClient(t, h)
	send(h, c1, &Message{Type: "challenge", TargetUserID: id1, Variant: "standard"})

	if waitForMessage(t, c1, "error") == nil {
		return
	}
}

// TestChallengeInvalid_Offline rejects a challenge to a userID that is not online.
func TestChallengeInvalid_Offline(t *testing.T) {
	h := newHub()
	go h.run()

	c1, _ := connectClient(t, h)
	send(h, c1, &Message{Type: "challenge", TargetUserID: "nobody-here", Variant: "standard"})

	if waitForMessage(t, c1, "error") == nil {
		return
	}
}

// TestChallengeInvalid_UnknownVariant rejects a challenge naming a variant that
// is not registered.
func TestChallengeInvalid_UnknownVariant(t *testing.T) {
	h := newHub()
	go h.run()

	c1, _ := connectClient(t, h)
	_, id2 := connectClient(t, h)

	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "definitely-not-a-variant"})
	if waitForMessage(t, c1, "error") == nil {
		return
	}
}

// TestChallengeInvalid_Busy rejects a challenge to a user already in a game.
func TestChallengeInvalid_Busy(t *testing.T) {
	h := newHub()
	go h.run()

	c1, id1 := connectClient(t, h)
	c2, id2 := connectClient(t, h)
	c3, _ := connectClient(t, h)

	// c1 and c2 start a game so both become busy.
	send(h, c1, &Message{Type: "challenge", TargetUserID: id2, Variant: "standard"})
	recv := waitForMessage(t, c2, "challenge_received")
	if recv == nil {
		return
	}
	send(h, c2, &Message{Type: "accept_challenge", ChallengeID: recv.ChallengeID})
	if waitForMessage(t, c1, "game_start") == nil {
		return
	}

	// c3 challenges the now-busy c1 and should get an error, not a challenge.
	send(h, c3, &Message{Type: "challenge", TargetUserID: id1, Variant: "standard"})
	if waitForMessage(t, c3, "error") == nil {
		return
	}
}

// waitForInGameCount reads from c.send until it sees a users_update with exactly
// n in-game users, or times out.
func waitForInGameCount(t *testing.T, c *Client, n int) *Message {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case b := <-c.send:
			var m Message
			if err := json.Unmarshal(b, &m); err != nil {
				continue
			}
			if m.Type != "users_update" {
				continue
			}
			count := 0
			for _, u := range m.Users {
				if u.InGame {
					count++
				}
			}
			if count == n {
				return &m
			}
		case <-timeout:
			t.Fatalf("timeout waiting for users_update with %d in-game users", n)
			return nil
		}
	}
}
