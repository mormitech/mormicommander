// Git status in the panels (board issue #23, F-NEW-7): in a folder inside a git work tree, a status letter beside each
// name (M changed, A added, D deleted, R renamed, U conflict, ? not tracked, ! ignored), ignored files shown, dimmed or
// treated as hidden files, and a badge in the path bar with the branch, a dot when something is not committed and the
// commits ahead of / behind the upstream. Outside a repository nothing is drawn.
//
// It is off by default (Show > Git status turns it on; Configuration > Git status settings… chooses the parts), so the
// panels look and behave exactly as before for anyone who does not want it. While it is on, every folder a panel reads
// is asked about once (Go: Git.Status, which runs "git status --porcelain=v2" when git is installed); the answer is
// kept with the listing it belongs to, so a reread, a tab switch or Ctrl+U asks again and an old answer never paints
// a new folder. The letters are drawn through the core's row class hook, so every view that draws names in rows
// (Full, Brief, Comments, custom columns) shows them, and nothing of the core or the views feature is replaced.
//
// Settings: "git" = {enabled, column, branch, ignored: "show" | "dim" | "hide"} (git-lib.js has the defaults).
// Control command "git" returns each panel's answer.

import './git.css';
import { command } from '../core/registry.js';
import { go } from '../core/go.js';
import { Panel } from '../core/panel.js';
import { formDialog, esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { isLocal } from './system-lib.js';
import { gitConfig, statusOf, statusWord, rowClasses, hidesIgnored, branchText, branchTitle, IGNORED_MODES } from './git-lib.js';

const cfg = () => gitConfig(theApp.setting('git', {}));

// the answer for what the panel shows now, or null (none yet, or it belongs to a listing the panel no longer shows)
const current = (p) => (p.gitInfo && p.gitInfo.forAll === p.all ? p.gitInfo.r : null);

Panel.rowClassHooks.push((p, e) => {
  const r = current(p);
  return r ? rowClasses(statusOf(r, e), cfg()) : '';
});

// markHidden treats ignored files as hidden ones ("hide" mode), or takes back what it marked before; true when
// something changed (the panel then sorts and filters again).
function markHidden(p, r, c) {
  let changed = false;
  for (const e of p.all) {
    const want = !!r && c.enabled && hidesIgnored(statusOf(r, e), c);
    if (want && !e.hidden) { e.hidden = true; e.gitHidden = true; changed = true; }
    else if (!want && e.gitHidden) { e.hidden = false; delete e.gitHidden; changed = true; }
  }
  return changed;
}

// paint draws the badge and the panel class for the answer the panel has (or removes them).
function paint(p) {
  const c = cfg(), r = c.enabled ? current(p) : null;
  p.el.classList.toggle('mc-git-col', !!(r && r.repo && c.column));
  let badge = p.$.pbtns.querySelector('.mc-git-badge');
  const text = r && c.branch ? branchText(r) : '';
  if (!text) { badge?.remove(); return; }
  if (!badge) {
    badge = document.createElement('span');
    badge.className = 'mc-git-badge';
    p.$.pbtns.prepend(badge);
  }
  badge.textContent = text;
  badge.title = branchTitle(r);
  badge.classList.toggle('dirty', !!r.dirty);
}

// apply shows a new answer (or the lack of one) in the panel.
function apply(p) {
  const r = cfg().enabled ? current(p) : null;
  if (markHidden(p, r, cfg())) p.rebuild();
  else p.renderRows();
  paint(p);
}

let seq = 0;
// refresh asks Go about the folder the panel shows, unless it already has the answer for this listing.
async function refresh(p, force = false) {
  const c = cfg();
  if (!c.enabled || !p.path || !isLocal(p.path)) {
    if (p.gitInfo) { p.gitInfo = null; apply(p); } else paint(p);
    return;
  }
  if (!force && p.gitInfo && p.gitInfo.forAll === p.all) { paint(p); return; }
  const all = p.all, path = p.path, mine = ++seq;
  p.gitSeq = mine;
  let r;
  try { r = await go.Git.Status(path); } catch { r = null; }
  if (p.gitSeq !== mine || p.all !== all) return; // the panel moved on meanwhile; its own request answers it
  p.gitInfo = { forAll: all, path, r: r && r.repo ? r : null };
  apply(p);
}

const refreshAll = (force = false) => { for (const p of theApp.panels || []) refresh(p, force); };

function save(patch) {
  theApp.setSetting('git', { ...cfg(), ...patch });
  for (const p of theApp.panels || []) { if (!cfg().enabled) p.gitInfo = null; apply(p); }
  refreshAll(true);
}

command({
  name: 'cm_GitStatus', label: 'Git status', menu: 'Show', menuAfter: 'cm_SwitchHidSys',
  checked: () => cfg().enabled,
  run: (app, args = {}) => {
    const on = typeof args.on === 'boolean' ? args.on : !cfg().enabled;
    save({ enabled: on });
    app.flash(on ? 'Git status is shown in folders that are in a git repository.' : 'Git status is not shown.');
    return true;
  },
});

command({
  name: 'cm_GitStatusSettings', label: 'Git status settings…', menu: 'Configuration', menuAfter: 'cm_ConfigDirHotlist',
  run: async () => {
    const c = cfg();
    const chk = (id, on, label) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
    const r = await formDialog('Git status settings',
      `${chk('git-on', c.enabled, 'Show the git status of folders in a git repository')}
       ${chk('git-col', c.column, 'A status letter beside each name (M, A, D, R, U, ?, !)')}
       ${chk('git-branch', c.branch, 'The branch, uncommitted changes and ahead / behind in the path bar')}
       <div class="mc-row"><label for="git-ign">Ignored files</label><select id="git-ign">${IGNORED_MODES.map(([v, l]) => `<option value="${v}"${v === c.ignored ? ' selected' : ''}>${esc(l)}</option>`).join('')}</select></div>`,
      (b) => ({
        enabled: b.querySelector('#git-on').checked, column: b.querySelector('#git-col').checked,
        branch: b.querySelector('#git-branch').checked, ignored: b.querySelector('#git-ign').value,
      }), null, { kind: 'git-settings' });
    if (!r) return false;
    save(r);
    return true;
  },
});

addControlCommand('git', async () => {
  const out = { ok: true, settings: cfg() };
  for (const p of theApp.panels || []) {
    const r = current(p);
    out[p.side] = r ? { ...r, words: Object.fromEntries(Object.entries(r.files || {}).map(([k, v]) => [k, statusWord(v)])) } : null;
  }
  return out;
});

export function init(app) {
  app.on('panel', (p) => { refresh(p); });
  refreshAll();
}
