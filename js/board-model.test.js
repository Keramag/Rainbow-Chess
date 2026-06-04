// Tests for the pure board-model helpers. Run with `node --test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  pieceFromChar,
  parseBoard,
  squareIndex,
  indexToFileRank,
  squareName,
  parseSquareName,
  squareColor,
  sideToMoveFromFen,
  displaySquares,
  edgeCoordinates,
  legalTargets,
  movableSquares,
  promotionOptions,
  isLegalMove,
  PIECE_GLYPH,
} from './board-model.js';

const START_FEN = 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1';

// at returns the piece on an algebraic square in a parsed 64-entry board.
function at(board, name) {
  const { file, rank } = parseSquareName(name);
  return board[squareIndex(file, rank)];
}

test('pieceFromChar derives type and colour from FEN case', () => {
  assert.deepEqual(pieceFromChar('K'), { type: 'king', color: 'white' });
  assert.deepEqual(pieceFromChar('q'), { type: 'queen', color: 'black' });
  assert.deepEqual(pieceFromChar('N'), { type: 'knight', color: 'white' });
  assert.deepEqual(pieceFromChar('p'), { type: 'pawn', color: 'black' });
  assert.equal(pieceFromChar('x'), null);
  assert.equal(pieceFromChar('1'), null);
  assert.equal(pieceFromChar(''), null);
  assert.equal(pieceFromChar('KK'), null);
});

test('parseBoard maps the standard start position', () => {
  const board = parseBoard(START_FEN);
  assert.equal(board.length, 64);

  // White back rank.
  assert.deepEqual(at(board, 'a1'), { type: 'rook', color: 'white' });
  assert.deepEqual(at(board, 'e1'), { type: 'king', color: 'white' });
  assert.deepEqual(at(board, 'd1'), { type: 'queen', color: 'white' });
  // White pawns on rank 2.
  assert.deepEqual(at(board, 'e2'), { type: 'pawn', color: 'white' });
  // Empty middle.
  assert.equal(at(board, 'e4'), null);
  assert.equal(at(board, 'd5'), null);
  // Black pieces.
  assert.deepEqual(at(board, 'e7'), { type: 'pawn', color: 'black' });
  assert.deepEqual(at(board, 'e8'), { type: 'king', color: 'black' });
  assert.deepEqual(at(board, 'h8'), { type: 'rook', color: 'black' });
});

test('parseBoard handles a Rainbow-style colour-mixed position', () => {
  // Standard piece *types* on standard squares, but colours assigned by board
  // symmetry rather than by board half: here a-file back-rank pieces are white,
  // h-file mirror pieces black, with mixed colours across the rank. The model
  // must colour each piece by its own FEN case, never by which half it sits in.
  const fen = 'RnbqkBNr/PppppppP/8/8/8/8/pPPPPPPp/rNBQKbnR w - - 0 1';
  const board = parseBoard(fen);

  // Black-cased pieces sitting on White's back rank (rank 1).
  assert.deepEqual(at(board, 'a1'), { type: 'rook', color: 'black' });
  assert.deepEqual(at(board, 'b1'), { type: 'knight', color: 'white' });
  assert.deepEqual(at(board, 'f1'), { type: 'bishop', color: 'black' });
  assert.deepEqual(at(board, 'h1'), { type: 'rook', color: 'white' });
  // White-cased pieces on Black's back rank (rank 8).
  assert.deepEqual(at(board, 'a8'), { type: 'rook', color: 'white' });
  assert.deepEqual(at(board, 'h8'), { type: 'rook', color: 'black' });
  // Mixed pawns (rank 2 = "pPPPPPPp", rank 7 = "PppppppP").
  assert.deepEqual(at(board, 'a2'), { type: 'pawn', color: 'black' });
  assert.deepEqual(at(board, 'a7'), { type: 'pawn', color: 'white' });
  assert.deepEqual(at(board, 'h2'), { type: 'pawn', color: 'black' });
});

test('parseBoard rejects structurally invalid FENs', () => {
  assert.throws(() => parseBoard(''), /empty FEN/);
  assert.throws(() => parseBoard('8/8/8/8/8/8/8 w - - 0 1'), /8 ranks/); // 7 ranks
  assert.throws(() => parseBoard('rnbqkbnr/ppppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR'), /overflows|too many/); // 9 pawns
  assert.throws(() => parseBoard('xnbqkbnr/8/8/8/8/8/8/8'), /invalid piece/);
});

test('squareIndex and indexToFileRank are inverse and engine-aligned', () => {
  assert.equal(squareIndex(0, 0), 0); // a1
  assert.equal(squareIndex(4, 0), 4); // e1
  assert.equal(squareIndex(4, 3), 28); // e4
  assert.equal(squareIndex(7, 7), 63); // h8
  for (let i = 0; i < 64; i++) {
    const { file, rank } = indexToFileRank(i);
    assert.equal(squareIndex(file, rank), i);
  }
});

test('squareName / parseSquareName round-trip and reject bad input', () => {
  assert.equal(squareName(0, 0), 'a1');
  assert.equal(squareName(4, 3), 'e4');
  assert.equal(squareName(7, 7), 'h8');
  assert.equal(squareName(-1, 0), '');
  assert.equal(squareName(0, 8), '');

  assert.deepEqual(parseSquareName('a1'), { file: 0, rank: 0 });
  assert.deepEqual(parseSquareName('e4'), { file: 4, rank: 3 });
  assert.deepEqual(parseSquareName('h8'), { file: 7, rank: 7 });
  assert.equal(parseSquareName('i9'), null);
  assert.equal(parseSquareName('e'), null);
  assert.equal(parseSquareName(''), null);

  for (let i = 0; i < 64; i++) {
    const { file, rank } = indexToFileRank(i);
    const name = squareName(file, rank);
    assert.deepEqual(parseSquareName(name), { file, rank });
  }
});

test('squareColor places a1 dark and matches the diagonal parity', () => {
  assert.equal(squareColor(0, 0), 'dark'); // a1
  assert.equal(squareColor(7, 0), 'light'); // h1
  assert.equal(squareColor(4, 3), 'light'); // e4
  assert.equal(squareColor(7, 7), 'dark'); // h8
});

test('sideToMoveFromFen reads the second FEN field', () => {
  assert.equal(sideToMoveFromFen(START_FEN), 'white');
  assert.equal(sideToMoveFromFen('8/8/8/8/8/8/8/8 b - - 0 1'), 'black');
  assert.equal(sideToMoveFromFen('rnbqkbnr/8/8/8/8/8/8/8'), 'white'); // placement only
  assert.equal(sideToMoveFromFen(undefined), 'white');
});

test('displaySquares orients the board for White (rank 8 on top)', () => {
  const sq = displaySquares('white');
  assert.equal(sq.length, 64);
  // Top-left is a8, bottom-left a1, bottom-right h1.
  assert.equal(sq[0].name, 'a8');
  assert.equal(sq[7].name, 'h8');
  assert.equal(sq[56].name, 'a1');
  assert.equal(sq[63].name, 'h1');
  // Grid bookkeeping is consistent.
  assert.deepEqual({ row: sq[0].row, col: sq[0].col }, { row: 0, col: 0 });
  assert.equal(sq[0].index, squareIndex(0, 7));
});

test('displaySquares flips the board for Black (rank 1 on top, h-file left)', () => {
  const sq = displaySquares('black');
  assert.equal(sq[0].name, 'h1');
  assert.equal(sq[7].name, 'a1');
  assert.equal(sq[56].name, 'h8');
  assert.equal(sq[63].name, 'a8');
  // The player's own back rank sits at the bottom in both orientations.
});

test('edgeCoordinates labels the bottom row with files and left column with ranks (White)', () => {
  const sq = displaySquares('white');
  const by = (name) => sq.find((s) => s.name === name);
  // a1 is the bottom-left corner: it carries both its file and rank.
  assert.deepEqual(edgeCoordinates(by('a1')), { file: 'a', rank: '1' });
  // h1 is the bottom-right corner: file only (not the left column).
  assert.deepEqual(edgeCoordinates(by('h1')), { file: 'h', rank: null });
  // a8 is the top-left corner: rank only (not the bottom row).
  assert.deepEqual(edgeCoordinates(by('a8')), { file: null, rank: '8' });
  // An interior square carries no edge labels.
  assert.deepEqual(edgeCoordinates(by('d4')), { file: null, rank: null });
});

test('edgeCoordinates follows the flip for Black (bottom-left is h8)', () => {
  const sq = displaySquares('black');
  const bottomLeft = sq.find((s) => s.row === 7 && s.col === 0);
  assert.equal(bottomLeft.name, 'h8');
  assert.deepEqual(edgeCoordinates(bottomLeft), { file: 'h', rank: '8' });
});

test('edgeCoordinates tolerates a malformed square', () => {
  assert.deepEqual(edgeCoordinates(), { file: null, rank: null });
  assert.deepEqual(edgeCoordinates({}), { file: null, rank: null });
});

test('legalTargets derives highlight destinations from the move list', () => {
  const legal = [
    { from: 'e2', to: 'e3' },
    { from: 'e2', to: 'e4' },
    { from: 'g1', to: 'f3' },
    { from: 'g1', to: 'h3' },
  ];
  assert.deepEqual(legalTargets(legal, 'e2'), ['e3', 'e4']);
  assert.deepEqual(legalTargets(legal, 'g1'), ['f3', 'h3']);
  assert.deepEqual(legalTargets(legal, 'a1'), []);
  assert.deepEqual(legalTargets(undefined, 'e2'), []);
});

test('legalTargets de-duplicates promotion destinations', () => {
  const legal = [
    { from: 'e7', to: 'e8', promotion: 'q' },
    { from: 'e7', to: 'e8', promotion: 'r' },
    { from: 'e7', to: 'e8', promotion: 'b' },
    { from: 'e7', to: 'e8', promotion: 'n' },
  ];
  assert.deepEqual(legalTargets(legal, 'e7'), ['e8']);
});

test('movableSquares collects every source with a legal move', () => {
  const legal = [
    { from: 'e2', to: 'e4' },
    { from: 'g1', to: 'f3' },
    { from: 'e2', to: 'e3' },
  ];
  const set = movableSquares(legal);
  assert.ok(set.has('e2'));
  assert.ok(set.has('g1'));
  assert.equal(set.has('a2'), false);
  assert.equal(set.size, 2);
});

test('promotionOptions returns the variant-allowed promotions only', () => {
  // Standard: all four offered.
  const standard = [
    { from: 'e7', to: 'e8', promotion: 'q' },
    { from: 'e7', to: 'e8', promotion: 'r' },
    { from: 'e7', to: 'e8', promotion: 'b' },
    { from: 'e7', to: 'e8', promotion: 'n' },
  ];
  assert.deepEqual(promotionOptions(standard, 'e7', 'e8'), ['q', 'r', 'b', 'n']);

  // Rainbow: only knight/bishop appear in the legal-move list, so the picker is
  // restricted automatically — no separate variant lookup needed.
  const rainbow = [
    { from: 'e7', to: 'e8', promotion: 'n' },
    { from: 'e7', to: 'e8', promotion: 'b' },
  ];
  assert.deepEqual(promotionOptions(rainbow, 'e7', 'e8'), ['n', 'b']);

  // A plain (non-promotion) move yields no options.
  assert.deepEqual(promotionOptions([{ from: 'e2', to: 'e4' }], 'e2', 'e4'), []);
});

test('isLegalMove validates with and without a promotion piece', () => {
  const legal = [
    { from: 'e2', to: 'e4' },
    { from: 'e7', to: 'e8', promotion: 'n' },
    { from: 'e7', to: 'e8', promotion: 'b' },
  ];
  assert.equal(isLegalMove(legal, 'e2', 'e4'), true);
  assert.equal(isLegalMove(legal, 'e2', 'e5'), false);
  // Destination matches but the specific promotion must too.
  assert.equal(isLegalMove(legal, 'e7', 'e8'), true); // promotion unspecified -> any
  assert.equal(isLegalMove(legal, 'e7', 'e8', 'n'), true);
  assert.equal(isLegalMove(legal, 'e7', 'e8', 'q'), false); // queen not offered (Rainbow)
});

test('PIECE_GLYPH covers every piece type with a single shape per type', () => {
  for (const type of ['king', 'queen', 'rook', 'bishop', 'knight', 'pawn']) {
    assert.equal(typeof PIECE_GLYPH[type], 'string');
    assert.ok(PIECE_GLYPH[type].length >= 1);
  }
});
