package engine

import "testing"

func TestSquareAlgebraicRoundTrip(t *testing.T) {
	for i := 0; i < 64; i++ {
		sq := SquareFromIndex(i)
		got, err := ParseSquare(sq.String())
		if err != nil {
			t.Fatalf("ParseSquare(%q): unexpected error: %v", sq.String(), err)
		}
		if got != sq {
			t.Errorf("round trip for index %d: got %v, want %v", i, got, sq)
		}
		if got.Index() != i {
			t.Errorf("Index() for %v: got %d, want %d", got, got.Index(), i)
		}
	}
}

func TestParseSquareKnownValues(t *testing.T) {
	cases := []struct {
		in   string
		file int8
		rank int8
	}{
		{"a1", 0, 0},
		{"h1", 7, 0},
		{"a8", 0, 7},
		{"h8", 7, 7},
		{"e4", 4, 3},
		{"d5", 3, 4},
	}
	for _, c := range cases {
		sq, err := ParseSquare(c.in)
		if err != nil {
			t.Fatalf("ParseSquare(%q): %v", c.in, err)
		}
		if sq.File != c.file || sq.Rank != c.rank {
			t.Errorf("ParseSquare(%q) = {%d,%d}, want {%d,%d}", c.in, sq.File, sq.Rank, c.file, c.rank)
		}
	}
}

func TestParseSquareErrors(t *testing.T) {
	for _, in := range []string{"", "e", "e9", "i4", "44", "z1", "e44"} {
		if _, err := ParseSquare(in); err == nil {
			t.Errorf("ParseSquare(%q): expected error, got nil", in)
		}
	}
}

func TestSquareValid(t *testing.T) {
	if !(Square{File: 0, Rank: 0}).Valid() {
		t.Error("a1 should be valid")
	}
	for _, sq := range []Square{{File: -1, Rank: 0}, {File: 8, Rank: 0}, {File: 0, Rank: -1}, {File: 0, Rank: 8}} {
		if sq.Valid() {
			t.Errorf("%+v should be invalid", sq)
		}
	}
}

func TestMirror(t *testing.T) {
	cases := [][2]int8{{0, 7}, {1, 6}, {2, 5}, {3, 4}, {4, 3}, {5, 2}, {6, 1}, {7, 0}}
	for _, c := range cases {
		if got := Mirror(c[0]); got != c[1] {
			t.Errorf("Mirror(%d) = %d, want %d", c[0], got, c[1])
		}
	}
	// Mirror is an involution: applying it twice is the identity.
	for x := int8(0); x < 8; x++ {
		if Mirror(Mirror(x)) != x {
			t.Errorf("Mirror(Mirror(%d)) != %d", x, x)
		}
	}
}

func TestCastlingRights(t *testing.T) {
	cr := CastlingRights(0)
	cr = cr.With(WhiteKingside).With(BlackQueenside)
	if !cr.Has(WhiteKingside) || !cr.Has(BlackQueenside) {
		t.Error("expected WhiteKingside and BlackQueenside set")
	}
	if cr.Has(WhiteQueenside) || cr.Has(BlackKingside) {
		t.Error("did not expect WhiteQueenside or BlackKingside set")
	}
	cr = cr.Without(WhiteKingside)
	if cr.Has(WhiteKingside) {
		t.Error("WhiteKingside should have been cleared")
	}
}

func TestCloneIsDeep(t *testing.T) {
	pos, err := ParseFEN("rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3")
	if err != nil {
		t.Fatal(err)
	}
	c := pos.Clone()
	// Mutate the clone; original must be unaffected.
	c.SetPiece(Sq(0, 0), Piece{})
	c.SideToMove = Black
	*c.EnPassant = Sq(0, 0)
	if pos.PieceAt(Sq(0, 0)).IsEmpty() {
		t.Error("clone mutation leaked into original board")
	}
	if pos.SideToMove != White {
		t.Error("clone mutation leaked into original side-to-move")
	}
	if pos.EnPassant.String() != "d6" {
		t.Errorf("clone mutation leaked into original en-passant: got %v", pos.EnPassant)
	}
}
