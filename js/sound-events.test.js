// Tests for the pure, DOM-free sound-event classifier. Run with `node --test`.
//
// These pin down the single-sound-per-update contract and its strict priority
// (game-end > check > capture > move), plus the player-relative resolution of
// the three game-end cues and the no-throw fallback on malformed FENs.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { SOUND_EVENTS, SOUND_SPECS, eventForUpdate } from './sound-events.js';

const START_FEN = 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1';
// After 1.e4 — 32 pieces still, no capture.
const AFTER_E4 = 'rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1';

// --- success cases --------------------------------------------------------

test('a plain move (no capture, no check, not terminal) -> move', () => {
  const ev = eventForUpdate({
    prevFen: START_FEN,
    fen: AFTER_E4,
    inCheck: false,
    result: { outcome: 'ongoing' },
    myColor: 'white',
  });
  assert.equal(ev, SOUND_EVENTS.MOVE);
});

test('a capture (piece count drops by one) -> capture', () => {
  // 1.e4 d5 2.exd5 — White pawn takes the d5 pawn: 32 -> 31 pieces.
  const beforeCapture = 'rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 2';
  const afterCapture = 'rnbqkbnr/ppp1pppp/8/3P4/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 2';
  const ev = eventForUpdate({
    prevFen: beforeCapture,
    fen: afterCapture,
    inCheck: false,
    result: { outcome: 'ongoing' },
    myColor: 'white',
  });
  assert.equal(ev, SOUND_EVENTS.CAPTURE);
});

test('an en-passant capture (a pawn vanishes) -> capture', () => {
  // White e5 pawn takes a black d5 pawn en passant onto d6: 32 -> 31 pieces.
  const beforeEp = 'rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3';
  const afterEp = 'rnbqkbnr/ppp1pppp/3P4/8/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 3';
  const ev = eventForUpdate({
    prevFen: beforeEp,
    fen: afterEp,
    inCheck: false,
    result: { outcome: 'ongoing' },
    myColor: 'white',
  });
  assert.equal(ev, SOUND_EVENTS.CAPTURE);
});

test('a checking move (inCheck, no terminal result) -> check', () => {
  const ev = eventForUpdate({
    prevFen: START_FEN,
    fen: AFTER_E4,
    inCheck: true,
    result: { outcome: 'ongoing' },
    myColor: 'black',
  });
  assert.equal(ev, SOUND_EVENTS.CHECK);
});

test('checkmate as the winner -> gameEndWin', () => {
  // White delivers mate; the viewing player is White.
  const ev = eventForUpdate({
    prevFen: START_FEN,
    fen: AFTER_E4,
    inCheck: true,
    result: { outcome: 'white_wins', reason: 'checkmate' },
    myColor: 'white',
  });
  assert.equal(ev, SOUND_EVENTS.GAME_END_WIN);
});

test('checkmate as the loser (opposite myColor) -> gameEndLoss', () => {
  const ev = eventForUpdate({
    prevFen: START_FEN,
    fen: AFTER_E4,
    inCheck: true,
    result: { outcome: 'white_wins', reason: 'checkmate' },
    myColor: 'black',
  });
  assert.equal(ev, SOUND_EVENTS.GAME_END_LOSS);
});

test('stalemate -> gameEndDraw for either colour', () => {
  for (const myColor of ['white', 'black']) {
    const ev = eventForUpdate({
      prevFen: START_FEN,
      fen: AFTER_E4,
      inCheck: false,
      result: { outcome: 'draw', reason: 'stalemate' },
      myColor,
    });
    assert.equal(ev, SOUND_EVENTS.GAME_END_DRAW, `myColor=${myColor}`);
  }
});

test('resign / timeout / disconnect terminal result -> game-end event (not a move sound)', () => {
  // A resign update may arrive with an unchanged fen; the terminal result still
  // drives the cue.
  const resign = eventForUpdate({
    prevFen: START_FEN,
    fen: START_FEN,
    inCheck: false,
    result: { outcome: 'white_wins', reason: 'resignation' },
    myColor: 'white',
  });
  assert.equal(resign, SOUND_EVENTS.GAME_END_WIN);

  // Turn-timeout rides the same terminal-result DTO; the loser hears the loss cue.
  const timeout = eventForUpdate({
    prevFen: START_FEN,
    fen: START_FEN,
    inCheck: false,
    result: { outcome: 'white_wins', reason: 'timeout' },
    myColor: 'black',
  });
  assert.equal(timeout, SOUND_EVENTS.GAME_END_LOSS);

  const disconnect = eventForUpdate({
    prevFen: START_FEN,
    fen: START_FEN,
    inCheck: false,
    result: { outcome: 'black_wins', reason: 'opponent left' },
    myColor: 'black',
  });
  assert.equal(disconnect, SOUND_EVENTS.GAME_END_WIN);
});

// --- edge / priority cases ------------------------------------------------

test('a checkmating capture-with-check yields a game-end event, not capture/check', () => {
  // Terminal result + inCheck + a piece-count drop all at once: priority must
  // pick the game-end cue.
  const beforeMate = 'rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 2';
  const afterMate = 'rnbqkbnr/ppp1pppp/8/3P4/8/8/PPPP1PPP/RNBQKBNR b KQkq - 0 2';
  const ev = eventForUpdate({
    prevFen: beforeMate,
    fen: afterMate,
    inCheck: true,
    result: { outcome: 'white_wins', reason: 'checkmate' },
    myColor: 'white',
  });
  assert.equal(ev, SOUND_EVENTS.GAME_END_WIN);
});

test('no terminal result and no usable fen -> null (nothing to announce)', () => {
  assert.equal(
    eventForUpdate({ prevFen: START_FEN, fen: undefined, inCheck: false, result: undefined, myColor: 'white' }),
    null,
  );
  assert.equal(
    eventForUpdate({ prevFen: START_FEN, fen: '', inCheck: false, result: { outcome: 'ongoing' }, myColor: 'white' }),
    null,
  );
  // No args at all must not throw.
  assert.equal(eventForUpdate(), null);
});

test('malformed prevFen or fen falls back to move without throwing', () => {
  // Unparseable fen but present -> move (capture detection degrades, never throws).
  assert.equal(
    eventForUpdate({ prevFen: START_FEN, fen: 'not-a-fen', inCheck: false, result: { outcome: 'ongoing' }, myColor: 'white' }),
    SOUND_EVENTS.MOVE,
  );
  // Unparseable prevFen, valid fen -> move (cannot prove a capture).
  assert.equal(
    eventForUpdate({ prevFen: 'garbage', fen: AFTER_E4, inCheck: false, result: { outcome: 'ongoing' }, myColor: 'white' }),
    SOUND_EVENTS.MOVE,
  );
});

test('same piece count (no capture) -> move', () => {
  const ev = eventForUpdate({
    prevFen: START_FEN,
    fen: AFTER_E4,
    inCheck: false,
    result: undefined,
    myColor: 'white',
  });
  assert.equal(ev, SOUND_EVENTS.MOVE);
});

test('SOUND_SPECS has a recipe for every name in SOUND_EVENTS', () => {
  for (const name of Object.values(SOUND_EVENTS)) {
    const spec = SOUND_SPECS[name];
    assert.ok(spec, `missing SOUND_SPECS entry for "${name}"`);
    assert.ok(Array.isArray(spec.steps) && spec.steps.length > 0, `"${name}" has no steps`);
    for (const step of spec.steps) {
      assert.equal(typeof step.freq, 'number', `"${name}" step freq is a number`);
      assert.equal(typeof step.ms, 'number', `"${name}" step ms is a number`);
    }
  }
});
