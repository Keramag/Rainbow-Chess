// Tests for the Web Audio glue. Run with `node --test`.
//
// There is no AudioContext under `node --test`, so these only pin down the
// Node-safety contract: importing, constructing, and calling every method must be
// a no-throw no-op when Web Audio is unavailable. That guard is what lets app.js
// (which constructs an AudioPlayer) be imported by the pure-logic suite without
// breaking it. Real, audible playback is verified manually in a browser (see the
// plan's Post-Completion section).
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { AudioPlayer } from './audio.js';
import { SOUND_SPECS } from './sound-events.js';

test('constructs without Web Audio and reports unsupported', () => {
  const p = new AudioPlayer();
  assert.equal(p.supported, false);
  assert.equal(p.ctx, null);
  assert.equal(p.muted, false);
});

test('playEvent / play / setMuted are no-throw no-ops in a non-browser env', () => {
  const p = new AudioPlayer();
  assert.doesNotThrow(() => p.playEvent('move'));
  assert.doesNotThrow(() => p.play(SOUND_SPECS.move));
  assert.doesNotThrow(() => p.playEvent('gameEndWin'));

  assert.doesNotThrow(() => p.setMuted(true));
  assert.equal(p.muted, true);
  assert.doesNotThrow(() => p.play(SOUND_SPECS.capture));
  assert.doesNotThrow(() => p.setMuted(false));
  assert.equal(p.muted, false);

  // No context was ever created since Web Audio is unavailable.
  assert.equal(p.ctx, null);
});

test('unknown event names and malformed specs do not throw', () => {
  const p = new AudioPlayer();
  assert.doesNotThrow(() => p.playEvent('does-not-exist'));
  assert.doesNotThrow(() => p.playEvent(undefined));
  assert.doesNotThrow(() => p.play(undefined));
  assert.doesNotThrow(() => p.play({}));
  assert.doesNotThrow(() => p.play({ steps: 'nope' }));
});

test('resume and unlockOnFirstGesture tolerate a missing context/target', () => {
  const p = new AudioPlayer();
  assert.doesNotThrow(() => p.resume());
  assert.doesNotThrow(() => p.unlockOnFirstGesture(undefined));
  assert.doesNotThrow(() => p.unlockOnFirstGesture(null));

  // A target with addEventListener is tolerated; with Web Audio unavailable no
  // listeners are attached (nothing to unlock), and nothing throws.
  const fakeTarget = {
    addEventListener() {},
    removeEventListener() {},
  };
  assert.doesNotThrow(() => p.unlockOnFirstGesture(fakeTarget));
});

test('ensureContext returns null when Web Audio is unavailable', () => {
  const p = new AudioPlayer();
  assert.equal(p.ensureContext(), null);
});
