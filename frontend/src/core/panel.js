// One file panel: its folder, entries, cursor, selection, sort and history, and how it is drawn.
//
// The list is virtual: only the rows on screen (plus a few) are in the DOM, inside a tall spacer, so a folder of 100,000
// files scrolls as smoothly as one of ten (F-742). Rows are drawn as one HTML string with escaped names, and one
// delegated handler per panel takes the mouse.
//
// Features use the panel through its methods (go, reload, setCursor, select..., targets) and the app's events
// ('panel', 'cursor', 'selection'), never by poking the DOM. See docs/ARCHITECTURE.md for the list.

import { go, errText } from './go.js';
import { F, splitExt, date, attr, size, plural, cellSize, display, footerSize, dosName } from './format.js';
import { sortEntries } from './sort.js';
import { fitCrumbs } from './fitpath.js';
import { track } from './busy.js';
import { esc } from './dialogs.js';
import { t } from './i18n.js';
import { hiddenMarkHTML } from './hidden.js';
import { historyFocus } from './histfocus.js';
import { attachMiddlePan } from './midpan.js';
import { attachPathComplete } from './pathcomplete.js';
import { usedPercent } from '../features/drives-lib.js';

// usageBar is the thin "how full" bar under a drive button (Options > Layout, off by default: F-714).
function usageBar(on, v) {
  const pct = on ? usedPercent(v) : null;
  return pct == null ? '' : `<i class="mc-drive-use${pct >= 90 ? ' full' : ''}" style="--use:${pct}%" aria-hidden="true"></i>`;
}

// The columns of the Full view: id, title, extra class. Sort commands per column live in the sorting feature.
export const COLUMNS = [['name', 'Name'], ['ext', 'Ext'], ['size', 'Size', 'r'], ['date', 'Date'], ['attr', 'Attr']];
const SORT_CMD = { name: 'cm_SrcByName', ext: 'cm_SrcByExt', size: 'cm_SrcBySize', date: 'cm_SrcByDateTime' };

// toEntry turns a row from Go ([name, size, mtime, flags, mode]) into the object the panel keeps.
export function toEntry(row) {
  const [name, sz, mtime, flags, mode] = row;
  const dir = !!(flags & F.DIR);
  const [base, ext] = dir ? [name, ''] : splitExt(name);
  return { name, base, ext, size: sz, mtime, flags, mode, dir, link: !!(flags & F.LINK), hidden: !!(flags & F.HIDDEN), exec: !!(flags & F.EXEC), broken: !!(flags & F.BROKEN) };
}

const UP = { name: '..', base: '..', ext: '', size: 0, mtime: 0, flags: F.DIR, mode: 0, dir: true, up: true };
// ".." in a root folder (Options > Display > Show .. in root folders): it opens the drive list instead of a parent
// (or, with Display > ".. in a drive's root opens the drive list as a folder", the drives:// folder: F-716)
const UP_ROOT = { ...UP, root: true };

// splitPath cuts a path into the parts the path bar shows, each with the path it stands for.
export function splitPath(p) {
  const m = /^([a-z][a-z0-9+.-]*:\/\/[^/]*\/?)(.*)$/i.exec(p); // ftp://host/a/b
  let head = '', rest = p, sep = '/';
  if (m) { head = m[1].endsWith('/') ? m[1] : m[1] + '/'; rest = m[2]; }
  else if (/^[a-z]:\\/i.test(p) || (p.includes('\\') && !p.includes('/'))) { sep = '\\'; const i = p.indexOf('\\'); head = p.slice(0, i + 1); rest = p.slice(i + 1); }
  else if (p.startsWith('/')) { head = '/'; rest = p.slice(1); }
  const parts = [{ label: head, path: head || p }];
  let acc = head;
  const names = rest.split(sep).filter(Boolean);
  names.forEach((n, i) => {
    acc = acc && !acc.endsWith(sep) && !acc.endsWith('/') ? acc + sep + n : acc + n;
    parts.push({ label: n + (i < names.length - 1 ? sep : ''), path: acc });
  });
  return parts.filter((x) => x.label);
}

export class Panel {
  constructor(app, side, el) {
    this.app = app;
    this.side = side;          // "left" or "right"
    this.el = el;
    this.path = '';
    this.parent = '';
    this.all = [];             // every entry of the folder, as read
    this.entries = [];         // what is shown: filtered, sorted, ".." first
    this.index = new Map();    // name → index in entries
    this.cursor = 0;
    this.selected = new Set(); // names
    this.saved = [];           // names kept by saveSelection(), before each file job and by cm_SaveSelection (Num / brings them back)
    this.sort = { key: 'name', desc: false };
    this.back = [];            // folders for Alt+Left
    this.fwd = [];             // and Alt+Right
    this.lastCursor = new Map(); // folder → the name the cursor was on when the folder was left (F-074)
    this.filter = null;        // a function entries must pass (quick filter, Ctrl+F11 programs...), or null
    this.filterLabel = '';     // shown in the path bar while a filter is on
    this.dirSizes = new Map(); // folder name → calculated size (Space, Alt+Shift+Enter)
    this.loading = false;
    this.error = '';
    this.waiting = ''; // "Connecting…" while a network folder opened at once is read (_go)
    this.anchor = -1;          // where a Shift range selection starts
    this.seq = 0;
    this.rowH = 20;
    this.build();
  }

  get other() { return this.app.panels.find((p) => p !== this); }
  get isActive() { return this.app.cur === this; }
  get cur() { return this.entries[this.cursor]; }

  // translateHeader writes the words of the column header (and their tooltips) in the interface language (#113); the
  // header is built once, so a language file read later calls it again.
  translateHeader() {
    for (const [id, word] of COLUMNS) {
      const el = this.$?.header?.querySelector(`[data-col="${id}"]`);
      if (!el) continue;
      el.textContent = t(word);
      el.title = t('Sort by {column} (Ctrl/Cmd+click: as the second sort key)', { column: t(word).toLowerCase() });
    }
  }

  build() {
    const cols = COLUMNS.map(([id, , c]) => `<span data-col="${id}" class="${c || ''}"></span>`).join('');
    this.el.innerHTML = `<div class="drivebar" role="toolbar" aria-label="Drives"></div><div class="driveinfo"></div><div class="tabbar" hidden></div>
      <div class="pathbar"><span class="crumbs"></span><span class="filter" hidden></span><span class="pbtns"></span><button class="hist" title="Folders you visited (Alt+Down)" aria-label="Folder history">▾</button></div>
      <div class="header">${cols}</div>
      <div class="list" tabindex="0" role="listbox" aria-label="Files"><div class="spacer"></div><div class="rows"></div><div class="msg" hidden></div></div>
      <div class="status"></div>`;
    const $ = (s) => this.el.querySelector(s);
    this.$ = { drives: $('.drivebar'), driveinfo: $('.driveinfo'), tabbar: $('.tabbar'), pathbar: $('.pathbar'), crumbs: $('.crumbs'), filter: $('.filter'),
      pbtns: $('.pbtns'), hist: $('.hist'), header: $('.header'), list: $('.list'), spacer: $('.spacer'), rows: $('.rows'), msg: $('.msg'), status: $('.status') };
    this.translateHeader();
    this.measure();
    const { list, header, crumbs, pathbar, hist } = this.$;
    let raf = 0;
    list.addEventListener('scroll', () => { if (!raf) raf = requestAnimationFrame(() => { raf = 0; this.renderRows(); }); });
    new ResizeObserver(() => this.renderRows()).observe(list);
    // a long path is shortened at the start to fit the path bar, again whenever the bar's width changes (issue #129)
    new ResizeObserver(() => fitCrumbs(crumbs)).observe(crumbs);
    list.addEventListener('mousedown', (ev) => this.onMouseDown(ev));
    // middle button held: drag the list up and down with a grab cursor; a middle click without moving goes to
    // Panel.middleClick (Options > Drag & drop > Middle click on a folder: features/mouseopts.js)
    attachMiddlePan(list, undefined, (ev) => Panel.middleClick?.(this, ev));
    // onRow: whether the click was on a row; taken from the event's own target, which is what lies under the pointer
    // for a real click and stays right for control mode's clicks in a window that is hidden or covered (issue #42)
    list.addEventListener('contextmenu', (ev) => {
      ev.preventDefault();
      const args = { x: ev.clientX, y: ev.clientY, mouse: true, onRow: !!ev.target?.closest?.('.row') };
      if (Panel.contextGate?.(this, ev, args)) return; // (the right button selects, or drags: features/mouseopts.js)
      this.app.run('cm_ContextMenu', args, { quiet: true });
    });
    header.addEventListener('click', (ev) => {
      const c = ev.target.closest('[data-col]')?.dataset.col;
      if (!c || !SORT_CMD[c]) return;
      this.app.setActive(this);
      if (ev.ctrlKey || ev.metaKey) this.addSortKey(c); // F-114: Ctrl/Cmd+click adds a second sort key
      else this.app.run(SORT_CMD[c]);
    });
    crumbs.addEventListener('click', (ev) => {
      const s = ev.target.closest('[data-p]');
      if (!s) return;
      ev.stopPropagation();
      this.app.setActive(this);
      this.go(s.dataset.p);
    });
    pathbar.addEventListener('click', (ev) => {
      if (ev.target.closest('button, input, [data-p], .pbtns')) return;
      this.app.setActive(this);
      this.editPath();
    });
    hist.addEventListener('click', (ev) => { ev.stopPropagation(); this.app.setActive(this); this.app.run('cm_DirectoryHistory'); });
    this.$.driveinfo.addEventListener('click', (ev) => {
      if (!ev.target.closest('.mc-drivecombo') || document.body.classList.contains('mc-config-no-drivecombo')) return;
      this.app.setActive(this);
      this.app.run(this.side === 'left' ? 'cm_LeftOpenDrives' : 'cm_RightOpenDrives');
    });
  }

  // measure reads the row height from the CSS (it changes with the font size).
  measure() {
    const h = parseFloat(getComputedStyle(this.$.list).getPropertyValue('--row-h'));
    this.rowH = h > 0 ? h : 20;
  }

  fullPath(name) {
    if (name === '..') return this.parent;
    for (const h of Panel.pathHooks) { const r = h(this, name); if (r) return r; } // virtual folders (search results)
    const sep = this.app.sepFor(this.path);
    return this.path.endsWith(sep) || this.path.endsWith('/') ? this.path + name : this.path + sep + name;
  }

  // ---- reading folders ----

  // go shows a folder. focus: the name to put the cursor on (default: where it was the last time in that folder).
  // history: false for moves through the history itself. restore: true for history moves (Alt+Left/Right, the Alt+Down
  // list), which put the cursor back where it was in that folder, or on the subfolder just left (#126). Resolves true when the folder is shown, false when it could
  // not be read (the panel stays where it was and the reason is shown).
  go(path, opts = {}) { return track(this._go(path, opts)); }

  async _go(path, { focus = null, history = true, keepSelection = false, restore = false } = {}) {
    const seq = ++this.seq;
    this.loading = true;
    // A folder a hook says is slow to open (a network folder: features/network.js) is opened in the panel at once,
    // empty, with "Connecting…" and a spinner in it, and fills when it is read; a failure is told there too. Any other
    // folder keeps the panel as it is (faded) until it is read.
    let early = false;
    if (!Panel.quietLoadHooks.some((h) => h(this, path))) this.el.classList.add('loading');
    else if (path !== this.path) {
      let parent = '';
      try { parent = (await go.FS.Parent(path)) || ''; } catch { /* no ".." until it is read */ }
      if (seq !== this.seq) return false;
      const from = this.path;
      if (from) {
        if (this.cur) this.lastCursor.set(from, this.cur.name);
        if (history) { this.back.push(from); if (this.back.length > 200) this.back.shift(); this.fwd = []; }
      }
      early = true;
      this.path = path;
      this.parent = parent;
      this.error = '';
      this.all = [];
      this.selected = new Set();
      this.dirSizes = new Map();
      this.filter = null;
      this.filterLabel = '';
      this.waiting = 'Connecting…';
      this.rebuild(null, 0);
      this.app.panelChanged(this, { moved: true });
    }
    let l;
    try {
      l = await go.FS.List(this.side, path);
    } catch (e) {
      if (seq !== this.seq) return false;
      this.loading = false;
      this.el.classList.remove('loading');
      const msg = errText(e);
      const shown = early && this.path === path; // (the panel stands at the folder: it says why there)
      this.waiting = '';
      if (!msg.startsWith('Stopped reading') && !shown) this.app.flash(msg, 'err');
      if (!this.path || shown) { this.error = msg; this.render(); }
      return false;
    }
    if (seq !== this.seq) return false; // a newer request won
    this.waiting = '';
    const prev = this.path;
    const same = prev === l.path && !early;
    if (prev && !same && !early) {
      if (this.cur) this.lastCursor.set(prev, this.cur.name);
      if (history) {
        this.back.push(prev);
        if (this.back.length > 200) this.back.shift();
        this.fwd = [];
      }
    }
    const oldCursor = this.cursor;
    const keepName = same ? this.cur?.name : null;
    this.path = l.path;
    this.parent = l.parent || '';
    this.error = '';
    this.all = (l.entries || []).map(toEntry);
    if (!same) { this.dirSizes = new Map(); this.filter = null; this.filterLabel = ''; }
    if (!same || !keepSelection) {
      if (!same) this.selected = new Set();
    }
    const names = new Set(this.all.map((e) => e.name));
    for (const n of this.selected) if (!names.has(n)) this.selected.delete(n);
    // Entering a folder puts the cursor on ".." (the top). Going up still lands on the folder you came from, because
    // that passes focus. Restoring the last cursor of a folder you come back to (TC's F-074) is a setting, off by
    // default: "rememberCursor".
    const remembered = this.app.setting('rememberCursor', false) ? this.lastCursor.get(l.path) : null;
    const back = restore && !same ? historyFocus(l.path, prev, this.lastCursor, this.app.sepFor(l.path)) : null;
    const want = focus ?? (same ? keepName : (back ?? remembered));
    this.rebuild(want, same ? oldCursor : 0);
    this.loading = false;
    this.el.classList.remove('loading');
    this.app.panelChanged(this, { moved: !same || early });
    return true;
  }

  // reload reads the folder again; the cursor stays on the same file if it is still there (else at the same place).
  // A "hide equal files" filter of Compare folders (marked mcCompare) belongs to that compare, so a reread drops it.
  // So does Show only selected / Hide selected files (marked mcReread, F-408), which gives back the filter it was put on.
  reload() {
    if (!this.path) return Promise.resolve(false);
    while (this.filter?.mcReread) { const f = this.filter; this.filter = f.mcPrev || null; this.filterLabel = this.filter ? f.mcPrevLabel || '' : ''; }
    if (this.filter?.mcCompare) { this.filter = null; this.filterLabel = ''; }
    return this.go(this.path, { history: false, keepSelection: true });
  }

  goBack() {
    const p = this.back.pop();
    if (!p) return Promise.resolve(false);
    this.fwd.push(this.path);
    return this.go(p, { history: false, restore: true });
  }

  goForward() {
    const p = this.fwd.pop();
    if (!p) return Promise.resolve(false);
    this.back.push(this.path);
    return this.go(p, { history: false, restore: true });
  }

  // rebuild applies hidden files, the filter and the sort to what was read, and puts the cursor on focusName (or at
  // fallbackIndex when that name is not shown).
  rebuild(focusName = this.cur?.name, fallbackIndex = this.cursor) {
    const show = this.app.showHidden;
    // system files (Windows) count as hidden, unless Options > Display > Show system files is on
    const sys = this.app.config?.values?.['Display.ShowSystem'] === '1';
    const list = this.all.filter((e) => (show || !(e.hidden || (!sys && e.flags & F.SYSTEM))) && (!this.filter || this.filter(e)) && !Panel.hideHooks.some((h) => h(this, e)));
    for (const e of list) e.dirSize = e.dir ? this.dirSizes.get(e.name) : undefined;
    if (this.parent) list.push(UP);
    else if (this.path && !/^[a-z][a-z0-9+.-]*:\/\//i.test(this.path) && (this.app.config?.values?.['Display.ShowParentInRoot'] === '1' || this.app.config?.values?.['Display.DrivesFolderInRoot'] === '1')) list.push(UP_ROOT);
    this.entries = sortEntries(list, this.sort, {
      natural: this.app.setting('naturalSort', true), dirsLikeFiles: this.app.setting('dirsLikeFiles', false),
      method: this.app.setting('sortMethod', ''), locale: this.app.setting('sortLocale', ''),
    });
    this.index = new Map(this.entries.map((e, i) => [e.name, i]));
    for (const n of this.selected) if (!this.index.has(n)) this.selected.delete(n);
    let i = focusName != null ? this.index.get(focusName) : undefined;
    if (i === undefined) i = Math.min(fallbackIndex, this.entries.length - 1);
    this.cursor = Math.max(0, i);
    this.render();
    this.ensureVisible(true);
  }

  // sortBy sorts by a column; the same column again reverses. Size and date start largest/newest first (TC).
  // The second sort key (sort.then, F-114) stays when only the direction changes, and goes with a new column.
  sortBy(key, desc) {
    if (desc === undefined) desc = this.sort.key === key ? !this.sort.desc : key === 'size' || key === 'date';
    const then = this.sort.key === key && this.sort.then?.key !== key ? this.sort.then : undefined;
    this.sort = then ? { key, desc, then } : { key, desc };
    this.rebuild();
    this.app.panelChanged(this, { moved: false });
  }

  // addSortKey (F-114, Ctrl/Cmd+click on a column title) sorts by key as well, after the first sort key: files equal
  // by the first key are ordered by this one. Again on the same column reverses it. Only name, extension, size and
  // date can be combined; with the first column, or when it cannot be combined, it works like a plain click.
  addSortKey(key) {
    const can = (k) => ['name', 'ext', 'size', 'date'].includes(k);
    if (key === this.sort.key || !can(key) || !can(this.sort.key)) return this.sortBy(key);
    const t = this.sort.then;
    const desc = t?.key === key ? !t.desc : key === 'size' || key === 'date';
    this.sort = { key: this.sort.key, desc: this.sort.desc, then: { key, desc } };
    this.rebuild();
    this.app.panelChanged(this, { moved: false });
  }

  // setFilter shows only entries that pass fn (null: all), with a label in the path bar ("*.txt", "Programs").
  setFilter(fn, label = '') {
    this.filter = fn;
    this.filterLabel = fn ? label : '';
    this.rebuild();
    this.app.panelChanged(this, { moved: false });
  }

  // swapWith exchanges everything a panel shows with the other panel (cm_Exchange). Features that keep more state per
  // panel (tabs) add their field names to Panel.STATE_KEYS.
  swapWith(o) {
    for (const k of Panel.STATE_KEYS) [this[k], o[k]] = [o[k], this[k]];
    for (const p of [this, o]) { p.render(); p.ensureVisible(true); }
  }

  // ---- cursor ----

  setCursor(i) {
    i = Math.max(0, Math.min(this.entries.length - 1, i));
    if (i === this.cursor) { this.ensureVisible(); return; }
    this.cursor = i;
    this.ensureVisible();
    this.renderRows();
    this.app.emit('cursor', this);
  }

  moveCursor(d) { this.setCursor(this.cursor + d); }

  pageSize() { return Math.max(1, Math.floor(this.$.list.clientHeight / this.rowH) - 1); }

  // focusName puts the cursor on a name; false when it is not shown.
  focusName(name) {
    const i = this.index.get(name);
    if (i === undefined) return false;
    this.setCursor(i);
    return true;
  }

  ensureVisible(center = false) {
    const list = this.$.list, h = this.rowH, y = this.cursor * h;
    const view = list.clientHeight;
    if (!view) return;
    if (center && (y < list.scrollTop || y + h > list.scrollTop + view)) list.scrollTop = Math.max(0, y - view / 2);
    else if (y < list.scrollTop) list.scrollTop = y;
    else if (y + h > list.scrollTop + view) list.scrollTop = y + h - view;
    this.renderRows();
  }

  // ---- selection ----

  isSelected(e) { return !!e && !e.up && this.selected.has(e.name); }

  select(e, on = true) {
    if (!e || e.up) return;
    if (on) this.selected.add(e.name); else this.selected.delete(e.name);
  }

  toggle(e) { this.select(e, !this.isSelected(e)); }

  // selectWhere selects (or unselects) every shown entry fn says yes to.
  selectWhere(fn, on = true) {
    for (const e of this.entries) if (!e.up && fn(e)) this.select(e, on);
    this.selectionChanged();
  }

  clearSelection() {
    this.selected.clear();
    this.selectionChanged();
  }

  // saveSelection keeps the selected names for cm_RestoreSelection (Num /). File-job features call it before they start.
  saveSelection() { this.saved = [...this.selected]; }

  selectionChanged() {
    this.renderRows();
    this.renderStatus();
    this.app.emit('selection', this);
  }

  selectedEntries() { return this.entries.filter((e) => !e.up && this.selected.has(e.name)); }

  // targets is what a command works on, Total Commander's way: the selected entries, or the one under the cursor when
  // nothing is selected (never "..").
  targets() {
    const sel = this.selectedEntries();
    if (sel.length) return sel;
    return this.cur && !this.cur.up ? [this.cur] : [];
  }

  // ---- mouse ----

  onMouseDown(ev) {
    this.app.setActive(this);
    const row = ev.target.closest('.row');
    if (!row) { this.onEmptyPress(ev); return; }
    const i = +row.dataset.i;
    const e = this.entries[i];
    if (ev.button === 2) { if (!Panel.rightPress?.(this, ev, i)) this.setCursor(i); return; }
    if (ev.button !== 0) return;
    // A double-click opens the entry like Enter (Ctrl/Cmd: the folder in a new tab). It is taken from the second
    // press (ev.detail 2) on the same row, not from a 'dblclick' event: the first press redraws the rows, so the
    // browser sees two different elements and the draggable rows can swallow 'dblclick' altogether.
    if (ev.detail === 2 && this.lastPress === i) {
      ev.preventDefault();
      this.setCursor(i);
      this.app.run(ev.ctrlKey || ev.metaKey ? 'cm_OpenDirInNewTab' : 'cm_Enter');
      return;
    }
    this.lastPress = i;
    if (ev.ctrlKey || ev.metaKey) {
      this.toggle(e);
      this.anchor = i;
      this.setCursor(i);
      this.selectionChanged();
    } else if (ev.shiftKey && this.anchor >= 0) {
      const [a, b] = this.anchor < i ? [this.anchor, i] : [i, this.anchor];
      for (let k = a; k <= b; k++) this.select(this.entries[k], true);
      this.setCursor(i);
      this.selectionChanged();
    } else {
      this.anchor = i;
      this.setCursor(i);
    }
  }

  // onEmptyPress: a double-click on the empty space below the files goes to the parent folder when Options > Operation >
  // Double click on empty space goes up is on (off by default). A press on the list's scroll bar is not empty space.
  onEmptyPress(ev) {
    if (ev.button !== 0 || ev.detail !== 2 || this.app.config?.values?.['Operation.DoubleClickParent'] !== '1') return;
    const list = this.$.list, r = list.getBoundingClientRect();
    if (ev.clientX - r.left > list.clientWidth || ev.clientY - r.top > list.clientHeight) return;
    if (!this.parent) return;
    ev.preventDefault();
    this.app.run('cm_GoToParent');
  }

  // ---- path editing (click on the path bar, cm_EditPath) ----

  editPath() {
    const { pathbar, crumbs } = this.$;
    if (pathbar.querySelector('input')) return;
    const input = document.createElement('input');
    input.type = 'text';
    input.value = this.path;
    input.spellcheck = false;
    input.setAttribute('aria-label', 'Folder');
    crumbs.hidden = true;
    pathbar.insertBefore(input, crumbs);
    attachPathComplete(input, () => this.path); // (folder names complete as they are typed, F-782)
    input.focus();
    input.select();
    let closed = false;
    const close = () => {
      if (closed) return;
      closed = true;
      input.remove();
      crumbs.hidden = false;
      this.$.list.focus();
    };
    input.addEventListener('keydown', async (ev) => {
      ev.stopPropagation();
      if (ev.key === 'Escape') { ev.preventDefault(); close(); }
      if (ev.key === 'Enter') {
        ev.preventDefault();
        const v = input.value.trim();
        close();
        if (v) await this.go(this.app.resolvePath(v, this.path));
      }
    });
    input.addEventListener('blur', close);
  }

  // ---- drawing ----

  render() {
    const { spacer, msg, header, crumbs, filter } = this.$;
    this.el.classList.toggle('active', this.isActive);
    spacer.style.height = `${this.entries.length * this.rowH}px`;
    header.querySelectorAll('[data-col]').forEach((s) => {
      const on = s.dataset.col === this.sort.key;
      const on2 = !on && s.dataset.col === this.sort.then?.key;
      s.classList.toggle('sorted', on);
      s.classList.toggle('sorted2', on2);
      s.classList.toggle('desc', (on && this.sort.desc) || (on2 && !!this.sort.then.desc));
    });
    crumbs.innerHTML = splitPath(this.path).map((p) => `<span data-p="${esc(p.path)}">${esc(p.label)}</span>`).join('');
    crumbs.title = this.path;
    fitCrumbs(crumbs);
    filter.hidden = !this.filterLabel;
    filter.textContent = this.filterLabel;
    if (this.error) { msg.hidden = false; msg.className = 'msg err'; msg.textContent = this.error; }
    else if (this.waiting) { msg.hidden = false; msg.className = 'msg waiting'; msg.textContent = this.waiting; }
    else if (this.path && this.entries.filter((e) => !e.up).length === 0) { msg.hidden = false; msg.className = 'msg'; msg.textContent = this.filter ? 'No files match the filter.' : 'This folder is empty.'; }
    else msg.hidden = true;
    this.renderRows();
    this.renderStatus();
  }

  renderRows() {
    const { list, rows } = this.$;
    const h = this.rowH;
    const first = Math.max(0, Math.floor(list.scrollTop / h) - 3);
    const last = Math.min(this.entries.length, first + Math.ceil((list.clientHeight || 600) / h) + 7);
    const plat = this.app.platform;
    const active = this.isActive;
    let html = '';
    for (let i = first; i < last; i++) {
      const e = this.entries[i];
      let cls = 'row';
      if (i === this.cursor) cls += ' cur';
      if (e.up) cls += ' up dir';
      else {
        if (e.dir) cls += ' dir';
        if (this.selected.has(e.name)) cls += ' sel';
        if (e.hidden) cls += ' hid';
        if (e.link) cls += ' link';
        if (e.broken) cls += ' broken';
        if (e.exec && !e.dir) cls += ' exe';
        cls += Panel.extraRowClass(this, e);
      }
      const sep = display.separateExt;
      let name = e.dir || !sep ? e.name : e.base;
      const dos = display.longNames === false && !e.up; // Options > Display > Show long names off: 8.3 names
      if (dos) { const d = dosName(e.name, e.dir); const k = d.lastIndexOf('.'); name = e.dir || !sep || k < 0 ? d : d.slice(0, k); }
      const sz = e.dir ? (e.dirSize != null ? cellSize(e.dirSize) : '&lt;DIR&gt;') : cellSize(e.size);
      const tip = Panel.tipAttr(this, e, name.length > 28);
      const shown = e.dir && !e.up && display.brackets ? `[${esc(name)}]` : esc(name);
      // screen readers (F-764): each row is an option of the "Files" list box, named by the file, its kind and whether
      // it is selected; the cursor row is the list's active descendant
      const label = `${e.up ? '.. (parent folder)' : e.name}${e.dir && !e.up ? ', folder' : ''}${!e.up && this.selected.has(e.name) ? ', selected' : ''}`;
      html += `<div class="${cls}" data-i="${i}" role="option" aria-label="${esc(label)}"${i === this.cursor ? ` id="mc-cur-${this.side}"` : ''}${i === this.cursor && active ? ' aria-selected="true"' : ''}><span class="n">${Panel.rowIcon(this, e)}<b${tip}>${shown}</b></span><span class="e">${sep && !e.dir ? esc(dos ? (dosName(e.name).split('.')[1] || '') : e.ext) : ''}</span><span class="s r">${sz}</span><span class="d">${e.up ? '' : date(e.mtime)}</span><span class="a">${e.up ? '' : attr(e.mode, e.flags, plat)}</span></div>`;
    }
    rows.style.transform = `translateY(${first * h}px)`;
    rows.innerHTML = html;
    if (this.cursor >= first && this.cursor < last) list.setAttribute?.('aria-activedescendant', `mc-cur-${this.side}`);
    else list.removeAttribute?.('aria-activedescendant');
  }

  // stats counts what is shown and what is selected (for the status line and control mode).
  stats() {
    let files = 0, dirs = 0, total = 0, selFiles = 0, selDirs = 0, selBytes = 0;
    for (const e of this.entries) {
      if (e.up) continue;
      const sel = this.selected.has(e.name);
      const n = e.dir ? (e.dirSize || 0) : e.size;
      if (e.dir) { dirs++; if (sel) selDirs++; } else { files++; if (sel) selFiles++; }
      total += n;
      if (sel) selBytes += n;
    }
    return { files, dirs, total, selFiles, selDirs, selBytes };
  }

  renderStatus() {
    const s = this.stats();
    let line = t('{selSize} of {size} in {sel} of {files} selected', { selSize: footerSize(s.selBytes), size: footerSize(s.total), sel: s.selFiles,
      files: plural(s.files, t('file'), t('files')) });
    if (s.dirs) line += t(', {sel} of {folders}', { sel: s.selDirs, folders: plural(s.dirs, t('folder'), t('folders')) });
    this.$.status.textContent = line;
    // while hidden files are shown the line ends with a "Hidden: shown" marker (Layout > "Show the hidden files
    // marker" turns it off by hiding it with a body class).
    const mark = hiddenMarkHTML(this.app.showHidden);
    if (mark) this.$.status.insertAdjacentHTML('beforeend', mark);
  }

  // renderDrives draws the drive bar: one button per volume, the one the panel is on highlighted.
  renderDrives(volumes, currentPath) {
    const on = this.app.driveFor(currentPath ?? this.path);
    // the bar comes in groups with a divider between them: the disks (and Home), the network drives (mounted shares and
    // the shares you mapped, core/drivemerge.js), and the folders you mapped
    const group = (v) => (v.kind === 'network' || v.net ? 1 : v.kind === 'map' ? 2 : 0);
    let last = -1;
    // the Network button (features/network.js, #148) closes the bar: servers and shares on the local network
    // Home and Network show only their icon (the tooltip names them); Network sits at the right end of the bar
    const inNet = /^(network|smb|nfs|afp):\/\//i.test(currentPath ?? this.path ?? '');
    const net = this.app.openNetwork ? `<button type="button" class="mc-drive-net mc-drive-icon${inNet ? ' on' : ''}" data-net="1" title="Network: servers and shared folders on the local network" aria-label="Network">${driveIcon('lan')}</button>` : '';
    // Options > Layout > "Show how full each drive is under its drive button" (F-714): a thin bar along each button
    const usage = this.app.config?.values?.['Layout.DriveBarUsage'] === '1';
    this.$.drives.innerHTML = volumes.map((v, i) => {
      if (v.hidden) return ''; // (a mounted share a mapped button stands for: core/drivemerge.js)
      const iconOnly = v.kind === 'home';
      const name = esc(shortName(v, this.app.platform));
      const g = group(v), sep = last >= 0 && g !== last;
      last = g;
      return `${sep ? '<span class="mc-drive-sep" aria-hidden="true"></span>' : ''}<button type="button" data-i="${i}" class="${v === on && !inNet ? 'on' : ''}${iconOnly ? ' mc-drive-icon' : ''}"${iconOnly ? ` aria-label="${name}"` : ''} title="${esc(v.label ? `${v.name} ${v.label}` : v.name)} – ${esc(v.path)}${v.total ? ` – ${esc(size(v.free))} free of ${esc(size(v.total))}` : ''}">${driveIcon(v.net ? 'network' : v.kind)}${iconOnly ? '' : name}${usageBar(usage, v)}</button>`;
    }).join('') + net;
    this.$.drives.onclick = (ev) => {
      const b = ev.target.closest('button');
      if (!b) return;
      this.app.setActive(this);
      if (b.dataset.net) { this.app.openNetwork(this); return; }
      this.go(this.app.drivePath(this, volumes[+b.dataset.i]));
    };
  }

  renderDriveInfo(v) {
    if (!v) { this.$.driveinfo.textContent = ''; return; }
    const name = v.name || v.path;
    // the volume's name is the drive combo box (Options > Layout > Show drive combo box): a click lists the drives
    this.$.driveinfo.innerHTML = `<button type="button" class="mc-drivecombo" title="Drives (${this.side === 'left' ? 'Alt+F1' : 'Alt+F2'})">[${esc(name)}]</button>${v.total ? `<span>  ${esc(size(v.free))} free of ${esc(size(v.total))}</span>` : ''}`;
  }
}

// Features with virtual folders (Feed to listbox) map a name to the real file: hook(panel, name) → full path or ''.
Panel.pathHooks = [];

// Features with folders that are slow to open (a network folder: it may have to connect first) return true from
// hook(panel, path) when go starts for such a folder: the panel then goes there at once and shows "Connecting…" inside
// until it is read (features/network.js), instead of fading the folder it was in.
Panel.quietLoadHooks = [];

// Features that leave entries out of every list (the ignore list: features/config/listopts.js) add hook(panel, entry)
// → true to hide it. Unlike the panel's filter, they stay through folder changes and filters.
Panel.hideHooks = [];

// Mouse hooks of features/mouseopts.js: rightPress(panel, ev, rowIndex) → true when it took the right press;
// contextGate(panel, ev, args) → true when the right-click menu must not open now; middleClick(panel, ev).
Panel.rightPress = null;
Panel.contextGate = null;
Panel.middleClick = null;

// iconHTML is a row's icon: the page's own by default; features/config/fileicons.js gives the system's (Options > Icons).
Panel.iconHTML = () => '<i class="ic"></i>';
// rowIcon is what a row shows: the icon of an internal association naming one (features/assoc.js, F-323), else iconHTML.
Panel.assocIcon = null;
Panel.rowIcon = (p, e) => Panel.assocIcon?.(p, e) || Panel.iconHTML(p, e);

// tipAttr is the tooltip attribute of a row's name: by default the whole name when it may be cut off (cut: a hint
// from the renderer). features/config/listopts.js replaces it to follow Options > Tooltips.
Panel.tipAttr = (panel, e, cut) => (cut ? ` title="${esc(e.name)}"` : '');

// Features that mark rows (cut files are dimmed until pasted) add a class: hook(panel, entry) → class name or ''.
// extraRowClass joins what the hooks return, with a leading space, for the row renderers (here and views.js).
Panel.rowClassHooks = [];
Panel.extraRowClass = (panel, e) => {
  let cls = '';
  for (const h of Panel.rowClassHooks) { const c = h(panel, e); if (c) cls += ` ${c}`; }
  return cls;
};

// Features that colour names (the colour labels of features/notes.js) return a CSS colour: hook(panel, entry) → colour
// or ''. The first one that answers wins, and it wins over Colour by file type (features/config/layout.js paints both).
Panel.nameColorHooks = [];
Panel.nameColor = (panel, e) => {
  for (const h of Panel.nameColorHooks) { const c = h(panel, e); if (c) return c; }
  return '';
};

Panel.STATE_KEYS = ['path', 'parent', 'all', 'entries', 'index', 'cursor', 'selected', 'sort', 'back', 'fwd', 'lastCursor', 'filter', 'filterLabel', 'dirSizes', 'error'];

function shortName(v, platform) {
  if (platform === 'windows' && /^[A-Z]:/i.test(v.name)) return v.name.slice(0, 1).toLowerCase();
  return v.name;
}

function driveIcon(kind) {
  const p = {
    home: '<path d="M4 11l8-6 8 6v9H4z"/><path d="M10 20v-5h4v5"/>',
    map: '<path d="M7 4h10v16l-5-4-5 4z"/>',
    network: '<rect x="3" y="4" width="18" height="11" rx="2"/><path d="M12 15v4M7 20h10"/>',
    cdrom: '<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="2"/>',
    removable: '<rect x="7" y="3" width="10" height="18" rx="2"/><path d="M10 7h4"/>',
    lan: '<rect x="9" y="3" width="6" height="5" rx="1"/><rect x="3" y="16" width="6" height="5" rx="1"/><rect x="15" y="16" width="6" height="5" rx="1"/><path d="M12 8v4M6 16v-4h12v4"/>',
  }[kind] || '<rect x="3" y="13" width="18" height="7" rx="2"/><path d="M3.4 13l2.2-6.6A2 2 0 0 1 7.5 5h9a2 2 0 0 1 1.9 1.4L20.6 13"/>';
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${p}</svg>`;
}
