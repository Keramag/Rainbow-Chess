package engine

import (
	"reflect"
	"sort"
	"testing"
)

// legalMovesFrom returns the sorted UCI strings of all legal moves originating
// on `from`.
func legalMovesFrom(pos *Position, from Square) []string {
	var out []string
	for _, m := range LegalMoves(pos) {
		if m.From == from {
			out = append(out, m.String())
		}
	}
	sort.Strings(out)
	return out
}

// hasLegal reports whether a legal move with the given UCI string exists.
func hasLegal(pos *Position, uci string) bool {
	for _, m := range LegalMoves(pos) {
		if m.String() == uci {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Legal-move filtering: pins, blocking, capturing and escaping check.
// ---------------------------------------------------------------------------

func TestLegalMovesPinnedPiece(t *testing.T) {
	// White bishop on e2 is absolutely pinned by the black rook on e8 against
	// the white king on e1: it may only move along the e-file (here nowhere it
	// helps) so it has no legal moves at all.
	pos := mustParse(t, "4r3/8/8/8/8/8/4B3/4K3 w - - 0 1")
	got := legalMovesFrom(pos, mustSquare(t, "e2"))
	if len(got) != 0 {
		t.Errorf("pinned bishop should have no legal moves, got %v", got)
	}
	// Pseudo-legal generation still offers the (illegal) escape, confirming the
	// filter is what removes it.
	if !hasMove(PseudoLegalMoves(pos), "e2d3") {
		t.Fatal("precondition: pseudo-legal e2d3 expected")
	}
}

func TestLegalMovesPinnedRookSlidesAlongPin(t *testing.T) {
	// A rook pinned along the same file it could move on may still slide along
	// the pin line in both directions (staying between king and pinner) and
	// capture the pinner. White rook e4 pinned by black rook e8 against king e1:
	// every legal move stays on the e-file, up to and including the capture e8.
	pos := mustParse(t, "4r3/8/8/8/4R3/8/8/4K3 w - - 0 1")
	got := legalMovesFrom(pos, mustSquare(t, "e4"))
	want := []string{"e4e2", "e4e3", "e4e5", "e4e6", "e4e7", "e4e8"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pinned rook moves = %v, want %v", got, want)
	}
}

func TestLegalMovesMustBlockCheck(t *testing.T) {
	// White king e1 in check from the black rook on e8 down the open e-file. A
	// white rook sits on a4. Legal responses: block on the e-file (a4e4) or move
	// the king off it.
	pos := mustParse(t, "4r3/8/8/8/R7/8/8/4K3 w - - 0 1")
	if !hasLegal(pos, "a4e4") {
		t.Error("expected blocking move a4e4 to be legal")
	}
	// A rook move that does not block (e.g. a4a5) must be illegal.
	if hasLegal(pos, "a4a5") {
		t.Error("a4a5 does not address the check and must be illegal")
	}
	// The king may also step off the e-file.
	if !hasLegal(pos, "e1d1") || !hasLegal(pos, "e1f1") {
		t.Error("king should be able to step off the checked file")
	}
	if hasLegal(pos, "e1e2") {
		t.Error("king may not stay on the file attacked by the rook")
	}
}

func TestLegalMovesMustCaptureChecker(t *testing.T) {
	// White king on h1 is checked by a black knight on g3. The white rook on g8
	// can capture the knight (g8g3); other rook moves are illegal.
	pos := mustParse(t, "6R1/8/8/8/8/6n1/8/7K w - - 0 1")
	if !hasLegal(pos, "g8g3") {
		t.Error("expected the checking knight to be capturable via g8g3")
	}
	if hasLegal(pos, "g8a8") {
		t.Error("a rook move that ignores the check must be illegal")
	}
}

func TestLegalMovesKingCannotMoveIntoCheck(t *testing.T) {
	// White king e1 with a black rook controlling the d-file (d8). The king may
	// not step onto d1/d2 but may go to f1/f2/e2.
	pos := mustParse(t, "3r4/8/8/8/8/8/8/4K3 w - - 0 1")
	got := legalMovesFrom(pos, mustSquare(t, "e1"))
	want := []string{"e1e2", "e1f1", "e1f2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("king escape moves = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// ApplyMove mechanics.
// ---------------------------------------------------------------------------

func TestApplyMoveSimpleAndClocks(t *testing.T) {
	pos := mustParse(t, StartingFEN)
	next, err := ApplyMove(pos, Move{From: mustSquare(t, "e2"), To: mustSquare(t, "e4")})
	if err != nil {
		t.Fatalf("ApplyMove e2e4: %v", err)
	}
	// Original is untouched (immutability by convention): e2 still holds the pawn.
	if pos.PieceAt(mustSquare(t, "e2")).Type != Pawn {
		t.Error("ApplyMove mutated the source position (e2 emptied)")
	}
	// New position: pawn moved, side flipped, double-push EP target set on e3,
	// halfmove clock reset (pawn move), fullmove unchanged (White moved).
	if next.PieceAt(mustSquare(t, "e4")).Type != Pawn || !next.PieceAt(mustSquare(t, "e2")).IsEmpty() {
		t.Error("pawn was not moved e2->e4 in the new position")
	}
	if next.SideToMove != Black {
		t.Error("side to move should flip to Black")
	}
	if next.EnPassant == nil || *next.EnPassant != mustSquare(t, "e3") {
		t.Errorf("expected en-passant target e3, got %v", next.EnPassant)
	}
	if next.HalfMove != 0 {
		t.Errorf("halfmove clock should reset on a pawn move, got %d", next.HalfMove)
	}
	if next.FullMove != 1 {
		t.Errorf("fullmove should stay 1 after White's move, got %d", next.FullMove)
	}
}

func TestApplyMoveHalfmoveIncrementAndFullmove(t *testing.T) {
	// A quiet knight move by Black: halfmove clock increments, fullmove advances.
	pos := mustParse(t, "rnbqkbnr/pppppppp/8/8/8/5N2/PPPPPPPP/RNBQKB1R b KQkq - 1 1")
	next, err := ApplyMove(pos, Move{From: mustSquare(t, "g8"), To: mustSquare(t, "f6")})
	if err != nil {
		t.Fatalf("ApplyMove g8f6: %v", err)
	}
	if next.HalfMove != 2 {
		t.Errorf("halfmove clock = %d, want 2 (incremented on quiet move)", next.HalfMove)
	}
	if next.FullMove != 2 {
		t.Errorf("fullmove = %d, want 2 after Black's move", next.FullMove)
	}
	if next.SideToMove != White {
		t.Error("side to move should flip to White")
	}
}

func TestApplyMoveCaptureResetsClock(t *testing.T) {
	// White pawn on e4 captures the black pawn on d5; halfmove clock resets and
	// the captured pawn is gone.
	pos := mustParse(t, "8/8/8/3p4/4P3/8/8/8 w - - 5 9")
	next, err := ApplyMove(pos, Move{From: mustSquare(t, "e4"), To: mustSquare(t, "d5")})
	if err != nil {
		t.Fatalf("ApplyMove e4d5: %v", err)
	}
	if next.PieceAt(mustSquare(t, "d5")).Color != White {
		t.Error("white pawn should occupy d5 after the capture")
	}
	if next.HalfMove != 0 {
		t.Errorf("halfmove clock should reset on a capture, got %d", next.HalfMove)
	}
}

func TestApplyMoveEnPassantRemovesCorrectPawn(t *testing.T) {
	// White d5 captures e6 en passant; the captured black pawn is the one on e5,
	// not anything on e6.
	pos := mustParse(t, "8/8/8/3Pp3/8/8/8/8 w - e6 0 1")
	next, err := ApplyMove(pos, Move{From: mustSquare(t, "d5"), To: mustSquare(t, "e6")})
	if err != nil {
		t.Fatalf("ApplyMove d5e6 e.p.: %v", err)
	}
	if next.PieceAt(mustSquare(t, "e6")).Type != Pawn || next.PieceAt(mustSquare(t, "e6")).Color != White {
		t.Error("white pawn should land on e6")
	}
	if !next.PieceAt(mustSquare(t, "e5")).IsEmpty() {
		t.Error("the en-passant-captured black pawn on e5 should be removed")
	}
	if !next.PieceAt(mustSquare(t, "d5")).IsEmpty() {
		t.Error("d5 should be vacated")
	}
	if next.EnPassant != nil {
		t.Error("en-passant target should be cleared after the capture")
	}
}

func TestApplyMoveCastlingMovesRook(t *testing.T) {
	cases := []struct {
		name             string
		fen              string
		king             string // e.g. "e1g1"
		wantRookFrom     string
		wantRookTo       string
		wantKingSquare   string
		castleRightsGone CastlingRights
	}{
		{"white kingside", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1g1", "h1", "f1", "g1", WhiteKingside | WhiteQueenside},
		{"white queenside", "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1", "e1c1", "a1", "d1", "c1", WhiteKingside | WhiteQueenside},
		{"black kingside", "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1", "e8g8", "h8", "f8", "g8", BlackKingside | BlackQueenside},
		{"black queenside", "r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1", "e8c8", "a8", "d8", "c8", BlackKingside | BlackQueenside},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos := mustParse(t, c.fen)
			from := mustSquare(t, c.king[:2])
			to := mustSquare(t, c.king[2:])
			next, err := ApplyMove(pos, Move{From: from, To: to})
			if err != nil {
				t.Fatalf("ApplyMove %s: %v", c.king, err)
			}
			if next.PieceAt(mustSquare(t, c.wantKingSquare)).Type != King {
				t.Errorf("king should be on %s", c.wantKingSquare)
			}
			if !next.PieceAt(mustSquare(t, c.wantRookFrom)).IsEmpty() {
				t.Errorf("rook origin %s should be empty", c.wantRookFrom)
			}
			if next.PieceAt(mustSquare(t, c.wantRookTo)).Type != Rook {
				t.Errorf("rook should have hopped to %s", c.wantRookTo)
			}
			if next.Castling.Has(c.castleRightsGone) {
				t.Errorf("castling rights %v should be gone after castling", c.castleRightsGone)
			}
		})
	}
}

func TestApplyMovePromotionPlacesChosenPiece(t *testing.T) {
	for _, pt := range []PieceType{Queen, Rook, Bishop, Knight} {
		pos := mustParse(t, "8/4P3/8/8/8/8/8/4k1K1 w - - 0 1")
		next, err := ApplyMove(pos, Move{From: mustSquare(t, "e7"), To: mustSquare(t, "e8"), Promotion: pt})
		if err != nil {
			t.Fatalf("ApplyMove promotion to %v: %v", pt, err)
		}
		got := next.PieceAt(mustSquare(t, "e8"))
		if got.Type != pt || got.Color != White {
			t.Errorf("promotion square holds %+v, want type %v white", got, pt)
		}
		if !next.PieceAt(mustSquare(t, "e7")).IsEmpty() {
			t.Error("the pawn's origin square should be empty after promotion")
		}
	}
}

func TestApplyMoveRookMoveDropsItsRightOnly(t *testing.T) {
	// Moving the h1 rook removes only White's king-side right; queen-side and
	// both black rights survive.
	pos := mustParse(t, "r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	next, err := ApplyMove(pos, Move{From: mustSquare(t, "h1"), To: mustSquare(t, "h5")})
	if err != nil {
		t.Fatalf("ApplyMove h1h5: %v", err)
	}
	if next.Castling.Has(WhiteKingside) {
		t.Error("White king-side right should be gone after the h1 rook moves")
	}
	if !next.Castling.Has(WhiteQueenside) || !next.Castling.Has(BlackKingside) || !next.Castling.Has(BlackQueenside) {
		t.Errorf("other castling rights should survive, got %v", next.Castling)
	}
}

func TestApplyMoveCapturingRookRemovesEnemyRight(t *testing.T) {
	// A white rook capturing the black rook on h8 strips Black's king-side right.
	pos := mustParse(t, "r3k2r/7R/8/8/8/8/8/4K3 w kq - 0 1")
	next, err := ApplyMove(pos, Move{From: mustSquare(t, "h7"), To: mustSquare(t, "h8")})
	if err != nil {
		t.Fatalf("ApplyMove h7h8 (capture): %v", err)
	}
	if next.Castling.Has(BlackKingside) {
		t.Error("capturing the h8 rook should remove Black's king-side right")
	}
	if !next.Castling.Has(BlackQueenside) {
		t.Error("Black queen-side right should survive")
	}
}

func TestApplyMoveIllegalMoveErrors(t *testing.T) {
	pos := mustParse(t, StartingFEN)
	// A move that is not even pseudo-legal.
	if _, err := ApplyMove(pos, Move{From: mustSquare(t, "e2"), To: mustSquare(t, "e5")}); err == nil {
		t.Error("expected an error for the illegal jump e2e5")
	}
	// A promotion with no promotion piece specified does not match any legal
	// move and must error.
	promo := mustParse(t, "8/4P3/8/8/8/8/8/4k1K1 w - - 0 1")
	if _, err := ApplyMove(promo, Move{From: mustSquare(t, "e7"), To: mustSquare(t, "e8")}); err == nil {
		t.Error("expected an error for a promotion lacking a promotion piece")
	}
	// A move that leaves the king in check (pinned bishop) must error.
	pinned := mustParse(t, "4r3/8/8/8/8/8/4B3/4K3 w - - 0 1")
	if _, err := ApplyMove(pinned, Move{From: mustSquare(t, "e2"), To: mustSquare(t, "d3")}); err == nil {
		t.Error("expected an error for moving a pinned piece off the pin line")
	}
}

// ---------------------------------------------------------------------------
// Game result: checkmate, stalemate, ongoing.
// ---------------------------------------------------------------------------

func TestResultOngoing(t *testing.T) {
	pos := mustParse(t, StartingFEN)
	if r := Result(pos); r.Outcome != Ongoing || r.IsOver() {
		t.Errorf("start position should be Ongoing, got %+v", r)
	}
}

func TestResultFoolsMate(t *testing.T) {
	// Fool's mate: 1. f3 e5 2. g4 Qh4#. Black queen on h4 mates the white king.
	pos := mustParse(t, "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3")
	if !IsInCheck(pos, White) {
		t.Fatal("precondition: white king should be in check")
	}
	r := Result(pos)
	if r.Outcome != BlackWins || r.Reason != "checkmate" {
		t.Errorf("fool's mate should be BlackWins/checkmate, got %+v", r)
	}
}

func TestResultBackRankMateWhiteWins(t *testing.T) {
	// Black king on h8 mated by the white rook on a8; black king has no escape
	// (own pawns on g7/h7). White delivered mate, so White wins.
	pos := mustParse(t, "R5k1/5ppp/8/8/8/8/8/6K1 b - - 0 1")
	r := Result(pos)
	if r.Outcome != WhiteWins || r.Reason != "checkmate" {
		t.Errorf("back-rank mate should be WhiteWins/checkmate, got %+v", r)
	}
}

func TestResultStalemate(t *testing.T) {
	// Classic stalemate: black king on h8, white king f7, white queen g6. Black
	// is not in check but has no legal move.
	pos := mustParse(t, "7k/5K2/6Q1/8/8/8/8/8 b - - 0 1")
	if IsInCheck(pos, Black) {
		t.Fatal("precondition: black king should NOT be in check")
	}
	r := Result(pos)
	if r.Outcome != Draw || r.Reason != "stalemate" {
		t.Errorf("expected Draw/stalemate, got %+v", r)
	}
}

// ---------------------------------------------------------------------------
// perft: the strongest guard against move-generation / apply regressions.
// Reference node counts are the well-known published values.
// ---------------------------------------------------------------------------

// perft counts the number of leaf nodes in the full legal-move tree to the given
// depth. It exercises LegalMoves and applyMechanical together.
func perft(pos *Position, depth int) int {
	if depth == 0 {
		return 1
	}
	moves := LegalMoves(pos)
	if depth == 1 {
		return len(moves)
	}
	total := 0
	for _, m := range moves {
		total += perft(applyMechanical(pos, m), depth-1)
	}
	return total
}

func TestPerft(t *testing.T) {
	cases := []struct {
		name   string
		fen    string
		counts []int // index 0 -> depth 1, etc.
	}{
		// Standard start position.
		{"start", StartingFEN, []int{20, 400, 8902}},
		// "Kiwipete": castling, checks and a rich middlegame — depth 3.
		{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", []int{48, 2039, 97862}},
		// En-passant / promotion-rich endgame (chessprogramming position 3).
		{"ep-position-3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", []int{14, 191, 2812}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos := mustParse(t, c.fen)
			for i, want := range c.counts {
				depth := i + 1
				if got := perft(pos, depth); got != want {
					t.Errorf("perft(%q, %d) = %d, want %d", c.name, depth, got, want)
				}
			}
		})
	}
}
