// The menu bar, laid out like Total Commander's (F-010): Files, Mark, Commands, Net, Show, Configuration, Start, Help.
// Each item names a cm_* command; the menu runs it exactly like its key does. Items whose command no feature has
// registered yet are greyed out, so the menu also shows how far parity has come. Go builds the native menu from this
// tree (menu.go); on macOS the application menu, Edit and Window are added there.
//
// A feature does not need to edit this file for a command that is listed here. A command that is not listed can add
// itself to a menu with `menu: 'Show'` (and optionally `menuAfter: 'cm_SrcLong'`) in its command({...}).

import { go, on } from './go.js';
import { commandList, getCommand, hasCommand, onChange } from './registry.js';
import { app, LABELS } from './app.js';
import { menuLabel } from './hidden.js';

// [label, command] or '-' for a separator or [label, [items]] for a submenu.
export const MENUS = [
  ['Files', [
    ['Change attributes…', 'cm_SetAttrib'], ['Pack…', 'cm_PackFiles'], ['Unpack…', 'cm_UnpackFiles'], ['Test archives', 'cm_TestArchive'],
    ['Compare by content…', 'cm_CompareFilesByContent'], ['Internal associations…', 'cm_InternalAssociate'], ['Properties…', 'cm_Properties'],
    ['Calculate occupied space…', 'cm_GetFileSpace'], ['Multi-rename tool…', 'cm_MultiRenameFiles'], ['Edit comment…', 'cm_EditComment'], '-',
    ['Split file…', 'cm_FileSplit'], ['Combine files…', 'cm_FileCombine'], ['Encode file…', 'cm_UUEncode'], ['Decode file…', 'cm_UUDecode'],
    ['Create checksum files…', 'cm_CRCCreate'], ['Verify checksums…', 'cm_CRCCheck'], '-',
    ['Create symbolic link…', 'cm_CreateSymbolicLink'], ['Create hard link…', 'cm_CreateHardLink'], ['Wipe…', 'cm_Wipe'], '-',
    ['Print file list…', 'cm_PrintDir'], '-', ['Quit', 'cm_Exit'],
  ]],
  ['Mark', [
    ['Select group…', 'cm_SpreadSelection'], ['Unselect group…', 'cm_ShrinkSelection'], ['Select all', 'cm_SelectAll'], ['Unselect all', 'cm_ClearAll'],
    ['Invert selection', 'cm_ExchangeSelection'], ['Select all with the same extension', 'cm_SelectCurrentExtension'],
    ['Unselect all with the same extension', 'cm_UnselectCurrentExtension'], ['Select all with the same name', 'cm_SelectCurrentName'],
    ['Restore selection', 'cm_RestoreSelection'], '-',
    ['Save selection', 'cm_SaveSelection'], ['Load selection', 'cm_LoadSelection'], ['Save selection to file…', 'cm_SaveSelectionToFile'],
    ['Load selection from file…', 'cm_LoadSelectionFromFile'], ['Load selection from clipboard', 'cm_LoadSelectionFromClip'], '-',
    ['Copy names to clipboard', 'cm_CopyNamesToClip'], ['Copy names with full path', 'cm_CopyFullNamesToClip'], ['Copy names with details', 'cm_CopyDetailsToClip'],
    ['Copy folder path', 'cm_CopySrcPathToClip'], '-',
    ['Compare folders', 'cm_CompareDirs'], ['Compare folders with subfolders', 'cm_CompareDirsWithSubdirs'],
  ]],
  ['Commands', [
    ['Folder hotlist', 'cm_DirectoryHotlist'], ['Folder history', 'cm_DirectoryHistory'], ['Folder tree…', 'cm_CDtree'], '-',
    ['Find files…', 'cm_SearchFor'], ['Find files in a new window…', 'cm_SearchStandalone'], ['Volume information…', 'cm_VolumeId'], ['Synchronize folders…', 'cm_SyncDirs'], '-',
    ['Target = source', 'cm_MatchSrc'], ['Swap panels', 'cm_Exchange'], ['Swap panels with tabs', 'cm_ExchangeWithTabs'],
    ['Show folder under cursor in left panel', 'cm_TransferLeft'], ['Show folder under cursor in right panel', 'cm_TransferRight'], '-',
    ['Branch view (all files in subfolders)', 'cm_DirBranch'], ['Branch view of selected folders', 'cm_DirBranchSel'], '-',
    ['Open terminal here', 'cm_ExecuteDOS'], ['__REVEAL__', 'cm_Explorer'], ['Go to desktop folder', 'cm_OpenDesktop'], ['Go to home folder', 'cm_GoToHome'],
    ['Go to root folder', 'cm_GoToRoot'], '-',
    ['Command line', 'cm_FocusCmdLine'], ['All commands…', 'cm_CommandBrowser'],
  ]],
  ['Net', [
    ['FTP connect…', 'cm_FtpConnect'], ['New FTP connection…', 'cm_FtpNew'], ['Disconnect', 'cm_FtpDisconnect'], '-',
    ['Show hidden FTP files', 'cm_FtpHiddenFiles'], ['Transfer mode', 'cm_FtpTransferMode'], ['Send FTP command…', 'cm_FtpSendCommand'], '-',
    ['Background transfers…', 'cm_BackgroundTransferManager'], ['Download list…', 'cm_FtpDownloadList'], '-',
    ['Network neighbourhood…', 'cm_OpenNetwork'], ['Connect network drive…', 'cm_NetConnect'], ['Disconnect network drive…', 'cm_NetDisconnect'],
  ]],
  ['Show', [
    ['Brief', 'cm_SrcShort'], ['Full', 'cm_SrcLong'], ['Comments', 'cm_SrcComments'], ['Thumbnails', 'cm_SrcThumbs'], ['Custom columns…', 'cm_SrcCustomViewMenu'],
    ['Tree', 'cm_SrcTree'], ['Quick view', 'cm_SrcQuickView'], '-',
    ['Separate tree', [['Off', 'cm_SeparateTreeOff'], ['One for both panels', 'cm_SeparateTree1'], ['One per panel', 'cm_SeparateTree2']]], '-',
    ['All files', 'cm_SrcAllFiles'], ['Programs', 'cm_SrcExecs'], ['Custom filter', 'cm_SrcUserSpec'], ['Define custom filter…', 'cm_SrcUserDef'], '-',
    ['Sort by name', 'cm_SrcByName'], ['Sort by extension', 'cm_SrcByExt'], ['Sort by date', 'cm_SrcByDateTime'], ['Sort by size', 'cm_SrcBySize'],
    ['Unsorted', 'cm_SrcUnsorted'], ['Reverse order', 'cm_SrcNegOrder'], '-',
    ['Hidden files', 'cm_SwitchHidSys'], ['Reread', 'cm_RereadSource'], '-',
    ['Panels on top of each other', 'cm_HorizontalPanels'], ['Larger text', 'cm_ZoomIn'], ['Smaller text', 'cm_ZoomOut'],
  ]],
  ['Configuration', [
    ['Options…', 'cm_Config'], ['Keyboard shortcuts…', 'cm_ConfigHotkeys'], ['Change button bar…', 'cm_ButtonConfig'], ['Change start menu…', 'cm_ConfigStartMenu'],
    ['Folder hotlist settings…', 'cm_ConfigDirHotlist'], '-',
    ['Save position', 'cm_ConfigSavePos'], ['Save settings', 'cm_ConfigSaveSettings'], ['Edit settings file', 'cm_ConfigChangeIniFiles'],
  ]],
  ['Start', [['Change start menu…', 'cm_ConfigStartMenu']]],
  ['Help', [
    ['Help', 'cm_HelpIndex'], ['Keyboard shortcuts', 'cm_Keyboard'], '-', ['Website', 'cm_VisitHomepage'], ['Check for updates', 'cm_CheckForUpdates'], '-',
    ['About mormiCommander', 'cm_About'],
  ]],
];

const REVEAL = { darwin: 'Show in Finder', windows: 'Show in Explorer', linux: 'Open in the file manager' };

function collectLabels(items) {
  for (const it of items) {
    if (Array.isArray(it) && Array.isArray(it[1])) collectLabels(it[1]);
    else if (Array.isArray(it)) LABELS[it[1]] = it[0] === '__REVEAL__' ? REVEAL[app.platform] || REVEAL.linux : it[0];
  }
}

function toSpec(items) {
  const out = [];
  for (const it of items) {
    if (it === '-') { out.push({ sep: true }); continue; }
    const [label, x] = it;
    if (Array.isArray(x)) { out.push({ label, items: toSpec(x) }); continue; }
    const c = getCommand(x);
    let text = label === '__REVEAL__' ? REVEAL[app.platform] || REVEAL.linux : label;
    // a command with menuKey shows its key in the label ("Hidden files (Ctrl+H)"): native menu items carry no
    // shortcuts of their own (menu.go), so this is the only way the menu tells the key.
    if (c?.menuKey) text = menuLabel(text, app.keyLabel(x));
    const item = { label: text, cmd: x, disabled: !c || (c.enabled ? !c.enabled(app) : false) };
    if (c?.checked) item.checked = !!c.checked(app);
    out.push(item);
  }
  return out;
}

// Items a feature adds to a menu at run time (the user's Start menu entries, a Layout submenu): addMenuItems('Start', fn)
// where fn() returns items in the MENUS format ([label, cmd], '-', [label, [items]]); they go at the end of that menu.
const providers = [];
export function addMenuItems(menu, fn) { providers.push({ menu, fn }); refreshMenu(); }

// A custom main menu (Options > Misc > Custom main menu file, features/config/layout.js) replaces MENUS: the menus of
// the file, in the MENUS format; null for the built-in menu bar. The menus that features add to stay where they are.
let custom = null;
export function setCustomMenus(menus) { custom = menus && menus.length ? menus : null; refreshMenu(); }

// menuSpec is the whole tree, with the commands that put themselves in a menu (command({menu: 'Show'})) added.
export function menuSpec() {
  const base = custom || MENUS;
  const listed = new Set();
  const walk = (items) => items.forEach((it) => Array.isArray(it) && (Array.isArray(it[1]) ? walk(it[1]) : listed.add(it[1])));
  walk(base);
  const menus = base.map(([label, items]) => [label, [...items]]);
  for (const c of commandList()) {
    if (!c.menu || listed.has(c.name)) continue;
    const m = menus.find(([l]) => l === c.menu);
    if (!m) continue;
    const at = c.menuAfter ? m[1].findIndex((it) => Array.isArray(it) && it[1] === c.menuAfter) : -1;
    if (at >= 0) m[1].splice(at + 1, 0, [c.label, c.name]);
    else m[1].push([c.label, c.name]);
  }
  for (const p of providers) {
    const m = menus.find(([l]) => l === p.menu);
    let items = [];
    try { items = p.fn() || []; } catch (e) { console.error(e); }
    if (m && items.length) m[1].push(...items);
  }
  return menus.map(([label, items]) => ({ label, items: toSpec(items) }));
}

let timer = 0;
let last = '';

// refreshMenu sends the menu to Go when it changed (commands added, check marks flipped). Cheap to call often.
export function refreshMenu() {
  clearTimeout(timer);
  timer = setTimeout(() => {
    const spec = menuSpec();
    const s = JSON.stringify(spec);
    if (s === last) return;
    last = s;
    go.App.SetMenu(spec).catch(() => {});
  }, 30);
}

export function initMenus() {
  collectLabels(MENUS);
  on('menu', (cmd) => app.run(cmd));
  onChange(refreshMenu);
  for (const ev of ['panel', 'active', 'settings']) app.on(ev, refreshMenu);
  refreshMenu();
}
