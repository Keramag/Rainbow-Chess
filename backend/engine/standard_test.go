package engine

import (
	"reflect"
	"testing"
)

func TestStandardNameAndInitialPosition(t *testing.T) {
	s := NewStandard()
	if s.Name() != "standard" {
		t.Errorf("Name() = %q, want %q", s.Name(), "standard")
	}
	if got := s.InitialPosition().FEN(); got != StartingFEN {
		t.Errorf("InitialPosition().FEN() = %q, want %q", got, StartingFEN)
	}
}

func TestStandardPromotionPieces(t *testing.T) {
	s := NewStandard()
	want := []PieceType{Queen, Rook, Bishop, Knight}
	if got := s.PromotionPieces(); !reflect.DeepEqual(got, want) {
		t.Errorf("PromotionPieces() = %v, want %v", got, want)
	}
}

// TestStandardShortGame plays a few opening plies through the variant's own
// ApplyMove/LegalMoves and checks the position (and bookkeeping) after each ply.
func TestStandardShortGame(t *testing.T) {
	s := NewStandard()
	pos := s.InitialPosition()

	plies := []struct {
		from, to string
		wantFEN  string
	}{
		{"e2", "e4", "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1"},
		{"e7", "e5", "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2"},
		{"g1", "f3", "rnbqkbnr/pppp1ppp/8/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R b KQkq - 1 2"},
		{"b8", "c6", "r1bqkbnr/pppp1ppp/2n5/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3"},
	}

	for i, ply := range plies {
		move := Move{From: mustSquare(t, ply.from), To: mustSquare(t, ply.to)}
		next, err := s.ApplyMove(pos, move)
		if err != nil {
			t.Fatalf("ply %d %s%s: ApplyMove: %v", i, ply.from, ply.to, err)
		}
		if got := next.FEN(); got != ply.wantFEN {
			t.Fatalf("ply %d %s%s: FEN = %q, want %q", i, ply.from, ply.to, got, ply.wantFEN)
		}
		if r := s.Result(next); r.IsOver() {
			t.Fatalf("ply %d: game unexpectedly over: %+v", i, r)
		}
		pos = next
	}
}

// TestStandardPromotionToAllFour confirms every piece type Standard advertises
// can actually be promoted to via the variant's ApplyMove.
func TestStandardPromotionToAllFour(t *testing.T) {
	s := NewStandard()
	// White pawn on a7 ready to promote; both kings present and out of reach.
	pos := mustParse(t, "8/P7/8/8/8/8/8/k6K w - - 0 1")

	for _, promo := range s.PromotionPieces() {
		move := Move{From: mustSquare(t, "a7"), To: mustSquare(t, "a8"), Promotion: promo}
		next, err := s.ApplyMove(pos, move)
		if err != nil {
			t.Fatalf("promote to %s: ApplyMove: %v", promo, err)
		}
		placed := next.PieceAt(mustSquare(t, "a8"))
		if placed.Type != promo || placed.Color != White {
			t.Errorf("promote to %s: a8 holds %+v, want white %s", promo, placed, promo)
		}
	}
}

// TestVariantApplyMoveRejectsDisallowedPromotion exercises the promotion
// whitelist directly: a variant whose PromotionPieces omits a type must reject a
// move promoting to it, even though the engine considers the push itself legal.
func TestVariantApplyMoveRejectsDisallowedPromotion(t *testing.T) {
	// Same shape as NewStandard but only the queen is allowed.
	queenOnly := &Standard{name: "queen-only", promotions: []PieceType{Queen}}
	pos := mustParse(t, "8/P7/8/8/8/8/8/k6K w - - 0 1")

	// Queen promotion is allowed.
	if _, err := queenOnly.ApplyMove(pos, Move{From: mustSquare(t, "a7"), To: mustSquare(t, "a8"), Promotion: Queen}); err != nil {
		t.Fatalf("queen promotion should be allowed: %v", err)
	}
	// Knight promotion is not in the whitelist and must be rejected.
	for _, promo := range []PieceType{Rook, Bishop, Knight} {
		move := Move{From: mustSquare(t, "a7"), To: mustSquare(t, "a8"), Promotion: promo}
		if _, err := queenOnly.ApplyMove(pos, move); err == nil {
			t.Errorf("promotion to %s should be rejected by queen-only variant", promo)
		}
	}
}

// TestStandardApplyMoveRejectsIllegalMove confirms the variant surfaces the
// engine's illegal-move error rather than applying a bogus move.
func TestStandardApplyMoveRejectsIllegalMove(t *testing.T) {
	s := NewStandard()
	pos := s.InitialPosition()
	// e2-e5 is not a legal pawn move.
	if _, err := s.ApplyMove(pos, Move{From: mustSquare(t, "e2"), To: mustSquare(t, "e5")}); err == nil {
		t.Error("ApplyMove(e2e5) = nil error, want illegal-move error")
	}
}
