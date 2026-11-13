// The Options dialog (cm_Config, Cmd+, on macOS; FEATURES.md section 17): a list of pages on the left, the options of
// the chosen page on the right, a search box over all of them (F-827), and OK / Apply / Cancel (F-412). The pages and
// options come from internal/config (Go's Configuration.Schema), so the dialog grows with the engine. Pages of our own:
// Keyboard shortcuts (redefine hotkeys, F-431), User commands (em_ commands, F-311) and Settings file (where it is,
// edit it, import Total Commander's wincmd.ini, back up and restore: F-443 to F-447, F-826). The Colors page also edits
// the colours themselves and the colours by file type (F-417, F-435).
//
// Every option is a field whose id is its "Page.Field" name, so control mode reads the whole dialog with "dialog" and
// sets any option with dialog_answer: {"button": "OK", "fields": {"Display.ShowHidden": true}}.

import { command, hasCommand, commandList, getCommand, setupKeys, userKeymap } from '../../core/registry.js';
import { DEFAULT_KEYS, MAC_KEYS } from '../../core/keymap.js';
import { modal, messageDialog, inputDialog, esc } from '../../core/dialogs.js';
import { normalizeCombo } from '../../core/keys.js';
import { track } from '../../core/busy.js';
import { go, errText } from '../../core/go.js';
import { LABELS } from '../../core/app.js';
import { refreshMenu } from '../../core/menus.js';
import { renderFkeys } from '../../core/fkeys.js';
import { state, loadSchema, loadValues, currentValue, BRIDGE, fillSelect } from './util.js';
import { keyText } from './keys.js';
import { applyAll, loadColors } from './layout.js';
import { registerUserCommands } from './usercmds.js';

const EXTRA_PAGES = {
  Hotkeys: { title: 'Keyboard shortcuts', after: 'Misc', words: 'keys keyboard shortcuts hotkeys redefine shortcut key' },
  UserCommands: { title: 'User commands', after: 'Hotkeys', words: 'user commands em_ usercmd' },
  SettingsFile: { title: 'Settings file', after: null, words: 'settings file ini import export back up backup restore edit total commander wincmd portable reload' },
};

const NOTES = {
  Language: 'mormiCommander speaks English for now. Translations will come as language files; the choice here is kept for them.',
  Layout: 'The panel widths change with the line between the panels (double-click it for half and half).',
  Plugins: 'Total Commander plugins are Windows programs and do not work here. mormiCommander has its packers and viewers built in, and can use these programs for more.',
  Hotlist: 'The folders of the hotlist itself are changed with Ctrl+D > Configure.',
  CustomColumns: 'The column sets themselves are changed from Show > Custom columns.',
};

const COLOR_KEYS = [
  ['fore', 'Text'], ['back', 'Background'], ['back2', 'Second line background'], ['mark', 'Selected files'], ['cursor', 'Cursor'],
  ['cursorText', 'Text under the cursor'], ['inactiveCursor', 'Cursor in the inactive panel'], ['inactiveMark', 'Selected files in the inactive panel'],
  ['inactiveBack', 'Background of the inactive panel'],
];

const ICONIC = [{ value: '0', label: 'Normal window' }, { value: '1', label: 'Minimized' }, { value: '-1', label: 'Maximized' }];

// ---- keymap helpers ----
export function defaultKeys(platform) {
  const m = new Map();
  const put = (k, v) => m.set(normalizeCombo(k), v);
  for (const [k, v] of Object.entries(DEFAULT_KEYS)) put(k, v);
  if (platform === 'darwin') for (const [k, v] of Object.entries(MAC_KEYS)) put(k, v);
  for (const c of commandList()) for (const k of c.keys || []) if (!m.has(normalizeCombo(k))) put(k, c.name);
  return m;
}

function effectiveKeys(platform, pending) {
  const m = defaultKeys(platform);
  for (const [k, v] of Object.entries(pending)) { if (v) m.set(k, v); else m.delete(k); }
  return m;
}

// saveKeymap puts a new set of key changes to work and saves it (settings.json for the core, the ini file's [Shortcuts]).
export async function saveKeymap(app, pending) {
  setupKeys(app.platform, pending);
  await app.setSetting('keymap', { ...pending });
  await go.Configuration.SetKeymap(pending).catch((e) => app.flash(errText(e), 'err'));
  refreshMenu();
  renderFkeys();
}

export function allCommandNames(platform) {
  const s = new Set([...commandList().map((c) => c.name), ...Object.keys(LABELS), ...defaultKeys(platform).values()]);
  return [...s].filter((n) => /^(cm|em)_/.test(n)).sort((a, b) => a.localeCompare(b));
}
export const commandLabel = (n) => (getCommand(n)?.label || LABELS[n] || n).replace(/…$/, '');

// ---- the dialog ----
function fieldHTML(f, value) {
  const id = esc(f.name);
  const name = f.name.split('.').pop();
  if (f.type === 'bool') {
    return `<label class="mc-check mc-config-field" data-field="${id}"><input type="checkbox" id="${id}"${value === '1' ? ' checked' : ''}> ${esc(f.label)}</label>`;
  }
  let input;
  if (f.options?.length) {
    input = `<select id="${id}">${f.options.map((o) => `<option value="${o.value}"${String(o.value) === value ? ' selected' : ''}>${esc(o.label)}</option>`).join('')}</select>`;
  } else if (f.type === 'int') {
    input = `<input type="number" id="${id}" value="${esc(value)}"${f.min != null ? ` min="${f.min}"` : ''}${f.max != null ? ` max="${f.max}"` : ''}>`;
  } else {
    input = `<input type="text" id="${id}" spellcheck="false" value="${esc(value)}" data-name="${esc(name)}">`;
  }
  return `<div class="mc-row mc-config-field" data-field="${id}"><label for="${id}">${esc(f.label)}</label>${input}</div>`;
}

function colorsHTML() {
  return `<h4>Colours</h4>
    <div class="mc-config-inline"><div class="mc-row"><label for="col-which">Colours for</label><select id="col-which"><option value="light">the light theme</option><option value="dark">the dark theme</option></select></div>
    <div class="mc-row"><label for="col-preset">Start from</label><select id="col-preset"></select></div><button type="button" class="col-use">Use these colours</button></div>
    <p class="mc-note">Colours that are not ticked follow the theme.</p>
    <div class="mc-config-colorgrid">${COLOR_KEYS.map(([k, l]) => `<label class="mc-check"><input type="checkbox" id="col-${k}-on"> ${esc(l)}</label><input type="color" id="col-${k}" aria-label="${esc(l)}">`).join('')}</div>
    <h4>Colours by file type</h4>
    <div class="mc-config-listedit"><select id="colf-list" size="6" aria-label="Colours by file type"></select>
      <div class="mc-config-listfields"><div class="mc-row"><label for="colf-mask">Files (for example *.zip *.7z, or folders: *\\)</label><input type="text" id="colf-mask" spellcheck="false"></div>
      <div class="mc-row"><label for="colf-color">Colour</label><input type="color" id="colf-color"></div>
      <div class="mc-config-rowbtns"><button type="button" class="colf-add">Add type colour</button><button type="button" class="colf-del">Remove type colour</button></div></div></div>`;
}

function hotkeysHTML() {
  return `<div class="mc-row"><label for="hk-filter">Find a command (name, words or key)</label><input type="text" id="hk-filter" spellcheck="false"></div>
    <div class="mc-config-listedit"><select id="hk-list" size="14" aria-label="Commands"></select>
      <div class="mc-config-listfields"><p class="mc-note" id="hk-info"></p>
      <div class="mc-row"><label for="hk-keys">Keys of this command</label><select id="hk-keys" size="4"></select></div>
      <div class="mc-row"><label for="hk-key">New key: click here and press it</label><input type="text" id="hk-key" data-keycapture="1" spellcheck="false" placeholder="Press a key"></div>
      <p class="mc-note" id="hk-conflict"></p>
      <div class="mc-config-rowbtns"><button type="button" class="hk-assign">Assign key</button><button type="button" class="hk-remove">Remove key</button><button type="button" class="hk-restore">Restore default keys</button></div></div></div>`;
}

function userCmdsHTML() {
  return `<p class="mc-note">User commands (em_name) can be put on buttons, in the Start menu and on keys like any command. They are kept in usercmd.ini.</p>
    <div class="mc-config-listedit"><select id="uc-list" size="10" aria-label="User commands"></select>
      <div class="mc-config-listfields">
      <div class="mc-row"><label for="uc-name">Name (em_…)</label><input type="text" id="uc-name" spellcheck="false"></div>
      <div class="mc-row"><label for="uc-menu">Title in menus and tooltips</label><input type="text" id="uc-menu"></div>
      <div class="mc-row"><label for="uc-cmd">Command (program, cm_ command, cd folder)</label><input type="text" id="uc-cmd" spellcheck="false" list="mc-config-cmds"></div>
      <div class="mc-row"><label for="uc-params">Parameters (%P path, %N name, %S selected names, ? to ask first)</label><input type="text" id="uc-params" spellcheck="false"></div>
      <div class="mc-row"><label for="uc-path">Start folder</label><input type="text" id="uc-path" spellcheck="false"></div>
      <div class="mc-row"><label for="uc-iconic">Run</label><select id="uc-iconic">${ICONIC.map((o) => `<option value="${o.value}">${o.label}</option>`).join('')}</select></div>
      <div class="mc-config-rowbtns"><button type="button" class="uc-new">New command</button><button type="button" class="uc-del">Delete command</button></div></div></div>`;
}

function settingsFileHTML(info) {
  const where = info.override ? 'given on the command line (-I=)' : info.portable ? 'next to the program (portable)' : 'in your settings folder';
  const warn = (info.warnings || []).length ? `<p class="mc-note">Problems found in the file: ${esc(info.warnings.join('; '))}</p>` : '';
  return `<p class="mc-msg">The settings are kept in <b class="mc-config-path">${esc(info.ini || '')}</b>, ${where}. It is a plain text file in the style of Total Commander's wincmd.ini.${info.exists ? '' : ' (It is written the first time a setting changes.)'}</p>${warn}
    <div class="mc-config-rowbtns mc-config-col">
    <button type="button" class="sf-edit">Edit settings file</button><button type="button" class="sf-reload">Read settings file again</button>
    <button type="button" class="sf-import">Import Total Commander settings…</button><button type="button" class="sf-export">Back up settings…</button><button type="button" class="sf-restore">Restore from backup…</button></div>
    <p class="mc-note">Starting with <code>-I=file</code> uses another settings file; a settings file named mormicommander.ini next to the program makes the program portable.</p>`;
}

// openOptions shows the dialog; page: the page to show first.
export async function openOptions(app, page = '') {
  for (let again = page || 'Layout'; again;) again = await showDialog(app, again);
  return true;
}

async function showDialog(app, startPage) {
  const schema = await loadSchema();
  await loadValues();
  await loadColors();
  const [colorsData, info, ucList] = await Promise.all([
    go.Configuration.Colors().catch(() => null), go.Configuration.Info().catch(() => ({})), go.ConfigBar.UserCommands().catch(() => []),
  ]);
  const pages = [...schema.pages];
  for (const [id, x] of Object.entries(EXTRA_PAGES)) {
    const at = x.after ? pages.findIndex((p) => p.id === x.after) : -1;
    if (at >= 0) pages.splice(at + 1, 0, { id, title: x.title }); else pages.push({ id, title: x.title });
  }
  const initial = {};
  for (const f of schema.fields) initial[f.name] = currentValue(app, f.name);
  let pendingKeys = { ...userKeymap() };
  let keysDirty = false;
  const pendingColors = colorsData ? JSON.parse(JSON.stringify({ light: colorsData.light, dark: colorsData.dark })) : null;
  let colorsDirty = false;
  let ucs = (ucList || []).map((u) => ({ ...u }));
  let ucDirty = false;

  return modal((box, done) => {
    box.classList.add('mc-config-box');
    const byPage = (id) => schema.fields.filter((f) => f.pageId === id);
    const sections = pages.map((p) => {
      let body = NOTES[p.id] ? `<p class="mc-note">${esc(NOTES[p.id])}</p>` : '';
      if (p.id === 'Hotkeys') body += hotkeysHTML();
      else if (p.id === 'UserCommands') body += userCmdsHTML();
      else if (p.id === 'SettingsFile') body += settingsFileHTML(info || {});
      else body += byPage(p.id).map((f) => fieldHTML(f, initial[f.name])).join('');
      if (p.id === 'Colors' && pendingColors) body += colorsHTML();
      return `<section class="mc-config-page" data-page="${esc(p.id)}" hidden><h3>${esc(p.title)}</h3>${body}</section>`;
    }).join('');
    box.innerHTML = `<h2>Options</h2>
      <datalist id="mc-config-cmds">${allCommandNames(app.platform).map((n) => `<option value="${esc(n)}">${esc(commandLabel(n))}</option>`).join('')}</datalist>
      <div class="mc-config">
        <div class="mc-config-side"><input type="text" id="cfg-search" placeholder="Search options" spellcheck="false" aria-label="Search options">
          <select id="cfg-page" size="20" aria-label="Pages">${pages.map((p) => `<option value="${esc(p.id)}">${esc(p.title)}</option>`).join('')}</select></div>
        <div class="mc-config-main">${sections}</div>
      </div>
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="button" class="apply">Apply</button><button type="submit" class="primary">OK</button></div>`;
    const $ = (s) => box.querySelector(s);
    const err = $('.mc-err');
    const pageSel = $('#cfg-page');
    const show = (id) => {
      if (!pages.some((p) => p.id === id)) id = pages[0].id;
      pageSel.value = id;
      box.querySelectorAll('.mc-config-page').forEach((s) => { s.hidden = s.dataset.page !== id; });
      $('.mc-config-main').scrollTop = 0;
    };
    pageSel.addEventListener('change', () => show(pageSel.value));
    show(startPage);

    // search (F-827)
    $('#cfg-search').addEventListener('input', async (ev) => {
      const q = ev.target.value.trim();
      box.querySelectorAll('.mc-config-hit').forEach((e) => e.classList.remove('mc-config-hit'));
      if (!q) { [...pageSel.options].forEach((o) => { o.hidden = false; }); return; }
      const r = await go.Configuration.Search(q).catch(() => ({ pages: [], fields: [] }));
      if (ev.target.value.trim() !== q) return;
      const words = q.toLowerCase().split(/\s+/);
      const hitPages = new Set(r.pages || []);
      for (const [id, x] of Object.entries(EXTRA_PAGES)) if (words.every((w) => (x.title + ' ' + x.words).toLowerCase().includes(w))) hitPages.add(id);
      for (const n of r.fields || []) box.querySelector(`.mc-config-field[data-field="${CSS.escape(n)}"]`)?.classList.add('mc-config-hit');
      [...pageSel.options].forEach((o) => { o.hidden = !hitPages.has(o.value); });
      const first = pages.find((p) => hitPages.has(p.id));
      if (first && !hitPages.has(pageSel.value)) show(first.id);
    });

    // ---- Colors page ----
    if (pendingColors) {
      const which = () => $('#col-which').value;
      const preset = $('#col-preset');
      preset.innerHTML = '<option value="">Theme colours</option>' + (colorsData.builtin || []).map((s, i) => `<option value="${i}">${esc(s.name)}</option>`).join('');
      const paint = () => {
        const s = pendingColors[which()];
        for (const [k] of COLOR_KEYS) {
          const v = s.colors[k] || '';
          $(`#col-${k}-on`).checked = !!v;
          $(`#col-${k}`).value = v || '#808080';
          $(`#col-${k}`).disabled = !v;
        }
        fillSelect($('#colf-list'), s.filters.map((f, i) => ({ value: i, label: `${f.mask}  ${f.color || ''}` })));
        paintFilter();
      };
      const paintFilter = () => {
        const f = pendingColors[which()].filters[+$('#colf-list').value];
        $('#colf-mask').value = f?.mask || '';
        $('#colf-color').value = f?.color || '#3399ff';
      };
      for (const [k] of COLOR_KEYS) {
        const sync = () => {
          const onEl = $(`#col-${k}-on`);
          $(`#col-${k}`).disabled = !onEl.checked;
          pendingColors[which()].colors[k] = onEl.checked ? $(`#col-${k}`).value : '';
          colorsDirty = true;
        };
        $(`#col-${k}-on`).addEventListener('change', sync);
        $(`#col-${k}`).addEventListener('input', sync);
      }
      $('#col-which').addEventListener('change', paint);
      $('.col-use').onclick = () => {
        const s = pendingColors[which()];
        const p = preset.value === '' ? null : colorsData.builtin[+preset.value];
        for (const [k] of COLOR_KEYS) s.colors[k] = p ? p.colors[k] || '' : '';
        colorsDirty = true;
        paint();
      };
      $('#colf-list').addEventListener('change', paintFilter);
      const editFilter = () => {
        const f = pendingColors[which()].filters[+$('#colf-list').value];
        if (!f) return;
        f.mask = $('#colf-mask').value;
        f.color = $('#colf-color').value;
        const o = $('#colf-list').selectedOptions[0];
        if (o) o.textContent = `${f.mask}  ${f.color}`;
        colorsDirty = true;
      };
      $('#colf-mask').addEventListener('input', editFilter);
      $('#colf-color').addEventListener('input', editFilter);
      $('.colf-add').onclick = () => {
        const list = pendingColors[which()].filters;
        list.push({ mask: $('#colf-mask').value.trim() || '*.txt', color: $('#colf-color').value });
        colorsDirty = true;
        paint();
        $('#colf-list').value = String(list.length - 1);
        paintFilter();
      };
      $('.colf-del').onclick = () => {
        const list = pendingColors[which()].filters;
        const i = +$('#colf-list').value;
        if (!(i >= 0 && i < list.length)) return;
        list.splice(i, 1);
        colorsDirty = true;
        paint();
      };
      $('#col-which').value = document.documentElement.dataset.theme === 'light' || (!document.documentElement.dataset.theme && window.matchMedia?.('(prefers-color-scheme: light)').matches) ? 'light' : 'dark';
      paint();
    }

    // ---- Keyboard shortcuts page ----
    const names = allCommandNames(app.platform);
    const hkList = $('#hk-list');
    const paintHk = () => {
      const eff = effectiveKeys(app.platform, pendingKeys);
      const keysOf = new Map();
      for (const [k, v] of eff) { if (!keysOf.has(v)) keysOf.set(v, []); keysOf.get(v).push(k); }
      const q = $('#hk-filter').value.trim().toLowerCase();
      const items = names.filter((n) => {
        if (!q) return true;
        const hay = `${n} ${commandLabel(n)} ${(keysOf.get(n) || []).join(' ')}`.toLowerCase();
        return q.split(/\s+/).every((w) => hay.includes(w));
      }).map((n) => ({ value: n, label: `${commandLabel(n)} — ${n}${keysOf.get(n)?.length ? '   [' + keysOf.get(n).map(keyText).join(', ') + ']' : ''}` }));
      fillSelect(hkList, items);
      if (!hkList.value && items.length) hkList.value = items[0].value;
      const cur = hkList.value;
      fillSelect($('#hk-keys'), (keysOf.get(cur) || []).map((k) => ({ value: k, label: keyText(k) })));
      $('#hk-info').textContent = cur ? `${commandLabel(cur)} (${cur})` : '';
      conflict();
    };
    const conflict = () => {
      const k = normalizeCombo($('#hk-key').value);
      const eff = effectiveKeys(app.platform, pendingKeys);
      const used = k && eff.get(k);
      $('#hk-conflict').textContent = used && used !== hkList.value ? `${keyText(k)} now runs ${commandLabel(used)}; assigning it takes it from there.` : '';
    };
    $('#hk-filter').addEventListener('input', paintHk);
    hkList.addEventListener('change', paintHk);
    $('#hk-key').addEventListener('input', conflict);
    $('.hk-assign').onclick = () => {
      const k = normalizeCombo($('#hk-key').value);
      const cmd = hkList.value;
      if (!k || !cmd) { err.hidden = false; err.textContent = 'Choose a command and press the new key in the key field.'; return; }
      err.hidden = true;
      pendingKeys[k] = cmd;
      keysDirty = true;
      $('#hk-key').value = '';
      paintHk();
    };
    $('.hk-remove').onclick = () => {
      const k = $('#hk-keys').value;
      if (!k) return;
      if (defaultKeys(app.platform).has(k)) pendingKeys[k] = ''; else delete pendingKeys[k];
      keysDirty = true;
      paintHk();
    };
    $('.hk-restore').onclick = () => { pendingKeys = {}; keysDirty = true; paintHk(); };
    paintHk();

    // ---- User commands page ----
    const ucSel = $('#uc-list');
    const ucFields = ['name', 'menu', 'cmd', 'params', 'path', 'iconic'];
    const paintUc = (keep = true) => {
      fillSelect(ucSel, ucs.map((u, i) => ({ value: i, label: `${u.name}${u.menu ? ' — ' + u.menu : ''}` })), keep);
      if (!ucSel.value && ucs.length) ucSel.value = '0';
      const u = ucs[+ucSel.value];
      for (const f of ucFields) { const el = $(`#uc-${f}`); el.value = u ? String(u[f] ?? '') : ''; el.disabled = !u; }
    };
    ucSel.addEventListener('change', () => paintUc());
    for (const f of ucFields) {
      $(`#uc-${f}`).addEventListener('input', () => {
        const u = ucs[+ucSel.value];
        if (!u) return;
        u[f] = f === 'iconic' ? parseInt($(`#uc-${f}`).value, 10) || 0 : $(`#uc-${f}`).value;
        ucDirty = true;
        const o = ucSel.selectedOptions[0];
        if (o) o.textContent = `${u.name}${u.menu ? ' — ' + u.menu : ''}`;
      });
    }
    $('.uc-new').onclick = () => {
      let n = 1;
      while (ucs.some((u) => u.name.toLowerCase() === `em_command${n}`)) n++;
      ucs.push({ name: `em_command${n}`, menu: '', cmd: '', params: '', path: '', icon: '', iconic: 0 });
      ucDirty = true;
      paintUc(false);
      ucSel.value = String(ucs.length - 1);
      paintUc();
      $('#uc-name').focus();
    };
    $('.uc-del').onclick = () => {
      const i = +ucSel.value;
      if (!(i >= 0 && i < ucs.length)) return;
      ucs.splice(i, 1);
      ucDirty = true;
      paintUc(false);
    };
    paintUc();

    // ---- Settings file page ----
    const fail = (e) => { err.hidden = false; err.textContent = errText(e); };
    const choose = async (title, save, name) => {
      const picked = await go.Configuration.ChooseFile(title, save, name, '*.ini').catch(() => '');
      return inputDialog({ title, label: 'File', value: picked || (save ? `${app.home}${app.sep}${name}` : ''), okLabel: save ? 'Save' : 'Open', kind: 'config-file' });
    };
    $('.sf-edit').onclick = async () => {
      try { const p = await go.Configuration.EditFile(); app.flash(`Editing ${p}. Saved changes are read in at once.`); } catch (e) { fail(e); }
    };
    $('.sf-reload').onclick = async () => {
      try { await go.Configuration.Reload(); done('SettingsFile'); } catch (e) { fail(e); }
    };
    $('.sf-import').onclick = async () => {
      const path = await choose('Import Total Commander settings', false, 'wincmd.ini');
      if (!path) return;
      const how = await messageDialog({ Title: 'Import the folder hotlist', Message: 'Add the imported hotlist to yours (in a submenu), or replace yours with it?', Buttons: ['Add', 'Replace', 'Cancel'], Kind: 'config-import' });
      if (how === 'Cancel') return;
      try {
        const r = await go.Configuration.ImportTC(path, how === 'Replace');
        const lines = [...(r.imported || [])];
        if (r.warnings?.length) lines.push('', 'Notes:', ...r.warnings);
        await messageDialog({ Title: 'Settings imported', Message: lines.join('\n') || 'Nothing to import was found.', Kind: 'config-imported' });
        done('SettingsFile');
      } catch (e) { fail(e); }
    };
    $('.sf-export').onclick = async () => {
      const path = await choose('Back up settings', true, 'mormicommander-backup.ini');
      if (!path) return;
      try { await go.Configuration.Export(path); app.flash(`The settings were saved to ${path}.`); } catch (e) { fail(e); }
    };
    $('.sf-restore').onclick = async () => {
      const path = await choose('Restore settings from a backup', false, 'mormicommander-backup.ini');
      if (!path) return;
      const ok = await messageDialog({ Title: 'Restore settings?', Message: `All settings are replaced by those in ${path}.`, Buttons: ['Restore', 'Cancel'], Kind: 'config-restore' });
      if (ok !== 'Restore') return;
      try { await go.Configuration.Restore(path); done('SettingsFile'); } catch (e) { fail(e); }
    };

    // ---- OK / Apply ----
    const collect = () => {
      const changes = {};
      for (const f of schema.fields) {
        const el = box.querySelector(`[id="${CSS.escape(f.name)}"]`);
        if (!el) continue;
        let v = el.type === 'checkbox' ? (el.checked ? '1' : '0') : String(el.value);
        if (f.type === 'int') {
          let n = parseInt(v, 10);
          if (!Number.isFinite(n)) n = parseInt(initial[f.name], 10) || 0;
          if (f.min != null) n = Math.max(f.min, n);
          if (f.max != null) n = Math.min(f.max, n);
          v = String(n);
        }
        if (v !== initial[f.name]) changes[f.name] = v;
      }
      return changes;
    };
    let saving = false;
    const save = async () => {
      if (saving) return false;
      saving = true;
      err.hidden = true;
      try {
        const changes = collect();
        if (ucDirty) {
          const bad = ucs.find((u) => !/^em_[A-Za-z0-9_]+$/.test(u.name));
          if (bad) { show('UserCommands'); throw new Error(`"${bad.name}" is not a user command name: it starts with em_ and has letters, digits and _ only.`); }
        }
        // write the options the file keeps even when a feature also keeps them (so the file is complete)
        const changed = Object.keys(changes).length ? await go.Configuration.Apply(changes) : [];
        for (const n of Object.keys(changes)) { await BRIDGE[n]?.set(app, changes[n]); initial[n] = changes[n]; }
        if (keysDirty) { await saveKeymap(app, pendingKeys); keysDirty = false; }
        if (colorsDirty && pendingColors) { await go.Configuration.SetColors(pendingColors.light, pendingColors.dark); colorsDirty = false; }
        if (ucDirty) { await go.ConfigBar.SaveUserCommands(ucs); ucDirty = false; await registerUserCommands(app); }
        await loadValues(app);
        await loadColors();
        applyAll(app.startTheme);
        app.emit('config', changed);
        app.emit('settings');
        return true;
      } catch (e) {
        fail(e);
        return false;
      } finally {
        saving = false;
      }
    };
    $('.cancel').onclick = () => done(null);
    $('.apply').onclick = () => track(save());
    box.onsubmit = (ev) => { ev.preventDefault(); track(save().then((ok) => { if (ok) done(null); })); };
  }, null, { kind: 'config' });
}

// ---- commands ----
command({ name: 'cm_Config', label: 'Options…', run: (app, args) => openOptions(app, args?.page || '') });
command({ name: 'cm_ConfigHotkeys', label: 'Keyboard shortcuts…', run: (app) => openOptions(app, 'Hotkeys') });
command({
  name: 'cm_ConfigSaveSettings', label: 'Save settings',
  run: async (app) => { await go.Configuration.Save(); app.saveSession(); app.flash('The settings were saved.'); return true; },
});
command({
  name: 'cm_ConfigSavePos', label: 'Save position',
  run: async (app) => { await go.Configuration.SavePosition(); app.saveSession(); app.flash('The window position and the panel widths were saved.'); return true; },
});
command({
  name: 'cm_ConfigChangeIniFiles', label: 'Edit settings file',
  run: async (app) => { const p = await go.Configuration.EditFile(); app.flash(`Editing ${p}. Saved changes are read in at once.`); return true; },
});

export function initOptions(app) {
  // the hotlist feature may bring its own settings dialog; otherwise its page of the Options dialog serves
}
