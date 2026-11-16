// Context menus (FEATURES.md section 37, docs/spec/area-8.md section 7) as the page's own popup lists, so they work
// the same on every system and control mode can read and choose them ("popup" / "popup_choose"):
//   - files (right-click, Shift+F10, the Menu key; F-703, F-708): Open, Open with…, View, Edit, Cut, Copy, Paste, Copy
//     path, Copy name, Rename, Delete, Pack…, Show in Finder, Open terminal here, Properties;
//   - the empty part of the list or ".." (F-704): New folder, New file, Paste, Reread, Sort by…, Open terminal here,
//     Show the folder in Finder, Copy path, Folder properties…;
//   - a drive button (F-059, F-705, F-710): Open, Open in the other panel, Eject (removable and network volumes),
//     Properties…; cm_EjectVolume ejects the volume of the active panel;
//   - the path bar: Copy path, Edit path, Show in Finder, Open terminal here.
// Items whose command no feature has yet are greyed out, not hidden.

import { command, hasCommand, getCommand } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { showList } from '../core/popup.js';
import { modal, esc } from '../core/dialogs.js';
import { size, bytes, plural } from '../core/format.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { isLocal, baseName } from './system-lib.js';
import { anchorFor, revealLabel, openWith } from './system.js';
import { mapFolder, unmapFolder, moveMap, isMapped, mapIndex, mapCount } from './drivemaps.js';

const SEP = { sep: true };

// item is a menu line that runs a command (greyed out when it is not there or not possible now).
function item(app, label, cmd, args) {
  const c = getCommand(cmd);
  const off = !hasCommand(cmd) || (c?.enabled ? !c.enabled(app) : false);
  return { label, detail: app.keyLabel(cmd), value: { cmd, args }, disabled: off };
}
const fn = (label, run, disabled = false, detail = '') => ({ label, detail, value: { run }, disabled });

// menu shows a list and runs what was chosen.
async function menu(app, title, items, anchor, kind) {
  const list = items.filter((x, i, a) => !(x.sep && (i === 0 || a[i - 1].sep || i === a.length - 1)));
  const r = await showList({ title, items: list, anchor, kind });
  if (!r) return false;
  const v = r.item.value;
  try {
    if (v.run) return await v.run();
    return await app.run(v.cmd, v.args || {});
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
}

async function copyText(app, text, what) {
  await go.SystemClip.WriteText(text);
  app.flash(`Copied ${what} to the clipboard.`);
  return true;
}

// ---- files and the folder's background ----

function fileMenu(app, p, args) {
  const list = p.targets();
  const one = list.length === 1 ? list[0] : null;
  const paths = list.map((e) => p.fullPath(e.name));
  const local = isLocal(p.path);
  const nl = app.platform === 'windows' ? '\r\n' : '\n';
  const title = one ? one.name : plural(list.length, 'item');
  const items = [
    item(app, 'Open', 'cm_Enter'),
    one?.dir ? item(app, 'Open in a new tab', 'cm_OpenDirInNewTab') : null,
    one?.dir && !one.up ? fn(isMapped(app, p.fullPath(one.name)) ? 'Rename on the drive bar…' : 'Map to the drive bar…', () => mapFolder(app, p.fullPath(one.name))) : null,
    fn('Open with…', () => openWith(app, args), !local || list.every((e) => e.dir && !/\.app$/i.test(e.name))),
    item(app, 'View', 'cm_List'),
    item(app, 'Edit', 'cm_Edit'),
    SEP,
    item(app, 'Cut', 'cm_CutToClipboard'),
    item(app, 'Copy', 'cm_CopyToClipboard'),
    item(app, 'Paste', 'cm_PasteFromClipboard'),
    SEP,
    fn(paths.length > 1 ? 'Copy paths' : 'Copy path', () => copyText(app, paths.join(nl), paths.length > 1 ? 'the paths' : 'the path')),
    fn(list.length > 1 ? 'Copy names' : 'Copy name', () => copyText(app, list.map((e) => e.name).join(nl), list.length > 1 ? 'the names' : 'the name')),
    SEP,
    item(app, 'Rename', 'cm_RenameOnly'),
    item(app, 'Delete', 'cm_Delete'),
    SEP,
    item(app, 'Pack…', 'cm_PackFiles'),
    one && !one.dir ? item(app, 'Split…', 'cm_FileSplit') : null,
    SEP,
    { ...item(app, revealLabel(app), 'cm_Explorer'), disabled: !local || !hasCommand('cm_Explorer') },
    { ...item(app, 'Open terminal here', 'cm_ExecuteDOS'), disabled: !local },
    item(app, 'Properties', 'cm_Properties'),
  ].filter(Boolean);
  return menu(app, title, items, anchorFor(p, args), 'context-file');
}

function backgroundMenu(app, p, args) {
  const local = isLocal(p.path);
  const items = [
    item(app, 'New folder…', 'cm_MkDir'),
    item(app, 'New file…', 'cm_EditNewFile'),
    item(app, 'Paste', 'cm_PasteFromClipboard'),
    SEP,
    item(app, 'Reread', 'cm_RereadSource'),
    fn('Sort by…', () => sortMenu(app, p, args)),
    SEP,
    { ...item(app, 'Open terminal here', 'cm_ExecuteDOS'), disabled: !local },
    fn(revealLabel(app), () => go.SystemLaunch.OpenFolder(p.path), !local),
    fn('Copy path', () => copyText(app, p.path, 'the path')),
    fn(isMapped(app, p.path) ? 'Rename on the drive bar…' : 'Map this folder to the drive bar…', () => mapFolder(app, p.path)),
    fn('Folder properties…', () => folderProps(app, p)),
  ];
  return menu(app, baseName(p.path) || p.path, items, args.x != null ? { x: args.x, y: args.y } : p.$.list, 'context-folder');
}

function sortMenu(app, p, args) {
  const items = [['Name', 'cm_SrcByName'], ['Extension', 'cm_SrcByExt'], ['Date', 'cm_SrcByDateTime'], ['Size', 'cm_SrcBySize'], ['Unsorted', 'cm_SrcUnsorted']]
    .map(([l, c]) => ({ ...item(app, l, c), detail: (getCommand(c)?.checked?.(app) ? '✓ ' : '') + app.keyLabel(c) }));
  return menu(app, 'Sort by', items, args.x != null ? { x: args.x, y: args.y } : p.$.list, 'context-sort');
}

// props shows a box of name/value lines with buttons; the answer is the label of the button pressed.
function props(title, rows, buttons, kind, note = '') {
  return modal((box, done) => {
    box.classList.add('mc-system-props');
    box.innerHTML = `<h2>${esc(title)}</h2><table>${rows.filter(([, v]) => v !== '' && v != null).map(([k, v]) => `<tr><th>${esc(k)}</th><td>${esc(v)}</td></tr>`).join('')}</table>
      ${note ? `<p class="mc-note">${esc(note)}</p>` : ''}<div class="mc-btns">${buttons.map((b, i) => `<button type="button"${i === buttons.length - 1 ? ' class="primary"' : ''}>${esc(b)}</button>`).join('')}</div>`;
    box.querySelectorAll('button').forEach((b) => { b.onclick = () => done(b.textContent); });
    box.onsubmit = (ev) => { ev.preventDefault(); done(buttons.at(-1)); };
  }, 'Close', { kind });
}

async function folderProps(app, p) {
  const s = p.stats();
  const v = await go.FS.VolumeOf(p.path).catch(() => null);
  await props(baseName(p.path) || p.path, [
    ['Folder', p.path],
    ['Contains', `${plural(s.files, 'file')}, ${plural(s.dirs, 'folder')}`],
    ['Size of the files', `${size(s.total)} (${bytes(s.total)} bytes)`],
    ['Volume', v ? v.name || v.path : ''],
    ['Free on the volume', v?.total ? `${size(v.free)} of ${size(v.total)}` : ''],
  ], ['Close'], 'folder-props');
  return true;
}

// ---- drives (section 38) ----

const ejectable = (app, v) => v && ['removable', 'network', 'cdrom'].includes(v.kind) && v.path !== '/' && !app.demo;

// eject moves the panels off the volume, then ejects it (F-710).
async function eject(app, v) {
  for (const p of app.panels) if (app.volumeFor(p.path) === v) await p.go(app.home);
  try {
    await go.SystemFiles.Eject(v.path);
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
  app.flash(v.kind === 'network' ? `Disconnected ${v.name}.` : `Ejected ${v.name}. It can be removed now.`);
  // a mapped network share's button goes with its volume (ejecting is how such a button is removed)
  for (const m of app.volumes.filter((x) => x.kind === 'map' && x.net && x.mount === v.path)) await unmapFolder(app, m.path);
  await app.loadVolumes();
  return true;
}

// volumeProps shows what is known about a volume (its Properties box). note: an extra sentence under the table.
async function volumeProps(app, v, note = '') {
  const x = await go.SystemFiles.VolumeProps(v.path);
  const buttons = ejectable(app, v) ? ['Eject', 'Close'] : ['Close'];
  const b = await props(v.label ? `${v.name} ${v.label}` : v.name, [
    ['Name', x.name || v.name],
    ['Mounted at', x.root || v.path],
    ['Kind', { fixed: 'Built-in disk', removable: 'Removable', network: 'Network', cdrom: 'Optical disc', home: 'Home folder' }[v.kind] || v.kind],
    ['File system', x.fileSystem],
    ['Device', x.device],
    ['Size', x.total ? `${size(x.total)} (${bytes(x.total)} bytes)` : ''],
    ['Used', x.total ? `${size(x.used)} (${Math.round((x.used / x.total) * 100)}%)` : ''],
    ['Free', x.total ? `${size(x.available)} (${bytes(x.available)} bytes)` : ''],
    ['Read only', x.readOnly ? 'Yes' : ''],
  ], buttons, 'volume-props', note);
  if (b === 'Eject') return eject(app, v);
  return true;
}

// ejectShare disconnects a mapped network share (when it is connected) and takes its button off the drive bar: for such
// a button Eject is the way to remove it.
async function ejectShare(app, v) {
  const vol = v.mount ? app.volumes.find((x) => x.path === v.mount) : null;
  if (vol && !(await eject(app, vol))) return false; // (eject takes the button off too)
  if (!vol) { await unmapFolder(app, v.path); app.flash(`Removed ${v.name}.`); }
  return true;
}

function driveMenu(app, p, v, anchor) {
  if (v.kind === 'map' && v.net) {
    // a mapped network share is a network drive: it is ejected, not removed (core/drivemerge.js)
    return menu(app, `${v.name} – ${v.path}`, [
      fn('Open', () => { app.setActive(p); return p.go(v.path); }),
      fn('Open in the other panel', () => p.other.go(v.path)),
      SEP,
      fn('Rename…', () => mapFolder(app, v.path)),
      SEP,
      fn('Eject', () => ejectShare(app, v)),
    ], anchor, 'context-drive');
  }
  if (v.kind === 'map') {
    const i = mapIndex(app, v.path), n = mapCount();
    return menu(app, `${v.name} – ${v.path}`, [
      fn('Open', () => { app.setActive(p); return p.go(v.path); }),
      fn('Open in the other panel', () => p.other.go(v.path)),
      SEP,
      fn('Rename…', () => mapFolder(app, v.path)),
      fn('Move left', () => moveMap(app, v.path, -1), i <= 0),
      fn('Move right', () => moveMap(app, v.path, 1), i < 0 || i >= n - 1),
      SEP,
      fn('Remove from the drive bar', () => unmapFolder(app, v.path)),
    ], anchor, 'context-drive');
  }
  const items = [
    fn('Open', () => { app.setActive(p); return p.go(v.path); }),
    fn('Open in the other panel', () => p.other.go(v.path)),
    SEP,
    fn('Eject', () => eject(app, v), !ejectable(app, v)),
    fn('Properties…', () => volumeProps(app, v)),
  ];
  return menu(app, v.label ? `${v.name} ${v.label}` : v.name, items, anchor, 'context-drive');
}

command({
  name: 'cm_EjectVolume', label: 'Eject volume', menu: 'Commands', menuAfter: 'cm_VolumeId',
  enabled: (app) => ejectable(app, app.volumeFor(app.cur?.path || '')),
  run: (app) => {
    const v = app.volumeFor(app.cur.path);
    if (!ejectable(app, v)) { app.flash('This volume can\'t be ejected.'); return false; }
    return eject(app, v);
  },
});

// cm_VolumeId (Commands > Volume information, board issue #38): Total Commander's "Volume label" dialog changes the
// label of a drive. Here it shows the Properties box of the volume the active panel is on (name, mount point, file
// system, device, size, free space). Changing the label is deliberately not offered: on macOS it means running
// diskutil rename on a mounted disk, and a mistake there renames the wrong volume, so the box says where the system
// lets the user do it safely instead.
const RENAME_NOTE = {
  darwin: 'To change the name of the volume, rename it in Finder or in Disk Utility.',
  windows: 'To change the label of the drive, use its Properties in Explorer.',
  linux: 'To change the label of the volume, use the disk tool of your desktop or the file system\'s own label tool.',
};
command({
  name: 'cm_VolumeId', label: 'Volume information…',
  run: (app) => {
    const path = app.cur?.path || '';
    const v = app.volumeFor(path);
    if (!v || /^[a-z][a-z0-9+.-]*:\/\//i.test(path)) { app.flash('The active panel is not on a volume of this computer.'); return false; }
    return volumeProps(app, v, RENAME_NOTE[app.platform] || RENAME_NOTE.linux);
  },
});

// ---- the command ----

command({
  // Shift+F10 / the Menu key / right-click: the menu of the selection, of the file under the cursor, or (on ".." or
  // the empty part of the list) of the folder.
  name: 'cm_ContextMenu', label: 'Context menu',
  run: (app, args = {}) => {
    const p = app.cur;
    let onRow = true;
    if (args.mouse && typeof args.onRow === 'boolean') onRow = args.onRow;
    else if (args.mouse && args.x != null) onRow = !!document.elementFromPoint(args.x, args.y)?.closest?.('.row');
    const e = p.cur;
    if (!onRow || !e || (e.up && !p.selectedEntries().length)) return backgroundMenu(app, p, args);
    // a right-click on a file outside the selection is about that file alone (as in Finder / Explorer)
    if (args.mouse && !p.isSelected(e) && p.selectedEntries().length) p.clearSelection();
    return fileMenu(app, p, args);
  },
});

export function init(app) {
  for (const p of app.panels) {
    p.$.drives.addEventListener('contextmenu', (ev) => {
      const b = ev.target.closest('button');
      if (!b) return;
      ev.preventDefault();
      const v = app.volumes[+b.dataset.i];
      if (v) driveMenu(app, p, v, { x: ev.clientX, y: ev.clientY });
    });
    p.$.pathbar.addEventListener('contextmenu', (ev) => {
      ev.preventDefault();
      app.setActive(p);
      const local = isLocal(p.path);
      menu(app, p.path, [
        fn('Copy path', () => copyText(app, p.path, 'the path')),
        item(app, 'Edit path', 'cm_EditPath'),
        fn(isMapped(app, p.path) ? 'Rename on the drive bar…' : 'Map this folder to the drive bar…', () => mapFolder(app, p.path)),
        SEP,
        fn(revealLabel(app), () => go.SystemLaunch.OpenFolder(p.path), !local),
        { ...item(app, 'Open terminal here', 'cm_ExecuteDOS'), disabled: !local },
      ], { x: ev.clientX, y: ev.clientY }, 'context-path');
    });
  }
}

// drive_menu {side, name|index}: opens the menu of a drive button, as a right-click does (for control mode).
addControlCommand('drive_menu', async (m) => {
  const app = theApp;
  const p = app.panel(m.side || 'active');
  const v = m.name != null ? app.volumes.find((x) => x.name === m.name) : app.volumes[m.index ?? 0];
  if (!v) throw new Error(`no drive ${m.name ?? m.index}`);
  driveMenu(app, p, v, p.$.drives);
  return null;
});
