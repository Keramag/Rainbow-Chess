package main

import (
	"fmt"

	"rainbow-chess/engine"
)

// moves.go bridges the engine's Move type and the wire MoveDTO. The engine deals
// in file/rank Squares and PieceType promotions; the protocol uses algebraic
// squares ("e2") and single-letter promotion codes ("q","r","b","n"). These
// helpers keep that translation in one place so the hub never reaches into
// engine internals when shipping legal-move lists to clients.

// promotionLetters maps a promotion piece type to its lowercase wire code (the
// engine's FEN letter for that piece).
var promotionLetters = map[engine.PieceType]string{
	engine.Queen:  "q",
	engine.Rook:   "r",
	engine.Bishop: "b",
	engine.Knight: "n",
}

// letterToPromotion is the inverse of promotionLetters: it turns a wire
// promotion code back into the engine's piece type so an incoming move can be
// reconstructed. Only the four real promotion pieces are accepted.
var letterToPromotion = map[string]engine.PieceType{
	"q": engine.Queen,
	"r": engine.Rook,
	"b": engine.Bishop,
	"n": engine.Knight,
}

// moveToDTO converts a single engine move into its wire form.
func moveToDTO(m engine.Move) MoveDTO {
	dto := MoveDTO{From: m.From.String(), To: m.To.String()}
	if m.Promotion != engine.None {
		dto.Promotion = promotionLetters[m.Promotion]
	}
	return dto
}

// movesToDTO converts a list of engine moves into wire form, preserving order so
// the client can rely on the variant's documented promotion ordering.
func movesToDTO(moves []engine.Move) []MoveDTO {
	out := make([]MoveDTO, len(moves))
	for i, m := range moves {
		out[i] = moveToDTO(m)
	}
	return out
}

// dtoToMove parses an incoming wire move into a bare engine.Move (From, To and
// an optional Promotion). The special-move flags are intentionally left unset:
// the engine's ApplyMove looks up its own canonical move by from/to/promotion,
// so a {from,to,promotion} envelope is all the server needs from the client.
// Malformed squares or an unknown promotion code yield an error.
func dtoToMove(dto MoveDTO) (engine.Move, error) {
	from, err := engine.ParseSquare(dto.From)
	if err != nil {
		return engine.Move{}, fmt.Errorf("bad from square: %w", err)
	}
	to, err := engine.ParseSquare(dto.To)
	if err != nil {
		return engine.Move{}, fmt.Errorf("bad to square: %w", err)
	}
	promo := engine.None
	if dto.Promotion != "" {
		p, ok := letterToPromotion[dto.Promotion]
		if !ok {
			return engine.Move{}, fmt.Errorf("invalid promotion %q", dto.Promotion)
		}
		promo = p
	}
	return engine.Move{From: from, To: to, Promotion: promo}, nil
}

// outcomeString renders an engine outcome as its wire string, matching the
// values documented on ResultDTO.
func outcomeString(o engine.Outcome) string {
	switch o {
	case engine.WhiteWins:
		return "white_wins"
	case engine.BlackWins:
		return "black_wins"
	case engine.Draw:
		return "draw"
	default:
		return "ongoing"
	}
}

// resultToDTO converts an engine game result into its wire form. It is always
// non-nil so a game_update can carry the current outcome (an ongoing game
// reports outcome "ongoing").
func resultToDTO(r engine.GameResult) *ResultDTO {
	return &ResultDTO{Outcome: outcomeString(r.Outcome), Reason: r.Reason}
}
