package main

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"rainbow-chess/engine"
)

// storage_test.go exercises the SQLite persistence layer: schema/dir init,
// SaveGame + read-back (variant and result recorded correctly), the nil-store
// no-op sink, missing-row handling, and the end-to-end wiring of the hub's
// gameEnded hook to SaveGame.

// openTestStore opens a Store at a fresh temp path and registers cleanup.
func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	// A nested subdir so we also exercise OpenStore's MkdirAll.
	path := filepath.Join(t.TempDir(), "data", "games.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

// TestOpenStore_InitializesSchemaAndDir confirms OpenStore creates the parent
// directory and a usable `games` table (an INSERT/SELECT round-trips).
func TestOpenStore_InitializesSchemaAndDir(t *testing.T) {
	s, path := openTestStore(t)

	if _, err := s.db.Exec(`INSERT INTO games (id) VALUES ('probe')`); err != nil {
		t.Fatalf("games table not usable after OpenStore: %v", err)
	}
	var id string
	if err := s.db.QueryRow(`SELECT id FROM games WHERE id = 'probe'`).Scan(&id); err != nil {
		t.Fatalf("read back probe row: %v", err)
	}
	if id != "probe" {
		t.Errorf("probe id = %q, want probe", id)
	}
	if filepath.Base(path) != "games.db" {
		t.Errorf("db path = %q, want it to end in games.db", path)
	}
}

// TestSaveGame_RoundTrip saves a finished game and reads it back, checking the
// variant, result split into outcome+termination, player names, and the UCI
// move history are all recorded.
func TestSaveGame_RoundTrip(t *testing.T) {
	s, _ := openTestStore(t)

	start := time.Now().Add(-2 * time.Minute).Truncate(time.Second)
	end := start.Add(90 * time.Second)
	g := &Game{
		ID:        "game-rainbow-1",
		Variant:   "rainbow",
		White:     &User{Username: "AmberAardvark42"},
		Black:     &User{Username: "CrimsonCrane07"},
		Result:    engine.GameResult{Outcome: engine.BlackWins, Reason: "checkmate"},
		GameOver:  true,
		StartTime: start,
		EndTime:   end,
		Moves:     []string{"e2e4", "e7e5", "d1h5", "b8c6", "f1c4", "g8f6", "h5f7"},
	}

	<-s.SaveGame(g) // block until the async write completes

	got, err := s.LoadGame("game-rainbow-1")
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if got.Variant != "rainbow" {
		t.Errorf("variant = %q, want rainbow", got.Variant)
	}
	if got.Result != "black_wins" {
		t.Errorf("result = %q, want black_wins", got.Result)
	}
	if got.Termination != "checkmate" {
		t.Errorf("termination = %q, want checkmate", got.Termination)
	}
	if got.WhiteName != "AmberAardvark42" || got.BlackName != "CrimsonCrane07" {
		t.Errorf("names = %q/%q, want AmberAardvark42/CrimsonCrane07", got.WhiteName, got.BlackName)
	}
	if len(got.Moves) != len(g.Moves) {
		t.Fatalf("move count = %d, want %d", len(got.Moves), len(g.Moves))
	}
	for i, mv := range g.Moves {
		if got.Moves[i] != mv {
			t.Errorf("move[%d] = %q, want %q", i, got.Moves[i], mv)
		}
	}
	if !got.StartedAt.Equal(start) {
		t.Errorf("startedAt = %v, want %v", got.StartedAt, start)
	}
	if !got.EndedAt.Equal(end) {
		t.Errorf("endedAt = %v, want %v", got.EndedAt, end)
	}
}

// TestSaveGame_StandardDraw checks a stalemate draw is recorded as draw/stalemate
// for the standard variant (the other variant + outcome branch).
func TestSaveGame_StandardDraw(t *testing.T) {
	s, _ := openTestStore(t)

	g := &Game{
		ID:       "game-std-draw",
		Variant:  "standard",
		White:    &User{Username: "W"},
		Black:    &User{Username: "B"},
		Result:   engine.GameResult{Outcome: engine.Draw, Reason: "stalemate"},
		GameOver: true,
	}
	<-s.SaveGame(g)

	got, err := s.LoadGame("game-std-draw")
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if got.Variant != "standard" || got.Result != "draw" || got.Termination != "stalemate" {
		t.Errorf("got %s/%s/%s, want standard/draw/stalemate", got.Variant, got.Result, got.Termination)
	}
	if len(got.Moves) != 0 {
		t.Errorf("moves = %v, want empty for a no-move game", got.Moves)
	}
}

// TestSaveGame_NilUserName tolerates a player that has already disconnected
// (nil *User) without panicking, recording an empty name.
func TestSaveGame_NilUserName(t *testing.T) {
	s, _ := openTestStore(t)

	g := &Game{
		ID:      "game-nil-player",
		Variant: "standard",
		White:   &User{Username: "Survivor"},
		Black:   nil, // disconnected
		Result:  engine.GameResult{Outcome: engine.WhiteWins, Reason: "opponent disconnected"},
	}
	<-s.SaveGame(g)

	got, err := s.LoadGame("game-nil-player")
	if err != nil {
		t.Fatalf("LoadGame: %v", err)
	}
	if got.WhiteName != "Survivor" || got.BlackName != "" {
		t.Errorf("names = %q/%q, want Survivor/empty", got.WhiteName, got.BlackName)
	}
	if got.Result != "white_wins" || got.Termination != "opponent disconnected" {
		t.Errorf("got %s/%s, want white_wins/opponent disconnected", got.Result, got.Termination)
	}
}

// TestLoadGame_Missing returns sql.ErrNoRows for an unknown game id.
func TestLoadGame_Missing(t *testing.T) {
	s, _ := openTestStore(t)
	if _, err := s.LoadGame("does-not-exist"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestSaveGame_NilStoreIsNoOp confirms a nil Store is a safe no-op sink: the
// done channel closes and nothing panics. This is what lets a hub run without
// persistence (e.g. in the other package tests).
func TestSaveGame_NilStoreIsNoOp(t *testing.T) {
	var s *Store
	select {
	case <-s.SaveGame(&Game{ID: "x"}):
		// closed immediately, as expected
	case <-time.After(time.Second):
		t.Fatal("nil-store SaveGame did not close its done channel")
	}
}

// TestStore_WiredToHubPersistsFinishedGame is the integration test for the Task
// 10 wiring: a Store hooked into Hub.gameEnded persists a real game played to a
// resignation, recorded with the correct variant and result.
func TestStore_WiredToHubPersistsFinishedGame(t *testing.T) {
	s, _ := openTestStore(t)

	h := newHub()
	persisted := make(chan string, 4)
	// Mirror main.go's wiring, but signal the test once the async write lands.
	h.gameEnded = func(g *Game) {
		done := s.SaveGame(g)
		id := g.ID
		go func() { <-done; persisted <- id }()
	}
	go h.run()

	c1, _, gameID := startGame(t, h, "standard")
	send(h, c1, &Message{Type: "resign", GameID: gameID})

	select {
	case id := <-persisted:
		got, err := s.LoadGame(id)
		if err != nil {
			t.Fatalf("LoadGame(%q): %v", id, err)
		}
		if got.Variant != "standard" {
			t.Errorf("variant = %q, want standard", got.Variant)
		}
		if got.Result != "black_wins" || got.Termination != "resignation" {
			t.Errorf("got %s/%s, want black_wins/resignation", got.Result, got.Termination)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("game was not persisted via the gameEnded hook")
	}
}
