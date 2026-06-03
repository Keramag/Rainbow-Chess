package engine

import "testing"

// mustParse is a test helper that parses a FEN or fails the test.
func mustParse(t *testing.T, fen string) *Position {
	t.Helper()
	pos, err := ParseFEN(fen)
	if err != nil {
		t.Fatalf("ParseFEN(%q): %v", fen, err)
	}
	return pos
}

// mustSquare parses algebraic coordinates or fails the test.
func mustSquare(t *testing.T, s string) Square {
	t.Helper()
	sq, err := ParseSquare(s)
	if err != nil {
		t.Fatalf("ParseSquare(%q): %v", s, err)
	}
	return sq
}

func TestIsSquareAttackedPerPiece(t *testing.T) {
	cases := []struct {
		name     string
		fen      string
		target   string
		byColor  Color
		attacked bool
	}{
		// A lone white knight on d4 attacks the L-shaped squares.
		{"knight attacks e6", "8/8/8/8/3N4/8/8/8 w - - 0 1", "e6", White, true},
		{"knight attacks c2", "8/8/8/8/3N4/8/8/8 w - - 0 1", "c2", White, true},
		{"knight no attack d5", "8/8/8/8/3N4/8/8/8 w - - 0 1", "d5", White, false},

		// White bishop on c1 attacks along the open a3-h6 diagonal.
		{"bishop attacks h6", "8/8/8/8/8/8/8/2B5 w - - 0 1", "h6", White, true},
		{"bishop attacks a3", "8/8/8/8/8/8/8/2B5 w - - 0 1", "a3", White, true},
		{"bishop no orthogonal", "8/8/8/8/8/8/8/2B5 w - - 0 1", "c8", White, false},

		// White rook on a1 attacks down the a-file and along rank 1.
		{"rook attacks a8", "8/8/8/8/8/8/8/R7 w - - 0 1", "a8", White, true},
		{"rook attacks h1", "8/8/8/8/8/8/8/R7 w - - 0 1", "h1", White, true},
		{"rook no diagonal", "8/8/8/8/8/8/8/R7 w - - 0 1", "b2", White, false},

		// White queen on d4 attacks orthogonally and diagonally.
		{"queen attacks d8", "8/8/8/8/3Q4/8/8/8 w - - 0 1", "d8", White, true},
		{"queen attacks h8", "8/8/8/8/3Q4/8/8/8 w - - 0 1", "h8", White, true},
		{"queen attacks a1", "8/8/8/8/3Q4/8/8/8 w - - 0 1", "a1", White, true},
		{"queen no knight square", "8/8/8/8/3Q4/8/8/8 w - - 0 1", "e6", White, false},

		// White king on e1 attacks adjacent squares only.
		{"king attacks e2", "8/8/8/8/8/8/8/4K3 w - - 0 1", "e2", White, true},
		{"king attacks d2", "8/8/8/8/8/8/8/4K3 w - - 0 1", "d2", White, true},
		{"king no reach e3", "8/8/8/8/8/8/8/4K3 w - - 0 1", "e3", White, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos := mustParse(t, c.fen)
			got := IsSquareAttacked(pos, mustSquare(t, c.target), c.byColor)
			if got != c.attacked {
				t.Errorf("IsSquareAttacked(%s, %s) = %v, want %v", c.fen, c.target, got, c.attacked)
			}
		})
	}
}

func TestPawnAttackDirectionByColor(t *testing.T) {
	cases := []struct {
		name     string
		fen      string
		target   string
		byColor  Color
		attacked bool
	}{
		// White pawn on e4 attacks the two squares one rank higher (d5, f5),
		// never the squares behind it (d3, f3).
		{"white pawn attacks d5", "8/8/8/8/4P3/8/8/8 w - - 0 1", "d5", White, true},
		{"white pawn attacks f5", "8/8/8/8/4P3/8/8/8 w - - 0 1", "f5", White, true},
		{"white pawn not behind d3", "8/8/8/8/4P3/8/8/8 w - - 0 1", "d3", White, false},
		{"white pawn not forward e5", "8/8/8/8/4P3/8/8/8 w - - 0 1", "e5", White, false},

		// Black pawn on e5 attacks one rank lower (d4, f4).
		{"black pawn attacks d4", "8/8/8/4p3/8/8/8/8 w - - 0 1", "d4", Black, true},
		{"black pawn attacks f4", "8/8/8/4p3/8/8/8/8 w - - 0 1", "f4", Black, true},
		{"black pawn not behind d6", "8/8/8/4p3/8/8/8/8 w - - 0 1", "d6", Black, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos := mustParse(t, c.fen)
			got := IsSquareAttacked(pos, mustSquare(t, c.target), c.byColor)
			if got != c.attacked {
				t.Errorf("IsSquareAttacked(%s, %s, %v) = %v, want %v", c.fen, c.target, c.byColor, got, c.attacked)
			}
		})
	}
}

func TestSlidingAttacksBlocked(t *testing.T) {
	// White rook on a1, a white pawn on a3 blocks the file beyond it.
	pos := mustParse(t, "8/8/8/8/8/P7/8/R7 w - - 0 1")
	if !IsSquareAttacked(pos, mustSquare(t, "a2"), White) {
		t.Error("rook should attack a2 (before blocker)")
	}
	if IsSquareAttacked(pos, mustSquare(t, "a4"), White) {
		t.Error("rook should NOT attack a4 (beyond blocker)")
	}
	// The blocker square itself is "attacked" (rook bears on it).
	if !IsSquareAttacked(pos, mustSquare(t, "a3"), White) {
		t.Error("rook should attack the blocker square a3")
	}

	// Bishop on c1 with a blocker on e3 stops the diagonal at e3.
	pos = mustParse(t, "8/8/8/8/8/4p3/8/2B5 w - - 0 1")
	if !IsSquareAttacked(pos, mustSquare(t, "d2"), White) {
		t.Error("bishop should attack d2 (before blocker)")
	}
	if IsSquareAttacked(pos, mustSquare(t, "f4"), White) {
		t.Error("bishop should NOT attack f4 (beyond blocker)")
	}
}

func TestIsSquareAttackedInvalidSquare(t *testing.T) {
	pos := mustParse(t, StartingFEN)
	if IsSquareAttacked(pos, Square{File: -1, Rank: 0}, White) {
		t.Error("an off-board square cannot be attacked")
	}
}

func TestKingSquare(t *testing.T) {
	pos := mustParse(t, StartingFEN)
	wk, ok := KingSquare(pos, White)
	if !ok || wk != mustSquare(t, "e1") {
		t.Errorf("white king square = %v (found=%v), want e1", wk, ok)
	}
	bk, ok := KingSquare(pos, Black)
	if !ok || bk != mustSquare(t, "e8") {
		t.Errorf("black king square = %v (found=%v), want e8", bk, ok)
	}

	// A position with no white king reports not-found.
	noKing := mustParse(t, "4k3/8/8/8/8/8/8/8 w - - 0 1")
	if _, ok := KingSquare(noKing, White); ok {
		t.Error("expected no white king to be found")
	}
}

func TestIsInCheck(t *testing.T) {
	cases := []struct {
		name    string
		fen     string
		color   Color
		inCheck bool
	}{
		// Start position: neither king is in check.
		{"start not in check (white)", StartingFEN, White, false},
		{"start not in check (black)", StartingFEN, Black, false},

		// Black king on e8 checked by a white rook down the open e-file.
		{"rook check on e-file", "4k3/8/8/8/8/8/8/4R3 b - - 0 1", Black, true},
		// Same rook, but a black pawn on e5 blocks the check.
		{"check blocked by pawn", "4k3/8/8/4p3/8/8/8/4R3 b - - 0 1", Black, false},

		// White king on e1 checked by a black bishop on the a5-e1 diagonal.
		{"bishop diagonal check", "8/8/8/b7/8/8/8/4K3 w - - 0 1", White, true},

		// Knight check: black king e8, white knight on f6.
		{"knight check", "4k3/8/5N2/8/8/8/8/4K3 b - - 0 1", Black, true},

		// Discovered check: white king e1, black rook on e8, with the only
		// blocker (a black piece) NOT on the e-file, so the king is exposed.
		{"open e-file rook check", "4r3/8/8/8/8/8/8/4K3 w - - 0 1", White, true},

		// A position where a king is absent is treated as not in check.
		{"no king not in check", "8/8/8/8/8/8/8/4R3 b - - 0 1", Black, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos := mustParse(t, c.fen)
			if got := IsInCheck(pos, c.color); got != c.inCheck {
				t.Errorf("IsInCheck(%s, %v) = %v, want %v", c.fen, c.color, got, c.inCheck)
			}
		})
	}
}

// TestDiscoveredCheckLine verifies a true discovered-check scenario: a piece
// moves off a line, unmasking a slider behind it. We model the "after" state
// directly via FEN — the white king on h1 is exposed to a black queen on a1
// along rank 1 once the intervening square is empty, but is safe when a piece
// sits between them.
func TestDiscoveredCheckLine(t *testing.T) {
	exposed := mustParse(t, "8/8/8/8/8/8/8/q6K w - - 0 1")
	if !IsInCheck(exposed, White) {
		t.Error("white king should be in check from the queen along rank 1")
	}
	blocked := mustParse(t, "8/8/8/8/8/8/8/q3N2K w - - 0 1")
	if IsInCheck(blocked, White) {
		t.Error("white king should be shielded by the knight on e1")
	}
}
