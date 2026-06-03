// multiplayer.js — WebSocket client for Rainbow Chess.
//
// Adapted from virusgame's MultiplayerClient, but deliberately split so the
// rules-agnostic transport layer stays DOM-free and unit-testable: handleMessage
// only updates plain client state and re-emits the message to a registered
// handler. All DOM/rendering glue lives in app.js (and later chess.js), which
// registers handlers via on(). The node --test suite drives handleMessage with
// fake messages and asserts both the state updates and the dispatch routing.
//
// The server is authoritative: it sends FEN + side-to-move + the legal-move list
// + result on every game_update, so this client never re-implements chess rules —
// it just relays the player's intent (challenge / accept / move / resign) and
// forwards server state to the UI.

// MESSAGE_TYPES is the set of server -> client message types this client routes.
// Exported so tests (and app.js) can assert full coverage without hard-coding the
// list in two places.
export const MESSAGE_TYPES = [
  'welcome',
  'users_update',
  'challenge_received',
  'challenge_declined',
  'challenge_expired',
  'game_start',
  'game_update',
  'opponent_disconnected',
  'error',
];

export class MultiplayerClient {
  // handlers maps a message type (or the lifecycle events 'open'/'close') to a
  // callback invoked with the message after internal state is updated. Pass an
  // object up front or register later with on().
  constructor(handlers = {}) {
    this.ws = null;
    this.connected = false;
    this.reconnect = true;

    // Identity, learned from `welcome`.
    this.userId = null;
    this.username = null;
    this.variants = [];

    // Online roster (excludes self), learned from `users_update`.
    this.onlineUsers = [];

    // Current game, set by `game_start` and cleared when it ends.
    this.gameId = null;
    this.color = null; // "white" | "black"
    this.variant = null;

    this.handlers = { ...handlers };
  }

  // on registers (or replaces) the handler for a message type / lifecycle event.
  // Returns this for chaining.
  on(type, fn) {
    this.handlers[type] = fn;
    return this;
  }

  // emit invokes the registered handler for type, if any. Internal.
  emit(type, msg) {
    const fn = this.handlers[type];
    if (typeof fn === 'function') fn(msg);
  }

  // connect opens the WebSocket to the same host that served the page, choosing
  // ws/wss to match http/https. Browser-only (relies on window/WebSocket); the
  // test suite never calls it.
  connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this.connected = true;
      this.emit('open', null);
    };
    this.ws.onmessage = (event) => this.receive(event.data);
    this.ws.onclose = () => {
      this.connected = false;
      this.emit('close', null);
      if (this.reconnect) setTimeout(() => this.connect(), 3000);
    };
    this.ws.onerror = (err) => {
      console.error('WebSocket error:', err);
    };
  }

  // disconnect closes the socket and suppresses the auto-reconnect.
  disconnect() {
    this.reconnect = false;
    if (this.ws) this.ws.close();
  }

  // clearGame drops the current game context. The server ends any game the
  // moment a player's socket drops and supports no rejoin (a reconnect comes
  // back as a brand-new anonymous user), so when our own connection is lost the
  // in-flight game is gone. Clearing it here also neutralises the move/resign
  // senders in app.js, which are guarded on a live gameId.
  clearGame() {
    this.gameId = null;
    this.color = null;
    this.variant = null;
  }

  // receive parses a raw frame (possibly several newline-separated JSON messages,
  // as the Go server may coalesce) and dispatches each. Malformed lines are
  // logged and skipped rather than throwing.
  receive(data) {
    for (const line of String(data).trim().split('\n')) {
      const s = line.trim();
      if (!s) continue;
      let msg;
      try {
        msg = JSON.parse(s);
      } catch (err) {
        console.error('Error parsing message:', err, 'Data:', s);
        continue;
      }
      this.handleMessage(msg);
    }
  }

  // send JSON-encodes and transmits a message if the socket is open.
  send(message) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(message));
    }
  }

  // handleMessage updates client state for the message and re-emits it to the
  // registered handler. This is the single dispatch point and the core of the
  // tested surface — it must stay DOM-free.
  handleMessage(msg) {
    if (!msg || typeof msg.type !== 'string') return;

    switch (msg.type) {
      case 'welcome':
        this.userId = msg.userId || null;
        this.username = msg.username || null;
        this.variants = Array.isArray(msg.variants) ? msg.variants : [];
        break;

      case 'users_update':
        // The roster the UI renders Challenge buttons for never includes self.
        this.onlineUsers = (Array.isArray(msg.users) ? msg.users : []).filter(
          (u) => u && u.userId !== this.userId,
        );
        break;

      case 'game_start':
        this.gameId = msg.gameId || null;
        this.color = msg.color || null;
        this.variant = msg.variant || null;
        break;

      case 'game_update':
        // Clear the current game once the server reports a terminal result.
        if (isGameOver(msg.result)) {
          this.gameId = null;
          this.color = null;
          this.variant = null;
        }
        break;

      case 'opponent_disconnected':
        this.gameId = null;
        this.color = null;
        this.variant = null;
        break;

      // challenge_received / challenge_declined / challenge_expired / error carry
      // no client-state changes — they are purely UI-facing and just routed on.
      default:
        break;
    }

    this.emit(msg.type, msg);
  }

  // --- Convenience senders (client -> server) -----------------------------

  challenge(targetUserId, variant) {
    this.send({ type: 'challenge', targetUserId, variant });
  }

  acceptChallenge(challengeId) {
    this.send({ type: 'accept_challenge', challengeId });
  }

  declineChallenge(challengeId) {
    this.send({ type: 'decline_challenge', challengeId });
  }

  // move sends a move as algebraic squares plus an optional promotion piece
  // letter ("q"/"r"/"b"/"n"), matching the server's MoveDTO.
  move(gameId, from, to, promotion) {
    const move = { from, to };
    if (promotion) move.promotion = promotion;
    this.send({ type: 'move', gameId, move });
  }

  resign(gameId) {
    this.send({ type: 'resign', gameId });
  }
}

// isGameOver reports whether a ResultDTO marks the game finished. A missing
// result, or one whose outcome is "ongoing"/empty, means play continues.
export function isGameOver(result) {
  return Boolean(result && result.outcome && result.outcome !== 'ongoing');
}
