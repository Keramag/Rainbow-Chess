// Package engine implements the rules core of the Rainbow-Chess platform.
//
// The engine is deliberately variant-agnostic: pawn direction, start ranks and
// promotion ranks are all derived from a piece's Color rather than from which
// half of the board it sits on. This is what allows the same move generator to
// serve both Standard chess and the colour-mixed Rainbow variant without any
// special-casing. See variant.go for the pluggable Variant abstraction built on
// top of these primitives.
package engine

// Color identifies which side a piece belongs to and whose turn it is.
type Color int8

const (
	White Color = iota
	Black
)

// Opposite returns the other color.
func (c Color) Opposite() Color {
	if c == White {
		return Black
	}
	return White
}

// String renders the color as "white"/"black".
func (c Color) String() string {
	if c == White {
		return "white"
	}
	return "black"
}

// PieceType enumerates the kinds of pieces; None marks an empty square.
type PieceType int8

const (
	None PieceType = iota
	Pawn
	Knight
	Bishop
	Rook
	Queen
	King
)

// String renders the piece type as a lower-case English name ("pawn", "knight",
// …); the empty type renders as "none". Handy for error messages and logging.
func (t PieceType) String() string {
	switch t {
	case Pawn:
		return "pawn"
	case Knight:
		return "knight"
	case Bishop:
		return "bishop"
	case Rook:
		return "rook"
	case Queen:
		return "queen"
	case King:
		return "king"
	default:
		return "none"
	}
}

// Piece is a colored piece occupying a square. The zero value (Type == None) is
// an empty square; its Color is meaningless in that case.
type Piece struct {
	Type  PieceType
	Color Color
}

// IsEmpty reports whether the square holds no piece.
func (p Piece) IsEmpty() bool { return p.Type == None }

// Square is a board coordinate. File 0-7 maps to files a-h; Rank 0-7 maps to
// ranks 1-8. Rank 0 is White's back rank, Rank 7 is Black's back rank.
type Square struct {
	File int8 // 0-7 (a-h)
	Rank int8 // 0-7 (1-8)
}

// Sq is a convenience constructor for a Square from file/rank ints.
func Sq(file, rank int) Square { return Square{File: int8(file), Rank: int8(rank)} }

// Index returns the 0-63 board-array index for the square (Rank*8 + File).
func (s Square) Index() int { return int(s.Rank)*8 + int(s.File) }

// Valid reports whether the square lies on the board.
func (s Square) Valid() bool {
	return s.File >= 0 && s.File < 8 && s.Rank >= 0 && s.Rank < 8
}

// SquareFromIndex is the inverse of Index for a 0-63 board-array index.
func SquareFromIndex(i int) Square {
	return Square{File: int8(i % 8), Rank: int8(i / 8)}
}

// Move describes a single ply. Promotion is None unless the move promotes a
// pawn. The boolean flags mark special moves so ApplyMove can perform the rook
// hop, the en-passant capture, and en-passant target bookkeeping respectively.
type Move struct {
	From         Square
	To           Square
	Promotion    PieceType // None unless this is a promotion
	IsCastle     bool
	IsEnPassant  bool
	IsDoublePush bool
}

// Outcome is the high-level result of a game.
type Outcome int8

const (
	Ongoing Outcome = iota
	WhiteWins
	BlackWins
	Draw
)

// GameResult pairs an Outcome with a human-readable reason (e.g. "checkmate",
// "stalemate", "resignation", "timeout").
type GameResult struct {
	Outcome Outcome
	Reason  string
}

// IsOver reports whether the game has finished.
func (r GameResult) IsOver() bool { return r.Outcome != Ongoing }
