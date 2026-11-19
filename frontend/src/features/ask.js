// The Ask AI dialog (issue #146, step 3; cm_AIAsk, Ctrl+Shift+A, in the Commands menu): the general prompt of the AI
// layer. The user writes what mormiCommander should do in plain words ("show the PDFs of this week in the left panel,
// sorted by size", "make up a dark colour scheme", "pack the selected photos into a zip on the right"); Enter sends it
// to the model (Go: AI.Start with the provider chosen in Options > AI), which works through the page's tools
// (core/aitools.js) and nothing else. The tools only read, navigate, select, search, change the view and the look, and
// pack copies into a new archive: none deletes, moves, renames, overwrites or runs a program (aitools-allow.js), and
// the context the model gets says so, so it tells the user what it cannot do instead of trying.
//
// The dialog is modeless where dialogs have windows of their own, so the panels stay usable and the user watches the
// model work on them. It shows which providers can be used, and a log of the run: the tools called (as readable
// lines, aitools-more-lib.js describeAnyTool), the model's text, the final answer or the error. Stop stops the run;
// closing the dialog stops it too. The last prompts are kept (setting "aiAskHistory"): Up and Down in an empty prompt
// box, or with the caret at its start, go through them.

import { go, errText } from '../core/go.js';
import { command, getCommand } from '../core/registry.js';
import { modal, esc, focusDialog } from '../core/dialogs.js';
import { onAIProgress } from '../core/aitools.js';
import { app as theApp } from '../core/app.js';
import { comboFromEvent } from '../core/keys.js';
import { askContext, describeAnyTool, pushHistory, historyStep, providerLines } from './aitools-more-lib.js';
import './ask.css';
import { translateTree, tr } from '../core/i18n.js';

const LOG_MAX = 300;
const HISTORY_KEY = 'aiAskHistory';
const NOT_AVAILABLE = 'AI is not available in this build.';

// The dialog's run. Like the Find dialog's AI tab (find-ai.js): progress events that come before AI.Start answered
// are kept until the run's number is known.
const A = {
  box: null, done: null,
  run: 0, running: false, starting: null, early: [], stopWanted: false,
  log: [],            // {kind: "note"|"tool"|"text"|"answer"|"error", text}
  providers: null,    // providerLines() of the last AI.Providers()
  hist: { at: 0, draft: '' },
};

const $ = (sel) => A.box?.querySelector(sel);
const notAvailable = (e) => /AI\.\w+ is not available/.test(errText(e));
const history = () => { const h = theApp.setting(HISTORY_KEY, []); return Array.isArray(h) ? h.filter((x) => typeof x === 'string') : []; };

// ---- the log ----

function push(kind, text) {
  if (!text) return;
  A.log.push({ kind, text });
  if (A.log.length > LOG_MAX) A.log.splice(0, A.log.length - LOG_MAX);
  renderLog();
}

function renderLog() {
  const el = $('.mc-ask-log');
  if (!el) return;
  el.innerHTML = A.log.length
    ? A.log.map((x) => `<div class="ai-${x.kind}">${esc(x.text)}</div>`).join('')
    : `<div class="ai-empty">${esc(tr('Write what mormiCommander should do, then press Enter. The AI works with mormiCommander\'s own tools: it can look around, open folders, select, search, sort, change the view and the colours, and pack copies into a new archive. It cannot delete, move, rename or overwrite files, or run programs.'))}</div>`;
  el.scrollTop = el.scrollHeight;
}

function renderProviders() {
  const el = $('.mc-ask-provider');
  if (!el) return;
  const p = A.providers;
  if (!p) { el.textContent = tr('Looking for AI providers…'); el.className = 'mc-ask-provider'; return; }
  el.className = `mc-ask-provider${p.ok ? ' ok' : ' bad'}`;
  // (plain text, no links: the page links only to the sites privacy.test.js allows)
  el.innerHTML = p.lines.map((l) => `<div class="${l.ok ? 'ok' : 'bad'}"${l.error ? ` title="${esc(l.error)}"` : ''}>${esc(l.text)}</div>`).join('');
}

function sync() {
  const start = $('.ask-start'), stop = $('.ask-stop');
  if (start) start.disabled = A.running;
  if (stop) stop.disabled = !A.running;
}

async function refreshProviders() {
  try { A.providers = providerLines(await go.AI.Providers()); } catch { A.providers = providerLines(null); }
  renderProviders();
}

// ---- progress of the run ----

function progress(ev, run) {
  if (!A.run) { if (A.starting) A.early.push([ev, run]); return; }
  if (run !== A.run) return;
  switch (ev.kind) {
    case 'text': {
      const last = A.log[A.log.length - 1];
      if (last?.kind === 'text') { last.text += ev.text || ''; renderLog(); } else push('text', ev.text);
      break;
    }
    case 'tool': push('tool', describeAnyTool(ev.tool, ev.args, theApp.home, (n) => getCommand(n)?.label || '')); break;
    case 'done': {
      // the final answer is usually the model's last text again: that entry becomes the answer instead of a copy
      const last = A.log[A.log.length - 1];
      const t = String(ev.text || '').trim();
      if (last?.kind === 'text' && last.text.trim() === t) last.kind = 'answer';
      else if (t) A.log.push({ kind: 'answer', text: t });
      ended();
      break;
    }
    case 'error': push('error', ev.error || ev.text || 'The run failed.'); ended(); break;
    default: break;
  }
}

function ended() {
  A.running = false;
  renderLog();
  sync();
}

onAIProgress(0, progress);

// ---- starting and stopping ----

const panelOf = (p) => ({ side: p.side, path: p.path, cursor: p.cur && !p.cur.up ? p.cur.name : '', selected: p.selectedEntries().length });

async function start() {
  const input = $('#ask-prompt');
  if (!A.box || !input || A.running) return;
  const prompt = input.value.trim();
  if (!prompt) { input.focus(); return; }
  theApp.setSetting(HISTORY_KEY, pushHistory(history(), prompt));
  A.hist = { at: history().length, draft: '' };
  const context = askContext({ active: panelOf(theApp.cur), other: panelOf(theApp.other), platform: theApp.platform, home: theApp.home });
  Object.assign(A, { run: 0, running: true, early: [], stopWanted: false, log: [] });
  push('note', `› ${prompt}`);
  sync();
  A.starting = Promise.resolve(go.AI.Start({ prompt, context, provider: '' }));
  try {
    A.run = await A.starting;
  } catch (e) {
    A.running = false;
    push('error', notAvailable(e) ? NOT_AVAILABLE : errText(e));
    sync();
    return;
  } finally {
    A.starting = null;
  }
  if (A.stopWanted) go.AI.Stop(A.run).catch(() => {});
  const early = A.early;
  A.early = [];
  for (const [ev, run] of early) progress(ev, run);
}

function stop() {
  if (!A.running) return;
  push('note', 'Stopping…');
  if (!A.run) { A.stopWanted = true; return; }
  go.AI.Stop(A.run).catch(() => {});
}

// ---- keys of the prompt box ----

// historyKey handles Up and Down in the prompt box: through the last prompts when the box is empty or the caret is at
// its start (Up) or end (Down), so moving between lines of a long prompt still works.
function historyKey(ev, combo) {
  const t = ev.target;
  if (t?.id !== 'ask-prompt' || (combo !== 'Up' && combo !== 'Down')) return false;
  const list = history();
  if (!list.length) return false;
  const empty = !t.value;
  const atStart = t.selectionStart === 0 && t.selectionEnd === 0;
  const atEnd = t.selectionStart === t.value.length;
  if (combo === 'Up' && !(empty || atStart)) return false;
  if (combo === 'Down' && !(empty || atEnd || A.hist.at < list.length)) return false;
  if (A.hist.at > list.length) A.hist.at = list.length;
  if (A.hist.at === list.length) A.hist.draft = t.value;
  const r = historyStep(list, A.hist.at, combo === 'Up' ? -1 : 1, A.hist.draft);
  if (r.at === A.hist.at) return combo === 'Up'; // (at the oldest: Up does nothing more; at the draft: Down moves the caret)
  A.hist.at = r.at;
  t.value = r.text;
  t.setSelectionRange?.(combo === 'Up' ? 0 : t.value.length, combo === 'Up' ? 0 : t.value.length);
  return true;
}

function onKey(ev) {
  if (ev.isComposing) return false;
  const combo = comboFromEvent(ev);
  if (combo === 'Enter' && ev.target?.id === 'ask-prompt') { start(); return true; }
  if (combo === 'Esc' && A.running) { stop(); return true; }
  return historyKey(ev, combo);
}

// ---- the dialog ----

function openAsk() {
  if (A.box) { $('#ask-prompt')?.focus(); focusDialog(A.box); return true; }
  A.hist = { at: history().length, draft: '' };
  const shown = modal((box, done) => {
    A.box = box;
    A.done = done;
    box.classList.add('mc-ask');
    box.innerHTML = `<h2>Ask AI</h2>
      <textarea id="ask-prompt" rows="3" spellcheck="false" data-enter-submits autofocus aria-label="What should mormiCommander do?"
        placeholder="What should mormiCommander do? E.g. show the PDFs I changed this week in the left panel, sorted by size"></textarea>
      <div class="mc-ask-provider"></div>
      <div class="mc-ask-log" aria-live="polite"></div>
      <p class="mc-note mc-ask-note">Enter asks, Shift+Enter starts a new line, Up and Down bring back earlier prompts.</p>
      <div class="mc-btns">
        <button type="submit" class="primary ask-start">Ask</button>
        <button type="button" class="ask-stop">Stop</button>
        <button type="button" class="ask-close">Close</button>
      </div>`;
    translateTree(box);
    box.onsubmit = (ev) => { ev.preventDefault(); start(); };
    box.querySelector('.ask-stop').onclick = stop;
    box.querySelector('.ask-close').onclick = () => done(null);
    renderProviders();
    renderLog();
    sync();
    refreshProviders();
  }, null, { kind: 'ai-ask', modeless: true, onKey });
  shown.then(() => {
    stop();
    A.box = null;
    A.done = null;
  });
  return true; // (the dialog stays open; the command is done)
}

command({
  name: 'cm_AIAsk', label: 'Ask AI…', menu: 'Commands', menuAfter: 'cm_CommandPalette', keys: ['Ctrl+Shift+A', 'Shift+Cmd+A'],
  run: () => openAsk(),
});
