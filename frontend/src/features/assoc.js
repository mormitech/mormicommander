// Internal associations (Files > Internal associations…, cm_InternalAssociate, F-323): file masks mapped to programs or
// cm_* commands inside mormiCommander, independent of the system. The setting "internalAssociations" holds one
// "mask list = command" per line ("*.txt *.md = code --wait "%1"", "*.log = cm_List"); Enter on a matching file uses the
// first matching line before the system's app (system.js asks openAssociated first). Indented lines below an entry add
// to it: "view = command" for F3, "edit = command" for F4, "icon = picture file or a character" for the panel icon of
// matching files, "desc = text":
//
//   *.txt *.md = code --wait "%1"
//     view = cm_List
//     edit = myedit "%1"
//     icon = 📝
//
// Go parses and runs it (SystemLaunch.OpenInternalVerb over internal/launch: TC masks, %1 and the other placeholders).
// Empty, the default, leaves Enter, F3, F4 and the icons exactly as they were, and then Go is not even asked. Old
// one-line entries are read as they always were (only the open verb).

import { command } from '../core/registry.js';
import { go } from '../core/go.js';
import { formDialog, esc } from '../core/dialogs.js';
import { Panel } from '../core/panel.js';
import { realCommand } from './config/run.js';
import { compileMask } from './config/util.js';

export const ASSOC_KEY = 'internalAssociations';

const assocText = (app) => String(app.setting(ASSOC_KEY, '') || '');

const FIELDS = ['open', 'view', 'edit', 'icon', 'desc'];

// parseAssoc reads the setting into entries {masks, open, view, edit, icon, desc}, as Go's ParseAssociationLines does
// (comments and lines without "=" are skipped; entries are kept even when empty, so the dialog can show them).
export function parseAssoc(text) {
  const out = [];
  let last = null;
  for (const raw of String(text || '').split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#') || line.startsWith(';')) continue;
    const at = line.indexOf('=');
    if (at < 0) continue;
    const key = line.slice(0, at).trim(), value = line.slice(at + 1).trim();
    if (!key) continue;
    const lk = key.toLowerCase();
    if (/^[ \t]/.test(raw) && last && FIELDS.includes(lk)) { last[lk] = value; continue; }
    last = { masks: key, open: value, view: '', edit: '', icon: '', desc: '' };
    out.push(last);
  }
  return out;
}

// serializeAssoc writes entries back: the old one-line form when an entry has only masks and a command for Enter, so
// a list without view/edit/icon is saved exactly as the old dialog saved it.
export function serializeAssoc(entries) {
  const lines = [];
  for (const e of entries) {
    const masks = String(e.masks || '').trim();
    if (!masks) continue;
    lines.push(`${masks} = ${String(e.open || '').trim()}`.trimEnd());
    for (const k of ['view', 'edit', 'icon', 'desc']) {
      const v = String(e[k] || '').trim().replace(/\n/g, ' ');
      if (v) lines.push(`  ${k} = ${v}`);
    }
  }
  return lines.length ? `${lines.join('\n')}\n` : '';
}

const hasVerb = (app, verb) => parseAssoc(assocText(app)).some((e) => e[verb]);

// runAssociation asks Go to carry out the association for verb on path; true when one matched. An internal command
// runs here, told not to look at the associations again (so "view = cm_List" opens the built-in viewer).
async function runAssociation(app, verb, name, path, dir) {
  const a = verb === 'open' ? await go.SystemLaunch.OpenInternal(path, dir) : await go.SystemLaunch.OpenInternalVerb(verb, path, dir);
  if (!a?.handled) return false;
  if (a.internal) {
    const args = verb === 'open' ? (a.args ? { params: a.args } : undefined) : { ...(a.args ? { params: a.args } : {}), noAssoc: true };
    await app.run(realCommand(a.internal), args);
  } else app.flash(`Opened ${name}.`);
  return true;
}

// openAssociated applies the internal associations to Enter on a file; true when one matched and was carried out
// (errors are thrown to the caller, which shows them like any other open error).
export async function openAssociated(app, e, path, dir) {
  if (!assocText(app).trim()) return false;
  return runAssociation(app, 'open', e.name, path, dir);
}

const isLocal = (p) => !!p && !/^[a-z][a-z0-9+.-]*:\/\//i.test(p);

// viewEditAssociated is F3 (verb "view") or F4 ("edit") on a local file: the association's command when an entry with
// that verb matches; false otherwise (and always when no entry has the verb: then Go is not asked), so the built-in
// viewer and the configured editor work as before.
export async function viewEditAssociated(app, verb, path, dir, args = {}) {
  if (args.noAssoc || !isLocal(path) || !hasVerb(app, verb)) return false;
  return runAssociation(app, verb, path.split(/[\\/]/).pop(), path, dir);
}

// ---- icons ----

let iconRules = null; // [{test, icon}] for the entries with an icon, rebuilt when the setting changes
let iconsFor = null; // the setting text iconRules was made from
const iconFiles = new Map(); // picture path → data URL ('' when it can't be used)
let iconAsk = 0;

const isIconFile = (s) => /[\\/]/.test(s);

function rules(app) {
  const text = assocText(app);
  if (text !== iconsFor) {
    iconsFor = text;
    iconRules = text.trim() ? parseAssoc(text).filter((e) => e.icon).map((e) => ({ test: compileMask(e.masks), icon: e.icon })) : [];
  }
  return iconRules;
}

// assocIcon is the icon of a file whose entry names one: a picture file (read by Go) or a short text such as an emoji;
// '' for every other row (the list's usual icon).
export function assocIcon(app, p, e) {
  if (!e || e.up || e.dir) return '';
  const list = rules(app);
  if (!list.length) return '';
  const r = list.find((x) => x.test(e.name, false));
  if (!r) return '';
  if (!isIconFile(r.icon)) return `<i class="ic mc-assoc-ic" aria-hidden="true">${esc([...r.icon].slice(0, 2).join(''))}</i>`;
  const url = iconFiles.get(r.icon);
  if (url === undefined) {
    iconFiles.set(r.icon, null);
    clearTimeout(iconAsk);
    iconAsk = setTimeout(async () => {
      const want = [...iconFiles].filter(([, v]) => v === null).map(([k]) => k);
      let got = {};
      try { got = await go.Icons.FromFiles(want) || {}; } catch { got = {}; }
      for (const k of want) iconFiles.set(k, got[k] || '');
      for (const pn of app.panels || []) pn.renderRows?.();
    }, 20);
    return '';
  }
  return url ? `<img class="ic" src="${esc(url)}" alt="" draggable="false">` : '';
}

// ---- the dialog ----

const blank = () => ({ masks: '', open: '', view: '', edit: '', icon: '', desc: '' });
const entryLabel = (e) => `${e.masks || '(no files)'}${e.desc ? ` – ${e.desc}` : ''}`;

// internalAssociations is the dialog: the entries in a list (add, change, remove, move), each with its files, the
// commands for Enter, F3 and F4 and an icon; "As text" shows the setting itself, which is what is saved (so notes in
// the text stay as long as the list is not changed). Blank text clears the associations.
export async function internalAssociations(app) {
  const text = assocText(app);
  const field = (id, label, ph = '') => `<div class="mc-row"><label for="as-${id}">${label}</label><input type="text" id="as-${id}" spellcheck="false" placeholder="${esc(ph)}"></div>`;
  const r = await formDialog('Internal associations',
    `<p class="mc-note">Files matching an entry open with its command instead of the system's app (Enter), the built-in viewer (F3) or the editor (F4); the first matching entry wins, and an empty command keeps the usual one. A command is a program with its parameters (%1 is the file) or an internal command (cm_…).</p>
    <div class="mc-config-listedit"><select id="as-list" size="9" aria-label="Associations"></select>
      <div class="mc-config-listfields">
        ${field('masks', 'Files (masks, e.g. *.txt *.md)', '*.txt *.md')}
        ${field('open', 'Open (Enter)', 'code --wait "%1"')}
        ${field('view', 'View (F3)', 'cm_List')}
        ${field('edit', 'Edit (F4)', 'myeditor "%1"')}
        ${field('icon', 'Icon (a picture file, or a character such as 📝)', '')}
        ${field('desc', 'Description', '')}
        <div class="mc-config-rowbtns"><button type="button" class="as-add">Add</button><button type="button" class="as-del">Remove</button><button type="button" class="as-up">Move up</button><button type="button" class="as-down">Move down</button></div>
      </div></div>
    <details class="as-text"><summary>As text</summary>
    <div class="mc-row"><label for="assoc">One entry per line ("files = command"), indented "view =", "edit =", "icon =" lines below it; lines starting with # are notes</label><textarea id="assoc" rows="8" spellcheck="false" placeholder="*.txt *.md = code --wait &quot;%1&quot;&#10;  view = cm_List&#10;*.log = cm_List">${esc(text)}</textarea></div></details>`,
    (box) => ({ text: box.querySelector('#assoc').value }), (box) => wireDialog(box), { kind: 'internal-associations' });
  if (r === null) return false;
  const v = r.text.trim() ? r.text : '';
  await app.setSetting(ASSOC_KEY, v);
  iconsFor = null;
  for (const p of app.panels || []) p.renderRows?.();
  app.flash(v ? 'Internal associations saved.' : 'No internal associations: Enter, F3 and F4 work as usual.');
  return true;
}

// wireDialog ties the list and the fields to the text box: changing the list writes the text again; typing in the text
// reads the list again.
export function wireDialog(box) {
  const $ = (s) => box.querySelector(s);
  const list = $('#as-list'), ta = $('#assoc');
  let entries = parseAssoc(ta.value);
  let at = entries.length ? 0 : -1;
  const fill = () => {
    list.innerHTML = entries.map((e, i) => `<option value="${i}"${i === at ? ' selected' : ''}>${esc(entryLabel(e))}</option>`).join('');
    const e = entries[at];
    for (const k of ['masks', ...FIELDS]) { const f = $(`#as-${k}`); f.value = e ? e[k] : ''; f.disabled = !e; }
    $('.as-del').disabled = !e; $('.as-up').disabled = at <= 0; $('.as-down').disabled = at < 0 || at >= entries.length - 1;
  };
  const write = () => { ta.value = serializeAssoc(entries); };
  list.addEventListener('change', () => { at = +list.value; fill(); });
  for (const k of ['masks', ...FIELDS]) {
    $(`#as-${k}`).addEventListener('input', (ev) => {
      if (!entries[at]) return;
      entries[at][k] = ev.target.value;
      if (list.options[at]) list.options[at].textContent = entryLabel(entries[at]);
      write();
    });
  }
  $('.as-add').addEventListener('click', () => { entries.splice(at + 1, 0, blank()); at += 1; fill(); write(); $('#as-masks').focus(); });
  $('.as-del').addEventListener('click', () => { if (at < 0) return; entries.splice(at, 1); at = Math.min(at, entries.length - 1); fill(); write(); });
  const move = (d) => { const j = at + d; if (at < 0 || j < 0 || j >= entries.length) return; [entries[at], entries[j]] = [entries[j], entries[at]]; at = j; fill(); write(); };
  $('.as-up').addEventListener('click', () => move(-1));
  $('.as-down').addEventListener('click', () => move(1));
  ta.addEventListener('input', () => { entries = parseAssoc(ta.value); at = Math.min(Math.max(at, 0), entries.length - 1); fill(); });
  fill();
}

command({ name: 'cm_InternalAssociate', label: 'Internal associations…', run: (app) => internalAssociations(app) });

// the icons of matching files come before the list's own (Panel.rowIcon, core/panel.js)
Panel.assocIcon = (p, e) => (p.app ? assocIcon(p.app, p, e) : '');
