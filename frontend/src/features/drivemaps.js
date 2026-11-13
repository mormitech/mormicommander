// Mapped folders: any folder can be put on the drive bar as a quick jump, under a name you choose ("Map to the drive
// bar…" in the context menu of a folder, of the folder's background and of the path bar; ours, no TC equivalent).
// The buttons come after the real drives and Home, behind a thin divider, so the real drives (and the drive letters
// on Windows) keep their places and keys. They also appear in the drive list (Alt+F1 / Alt+F2) and light up while a
// panel is inside them. A mapped button's own context menu renames, moves or removes it.
//
// Stored in the settings as "driveMaps": [{name, path}], in drive bar order; app.loadVolumes() adds them to the
// volumes with kind "map".

import { command } from '../core/registry.js';
import { inputDialog } from '../core/dialogs.js';
import { getSetting, setSetting } from '../core/settings.js';

const maps = () => (getSetting('driveMaps', []) || []).filter((m) => m && m.path);
const same = (app, a, b) => (app.platform === 'windows' ? a.toLowerCase() === b.toLowerCase() : a === b);

async function save(app, list) {
  await setSetting('driveMaps', list);
  await app.loadVolumes();
}

// mapFolder asks for a name and puts path on the drive bar (or renames it when it is there already).
export async function mapFolder(app, path) {
  if (!path) return false;
  const list = maps();
  const had = list.find((m) => same(app, m.path, path));
  const base = path.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || path;
  const name = await inputDialog({
    title: had ? 'Rename the drive bar button' : 'Map to the drive bar',
    label: `Name for ${path}`,
    value: had ? had.name : base,
    okLabel: had ? 'Rename' : 'Map',
    kind: 'drive-map',
    validate: (v) => (v.trim() ? '' : 'Type a name.'),
  });
  if (name == null) return false;
  if (had) had.name = name.trim();
  else list.push({ name: name.trim(), path });
  await save(app, list);
  app.flash(had ? `Renamed to "${name.trim()}".` : `"${name.trim()}" is on the drive bar.`);
  return true;
}

export async function unmapFolder(app, path) {
  await save(app, maps().filter((m) => !same(app, m.path, path)));
  return true;
}

// moveMap moves a mapped button one place left (-1) or right (+1) among the mapped buttons.
export async function moveMap(app, path, by) {
  const list = maps();
  const i = list.findIndex((m) => same(app, m.path, path));
  const j = i + by;
  if (i < 0 || j < 0 || j >= list.length) return false;
  [list[i], list[j]] = [list[j], list[i]];
  await save(app, list);
  return true;
}

export const isMapped = (app, path) => maps().some((m) => same(app, m.path, path));
export const mapIndex = (app, path) => maps().findIndex((m) => same(app, m.path, path));
export const mapCount = () => maps().length;

command({
  // args.path, else the folder under the cursor, else the panel's folder.
  name: 'cm_MapFolder', label: 'Map to the drive bar…',
  run: (app, args = {}) => {
    const p = app.cur;
    const e = p.cur;
    const path = args.path || (e && e.dir && !e.up ? p.fullPath(e.name) : p.path);
    return mapFolder(app, path);
  },
});
