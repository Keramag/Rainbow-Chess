package engine

// movegen.go produces pseudo-legal moves: every move a piece could make under
// the movement rules, WITHOUT yet checking whether it leaves the mover's own
// king in check. That final legality filter lives in LegalMoves (see Task 4);
// keeping the two stages separate is the classic chess-engine structure and
// makes both easy to test.
//
// Like the rest of the engine the generator is colour-relative: pawn push
// direction, double-push start rank and promotion rank are all derived from the
// pawn's Color (never from which half of the board it sits on), which is exactly
// what lets the same generator serve the colour-mixed Rainbow variant unchanged.
//
// One exception to the "no check filtering here" rule is castling: the
// not-out-of / not-through / not-into-check requirement is part of the castling
// move's definition, so it is enforced during generation.

// queenDirs are the eight ray directions of a queen (bishop + rook combined).
var queenDirs = [8][2]int8{
	{1, 1}, {1, -1}, {-1, 1}, {-1, -1},
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
}

// promotionTypes lists the pieces a pawn may promote to, in conventional
// strength order. The generator always expands a promotion into all four; a
// variant that restricts promotion (e.g. Rainbow → {Knight, Bishop}) enforces
// that separately via PromotionPieces and ApplyMove rejection.
var promotionTypes = [4]PieceType{Queen, Rook, Bishop, Knight}

// PseudoLegalMoves returns every pseudo-legal move for the side to move in pos.
// Moves that would leave the mover's own king in check are NOT filtered out here
// (LegalMoves does that); castling, however, already excludes check violations.
func PseudoLegalMoves(pos *Position) []Move {
	moves := make([]Move, 0, 48)
	us := pos.SideToMove
	for i := 0; i < 64; i++ {
		p := pos.Board[i]
		if p.IsEmpty() || p.Color != us {
			continue
		}
		from := SquareFromIndex(i)
		switch p.Type {
		case Pawn:
			moves = genPawnMoves(pos, from, us, moves)
		case Knight:
			moves = genKnightMoves(pos, from, us, moves)
		case Bishop:
			moves = genSlidingMoves(pos, from, us, bishopDirs[:], moves)
		case Rook:
			moves = genSlidingMoves(pos, from, us, rookDirs[:], moves)
		case Queen:
			moves = genSlidingMoves(pos, from, us, queenDirs[:], moves)
		case King:
			moves = genKingMoves(pos, from, us, moves)
		}
	}
	return moves
}

// genPawnMoves appends a pawn's pushes, captures, en-passant capture and
// promotions. Direction and the start/promotion ranks come from the pawn color.
func genPawnMoves(pos *Position, from Square, us Color, moves []Move) []Move {
	fwd := pawnForward(us)
	promoRank := pawnPromotionRank(us)

	// Single push onto an empty square.
	one := Square{File: from.File, Rank: from.Rank + fwd}
	if one.Valid() && pos.PieceAt(one).IsEmpty() {
		if one.Rank == promoRank {
			moves = appendPromotions(moves, from, one)
		} else {
			moves = append(moves, Move{From: from, To: one})
		}
		// Double push from the start rank, only if the intermediate square
		// (the single-push square above) was also empty.
		if from.Rank == pawnStartRank(us) {
			two := Square{File: from.File, Rank: from.Rank + 2*fwd}
			if two.Valid() && pos.PieceAt(two).IsEmpty() {
				moves = append(moves, Move{From: from, To: two, IsDoublePush: true})
			}
		}
	}

	// Diagonal captures (including en passant).
	for _, df := range [2]int8{-1, 1} {
		to := Square{File: from.File + df, Rank: from.Rank + fwd}
		if !to.Valid() {
			continue
		}
		target := pos.PieceAt(to)
		switch {
		case !target.IsEmpty() && target.Color != us:
			if to.Rank == promoRank {
				moves = appendPromotions(moves, from, to)
			} else {
				moves = append(moves, Move{From: from, To: to})
			}
		case target.IsEmpty() && pos.EnPassant != nil && to == *pos.EnPassant:
			// En passant: the captured pawn sits beside us, on the
			// destination file but the mover's own rank. Require it to be a
			// real enemy pawn so a stray FEN en-passant flag can't conjure
			// an illegal capture.
			capSq := Square{File: to.File, Rank: from.Rank}
			cap := pos.PieceAt(capSq)
			if cap.Type == Pawn && cap.Color != us {
				moves = append(moves, Move{From: from, To: to, IsEnPassant: true})
			}
		}
	}
	return moves
}

// appendPromotions expands a single pawn move into the four promotion choices.
func appendPromotions(moves []Move, from, to Square) []Move {
	for _, pt := range promotionTypes {
		moves = append(moves, Move{From: from, To: to, Promotion: pt})
	}
	return moves
}

// genKnightMoves appends a knight's L-shaped moves onto empty or enemy squares.
func genKnightMoves(pos *Position, from Square, us Color, moves []Move) []Move {
	for _, off := range knightOffsets {
		to := Square{File: from.File + off[0], Rank: from.Rank + off[1]}
		if !to.Valid() {
			continue
		}
		target := pos.PieceAt(to)
		if target.IsEmpty() || target.Color != us {
			moves = append(moves, Move{From: from, To: to})
		}
	}
	return moves
}

// genSlidingMoves appends moves for a sliding piece along the given directions,
// stopping a ray at the first occupied square (capturing it if it is an enemy).
func genSlidingMoves(pos *Position, from Square, us Color, dirs [][2]int8, moves []Move) []Move {
	for _, d := range dirs {
		cur := Square{File: from.File + d[0], Rank: from.Rank + d[1]}
		for cur.Valid() {
			target := pos.PieceAt(cur)
			if target.IsEmpty() {
				moves = append(moves, Move{From: from, To: cur})
			} else {
				if target.Color != us {
					moves = append(moves, Move{From: from, To: cur})
				}
				break // any piece blocks the rest of the ray
			}
			cur = Square{File: cur.File + d[0], Rank: cur.Rank + d[1]}
		}
	}
	return moves
}

// genKingMoves appends the king's one-square steps plus any available castling.
func genKingMoves(pos *Position, from Square, us Color, moves []Move) []Move {
	for _, off := range kingOffsets {
		to := Square{File: from.File + off[0], Rank: from.Rank + off[1]}
		if !to.Valid() {
			continue
		}
		target := pos.PieceAt(to)
		if target.IsEmpty() || target.Color != us {
			moves = append(moves, Move{From: from, To: to})
		}
	}
	return genCastlingMoves(pos, from, us, moves)
}

// genCastlingMoves appends king-side / queen-side castling when legal: the right
// is present, the king is on its home square, the squares between king and rook
// are empty, the rook is in place, and the king neither starts in check nor
// passes through or lands on an attacked square. Castling squares are the
// standard ones for the color's back rank (king e-file, rooks a/h files).
func genCastlingMoves(pos *Position, from Square, us Color, moves []Move) []Move {
	var backRank int8
	var ksRight, qsRight CastlingRights
	if us == White {
		backRank = 0
		ksRight, qsRight = WhiteKingside, WhiteQueenside
	} else {
		backRank = 7
		ksRight, qsRight = BlackKingside, BlackQueenside
	}

	// The king must sit on its home square (e-file of the back rank).
	if from.File != 4 || from.Rank != backRank {
		return moves
	}
	them := us.Opposite()
	// A king may not castle out of check.
	if IsSquareAttacked(pos, from, them) {
		return moves
	}

	// King side: f and g empty, rook on h, king passes f and g safely.
	if pos.Castling.Has(ksRight) {
		f := Square{File: 5, Rank: backRank}
		g := Square{File: 6, Rank: backRank}
		rook := pos.PieceAt(Square{File: 7, Rank: backRank})
		if pos.PieceAt(f).IsEmpty() && pos.PieceAt(g).IsEmpty() &&
			rook.Type == Rook && rook.Color == us &&
			!IsSquareAttacked(pos, f, them) && !IsSquareAttacked(pos, g, them) {
			moves = append(moves, Move{From: from, To: g, IsCastle: true})
		}
	}

	// Queen side: b, c and d empty, rook on a, king passes d and c safely.
	// The b-file square need only be empty (the king never stands on it).
	if pos.Castling.Has(qsRight) {
		b := Square{File: 1, Rank: backRank}
		c := Square{File: 2, Rank: backRank}
		d := Square{File: 3, Rank: backRank}
		rook := pos.PieceAt(Square{File: 0, Rank: backRank})
		if pos.PieceAt(b).IsEmpty() && pos.PieceAt(c).IsEmpty() && pos.PieceAt(d).IsEmpty() &&
			rook.Type == Rook && rook.Color == us &&
			!IsSquareAttacked(pos, d, them) && !IsSquareAttacked(pos, c, them) {
			moves = append(moves, Move{From: from, To: c, IsCastle: true})
		}
	}
	return moves
}

// pawnStartRank returns the rank a pawn of color c starts on (and may
// double-push from): rank 1 for White, rank 6 for Black.
func pawnStartRank(c Color) int8 {
	if c == White {
		return 1
	}
	return 6
}

// pawnPromotionRank returns the rank a pawn of color c promotes on: rank 7 for
// White, rank 0 for Black.
func pawnPromotionRank(c Color) int8 {
	if c == White {
		return 7
	}
	return 0
}
