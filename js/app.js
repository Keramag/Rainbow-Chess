// app.js — thin DOM glue that wires the MultiplayerClient to the page shell.
//
// This is the only module that touches the DOM. It owns no rules logic. Two
// pure modules do the thinking it renders from:
//   - game-state.js: the high-level screen state (menu / playing / over) plus
//     the player/variant context and any transient notice. app.js never decides
//     a transition inline — it dispatch()es each server message into the reducer
//     and re-renders, so the whole game lifecycle stays unit-tested.
//   - chess.js (BoardView): the board itself and click-to-move.
// Everything else here (header, online-users, challenge prompt, toasts) is glue.

import { MultiplayerClient } from './multiplayer.js';
import { populateVariantPicker, variantLabel } from './variants.js';
import { BoardView } from './chess.js';
import { PHASE, initialState, reduce, returnToMenu, clearNotice, playerOutcome } from './game-state.js';

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
  gameInfo: $('game-info'),
  boardRoot: $('board-root'),
  resignBtn: $('resign-btn'),
  gameOver: $('game-over'),
  gameOverText: $('game-over-text'),
  backToMenuBtn: $('back-to-menu'),
  gameIdle: $('game-idle'),
  toast: $('toast'),
};

const client = new MultiplayerClient();

// ui is the high-level screen state, owned by the pure reducer. dispatch() is
// the single place server (and local) events fold into it before a re-render.
let ui = initialState();

function dispatch(msg) {
  ui = reduce(ui, msg);
  renderUI();
}

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

// renderUI reflects the reducer's screen state onto the page: which panel is
// visible, the in-game player/variant header, the game-over offer, and any
// transient notice (decline / expiry / disconnect / error) as a toast.
function renderUI() {
  const inGame = ui.phase !== PHASE.MENU;
  if (els.gameArea) els.gameArea.hidden = !inGame;
  if (els.gameIdle) els.gameIdle.hidden = inGame;
  // Resign is only meaningful while a game is actually in progress.
  if (els.resignBtn) els.resignBtn.disabled = ui.phase !== PHASE.PLAYING;

  renderGameInfo();
  renderGameOver();

  if (ui.notice) {
    toast(ui.notice.text);
    ui = clearNotice(ui);
  }
}

// renderGameInfo shows the active variant and both players with their colors,
// marking which side is "you". client.username is known by game_start (welcome
// always arrives first); opponentName is remembered when we issue/accept.
function renderGameInfo() {
  if (!els.gameInfo) return;
  const g = ui.game;
  if (!g) {
    els.gameInfo.textContent = '';
    return;
  }
  const me = client.username || 'You';
  const opp = g.opponentName || 'Opponent';
  const white = g.myColor === 'white' ? me : opp;
  const black = g.myColor === 'white' ? opp : me;
  const youWhite = g.myColor === 'white' ? ' (you)' : '';
  const youBlack = g.myColor === 'black' ? ' (you)' : '';
  els.gameInfo.textContent = `${variantLabel(g.variant)} · White: ${white}${youWhite}  vs  Black: ${black}${youBlack}`;
}

// renderGameOver shows the result and the "new game / back to menu" offer when a
// game has ended; the final board stays on screen behind it.
function renderGameOver() {
  if (!els.gameOver) return;
  const g = ui.game;
  if (ui.phase !== PHASE.OVER || !g || !g.result) {
    els.gameOver.hidden = true;
    return;
  }
  const outcome = playerOutcome(g.result, g.myColor);
  const headline = outcome === 'win' ? 'You win' : outcome === 'loss' ? 'You lose' : 'Draw';
  const reason = g.result.reason ? ` (${g.result.reason})` : '';
  if (els.gameOverText) els.gameOverText.textContent = `${headline}${reason}`;
  els.gameOver.hidden = false;
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
  .on('challenge_declined', (msg) => dispatch(msg))
  .on('challenge_expired', (msg) => {
    hideChallengePrompt();
    dispatch(msg);
  })
  .on('game_start', (msg) => {
    hideChallengePrompt();
    // The board renderer needs the raw message + the opponent name; the reducer
    // tracks the surrounding screen state (and the same opponent name).
    board.start(msg, opponentName);
    dispatch({ ...msg, opponentName });
  })
  .on('game_update', (msg) => {
    board.update(msg);
    dispatch(msg);
  })
  .on('opponent_disconnected', (msg) => dispatch(msg))
  .on('error', (msg) => dispatch(msg));

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
if (els.backToMenuBtn) {
  // "New game / back to menu": discard the finished game and return to the
  // lobby-less menu, where the player can issue or accept a fresh challenge.
  els.backToMenuBtn.addEventListener('click', () => {
    ui = returnToMenu(ui);
    opponentName = '';
    renderUI();
  });
}

// The footer's commit SHA is substituted at image-build time (Dockerfile seds
// __COMMIT_SHA__). When the server is run straight from the repo (local dev) no
// substitution happens, so fall back to a "dev" label rather than showing the
// raw placeholder token.
const commitEl = $('commit-sha');
if (commitEl && commitEl.textContent.includes('__COMMIT_SHA__')) {
  commitEl.textContent = 'Commit: dev';
}

setStatus(false);
renderUI();
client.connect();
