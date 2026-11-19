// File jobs from the page's side (copy, move, delete, trash, wipe): start one in Go (feature_jobs.go, internal/fileops),
// show its progress, ask the questions the engine asks (a file is in the way, a file is read-only, a file can't be
// read, a folder is not empty...), keep the list of background jobs, and resolve with the final state. Features call:
//
//   const p = await runJob({kind: 'copy', sources: [...full paths], target: '/folder', rename: '', overwrite: ''},
//                          {title: 'Copying', background: false});
//   p.state is 'done', 'failed' (p.errors has sentences) or 'cancelled'; both panels are reread afterwards.
//
// A foreground job shows a progress box (after a moment, so a small copy doesn't flash one) with Background (F2),
// Pause and Cancel. A background job (the copy dialog's F2 Queue, or Background in the progress box) shows a small
// indicator above the function keys; showTransfers() opens the background transfer window (TC's Background Transfer
// Manager) with pause, continue, start now, stop and a speed limit.
//
// Every box here is a page dialog, so control mode reads and answers it ("dialog" / "dialog_answer"): kinds "job"
// (progress), "overwrite", "overwrite-readonly", "job-error", "no-space", "delete-folder", "delete-readonly",
// "rename-target", "transfers" and "job-errors".
//
// The error list (board issue #15): the progress box and the transfer window show how many errors a job has had, the
// transfer window's "Errors…" opens the list of a finished job's failed paths with their reasons, and showErrorList
// offers Retry failed, Copy list and Save log. With the "Skip errors in background" setting on (fileops setting
// skipErrorsBackground, off by default), a job sent to the background stops asking Retry / Skip, and a job that
// skipped errors opens the list by itself when it ends, instead of the short message. core/joberrors.js has the logic.

import './jobs.css';
import { go, on, errText, setClipboardText } from './go.js';
import { modal, messageDialog, inputDialog, esc } from './dialogs.js';
import { size, date, bytes, plural } from './format.js';
import { t as tt } from './i18n.js';
import { track } from './busy.js';
import { app } from './app.js';
import { holdProgress, progressHeld, showWhenFree } from './progresshold.js';
import { ERROR_BUTTONS, offeredButtons } from './jobchoices.js';
import { failuresOf, errorCount, listText, logText, logName, retryRequests, listAtEnd, deniedHint, anyDenied } from './joberrors.js';

// Job engines of features (archives and servers: features/vfs.js) offer themselves here: start(req) resolves with a
// job number, or 0 when the job is not theirs (then it goes to Go's Jobs); cancel(id) and answer(id, a) serve their jobs.
export const jobBackends = [];
const backendOf = new Map(); // job number -> backend
const cancelJob = (id) => {
  const r = jobs.get(id);
  if (r?.ext) return Promise.resolve(r.ext.cancel());
  return backendOf.get(id) ? backendOf.get(id).cancel(id) : go.Jobs.Cancel(id);
};

const jobs = new Map(); // id → record {id, req, title, p, background, done, resolve, ui}
const subs = new Set();
let listening = false;

// onJobs calls fn whenever a job changes (the transfer window and the indicator listen); it returns an unsubscribe.
export function onJobs(fn) { subs.add(fn); return () => subs.delete(fn); }
function notify() {
  for (const f of subs) { try { f(); } catch (e) { console.error(e); } }
  paintIndicator();
}

function record(id) {
  let r = jobs.get(id);
  if (!r) {
    r = { id, req: null, title: '', p: null, background: true, done: false, resolve: null, ui: null, claimed: false, seen: Date.now() };
    jobs.set(id, r);
  }
  return r;
}

function listen() {
  if (listening) return;
  listening = true;
  on('job', (p) => { const r = record(p.id); r.p = p; r.ui?.paint(); notify(); });
  on('job-ask', (p) => { const r = record(p.id); r.p = p; r.ui?.paint(); notify(); ask(r); });
  on('job-done', (p) => {
    const r = record(p.id);
    r.p = p;
    r.done = true;
    r.ui?.close();
    const resolve = r.resolve;
    r.resolve = null;
    if (resolve) resolve(p);
    else setTimeout(() => { if (!r.claimed) finishedInBackground(r); }, 300); // (runJob may not have heard of it yet)
    notify();
    // Options > Background queue > Keep finished jobs in the list off: a finished job leaves the list a moment later
    if (cfgOpt('BackgroundQueue.KeepFinished', '1') === '0') {
      setTimeout(() => { if (jobs.get(p.id)?.done) { jobs.delete(p.id); go.Jobs.Forget().catch(() => {}); notify(); } }, 3000);
    }
  });
}

// jobList is every job this page knows, oldest first: {id, kind, state, title, background, target, sources, ...Progress}.
export function jobList() {
  return [...jobs.values()].filter((r) => r.p).map((r) => ({ ...r.p, title: r.title, background: r.background, sources: r.req?.sources || [] }));
}

// refreshJobs merges the Go side's list (jobs started before this page listened, or by another feature).
export async function refreshJobs() {
  listen();
  const list = await go.Jobs.List().catch(() => []);
  for (const p of list || []) { const r = record(p.id); r.p = p; r.seen = 0; if (['done', 'failed', 'cancelled'].includes(p.state)) r.done = true; }
  notify();
}

const VERB = { copy: 'Copying', move: 'Moving', delete: 'Deleting', trash: 'Moving to the trash', wipe: 'Wiping', pack: 'Packing', unpack: 'Unpacking', test: 'Testing archives' };
const KIND = { copy: 'Copy', move: 'Move', delete: 'Delete', trash: 'Trash', wipe: 'Wipe', pack: 'Pack', unpack: 'Unpack', test: 'Test' };
const STATE = { queued: 'Waiting', running: 'Running', paused: 'Paused', asking: 'Waiting for an answer', done: 'Done', failed: 'Done with errors', cancelled: 'Stopped' };
export const JOB_WORDS = { VERB, KIND, STATE }; // (for the translation test, #113)
// pl is plural() with the words translated (#113): Hungarian, for one, keeps the singular after a number.
const pl = (n, word, many = `${word}s`) => plural(n, tt(word), tt(many));
const base = (p) => String(p || '').split(/[\\/]/).filter(Boolean).pop() || p || '';

export function fraction(p) {
  if (!p) return 0;
  if (p.state === 'done') return 1;
  const f = p.bytes > 0 ? p.bytesDone / p.bytes : p.files > 0 ? p.filesDone / p.files : 0;
  return Math.max(0, Math.min(1, f));
}

// eta formats seconds as "0:42", "12:05" or "1:02:03".
export function eta(s) {
  if (s == null || s < 0) return '';
  s = Math.round(s);
  const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
  const two = (n) => String(n).padStart(2, '0');
  return h ? `${h}:${two(m)}:${two(sec)}` : `${m}:${two(sec)}`;
}

// ---- jobs another engine runs (server transfers, F-382) ----

// trackJob lists a job that another engine runs and reports (the server transfers of features/net.js) in the
// background transfer window, next to the file jobs. key must not clash with the numbers of Go's file jobs (net.js uses
// the negative transfer number); cancel stops the job. It returns {update(p), done(p), background()}: update takes a
// progress record ({kind, state, files, filesDone, bytes, bytesDone, current, target, errors}), done the final one,
// background moves the job to the background indicator (its progress box was sent away). Pause and the speed limit
// are not offered for such jobs.
export function trackJob(key, { req = null, title = '', cancel = () => {}, background = false } = {}) {
  listen();
  const r = record(key);
  r.ext = { cancel };
  r.claimed = true;
  r.req = req;
  r.title = title;
  r.background = background;
  r.p = { id: key, kind: req?.kind || 'copy', state: 'running', files: 0, filesDone: 0, bytes: 0, bytesDone: 0, current: '', target: req?.target || '', server: true };
  notify();
  return {
    update(p) { if (r.done) return; r.p = { ...p, id: key, server: true }; notify(); },
    done(p) {
      r.p = { ...(p || r.p), id: key, server: true };
      r.done = true;
      notify();
      if (cfgOpt('BackgroundQueue.KeepFinished', '1') === '0') {
        setTimeout(() => { if (jobs.get(key) === r) { jobs.delete(key); notify(); } }, 3000);
      }
    },
    background() { if (!r.done) { r.background = true; notify(); } },
  };
}

// ---- starting ----

// runJob starts a job and resolves with its final Progress. Errors starting it reject with a sentence.
// opts.background: no progress box, the job shows in the background indicator and the transfer window.
export function runJob(req, { title, background = false } = {}) {
  listen();
  const started = (async () => {
    let id = 0;
    for (const b of jobBackends) {
      id = await b.start(req).catch((e) => { throw new Error(errText(e)); });
      if (id) { backendOf.set(id, b); break; }
    }
    if (!id) id = await go.Jobs.Start(req).catch((e) => { throw new Error(errText(e)); });
    const r = record(id);
    r.claimed = true;
    r.req = req;
    r.title = title || VERB[req.kind] || 'Working';
    r.background = background;
    if (!r.p) r.p = { id, kind: req.kind, state: req.queued ? 'queued' : 'running', files: 0, filesDone: 0, bytes: 0, bytesDone: 0, current: '', target: req.target || '' };
    const result = new Promise((resolve) => { r.resolve = resolve; });
    if (r.done) { r.resolve = null; return { r, result: Promise.resolve(r.p) }; } // (it ended before we got here)
    if (!background) foreground(r);
    notify();
    // Options > Background queue > Show the transfer queue window: a job sent to the background or the queue opens it
    if (background && cfgOpt('BackgroundQueue.ShowWindow', '0') === '1' && !transfersOpen) showTransfers();
    return { r, result };
  })();
  const finish = async ({ r, result }) => {
    const final = await result;
    await Promise.all(app.panels.map((pn) => pn.reload()));
    report(r, final);
    return final;
  };
  let out;
  if (background) {
    // only the start is work in progress: the job runs on while the user (or a script) goes on
    out = track(started).then(finish);
  } else {
    out = track(started.then(finish));
  }
  // .started resolves once the job runs (or rejects with the reason it was refused), for callers that don't wait for the end
  out.started = started.then(() => true);
  out.started.catch(() => {});
  return out;
}

// report tells how a job ended: errors stay on screen longer; a background job also says when it is done.
function report(r, final) {
  if (listAtEnd({ skipErrorsBackground: skipErrorsSetting(), background: r.background, onError: r.req?.onError, final })) {
    showErrorList(r, final);
    return;
  }
  if (final.state === 'failed' && final.errors?.length) {
    app.flash(final.errors.slice(0, 3).join('\n') + (final.errors.length > 3 ? `\n…and ${final.errors.length - 3} more.` : ''), 'err');
  } else if (final.state === 'done' && final.summary && (r.background || summaryWanted(r.req))) {
    app.flash(final.summary);
  } else if (final.state === 'cancelled' && r.background) {
    app.flash(tt('{job} stopped.', { job: tt(KIND[final.kind] || 'Job') }));
  }
}

// summaryWanted: Options > Copy/Delete > Show a summary when a copy or move ends (off by default, F-492) also says how a
// foreground copy or move went.
export function summaryWanted(req, values = app.config?.values) {
  return (req?.kind === 'copy' || req?.kind === 'move') && (values?.['CopyDelete.ShowSummary'] ?? '0') === '1';
}

function finishedInBackground(r) {
  // a job nobody awaits (started by another page part, or before a reload): reread and say how it went
  Promise.all(app.panels.map((pn) => pn.reload())).then(() => report(r, r.p));
}

// ---- the progress box of a foreground job ----

// holdProgress (core/progresshold.js) keeps new progress boxes from opening while a question a job waits for is on
// screen (the password of an encrypted archive, or any question ask() shows): the box would cover the question, and
// keys and control mode would go to the box. It returns the function that ends the hold.
export { holdProgress };

function foreground(r) {
  let box = null, closeBox = null, shown = false;
  const ui = {
    paint: () => {},
    close: () => { cancelTimer(); closeBox?.('closed'); },
    showNow: () => { if (!shown && !progressHeld()) { cancelTimer(); show(); } }, // (an open question stays on top)
  };
  r.ui = ui;
  const cancelTimer = showWhenFree(show, 400); // waits while a question is open, so it stays on top
  function show() {
    if (shown || r.done) return;
    shown = true;
    modal((b, done) => {
      box = b;
      closeBox = done;
      b.classList.add('mc-jobs-progress');
      b.innerHTML = `<h2></h2><p class="mc-msg"></p><p class="mc-jobs-path"></p>
        <div class="mc-jobs-bar cur" title="${esc(tt('This file'))}"><i></i></div><div class="mc-jobs-bar total" title="${esc(tt('All files'))}"><i></i></div>
        <p class="mc-note"></p>
        <div class="mc-btns"><button type="button" class="bg" title="${esc(tt('Go on in the background (F2)'))}">${esc(tt('Background'))}</button><button type="button" class="pause">${esc(tt('Pause'))}</button><button type="button" class="cancel primary">${esc(tt('Cancel'))}</button></div>`;
      const $ = (s) => b.querySelector(s);
      ui.paint = () => {
        const p = r.p;
        const pct = Math.round(fraction(p) * 100);
        $('h2').textContent = `${pct}% ${tt(r.title)}`;
        $('.mc-msg').textContent = p.state === 'paused' ? tt('Paused: {name}', { name: base(p.current) }) : base(p.current) || (p.files ? '' : tt('Counting files…'));
        const to = r.req?.target && (p.kind === 'copy' || p.kind === 'move') ? ` → ${r.req.target}` : '';
        $('.mc-jobs-path').textContent = p.current ? `${p.current}${to}` : '';
        const curBar = $('.mc-jobs-bar.cur');
        curBar.hidden = !(p.currentSize > 0);
        curBar.firstElementChild.style.width = `${p.currentSize > 0 ? Math.round(Math.min(1, p.currentDone / p.currentSize) * 100) : 0}%`;
        $('.mc-jobs-bar.total i').style.width = `${pct}%`;
        const parts = [tt('{done} of {files}', { done: p.filesDone, files: pl(p.files, 'file') })];
        if (p.bytes) parts.push(tt('{done} of {size}', { done: size(p.bytesDone), size: size(p.bytes) }));
        if (p.speed > 0 && p.state === 'running') parts.push(`${size(p.speed)}/s`);
        if (p.eta > 0 && p.state === 'running') parts.push(tt('{time} left', { time: eta(p.eta) }));
        if (errorCount(p) > 0) parts.push(pl(errorCount(p), 'error'));
        $('.mc-note').textContent = parts.join(' · ');
        $('.pause').textContent = tt(p.state === 'paused' ? 'Continue' : 'Pause');
      };
      ui.paint();
      $('.cancel').onclick = () => done('cancel');
      $('.bg').onclick = () => done('background');
      $('.pause').onclick = () => (r.p.state === 'paused' ? go.Jobs.Resume(r.id) : go.Jobs.Pause(r.id)).catch(() => {});
      b.onsubmit = (ev) => ev.preventDefault();
    }, 'cancel', {
      kind: 'job',
      onKey: (ev) => {
        if (ev.key === 'F2') { closeBox?.('background'); return true; }
        if (ev.key === 'Enter') return true; // Enter must not stop a job by accident
        return false;
      },
    }).then((v) => {
      ui.paint = () => {};
      if (v === 'cancel' && !r.done) cancelJob(r.id).catch(() => {});
      if (v === 'background' && !r.done) {
        r.background = true;
        r.ui = null;
        // "Skip errors in background": from now on a failed file is noted for the list instead of asked about
        if (skipErrorsSetting() && !backendOf.get(r.id)) go.Jobs.SkipErrors(r.id).catch(() => {});
        notify();
      }
    });
  }
}

// ---- questions ----

function answer(r, choice, newName = '') {
  const be = backendOf.get(r.id);
  Promise.resolve(be ? be.answer(r.id, { choice, newName }) : go.Jobs.Answer(r.id, { choice, newName })).catch(() => {});
}

// pick shows a question with buttons [label, choice], only those the engine offers; Esc is Cancel.
async function pick(r, o, buttons) {
  const list = offeredButtons(buttons, r.p.ask.choices);
  const b = await messageDialog({ ...o, Buttons: list.map(([l]) => l), CancelButton: list.find(([, c]) => c === 'cancel')?.[0] });
  return list.find(([l]) => l === b)?.[1] || 'cancel';
}

// ask shows the question a job waits for. The progress box opens first when it is due anyway; while the question is
// open no progress box opens (it would cover the question: board issue #123).
async function ask(r) {
  const c = r.p?.ask;
  if (!c) return;
  r.ui?.showNow?.();
  const release = holdProgress();
  try { return await askNow(r, c); } finally { release(); }
}

async function askNow(r, c) {
  const name = base(c.target || c.source);
  switch (c.type) {
    case '': return overwrite(r, c);
    case 'readonly':
      return answer(r, await pick(r, { Title: 'Read-only file', Message: tt('{name} is read-only.\n\nOverwrite it anyway?', { name: c.target }), Kind: 'overwrite-readonly', DefaultButton: 'Skip' },
        [['Overwrite', 'overwrite'], ['Overwrite all', 'overwrite_all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']]));
    case 'hidden': // (only with Options > Copy/Delete > Confirm overwriting hidden and system files, F-467)
      return answer(r, await pick(r, { Title: 'Hidden or system file', Message: tt('{name} is a hidden or system file.\n\nOverwrite it anyway?', { name: c.target }), Kind: 'overwrite-hidden', DefaultButton: 'Skip' },
        [['Overwrite', 'overwrite'], ['Overwrite all', 'overwrite_all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']]));
    case 'error':
      return answer(r, await pick(r, { Title: 'Problem', Message: (c.error || tt('Something went wrong with {name}.', { name })) + (c.denied ? `\n\n${deniedHint(app.platform)}` : ''), Kind: 'job-error', DefaultButton: 'Retry' },
        ERROR_BUTTONS));
    case 'no_space':
      return answer(r, await pick(r, { Title: 'Not enough space', Message: tt('{name} has {free} free, but this needs {need}.\n\nStart anyway?', { name: c.target, free: size(c.free), need: size(c.need) }), Kind: 'no-space', DefaultButton: 'Cancel' },
        [['Start anyway', 'yes'], ['Cancel', 'cancel']]));
    case 'delete_dir': {
      const verb = r.p.kind === 'wipe' ? 'Wipe' : 'Delete';
      return answer(r, await pick(r, { Title: 'Folder is not empty', Message: verb === 'Wipe' ? tt('The folder {name} is not empty.\n\nWipe it with everything in it?', { name: c.source })
        : tt('The folder {name} is not empty.\n\nDelete it with everything in it?', { name: c.source }), Kind: 'delete-folder', DefaultButton: 'Yes', Danger: true },
        [['Yes', 'yes'], ['All', 'all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']]));
    }
    case 'delete_readonly':
      return answer(r, await pick(r, { Title: 'Read-only file', Message: tt('{name} is read-only.\n\nDelete it anyway?', { name: c.source }), Kind: 'delete-readonly', DefaultButton: 'Yes', Danger: true },
        [['Yes', 'yes'], ['All', 'all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']]));
    default:
      return answer(r, 'cancel');
  }
}

// The overwrite question (TC's "Confirm file replace"): both files with size and date, the newer date marked, and
// every answer the engine offers. Rare answers sit behind "More options".
const MAIN = [['Overwrite', 'overwrite'], ['Overwrite all', 'overwrite_all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']];
const SECOND = [['Overwrite all older', 'older_all'], ['Rename…', 'rename'], ['Compare', 'compare']];
const MORE = [['Auto-rename', 'auto_rename'], ['Auto-rename all', 'auto_rename_all'], ['Rename target', 'rename_target'], ['Rename all targets', 'rename_target_all'],
  ['Copy all larger', 'larger_all'], ['Copy all smaller', 'smaller_all'], ['Append', 'append'], ['Resume', 'resume']];

// setCompareTool is how features/comparefiles.js hands over its window (left, right) → Promise, so the overwrite question
// can open the two files in the compare-by-content tool (F-466) without core depending on a feature.
let compareTool = null;
export function setCompareTool(fn) { compareTool = fn; }

async function overwrite(r, c) {
  const offered = new Set(c.choices || []);
  // "Compare tool…" (F-466): next to the quick Compare, for two files on this computer; the question comes back after
  const toolBtn = compareTool && offered.has('compare') && !backendOf.get(r.id)
    ? `<button type="button" data-choice="compare_tool" title="${esc(tt('Open both files in the compare-by-content tool'))}">${esc(tt('Compare tool…'))}</button>` : '';
  const row = (list, cls = '') => `<div class="mc-btns ${cls}">${list.filter(([, ch]) => offered.has(ch))
    .map(([l, ch]) => `<button type="button" data-choice="${ch}"${ch === 'overwrite' ? ' class="primary"' : ''}>${esc(tt(l))}</button>`).join('')}</div>`;
  const newer = c.srcTime > c.dstTime ? 'src' : c.dstTime > c.srcTime ? 'dst' : '';
  const file = (label, path, sz, t, who) => `<div class="mc-jobs-file"><span class="l">${label}</span><span class="p">${esc(path)}</span>
    <span class="i">${esc(tt('{n} bytes', { n: bytes(sz) }))} · <span class="${newer === who ? 'newer' : ''}">${esc(date(t))}${newer === who ? ` ${esc(tt('(newer)'))}` : ''}</span></span></div>`;
  const same = c.same == null ? '' : `<p class="mc-jobs-same ${c.same ? 'eq' : 'ne'}">${esc(tt(c.same ? 'The two files have the same content.' : 'The two files are different.'))}</p>`;
  // with the undo journal on (board #13), say that this is one of the things it can't take back
  const noUndo = app.setting('undoJournal', true) === false ? '' : `<p class="mc-jobs-noundo">${esc(tt('Replacing a file can\'t be undone.'))}</p>`;
  const verb = r.p.kind === 'move' ? 'Move' : 'Copy';
  const hasMore = MORE.some(([, ch]) => offered.has(ch));
  const choice = await modal((box, done) => {
    box.classList.add('mc-jobs-overwrite');
    box.innerHTML = `<h2>${esc(tt('Replace file?'))}</h2><p class="mc-msg">${esc(tt('{name} is already in the target folder.', { name: base(c.target) }))}</p>
      ${file(esc(tt('Overwrite this file:')), c.target, c.dstSize, c.dstTime, 'dst')}${file(esc(verb === 'Move' ? tt('With this one (move):') : tt('With this one (copy):')), c.source, c.srcSize, c.srcTime, 'src')}${same}${noUndo}
      ${row(MAIN)}${row(SECOND).replace(/<\/div>$/, `${toolBtn}</div>`)}${hasMore ? `<div class="mc-btns"><button type="button" class="mc-jobs-more">${esc(tt('More options'))}</button></div><div class="mc-jobs-morebox" hidden>${row(MORE)}</div>` : ''}`;
    box.querySelectorAll('[data-choice]').forEach((b) => { b.onclick = () => done(b.dataset.choice); });
    const more = box.querySelector('.mc-jobs-more');
    if (more) more.onclick = () => { const m = box.querySelector('.mc-jobs-morebox'); m.hidden = !m.hidden; more.textContent = tt(m.hidden ? 'More options' : 'Fewer options'); };
    box.onsubmit = (ev) => { ev.preventDefault(); done('overwrite'); };
  }, 'cancel', {
    kind: 'overwrite',
    // Enter presses the focused button (Overwrite at first), also for keys sent by control mode
    onKey: (ev) => {
      if (ev.key !== 'Enter') return false;
      const b = ev.target.closest?.('.mc-jobs-overwrite button') || document.querySelector('.mc-jobs-overwrite [data-choice="overwrite"]');
      b?.click();
      return true;
    },
  });
  if (choice === 'compare_tool') {
    try { await compareTool(c.target, c.source); } catch (e) { app.flash(errText(e), 'err'); }
    return overwrite(r, c); // back to the question
  }
  if (choice === 'rename') {
    const name = base(c.target);
    const dot = name.lastIndexOf('.');
    const v = await inputDialog({ title: `${verb} under another name`, label: 'New name', value: name, select: [0, dot > 0 ? dot : name.length], kind: 'rename-target',
      validate: (t) => tt(!t.trim() ? 'Type a name.' : /[\\/]/.test(t) ? 'A name can\'t contain / or \\.' : t === name ? 'That name is taken: type another one.' : '') });
    if (v === null) return overwrite(r, c); // back to the question
    return answer(r, 'rename', v);
  }
  return answer(r, choice);
}

// ---- background indicator and transfer window ----

let indicator = null;
function paintIndicator() {
  const live = [...jobs.values()].filter((r) => r.background && !r.done && r.p && (r.claimed || Date.now() - r.seen > 300));
  if (!indicator) {
    if (!live.length) return;
    indicator = document.createElement('button');
    indicator.type = 'button';
    indicator.className = 'mc-jobs-indicator';
    indicator.title = tt('Background transfers');
    indicator.addEventListener('mousedown', (ev) => ev.preventDefault());
    indicator.onclick = () => app.run('cm_BackgroundTransferManager');
    document.body.append(indicator);
  }
  indicator.hidden = !live.length;
  if (!live.length) return;
  const asking = live.filter((r) => r.p.state === 'asking').length;
  const bytesAll = live.reduce((s, r) => s + (r.p.bytes || 0), 0), doneAll = live.reduce((s, r) => s + (r.p.bytesDone || 0), 0);
  const pct = bytesAll ? Math.round(doneAll / bytesAll * 100) : Math.round(live.reduce((s, r) => s + fraction(r.p), 0) / live.length * 100);
  indicator.textContent = asking ? `${pl(live.length, 'job')} · ${tt('waiting for an answer')}` : `${pl(live.length, 'job')} · ${pct}%`;
  indicator.classList.toggle('asking', asking > 0);
}

// skipErrorsSetting is the "Skip errors in background" option (Configuration > Copy/Delete), off by default.
function skipErrorsSetting() { return !!(app.setting('fileops', {}) || {}).skipErrorsBackground; }

function speedSetting() { const s = app.setting('fileops', {}) || {}; return { on: !!s.speedLimitOn, kb: Number(s.speedLimitKB) || 0 }; }

// backgroundSpeedLimit is the limit (bytes per second) new background and queued jobs start with.
export function backgroundSpeedLimit() { const s = speedSetting(); return s.on && s.kb > 0 ? s.kb * 1024 : 0; }

// showTransfers opens the background transfer window: every job with its state, progress and speed.
let transfersOpen = false;
const cfgOpt = (k, d) => app.config?.values?.[k] ?? d;

export async function showTransfers() {
  if (transfersOpen) return;
  transfersOpen = true;
  try { await transfersWindow(); } finally { transfersOpen = false; }
}

async function transfersWindow() {
  await refreshJobs();
  let selected = null;
  let unsub = () => {};
  const sp = speedSetting();
  await modal((box, done) => {
    box.classList.add('mc-jobs-transfers');
    box.innerHTML = `<h2>${esc(tt('Background transfers'))}</h2><p class="mc-msg"></p><div class="mc-jobs-list" role="listbox" aria-label="${esc(tt('Jobs'))}"></div>
      <div class="mc-jobs-bar total"><i></i></div><p class="mc-note mc-jobs-detail"></p>
      <div class="mc-jobs-limit"><label class="mc-check"><input type="checkbox" id="limitOn"${sp.on ? ' checked' : ''}> ${esc(tt('Speed limit'))}</label>
        <input type="text" id="limit" inputmode="numeric" value="${sp.kb || ''}" placeholder="0" aria-label="${esc(tt('Speed limit in KB per second'))}"> <span>KB/s</span></div>
      <div class="mc-btns"><button type="button" class="pause">${esc(tt('Pause'))}</button><button type="button" class="now">${esc(tt('Start now'))}</button><button type="button" class="stop">${esc(tt('Stop job'))}</button>
        <button type="button" class="errs" title="${esc(tt('The files that failed, with the reasons'))}">${esc(tt('Errors…'))}</button><button type="button" class="clear">${esc(tt('Clear finished'))}</button><button type="button" class="close primary">${esc(tt('Close'))}</button></div>`;
    const $ = (s) => box.querySelector(s);
    const list = $('.mc-jobs-list');
    const cur = () => jobs.get(selected);
    const paint = () => {
      const all = [...jobs.values()].filter((r) => r.p);
      if (!jobs.has(selected)) selected = (all.find((r) => !r.done) || all[0])?.id ?? null;
      $('.mc-msg').textContent = all.length ? tt('{jobs} at work, {done} finished.', { jobs: pl(all.filter((r) => !r.done).length, 'job'), done: all.filter((r) => r.done).length }) : tt('No transfers. Press F2 in the copy or move dialog to queue one.');
      list.innerHTML = all.map((r) => {
        const p = r.p;
        const src = r.req?.sources?.length ? (r.req.sources.length === 1 ? base(r.req.sources[0]) : pl(r.req.sources.length, 'item')) : pl(p.files || 0, 'file');
        const to = p.target ? ` → ${p.target}` : '';
        return `<div class="mc-jobs-row${r.id === selected ? ' on' : ''} st-${p.state}" data-id="${r.id}" role="option" aria-selected="${r.id === selected}">
          <span class="k">${esc(tt(KIND[p.kind] || p.kind))}${p.queued ? ` ${esc(tt('(queue)'))}` : ''}${r.ext ? ` ${esc(tt('(server)'))}` : ''}</span><span class="w" title="${esc(src + to)}">${esc(src + to)}</span>
          <span class="s">${esc(tt(STATE[p.state] || p.state))}</span><span class="pc">${Math.round(fraction(p) * 100)}%</span>
          <span class="sp">${p.state === 'running' && p.speed > 0 ? esc(size(p.speed)) + '/s' : ''}</span></div>`;
      }).join('');
      const r = cur();
      const p = r?.p;
      $('.mc-jobs-bar i').style.width = `${p ? Math.round(fraction(p) * 100) : 0}%`;
      $('.mc-jobs-detail').textContent = !p ? '' : p.summary && r.done ? p.summary + (p.errors?.length ? ` ${p.errors[0]}` : '')
        : [base(p.current), tt('{done} of {files}', { done: p.filesDone, files: pl(p.files, 'file') }), p.bytes ? tt('{done} of {size}', { done: size(p.bytesDone), size: size(p.bytes) }) : '',
          p.eta > 0 ? tt('{time} left', { time: eta(p.eta) }) : '', errorCount(p) > 0 ? pl(errorCount(p), 'error') : ''].filter(Boolean).join(' · ');
      const live = p && !r.done;
      $('.pause').textContent = tt(p?.state === 'paused' ? 'Continue' : 'Pause');
      $('.pause').disabled = !live || p.state === 'queued' || !!r.ext; // (a server transfer can only be stopped)
      $('.now').disabled = !(live && p.state === 'queued');
      $('.stop').disabled = !live;
      $('.clear').disabled = !all.some((x) => x.done);
      $('.errs').disabled = !(p && r.done && errorCount(p) > 0) || !!r.ext; // (a server transfer's errors were flashed when it ended)
    };
    list.addEventListener('mousedown', (ev) => { const row = ev.target.closest('[data-id]'); if (row) { selected = +row.dataset.id; paint(); } });
    $('.pause').onclick = () => { const r = cur(); if (r && !r.ext) (r.p.state === 'paused' ? go.Jobs.Resume(r.id) : go.Jobs.Pause(r.id)).catch(() => {}); };
    $('.now').onclick = () => { const r = cur(); if (r && !r.ext) go.Jobs.RunNow(r.id).catch(() => {}); };
    $('.stop').onclick = () => { const r = cur(); if (r) cancelJob(r.id).catch(() => {}); };
    $('.errs').onclick = () => { const r = cur(); if (r?.done && !r.ext && errorCount(r.p) > 0) showErrorList(r, r.p); };
    $('.clear').onclick = async () => {
      await go.Jobs.Forget().catch(() => {});
      for (const [id, r] of jobs) if (r.done) jobs.delete(id);
      notify();
    };
    $('.close').onclick = () => done();
    const applyLimit = () => {
      const on = $('#limitOn').checked, kb = Math.max(0, parseInt($('#limit').value, 10) || 0);
      app.setSetting('fileops', { ...(app.setting('fileops', {}) || {}), speedLimitOn: on, speedLimitKB: kb });
      const bps = on ? kb * 1024 : 0;
      for (const r of jobs.values()) if (!r.done && r.background && !r.ext) go.Jobs.SetSpeedLimit(r.id, bps).catch(() => {});
    };
    $('#limitOn').addEventListener('change', applyLimit);
    $('#limit').addEventListener('change', applyLimit);
    box.onsubmit = (ev) => { ev.preventDefault(); applyLimit(); };
    box._move = (d) => {
      const ids = [...jobs.values()].filter((r) => r.p).map((r) => r.id);
      const i = ids.indexOf(selected);
      if (ids.length) { selected = ids[Math.max(0, Math.min(ids.length - 1, i + d))]; paint(); }
    };
    unsub = onJobs(paint);
    paint();
  }, undefined, {
    kind: 'transfers',
    onKey: (ev) => {
      const box = document.querySelector('.mc-jobs-transfers');
      if (ev.target.id === 'limit') return false;
      if (ev.key === 'ArrowDown') { box?._move(1); return true; }
      if (ev.key === 'ArrowUp') { box?._move(-1); return true; }
      return false;
    },
  });
  unsub();
}

// ---- the error list of a finished job ----

// showErrorList shows the files a job could not handle, each with its path and the plain-English reason, and offers
// Retry failed (a new job with the same options, for just those files), Copy list (to the clipboard) and Save log (to
// a text file). It resolves when the list is closed.
export async function showErrorList(r, p = r.p) {
  const failures = failuresOf(p);
  const { requests, skipped } = retryRequests(r.req, failures);
  const count = errorCount(p);
  const choice = await modal((box, done) => {
    box.classList.add('mc-jobs-errors');
    const rows = failures.map((f) => {
      const path = f.source || f.target;
      return path ? `<li><span class="p">${esc(path)}</span><span class="r">${esc(f.reason || f.message)}</span></li>` : `<li><span class="r">${esc(f.message)}</span></li>`;
    }).join('');
    const more = count - failures.length;
    box.innerHTML = `<h2>${esc(`${tt(KIND[p.kind] || 'Job')}: ${pl(count, 'error')}`)}</h2>
      <p class="mc-msg">${esc(p.summary || '')}</p>
      <ul class="mc-jobs-errlist" aria-label="${esc(tt('Failed files'))}">${rows}</ul>
      ${more > 0 ? `<p class="mc-note">${esc(tt('…and {n} more errors that were not listed.', { n: more }))}</p>` : ''}
      ${anyDenied(failures) ? `<p class="mc-note mc-jobs-denied">${esc(deniedHint(app.platform))}</p>` : ''}
      ${requests.length && skipped ? `<p class="mc-note">${esc(tt('{errors} can\'t be retried by itself.', { errors: pl(skipped, 'error') }))}</p>` : ''}
      <p class="mc-note mc-jobs-errstate" aria-live="polite"></p>
      <div class="mc-btns"><button type="button" class="retry"${requests.length ? '' : ' disabled'}>${esc(tt('Retry failed'))}</button><button type="button" class="copy">${esc(tt('Copy list'))}</button>
        <button type="button" class="save">${esc(tt('Save log'))}</button><button type="button" class="close primary">${esc(tt('Close'))}</button></div>`;
    const $ = (sel) => box.querySelector(sel);
    const say = (t) => { $('.mc-jobs-errstate').textContent = t; };
    $('.retry').onclick = () => done('retry');
    $('.close').onclick = () => done('close');
    $('.copy').onclick = async () => say(await setClipboardText(listText(failures)) ? tt('Copied {lines}.', { lines: pl(failures.length, 'line') }) : tt('The clipboard can\'t be written.'));
    $('.save').onclick = async () => {
      try {
        const path = await go.Jobs.SaveErrorLog(logName(p), logText(p, failures));
        if (path) say(tt('Saved to {path}.', { path }));
      } catch (e) { say(errText(e)); }
    };
    box.onsubmit = (ev) => { ev.preventDefault(); done('close'); };
  }, 'close', { kind: 'job-errors' });
  if (choice !== 'retry') return;
  for (const req of requests) {
    runJob(req, { title: r.title, background: r.background }).catch((e) => app.flash(errText(e), 'err'));
  }
}
