package engine

import (
	"reflect"
	"sort"
	"testing"
)

// movesFrom returns the sorted UCI strings of all moves originating on `from`.
func movesFrom(moves []Move, from Square) []string {
	var out []string
	for _, m := range moves {
		if m.From == from {
			out = append(out, m.String())
		}
	}
	sort.Strings(out)
	return out
}

// hasMove reports whether a move with the given UCI string is present.
func hasMove(moves []Move, uci string) bool {
	for _, m := range moves {
		if m.String() == uci {
			return true
		}
	}
	return false
}

// findMove returns the first move matching the UCI string (and whether found).
func findMove(moves []Move, uci string) (Move, bool) {
	for _, m := range moves {
		if m.String() == uci {
			return m, true
		}
	}
	return Move{}, false
}

func TestPseudoLegalKnight(t *testing.T) {
	// A lone white knight on d4 has all eight L-moves available.
	pos := mustParse(t, "8/8/8/8/3N4/8/8/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "d4"))
	want := []string{"d4b3", "d4b5", "d4c2", "d4c6", "d4e2", "d4e6", "d4f3", "d4f5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("knight moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalKnightBlockedByOwn(t *testing.T) {
	// Own pawns on c2 and e2 remove those two targets; an enemy on b3 stays a
	// legal capture.
	pos := mustParse(t, "8/8/8/8/3N4/1p6/2P1P3/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "d4"))
	want := []string{"d4b3", "d4b5", "d4c6", "d4e6", "d4f3", "d4f5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("knight moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalSliderCounts(t *testing.T) {
	cases := []struct {
		name  string
		fen   string
		from  string
		count int
		spot  []string // a few moves that must be present
	}{
		{"bishop d4", "8/8/8/8/3B4/8/8/8 w - - 0 1", "d4", 13, []string{"d4a1", "d4h8", "d4g1", "d4a7"}},
		{"rook d4", "8/8/8/8/3R4/8/8/8 w - - 0 1", "d4", 14, []string{"d4d1", "d4d8", "d4a4", "d4h4"}},
		{"queen d4", "8/8/8/8/3Q4/8/8/8 w - - 0 1", "d4", 27, []string{"d4a1", "d4h8", "d4d8", "d4h4"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos := mustParse(t, c.fen)
			got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, c.from))
			if len(got) != c.count {
				t.Errorf("%s move count = %d, want %d (%v)", c.name, len(got), c.count, got)
			}
			for _, m := range c.spot {
				if !hasMove(PseudoLegalMoves(pos), m) {
					t.Errorf("%s missing expected move %s", c.name, m)
				}
			}
		})
	}
}

func TestPseudoLegalSliderBlockedAndCapture(t *testing.T) {
	// White rook on a1: own pawn on a4 caps the file at a3 (a4 not reachable),
	// enemy pawn on d1 is a capture that stops the rank there (e1+ unreachable).
	pos := mustParse(t, "8/8/8/8/P7/8/8/R2p4 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "a1"))
	want := []string{"a1a2", "a1a3", "a1b1", "a1c1", "a1d1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rook moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalKingNoCastle(t *testing.T) {
	// A lone king off its home square has only its eight steps (here five fit
	// on the board) and no castling.
	pos := mustParse(t, "8/8/8/8/8/8/8/4K3 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e1"))
	want := []string{"e1d1", "e1d2", "e1e2", "e1f1", "e1f2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("king moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalPawnPushes(t *testing.T) {
	// White pawn on its start rank: single and double push.
	pos := mustParse(t, "8/8/8/8/8/8/4P3/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e2"))
	want := []string{"e2e3", "e2e4"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pawn push moves = %v, want %v", got, want)
	}
	// The double push must carry the IsDoublePush flag.
	if m, ok := findMove(PseudoLegalMoves(pos), "e2e4"); !ok || !m.IsDoublePush {
		t.Errorf("e2e4 IsDoublePush flag missing (found=%v)", ok)
	}
}

func TestPseudoLegalPawnBlocked(t *testing.T) {
	// A blocker directly ahead removes both the single and the double push.
	pos := mustParse(t, "8/8/8/8/8/4p3/4P3/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e2"))
	if len(got) != 0 {
		t.Errorf("blocked pawn should have no moves, got %v", got)
	}
}

func TestPseudoLegalPawnNoDoublePushThroughOccupied(t *testing.T) {
	// The single push is open but the double-push destination is occupied:
	// only the single push is generated.
	pos := mustParse(t, "8/8/8/8/4p3/8/4P3/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e2"))
	want := []string{"e2e3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pawn moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalPawnCaptures(t *testing.T) {
	// White pawn on e4 with enemies on d5 and f5: push plus two captures.
	pos := mustParse(t, "8/8/8/3p1p2/4P3/8/8/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e4"))
	want := []string{"e4d5", "e4e5", "e4f5"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pawn capture moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalPromotionPush(t *testing.T) {
	// A pawn reaching the last rank expands into all four promotions.
	pos := mustParse(t, "8/4P3/8/8/8/8/8/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e7"))
	want := []string{"e7e8b", "e7e8n", "e7e8q", "e7e8r"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("promotion moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalPromotionCapture(t *testing.T) {
	// Push-promotion (4) plus capture-promotion onto d8 (4) = 8 moves.
	pos := mustParse(t, "3r4/4P3/8/8/8/8/8/8 w - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e7"))
	want := []string{
		"e7d8b", "e7d8n", "e7d8q", "e7d8r",
		"e7e8b", "e7e8n", "e7e8q", "e7e8r",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("promotion-capture moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalBlackPawnDirection(t *testing.T) {
	// Black pawns push toward rank 1 and promote on rank 1 — direction comes
	// from color, not board half.
	pos := mustParse(t, "8/8/8/8/8/8/4p3/8 b - - 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e2"))
	want := []string{"e2e1b", "e2e1n", "e2e1q", "e2e1r"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("black promotion moves = %v, want %v", got, want)
	}

	// A black pawn on its start rank double-pushes downward.
	pos = mustParse(t, "8/4p3/8/8/8/8/8/8 b - - 0 1")
	got = movesFrom(PseudoLegalMoves(pos), mustSquare(t, "e7"))
	want = []string{"e7e5", "e7e6"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("black push moves = %v, want %v", got, want)
	}
}

func TestPseudoLegalEnPassantWhite(t *testing.T) {
	// Black has just played e7-e5; White's d5 pawn may capture en passant.
	pos := mustParse(t, "8/8/8/3Pp3/8/8/8/8 w - e6 0 1")
	got := movesFrom(PseudoLegalMoves(pos), mustSquare(t, "d5"))
	want := []string{"d5d6", "d5e6"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("en-passant moves = %v, want %v", got, want)
	}
	if m, ok := findMove(PseudoLegalMoves(pos), "d5e6"); !ok || !m.IsEnPassant {
		t.Errorf("d5e6 IsEnPassant flag missing (found=%v)", ok)
	}
}

func TestPseudoLegalEnPassantBlack(t *testing.T) {
	// White has just played e2-e4; Black's d4 pawn captures en passant to e3.
	pos := mustParse(t, "8/8/8/8/3pP3/8/8/8 b - e3 0 1")
	if m, ok := findMove(PseudoLegalMoves(pos), "d4e3"); !ok || !m.IsEnPassant {
		t.Errorf("expected black en-passant d4e3 with flag set (found=%v)", ok)
	}
}

func TestPseudoLegalEnPassantStaleFlagIgnored(t *testing.T) {
	// An en-passant target with no enemy pawn beside the capturer must not
	// produce a phantom capture.
	pos := mustParse(t, "8/8/8/3P4/8/8/8/8 w - e6 0 1")
	if hasMove(PseudoLegalMoves(pos), "d5e6") {
		t.Error("phantom en-passant generated with no pawn to capture")
	}
}

func TestPseudoLegalCastlingBothSides(t *testing.T) {
	// Clear back ranks with all rights: both castles are available.
	pos := mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	moves := PseudoLegalMoves(pos)
	if m, ok := findMove(moves, "e1g1"); !ok || !m.IsCastle {
		t.Errorf("expected king-side castle e1g1 with flag (found=%v)", ok)
	}
	if m, ok := findMove(moves, "e1c1"); !ok || !m.IsCastle {
		t.Errorf("expected queen-side castle e1c1 with flag (found=%v)", ok)
	}

	// Black to move castles the same way on rank 8.
	pos = mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1")
	moves = PseudoLegalMoves(pos)
	if !hasMove(moves, "e8g8") || !hasMove(moves, "e8c8") {
		t.Error("expected both black castles e8g8 and e8c8")
	}
}

func TestPseudoLegalCastlingBlockedByPieces(t *testing.T) {
	// A knight on b1 blocks queen side; a bishop on f1 blocks king side.
	pos := mustParse(t, "r3k2r/8/8/8/8/8/8/RN2KB1R w KQkq - 0 1")
	moves := PseudoLegalMoves(pos)
	if hasMove(moves, "e1g1") {
		t.Error("king-side castle should be blocked by the bishop on f1")
	}
	if hasMove(moves, "e1c1") {
		t.Error("queen-side castle should be blocked by the knight on b1")
	}
}

func TestPseudoLegalCastlingNoRights(t *testing.T) {
	// Empty path but no castling rights → no castling moves.
	pos := mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R w - - 0 1")
	moves := PseudoLegalMoves(pos)
	if hasMove(moves, "e1g1") || hasMove(moves, "e1c1") {
		t.Error("castling generated despite no rights")
	}
}

func TestPseudoLegalCastlingThroughAttack(t *testing.T) {
	// A black rook on f7 attacks down the open f-file onto f1, the king's
	// king-side transit square — so only queen-side castling remains.
	pos := mustParse(t, "4k3/5r2/8/8/8/8/8/R3K2R w KQkq - 0 1")
	moves := PseudoLegalMoves(pos)
	if hasMove(moves, "e1g1") {
		t.Error("king-side castle should be illegal: f1 is attacked")
	}
	if !hasMove(moves, "e1c1") {
		t.Error("queen-side castle should still be available")
	}
}

func TestPseudoLegalCastlingWhileInCheck(t *testing.T) {
	// A black rook on e7 checks the king down the e-file: no castling either way.
	pos := mustParse(t, "4k3/4r3/8/8/8/8/8/R3K2R w KQkq - 0 1")
	moves := PseudoLegalMoves(pos)
	if hasMove(moves, "e1g1") || hasMove(moves, "e1c1") {
		t.Error("a king in check may not castle")
	}
}

func TestPseudoLegalDoesNotFilterSelfCheck(t *testing.T) {
	// Pseudo-legal generation deliberately leaves in moves that expose the
	// mover's own king (the bishop on e2 is pinned by the rook on e8). The
	// LegalMoves filter (Task 4) is what removes them.
	pos := mustParse(t, "4r3/8/8/8/8/8/4B3/4K3 w - - 0 1")
	if !hasMove(PseudoLegalMoves(pos), "e2d3") {
		t.Error("pseudo-legal generation should include the pinned bishop's move e2d3")
	}
}

func TestPseudoLegalStartPositionCount(t *testing.T) {
	// The standard start position has exactly 20 pseudo-legal moves
	// (16 pawn moves + 4 knight moves), which here also equals the legal count.
	pos := mustParse(t, StartingFEN)
	if n := len(PseudoLegalMoves(pos)); n != 20 {
		t.Errorf("start position pseudo-legal move count = %d, want 20", n)
	}
}
