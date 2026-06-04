# CLAUDE.md

Guidance for working in this repository. Read this before changing rules,
transport, or rendering — the layering below is deliberate and is what keeps new
variants cheap.

## What this is

A variant-ready chess platform: two anonymous players play 1v1 over WebSockets.
Standard chess is the base; named **variants** embed it and override only the
rules they change. See `README.md` for how to run and play, and `Prd.md` for the
Rainbow spec.

## The three layers (keep them separate)

The single most important architectural rule: **rules, transport, and rendering
never leak into each other.** This separation is what makes future variants cheap
and keeps the client from re-implementing chess.

### 1. Rules — `backend/engine/` (pure, no I/O)

The engine knows chess and nothing else. No networking, no database, no logging.

- `types.go` — `Color`, `PieceType`, `Piece`, `Square`, `Move`, `GameResult`.
- `board.go` / `fen.go` — `Position` (board, side to move, castling rights, en
  passant, clocks), get/set/clone, algebraic↔square, the `Mirror(x)=7-x` helper,
  and FEN parse/generate.
- `attacks.go` — `IsSquareAttacked`, `KingSquare`, `IsInCheck`.
- `movegen.go` — `PseudoLegalMoves` (pawn/knight/sliders/king incl. castling).
- `legal.go` — `LegalMoves` (pseudo-legal filtered by "is my king left in
  check?"), `ApplyMove` (returns a **new** `Position`), and `Result`.
- `variant.go` / `standard.go` / `rainbow.go` — the variant abstraction (below).

`Position` is **immutable by convention**: `ApplyMove` returns a fresh
`*Position` rather than mutating. This simplifies legality testing (apply-and-test
without undo) and history.

Move legality = pseudo-legal generation + a "does my king end up attacked?"
filter. Attack detection drives check, checkmate, castling-through-check, and
pins — there is one source of truth for "attacked," reused everywhere.

Pawn logic (push direction, start rank, promotion rank) is derived from the
piece's **`Color`**, never from which half of the board it sits on. This is the
key generality that lets the same generator serve Rainbow, where colours are
scattered across both home ranks.

`perft` (move-count) tests are the strongest guard against move-gen regressions;
keep them green and add positions when touching generation.

### 2. Transport — `backend/` (hub / WebSocket)

- `main.go` — entry: serves static frontend + `GET /ws`, no-cache for JS/CSS,
  opens the SQLite store and wires `hub.gameEnded` to `SaveGame`.
- `hub.go` — the heart: a **single goroutine** owns all shared state and
  processes `register` / `unregister` / `handleMessage` off channels. There are
  no mutexes on hub state because only that goroutine touches it. Handles the
  challenge lifecycle (30s expiry), the move protocol, resign, the per-turn
  auto-resign timer, and game-end persistence.
- `client.go` — per-connection read/write pumps + ping.
- `types.go` — the WS `Message` envelope and `User` / `Challenge` / `Game`.
- `names.go` — random anonymous usernames (`Adjective+Animal+NN`).
- `storage.go` — SQLite (`modernc.org/sqlite`); async save on game end.

The transport layer calls the engine through the **`Variant`** interface only. It
never inspects piece colours or computes legality itself; it asks the variant for
`LegalMoves` / `ApplyMove` / `Result`. Adding a variant therefore touches **zero**
transport code.

### 3. Rendering — `js/` (vanilla ES modules, no build step)

Served directly by the Go server and loaded via `<script type="module">`. Pure
logic lives in DOM-free modules so Node's built-in test runner (`node --test`)
can exercise them with zero dependencies; DOM glue is kept thin.

- `multiplayer.js` — `MultiplayerClient`: `connect` / `send` and a `handleMessage`
  dispatcher. DOM-free state-updater; handlers registered via `on()`.
- `variants.js` — parse the `welcome` variant list, populate the picker.
- `board-model.js` — **pure**: FEN→8×8 model, square↔coordinate mapping,
  highlight-target derivation from the server's legal-move list.
- `sound-events.js` — **pure**: classify a server update into one sound event
  (`eventForUpdate`) and map events to synth specs (`SOUND_SPECS`). DOM-free and
  unit-tested; reads only server-authoritative fields (`fen`, `inCheck`,
  `result`) — no chess rules.
- `audio.js` — **thin glue**: Web Audio `AudioPlayer` that synthesizes the specs;
  no-ops entirely when no `AudioContext` exists (so it's safe to import under
  `node --test`). Autoplay policy handled via a one-time gesture unlock.
- `chess.js` — `BoardView`: DOM board render (color-correct pieces — no
  assumption that bottom = white) + click-to-move + promotion picker.
- `game-state.js` — **pure reducer**: high-level screen state
  (`menu` / `playing` / `over`), player/variant context, transient notices, and
  `endgameHeadline` for the end-of-game overlay.
- `app.js` — the only module that touches the page shell; owns no rules logic.
  Wires sound playback and the end-of-game overlay from reducer state.

The client **never re-implements chess rules.** It highlights from the
server-provided legal-move list, shows check from the server's `inCheck` flag, and
limits the promotion picker to the pieces present in the legal-move list. It also
**plays sounds derived only from server-authoritative fields** (`fen`, `inCheck`,
`result`) — e.g. capture is detected by a piece-count drop in the FEN, never by
re-deriving the move. If you find yourself computing legality in JS, push it to
the server instead.

## FEN as the wire format (decision)

Positions always travel as **FEN**, in both directions of the game protocol.

- FEN encodes per-square piece **type and colour**, so it serializes a Rainbow
  position (mixed colours on both home ranks) exactly as well as a standard one —
  one wire format for every variant.
- The server is authoritative: every `game_start` / `game_update` carries the FEN
  plus side-to-move, an `inCheck` flag, the full legal-move list for the side to
  move, the last move, and the result. The client renders from that and nothing
  else.
- Moves on the wire are algebraic squares + an optional promotion letter
  (`{from, to, promotion?}`), e.g. `e7→e8` `promotion:"n"`.

See `README.md` → "WebSocket protocol" for the full message list.

## The variant-extension pattern

```go
type Variant interface {
    Name() string
    InitialPosition() *Position
    LegalMoves(*Position) []Move
    ApplyMove(*Position, Move) (*Position, error)
    Result(*Position) GameResult
    PromotionPieces() []PieceType
}
```

- **`Standard`** implements the whole interface by delegating to the engine's
  package-level functions. Its two configurable knobs — the **name** and the
  **promotion whitelist** — are *fields*, not hard-coded returns. This matters:
  Go embedding promotes methods but does **not** do virtual dispatch. When a
  variant embeds `Standard` and a caller invokes the inherited `ApplyMove`, that
  method runs with the embedded `*Standard` receiver and can only see the
  embedded `Standard`'s state. Reading name/promotions from fields lets a variant
  configure `Standard` once in its constructor and correctly inherit `Name`,
  `PromotionPieces`, and the promotion-restricting `ApplyMove`.
- **`Rainbow`** embeds `Standard`, sets `name:"rainbow"` and
  `promotions:{Knight,Bishop}`, and overrides **only** `InitialPosition` (the
  symmetric colour assignment). Everything else is inherited. This is the proof
  the abstraction works: a real, distinct variant in a few dozen lines.
- **Registry**: variants call `Register(name, v)` from an `init()`.
  `engine.List()` feeds the frontend variant picker (sorted, stable order);
  `engine.Get(name)` is used at game creation. The transport layer learns about a
  new variant for free.

To add a variant: new file in `backend/engine/`, embed `Standard`, override the
methods whose rules differ, `Register` in `init()`, add tests. See `README.md` →
"How to add a new variant" for a copy-paste skeleton.

## Documented rule decisions

- **Rainbow pawns move by colour, not board half** (white toward rank 8, black
  toward rank 1) — derived from `Color` in the shared engine, so the unchanged
  Standard generator serves Rainbow. A white pawn that lands on rank 7 can promote
  almost immediately; intended.
- **Rainbow inherits castling and en passant unchanged**; only colouring and the
  promotion whitelist differ.
- **Rainbow keeps kings and queens native; no king starts in check.** The
  colouring shuffles every piece in symmetric mirror pairs **except** each side's
  king and queen — those keep their native colour (white d1/e1, black d8/e8), so
  the central d/e back-rank pair is deliberately same-side and is exempt from the
  mirror-symmetry invariant (`validate` skips royalty). Because a recoloured pawn
  can still attack a native king, `Rainbow.InitialPosition` re-rolls until
  **neither** king is in check (and White has a reply), guaranteeing a game never
  begins in check or already over. `buildInitialPosition` stays the pure
  single-shot primitive tests assert against. *(This supersedes the earlier
  "both kings start in check" design.)*
- **Player colour is randomised per game.** When a challenge is accepted the hub
  flips a coin (`Hub.coinFlip`, seeded in `newHub`) to decide which player is
  White, so you don't always play the same colour. Both players are told their
  colour on `game_start`, so nothing downstream depends on who challenged. Tests
  override `coinFlip` for determinism.
- **Draw detection is stalemate only** this iteration. Repetition, 50-move, and
  insufficient-material draws are out of scope. Checkmate = win, stalemate = draw.
- **The board shows edge coordinates and a1 is dark.** Square colour is pure
  (`squareColor`, a1 dark by parity); `edgeCoordinates` derives file letters along
  the bottom row and rank numbers down the left column, oriented for the viewing
  player (so the labels follow the board flip). Rendering stays in `chess.js`.
- **Move/capture/check/game-end sounds are client-side, one per update.** The
  client synthesizes four cues (move, capture, check, game-end) with the Web Audio
  API — no asset files, no backend changes. Classification is pure
  (`sound-events.js`) and reads only server-authoritative fields. Exactly **one
  sound per server update**, with strict priority **game-end > check > capture >
  move**, so a decisive move (e.g. a checkmating capture-with-check) plays only the
  game-end cue. Game-end is relative to the player (`gameEndWin` / `gameEndLoss` /
  `gameEndDraw`, resolved against `myColor` like `playerOutcome`).
- **Winning is silly on purpose: a pig that brays like a donkey.** On a win the
  end-of-game overlay pops a 🐷 and the `gameEndWin` cue is a synthesized donkey
  "hee-haw" (`donkeyBray` in `sound-events.js`) — still Web-Audio-only, no asset
  files. A real recorded bray would sound better but would need a served audio
  file and a backend static-whitelist change, so it's intentionally left synthesized.

## Testing

`make test` is the single entry point (CI and local match):

- Backend: `cd backend && go test ./...` — engine table-driven tests + `perft`,
  hub connect/disconnect, challenge lifecycle, move/turn enforcement, game-end +
  persistence. The engine package is correctness-critical; keep coverage high
  (currently ~96%).
- Frontend: `node --test` — pure logic only (`board-model`, `variants`,
  `multiplayer` dispatch, `game-state` reducer). No DOM, no deps.

**Every change to code must come with tests in the same change** (success + edge
cases). All tests must pass before moving on. Maintain the rules/transport/render
separation — that is the property the whole platform rests on.

## Out of scope (this iteration)

AI / bots, lobby / 3-4 player, user accounts / persistent auth, variant-authoring
UI, ratings, spectators, and draw detection beyond stalemate. The virusgame
blueprint's `ai.js`, bot-hoster, lobby, and neutral-cell mechanics are
intentionally **not** copied.
