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

// maxInitialAttempts bounds the playability re-roll in InitialPosition. With
// roughly three quarters of colourings playable, reaching this many consecutive
// rejections is statistically impossible; the cap exists only so a deep bug
// surfaces as a panic rather than an infinite loop.
const maxInitialAttempts = 1000

// InitialPosition returns a fresh Rainbow starting position: standard piece
// types on standard squares, recoloured structured-randomly subject to the
// symmetry constraint (kings and queens excepted — they stay their native
// colour), and guaranteed to start with NEITHER king in check. It advances the
// variant's RNG, so successive games get different colourings.
//
// Why the re-roll: keeping each side's king and queen native removes the
// dominant source of an opening check, but a recoloured pawn on d2/f2 (or d7/f7)
// can still attack the native king on e1 (or e8). Such a colouring would begin
// the game already in check — which we don't want — so we discard any colouring
// in which either king is attacked (or White somehow has no legal reply) and
// roll again. The rejection only removes check-at-start colourings, so it cannot
// bias the colour distribution among accepted games. buildInitialPosition stays
// the pure, single-shot primitive the tests assert against; the no-check
// guarantee lives here, on the production path the hub uses to start a game.
func (r *Rainbow) InitialPosition() *Position {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < maxInitialAttempts; i++ {
		pos := r.buildInitialPosition(r.rng)
		if !IsInCheck(pos, White) && !IsInCheck(pos, Black) && len(LegalMoves(pos)) > 0 {
			return pos
		}
	}
	panic("engine: rainbow could not produce a check-free initial position")
}

// buildInitialPosition does the actual colour assignment using the supplied RNG.
// It is separated from InitialPosition so tests can inject a deterministically
// seeded *rand.Rand and assert exact, reproducible positions.
//
// The algorithm starts from the standard position (fixing every piece TYPE on
// its standard square) and then, for each mirror-pair of files {x, 7-x} on each
// occupied rank, flips one coin: the left square gets a random colour and its
// mirror partner gets the opposite. The kings and queens are the exception — on
// the two back ranks the centre pair is the d/e files (queen and king), and that
// pair is skipped so both royals keep their native colour (white on rank 1,
// black on rank 8). This leaves exactly one king of each colour with no repair
// needed, and is what lets InitialPosition guarantee a check-free start. Every
// other piece (rooks, bishops, knights and all pawns, including the d/e pawns)
// is recoloured in mirror pairs, so the symmetry constraint still holds for all
// non-royal pieces.
func (r *Rainbow) buildInitialPosition(rng *rand.Rand) *Position {
	pos, err := ParseFEN(StartingFEN)
	if err != nil {
		panic(fmt.Sprintf("engine: invalid StartingFEN: %v", err))
	}

	for _, y := range rainbowColoredRanks {
		backRank := y == 0 || y == 7
		// Files 0-3 each pair with their mirror 7-x (4-7); colouring the left
		// member and mirroring it covers all eight files of the rank.
		for x := int8(0); x < 4; x++ {
			// Skip the king/queen pair (d/e files) on the back ranks: royalty is
			// not shuffled, so the kings stay native and the game never opens
			// with a king attacked by its own side's geometry.
			if backRank && x == 3 {
				continue
			}
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

	if err := r.validate(pos); err != nil {
		// A construction that fails its own invariant is a programming error,
		// not a recoverable condition — fail loudly at startup/first game.
		panic(fmt.Sprintf("engine: rainbow initial position invalid: %v", err))
	}
	return pos
}

// validate asserts the two invariants the Rainbow position must always satisfy:
// the colour-symmetry constraint (for every occupied NON-ROYAL square (x,y) the
// mirror square (7-x,y) is occupied by the opposite colour) and the presence of
// exactly one king of each colour. Kings and queens are exempt from the symmetry
// check: they keep their native colour, so the central d/e pair on each back rank
// is deliberately same-side (white queen + white king on rank 1, black on rank
// 8) rather than mirror-opposite. It returns a descriptive error rather than
// panicking so callers choose how to react; buildInitialPosition treats any
// failure as fatal.
func (r *Rainbow) validate(pos *Position) error {
	for y := int8(0); y < 8; y++ {
		for x := int8(0); x < 8; x++ {
			sq := Sq(int(x), int(y))
			p := pos.PieceAt(sq)
			if p.IsEmpty() {
				continue
			}
			// Royalty is native, not mirrored — skip the king/queen squares so
			// the (white) d1/e1 and (black) d8/e8 pairs don't trip the symmetry
			// rule that governs every other piece.
			if p.Type == King || p.Type == Queen {
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
