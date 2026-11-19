// File tags (issue #28, F-869): the tags the system's own file manager shows — Finder tags on macOS, user.xdg.tags on
// Linux — kept in the files themselves (Go: feature_tags.go, internal/tags), so a tag set in Finder shows here and one
// set here shows in Finder, and copies keep them. Windows has no file tags: the commands say so and stay disabled.
//
// - Show > File tags (cm_ShowTags, off by default) draws a coloured dot per tag after each name (the tooltip names
//   them; "File tags settings…" can show the names too) and the tags of the item under the cursor in the status line.
//   Only the rows on screen are looked up, in batches, as notes.js does. Custom columns can show [=mc.tags] anyway.
// - Files > Tags… (cm_EditTags; also in the file context menu) adds and removes tags on the selection. No question
//   first: a tag change is not destructive, and it is one step of the undo journal (Cmd+Z puts the old tags back).
// - Mark > Select by tag… (cm_SelectByTag) selects the files of the folder with a tag; Show > Only files with tag…
//   (cm_FilterByTag) shows only those (choosing it again, or another folder, shows all again).
//
// Settings: "tags" = {enabled, names} (tags-lib.js has the defaults). Control command "tags" returns what the active
// panel has looked up.

import './tags.css';
import { command } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { Panel } from '../core/panel.js';
import { formDialog, esc } from '../core/dialogs.js';
import { showList } from '../core/popup.js';
import { plural } from '../core/format.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { isLocal } from './system-lib.js';
import { tagsConfig, marksHTML, tagText, summary, parseNames, changes, folderTags, hasTag, colorOf, COLORS } from './tags-lib.js';
import { tm, tn, te } from '../core/i18n.js';

const cfg = () => tagsConfig(theApp.setting('tags', {}));
let info = { supported: false, style: '', colors: false };
const supported = () => !!info.supported;

// ---- looking up the rows on screen ----

const cache = new Map(); // full path → [tags] or null (none)
const pending = new Set();
let timer = 0;
let gen = 0;

const keyOf = (p, e) => (!e || e.up || !p.path || !e.name || !isLocal(p.path) ? '' : p.fullPath(e.name));

function tagsOf(p, e) {
  if (!supported() || !cfg().enabled) return null;
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
  try { r = (await go.Tags.Lookup(list)) || {}; } catch { r = {}; }
  if (g !== gen) return;
  if (cache.size > 50000) cache.clear();
  let hit = false;
  for (const k of list) { const v = r[k] || null; cache.set(k, v); if (v) hit = true; }
  if (hit) repaint();
}

function repaint() {
  for (const p of theApp.panels || []) {
    try { p.renderRows(); tagStatus(p); } catch { /* not ready yet */ }
  }
}

function reset() {
  gen++;
  cache.clear();
  pending.clear();
  repaint();
}

Panel.rowClassHooks.push((p, e) => (tagsOf(p, e)?.length ? 'mc-tags-has' : ''));

// marks adds the dots (and names) to the rows just drawn.
function marks(p) {
  if (!supported() || !cfg().enabled) return;
  for (const row of p.$.rows.children) {
    if (!row.classList.contains('mc-tags-has') || row.querySelector('.mc-tags-mark')) continue;
    const v = cache.get(keyOf(p, p.entries[+row.dataset.i]));
    if (!v?.length) continue;
    const m = document.createElement('span');
    m.className = 'mc-tags-mark';
    m.title = `Tags: ${tagText(v)}`;
    m.innerHTML = marksHTML(v, cfg());
    (row.querySelector('.n') || row).append(m);
  }
}

// tagStatus shows the tags of the item under the cursor at the end of the panel's status line.
function tagStatus(p) {
  const st = p.$?.status;
  if (!st) return;
  st.querySelector('.mc-tags-status')?.remove();
  const v = tagsOf(p, p.cur);
  if (!v?.length) return;
  const s = document.createElement('span');
  s.className = 'mc-tags-status';
  s.innerHTML = `<b>Tags:</b> ${esc(tagText(v))}`;
  st.append(s);
}

const renderStatus = Panel.prototype.renderStatus;
Panel.prototype.renderStatus = function tagsRenderStatus() {
  renderStatus.call(this);
  tagStatus(this);
};

// ---- commands ----

// lookup asks Go for the tags of the paths (not only the ones on screen).
async function lookup(paths) {
  try { return (await go.Tags.Lookup(paths)) || {}; } catch { return {}; }
}

function why(app) {
  if (!supported()) { app.flash(app.platform === 'windows' ? 'Windows has no file tags.' : 'This system has no file tags.', 'err'); return true; }
  if (!isLocal(app.cur?.path)) { app.flash('File tags are only on this computer\'s disks.', 'err'); return true; }
  return false;
}

const hasTargets = (app) => supported() && !!app.cur && isLocal(app.cur.path) && app.cur.targets().some((e) => !e.up);

// folderList is every tag used in the panel's folder (at most 5000 entries are looked at).
async function folderList(p) {
  const paths = p.all.filter((e) => !e.up).slice(0, 5000).map((e) => p.fullPath(e.name));
  const map = await lookup(paths);
  return { map, list: folderTags(map) };
}

async function editTags(app, args = {}) {
  if (why(app)) return false;
  const p = app.cur;
  const entries = p.targets().filter((e) => !e.up);
  const paths = entries.map((e) => p.fullPath(e.name));
  if (!paths.length) { app.flash('Choose the files or folders to tag.'); return false; }
  let add, remove;
  if (Array.isArray(args.add) || Array.isArray(args.remove)) {
    add = (args.add || []).map((t) => (typeof t === 'string' ? { name: t, color: 0 } : t));
    remove = args.remove || [];
  } else {
    const have = await lookup(paths);
    const { list: extra } = await folderList(p);
    const rows = summary(paths.map((k) => have[k] || []), { colours: info.colors, extra });
    const n = paths.length;
    const what = entries.length === 1 ? entries[0].name : plural(entries.length, 'item');
    const dot = (t) => (colorOf(t) ? `<i class="mc-tags-dot" style="background:${colorOf(t)}"></i>` : '<i class="mc-tags-dot none"></i>');
    const colorPick = info.colors
      ? `<select id="mc-tags-color" aria-label="Colour of the new tags">${COLORS.map(([c], i) => `<option value="${i}">${esc(i ? c : 'No colour')}</option>`).join('')}</select>`
      : '';
    const r = await formDialog(`Tags of ${what}`,
      `<div class="mc-tags-list">${rows.map((t, i) => `<label class="mc-check"><input type="checkbox" data-i="${i}"${t.count === n ? ' checked' : ''}> ${dot(t)} ${esc(t.name)}${t.count && t.count < n ? ` <span class="mc-note">(${t.count} of ${n})</span>` : ''}</label>`).join('') || '<p class="mc-note">No tags yet.</p>'}</div>
       <div class="mc-row"><label for="mc-tags-new">New tags</label><input type="text" id="mc-tags-new" placeholder="Comma-separated" spellcheck="false" autocomplete="off">${colorPick}</div>
       <p class="mc-note">Kept in the ${entries.length === 1 ? 'file' : 'files'} as ${info.style === 'finder' ? 'Finder tags' : 'user.xdg.tags'}. Files &gt; Undo takes the change back.</p>`,
      (box) => ({
        states: rows.map((_, i) => { const c = box.querySelector(`input[data-i="${i}"]`); return c.indeterminate ? 'mixed' : c.checked; }),
        typed: parseNames(box.querySelector('#mc-tags-new').value),
        color: Number(box.querySelector('#mc-tags-color')?.value || 0),
      }),
      (box) => {
        rows.forEach((t, i) => { if (t.count && t.count < n) box.querySelector(`input[data-i="${i}"]`).indeterminate = true; });
        box.querySelector('#mc-tags-new').focus();
      },
      { okLabel: 'Save', kind: 'tags-edit' });
    if (!r) return false;
    ({ add, remove } = changes(rows, r.states, n, r.typed, r.color));
  }
  if (!add.length && !remove.length) return true;
  let res;
  try { res = await go.Tags.Change(paths, add, remove); } catch (e) { app.flash(errText(e), 'err'); return false; }
  for (const k of paths) cache.delete(k);
  repaint();
  if (res.failed?.length) {
    app.flash(tm('Tags could not be changed on {what}: {error}', { what: tn(res.failed.length, 'item'), error: te(res.failed[0].error) }), 'err');
    return false;
  }
  app.flash(res.changed ? `Changed the tags of ${plural(res.changed, 'item')}.` : 'The tags were already so.');
  return true;
}

// pickTag asks which of the folder's tags (args.tag skips the question).
async function pickTag(app, args, title, kind) {
  if (why(app)) return null;
  const p = app.cur;
  const { map, list } = await folderList(p);
  let tag = typeof args.tag === 'string' ? args.tag : '';
  if (!tag) {
    if (!list.length) { app.flash('No file in this folder has a tag.'); return null; }
    const items = list.map((t) => ({ label: t.name, detail: t.color ? COLORS[t.color][0] : '', value: t.name }));
    const r = await showList({ title, items, kind });
    if (!r) return null;
    tag = r.item.value;
  }
  return { p, tag, map };
}

async function selectByTag(app, args = {}) {
  const r = await pickTag(app, args, 'Select by tag', 'tags-select');
  if (!r) return false;
  const { p, tag, map } = r;
  const want = (e) => !e.up && hasTag(map[p.fullPath(e.name)], tag);
  const n = p.entries.filter(want).length;
  if (!n) { app.flash(tm('Nothing here has the tag {tag}.', { tag })); return false; }
  p.selectWhere(want, args.select !== false);
  return true;
}

async function filterByTag(app, args = {}) {
  const p0 = app.cur;
  if (p0?.filter?.mcTags && args.tag === undefined) { p0.setFilter(null); return true; } // a second time: all again
  const r = await pickTag(app, args, 'Show only files with the tag', 'tags-filter');
  if (!r) return false;
  const { p, tag, map } = r;
  const fn = (e) => hasTag(map[p.fullPath(e.name)], tag);
  fn.mcTags = tag;
  p.setFilter(fn, `Tag: ${tag}`);
  return true;
}

function save(patch) {
  theApp.setSetting('tags', { ...cfg(), ...patch });
  reset();
}

command({
  name: 'cm_ShowTags', label: 'File tags', menu: 'Show', menuAfter: 'cm_GitStatus',
  checked: () => supported() && cfg().enabled,
  run: (app, args = {}) => {
    if (!supported()) return why(app) && false;
    const on = typeof args.on === 'boolean' ? args.on : !cfg().enabled;
    save({ enabled: on });
    app.flash(on ? 'File tags are shown after the names.' : 'File tags are not shown.');
    return true;
  },
});

command({
  name: 'cm_TagsSettings', label: 'File tags settings…', menu: 'Configuration', menuAfter: 'cm_GitStatusSettings',
  run: async (app) => {
    if (!supported()) return why(app) && false;
    const c = cfg();
    const chk = (id, v, label) => `<label class="mc-check"><input type="checkbox" id="${id}"${v ? ' checked' : ''}> ${esc(label)}</label>`;
    const r = await formDialog('File tags settings',
      `${chk('tags-on', c.enabled, 'Show file tags after the names (a coloured dot per tag)')}
       ${chk('tags-names', c.names, 'Show the tag names beside the dots')}
       <p class="mc-note">Custom columns can show the tags with the field [=mc.tags].</p>`,
      (b) => ({ enabled: b.querySelector('#tags-on').checked, names: b.querySelector('#tags-names').checked }), null, { kind: 'tags-settings' });
    if (!r) return false;
    save(r);
    return true;
  },
});

command({ name: 'cm_EditTags', label: 'Tags…', menu: 'Files', menuAfter: 'cm_ColorLabel', enabled: hasTargets, run: editTags });
command({ name: 'cm_SelectByTag', label: 'Select by tag…', menu: 'Mark', menuAfter: 'cm_SelectFromOtherPanel', enabled: (app) => supported() && isLocal(app.cur?.path), run: selectByTag });
command({
  name: 'cm_FilterByTag', label: 'Only files with tag…', menu: 'Show', menuAfter: 'cm_ShowTags',
  enabled: (app) => supported() && isLocal(app.cur?.path), checked: (app) => !!app.cur?.filter?.mcTags, run: filterByTag,
});

addControlCommand('tags', (m = {}) => {
  const p = theApp.panel(m.side || 'active');
  const out = {};
  for (const e of p.entries) {
    const v = cache.get(keyOf(p, e));
    if (v) out[e.name] = v;
  }
  return { info, settings: cfg(), tags: out };
});

const seenAll = new WeakMap();

export async function init(app) {
  for (const p of app.panels) {
    new MutationObserver(() => marks(p)).observe(p.$.rows, { childList: true });
  }
  app.on('cursor', (p) => tagStatus(p));
  // a folder read again (F2, a change seen on disk, coming back to it) asks again: tags may have been set in Finder
  app.on('panel', (p) => {
    if (!supported() || !cfg().enabled || !p.path || !isLocal(p.path) || seenAll.get(p) === p.all) return;
    seenAll.set(p, p.all); // (only a new listing, not every cursor move or selection change)
    const dir = p.fullPath('');
    let dropped = false;
    for (const k of cache.keys()) if (k.startsWith(dir) && !k.slice(dir.length).match(/[\\/]/)) { cache.delete(k); dropped = true; }
    if (dropped) p.renderRows();
  });
  on('tags-changed', reset);
  try { info = (await go.Tags.Info()) || info; } catch { /* no Go side (tests) */ }
  if (supported() && cfg().enabled) repaint();
}

// tagsFor is the tags of one path for the Properties dialog: [tags], or null where the system has none.
export async function tagsFor(path) {
  if (!supported() || !isLocal(path)) return null;
  return (await lookup([path]))[path] || [];
}

// tagsLine is the Properties dialog's line for them.
export const tagsLine = (list) => (list.length ? marksHTML(list, { names: true }) : '<span class="mc-note">None</span>');
