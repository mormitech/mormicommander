// Folder sizes (internal/dirsize through Tools.Space): Space on a folder counts it (F-118, F-736, option "countSpace",
// TC's CountSpace, F-797), Alt+Shift+Enter counts every folder in the panel (cm_CountDirContent, F-183, F-264), Ctrl+L
// shows the occupied space of the selection (cm_GetFileSpace, F-244, F-325, F-737). Counted sizes replace <DIR> in the
// size column (F-091) and count in the status line's selected total (F-752). While a folder is counted, its size
// column shows the running number and the status line says what is going on; Esc stops the count.

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { addKeyHandler } from '../../core/keyboard.js';
import { dialogOpen, messageDialog } from '../../core/dialogs.js';
import { addControlCommand } from '../../core/control.js';
import { track } from '../../core/busy.js';
import { bytes, size, plural } from '../../core/format.js';
import { newId, onProgress, runTool, isLocal, needLocal, targets, showError } from './common.js';

const scans = new Map(); // id → {panel, path, names, cancel}

// countSizes counts folders of a panel in the background and writes the sizes into the size column.
export function countSizes(app, p, entries) {
  const dirs = entries.filter((e) => e.dir && !e.up);
  if (!dirs.length || !isLocal(app, p.path)) return Promise.resolve(null);
  const id = newId('size');
  const path = p.path;
  const names = dirs.map((e) => e.name);
  const live = (name) => (p.path === path ? p.entries[p.index.get(name)] : null);
  const status = (pr) => {
    if (p.path !== path) return;
    const what = names.length === 1 ? names[0] : `${pr.itemsDone ?? 0} of ${plural(names.length, 'folder')}`;
    p.$.status.textContent = `Counting ${what}… ${size(pr.bytes || 0)} in ${plural(pr.files || 0, 'file')} (Esc stops)`;
  };
  const stop = onProgress(id, (pr) => {
    if (p.path !== path) return;
    if (pr.itemDone !== undefined) {
      const e = live(names[pr.itemDone]);
      if (pr.complete) p.dirSizes.set(names[pr.itemDone], pr.bytes);
      if (e) e.dirSize = pr.complete ? pr.bytes : undefined;
      p.renderRows();
      return;
    }
    // running total of the folder being counted: its share is the total minus the folders already done
    const done = names.slice(0, pr.item).reduce((n, nm) => n + (p.dirSizes.get(nm) || 0), 0);
    const e = live(names[pr.item]);
    if (e) { e.dirSize = Math.max(0, (pr.bytes || 0) - done); p.renderRows(); }
    status(pr);
  });
  status({ bytes: 0, files: 0, itemsDone: 0 });
  const scan = { panel: p, path, names, cancel: () => go.Tools.Cancel(id) };
  scans.set(id, scan);
  return track(go.Tools.Space(id, dirs.map((e) => p.fullPath(e.name))).then((r) => {
    if (p.path === path) {
      r.items.forEach((it, i) => {
        const e = live(names[i]);
        if (it.complete) { p.dirSizes.set(names[i], it.bytes); if (e) e.dirSize = it.bytes; }
        else { p.dirSizes.delete(names[i]); if (e) e.dirSize = undefined; }
      });
      p.renderRows();
      p.renderStatus();
      if (r.canceled) app.flash('Stopped counting.');
    }
    return r;
  }).catch((e) => {
    if (p.path === path) { for (const n of names) { const en = live(n); if (en && !p.dirSizes.has(n)) en.dirSize = undefined; } p.renderRows(); p.renderStatus(); }
    app.flash(String(e?.message ?? e).replace(/^Error:\s*/, ''), 'err');
    return null;
  }).finally(() => { stop(); scans.delete(id); }));
}

command({
  name: 'cm_CountDirContent', label: 'Calculate folder sizes',
  run: (app) => countSizes(app, app.cur, app.cur.entries),
});

command({
  name: 'cm_GetFileSpace', label: 'Calculate occupied space…',
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (!paths.length) { app.flash('Nothing is selected.'); return; }
    let r;
    try {
      r = await runTool('Calculating occupied space', (id) => go.Tools.Space(id, paths), { delay: 250 });
    } catch (e) { await showError(e.message); return; }
    // computed folder sizes go into the size column, as Space does
    r.items.forEach((it, i) => {
      const e = entries[i];
      if (e?.dir && it.complete) { p.dirSizes.set(e.name, it.bytes); e.dirSize = it.bytes; }
    });
    p.renderRows();
    p.renderStatus();
    if (r.canceled) { app.flash('Stopped counting.'); return; }
    const t = r.total;
    const lines = [
      `Total: ${bytes(t.bytes)} bytes (${size(t.bytes)}) in ${plural(t.files, 'file')}, ${plural(t.dirs, 'folder')}`,
      `Size on disk: ${bytes(t.allocated)} bytes (${size(t.allocated)})${r.blockSize ? `, cluster size ${bytes(r.blockSize)} bytes` : ''}`,
    ];
    if (t.links) lines.push(`${plural(t.links, 'symbolic link')} not followed.`);
    if (r.skipped) lines.push(`${plural(r.skipped, 'folder')} on other drives or in link loops not counted.`);
    if (t.errors) lines.push(`${plural(t.errors, 'item')} could not be read (no permission?).`);
    await messageDialog({ Title: 'Occupied space', Message: lines.join('\n'), Kind: 'tools-space' });
  },
});

export function init(app) {
  // Space on a folder counts it when it becomes selected (TC's CountSpace option, default on). Ins and the mouse
  // never count; Space on ".." does nothing.
  app.on('space-select', (p, e) => {
    if (!e || e.up || !e.dir || !p.isSelected(e)) return;
    if (!app.setting('countSpace', true)) return;
    countSizes(app, p, [e]);
  });
  // Esc stops counting (only while a count runs and no dialog is open).
  addKeyHandler((ev, combo) => {
    if (combo !== 'Esc' || !scans.size || dialogOpen()) return false;
    for (const s of scans.values()) s.cancel();
    return true;
  });
}

addControlCommand('tools', async () => ({ ok: true, counting: [...scans.values()].map((s) => ({ side: s.panel.side, path: s.path, names: s.names })) }));
