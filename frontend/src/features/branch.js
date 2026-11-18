// Branch view (F-086 cm_DirBranch, Ctrl+B; F-087 cm_DirBranchSel, Ctrl+Shift+B): the panel shows every file of the
// folder and all its subfolders as one flat list, each named by its path relative to the folder ("sub/deeper/a.txt").
// Ctrl+Shift+B lists only the selected files and the files of the selected folders. Ctrl+B again, Ctrl+Shift+B again
// or ".." goes back to the folder, with the cursor on the subfolder the file under the cursor came from.
//
// The list is a folder of its own for the panel (Go: feature_branch.go), so the panel needs nothing new: sorting,
// selection, quick search and the status line work as in any folder. A Panel path hook maps each name back to the
// real file (the folder the view lists is the panel's ".."), so view (F3), edit (F4), copy (F5), move (F6), delete
// (F8), Multi-rename (Ctrl+M, each file renamed in its own folder, F-511) and the other file commands work on the real
// files. Go walks the tree while the panel waits for the listing, off the page's thread; after a moment a notice over
// the list counts the files found so far, and Esc or its Stop button stops the walk (the panel stays where it was).
// Very big trees are cut at a limit, which the message after the listing tells.
//
// Control command "branch": what each panel shows ({branch, selection, base}) and the listings Go is running.

import './branch.css';
import { command } from '../core/registry.js';
import { Panel } from '../core/panel.js';
import { go, errText } from '../core/go.js';
import { esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { isBranchPath, isSelectionBranch, canBranch, branchPathFor, firstPart, realPath, progressText, doneText, pickWalk } from './branch-lib.js';

const group = new Intl.NumberFormat();
const fmt = (n) => group.format(n);

// ---- names to real files ----

Panel.pathHooks.push((panel, name) => (isBranchPath(panel.path) && panel.parent ? realPath(panel.parent, name, theApp.sepFor(panel.parent)) : ''));

// ---- entering and leaving ----

// leave goes back to the folder the branch view lists, with the cursor on where the file under the cursor came from.
function leave(app, p) {
  const e = p.cur;
  const focus = e && !e.up ? firstPart(e.name, app.platform) : null;
  return p.go(p.parent, { focus });
}

// refusal says why the active panel's folder can't be shown as a branch view ('' when it can).
function refusal(app, p) {
  if (app.demo) return 'This is the demo: its files are not real, so there is no branch view.';
  if (!canBranch(p.path) || p.vfs?.kind === 'archive') return 'The branch view works only for folders on this computer, not inside archives, on servers or in search results.';
  return '';
}

async function enter(app, p, path) {
  const ok = await p.go(path);
  if (!ok) return false;
  p.$.list.focus({ preventScroll: true });
  return true;
}

async function branchAll(app) {
  const p = app.cur;
  if (isBranchPath(p.path)) return leave(app, p);
  const why = refusal(app, p);
  if (why) { app.flash(why, 'err'); return false; }
  return enter(app, p, branchPathFor(p.path));
}

async function branchSelected(app) {
  const p = app.cur;
  if (isBranchPath(p.path)) return leave(app, p);
  const why = refusal(app, p);
  if (why) { app.flash(why, 'err'); return false; }
  // TC lists the selected folders (and files); with nothing selected, the folder under the cursor
  let items = p.selectedEntries();
  if (!items.length && p.cur && !p.cur.up && p.cur.dir) items = [p.cur];
  if (!items.length) { app.flash('Select the folders to list first.'); return false; }
  let path;
  try { path = await go.Branch.Select(p.path, items.map((e) => e.name)); } catch (e) { app.flash(errText(e), 'err'); return false; }
  return enter(app, p, path);
}

command({ name: 'cm_DirBranch', menuKey: true, label: 'Branch view (all files in subfolders)', short: 'Branch', checked: (app) => isBranchPath(app.cur?.path) && !isSelectionBranch(app.cur?.path), run: (app) => branchAll(app) });
command({ name: 'cm_DirBranchSel', menuKey: true, label: 'Branch view of selected folders', short: 'Branch sel', checked: (app) => isSelectionBranch(app.cur?.path), run: (app) => branchSelected(app) });

// ---- the path bar badge and the message after a listing ----

async function onPanel(app, p, info) {
  let badge = p.$.pbtns.querySelector('.mc-branch-badge');
  if (!isBranchPath(p.path)) { badge?.remove(); p.el.classList.remove('mc-branch'); return; }
  p.el.classList.add('mc-branch');
  if (!badge) {
    badge = document.createElement('span');
    badge.className = 'mc-branch-badge';
    p.$.pbtns.prepend(badge);
  }
  const sel = isSelectionBranch(p.path);
  badge.textContent = sel ? 'BRANCH · selected' : 'BRANCH';
  let bi = null;
  try { bi = await go.Branch.Info(p.path); } catch { /* the demo or no Go */ }
  badge.title = `${sel ? 'Branch view of the selected files and folders of' : 'Branch view: every file of'} ${p.parent}${sel ? '' : ' and its subfolders'}. `
    + 'File commands work on the real files. Ctrl+B or ".." goes back.' + (bi?.truncated ? ` Only the first ${fmt(bi.limit)} files are shown.` : '');
  if (info?.moved && bi) app.flash(doneText(bi, fmt), bi.truncated ? 'err' : undefined);
}

// ---- the notice while a branch view is read ----

// watch shows, over a panel that has been reading a branch view for a moment, how many files were found so far, with
// a Stop button. It runs only while a panel is reading, and asks Go only then. The panel's ownLoadingNotice flag
// tells the "Not responding" notice (slowread.js) to stay away: a big tree is slow, not stuck.
function watch(app) {
  const state = new Map(); // panel → {since, el}
  let busy = false;
  const tick = async () => {
    if (busy) return;
    const reading = app.panels.filter((p) => p.loading);
    for (const p of app.panels) if (!p.loading) drop(p, state);
    if (!reading.length) return;
    busy = true;
    let walks = [];
    try { walks = await go.Branch.Progress(); } catch { walks = []; } finally { busy = false; }
    const now = Date.now();
    for (const p of reading) {
      const s = state.get(p) || { since: now, el: null };
      state.set(p, s);
      const w = p.loading ? pickWalk(walks, p.path, p.other?.path) : null;
      if (!w) { if (s.el) drop(p, state); continue; }
      p.ownLoadingNotice = true;
      if (now - s.since < 400) continue;
      if (!s.el) s.el = show(p);
      const t = progressText(w.files, p.path, fmt);
      s.el.querySelector('.mc-branch-title').textContent = t.title;
      s.el.querySelector('.mc-branch-hint').textContent = t.hint;
    }
  };
  setInterval(tick, 250);
}

function drop(p, state) {
  const s = state.get(p);
  if (s?.el) s.el.remove();
  state.delete(p);
  p.ownLoadingNotice = false;
}

function show(p) {
  const el = document.createElement('div');
  el.className = 'slowread mc-branch-reading';
  el.setAttribute('role', 'status');
  el.innerHTML = `<div class="slowread-title mc-branch-title">${esc('Reading the subfolders…')}</div><div class="slowread-hint mc-branch-hint"></div><button type="button">Stop</button>`;
  el.addEventListener('mousedown', (ev) => ev.stopPropagation());
  el.querySelector('button').addEventListener('click', () => {
    go.FS.CancelList(p.side).catch(() => {});
    p.$.list.focus();
  });
  el.style.top = `${p.$.list.scrollTop + 30}px`;
  p.$.list.append(el);
  return el;
}

addControlCommand('branch', async () => {
  let walks = [];
  try { walks = await go.Branch.Progress(); } catch { /* no Go */ }
  const panels = {};
  for (const p of theApp.panels) panels[p.side] = { branch: isBranchPath(p.path), selection: isSelectionBranch(p.path), base: isBranchPath(p.path) ? p.parent : '' };
  return { ok: true, panels, walks };
});

export function init(app) {
  app.on('panel', (p, info) => { onPanel(app, p, info); });
  watch(app);
}
