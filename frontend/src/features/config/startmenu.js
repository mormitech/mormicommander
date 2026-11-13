// The Start menu (FEATURES.md section 18): the user's own entries in the Start menu of the menu bar, with submenus and
// separators (F-448), each with an optional key of its own (F-449), and the dialog that changes them (Configuration >
// Change start menu, cm_ConfigStartMenu, F-450). Entries are kept in the [user] section of the settings file
// (internal/config) and run like buttons: cm_ / em_ commands, "cd folder", or a program with parameters.

import { command, getCommand } from '../../core/registry.js';
import { addKeyHandler } from '../../core/keyboard.js';
import { addMenuItems, refreshMenu } from '../../core/menus.js';
import { modal, esc, dialogOpen } from '../../core/dialogs.js';
import { popupOpen } from '../../core/popup.js';
import { addControlCommand } from '../../core/control.js';
import { normalizeCombo } from '../../core/keys.js';
import { track } from '../../core/busy.js';
import { go, errText } from '../../core/go.js';
import { app as theApp } from '../../core/app.js';
import { fillSelect } from './util.js';
import { keyText, hotkeyOK } from './keys.js';
import { runButton } from './run.js';
import { allCommandNames, commandLabel } from './options.js';

let items = []; // the flat list from Go: {kind, depth, title, cmd, params, path, key}
const keys = new Map(); // combo → index in items

const cmdName = (i) => `cm_StartMenu_${i + 1}`;

function ensureCommands() {
  items.forEach((it, i) => {
    const name = cmdName(i);
    const label = it.title || it.cmd || 'Start menu entry';
    const c = getCommand(name);
    if (c) { c.label = label; return; }
    command({
      name, label,
      run: (app) => {
        const cur = items[i];
        if (!cur || cur.kind !== 'item') return false;
        return runButton(app, { cmd: cur.cmd, params: cur.params, path: cur.path, tooltip: cur.title }, { title: cur.title });
      },
    });
  });
}

// menuTree turns the flat list into the menu layout of core/menus.js.
function menuTree() {
  const root = [];
  const stack = [root];
  items.forEach((it, i) => {
    const d = Math.max(0, Math.min(it.depth, stack.length - 1));
    stack.length = d + 1;
    const list = stack[d];
    if (it.kind === 'sep') list.push('-');
    else if (it.kind === 'submenu') { const sub = []; list.push([it.title, sub]); stack.push(sub); }
    else {
      const k = it.key ? `   ${keyText(it.key)}` : '';
      list.push([`${(it.title || it.cmd).replace(/&(.)/g, '$1')}${k}`, cmdName(i)]);
    }
  });
  const clean = (l) => l.filter((x) => !Array.isArray(x) || !Array.isArray(x[1]) || (x[1] = clean(x[1])).length);
  const out = clean(root);
  return out.length ? ['-', ...out] : [];
}

export async function loadStartMenu() {
  try { items = (await go.ConfigStart.Menu()) || []; } catch { items = []; }
  ensureCommands();
  keys.clear();
  items.forEach((it, i) => { if (it.kind === 'item' && it.key && hotkeyOK(it.key)) keys.set(normalizeCombo(it.key), i); });
  refreshMenu();
}

addMenuItems('Start', menuTree);

// the entries' own keys come before the keymap (F-449)
addKeyHandler((ev, combo) => {
  if (!keys.size || dialogOpen() || popupOpen()) return false;
  const i = keys.get(combo);
  if (i === undefined) return false;
  theApp.run(cmdName(i));
  return true;
});

// ---- Change start menu (F-450) ----
export function openStartMenuConfig(app) {
  const pending = items.map((it) => ({ ...it }));
  return modal((box, done) => {
    box.classList.add('mc-config-box');
    box.innerHTML = `<h2>Change start menu</h2>
      <datalist id="mc-config-sm-cmds">${allCommandNames(app.platform).map((n) => `<option value="${esc(n)}">${esc(commandLabel(n))}</option>`).join('')}</datalist>
      <div class="mc-config-listedit"><select id="sm-list" size="14" aria-label="Start menu entries"></select>
        <div class="mc-config-listfields">
          <div class="mc-row"><label for="sm-title">Title (&amp; before a letter underlines it)</label><input type="text" id="sm-title"></div>
          <div class="mc-row"><label for="sm-cmd">Command (cm_ command, em_ command, program, or cd folder)</label><input type="text" id="sm-cmd" spellcheck="false" list="mc-config-sm-cmds"></div>
          <div class="mc-row"><label for="sm-params">Parameters (%P path, %N name, %S selected names; ? asks first)</label><input type="text" id="sm-params" spellcheck="false"></div>
          <div class="mc-row"><label for="sm-path">Start folder</label><input type="text" id="sm-path" spellcheck="false"></div>
          <div class="mc-row"><label for="sm-key">Key: click here and press it (Backspace: none)</label><input type="text" id="sm-key" data-keycapture="1" spellcheck="false" placeholder="No key"></div>
        </div>
      </div>
      <div class="mc-config-rowbtns"><button type="button" class="sm-add">Add entry</button><button type="button" class="sm-sub">Add submenu</button><button type="button" class="sm-sep">Add separator</button>
        <button type="button" class="sm-del">Delete</button><button type="button" class="sm-up">Move up</button><button type="button" class="sm-down">Move down</button>
        <button type="button" class="sm-out">Move out of submenu</button><button type="button" class="sm-in">Move into submenu</button></div>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    const $ = (s) => box.querySelector(s);
    const err = $('.mc-err');
    const list = $('#sm-list');
    const FIELDS = ['title', 'cmd', 'params', 'path', 'key'];
    const labelOf = (it) => {
      const pad = '    '.repeat(it.depth);
      if (it.kind === 'sep') return `${pad}────────`;
      if (it.kind === 'submenu') return `${pad}▸ ${it.title}`;
      return `${pad}${it.title || it.cmd || '(new entry)'}${it.key ? '   ' + keyText(it.key) : ''}`;
    };
    const paint = (sel) => {
      fillSelect(list, pending.map((it, i) => ({ value: i, label: labelOf(it) })), sel === undefined);
      if (sel !== undefined && pending.length) list.value = String(Math.max(0, Math.min(sel, pending.length - 1)));
      if (!list.value && pending.length) list.value = '0';
      const it = pending[+list.value];
      for (const f of FIELDS) {
        const el = $(`#sm-${f}`);
        el.value = it ? it[f] || '' : '';
        el.disabled = !it || it.kind === 'sep' || (it.kind === 'submenu' && f !== 'title');
      }
    };
    for (const f of FIELDS) {
      $(`#sm-${f}`).addEventListener('input', () => {
        const it = pending[+list.value];
        if (!it) return;
        it[f] = $(`#sm-${f}`).value;
        const o = list.selectedOptions[0];
        if (o) o.textContent = labelOf(it);
      });
    }
    list.addEventListener('change', () => paint(+list.value));
    // depth rules: an entry can be one deeper than a submenu right above it, or as deep as the entry above
    const maxDepth = (i) => (i <= 0 ? 0 : pending[i - 1].kind === 'submenu' ? pending[i - 1].depth + 1 : pending[i - 1].depth);
    const fix = () => { pending.forEach((it, i) => { it.depth = Math.max(0, Math.min(it.depth, maxDepth(i))); }); };
    const insert = (it) => {
      const at = list.value === '' ? pending.length : +list.value + 1;
      const prev = pending[at - 1];
      it.depth = prev ? (prev.kind === 'submenu' ? prev.depth + 1 : prev.depth) : 0;
      pending.splice(at, 0, it);
      fix();
      paint(at);
    };
    $('.sm-add').onclick = () => { insert({ kind: 'item', title: '', cmd: '', params: '', path: '', key: '' }); $('#sm-title').focus(); };
    $('.sm-sub').onclick = () => { insert({ kind: 'submenu', title: 'New submenu', cmd: '', params: '', path: '', key: '' }); $('#sm-title').focus(); };
    $('.sm-sep').onclick = () => insert({ kind: 'sep', title: '-', cmd: '', params: '', path: '', key: '' });
    $('.sm-del').onclick = () => {
      const i = +list.value;
      if (!(i >= 0 && i < pending.length)) return;
      let n = 1; // a submenu goes with its entries
      if (pending[i].kind === 'submenu') while (i + n < pending.length && pending[i + n].depth > pending[i].depth) n++;
      pending.splice(i, n);
      fix();
      paint(i);
    };
    const move = (d) => {
      const i = +list.value, j = i + d;
      if (!(i >= 0 && j >= 0 && j < pending.length)) return;
      [pending[i], pending[j]] = [pending[j], pending[i]];
      fix();
      paint(j);
    };
    $('.sm-up').onclick = () => move(-1);
    $('.sm-down').onclick = () => move(1);
    $('.sm-out').onclick = () => { const it = pending[+list.value]; if (it && it.depth > 0) { it.depth--; fix(); paint(+list.value); } };
    $('.sm-in').onclick = () => { const i = +list.value; const it = pending[i]; if (it && it.depth < maxDepth(i)) { it.depth++; paint(i); } };
    paint(0);
    $('.cancel').onclick = () => done(false);
    box.onsubmit = (ev) => {
      ev.preventDefault();
      const bad = pending.find((it) => it.key && !hotkeyOK(it.key));
      if (bad) { err.hidden = false; err.textContent = `${keyText(bad.key)} would take a key used for typing; use a key with Ctrl, Alt or Cmd, or a function key.`; return; }
      track(go.ConfigStart.SaveMenu(pending).then(async () => { await loadStartMenu(); done(true); }, (e) => { err.hidden = false; err.textContent = errText(e); }));
    };
  }, false, { kind: 'startmenu-config' });
}

command({ name: 'cm_ConfigStartMenu', label: 'Change start menu…', run: (app) => openStartMenuConfig(app) });

addControlCommand('startmenu', async () => ({ ok: true, items, keys: Object.fromEntries(keys) }));

export function initStartMenu() { return loadStartMenu(); }
