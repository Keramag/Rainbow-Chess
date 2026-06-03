# Rainbow Chess

A variant-ready chess platform where two anonymous players play 1-on-1 over
WebSockets. Standard chess is the base rule set; named **variants** embed it and
override only the rules they change. The first iteration ships two real variants
end-to-end to prove the abstraction:

- **Standard** — full legal chess: all moves, castling, en passant, promotion to
  Q/R/B/N, check / checkmate / stalemate.
- **Rainbow** — standard piece types on standard squares, but colours are
  assigned by board symmetry (`mirror(x) = 7 - x`, mirrored squares are opposite
  colours, structured-random per game), and promotion is restricted to
  **Knight / Bishop** (see `Prd.md`).

The server is authoritative: it runs the rules engine and sends every client the
position as **FEN**, the side to move, the legal-move list, and the result on
each update. The vanilla-JS frontend only renders and relays clicks — it never
re-implements chess logic.

## Architecture at a glance

```
backend/engine/   Pure chess rules. No I/O. The Variant abstraction lives here.
backend/          WebSocket transport: hub (single goroutine), client pumps,
                  anonymous identity, challenge flow, move protocol, SQLite save.
js/               Frontend ES modules (no build step), served by the Go server.
index.html        Page shell + style.css.
```

The three layers — **rules (engine)**, **transport (hub/WS)**, and **rendering
(frontend)** — are kept strictly separate. That separation is what makes adding a
new variant cheap. See `CLAUDE.md` for the deeper contributor-facing tour.

## Run it locally

Requirements: Go 1.24+ and Node 22+ (Node only for the frontend tests).

```bash
# from the repo root
cd backend
go run .
```

The server starts on **http://localhost:8080** and serves the static frontend
from the repo root (one level up from `backend/`). Open the URL in your browser.

> The SQLite game database is created at `backend/data/games.db` on first run.

## Play a 1v1 game

There is **no lobby, no AI, and no accounts** — just anonymous 1v1 challenges.

1. Open **http://localhost:8080** in two browser tabs (or two machines on the
   same host). Each tab is assigned a random username on connect.
2. Each tab sees the other in the **online users** panel.
3. In one tab, pick a variant (**Standard** or **Rainbow**) from the picker next
   to a user and click **Challenge**.
4. The other tab gets an accept/decline prompt. On **Accept**, a game starts: the
   challenger plays **White**, the acceptor plays **Black**.
5. Click a piece to see its legal moves highlighted, then click a target to move.
   Pawn promotions show a picker limited to the variant's allowed pieces
   (Q/R/B/N for Standard, N/B for Rainbow).
6. The game ends on checkmate (win), stalemate (draw), resignation, move-timeout
   (auto-resign), or opponent disconnect. A game-over panel offers a return to
   the menu for a fresh challenge.

A challenge that is not accepted within 30 seconds expires.

## Running the tests

The `Makefile` is the single entry point so local runs and CI match exactly:

```bash
make test            # backend (go test ./...) + frontend (node --test)
make test-backend    # cd backend && go test ./...
make test-frontend   # node --test
```

The engine is correctness-critical and is heavily tested, including `perft`
move-count checks on known positions and Rainbow symmetry-invariant tests across
many seeds.

## How to add a new variant

A variant is anything that implements `engine.Variant` and registers itself.
Because Go embedding inherits `Standard`'s methods, you usually only write the
handful of methods whose rules actually differ.

The `Variant` interface (`backend/engine/variant.go`):

```go
type Variant interface {
    Name() string                                  // registry key
    InitialPosition() *Position                    // fresh starting position
    LegalMoves(pos *Position) []Move               // legal moves for side to move
    ApplyMove(pos *Position, move Move) (*Position, error)
    Result(pos *Position) GameResult               // ongoing / win / draw
    PromotionPieces() []PieceType                  // order a UI offers them
}
```

### The cheapest path: embed `Standard` and override what changes

`Standard` reads its two configurable knobs — the variant **name** and the
**promotion whitelist** — from fields rather than hard-coding them. (Go embedding
promotes methods but does *not* do virtual dispatch, so configuration must live
in state the inherited methods can see.) That means a variant can change its name
and promotion rules just by setting those fields in its constructor, and inherit
`Name`, `PromotionPieces`, the promotion-restricting `ApplyMove`, `LegalMoves`,
and `Result` unchanged.

`Rainbow` (`backend/engine/rainbow.go`) is the worked example — it overrides only
`InitialPosition`:

```go
package engine

type MyVariant struct {
    Standard
}

func NewMyVariant() *MyVariant {
    return &MyVariant{
        // Configure the inherited Standard: a unique name and the promotion set.
        Standard: Standard{
            name:       "myvariant",
            promotions: []PieceType{Queen, Knight}, // whatever your rule allows
        },
    }
}

// Register at package init so List()/Get() see it with no transport changes.
func init() { Register("myvariant", NewMyVariant()) }

// Override only the methods whose rules differ. For example, a custom setup:
func (v *MyVariant) InitialPosition() *Position {
    pos, _ := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
    return pos
}
```

### Steps

1. Create `backend/engine/<name>.go` with a struct embedding `Standard`.
2. Set `name` and `promotions` in the constructor; override any method whose
   rules genuinely differ (movement, setup, result, …).
3. Call `Register("<name>", New...())` from an `init()`.
4. Add table-driven tests in `backend/engine/<name>_test.go`.

That's all. `engine.List()` (which feeds the frontend variant picker) and
`engine.Get(name)` (used to create a game) pick it up automatically — the
transport layer and the frontend need no changes.

## Rule decisions (documented)

- **Rainbow pawn direction is by colour, never by board half.** A white pawn
  always advances toward rank 8 and a black pawn toward rank 1, exactly as the
  engine derives push direction, start rank, and promotion rank from `Color`.
  Because Rainbow scatters colours across both home ranks, a white pawn that ends
  up on rank 7 advances toward rank 8 and can promote almost immediately — the
  intended consequence of colour-relative pawns, and what lets the unchanged
  Standard generator serve Rainbow.
- **Rainbow inherits castling and en passant unchanged** from Standard; only the
  initial colouring and the promotion whitelist differ.
- **Both Rainbow kings start in check.** Symmetric colouring of the dense
  standard layout always leaves both kings attacked on move one — a structural
  property, not a bug. The side to move simply answers the check. Rainbow re-rolls
  the colouring only when White (always the side to move) would have no legal
  reply, so a game never starts already lost.
- **Draw detection is limited to stalemate** this iteration. Draw by repetition,
  the 50-move rule, and insufficient material are out of scope and can be a later
  iteration. Checkmate is a win; stalemate is a draw.

## WebSocket protocol (server-authoritative)

Positions always travel as **FEN**; the legal-move list for the side to move is
always included, so the client highlights and validates nothing on its own.
Squares are algebraic (`"e2"`, `"e4"`); promotions are piece letters
(`"q"`, `"r"`, `"b"`, `"n"`).

Client → server:
`challenge {targetUserId, variant}`,
`accept_challenge {challengeId}`,
`decline_challenge {challengeId}`,
`move {gameId, from, to, promotion?}`,
`resign {gameId}`.

Server → client:
`welcome {userId, username, variants[]}`,
`users_update {users[]}`,
`challenge_received {challengeId, fromUserId, fromUsername, variant}`,
`challenge_declined`, `challenge_expired`,
`game_start {gameId, variant, color, fen, sideToMove, inCheck, legalMoves}`,
`game_update {gameId, fen, sideToMove, inCheck, legalMoves, lastMove, result}`,
`opponent_disconnected`, `error {message}`.

## Deployment

A multi-stage `Dockerfile` builds the Go server and bundles the static frontend
into an alpine image; `docker-compose.yml` carries Traefik labels (including the
WebSocket upgrade headers) and mounts the SQLite volume. CI
(`.github/workflows/deploy.yml`) runs `make test`, builds and pushes the image to
GHCR, and fires a Portainer redeploy webhook. Copy `.env.example` to `.env` and
fill in `HOSTNAME`, `NETWORK_NAME`, and `GIT_SHA` for your host. See `.env.example`
and the compose file for details.

## Project layout

```
backend/
  engine/         chess rules + the Variant abstraction (standard.go, rainbow.go)
  main.go         entry: static serving + /ws + SQLite wiring
  hub.go          single-goroutine message router (register/unregister/messages)
  client.go       per-connection read/write pumps + ping
  types.go        WS Message envelope + User/Challenge/Game
  names.go        random anonymous usernames
  storage.go      SQLite game persistence (modernc.org/sqlite)
js/
  multiplayer.js  MultiplayerClient WS wrapper (pure, DOM-free)
  variants.js     variant-list parsing for the picker
  board-model.js  FEN -> 8x8 model, coordinate mapping, highlight derivation
  chess.js        BoardView: DOM board render + click-to-move
  game-state.js   pure reducer for screen state (menu / playing / over)
  app.js          thin DOM glue wiring it all together
index.html        page shell      style.css   styling
Dockerfile  docker-compose.yml  .env.example  Makefile  .github/workflows/
```
