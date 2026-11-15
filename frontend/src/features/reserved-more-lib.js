// Pure helpers of the second slice of reserved Total Commander commands that got a body in board issue #38
// (features/reserved-more.js): which panels show the folder tree for the "separate tree" commands, the items of the
// in-window menu (F9, F10), the items of the folder tree popup (Alt+F10) and the text of a printed file list. No DOM
// and no Go here, so vitest can test them.

// ---- separate tree (Show > Separate tree, Ctrl+Shift+F8) ----

// Total Commander's separate tree is a pane of its own beside the file panels. This app has no such pane; it has the
// tree view inside a panel (Ctrl+F8). The separate tree commands are therefore mapped onto that view:
//   Off                  no panel shows the tree,
//   One for both panels  exactly one panel shows the tree (the active one, unless the other one already does),
//   One per panel        both panels show the tree,
// and the cycle (Ctrl+Shift+F8) goes Off -> One -> Two -> Off, as in TC.

// treeLevel says which of the three states the panels are in: state is {active, other}, true when that panel shows
// the tree view.
export function treeLevel(state) {
  const n = (state?.active ? 1 : 0) + (state?.other ? 1 : 0);
  return n === 0 ? 'off' : n === 1 ? 'one' : 'two';
}

// treeTarget says which panels should show the tree after a separate tree command: want is 'off', 'one', 'two' or
// 'cycle'. For 'one' a panel that already shows the tree keeps it, so choosing the same item twice changes nothing;
// coming down from two trees, the active panel keeps its tree, since that is the one the user is working in.
export function treeTarget(state, want) {
  const level = treeLevel(state);
  if (want === 'cycle') want = level === 'off' ? 'one' : level === 'one' ? 'two' : 'off';
  switch (want) {
    case 'off': return { active: false, other: false };
    case 'two': return { active: true, other: true };
    case 'one':
      if (level === 'one') return { active: !!state.active, other: !!state.other };
      return { active: true, other: false };
    default: return { active: !!state?.active, other: !!state?.other };
  }
}

// ---- the in-window menu (cm_ActivateMenu, F9 / F10) ----

// menuItems turns one level of the menu tree (core/menus.js menuSpec: {label, cmd, disabled, checked}, {label, items}
// or {sep: true}) into popup items (core/popup.js showList). keyFor(cmd) gives the key text shown next to a command.
// A submenu is marked sub, so Right opens it; a ticked toggle shows a check mark in front of its label. Separators at
// the start, at the end or next to each other are left out (items added at run time can leave them there).
export function menuItems(level, keyFor = () => '') {
  const out = [];
  for (const it of Array.isArray(level) ? level : []) {
    if (!it) continue;
    if (it.sep) { if (out.length && !out[out.length - 1].sep) out.push({ sep: true }); continue; }
    if (Array.isArray(it.items)) { out.push({ label: `${it.label} ▸`, value: { items: it.items, label: it.label }, sub: true }); continue; }
    const mark = it.checked === undefined ? '' : it.checked ? '✓ ' : '   ';
    out.push({ label: `${mark}${it.label}`, detail: it.cmd ? keyFor(it.cmd) || '' : '', value: { cmd: it.cmd }, disabled: !!it.disabled });
  }
  while (out.length && out[out.length - 1].sep) out.pop();
  return out;
}

// topMenuItems is the first level of the in-window menu: the menu bar's titles, each opening its menu.
export function topMenuItems(spec) {
  return (Array.isArray(spec) ? spec : []).filter((m) => m && Array.isArray(m.items) && m.items.length)
    .map((m) => ({ label: `${m.label} ▸`, value: { items: m.items, label: m.label }, sub: true }));
}

// ---- the folder tree popup (cm_CDtree, Alt+F10) ----

// dirNames picks the folder names out of a Go listing's rows ([name, size, mtime, flags, …]), without hidden ones
// (names starting with a dot, or the hidden flag) unless showHidden, sorted the way people read numbers.
export function dirNames(rows, { showHidden = false, dirFlag = 1, hiddenFlag = 4 } = {}) {
  return (Array.isArray(rows) ? rows : [])
    .filter((r) => Array.isArray(r) && (r[3] & dirFlag) && r[0] !== '.' && r[0] !== '..')
    .filter((r) => showHidden || (!String(r[0]).startsWith('.') && !(r[3] & hiddenFlag)))
    .map((r) => String(r[0]))
    .sort((a, b) => a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' }));
}

// joinPath puts a name under a folder with the folder's own separator.
export function joinPath(dir, name, sep = '/') {
  return /[\\/]$/.test(dir) ? dir + name : dir + sep + name;
}

// folderItems is one level of the folder tree popup: first "Go to <folder>" (the folder being looked at, chosen with
// Enter right away), then ".." when there is a parent, then the subfolders. Choosing ".." or a subfolder shows that
// folder's level; choosing "Go to" changes the panel's folder.
export function folderItems(path, parent, names, sep = '/') {
  const items = [{ label: `Go to ${path}`, value: { go: path } }];
  if (parent && parent !== path) items.push({ label: '..', detail: 'up one level', value: { open: parent, from: path } });
  if (names.length) items.push({ sep: true });
  for (const n of names) items.push({ label: n, value: { open: joinPath(path, n, sep) }, sub: true });
  return items;
}

// ---- printing the file list (cm_PrintDir) ----

const pad2 = (n) => String(n).padStart(2, '0');

// isoDate writes a time (ms) as yyyy-MM-dd HH:mm in local time, the same on every system and language, so a printed
// list reads the same everywhere.
export function isoDate(ms) {
  if (!ms) return '';
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) return '';
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
}

// groupDigits writes a byte count with thin groups of three ("1 234 567"), independent of the language settings.
export function groupDigits(n) {
  return String(Math.max(0, Math.round(Number(n) || 0))).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');
}

// fileListText is the printed file list of a folder: a heading with the folder and the time, one line per entry
// (name, size or <DIR>, date) in the order the panel shows them, and a summary line. entries are panel entries
// ({name, dir, size, mtime, up}); ".." is left out. A long name pushes its line wider rather than being cut.
export function fileListText({ path, entries, when = Date.now(), filter = '' }) {
  const list = (Array.isArray(entries) ? entries : []).filter((e) => e && !e.up);
  const sizeText = (e) => (e.dir ? (e.dirSize != null ? groupDigits(e.dirSize) : '<DIR>') : groupDigits(e.size));
  const nameW = Math.min(60, Math.max(4, ...list.map((e) => String(e.name).length)));
  const sizeW = Math.max(5, ...list.map((e) => sizeText(e).length));
  const lines = [`Folder: ${path}`, `Printed: ${isoDate(when)}${filter ? `   Filter: ${filter}` : ''}`, ''];
  lines.push(`${'Name'.padEnd(nameW)}  ${'Size'.padStart(sizeW)}  Date`);
  lines.push(`${'-'.repeat(nameW)}  ${'-'.repeat(sizeW)}  ${'-'.repeat(16)}`);
  for (const e of list) lines.push(`${String(e.name).padEnd(nameW)}  ${sizeText(e).padStart(sizeW)}  ${isoDate(e.mtime)}`.trimEnd());
  const files = list.filter((e) => !e.dir), dirs = list.length - files.length;
  const total = files.reduce((s, e) => s + (Number(e.size) || 0), 0);
  const word = (n, w) => `${n} ${n === 1 ? w : `${w}s`}`;
  lines.push('', `${word(files.length, 'file')} (${groupDigits(total)} bytes), ${word(dirs, 'folder')}`);
  return `${lines.join('\n')}\n`;
}
