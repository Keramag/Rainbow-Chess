package main

import "rainbow-chess/engine"

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
