package engine

import "fmt"

// Standard is full legal chess: the base Variant on which every other variant
// builds. It is a thin adapter — LegalMoves / ApplyMove / Result delegate to the
// engine's package-level rules functions — plus the two pieces of per-variant
// configuration that subclasses customise: the variant name and the set of
// pieces a pawn may promote to.
//
// Those two knobs are plain fields rather than hard-coded returns on purpose.
// Go's embedding promotes methods but does not do virtual dispatch: when Rainbow
// embeds Standard and a caller invokes the inherited ApplyMove, that method runs
// with the embedded *Standard as its receiver, so it can only see the embedded
// Standard's state — not any method Rainbow might override. By reading the name
// and promotion list from fields, a variant configures Standard once in its
// constructor and correctly inherits Name, PromotionPieces and the
// promotion-restricting ApplyMove without re-implementing them.
type Standard struct {
	name       string
	promotions []PieceType
}

// NewStandard returns the Standard variant: name "standard", promotions to
// queen, rook, bishop or knight.
func NewStandard() *Standard {
	return &Standard{
		name:       "standard",
		promotions: []PieceType{Queen, Rook, Bishop, Knight},
	}
}

func init() { Register("standard", NewStandard()) }

// Name returns the variant's registry identifier.
func (s *Standard) Name() string { return s.name }

// InitialPosition returns the standard chess starting position. The starting
// FEN is a package constant known to be valid, so a parse error here would be a
// programming error and is treated as unrecoverable.
func (s *Standard) InitialPosition() *Position {
	pos, err := ParseFEN(StartingFEN)
	if err != nil {
		panic(fmt.Sprintf("engine: invalid StartingFEN: %v", err))
	}
	return pos
}

// LegalMoves returns every legal move for the side to move.
func (s *Standard) LegalMoves(pos *Position) []Move { return LegalMoves(pos) }

// ApplyMove validates move under this variant's rules and returns the resulting
// position. Beyond the engine's legality check it enforces the variant's
// promotion whitelist: a promotion to a piece this variant does not allow is
// rejected before the move is applied. This is the single point at which Rainbow
// (which permits only knight/bishop) restricts promotions, inherited unchanged.
func (s *Standard) ApplyMove(pos *Position, move Move) (*Position, error) {
	if move.Promotion != None && !s.allowsPromotion(move.Promotion) {
		return nil, fmt.Errorf("variant %q does not allow promotion to %s", s.name, move.Promotion)
	}
	return ApplyMove(pos, move)
}

// Result reports the high-level outcome of pos.
func (s *Standard) Result(pos *Position) GameResult { return Result(pos) }

// PromotionPieces lists the piece types a pawn may promote to.
func (s *Standard) PromotionPieces() []PieceType { return s.promotions }

// allowsPromotion reports whether t is in this variant's promotion whitelist.
func (s *Standard) allowsPromotion(t PieceType) bool {
	for _, p := range s.promotions {
		if p == t {
			return true
		}
	}
	return false
}
