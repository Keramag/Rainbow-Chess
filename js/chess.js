// chess.js — DOM board renderer + click-to-move, layered on board-model.js.
//
// This is presentation only. All chess knowledge stays server-side: the hub
// sends FEN + the legal-move list + side-to-move + an in-check flag + the
// result on every update, and this view just paints it and relays the player's
// click intent back as a `move`. It never decides what is legal — it reads that
// straight out of the move list (see board-model.js), which is also why the
// promotion picker automatically respects each variant's allowed pieces
// (Standard offers Q/R/B/N; Rainbow's list only contains N/B).
//
// Rainbow correctness: every piece is rendered by its OWN colour (CSS classes
// .piece.white / .piece.black on a single shared glyph), never by which half of
// the board it occupies, and the board is flipped by the viewing player's own
// colour — so a colour-mixed start position renders exactly as the server sees
// it.

import {
  parseBoard,
  displaySquares,
  edgeCoordinates,
  legalTargets,
  movableSquares,
  promotionOptions,
  sideToMoveFromFen,
  PIECE_GLYPH,
} from './board-model.js';

// PROMOTION_NAME maps a promotion letter to the piece type for glyph lookup.
const PROMOTION_NAME = { q: 'queen', r: 'rook', b: 'bishop', n: 'knight' };

// BoardView owns the board grid, the turn/check/result banner and a transient
// promotion picker. It re-renders from server state; it holds no rules.
export class BoardView {
  // root is the container element. onMove(from, to, promotion?) is called when
  // the player completes a legal move; the caller forwards it to the server.
  constructor(root, { onMove } = {}) {
    this.root = root || null;
    this.onMove = typeof onMove === 'function' ? onMove : () => {};

    // Per-game immutable-ish context.
    this.gameId = null;
    this.orientation = 'white'; // the viewing player's colour
    this.variant = null;
    this.opponentName = '';

    // Server-driven state, refreshed on every update.
    this.fen = null;
    this.legalMoves = [];
    this.sideToMove = 'white';
    this.lastMove = null; // {from, to}
    this.result = null; // {outcome, reason} | null
    this.inCheck = false;

    // Local interaction state.
    this.selected = null; // currently selected source square name
    this.pendingPromotion = null; // {from, to, options} while the picker is open

    this.build();
  }

  // build lays out the static skeleton (banner + board grid + picker overlay)
  // once; render() only updates contents and classes thereafter. No-op without
  // a DOM (e.g. under the Node test runner, where this module is not used).
  build() {
    if (!this.root || typeof document === 'undefined') return;
    this.root.classList.add('chess');
    this.root.textContent = '';

    this.bannerEl = document.createElement('div');
    this.bannerEl.className = 'board-banner';
    this.root.appendChild(this.bannerEl);

    this.boardEl = document.createElement('div');
    this.boardEl.className = 'board';
    this.root.appendChild(this.boardEl);

    this.pickerEl = document.createElement('div');
    this.pickerEl.className = 'promotion-picker';
    this.pickerEl.hidden = true;
    this.root.appendChild(this.pickerEl);

    // One delegated click handler for the whole board.
    this.boardEl.addEventListener('click', (e) => {
      const cell = e.target.closest('.square');
      if (cell && cell.dataset.square) this.onSquareClick(cell.dataset.square);
    });
  }

  // start initialises the view from a game_start message and renders the
  // opening position. msg = {gameId, variant, color, fen, legalMoves, inCheck}.
  start(msg, opponentName = '') {
    this.gameId = msg.gameId || null;
    this.orientation = msg.color === 'black' ? 'black' : 'white';
    this.variant = msg.variant || null;
    this.opponentName = opponentName || '';

    this.fen = msg.fen || null;
    this.legalMoves = Array.isArray(msg.legalMoves) ? msg.legalMoves : [];
    this.sideToMove = msg.sideToMove || sideToMoveFromFen(this.fen);
    this.lastMove = null;
    this.result = null;
    this.inCheck = Boolean(msg.inCheck);

    this.selected = null;
    this.closePromotion();
    this.render();
  }

  // update refreshes from a game_update message and re-renders. msg =
  // {fen, sideToMove, legalMoves, lastMove, result, inCheck}.
  update(msg) {
    if (msg.fen) this.fen = msg.fen;
    this.legalMoves = Array.isArray(msg.legalMoves) ? msg.legalMoves : [];
    this.sideToMove = msg.sideToMove || sideToMoveFromFen(this.fen);
    this.lastMove = msg.lastMove || null;
    this.result = msg.result && msg.result.outcome ? msg.result : null;
    this.inCheck = Boolean(msg.inCheck);

    this.selected = null;
    this.closePromotion();
    this.render();
  }

  // isMyTurn reports whether it is the viewing player's move and the game is
  // still going — the only state in which clicks select/move.
  isMyTurn() {
    return !this.gameOver() && this.sideToMove === this.orientation;
  }

  gameOver() {
    return Boolean(this.result && this.result.outcome && this.result.outcome !== 'ongoing');
  }

  // onSquareClick drives the two-click move flow: first click selects one of the
  // player's movable pieces; second click either plays a legal target (opening
  // the promotion picker when needed), re-selects another own piece, or clears.
  onSquareClick(name) {
    if (this.pendingPromotion || !this.isMyTurn()) return;

    if (this.selected && this.selected !== name) {
      const targets = legalTargets(this.legalMoves, this.selected);
      if (targets.includes(name)) {
        this.commitMove(this.selected, name);
        return;
      }
    }

    // (Re)select if this square holds a piece that has at least one legal move;
    // otherwise clear the selection.
    if (movableSquares(this.legalMoves).has(name)) {
      this.selected = this.selected === name ? null : name;
    } else {
      this.selected = null;
    }
    this.render();
  }

  // commitMove plays from->to, prompting for a promotion piece first when the
  // move list shows this destination admits promotions.
  commitMove(from, to) {
    const promotions = promotionOptions(this.legalMoves, from, to);
    if (promotions.length > 0) {
      this.openPromotion(from, to, promotions);
      return;
    }
    this.selected = null;
    this.onMove(from, to);
    // Optimism is the server's job; we wait for the authoritative game_update
    // to re-render, but drop the highlight immediately for responsiveness.
    this.render();
  }

  openPromotion(from, to, options) {
    this.pendingPromotion = { from, to, options };
    this.render();
  }

  closePromotion() {
    this.pendingPromotion = null;
    if (this.pickerEl) this.pickerEl.hidden = true;
  }

  choosePromotion(letter) {
    const pending = this.pendingPromotion;
    if (!pending) return;
    this.closePromotion();
    this.selected = null;
    this.onMove(pending.from, pending.to, letter);
    this.render();
  }

  // --- Rendering ----------------------------------------------------------

  render() {
    if (!this.boardEl) return;
    this.renderBanner();
    this.renderBoard();
    this.renderPromotion();
  }

  renderBanner() {
    if (!this.bannerEl) return;
    const variantLabel = this.variant
      ? this.variant.charAt(0).toUpperCase() + this.variant.slice(1)
      : 'Chess';

    let status;
    let tone = 'turn';
    if (this.gameOver()) {
      status = this.resultText();
      tone = 'over';
    } else if (this.isMyTurn()) {
      status = this.inCheck ? 'Your move — you are in check!' : 'Your move';
      if (this.inCheck) tone = 'check';
    } else {
      const who = this.opponentName ? `${this.opponentName}'s` : "Opponent's";
      status = this.inCheck ? `${who} move — check given` : `${who} move`;
    }

    this.bannerEl.dataset.tone = tone;
    this.bannerEl.textContent = `${variantLabel} · you play ${this.orientation} — ${status}`;
  }

  // resultText turns the terminal result into a player-relative sentence.
  resultText() {
    const r = this.result || {};
    const reason = r.reason ? ` (${r.reason})` : '';
    if (r.outcome === 'draw') return `Draw${reason}`;
    const iWon =
      (r.outcome === 'white_wins' && this.orientation === 'white') ||
      (r.outcome === 'black_wins' && this.orientation === 'black');
    return `${iWon ? 'You win' : 'You lose'}${reason}`;
  }

  renderBoard() {
    const board = this.safeParse();
    const squares = displaySquares(this.orientation);
    const targets = this.selected ? legalTargets(this.legalMoves, this.selected) : [];
    const last = this.lastMove || {};

    this.boardEl.textContent = '';
    for (const sq of squares) {
      const cell = document.createElement('div');
      cell.className = `square ${sq.color}`;
      cell.dataset.square = sq.name;

      if (sq.name === this.selected) cell.classList.add('selected');
      if (sq.name === last.from || sq.name === last.to) cell.classList.add('last-move');

      const piece = board ? board[sq.index] : null;
      if (piece) {
        const span = document.createElement('span');
        span.className = `piece ${piece.color}`;
        span.textContent = PIECE_GLYPH[piece.type] || '';
        cell.appendChild(span);
        // Mark the king square when its side is in check.
        if (
          piece.type === 'king' &&
          this.inCheck &&
          !this.gameOver() &&
          piece.color === this.sideToMove
        ) {
          cell.classList.add('in-check');
        }
      }

      if (targets.includes(sq.name)) {
        cell.classList.add(piece ? 'capture-target' : 'move-target');
      }

      // Edge coordinates: file letters along the bottom row, rank numbers down
      // the left column (oriented for the viewing player). The corner square
      // carries both. Tinted by square colour so they read on either shade.
      const labels = edgeCoordinates(sq);
      if (labels.rank) {
        const r = document.createElement('span');
        r.className = 'coord coord-rank';
        r.textContent = labels.rank;
        cell.appendChild(r);
      }
      if (labels.file) {
        const f = document.createElement('span');
        f.className = 'coord coord-file';
        f.textContent = labels.file;
        cell.appendChild(f);
      }

      this.boardEl.appendChild(cell);
    }
  }

  renderPromotion() {
    if (!this.pickerEl) return;
    if (!this.pendingPromotion) {
      this.pickerEl.hidden = true;
      this.pickerEl.textContent = '';
      return;
    }
    this.pickerEl.hidden = false;
    this.pickerEl.textContent = '';

    const label = document.createElement('div');
    label.className = 'promotion-label';
    label.textContent = 'Promote to';
    this.pickerEl.appendChild(label);

    const row = document.createElement('div');
    row.className = 'promotion-options';
    for (const letter of this.pendingPromotion.options) {
      const btn = document.createElement('button');
      btn.className = 'promotion-option';
      const glyph = document.createElement('span');
      glyph.className = `piece ${this.orientation}`;
      glyph.textContent = PIECE_GLYPH[PROMOTION_NAME[letter]] || letter;
      btn.appendChild(glyph);
      btn.addEventListener('click', () => this.choosePromotion(letter));
      row.appendChild(btn);
    }
    this.pickerEl.appendChild(row);
  }

  // safeParse parses the current FEN, swallowing a malformed string (returns
  // null) so a single bad frame can never throw out of the render loop.
  safeParse() {
    if (!this.fen) return null;
    try {
      return parseBoard(this.fen);
    } catch (err) {
      console.error('board parse failed:', err);
      return null;
    }
  }
}
