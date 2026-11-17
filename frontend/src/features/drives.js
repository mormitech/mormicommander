// The drive popup (Alt+F1 for the left panel, Alt+F2 for the right, F-058 / F-174 / F-175): every drive or volume with
// its free space; Up/Down and Enter, or the first letter of its name (the drive letter on Windows) picks it (F-783).

import { command } from '../core/registry.js';
import { showList } from '../core/popup.js';
import { size } from '../core/format.js';

async function openDrives(app, panel) {
  await app.loadVolumes();
  const vols = app.volumes.filter((v) => !v.hidden); // (a mounted share a mapped button stands for: core/drivemerge.js)
  if (!vols.length) { app.flash('No drives found.'); return false; }
  const on = app.driveFor(panel.path);
  const items = vols.map((v) => ({
    label: v.label ? `${v.name} ${v.label}` : v.name,
    detail: v.stale ? `${v.path}: not responding` : v.total ? `${size(v.free)} free of ${size(v.total)}` : v.path,
    key: v.kind === 'home' ? '~' : v.kind === 'map' ? '' : app.platform === 'windows' ? v.name[0] : '',
    value: v,
  }));
  const r = await showList({ title: `Drive for the ${panel.side} panel`, items, anchor: panel.$.drives, index: Math.max(0, vols.indexOf(on)), kind: 'drives' });
  if (!r) return false;
  app.setActive(panel);
  return panel.go(app.drivePath(panel, r.item.value));
}

command({ name: 'cm_LeftOpenDrives', label: 'Drive list, left panel', run: (app) => openDrives(app, app.left) });
command({ name: 'cm_RightOpenDrives', label: 'Drive list, right panel', run: (app) => openDrives(app, app.right) });
command({ name: 'cm_SrcOpenDrives', label: 'Drive list', run: (app) => openDrives(app, app.cur) });
command({ name: 'cm_TrgOpenDrives', label: 'Drive list, other panel', run: (app) => openDrives(app, app.other) });
