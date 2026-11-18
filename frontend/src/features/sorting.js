// Sorting the active panel (FEATURES.md section 5): Ctrl+F3 name, Ctrl+F4 extension, Ctrl+F5 date (newest first),
// Ctrl+F6 size (largest first), Ctrl+F7 unsorted; the same key again reverses. Clicking a column title runs these too.

import { command } from '../core/registry.js';

const sortCmd = (name, label, key) => command({
  name, label, menuKey: true, checked: (app) => app.cur.sort.key === key,
  run: (app) => app.cur.sortBy(key),
});
sortCmd('cm_SrcByName', 'Sort by name', 'name');
sortCmd('cm_SrcByExt', 'Sort by extension', 'ext');
sortCmd('cm_SrcByDateTime', 'Sort by date', 'date');
sortCmd('cm_SrcBySize', 'Sort by size', 'size');
command({ name: 'cm_SrcUnsorted', menuKey: true, label: 'Unsorted', checked: (app) => app.cur.sort.key === 'none', run: (app) => app.cur.sortBy('none', false) });
command({ name: 'cm_SrcNegOrder', menuKey: true, label: 'Reverse order', run: (app) => app.cur.sortBy(app.cur.sort.key, !app.cur.sort.desc) });
