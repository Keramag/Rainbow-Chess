package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// StartingFEN is the standard chess starting position.
const StartingFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

// pieceTypeToFENChar returns the FEN letter for a piece type, upper-case for
// White and lower-case for Black.
func pieceTypeToFENChar(t PieceType, c Color) byte {
	var ch byte
	switch t {
	case Pawn:
		ch = 'p'
	case Knight:
		ch = 'n'
	case Bishop:
		ch = 'b'
	case Rook:
		ch = 'r'
	case Queen:
		ch = 'q'
	case King:
		ch = 'k'
	default:
		return '?'
	}
	if c == White {
		ch -= 'a' - 'A' // to upper-case
	}
	return ch
}

// fenCharToPiece is the inverse of pieceTypeToFENChar.
func fenCharToPiece(ch byte) (Piece, error) {
	color := White
	lower := ch
	if ch >= 'a' && ch <= 'z' {
		color = Black
	} else if ch >= 'A' && ch <= 'Z' {
		lower = ch + ('a' - 'A')
	} else {
		return Piece{}, fmt.Errorf("invalid piece char %q", string(ch))
	}
	var t PieceType
	switch lower {
	case 'p':
		t = Pawn
	case 'n':
		t = Knight
	case 'b':
		t = Bishop
	case 'r':
		t = Rook
	case 'q':
		t = Queen
	case 'k':
		t = King
	default:
		return Piece{}, fmt.Errorf("invalid piece char %q", string(ch))
	}
	return Piece{Type: t, Color: color}, nil
}

// ParseFEN parses a FEN string into a Position. It accepts the standard 6-field
// form; the halfmove and fullmove fields may be omitted (defaulting to 0 and 1).
func ParseFEN(fen string) (*Position, error) {
	fields := strings.Fields(fen)
	if len(fields) != 4 && len(fields) != 6 {
		return nil, fmt.Errorf("invalid FEN: want 4 or 6 fields, got %d", len(fields))
	}

	pos := &Position{FullMove: 1}

	// Field 1: piece placement, ranks 8 down to 1.
	ranks := strings.Split(fields[0], "/")
	if len(ranks) != 8 {
		return nil, fmt.Errorf("invalid FEN board: want 8 ranks, got %d", len(ranks))
	}
	for r := 0; r < 8; r++ {
		rank := int8(7 - r) // first FEN rank string is rank 8 (index 7)
		file := int8(0)
		for i := 0; i < len(ranks[r]); i++ {
			ch := ranks[r][i]
			if ch >= '1' && ch <= '8' {
				file += int8(ch - '0')
				if file > 8 {
					return nil, fmt.Errorf("invalid FEN rank %q: overflows 8 files", ranks[r])
				}
				continue
			}
			if file >= 8 {
				return nil, fmt.Errorf("invalid FEN rank %q: too many squares", ranks[r])
			}
			piece, err := fenCharToPiece(ch)
			if err != nil {
				return nil, fmt.Errorf("invalid FEN rank %q: %w", ranks[r], err)
			}
			pos.SetPiece(Square{File: file, Rank: rank}, piece)
			file++
		}
		if file != 8 {
			return nil, fmt.Errorf("invalid FEN rank %q: covers %d files, want 8", ranks[r], file)
		}
	}

	// Field 2: side to move.
	switch fields[1] {
	case "w":
		pos.SideToMove = White
	case "b":
		pos.SideToMove = Black
	default:
		return nil, fmt.Errorf("invalid FEN side to move %q", fields[1])
	}

	// Field 3: castling rights.
	if fields[2] != "-" {
		for i := 0; i < len(fields[2]); i++ {
			switch fields[2][i] {
			case 'K':
				pos.Castling = pos.Castling.With(WhiteKingside)
			case 'Q':
				pos.Castling = pos.Castling.With(WhiteQueenside)
			case 'k':
				pos.Castling = pos.Castling.With(BlackKingside)
			case 'q':
				pos.Castling = pos.Castling.With(BlackQueenside)
			default:
				return nil, fmt.Errorf("invalid FEN castling field %q", fields[2])
			}
		}
	}

	// Field 4: en-passant target.
	if fields[3] != "-" {
		sq, err := ParseSquare(fields[3])
		if err != nil {
			return nil, fmt.Errorf("invalid FEN en-passant target: %w", err)
		}
		pos.EnPassant = &sq
	}

	// Fields 5 & 6: clocks (optional).
	if len(fields) == 6 {
		half, err := strconv.Atoi(fields[4])
		if err != nil || half < 0 {
			return nil, fmt.Errorf("invalid FEN halfmove clock %q", fields[4])
		}
		full, err := strconv.Atoi(fields[5])
		if err != nil || full < 1 {
			return nil, fmt.Errorf("invalid FEN fullmove number %q", fields[5])
		}
		pos.HalfMove = half
		pos.FullMove = full
	}

	return pos, nil
}

// FEN renders the position as a 6-field FEN string.
func (p *Position) FEN() string {
	var b strings.Builder

	// Field 1: piece placement.
	for r := 7; r >= 0; r-- {
		empty := 0
		for f := 0; f < 8; f++ {
			piece := p.PieceAt(Square{File: int8(f), Rank: int8(r)})
			if piece.IsEmpty() {
				empty++
				continue
			}
			if empty > 0 {
				b.WriteByte(byte('0' + empty))
				empty = 0
			}
			b.WriteByte(pieceTypeToFENChar(piece.Type, piece.Color))
		}
		if empty > 0 {
			b.WriteByte(byte('0' + empty))
		}
		if r > 0 {
			b.WriteByte('/')
		}
	}

	// Field 2: side to move.
	if p.SideToMove == White {
		b.WriteString(" w ")
	} else {
		b.WriteString(" b ")
	}

	// Field 3: castling rights.
	if p.Castling == 0 {
		b.WriteByte('-')
	} else {
		if p.Castling.Has(WhiteKingside) {
			b.WriteByte('K')
		}
		if p.Castling.Has(WhiteQueenside) {
			b.WriteByte('Q')
		}
		if p.Castling.Has(BlackKingside) {
			b.WriteByte('k')
		}
		if p.Castling.Has(BlackQueenside) {
			b.WriteByte('q')
		}
	}

	// Field 4: en-passant target.
	b.WriteByte(' ')
	if p.EnPassant != nil {
		b.WriteString(p.EnPassant.String())
	} else {
		b.WriteByte('-')
	}

	// Fields 5 & 6: clocks.
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(p.HalfMove))
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(p.FullMove))

	return b.String()
}
