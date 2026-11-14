// The Multi-Rename tool (Ctrl+M, cm_MultiRenameFiles, FEATURES.md section 22, behaviour from docs/spec/area-6.md
// section 1): a big modal window with the name and extension masks and their placeholder buttons, search & replace,
// the case drop-down, the counter, a live preview of every new name, saved settings (F2), names from a file, the
// clipboard or the editor (F3), chained steps (F5), Start! with conflict reporting, and Undo. All the renaming logic is
// internal/multirename behind feature_multirename.go (go.MultiRename.*); this file only collects the input and draws
// the result. The window is a page dialog (kind "multirename"), so control mode reads its fields and presses its
// buttons; the "multirename" control command returns the preview rows.

import './multirename.css';
import { command } from '../core/registry.js';
import { modal, messageDialog, inputDialog, formDialog, infoDialog, esc } from '../core/dialogs.js';
import { go, on, errText } from '../core/go.js';
import { addControlCommand } from '../core/control.js';
import { size as fmtSize, date as fmtDate, plural } from '../core/format.js';
import { fieldMenuItems } from './multirename-lib.js';
import { isBranchPath } from './branch-lib.js';

const SETTING = 'multiRename';
const HISTORY_MAX = 20;

// The placeholder menus. Every item inserts its text at the caret of the mask field.
const MORE = [
  ['[P]', 'Parent folder'], ['[G]', 'Grandparent folder'], '-',
  ['[d]', 'Date'], ['[t]', 'Time'], ['[Y]', 'Year (4 digits)'], ['[y]', 'Year (2 digits)'], ['[M]', 'Month'],
  ['[D]', 'Day'], ['[h]', 'Hour'], ['[m]', 'Minute'], ['[s]', 'Second'], '-',
  ['[U]', 'Uppercase from here'], ['[L]', 'Lowercase from here'], ['[F]', 'First letter of each word uppercase from here'],
  ['[n]', 'Original case from here'], '-',
  ['[C:a]', 'Counter with automatic width'], ['[Ca]', 'Letter counter (a, b, c…)'], ['[C10+5:3]', 'Counter: start 10, step 5, 3 digits'], '-',
  ['[N1]', 'First character of the name'], ['[N-1]', 'Last character of the name'], ['[N2-]', 'Name from the 2nd character'],
  ['[[]', 'Literal ['], ['[]]', 'Literal ]'],
];

const CASES = [['', 'Unchanged'], ['lower', 'lowercase'], ['upper', 'UPPERCASE'], ['first', 'First letter uppercase'], ['words', 'First letter of each word uppercase']];

let cur = null; // the open window's state, for control mode

// ---- settings <-> form ----

function readForm(box) {
  const v = (id) => box.querySelector(`#${id}`).value;
  const c = (id) => box.querySelector(`#${id}`).checked;
  const int = (id, def) => { const n = parseInt(v(id), 10); return Number.isFinite(n) ? n : def; };
  return {
    nameMask: v('mr-name'), extMask: v('mr-ext'), search: v('mr-search'), replace: v('mr-replace'),
    caseSensitive: c('mr-case'), onlyFirst: c('mr-first'), includeExt: c('mr-inext'), regex: c('mr-regex'), subst: c('mr-subst'),
    case: v('mr-casemode'), counterStart: int('mr-cstart', 1), counterStep: int('mr-cstep', 1), counterDigits: int('mr-cdigits', 1),
    counterResetPerFolder: c('mr-creset'), removeAccents: c('mr-accents'), splitDirExt: c('mr-dirext'),
  };
}

function writeForm(box, s) {
  const set = (id, val) => { const el = box.querySelector(`#${id}`); if (el.type === 'checkbox') el.checked = !!val; else el.value = val ?? ''; };
  set('mr-name', s.nameMask ?? '[N]'); set('mr-ext', s.extMask ?? '[E]'); set('mr-search', s.search); set('mr-replace', s.replace);
  set('mr-case', s.caseSensitive); set('mr-first', s.onlyFirst); set('mr-inext', s.includeExt); set('mr-regex', s.regex); set('mr-subst', s.subst);
  set('mr-casemode', s.case || ''); set('mr-cstart', s.counterStart ?? 1); set('mr-cstep', s.counterStep ?? 1);
  set('mr-cdigits', String(s.counterDigits ?? 1)); set('mr-creset', s.counterResetPerFolder); set('mr-accents', s.removeAccents);
  set('mr-dirext', s.splitDirExt);
  box.querySelector('#mr-subst').disabled = !s.regex;
}

// engineSettings turns the form into internal/multirename's Settings. "Subst." (RegEx only) replaces the whole name
// with the replace text: the pattern is widened to match the whole name, so $1… still refer to the user's groups.
function engineSettings(f) {
  const { subst, ...s } = f;
  if (f.regex && subst && f.search) s.search = `^.*?(?:${f.search}).*$`;
  return s;
}

// ---- small helpers ----

function insertAt(input, text) {
  const a = input.selectionStart ?? input.value.length, b = input.selectionEnd ?? a;
  input.focus();
  input.setRangeText(text, a, b, 'end');
  input.dispatchEvent(new Event('input', { bubbles: true }));
}

function pushHistory(hist, key, value) {
  if (!value) return;
  const list = (hist[key] || []).filter((x) => x !== value);
  list.unshift(value);
  hist[key] = list.slice(0, HISTORY_MAX);
}

const splitName = (name, isDir) => {
  if (isDir) return [name, ''];
  const i = name.lastIndexOf('.');
  return i > 0 ? [name.slice(0, i), name.slice(i + 1)] : [name, ''];
};

// ---- the window ----

async function openTool(app) {
  const p = app.cur;
  const targets = p.targets();
  if (!targets.length) return;
  // A branch view (branch.js) lists real files of many folders, named by their path relative to the folder it lists
  // (its ".."): Go renames each file in its own folder, so [P] and [G] and the conflict check are per folder (F-511).
  const branch = isBranchPath(p.path);
  if (p.path.includes('://') && !branch) { app.flash('Files in archives and on servers can\'t be renamed with the Multi-Rename tool.', 'err'); return; }
  let start;
  try {
    start = await go.MultiRename.Start(branch ? p.parent : p.path, targets.map((e) => e.name));
  } catch (e) {
    await messageDialog({ Title: 'Multi-Rename tool', Message: errText(e), Kind: 'multirename-error' });
    return;
  }
  const saved = app.setting(SETTING, {}) || {};
  const hist = { ...(saved.history || {}) };
  const st = {
    app, box: null, rows: start.rows, defaults: start.defaults, fields: start.fields || [], media: start.media || [], presets: start.presets || [],
    undo: start.undo, preview: null, sel: new Set([0]), focus: 0, seq: 0, timer: 0, stale: false, busy: false, renamed: 0,
    preset: '', status: '', sort: { key: '', desc: false }, menu: null,
  };
  cur = st;

  // Keys the window handles itself. The listener goes in before the modal's own, which stops every key from going
  // further while the dialog is on top.
  const keys = (ev) => onKey(st, ev);
  window.addEventListener('keydown', keys, true);
  const offProgress = on('multirename-progress', (pr) => { if (st.busy) setStatus(st, `Renaming… ${pr.done} of ${pr.total}`); });
  try {
    await modal((box, done) => build(st, box, done, saved, hist), null, { kind: 'multirename' });
  } finally {
    window.removeEventListener('keydown', keys, true);
    offProgress?.();
    clearTimeout(st.timer);
    const f = st.box ? readForm(st.box) : null;
    const next = { ...saved, history: st.histNow || hist };
    if (f) next.last = f;
    if (st.size) { next.width = st.size[0]; next.height = st.size[1]; }
    app.setSetting(SETTING, next);
    go.MultiRename.Close().catch(() => {});
    cur = null;
    // the panels show the new names
    for (const panel of app.panels) panel.reload();
  }
}

function build(st, box, done, saved, hist) {
  st.box = box;
  st.done = done;
  box.classList.add('mc-multirename');
  if (saved.width > 400) box.style.width = `${Math.min(saved.width, innerWidth * 0.98)}px`;
  if (saved.height > 300) box.style.height = `${Math.min(saved.height, innerHeight * 0.96)}px`;
  // remember the size the user gives the window (read while it is on screen; it is gone when the promise ends)
  new ResizeObserver(() => { if (box.isConnected && box.offsetWidth) st.size = [box.offsetWidth, box.offsetHeight]; }).observe(box);
  const dl = (id, key) => `<datalist id="${id}">${(hist[key] || []).map((x) => `<option value="${esc(x)}">`).join('')}</datalist>`;
  const btn = (cls, text, title, data = '') => `<button type="button" class="${cls}" title="${esc(title)}" ${data}>${esc(text)}</button>`;
  box.innerHTML = `<h2>Multi-Rename tool</h2>
  <div class="mc-multirename-top">
    <fieldset class="mc-multirename-g mc-multirename-gname"><legend>Rename mask: file name</legend>
      <input id="mr-name" type="text" spellcheck="false" list="mr-hist-name" autofocus aria-label="Rename mask: file name">${dl('mr-hist-name', 'nameMask')}
      <div class="mc-multirename-ph" data-target="mr-name">
        ${btn('', '[N] Name', 'The old name', 'data-ins="[N]"')}${btn('', '[N#-#] Range…', 'Part of the old name', 'data-range="N"')}
        ${btn('', '[C] Counter', 'The counter', 'data-ins="[C]"')}${btn('', '[YMD] Date', 'Year, month and day of the file date', 'data-ins="[Y][M][D]"')}
        ${btn('', '[hms] Time', 'Hour, minute and second of the file time', 'data-ins="[h][m][s]"')}${btn('', '[=?] Field…', 'A field such as the size, the week day, the photo date (EXIF) or an audio tag', 'data-fields="1"')}
        ${btn('', '+ More…', 'All other placeholders', 'data-more="1"')}
      </div>
      <p class="mc-multirename-ferr" data-for="nameMask" hidden></p>
    </fieldset>
    <fieldset class="mc-multirename-g mc-multirename-gext"><legend>Extension</legend>
      <input id="mr-ext" type="text" spellcheck="false" list="mr-hist-ext" aria-label="Extension mask">${dl('mr-hist-ext', 'extMask')}
      <div class="mc-multirename-ph" data-target="mr-ext">
        ${btn('', '[E] Extension', 'The old extension', 'data-ins="[E]"')}${btn('', '[E#-#] Range…', 'Part of the old extension', 'data-range="E"')}
        ${btn('', '[C] Counter', 'The counter', 'data-ins="[C]"')}${btn('', '[=?] Field…', 'A field such as the size, the week day, the photo date (EXIF) or an audio tag', 'data-fields="1"')}
      </div>
      <p class="mc-multirename-ferr" data-for="extMask" hidden></p>
    </fieldset>
    <fieldset class="mc-multirename-g mc-multirename-gsearch"><legend>Search &amp; replace</legend>
      <label class="mc-multirename-l">Search for<input id="mr-search" type="text" spellcheck="false" list="mr-hist-search"></label>${dl('mr-hist-search', 'search')}
      <label class="mc-multirename-l">Replace with<input id="mr-replace" type="text" spellcheck="false" list="mr-hist-replace"></label>${dl('mr-hist-replace', 'replace')}
      <div class="mc-multirename-checks">
        <label title="Case sensitive"><input id="mr-case" type="checkbox">^Aa</label>
        <label title="Replace only the first match"><input id="mr-first" type="checkbox">1x</label>
        <label title="Search the extension too"><input id="mr-inext" type="checkbox">[E]</label>
        <label title="Regular expressions; $1, $2… in the replacement"><input id="mr-regex" type="checkbox">RegEx</label>
        <label title="With RegEx: replace the whole name with the replacement"><input id="mr-subst" type="checkbox">Subst.</label>
      </div>
      <p class="mc-multirename-ferr" data-for="search" hidden></p>
    </fieldset>
    <div class="mc-multirename-col">
      <fieldset class="mc-multirename-g"><legend>Upper/lowercase</legend>
        <select id="mr-casemode" aria-label="Upper/lowercase">${CASES.map(([v, l]) => `<option value="${v}">${esc(l)}</option>`).join('')}</select>
        <label class="mc-multirename-c" title="árvíztűrő becomes arvizturo"><input id="mr-accents" type="checkbox">Remove accents</label>
      </fieldset>
      <fieldset class="mc-multirename-g mc-multirename-gcount"><legend>Define counter [C]</legend>
        <label class="mc-multirename-n">Start at<input id="mr-cstart" type="number" step="1"></label>
        <label class="mc-multirename-n">Step by<input id="mr-cstep" type="number" step="1"></label>
        <label class="mc-multirename-n">Digits<select id="mr-cdigits"><option value="-1">Auto</option>${[1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map((n) => `<option>${n}</option>`).join('')}</select></label>
        <label class="mc-multirename-c"><input id="mr-creset" type="checkbox">Restart in each folder</label>
        <label class="mc-multirename-c" title="Treat the part after the last dot of a folder name as its extension"><input id="mr-dirext" type="checkbox">Folders have extensions</label>
      </fieldset>
    </div>
  </div>
  <div class="mc-multirename-warn" hidden></div>
  <div class="mc-multirename-list" tabindex="0" aria-label="Files to rename"><table><thead><tr>
    <th data-sort="name">Old name</th><th data-sort="ext">Ext</th><th>New name</th><th data-sort="size" class="num">Size</th><th data-sort="date">Date</th><th data-sort="path" class="mc-multirename-path">Path</th>
  </tr></thead><tbody></tbody></table></div>
  <div class="mc-multirename-status" aria-live="polite"></div>
  <div class="mc-btns mc-multirename-btns">
    <button type="button" class="mc-multirename-menu-btn" title="F2">Load/save settings…</button>
    <button type="button" class="mc-multirename-edit" title="F3: edit the new names in the editor">Edit names…</button>
    <button type="button" class="mc-multirename-next" title="F5: take the new names as the start of another step">Next step</button>
    <span class="mc-multirename-gap"></span>
    <button type="button" class="mc-multirename-undo" title="Give the files of the last rename their old names back">Undo</button>
    <button type="button" class="mc-multirename-resbtn">Result list…</button>
    <button type="submit" class="primary mc-multirename-start" title="Enter">Start!</button>
    <button type="button" class="mc-multirename-close" title="Esc">Close</button>
  </div>`;

  writeForm(box, { ...st.defaults, ...(saved.last || {}) });
  const list = box.querySelector('.mc-multirename-list');

  box.addEventListener('input', (ev) => {
    if (ev.target.id === 'mr-regex') box.querySelector('#mr-subst').disabled = !ev.target.checked;
    if (ev.target.closest('.mc-multirename-top')) { st.stale = false; schedule(st); }
  });
  box.addEventListener('change', (ev) => { if (ev.target.closest('.mc-multirename-top')) { st.stale = false; schedule(st); } });
  box.onsubmit = (ev) => { ev.preventDefault(); startRename(st); };
  box.querySelector('.mc-multirename-close').onclick = () => done(null);
  box.querySelector('.mc-multirename-menu-btn').onclick = (ev) => settingsMenu(st, ev.currentTarget);
  box.querySelector('.mc-multirename-edit').onclick = () => editNames(st);
  box.querySelector('.mc-multirename-next').onclick = () => nextStep(st);
  box.querySelector('.mc-multirename-undo').onclick = () => undo(st);
  box.querySelector('.mc-multirename-resbtn').onclick = () => resultList(st);

  // placeholder buttons
  box.querySelectorAll('.mc-multirename-ph').forEach((ph) => {
    const target = () => box.querySelector(`#${ph.dataset.target}`);
    ph.addEventListener('mousedown', (ev) => { if (ev.target.closest('button')) ev.preventDefault(); }); // keep the caret
    ph.addEventListener('click', (ev) => {
      const b = ev.target.closest('button');
      if (!b) return;
      if (b.dataset.ins) insertAt(target(), b.dataset.ins);
      else if (b.dataset.range) rangePicker(st, target(), b.dataset.range);
      else if (b.dataset.fields) showMenu(st, b, fieldMenuItems(st.fields, st.media).map((m) => m.sep ? m : { label: m.label, detail: m.detail, run: () => insertAt(target(), m.text) }), 'fields');
      else if (b.dataset.more) showMenu(st, b, MORE.map((m) => m === '-' ? { sep: true } : { label: `${m[0]} ${m[1]}`, run: () => insertAt(target(), m[0]) }), 'more');
    });
  });

  // the list: click to select, headers sort, rows drag to reorder
  list.querySelector('thead').addEventListener('click', (ev) => {
    const th = ev.target.closest('th[data-sort]');
    if (th) sortBy(st, th.dataset.sort);
  });
  const tbody = list.querySelector('tbody');
  tbody.addEventListener('mousedown', (ev) => {
    const tr = ev.target.closest('tr');
    if (!tr) return;
    const i = +tr.dataset.i;
    if (ev.shiftKey) { st.sel = new Set(range(Math.min(st.focus, i), Math.max(st.focus, i))); }
    else if (ev.ctrlKey || ev.metaKey) { if (st.sel.has(i)) st.sel.delete(i); else st.sel.add(i); st.focus = i; }
    else if (!st.sel.has(i)) { st.sel = new Set([i]); st.focus = i; }
    else st.focus = i;
    paintSel(st);
  });
  tbody.addEventListener('dragstart', (ev) => {
    const tr = ev.target.closest('tr');
    if (!tr) return;
    const i = +tr.dataset.i;
    if (!st.sel.has(i)) { st.sel = new Set([i]); st.focus = i; paintSel(st); }
    ev.dataTransfer.effectAllowed = 'move';
    ev.dataTransfer.setData('text/plain', 'mc-multirename');
  });
  tbody.addEventListener('dragover', (ev) => {
    const tr = ev.target.closest('tr');
    if (!tr) return;
    ev.preventDefault();
    const r = tr.getBoundingClientRect();
    tbody.querySelectorAll('.drop-before, .drop-after').forEach((x) => x.classList.remove('drop-before', 'drop-after'));
    tr.classList.add(ev.clientY < r.top + r.height / 2 ? 'drop-before' : 'drop-after');
  });
  tbody.addEventListener('dragleave', (ev) => { if (!tbody.contains(ev.relatedTarget)) tbody.querySelectorAll('.drop-before, .drop-after').forEach((x) => x.classList.remove('drop-before', 'drop-after')); });
  tbody.addEventListener('drop', (ev) => {
    const tr = ev.target.closest('tr');
    tbody.querySelectorAll('.drop-before, .drop-after').forEach((x) => x.classList.remove('drop-before', 'drop-after'));
    if (!tr) return;
    ev.preventDefault();
    const r = tr.getBoundingClientRect();
    const at = +tr.dataset.i + (ev.clientY < r.top + r.height / 2 ? 0 : 1);
    moveSelectedTo(st, at);
  });

  paintUndo(st);
  render(st);
  refresh(st);
}

const range = (a, b) => Array.from({ length: b - a + 1 }, (_, k) => a + k);

// ---- preview ----

function schedule(st) {
  clearTimeout(st.timer);
  st.timer = setTimeout(() => refresh(st), 40);
}

async function refresh(st) {
  if (!st.box) return null;
  const seq = ++st.seq;
  const s = st.stale ? st.defaults : engineSettings(readForm(st.box));
  const pv = await go.MultiRename.Preview(s).catch((e) => ({ error: errText(e) }));
  if (seq !== st.seq || !st.box.isConnected) return pv;
  if (pv.error) { setStatus(st, pv.error); return pv; }
  st.preview = pv;
  st.rows = pv.rows;
  render(st);
  return pv;
}

function render(st) {
  const box = st.box;
  const pv = st.preview;
  const rows = st.rows || [];
  const multi = new Set(rows.map((r) => r.dir)).size > 1;
  box.classList.toggle('mc-multirename-multi', multi);
  const html = rows.map((r, i) => {
    const bad = r.problems?.length;
    const warn = !bad && r.warnings?.length;
    const changed = r.new !== r.old;
    const tip = bad ? r.problems.map((x) => x.detail).join('\n') : warn ? r.warnings.join('\n') : '';
    const [, ext] = splitName(r.old, r.isDir);
    return `<tr data-i="${i}" draggable="true" class="${bad ? 'bad' : warn ? 'warn' : ''}${changed ? ' changed' : ''}"${tip ? ` title="${esc(tip)}"` : ''}>`
      + `<td>${r.isDir ? '<span class="mc-multirename-dir">▸</span>' : ''}${esc(r.old)}</td><td>${esc(r.isDir ? '' : ext)}</td>`
      + `<td class="new">${esc(r.new)}</td><td class="num">${r.isDir ? '&lt;DIR&gt;' : esc(fmtSize(r.size))}</td><td>${esc(fmtDate(r.time))}</td>`
      + `<td class="mc-multirename-path">${esc(r.dir)}</td></tr>`;
  }).join('');
  box.querySelector('tbody').innerHTML = html;
  box.querySelectorAll('th[data-sort]').forEach((th) => {
    th.classList.toggle('sorted', th.dataset.sort === st.sort.key);
    th.classList.toggle('desc', th.dataset.sort === st.sort.key && st.sort.desc);
  });
  paintSel(st, false);

  // field errors (a mask or the regex can't be read)
  box.querySelectorAll('.mc-multirename-ferr').forEach((p) => {
    const on = pv && pv.field === p.dataset.for;
    p.hidden = !on;
    p.textContent = on ? pv.fieldError : '';
  });
  box.querySelectorAll('#mr-name, #mr-ext, #mr-search').forEach((el) => el.classList.remove('mc-multirename-badfield'));
  if (pv?.field) box.querySelector({ nameMask: '#mr-name', extMask: '#mr-ext', search: '#mr-search' }[pv.field])?.classList.add('mc-multirename-badfield');
  if (pv && !pv.field && pv.fieldError) setStatus(st, pv.fieldError);

  // warnings banner: loaded names, steps, list-wide warnings
  const notes = [];
  if (pv?.names >= 0) notes.push(`Loaded names (${plural(pv.names, 'line')}) replace the masks. "Default" in Load/save settings forgets them.`);
  if (pv?.steps) notes.push('Chained steps: the old names shown are the result of the steps before. "Default" starts over.');
  for (const w of pv?.warnings || []) notes.push(w);
  const warn = box.querySelector('.mc-multirename-warn');
  warn.hidden = !notes.length;
  warn.innerHTML = notes.map((n) => `<div>${esc(n)}</div>`).join('');

  if (!st.busy) setStatus(st, summary(st));
}

function summary(st) {
  const pv = st.preview;
  const n = (st.rows || []).length;
  let t = plural(n, 'file');
  if (!pv) return t;
  if (pv.field) return `${t} — the ${pv.field === 'search' ? 'search text' : 'mask'} has a mistake.`;
  t += `, ${plural(pv.changes, 'new name')}`;
  if (pv.errors) t += `, ${plural(pv.errors, 'problem')} (shown in red)`;
  if (st.renamed && st.stale) t = `Renamed ${plural(st.renamed, 'file')}. ${t}`;
  return t + '.';
}

function setStatus(st, text) {
  st.status = text;
  const el = st.box?.querySelector('.mc-multirename-status');
  if (el) el.textContent = text;
}

function paintSel(st, scroll = true) {
  const rows = st.box.querySelectorAll('tbody tr');
  rows.forEach((tr) => {
    const i = +tr.dataset.i;
    tr.classList.toggle('sel', st.sel.has(i));
    tr.classList.toggle('focus', i === st.focus);
  });
  if (scroll) rows[st.focus]?.scrollIntoView({ block: 'nearest' });
}

// ---- order ----

async function sortBy(st, key) {
  st.sort = { key, desc: st.sort.key === key ? !st.sort.desc : false };
  await go.MultiRename.Sort(key, st.sort.desc).catch(() => {});
  st.sel = new Set([0]);
  st.focus = 0;
  await refresh(st);
}

async function applyOrder(st, order) {
  try { await go.MultiRename.Reorder(order); } catch (e) { setStatus(st, errText(e)); return; }
  const where = new Map(order.map((o, i) => [o, i]));
  st.sel = new Set([...st.sel].map((i) => where.get(i)));
  st.focus = where.get(st.focus) ?? 0;
  st.sort = { key: '', desc: false };
  // show the new order at once, the new names follow
  st.rows = order.map((o) => st.rows[o]);
  render(st);
  await refresh(st);
}

// moveSelected moves the selected rows one place up (-1) or down (+1), as Shift+Up/Down.
function moveSelected(st, d) {
  const n = st.rows.length;
  const order = range(0, n - 1);
  const sel = [...st.sel].sort((a, b) => a - b);
  if (!sel.length || (d < 0 && sel[0] === 0) || (d > 0 && sel[sel.length - 1] === n - 1)) return;
  const walk = d < 0 ? sel : [...sel].reverse();
  for (const i of walk) { const j = i + d; [order[i], order[j]] = [order[j], order[i]]; }
  // order[k] is the old index that goes to k
  applyOrder(st, order);
}

// moveSelectedTo puts the selected rows before position at (drag and drop).
function moveSelectedTo(st, at) {
  const n = st.rows.length;
  const sel = [...st.sel].sort((a, b) => a - b);
  const rest = range(0, n - 1).filter((i) => !st.sel.has(i));
  const before = sel.filter((i) => i < at).length;
  const pos = Math.max(0, Math.min(rest.length, at - before));
  const order = [...rest.slice(0, pos), ...sel, ...rest.slice(pos)];
  if (order.every((o, i) => o === i)) return;
  applyOrder(st, order);
}

// ---- keys ----

function isTop(st) {
  const all = document.querySelectorAll('.mc-modal');
  return st.box && all[all.length - 1] === st.box.parentElement;
}

function onKey(st, ev) {
  if (!isTop(st)) return;
  const stop = () => { ev.preventDefault(); ev.stopImmediatePropagation(); };
  if (st.menu) {
    const items = [...st.menu.el.querySelectorAll('button:not([disabled])')];
    const i = items.indexOf(document.activeElement);
    if (ev.key === 'Escape') { stop(); closeMenu(st); return; }
    if (ev.key === 'ArrowDown') { stop(); items[(i + 1) % items.length]?.focus(); return; }
    if (ev.key === 'ArrowUp') { stop(); items[(i - 1 + items.length) % items.length]?.focus(); return; }
    if (ev.key === 'Home') { stop(); items[0]?.focus(); return; }
    if (ev.key === 'End') { stop(); items.at(-1)?.focus(); return; }
    if (ev.key === 'Tab') { stop(); return; }
    return;
  }
  if (ev.key === 'F2' || ev.key === 'F10') { stop(); settingsMenu(st, st.box.querySelector('.mc-multirename-menu-btn')); return; }
  if (ev.key === 'F3') { stop(); editNames(st); return; }
  if (ev.key === 'F5') { stop(); nextStep(st); return; }
  const inList = document.activeElement === st.box.querySelector('.mc-multirename-list');
  if (!inList) return;
  const n = st.rows.length;
  if (ev.key === 'Enter' && !ev.shiftKey && !ev.ctrlKey && !ev.altKey && !ev.metaKey) { stop(); startRename(st); return; }
  if (ev.shiftKey && (ev.key === 'ArrowUp' || ev.key === 'ArrowDown')) { stop(); moveSelected(st, ev.key === 'ArrowUp' ? -1 : 1); return; }
  const go2 = (i) => { st.focus = Math.max(0, Math.min(n - 1, i)); st.sel = new Set([st.focus]); paintSel(st); };
  if (ev.key === 'ArrowUp') { stop(); go2(st.focus - 1); return; }
  if (ev.key === 'ArrowDown') { stop(); go2(st.focus + 1); return; }
  if (ev.key === 'Home') { stop(); go2(0); return; }
  if (ev.key === 'End') { stop(); go2(n - 1); return; }
  if (ev.key === 'PageUp') { stop(); go2(st.focus - 15); return; }
  if (ev.key === 'PageDown') { stop(); go2(st.focus + 15); return; }
  if ((ev.ctrlKey || ev.metaKey) && ev.code === 'KeyA') { stop(); st.sel = new Set(range(0, n - 1)); paintSel(st, false); return; }
  if ((ev.ctrlKey || ev.metaKey) && ev.code === 'KeyC') {
    stop();
    const text = [...st.sel].sort((a, b) => a - b).map((i) => `${st.rows[i].old}\t${st.rows[i].new}`).join('\n');
    navigator.clipboard?.writeText(text).then(() => setStatus(st, `Copied ${plural(st.sel.size, 'line')}.`), () => setStatus(st, 'The clipboard can\'t be written.'));
  }
}

// ---- menus inside the window (popups of the core can't be used over a modal) ----

function showMenu(st, anchor, items, kind) {
  closeMenu(st);
  const el = document.createElement('div');
  el.className = 'mc-multirename-menu';
  el.setAttribute('role', 'menu');
  el.dataset.kind = kind;
  for (const it of items) {
    if (it.sep) { el.append(Object.assign(document.createElement('div'), { className: 'sep' })); continue; }
    const b = document.createElement('button');
    b.type = 'button';
    b.setAttribute('role', 'menuitem');
    b.textContent = it.label;
    if (it.disabled) b.disabled = true;
    if (it.detail) b.title = it.detail;
    b.onclick = () => { closeMenu(st); it.run(); };
    el.append(b);
  }
  st.box.append(el);
  const br = st.box.getBoundingClientRect(), ar = anchor.getBoundingClientRect();
  el.style.maxHeight = `${Math.max(120, br.height - 8)}px`;
  const h = el.offsetHeight, w = el.offsetWidth;
  let top = ar.bottom - br.top + 2;
  if (top + h > br.height - 4) top = ar.top - br.top - h - 2;
  top = Math.max(4, Math.min(br.height - h - 4, top));
  el.style.top = `${top}px`;
  el.style.left = `${Math.max(4, Math.min(br.width - w - 4, ar.left - br.left))}px`;
  const outside = (ev) => { if (!el.contains(ev.target)) closeMenu(st); };
  setTimeout(() => window.addEventListener('mousedown', outside, true));
  st.menu = { el, outside, anchor };
  el.querySelector('button:not([disabled])')?.focus();
}

function closeMenu(st) {
  if (!st.menu) return;
  window.removeEventListener('mousedown', st.menu.outside, true);
  st.menu.el.remove();
  const a = st.menu.anchor;
  st.menu = null;
  if (a?.isConnected) a.focus();
}

// ---- range picker ([N#-#], [E#-#]) ----

async function rangePicker(st, input, letter) {
  const row = st.rows[st.focus] || st.rows[0];
  if (!row) return;
  const [base, ext] = splitName(row.old, row.isDir);
  const text = letter === 'E' ? ext : base;
  const html = `<p class="mc-note">Select the characters with the mouse or type their positions.</p>
    <div class="mc-row"><label for="mr-range-text">${letter === 'E' ? 'Extension' : 'Name'} of ${esc(row.old)}</label>
    <input id="mr-range-text" type="text" readonly spellcheck="false" class="mc-multirename-rangetext" value="${esc(text)}" autofocus data-select="0,${text.length}"></div>
    <div class="mc-multirename-rangenums">
      <label>From<input id="mr-range-from" type="number" min="1" value="1"></label>
      <label>To<input id="mr-range-to" type="number" min="1" value="${Math.max(1, [...text].length)}"></label>
    </div>
    <label class="mc-check"><input type="radio" name="mr-range-dir" id="mr-range-start" checked>Count from the start</label>
    <label class="mc-check"><input type="radio" name="mr-range-dir" id="mr-range-end">Count from the end</label>
    <p class="mc-note">Placeholder: <code id="mr-range-out"></code></p>`;
  const chars = [...text];
  const len = chars.length;
  const make = (box) => {
    const from = parseInt(box.querySelector('#mr-range-from').value, 10);
    const to = parseInt(box.querySelector('#mr-range-to').value, 10);
    if (!(from >= 1) || !(to >= from)) return null;
    if (box.querySelector('#mr-range-end').checked && len) {
      const a = len - from + 1, b = len - to + 1; // positions counted from the end
      if (a === b) return `[${letter}-${a}]`;
      return b === 1 ? `[${letter}-${a}-]` : `[${letter}-${a}-${b}]`;
    }
    if (from === to) return `[${letter}${from}]`;
    return len && to >= len ? `[${letter}${from}-]` : `[${letter}${from}-${to}]`;
  };
  const r = await formDialog('Choose characters', html, (box) => { const ph = make(box); return ph ? { ph } : 'Give a start and an end, the end not before the start.'; }, (box) => {
    const t = box.querySelector('#mr-range-text');
    const show = () => { box.querySelector('#mr-range-out').textContent = make(box) || '—'; };
    const fromSel = () => {
      // selection offsets are UTF-16 units; count characters like the engine does
      const a = [...t.value.slice(0, t.selectionStart)].length, b = [...t.value.slice(0, t.selectionEnd)].length;
      if (b > a) { box.querySelector('#mr-range-from').value = a + 1; box.querySelector('#mr-range-to').value = b; }
      show();
    };
    t.addEventListener('select', fromSel);
    t.addEventListener('mouseup', fromSel);
    t.addEventListener('keyup', fromSel);
    box.addEventListener('input', show);
    box.addEventListener('change', show);
    show();
  }, { okLabel: 'Insert', kind: 'multirename-range' });
  if (r) insertAt(input, r.ph);
}

// ---- F2: load/save settings ----

function settingsMenu(st, anchor) {
  if (st.busy) return;
  const items = st.presets.map((name) => ({ label: name, run: () => loadPreset(st, name) }));
  if (items.length) items.push({ sep: true });
  items.push(
    { label: st.preset ? `Save settings "${st.preset}"` : 'Save settings…', run: () => (st.preset ? savePreset(st, st.preset) : savePresetAs(st)) },
    { label: 'Save settings as…', run: () => savePresetAs(st) },
    { label: 'Delete saved settings…', disabled: !st.presets.length, run: () => deletePreset(st) },
    { label: 'Default', run: () => resetAll(st) },
    { sep: true },
    { label: 'Load names from file…', run: () => loadNames(st, () => go.MultiRename.LoadNamesFile()) },
    { label: 'Load names from clipboard', run: () => loadNames(st, () => go.MultiRename.LoadNamesClipboard()) },
    { label: 'Edit names…', run: () => editNames(st) },
    { label: 'Forget loaded names', disabled: !(st.preview?.names >= 0), run: async () => { await go.MultiRename.ClearNames(); refresh(st); } },
  );
  showMenu(st, anchor, items, 'settings');
}

async function loadPreset(st, name) {
  try {
    const s = await go.MultiRename.LoadPreset(name);
    writeForm(st.box, { ...st.defaults, ...s, subst: false });
    st.preset = name;
    st.stale = false;
    setStatus(st, `Loaded the settings "${name}".`);
    await refresh(st);
  } catch (e) { await messageDialog({ Title: 'Load settings', Message: errText(e), Kind: 'multirename-error' }); }
}

async function savePreset(st, name) {
  const f = readForm(st.box);
  const s = { ...f };
  delete s.subst;
  try {
    await go.MultiRename.SavePreset(name, s);
    st.preset = name;
    st.presets = await go.MultiRename.Presets().catch(() => st.presets);
    setStatus(st, `Saved the settings as "${name}".`);
  } catch (e) { await messageDialog({ Title: 'Save settings', Message: errText(e), Kind: 'multirename-error' }); }
}

async function savePresetAs(st) {
  const name = await inputDialog({
    title: 'Save settings', label: 'Name of the settings', value: st.preset || '', okLabel: 'Save', kind: 'multirename-save',
    validate: (v) => (!v.trim() ? 'Give the settings a name.' : v.length > 252 ? 'The name is too long (at most 252 characters).' : ''),
  });
  if (name === null) return;
  if (st.presets.includes(name) && name !== st.preset) {
    const b = await messageDialog({ Title: 'Save settings', Message: `Settings named "${name}" are saved already. Replace them?`, Buttons: ['Replace', 'Cancel'], Kind: 'multirename-replace' });
    if (b !== 'Replace') return;
  }
  await savePreset(st, name);
}

async function deletePreset(st) {
  const html = `<div class="mc-row"><label for="mr-preset">Saved settings</label><select id="mr-preset">${st.presets.map((p) => `<option${p === st.preset ? ' selected' : ''}>${esc(p)}</option>`).join('')}</select></div>`;
  const pick = await formDialog('Delete saved settings', html, (box) => { const v = box.querySelector('#mr-preset').value; return v ? { name: v } : 'Choose the settings to delete.'; }, null, { okLabel: 'Delete', kind: 'multirename-delete' });
  if (!pick) return;
  const { name } = pick;
  try {
    await go.MultiRename.DeletePreset(name);
    if (st.preset === name) st.preset = '';
    st.presets = await go.MultiRename.Presets().catch(() => st.presets.filter((p) => p !== name));
    setStatus(st, `Deleted the settings "${name}".`);
  } catch (e) { await messageDialog({ Title: 'Delete settings', Message: errText(e), Kind: 'multirename-error' }); }
}

// resetAll is "Default" (F-518): every field back to its default, loaded names and chained steps forgotten.
async function resetAll(st) {
  writeForm(st.box, { ...st.defaults, subst: false });
  st.preset = '';
  st.stale = false;
  await go.MultiRename.Reset().catch(() => {});
  await refresh(st);
}

async function loadNames(st, fn) {
  try {
    const n = await fn();
    if (n < 0) return;
    st.stale = false;
    await refresh(st);
    if (n !== st.rows.length) setStatus(st, `${plural(n, 'name')} loaded for ${plural(st.rows.length, 'file')}.`);
  } catch (e) { await messageDialog({ Title: 'Load names', Message: errText(e), Kind: 'multirename-error' }); }
}

// ---- F3: edit names in the editor ----

async function editNames(st) {
  if (st.busy) return;
  closeMenu(st);
  try {
    await go.MultiRename.EditNames(engineSettings(readForm(st.box)));
  } catch (e) {
    await messageDialog({ Title: 'Edit names', Message: errText(e), Kind: 'multirename-error' });
    return;
  }
  const b = await messageDialog({
    Title: 'Edit names', Kind: 'multirename-edit',
    Message: 'The new names are open in the editor, one per line. Change them, save the file, then choose "Use names".',
    Buttons: ['Use names', 'Cancel'],
  });
  if (b === 'Use names') await loadNames(st, () => go.MultiRename.ReadEditedNames());
}

// ---- F5: next step ----

async function nextStep(st) {
  if (st.busy) return;
  closeMenu(st);
  try {
    await go.MultiRename.NextStep(engineSettings(readForm(st.box)));
  } catch (e) {
    await messageDialog({ Title: 'Next step', Message: errText(e), Kind: 'multirename-error' });
    return;
  }
  // the masks start over on the new names
  const keep = readForm(st.box);
  writeForm(st.box, { ...st.defaults, counterStart: keep.counterStart, counterStep: keep.counterStep, counterDigits: keep.counterDigits, subst: false });
  st.stale = false;
  await refresh(st);
}

// ---- Start! ----

async function startRename(st) {
  if (st.busy) return;
  closeMenu(st);
  clearTimeout(st.timer);
  const pv = await refresh(st);
  if (!pv || pv.error) return;
  if (st.stale || !pv.changes) { setStatus(st, 'No name changes: nothing to rename.'); return; }
  if (pv.field) {
    await messageDialog({ Title: 'Multi-Rename tool', Message: pv.fieldError, Kind: 'multirename-error' });
    return;
  }
  if (pv.errors) {
    const bad = pv.rows.filter((r) => r.problems?.length);
    const lines = bad.slice(0, 12).map((r) => `${r.old} → ${r.new}: ${r.problems.map((x) => x.detail).join(' ')}`);
    if (bad.length > 12) lines.push(`… and ${bad.length - 12} more`);
    await messageDialog({ Title: 'Can\'t rename', Message: `These new names can't be used:\n\n${lines.join('\n')}\n\nChange the masks or the names first.`, Kind: 'multirename-conflicts' });
    return;
  }
  const f = readForm(st.box);
  for (;;) {
    st.busy = true;
    st.box.classList.add('mc-multirename-busy');
    setStatus(st, `Renaming ${plural(pv.changes, 'file')}…`);
    let r = null, err = null;
    try { r = await go.MultiRename.Execute(); } catch (e) { err = errText(e); }
    st.busy = false;
    st.box.classList.remove('mc-multirename-busy');
    if (!err) {
      st.renamed = r.renamed;
      st.stale = true;
      saveHistory(st, f);
      st.undo = await go.MultiRename.UndoInfo().catch(() => st.undo);
      paintUndo(st);
      await refresh(st);
      if (r.note) setStatus(st, `${summary(st)} ${r.note}`);
      for (const panel of st.app.panels) panel.reload();
      return;
    }
    const b = await messageDialog({ Title: 'Rename failed', Message: err, Buttons: ['Retry', 'Cancel'], Kind: 'multirename-failed' });
    if (b !== 'Retry') { await refresh(st); return; }
    const again = await refresh(st);
    if (!again || again.errors || !again.changes) return;
  }
}

function saveHistory(st, f) {
  const s = st.app.setting(SETTING, {}) || {};
  const hist = { ...(s.history || {}) };
  pushHistory(hist, 'nameMask', f.nameMask);
  pushHistory(hist, 'extMask', f.extMask);
  pushHistory(hist, 'search', f.search);
  pushHistory(hist, 'replace', f.replace);
  st.app.setSetting(SETTING, { ...s, history: hist });
  st.histNow = hist;
}

// ---- Undo ----

function paintUndo(st) {
  const b = st.box?.querySelector('.mc-multirename-undo');
  if (!b) return;
  b.disabled = !st.undo?.available;
  b.title = st.undo?.available ? `Give ${plural(st.undo.count, 'file')} renamed at ${st.undo.time} their old names back` : 'Nothing to undo';
}

async function undo(st) {
  if (st.busy) return;
  const info = await go.MultiRename.UndoInfo().catch(() => null);
  if (!info?.available) { setStatus(st, 'There is nothing to undo.'); return; }
  const b = await messageDialog({ Title: 'Undo', Message: `Give the ${plural(info.count, 'file')} renamed at ${info.time} their old names back?`, Buttons: ['Undo', 'Cancel'], Kind: 'multirename-undo' });
  if (b !== 'Undo') return;
  try {
    const n = await go.MultiRename.Undo();
    st.undo = await go.MultiRename.UndoInfo().catch(() => ({ available: false }));
    paintUndo(st);
    st.stale = true;
    st.renamed = 0;
    await refresh(st);
    setStatus(st, `${plural(n, 'file')} got their old names back.`);
    for (const panel of st.app.panels) panel.reload();
  } catch (e) {
    await messageDialog({ Title: 'Undo failed', Message: errText(e), Kind: 'multirename-error' });
  }
}

// undoFromPanel is cm_MultiRenameFiles with the parameter <UNDO> (asks first) or ==<UNDO> (does not ask).
async function undoFromPanel(app, ask) {
  const info = await go.MultiRename.UndoInfo().catch(() => null);
  if (!info?.available) { app.flash('There is no Multi-Rename batch to undo.'); return; }
  if (ask) {
    const b = await messageDialog({ Title: 'Undo multi-rename', Message: `Give the ${plural(info.count, 'file')} renamed at ${info.time} their old names back?`, Buttons: ['Undo', 'Cancel'], Kind: 'multirename-undo' });
    if (b !== 'Undo') return;
  }
  try {
    const n = await go.MultiRename.Undo();
    app.flash(`${plural(n, 'file')} got their old names back.`);
  } catch (e) {
    await messageDialog({ Title: 'Undo failed', Message: errText(e), Kind: 'multirename-error' });
  }
  for (const panel of app.panels) panel.reload();
}

// ---- Result list ----

async function resultList(st) {
  const text = await go.MultiRename.ResultText().catch(() => '');
  if (!text) { setStatus(st, 'Nothing renamed yet and no name changes.'); return; }
  await infoDialog(`<h2>Result list</h2><textarea id="mr-result" class="mc-multirename-result" readonly spellcheck="false">${esc(text)}</textarea>`, 'mc-multirename-info', 'multirename-result');
}

// ---- the command ----

command({
  name: 'cm_MultiRenameFiles',
  label: 'Multi-rename tool…',
  short: 'Rename',
  enabled: (app) => !app.cur?.path?.includes('://') || isBranchPath(app.cur.path),
  run: async (app, args = {}) => {
    const param = String(args.param ?? args.params ?? '').trim();
    if (param === '==<UNDO>' || args.undo === 'noask') return undoFromPanel(app, false);
    if (param === '<UNDO>' || args.undo) return undoFromPanel(app, true);
    if (cur) return; // already open
    return openTool(app);
  },
});

// Control mode: {"cmd": "multirename"} → the rows of the open window; {"sort": "name", "desc": false} sorts,
// {"set": {"mr-name": "[N]_[C]"}} fills fields and waits for the preview, {"order": [2, 0, 1]} reorders, {"select": [0, 1]} selects rows (for Shift+Up/Down tests).
addControlCommand('multirename', async (m = {}) => {
  const st = cur;
  if (!st) return { ok: true, open: false };
  if (m.set && typeof m.set === 'object') {
    // fill fields without pressing a button (dialog_answer with no button would press Start!)
    for (const [id, v] of Object.entries(m.set)) {
      const el = st.box.querySelector(`#${CSS.escape(id)}`);
      if (!el) throw new Error(`the Multi-Rename tool has no field "${id}"`);
      if (el.type === 'checkbox') el.checked = !!v; else el.value = v;
      el.dispatchEvent(new Event('input', { bubbles: true }));
    }
    clearTimeout(st.timer);
    st.stale = false;
    await refresh(st);
  }
  if (m.sort) await sortBy(st, m.sort);
  if (Array.isArray(m.order)) await applyOrder(st, m.order);
  if (Array.isArray(m.select)) { st.sel = new Set(m.select); st.focus = m.select[0] ?? 0; paintSel(st); }
  if (m.refresh) await refresh(st);
  const pv = st.preview;
  return {
    ok: true, open: true, status: st.status, preset: st.preset, presets: st.presets, undo: st.undo, stale: st.stale,
    rows: (st.rows || []).map((r) => ({ old: r.old, new: r.new, dir: r.dir, isDir: r.isDir, problems: r.problems || [], warnings: r.warnings || [] })),
    warnings: pv?.warnings || [], errors: pv?.errors ?? 0, changes: pv?.changes ?? 0, field: pv?.field || '', fieldError: pv?.fieldError || '',
    names: pv?.names ?? -1, steps: !!pv?.steps, selected: [...st.sel], focus: st.focus,
    menu: st.menu ? [...st.menu.el.querySelectorAll('button')].map((b) => b.textContent) : null,
  };
});
