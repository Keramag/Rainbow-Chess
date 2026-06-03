// game-state.js — pure, DOM-free reducer for the app's high-level UI state.
//
// chess.js (BoardView) owns the in-game board itself; this module owns the
// surrounding "which screen am I on" state: the lobby-less menu, an active game
// (against whom, as which colour, in which variant), or a finished game's result
// with a "back to menu" offer. app.js renders from this state and never computes
// transitions inline — that is what keeps the game-lifecycle transitions
// (game_start -> game_update -> game-over) unit-testable under `node --test`.
//
// The server is authoritative, so the reducer only mirrors what the hub sends
// (FEN, side-to-move, legal moves, in-check flag, result) plus the bits of UI
// context the wire doesn't carry: the opponent's display name (supplied by the
// caller on game_start) and the player-relative outcome. It holds no chess rules.

// PHASE enumerates the mutually exclusive top-level screens.
export const PHASE = {
  MENU: 'menu', // no active game — challenge a player to start
  PLAYING: 'playing', // a game is in progress
  OVER: 'over', // a game has finished; result shown with a back-to-menu offer
};

// initialState is the lobby-less menu: no active game, nothing to surface.
export function initialState() {
  return { phase: PHASE.MENU, game: null, notice: null };
}

// isOver reports whether a ResultDTO marks the game finished. A missing result,
// or one whose outcome is "ongoing"/empty, means play continues.
export function isOver(result) {
  return Boolean(result && result.outcome && result.outcome !== 'ongoing');
}

// playerOutcome maps a finished result to the viewing player's perspective —
// 'win' | 'loss' | 'draw' — or null if the game is not over. myColor is the
// viewing player's colour ('white'/'black').
export function playerOutcome(result, myColor) {
  if (!isOver(result)) return null;
  if (result.outcome === 'draw') return 'draw';
  const iAmWhite = myColor === 'white';
  const whiteWon = result.outcome === 'white_wins';
  return whiteWon === iAmWhite ? 'win' : 'loss';
}

// sideOf extracts the side to move from a FEN's second field, defaulting to
// 'white' so a placement-only string still yields a sane value. Kept local so
// the reducer has no import cycle with board-model.js.
function sideOf(fen) {
  if (typeof fen !== 'string') return 'white';
  const field = fen.trim().split(/\s+/)[1];
  return field === 'b' ? 'black' : 'white';
}

// winFor returns the ResultDTO outcome string for the given colour winning.
function winFor(color) {
  return color === 'black' ? 'black_wins' : 'white_wins';
}

// reduce applies a server message (or a local action) to the UI state and
// returns the NEXT state without mutating the input. Messages it does not model
// (welcome / users_update / challenge_received — identity & roster, owned by
// app.js directly) pass through unchanged.
//
// game_start may carry an extra `opponentName` the wire itself does not include;
// app.js knows it (it issued or accepted the challenge) and merges it in.
export function reduce(state, msg) {
  if (!msg || typeof msg.type !== 'string') return state;

  switch (msg.type) {
    case 'game_start': {
      const game = {
        gameId: msg.gameId || null,
        variant: msg.variant || null,
        myColor: msg.color === 'black' ? 'black' : 'white',
        opponentName: msg.opponentName || '',
        fen: msg.fen || null,
        sideToMove: msg.sideToMove || sideOf(msg.fen),
        legalMoves: Array.isArray(msg.legalMoves) ? msg.legalMoves : [],
        lastMove: null,
        inCheck: Boolean(msg.inCheck),
        result: null,
      };
      return { phase: PHASE.PLAYING, game, notice: null };
    }

    case 'game_update': {
      // Ignore updates with no game in flight or for a different game id.
      if (!state.game) return state;
      if (msg.gameId && state.game.gameId && msg.gameId !== state.game.gameId) return state;

      const over = isOver(msg.result);
      const game = {
        ...state.game,
        fen: msg.fen || state.game.fen,
        sideToMove: msg.sideToMove || state.game.sideToMove,
        legalMoves: Array.isArray(msg.legalMoves) ? msg.legalMoves : [],
        lastMove: msg.lastMove || null,
        inCheck: Boolean(msg.inCheck),
        result: over ? msg.result : null,
      };
      return { ...state, phase: over ? PHASE.OVER : PHASE.PLAYING, game };
    }

    case 'opponent_disconnected': {
      const notice = { kind: 'opponent_disconnected', text: 'Opponent disconnected — you win.' };
      if (!state.game) return { ...state, notice };
      const result = isOver(msg.result)
        ? msg.result
        : { outcome: winFor(state.game.myColor), reason: 'opponent left' };
      return { phase: PHASE.OVER, game: { ...state.game, result }, notice };
    }

    case 'challenge_declined':
      return { ...state, notice: { kind: 'challenge_declined', text: 'Your challenge was declined.' } };

    case 'challenge_expired':
      return { ...state, notice: { kind: 'challenge_expired', text: 'A challenge expired.' } };

    case 'error':
      return { ...state, notice: { kind: 'error', text: msg.message || 'Error' } };

    default:
      return state;
  }
}

// returnToMenu is the local "new game / back to menu" action offered when a game
// ends: it discards the finished game and returns to the lobby-less menu.
export function returnToMenu() {
  return initialState();
}

// clearNotice drops a surfaced notice once app.js has shown it (e.g. as a
// toast), so the same message is not re-shown on the next render.
export function clearNotice(state) {
  if (!state.notice) return state;
  return { ...state, notice: null };
}
