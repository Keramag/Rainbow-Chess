package engine

import "testing"

func TestFENRoundTrip(t *testing.T) {
	fens := []string{
		StartingFEN,
		// Mid-game (after 1.e4 c5 2.Nf3).
		"rnbqkbnr/pp1ppppp/8/2p5/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2",
		// En-passant target available.
		"rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3",
		// Partial castling rights.
		"r3k2r/8/8/8/8/8/8/R3K2R w Kq - 5 20",
		// No castling rights, black to move.
		"8/8/8/4k3/8/4K3/8/8 b - - 12 60",
		// Promotion-rich, only kings and a pawn.
		"4k3/P7/8/8/8/8/8/4K3 w - - 0 1",
	}
	for _, fen := range fens {
		pos, err := ParseFEN(fen)
		if err != nil {
			t.Fatalf("ParseFEN(%q): %v", fen, err)
		}
		if got := pos.FEN(); got != fen {
			t.Errorf("round trip mismatch:\n got %q\nwant %q", got, fen)
		}
	}
}

func TestParseFENStartingPosition(t *testing.T) {
	pos, err := ParseFEN(StartingFEN)
	if err != nil {
		t.Fatal(err)
	}
	if pos.SideToMove != White {
		t.Error("starting position: White should be to move")
	}
	if pos.FullMove != 1 || pos.HalfMove != 0 {
		t.Errorf("starting position: clocks = %d/%d, want 0/1", pos.HalfMove, pos.FullMove)
	}
	if !pos.Castling.Has(WhiteKingside) || !pos.Castling.Has(WhiteQueenside) ||
		!pos.Castling.Has(BlackKingside) || !pos.Castling.Has(BlackQueenside) {
		t.Error("starting position: all four castling rights should be present")
	}
	// Spot-check a few squares.
	checks := []struct {
		sq   string
		want Piece
	}{
		{"a1", Piece{Type: Rook, Color: White}},
		{"e1", Piece{Type: King, Color: White}},
		{"d1", Piece{Type: Queen, Color: White}},
		{"e2", Piece{Type: Pawn, Color: White}},
		{"e8", Piece{Type: King, Color: Black}},
		{"d8", Piece{Type: Queen, Color: Black}},
		{"h8", Piece{Type: Rook, Color: Black}},
		{"e4", Piece{}}, // empty
	}
	for _, c := range checks {
		sq, _ := ParseSquare(c.sq)
		if got := pos.PieceAt(sq); got != c.want {
			t.Errorf("piece at %s = %+v, want %+v", c.sq, got, c.want)
		}
	}
}

func TestParseFENFourFieldDefaults(t *testing.T) {
	// Four-field FEN should default clocks to 0/1.
	pos, err := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -")
	if err != nil {
		t.Fatalf("four-field FEN: %v", err)
	}
	if pos.HalfMove != 0 || pos.FullMove != 1 {
		t.Errorf("four-field defaults: got clocks %d/%d, want 0/1", pos.HalfMove, pos.FullMove)
	}
}

func TestParseFENErrors(t *testing.T) {
	bad := []struct {
		name string
		fen  string
	}{
		{"empty", ""},
		{"too few fields", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w"},
		{"too many ranks", "rnbqkbnr/pppppppp/8/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"},
		{"too few ranks", "rnbqkbnr/pppppppp/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"},
		{"bad piece char", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNX w KQkq - 0 1"},
		{"rank overflows", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNRR w KQkq - 0 1"},
		{"rank too short", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBN w KQkq - 0 1"},
		{"digit overflow", "rnbqkbnr/pppppppp/9/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"},
		{"bad side", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR x KQkq - 0 1"},
		{"bad castling", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w XQkq - 0 1"},
		{"bad enpassant", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq e9 0 1"},
		{"bad halfmove", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - x 1"},
		{"negative halfmove", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - -1 1"},
		{"bad fullmove", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 z"},
		{"zero fullmove", "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 0"},
	}
	for _, c := range bad {
		if _, err := ParseFEN(c.fen); err == nil {
			t.Errorf("%s: expected error for %q, got nil", c.name, c.fen)
		}
	}
}

func TestMoveString(t *testing.T) {
	cases := []struct {
		m    Move
		want string
	}{
		{Move{From: Sq(4, 1), To: Sq(4, 3)}, "e2e4"},
		{Move{From: Sq(4, 6), To: Sq(4, 7), Promotion: Queen}, "e7e8q"},
		{Move{From: Sq(1, 6), To: Sq(1, 7), Promotion: Knight}, "b7b8n"},
	}
	for _, c := range cases {
		if got := c.m.String(); got != c.want {
			t.Errorf("Move.String() = %q, want %q", got, c.want)
		}
	}
}
