// The directory hotlist (Ctrl+D) and the folder histories (Alt+Down, frequently used folders): FEATURES.md section 31,
// behaviour from docs/spec area-8 sections 1 and 2. The hotlist tree, its commands (cd, cd file.tab, cm_*, em_*,
// programs), variable expansion, search, TC [DirMenu] import and the frequently used counts are the settings engine's
// (internal/config, through feature_tabshotlist.go); this file draws the popups and the Configure dialog.
//
// Popup keys: Up/Down, Enter (Shift+Enter opens the folder in a new tab, F-777), the "&" letter of an entry picks it
// (F-656), Enter on a submenu opens it and Esc goes back up. Choosing a folder in a locked tab opens a new tab (the
// tabs feature's lock rule catches every folder change).

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { showList } from '../core/popup.js';
import { messageDialog, inputDialog, formDialog, modal, esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { openInNewTab, loadTabFile, saveBothTabs, saveSoon } from './tabs.js';

const trimSep = (x) => (x.length > 1 ? x.replace(/[\\/]+$/, '') : x) || x;
const folderName = (p) => { const t = trimSep(p || ''); return t.split(/[\\/]/).pop() || t; };
const quote = (s) => (/\s/.test(s) && !s.startsWith('"') ? `"${s}"` : s);
const shortCmd = (it) => (it.cmd || '').replace(/^cd\s+/i, '').replace(/^"(.*)"$/, '$1');

// anchorFor puts a popup at the file cursor (TC opens the hotlist there), or under the path bar.
function anchorFor(p) {
  const row = p.$.rows.querySelector('.row.cur');
  if (row) { const r = row.getBoundingClientRect(); if (r.bottom > 0 && r.top < innerHeight) return { x: r.left + 24, y: r.bottom }; }
  return p.$.pathbar;
}

// ---- running an entry ----

async function applyMask(p, mask) {
  const names = p.all.map((e) => e.name), dirs = p.all.map((e) => !!e.dir);
  const ok = await go.TabsHotlist.MatchMask(mask, names, dirs);
  const keep = new Set(names.filter((_, i) => ok[i]));
  p.setFilter((e) => e.dir || keep.has(e.name), mask);
}

export async function runEntry(p, it, { shift = false } = {}) {
  const a = await go.TabsHotlist.Resolve(it.cmd || '', it.target || '');
  switch (a.kind) {
    case 'cd': {
      if (!a.path) return false;
      let ok;
      if (shift) { ok = await openInNewTab(p, a.path); p = theApp.panel(p.side); }
      else ok = await p.go(a.path);
      if (ok && a.filter) await applyMask(p, a.filter);
      if (a.target) await p.other.go(a.target);
      return ok;
    }
    case 'tabs': return loadTabFile(a.path, p);
    case 'internal': theApp.setActive(p); return theApp.run(a.command, a.args ? { args: a.args } : {});
    case 'user': theApp.setActive(p); return theApp.run(a.command, a.args ? { args: a.args } : {});
    case 'program': await go.TabsHotlist.RunProgram(a.command, a.args, p.path); return true;
  }
  return false;
}

// ---- the hotlist popup ----

async function showHotlist(p, { anchor } = {}) {
  const tree = await go.TabsHotlist.Hotlist();
  const contains = p.path ? await go.TabsHotlist.Contains(p.path) : false;
  const stack = [];
  let level = tree, at = 0, crumbs = [];
  anchor ??= anchorFor(p);
  for (;;) {
    const items = level.map((it) => (it.kind === 'sep' ? { sep: true }
      : { label: it.kind === 'menu' ? `${it.label || it.title} ▸` : it.label || it.title || shortCmd(it), key: it.key || '', detail: it.kind === 'cmd' ? shortCmd(it) : '', value: it }));
    if (!stack.length) {
      if (items.length) items.push({ sep: true });
      if (contains) items.push({ label: 'Remove current folder', key: 'r', value: 'remove' });
      else items.push({ label: 'Add current folder…', key: 'a', value: 'add' });
      items.push({ label: 'Add current folder and target…', value: 'addboth' });
      items.push({ label: 'Add current tabs…', value: 'addtabs' });
      items.push({ label: 'Search…', value: 'search' });
      items.push({ label: 'Configure…', key: 'c', value: 'config' });
    }
    const r = await showList({ title: stack.length ? crumbs.join(' › ') : 'Folder hotlist', items, anchor, index: at, kind: 'hotlist' });
    if (!r) {
      if (!stack.length) return false;
      ({ level, at, crumbs } = stack.pop());
      continue;
    }
    const v = r.item.value;
    if (v?.kind === 'menu') { stack.push({ level, at: r.index, crumbs }); crumbs = [...crumbs, v.label || v.title]; level = v.items || []; at = 0; continue; }
    if (typeof v === 'string') return special(p, v);
    return runEntry(p, v, { shift: r.shift });
  }
}

async function special(p, what) {
  switch (what) {
    case 'remove': {
      const ok = await go.TabsHotlist.RemoveFolder(p.path);
      theApp.flash(ok ? `${folderName(p.path)} is no longer in the hotlist.` : `${folderName(p.path)} is not in the hotlist.`);
      return ok;
    }
    case 'add': case 'addboth': {
      const both = what === 'addboth';
      const name = await inputDialog({ title: 'Add to the hotlist', label: 'Name of the new entry', value: folderName(p.path), kind: 'hotlist-add',
        note: both ? `Opens ${p.path} here and ${p.other.path} in the other panel.` : p.path });
      if (name === null) return false;
      await go.TabsHotlist.AddFolder(name.trim(), p.path, both ? p.other.path : '');
      theApp.flash(`${name.trim() || p.path} is in the hotlist.`);
      return true;
    }
    case 'addtabs': {
      const name = await inputDialog({ title: 'Add current tabs to the hotlist', label: 'Name of the new entry', value: `${folderName(theApp.left.path)} – ${folderName(theApp.right.path)} (tabs)`,
        kind: 'hotlist-add-tabs', note: 'The tabs of both panels are saved to a tab file; the entry opens them again.', validate: (v) => (v.trim() ? '' : 'Please give the entry a name.') });
      if (!name) return false;
      const dir = await go.TabsHotlist.TabDir();
      if (!dir) { theApp.flash('There is no settings folder to keep the tab file in.', 'err'); return false; }
      const file = `${dir}${theApp.sepFor(dir)}${name.trim().replace(/[\\/:*?"<>|]/g, '_')}.tab`;
      await saveBothTabs(file);
      await go.TabsHotlist.AddFolder(name.trim(), file, '');
      theApp.flash(`${name.trim()} is in the hotlist.`);
      return true;
    }
    case 'search': return searchHotlist(p);
    case 'config': return theApp.run('cm_ConfigDirHotlist');
  }
  return false;
}

// searchHotlist: type words, see every entry (in any submenu) that has all of them (F-658).
async function searchHotlist(p, query) {
  query ??= await inputDialog({ title: 'Search the hotlist', label: 'Words to look for in names and folders', kind: 'hotlist-search' });
  if (query === null) return false;
  const found = await go.TabsHotlist.Filter(query);
  if (!found.length) { theApp.flash(`No hotlist entry has ${query}.`); return false; }
  const r = await showList({ title: `Hotlist: ${query}`, kind: 'hotlist-search-results', anchor: anchorFor(p),
    items: found.map((m) => ({ label: m.breadcrumb ? `${m.breadcrumb} › ${m.title}` : m.title, detail: shortCmd(m), value: m })) });
  if (!r) return false;
  return runEntry(p, r.item.value, { shift: r.shift });
}

// ---- the Configure dialog (cm_ConfigDirHotlist, F-654) ----

const clone = (x) => JSON.parse(JSON.stringify(x));

function configHotlist(app) {
  return go.TabsHotlist.Hotlist().then((tree) => modal((box, done) => {
    tree = clone(tree);
    let sel = tree.length ? '0' : '';
    let cut = null;
    const at = (path) => {
      const ix = path.split('.').map(Number);
      let list = tree, it = null;
      for (let k = 0; k < ix.length; k++) { it = list[ix[k]]; if (!it) return null; if (k < ix.length - 1) list = it.items || []; }
      return { list, index: ix[ix.length - 1], item: it, parent: ix.slice(0, -1).join('.') };
    };
    const flat = () => {
      const out = [];
      const walk = (list, pre, depth) => list.forEach((it, i) => {
        const path = pre ? `${pre}.${i}` : String(i);
        out.push({ path, it, depth });
        if (it.kind === 'menu') walk(it.items || (it.items = []), path, depth + 1);
      });
      walk(tree, '', 0);
      return out;
    };
    const text = (it) => (it.kind === 'sep' ? '────────────' : it.kind === 'menu' ? `▸ ${it.title}` : it.title || shortCmd(it) || '(no name)');
    box.classList.add('mc-hotlist-box');
    box.innerHTML = `<h2>Folder hotlist</h2>
      <div class="mc-hotlist-cfg">
        <select id="hl-list" size="14" aria-label="Hotlist entries"></select>
        <div class="mc-hotlist-side">
          <button type="button" data-a="add">Add item…</button><button type="button" data-a="submenu">Add submenu…</button><button type="button" data-a="sep">Add separator</button>
          <button type="button" data-a="rename">Rename…</button><button type="button" data-a="delete">Delete</button>
          <button type="button" data-a="up">Move up</button><button type="button" data-a="down">Move down</button>
          <button type="button" data-a="cut">Cut</button><button type="button" data-a="paste">Paste</button><button type="button" data-a="sort">Sort submenu</button>
        </div>
      </div>
      <div class="mc-row"><label for="hl-cmd">Command (cd folder, cd file.tab, cm_… command or a program; $HOME, %VAR% and ~ work)</label>
        <div class="mc-hotlist-line"><input type="text" id="hl-cmd" spellcheck="false"><button type="button" data-a="here">This folder</button><button type="button" data-a="there">Other folder</button><button type="button" data-a="browse">Browse…</button></div></div>
      <div class="mc-row"><label for="hl-target">Target: folder for the other panel (optional)</label>
        <div class="mc-hotlist-line"><input type="text" id="hl-target" spellcheck="false"><button type="button" data-a="target">Target: other folder</button><button type="button" data-a="notarget">No target</button></div></div>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" data-a="import" class="mc-hotlist-left">Import from Total Commander…</button><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    const $ = (s) => box.querySelector(s);
    const list = $('#hl-list'), cmd = $('#hl-cmd'), target = $('#hl-target');
    const err = (m) => { const e = $('.mc-err'); e.hidden = !m; e.textContent = m || ''; };
    const draw = () => {
      const rows = flat();
      if (sel && !rows.some((r) => r.path === sel)) sel = rows.length ? rows[Math.min(rows.length - 1, 0)].path : '';
      list.innerHTML = rows.map((r) => `<option value="${r.path}"${r.path === sel ? ' selected' : ''}>${' '.repeat(r.depth)}${esc(text(r.it))}</option>`).join('');
      const cur = sel ? at(sel)?.item : null;
      const isCmd = cur?.kind === 'cmd';
      cmd.value = isCmd ? cur.cmd || '' : '';
      target.value = isCmd ? cur.target || '' : '';
      for (const el of [cmd, target, ...box.querySelectorAll('[data-a=here],[data-a=there],[data-a=browse],[data-a=target],[data-a=notarget]')]) el.disabled = !isCmd;
      for (const a of ['rename', 'delete', 'up', 'down', 'cut']) $(`[data-a=${a}]`).disabled = !cur;
      $('[data-a=rename]').disabled = !cur || cur.kind === 'sep';
      $('[data-a=paste]').disabled = !cut;
      err('');
    };
    // insert puts a new item after the selected one (into a selected empty submenu: as its first entry)
    const insert = (it) => {
      if (!sel) { tree.push(it); sel = String(tree.length - 1); return; }
      const c = at(sel);
      if (c.item.kind === 'menu' && !(c.item.items || []).length) { c.item.items = [it]; sel = `${sel}.0`; return; }
      c.list.splice(c.index + 1, 0, it);
      sel = c.parent ? `${c.parent}.${c.index + 1}` : String(c.index + 1);
    };
    list.addEventListener('change', () => { sel = list.value; draw(); });
    cmd.addEventListener('input', () => { const c = sel && at(sel); if (c?.item.kind === 'cmd') c.item.cmd = cmd.value; });
    target.addEventListener('input', () => { const c = sel && at(sel); if (c?.item.kind === 'cmd') c.item.target = target.value; });
    const setCmd = (v) => { cmd.value = v; cmd.dispatchEvent(new Event('input')); };
    const setTarget = (v) => { target.value = v; target.dispatchEvent(new Event('input')); };
    box.querySelector('.cancel').onclick = () => done(false);
    box.onsubmit = async (ev) => {
      ev.preventDefault();
      try { await go.TabsHotlist.SetHotlist(tree); done(true); } catch (e) { err(errText(e)); }
    };
    box.addEventListener('click', async (ev) => {
      const a = ev.target.closest('button[data-a]')?.dataset.a;
      if (!a) return;
      const c = sel ? at(sel) : null;
      try {
        switch (a) {
          case 'add': {
            const t = await inputDialog({ title: 'Add item', label: 'Name of the new entry ("&" before a letter makes it the hotkey)', value: folderName(app.cur.path), kind: 'hotlist-item' });
            if (t !== null) insert({ kind: 'cmd', title: t.trim(), cmd: `cd ${quote(app.cur.path)}`, target: '' });
            break;
          }
          case 'submenu': {
            const t = await inputDialog({ title: 'Add submenu', label: 'Name of the submenu', kind: 'hotlist-submenu', validate: (v) => (v.trim() ? '' : 'Please give the submenu a name.') });
            if (t) insert({ kind: 'menu', title: t.trim(), items: [] });
            break;
          }
          case 'sep': insert({ kind: 'sep', title: '' }); break;
          case 'rename': {
            if (!c || c.item.kind === 'sep') break;
            const t = await inputDialog({ title: 'Rename', label: 'New name ("&" before a letter makes it the hotkey)', value: c.item.title, kind: 'hotlist-rename' });
            if (t !== null) c.item.title = t.trim();
            break;
          }
          case 'delete': {
            if (!c) break;
            if (c.item.kind === 'menu' && (c.item.items || []).length) {
              const y = await messageDialog({ Title: 'Delete submenu', Message: `Delete the submenu ${c.item.title} with everything in it?`, Buttons: ['Delete', 'Cancel'], Kind: 'hotlist-delete-submenu' });
              if (y !== 'Delete') break;
            }
            c.list.splice(c.index, 1);
            const n = Math.min(c.index, c.list.length - 1);
            sel = n >= 0 ? (c.parent ? `${c.parent}.${n}` : String(n)) : c.parent;
            break;
          }
          case 'up': case 'down': {
            if (!c) break;
            const to = c.index + (a === 'up' ? -1 : 1);
            if (to < 0 || to >= c.list.length) break; // (an entry leaves its submenu with Cut and Paste)
            [c.list[c.index], c.list[to]] = [c.list[to], c.list[c.index]];
            sel = c.parent ? `${c.parent}.${to}` : String(to);
            break;
          }
          case 'cut': {
            if (!c) break;
            cut = c.list.splice(c.index, 1)[0];
            const n = Math.min(c.index, c.list.length - 1);
            sel = n >= 0 ? (c.parent ? `${c.parent}.${n}` : String(n)) : c.parent;
            break;
          }
          case 'paste': if (cut) { insert(cut); cut = null; } break;
          case 'sort': {
            const lst = c ? c.list : tree;
            const key = (it) => (it.title || '').replace(/&(.)/g, '$1').toLowerCase();
            // separators split the level into groups that are sorted on their own
            let start = 0;
            for (let k = 0; k <= lst.length; k++) {
              if (k === lst.length || lst[k].kind === 'sep') {
                const part = lst.slice(start, k).sort((x, y) => key(x).localeCompare(key(y)));
                lst.splice(start, part.length, ...part);
                start = k + 1;
              }
            }
            break;
          }
          case 'here': setCmd(`cd ${quote(app.cur.path)}`); break;
          case 'there': setCmd(`cd ${quote(app.other.path)}`); break;
          case 'browse': { const d = await go.App.PickFolder('Choose a folder for the hotlist', app.cur.path); if (d) setCmd(`cd ${quote(d)}`); break; }
          case 'target': setTarget(app.other.path); break;
          case 'notarget': setTarget(''); break;
          case 'import': {
            const f = await go.TabsHotlist.ChooseFile(false, 'Import the hotlist of Total Commander (wincmd.ini)', '', 'ini');
            if (!f) break;
            const items = await go.TabsHotlist.ReadTC(f);
            tree.push(...(tree.length && items.length ? [{ kind: 'sep', title: '' }] : []), ...items);
            draw();
            err('');
            theApp.flash(`${items.length} entries were imported; OK keeps them.`);
            return;
          }
        }
        draw();
      } catch (e) { err(errText(e)); }
    });
    draw();
  }, false, { kind: 'hotlist-config' }));
}

// ---- folder history (Alt+Down, F-055, F-660) ----

const histMax = () => Math.max(5, Math.min(200, +theApp.setting('historyLength', 30) || 30));
const DIGITS = '1234567890';

async function showHistory(p, { anchor } = {}) {
  const line = [...p.back, p.path, ...[...p.fwd].reverse()];
  const curPos = p.back.length;
  // each folder once; the current folder stays where it is
  const seen = new Set();
  const rows = [];
  const key = (x) => (theApp.platform === 'linux' ? trimSep(x) : trimSep(x).toLowerCase());
  for (let i = line.length - 1; i >= 0; i--) {
    const k = key(line[i]);
    if (i !== curPos && (seen.has(k) || k === key(p.path))) continue;
    seen.add(k);
    rows.unshift({ path: line[i], pos: i });
  }
  if (theApp.setting('reverseHistory', false)) rows.reverse();
  const items = rows.map((r, n) => ({ label: r.path, key: DIGITS[n] || '', detail: r.pos === curPos ? 'current' : '', value: r, path: true }));
  items.push({ sep: true }, { label: 'Frequently used folders…', value: 'frequent' }, { label: 'Clear history', value: 'clear', disabled: rows.length < 2 }, { label: 'History settings…', value: 'settings' });
  const r = await showList({ title: 'Folder history', items, anchor: anchor ?? anchorFor(p), index: Math.max(0, rows.findIndex((x) => x.pos === curPos)), kind: 'history' });
  if (!r) return false;
  const v = r.item.value;
  if (v === 'frequent') return showFrequent(p, { anchor });
  if (v === 'settings') return historySettings();
  if (v === 'clear') { p.back = []; p.fwd = []; saveSoon(); theApp.flash('The folder history of this tab is cleared.'); return true; }
  if (v.pos === curPos) return false;
  if (r.shift) return openInNewTab(p, v.path);
  if (theApp.setting('historyAppend', true)) return p.go(v.path, { restore: true });
  // move through the history without adding to it, so Alt+Right still goes forward (HistoryAppend=0)
  const [back, fwd] = [p.back, p.fwd];
  p.back = line.slice(0, v.pos);
  p.fwd = line.slice(v.pos + 1).reverse();
  const ok = await p.go(v.path, { history: false, restore: true });
  if (!ok) { p.back = back; p.fwd = fwd; }
  return ok;
}

async function showFrequent(p, { anchor } = {}) {
  const list = await go.TabsHotlist.Frequent(20);
  const items = list.map((x, n) => ({ label: x, key: DIGITS[n] || '', value: x, path: true }));
  if (!items.length) items.push({ label: 'No folders counted yet', disabled: true });
  items.push({ sep: true }, { label: 'Clear frequently used folders', value: 'clear', disabled: !list.length });
  const r = await showList({ title: 'Frequently used folders', items, anchor: anchor ?? anchorFor(p), kind: 'frequent' });
  if (!r) return false;
  if (r.item.value === 'clear') { await go.TabsHotlist.ClearFrequent(); theApp.flash('The frequently used folders are cleared.'); return true; }
  return r.shift ? openInNewTab(p, r.item.value) : p.go(r.item.value);
}

async function historySettings() {
  const r = await formDialog('Folder history settings',
    `<div class="mc-row"><label for="hist-size">Folders kept in each tab's history (5 to 200)</label><input type="text" id="hist-size" value="${histMax()}" autofocus data-select="0,3"></div>
     <label class="mc-check"><input type="checkbox" id="hist-reverse"${theApp.setting('reverseHistory', false) ? ' checked' : ''}> Newest folder at the top of the list</label>
     <label class="mc-check"><input type="checkbox" id="hist-append"${theApp.setting('historyAppend', true) ? ' checked' : ''}> A folder chosen from the list is added at the end of the history</label>`,
    (b) => {
      const n = parseInt(b.querySelector('#hist-size').value, 10);
      if (!(n >= 5 && n <= 200)) return 'The history size must be a number from 5 to 200.';
      return { n, reverse: b.querySelector('#hist-reverse').checked, append: b.querySelector('#hist-append').checked };
    }, null, { kind: 'history-settings' });
  if (!r) return false;
  theApp.setSetting('historyLength', r.n);
  theApp.setSetting('reverseHistory', r.reverse);
  theApp.setSetting('historyAppend', r.append);
  for (const p of theApp.panels) if (p.back.length > r.n) p.back = p.back.slice(-r.n);
  saveSoon();
  return true;
}

// ---- commands ----

const panelArg = (app, args) => app.panel(args.panel || 'active');

command({
  name: 'cm_DirectoryHotlist', label: 'Folder hotlist',
  run: (app, args) => {
    const p = panelArg(app, args);
    if (args.search !== undefined) return searchHotlist(p, String(args.search));
    return showHotlist(p, { anchor: args.anchor });
  },
});
command({ name: 'cm_ConfigDirHotlist', label: 'Folder hotlist settings…', run: (app) => configHotlist(app) });
command({ name: 'cm_DirectoryHistory', label: 'Folder history', run: (app, args) => showHistory(panelArg(app, args), { anchor: args.anchor }) });
command({
  name: 'cm_FrequentlyUsedDirs', label: 'Frequently used folders', keys: ['Alt+Shift+Down'], menu: 'Commands', menuAfter: 'cm_DirectoryHistory',
  run: (app, args) => showFrequent(panelArg(app, args)),
});
command({
  name: 'cm_ClearDirHistory', label: 'Clear folder history',
  run: (app) => { const p = app.cur; p.back = []; p.fwd = []; saveSoon(); app.flash('The folder history of this tab is cleared.'); return true; },
});
command({ name: 'cm_DirHistorySettings', label: 'Folder history settings…', run: () => historySettings() });
command({ name: 'cm_ConfigSaveDirHistory', label: 'Save folder history', run: () => { saveSoon(); return true; } });

// ---- control mode (for QA) ----
addControlCommand('hotlist', async () => ({ ok: true, hotlist: await go.TabsHotlist.Hotlist() }));
addControlCommand('frequent', async () => ({ ok: true, folders: await go.TabsHotlist.Frequent(20) }));

// ---- start-up ----

export function init(app) {
  // the star button in each path bar opens the hotlist for that panel (F-657)
  for (const p of app.panels) {
    const star = document.createElement('button');
    star.type = 'button';
    star.className = 'mc-hotlist-star';
    star.textContent = '★';
    star.title = 'Folder hotlist (Ctrl+D)';
    star.setAttribute('aria-label', 'Folder hotlist');
    star.addEventListener('click', (ev) => { ev.stopPropagation(); app.setActive(p); showHotlist(p, { anchor: star }); });
    p.$.pbtns.append(star);
  }
  app.on('panel', (p, info = {}) => {
    const max = histMax();
    if (p.back.length > max) p.back = p.back.slice(-max);
    if (info.moved && p.path && !app.demo) go.TabsHotlist.Visit(p.path).catch(() => {});
  });
}
