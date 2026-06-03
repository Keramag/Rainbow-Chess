package engine

// attacks.go implements square-attack detection: the question "is square sq
// attacked by any piece of color byColor?". This single primitive drives check
// detection, checkmate/stalemate, castling-through-check rules, and the legal-
// move pin filter. Like the rest of the engine it is colour-relative (pawn
// attack direction depends on the pawn's Color, never on which half of the
// board it occupies) so it works unchanged for the colour-mixed Rainbow variant.

// knightOffsets are the eight (file, rank) deltas of a knight's move.
var knightOffsets = [8][2]int8{
	{1, 2}, {2, 1}, {2, -1}, {1, -2},
	{-1, -2}, {-2, -1}, {-2, 1}, {-1, 2},
}

// kingOffsets are the eight (file, rank) deltas of a king's move.
var kingOffsets = [8][2]int8{
	{1, 0}, {1, 1}, {0, 1}, {-1, 1},
	{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
}

// bishopDirs are the four diagonal ray directions.
var bishopDirs = [4][2]int8{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

// rookDirs are the four orthogonal ray directions.
var rookDirs = [4][2]int8{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

// IsSquareAttacked reports whether square sq is attacked by any piece of color
// byColor in the given position. The square itself need not be occupied; this
// is a pure geometric query used by check/castling logic. Blocking pieces stop
// sliding attacks along a ray.
func IsSquareAttacked(pos *Position, sq Square, byColor Color) bool {
	if !sq.Valid() {
		return false
	}

	// Pawn attacks. A pawn of byColor attacks diagonally in its forward
	// direction (White toward higher ranks, Black toward lower ranks). So an
	// attacking pawn sits one rank "behind" sq relative to that direction.
	pawnRank := sq.Rank - pawnForward(byColor)
	for _, df := range [2]int8{-1, 1} {
		from := Square{File: sq.File + df, Rank: pawnRank}
		if from.Valid() {
			p := pos.PieceAt(from)
			if p.Type == Pawn && p.Color == byColor {
				return true
			}
		}
	}

	// Knight attacks.
	for _, off := range knightOffsets {
		from := Square{File: sq.File + off[0], Rank: sq.Rank + off[1]}
		if from.Valid() {
			p := pos.PieceAt(from)
			if p.Type == Knight && p.Color == byColor {
				return true
			}
		}
	}

	// King attacks (adjacent squares).
	for _, off := range kingOffsets {
		from := Square{File: sq.File + off[0], Rank: sq.Rank + off[1]}
		if from.Valid() {
			p := pos.PieceAt(from)
			if p.Type == King && p.Color == byColor {
				return true
			}
		}
	}

	// Diagonal sliders: bishop or queen.
	if rayHitsAttacker(pos, sq, byColor, bishopDirs[:], Bishop) {
		return true
	}
	// Orthogonal sliders: rook or queen.
	if rayHitsAttacker(pos, sq, byColor, rookDirs[:], Rook) {
		return true
	}

	return false
}

// rayHitsAttacker walks each direction from sq until it leaves the board or
// hits a piece. If the first piece encountered along a ray belongs to byColor
// and is either the given slider type or a Queen, the square is attacked.
func rayHitsAttacker(pos *Position, sq Square, byColor Color, dirs [][2]int8, slider PieceType) bool {
	for _, d := range dirs {
		cur := Square{File: sq.File + d[0], Rank: sq.Rank + d[1]}
		for cur.Valid() {
			p := pos.PieceAt(cur)
			if !p.IsEmpty() {
				if p.Color == byColor && (p.Type == slider || p.Type == Queen) {
					return true
				}
				break // any piece blocks the ray
			}
			cur = Square{File: cur.File + d[0], Rank: cur.Rank + d[1]}
		}
	}
	return false
}

// pawnForward returns the rank delta of a forward pawn step for the color:
// +1 for White (toward higher ranks), -1 for Black. This is the single source
// of truth for pawn directionality across the engine.
func pawnForward(c Color) int8 {
	if c == White {
		return 1
	}
	return -1
}

// KingSquare returns the square of color's king and true if found. If the king
// is absent (which a well-formed position never is) it returns the zero square
// and false.
func KingSquare(pos *Position, color Color) (Square, bool) {
	for i := 0; i < 64; i++ {
		p := pos.Board[i]
		if p.Type == King && p.Color == color {
			return SquareFromIndex(i), true
		}
	}
	return Square{}, false
}

// IsInCheck reports whether color's king is currently attacked by the opponent.
// A position with no king of that color is treated as not in check.
func IsInCheck(pos *Position, color Color) bool {
	ksq, ok := KingSquare(pos, color)
	if !ok {
		return false
	}
	return IsSquareAttacked(pos, ksq, color.Opposite())
}
