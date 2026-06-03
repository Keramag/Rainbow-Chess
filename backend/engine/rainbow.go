package engine

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Rainbow is the colour-mixed variant and the first real proof that the Variant
// abstraction earns its keep: it reuses Standard's entire rule set and changes
// only two things — how the board is coloured at the start, and which pieces a
// pawn may promote to.
//
// It embeds Standard rather than re-implementing the interface. Following the
// pattern documented in standard.go, the two per-variant knobs (name and
// promotion whitelist) are configured as fields on the embedded Standard in
// NewRainbow, so the inherited Name, PromotionPieces and — crucially — the
// promotion-restricting ApplyMove all behave correctly for Rainbow without a
// single overridden method. Only InitialPosition, whose colouring is genuinely
// different, is overridden here.
//
// Pawn-direction decision: pawns move by COLOUR, never by board half — a white
// pawn always advances toward rank 8 and a black pawn toward rank 1, exactly as
// the engine already derives push direction, start rank and promotion rank from
// Color. Because Rainbow scatters colours across both home ranks, a square that
// holds a white pawn on rank 7 (Black's home rank) will advance toward rank 8
// and can promote almost immediately; this is the intended, documented
// consequence of colour-relative pawns and is what lets the unchanged Standard
// generator serve Rainbow.
type Rainbow struct {
	Standard

	// mu guards rng: the registered singleton's InitialPosition is called once
	// per new game and must be safe even if two games start concurrently.
	mu  sync.Mutex
	rng *rand.Rand
}

// NewRainbow returns the Rainbow variant: name "rainbow", promotions restricted
// to knight and bishop, seeded from the wall clock so each process produces its
// own sequence of colourings. Tests that need determinism build positions via
// buildInitialPosition with their own seeded *rand.Rand.
func NewRainbow() *Rainbow {
	return &Rainbow{
		Standard: Standard{name: "rainbow", promotions: []PieceType{Knight, Bishop}},
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func init() { Register("rainbow", NewRainbow()) }

// rainbowColoredRanks are the ranks the standard layout occupies (the two home
// ranks per side). Every occupied square lives on one of these; ranks 2-5 are
// empty in the initial position.
var rainbowColoredRanks = [4]int8{0, 1, 6, 7}

// InitialPosition returns a fresh Rainbow starting position: standard piece
// types on standard squares, recoloured structured-randomly subject to the
// symmetry constraint. It advances the variant's RNG, so successive games get
// different colourings.
func (r *Rainbow) InitialPosition() *Position {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buildInitialPosition(r.rng)
}

// buildInitialPosition does the actual colour assignment using the supplied RNG.
// It is separated from InitialPosition so tests can inject a deterministically
// seeded *rand.Rand and assert exact, reproducible positions.
//
// The algorithm starts from the standard position (fixing every piece TYPE on
// its standard square) and then, for each mirror-pair of files {x, 7-x} on each
// occupied rank, flips one coin: the left square gets a random colour and its
// mirror partner gets the opposite. That alone satisfies the symmetry constraint
// for all 32 pieces. The only thing it does not guarantee is one king of each
// colour — the two king squares (e1, e8) are coloured independently and may
// collide — so a final repair flips the rank-8 king's pair when needed, which
// preserves symmetry and yields a uniform 50/50 split between the two valid king
// arrangements.
func (r *Rainbow) buildInitialPosition(rng *rand.Rand) *Position {
	pos, err := ParseFEN(StartingFEN)
	if err != nil {
		panic(fmt.Sprintf("engine: invalid StartingFEN: %v", err))
	}

	for _, y := range rainbowColoredRanks {
		// Files 0-3 each pair with their mirror 7-x (4-7); colouring the left
		// member and mirroring it covers all eight files of the rank.
		for x := int8(0); x < 4; x++ {
			left := Sq(int(x), int(y))
			right := Sq(int(Mirror(x)), int(y))

			var c Color
			if rng.Intn(2) == 0 {
				c = White
			} else {
				c = Black
			}
			setColor(pos, left, c)
			setColor(pos, right, c.Opposite())
		}
	}

	// Guarantee one white king and one black king. In the standard layout the
	// king squares are e1 (file 4, rank 0) and e8 (file 4, rank 7); recolouring
	// may have made them the same colour. If so, flip e8 together with its
	// mirror partner d8 — flipping the whole pair keeps them opposite (symmetry
	// intact) and changes the e8 king's colour to differ from e1's.
	e1 := Sq(4, 0)
	e8 := Sq(4, 7)
	d8 := Sq(3, 7)
	if pos.PieceAt(e1).Color == pos.PieceAt(e8).Color {
		flipColor(pos, e8)
		flipColor(pos, d8)
	}

	if err := r.validate(pos); err != nil {
		// A construction that fails its own invariant is a programming error,
		// not a recoverable condition — fail loudly at startup/first game.
		panic(fmt.Sprintf("engine: rainbow initial position invalid: %v", err))
	}
	return pos
}

// validate asserts the two invariants the Rainbow position must always satisfy:
// the colour-symmetry constraint (for every occupied square (x,y) the mirror
// square (7-x,y) is occupied by the opposite colour) and the presence of exactly
// one king of each colour. It returns a descriptive error rather than panicking
// so callers choose how to react; buildInitialPosition treats any failure as
// fatal.
func (r *Rainbow) validate(pos *Position) error {
	for y := int8(0); y < 8; y++ {
		for x := int8(0); x < 8; x++ {
			sq := Sq(int(x), int(y))
			p := pos.PieceAt(sq)
			if p.IsEmpty() {
				continue
			}
			msq := Sq(int(Mirror(x)), int(y))
			mp := pos.PieceAt(msq)
			if mp.IsEmpty() {
				return fmt.Errorf("symmetry: %s is occupied but mirror %s is empty", sq, msq)
			}
			if mp.Color != p.Color.Opposite() {
				return fmt.Errorf("symmetry: %s (%s) and mirror %s (%s) are not opposite colours", sq, p.Color, msq, mp.Color)
			}
		}
	}

	var whiteKings, blackKings int
	for i := 0; i < 64; i++ {
		p := pos.Board[i]
		if p.Type != King {
			continue
		}
		if p.Color == White {
			whiteKings++
		} else {
			blackKings++
		}
	}
	if whiteKings != 1 || blackKings != 1 {
		return fmt.Errorf("kings: want exactly one of each colour, got %d white and %d black", whiteKings, blackKings)
	}
	return nil
}

// setColor rewrites the colour of the piece on sq, keeping its type. The square
// is assumed occupied (true for every square the colouring loop touches).
func setColor(pos *Position, sq Square, c Color) {
	p := pos.PieceAt(sq)
	p.Color = c
	pos.SetPiece(sq, p)
}

// flipColor swaps the colour of the piece on sq to its opposite.
func flipColor(pos *Position, sq Square) {
	p := pos.PieceAt(sq)
	p.Color = p.Color.Opposite()
	pos.SetPiece(sq, p)
}
