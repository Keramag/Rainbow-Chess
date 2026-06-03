# Rainbow Chess Foundation — Variant-Ready Chess Platform

## Overview
- Build the foundation for a chess-like board-game platform where two humans play 1-on-1 over WebSockets, modeled on the architecture of the `../virusgame` project but with chess rules instead of the virus game.
- **Core differentiator:** the rules engine is built around a pluggable **Variant** abstraction from day one. Standard chess is the base implementation; named variants embed it and override only the rules they change. This is the platform on which future chess-rule experiments will be built.
- This first iteration ships two real, distinct variants end-to-end to prove the abstraction:
  - **Standard** — full legal chess (all moves, castling, en passant, promotion to Q/R/B/N, check/checkmate/stalemate).
  - **Rainbow** — standard piece types in standard squares, but colors assigned by board symmetry (`mirror(x)=7-x`, mirrored squares opposite color, structured-random per game), promotion restricted to Knight/Bishop (per `Prd.md`).
- **Multiplayer model (copied from virusgame):** anonymous identity (random usernames), online-users list, direct **1v1 challenge → accept** flow. **No lobby, no AI, no bots.**
- **Frontend:** vanilla JS (ES modules, no build step), served directly by the Go server, reusing virusgame's WebSocket-client and styling patterns.
- **Deploy:** multi-stage Dockerfile → GHCR → GitHub Actions → Portainer webhook, with Traefik labels (WebSocket upgrade), copied/adapted from virusgame.

### Out of scope (this iteration)
- AI / bot opponents (the `ai.js`, `backend/cmd/bot-hoster/`, `add_bot`/`bot_wanted` parts of virusgame are explicitly NOT copied).
- Lobby / 3-4 player multiplayer (virusgame's lobby system is dropped; only 1v1 challenge).
- User accounts / persistent auth (anonymous random usernames, as in virusgame).
- Variant authoring UI, ratings/ELO, spectators, draw-by-repetition / 50-move / insufficient-material draw detection (foundation handles checkmate + stalemate; other draw rules can be a later iteration).

## Context (from discovery)

### Source blueprint: `../virusgame` (`/Users/iv/Projects/virusgame`)
- **Backend (Go 1.24):** `backend/main.go` (entry + static serving + `/ws`), `client.go` (per-connection read/write pumps), `hub.go` (single-goroutine message router — the heart), `types.go` (structs + WS `Message`), `storage.go` (SQLite via `modernc.org/sqlite` + PGN export), `names.go` (random usernames). Deps: `gorilla/websocket`, `google/uuid`, `modernc.org/sqlite`.
- **1v1 challenge flow to copy:** `hub.go:532-687` — `handleChallenge` (creates `Challenge`, 30s expiry, sends `challenge_received`) → `handleAcceptChallenge` (creates `Game`, marks players in-game, broadcasts `game_start`). Online users tracked in `hub.users`; `users_update` broadcast.
- **Parts to DROP:** `ai.js`, `backend/cmd/bot-hoster/`, lobby handlers (`hub.go:1659-2150`), neutral-cell / connectivity-BFS mechanics, all bot message types.
- **Frontend:** root-level `index.html`, `style.css`, `multiplayer.js` (`MultiplayerClient` WS class: `connect`/`send`/`handleMessage`-dispatch), served by Go (no build step). Reuse the WS client + challenge UI + styling.
- **Deploy:** `Dockerfile` (multi-stage golang-alpine → alpine), `docker-compose.yml` (Traefik labels incl. WS upgrade headers, SQLite volume), `.github/workflows/deploy.yml` (test → build → push GHCR → Portainer webhook).

### Target spec: `Prd.md` (Rainbow-Chess)
- 8×8 standard board & standard starting squares; symmetric color assignment (`mirror(x)=7-x`, mirrored squares opposite color, "structured-random via symmetry"); full legal chess + check enforcement; promotion limited to Knight/Bishop; checkmate = win, stalemate = draw; engine must validate symmetry constraint + both kings present.

### Patterns / decisions
- New Go engine package `backend/engine/` (no existing chess code to reuse — built fresh, fully unit-tested).
- Wire format for positions: **FEN** (the engine produces/parses it; FEN encodes per-square piece+color, so it serializes Rainbow positions too). Server is authoritative and sends FEN + side-to-move + legal-move list + result on every update so the client never duplicates rule logic.
- Frontend pure logic in ES modules so it is importable by Node's built-in test runner (`node --test`) with zero extra deps; DOM glue kept thin.

## Development Approach
- **Testing approach**: Regular (code first, then tests within the SAME task — tests are a required deliverable of every task, never deferred).
- Complete each task fully before moving to the next.
- Make small, focused changes; copy/adapt from `../virusgame` where noted rather than inventing.
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task (success + error/edge scenarios), listed as separate checklist items.
- **CRITICAL: all tests must pass before starting the next task** — no exceptions.
- **CRITICAL: update this plan file when scope changes during implementation.**
- Backend tests: `go test ./...` (run from `backend/`). Frontend tests: `node --test` over ES-module logic files.
- Maintain a clean separation between **rules (engine)**, **transport (hub/WS)**, and **rendering (frontend)** — this separation is what makes future variants cheap.

## Testing Strategy
- **Unit tests**: required for every task.
  - Engine: table-driven tests using FEN positions; include `perft` (move-count) checks on known positions as a strong correctness signal, plus explicit checkmate/stalemate/promotion/castling/en-passant cases, and Rainbow symmetry-invariant tests across many seeds.
  - Server: hub connect/disconnect, challenge lifecycle, move validation/turn enforcement, game-over + persistence.
  - Frontend: pure helpers (FEN→board model, square↔coordinate mapping, legal-move highlight derivation, variant-list parsing) via `node --test`.
- **E2E tests**: no heavy framework is introduced this iteration (virusgame has none). Manual browser smoke-test scenarios (two tabs) are documented in Post-Completion. If a richer e2e harness is wanted, it is a later iteration.

## Progress Tracking
- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Keep this plan in sync with actual work done.

## What Goes Where
- **Implementation Steps** (`[ ]` checkboxes): code, tests, and docs achievable inside this repo and automatable by the agent.
- **Post-Completion** (no checkboxes): manual browser testing, real deployment verification, and GHCR/Portainer/DNS secrets that require external action.

## Implementation Steps

### Task 1: Engine core types, board, and FEN
- [x] create `backend/engine/types.go`: `Color` (White/Black), `PieceType` (None, Pawn, Knight, Bishop, Rook, Queen, King), `Piece`, `Square` (file 0-7, rank 0-7), `Move` (From, To, Promotion, plus IsCastle/IsEnPassant/IsDoublePush flags), `GameResult` (Ongoing/WhiteWins/BlackWins/Draw with reason)
- [x] create `backend/engine/board.go`: `Position` struct (8×8 piece array, side to move, castling rights, en-passant target, halfmove clock, fullmove number), get/set/clone helpers, algebraic↔square conversion, and the `mirror(x)=7-x` helper (FEN parse/generate split into `backend/engine/fen.go` for clarity)
- [x] implement FEN parse (`ParseFEN`) and FEN generate (`(*Position).FEN()`)
- [x] write tests for square/algebraic conversion and `mirror`
- [x] write tests for FEN round-trip on several positions (start position, mid-game, en-passant, castling-rights variants) + error cases (malformed FEN)
- [x] run `go test ./...` - must pass before next task

### Task 2: Square-attack detection
- [x] create `backend/engine/attacks.go`: `IsSquareAttacked(pos, sq, byColor)` covering pawn attacks (color-directional), knight, king, and sliding (bishop/rook/queen) attacks with proper blocking
- [x] add `KingSquare(pos, color)` and `IsInCheck(pos, color)` helpers built on attack detection
- [x] write tests for attack detection from FEN positions (each piece type, blocked vs. open lines, pawn-attack direction per color)
- [x] write tests for `IsInCheck` (in-check and not-in-check positions, discovered lines)
- [x] run `go test ./...` - must pass before next task

### Task 3: Pseudo-legal move generation
- [x] create `backend/engine/movegen.go`: `PseudoLegalMoves(pos)` per piece — pawn (single push, double push from start rank, diagonal captures, en passant, promotion expansion), knight, bishop/rook/queen (sliding), king (incl. castling king/queen-side with empty-square + not-through/into-check + rights checks)
- [x] ensure pawn direction and start/promotion ranks are derived from piece **color** (not board half) so the generator is variant-agnostic
- [x] write per-piece move-generation tests from FEN positions (success cases incl. promotion-to-4, en passant, both castles available/blocked)
- [x] write edge-case tests (pawn blocked, no double-push when intermediate occupied, castling unavailable when rights lost / path attacked)
- [x] run `go test ./...` - must pass before next task

### Task 4: Legal moves, ApplyMove, and game result
- [x] add `LegalMoves(pos)` = pseudo-legal filtered so the mover's king is not left in check (apply-and-test)
- [x] implement `ApplyMove(pos, move)` returning a new `Position`: move piece, handle capture/en-passant/castling rook move/promotion, update castling rights, en-passant target, halfmove & fullmove counters, flip side to move; return error on illegal move
- [x] implement `Result(pos)`: no legal moves + in check → checkmate (loser = side to move); no legal moves + not in check → stalemate (draw); else ongoing
- [x] write tests for legal-move filtering (pinned pieces, must-block/capture/move-king out of check)
- [x] write tests for `ApplyMove` (capture, en-passant capture removes correct pawn, castling moves rook, promotion places chosen piece, rights/clocks updated) + illegal-move error
- [x] write checkmate + stalemate tests (e.g., fool's mate, a known stalemate FEN) and `perft(1..3)` count checks on at least the start position and one tactical position
- [x] run `go test ./...` - must pass before next task

### Task 5: Variant interface, registry, and Standard variant
- [x] create `backend/engine/variant.go`: `Variant` interface (`Name() string`, `InitialPosition() *Position`, `LegalMoves(*Position) []Move`, `ApplyMove(*Position, Move) (*Position, error)`, `Result(*Position) GameResult`, `PromotionPieces() []PieceType`) and a registry (`Register`, `Get(name)`, `List()`)
- [x] create `backend/engine/standard.go`: `Standard` struct implementing the interface — standard initial position, `LegalMoves`/`ApplyMove`/`Result` delegating to the engine, `PromotionPieces()` = {Queen, Rook, Bishop, Knight}; register as `"standard"`
- [x] ensure `ApplyMove` rejects promotions to a piece not in the variant's `PromotionPieces()`
- [x] write tests for the registry (register/get/list, unknown-name error)
- [x] write tests for the Standard variant (correct initial position/FEN, a short legal game sequence, promotion to all four allowed)
- [x] run `go test ./...` - must pass before next task

### Task 6: Rainbow variant
- [x] create `backend/engine/rainbow.go`: `Rainbow` struct embedding `Standard`, registered as `"rainbow"`
- [x] override `InitialPosition()`: place standard piece types on standard squares, then assign colors structured-randomly subject to the symmetry constraint (for every occupied square `(x,y)`, `(7-x,y)` is opposite color); accept an injectable RNG/seed so tests are deterministic; document pawn-direction decision (by color — white toward higher rank) in a code comment
- [x] override `PromotionPieces()` = {Knight, Bishop} only
- [x] add a `validate()` that asserts the symmetry constraint holds and both kings exist; call it on init
- [x] write tests: symmetry invariant holds across many seeds, both kings present, promotion list = {Knight, Bishop} and `ApplyMove` rejects Q/R promotion, initial position passes `validate()`
- [x] run `go test ./...` - must pass before next task

### Task 7: WebSocket hub, client, and anonymous identity
- [ ] create `backend/main.go`, `backend/client.go`, `backend/names.go` adapted from virusgame (read/write pumps, ping, random `Adjective+Animal+NN` usernames); serve static frontend + `GET /ws`; no-cache middleware for JS/CSS
- [ ] create `backend/hub.go` (single goroutine: `register`/`unregister`/`handleMessage` channels; `users` map) and `backend/types.go` (`User`, chess `Game`, WS `Message`) — **stripped of all lobby/bot/neutral logic**
- [ ] implement connect (assign username, send `welcome` including the variant list from `engine.List()`) and disconnect cleanup; broadcast `users_update` (online users available to challenge)
- [ ] initialize `backend/go.mod` with `gorilla/websocket`, `google/uuid`, `modernc.org/sqlite`
- [ ] write tests for hub connect/disconnect, username assignment, and online-users list updates
- [ ] write tests for the `welcome` payload containing both registered variants
- [ ] run `go test ./...` - must pass before next task

### Task 8: 1v1 challenge → accept flow
- [ ] implement `challenge` (target user + chosen `variant`), `challenge_received`, `accept_challenge`, `decline_challenge`, and 30s expiry ticker — adapted from virusgame `hub.go:532-687`, with `variant` added
- [ ] on accept: validate variant exists, create chess `Game` via `engine.Get(variant).InitialPosition()`, assign colors (challenger = White, acceptor = Black), mark both users in-game, broadcast `game_start` with `{gameId, variant, color, fen, legalMoves}`
- [ ] reject challenges to busy/offline users and self-challenges
- [ ] write tests for the full challenge lifecycle (create → received → accept creates game with correct variant & initial FEN)
- [ ] write tests for expiry, decline, and invalid challenges (offline/busy/self/unknown-variant)
- [ ] run `go test ./...` - must pass before next task

### Task 9: In-game move protocol
- [ ] implement `move` handler: look up the game's variant, validate the incoming `{from,to,promotion}` against `LegalMoves`, `ApplyMove`, then broadcast `game_update` = `{fen, sideToMove, legalMoves, lastMove, result}` to both players
- [ ] enforce turn ownership (reject moves from the player not on turn / not in the game) and surface illegal moves as an `error` message to the sender only
- [ ] implement `resign` and a per-turn move timer (auto-resign), then end the game and trigger persistence (Task 10 hook) on checkmate/stalemate/resign/timeout/disconnect
- [ ] write tests for legal move application + broadcast payload, and illegal/out-of-turn move rejection
- [ ] write tests for game-ending paths (checkmate sets correct winner, stalemate = draw, resign, disconnect)
- [ ] run `go test ./...` - must pass before next task

### Task 10: SQLite game persistence
- [ ] create `backend/storage.go` adapted from virusgame: `games` table (`id, started_at, ended_at, variant, white_name, black_name, result, termination, moves`) where `moves` is the move list (UCI/SAN or FEN history) sufficient to review the game
- [ ] async save on game end (non-blocking goroutine), DB at `backend/data/games.db`
- [ ] wire the Task 9 game-end hook to `SaveGame`
- [ ] write tests for schema init, `SaveGame`, and reading a saved game back (incl. correct variant + result recorded)
- [ ] run `go test ./...` - must pass before next task

### Task 11: Frontend shell, WebSocket client, and challenge UI
- [ ] create `index.html` + `style.css` adapted from virusgame (header with own username, online-users panel with Challenge buttons + variant picker, incoming-challenge accept/decline prompt, game area placeholder)
- [ ] create `js/multiplayer.js` (ES module): `MultiplayerClient` adapted from virusgame — `connect`, `send`, and `handleMessage` dispatch for `welcome`/`users_update`/`challenge_received`/`game_start`/`game_update`/`error`
- [ ] create `js/variants.js` (ES module): parse the variant list from `welcome` and populate the challenge variant picker
- [ ] wire the static files to be served by the Go server (Task 7) and loaded via `<script type="module">`
- [ ] write `node --test` tests for the variant-list parser and the message-dispatch routing (pure logic, DOM-free)
- [ ] run `node --test` (frontend) and `go test ./...` - must pass before next task

### Task 12: Chess board rendering and click-to-move
- [ ] create `js/board-model.js` (ES module, pure): FEN→8×8 model, square↔pixel/coordinate mapping, and derivation of highlight targets from the server-provided legal-move list
- [ ] create `js/chess.js` (ES module): render the board from `fen` with color-correct Unicode/SVG pieces, click source→target (with a promotion picker limited to the variant's allowed pieces), send `move`, re-render on `game_update`, and show turn / check / game-over banners
- [ ] handle Rainbow correctly: render each piece by its own color (positions are color-mixed), no assumption that bottom = white
- [ ] write `node --test` tests for `board-model.js` (FEN→model for start + a Rainbow-style mixed position, coordinate mapping, highlight derivation)
- [ ] run `node --test` (frontend) and `go test ./...` - must pass before next task

### Task 13: End-to-end game wiring + variant selection
- [ ] connect challenge picker → `game_start` → board render → moves → `game_update` → game-over, so a full game is playable between two browser tabs for BOTH variants
- [ ] display the active variant name and both players' usernames/colors during the game; offer "new game / back to lobby-less menu" on game end
- [ ] handle disconnect/opponent-left and challenge-expiry/decline gracefully in the UI
- [ ] write `node --test` tests for the game-state reducer (applying `game_start`/`game_update`/game-over to local UI state)
- [ ] run `node --test` (frontend) and `go test ./...` - must pass before next task

### Task 14: Dockerfile + docker-compose
- [ ] create multi-stage `Dockerfile` adapted from virusgame (build Go server in golang-alpine, copy binary + static frontend into alpine, inject `COMMIT_SHA`, `EXPOSE 8080`) — **no bot-hoster stage**
- [ ] create `docker-compose.yml` adapted from virusgame: Traefik labels (host rule, websecure, certresolver, service port 8080, WS upgrade headers), SQLite volume (`./backend/data:/app/backend/data`), GHCR image with `${GIT_SHA}`, `restart: unless-stopped`
- [ ] add `.env.example` documenting `HOSTNAME`, `NETWORK_NAME`, `GIT_SHA`, and the GHCR image owner/repo
- [ ] verify `docker build .` succeeds and the container serves the app on `:8080` (build + smoke as the automatable check)
- [ ] run `go test ./...` - must pass before next task

### Task 15: GitHub Actions CI/CD
- [ ] create `.github/workflows/deploy.yml` adapted from virusgame: job 1 `go test ./...` (+ `node --test` for frontend), job 2 build & push image to `ghcr.io/<owner>/rainbow-chess:${SHA}`, job 3 trigger Portainer redeploy webhook
- [ ] update the image tag in `docker-compose.yml` from CI (sed/commit pattern as in virusgame), gated on tests passing
- [ ] validate the workflow YAML parses and references the correct test/build commands and secrets (`PORTAINER_REDEPLOY_HOOK`)
- [ ] write/adjust a tiny CI smoke step or `Makefile` target (`make test`) wrapping `go test ./...` + `node --test` so CI and local match
- [ ] run `go test ./...` and `node --test` - must pass before next task

### Task 16: Verify acceptance criteria
- [ ] verify all Overview requirements are implemented: two registered variants (standard + rainbow), 1v1 challenge w/o lobby, no AI/bot code present, full legal chess for standard, Rainbow symmetry + N/B promotion
- [ ] verify edge cases: checkmate/stalemate end games, illegal/out-of-turn moves rejected, challenge expiry/decline, disconnect handling
- [ ] run the full backend suite `go test ./...` and frontend `node --test`
- [ ] run `go vet ./...` (and `gofmt -l` / any configured linter) — all issues fixed
- [ ] confirm test coverage of the `engine` package is solid (engine is correctness-critical; aim 80%+)

### Task 17: Documentation
- [ ] write `README.md`: project overview, how to run locally (`go run` + open browser), how to play a 1v1 game, and **how to add a new Variant** (implement the interface / embed Standard + override + `Register`)
- [ ] create/initialize `CLAUDE.md` capturing the engine/hub/frontend separation, the FEN wire-format decision, and the variant-extension pattern for future rule experiments
- [ ] note documented rule decisions (Rainbow pawn direction by color; castling/en-passant inherited by Rainbow; draw detection limited to stalemate this iteration)

## Technical Details

### Engine (`backend/engine/`)
- `Position` is immutable-by-convention: `ApplyMove` returns a new `*Position` (simplifies legality testing and history).
- Move legality = pseudo-legal generation + "does my king end up attacked?" filter. Attack detection drives check, checkmate, castling-through-check, and pins.
- Color-relative pawn logic (start rank, push direction, promotion rank derived from `Color`) is what lets the same generator serve both variants without special-casing Rainbow.
- `perft` tests are the strongest guard against move-gen regressions; include them for the start position and at least one castling/en-passant/promotion-rich position.

### Variant abstraction
```
type Variant interface {
    Name() string
    InitialPosition() *Position
    LegalMoves(*Position) []Move
    ApplyMove(*Position, Move) (*Position, error)
    Result(*Position) GameResult
    PromotionPieces() []PieceType
}
// Standard implements all of it via the engine.
// Rainbow embeds Standard; overrides InitialPosition (symmetric color
// assignment) and PromotionPieces ({Knight, Bishop}); everything else inherited.
```
- Registry: `engine.Register(name, Variant)` at package init; `engine.List()` feeds the frontend variant picker; `engine.Get(name)` used at game creation.

### WebSocket protocol (server-authoritative)
- Client → server: `challenge {targetUserId, variant}`, `accept_challenge {challengeId}`, `decline_challenge`, `move {gameId, from, to, promotion?}`, `resign {gameId}`.
- Server → client: `welcome {userId, username, variants[]}`, `users_update {users[]}`, `challenge_received {challengeId, from, variant}`, `challenge_expired`, `game_start {gameId, variant, color, fen, legalMoves}`, `game_update {gameId, fen, sideToMove, legalMoves, lastMove, result}`, `error {message}`.
- Positions always travel as **FEN**; legal moves for the side to move are always included so the client highlights without re-implementing rules.

### Identity / sessions
- Anonymous: random username assigned on WS connect (virusgame `names.go`); no accounts, no persistence across reconnect (foundation).

## Post-Completion
*Items requiring manual intervention or external systems — no checkboxes, informational only.*

**Manual verification:**
- Two-tab browser smoke test: connect both, challenge with Standard → play to checkmate; repeat with Rainbow → confirm color-mixed setup, that the symmetry looks right, and that promotion offers only Knight/Bishop.
- Verify WebSocket upgrade works behind Traefik in the deployed environment (the upgrade-header middleware).
- Confirm SQLite volume persists games across container restarts.

**External system updates:**
- Create the GitHub repo + GHCR package; set image owner/repo in `docker-compose.yml` and the workflow.
- Configure GitHub Actions secrets: `PORTAINER_REDEPLOY_HOOK` (and confirm `GITHUB_TOKEN` package write perms for GHCR).
- Create the Portainer stack pointing at `docker-compose.yml`; set `HOSTNAME` (DNS A record) and the external Traefik `NETWORK_NAME`.
- First deploy: push to the default branch, confirm CI builds/pushes the image and the Portainer webhook redeploys.
