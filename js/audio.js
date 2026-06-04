// audio.js — thin, side-effecting Web Audio glue: turns a pure synth recipe from
// sound-events.js into actual sound. All rules/classification logic lives in the
// pure module; this file only schedules oscillators.
//
// Node-safety contract: this module must import and construct cleanly under
// `node --test`, where there is no AudioContext. When Web Audio is unavailable
// every method is a no-throw no-op, so importing app.js (which constructs an
// AudioPlayer) never breaks the pure-logic test runner. Real playback is verified
// manually in a browser (see the plan's Post-Completion section).

import { SOUND_SPECS } from './sound-events.js';

// Resolve the Web Audio constructor once. `typeof` on an undeclared global is
// safe (yields 'undefined'), so this is fine under Node where neither name
// exists. webkitAudioContext covers older Safari.
const AudioCtx =
  typeof AudioContext !== 'undefined'
    ? AudioContext
    : typeof webkitAudioContext !== 'undefined'
      ? webkitAudioContext
      : null;

// Envelope/level constants, kept here so the feel can be tuned in one place.
const ATTACK_S = 0.005; // 5ms linear fade-in to avoid a click on note start
const RELEASE_S = 0.04; // 40ms linear fade-out to avoid a click on note end
const PEAK_GAIN = 0.18; // gentle peak so cues are audible but never harsh
const TAIL_S = 0.02; // small stop() pad past the envelope end

export class AudioPlayer {
  constructor() {
    this.ctx = null; // the single shared AudioContext, created lazily on first use
    this.muted = false;
    this.supported = AudioCtx !== null;
  }

  // ensureContext lazily creates the one shared AudioContext on first use and
  // returns it, or null when Web Audio is unavailable (so callers can no-op). A
  // construction failure flips `supported` off so we never retry.
  ensureContext() {
    if (!this.supported) return null;
    if (!this.ctx) {
      try {
        this.ctx = new AudioCtx();
      } catch {
        this.supported = false;
        return null;
      }
    }
    return this.ctx;
  }

  // setMuted globally silences (or re-enables) playback. A UI toggle is out of
  // scope; this flag keeps wiring one cheap later.
  setMuted(value) {
    this.muted = Boolean(value);
  }

  // resume nudges a suspended context back to running (browser autoplay policy).
  // Safe when there is no context yet or Web Audio is unavailable.
  resume() {
    const ctx = this.ctx;
    if (ctx && ctx.state === 'suspended' && typeof ctx.resume === 'function') {
      // The returned promise is intentionally ignored; a rejection is non-fatal.
      try {
        ctx.resume();
      } catch {
        /* ignore */
      }
    }
  }

  // unlockOnFirstGesture attaches a one-time pointerdown/keydown listener that
  // creates/resumes the context on the first user gesture, satisfying autoplay
  // policy so the first cue isn't swallowed. Tolerates a missing target and a
  // missing/late context.
  unlockOnFirstGesture(target) {
    if (!this.supported || !target || typeof target.addEventListener !== 'function') return;
    const unlock = () => {
      this.ensureContext();
      this.resume();
      target.removeEventListener('pointerdown', unlock);
      target.removeEventListener('keydown', unlock);
    };
    target.addEventListener('pointerdown', unlock, { once: true });
    target.addEventListener('keydown', unlock, { once: true });
  }

  // playEvent looks up the recipe for an event name and plays it. Unknown names
  // (or a null event from the classifier) no-op.
  playEvent(eventName) {
    const spec = SOUND_SPECS[eventName];
    if (spec) this.play(spec);
  }

  // play schedules each step as oscillator -> gain -> destination, with a short
  // linear attack/release envelope to avoid clicks. Steps play in sequence. No-op
  // when muted, given a bad spec, or when Web Audio is unavailable.
  play(spec) {
    if (this.muted || !spec || !Array.isArray(spec.steps)) return;
    const ctx = this.ensureContext();
    if (!ctx) return;
    this.resume();

    let t = ctx.currentTime;
    for (const step of spec.steps) {
      // Floor the tone at attack+release so the envelope below always ramps
      // monotonically — a tone shorter than the attack would otherwise schedule
      // the hold after the release. Every real spec step is far longer.
      const dur = Math.max(ATTACK_S + RELEASE_S, (step.ms || 0) / 1000);
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = step.type || 'sine';
      osc.frequency.value = step.freq || 440;

      // Linear attack up to peak, hold, then linear release back to silence.
      const holdEnd = t + Math.max(ATTACK_S, dur - RELEASE_S);
      gain.gain.setValueAtTime(0, t);
      gain.gain.linearRampToValueAtTime(PEAK_GAIN, t + ATTACK_S);
      gain.gain.setValueAtTime(PEAK_GAIN, holdEnd);
      gain.gain.linearRampToValueAtTime(0, t + dur);

      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start(t);
      osc.stop(t + dur + TAIL_S);
      t += dur;
    }
  }
}
