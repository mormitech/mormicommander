// The command palette (board #21, F-857) and the command browser (cm_CommandBrowser, F-373): one search box over
// everything the app can do or go to. Ctrl+Shift+P (also Cmd+Shift+P on macOS) opens the palette: every registered
// command with its keys, the folder hotlist, the folder history of both panels, the open tabs and the pages of
// Options. Typing filters and ranks them (fuzzy: "sync" finds Synchronize folders, "sydi" too), recently chosen items
// come first, Up/Down/PgUp/PgDn/Home/End move, Enter runs the highlighted item and Esc closes, so it works with the
// keyboard only; a click runs an item too. "All commands…" (Commands menu) opens the same dialog with only the
// commands, and its "Keyboard shortcuts…" button goes to Options to give one a key. F2 or "Assign key…" (board issue
// #38, F-373) opens Options > Keyboard shortcuts with the highlighted command already picked, so a key can be given to
// it straight from the list; this works in the palette too when the highlighted item is a command.
//
// The palette is additive: nothing else changes, and it only remembers what was chosen in it (settings key
// "commandPalette": {recent: [item keys]}). The matching and ranking are pure (palette-lib.js). Control mode: the
// dialog is an ordinary page dialog of kind "palette" (field "palette-q"; answering with no button runs the
// highlighted item), and "palette" with {query, only} returns the ranked items without opening anything.

import './palette.css';
import { command, commandList, keysFor } from '../core/registry.js';
import { modal, esc } from '../core/dialogs.js';
import { label as keyLabel } from '../core/keys.js';
import { addControlCommand } from '../core/control.js';
import { go } from '../core/go.js';
import { app as theApp } from '../core/app.js';
import { rankItems, noteRecent, commandWords, uniqueFolders, flattenHotlist, assignTarget } from './palette-lib.js';
import { tabList, activate } from './tabs.js';
import { runEntry } from './hotlist.js';
import { loadSchema } from './config/util.js';
import { EXTRA_PAGES } from './config/options.js';

const SELF = new Set(['cm_CommandPalette', 'cm_CommandBrowser']);
const SHOW_MAX = 200; // rows drawn at once; typing narrows the rest down
const GROUPS = { cmd: 'Command', tab: 'Tab', hotlist: 'Hotlist', dir: 'Folder', page: 'Options' };

const recentList = (app) => { const s = app.setting('commandPalette', {}) || {}; return Array.isArray(s.recent) ? s.recent : []; };
function remember(app, key) {
  const s = app.setting('commandPalette', {}) || {};
  app.setSetting('commandPalette', { ...s, recent: noteRecent(s.recent, key) });
}

// ---- the items ----

function commandItems(app) {
  const out = [];
  for (const c of commandList()) {
    if (SELF.has(c.name)) continue;
    let off = false;
    try { off = !!(c.enabled && !c.enabled(app)); } catch { off = false; }
    const keys = keysFor(c.name).map((k) => keyLabel(k, app.platform));
    out.push({ key: `cmd:${c.name}`, group: 'cmd', label: String(c.label || c.name), hint: keys.join(', '), detail: c.name,
      words: commandWords(c.name), disabled: off, run: () => app.run(c.name) });
  }
  out.sort((a, b) => a.label.localeCompare(b.label));
  return out;
}

function tabItems(app) {
  const out = [];
  for (const side of ['left', 'right']) {
    for (const t of tabList(side)) {
      if (t.active && tabList(side).length < 2) continue; // a lone tab is just the panel
      out.push({ key: `tab:${side}:${t.path}`, group: 'tab', label: t.caption || t.path, detail: t.path, hint: `${side === 'left' ? 'Left' : 'Right'}${t.active ? ', shown' : ''}`,
        run: async () => { const ok = await activate(side, t.index); app.setActive(app.panel(side)); return ok; } });
    }
  }
  return out;
}

async function hotlistItems(app) {
  const tree = await go.TabsHotlist.Hotlist().catch(() => []);
  return flattenHotlist(tree).map(({ entry, label, trail }) => {
    const cmd = String(entry.cmd || '');
    return { key: `hotlist:${trail}/${label}/${cmd}`, group: 'hotlist', label, detail: cmd.replace(/^cd\s+/i, '').replace(/^"(.*)"$/, '$1'), hint: trail,
      run: () => runEntry(app.cur, entry) };
  });
}

function folderItems(app) {
  const paths = [];
  for (const p of [app.cur, app.other]) if (p) paths.push(...[...(p.back || [])].reverse(), ...(p.fwd || []));
  return uniqueFolders(paths, app.platform === 'linux').map((path) => ({ key: `dir:${path}`, group: 'dir', label: path, run: () => app.cur.go(path) }));
}

async function pageItems(app) {
  const schema = await loadSchema().catch(() => null);
  const pages = [...(schema?.pages || []).map((p) => ({ id: p.id, title: p.title })), ...Object.entries(EXTRA_PAGES).map(([id, x]) => ({ id, title: x.title, words: x.words }))];
  return pages.map((p) => ({ key: `page:${p.id}`, group: 'page', label: `Options: ${p.title}`, words: p.words ? [p.words] : [], run: () => app.run('cm_Config', { page: p.id }) }));
}

// gather collects the items: only 'commands' for the command browser, everything for the palette. A source that fails
// (no hotlist yet, settings not loaded) is left out rather than stopping the palette.
export async function gather(app, only = '') {
  const items = commandItems(app);
  if (only === 'commands') return items;
  const extra = await Promise.all([Promise.resolve().then(() => tabItems(app)), hotlistItems(app), Promise.resolve().then(() => folderItems(app)), pageItems(app)]
    .map((p) => p.catch(() => [])));
  return [...items, ...extra.flat()];
}

// ---- the dialog ----

async function openPalette(app, { only = '', query = '' } = {}) {
  const items = await gather(app, only);
  const recent = recentList(app);
  const browser = only === 'commands';
  let onKey = () => false; // set by the builder: the list's own keys (Up/Down, PgUp/PgDn, Ctrl+Home/End)
  const chosen = await modal((box, done) => {
    box.classList.add('mc-palette');
    box.innerHTML = `<h2>${browser ? 'All commands' : 'Command palette'}</h2>
      <input id="palette-q" type="text" spellcheck="false" autofocus aria-label="Search" placeholder="${browser ? 'Type to find a command' : 'Type a command, folder, tab or setting'}" value="${esc(query)}">
      <div class="mc-palette-list" role="listbox"></div>
      <div class="mc-palette-foot"><span class="mc-palette-count"></span>
        ${browser ? '<button type="button" class="mc-palette-assign" title="Give the highlighted command a key (F2)">Assign key…</button><button type="button" class="mc-palette-keys">Keyboard shortcuts…</button>' : ''}
        <button type="button" class="cancel">Close</button></div>`;
    const input = box.querySelector('#palette-q');
    const list = box.querySelector('.mc-palette-list');
    const count = box.querySelector('.mc-palette-count');
    let shown = [], at = 0;
    const paint = () => {
      list.querySelectorAll('.mc-palette-it').forEach((d) => d.classList.toggle('on', +d.dataset.i === at));
      list.querySelector('.mc-palette-it.on')?.scrollIntoView({ block: 'nearest' });
    };
    const refresh = () => {
      const all = rankItems(items, input.value, recent);
      shown = all.slice(0, SHOW_MAX);
      at = Math.max(0, shown.findIndex((it) => !it.disabled));
      list.innerHTML = shown.length ? shown.map((it, i) => `<div class="mc-palette-it${it.disabled ? ' off' : ''}" data-i="${i}" role="option">
          ${browser ? '' : `<span class="mc-palette-g">${esc(GROUPS[it.group] || '')}</span>`}<span class="mc-palette-l">${esc(it.label)}</span>
          ${it.detail && it.detail !== it.label ? `<span class="mc-palette-d">${esc(it.detail)}</span>` : ''}${it.hint ? `<span class="mc-palette-k">${esc(it.hint)}</span>` : ''}</div>`).join('')
        : '<p class="mc-palette-none">Nothing matches.</p>';
      count.textContent = all.length > shown.length ? `${shown.length} of ${all.length} shown` : `${all.length} ${all.length === 1 ? 'item' : 'items'}`;
      paint();
    };
    const move = (d) => {
      if (!shown.length) return;
      at = Math.max(0, Math.min(shown.length - 1, at + d));
      paint();
    };
    const choose = (i) => {
      const it = shown[i];
      if (!it) return;
      if (it.disabled) { app.flash(`${it.label.replace(/…$/, '')} is not available now.`); return; }
      done(it);
    };
    input.addEventListener('input', refresh);
    list.addEventListener('mousemove', (ev) => { const d = ev.target.closest('.mc-palette-it'); if (d && at !== +d.dataset.i) { at = +d.dataset.i; paint(); } });
    list.addEventListener('click', (ev) => { const d = ev.target.closest('.mc-palette-it'); if (d) choose(+d.dataset.i); });
    box.querySelector('.cancel').onclick = () => done(null);
    const keysBtn = box.querySelector('.mc-palette-keys');
    if (keysBtn) keysBtn.onclick = () => done({ key: '', run: () => app.run('cm_ConfigHotkeys') });
    // assign opens the keyboard shortcuts for the highlighted command (a disabled one can get a key too)
    const assign = () => {
      const name = assignTarget(shown[at]);
      if (!name) { app.flash('Highlight a command to give it a key.'); return; }
      done({ key: '', run: () => app.run('cm_ConfigHotkeys', { command: name }) });
    };
    const assignBtn = box.querySelector('.mc-palette-assign');
    if (assignBtn) assignBtn.onclick = assign;
    box.onsubmit = (ev) => { ev.preventDefault(); choose(at); };
    onKey = (ev) => {
      const page = 8;
      switch (ev.key) {
        case 'ArrowDown': move(1); return true;
        case 'ArrowUp': move(-1); return true;
        case 'PageDown': move(page); return true;
        case 'PageUp': move(-page); return true;
        case 'F2': assign(); return true;
        default: break;
      }
      if (ev.ctrlKey && !ev.altKey && !ev.metaKey && (ev.key === 'Home' || ev.key === 'End')) { at = ev.key === 'Home' ? 0 : shown.length - 1; move(0); return true; }
      return false;
    };
    refresh();
  }, null, { kind: 'palette', onKey: (ev) => onKey(ev) });
  if (!chosen) return false;
  if (chosen.key) remember(app, chosen.key);
  return chosen.run();
}

command({ name: 'cm_CommandPalette', label: 'Command palette…', keys: ['Ctrl+Shift+P', 'Shift+Cmd+P'], menu: 'Commands', menuAfter: 'cm_CommandBrowser',
  run: (app, args) => openPalette(app, { query: args?.query || '' }) });
command({ name: 'cm_CommandBrowser', label: 'All commands…', run: (app, args) => openPalette(app, { only: 'commands', query: args?.query || '' }) });

// "palette": the ranked items for a query, as the palette would show them ({query, only: 'commands'}), for QA.
addControlCommand('palette', async (m) => {
  const items = rankItems(await gather(theApp, m.only || ''), m.query || '', recentList(theApp), m.limit || 50);
  return { ok: true, items: items.map((it) => ({ key: it.key, group: it.group, label: it.label, detail: it.detail || '', hint: it.hint || '', disabled: !!it.disabled })) };
});
