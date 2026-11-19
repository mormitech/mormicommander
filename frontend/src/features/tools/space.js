// Folder sizes (internal/dirsize through Tools.Space): Space on a folder counts it (F-118, F-736, option "countSpace",
// TC's CountSpace, F-797), Alt+Shift+Enter counts every folder in the panel (cm_CountDirContent, F-183, F-264), Ctrl+L
// shows the occupied space of the selection (cm_GetFileSpace, F-244, F-325, F-737). Counted sizes replace <DIR> in the
// size column (F-091) and count in the status line's selected total (F-752). While a folder is counted, its size
// column shows the running number and the status line says what is going on; Esc stops the count.
//
// Other features may count through countSizes too (the folder size treemap, features/treemap.js): they name themselves
// as owner, get onUpdate calls whenever a size changes, and stop their own counts with stopCounting; Esc then belongs
// to them, so the Esc handler here leaves counts that have an owner alone.

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { addKeyHandler } from '../../core/keyboard.js';
import { dialogOpen, messageDialog } from '../../core/dialogs.js';
import { addControlCommand } from '../../core/control.js';
import { track } from '../../core/busy.js';
import { bytes, size, plural } from '../../core/format.js';
import { newId, onProgress, runTool, isLocal, needLocal, targets, showError } from './common.js';
import { countSpaceOn } from './space-lib.js';
import { tm, tn, tjoin } from '../../core/i18n.js';

const scans = new Map(); // id → {panel, path, names, cancel, owner}

// countSizes counts folders of a panel in the background and writes the sizes into the size column. opts.owner names
// the feature that started it (Esc does not stop it then), opts.onUpdate runs after every change of a size and
// opts.hint replaces "Esc stops" in the status line.
export function countSizes(app, p, entries, { owner = '', onUpdate = null, hint = 'Esc stops' } = {}) {
  const dirs = entries.filter((e) => e.dir && !e.up);
  if (!dirs.length || !isLocal(app, p.path)) return Promise.resolve(null);
  const id = newId('size');
  const path = p.path;
  const names = dirs.map((e) => e.name);
  const live = (name) => (p.path === path ? p.entries[p.index.get(name)] : null);
  const status = (pr) => {
    if (p.path !== path) return;
    const what = names.length === 1 ? names[0] : `${pr.itemsDone ?? 0} of ${plural(names.length, 'folder')}`;
    p.$.status.textContent = `Counting ${what}… ${size(pr.bytes || 0)} in ${plural(pr.files || 0, 'file')} (${hint})`;
  };
  const stop = onProgress(id, (pr) => {
    if (p.path !== path) return;
    if (pr.itemDone !== undefined) {
      const e = live(names[pr.itemDone]);
      if (pr.complete) p.dirSizes.set(names[pr.itemDone], pr.bytes);
      if (e) e.dirSize = pr.complete ? pr.bytes : undefined;
      p.renderRows();
      onUpdate?.();
      return;
    }
    // running total of the folder being counted: its share is the total minus the folders already done
    const done = names.slice(0, pr.item).reduce((n, nm) => n + (p.dirSizes.get(nm) || 0), 0);
    const e = live(names[pr.item]);
    if (e) { e.dirSize = Math.max(0, (pr.bytes || 0) - done); p.renderRows(); onUpdate?.(); }
    status(pr);
  });
  status({ bytes: 0, files: 0, itemsDone: 0 });
  const scan = { panel: p, path, names, owner, cancel: () => go.Tools.Cancel(id) };
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
      onUpdate?.();
      if (r.canceled && !owner) app.flash('Stopped counting.');
    }
    return r;
  }).catch((e) => {
    if (p.path === path) { for (const n of names) { const en = live(n); if (en && !p.dirSizes.has(n)) en.dirSize = undefined; } p.renderRows(); p.renderStatus(); onUpdate?.(); }
    app.flash(String(e?.message ?? e).replace(/^Error:\s*/, ''), 'err');
    return null;
  }).finally(() => { stop(); scans.delete(id); }));
}

// stopCounting stops the counts a feature started (owner), only those of panel p when given.
export function stopCounting(owner, p = null) {
  for (const s of scans.values()) if (s.owner === owner && (!p || s.panel === p)) s.cancel();
}

// counting says whether a feature (owner) has a count running, in panel p when given.
export const counting = (owner, p = null) => [...scans.values()].some((s) => s.owner === owner && (!p || s.panel === p));

command({
  name: 'cm_CountDirContent', label: 'Calculate folder sizes',
  run: (app) => countSizes(app, app.cur, app.cur.entries),
});

command({
  name: 'cm_GetFileSpace', menuKey: true, label: 'Calculate occupied space…',
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
      tm('Total: {bytes} bytes ({size}) in {files}, {folders}', { bytes: bytes(t.bytes), size: size(t.bytes), files: tn(t.files, 'file'), folders: tn(t.dirs, 'folder') }),
      r.blockSize ? tm('Size on disk: {bytes} bytes ({size}), cluster size {block} bytes', { bytes: bytes(t.allocated), size: size(t.allocated), block: bytes(r.blockSize) })
        : tm('Size on disk: {bytes} bytes ({size})', { bytes: bytes(t.allocated), size: size(t.allocated) }),
    ];
    if (t.links) lines.push(tm('{what} not followed.', { what: tn(t.links, 'symbolic link') }));
    if (r.skipped) lines.push(tm('{what} on other drives or in link loops not counted.', { what: tn(r.skipped, 'folder') }));
    if (t.errors) lines.push(tm('{what} could not be read (no permission?).', { what: tn(t.errors, 'item') }));
    await messageDialog({ Title: 'Occupied space', Message: tjoin(lines), Kind: 'tools-space' });
  },
});

export function init(app) {
  // Space on a folder counts it when it becomes selected (TC's CountSpace option, default on). Ins and the mouse
  // never count; Space on ".." does nothing. Without a choice in settings.json, the settings file's SpaceCalcSize
  // decides (a copied wincmd.ini), see space-lib.js.
  app.on('space-select', (p, e) => {
    if (!e || e.up || !e.dir || !p.isSelected(e)) return;
    if (!countSpaceOn(app.setting('countSpace', undefined), app.config?.values?.['Operation.SpaceCalcSize'])) return;
    countSizes(app, p, [e]);
  });
  // Esc stops counting (only while a count runs and no dialog is open).
  addKeyHandler((ev, combo) => {
    const own = [...scans.values()].filter((s) => !s.owner);
    if (combo !== 'Esc' || !own.length || dialogOpen()) return false;
    for (const s of own) s.cancel();
    return true;
  });
}

addControlCommand('tools', async () => ({ ok: true, counting: [...scans.values()].map((s) => ({ side: s.panel.side, path: s.path, names: s.names })) }));
