package engine

import (
	"math/rand"
	"reflect"
	"testing"
)

// seededRainbow returns a Rainbow plus a deterministically seeded RNG so a test
// can build an exact, reproducible initial position via buildInitialPosition.
func seededRainbow(seed int64) (*Rainbow, *rand.Rand) {
	return NewRainbow(), rand.New(rand.NewSource(seed))
}

func TestRainbowNameAndPromotionPieces(t *testing.T) {
	r := NewRainbow()
	if r.Name() != "rainbow" {
		t.Errorf("Name() = %q, want %q", r.Name(), "rainbow")
	}
	want := []PieceType{Knight, Bishop}
	if got := r.PromotionPieces(); !reflect.DeepEqual(got, want) {
		t.Errorf("PromotionPieces() = %v, want %v", got, want)
	}
}

func TestRainbowRegisteredByInit(t *testing.T) {
	v, err := Get("rainbow")
	if err != nil {
		t.Fatalf("Get(rainbow): %v", err)
	}
	if v.Name() != "rainbow" {
		t.Errorf("Get(rainbow).Name() = %q, want %q", v.Name(), "rainbow")
	}
	found := false
	for _, n := range List() {
		if n == "rainbow" {
			found = true
		}
	}
	if !found {
		t.Errorf("List() = %v, missing %q", List(), "rainbow")
	}
}

// checkSymmetry verifies the colour-symmetry invariant independently of the
// engine's own validate(), so the test does not merely trust the code under
// test: for every occupied square the file-mirror square must hold the opposite
// colour.
func checkSymmetry(t *testing.T, pos *Position) {
	t.Helper()
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
				t.Fatalf("square %s occupied but mirror %s empty", sq, msq)
			}
			if mp.Color != p.Color.Opposite() {
				t.Fatalf("square %s is %s but mirror %s is %s (want opposite)", sq, p.Color, msq, mp.Color)
			}
		}
	}
}

// countKings returns the number of white and black kings on the board.
func countKings(pos *Position) (white, black int) {
	for i := 0; i < 64; i++ {
		p := pos.Board[i]
		if p.Type != King {
			continue
		}
		if p.Color == White {
			white++
		} else {
			black++
		}
	}
	return white, black
}

// TestRainbowSymmetryAcrossManySeeds is the core invariant test: across a wide
// range of seeds, every generated position must satisfy the symmetry constraint,
// have exactly one king of each colour, and pass the engine's own validate().
func TestRainbowSymmetryAcrossManySeeds(t *testing.T) {
	r := NewRainbow()
	for seed := int64(0); seed < 200; seed++ {
		pos := r.buildInitialPosition(rand.New(rand.NewSource(seed)))

		checkSymmetry(t, pos)

		wk, bk := countKings(pos)
		if wk != 1 || bk != 1 {
			t.Fatalf("seed %d: kings = %d white, %d black; want 1 and 1", seed, wk, bk)
		}

		if err := r.validate(pos); err != nil {
			t.Fatalf("seed %d: validate() = %v, want nil", seed, err)
		}
	}
}

// TestRainbowKeepsStandardPieceTypes confirms only colours change: every
// square's piece TYPE must match the standard starting position.
func TestRainbowKeepsStandardPieceTypes(t *testing.T) {
	r, rng := seededRainbow(42)
	pos := r.buildInitialPosition(rng)
	std := NewStandard().InitialPosition()

	for i := 0; i < 64; i++ {
		if pos.Board[i].Type != std.Board[i].Type {
			sq := SquareFromIndex(i)
			t.Errorf("square %s: type = %s, want %s (standard layout)", sq, pos.Board[i].Type, std.Board[i].Type)
		}
	}
	// Side to move, clocks and structural fields are inherited from the
	// standard start unchanged.
	if pos.SideToMove != White {
		t.Errorf("SideToMove = %s, want white", pos.SideToMove)
	}
}

// TestRainbowInitialPositionValidates exercises validate() on a freshly built
// position directly (the "call it on init" path is covered by construction).
func TestRainbowInitialPositionValidates(t *testing.T) {
	r, rng := seededRainbow(7)
	if err := r.validate(r.buildInitialPosition(rng)); err != nil {
		t.Errorf("validate(initial) = %v, want nil", err)
	}
}

// TestRainbowValidateRejectsBadPosition makes sure validate() actually catches
// violations rather than always returning nil.
func TestRainbowValidateRejectsBadPosition(t *testing.T) {
	r := NewRainbow()

	// Symmetry violation: a lone white pawn whose mirror square (7,1) is empty.
	// (The king pair below is itself symmetric, so the pawn is the only fault.)
	broken := NewPosition()
	broken.SetPiece(Sq(0, 1), Piece{Type: Pawn, Color: White})
	broken.SetPiece(Sq(4, 0), Piece{Type: King, Color: White})
	broken.SetPiece(Sq(3, 0), Piece{Type: King, Color: Black})
	if err := r.validate(broken); err == nil {
		t.Error("validate(asymmetric position) = nil, want error")
	}

	// Missing-king case: a symmetric board with no kings at all.
	noKings := NewPosition()
	noKings.SetPiece(Sq(0, 0), Piece{Type: Rook, Color: White})
	noKings.SetPiece(Sq(7, 0), Piece{Type: Rook, Color: Black})
	if err := r.validate(noKings); err == nil {
		t.Error("validate(no kings) = nil, want error")
	}
}

// TestRainbowApplyMoveRejectsQueenRookPromotion verifies the inherited promotion
// whitelist: knight/bishop promotions succeed, queen/rook are rejected.
func TestRainbowApplyMoveRejectsQueenRookPromotion(t *testing.T) {
	r := NewRainbow()
	// White pawn on a7 ready to promote; one king of each colour, out of reach.
	pos := mustParse(t, "8/P7/8/8/8/8/8/k6K w - - 0 1")

	for _, promo := range []PieceType{Knight, Bishop} {
		move := Move{From: mustSquare(t, "a7"), To: mustSquare(t, "a8"), Promotion: promo}
		next, err := r.ApplyMove(pos, move)
		if err != nil {
			t.Fatalf("promote to %s: ApplyMove: %v", promo, err)
		}
		if got := next.PieceAt(mustSquare(t, "a8")); got.Type != promo || got.Color != White {
			t.Errorf("promote to %s: a8 holds %+v, want white %s", promo, got, promo)
		}
	}

	for _, promo := range []PieceType{Queen, Rook} {
		move := Move{From: mustSquare(t, "a7"), To: mustSquare(t, "a8"), Promotion: promo}
		if _, err := r.ApplyMove(pos, move); err == nil {
			t.Errorf("promotion to %s should be rejected by rainbow", promo)
		}
	}
}

// TestRainbowDeterministicPerSeed confirms the injectable RNG fully determines
// the position: the same seed must produce byte-identical FEN.
func TestRainbowDeterministicPerSeed(t *testing.T) {
	r := NewRainbow()
	a := r.buildInitialPosition(rand.New(rand.NewSource(12345)))
	b := r.buildInitialPosition(rand.New(rand.NewSource(12345)))
	if a.FEN() != b.FEN() {
		t.Errorf("same seed produced different positions:\n %q\n %q", a.FEN(), b.FEN())
	}
}

// TestRainbowVariesAcrossSeeds confirms the colouring is actually random: many
// seeds must yield more than one distinct position (otherwise the RNG is unused)
// and must exercise both valid king arrangements (white on e1 and black on e1).
func TestRainbowVariesAcrossSeeds(t *testing.T) {
	r := NewRainbow()
	seen := map[string]bool{}
	whiteKingOnE1, blackKingOnE1 := false, false
	e1 := Sq(4, 0)
	for seed := int64(0); seed < 100; seed++ {
		pos := r.buildInitialPosition(rand.New(rand.NewSource(seed)))
		seen[pos.FEN()] = true
		if pos.PieceAt(e1).Color == White {
			whiteKingOnE1 = true
		} else {
			blackKingOnE1 = true
		}
	}
	if len(seen) < 2 {
		t.Errorf("expected varied positions across seeds, got %d distinct", len(seen))
	}
	if !whiteKingOnE1 || !blackKingOnE1 {
		t.Errorf("expected both king arrangements across seeds; whiteKingOnE1=%v blackKingOnE1=%v", whiteKingOnE1, blackKingOnE1)
	}
}
