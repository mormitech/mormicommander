// The button bar under the menu and the vertical bar between the panels (FEATURES.md section 11). The buttons come from
// .bar files (internal/config; a fresh install shows the default bar, F-304) and run like Total Commander's: a cm_ or
// em_ command, a program with parameters (%P %N %S %T %L ... F-310, "?" to edit them first, F-814), "cd folder", or
// another .bar file that drops down as a sub-bar (F-308). Right-click a button to change, delete, cut, copy or paste it
// (F-313, TC's clipboard format); drop a file on the bar to add a button for it (F-306) or on a button to give it the
// files (F-307). Configuration > Change button bar (cm_ButtonConfig, F-305) edits both bars, switches .bar files
// (F-309), and sets the icon size, the flat look and wrapping (F-312). Alt+Shift+F11 (cm_FocusButtonBar) puts the
// keyboard on the bar: arrows move, Enter runs, Esc goes back to the panel.

import { command, getCommand } from '../../core/registry.js';
import { addKeyHandler } from '../../core/keyboard.js';
import { modal, messageDialog, inputDialog, esc } from '../../core/dialogs.js';
import { showList } from '../../core/popup.js';
import { addControlCommand } from '../../core/control.js';
import { track } from '../../core/busy.js';
import { go, errText, onFileDrop } from '../../core/go.js';
import { app as theApp } from '../../core/app.js';
import { on, fillSelect, applyOptions } from './util.js';
import { svg, iconName } from './icons.js';
import { runButton, realCommand } from './run.js';
import { applyLayout } from './layout.js';
import { allCommandNames, commandLabel } from './options.js';

const bars = { main: { path: '', buttons: [] }, vertical: { path: '', buttons: [] } };
let look = { iconSize: 16, flat: true, wrap: false };
const iconCache = new Map(); // icon file → data: URL or ''
const ELS = { main: 'toolbar', vertical: 'vbar' };
let clip = null; // the last button cut or copied, for Paste when the system clipboard can't be read

export async function loadBars() {
  for (const which of ['main', 'vertical']) {
    try { bars[which] = await go.ConfigBar.Bar(which); } catch (e) { bars[which] = { path: '', buttons: [] }; console.error(e); }
  }
  try { look = { ...look, ...(await go.ConfigBar.Options()) }; } catch { /* defaults */ }
  renderBars();
}

const tipOf = (b) => {
  if (b.tooltip) return b.tooltip;
  const c = realCommand(b.cmd || '');
  if (/^(cm|em)_/.test(c)) return commandLabel(c);
  return b.cmd || '';
};

export function renderBars() {
  for (const which of ['main', 'vertical']) {
    const el = document.getElementById(ELS[which]);
    if (!el) continue;
    el.classList.add('mc-config-bar');
    el.classList.toggle('mc-config-bar-flat', !!look.flat);
    el.classList.toggle('mc-config-bar-wrap', !!look.wrap && which === 'main');
    el.style.setProperty('--mc-config-icon', `${look.iconSize || 16}px`);
    el.innerHTML = (bars[which].buttons || []).map((b, i) => {
      if (b.sep) return '<span class="mc-config-sep" aria-hidden="true"></span>';
      const name = iconName(b);
      const img = iconCache.get(b.icon);
      const c = realCommand((b.cmd || '').trim());
      const missing = /^cm_/.test(c) && !getCommand(c);
      const key = /^(cm|em)_/.test(c) ? theApp.keyLabel(c) : '';
      const tip = tipOf(b) + (key ? ` (${key})` : '');
      return `<button type="button" class="mc-config-btn${missing ? ' missing' : ''}" data-i="${i}" data-tip="${esc(tip)}" aria-label="${esc(tipOf(b))}" tabindex="-1">${img ? `<img src="${esc(img)}" alt="">` : svg(name)}</button>`;
    }).join('');
    // image files as icons: loaded once, then drawn again
    for (const b of bars[which].buttons || []) {
      if (b.sep || !b.icon || b.icon.startsWith('mc:') || iconCache.has(b.icon)) continue;
      iconCache.set(b.icon, '');
      go.ConfigBar.IconData(b.icon).then((d) => { if (d) { iconCache.set(b.icon, d); renderBars(); } }).catch(() => {});
    }
  }
  applyLayout();
}

async function saveBar(which) {
  const path = await go.ConfigBar.SaveBar(which, bars[which].path, bars[which].buttons);
  bars[which].path = path;
  renderBars();
}

function barOf(el) {
  const bar = el.closest('#toolbar, #vbar');
  return bar?.id === 'vbar' ? 'vertical' : 'main';
}

// ---- dropped files (F-306, F-307) ----
function droppedPaths(dt) {
  const out = [];
  const uris = dt.getData('text/uri-list');
  if (uris) {
    for (const line of uris.split(/\r?\n/)) {
      const l = line.trim();
      if (!l || l.startsWith('#') || !l.startsWith('file://')) continue;
      try {
        let p = decodeURIComponent(new URL(l).pathname);
        if (/^\/[a-z]:\//i.test(p)) p = p.slice(1).replace(/\//g, '\\');
        out.push(p);
      } catch { /* not a file URL */ }
    }
  }
  if (!out.length) {
    const t = dt.getData('text/plain') || '';
    for (const line of t.split(/\r?\n/)) { const l = line.trim(); if (/^(\/|[a-z]:\\|\\\\)/i.test(l)) out.push(l); }
  }
  return out;
}

async function dropOn(which, index, paths) {
  if (!paths.length) return;
  if (index >= 0) {
    const b = bars[which].buttons[index];
    if (b && !b.sep) await runButton(theApp, b, { dropped: paths });
    return;
  }
  for (const p of paths) bars[which].buttons.push(await go.ConfigBar.ButtonForFile(p));
  await saveBar(which);
  theApp.flash(paths.length === 1 ? 'A button for the file was added.' : `${paths.length} buttons were added.`);
}

// ---- right-click menu (F-313) ----
async function contextMenu(which, i, at) {
  const b = bars[which].buttons[i];
  const items = [
    { label: 'Change…', value: 'change', disabled: !b }, { label: 'Delete', value: 'delete', disabled: !b }, { sep: true },
    { label: 'Cut', value: 'cut', disabled: !b || b.sep }, { label: 'Copy', value: 'copy', disabled: !b || b.sep }, { label: 'Paste', value: 'paste' }, { sep: true },
    { label: 'Change button bar…', value: 'config' },
  ];
  const r = await showList({ title: b ? tipOf(b) || 'Separator' : 'Button bar', items, anchor: at, kind: 'button-menu' });
  if (!r) return;
  try {
    switch (r.item.value) {
      case 'change': return openButtonConfig(theApp, { which, index: i });
      case 'delete': {
        const ok = await messageDialog({ Title: 'Delete button', Message: `Delete the button "${tipOf(b) || 'separator'}"?`, Buttons: ['Delete', 'Cancel'], Kind: 'button-delete' });
        if (ok !== 'Delete') return;
        bars[which].buttons.splice(i, 1);
        return saveBar(which);
      }
      case 'cut': case 'copy':
        clip = { ...b };
        await go.ConfigBar.CopyButton(b).catch(() => {});
        if (r.item.value === 'cut') { bars[which].buttons.splice(i, 1); await saveBar(which); }
        return;
      case 'paste': {
        let nb = await go.ConfigBar.PasteButton().catch(() => null);
        if (!nb) nb = clip;
        if (!nb) { theApp.flash('The clipboard does not hold a button.'); return; }
        bars[which].buttons.splice(i >= 0 ? i + 1 : bars[which].buttons.length, 0, { ...nb });
        return saveBar(which);
      }
      case 'config': return openButtonConfig(theApp, { which, index: i });
    }
  } catch (e) { theApp.flash(errText(e), 'err'); }
}

function wire(el) {
  el.addEventListener('mousedown', (ev) => { if (ev.button === 0) ev.preventDefault(); }); // the panel keeps the keyboard
  el.addEventListener('click', (ev) => {
    const btn = ev.target.closest('.mc-config-btn');
    if (!btn) return;
    const which = barOf(btn);
    const b = bars[which].buttons[+btn.dataset.i];
    if (b) runButton(theApp, b, { anchor: btn });
  });
  el.addEventListener('contextmenu', (ev) => {
    ev.preventDefault();
    const btn = ev.target.closest('.mc-config-btn, .mc-config-sep');
    const which = barOf(el);
    const i = btn?.classList.contains('mc-config-btn') ? +btn.dataset.i : btn ? [...el.children].indexOf(btn) : -1;
    contextMenu(which, i, { x: ev.clientX, y: ev.clientY });
  });
  el.addEventListener('dragover', (ev) => { ev.preventDefault(); ev.dataTransfer.dropEffect = 'copy'; el.classList.add('mc-config-drop'); });
  el.addEventListener('dragleave', () => el.classList.remove('mc-config-drop'));
  el.addEventListener('drop', (ev) => {
    ev.preventDefault();
    el.classList.remove('mc-config-drop');
    const btn = ev.target.closest('.mc-config-btn');
    track(dropOn(barOf(el), btn ? +btn.dataset.i : -1, droppedPaths(ev.dataTransfer)).catch((e) => theApp.flash(errText(e), 'err')));
  });
}

// ---- the keyboard on the bar (cm_FocusButtonBar) ----
function barButtons() {
  return [...document.querySelectorAll('#toolbar:not([hidden]) .mc-config-btn, #vbar:not([hidden]) .mc-config-btn')];
}
command({
  name: 'cm_FocusButtonBar', label: 'Go to the button bar',
  run: (app) => {
    const list = barButtons();
    if (!list.length) { app.flash('The button bar is hidden or empty.'); return false; }
    list.forEach((b) => { b.tabIndex = -1; });
    list[0].tabIndex = 0;
    list[0].focus();
    return true;
  },
});
addKeyHandler((ev, combo) => {
  const ae = document.activeElement;
  if (!ae?.classList?.contains('mc-config-btn')) return false;
  const list = barButtons();
  const i = list.indexOf(ae);
  const go2 = (j) => { const b = list[(j + list.length) % list.length]; b?.focus(); return true; };
  switch (combo) {
    case 'Left': case 'Up': return go2(i - 1);
    case 'Right': case 'Down': return go2(i + 1);
    case 'Home': return go2(0);
    case 'End': return go2(list.length - 1);
    case 'Enter': case 'Space': ae.click(); return true;
    case 'Esc': case 'Tab': theApp.cur.$.list.focus(); return true;
    case 'Apps': case 'Shift+F10': {
      const r = ae.getBoundingClientRect();
      contextMenu(barOf(ae), +ae.dataset.i, { x: r.left, y: r.bottom });
      return true;
    }
  }
  return false;
});

// ---- Change button bar (cm_ButtonConfig, F-305) ----
const ICONIC = [{ value: '0', label: 'Normal window' }, { value: '1', label: 'Minimized' }, { value: '-1', label: 'Maximized' }];

export function openButtonConfig(app, { which = 'main', index = 0 } = {}) {
  const pending = { main: JSON.parse(JSON.stringify(bars.main)), vertical: JSON.parse(JSON.stringify(bars.vertical)) };
  const dirty = { main: false, vertical: false };
  return modal((box, done) => {
    box.classList.add('mc-config-box', 'mc-config-barbox');
    box.innerHTML = `<h2>Change button bar</h2>
      <datalist id="mc-config-bar-cmds">${allCommandNames(app.platform).map((n) => `<option value="${esc(n)}">${esc(commandLabel(n))}</option>`).join('')}</datalist>
      <div class="mc-config-inline">
        <div class="mc-row"><label for="bc-which">Bar</label><select id="bc-which"><option value="main">Button bar</option><option value="vertical">Vertical button bar</option></select></div>
        <div class="mc-row mc-config-grow"><label for="bc-file">Bar file</label><input type="text" id="bc-file" spellcheck="false" list="mc-config-barfiles"><datalist id="mc-config-barfiles"></datalist></div>
        <button type="button" class="bc-load">Open bar file…</button>
      </div>
      <div class="mc-config-listedit"><select id="bc-list" size="13" aria-label="Buttons"></select>
        <div class="mc-config-listfields">
          <div class="mc-row"><label for="bc-cmd">Command (cm_ command, em_ command, program, cd folder, or a .bar file)</label><input type="text" id="bc-cmd" spellcheck="false" list="mc-config-bar-cmds"></div>
          <div class="mc-row"><label for="bc-params">Parameters (%P path, %N name, %S selected, %T target path, %L list file; ? asks first)</label><input type="text" id="bc-params" spellcheck="false"></div>
          <div class="mc-row"><label for="bc-path">Start folder</label><input type="text" id="bc-path" spellcheck="false"></div>
          <div class="mc-row"><label for="bc-icon">Icon (an image file, or empty for the command's icon)</label><input type="text" id="bc-icon" spellcheck="false"></div>
          <div class="mc-row"><label for="bc-tooltip">Tooltip</label><input type="text" id="bc-tooltip"></div>
          <div class="mc-row"><label for="bc-iconic">Run</label><select id="bc-iconic">${ICONIC.map((o) => `<option value="${o.value}">${o.label}</option>`).join('')}</select></div>
        </div>
      </div>
      <div class="mc-config-rowbtns"><button type="button" class="bc-add">Add button</button><button type="button" class="bc-sep">Add separator</button><button type="button" class="bc-del">Delete</button>
        <button type="button" class="bc-up">Move up</button><button type="button" class="bc-down">Move down</button><button type="button" class="bc-copy">Copy</button><button type="button" class="bc-paste">Paste</button></div>
      <div class="mc-config-inline">
        <div class="mc-row"><label for="bc-size">Icon size</label><select id="bc-size"><option value="16">Small</option><option value="24">Medium</option><option value="32">Large</option></select></div>
        <label class="mc-check"><input type="checkbox" id="bc-flat"> Flat buttons</label><label class="mc-check"><input type="checkbox" id="bc-wrap"> Wrap onto more lines</label>
        <label class="mc-check"><input type="checkbox" id="bc-show"> Show this bar</label>
      </div>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    const $ = (s) => box.querySelector(s);
    const err = $('.mc-err');
    const fail = (e) => { err.hidden = false; err.textContent = errText(e); };
    const W = () => $('#bc-which').value;
    const list = $('#bc-list');
    const FIELDS = ['cmd', 'params', 'path', 'icon', 'tooltip', 'iconic'];
    const labelOf = (b) => (b.sep ? '──────── separator' : `${tipOf(b) || '(empty)'}${b.cmd && tipOf(b) !== b.cmd ? '   ' + b.cmd : ''}`);
    const showFlag = { main: on('Layout.ButtonBar'), vertical: on('Layout.ButtonBarVertical') };
    const paint = (sel) => {
      const bs = pending[W()].buttons;
      fillSelect(list, bs.map((b, i) => ({ value: i, label: labelOf(b) })), sel === undefined);
      if (sel !== undefined) list.value = String(Math.max(0, Math.min(sel, bs.length - 1)));
      if (!list.value && bs.length) list.value = '0';
      const b = bs[+list.value];
      for (const f of FIELDS) {
        const el = $(`#bc-${f}`);
        el.value = b && !b.sep ? String(b[f] ?? (f === 'iconic' ? 0 : '')) : '';
        el.disabled = !b || b.sep;
      }
      $('#bc-file').value = pending[W()].path || '';
      $('#bc-show').checked = showFlag[W()];
    };
    for (const f of FIELDS) {
      $(`#bc-${f}`).addEventListener('input', () => {
        const b = pending[W()].buttons[+list.value];
        if (!b || b.sep) return;
        b[f] = f === 'iconic' ? parseInt($(`#bc-${f}`).value, 10) || 0 : $(`#bc-${f}`).value;
        if (f === 'cmd') b.subBar = /\.bar("?\s|"?$)/i.test(b.cmd.trim());
        dirty[W()] = true;
        const o = list.selectedOptions[0];
        if (o) o.textContent = labelOf(b);
      });
    }
    list.addEventListener('change', () => paint(+list.value));
    $('#bc-which').addEventListener('change', () => paint(0));
    $('#bc-file').addEventListener('change', () => { pending[W()].path = $('#bc-file').value.trim(); dirty[W()] = true; });
    $('#bc-show').addEventListener('change', () => { showFlag[W()] = $('#bc-show').checked; });
    const insert = (b) => {
      const bs = pending[W()].buttons;
      const at = list.value === '' ? bs.length : +list.value + 1;
      bs.splice(at, 0, b);
      dirty[W()] = true;
      paint(at);
    };
    $('.bc-add').onclick = () => { insert({ icon: '', cmd: '', params: '', path: '', tooltip: '', iconic: 0, sep: false, subBar: false }); $('#bc-cmd').focus(); };
    $('.bc-sep').onclick = () => insert({ sep: true, icon: '', cmd: '' });
    $('.bc-del').onclick = () => {
      const bs = pending[W()].buttons;
      const i = +list.value;
      if (!(i >= 0 && i < bs.length)) return;
      bs.splice(i, 1);
      dirty[W()] = true;
      paint(i);
    };
    const move = (d) => {
      const bs = pending[W()].buttons;
      const i = +list.value, j = i + d;
      if (!(i >= 0 && j >= 0 && j < bs.length)) return;
      [bs[i], bs[j]] = [bs[j], bs[i]];
      dirty[W()] = true;
      paint(j);
    };
    $('.bc-up').onclick = () => move(-1);
    $('.bc-down').onclick = () => move(1);
    $('.bc-copy').onclick = async () => {
      const b = pending[W()].buttons[+list.value];
      if (!b || b.sep) return;
      clip = { ...b };
      await go.ConfigBar.CopyButton(b).catch(() => {});
    };
    $('.bc-paste').onclick = async () => {
      let nb = await go.ConfigBar.PasteButton().catch(() => null);
      if (!nb) nb = clip;
      if (!nb) { fail('The clipboard does not hold a button.'); return; }
      insert({ ...nb });
    };
    $('.bc-load').onclick = async () => {
      const picked = await go.Configuration.ChooseFile('Open a button bar', false, '', '*.bar').catch(() => '');
      const path = await inputDialog({ title: 'Open a button bar', label: 'Bar file', value: picked || $('#bc-file').value, okLabel: 'Open', kind: 'bar-file' });
      if (!path) return;
      try {
        const bar = await go.ConfigBar.LoadBar(path);
        pending[W()] = { path: bar.path, buttons: bar.buttons };
        dirty[W()] = true;
        paint(0);
      } catch (e) { fail(e); }
    };
    go.ConfigBar.ListBars().then((files) => { $('#mc-config-barfiles').innerHTML = (files || []).map((f) => `<option value="${esc(f)}">`).join(''); }).catch(() => {});
    $('#bc-size').value = String(look.iconSize || 16);
    $('#bc-flat').checked = !!look.flat;
    $('#bc-wrap').checked = !!look.wrap;
    $('#bc-which').value = which;
    paint(index >= 0 ? index : 0);

    $('.cancel').onclick = () => done(false);
    box.onsubmit = (ev) => {
      ev.preventDefault();
      track((async () => {
        try {
          for (const w of ['main', 'vertical']) {
            if (!dirty[w]) continue;
            const path = await go.ConfigBar.SaveBar(w, pending[w].path, pending[w].buttons);
            bars[w] = { ...pending[w], path };
          }
          look = { iconSize: +$('#bc-size').value, flat: $('#bc-flat').checked, wrap: $('#bc-wrap').checked };
          await go.ConfigBar.SetOptions(look.iconSize, look.flat, look.wrap);
          const vis = {};
          if (showFlag.main !== on('Layout.ButtonBar')) vis['Layout.ButtonBar'] = showFlag.main ? '1' : '0';
          if (showFlag.vertical !== on('Layout.ButtonBarVertical')) vis['Layout.ButtonBarVertical'] = showFlag.vertical ? '1' : '0';
          if (Object.keys(vis).length) await applyOptions(vis);
          renderBars();
          done(true);
        } catch (e) { fail(e); }
      })());
    };
  }, false, { kind: 'button-config' });
}

command({ name: 'cm_ButtonConfig', label: 'Change button bar…', run: (app, args) => openButtonConfig(app, args || {}) });

addControlCommand('buttonbar', async () => ({ ok: true, main: bars.main, vertical: bars.vertical, look,
  rendered: [...document.querySelectorAll('#toolbar .mc-config-btn')].map((b) => b.dataset.tip) }));
addControlCommand('buttonbar_click', async (m) => {
  const which = m.bar === 'vertical' ? 'vertical' : 'main';
  const b = bars[which].buttons[+m.index];
  if (!b || b.sep) throw new Error(`there is no button ${m.index}`);
  runButton(theApp, b, { dropped: m.dropped || [] });
  return { ok: true };
});

export function initButtonBar(app) {
  for (const id of Object.values(ELS)) { const el = document.getElementById(id); if (el) wire(el); }
  // labels of commands registered later (and key changes) show in the tooltips
  app.on('ready', renderBars);
  app.on('settings', renderBars);
  // Wails' own file drop (when the window has it on) gives real paths
  onFileDrop((x, y, paths) => {
    const el = document.elementFromPoint(x, y);
    const bar = el?.closest?.('#toolbar, #vbar');
    if (!bar) return;
    const btn = el.closest('.mc-config-btn');
    track(dropOn(barOf(bar), btn ? +btn.dataset.i : -1, paths || []).catch((e) => app.flash(errText(e), 'err')));
  });
  return loadBars();
}
