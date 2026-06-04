// sound-events.js — pure, DOM-free classification of a server game update into
// the single sound it should play, plus the synth recipe for each event.
//
// The client re-implements zero chess rules here: classification reads only the
// server-authoritative fields already on every update (`fen`, `inCheck`,
// `result`) and the viewing player's colour. It never computes legality. The
// Web-Audio glue that turns a recipe into actual sound lives in audio.js; this
// module is side-effect-free and importable under `node --test`.
//
// One sound per update. A strict priority — game-end > check > capture > move —
// guarantees a decisive move (e.g. a checkmating capture that also gives check)
// plays only the game-end cue, never a capture/check/move cue underneath it.

import { parseBoard } from './board-model.js';

// SOUND_EVENTS are the event names eventForUpdate can return. Single-tone events
// (move/capture/check) are everyday cues; the three gameEnd* events are the
// terminal cue resolved to the viewing player's perspective.
export const SOUND_EVENTS = {
  MOVE: 'move',
  CAPTURE: 'capture',
  CHECK: 'check',
  GAME_END_WIN: 'gameEndWin',
  GAME_END_LOSS: 'gameEndLoss',
  GAME_END_DRAW: 'gameEndDraw',
};

// SOUND_SPECS maps each event name to a synth recipe: an ordered list of tone
// `steps`, each `{ freq (Hz), ms (duration), type (oscillator wave) }`. Single
// tones for the everyday cues (distinct pitches so they are told apart); short
// arpeggios for game end — rising for a win, descending for a loss/draw. All the
// tunable constants live here so the palette can be adjusted in one place.
export const SOUND_SPECS = {
  // A soft mid sine — the quiet, common case.
  [SOUND_EVENTS.MOVE]: { steps: [{ freq: 440, ms: 90, type: 'sine' }] },
  // A brighter, slightly punchy triangle to mark a capture.
  [SOUND_EVENTS.CAPTURE]: { steps: [{ freq: 660, ms: 110, type: 'triangle' }] },
  // A tense high square to flag check.
  [SOUND_EVENTS.CHECK]: { steps: [{ freq: 880, ms: 140, type: 'square' }] },
  // Rising major arpeggio — victory.
  [SOUND_EVENTS.GAME_END_WIN]: {
    steps: [
      { freq: 523.25, ms: 130, type: 'sine' }, // C5
      { freq: 659.25, ms: 130, type: 'sine' }, // E5
      { freq: 783.99, ms: 220, type: 'sine' }, // G5
    ],
  },
  // Descending minor arpeggio — defeat.
  [SOUND_EVENTS.GAME_END_LOSS]: {
    steps: [
      { freq: 523.25, ms: 130, type: 'sine' }, // C5
      { freq: 415.3, ms: 130, type: 'sine' }, //  G#4
      { freq: 311.13, ms: 240, type: 'sine' }, // D#4
    ],
  },
  // Gentle descending pair — a draw: neither triumphant nor grim.
  [SOUND_EVENTS.GAME_END_DRAW]: {
    steps: [
      { freq: 523.25, ms: 150, type: 'sine' }, // C5
      { freq: 392.0, ms: 230, type: 'sine' }, //  G4
    ],
  },
};

// isTerminal reports whether a ResultDTO marks the game finished. Mirrors
// game-state.isOver so classification and the reducer agree on "over"; kept local
// to avoid an import cycle and to keep this module dependency-light.
function isTerminal(result) {
  return Boolean(result && result.outcome && result.outcome !== 'ongoing');
}

// gameEndEventFor resolves a terminal result to the viewing player's cue. Uses
// the same win/lose logic as game-state.playerOutcome: a draw is a draw; else the
// side that won is compared against myColor.
function gameEndEventFor(result, myColor) {
  if (result.outcome === 'draw') return SOUND_EVENTS.GAME_END_DRAW;
  const iAmWhite = myColor === 'white';
  const whiteWon = result.outcome === 'white_wins';
  return whiteWon === iAmWhite ? SOUND_EVENTS.GAME_END_WIN : SOUND_EVENTS.GAME_END_LOSS;
}

// pieceCount returns how many pieces stand on a FEN's board, or null if the FEN
// cannot be parsed (so callers can fall back rather than throw).
function pieceCount(fen) {
  try {
    return parseBoard(fen).reduce((n, sq) => (sq ? n + 1 : n), 0);
  } catch {
    return null;
  }
}

// eventForUpdate decides the single sound an incoming update should play, or
// null if none applies. Priority (highest first): terminal result -> check ->
// capture -> move.
//
//   - terminal `result` -> gameEndWin / gameEndLoss / gameEndDraw vs `myColor`;
//   - else `inCheck` truthy -> 'check';
//   - else a capture (fewer pieces in `fen` than `prevFen`; this also covers
//     en passant, where a pawn disappears) -> 'capture';
//   - else a real move (a usable `fen` is present) -> 'move';
//   - else (no usable position and no terminal result) -> null.
//
// A FEN that fails to parse never throws out of here: capture detection simply
// falls back, so a malformed-but-present `fen` still yields a 'move'.
export function eventForUpdate({ prevFen, fen, inCheck, result, myColor } = {}) {
  if (isTerminal(result)) return gameEndEventFor(result, myColor);
  if (inCheck) return SOUND_EVENTS.CHECK;

  // No usable new position and not terminal: nothing to announce.
  if (typeof fen !== 'string' || fen.length === 0) return null;

  const after = pieceCount(fen);
  const before = pieceCount(prevFen);
  // Only call it a capture when both counts are known and the board shrank;
  // an unparseable side degrades to a plain move rather than a false capture.
  if (after !== null && before !== null && after < before) {
    return SOUND_EVENTS.CAPTURE;
  }
  return SOUND_EVENTS.MOVE;
}
