// Drag and drop (FEATURES.md section 36, docs/spec/area-8.md section 6). Files dragged from a panel are dropped:
//   - on the other panel (its folder), a folder or ".." in either panel, a drive button (that drive's folder), a part of
//     the path bar, or anything marked data-drop-path="…" (tabs can use it): copied there (F-694, F-696, F-697);
//     Shift moves, Ctrl+Shift or Alt makes symbolic links (TC); on macOS also Finder's Option = copy, Cmd = move,
//     Cmd+Option = link (F-851). Dropping on the panel's own folder does nothing.
//   - on a program in a panel: it runs with the files as parameters;
//   - on the command line: their paths are inserted (F-699).
// Files dragged from Finder / Explorer / other apps are copied into the panel or folder they are dropped on (F-695;
// Wails gives their paths); dragged out of the window they carry file URLs and paths as text. The drop dialog asks
// first (setting "confirmDrop", default on, F-719) and lets the user pick copy / move / link and change the folder.
// While dragging, the list scrolls near its top and bottom edge (F-701) and, with the setting "springLoaded", a folder
// opens after hovering over it ("springDelay" ms, default 1000; F-702).
//
// HTML drag events can't be made by control mode, so it has "drop" and "filedrop" commands that do what a drop does.

import { addControlCommand } from '../core/control.js';
import { cmdline } from '../core/cmdline.js';
import { go, errText } from '../core/go.js';
import { plural } from '../core/format.js';
import { app as theApp } from '../core/app.js';
import { isLocal, quoteArg, transferFiles } from './system-lib.js';

const MIME = 'application/x-mormicommander';
let drag = null;        // {from: panel, paths}: the drag that started in a panel
let dragEnded = 0;      // when it ended (a drop of our own files must not come back as a Finder drop)
let lit = null;         // the highlighted drop target
let spring = null;      // {path, timer}
let lastScroll = 0;

// opFor is the operation a drop does with the modifiers held (TC rules, plus Finder's on macOS).
export function opFor(ev, platform) {
  const mac = platform === 'darwin';
  if ((ev.ctrlKey && ev.shiftKey) || (mac ? ev.metaKey && ev.altKey : ev.altKey)) return 'link';
  if (ev.shiftKey || (mac && ev.metaKey)) return 'move';
  return 'copy';
}

// targetAt is what lies under the pointer: {kind: 'folder'|'program'|'cmdline', path, panel, el, entry}.
function targetAt(app, el) {
  if (!el?.closest) return null;
  if (el.closest('#cmdline')) return { kind: 'cmdline', el: el.closest('#cmdline') };
  const marked = el.closest('[data-drop-path]');
  if (marked) return { kind: 'folder', path: marked.dataset.dropPath, el: marked, panel: app.panels.find((p) => p.el.contains(marked)) || null };
  const pel = el.closest('.panel');
  const p = pel && app.panels.find((x) => x.el === pel);
  if (!p || !p.path) return null;
  const b = el.closest('.drivebar button');
  if (b) {
    const v = app.volumes[+b.dataset.i];
    if (!v) return null;
    return { kind: 'folder', panel: p, path: app.volumeFor(p.path) === v ? p.path : v.path, el: b };
  }
  const crumb = el.closest('.crumbs [data-p]');
  if (crumb) return { kind: 'folder', panel: p, path: crumb.dataset.p, el: crumb };
  const row = el.closest('.row');
  if (row) {
    const e = p.entries[+row.dataset.i];
    if (e?.up && p.parent) return { kind: 'folder', panel: p, path: p.parent, el: row, entry: e };
    if (e?.dir) return { kind: 'folder', panel: p, path: p.fullPath(e.name), el: row, entry: e };
    if (e?.exec && !e.dir && isLocal(p.path)) return { kind: 'program', panel: p, path: p.fullPath(e.name), el: row, entry: e };
  }
  return { kind: 'folder', panel: p, path: p.path, el: p.$.list };
}

const within = (p, dir) => p === dir || p.startsWith(dir.endsWith('/') || dir.endsWith('\\') ? dir : dir + (dir.includes('\\') && !dir.includes('/') ? '\\' : '/'));

// allowed says whether files from a panel may be dropped on t: not on their own folder, not into themselves.
function allowed(d, t) {
  if (!t) return false;
  if (t.kind === 'cmdline') return true;
  if (!isLocal(t.path)) return false;
  if (t.kind === 'program') return !d.paths.includes(t.path);
  if (d.from && t.path === d.from.path) return false;
  return !d.paths.some((s) => within(t.path, s));
}

function light(el) {
  if (lit === el) { el?.classList.add('mc-system-drop'); return; }
  lit?.classList.remove('mc-system-drop');
  lit = el || null;
  lit?.classList.add('mc-system-drop');
}

function stopSpring() { if (spring) { clearTimeout(spring.timer); spring = null; } }

// hover does the things that happen while something is dragged over a panel: auto-scroll and spring-loaded folders.
function hover(app, ev, t) {
  const p = t?.panel;
  if (p && p.$.list.contains(ev.target)) {
    const r = p.$.list.getBoundingClientRect();
    const now = Date.now();
    if (now - lastScroll > 60) {
      if (ev.clientY < r.top + p.rowH) { p.$.list.scrollTop -= p.rowH; lastScroll = now; }
      else if (ev.clientY > r.bottom - p.rowH) { p.$.list.scrollTop += p.rowH; lastScroll = now; }
    }
  }
  const springy = app.setting('springLoaded', false) && t?.kind === 'folder' && t.entry && p;
  if (!springy) { stopSpring(); return; }
  if (spring?.path === t.path) return;
  stopSpring();
  const path = t.path;
  spring = { path, timer: setTimeout(() => { spring = null; app.setActive(p); p.go(path); }, +app.setting('springDelay', 1000) || 1000) };
}

function finish() {
  drag = null;
  dragEnded = Date.now();
  light(null);
  stopSpring();
}

const fileURL = (p) => {
  const s = p.replace(/\\/g, '/');
  return 'file://' + (s.startsWith('/') ? '' : '/') + encodeURI(s).replace(/#/g, '%23').replace(/\?/g, '%3F');
};

// performDrop does what dropping paths on t does. fromPanel is the panel they came from (null: another app).
export async function performDrop(app, paths, t, op, fromPanel = null) {
  if (!t || !paths.length) return false;
  if (t.kind === 'cmdline') {
    cmdline.add(paths.map((p) => quoteArg(p, app.platform)).join(' '));
    cmdline.focus();
    return true;
  }
  if (t.kind === 'program') {
    try {
      await go.SystemLaunch.RunWith(t.path, paths, t.panel?.path || '');
      app.flash(`Started ${t.entry?.name || t.path} with ${plural(paths.length, 'file')}.`);
    } catch (e) { app.flash(errText(e), 'err'); }
    return true;
  }
  if (fromPanel && !allowed({ from: fromPanel, paths }, t)) return false;
  if (t.panel) app.setActive(t.panel);
  try {
    return await transferFiles(app, op, paths, t.path, { confirm: app.setting('confirmDrop', true) !== false });
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
}

export function init(app) {
  for (const p of app.panels) {
    const rows = p.$.rows;
    rows.setAttribute('draggable', 'true');
    rows.addEventListener('dragstart', (ev) => {
      // the press put the cursor on the row (Panel.onMouseDown); the selection goes when that row is selected
      const e = p.cur;
      const onRow = ev.target?.closest?.('.row') || document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.('.row');
      if (!e || e.up || !onRow || !isLocal(p.path) || app.demo) { ev.preventDefault(); return; }
      const list = p.isSelected(e) ? p.selectedEntries() : [e];
      const paths = list.map((x) => p.fullPath(x.name));
      drag = { from: p, paths };
      const dt = ev.dataTransfer;
      dt.effectAllowed = 'all';
      dt.setData(MIME, JSON.stringify({ side: p.side, paths }));
      dt.setData('text/uri-list', paths.map(fileURL).join('\r\n'));
      dt.setData('text/plain', paths.join('\n'));
      const img = document.createElement('div');
      img.className = 'mc-system-dragimg';
      img.textContent = list.length === 1 ? list[0].name : plural(list.length, 'file');
      document.body.append(img);
      dt.setDragImage(img, -12, -8);
      setTimeout(() => img.remove(), 0);
    });
    rows.addEventListener('dragend', finish);
  }

  document.addEventListener('dragover', (ev) => {
    const types = [...(ev.dataTransfer?.types || [])];
    const ours = !!drag && types.includes(MIME);
    if (!ours && !types.includes('Files')) return;
    const t = targetAt(app, ev.target);
    if (ours && !allowed(drag, t)) { ev.dataTransfer.dropEffect = 'none'; light(null); stopSpring(); return; }
    if (!t) { light(null); return; }
    ev.preventDefault();
    const op = ours ? opFor(ev, app.platform) : 'copy';
    ev.dataTransfer.dropEffect = t.kind === 'folder' ? op : 'copy';
    light(t.el);
    hover(app, ev, t);
  });
  document.addEventListener('dragleave', (ev) => { if (!ev.relatedTarget) { light(null); stopSpring(); } });
  document.addEventListener('drop', (ev) => {
    const types = [...(ev.dataTransfer?.types || [])];
    if (!types.includes(MIME)) { light(null); stopSpring(); return; } // files from other apps come through OnFileDrop
    ev.preventDefault();
    const d = drag;
    const t = targetAt(app, ev.target);
    const op = opFor(ev, app.platform);
    finish();
    if (!d || !allowed(d, t)) return;
    performDrop(app, d.paths, t, op, d.from);
  });

  // files from Finder / Explorer / other apps, with their real paths (Wails, main.go DragAndDrop)
  const rt = globalThis.window?.runtime;
  rt?.OnFileDrop?.((x, y, paths) => {
    light(null);
    stopSpring();
    if (drag || Date.now() - dragEnded < 1500 || !paths?.length) return; // our own drag, seen by the system too
    const t = targetAt(app, document.elementFromPoint(x, y)) || { kind: 'folder', panel: app.cur, path: app.cur.path, el: null };
    performDrop(app, paths, t, 'copy', null);
  }, false);
}

// ---- control mode: drops without a mouse ----

// {from: "left", names: ["a.txt"], to: "right", name: "sub" (a folder in the target panel; "" = its folder), target:
// "cmdline" (instead of to/name), op: "copy"|"move"|"link"}. The drop dialog comes up as with a real drop.
addControlCommand('drop', async (m) => {
  const app = theApp;
  const from = app.panel(m.from || 'active');
  const names = m.names?.length ? m.names : from.targets().map((e) => e.name);
  const paths = names.map((n) => from.fullPath(n));
  let t;
  if (m.target === 'cmdline') t = { kind: 'cmdline' };
  else {
    const to = app.panel(m.to || 'other');
    const e = m.name ? to.entries[to.index.get(m.name)] : null;
    if (m.name && !e) throw new Error(`no "${m.name}" in ${to.path}`);
    if (e?.up) t = { kind: 'folder', panel: to, path: to.parent, entry: e };
    else if (e?.dir) t = { kind: 'folder', panel: to, path: to.fullPath(e.name), entry: e };
    else if (e?.exec) t = { kind: 'program', panel: to, path: to.fullPath(e.name), entry: e };
    else t = { kind: 'folder', panel: to, path: to.path };
    if (!allowed({ from, paths }, t)) return { ok: true, dropped: false, reason: 'not allowed here' };
  }
  performDrop(app, paths, t, m.op || 'copy', from);
  return null;
});

// {paths: ["/abs/file"], to: "left", name: "sub"}: as if the files were dropped from Finder / Explorer.
addControlCommand('filedrop', async (m) => {
  const app = theApp;
  const to = app.panel(m.to || 'active');
  const e = m.name ? to.entries[to.index.get(m.name)] : null;
  const t = e?.dir ? { kind: 'folder', panel: to, path: e.up ? to.parent : to.fullPath(e.name) } : { kind: 'folder', panel: to, path: to.path };
  performDrop(app, m.paths || [], t, 'copy', null);
  return null;
});
