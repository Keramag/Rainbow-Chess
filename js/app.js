// app.js — thin DOM glue that wires the MultiplayerClient to the page shell.
//
// This is the only module that touches the DOM. It owns no rules logic: it
// registers handlers on the client and reflects server state into the header,
// online-users panel, incoming-challenge prompt, and game-area placeholder. The
// real chess board rendering is layered on in a later task; for now the game
// area shows the variant, colors, and the raw FEN the server sent so a game is
// observably "started" end to end.

import { MultiplayerClient } from './multiplayer.js';
import { populateVariantPicker, variantLabel } from './variants.js';
import { BoardView } from './chess.js';

const $ = (id) => document.getElementById(id);

const els = {
  status: $('connection-status'),
  username: $('own-username'),
  variantPicker: $('variant-picker'),
  usersList: $('users-list'),
  usersEmpty: $('users-empty'),
  challengePrompt: $('challenge-prompt'),
  challengeText: $('challenge-text'),
  acceptBtn: $('accept-challenge'),
  declineBtn: $('decline-challenge'),
  gameArea: $('game-area'),
  boardRoot: $('board-root'),
  resignBtn: $('resign-btn'),
  gameIdle: $('game-idle'),
  toast: $('toast'),
};

const client = new MultiplayerClient();

// The board renderer owns all in-game rendering and click-to-move; it relays a
// completed move back to the server through the client.
const board = new BoardView(els.boardRoot, {
  onMove: (from, to, promotion) => {
    if (client.gameId) client.move(client.gameId, from, to, promotion);
  },
});

// Track the most recent incoming challenge so the prompt's Accept/Decline
// buttons know which one they act on.
let pendingChallenge = null;

// Best-effort opponent name for the in-game banner: remembered when we send a
// challenge (we know the target) or accept one (we know the challenger).
let opponentName = '';

// --- Rendering ------------------------------------------------------------

function setStatus(connected) {
  if (!els.status) return;
  els.status.textContent = connected ? 'Connected' : 'Disconnected';
  els.status.classList.toggle('online', connected);
  els.status.classList.toggle('offline', !connected);
}

function renderUsers() {
  if (!els.usersList) return;
  els.usersList.textContent = '';
  const users = client.onlineUsers;
  if (els.usersEmpty) els.usersEmpty.hidden = users.length > 0;

  for (const user of users) {
    const row = document.createElement('li');
    row.className = 'user-row';

    const name = document.createElement('span');
    name.className = 'user-name';
    name.textContent = user.username;
    row.appendChild(name);

    const btn = document.createElement('button');
    btn.className = 'challenge-btn';
    if (user.inGame) {
      btn.textContent = 'In game';
      btn.disabled = true;
    } else {
      btn.textContent = 'Challenge';
      btn.addEventListener('click', () => {
        const variant = els.variantPicker ? els.variantPicker.value : 'standard';
        opponentName = user.username;
        client.challenge(user.userId, variant);
        toast(`Challenge sent to ${user.username} (${variantLabel(variant)})`);
      });
    }
    row.appendChild(btn);
    els.usersList.appendChild(row);
  }
}

function showChallengePrompt(msg) {
  pendingChallenge = msg;
  if (els.challengeText) {
    els.challengeText.textContent = `${msg.fromUsername} challenges you to ${variantLabel(
      msg.variant,
    )} chess.`;
  }
  if (els.challengePrompt) els.challengePrompt.hidden = false;
}

function hideChallengePrompt() {
  pendingChallenge = null;
  if (els.challengePrompt) els.challengePrompt.hidden = true;
}

function showGame(msg) {
  if (els.gameArea) els.gameArea.hidden = false;
  if (els.gameIdle) els.gameIdle.hidden = true;
  if (els.resignBtn) els.resignBtn.disabled = false;
  board.start(msg, opponentName);
}

function endGame() {
  if (els.gameArea) els.gameArea.hidden = true;
  if (els.gameIdle) els.gameIdle.hidden = false;
}

function updateGame(msg) {
  board.update(msg);
  if (msg.result && msg.result.outcome && msg.result.outcome !== 'ongoing') {
    if (els.resignBtn) els.resignBtn.disabled = true;
    // The board keeps the final position on screen; clear the game area after a
    // short beat so players can read the result before returning to the menu.
    setTimeout(endGame, 5000);
  }
}

let toastTimer = null;
function toast(message) {
  if (!els.toast) return;
  els.toast.textContent = message;
  els.toast.hidden = false;
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    els.toast.hidden = true;
  }, 4000);
}

// --- Wiring ---------------------------------------------------------------

client
  .on('open', () => setStatus(true))
  .on('close', () => setStatus(false))
  .on('welcome', (msg) => {
    if (els.username) els.username.textContent = msg.username;
    populateVariantPicker(els.variantPicker, msg.variants);
  })
  .on('users_update', () => renderUsers())
  .on('challenge_received', (msg) => showChallengePrompt(msg))
  .on('challenge_declined', () => toast('Your challenge was declined.'))
  .on('challenge_expired', () => {
    hideChallengePrompt();
    toast('A challenge expired.');
  })
  .on('game_start', (msg) => {
    hideChallengePrompt();
    showGame(msg);
  })
  .on('game_update', (msg) => updateGame(msg))
  .on('opponent_disconnected', () => {
    toast('Opponent disconnected — you win.');
    setTimeout(endGame, 5000);
  })
  .on('error', (msg) => toast(msg.message || 'Error'));

if (els.acceptBtn) {
  els.acceptBtn.addEventListener('click', () => {
    if (pendingChallenge) {
      opponentName = pendingChallenge.fromUsername || '';
      client.acceptChallenge(pendingChallenge.challengeId);
    }
    hideChallengePrompt();
  });
}
if (els.resignBtn) {
  els.resignBtn.addEventListener('click', () => {
    if (client.gameId) client.resign(client.gameId);
  });
}
if (els.declineBtn) {
  els.declineBtn.addEventListener('click', () => {
    if (pendingChallenge) client.declineChallenge(pendingChallenge.challengeId);
    hideChallengePrompt();
  });
}

setStatus(false);
client.connect();
