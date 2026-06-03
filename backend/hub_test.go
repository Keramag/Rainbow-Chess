package main

import (
	"encoding/json"
	"testing"
	"time"

	"rainbow-chess/engine"
)

// waitForMessage reads from c.send until it sees a message of the given type or
// times out. Returns the decoded message (nil on timeout).
func waitForMessage(t *testing.T, c *Client, msgType string) *Message {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case b := <-c.send:
			var m Message
			if err := json.Unmarshal(b, &m); err != nil {
				continue
			}
			if m.Type == msgType {
				return &m
			}
		case <-timeout:
			t.Fatalf("timeout waiting for message type %q", msgType)
			return nil
		}
	}
}

// waitForUserCount reads from c.send until it sees a users_update listing exactly
// n users or times out. Connect/disconnect emit several users_update messages as
// the roster changes, so tests assert on the specific count they expect.
func waitForUserCount(t *testing.T, c *Client, n int) *Message {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case b := <-c.send:
			var m Message
			if err := json.Unmarshal(b, &m); err != nil {
				continue
			}
			if m.Type == "users_update" && len(m.Users) == n {
				return &m
			}
		case <-timeout:
			t.Fatalf("timeout waiting for users_update with %d users", n)
			return nil
		}
	}
}

func newTestClient(h *Hub) *Client {
	return &Client{hub: h, send: make(chan []byte, 256)}
}

func TestHubConnect_WelcomeAndUsername(t *testing.T) {
	h := newHub()
	go h.run()

	c := newTestClient(h)
	h.register <- c

	msg := waitForMessage(t, c, "welcome")
	if msg == nil {
		return
	}
	if msg.UserID == "" {
		t.Error("welcome message missing userId")
	}
	if msg.Username == "" {
		t.Error("welcome message missing username")
	}
	// The username assigned in the welcome must follow the anonymous format.
	if !nameRe.MatchString(msg.Username) {
		t.Errorf("welcome username %q does not match Adjective+Animal+NN", msg.Username)
	}
}

func TestHubConnect_WelcomeContainsBothVariants(t *testing.T) {
	h := newHub()
	go h.run()

	c := newTestClient(h)
	h.register <- c

	msg := waitForMessage(t, c, "welcome")
	if msg == nil {
		return
	}

	want := map[string]bool{"standard": false, "rainbow": false}
	for _, v := range msg.Variants {
		if _, ok := want[v]; ok {
			want[v] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("welcome variants missing %q; got %v", name, msg.Variants)
		}
	}
}

func TestHubUsersUpdate_ConnectAndDisconnect(t *testing.T) {
	h := newHub()
	go h.run()

	// First client connects: roster of one.
	c1 := newTestClient(h)
	h.register <- c1
	waitForMessage(t, c1, "welcome")
	waitForUserCount(t, c1, 1)

	// Second client connects: c1 should see the roster grow to two.
	c2 := newTestClient(h)
	h.register <- c2
	waitForMessage(t, c2, "welcome")
	two := waitForUserCount(t, c1, 2)
	if two == nil {
		return
	}

	// Both users should be present, each with a distinct id and not in a game.
	ids := map[string]bool{}
	for _, u := range two.Users {
		if u.UserID == "" || u.Username == "" {
			t.Errorf("user info missing id/username: %+v", u)
		}
		if u.InGame {
			t.Errorf("freshly-connected user %q should not be in a game", u.Username)
		}
		ids[u.UserID] = true
	}
	if len(ids) != 2 {
		t.Errorf("expected 2 distinct user ids, got %d", len(ids))
	}

	// Second client disconnects: c1 should see the roster shrink back to one.
	h.unregister <- c2
	waitForUserCount(t, c1, 1)
}

// TestEvictClient_FullBufferTearsDownUserAndGame verifies that a client whose
// send buffer is full is not merely dropped from the clients map but fully torn
// down: its user is removed, its in-progress game ends in the opponent's favor,
// the opponent is freed, and its send channel is closed. Before the fix, a
// buffer-full client was deleted from h.clients without running handleDisconnect,
// so it leaked as a ghost user with an orphaned game (the readPump's later
// unregister no-ops once the client is gone from the map). This drives the
// teardown synchronously (no run goroutine) so the assertions are deterministic.
func TestEvictClient_FullBufferTearsDownUserAndGame(t *testing.T) {
	h := newHub()

	v, err := engine.Get("standard")
	if err != nil {
		t.Fatalf("engine.Get(standard): %v", err)
	}

	// The stuck client gets a 1-slot send buffer we pre-fill so the next send
	// overflows; the opponent reads normally with room to spare.
	stuck := &Client{hub: h, send: make(chan []byte, 1)}
	stuckUser := &User{ID: "stuck", Username: "Stuck", Client: stuck, InGame: true, GameID: "g1"}
	stuck.user = stuckUser

	opp := &Client{hub: h, send: make(chan []byte, 8)}
	oppUser := &User{ID: "opp", Username: "Opp", Client: opp, InGame: true, GameID: "g1"}
	opp.user = oppUser

	h.clients[stuck] = true
	h.clients[opp] = true
	h.users["stuck"] = stuckUser
	h.users["opp"] = oppUser
	h.games["g1"] = &Game{
		ID:       "g1",
		Variant:  "standard",
		White:    stuckUser,
		Black:    oppUser,
		Position: v.InitialPosition(),
	}

	stuck.send <- []byte("filler") // buffer (size 1) now full

	// Any send to the stuck client now overflows and must trigger full teardown.
	h.sendToClient(stuck, &Message{Type: "users_update"})

	if _, ok := h.clients[stuck]; ok {
		t.Error("evicted client still present in h.clients")
	}
	if _, ok := h.users["stuck"]; ok {
		t.Error("evicted client's user still present in h.users (ghost user leak)")
	}
	if _, ok := h.games["g1"]; ok {
		t.Error("game was not ended/removed after a player was evicted")
	}
	if oppUser.InGame || oppUser.GameID != "" {
		t.Errorf("opponent was not freed: InGame=%v GameID=%q", oppUser.InGame, oppUser.GameID)
	}

	// The opponent should have been told its opponent disconnected.
	if !sawMessageType(opp, "opponent_disconnected") {
		t.Error("opponent did not receive opponent_disconnected")
	}

	// The evicted client's send channel must be closed (drain the filler first).
	<-stuck.send
	if _, ok := <-stuck.send; ok {
		t.Error("evicted client's send channel was not closed")
	}
}

// sawMessageType drains everything currently buffered on a test client's send
// channel and reports whether any message had the given type.
func sawMessageType(c *Client, msgType string) bool {
	for {
		select {
		case b := <-c.send:
			var m Message
			if err := json.Unmarshal(b, &m); err == nil && m.Type == msgType {
				return true
			}
		default:
			return false
		}
	}
}

func TestHubDisconnect_UnknownClientIsSafe(t *testing.T) {
	h := newHub()
	go h.run()

	// Unregistering a client that was never registered must not panic or hang;
	// the run loop guards on membership in h.clients. A subsequent real connect
	// still works, proving the loop survived.
	stray := newTestClient(h)
	h.unregister <- stray

	c := newTestClient(h)
	h.register <- c
	waitForMessage(t, c, "welcome")
}
