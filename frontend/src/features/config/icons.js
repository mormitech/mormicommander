// Icons of the button bar: the default bar's buttons name built-in icons ("mc:find"); buttons for cm_* commands get the
// icon of their command; other buttons get a generic icon (program, sub-bar, user command). Line icons on a 24 grid,
// drawn with currentColor so both themes work.

const P = {
  reread: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
  brief: '<path d="M4 6h6M4 12h6M4 18h6M14 6h6M14 12h6M14 18h6"/>',
  full: '<path d="M4 6h16M4 12h16M4 18h16"/><path d="M14 4v16"/>',
  thumbnails: '<rect x="4" y="4" width="7" height="7" rx="1"/><rect x="13" y="4" width="7" height="7" rx="1"/><rect x="4" y="13" width="7" height="7" rx="1"/><rect x="13" y="13" width="7" height="7" rx="1"/>',
  tree: '<path d="M5 4v14h5M5 9h5"/><rect x="10" y="6" width="9" height="5" rx="1"/><rect x="10" y="15" width="9" height="5" rx="1"/>',
  invert: '<rect x="4" y="4" width="16" height="16" rx="2"/><path d="M4 20 20 4M12 4v16" opacity=".5"/>',
  back: '<path d="M15 5l-7 7 7 7"/>',
  forward: '<path d="M9 5l7 7-7 7"/>',
  pack: '<path d="M4 8l8-4 8 4v8l-8 4-8-4z"/><path d="M4 8l8 4 8-4M12 12v8"/>',
  unpack: '<path d="M4 10v6l8 4 8-4v-6"/><path d="M12 3v9M8 8l4 4 4-4"/>',
  find: '<circle cx="10.5" cy="10.5" r="6"/><path d="M15 15l5 5"/>',
  view: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>',
  rename: '<path d="M4 7h10M4 12h7M4 17h10"/><path d="M15 15l4-4 2 2-4 4h-2z"/>',
  sync: '<path d="M4 9h13l-3-3M20 15H7l3 3"/>',
  ftp: '<circle cx="12" cy="12" r="8"/><path d="M4 12h16M12 4c2.5 2.5 2.5 13.5 0 16M12 4c-2.5 2.5-2.5 13.5 0 16"/>',
  hotlist: '<path d="M12 4l2.4 5 5.6.6-4.2 3.8 1.2 5.6L12 16.2 7 19l1.2-5.6L4 9.6 9.6 9z"/>',
  edit: '<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M13 7l4 4"/>',
  compare: '<rect x="3" y="4" width="7" height="16" rx="1"/><rect x="14" y="4" width="7" height="16" rx="1"/><path d="M6 9h1M6 13h1M17 9h1M17 15h1"/>',
  copy: '<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"/>',
  move: '<path d="M4 12h14M14 7l5 5-5 5"/>',
  delete: '<path d="M5 7h14M10 7V4h4v3M7 7l1 13h8l1-13"/>',
  mkdir: '<path d="M3 7a1 1 0 0 1 1-1h5l2 2h9a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"/><path d="M12 11v5M9.5 13.5h5"/>',
  terminal: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 10l3 2-3 2M12 15h5"/>',
  hidden: '<path d="M3 3l18 18"/><path d="M10.6 6.2A9.8 9.8 0 0 1 12 6c6.4 0 10 6 10 6a17 17 0 0 1-3 3.6M6.5 7.6A17 17 0 0 0 2 12s3.6 6 10 6a9 9 0 0 0 4-.9"/>',
  config: '<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1 7 17M17 7l2.1-2.1"/>',
  home: '<path d="M4 11l8-7 8 7v9h-5v-6H9v6H4z"/>',
  up: '<path d="M12 19V5M6 11l6-6 6 6"/>',
  cmd: '<path d="M5 7l5 5-5 5M12 17h7"/>',
  user: '<circle cx="12" cy="8" r="4"/><path d="M4 21c1-4 4.5-6 8-6s7 2 8 6"/>',
  program: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 8h18"/><circle cx="6" cy="6" r=".5"/><circle cx="8.5" cy="6" r=".5"/>',
  bar: '<rect x="3" y="5" width="18" height="6" rx="1"/><path d="M8 15l4 4 4-4"/>',
  file: '<path d="M6 3h8l5 5v13H6z"/><path d="M14 3v5h5"/>',
};

// Commands with an icon of their own when a button names them without one.
const BY_COMMAND = {
  cm_RereadSource: 'reread', cm_SrcShort: 'brief', cm_SrcLong: 'full', cm_SrcThumbs: 'thumbnails', cm_SrcTree: 'tree', cm_ExchangeSelection: 'invert',
  cm_GoToPreviousDir: 'back', cm_GoToNextDir: 'forward', cm_GoToPrev: 'back', cm_GoToNext: 'forward', cm_PackFiles: 'pack', cm_UnpackFiles: 'unpack',
  cm_SearchFor: 'find', cm_List: 'view', cm_MultiRenameFiles: 'rename', cm_MultiRename: 'rename', cm_SyncDirs: 'sync', cm_FtpConnect: 'ftp',
  cm_DirectoryHotlist: 'hotlist', cm_Edit: 'edit', cm_CompareFilesByContent: 'compare', cm_Copy: 'copy', cm_RenMov: 'move', cm_Delete: 'delete',
  cm_MkDir: 'mkdir', cm_ExecuteDOS: 'terminal', cm_SwitchHidSys: 'hidden', cm_Config: 'config', cm_GoToHome: 'home', cm_GoToParent: 'up',
};

export const ICON_NAMES = Object.keys(P);

export function svg(name) {
  return `<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">${P[name] || P.program}</svg>`;
}

// iconName is the built-in icon a button shows ('' when it should use its own image file).
export function iconName(btn) {
  const icon = String(btn.icon || '');
  if (icon.startsWith('mc:')) return P[icon.slice(3)] ? icon.slice(3) : 'cmd';
  const cmd = String(btn.cmd || '').trim();
  if (btn.subBar) return 'bar';
  if (BY_COMMAND[cmd]) return BY_COMMAND[cmd];
  if (/^cm_/i.test(cmd)) return 'cmd';
  if (/^em_/i.test(cmd)) return 'user';
  if (/^cd\s/i.test(cmd)) return 'up';
  return /\.(app|exe|com|bat|cmd|sh)"?$/i.test(cmd) || !/\.\w+"?$/.test(cmd) ? 'program' : 'file';
}
