// What the options change on screen, at start and the moment OK or Apply is pressed (F-413, F-414, F-416, F-417,
// F-431, F-435): which bars are visible, panels side by side or on top of each other, fonts, the theme and the colours
// (also by file type), the window title, the exit question. The cm_Vis* commands of Total Commander toggle the bars
// from the keyboard, the command line and the Configuration > Layout menu.

import { command, getCommand } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { messageDialog } from '../../core/dialogs.js';
import { addMenuItems } from '../../core/menus.js';
import { setDisplay } from '../../core/format.js';
import { state, opt, on, num, applyOptions, compileMask, isDark } from './util.js';
import { applyIgnoreList } from './listopts.js';
import { applyIcons, rowHeight } from './fileicons.js';

// Layout option → body class that hides an element (see config.css).
const HIDE = {
  'Layout.CmdLine': 'mc-config-no-cmdline', 'Layout.KeyButtons': 'mc-config-no-fkeys', 'Layout.StatusBar': 'mc-config-no-status',
  'Layout.DriveBar1': 'mc-config-no-drives', 'Layout.CurDir': 'mc-config-no-pathbar', 'Layout.TabHeader': 'mc-config-no-header',
  'Layout.HistoryHotlist': 'mc-config-no-pbtns', 'Layout.FreeSpace': 'mc-config-no-driveinfo',
  'Layout.HiddenMarker': 'mc-config-no-hiddenmark', 'Layout.DriveCombo': 'mc-config-no-drivecombo',
  'Layout.DriveBar2': 'mc-config-one-drivebar', 'Layout.BreadCrumbBar': 'mc-config-no-crumbs',
  'Display.FullRowSelect': 'mc-config-name-cursor',
};

let app = null;
let colors = null; // {light, dark}: the schemes from Go
const rowObservers = [];

function relayout() {
  for (const p of app?.panels || []) { try { p.measure(); p.render(); p.ensureVisible(); } catch { /* not ready yet */ } }
}

export function applyLayout() {
  const b = document.body.classList;
  for (const [name, cls] of Object.entries(HIDE)) b.toggle(cls, !on(name));
  b.toggle('mc-config-flat', on('Layout.InterfaceFlat'));
  go.App.SetMenuBar(on('Layout.MenuBar')).catch(() => {}); // (Windows and Linux; the macOS menu bar always stays)
  syncSeparateTree();
  b.toggle('mc-config-flat-drives', on('Layout.DriveBarFlat'));
  const tb = document.getElementById('toolbar'), vb = document.getElementById('vbar');
  if (tb) tb.hidden = !on('Layout.ButtonBar') || !tb.childElementCount;
  if (vb) vb.hidden = !on('Layout.ButtonBarVertical') || !vb.childElementCount;
  const horiz = on('Layout.Horizontal');
  const panels = document.getElementById('panels');
  if (panels && panels.classList.contains('horizontal') !== horiz) {
    panels.classList.toggle('horizontal', horiz);
    app.setSetting('horizontal', horiz);
  }
  relayout();
}

// Layout > Separate folder tree: 0 off, 1 one tree for both panels, 2 one per panel, as the cm_SeparateTree* commands
// do (features/reserved-more.js). Only a change of the option runs the command, so a tree opened by hand stays.
let treeApplied = 0;
function syncSeparateTree() {
  const want = Math.max(0, Math.min(2, num('Layout.SeparateTree', 0)));
  if (want === treeApplied || !app?.panels?.every((p) => p.path)) return;
  treeApplied = want;
  app.run(['cm_SeparateTreeOff', 'cm_SeparateTree1', 'cm_SeparateTree2'][want], {}, { quiet: true });
}

export function applyFonts() {
  const root = document.documentElement.style;
  const px = Math.max(8, Math.min(40, num('Font.PanelSize', 13)));
  const family = opt('Font.PanelFont').trim();
  const fam = family ? `"${family.replace(/"/g, '')}", ` : '';
  root.setProperty('--font', `${on('Font.PanelBold') ? '600 ' : ''}${px}px ${fam}-apple-system, "Segoe UI", system-ui, sans-serif`);
  root.setProperty('--mono', `${px - 1}px ui-monospace, "SF Mono", Menlo, Consolas, monospace`);
  root.setProperty('--row-h', `${rowHeight(Math.round(px * 1.55))}px`); // (taller for large icons: Options > Icons)
  const dlg = opt('Font.DialogFont').trim();
  root.setProperty('--mc-config-dialog-font', `${num('Font.DialogSize', 13)}px ${dlg ? `"${dlg.replace(/"/g, '')}", ` : ''}-apple-system, "Segoe UI", system-ui, sans-serif`);
  if (app && app.setting('fontSize', 13) !== px) app.setSetting('fontSize', px); // window.js reads it at start (Ctrl+= / Ctrl+-)
  relayout();
}

export function applyTheme(startTheme) {
  if (startTheme === 'light' || startTheme === 'dark') return; // --theme wins (screenshots)
  const m = num('Colors.DarkMode', 2);
  const t = m === 0 ? 'light' : m === 1 ? 'dark' : '';
  if (t) document.documentElement.dataset.theme = t; else delete document.documentElement.dataset.theme;
}

// ---- colours ----
let filters = []; // [{test, color}] for colouring by file type
let alternate = false; // every second line on another background

function schemeCSS() {
  const s = colors ? (isDark() ? colors.dark : colors.light) : null;
  const c = s?.colors || {};
  const vars = [];
  const rules = [];
  if (c.fore) vars.push(`--text:${c.fore}`);
  if (c.back) vars.push(`--bg:${c.back}`);
  if (c.mark) vars.push(`--sel:${c.mark}`);
  if (c.cursor) vars.push(`--cur:${c.cursor}`);
  if (c.inactiveCursor) vars.push(`--cur-off:${c.inactiveCursor}`);
  if (c.back2) vars.push(`--row-alt:${c.back2}`);
  else if (on('Colors.AlternateLines')) vars.push('--row-alt:color-mix(in srgb, var(--text) 5%, transparent)');
  if (c.cursorText) rules.push(`.panel.active .row.cur, .panel.active .row.cur * { color: ${c.cursorText} !important; }`);
  if (c.inactiveMark) rules.push(`.panel:not(.active) .row.sel, .panel:not(.active) .row.sel * { color: ${c.inactiveMark} !important; }`);
  if (c.inactiveBack) rules.push(`.panel:not(.active) .list { background: ${c.inactiveBack}; }`);
  alternate = on('Colors.AlternateLines') || !!c.back2;
  if (alternate) rules.push('.rows .row.mc-config-alt { background-color: var(--row-alt); }');
  if (s?.inverseCursor) rules.push('.panel.active .row.cur { background: var(--text); color: var(--bg); }');
  if (on('Colors.InactiveDimmed')) rules.push('.panel:not(.active) .rows { opacity: .82; }');
  if (on('Colors.CursorFrameOnly')) rules.push('.panel.active .row.cur { background: transparent; box-shadow: inset 0 0 0 1px var(--accent); }');
  if (on('Colors.MarkBold')) rules.push('.row.sel, .row.sel * { font-weight: 700; }');
  const mb = opt('Colors.SelectionBgColor').trim();
  if (mb) rules.push(`.row.sel { background-color: ${/^[#\w(),.\s%-]+$/.test(mb) ? mb : 'transparent'}; }`);
  if (on('Colors.GradientCursor')) rules.push('.panel.active .row.cur { background: linear-gradient(90deg, var(--cur), transparent); }');
  filters = on('Colors.ColorByType') ? (s?.filters || []).filter((f) => f.mask && f.color).map((f) => ({ test: compileMask(f.mask), color: f.color })) : [];
  return (vars.length ? `:root, :root[data-theme] { ${vars.join('; ')}; }\n` : '') + rules.join('\n');
}

export async function loadColors() {
  try { colors = await go.Configuration.Colors(); } catch { colors = null; }
}

export function applyColors() {
  let st = document.getElementById('mc-config-colors');
  if (!st) { st = document.createElement('style'); st.id = 'mc-config-colors'; document.head.append(st); }
  st.textContent = schemeCSS();
  for (const p of app?.panels || []) paintRows(p);
}

function paintRows(p) {
  if (!filters.length && !alternate && !p.mcConfigPainted) return;
  p.mcConfigPainted = filters.length > 0 || alternate;
  for (const row of p.$.rows.children) {
    const i = +row.dataset.i;
    const e = p.entries[i];
    row.classList.toggle('mc-config-alt', alternate && i % 2 === 1);
    const name = row.querySelector('.n b') || row;
    let color = '';
    if (e && !e.up && filters.length) for (const f of filters) if (f.test(e.name, !!e.dir)) { color = f.color; break; }
    if (color) name.style.color = color; else if (name.style.color) name.style.color = '';
  }
}

// ---- window title, exit question ----
function title() {
  const t = opt('Misc.WindowTitle', 'mormiCommander').trim();
  if (!t || t === 'mormiCommander' || !app?.cur?.path) return;
  go.App.SetTitle(`${t} – ${app.cur.path}`).catch(() => {});
}

function wrapExit() {
  const c = getCommand('cm_Exit');
  if (!c || c.mcConfigWrapped) return;
  const run = c.run;
  c.mcConfigWrapped = true;
  c.run = async (a, args) => {
    if (on('Misc.ConfirmExit') && !args?.force) {
      const b = await messageDialog({ Title: 'Quit mormiCommander?', Message: 'Do you want to quit mormiCommander?', Buttons: ['Quit', 'Cancel'], Kind: 'confirm-exit' });
      if (b !== 'Quit') return false;
    }
    return run(a, args);
  };
}

// ---- the Display page: how the file lists look (F-414) ----

// applyDisplay hands the size, date and name options to the panel rows (core/format.js) and draws the panels again.
export function applyDisplay() {
  setDisplay({
    brackets: on('Display.DirBrackets'), separateExt: on('Display.SeparateExt'), sizeStyle: num('Display.SizeStyle', 0),
    thousands: on('Display.ThousandsSeparator'), dateFormat: opt('Display.DateFormat').trim(), time24: on('Display.Time24'),
    seconds: on('Display.ShowSeconds'), footerStyle: num('Display.SizeFooter', 4), longNames: on('Display.LongNames'),
  });
  document.body.classList.toggle('mc-config-no-ext', !on('Display.SeparateExt'));
  document.body.classList.toggle('mc-hover-rows', on('Display.HoverHighlight'));
  // system files and ".." in root folders change what the list holds, so the panels are built again
  for (const p of app?.panels || []) { try { if (p.path) p.rebuild(); } catch { /* not ready yet */ } }
  relayout();
}

// syncHidden makes the panels show hidden files as the settings file says (at start, after a reload or an import).
function syncHidden() {
  if (!app || !state.loaded) return;
  const want = on('Display.ShowHidden');
  if (want === !!app.showHidden) return;
  app.showHidden = want;
  for (const p of app.panels || []) { try { p.rebuild(); app.panelChanged(p, { moved: false }); } catch { /* not loaded yet */ } }
}

// Ctrl+H (cm_SwitchHidSys) also writes its new state to the settings file, so the file and the panels agree.
function wrapHidden() {
  const c = getCommand('cm_SwitchHidSys');
  if (!c || c.mcConfigWrapped) return;
  const run = c.run;
  c.mcConfigWrapped = true;
  c.run = async (a, args) => {
    const r = await run(a, args);
    const v = a.showHidden ? '1' : '0';
    if (opt('Display.ShowHidden') !== v) await applyOptions({ 'Display.ShowHidden': v }).catch((e) => console.error(e));
    return r;
  };
}

// applyAll puts every option on screen (at start, after a reload of the settings file).
export function applyAll(startTheme) {
  applyIcons();
  applyLayout();
  applyFonts();
  applyTheme(startTheme);
  applyColors();
  applyDisplay();
  syncHidden();
  title();
  applyIgnoreList();
}

// ---- cm_Vis* toggles ----
const VIS = [
  ['cm_VisButtonbar', 'Layout.ButtonBar', 'Button bar'], ['cm_VisButtonbar2', 'Layout.ButtonBarVertical', 'Vertical button bar'],
  ['cm_VisDriveButtons', 'Layout.DriveBar1', 'Drive buttons'], ['cm_VisCurDir', 'Layout.CurDir', 'Current folder line'],
  ['cm_VisTabHeader', 'Layout.TabHeader', 'Sort header'], ['cm_VisStatusbar', 'Layout.StatusBar', 'Status bar'],
  ['cm_VisCmdLine', 'Layout.CmdLine', 'Command line'], ['cm_VisKeyButtons', 'Layout.KeyButtons', 'Function key buttons'],
  ['cm_VisHistHotButtons', 'Layout.HistoryHotlist', 'History and hotlist buttons'], ['cm_VisDriveCombo', 'Layout.FreeSpace', 'Free space line'],
  ['cm_VisFlatInterface', 'Layout.InterfaceFlat', 'Flat interface'], ['cm_VisFlatDriveButtons', 'Layout.DriveBarFlat', 'Flat drive buttons'],
  ['cm_VisTwoDriveButtons', 'Layout.DriveBar2', 'Two drive button bars'], ['cm_VisBreadCrumbs', 'Layout.BreadCrumbBar', 'Breadcrumb path bar'],
];
for (const [name, field, label] of VIS) {
  command({
    name, label: `Show ${label.toLowerCase()}`, checked: () => on(field),
    run: async () => { await applyOptions({ [field]: on(field) ? '0' : '1' }); applyLayout(); },
  });
}
addMenuItems('Configuration', () => ['-', ['Layout', VIS.map(([name, , label]) => [label, name])]]);

export function initLayout(a) {
  app = a;
  wrapExit();
  wrapHidden();
  const mo = (p) => {
    const o = new MutationObserver(() => paintRows(p));
    o.observe(p.$.rows, { childList: true });
    rowObservers.push(o);
  };
  for (const p of app.panels) mo(p);
  app.on('panel', () => { for (const p of app.panels) paintRows(p); title(); });
  app.on('ready', syncSeparateTree);
  app.on('active', title);
  window.matchMedia?.('(prefers-color-scheme: light)').addEventListener?.('change', applyColors);
  return state;
}
