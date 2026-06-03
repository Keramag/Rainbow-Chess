package engine

import (
	"fmt"
	"sort"
	"sync"
)

// variant.go defines the pluggable rules abstraction that the whole platform is
// built around. A Variant bundles everything the server needs to run a game
// under a particular rule set: how the board starts, which moves are legal, how
// a move changes the position, when the game is over, and which pieces a pawn
// may promote to. Standard chess (standard.go) is the base implementation;
// named variants embed Standard and override only the methods whose rules they
// change, so a new rule experiment costs only the lines that differ.
//
// The registry lets variants advertise themselves at package-init time. The
// server reads List() to build the challenge picker and Get(name) to create a
// game, so adding a variant never touches the transport layer.
type Variant interface {
	// Name is the variant's stable identifier (also its registry key).
	Name() string
	// InitialPosition returns a fresh starting position for a new game.
	InitialPosition() *Position
	// LegalMoves returns every legal move for the side to move in pos.
	LegalMoves(pos *Position) []Move
	// ApplyMove validates move under this variant's rules and returns the
	// resulting new Position, or an error if the move is not allowed.
	ApplyMove(pos *Position, move Move) (*Position, error)
	// Result reports the high-level outcome of pos (ongoing / win / draw).
	Result(pos *Position) GameResult
	// PromotionPieces lists the piece types a pawn may promote to, in the
	// order a UI should offer them.
	PromotionPieces() []PieceType
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Variant{}
)

// Register adds v to the global variant registry under name. It is meant to be
// called from a variant file's init(), so it panics on a programming error — an
// empty name or a duplicate registration — to surface the mistake at startup
// rather than letting a silently-missing variant fail later.
func Register(name string, v Variant) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if name == "" {
		panic("engine: Register called with empty variant name")
	}
	if v == nil {
		panic(fmt.Sprintf("engine: Register %q with nil variant", name))
	}
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("engine: variant %q already registered", name))
	}
	registry[name] = v
}

// Get returns the variant registered under name, or an error if no variant has
// that name.
func Get(name string) (Variant, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	v, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown variant %q", name)
	}
	return v, nil
}

// List returns the names of all registered variants, sorted alphabetically so
// the order is stable across runs (the frontend picker and tests rely on this).
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
