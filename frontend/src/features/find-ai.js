// The AI tab of the Find files dialog and the find_files tool (issue #146, step 1 of the AI layer). The user writes
// what to find in plain words ("PDFs in Documents I changed this week, bigger than 1 MB"); Start search sends it to
// the model (Go: AI.Start with the provider chosen in Configuration > AI settings: the Claude Code CLI, or OpenRouter),
// which answers by calling find_files, and the search runs as a normal Find files search: the results fill the
// dialog's list, so Go to file, View, Feed to listbox and the rest work on them as on any search. The tab shows which
// provider and model a run will use (or what is missing, with a button to the AI settings, ai-settings.js), and a
// short log of the run: the tools the model calls, its text, the final answer or the error.
//
// find_files is one tool of core/aitools.js. Called by a run of this dialog's AI tab, it searches through the dialog
// (find.js findDialog.search); called by any other run, it runs go.Find.Start itself and reads the results back.
// Either way the model gets the count and the first results as text (aitools-lib.js findSummary), and the search is
// stopped before Go's 120 s wait for a tool answer runs out, so the model gets what was found rather than an error.
//
// Settings: "findAiPrompt" (the last prompt, put back in the box next time).

import { go, on, errText } from '../core/go.js';
import { esc } from '../core/dialogs.js';
import { addAITool, onAIProgress } from '../core/aitools.js';
import { app as theApp } from '../core/app.js';
import { addFindTab, findDialog } from './find.js';
import { openAISettings } from './ai-settings.js';
import { findRequest, findSummary, describeTool, findContext, providerLine, day } from './aitools-lib.js';
import { tr } from '../core/i18n.js';

const TOOL_TIMEOUT = 100_000; // ms a search may take before it is stopped (Go waits 120 s for a tool's answer)
const LOG_MAX = 200;
const NOT_AVAILABLE = 'AI is not available in this build.';

// The AI run of the dialog. It outlives the dialog's box: the log and the answer are drawn again when the dialog
// comes back (after Go to file, Alt+F7), but a run still going when the dialog closes is stopped.
const A = {
  run: 0,           // the run AI.Start answered (0: none yet)
  running: false,
  starting: null,   // the AI.Start promise while it is pending
  early: [],        // progress events that came before AI.Start answered (their run number is not known yet)
  stopWanted: false,
  log: [],          // {kind: "note"|"tool"|"text"|"answer"|"error", text}
  provider: null,   // providerLine() of the last AI.Providers()
};

const $ = (sel) => findDialog.box?.querySelector(sel);
const notAvailable = (e) => /AI\.\w+ is not available/.test(errText(e));

// ---- the log ----

function push(kind, text) {
  if (!text) return;
  A.log.push({ kind, text });
  if (A.log.length > LOG_MAX) A.log.splice(0, A.log.length - LOG_MAX);
  renderLog();
}

function renderLog() {
  const el = $('.mc-find-ai-log');
  if (!el) return;
  el.innerHTML = A.log.length
    ? A.log.map((x) => `<div class="ai-${x.kind}">${esc(x.text)}</div>`).join('')
    : `<div class="ai-empty">${esc(tr('Describe the files you are looking for, then press Enter or Start search. The model searches with Find files; the results fill the list below.'))}</div>`;
  el.scrollTop = el.scrollHeight;
}

function renderProvider() {
  const el = $('.mc-find-ai-provider');
  if (!el) return;
  const p = A.provider;
  if (!p) { el.textContent = tr('Looking for the AI provider…'); el.className = 'mc-find-ai-provider'; return; }
  el.className = `mc-find-ai-provider${p.ok ? ' ok' : ' bad'}`;
  // (plain text, not a link: the page links only to the sites privacy.test.js allows)
  el.textContent = p.text;
  el.title = p.error || '';
}

async function refreshProvider() {
  try { A.provider = providerLine(await go.AI.Providers()); } catch { A.provider = providerLine(null); }
  renderProvider();
}

// ---- progress of the run ----

function progress(ev, run) {
  if (!A.run) { if (A.starting) A.early.push([ev, run]); return; }
  if (run !== A.run) return;
  switch (ev.kind) {
    case 'text': {
      // (the text may come in pieces: they join the text entry the log ends with)
      const last = A.log[A.log.length - 1];
      if (last?.kind === 'text') { last.text += ev.text || ''; renderLog(); } else push('text', ev.text);
      break;
    }
    case 'tool': push('tool', describeTool(ev.tool, ev.args, theApp.home)); break;
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
  findDialog.sync();
}

onAIProgress(0, progress);

// ---- starting and stopping ----

async function startAI() {
  const box = findDialog.box;
  const input = $('#ff-ai-prompt');
  if (!box || !input || A.running) return;
  const prompt = input.value.trim();
  if (!prompt) { findDialog.showError('Write what you are looking for.'); input.focus(); return; }
  findDialog.showError('');
  theApp.setSetting('findAiPrompt', prompt);
  const searchIn = box.querySelector('#ff-in')?.value || '';
  const context = findContext({
    active: theApp.cur, other: theApp.other, platform: theApp.platform, home: theApp.home, searchIn,
  });
  Object.assign(A, { run: 0, running: true, early: [], stopWanted: false, log: [] });
  push('note', `Asking ${A.provider?.ok ? A.provider.name : 'the AI'}…`);
  findDialog.sync();
  // (no provider of its own: Go uses the one chosen in the AI settings)
  A.starting = Promise.resolve(go.AI.Start({ prompt, context, provider: '' }));
  try {
    A.run = await A.starting;
  } catch (e) {
    A.running = false;
    push('error', notAvailable(e) ? NOT_AVAILABLE : errText(e));
    findDialog.sync();
    return;
  } finally {
    A.starting = null;
  }
  if (A.stopWanted) go.AI.Stop(A.run).catch(() => {});
  const early = A.early;
  A.early = [];
  for (const [ev, run] of early) progress(ev, run);
}

function stopAI() {
  if (!A.running) return;
  if (!A.run) { A.stopWanted = true; return; }
  go.AI.Stop(A.run).catch(() => {});
}

// ---- the tab ----

addFindTab({
  id: 'ai',
  label: 'AI',
  html: () => `<textarea id="ff-ai-prompt" rows="3" spellcheck="false" data-enter-submits aria-label="What to find"
      placeholder="What are you looking for? E.g. PDFs in Documents changed this week, bigger than 1 MB">${esc(theApp.setting('findAiPrompt', '') || '')}</textarea>
    <div class="mc-find-ai-providerbar"><div class="mc-find-ai-provider"></div>
      <button type="button" class="mc-find-ai-settings" title="Choose the AI provider, the model and the API key">AI settings…</button></div>
    <div class="mc-find-ai-log" aria-live="polite"></div>`,
  setup: () => {
    renderProvider(); renderLog(); refreshProvider();
    const b = $('.mc-find-ai-settings');
    if (b) b.onclick = async () => { await openAISettings(); refreshProvider(); $('#ff-ai-prompt')?.focus(); };
  },
  shown: () => { refreshProvider(); const t = $('#ff-ai-prompt'); t?.focus(); },
  start: startAI,
  busy: () => A.running,
  stop: stopAI,
  // Enter in the prompt starts (a text area would add a new line; Shift+Enter still does)
  key: (ev, combo) => {
    if (combo !== 'Enter' || ev.target?.id !== 'ff-ai-prompt') return false;
    if (!A.running && !findDialog.running) startAI();
    return true;
  },
  closed: stopAI,
});

// ---- find_files ----

// searchDirect runs a search without the dialog (a run that did not come from its AI tab) and reads the results.
async function searchDirect(req) {
  let done = null, id = 0;
  const dones = new Map();
  const off = on('find-done', (d) => { if (d?.id) dones.set(d.id, d); });
  try {
    id = await go.Find.Start(req);
    const t0 = Date.now();
    let stopped = false;
    while (await go.Find.Running(id)) {
      if (!stopped && Date.now() - t0 > TOOL_TIMEOUT) { stopped = true; go.Find.Stop(id).catch(() => {}); }
      await new Promise((r) => setTimeout(r, 200));
    }
    const items = (await go.Find.Results(id)) || [];
    done = dones.get(id) || (stopped ? { stopped: true } : null);
    return findSummary(items, done);
  } finally {
    off();
    if (id) go.Find.Forget(id).catch(() => {});
  }
}

addAITool({
  name: 'find_files',
  description: 'Searches for files and folders with mormiCommander\'s Find files (by name, text inside files, size, date, kind). '
    + 'Answers how many were found and lists up to 200 (path, size, modification date). When the user asked from the Find files dialog, '
    + 'the results also fill its result list. Wildcards: * and ?; several name patterns are separated by ";" (like "*.pdf;*.docx").',
  schema: {
    type: 'object',
    properties: {
      searchIn: { type: 'string', description: 'Folders to search in, separated by ";". "~" is the home folder. Default: the active panel\'s folder.' },
      namePattern: { type: 'string', description: 'Name masks like "*.pdf;*.docx" (default "*"), or a regular expression when nameRegex is true.' },
      nameRegex: { type: 'boolean', description: 'namePattern is a regular expression.' },
      text: { type: 'string', description: 'Text the files must contain.' },
      textRegex: { type: 'boolean', description: 'text is a regular expression.' },
      caseSensitive: { type: 'boolean', description: 'The text must match with the same upper and lower case letters.' },
      depth: { type: 'integer', description: '-1 (default): all subfolders; 0: the folder only; n: n levels below.' },
      kind: { type: 'string', enum: ['files', 'dirs'], description: 'Only files or only folders (default both).' },
      sizeOp: { type: 'string', enum: ['>', '<', '='], description: 'How the size compares with sizeValue (default ">").' },
      sizeValue: { type: 'number', description: 'A size, in sizeUnit.' },
      sizeUnit: { type: 'string', enum: ['B', 'KB', 'MB', 'GB'], description: 'Unit of sizeValue (default B).' },
      dateFrom: { type: 'string', description: 'Modified on or after this day, YYYY-MM-DD.' },
      dateTo: { type: 'string', description: 'Modified on or before this day, YYYY-MM-DD.' },
      olderThanDays: { type: 'integer', description: 'Modified more than this many days ago.' },
      newerThanDays: { type: 'integer', description: 'Modified within this many days.' },
      excludeDirs: { type: 'string', description: 'Folder names to skip, separated by ";" (like "node_modules;.git").' },
      includeHidden: { type: 'boolean', description: 'Search hidden files and folders too (default false).' },
    },
  },
  run: async (args, ctx) => {
    if (A.starting) await A.starting.catch(() => {}); // (a call this early may be from the run that is starting)
    const req = findRequest(args, { activePath: theApp.cur?.path || theApp.home, home: theApp.home, sep: theApp.sep, today: day(new Date()) });
    if (!(findDialog.box && ctx?.run && ctx.run === A.run)) return searchDirect(req);
    const r = await findDialog.search(req, { timeout: TOOL_TIMEOUT });
    if (r.error) throw new Error(r.error);
    push('note', findDialog.status());
    return findSummary(r.items, r.done, { where: 'They are shown in the Find files dialog\'s result list.' });
  },
});
