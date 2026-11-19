// Notes and colour labels (board issue #147): the user gives files and folders a short note and a colour label, which
// mormiCommander keeps in its own database in the settings folder (Go: feature_notes.go, notes.json), never in the files,
// so they work on every disk and on servers. Renames and moves done inside mormiCommander take them along.
//
// They are called "notes" here, not "comments", so they are not mixed up with Total Commander's descript.ion comments
// (cm_EditComment, Ctrl+Z, the Comments view), which stay as they are and are stored in the folder itself.
//
// - A colour label is the name's colour; it wins over Colour by file type (Panel.nameColorHooks, which
//   config/layout.js paints).
// - A note shows as a small marker after the name (its tooltip is the note), and in the status line for the item under
//   the cursor. Properties (tools/props.js) has a Note field and a Colour label picker too.
// - Only the rows on screen are looked up: the row hooks ask for what they draw, the asks are sent to Go in one batch,
//   and the panels draw again when an answer has something to show. Answers are kept until Go says the notes changed.
//
// Commands (ours): cm_EditNote "Note…", cm_ColorLabel "Colour label…" (presets, a custom colour, no colour), both for
// the selection (or the item under the cursor) and in the file context menu; cm_CleanNotes removes the entries of
// files and folders that are gone.

import './notes.css';
import { command } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { Panel } from '../core/panel.js';
import { formDialog, messageDialog, esc } from '../core/dialogs.js';
import { showList } from '../core/popup.js';
import { plural } from '../core/format.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { anchorFor } from './system.js';
import { tm, tn } from '../core/i18n.js';

// The preset colours: readable as text on the light and the dark theme.
export const COLORS = [
  ['Red', '#e5484d'], ['Orange', '#f76b15'], ['Yellow', '#d4a72c'], ['Green', '#30a46c'],
  ['Blue', '#0090ff'], ['Purple', '#8e4ec6'], ['Grey', '#8b8d98'],
];
const colorName = (c) => COLORS.find(([, v]) => v === c)?.[0] || c;

// ---- looking up the rows on screen ----

const cache = new Map(); // full path → {note, color} or null (nothing)
const pending = new Set();
let timer = 0;
let gen = 0; // bumped when everything is asked again, so an old answer is dropped

const keyOf = (p, e) => (!e || e.up || !p.path || !e.name ? '' : p.fullPath(e.name));

// info is what is known of the entry: {note, color}, null for nothing, undefined while it is being asked.
function info(p, e) {
  const k = keyOf(p, e);
  if (!k) return null;
  const v = cache.get(k);
  if (v === undefined && !pending.has(k)) {
    pending.add(k);
    if (!timer) timer = setTimeout(flush, 15);
  }
  return v;
}

async function flush() {
  timer = 0;
  const list = [...pending];
  pending.clear();
  if (!list.length) return;
  const g = gen;
  let r = {};
  try { r = (await go.Notes.Lookup(list)) || {}; } catch { r = {}; }
  if (g !== gen) return;
  if (cache.size > 50000) cache.clear(); // a long session in big folders: start again
  let hit = false;
  for (const k of list) { const v = r[k] || null; cache.set(k, v); if (v) hit = true; }
  if (hit) repaint();
}

function repaint() {
  for (const p of theApp.panels || []) {
    try { p.renderRows(); noteStatus(p); } catch { /* not ready yet */ }
  }
}

// reset forgets every answer and asks again for the rows on screen.
function reset() {
  gen++;
  cache.clear();
  pending.clear();
  repaint();
}

Panel.rowClassHooks.push((p, e) => {
  const v = info(p, e);
  return v ? `${v.note ? 'mc-notes-has' : ''}${v.color ? ' mc-notes-label' : ''}`.trim() : '';
});
Panel.nameColorHooks.push((p, e) => info(p, e)?.color || '');

const MARK = '<svg viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 4h14v12l-4 4H5z"/><path d="M15 20v-4h4M8 9h8M8 13h5"/></svg>';

// marks adds the note marker (with the note as its tooltip) to the rows just drawn.
function marks(p) {
  for (const row of p.$.rows.children) {
    if (!row.classList.contains('mc-notes-has') || row.querySelector('.mc-notes-mark')) continue;
    const v = cache.get(keyOf(p, p.entries[+row.dataset.i]));
    if (!v?.note) continue;
    const m = document.createElement('i');
    m.className = 'mc-notes-mark';
    m.title = `Note: ${v.note}`;
    m.innerHTML = MARK;
    (row.querySelector('.n') || row).append(m);
  }
}

// noteStatus shows the note of the item under the cursor at the end of the panel's status line.
function noteStatus(p) {
  const st = p.$?.status;
  if (!st) return;
  st.querySelector('.mc-notes-status')?.remove();
  const v = info(p, p.cur);
  if (!v?.note) return;
  const s = document.createElement('span');
  s.className = 'mc-notes-status';
  const line = v.note.split('\n')[0];
  s.innerHTML = `<b>Note:</b> ${esc(line.length > 120 ? `${line.slice(0, 120)}…` : line)}`;
  s.title = v.note;
  st.append(s);
}

const renderStatus = Panel.prototype.renderStatus;
Panel.prototype.renderStatus = function notesRenderStatus() {
  renderStatus.call(this);
  noteStatus(this);
};

// ---- setting them ----

// targets is the selection, or the item under the cursor: [{paths, entries}].
function targets(app) {
  const p = app.cur;
  const entries = p.targets().filter((e) => !e.up);
  return { p, entries, paths: entries.map((e) => p.fullPath(e.name)) };
}

// known asks Go for the notes of the paths (not only the ones on screen).
async function known(paths) {
  try { return (await go.Notes.Lookup(paths)) || {}; } catch { return {}; }
}

function changed(paths, patch) {
  for (const k of paths) {
    const v = { ...(cache.get(k) || {}), ...patch };
    cache.set(k, v.note || v.color ? v : null);
  }
  repaint();
}

// pickerHTML is the colour label picker of the Properties dialog: none, the presets, a custom colour.
export function pickerHTML(current = '', id = 'nl') {
  const custom = current && !COLORS.some(([, v]) => v === current);
  const radio = (val, label, dot) => `<label title="${esc(label)}"><input type="radio" name="${id}-color" value="${esc(val)}"${val === current || (val === '*' && custom) ? ' checked' : ''}>${dot}</label>`;
  return `<div class="mc-notes-swatches" role="radiogroup" aria-label="Colour label">
    ${radio('', 'No colour', '<span class="mc-notes-dot none"></span>')}
    ${COLORS.map(([n, v]) => radio(v, n, `<span class="mc-notes-dot" style="background:${v}"></span>`)).join('')}
    ${radio('*', 'Custom colour', '')}<input type="color" id="${id}-custom" value="${esc(custom ? current : '#888888')}" aria-label="Custom colour">
  </div>`;
}

// pickerSetup makes a change of the custom colour choose it.
export function pickerSetup(box, id = 'nl') {
  const c = box.querySelector(`#${id}-custom`);
  if (c) c.addEventListener('input', () => { const r = box.querySelector(`input[name="${id}-color"][value="*"]`); if (r) r.checked = true; });
}

// pickerValue is the colour chosen ("" for none).
export function pickerValue(box, id = 'nl') {
  const v = box.querySelector(`input[name="${id}-color"]:checked`)?.value || '';
  return v === '*' ? box.querySelector(`#${id}-custom`).value.toLowerCase() : v;
}

// noteFor is the note and colour of one path, from Go.
export async function noteFor(path) {
  try { return (await go.Notes.Get(path)) || {}; } catch { return {}; }
}

// saveNote stores the note and colour of one path when they changed (the Properties dialog).
export async function saveNote(path, before, note, color) {
  note = (note || '').trim();
  if (note === (before.note || '') && color === (before.color || '')) return true;
  try {
    await go.Notes.Set(path, note, color);
  } catch (e) {
    theApp.flash(errText(e), 'err');
    return false;
  }
  changed([path], { note, color });
  return true;
}

async function editNote(app, args = {}) {
  const { entries, paths } = targets(app);
  if (!paths.length) { app.flash('Choose the files or folders to give a note.'); return false; }
  let text = args.note;
  if (typeof text !== 'string') {
    const have = await known(paths);
    const notes = paths.map((k) => have[k]?.note || '');
    const same = notes.every((n) => n === notes[0]);
    const what = entries.length === 1 ? entries[0].name : plural(entries.length, 'item');
    const hint = !same ? `<p class="mc-note">The ${esc(plural(entries.length, 'item'))} have different notes; this one replaces them all.</p>` : '';
    const r = await formDialog(`Note for ${what}`,
      `<div class="mc-row"><textarea id="mc-notes-text" class="mc-notes-text" spellcheck="true" aria-label="Note" autofocus>${esc(same ? notes[0] : '')}</textarea></div>
       ${hint}<p class="mc-note">Kept by mormiCommander, not in the file. Leave it empty to remove the note.</p>`,
      (box) => ({ text: box.querySelector('#mc-notes-text').value }), // (a string would be shown as an error)
      (box) => {
        const t = box.querySelector('#mc-notes-text');
        // Enter makes a new line; Ctrl+Enter or Cmd+Enter saves
        t.addEventListener('keydown', (ev) => { if (ev.key === 'Enter' && (ev.ctrlKey || ev.metaKey)) { ev.preventDefault(); box.requestSubmit?.(); } else if (ev.key === 'Enter') ev.stopPropagation(); });
      },
      { okLabel: 'Save', kind: 'notes-edit' });
    if (!r) return false;
    text = r.text;
  }
  try {
    await go.Notes.SetNote(paths, text);
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
  changed(paths, { note: text.trim() });
  return true;
}

async function customColor(start) {
  return formDialog('Custom colour label',
    `<div class="mc-row"><label for="mc-notes-custom">Colour</label><input type="color" id="mc-notes-custom" value="${esc(start || '#888888')}"></div>`,
    (box) => ({ color: box.querySelector('#mc-notes-custom').value.toLowerCase() }), null, { kind: 'notes-color' });
}

async function colorLabel(app, args = {}) {
  const { entries, paths } = targets(app);
  if (!paths.length) { app.flash('Choose the files or folders to give a colour label.'); return false; }
  let color = args.color;
  if (typeof color !== 'string') {
    const have = await known(paths);
    const cur = paths.map((k) => have[k]?.color || '');
    const one = cur.every((c) => c === cur[0]) ? cur[0] : null;
    const mark = (v) => (one === v ? '✓' : '');
    const items = [
      ...COLORS.map(([n, v]) => ({ label: n, detail: mark(v), value: v })),
      { label: 'Custom colour…', detail: one && !COLORS.some(([, v]) => v === one) ? '✓' : '', value: '*' },
      { sep: true },
      { label: 'No colour', detail: mark(''), value: '' },
    ];
    const r = await showList({ title: entries.length === 1 ? `Colour label of ${entries[0].name}` : `Colour label of ${plural(entries.length, 'item')}`, items, anchor: anchorFor(app.cur, args), kind: 'notes-color-menu' });
    if (!r) return false;
    color = r.item.value;
    if (color === '*') {
      const c = await customColor(one || '');
      if (!c) return false;
      color = c.color;
    }
  }
  try {
    await go.Notes.SetColor(paths, color);
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
  changed(paths, { color: color.toLowerCase() });
  return true;
}

const hasTargets = (app) => !!app.cur && app.cur.targets().some((e) => !e.up);

command({ name: 'cm_EditNote', label: 'Note…', menu: 'Files', menuAfter: 'cm_EditComment', enabled: hasTargets, run: editNote });
command({ name: 'cm_ColorLabel', label: 'Colour label…', menu: 'Files', menuAfter: 'cm_EditNote', enabled: hasTargets, run: colorLabel });

command({
  name: 'cm_CleanNotes', label: 'Clean up notes and colour labels…', menu: 'Files', menuAfter: 'cm_ColorLabel',
  run: async (app) => {
    let r;
    try { r = await go.Notes.CleanPreview(); } catch (e) { app.flash(errText(e), 'err'); return false; }
    const kept = [];
    if (r.servers) kept.push(`${plural(r.servers, 'entry', 'entries')} on servers ${r.servers === 1 ? 'is' : 'are'} not checked`);
    if (r.unplugged) kept.push(`${plural(r.unplugged, 'entry', 'entries')} on disks that are not connected now ${r.unplugged === 1 ? 'is' : 'are'} kept`);
    const tail = kept.length ? ` (${kept.join('; ')})` : '';
    if (!r.gone?.length) {
      await messageDialog({ Title: 'Clean up notes and colour labels', Message: `No entry is for a file or folder that is gone${tail}.`, Buttons: ['OK'], Kind: 'notes-clean' });
      return true;
    }
    const show = r.gone.slice(0, 8).join('\n') + (r.gone.length > 8 ? `\n… and ${r.gone.length - 8} more` : '');
    const b = await messageDialog({
      Title: 'Clean up notes and colour labels',
      Message: `${plural(r.gone.length, 'entry', 'entries')} ${r.gone.length === 1 ? 'is' : 'are'} for files and folders that are gone${tail}. Remove ${r.gone.length === 1 ? 'it' : 'them'}?\n\n${show}`,
      Buttons: ['Remove', 'Cancel'], Kind: 'notes-clean',
    });
    if (b !== 'Remove') return false;
    try { await go.Notes.Forget(r.gone); } catch (e) { app.flash(errText(e), 'err'); return false; }
    reset();
    app.flash(tm('Removed {what}.', { what: tn(r.gone.length, 'entry', 'entries') }));
    return true;
  },
});

// notes {side}: the notes and colour labels of the rows the panel has looked up (for control mode and QA).
addControlCommand('notes', (m = {}) => {
  const p = theApp.panel(m.side || 'active');
  const out = {};
  for (const e of p.entries) {
    const v = cache.get(keyOf(p, e));
    if (v) out[e.name] = v;
  }
  return { notes: out };
});

export function init(app) {
  for (const p of app.panels) {
    new MutationObserver(() => marks(p)).observe(p.$.rows, { childList: true });
  }
  app.on('cursor', (p) => noteStatus(p));
  on('notes-changed', reset);
}
