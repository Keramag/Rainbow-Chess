package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// storage.go is the SQLite persistence layer: every finished game is recorded so
// it can be reviewed later. It is adapted from virusgame's storage.go but trimmed
// to chess — there is no PGN/turn reconstruction here. The move list travels as
// space-separated UCI strings (engine.Move.String()), which is sufficient to
// replay the game, and the outcome is split into a high-level `result`
// (white_wins/black_wins/draw) and a `termination` reason (checkmate, stalemate,
// resignation, timeout, opponent disconnected).
//
// The Store is decoupled from the hub via the Hub.gameEnded hook (wired in
// main.go): the hub stays free of database concerns and tests can run a hub with
// no Store (a nil *Store is a no-op sink).

// gamesSchema is the single-table schema. `moves` is the UCI move history;
// `result` is the wire outcome string; `termination` is the human-readable
// reason the game ended.
const gamesSchema = `
CREATE TABLE IF NOT EXISTS games (
	id          TEXT PRIMARY KEY,
	started_at  DATETIME,
	ended_at    DATETIME,
	variant     TEXT,
	white_name  TEXT,
	black_name  TEXT,
	result      TEXT,
	termination TEXT,
	moves       TEXT
);`

// Store wraps a SQLite database holding completed games. A nil *Store (or one
// with a nil db) is a valid no-op sink: SaveGame closes its done channel
// immediately and persists nothing, so the hub can run without a database.
type Store struct {
	db *sql.DB
}

// OpenStore opens (creating if needed) the SQLite database at dbPath and ensures
// the games schema exists. The parent directory is created if missing.
func OpenStore(dbPath string) (*Store, error) {
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db %q: %w", dbPath, err)
	}
	if _, err := db.Exec(gamesSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database handle. Safe to call on a nil Store.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveGame persists a finished game. Every field is captured synchronously so
// the caller (the hub goroutine) may keep mutating or discard the *Game the
// moment SaveGame returns; the actual INSERT runs on its own goroutine and never
// blocks the hub. The returned channel closes once the write has completed (or
// been skipped), which tests use to await persistence.
func (s *Store) SaveGame(game *Game) <-chan struct{} {
	done := make(chan struct{})
	if s == nil || s.db == nil {
		close(done)
		return done
	}

	// Snapshot all fields up front to avoid racing the hub, which may reuse the
	// *Game after this returns.
	id := game.ID
	startedAt := game.StartTime
	endedAt := game.EndTime
	variant := game.Variant
	whiteName := userName(game.White)
	blackName := userName(game.Black)
	result := outcomeString(game.Result.Outcome)
	termination := game.Result.Reason
	moves := strings.Join(game.Moves, " ")

	go func() {
		defer close(done)
		_, err := s.db.Exec(
			`INSERT OR REPLACE INTO games
			   (id, started_at, ended_at, variant, white_name, black_name, result, termination, moves)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, startedAt, endedAt, variant, whiteName, blackName, result, termination, moves,
		)
		if err != nil {
			log.Printf("storage: save game %s: %v", id, err)
			return
		}
		log.Printf("storage: saved game %s (%s, %s/%s)", id, variant, result, termination)
	}()
	return done
}

// SavedGame is a finished game as recorded in the database — the read-back form
// of SaveGame.
type SavedGame struct {
	ID          string
	StartedAt   time.Time
	EndedAt     time.Time
	Variant     string
	WhiteName   string
	BlackName   string
	Result      string   // wire outcome: white_wins / black_wins / draw / ongoing
	Termination string   // reason the game ended
	Moves       []string // UCI move history
}

// LoadGame reads a previously saved game by ID. It returns sql.ErrNoRows if no
// such game exists.
func (s *Store) LoadGame(id string) (*SavedGame, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store not initialized")
	}

	var (
		g     SavedGame
		moves string
	)
	err := s.db.QueryRow(
		`SELECT id, started_at, ended_at, variant, white_name, black_name, result, termination, moves
		   FROM games WHERE id = ?`, id,
	).Scan(&g.ID, &g.StartedAt, &g.EndedAt, &g.Variant, &g.WhiteName, &g.BlackName, &g.Result, &g.Termination, &moves)
	if err != nil {
		return nil, err
	}
	if moves != "" {
		g.Moves = strings.Split(moves, " ")
	}
	return &g, nil
}

// userName returns a connected user's display name, tolerating a nil user (a
// game whose player has already disconnected) so persistence never panics.
func userName(u *User) string {
	if u == nil {
		return ""
	}
	return u.Username
}

// databasePath returns where the games database lives. DB_PATH overrides it; in
// the container image (detected by /app/index.html, as in main.go's static-dir
// logic) it lands under the mounted /app/backend/data volume; otherwise it is
// relative to the backend working directory.
func databasePath() string {
	if p := os.Getenv("DB_PATH"); p != "" {
		return p
	}
	if _, err := os.Stat("/app/index.html"); err == nil {
		return "/app/backend/data/games.db"
	}
	return "data/games.db"
}
