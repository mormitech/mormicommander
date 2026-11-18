// Start-up parameters the page applies once the panels are ready (FEATURES.md section 41): -T opens the folders in new
// tabs (F-730), -S takes them as source and target (F-731), -P= picks the active panel (F-732), -S=L / -S=C / -S=S start
// the viewer, compare or synchronize on their own (F-852; -S=L opens only a Lister window with Options > Operation >
// StandaloneTools on). A second start with -O (or with "Allow only one copy" on)
// sends its command line here instead of opening a window (F-729, F-735): Go emits "config:open" with the same fields.
// A path to a file opens its folder with the cursor on the file, as Total Commander does.

import { hasCommand } from '../../core/registry.js';
import { go, on } from '../../core/go.js';
import { F } from '../../core/format.js';

async function show(app, panel, path, newTab) {
  if (!path) return;
  let dir = path, name = '';
  try {
    const st = await go.FS.Stat(path);
    if (st && !(st.flags & F.DIR)) { dir = await go.FS.Parent(path); name = st.name || path.split(/[\\/]/).pop(); }
  } catch { /* let the panel say what is wrong */ }
  if (newTab && hasCommand('cm_OpenNewTab')) {
    const was = app.cur;
    app.setActive(panel, { focus: false });
    await app.run('cm_OpenNewTab', {}, { quiet: true });
    app.setActive(was, { focus: false });
  }
  if (await panel.go(dir) && name) panel.focusName(name);
}

export async function applyStart(app, info) {
  if (!info) return;
  if (info.tool) return startTool(app, info);
  if (info.active === 'L' || info.active === 'R') app.setActive(info.active === 'R' ? app.right : app.left);
  let { left, right } = info;
  // -S without -P: the first folder goes to the panel that is active now
  if (info.sourceTarget && !info.active && app.cur === app.right) [left, right] = [right, left];
  await Promise.all([show(app, app.left, left, info.newTab), show(app, app.right, right, info.newTab)]);
}

async function startTool(app, info) {
  const f1 = info.file1, f2 = info.file2;
  switch (info.tool) {
    case 'lister':
      // only the Lister window (Options > Operation > StandaloneTools; Go hid the file manager's window): the panels
      // stay as they are, so nothing of this run changes the saved session
      if (info.standalone && f1) return app.run('cm_List', { path: f1, standalone: true });
      if (!f1) { app.flash('Give the file to view: mormicommander -S=L file'); return; }
      await show(app, app.left, f1, false);
      app.setActive(app.left);
      return app.run('cm_List', { file: f1, standalone: true, options: info.toolOpts || '' });
    case 'compare':
      if (f1) await show(app, app.left, f1, false);
      if (f2) await show(app, app.right, f2, false);
      return app.run('cm_CompareFilesByContent', { files: [f1, f2].filter(Boolean), standalone: true });
    case 'sync':
      if (f1) await show(app, app.left, f1, false);
      if (f2) await show(app, app.right, f2, false);
      return app.run('cm_SyncDirs', { standalone: true });
  }
  return false;
}

export function initStart(app) {
  app.on('ready', async () => {
    const info = await go.ConfigStart.StartParams().catch(() => null);
    if (info && (info.left || info.right || info.tool || info.newTab)) await applyStart(app, info);
  });
  on('config:open', (info) => { applyStart(app, info).catch((e) => app.flash(String(e?.message || e), 'err')); });
}
