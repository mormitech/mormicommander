// Find files (Alt+F7, cm_SearchFor; FEATURES.md section 24, spec docs/spec/area-5.md sections 1-7): the dialog with the
// General, Advanced, Plugins and Load/Save tabs, the result list that fills while the search runs, and its actions:
// Go to file, View (F3), Edit (F4), Feed to listbox, copy and save the list. The search itself is internal/find,
// reached through the Go object Find (feature_find.go); saved searches and histories are kept there too.
//
// The dialog is a page dialog (core/dialogs.js modal), so control mode reads its fields and buttons ("dialog") and
// answers it ("dialog_answer"): fields have ids ff-…, the tabs and every action are buttons. The control command "find"
// tells the results. A result list fed to a panel is the folder find://results-N; Panel.pathHooks maps its names to the
// real files, so every file command works on it.
//
// With the option FindFiles.Modeless (off by default; board issue #110) the dialog is modeless, as TC's is: its window
// leaves the main window usable (core/dialogs.js meta.modeless), so the panels keep working while a search runs, and
// Alt+F7 brings the open Find files window back to the front.
//
// After a search for duplicate files, Duplicates… (board issue #32, find-dups-lib.js) keeps one file of each group by a
// rule (newest, oldest, first path) and marks the other copies, moves them to the trash (an ordinary trash job) or
// replaces them with hard links (Find.LinkDuplicates, which compares the contents first). Nothing else changes.
//
// Other features add tabs after Load/Save with addFindTab (the AI tab, find-ai.js, issue #146) and run searches in
// the open dialog through findDialog, so their results land in the same list as a search started by hand.

import { attachPathComplete } from '../core/pathcomplete.js';
import './find.css';
import { command } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { modal, messageDialog, inputDialog, formDialog, esc, keyOwnerBox, focusDialog } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { Panel } from '../core/panel.js';
import { size, date, plural } from '../core/format.js';
import { track } from '../core/busy.js';
import { comboFromEvent } from '../core/keys.js';
import { findIncludeHidden } from '../core/hidden.js';
import { on as optOn, opt as optValue } from './config/util.js';
import { findDefaults } from './find-defaults.js';
import { runJob } from '../core/jobs.js';
import { DUP_RULES, planDuplicates, hasDuplicates, dropGone } from './find-dups-lib.js';

// ---- result lists in panels (Feed to listbox, F-541) ----

const feeds = new Map(); // find://results-N → the folder its names are relative to ('' = names are full paths)
const isFeed = (p) => /^find:\/\//.test(p || '');
const isAbs = (p) => /^([a-z]:\\|\\\\|\/|[a-z][a-z0-9+.-]*:\/\/)/i.test(p);

Panel.pathHooks.push((panel, name) => {
  if (!isFeed(panel.path)) return '';
  const base = feeds.get(panel.path);
  if (base === undefined || isAbs(name)) return isAbs(name) ? name : '';
  const sep = theApp.sepFor(base);
  return base.endsWith(sep) ? base + name : base + sep + name;
});

// ---- the dialog's state ----

const S = {
  box: null,         // the open dialog's form element
  done: null,        // closes it
  setup: null,       // Find.Setup(): histories, last search, saved searches, code pages
  runId: 0,
  running: false,
  items: [],         // results: {p, n, d, s, t, f, g, a}
  files: 0, dirs: 0,
  cur: -1,           // cursor in the result list
  sel: new Set(),    // selected result indexes
  anchor: -1,
  progress: null,
  last: null,        // the find-done of the current search
  error: '',
  finish: null,      // resolves when the search ends (control mode waits for it)
  resume: null,      // fields kept when the dialog was left with Go to file, so Alt+F7 brings the results back
  openSel: [],       // the panel's selection when the dialog opened ("Only search in selected")
  tab: 'general',
};
const inbox = new Map(); // events of a search that arrived before Start answered with its number

// ---- tabs added by other features (the AI tab, find-ai.js) ----

// addFindTab adds a tab after Load/Save. t: {id, label, html() (the page's inside), setup(box) (after the dialog is
// built), shown() (the tab was switched to), start() (Start search while the tab is shown, instead of a normal search),
// busy() (true while the tab's own work runs: Start is greyed and Cancel says Stop), stop() (Cancel / Esc while busy),
// key(ev, combo) (true when the tab used a key), closed() (the dialog closed)}. Every hook is optional.
const extraTabs = [];
export function addFindTab(t) { extraTabs.push(t); }
const tabHook = (name, ...args) => { for (const t of extraTabs) { try { t[name]?.(...args); } catch (e) { console.error(e); } } };
const busyTab = () => extraTabs.find((t) => t.busy?.());

const ROW_H = 20;
const DEPTHS = [[-1, 'all (unlimited depth)'], [0, 'current folder only'], ...Array.from({ length: 99 }, (_, i) => [i + 1, `${i + 1} level${i ? 's' : ''}`])];
const ATTRS = [['r', 'Read-only'], ['h', 'Hidden'], ['x', 'Executable'], ['l', 'Symbolic link'], ['a', 'Archive'], ['s', 'System'], ['c', 'Compressed'], ['e', 'Encrypted']];
const AGE_UNITS = ['minutes', 'hours', 'days', 'weeks', 'months', 'years'];

function attrAvailable(letter, plat) {
  if ('ase'.includes(letter)) return plat === 'windows';
  if (letter === 'c') return plat === 'windows' || plat === 'darwin';
  return true;
}

const $ = (sel) => S.box?.querySelector(sel);
const val = (id) => $(`#${id}`)?.value ?? '';
const chk = (id) => !!$(`#${id}`)?.checked;
const setVal = (id, v) => { const el = $(`#${id}`); if (el) el.value = v ?? ''; };
const setChk = (id, v) => { const el = $(`#${id}`); if (el) el.checked = !!v; };

function datalist(id, items) {
  return `<datalist id="${id}">${(items || []).map((x) => `<option value="${esc(x)}"></option>`).join('')}</datalist>`;
}

function html(setup) {
  const plat = setup.platform || theApp.platform;
  const cps = setup.codepages || [];
  const cpOpts = (def) => cps.map((c) => `<option value="${esc(c)}"${c === def ? ' selected' : ''}>${esc(c)}</option>`).join('');
  const attrRows = ATTRS.map(([l, t]) => `<label class="mc-find-attr${attrAvailable(l, plat) ? '' : ' off'}">${esc(t)}
      <select id="ff-attr-${l}"${attrAvailable(l, plat) ? '' : ' disabled'}><option value="">any</option><option value="+">set</option><option value="-">not set</option></select></label>`).join('');
  return `<h2>Find files</h2>
  <div class="mc-find-top">
    <div class="mc-find-tabs-wrap">
      <div class="mc-find-tabs" role="tablist">
        <button type="button" class="mc-find-tab on" data-tab="general" role="tab">General</button>
        <button type="button" class="mc-find-tab" data-tab="advanced" role="tab">Advanced</button>
        <button type="button" class="mc-find-tab" data-tab="plugins" role="tab">Plugins</button>
        <button type="button" class="mc-find-tab" data-tab="saved" role="tab">Load/Save</button>
        ${extraTabs.map((t) => `<button type="button" class="mc-find-tab" data-tab="${esc(t.id)}" role="tab">${esc(t.label)}</button>`).join('')}
      </div>
      <div class="mc-find-page" data-page="general">
        <div class="mc-find-line"><label for="ff-for">Search for:</label><input id="ff-for" type="text" list="ff-for-list" spellcheck="false" autofocus>
          <label class="mc-check"><input id="ff-regex" type="checkbox">RegEx</label></div>
        <div class="mc-find-line"><label for="ff-in">Search in:</label><input id="ff-in" type="text" list="ff-in-list" spellcheck="false">
          <button type="button" class="ff-browse" title="Choose a folder">&gt;&gt;</button>
          <select id="ff-drives" title="Search in a volume"><option value="">Volumes…</option></select></div>
        <div class="mc-find-opts">
          <label class="mc-check"><input id="ff-selected" type="checkbox">Only search in selected folders/files</label>
          <label class="mc-check"><input id="ff-results" type="checkbox">Only search in the current results</label>
          <label class="mc-check"><input id="ff-hidden" type="checkbox">Include hidden files</label>
        </div>
        <div class="mc-find-line"><label for="ff-depth">Search subfolders:</label><select id="ff-depth">${DEPTHS.map(([v, t]) => `<option value="${v}">${t}</option>`).join('')}</select>
          <label class="mc-check"><input id="ff-archives" type="checkbox">Search archives</label></div>
        <div class="mc-find-line"><label class="mc-check"><input id="ff-texton" type="checkbox">Find text:</label><input id="ff-text" type="text" list="ff-text-list" spellcheck="false"></div>
        <div class="mc-find-grid">
          <label class="mc-check"><input id="ff-whole" type="checkbox">Whole words only</label>
          <label class="mc-check"><input id="ff-case" type="checkbox">Case sensitive</label>
          <label class="mc-check"><input id="ff-not" type="checkbox">Not containing</label>
          <label class="mc-check"><input id="ff-hex" type="checkbox">Hex</label>
          <label class="mc-check"><input id="ff-tregex" type="checkbox">RegEx (2)</label>
          <span></span>
          <label class="mc-check enc"><input id="ff-ansi" type="checkbox">ANSI <select id="ff-ansicp" title="ANSI code page">${cpOpts('windows-1252')}</select></label>
          <label class="mc-check enc"><input id="ff-oem" type="checkbox">ASCII (DOS) <select id="ff-oemcp" title="DOS code page">${cpOpts('cp437')}</select></label>
          <label class="mc-check enc"><input id="ff-utf8" type="checkbox">UTF-8</label>
          <label class="mc-check enc"><input id="ff-utf16le" type="checkbox">Unicode (UTF-16)</label>
          <label class="mc-check enc"><input id="ff-utf16be" type="checkbox">UTF-16 big endian</label>
        </div>
      </div>
      <div class="mc-find-page" data-page="advanced" hidden>
        <div class="mc-find-line"><label class="mc-check"><input id="ff-dateon" type="checkbox">Date between:</label><input id="ff-from" type="date"><span>and:</span><input id="ff-to" type="date"></div>
        <div class="mc-find-line"><label class="mc-check"><input id="ff-ageon" type="checkbox"><select id="ff-agemode"><option value="newer">Not older than:</option><option value="older">Older than:</option></select></label>
          <input id="ff-agen" type="number" min="1" value="1" class="num"><select id="ff-ageunit">${AGE_UNITS.map((u) => `<option value="${u}"${u === 'days' ? ' selected' : ''}>${u}</option>`).join('')}</select></div>
        <div class="mc-find-line"><label class="mc-check"><input id="ff-sizeon" type="checkbox">File size:</label><select id="ff-sizeop"><option value="=">=</option><option value=">" selected>&gt;</option><option value="&lt;">&lt;</option></select>
          <input id="ff-size" type="number" min="0" value="0" class="num"><select id="ff-sizeunit"><option value="B">bytes</option><option value="KB" selected>KB</option><option value="MB">MB</option><option value="GB">GB</option><option value="TB">TB</option></select></div>
        <div class="mc-find-sub">Attributes:</div>
        <div class="mc-find-attrs">${attrRows}</div>
        <div class="mc-find-line"${plat === 'windows' ? ' hidden' : ''}><label for="ff-perms">Permissions:</label><input id="ff-perms" type="text" maxlength="9" spellcheck="false" placeholder="?????????" class="perms" title="rwxrwxrwx: a letter must be set, - must not be, ? anything"></div>
        <div class="mc-find-line"><label for="ff-kind">Find:</label><select id="ff-kind"><option value="">files and folders</option><option value="files">files only</option><option value="dirs">folders only</option></select>
          <label class="mc-check"><input id="ff-links" type="checkbox">Follow symbolic links</label>
          <label class="mc-check"><input id="ff-skiphidden" type="checkbox">Skip hidden files</label></div>
        <div class="mc-find-line"><label for="ff-exclude">Skip folders:</label><input id="ff-exclude" type="text" spellcheck="false" placeholder="node_modules;.git"></div>
        <div class="mc-find-line"><label class="mc-check"><input id="ff-dupon" type="checkbox">Find duplicate files:</label>
          <label class="mc-check"><input id="ff-dupname" type="checkbox" checked>same name</label>
          <label class="mc-check"><input id="ff-dupsize" type="checkbox" checked>same size</label>
          <label class="mc-check"><input id="ff-dupcontent" type="checkbox">same content</label></div>
      </div>
      <div class="mc-find-page" data-page="plugins" hidden>
        <p class="mc-note">Search plugins (WDX) are Windows programs and can't run here. Search by name, text, date, size,
          attributes and permissions on the General and Advanced tabs.</p>
      </div>
      <div class="mc-find-page" data-page="saved" hidden>
        <div class="mc-find-saved">
          <select id="ff-saved" size="8" aria-label="Saved searches"></select>
          <div class="mc-find-col"><button type="button" class="ff-load">Load</button><button type="button" class="ff-save">Save…</button><button type="button" class="ff-delete">Delete…</button></div>
        </div>
        <p class="mc-note">"Last search" is the search you started last. Load fills in the fields; it doesn't start the search.</p>
      </div>
      ${extraTabs.map((t) => `<div class="mc-find-page" data-page="${esc(t.id)}" hidden>${t.html?.() || ''}</div>`).join('')}
    </div>
    <div class="mc-find-col main">
      <button type="submit" class="primary ff-start">Start search</button>
      <button type="button" class="ff-cancel">Cancel</button>
    </div>
  </div>
  <p class="mc-err" hidden></p>
  <div class="mc-find-list" tabindex="0" role="listbox" aria-label="Search results"><div class="sp"></div><div class="rows"></div><div class="empty" hidden></div></div>
  <div class="mc-find-status" aria-live="polite"></div>
  <div class="mc-find-actions">
    <button type="button" class="ff-new">New search</button>
    <button type="button" class="ff-goto">Go to file</button>
    <button type="button" class="ff-view">View</button>
    <button type="button" class="ff-edit">Edit</button>
    <button type="button" class="ff-feed">Feed to listbox</button>
    <button type="button" class="ff-copy">Copy list</button>
    <button type="button" class="ff-savelist">Save list…</button>
    <button type="button" class="ff-dups" title="Keep one file of each group of duplicates; mark, trash or hard-link the others">Duplicates…</button>
  </div>
  ${datalist('ff-for-list', setup.history?.name)}${datalist('ff-in-list', setup.history?.in)}${datalist('ff-text-list', setup.history?.text)}`;
}

// ---- reading and filling the fields ----

function readForm() {
  const attrSet = [], attrClear = [];
  for (const [l] of ATTRS) {
    const v = val(`ff-attr-${l}`);
    if (v === '+') attrSet.push(l);
    if (v === '-') attrClear.push(l);
  }
  const enc = ['ansi', 'oem', 'utf8', 'utf16le', 'utf16be'].filter((e) => chk(`ff-${e}`));
  return {
    searchFor: val('ff-for'), nameRegex: chk('ff-regex'), searchIn: val('ff-in'),
    selected: chk('ff-selected') ? S.openSel : [], withinRun: chk('ff-results') && S.items.length ? S.runId : 0,
    depth: +val('ff-depth'), archives: chk('ff-archives'),
    textOn: chk('ff-texton'), text: val('ff-text'), wholeWords: chk('ff-whole'), caseSensitive: chk('ff-case'), notContaining: chk('ff-not'),
    hex: chk('ff-hex'), textRegex: chk('ff-tregex'), encodings: enc, ansiCodepage: val('ff-ansicp'), oemCodepage: val('ff-oemcp'),
    dateOn: chk('ff-dateon'), dateFrom: val('ff-from'), dateTo: val('ff-to'),
    ageOn: chk('ff-ageon'), ageOlder: val('ff-agemode') === 'older', ageN: Math.trunc(+val('ff-agen') || 0), ageUnit: val('ff-ageunit'),
    sizeOn: chk('ff-sizeon'), sizeOp: val('ff-sizeop'), sizeValue: Math.trunc(+val('ff-size') || 0), sizeUnit: val('ff-sizeunit'),
    attrSet: attrSet.join(''), attrClear: attrClear.join(''), perms: val('ff-perms'), kind: val('ff-kind'),
    followLinks: chk('ff-links'), skipHidden: chk('ff-skiphidden'), excludeDirs: val('ff-exclude'),
    dupOn: chk('ff-dupon'), dupName: chk('ff-dupname'), dupSize: chk('ff-dupsize'), dupContent: chk('ff-dupcontent'),
  };
}

// fill puts a request's values in the fields. keepIn: leave Search in alone when the request has none.
function fill(r, { keepIn = true } = {}) {
  if (!r) return;
  setVal('ff-for', r.searchFor || '*');
  setChk('ff-regex', r.nameRegex);
  if (r.searchIn || !keepIn) setVal('ff-in', r.searchIn || '');
  setVal('ff-depth', String(r.depth ?? -1));
  if (!$('#ff-depth').value) setVal('ff-depth', '-1');
  setChk('ff-archives', r.archives);
  setChk('ff-texton', r.textOn);
  setVal('ff-text', r.text || '');
  setChk('ff-whole', r.wholeWords); setChk('ff-case', r.caseSensitive); setChk('ff-not', r.notContaining);
  setChk('ff-hex', r.hex); setChk('ff-tregex', r.textRegex);
  const enc = r.encodings?.length ? r.encodings : ['ansi', 'utf8', 'utf16le'];
  for (const e of ['ansi', 'oem', 'utf8', 'utf16le', 'utf16be']) setChk(`ff-${e}`, enc.includes(e));
  if (r.ansiCodepage) setVal('ff-ansicp', r.ansiCodepage);
  if (r.oemCodepage) setVal('ff-oemcp', r.oemCodepage);
  setChk('ff-dateon', r.dateOn); setVal('ff-from', r.dateFrom || ''); setVal('ff-to', r.dateTo || '');
  setChk('ff-ageon', r.ageOn); setVal('ff-agemode', r.ageOlder ? 'older' : 'newer'); setVal('ff-agen', r.ageN || 1); setVal('ff-ageunit', r.ageUnit || 'days');
  setChk('ff-sizeon', r.sizeOn); setVal('ff-sizeop', r.sizeOp || '>'); setVal('ff-size', r.sizeValue || 0); setVal('ff-sizeunit', r.sizeUnit || 'KB');
  for (const [l] of ATTRS) setVal(`ff-attr-${l}`, (r.attrSet || '').includes(l) ? '+' : (r.attrClear || '').includes(l) ? '-' : '');
  setVal('ff-perms', r.perms && r.perms !== '?????????' ? r.perms : '');
  setVal('ff-kind', r.kind || '');
  setChk('ff-links', r.followLinks); setHidden(!r.skipHidden); setVal('ff-exclude', r.excludeDirs || '');
  setChk('ff-dupon', r.dupOn);
  if (r.dupOn) { setChk('ff-dupname', r.dupName); setChk('ff-dupsize', r.dupSize); setChk('ff-dupcontent', r.dupContent); }
  syncEnabled();
}

// setHidden ticks "Include hidden files" (General) and its other side, "Skip hidden files" (Advanced), together.
function setHidden(include) {
  setChk('ff-hidden', include);
  setChk('ff-skiphidden', !include);
}

// syncEnabled greys out what doesn't apply (spec area 5, 3.5: Hex greys the text options).
function syncEnabled() {
  if (!S.box) return;
  const hex = chk('ff-hex');
  for (const id of ['ff-whole', 'ff-case', 'ff-tregex', 'ff-ansi', 'ff-oem', 'ff-utf8', 'ff-utf16le', 'ff-utf16be', 'ff-ansicp', 'ff-oemcp']) $(`#${id}`).disabled = hex;
  $('#ff-selected').disabled = S.openSel.length === 0;
  if (!S.openSel.length) setChk('ff-selected', false);
  $('#ff-results').disabled = S.items.length === 0 || S.running;
  if ($('#ff-results').disabled) setChk('ff-results', false);
  $('#ff-in').disabled = chk('ff-selected') || chk('ff-results');
  const none = S.items.length === 0;
  const busy = S.running || !!busyTab();
  $('.ff-start').disabled = busy;
  $('.ff-cancel').textContent = busy ? 'Stop' : 'Cancel';
  for (const c of ['.ff-goto', '.ff-view', '.ff-edit', '.ff-copy', '.ff-savelist']) $(c).disabled = none;
  $('.ff-feed').disabled = none || S.running;
  $('.ff-dups').disabled = S.running || !hasDuplicates(S.items);
}

function showErr(text) {
  const e = $('.mc-err');
  if (!e) return;
  e.hidden = !text;
  e.textContent = text || '';
}

function switchTab(name) {
  if (!S.box) return;
  S.tab = name;
  S.box.querySelectorAll('.mc-find-tab').forEach((b) => b.classList.toggle('on', b.dataset.tab === name));
  S.box.querySelectorAll('.mc-find-page').forEach((p) => { p.hidden = p.dataset.page !== name; });
  if (name === 'saved') refreshSaved();
  extraTabs.find((t) => t.id === name)?.shown?.(S.box);
}

const TABS = ['general', 'advanced', 'plugins', 'saved'];
const allTabs = () => [...TABS, ...extraTabs.map((t) => t.id)];

async function refreshSaved(select) {
  let names = [];
  try { names = await go.Find.Saved(); } catch (e) { showErr(errText(e)); }
  const el = $('#ff-saved');
  if (!el) return;
  el.innerHTML = `<option value="">Last search</option>` + (names || []).map((n) => `<option value="${esc(n)}">${esc(n)}</option>`).join('');
  el.value = select ?? '';
  if (el.selectedIndex < 0) el.selectedIndex = 0;
}

// ---- searching ----

// startSearch starts the search the fields describe, or `given` (a request made elsewhere: the AI tab's find_files),
// which then stays on the shown tab and leaves the focus where it is. It answers the error, or '' when it started.
async function startSearch(given) {
  if (!S.box || S.running) return S.running ? 'A search is running already.' : 'The Find files dialog is closed.';
  showErr('');
  const req = given && typeof given === 'object' ? given : readForm();
  let id;
  try { id = await go.Find.Start(req); } catch (e) {
    showErr(errText(e));
    const focus = /folder to search in/i.test(errText(e)) ? '#ff-in' : /text|hex|character set/i.test(errText(e)) ? '#ff-text' : null;
    if (focus && req !== given) { switchTab('general'); $(focus)?.focus(); }
    return errText(e);
  }
  S.runId = id;
  S.req = req;
  S.items = []; S.files = 0; S.dirs = 0; S.cur = -1; S.sel = new Set(); S.anchor = -1;
  S.running = true; S.progress = null; S.last = null; S.error = '';
  let resolve;
  S.finish = new Promise((r) => { resolve = r; });
  S.finishResolve = resolve;
  track(S.finish);
  const early = inbox.get(id);
  inbox.clear();
  if (early) { for (const items of early.items) addItems(items); if (early.done) finished(early.done); }
  syncEnabled();
  render();
  if (req !== given) $('.mc-find-list')?.focus();
  return '';
}

// reset forgets the results of the last dialog (a search still running is stopped and no longer waited for).
function reset() {
  if (S.running) go.Find.Stop(S.runId);
  S.finishResolve?.();
  S.items = []; S.files = 0; S.dirs = 0; S.cur = -1; S.sel = new Set(); S.anchor = -1;
  S.last = null; S.progress = null; S.running = false; S.runId = 0;
}

function stopSearch() {
  if (S.running) go.Find.Stop(S.runId);
}

// stopAll is Stop (Cancel, Esc) while something runs: a tab's own work (an AI run) and the search.
function stopAll() {
  busyTab()?.stop?.();
  stopSearch();
}

function addItems(items) {
  for (const it of items) { S.items.push(it); if (it.f) S.dirs++; else S.files++; }
  if (S.cur < 0 && S.items.length) { S.cur = 0; S.anchor = 0; }
  scheduleRender();
}

function finished(d) {
  S.running = false;
  S.last = d;
  S.finishResolve?.();
  if (S.box) {
    syncEnabled();
    render();
    if (S.items.length && document.activeElement?.closest?.('.mc-find') === S.box) $('.mc-find-list')?.focus();
  }
}

let listening = false;
function listen() {
  if (listening) return;
  listening = true;
  const stash = (id) => { if (!inbox.has(id)) inbox.set(id, { items: [], done: null }); return inbox.get(id); };
  on('find-results', (m) => {
    if (m.id === S.runId) addItems(m.items || []);
    else if (m.id > S.runId) stash(m.id).items.push(m.items || []);
  });
  on('find-progress', (m) => { if (m.id === S.runId && S.running) { S.progress = m.progress; renderStatus(); } });
  on('find-done', (d) => {
    if (d.id === S.runId) finished(d);
    else if (d.id > S.runId) stash(d.id).done = d;
  });
}

// ---- drawing the results ----

let raf = 0;
function scheduleRender() { if (!raf) raf = requestAnimationFrame(() => { raf = 0; render(); }); }

function render() {
  if (!S.box) return;
  const list = $('.mc-find-list');
  const sp = list.querySelector('.sp'), rows = list.querySelector('.rows'), empty = list.querySelector('.empty');
  sp.style.height = `${S.items.length * ROW_H}px`;
  const first = Math.max(0, Math.floor(list.scrollTop / ROW_H) - 3);
  const last = Math.min(S.items.length, first + Math.ceil((list.clientHeight || 300) / ROW_H) + 7);
  let h = '';
  for (let i = first; i < last; i++) {
    const it = S.items[i];
    let cls = 'r';
    if (i === S.cur) cls += ' cur';
    if (S.sel.has(i)) cls += ' sel';
    if (it.f) cls += ' dir';
    if (it.g) { cls += it.g % 2 ? ' g1' : ' g0'; if (i > 0 && S.items[i - 1].g !== it.g) cls += ' gs'; }
    const name = it.f ? `[${it.n}]` : it.n;
    h += `<div class="${cls}" data-i="${i}" title="${esc(it.p)}"><span class="n">${esc(name)}</span><span class="d"><bdi>${esc(it.d)}</bdi></span>`
      + `<span class="s">${it.f ? '' : esc(size(it.s))}</span><span class="t">${esc(date(it.t))}</span>${it.g ? `<span class="g">${it.g}</span>` : ''}</div>`;
  }
  rows.style.transform = `translateY(${first * ROW_H}px)`;
  rows.innerHTML = h;
  empty.hidden = !(S.last && !S.items.length);
  empty.textContent = 'No files found.';
  renderStatus();
}

function statusText() {
  if (S.running) {
    const where = S.progress?.dir ? `Searching ${S.progress.dir}` : 'Searching…';
    return `${where} — ${plural(S.items.length, 'result')}`;
  }
  const d = S.last;
  if (!d) return S.items.length ? `${plural(S.files, 'file')} and ${plural(S.dirs, 'folder')}` : '';
  if (d.error) return d.error;
  let t = S.items.length ? `${plural(S.files, 'file')} and ${plural(S.dirs, 'folder')} found` : 'No files found';
  if (d.groups) t += ` in ${plural(d.groups, 'group')} of duplicates`;
  if (d.stopped) t = `Search stopped: ${t}`;
  if (d.limit) t = `Search stopped at ${plural(d.limit, 'result')} (Options > Find files): ${t}`;
  if (d.errors) t += `; ${plural(d.errors, 'folder or file', 'folders or files')} could not be read`;
  return t + '.';
}

function renderStatus() {
  const el = $('.mc-find-status');
  if (!el) return;
  el.textContent = statusText();
  el.title = (S.last?.problems || []).map((x) => `${x.path}: ${x.err}`).join('\n');
}

function ensureVisible() {
  const list = $('.mc-find-list');
  if (!list || S.cur < 0) return;
  const y = S.cur * ROW_H, view = list.clientHeight;
  if (y < list.scrollTop) list.scrollTop = y;
  else if (y + ROW_H > list.scrollTop + view) list.scrollTop = y + ROW_H - view;
}

function moveTo(i, { shift = false, toggle = false } = {}) {
  if (!S.items.length) return;
  i = Math.max(0, Math.min(S.items.length - 1, i));
  if (toggle) {
    if (S.sel.has(i)) S.sel.delete(i); else S.sel.add(i);
    S.anchor = i;
  } else if (shift && S.anchor >= 0) {
    S.sel = new Set();
    const [a, b] = S.anchor < i ? [S.anchor, i] : [i, S.anchor];
    for (let k = a; k <= b; k++) S.sel.add(k);
  } else {
    S.sel = new Set([i]);
    S.anchor = i;
  }
  S.cur = i;
  ensureVisible();
  render();
}

// targets: the selected results, or the one under the cursor.
function targets() {
  if (S.sel.size) return [...S.sel].sort((a, b) => a - b).map((i) => S.items[i]).filter(Boolean);
  return S.cur >= 0 && S.items[S.cur] ? [S.items[S.cur]] : [];
}

// ---- actions on results ----

// gotoItem shows a result in the active panel with the cursor on it (F-539). A folder that is gone gives the nearest
// folder above that is still there.
async function gotoItem(it) {
  const p = theApp.cur;
  let dir = it.d, focus = it.n;
  for (let n = 0; n < 30 && dir; n++) {
    if (await p.go(dir, { focus })) { p.$.list.focus(); return true; }
    const up = await go.FS.Parent(dir).catch(() => '');
    if (!up || up === dir) break;
    focus = dir.replace(/[\\/]+$/, '').split(/[\\/]/).pop();
    dir = up;
  }
  return false;
}

async function gotoFile() {
  const it = S.items[S.cur] || targets()[0];
  if (!it) return;
  S.resume = readForm();
  S.done?.('goto');
  await gotoItem(it);
}

// view / edit run the viewer and editor commands with the result paths (args.path, args.paths), F-540.
// A file found inside an archive is viewed from a temporary copy (cm_List unpacks it through FS.LocalCopy); it can't be
// edited from here yet. The result list gets the keys back when the Lister or the editor call is done.
async function viewEdit(cmd) {
  const files = targets().filter((x) => !x.f);
  if (!files.length) { theApp.flash('Folders can\'t be viewed; choose a file.', 'err'); return; }
  if (cmd === 'cm_Edit' && files[0].a) { theApp.flash('Editing a file inside an archive is not ready yet. View it (F3), or use Go to file.', 'err'); return; }
  const box = S.box;
  const modeless = isModeless();
  // (N / P in the Lister walk the selected results, or all the found files when one is viewed)
  const walk = files.length > 1 ? files : S.items.filter((x) => !x.f);
  await theApp.run(cmd, { path: files[0].p, paths: files.map((x) => x.p), sources: walk.map((x) => x.p), from: 'find' });
  const a = document.activeElement;
  if (box && S.box === box && ((!a?.closest('.mc-modal') && !modeless) || box.contains(a))) box.querySelector('.mc-find-list')?.focus({ preventScroll: true });
}

async function feed() {
  if (!S.items.length || S.running) return;
  const p = theApp.cur;
  let info;
  const back = isFeed(p.path) ? p.parent : p.path; // (a list fed over a list goes back where the first one came from)
  try { info = await go.Find.Feed(S.runId, back); } catch (e) { showErr(errText(e)); return; }
  feeds.set(info.path, info.base);
  S.resume = null;
  S.done?.('feed');
  if (await p.go(info.path)) {
    p.$.list.focus();
    theApp.flash(info.note || `${plural(info.count, 'result')} in the panel. Backspace goes back.`);
  }
}

async function copyList() {
  const list = (S.sel.size > 1 ? targets() : S.items).map((x) => x.p);
  try { await go.Find.CopyText(list, false); theApp.flash(`Copied ${plural(list.length, 'name')}.`); } catch (e) { showErr(errText(e)); }
}

async function saveList() {
  const list = (S.sel.size > 1 ? targets() : S.items).map((x) => x.p);
  const sep = theApp.sep || '/';
  const file = await inputDialog({ title: 'Save list', label: 'Save the list of results to this file:', value: `${theApp.home}${sep}search results.txt`, okLabel: 'Save', kind: 'find-savelist' });
  if (!file) return;
  try { await go.Find.SaveText(file, list, false, false); } catch (e) {
    const msg = errText(e);
    if (!/already there/.test(msg)) { showErr(msg); return; }
    const a = await messageDialog({ Title: 'Save list', Message: `${msg}\nReplace it?`, Buttons: ['Replace', 'Cancel'], Kind: 'find-overwrite' });
    if (a !== 'Replace') return;
    try { await go.Find.SaveText(file, list, false, true); } catch (e2) { showErr(errText(e2)); return; }
  }
  theApp.flash(`Saved ${plural(list.length, 'name')} to ${file}.`);
}

// ---- actions on duplicates (board issue #32, F-862) ----

const trashWord = () => (theApp.platform === 'windows' ? 'Recycle Bin' : theApp.platform === 'darwin' ? 'Trash' : 'trash');

// dupPreview tells what the chosen rule and action would do (the dialog's live summary).
function dupPreview(plan, action) {
  if (!plan.drop.length) return 'There is nothing to do: no group has two files that can be acted on.';
  const copies = plural(plan.drop.length, 'copy', 'copies');
  const groups = plural(plan.groups, 'group');
  let t = action === 'mark' ? `Marks ${copies} in ${groups}; one file of each group stays unmarked.`
    : action === 'link' ? `Replaces ${copies} in ${groups} with hard links to the kept file. Saves up to ${size(plan.bytes)}.`
      : `Moves ${copies} in ${groups} to the ${trashWord()}. Saves ${size(plan.bytes)}.`;
  if (action === 'link') t += ' Copies whose contents differ from the kept file are left as they are.';
  else if (action === 'trash' && !S.req?.dupContent) t += ' This search did not compare contents, so the files of a group may differ.';
  return t;
}

async function duplicateActions() {
  if (S.running || !hasDuplicates(S.items)) return;
  const ruleRows = DUP_RULES.map(([v, t], k) => `<label class="mc-check"><input type="radio" name="ff-dup-rule" value="${v}"${k ? '' : ' checked'}>${esc(t)}</label>`).join('');
  const acts = [['mark', 'Mark the other copies in the list'], ['trash', `Move the other copies to the ${trashWord()}`], ['link', 'Replace the other copies with hard links']];
  const actRows = acts.map(([v, t], k) => `<label class="mc-check"><input type="radio" name="ff-dup-act" value="${v}"${k ? '' : ' checked'}>${esc(t)}</label>`).join('');
  const pick = (box, name) => box.querySelector(`input[name=${name}]:checked`)?.value;
  const r = await formDialog('Duplicates', `<div class="mc-find-dups"><div class="mc-find-sub">In every group:</div>${ruleRows}
    <div class="mc-find-sub">Then:</div>${actRows}<p class="mc-note ff-dup-preview" aria-live="polite"></p></div>`,
  (box) => {
    const rule = pick(box, 'ff-dup-rule'), action = pick(box, 'ff-dup-act');
    const plan = planDuplicates(S.items, rule);
    if (!plan.drop.length) return 'There is nothing to do.';
    return { rule, action, plan };
  }, (box) => {
    const upd = () => { box.querySelector('.ff-dup-preview').textContent = dupPreview(planDuplicates(S.items, pick(box, 'ff-dup-rule')), pick(box, 'ff-dup-act')); };
    box.querySelectorAll('input[type=radio]').forEach((el) => el.addEventListener('change', upd));
    upd();
  }, { okLabel: 'Continue', kind: 'find-dups' });
  if (!r || !S.box) return;
  const { action, plan } = r;
  if (action === 'mark') {
    S.sel = new Set(plan.drop.map((d) => d.i));
    S.cur = plan.drop[0].i; S.anchor = S.cur;
    ensureVisible(); render();
    $('.mc-find-list')?.focus();
    theApp.flash(`Marked ${plural(plan.drop.length, 'copy', 'copies')} (${size(plan.bytes)}).`);
    return;
  }
  const copies = plural(plan.drop.length, 'copy', 'copies');
  if (action === 'trash') await trashDuplicates(plan, copies);
  else await linkDuplicates(plan, copies);
}

async function trashDuplicates(plan, copies) {
  const T = trashWord();
  if (!(await go.Jobs.TrashSupported().catch(() => false))) { showErr(`There is no ${T} here, so nothing was deleted.`); return; }
  const b = await messageDialog({ Title: 'Duplicates', Message: `Move ${copies} to the ${T}?\n\nOne file of each of ${plural(plan.groups, 'group')} is kept. This frees ${size(plan.bytes)}.`,
    Buttons: [`Move to ${T}`, 'Cancel'], DefaultButton: `Move to ${T}`, Kind: 'find-dups-trash' });
  if (b !== `Move to ${T}`) return;
  const sources = plan.drop.map((d) => S.items[d.i].p);
  try { await runJob({ kind: 'trash', sources, onError: 'ask' }, { title: `Moving to the ${T}` }); } catch (e) { showErr(errText(e)); }
  const gone = await go.Find.Missing(sources).catch(() => []);
  if (!S.box) return;
  forgetResults(gone || []);
  theApp.flash(`Moved ${plural((gone || []).length, 'copy', 'copies')} to the ${T}.`);
}

async function linkDuplicates(plan, copies) {
  const b = await messageDialog({ Title: 'Duplicates', Message: `Replace ${copies} with hard links to the kept files?\n\nEach copy is compared with its kept file first; a copy whose contents differ is left as it is. Afterwards the names share one file: changing one changes them all.`,
    Buttons: ['Replace', 'Cancel'], DefaultButton: 'Replace', Kind: 'find-dups-link' });
  if (b !== 'Replace') return;
  const pairs = plan.drop.map((d) => ({ keep: S.items[d.keep].p, dup: S.items[d.i].p }));
  let res = [];
  try { res = await track(go.Find.LinkDuplicates(pairs)) || []; } catch (e) { showErr(errText(e)); return; }
  const done = res.filter((x) => x.linked), bad = res.filter((x) => x.error);
  const saved = done.reduce((n, x) => n + (x.saved || 0), 0);
  let msg = `Replaced ${plural(done.length, 'copy', 'copies')} with hard links; ${size(saved)} saved.`;
  if (bad.length) {
    msg += `\n\n${plural(bad.length, 'copy was', 'copies were')} left as ${bad.length === 1 ? 'it was' : 'they were'}:\n`
      + bad.slice(0, 8).map((x) => `${x.dup}: ${x.error}`).join('\n') + (bad.length > 8 ? `\n… and ${bad.length - 8} more` : '');
    await messageDialog({ Title: 'Duplicates', Message: msg, Kind: 'find-dups-result' });
  } else theApp.flash(msg);
}

// forgetResults takes results that no longer exist out of the list (after moving copies to the trash).
function forgetResults(gonePaths) {
  if (!gonePaths.length) return;
  S.items = dropGone(S.items, gonePaths);
  S.files = S.items.filter((x) => !x.f).length; S.dirs = S.items.length - S.files;
  S.sel = new Set(); S.cur = S.items.length ? Math.min(Math.max(S.cur, 0), S.items.length - 1) : -1; S.anchor = S.cur;
  syncEnabled(); render();
}

function newSearch() {
  if (S.running) stopSearch();
  S.items = []; S.files = 0; S.dirs = 0; S.cur = -1; S.sel = new Set(); S.last = null; S.progress = null;
  showErr('');
  switchTab('general');
  syncEnabled();
  render();
  const f = $('#ff-for');
  f?.focus();
  f?.select();
}

async function loadSaved() {
  const name = val('ff-saved');
  try {
    const r = await go.Find.LoadSaved(name);
    fill(r, { keepIn: true });
    showErr('');
    switchTab('general');
  } catch (e) { showErr(errText(e)); }
}

async function saveSearch() {
  const cur = val('ff-saved');
  const r = await formDialog('Save search', `<div class="mc-row"><label for="ff-save-name">Save search as:</label><input id="ff-save-name" type="text" spellcheck="false" value="${esc(cur)}" autofocus></div>
    <label class="mc-check"><input id="ff-save-in" type="checkbox">Save the Search in folders too</label>`,
  (box) => {
    const name = box.querySelector('#ff-save-name').value.trim();
    if (!name) return 'Please give the search a name.';
    return { name, keepIn: box.querySelector('#ff-save-in').checked };
  }, null, { okLabel: 'Save', kind: 'find-save' });
  if (!r) return;
  try { await go.Find.Save(r.name, readForm(), r.keepIn); showErr(''); await refreshSaved(r.name); } catch (e) { showErr(errText(e)); }
}

async function deleteSaved() {
  const name = val('ff-saved');
  if (!name) { showErr('The last search can\'t be deleted; it is replaced by the next search.'); return; }
  const a = await messageDialog({ Title: 'Delete saved search', Message: `Delete the saved search "${name}"?`, Buttons: ['Delete', 'Cancel'], Danger: true, Kind: 'find-delete' });
  if (a !== 'Delete') return;
  try { await go.Find.DeleteSaved(name); await refreshSaved(); } catch (e) { showErr(errText(e)); }
}

// ---- keys inside the dialog ----

// The core's modal() takes every key while a dialog is open; this listener runs before it (it is added before the
// dialog opens) and handles the keys of the Find dialog: Esc stops a running search instead of closing, Ctrl+Tab
// switches tabs, and the result list's keys (arrows, Enter, F3, F4, Ctrl+A, Ctrl+C).
// A modeless Find files window (FindFiles.Modeless) takes only the keys aimed at it, not the panels' keys.
const isModeless = () => !!S.box?.parentElement?.classList.contains('mc-modeless');

function isTop(ev) {
  if (isModeless()) return keyOwnerBox(ev.target) === S.box;
  const all = document.querySelectorAll('.mc-modal');
  return S.box && all[all.length - 1] === S.box.parentElement;
}

function onKey(ev) {
  if (!isTop(ev) || ev.isComposing) return;
  const combo = comboFromEvent(ev);
  const stop = () => { ev.preventDefault(); ev.stopImmediatePropagation(); };
  if (combo === 'Esc' && (S.running || busyTab())) { stop(); stopAll(); return; }
  if (extraTabs.some((t) => t.key?.(ev, combo))) { stop(); return; }
  if (combo === 'Ctrl+Tab' || combo === 'Ctrl+PgDn' || combo === 'Ctrl+Shift+Tab' || combo === 'Ctrl+PgUp') {
    stop();
    const d = combo === 'Ctrl+Tab' || combo === 'Ctrl+PgDn' ? 1 : -1;
    const tabs = allTabs();
    switchTab(tabs[(tabs.indexOf(S.tab) + d + tabs.length) % tabs.length]);
    return;
  }
  if (combo === 'Enter' && ev.target?.id === 'ff-saved') { stop(); loadSaved(); return; }
  if (!ev.target?.closest?.('.mc-find-list')) return;
  const page = Math.max(1, Math.floor(($('.mc-find-list').clientHeight || 200) / ROW_H) - 1);
  const shift = ev.shiftKey;
  const keys = {
    Up: () => moveTo(S.cur - 1), Down: () => moveTo(S.cur + 1), PgUp: () => moveTo(S.cur - page), PgDn: () => moveTo(S.cur + page),
    Home: () => moveTo(0), End: () => moveTo(S.items.length - 1),
    'Shift+Up': () => moveTo(S.cur - 1, { shift }), 'Shift+Down': () => moveTo(S.cur + 1, { shift }), 'Shift+PgUp': () => moveTo(S.cur - page, { shift }),
    'Shift+PgDn': () => moveTo(S.cur + page, { shift }), 'Shift+Home': () => moveTo(0, { shift }), 'Shift+End': () => moveTo(S.items.length - 1, { shift }),
    Space: () => moveTo(S.cur, { toggle: true }),
    Enter: gotoFile, F3: () => viewEdit('cm_List'), F4: () => viewEdit('cm_Edit'),
    'Ctrl+A': () => { S.sel = new Set(S.items.map((_, i) => i)); render(); }, 'Cmd+A': () => { S.sel = new Set(S.items.map((_, i) => i)); render(); },
    'Ctrl+C': copyList, 'Cmd+C': copyList, 'Ctrl+Ins': copyList,
  };
  if (combo === 'Ins') { stop(); if (S.sel.has(S.cur)) S.sel.delete(S.cur); else S.sel.add(S.cur); S.cur = Math.min(S.items.length - 1, S.cur + 1); ensureVisible(); render(); return; }
  const f = keys[combo];
  if (f) { stop(); f(); }
}

// ---- opening the dialog ----

async function openDialog(app, args = {}) {
  if (S.box) { S.box.querySelector('#ff-for')?.focus(); focusDialog(S.box); return; }
  listen();
  const p = app.cur;
  let setup;
  try { setup = await go.Find.Setup(); } catch { setup = { history: {}, codepages: [], saved: [] }; }
  S.setup = setup;
  S.openSel = isFeed(p.path) ? [] : p.selectedEntries().map((e) => p.fullPath(e.name));
  const resume = S.resume;
  S.resume = null;
  if (!resume) reset();
  S.tab = 'general';
  window.addEventListener('keydown', onKey, true);
  const shown = modal((box, done) => {
    S.box = box;
    S.done = done;
    box.classList.add('mc-find');
    box.innerHTML = html(setup);
    attachPathComplete(box.querySelector('#ff-in'), () => app.cur?.path || ''); // (folder names complete, F-782)
    // fields
    if (resume) fill(resume, { keepIn: false });
    else {
      // the last search's fields, or Options > Find files when there is no last search (find-defaults.js)
      const fresh = findDefaults((name) => optValue(name));
      fill(setup.last || fresh, { keepIn: true });
      // hidden files as the last search had them, or as the panels show them when the option says so (issue #4)
      setHidden(findIncludeHidden({ last: setup.last, followPanel: optOn('FindFiles.HiddenFromPanel'), showHidden: app.showHidden, fresh: !fresh.skipHidden }));
      setVal('ff-for', setup.history?.name?.[0] || setup.last?.searchFor || '*');
      // (from a branch view the search starts in the folder it lists: its subfolders are searched anyway)
      setVal('ff-in', /^branch(sel)?:\/\//i.test(p.path) && p.parent ? p.parent : p.path);
    }
    const f = box.querySelector('#ff-for');
    f.dataset.select = `0,${f.value.length}`;
    go.Find.Volumes().then((vols) => {
      const sel = box.querySelector('#ff-drives');
      if (!sel || !vols?.length) return;
      sel.innerHTML = '<option value="">Volumes…</option>' + `<option value="${esc(vols.join(';'))}">All local volumes</option>` + vols.map((v) => `<option value="${esc(v)}">${esc(v)}</option>`).join('');
    }).catch(() => {});
    // events
    box.onsubmit = (ev) => {
      ev.preventDefault();
      const t = extraTabs.find((x) => x.id === S.tab && x.start);
      if (t) { if (!S.running && !busyTab()) t.start(); } else startSearch();
    };
    box.querySelectorAll('.mc-find-tab').forEach((b) => { b.onclick = () => switchTab(b.dataset.tab); });
    box.querySelector('#ff-hidden').onchange = (ev) => setChk('ff-skiphidden', !ev.target.checked);
    box.querySelector('#ff-skiphidden').onchange = (ev) => setChk('ff-hidden', !ev.target.checked);
    box.querySelector('.ff-cancel').onclick = () => { if (S.running || busyTab()) stopAll(); else done(null); };
    box.querySelector('.ff-browse').onclick = async () => {
      const first = (val('ff-in').split(';')[0] || p.path).replace(/^"|"$/g, '');
      const dir = await go.App.PickFolder('Search in', first).catch(() => '');
      if (dir) setVal('ff-in', dir);
    };
    box.querySelector('#ff-drives').onchange = (ev) => { if (ev.target.value) { setVal('ff-in', ev.target.value); ev.target.value = ''; } };
    box.querySelector('#ff-text').addEventListener('input', () => setChk('ff-texton', val('ff-text') !== ''));
    box.querySelector('#ff-dateon').addEventListener('change', () => { if (chk('ff-dateon')) setChk('ff-ageon', false); });
    box.querySelector('#ff-ageon').addEventListener('change', () => { if (chk('ff-ageon')) setChk('ff-dateon', false); });
    for (const id of ['ff-hex', 'ff-selected', 'ff-results']) box.querySelector(`#${id}`).addEventListener('change', syncEnabled);
    box.querySelector('.ff-new').onclick = newSearch;
    box.querySelector('.ff-goto').onclick = gotoFile;
    box.querySelector('.ff-view').onclick = () => viewEdit('cm_List');
    box.querySelector('.ff-edit').onclick = () => viewEdit('cm_Edit');
    box.querySelector('.ff-feed').onclick = feed;
    box.querySelector('.ff-copy').onclick = copyList;
    box.querySelector('.ff-savelist').onclick = saveList;
    box.querySelector('.ff-dups').onclick = duplicateActions;
    box.querySelector('.ff-load').onclick = loadSaved;
    box.querySelector('.ff-save').onclick = saveSearch;
    box.querySelector('.ff-delete').onclick = deleteSaved;
    box.querySelector('#ff-saved').addEventListener('dblclick', loadSaved);
    const list = box.querySelector('.mc-find-list');
    list.addEventListener('scroll', scheduleRender);
    list.addEventListener('mousedown', (ev) => {
      const row = ev.target.closest('.r');
      if (!row) return;
      moveTo(+row.dataset.i, { shift: ev.shiftKey, toggle: ev.ctrlKey || ev.metaKey });
    });
    list.addEventListener('dblclick', (ev) => { if (ev.target.closest('.r')) gotoFile(); });
    tabHook('setup', box);
    syncEnabled();
    requestAnimationFrame(render);
    if (args.fields) for (const [id, v] of Object.entries(args.fields)) { const el = box.querySelector(`#${CSS.escape(id)}`); if (el) { if (el.type === 'checkbox') el.checked = !!v; else el.value = v; } }
    if (args.start) queueMicrotask(startSearch);
  }, null, { kind: 'find', modeless: args.modeless === true || optOn('FindFiles.Modeless') });
  shown.then(() => {
    window.removeEventListener('keydown', onKey, true);
    if (S.running) stopSearch();
    S.box = null;
    S.done = null;
    tabHook('closed');
  });
  return true; // (the dialog stays open; the command is done)
}

// ---- for other features (the AI tab's find_files, find-ai.js) ----

// findDialog lets a feature use the open dialog: search(req) puts a request in the fields and runs it as Start search
// would, so the results fill the list and the status line; it answers {items, done, error} when the search is over.
// A search that runs longer than opts.timeout ms is stopped (what it found so far stays). sync() redraws the buttons
// after a tab's busy() changed; `box` is the open dialog (null when closed).
export const findDialog = {
  get box() { return S.box; },
  get tab() { return S.tab; },
  get running() { return S.running; },
  sync: () => { if (S.box) syncEnabled(); },
  showError: (t) => showErr(t),
  status: () => statusText(),
  async search(req, { timeout = 0 } = {}) {
    if (!S.box) return { items: [], done: null, error: 'The Find files dialog is closed.' };
    if (S.running) { stopSearch(); await S.finish; }
    fill(req, { keepIn: false });
    const error = await startSearch(req);
    if (error) return { items: [], done: null, error };
    const id = S.runId;
    let timer = 0;
    if (timeout > 0) timer = setTimeout(() => { if (S.runId === id) stopSearch(); }, timeout);
    await S.finish;
    clearTimeout(timer);
    if (S.runId !== id) return { items: [], done: null, error: 'Another search replaced this one.' };
    return { items: S.items.slice(), done: S.last, error: '' };
  },
};

// ---- commands ----

command({ name: 'cm_SearchFor', label: 'Find files…', short: 'Find', run: openDialog });
// cm_SearchStandalone (Commands > Find files in a new window, board issue #38): Total Commander starts Find files as a
// process of its own, so the main window stays usable while it searches. Here the same dialog opens modeless, in a
// window of its own, whatever Options > Find files > Modeless says; the search already runs as a background job in Go.
// With dialogs shown inside the main window (Options: dialogs in windows off) there is no window to put it in, and the
// dialog opens as usual. An open Find files dialog is brought to the front instead of opening a second one.
command({ name: 'cm_SearchStandalone', label: 'Find files in a new window…', run: (app, args) => openDialog(app, { ...(args || {}), modeless: true }) });

// ---- control mode ----

addControlCommand('find', async (m) => {
  if (m.wait && S.finish) await S.finish;
  return {
    ok: true, open: !!S.box, running: S.running, id: S.runId, tab: S.tab, status: statusText(), count: S.items.length,
    files: S.files, folders: S.dirs, cursor: S.cur, selected: [...S.sel],
    results: S.items.slice(0, m.limit || 5000).map((x) => (x.g ? { path: x.p, group: x.g, dir: !!x.f } : x.f ? { path: x.p, dir: true } : x.p)),
    feeds: Object.fromEntries(feeds),
  };
});

export function init(app) {
  listen();
  // a result list shown again after Ctrl+U, tabs or history needs its base folder: ask Go when we don't know it
  app.on('panel', (p) => {
    if (isFeed(p.path) && !feeds.has(p.path)) {
      go.Find.FeedBase(p.path).then((base) => { feeds.set(p.path, base ?? ''); p.rebuild(); }).catch(() => {});
    }
  });
}
