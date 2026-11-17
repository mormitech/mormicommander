// The function key bar at the bottom (F-023): F3 View … F8 Delete, Alt+F4 Exit. Each button runs whatever command its
// key runs (so a changed key changes the button too), and while Shift, Alt or Ctrl is held the buttons show the shifted
// commands, as Total Commander does. A button whose command no feature implements yet is dimmed; clicking it says so.

import { tr } from './i18n.js';
import { commandForKey, getCommand, onChange } from './registry.js';
import { onModifiers } from './keyboard.js';
import { app, LABELS } from './app.js';
import { esc } from './dialogs.js';

const KEYS = ['F3', 'F4', 'F5', 'F6', 'F7', 'F8'];

// Short texts for the bar; a command's own `short` wins.
export const SHORT = {
  cm_List: 'View', cm_Edit: 'Edit', cm_Copy: 'Copy', cm_RenMov: 'Move', cm_MkDir: 'New folder', cm_Delete: 'Delete', cm_Exit: 'Exit',
  cm_ListCurrent: 'View this', cm_EditNewFile: 'New file', cm_CopySamepanel: 'Copy here', cm_RenameOnly: 'Rename', cm_DeletePermanent: 'Delete now',
  cm_ListExternal: 'Other viewer', cm_PackFiles: 'Pack', cm_UnpackFiles: 'Unpack', cm_SearchFor: 'Find', cm_ShowCmdLineHistory: 'History',
  cm_SrcByName: 'By name', cm_SrcByExt: 'By ext', cm_SrcByDateTime: 'By date', cm_SrcBySize: 'By size', cm_SrcUnsorted: 'Unsorted', cm_SrcTree: 'Tree',
  cm_CompareDirs: 'Compare', cm_HelpIndex: 'Help', cm_RereadSource: 'Reread', cm_CreateShortcut: 'Link', cm_SeparateTreeCycle: 'Tree',
  cm_ListInternalOnly: 'Built-in viewer', cm_PackFilesMove: 'Move to archive', cm_Minimize: 'Minimize',
};

let held = new Set();

function prefix() {
  return ['Ctrl', 'Alt', 'Shift', 'Cmd'].filter((m) => held.has(m)).join('+');
}

export function renderFkeys() {
  const bar = document.getElementById('fkeys');
  if (!bar) return;
  const pre = prefix();
  const keys = KEYS.map((k) => (pre ? `${pre}+${k}` : k));
  if (!pre) keys.push('Alt+F4');
  bar.innerHTML = keys.map((combo) => {
    const name = commandForKey(combo);
    const c = name && getCommand(name);
    const text = tr(c?.short || SHORT[name] || (c?.label || LABELS[name] || '').replace(/…$/, '') || '');
    const keyText = app.platform === 'darwin' ? combo.replace('Alt+', '⌥').replace('Ctrl+', '⌃').replace('Shift+', '⇧').replace('Cmd+', '⌘') : combo;
    return `<button type="button" data-cmd="${esc(name || '')}" data-key="${esc(combo)}" class="${c ? '' : 'missing'}" ${name ? '' : 'disabled'} title="${esc(text)} (${esc(combo)})"><kbd>${esc(keyText)}</kbd>${esc(text)}</button>`;
  }).join('');
}

export function initFkeys() {
  const bar = document.getElementById('fkeys');
  bar.addEventListener('mousedown', (ev) => ev.preventDefault()); // the panel keeps the focus
  bar.addEventListener('click', (ev) => {
    const b = ev.target.closest('button[data-cmd]');
    if (b?.dataset.cmd) app.run(b.dataset.cmd, { button: b.dataset.key });
  });
  onModifiers((m) => { held = m; renderFkeys(); });
  onChange(renderFkeys);
  renderFkeys();
}
