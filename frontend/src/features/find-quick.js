// Quick search and quick filter in a panel (FEATURES.md F-066..F-070, F-068, F-274, F-275, F-286, F-780, F-781, F-801;
// spec docs/spec/area-2.md sections 7 and 8).
//
// Quick search: Ctrl+Alt+letter (TC's default; Alt+letter or plain letters by setting) opens a small box under the
// panel and puts the cursor on the first name that matches; more letters narrow it (a letter that matches nothing is
// not taken), Up/Down go to the previous/next match, Enter opens, Esc closes, any other key closes the box and does
// its own job. Quick filter (Ctrl+S, Ctrl+Shift+S with the last text): the same box, but the panel shows only the
// matching names; Enter keeps the filter, Esc (or Ctrl+F10, or another folder) removes it. Ctrl+S in the box switches
// between the two.
//
// Matching is done in Go (feature_find_quick.go over internal/masks): the panel's names are sent once, then each
// keystroke asks for the matching indexes. Options, settings key "quickSearch":
//   {mode: 'ctrlalt' | 'alt' | 'letter' | 'off' (or TC's 0..3), beginning: true, matchEnd: false, wildcards: true,
//    ignoreAccents: true, kind: '' | 'dirs' | 'files', filterDirs: true, autoFilter: false}

import { command, commandForKey } from '../core/registry.js';
import { addKeyHandler } from '../core/keyboard.js';
import { go } from '../core/go.js';
import { cmdline } from '../core/cmdline.js';
import { printable } from '../core/keys.js';
import { trigger } from './find-quick-keys.js';
import { track } from '../core/busy.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { isBranchPath, leafName } from './branch-lib.js';

const MODES = ['ctrlalt', 'alt', 'letter', 'off'];

function options() {
  const o = theApp.setting('quickSearch', {}) || {};
  let mode = o.mode ?? 'ctrlalt';
  if (typeof mode === 'number') mode = MODES[mode] || 'ctrlalt';
  return {
    mode, beginning: o.beginning ?? true, matchEnd: !!o.matchEnd, wildcards: o.wildcards ?? true, ignoreAccents: o.ignoreAccents ?? true,
    kind: o.kind || '', filterDirs: o.filterDirs ?? true, autoFilter: !!o.autoFilter,
  };
}

const QS = {
  panel: null,        // the panel the box is open on (null: closed)
  filter: false,      // filter mode
  text: '',
  el: null,
  chain: Promise.resolve(),
  loaded: new Map(),  // key → the entries array whose names Go has
  last: new Map(),    // side → the last quick filter text (Ctrl+Shift+S)
};

// queue runs async steps one after the other, in key order; control mode waits for them.
function queue(fn) {
  QS.chain = QS.chain.then(fn).catch((e) => console.warn('quick search:', e));
  track(QS.chain);
  return QS.chain;
}

const nameOf = (e) => (e.up ? '..' : e.dir ? `${e.name}/` : e.name);
// In a branch view the names hold folders ("sub/deeper/a.txt"); quick search looks at the file's own name there, as in
// TC, so typing "a" finds a.txt in any subfolder.
const branchNameOf = (e) => (e.up ? '..' : leafName(e.name, theApp.platform));

// match asks Go which names of the panel match the text: in search mode among the shown entries (the indexes are
// cursor positions), in filter mode among all the folder's entries.
async function match(p, text, filter) {
  const list = filter ? p.all : p.entries;
  const key = `${p.side}:${filter ? 'all' : 'shown'}`;
  if (QS.loaded.get(key) !== list) {
    await go.Find.QuickLoad(key, list.map(isBranchPath(p.path) ? branchNameOf : nameOf));
    QS.loaded.set(key, list);
  }
  const o = options();
  const idx = await go.Find.QuickMatch(key, text, { filter, beginning: o.beginning, matchEnd: o.matchEnd, wildcards: o.wildcards, ignoreAccents: o.ignoreAccents, kind: o.kind, filterDirs: o.filterDirs });
  return { list, idx: idx || [] };
}

// ---- the box ----

function draw() {
  const p = QS.panel;
  if (!p) return;
  if (!QS.el) {
    QS.el = document.createElement('div');
    QS.el.className = 'mc-find-qs';
    QS.el.setAttribute('role', 'search');
    QS.el.innerHTML = '<span class="l"></span><span class="t"></span><span class="c"></span>';
  }
  if (QS.el.parentElement !== p.el) p.el.insertBefore(QS.el, p.$.status);
  QS.el.classList.toggle('filter', QS.filter);
  QS.el.querySelector('.l').textContent = QS.filter ? 'Filter:' : 'Search:';
  QS.el.querySelector('.t').textContent = QS.text;
  QS.el.title = QS.filter ? 'Esc removes the filter, Enter keeps it, Ctrl+S switches to search' : 'Up/Down: other matches, Enter opens, Esc closes, Ctrl+S filters';
}

function open(p, filter, text = '') {
  QS.panel = p;
  QS.filter = filter;
  QS.text = text;
  draw();
}

// close hides the box; in filter mode keep says whether the filter stays.
function close(keep = true) {
  const p = QS.panel;
  if (!p) return;
  if (QS.filter && !keep) clearFilter(p);
  QS.panel = null;
  QS.text = '';
  QS.el?.remove();
}

const isQuick = (p) => p.filter?.mcQuick !== undefined;

function clearFilter(p) {
  if (isQuick(p)) p.setFilter(null);
}

// applyFilter shows only the names that match QS.text (no text: no filter).
async function applyFilter(p, text) {
  if (!text) { clearFilter(p); return; }
  const { list, idx } = await match(p, text, true);
  const names = new Set(idx.map((i) => list[i].name));
  const fn = (e) => names.has(e.name);
  fn.mcQuick = text;
  fn.mcAll = p.all;
  const curName = p.cur?.name;
  p.setFilter(fn, `*${text}*`);
  QS.last.set(p.side, text);
  // the cursor goes to the first match unless it is on one already
  if (!curName || !names.has(curName)) { const i = p.entries.findIndex((e) => !e.up); if (i >= 0) p.setCursor(i); }
}

// typeChar adds a character: in search mode only when something still matches (else it is not taken).
function typeChar(p, c, opening) {
  return queue(async () => {
    if (!opening && QS.panel !== p) return;
    const text = QS.text + c;
    const filter = opening ? opening.filter : QS.filter;
    if (filter) {
      if (opening) open(p, true, '');
      QS.text = text;
      draw();
      await applyFilter(p, text);
      return;
    }
    const { idx } = await match(p, text, false);
    if (!idx.length) return;
    if (opening) open(p, false, '');
    if (QS.panel !== p) return;
    QS.text = text;
    draw();
    p.setCursor(idx[0]);
  });
}

function backspace(p) {
  return queue(async () => {
    if (QS.panel !== p) return;
    QS.text = [...QS.text].slice(0, -1).join('');
    draw();
    if (QS.filter) { await applyFilter(p, QS.text); return; }
    if (!QS.text) return;
    const { idx } = await match(p, QS.text, false);
    if (idx.length) p.setCursor(idx[0]);
  });
}

// step goes to the next (d = 1) or previous (d = -1) match from the cursor; no wrap.
function step(p, d) {
  return queue(async () => {
    if (QS.panel !== p || !QS.text) return;
    const { idx } = await match(p, QS.text, false);
    const next = d > 0 ? idx.find((i) => i > p.cursor) : [...idx].reverse().find((i) => i < p.cursor);
    if (next !== undefined) p.setCursor(next);
  });
}

function toggleMode(p) {
  return queue(async () => {
    if (QS.panel !== p) return;
    QS.filter = !QS.filter;
    draw();
    if (QS.filter) await applyFilter(p, QS.text);
    else {
      clearFilter(p);
      if (QS.text) { const { idx } = await match(p, QS.text, false); if (idx.length) p.setCursor(idx[0]); }
    }
  });
}

// ---- keys ----

function onKey(ev, combo) {
  const t = ev.target;
  const inCmd = t?.id === 'cmd';
  const p = theApp.cur;
  const o = options();

  if (QS.panel) {
    if (QS.panel !== p || inCmd) { close(true); return false; }
    if (!combo) return false;
    if (combo === 'Esc') { close(false); return true; }
    if (combo === 'Enter') {
      const filter = QS.filter;
      queue(async () => { close(true); if (!filter) await theApp.run('cm_Enter'); });
      return true;
    }
    if (combo === 'Backspace') { backspace(p); return true; }
    if (combo === 'Ctrl+S') { toggleMode(p); return true; }
    if (!QS.filter && (combo === 'Down' || combo === 'Up')) { step(p, combo === 'Down' ? 1 : -1); return true; }
    if (QS.filter && ['Up', 'Down', 'PgUp', 'PgDn', 'Home', 'End', 'Ins', 'Shift+Up', 'Shift+Down'].includes(combo)) return false; // (moving in the filtered list keeps the box)
    const typed = (printable(ev) && ev.key?.length === 1) ? ev.key : trigger(ev, combo, o.mode);
    if (typed) { typeChar(p, typed); return true; }
    close(true);
    return false; // the key does its own job
  }

  if (o.mode === 'off' || inCmd || !p || !combo) return false;
  // Ctrl+F10 (all files) also removes a quick filter (F-781)
  if (combo === 'Ctrl+F10' && isQuick(p)) { clearFilter(p); return true; }
  const c = trigger(ev, combo, o.mode);
  if (!c) return false;
  if (o.mode === 'letter' && (cmdline.text || commandForKey(combo, true))) return false;
  if (o.mode !== 'letter' && commandForKey(combo)) return false; // a key the user bound to a command
  typeChar(p, c, { filter: o.autoFilter });
  return true;
}

// ---- commands ----

command({
  // TC's cm_ShowQuickSearch: the quick search box, empty, on the active panel.
  name: 'cm_ShowQuickSearch', label: 'Quick search',
  run: (app) => { close(true); open(app.cur, options().autoFilter, ''); },
});
command({
  // Ctrl+S: the box in filter mode (with the filter's text when one is on).
  name: 'cm_QuickFilter', label: 'Quick filter',
  run: (app, args = {}) => {
    const p = app.cur;
    const text = args.text ?? (isQuick(p) ? p.filter.mcQuick : '');
    close(true);
    open(p, true, text);
    if (text) return queue(() => applyFilter(p, text));
  },
  checked: (app) => isQuick(app.cur),
});
command({
  // Ctrl+Shift+S: the box in filter mode with the last filter text of this panel, applied at once (F-780).
  name: 'cm_QuickFilterLast', label: 'Quick filter with the last text',
  run: (app) => {
    const p = app.cur;
    const text = QS.last.get(p.side) || '';
    close(true);
    open(p, true, text);
    if (text) return queue(() => applyFilter(p, text));
  },
});

addControlCommand('quicksearch', async () => {
  await QS.chain;
  return { ok: true, open: !!QS.panel, side: QS.panel?.side ?? null, text: QS.text, filter: QS.filter, mode: options().mode,
    filterText: { left: theApp.left.filter?.mcQuick ?? null, right: theApp.right.filter?.mcQuick ?? null } };
});

export function init(app) {
  addKeyHandler(onKey);
  app.on('active', () => close(true));
  app.on('panel', (p, info = {}) => {
    if (info.moved && QS.panel === p) close(true);
    // a reread under a quick filter: match the new names again (Ctrl+R keeps the filter, spec area 2, 8.1)
    if (!info.moved && isQuick(p) && p.filter.mcAll !== p.all) queue(() => applyFilter(p, p.filter.mcQuick));
  });
  document.addEventListener('mousedown', (ev) => { if (QS.panel && !ev.target.closest?.('.mc-find-qs')) close(true); }, true);
}
