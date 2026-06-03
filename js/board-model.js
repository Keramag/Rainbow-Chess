// board-model.js — pure, DOM-free board logic for the renderer.
//
// The server is authoritative: it ships the position as FEN and the legal-move
// list for the side to move on every update, so this module never re-implements
// chess rules. It only does presentation-side derivations:
//   - FEN placement -> an 8x8 model of coloured pieces,
//   - square <-> algebraic / display-grid coordinate mapping (with board
//     flipping so each player sees their own pieces at the bottom),
//   - highlight / legal-target / promotion-option derivation from the
//     server-provided legal-move list.
//
// Everything here is side-effect-free and importable under `node --test`. The
// DOM glue that turns this model into elements lives in chess.js.
//
// Board indexing matches the Go engine exactly: index = rank*8 + file, where
// file 0..7 is a..h and rank 0..7 is 1..8 (rank 0 = White's back rank). This
// keeps the wire's algebraic squares and the model in lock-step.

// PIECE_BY_CHAR maps a lower-case FEN letter to a piece-type name. Colour is
// carried separately (FEN encodes it via letter case).
const PIECE_BY_CHAR = {
  p: 'pawn',
  n: 'knight',
  b: 'bishop',
  r: 'rook',
  q: 'queen',
  k: 'king',
};

// PIECE_GLYPH maps a piece type to a single Unicode shape. We deliberately use
// the *solid* glyph for both colours and let CSS colour each piece by its own
// colour — this is what makes Rainbow's colour-mixed boards render correctly
// (the shape never implies a side; the fill colour does).
export const PIECE_GLYPH = {
  king: '♚', // ♚
  queen: '♛', // ♛
  rook: '♜', // ♜
  bishop: '♝', // ♝
  knight: '♞', // ♞
  pawn: '♟', // ♟
};

// pieceFromChar turns a single FEN piece letter into {type, color}. Upper-case
// is White, lower-case is Black. Returns null for any non-piece character.
export function pieceFromChar(ch) {
  if (typeof ch !== 'string' || ch.length !== 1) return null;
  const lower = ch.toLowerCase();
  const type = PIECE_BY_CHAR[lower];
  if (!type) return null;
  return { type, color: ch === lower ? 'black' : 'white' };
}

// squareIndex returns the 0..63 board-array index for a file/rank pair,
// matching the engine's Square.Index() (rank*8 + file).
export function squareIndex(file, rank) {
  return rank * 8 + file;
}

// indexToFileRank is the inverse of squareIndex.
export function indexToFileRank(i) {
  return { file: i % 8, rank: Math.floor(i / 8) };
}

// squareName renders a file/rank pair as algebraic notation ("e4"). Returns ""
// for an off-board coordinate.
export function squareName(file, rank) {
  if (file < 0 || file > 7 || rank < 0 || rank > 7) return '';
  return String.fromCharCode(97 + file) + String(rank + 1);
}

// parseSquareName parses algebraic notation ("e4") into {file, rank}, or null
// if it is not a valid on-board square.
export function parseSquareName(name) {
  if (typeof name !== 'string' || name.length !== 2) return null;
  const file = name.charCodeAt(0) - 97; // 'a'
  const rank = name.charCodeAt(1) - 49; // '1'
  if (file < 0 || file > 7 || rank < 0 || rank > 7) return null;
  return { file, rank };
}

// squareColor returns 'light' or 'dark' for a square. a1 (file 0, rank 0) is a
// dark square, which fixes the parity of the whole board.
export function squareColor(file, rank) {
  return (file + rank) % 2 === 0 ? 'dark' : 'light';
}

// parseBoard parses the placement field of a FEN string into a flat 64-entry
// model indexed by squareIndex(file, rank). Each entry is null (empty) or
// {type, color}. Throws on a structurally invalid placement so callers can
// surface a clear error rather than rendering a corrupt board.
export function parseBoard(fen) {
  if (typeof fen !== 'string' || fen.length === 0) {
    throw new Error('parseBoard: empty FEN');
  }
  const placement = fen.trim().split(/\s+/)[0];
  const ranks = placement.split('/');
  if (ranks.length !== 8) {
    throw new Error(`parseBoard: want 8 ranks, got ${ranks.length}`);
  }

  const board = new Array(64).fill(null);
  for (let r = 0; r < 8; r++) {
    const rank = 7 - r; // first FEN rank string is rank 8 (rank index 7)
    let file = 0;
    for (const ch of ranks[r]) {
      if (ch >= '1' && ch <= '8') {
        file += ch.charCodeAt(0) - 48;
        if (file > 8) throw new Error(`parseBoard: rank "${ranks[r]}" overflows 8 files`);
        continue;
      }
      const piece = pieceFromChar(ch);
      if (!piece) throw new Error(`parseBoard: invalid piece char "${ch}"`);
      if (file >= 8) throw new Error(`parseBoard: rank "${ranks[r]}" has too many squares`);
      board[squareIndex(file, rank)] = piece;
      file++;
    }
    if (file !== 8) {
      throw new Error(`parseBoard: rank "${ranks[r]}" covers ${file} files, want 8`);
    }
  }
  return board;
}

// sideToMoveFromFen extracts the side to move from a FEN's second field,
// defaulting to 'white' when absent so a placement-only string still renders.
export function sideToMoveFromFen(fen) {
  if (typeof fen !== 'string') return 'white';
  const field = fen.trim().split(/\s+/)[1];
  return field === 'b' ? 'black' : 'white';
}

// displaySquares returns the 64 squares in display order (row 0 = top row,
// col 0 = left column), oriented so the given player's pieces sit at the
// bottom. With orientation 'white' rank 8 is on top and file a on the left;
// with 'black' the board is rotated 180° (rank 1 on top, file h on the left).
// Each entry carries everything the renderer needs: grid position (row/col),
// board coordinate (file/rank/index), algebraic name and square colour.
export function displaySquares(orientation = 'white') {
  const flip = orientation === 'black';
  const squares = [];
  for (let row = 0; row < 8; row++) {
    for (let col = 0; col < 8; col++) {
      const file = flip ? 7 - col : col;
      const rank = flip ? row : 7 - row;
      squares.push({
        row,
        col,
        file,
        rank,
        index: squareIndex(file, rank),
        name: squareName(file, rank),
        color: squareColor(file, rank),
      });
    }
  }
  return squares;
}

// legalTargets returns the unique destination squares reachable from `from`
// given the server's legal-move list. Used to highlight where a selected piece
// may go. Order follows first appearance in the list.
export function legalTargets(legalMoves, from) {
  const seen = new Set();
  const targets = [];
  for (const m of legalMoves || []) {
    if (m && m.from === from && !seen.has(m.to)) {
      seen.add(m.to);
      targets.push(m.to);
    }
  }
  return targets;
}

// movableSquares returns the set of source squares that have at least one legal
// move, so the renderer can mark which of the player's pieces are playable.
export function movableSquares(legalMoves) {
  const set = new Set();
  for (const m of legalMoves || []) {
    if (m && m.from) set.add(m.from);
  }
  return set;
}

// promotionOptions returns the distinct promotion piece-letters legal for the
// given from->to move (e.g. ['q','r','b','n'] for Standard, ['n','b'] for
// Rainbow). An empty array means the move is not a promotion, so the caller can
// send it directly without prompting. Because this is derived from the server's
// legal-move list, the variant's promotion restriction is honoured for free.
export function promotionOptions(legalMoves, from, to) {
  const opts = [];
  for (const m of legalMoves || []) {
    if (m && m.from === from && m.to === to && m.promotion && !opts.includes(m.promotion)) {
      opts.push(m.promotion);
    }
  }
  return opts;
}

// isLegalMove reports whether (from, to, promotion?) matches a move in the
// server's legal-move list. Promotion is compared only when supplied, so a
// caller can first check "is this a legal destination at all" and later pin
// down the specific promotion.
export function isLegalMove(legalMoves, from, to, promotion) {
  for (const m of legalMoves || []) {
    if (!m || m.from !== from || m.to !== to) continue;
    if (promotion === undefined || promotion === null || promotion === '') return true;
    if (m.promotion === promotion) return true;
  }
  return false;
}
