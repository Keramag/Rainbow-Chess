// Tests for the pure, DOM-free game-state reducer. Run with `node --test`.
//
// These exercise the full game-lifecycle the UI screens hang off of:
// game_start -> game_update(ongoing) -> game-over (checkmate / stalemate /
// resignation / opponent-disconnect), plus the transient notices (decline /
// expiry / error) and the back-to-menu reset.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
  PHASE,
  initialState,
  isOver,
  playerOutcome,
  reduce,
  returnToMenu,
  clearNotice,
} from './game-state.js';

const START_FEN = 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1';

test('initialState is the lobby-less menu with no game or notice', () => {
  const s = initialState();
  assert.equal(s.phase, PHASE.MENU);
  assert.equal(s.game, null);
  assert.equal(s.notice, null);
});

test('isOver distinguishes terminal results from ongoing', () => {
  assert.equal(isOver(undefined), false);
  assert.equal(isOver(null), false);
  assert.equal(isOver({ outcome: 'ongoing' }), false);
  assert.equal(isOver({ outcome: '' }), false);
  assert.equal(isOver({ outcome: 'draw', reason: 'stalemate' }), true);
  assert.equal(isOver({ outcome: 'white_wins', reason: 'checkmate' }), true);
});

test('game_start enters PLAYING and records colour, variant, opponent and position', () => {
  const next = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'rainbow',
    color: 'black',
    fen: START_FEN,
    sideToMove: 'white',
    inCheck: false,
    legalMoves: [{ from: 'e2', to: 'e4' }],
    opponentName: 'BraveTiger42',
  });

  assert.equal(next.phase, PHASE.PLAYING);
  assert.equal(next.game.gameId, 'g1');
  assert.equal(next.game.variant, 'rainbow');
  assert.equal(next.game.myColor, 'black');
  assert.equal(next.game.opponentName, 'BraveTiger42');
  assert.equal(next.game.fen, START_FEN);
  assert.equal(next.game.sideToMove, 'white');
  assert.equal(next.game.result, null);
  assert.deepEqual(next.game.legalMoves, [{ from: 'e2', to: 'e4' }]);
});

test('game_start defaults: white when colour absent, side-to-move derived from FEN', () => {
  const next = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g2',
    variant: 'standard',
    fen: 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1',
  });
  assert.equal(next.game.myColor, 'white');
  assert.equal(next.game.sideToMove, 'black', 'derived from the FEN side field');
  assert.deepEqual(next.game.legalMoves, []);
});

test('game_update mid-game stays PLAYING and refreshes position + flags', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
    sideToMove: 'white',
  });
  s = reduce(s, {
    type: 'game_update',
    gameId: 'g1',
    fen: 'rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1',
    sideToMove: 'black',
    inCheck: false,
    legalMoves: [{ from: 'e7', to: 'e5' }],
    lastMove: { from: 'e2', to: 'e4' },
    result: { outcome: 'ongoing' },
  });

  assert.equal(s.phase, PHASE.PLAYING, 'ongoing update keeps us in PLAYING');
  assert.equal(s.game.sideToMove, 'black');
  assert.deepEqual(s.game.lastMove, { from: 'e2', to: 'e4' });
  assert.deepEqual(s.game.legalMoves, [{ from: 'e7', to: 'e5' }]);
  assert.equal(s.game.result, null, 'ongoing result is not stored');
});

test('game_update with a terminal result moves to OVER and keeps the final position', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  const finalFen = 'rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3';
  s = reduce(s, {
    type: 'game_update',
    gameId: 'g1',
    fen: finalFen,
    sideToMove: 'white',
    result: { outcome: 'black_wins', reason: 'checkmate' },
  });

  assert.equal(s.phase, PHASE.OVER);
  assert.equal(s.game.fen, finalFen, 'final position is retained for display');
  assert.deepEqual(s.game.result, { outcome: 'black_wins', reason: 'checkmate' });
});

test('game_update missing fen keeps the previous position (e.g. resign update)', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  s = reduce(s, {
    type: 'game_update',
    gameId: 'g1',
    sideToMove: 'white',
    result: { outcome: 'black_wins', reason: 'resignation' },
  });
  assert.equal(s.phase, PHASE.OVER);
  assert.equal(s.game.fen, START_FEN, 'fen falls back to the last known position');
  assert.equal(s.game.result.reason, 'resignation');
});

test('game_update for a different game id is ignored', () => {
  const s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  const after = reduce(s, { type: 'game_update', gameId: 'OTHER', result: { outcome: 'white_wins' } });
  assert.equal(after, s, 'a stray update for another game leaves state untouched');
});

test('game_update with no game in flight is a no-op', () => {
  const s = initialState();
  const after = reduce(s, { type: 'game_update', gameId: 'g1', result: { outcome: 'draw' } });
  assert.equal(after, s);
});

test('stalemate update ends the game as a draw', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'black',
    fen: START_FEN,
  });
  s = reduce(s, {
    type: 'game_update',
    gameId: 'g1',
    sideToMove: 'white',
    result: { outcome: 'draw', reason: 'stalemate' },
  });
  assert.equal(s.phase, PHASE.OVER);
  assert.equal(playerOutcome(s.game.result, s.game.myColor), 'draw');
});

test('opponent_disconnected ends the game as a win and surfaces a notice', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'rainbow',
    color: 'black',
    fen: START_FEN,
  });
  s = reduce(s, { type: 'opponent_disconnected', gameId: 'g1' });

  assert.equal(s.phase, PHASE.OVER);
  assert.equal(s.game.result.outcome, 'black_wins', 'disconnect awards the win to the remaining player');
  assert.equal(s.game.result.reason, 'opponent left');
  assert.equal(playerOutcome(s.game.result, s.game.myColor), 'win');
  assert.equal(s.notice.kind, 'opponent_disconnected');
});

test('opponent_disconnected honours a server-supplied result if present', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  s = reduce(s, {
    type: 'opponent_disconnected',
    gameId: 'g1',
    result: { outcome: 'white_wins', reason: 'abandonment' },
  });
  assert.equal(s.game.result.reason, 'abandonment');
});

test('opponent_disconnected with no game still surfaces a notice without crashing', () => {
  const s = reduce(initialState(), { type: 'opponent_disconnected' });
  assert.equal(s.phase, PHASE.MENU);
  assert.equal(s.notice.kind, 'opponent_disconnected');
});

test('connection_lost mid-game ends the game as a loss and surfaces a notice', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  s = reduce(s, { type: 'connection_lost' });

  assert.equal(s.phase, PHASE.OVER);
  assert.equal(s.game.result.outcome, 'black_wins', 'our own disconnect awards the win to the opponent');
  assert.equal(s.game.result.reason, 'connection lost');
  assert.equal(playerOutcome(s.game.result, s.game.myColor), 'loss');
  assert.equal(s.notice.kind, 'connection_lost');
});

test('connection_lost outside a live game only surfaces a notice', () => {
  // On the menu: nothing to end.
  const onMenu = reduce(initialState(), { type: 'connection_lost' });
  assert.equal(onMenu.phase, PHASE.MENU);
  assert.equal(onMenu.game, null);
  assert.equal(onMenu.notice.kind, 'connection_lost');

  // A game that already finished keeps its real result; we only note the drop.
  let over = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  over = reduce(over, { type: 'game_update', gameId: 'g1', result: { outcome: 'white_wins', reason: 'checkmate' } });
  const dropped = reduce(over, { type: 'connection_lost' });
  assert.equal(dropped.phase, PHASE.OVER);
  assert.equal(dropped.game.result.reason, 'checkmate', 'the finished result is preserved, not overwritten');
  assert.equal(dropped.notice.kind, 'connection_lost');
});

test('playerOutcome maps results to the viewing player perspective', () => {
  assert.equal(playerOutcome(null, 'white'), null);
  assert.equal(playerOutcome({ outcome: 'ongoing' }, 'white'), null);
  assert.equal(playerOutcome({ outcome: 'white_wins' }, 'white'), 'win');
  assert.equal(playerOutcome({ outcome: 'white_wins' }, 'black'), 'loss');
  assert.equal(playerOutcome({ outcome: 'black_wins' }, 'black'), 'win');
  assert.equal(playerOutcome({ outcome: 'black_wins' }, 'white'), 'loss');
  assert.equal(playerOutcome({ outcome: 'draw' }, 'white'), 'draw');
});

test('challenge_declined / challenge_expired / error set a notice without touching the game', () => {
  const base = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });

  const declined = reduce(base, { type: 'challenge_declined', challengeId: 'c1' });
  assert.equal(declined.notice.kind, 'challenge_declined');
  assert.equal(declined.phase, PHASE.PLAYING, 'a decline notice does not disturb an active game');
  assert.equal(declined.game, base.game);

  const expired = reduce(initialState(), { type: 'challenge_expired' });
  assert.equal(expired.notice.kind, 'challenge_expired');

  const err = reduce(initialState(), { type: 'error', message: 'Illegal move' });
  assert.equal(err.notice.text, 'Illegal move');

  const errDefault = reduce(initialState(), { type: 'error' });
  assert.equal(errDefault.notice.text, 'Error');
});

test('returnToMenu resets to the lobby-less menu after a game ends', () => {
  let s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  s = reduce(s, { type: 'game_update', gameId: 'g1', result: { outcome: 'white_wins', reason: 'checkmate' } });
  assert.equal(s.phase, PHASE.OVER);

  const menu = returnToMenu(s);
  assert.equal(menu.phase, PHASE.MENU);
  assert.equal(menu.game, null);
  assert.equal(menu.notice, null);
});

test('clearNotice drops a surfaced notice and is a no-op when there is none', () => {
  const withNotice = reduce(initialState(), { type: 'challenge_expired' });
  const cleared = clearNotice(withNotice);
  assert.equal(cleared.notice, null);

  const none = initialState();
  assert.equal(clearNotice(none), none, 'no notice -> same object back');
});

test('reduce never mutates the input state and ignores malformed messages', () => {
  const s = reduce(initialState(), {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
  });
  const snapshot = JSON.parse(JSON.stringify(s));
  reduce(s, { type: 'game_update', gameId: 'g1', result: { outcome: 'white_wins' } });
  assert.deepEqual(s, snapshot, 'reduce returns a new state rather than mutating the old one');

  assert.equal(reduce(s, null), s);
  assert.equal(reduce(s, {}), s);
  assert.equal(reduce(s, { type: 42 }), s);
  assert.equal(reduce(s, { type: 'unknown_thing' }), s);
});

test('a full Standard game flows menu -> playing -> over -> menu', () => {
  let s = initialState();
  assert.equal(s.phase, PHASE.MENU);

  s = reduce(s, {
    type: 'game_start',
    gameId: 'g1',
    variant: 'standard',
    color: 'white',
    fen: START_FEN,
    sideToMove: 'white',
    opponentName: 'Opp',
  });
  assert.equal(s.phase, PHASE.PLAYING);

  s = reduce(s, {
    type: 'game_update',
    gameId: 'g1',
    fen: 'rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1',
    sideToMove: 'black',
    result: { outcome: 'ongoing' },
  });
  assert.equal(s.phase, PHASE.PLAYING);

  s = reduce(s, {
    type: 'game_update',
    gameId: 'g1',
    sideToMove: 'white',
    result: { outcome: 'white_wins', reason: 'checkmate' },
  });
  assert.equal(s.phase, PHASE.OVER);
  assert.equal(playerOutcome(s.game.result, s.game.myColor), 'win');

  s = returnToMenu(s);
  assert.equal(s.phase, PHASE.MENU);
  assert.equal(s.game, null);
});
