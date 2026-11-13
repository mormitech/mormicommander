// Help (FEATURES.md section 19): F1 opens the help (cm_HelpIndex) with its topics; Help > Keyboard shortcuts
// (cm_Keyboard) opens it on the keys. The keyboard page is docs/keyboard.md followed by every key as it is set now,
// generated from the command registry, so it always shows the user's own changes (F-451, F-452). The menus page lists
// the menus with their keys; the start-up page lists the command-line parameters. About is the About window (F-453).

import { command, allKeys, getCommand } from '../../core/registry.js';
import { infoDialog, esc } from '../../core/dialogs.js';
import { LABELS } from '../../core/app.js';
import { menuSpec } from '../../core/menus.js';
import { go } from '../../core/go.js';
import keyboardDoc from '../../../../docs/keyboard.md?raw';
import { keyText } from './keys.js';

// md renders the small Markdown subset of our docs: headings, paragraphs, lists, **bold**, `code`, [links](...).
export function md(text) {
  const inline = (s) => esc(s)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/\\\\/g, '\\');
  const out = [];
  let para = [], list = null;
  const flush = () => {
    if (para.length) { out.push(`<p>${inline(para.join(' '))}</p>`); para = []; }
    if (list) { out.push(`<${list.tag}>${list.items.map((i) => `<li>${inline(i)}</li>`).join('')}</${list.tag}>`); list = null; }
  };
  for (const raw of String(text).split(/\r?\n/)) {
    const line = raw.trimEnd();
    let m;
    if (!line.trim()) { flush(); continue; }
    if ((m = line.match(/^(#{1,4})\s+(.*)$/))) { flush(); const n = Math.min(4, m[1].length + 2); out.push(`<h${n}>${inline(m[2])}</h${n}>`); continue; }
    if ((m = line.match(/^\s*(\d+\.|[-*])\s+(.*)$/))) {
      const tag = /\d/.test(m[1]) ? 'ol' : 'ul';
      if (para.length) { out.push(`<p>${inline(para.join(' '))}</p>`); para = []; }
      if (!list || list.tag !== tag) { if (list) flush(); list = { tag, items: [] }; }
      list.items.push(m[2]);
      continue;
    }
    if (list && /^\s+/.test(raw)) { list.items[list.items.length - 1] += ' ' + line.trim(); continue; }
    if (list) flush();
    para.push(line.trim());
  }
  flush();
  return out.join('\n');
}

const label = (n) => (getCommand(n)?.label || LABELS[n] || n).replace(/…$/, '');

function keysTable() {
  const by = new Map();
  for (const [k, v] of allKeys()) { if (!by.has(v)) by.set(v, []); by.get(v).push(k); }
  const rows = [...by.entries()].map(([n, ks]) => ({ n, l: label(n), ks, ready: !!getCommand(n) }))
    .sort((a, b) => a.l.localeCompare(b.l));
  return `<table class="mc-config-keys"><thead><tr><th>Command</th><th>Keys</th></tr></thead><tbody>${rows.map((r) =>
    `<tr class="${r.ready ? '' : 'missing'}"><td>${esc(r.l)} <span class="mc-config-cmdname">${esc(r.n)}</span></td><td>${r.ks.map((k) => `<kbd>${esc(keyText(k))}</kbd>`).join(' ')}</td></tr>`).join('')}</tbody></table>`;
}

function menusPage() {
  const keysOf = new Map();
  for (const [k, v] of allKeys()) { if (!keysOf.has(v)) keysOf.set(v, []); keysOf.get(v).push(k); }
  const list = (items) => `<ul>${items.map((it) => {
    if (it.sep) return '';
    if (it.items) return `<li>${esc(it.label)}${list(it.items)}</li>`;
    const k = keysOf.get(it.cmd)?.[0];
    return `<li class="${it.disabled ? 'missing' : ''}">${esc(it.label)}${k ? ` <kbd>${esc(keyText(k))}</kbd>` : ''}</li>`;
  }).join('')}</ul>`;
  return `<h3>Menus</h3><p>Every menu item is a command; the key next to it runs the same command. Greyed items are not ready yet.</p>
    <div class="mc-config-menus">${menuSpec().map((m) => `<div><h4>${esc(m.label)}</h4>${list(m.items)}</div>`).join('')}</div>`;
}

const START = `<h3>Starting mormiCommander</h3>
  <p>mormiCommander understands Total Commander's start-up parameters, with <code>-</code> or <code>/</code>:</p>
  <table class="mc-config-keys"><tbody>
  <tr><td><code>mormicommander FOLDER1 FOLDER2</code></td><td>open the folders in the left and right panel</td></tr>
  <tr><td><code>-L=FOLDER</code>, <code>-R=FOLDER</code></td><td>the folder of the left / right panel</td></tr>
  <tr><td><code>-P=L</code>, <code>-P=R</code></td><td>make the left / right panel active</td></tr>
  <tr><td><code>-T</code></td><td>open the folders in new tabs</td></tr>
  <tr><td><code>-S</code></td><td>the folders are source and target (active and other panel), not left and right</td></tr>
  <tr><td><code>-O</code></td><td>open the folders in the window that is already open</td></tr>
  <tr><td><code>-N</code></td><td>always open a new window</td></tr>
  <tr><td><code>-I=FILE</code>, <code>-F=FILE</code></td><td>use another settings file / FTP settings file</td></tr>
  <tr><td><code>-S=L FILE</code></td><td>open only the viewer for a file (<code>-S=C A B</code>: compare two files, <code>-S=S A B</code>: synchronize two folders)</td></tr>
  </tbody></table>
  <p>A path to a file opens its folder with the cursor on the file. Options &gt; Operation &gt; "Allow only one copy" makes every start use the open window, like <code>-O</code>.</p>`;

async function settingsPage() {
  const info = await go.Configuration.Info().catch(() => ({}));
  return `<h3>Settings</h3><p>Configuration &gt; Options (${esc(keyText('Cmd+,'))} on a Mac) changes how mormiCommander looks and works. The settings are kept in a text file in the style of Total Commander's wincmd.ini:</p>
    <p><code>${esc(info.ini || '')}</code></p>
    <p>Options &gt; Settings file opens it in an editor, imports a Total Commander wincmd.ini (hotlist, keys, button bar, Start menu, colours, columns), and backs up or restores the settings. A file named mormicommander.ini next to the program makes the program portable.</p>`;
}

const INTRO = `<h3>Welcome</h3>
  <p>mormiCommander shows two folders side by side. The active panel (the source) has the coloured path bar; Tab switches panels. The function keys at the bottom do the main work: F3 view, F4 edit, F5 copy, F6 move or rename, F7 new folder, F8 delete, each from the active panel to the other one.</p>
  <ul><li>Select files with Space or Ins, or with Shift and the cursor keys; Num+ and Num- select by pattern.</li>
  <li>Type to use the command line under the panels: <code>cd folder</code> changes the folder, a command name such as <code>cm_Copy</code> runs it, anything else runs as a program.</li>
  <li>Ctrl+D is the folder hotlist, Alt+Down the folder history, Alt+F7 finds files, Ctrl+T opens a tab.</li>
  <li>The button bar under the menu runs commands and programs; right-click a button to change it.</li>
  <li>Everything in the menus has a key; Help &gt; Keyboard shortcuts lists them as they are set now.</li></ul>`;

const TOPICS = [['intro', 'Welcome'], ['keys', 'Keyboard shortcuts'], ['menus', 'Menus'], ['start', 'Start-up parameters'], ['settings', 'Settings']];

export async function openHelp(topic = 'intro') {
  const pages = {
    intro: INTRO,
    keys: `<div class="mc-config-doc">${md(keyboardDoc)}</div><h3>All keys as they are set now</h3>${keysTable()}`,
    menus: menusPage(),
    start: START,
    settings: await settingsPage(),
  };
  return infoDialog(`<h2>mormiCommander help</h2>
    <div class="mc-config-help"><nav>${TOPICS.map(([id, t]) => `<button type="button" data-topic="${id}"${id === topic ? ' class="on"' : ''}>${esc(t)}</button>`).join('')}</nav>
    <div class="mc-config-helpbody">${TOPICS.map(([id]) => `<section data-topic="${id}"${id === topic ? '' : ' hidden'}>${pages[id]}</section>`).join('')}</div></div>`, 'mc-config-helpbox', 'help')
    .then(() => true);
}

// the topic buttons switch pages (wired once for every help box that opens)
document.addEventListener('click', (ev) => {
  const b = ev.target.closest?.('.mc-config-help nav button[data-topic]');
  if (!b) return;
  const box = b.closest('.mc-config-help');
  box.querySelectorAll('nav button').forEach((x) => x.classList.toggle('on', x === b));
  box.querySelectorAll('section[data-topic]').forEach((s) => { s.hidden = s.dataset.topic !== b.dataset.topic; });
  box.querySelector('.mc-config-helpbody').scrollTop = 0;
});

command({ name: 'cm_HelpIndex', label: 'Help', run: (app, args) => openHelp(args?.topic || 'intro') });
command({ name: 'cm_Keyboard', label: 'Keyboard shortcuts', run: () => openHelp('keys') });
command({ name: 'cm_Register', label: 'License', run: () => go.App.ShowAbout() });
