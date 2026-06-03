package engine

import "fmt"

// legal.go closes the loop on the rules core. It turns the pseudo-legal moves
// from movegen.go into fully legal moves (those that do not leave the mover's
// own king in check), applies a move to produce the next Position, and derives
// the high-level game result (checkmate / stalemate / ongoing).
//
// Move legality follows the classic two-stage structure: generate pseudo-legal
// moves, then for each one apply it and ask "is my king attacked?". The same
// apply-and-test step also implements pins, must-block / must-capture and
// move-the-king-out-of-check, so no special pin logic is needed anywhere.
//
// Position is immutable by convention: applying a move always returns a fresh
// *Position (via Clone) and never mutates the receiver.

// LegalMoves returns every legal move for the side to move: the pseudo-legal
// moves with any that would leave the mover's own king in check removed.
func LegalMoves(pos *Position) []Move {
	pseudo := PseudoLegalMoves(pos)
	us := pos.SideToMove
	legal := make([]Move, 0, len(pseudo))
	for _, m := range pseudo {
		next := applyMechanical(pos, m)
		if !IsInCheck(next, us) {
			legal = append(legal, m)
		}
	}
	return legal
}

// ApplyMove validates move against the legal moves of pos and, if legal, returns
// the resulting new Position. The caller need only supply From, To and (for a
// promotion) Promotion — the special-move flags (castle / en-passant / double
// push) are taken from the engine's own canonical move, so a bare
// {from,to,promotion} coming off the wire is applied with the correct mechanics.
// An illegal (or unrecognised) move yields a non-nil error and a nil Position.
func ApplyMove(pos *Position, move Move) (*Position, error) {
	for _, lm := range LegalMoves(pos) {
		if lm.From == move.From && lm.To == move.To && lm.Promotion == move.Promotion {
			return applyMechanical(pos, lm), nil
		}
	}
	return nil, fmt.Errorf("illegal move %s", move.String())
}

// applyMechanical applies m to pos and returns the new Position WITHOUT checking
// legality. It trusts the special-move flags on m (which the engine's generators
// always set correctly) and is used both by LegalMoves' apply-and-test filter
// and, via a canonical lookup, by ApplyMove.
func applyMechanical(pos *Position, m Move) *Position {
	next := pos.Clone()
	moving := pos.PieceAt(m.From)
	us := moving.Color

	captured := pos.PieceAt(m.To)
	isCapture := !captured.IsEmpty() || m.IsEnPassant

	// Lift the moving piece off its origin square.
	next.SetPiece(m.From, Piece{})

	// En passant: remove the pawn that sits beside the mover, on the
	// destination file but the mover's own (origin) rank.
	if m.IsEnPassant {
		next.SetPiece(Square{File: m.To.File, Rank: m.From.Rank}, Piece{})
	}

	// Place the piece on its destination, promoting if requested.
	placed := moving
	if m.Promotion != None {
		placed = Piece{Type: m.Promotion, Color: us}
	}
	next.SetPiece(m.To, placed)

	// Castling: hop the rook to the far side of the king. Files are the
	// standard ones; the rank is the king's (and rook's) back rank.
	if m.IsCastle {
		backRank := m.From.Rank
		var rookFrom, rookTo Square
		if m.To.File == 6 { // king side: rook h -> f
			rookFrom = Square{File: 7, Rank: backRank}
			rookTo = Square{File: 5, Rank: backRank}
		} else { // queen side (To.File == 2): rook a -> d
			rookFrom = Square{File: 0, Rank: backRank}
			rookTo = Square{File: 3, Rank: backRank}
		}
		next.SetPiece(rookTo, pos.PieceAt(rookFrom))
		next.SetPiece(rookFrom, Piece{})
	}

	// En-passant target: set only for a double push (the square jumped over),
	// otherwise cleared.
	next.EnPassant = nil
	if m.IsDoublePush {
		ep := Square{File: m.From.File, Rank: (m.From.Rank + m.To.Rank) / 2}
		next.EnPassant = &ep
	}

	// Castling rights: a king move drops both of the mover's rights; a rook
	// leaving (m.From) or being captured on (m.To) its home square drops the
	// matching right.
	next.Castling = pos.Castling
	if moving.Type == King {
		if us == White {
			next.Castling = next.Castling.Without(WhiteKingside).Without(WhiteQueenside)
		} else {
			next.Castling = next.Castling.Without(BlackKingside).Without(BlackQueenside)
		}
	}
	next.Castling = next.Castling.Without(rookHomeRight(m.From)).Without(rookHomeRight(m.To))

	// Halfmove clock resets on a pawn move or any capture, else increments.
	if moving.Type == Pawn || isCapture {
		next.HalfMove = 0
	} else {
		next.HalfMove = pos.HalfMove + 1
	}

	// Fullmove number increments after Black completes a move.
	if us == Black {
		next.FullMove = pos.FullMove + 1
	}

	next.SideToMove = us.Opposite()
	return next
}

// rookHomeRight returns the castling right associated with a rook's home square,
// or 0 if sq is not a rook home square. Removing a 0 right is a no-op, so this is
// safe to call for every move's From and To squares. It keys purely off the
// square: if a right is still present, the relevant rook is by definition still
// on that square, so checking the piece type is unnecessary.
func rookHomeRight(sq Square) CastlingRights {
	switch {
	case sq.File == 0 && sq.Rank == 0:
		return WhiteQueenside
	case sq.File == 7 && sq.Rank == 0:
		return WhiteKingside
	case sq.File == 0 && sq.Rank == 7:
		return BlackQueenside
	case sq.File == 7 && sq.Rank == 7:
		return BlackKingside
	}
	return 0
}

// Result reports the high-level outcome of pos. With no legal moves the side to
// move is either checkmated (it is in check → the opponent wins) or stalemated
// (not in check → a draw). Otherwise the game is ongoing.
func Result(pos *Position) GameResult {
	if len(LegalMoves(pos)) > 0 {
		return GameResult{Outcome: Ongoing}
	}
	if IsInCheck(pos, pos.SideToMove) {
		if pos.SideToMove == White {
			return GameResult{Outcome: BlackWins, Reason: "checkmate"}
		}
		return GameResult{Outcome: WhiteWins, Reason: "checkmate"}
	}
	return GameResult{Outcome: Draw, Reason: "stalemate"}
}
