package engine

import (
	"fmt"
)

// Mirror maps a file (or rank) coordinate to its board-symmetric partner,
// mirror(x) = 7-x. It is the geometric heart of the Rainbow variant: for every
// occupied square (x,y) the square (7-x,y) must hold the opposite color.
func Mirror(x int8) int8 { return 7 - x }

// CastlingRights is a bitmask of the four castling possibilities.
type CastlingRights uint8

const (
	WhiteKingside CastlingRights = 1 << iota
	WhiteQueenside
	BlackKingside
	BlackQueenside
)

// Has reports whether the given right is present.
func (cr CastlingRights) Has(r CastlingRights) bool { return cr&r != 0 }

// With returns a copy of the rights with r added.
func (cr CastlingRights) With(r CastlingRights) CastlingRights { return cr | r }

// Without returns a copy of the rights with r removed.
func (cr CastlingRights) Without(r CastlingRights) CastlingRights { return cr &^ r }

// Position is a full chess position. By convention it is treated as immutable:
// ApplyMove returns a new *Position rather than mutating the receiver, which
// keeps legality testing and history tracking simple.
type Position struct {
	Board      [64]Piece
	SideToMove Color
	Castling   CastlingRights
	EnPassant  *Square // target square behind a pawn that just double-pushed; nil if none
	HalfMove   int     // halfmove clock (for the 50-move rule)
	FullMove   int     // starts at 1, increments after Black moves
}

// NewPosition returns an empty position with White to move and move number 1.
func NewPosition() *Position {
	return &Position{SideToMove: White, FullMove: 1}
}

// PieceAt returns the piece on the given square.
func (p *Position) PieceAt(sq Square) Piece { return p.Board[sq.Index()] }

// SetPiece places a piece on the given square (use a None-typed Piece to clear).
func (p *Position) SetPiece(sq Square, piece Piece) { p.Board[sq.Index()] = piece }

// Clone returns a deep copy of the position, including an independent copy of
// the en-passant target so the original is never aliased.
func (p *Position) Clone() *Position {
	c := *p
	if p.EnPassant != nil {
		ep := *p.EnPassant
		c.EnPassant = &ep
	}
	return &c
}

// String renders the position as its FEN string (handy for debugging output).
func (p *Position) String() string { return p.FEN() }

// ParseSquare parses algebraic coordinates like "e4" into a Square.
func ParseSquare(s string) (Square, error) {
	if len(s) != 2 {
		return Square{}, fmt.Errorf("invalid square %q: want 2 chars", s)
	}
	file := int8(s[0] - 'a')
	rank := int8(s[1] - '1')
	sq := Square{File: file, Rank: rank}
	if !sq.Valid() {
		return Square{}, fmt.Errorf("invalid square %q: out of range", s)
	}
	return sq, nil
}

// String renders a Square in algebraic notation (e.g. "e4").
func (s Square) String() string {
	if !s.Valid() {
		return "??"
	}
	return string(rune('a'+s.File)) + string(rune('1'+s.Rank))
}

// String renders a Move in long algebraic / UCI notation (e.g. "e2e4", "e7e8q").
func (m Move) String() string {
	out := m.From.String() + m.To.String()
	if m.Promotion != None {
		out += string(pieceTypeToFENChar(m.Promotion, Black)) // lowercase promotion suffix
	}
	return out
}
