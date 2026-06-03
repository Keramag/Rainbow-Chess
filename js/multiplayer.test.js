// Tests for the DOM-free MultiplayerClient dispatch + state. Run with `node --test`.
import { test } from 'node:test';
import assert from 'node:assert/strict';

import { MultiplayerClient, MESSAGE_TYPES, isGameOver } from './multiplayer.js';

// recordingClient returns a client plus a `seen` map capturing every emitted
// message keyed by type, so a test can both assert routing and inspect payloads.
function recordingClient() {
  const seen = {};
  const client = new MultiplayerClient();
  for (const type of MESSAGE_TYPES) {
    client.on(type, (msg) => {
      seen[type] = msg;
    });
  }
  return { client, seen };
}

test('welcome populates identity and variant list and routes to its handler', () => {
  const { client, seen } = recordingClient();
  const msg = {
    type: 'welcome',
    userId: 'u1',
    username: 'BraveTiger42',
    variants: ['standard', 'rainbow'],
  };
  client.handleMessage(msg);

  assert.equal(client.userId, 'u1');
  assert.equal(client.username, 'BraveTiger42');
  assert.deepEqual(client.variants, ['standard', 'rainbow']);
  assert.equal(seen.welcome, msg);
});

test('users_update stores the roster excluding self', () => {
  const { client } = recordingClient();
  client.handleMessage({ type: 'welcome', userId: 'me', username: 'Me', variants: [] });
  client.handleMessage({
    type: 'users_update',
    users: [
      { userId: 'me', username: 'Me', inGame: false },
      { userId: 'a', username: 'Alice', inGame: false },
      { userId: 'b', username: 'Bob', inGame: true },
    ],
  });

  assert.deepEqual(
    client.onlineUsers.map((u) => u.userId),
    ['a', 'b'],
  );
});

test('game_start records the current game', () => {
  const { client, seen } = recordingClient();
  const msg = {
    type: 'game_start',
    gameId: 'g1',
    variant: 'rainbow',
    color: 'black',
    fen: 'startpos',
    legalMoves: [],
  };
  client.handleMessage(msg);

  assert.equal(client.gameId, 'g1');
  assert.equal(client.color, 'black');
  assert.equal(client.variant, 'rainbow');
  assert.equal(seen.game_start, msg);
});

test('game_update mid-game keeps the game; terminal result clears it', () => {
  const { client, seen } = recordingClient();
  client.handleMessage({ type: 'game_start', gameId: 'g1', variant: 'standard', color: 'white' });

  client.handleMessage({
    type: 'game_update',
    gameId: 'g1',
    result: { outcome: 'ongoing' },
  });
  assert.equal(client.gameId, 'g1', 'ongoing update should not clear the game');

  const over = {
    type: 'game_update',
    gameId: 'g1',
    result: { outcome: 'white_wins', reason: 'checkmate' },
  };
  client.handleMessage(over);
  assert.equal(client.gameId, null, 'terminal result should clear the game');
  assert.equal(client.color, null);
  assert.equal(client.variant, null);
  assert.equal(seen.game_update, over);
});

test('opponent_disconnected clears the current game', () => {
  const { client, seen } = recordingClient();
  client.handleMessage({ type: 'game_start', gameId: 'g1', variant: 'standard', color: 'white' });
  const msg = { type: 'opponent_disconnected', gameId: 'g1', result: { outcome: 'white_wins' } };
  client.handleMessage(msg);

  assert.equal(client.gameId, null);
  assert.equal(seen.opponent_disconnected, msg);
});

test('clearGame drops the current game context (used when our own socket drops)', () => {
  const client = new MultiplayerClient();
  client.handleMessage({ type: 'game_start', gameId: 'g1', variant: 'standard', color: 'white' });
  assert.equal(client.gameId, 'g1');

  client.clearGame();
  assert.equal(client.gameId, null, 'gameId cleared so stale move/resign sends are suppressed');
  assert.equal(client.color, null);
  assert.equal(client.variant, null);
});

test('challenge / error messages route without mutating game state', () => {
  const { client, seen } = recordingClient();
  const received = { type: 'challenge_received', challengeId: 'c1', fromUsername: 'Alice', variant: 'rainbow' };
  const declined = { type: 'challenge_declined', challengeId: 'c1' };
  const expired = { type: 'challenge_expired', challengeId: 'c1' };
  const err = { type: 'error', message: 'Illegal move' };

  for (const m of [received, declined, expired, err]) client.handleMessage(m);

  assert.equal(seen.challenge_received, received);
  assert.equal(seen.challenge_declined, declined);
  assert.equal(seen.challenge_expired, expired);
  assert.equal(seen.error, err);
  assert.equal(client.gameId, null);
});

test('every routed message type reaches its handler', () => {
  const { client, seen } = recordingClient();
  for (const type of MESSAGE_TYPES) client.handleMessage({ type });
  for (const type of MESSAGE_TYPES) {
    assert.ok(type in seen, `handler for ${type} was not invoked`);
  }
});

test('unknown and malformed messages are ignored, not thrown', () => {
  const { client } = recordingClient();
  let fired = false;
  client.on('mystery', () => {
    fired = true;
  });
  client.handleMessage({ type: 'mystery' }); // routed but no state change
  assert.equal(fired, true);

  // No type / non-object: silently ignored.
  assert.doesNotThrow(() => client.handleMessage({}));
  assert.doesNotThrow(() => client.handleMessage(null));
  assert.doesNotThrow(() => client.handleMessage({ type: 42 }));
});

test('receive parses newline-separated frames and skips bad lines', () => {
  const { client, seen } = recordingClient();
  const originalError = console.error;
  console.error = () => {}; // the bad line is expected to log; keep test output clean
  try {
    client.receive(
      '{"type":"welcome","userId":"u1","username":"X","variants":["standard"]}\n' +
        'not-json\n' +
        '{"type":"error","message":"boom"}',
    );
  } finally {
    console.error = originalError;
  }
  assert.equal(client.userId, 'u1');
  assert.equal(seen.error.message, 'boom');
});

test('convenience senders emit the correct wire envelopes', () => {
  const client = new MultiplayerClient();
  const sent = [];
  client.send = (m) => sent.push(m); // stub transport

  client.challenge('target', 'rainbow');
  client.acceptChallenge('c1');
  client.declineChallenge('c2');
  client.move('g1', 'e2', 'e4');
  client.move('g1', 'e7', 'e8', 'n');
  client.resign('g1');

  assert.deepEqual(sent, [
    { type: 'challenge', targetUserId: 'target', variant: 'rainbow' },
    { type: 'accept_challenge', challengeId: 'c1' },
    { type: 'decline_challenge', challengeId: 'c2' },
    { type: 'move', gameId: 'g1', move: { from: 'e2', to: 'e4' } },
    { type: 'move', gameId: 'g1', move: { from: 'e7', to: 'e8', promotion: 'n' } },
    { type: 'resign', gameId: 'g1' },
  ]);
});

test('isGameOver distinguishes terminal results from ongoing', () => {
  assert.equal(isGameOver(undefined), false);
  assert.equal(isGameOver(null), false);
  assert.equal(isGameOver({ outcome: 'ongoing' }), false);
  assert.equal(isGameOver({ outcome: '' }), false);
  assert.equal(isGameOver({ outcome: 'draw', reason: 'stalemate' }), true);
  assert.equal(isGameOver({ outcome: 'black_wins', reason: 'resignation' }), true);
});
