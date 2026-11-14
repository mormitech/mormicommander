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
// "rename-target" and "transfers".

import './jobs.css';
import { go, on, errText } from './go.js';
import { modal, messageDialog, inputDialog, esc } from './dialogs.js';
import { size, date, bytes, plural } from './format.js';
import { track } from './busy.js';
import { app } from './app.js';
import { holdProgress, progressHeld, showWhenFree } from './progresshold.js';
import { ERROR_BUTTONS, offeredButtons } from './jobchoices.js';

// Job engines of features (archives and servers: features/vfs.js) offer themselves here: start(req) resolves with a
// job number, or 0 when the job is not theirs (then it goes to Go's Jobs); cancel(id) and answer(id, a) serve their jobs.
export const jobBackends = [];
const backendOf = new Map(); // job number -> backend
const cancelJob = (id) => (backendOf.get(id) ? backendOf.get(id).cancel(id) : go.Jobs.Cancel(id));

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
  if (final.state === 'failed' && final.errors?.length) {
    app.flash(final.errors.slice(0, 3).join('\n') + (final.errors.length > 3 ? `\n…and ${final.errors.length - 3} more.` : ''), 'err');
  } else if (r.background && final.state === 'done' && final.summary) {
    app.flash(final.summary);
  } else if (final.state === 'cancelled' && r.background) {
    app.flash(`${KIND[final.kind] || 'Job'} stopped.`);
  }
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
        <div class="mc-jobs-bar cur" title="This file"><i></i></div><div class="mc-jobs-bar total" title="All files"><i></i></div>
        <p class="mc-note"></p>
        <div class="mc-btns"><button type="button" class="bg" title="Go on in the background (F2)">Background</button><button type="button" class="pause">Pause</button><button type="button" class="cancel primary">Cancel</button></div>`;
      const $ = (s) => b.querySelector(s);
      ui.paint = () => {
        const p = r.p;
        const pct = Math.round(fraction(p) * 100);
        $('h2').textContent = `${pct}% ${r.title}`;
        $('.mc-msg').textContent = p.state === 'paused' ? `Paused: ${base(p.current)}` : base(p.current) || (p.files ? '' : 'Counting files…');
        const to = r.req?.target && (p.kind === 'copy' || p.kind === 'move') ? ` → ${r.req.target}` : '';
        $('.mc-jobs-path').textContent = p.current ? `${p.current}${to}` : '';
        const curBar = $('.mc-jobs-bar.cur');
        curBar.hidden = !(p.currentSize > 0);
        curBar.firstElementChild.style.width = `${p.currentSize > 0 ? Math.round(Math.min(1, p.currentDone / p.currentSize) * 100) : 0}%`;
        $('.mc-jobs-bar.total i').style.width = `${pct}%`;
        const parts = [`${p.filesDone} of ${plural(p.files, 'file')}`];
        if (p.bytes) parts.push(`${size(p.bytesDone)} of ${size(p.bytes)}`);
        if (p.speed > 0 && p.state === 'running') parts.push(`${size(p.speed)}/s`);
        if (p.eta > 0 && p.state === 'running') parts.push(`${eta(p.eta)} left`);
        $('.mc-note').textContent = parts.join(' · ');
        $('.pause').textContent = p.state === 'paused' ? 'Continue' : 'Pause';
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
      if (v === 'background' && !r.done) { r.background = true; r.ui = null; notify(); }
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
      return answer(r, await pick(r, { Title: 'Read-only file', Message: `${c.target} is read-only.\n\nOverwrite it anyway?`, Kind: 'overwrite-readonly', DefaultButton: 'Skip' },
        [['Overwrite', 'overwrite'], ['Overwrite all', 'overwrite_all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']]));
    case 'error':
      return answer(r, await pick(r, { Title: 'Problem', Message: c.error || `Something went wrong with ${name}.`, Kind: 'job-error', DefaultButton: 'Retry' },
        ERROR_BUTTONS));
    case 'no_space':
      return answer(r, await pick(r, { Title: 'Not enough space', Message: `${c.target} has ${size(c.free)} free, but this needs ${size(c.need)}.\n\nStart anyway?`, Kind: 'no-space', DefaultButton: 'Cancel' },
        [['Start anyway', 'yes'], ['Cancel', 'cancel']]));
    case 'delete_dir': {
      const verb = r.p.kind === 'wipe' ? 'Wipe' : 'Delete';
      return answer(r, await pick(r, { Title: 'Folder is not empty', Message: `The folder ${c.source} is not empty.\n\n${verb} it with everything in it?`, Kind: 'delete-folder', DefaultButton: 'Yes', Danger: true },
        [['Yes', 'yes'], ['All', 'all'], ['Skip', 'skip'], ['Skip all', 'skip_all'], ['Cancel', 'cancel']]));
    }
    case 'delete_readonly':
      return answer(r, await pick(r, { Title: 'Read-only file', Message: `${c.source} is read-only.\n\nDelete it anyway?`, Kind: 'delete-readonly', DefaultButton: 'Yes', Danger: true },
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

async function overwrite(r, c) {
  const offered = new Set(c.choices || []);
  const row = (list, cls = '') => `<div class="mc-btns ${cls}">${list.filter(([, ch]) => offered.has(ch))
    .map(([l, ch]) => `<button type="button" data-choice="${ch}"${ch === 'overwrite' ? ' class="primary"' : ''}>${esc(l)}</button>`).join('')}</div>`;
  const newer = c.srcTime > c.dstTime ? 'src' : c.dstTime > c.srcTime ? 'dst' : '';
  const file = (label, path, sz, t, who) => `<div class="mc-jobs-file"><span class="l">${label}</span><span class="p">${esc(path)}</span>
    <span class="i">${esc(bytes(sz))} bytes · <span class="${newer === who ? 'newer' : ''}">${esc(date(t))}${newer === who ? ' (newer)' : ''}</span></span></div>`;
  const same = c.same == null ? '' : `<p class="mc-jobs-same ${c.same ? 'eq' : 'ne'}">${c.same ? 'The two files have the same content.' : 'The two files are different.'}</p>`;
  // with the undo journal on (board #13), say that this is one of the things it can't take back
  const noUndo = app.setting('undoJournal', true) === false ? '' : '<p class="mc-jobs-noundo">Replacing a file can\'t be undone.</p>';
  const verb = r.p.kind === 'move' ? 'Move' : 'Copy';
  const hasMore = MORE.some(([, ch]) => offered.has(ch));
  const choice = await modal((box, done) => {
    box.classList.add('mc-jobs-overwrite');
    box.innerHTML = `<h2>Replace file?</h2><p class="mc-msg">${esc(base(c.target))} is already in the target folder.</p>
      ${file('Overwrite this file:', c.target, c.dstSize, c.dstTime, 'dst')}${file(`With this one (${verb.toLowerCase()}):`, c.source, c.srcSize, c.srcTime, 'src')}${same}${noUndo}
      ${row(MAIN)}${row(SECOND)}${hasMore ? `<div class="mc-btns"><button type="button" class="mc-jobs-more">More options</button></div><div class="mc-jobs-morebox" hidden>${row(MORE)}</div>` : ''}`;
    box.querySelectorAll('[data-choice]').forEach((b) => { b.onclick = () => done(b.dataset.choice); });
    const more = box.querySelector('.mc-jobs-more');
    if (more) more.onclick = () => { const m = box.querySelector('.mc-jobs-morebox'); m.hidden = !m.hidden; more.textContent = m.hidden ? 'More options' : 'Fewer options'; };
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
  if (choice === 'rename') {
    const name = base(c.target);
    const dot = name.lastIndexOf('.');
    const v = await inputDialog({ title: `${verb} under another name`, label: 'New name', value: name, select: [0, dot > 0 ? dot : name.length], kind: 'rename-target',
      validate: (t) => (!t.trim() ? 'Type a name.' : /[\\/]/.test(t) ? 'A name can\'t contain / or \\.' : t === name ? 'That name is taken: type another one.' : '') });
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
    indicator.title = 'Background transfers';
    indicator.addEventListener('mousedown', (ev) => ev.preventDefault());
    indicator.onclick = () => app.run('cm_BackgroundTransferManager');
    document.body.append(indicator);
  }
  indicator.hidden = !live.length;
  if (!live.length) return;
  const asking = live.filter((r) => r.p.state === 'asking').length;
  const bytesAll = live.reduce((s, r) => s + (r.p.bytes || 0), 0), doneAll = live.reduce((s, r) => s + (r.p.bytesDone || 0), 0);
  const pct = bytesAll ? Math.round(doneAll / bytesAll * 100) : Math.round(live.reduce((s, r) => s + fraction(r.p), 0) / live.length * 100);
  indicator.textContent = asking ? `${plural(live.length, 'job')} · waiting for an answer` : `${plural(live.length, 'job')} · ${pct}%`;
  indicator.classList.toggle('asking', asking > 0);
}

function speedSetting() { const s = app.setting('fileops', {}) || {}; return { on: !!s.speedLimitOn, kb: Number(s.speedLimitKB) || 0 }; }

// backgroundSpeedLimit is the limit (bytes per second) new background and queued jobs start with.
export function backgroundSpeedLimit() { const s = speedSetting(); return s.on && s.kb > 0 ? s.kb * 1024 : 0; }

// showTransfers opens the background transfer window: every job with its state, progress and speed.
export async function showTransfers() {
  await refreshJobs();
  let selected = null;
  let unsub = () => {};
  const sp = speedSetting();
  await modal((box, done) => {
    box.classList.add('mc-jobs-transfers');
    box.innerHTML = `<h2>Background transfers</h2><p class="mc-msg"></p><div class="mc-jobs-list" role="listbox" aria-label="Jobs"></div>
      <div class="mc-jobs-bar total"><i></i></div><p class="mc-note mc-jobs-detail"></p>
      <div class="mc-jobs-limit"><label class="mc-check"><input type="checkbox" id="limitOn"${sp.on ? ' checked' : ''}> Speed limit</label>
        <input type="text" id="limit" inputmode="numeric" value="${sp.kb || ''}" placeholder="0" aria-label="Speed limit in KB per second"> <span>KB/s</span></div>
      <div class="mc-btns"><button type="button" class="pause">Pause</button><button type="button" class="now">Start now</button><button type="button" class="stop">Stop job</button>
        <button type="button" class="clear">Clear finished</button><button type="button" class="close primary">Close</button></div>`;
    const $ = (s) => box.querySelector(s);
    const list = $('.mc-jobs-list');
    const cur = () => jobs.get(selected);
    const paint = () => {
      const all = [...jobs.values()].filter((r) => r.p);
      if (!jobs.has(selected)) selected = (all.find((r) => !r.done) || all[0])?.id ?? null;
      $('.mc-msg').textContent = all.length ? `${plural(all.filter((r) => !r.done).length, 'job')} at work, ${all.filter((r) => r.done).length} finished.` : 'No transfers. Press F2 in the copy or move dialog to queue one.';
      list.innerHTML = all.map((r) => {
        const p = r.p;
        const src = r.req?.sources?.length ? (r.req.sources.length === 1 ? base(r.req.sources[0]) : plural(r.req.sources.length, 'item')) : plural(p.files || 0, 'file');
        const to = p.target ? ` → ${p.target}` : '';
        return `<div class="mc-jobs-row${r.id === selected ? ' on' : ''} st-${p.state}" data-id="${r.id}" role="option" aria-selected="${r.id === selected}">
          <span class="k">${esc(KIND[p.kind] || p.kind)}${p.queued ? ' (queue)' : ''}</span><span class="w" title="${esc(src + to)}">${esc(src + to)}</span>
          <span class="s">${esc(STATE[p.state] || p.state)}</span><span class="pc">${Math.round(fraction(p) * 100)}%</span>
          <span class="sp">${p.state === 'running' && p.speed > 0 ? esc(size(p.speed)) + '/s' : ''}</span></div>`;
      }).join('');
      const r = cur();
      const p = r?.p;
      $('.mc-jobs-bar i').style.width = `${p ? Math.round(fraction(p) * 100) : 0}%`;
      $('.mc-jobs-detail').textContent = !p ? '' : p.summary && r.done ? p.summary + (p.errors?.length ? ` ${p.errors[0]}` : '')
        : [base(p.current), `${p.filesDone} of ${plural(p.files, 'file')}`, p.bytes ? `${size(p.bytesDone)} of ${size(p.bytes)}` : '', p.eta > 0 ? `${eta(p.eta)} left` : ''].filter(Boolean).join(' · ');
      const live = p && !r.done;
      $('.pause').textContent = p?.state === 'paused' ? 'Continue' : 'Pause';
      $('.pause').disabled = !live || p.state === 'queued';
      $('.now').disabled = !(live && p.state === 'queued');
      $('.stop').disabled = !live;
      $('.clear').disabled = !all.some((x) => x.done);
    };
    list.addEventListener('mousedown', (ev) => { const row = ev.target.closest('[data-id]'); if (row) { selected = +row.dataset.id; paint(); } });
    $('.pause').onclick = () => { const r = cur(); if (r) (r.p.state === 'paused' ? go.Jobs.Resume(r.id) : go.Jobs.Pause(r.id)).catch(() => {}); };
    $('.now').onclick = () => { const r = cur(); if (r) go.Jobs.RunNow(r.id).catch(() => {}); };
    $('.stop').onclick = () => { const r = cur(); if (r) cancelJob(r.id).catch(() => {}); };
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
      for (const r of jobs.values()) if (!r.done && r.background) go.Jobs.SetSpeedLimit(r.id, bps).catch(() => {});
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
