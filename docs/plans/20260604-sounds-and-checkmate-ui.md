# Move/Capture/Check/Game-End Sounds + Prominent End-of-Game Overlay

## Overview

Two user-facing additions to the chess client:

1. **Sound effects** for four in-game events — **piece move**, **capture**,
   **check**, and **game end** — synthesized in the browser with the Web Audio
   API (no asset files, no backend changes, no build step).
2. **A more prominent end-of-game UI**: a centered result card ("Checkmate —
   You win", "Stalemate — Draw", etc.) shown over a dimmed board, with the
   existing **New game** button below it.

**Important scoping note:** game-end *detection* already works end-to-end and is
**not** being re-implemented. The backend (`engine.Result`) already classifies
checkmate→win / stalemate→draw and the hub already ships a terminal `ResultDTO`
on `game_update` (checkmate, stalemate, resign, timeout) and on
`opponent_disconnected`. The frontend reducer already transitions
`playing → over`. This plan only adds **sound** and a **more prominent overlay**
layered on top of that existing, tested detection.

Both features respect the project's core separation (CLAUDE.md): **pure,
DOM-free logic lives in testable ES modules; DOM/Web-Audio glue stays thin.** The
client continues to re-implement zero chess rules — sound classification reads
only the server-authoritative fields already on the wire (`fen`, `inCheck`,
`result`, `lastMove`).

## Context (from discovery)

Files/components involved:

- **`js/sound-events.js`** (NEW, pure) — classify a server update into a sound
  event and map events to synth specs.
- **`js/audio.js`** (NEW, thin glue) — Web Audio `AudioPlayer`; no-op when no
  `AudioContext` (Node/tests).
- **`js/board-model.js`** (existing, pure) — `parseBoard(fen)` is reused to count
  pieces for capture detection. May gain a tiny `pieceCount(fen)` helper.
- **`js/game-state.js`** (existing, pure) — add an `endgameHeadline(result,
  myColor)` helper for the overlay; reuses existing `isOver` / `playerOutcome`.
- **`js/app.js`** (existing, DOM glue) — wire sound playback on `game_update` /
  `opponent_disconnected`; render the new overlay in `renderGameOver()`.
- **`index.html`** — wrap `#board-root` in a positioned `.board-wrap` and add the
  `#game-over-overlay` result card over the board.
- **`style.css`** — overlay card + board-dim styles.
- Test files: `js/sound-events.test.js` (NEW), `js/game-state.test.js`
  (extend), plus reuse of `js/board-model.js` helpers.

Related patterns found:

- Pure-logic modules are unit-tested with Node's built-in runner
  (`node --test`), `node:assert/strict`, no DOM, no deps
  (see `js/game-state.test.js`, `js/multiplayer.test.js`).
- The wire already carries everything sound classification needs:
  - checkmate / stalemate / resign / timeout → `game_update` with terminal
    `result` (`backend/hub.go:444` for moves, `backend/hub.go:536`
    `broadcastGameResult` for resign/timeout).
  - opponent disconnect → `opponent_disconnected` with a `result`
    (`backend/hub.go:174`).
  - `inCheck` flag and `result` DTO already ride every `game_update`
    (`backend/types.go`, `backend/moves.go:96` `resultToDTO`).
- Existing end-UI: `#game-over` panel (`index.html:42`), `renderGameOver()`
  (`js/app.js:160`), board banner `over` tone (`js/chess.js:208`),
  styles at `style.css:220` and `:361`.

Dependencies identified:

- No new runtime dependencies. Web Audio API is a browser built-in.
- No backend changes (the static-file whitelist stays untouched — no audio
  assets are served).

## Development Approach

- **Testing approach**: **Regular** (implement, then add tests in the same task
  before moving on).
- Complete each task fully before moving to the next.
- Make small, focused changes; preserve the rules/transport/render separation.
- **CRITICAL: every task that changes code MUST include new/updated tests** in
  the same task (success + edge cases), listed as separate checklist items.
- **CRITICAL: all tests must pass before starting the next task.**
- Keep all pure logic DOM-free so `node --test` exercises it with zero deps.
- Run `make test` after each task.
- Maintain backward compatibility: existing game-end detection, the reducer
  transition, and the `#game-over` New-game button keep working unchanged.

## Testing Strategy

- **Unit tests** (required, every code task):
  - `js/sound-events.test.js` — event classification + spec mapping (pure).
  - `js/game-state.test.js` — extend for `endgameHeadline` (pure).
  - `js/board-model.test.js` — extend if a `pieceCount` helper is added.
- **No DOM/e2e harness exists** in this repo (frontend tests are pure-logic
  only). The Web Audio glue (`js/audio.js`) and the DOM overlay rendering in
  `app.js`/`index.html`/`style.css` are presentation glue and are verified
  manually (see Post-Completion). Their *logic* is pushed into the pure,
  unit-tested modules above so the untested surface stays thin.
- **Backend**: `cd backend && go test ./...` must stay green (no backend code
  changes are planned; this is a regression guard).

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update this plan if scope changes during implementation.

## What Goes Where

- **Implementation Steps** (`[ ]`): code, unit tests, doc updates — all
  automatable by the agent.
- **Post-Completion** (no checkboxes): manual browser verification of actual
  audio playback and the overlay look/feel (requires a real browser + audio
  output, which the agent cannot do).

## Implementation Steps

### Task 1: Pure sound-event classification module (`js/sound-events.js`)

Create a DOM-free module that decides which single sound an incoming update
should play, plus the synth spec for each event. Single-sound-per-update with a
clear priority so a checkmating capture-with-check plays only the end sound.

- [x] create `js/sound-events.js` exporting `SOUND_EVENTS` (the event names:
      `'move'`, `'capture'`, `'check'`, `'gameEndWin'`, `'gameEndLoss'`,
      `'gameEndDraw'`) and `SOUND_SPECS` (event → `{ steps: [{freq, ms, type?}] }`
      synth recipes; single-tone events use a one-element `steps` array, game-end
      events use a short multi-tone arpeggio — descending for loss/draw, rising
      for win).
- [x] implement `eventForUpdate({ prevFen, fen, inCheck, result, myColor })`
      returning one event name or `null`, with priority **game-end > check >
      capture > move**:
      - terminal `result` (use existing `isOver` semantics) →
        `gameEndWin`/`gameEndLoss`/`gameEndDraw` resolved against `myColor`
        (reuse the same win/lose logic as `playerOutcome`);
      - else `inCheck` truthy → `'check'`;
      - else capture (fewer pieces in `fen` than `prevFen`, via `parseBoard`
        piece counts — this also covers en-passant since a pawn disappears) →
        `'capture'`;
      - else a real move (a valid `fen` is present) → `'move'`;
      - else (no usable position, e.g. resign with unchanged fen but no terminal
        result — should not happen) → `null`.
- [x] import `parseBoard` from `./board-model.js` for piece counting; on a parse
      error fall back to `'move'` (never throw out of classification).
- [x] write tests in `js/sound-events.test.js` (success cases): plain move,
      capture (piece count drops), en-passant capture, check, checkmate→win,
      checkmate→loss (opposite `myColor`), stalemate→draw, resign/disconnect
      terminal result → game-end event.
- [x] write tests (edge/priority cases): checkmate that is also a capture+check
      yields a `gameEnd*` event (not capture/check); `null`/missing `result`
      with no fen change handled; malformed `prevFen`/`fen` falls back to
      `'move'` without throwing; `SOUND_SPECS` has an entry for every name in
      `SOUND_EVENTS`.
- [x] run `make test` — must pass before Task 2.

### Task 2: Web Audio playback glue (`js/audio.js`)

A thin, side-effecting wrapper that turns a synth spec into sound. Safe to import
under Node (no `AudioContext`) so it never breaks the test runner.

- [x] create `js/audio.js` exporting an `AudioPlayer` class: lazily creates a
      single `AudioContext` on first use; `play(spec)` schedules each step as an
      oscillator → gain (short attack/release envelope to avoid clicks);
      `playEvent(eventName)` looks up `SOUND_SPECS[eventName]` and calls `play`.
- [x] guard for environments without Web Audio: if
      `typeof AudioContext === 'undefined' && typeof webkitAudioContext ===
      'undefined'`, all methods no-op (so importing/constructing is safe in
      Node).
- [x] handle browser autoplay policy: expose `resume()` (calls
      `AudioContext.resume()` if suspended) and `unlockOnFirstGesture(target)`
      that attaches a one-time `pointerdown`/`keydown` listener to create/resume
      the context after a user gesture; tolerate a missing/late context.
- [x] add a `muted` flag + `setMuted(bool)` so playback can be globally silenced
      (wiring a UI toggle is out of scope; the flag keeps that cheap later).
- [x] write tests in `js/audio.js`-adjacent coverage where pure: since
      `AudioContext` is absent under `node --test`, add a focused test asserting
      `new AudioPlayer()` constructs and `playEvent('move')` / `play(spec)` /
      `setMuted(true)` are **no-throw no-ops** in a non-browser environment
      (guards the Node-safety contract). Note in a comment that real playback is
      verified manually (Post-Completion).
- [x] run `make test` — must pass before Task 3.

### Task 3: Wire sounds into the client (`js/app.js`)

Play the classified sound from the single authoritative update points. Capture
detection needs the pre-update FEN, which `BoardView` still holds before
`board.update(msg)` overwrites it.

- [x] construct a single `AudioPlayer` in `app.js`; call
      `audio.unlockOnFirstGesture(document)` once at startup so the first
      challenge/accept/board click unlocks audio.
- [x] in the `game_update` handler: read `const prevFen = board.fen` **before**
      `board.update(msg)`, then after dispatch call
      `audio.playEvent(eventForUpdate({ prevFen, fen: msg.fen, inCheck:
      msg.inCheck, result: msg.result, myColor: board.orientation }))`
      (guard the `null` event).
- [x] in the `opponent_disconnected` handler: play the game-end event
      (terminal `result` is present on the message; classify with the same
      helper so win/lose/draw is correct).
- [x] do **not** play a sound on `game_start` (opening position is not a move);
      confirm no other handler emits spurious sounds.
- [x] write/extend tests: this wiring is DOM glue (not unit-tested directly), so
      ensure the *logic* it calls is fully covered by Task 1 tests; add any
      missing `eventForUpdate` case discovered while wiring. Re-run
      `make test` — must pass before Task 4.

### Task 4: Pure end-of-game headline helper (`js/game-state.js`)

Give the overlay a tested, player-relative headline instead of computing strings
inline in DOM glue.

- [x] add `endgameHeadline(result, myColor)` to `js/game-state.js` returning a
      structured `{ title, detail }` — e.g. `title` from the reason
      (`'Checkmate'` / `'Stalemate'` / `'Resignation'` / `'Timeout'` /
      `'Disconnected'`, title-cased; fallback to a generic `'Game over'`),
      `detail` from `playerOutcome` (`'You win'` / `'You lose'` / `'Draw'`).
- [x] reuse existing `isOver` / `playerOutcome`; return `null` when the result is
      not terminal.
- [x] write tests in `js/game-state.test.js` (success): checkmate as winner,
      checkmate as loser, stalemate→draw, resignation, timeout.
- [x] write tests (edge): unknown/empty reason → generic title; non-terminal /
      `null` result → `null`; correct relativity for both `myColor` values.
- [x] run `make test` — must pass before Task 5.

### Task 5: End-of-game overlay markup + styles (`index.html`, `style.css`)

Add the centered result card over the board and dim the board behind it. Keep the
existing **New game** button below the board.

- [x] in `index.html`, wrap `#board-root` in a `.board-wrap` (positioning
      context) and add a sibling `#game-over-overlay` card inside it containing
      `#game-over-title` and `#game-over-detail`; leave the existing `#game-over`
      panel below the board as the **New game** action (its `#back-to-menu`
      button + wiring are unchanged).
- [x] in `style.css`, style `.board-wrap { position: relative }`,
      `#game-over-overlay` as an absolutely-centered card (readable contrast,
      rounded, drop shadow) hidden by default, and dim the board when over
      (e.g. `.board-wrap.over #board-root { opacity: .45 }` plus
      `pointer-events: none` so the dead board can't be clicked).
- [x] ensure the overlay does not break the existing board banner `over` tone or
      the below `#game-over` panel layout.
- [x] tests: HTML/CSS only — no unit tests apply; the gating logic lives in the
      tested `endgameHeadline`/reducer. Add a one-line note in the plan's
      Post-Completion for manual visual check.
- [x] run `make test` — must stay green before Task 6.

### Task 6: Render the overlay (`js/app.js` `renderGameOver()`)

Drive the new overlay from existing reducer state; toggle the board-dim class.

- [x] cache the new elements (`gameOverOverlay`, `gameOverTitle`,
      `gameOverDetail`, `boardWrap`) in the `els` map.
- [x] in `renderGameOver()`: when `ui.phase === PHASE.OVER` and `ui.game.result`,
      compute `endgameHeadline(g.result, g.myColor)`, fill title/detail, show the
      overlay, and add the `over` class to `.board-wrap`; otherwise hide the
      overlay and remove the class. Keep the existing below-board `#game-over`
      panel (New game button) behavior intact.
- [x] ensure `returnToMenu` (New game) and the `connection_lost` path both clear
      the overlay and the board-dim class (re-render covers this).
- [x] tests: DOM glue (not unit-tested); rely on Task 4 coverage of
      `endgameHeadline`. Re-run `make test` — must pass before Task 7.

### Task 7: Verify acceptance criteria

- [ ] verify all four sound events classify correctly via the unit suite (move,
      capture, check, game-end win/loss/draw).
- [ ] verify game-end-via-resign, via-timeout, and via-opponent-disconnect all
      classify to a game-end event (not a move sound).
- [ ] verify the overlay shows the right headline for win/loss/draw and that the
      New-game button still returns to the menu and clears the overlay.
- [ ] run the full unit suite: `node --test` (frontend) and
      `cd backend && go test ./...` (backend regression) — all green.
- [ ] run `make test` (the single CI-matching entry point) — all green.
- [ ] confirm no backend files changed and the static-file whitelist is
      untouched (no audio assets served).

### Task 8: [Final] Update documentation

- [ ] update `README.md` if it enumerates client modules / behavior (add the
      sound + overlay note; no new run/build steps).
- [ ] update `CLAUDE.md` rendering section (`js/`) to list `sound-events.js`
      (pure) and `audio.js` (thin glue), reinforcing the
      pure-logic-vs-DOM-glue split, and note "client plays sounds derived only
      from server-authoritative fields; it still re-implements no rules."
- [ ] document the move/capture/check/game-end sound decision and the
      "single sound per update, priority game-end > check > capture > move"
      rule under CLAUDE.md "Documented rule decisions" (or a short rendering
      note).

*Note: ralphex automatically moves completed plans to `docs/plans/completed/`.*

## Technical Details

**Sound classification (pure, `js/sound-events.js`):**

- Input: `{ prevFen, fen, inCheck, result, myColor }` — all already available on
  the client at each update point.
- Output: one of `SOUND_EVENTS` or `null`.
- Priority (highest first): terminal `result` → `check` → `capture` → `move`.
  This guarantees exactly one sound per server update and that a decisive move
  (e.g. checkmating capture) plays the game-end cue, not a capture/move cue.
- Capture detection: `pieceCount(fen) < pieceCount(prevFen)` using
  `parseBoard`. Robust across normal captures and en passant (a pawn vanishes).
  Parse failure → treat as a plain `move`.
- Game-end relativity: resolve `gameEndWin` vs `gameEndLoss` against `myColor`
  using the same logic as the existing `playerOutcome`; `draw` → `gameEndDraw`.

**Synth specs (`SOUND_SPECS`):** each event maps to `{ steps: [{ freq, ms,
type }] }`. Single-tone for move/capture/check (distinct pitches, short
durations); short arpeggios for game end (rising for win, descending for
loss/draw). Tunable constants kept in one place.

**Web Audio glue (`js/audio.js`):** one shared `AudioContext`, lazily created;
each step = `OscillatorNode → GainNode → destination` with a tiny linear
attack/release to avoid clicks. No-ops entirely when Web Audio is unavailable
(Node/tests). Autoplay policy handled via a one-time gesture unlock + `resume()`.

**End overlay (DOM):** `.board-wrap` is the positioning context;
`#game-over-overlay` is absolutely centered over `#board-root`; the board dims
(opacity + `pointer-events: none`) while over. The existing below-board
`#game-over` panel keeps the **New game** button and its wiring. Headline text
comes from the pure, tested `endgameHeadline`.

**No backend / wire changes:** all required fields (`fen`, `inCheck`, `result`,
`lastMove`) are already broadcast. The static-file whitelist
(`backend/main.go`) is intentionally left alone since no audio files are served.

## Post-Completion

*Items requiring manual intervention — no checkboxes, informational only.*

**Manual verification (real browser + audio):**

- Open two browser tabs, start a game, and confirm:
  - a normal move plays the move sound on both sides;
  - a capture plays the distinct capture sound (incl. an en-passant capture);
  - a checking move plays the check sound;
  - delivering checkmate plays the game-end sound (win on the mating side, loss
    on the mated side) — and only the game-end sound, not a capture/move sound;
  - stalemate plays the draw game-end sound;
  - resign, turn-timeout, and closing the opponent's tab each play the
    game-end sound on the remaining player.
- Confirm audio unlocks correctly after the first user gesture (no console
  autoplay warnings after the first click).
- Confirm the centered result card renders over the dimmed board with the right
  headline (win/loss/draw + reason), the board can't be clicked while over, and
  **New game** clears the overlay and returns to the menu.
- Visual check (Task 5, HTML/CSS only): the `#game-over-overlay` card is centered
  over `#board-root` within `.board-wrap`, the board dims (`opacity: .45` +
  `pointer-events: none`) only when `.board-wrap.over` is set, and the below-board
  `#game-over` New-game panel keeps its existing layout.

**Optional follow-ups (out of scope this iteration):**

- A mute/volume toggle in the UI (the `AudioPlayer.muted` flag is already there
  to make this cheap).
- Replacing synthesized tones with recorded audio assets later (would require
  widening the `backend/main.go` static whitelist and adding a Docker
  static-copy step — deliberately avoided now).
