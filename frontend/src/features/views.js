// Views and the listing (FEATURES.md sections 4, 5, 32 and 33): Brief (Ctrl+F1, names in newspaper columns), Full
// (Ctrl+F2), Comments (Ctrl+Shift+F2, descript.ion), Thumbnails (Ctrl+Shift+F1), custom column sets (Shift+F1), the tree
// in the panel (Ctrl+F8), size / date formats, sort by a custom column, natural sort and "folders always by name", the
// left / right / target variants of every view and sort command, and Ctrl+Z (edit comment).
//
// The cells of the column views come from internal/listing (Go: Views.Cells), the pictures of the thumbnails view from
// internal/thumbs (Views.Thumbs → "views-thumb" events). The panel itself is the core's: this module only replaces how
// a panel draws its rows (renderRows, ensureVisible, pageSize on each panel object), never the core's files. With the
// default Full view and default formats nothing is replaced, so the core's own drawing is used.
//
// Settings: "views" = {sizeFormat, dateFormat, attrFormat, namesWhole, thumbSize, columnSets: [{name, autoSwitch,
// columns: [{title, template, width, align}]}], hiddenCols (views-cols-lib.js), left: {mode, set}, right: {mode, set}}; plus the core's "naturalSort"
// and "dirsLikeFiles". "left"/"right" are the views of the panels' active tabs: each folder tab keeps its own view
// (issue #128, registered with features/tabs.js; the pure helpers are in views-tab.js), saved with the tab as
// "view"/"set".

import { compileMask } from './config/util.js';
import { addMenuItems, refreshMenu } from '../core/menus.js';
import './views.css';
import { command } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { Panel, splitPath } from '../core/panel.js';
import { esc, modal, formDialog, messageDialog, inputDialog } from '../core/dialogs.js';
import { showList } from '../core/popup.js';
import { addKeyHandler } from '../core/keyboard.js';
import { addControlCommand } from '../core/control.js';
import { cellSize, date, attr, size as shortSize, display } from '../core/format.js';
import { cellFormat, shownName, extColumn } from './views-format.js';
import { app as theApp } from '../core/app.js';
import { addTabState } from './tabs.js';
import { nameComparer, SORT_METHODS, validLocale } from '../core/sort.js';
import { freshView, copyView, viewToSaved, viewFromSaved } from './views-tab.js';
import { viewKey, hiddenFor, visibleCols, toggleHidden } from './views-cols-lib.js';
import { mergeKids, cursorAfter } from './views-tree.js';
import { cleanThumbLines, thumbTileH, thumbSubHtml } from './views-thumbs-lib.js';

// ---- settings ----

const DEFAULTS = { sizeFormat: 'bytes', dateFormat: '', attrFormat: '', namesWhole: false, thumbSize: 96, columnSets: [] };
const cfg = () => ({ ...DEFAULTS, ...(theApp.setting('views', {}) || {}) });
function saveCfg(patch) {
  const next = { ...cfg(), ...patch };
  return theApp.setSetting('views', next);
}
const sets = () => (Array.isArray(cfg().columnSets) ? cfg().columnSets : []);
const findSet = (name) => sets().find((s) => s.name === name) || null;

const SIZE_FORMATS = [['bytes', 'Bytes (1,234,567)'], ['k', 'Kilobytes (1,206 k)'], ['M', 'Megabytes (1.2 M)'], ['G', 'Gigabytes (0.01 G)'], ['dynamic', 'Dynamic (1.18 M)']];
const DATE_FORMATS = [['', 'yyyy-MM-dd HH:mm (default)'], ['yyyy-MM-dd HH:mm:ss', 'yyyy-MM-dd HH:mm:ss'], ['dd.MM.yyyy HH:mm', 'dd.MM.yyyy HH:mm'],
  ['dd.MM.yy HH:mm', 'dd.MM.yy HH:mm'], ['MM/dd/yyyy hh:mm tt', 'MM/dd/yyyy hh:mm tt'], ['dd/MM/yyyy HH:mm', 'dd/MM/yyyy HH:mm']];
const ATTR_FORMATS = [['', 'As the system shows them'], ['unix', 'Unix (drwxr-xr-x)'], ['octal', 'Octal (0755)'], ['win', 'Windows (rahs)'], ['winfull', 'Windows, all (rahsce)']];

const defaultFormats = (c = cfg()) => (!c.sizeFormat || c.sizeFormat === 'bytes') && !c.dateFormat && !c.attrFormat && !c.namesWhole;
const thumbSize = () => Math.max(32, Math.min(512, Number(cfg().thumbSize) || 96));
// the field lines under each thumbnail (Thumbnails.FieldLines, F-679): column templates; none = the size or dimensions
const thumbLines = () => cleanThumbLines(cfg().thumbLines);

// ---- per-panel state (p.vs, swapped with the panel by Ctrl+U) ----

const MODES = ['brief', 'full', 'comments', 'custom', 'thumbs', 'tree'];

function vsOf(p) {
  if (!p.vs) p.vs = freshView();
  return p.vs;
}
const modeOf = (p) => vsOf(p).mode;
const hiddenNow = (p) => hiddenFor(cfg().hiddenCols, viewKey(vsOf(p)));
// (the default Full view with default formats is the core's drawing, unless the user hid one of its columns, F-672)
const isColumnMode = (p) => { const m = modeOf(p); return m === 'comments' || m === 'custom' || (m === 'full' && (!defaultFormats() || hiddenNow(p).length > 0)); };

// columns is the list of columns after the name for a column view: [{title, template, width, align, sort}] (sort: the
// core sort key a click on the header uses; none means "sort by this column" through the engine). The columns hidden
// from the header's right-click menu are left out (F-672); allColumns has them all.
function columns(p) {
  const def = allColumns(p), hidden = hiddenNow(p);
  if (!hidden.length) return def;
  return { ...def, ext: def.ext && !hidden.includes('Ext'), cols: visibleCols(def.cols, hidden) };
}

function allColumns(p) {
  const vs = vsOf(p), c = cfg();
  if (vs.mode === 'comments') return { ext: false, nameW: 'minmax(140px, 38%)', cols: [{ title: 'Comment', template: '[=tc.comment]', width: 0, align: 'left' }] };
  if (vs.mode === 'custom') {
    const s = findSet(vs.set);
    if (s) return { ext: false, nameW: 'minmax(120px, 1fr)', cols: (s.columns || []).map((x) => ({ title: x.title || '', template: x.template || '', width: +x.width || 0, align: x.align || 'left' })) };
  }
  const dw = Math.max(100, Math.round(((cellFormat(c, display).date || 'yyyy-MM-dd HH:mm').length) * 7.2) + 10);
  return {
    ext: extColumn(c, display), nameW: 'minmax(90px, 1fr)',
    cols: [
      { title: 'Size', template: '[=tc.size]', width: 96, align: 'right', sort: 'size' },
      { title: 'Date', template: '[=tc.writedate]', width: dw, align: 'left', sort: 'date' },
      { title: 'Attr', template: '[=tc.attributestr]', width: c.attrFormat === 'winfull' ? 60 : 84, align: 'left' },
    ],
  };
}

// ---- drawing ----

const coreHeader = new WeakMap();   // panel element → the core's header html
const ids = new WeakMap();          // p.all → a number, so a reread is noticed
let idSeq = 0;
const allId = (a) => { if (!ids.has(a)) ids.set(a, ++idSeq); return ids.get(a); };

let measureCtx = null;
function textWidth(list, entries) {
  if (!measureCtx) measureCtx = document.createElement('canvas').getContext('2d');
  const cs = getComputedStyle(list);
  measureCtx.font = cs.font || '13px system-ui';
  let w = 0;
  const n = Math.min(entries.length, 5000); // (a huge folder is measured by its first 5,000 names: close enough)
  for (let i = 0; i < n; i++) w = Math.max(w, measureCtx.measureText(entries[i].name).width * (entries[i].dir ? 1.08 : 1));
  return w;
}

function rowClass(p, e, i) {
  let cls = 'row';
  if (i === p.cursor) cls += ' cur';
  if (e.up) return cls + ' up dir';
  if (e.dir) cls += ' dir';
  if (p.selected.has(e.name)) cls += ' sel';
  if (e.hidden) cls += ' hid';
  if (e.link) cls += ' link';
  if (e.broken) cls += ' broken';
  if (e.exec && !e.dir) cls += ' exe';
  return cls + Panel.extraRowClass(p, e);
}

// brief geometry: rows per column and the column width
function briefGeo(p) {
  const list = p.$.list, vs = vsOf(p);
  const rows = Math.max(1, Math.floor(list.clientHeight / p.rowH));
  if (!vs.brief || vs.brief.entries !== p.entries || vs.brief.font !== p.rowH) {
    const w = textWidth(list, p.entries);
    vs.brief = { entries: p.entries, font: p.rowH, textW: w };
  }
  const cap = Math.max(160, list.clientWidth * 0.9);
  const colW = Math.round(Math.max(90, Math.min(cap, vs.brief.textW + 44)));
  return { rows, colW, cols: Math.ceil(p.entries.length / rows) };
}

// thumbnails geometry: tiles per row, tile size
function thumbGeo(p) {
  const list = p.$.list, s = thumbSize();
  const cols = Math.max(1, Math.floor(list.clientWidth / (s + 20)));
  const tileW = Math.floor(list.clientWidth / cols) || s + 20;
  const tileH = thumbTileH(s, thumbLines().length);
  return { cols, tileW, tileH, rows: Math.ceil(p.entries.length / cols), s };
}

function renderBrief(p) {
  const { list, rows, spacer } = p.$;
  const g = briefGeo(p);
  spacer.style.height = '1px';
  spacer.style.width = `${g.cols * g.colW}px`;
  const first = Math.max(0, Math.floor(list.scrollLeft / g.colW) - 1);
  const last = Math.min(g.cols, Math.ceil((list.scrollLeft + (list.clientWidth || 800)) / g.colW) + 1);
  const active = p.isActive;
  let html = '';
  for (let c = first; c < last; c++) {
    for (let r = 0; r < g.rows; r++) {
      const i = c * g.rows + r;
      if (i >= p.entries.length) break;
      const e = p.entries[i];
      const tip = Panel.tipAttr(p, e, true);
      html += `<div class="${rowClass(p, e, i)} mc-views-bi" data-i="${i}" style="left:${c * g.colW}px;top:${r * p.rowH}px;width:${g.colW}px"${i === p.cursor && active ? ' aria-selected="true"' : ''}><span class="n">${Panel.rowIcon(p, e)}<b${tip}>${esc(shownName(e, e.name, display))}</b></span></div>`;
    }
  }
  rows.style.transform = '';
  rows.style.width = `${g.cols * g.colW}px`;
  rows.innerHTML = html;
}

function renderThumbs(p) {
  const { list, rows, spacer } = p.$;
  const g = thumbGeo(p), vs = vsOf(p), t = vs.thumbs || {};
  spacer.style.width = '1px';
  spacer.style.height = `${g.rows * g.tileH}px`;
  const first = Math.max(0, Math.floor(list.scrollTop / g.tileH) - 1);
  const last = Math.min(g.rows, Math.ceil((list.scrollTop + (list.clientHeight || 600)) / g.tileH) + 1);
  const active = p.isActive;
  let html = '';
  const visible = [];
  for (let r = first; r < last; r++) {
    for (let c = 0; c < g.cols; c++) {
      const i = r * g.cols + c;
      if (i >= p.entries.length) break;
      const e = p.entries[i];
      const url = t.urls?.get(e.name);
      const icon = t.icons?.names?.[e.name];
      const svg = icon ? t.icons.svg[icon] : '';
      const pic = url ? `<img src="${url}" alt="" draggable="false">` : svg ? `<img class="icon" src="${svg}" alt="" draggable="false">` : '<i class="ic"></i>';
      const dims = t.dims?.get(e.name);
      const lines = vs.thumbCells?.lines, cells = lines && vs.thumbCells.path === p.path ? vs.thumbCells.map.get(e.name) : null;
      if (!e.up && !e.dir && !url) visible.push(e.name);
      html += `<div class="${rowClass(p, e, i)} mc-views-tile" data-i="${i}" style="left:${c * g.tileW}px;top:${r * g.tileH}px;width:${g.tileW}px;height:${g.tileH}px"${i === p.cursor && active ? ' aria-selected="true"' : ''}>`
        + `<div class="mc-views-pic" style="height:${g.s}px">${pic}</div><div class="mc-views-cap" title="${esc(e.name)}">${esc(e.name)}</div>`
        + thumbSubHtml(lines, cells, dims ? `${dims[0]} × ${dims[1]}` : e.up ? '' : e.dir ? (e.dirSize != null ? esc(shortSize(e.dirSize)) : '') : esc(shortSize(e.size)), esc) + '</div>';
    }
  }
  rows.style.transform = '';
  rows.style.width = '';
  rows.innerHTML = html;
  if (visible.length && t.gen) {
    clearTimeout(t.firstTimer);
    t.firstTimer = setTimeout(() => go.Views.ThumbsFirst(p.side, visible).catch(() => {}), 80);
  }
}

function renderColumns(p) {
  const { list, rows } = p.$;
  const h = p.rowH, vs = vsOf(p), def = columns(p);
  const first = Math.max(0, Math.floor(list.scrollTop / h) - 3);
  const last = Math.min(p.entries.length, first + Math.ceil((list.clientHeight || 600) / h) + 7);
  const active = p.isActive, plat = p.app.platform;
  const cells = vs.cells?.path === p.path ? vs.cells.map : null;
  const full = vs.mode === 'full';
  let html = '';
  for (let i = first; i < last; i++) {
    const e = p.entries[i];
    const name = def.ext ? (e.dir ? e.name : e.base) : e.name;
    const tip = Panel.tipAttr(p, e, name.length > 28);
    let row = `<div class="${rowClass(p, e, i)}" data-i="${i}"${i === p.cursor && active ? ' aria-selected="true"' : ''}><span class="n">${Panel.rowIcon(p, e)}<b${tip}>${esc(shownName(e, name, display))}</b></span>`;
    if (def.ext) row += `<span class="e">${esc(e.ext)}</span>`;
    let c = cells?.get(e.name);
    // (".." is not in the engine's rows; until the cells come, the Full view shows what the panel knows)
    if (!c && full) c = def.cols.map((col) => fullCell(col.template, e, plat));
    def.cols.forEach((col, k) => {
      const txt = c ? c[k] ?? '' : '';
      const raw = e.up && full && col.template === '[=tc.size]' ? txt : esc(txt);
      row += `<span class="mc-views-c${col.align === 'right' ? ' r' : col.align === 'center' ? ' c' : ''}"${txt.length > 24 ? ` title="${esc(txt)}"` : ''}>${raw}</span>`;
    });
    html += row + '</div>';
  }
  rows.style.transform = `translateY(${first * h}px)`;
  rows.style.width = '';
  rows.innerHTML = html;
}

// fullCell is a Full view cell drawn from the panel's own entry, before (or without) the engine's cells.
function fullCell(template, e, plat) {
  if (e.up) return template === '[=tc.size]' ? '&lt;DIR&gt;' : '';
  if (template === '[=tc.size]') return e.dir ? (e.dirSize != null ? cellSize(e.dirSize) : '<DIR>') : cellSize(e.size);
  if (template === '[=tc.writedate]') return date(e.mtime);
  if (template === '[=tc.attributestr]') return attr(e.mode, e.flags, plat);
  return '';
}

// ---- the header ----

function renderHeader(p) {
  const { header } = p.$;
  const vs = vsOf(p);
  const el = p.el;
  el.dataset.view = vs.mode;
  if (vs.mode === 'tree') { header.hidden = true; return; }
  header.hidden = false;
  el.classList.toggle('mc-views-cols', isColumnMode(p)); // (custom columns: the no-extension layout doesn't apply)
  if (!isColumnMode(p)) {
    el.style.removeProperty('--cols');
    if (header.dataset.views) { header.innerHTML = coreHeader.get(el); delete header.dataset.views; }
    markCoreHeader(p);
    return;
  }
  const def = columns(p);
  const tpl = [def.nameW];
  if (def.ext) tpl.push('56px');
  // Options > Custom columns > Fit columns to the panel width: the set's own widths give way, every column shares the room
  const fit = treeOpt('CustomColumns.EqualWidthOnOpen', '0') === '1';
  for (const c of def.cols) tpl.push(c.width && !fit ? `${c.width}px` : 'minmax(60px, 1fr)');
  el.style.setProperty('--cols', tpl.join(' '));
  let html = `<span data-vcol="name" title="Sort by name">Name</span>`;
  if (def.ext) html += `<span data-vcol="ext" title="Sort by extension">Ext</span>`;
  def.cols.forEach((c, k) => { html += `<span data-vcol="${k}" class="${c.align === 'right' ? 'r' : ''}" title="Sort by ${esc(c.title.toLowerCase() || `column ${k + 1}`)}${c.sort ? ' (Ctrl/Cmd+click: as the second sort key)' : ''}">${esc(c.title)}</span>`; });
  header.innerHTML = html;
  header.dataset.views = '1';
  const s = p.sort;
  header.querySelectorAll('[data-vcol]').forEach((sp) => {
    const v = sp.dataset.vcol;
    const col = def.cols[+v];
    const on = v === s.key || (col && (col.sort ? col.sort === s.key : s.key === `col${+v + 1}`));
    const k2 = s.then?.key;
    const on2 = !on && !!k2 && (v === k2 || (col && col.sort === k2));
    sp.classList.toggle('sorted', !!on);
    sp.classList.toggle('sorted2', on2);
    sp.classList.toggle('desc', (!!on && s.desc) || (on2 && !!s.then.desc));
  });
}

function markCoreHeader(p) {
  p.$.header.querySelectorAll('[data-col]').forEach((s) => {
    const on = s.dataset.col === p.sort.key;
    const on2 = !on && s.dataset.col === p.sort.then?.key;
    s.classList.toggle('sorted', on);
    s.classList.toggle('sorted2', on2);
    s.classList.toggle('desc', (on && p.sort.desc) || (on2 && !!p.sort.then.desc));
  });
}

// ---- the overrides of each panel ----

function install(p) {
  const core = {
    renderRows: p.renderRows.bind(p), ensureVisible: p.ensureVisible.bind(p), pageSize: p.pageSize.bind(p),
    rebuild: p.rebuild.bind(p), render: p.render.bind(p), targets: p.targets.bind(p),
  };
  coreHeader.set(p.el, p.$.header.innerHTML);
  vsOf(p);
  p.renderRows = () => {
    const m = modeOf(p);
    if (m === 'brief') return renderBrief(p);
    if (m === 'thumbs') return renderThumbs(p);
    if (m === 'tree') return renderTree(p);
    if (isColumnMode(p)) return renderColumns(p);
    return core.renderRows();
  };
  // (requestCells only asks again when the folder or the formats changed, e.g. a new Display option, issue #93)
  p.render = () => { core.render(); renderHeader(p); if (isColumnMode(p)) requestCells(p); if (modeOf(p) === 'tree') renderTreeStatus(p); };
  p.pageSize = () => {
    const m = modeOf(p);
    if (m === 'brief') return briefGeo(p).rows;
    if (m === 'thumbs') { const g = thumbGeo(p); return Math.max(1, Math.floor(p.$.list.clientHeight / g.tileH)) * g.cols; }
    return core.pageSize();
  };
  // the step of Shift+Left / Shift+Right (selection.js): a column in Brief, one tile in Thumbnails, a page otherwise
  // the step of Shift+Up / Shift+Down (selection.js): a row of tiles in Thumbnails, one entry otherwise (F-850)
  p.vertStep = () => (modeOf(p) === 'thumbs' ? thumbGeo(p).cols : 1);
  // the boxes of the entries for rubber-band selection (features/rubberband.js, F-700); none in the tree view
  p.bandGeo = () => {
    const m = modeOf(p);
    if (m === 'tree') return null;
    if (m === 'brief') { const g = briefGeo(p); return { kind: 'brief', rows: g.rows, colW: g.colW, rowH: p.rowH }; }
    if (m === 'thumbs') { const g = thumbGeo(p); return { kind: 'grid', cols: g.cols, tileW: g.tileW, tileH: g.tileH }; }
    return { kind: 'rows', rowH: p.rowH, width: Math.max(p.$.list.scrollWidth, p.$.list.clientWidth) };
  };
  p.sideStep = () => {
    const m = modeOf(p);
    if (m === 'brief') return briefGeo(p).rows;
    if (m === 'thumbs') return 1;
    return p.pageSize();
  };
  p.ensureVisible = (center = false) => {
    const m = modeOf(p), list = p.$.list;
    if (m === 'brief') {
      const g = briefGeo(p);
      const x = Math.floor(p.cursor / g.rows) * g.colW, view = list.clientWidth;
      if (view) {
        if (center && (x < list.scrollLeft || x + g.colW > list.scrollLeft + view)) list.scrollLeft = Math.max(0, x - view / 2);
        else if (x < list.scrollLeft) list.scrollLeft = x;
        else if (x + g.colW > list.scrollLeft + view) list.scrollLeft = x + g.colW - view;
      }
      return renderBrief(p);
    }
    if (m === 'thumbs') {
      const g = thumbGeo(p);
      const y = Math.floor(p.cursor / g.cols) * g.tileH, view = list.clientHeight;
      if (view) {
        if (center && (y < list.scrollTop || y + g.tileH > list.scrollTop + view)) list.scrollTop = Math.max(0, y - view / 2);
        else if (y < list.scrollTop) list.scrollTop = y;
        else if (y + g.tileH > list.scrollTop + view) list.scrollTop = y + g.tileH - view;
      }
      return renderThumbs(p);
    }
    if (m === 'tree') return treeEnsureVisible(p, center);
    return core.ensureVisible(center);
  };
  p.rebuild = (focusName = p.cur?.name, fallbackIndex = p.cursor) => {
    core.rebuild(focusName, fallbackIndex);
    afterRebuild(p, focusName);
  };
  // in the tree, a file command works on the folder chosen in the tree (never on what the hidden list has)
  p.targets = () => {
    if (modeOf(p) !== 'tree') return core.targets();
    const n = vsOf(p).tree?.nodes[vsOf(p).tree.at];
    if (!n || n.depth === 0) return [];
    const e = p.cur;
    return e && !e.up && e.name === n.name && treeParent(n)?.path === p.path ? [e] : [];
  };
  // the header of a column view sorts by its columns; right-click lists the column sets
  p.$.header.addEventListener('click', (ev) => {
    const c = ev.target.closest('[data-vcol]')?.dataset.vcol;
    if (c === undefined) return;
    p.app.setActive(p);
    sortByHeader(p, c, ev.ctrlKey || ev.metaKey);
  });
  p.$.header.addEventListener('contextmenu', (ev) => {
    ev.preventDefault();
    p.app.setActive(p);
    columnSetMenu(p.app, p, { x: ev.clientX, y: ev.clientY }, { header: true });
  });
  // Brief scrolls sideways with the wheel; Ctrl+wheel changes the size of thumbnails
  p.$.list.addEventListener('wheel', (ev) => {
    const m = modeOf(p);
    if (m === 'brief' && !ev.shiftKey && Math.abs(ev.deltaY) > Math.abs(ev.deltaX)) {
      ev.preventDefault();
      p.$.list.scrollLeft += ev.deltaY;
    } else if (m === 'thumbs' && (ev.ctrlKey || ev.metaKey)) {
      ev.preventDefault();
      setThumbSize(thumbSize() + (ev.deltaY < 0 ? 16 : -16));
    }
  }, { passive: false });
  p.$.list.addEventListener('mousedown', (ev) => treeMouse(p, ev));
  p.$.list.addEventListener('dblclick', (ev) => { if (modeOf(p) === 'tree' && ev.target.closest('.mc-views-node')) treeEnter(p); });
}

// sortByHeader sorts by a column title; add (Ctrl/Cmd+click, F-114) makes it the second sort key where it can be one.
function sortByHeader(p, c, add = false) {
  const def = columns(p);
  if (add && (c === 'name' || c === 'ext')) return p.addSortKey(c);
  if (c === 'name') return p.app.run('cm_SrcByName');
  if (c === 'ext') return p.app.run('cm_SrcByExt');
  const col = def.cols[+c];
  if (!col) return;
  if (add && col.sort) return p.addSortKey(col.sort);
  if (col.sort) return p.sortBy(col.sort);
  return p.sortBy(`col${+c + 1}`);
}

// afterRebuild puts the entries in the order of a custom column (the engine sorted them) and asks for new cells when
// the folder, its folder sizes or the view changed.
function afterRebuild(p, focusName) {
  const vs = vsOf(p);
  if (vs.mode === 'tree') return;
  const m = /^col(\d+)$/.exec(p.sort.key || '');
  if (isColumnMode(p)) requestCells(p);
  else if (vs.mode === 'thumbs') requestThumbCells(p);
  if (!m || !vs.cells?.order || vs.cells.path !== p.path || vs.cells.sortKey !== p.sort.key || vs.cells.desc !== p.sort.desc) return;
  const rank = new Map(vs.cells.order.map((n, i) => [n, i]));
  const up = p.entries.filter((e) => e.up);
  const rest = p.entries.filter((e) => !e.up);
  const big = vs.cells.order.length;
  rest.sort((a, b) => ((a.dir ? 0 : 1) - (b.dir ? 0 : 1)) || ((rank.get(a.name) ?? big) - (rank.get(b.name) ?? big)));
  p.entries = [...up, ...rest];
  p.index = new Map(p.entries.map((e, i) => [e.name, i]));
  const i = focusName != null ? p.index.get(focusName) : undefined;
  if (i !== undefined) p.cursor = i;
  p.renderRows();
  p.ensureVisible(true);
}

// ---- cells from the engine ----

function cellsSig(p) {
  const def = columns(p), c = cfg();
  const ds = [...p.dirSizes].sort().join('|');
  const m = /^col(\d+)$/.exec(p.sort.key || '');
  return JSON.stringify([p.path, allId(p.all), def.cols.map((x) => x.template), cellFormat(c, display), ds, m ? p.sort.key : '', m ? p.sort.desc : false, !!theApp.setting('naturalSort', true), theApp.setting('sortMethod', ''), theApp.setting('sortLocale', '')]);
}

let cellsSeq = 0;
async function requestCells(p, force = false) {
  const vs = vsOf(p);
  if (!p.path || !isColumnMode(p)) return;
  const sig = cellsSig(p);
  if (!force && sig === vs.cellsSig) return;
  vs.cellsSig = sig;
  const def = columns(p), c = cfg();
  const m = /^col(\d+)$/.exec(p.sort.key || '');
  const req = {
    path: p.path, columns: def.cols.map(({ title, template, width, align }) => ({ title, template, width, align })),
    format: cellFormat(c, display),
    dirSizes: Object.fromEntries(p.dirSizes), sortCol: m ? +m[1] - 1 : -1, desc: !!p.sort.desc, natural: !!theApp.setting('naturalSort', true),
    sortMethod: theApp.setting('sortMethod', '') || '', sortLocale: theApp.setting('sortLocale', '') || '',
  };
  const seq = ++cellsSeq;
  vs.cellsSeq = seq;
  let r;
  try {
    r = await go.Views.Cells(req);
  } catch (e) {
    if (vs.cellsSeq === seq) p.app.flash(errText(e), 'err');
    return;
  }
  if (vs.cellsSeq !== seq || p.vs !== vs || r.path !== p.path) return; // a newer request, or the panels were swapped
  vs.cells = { path: r.path, map: new Map(Object.entries(r.cells || {})), order: r.order || null, sortKey: m ? p.sort.key : '', desc: !!p.sort.desc, titles: r.titles, local: r.local };
  if (m && r.order) p.rebuild(); // re-sort by the column (the signature stays the same, so this asks for nothing new)
  else p.renderRows();
}

// ---- switching views ----

// save remembers the views both panels were set to (not the ones a folder rule chose).
function save() {
  if (theApp.demo) return;
  saveCfg({ left: vsOf(theApp.left).manual, right: vsOf(theApp.right).manual });
}

// applyView draws a panel again after its view changed.
function applyView(p) {
  const vs = vsOf(p), { list, spacer, rows } = p.$;
  if (vs.mode !== 'thumbs') stopThumbs(p);
  spacer.style.width = '';
  rows.style.width = '';
  if (vs.mode !== 'brief') list.scrollLeft = 0;
  if (vs.mode === 'brief') list.scrollTop = 0;
  rows.innerHTML = '';
  const m = /^col(\d+)$/.exec(p.sort.key || '');
  if (m && !(vs.mode === 'custom' || vs.mode === 'comments')) p.sort = { key: 'name', desc: false };
  vs.cellsSig = '';
  p.rebuild();
  if (vs.mode === 'thumbs') startThumbs(p);
  p.app.panelChanged(p, { moved: false });
}

// setMode switches a panel to a view; choosing the view it is in does nothing (TC), except a different column set.
function setMode(p, mode, set = '') {
  const vs = vsOf(p);
  if (vs.mode === 'tree' && mode !== 'tree') return leaveTree(p, { mode, set, toNode: true });
  if (mode === 'custom') {
    if (!findSet(set)) { p.app.flash(`There is no column set "${set}".`, 'err'); return false; }
    if (vs.mode === 'custom' && vs.set === set) return false;
  } else if (vs.mode === mode) return false;
  vs.mode = mode;
  vs.set = mode === 'custom' ? set : '';
  vs.manual = { mode, set: vs.set };
  vs.auto = false;
  vs.viewMode = ''; vs.modeAuto = null; // (chosen by hand: no view mode any more)
  modeLook(p, null);
  applyView(p);
  save();
  return true;
}

function customByNumber(p, n) {
  const s = sets()[n - 1];
  if (!s) { p.app.flash(sets().length ? `There is no column set ${n}.` : 'There are no column sets yet. Make one in Configuration > Custom columns.', 'err'); return false; }
  return setMode(p, 'custom', s.name);
}

function stepCustom(p, d) {
  const all = sets();
  if (!all.length) { p.app.flash('There are no column sets yet. Make one in Configuration > Custom columns.', 'err'); return false; }
  const vs = vsOf(p);
  const i = vs.mode === 'custom' ? all.findIndex((s) => s.name === vs.set) : (d > 0 ? -1 : all.length);
  return setMode(p, 'custom', all[(i + d + all.length) % all.length].name);
}

// the Shift+F1 popup and the header's right-click menu; the header's menu also lists the view's columns to show or hide
// and the column width commands (F-672, F-673)
async function columnSetMenu(app, p, anchor, { header = false } = {}) {
  const vs = vsOf(p);
  const items = [{ label: 'Full (default)', detail: vs.mode === 'full' ? '✓' : '', value: { mode: 'full' }, key: '0' }];
  sets().forEach((s, i) => items.push({ label: s.name, detail: vs.mode === 'custom' && vs.set === s.name ? '✓' : '', value: { mode: 'custom', set: s.name }, key: i < 9 ? String(i + 1) : '' }));
  if (header) {
    const cols = columnToggles(p);
    if (cols.length) items.push({ sep: true }, ...cols);
    items.push({ sep: true },
      { label: 'Fit column widths to contents', value: { cmd: 'cm_ColumnsFitWidth' } },
      { label: 'Equal column widths', value: { cmd: 'cm_ColumnsEqualWidth' } },
      { label: 'Default column widths', value: { cmd: 'cm_ColumnsDefaultWidth' } });
  }
  items.push({ sep: true }, { label: 'Configure custom columns…', value: { config: true } });
  const r = await showList({ title: 'Column sets', items, anchor: anchor || p.$.header, kind: 'column-sets' });
  if (!r) return false;
  const v = r.item.value;
  if (v.config) return app.run('cm_CustomColumnConfig');
  if (v.cmd) return app.run(v.cmd);
  if (v.toggle) return toggleColumn(p, v.toggle);
  return setMode(p, v.mode, v.set);
}

// columnToggles lists the columns of the panel's view (not the name) as popup items, checked when shown.
function columnToggles(p) {
  const key = viewKey(vsOf(p));
  if (!key || modeOf(p) === 'tree') return [];
  const def = allColumns(p), hidden = hiddenNow(p);
  const titles = [...(def.ext ? ['Ext'] : []), ...def.cols.map((c) => c.title || '')].filter((t, i, a) => t && a.indexOf(t) === i);
  return titles.map((t) => ({ label: `Column: ${t}`, detail: hidden.includes(t) ? '' : '✓', value: { toggle: t } }));
}

// toggleColumn hides a column of the panel's view, or shows it again; both panels showing that view follow.
function toggleColumn(p, title) {
  const key = viewKey(vsOf(p));
  if (!key) return false;
  saveCfg({ hiddenCols: toggleHidden(cfg().hiddenCols, key, title) });
  for (const q of p.app.panels) {
    const qs = vsOf(q);
    if (viewKey(qs) !== key) continue;
    if (/^col\d+$/.test(q.sort.key || '')) q.sort = { key: 'name', desc: false }; // (the column numbers moved)
    q.el.style.removeProperty('--cols');
    applyView(q);
  }
  return true;
}

// showHideColumns is Show > Show/hide columns…: the view's columns, choosing one shows or hides it (cm_ColumnsShowHide).
async function showHideColumns(app, p) {
  const items = columnToggles(p);
  if (!items.length) { app.flash('This view has no columns to show or hide.', 'err'); return false; }
  const r = await showList({ title: 'Show or hide columns', items, anchor: p.$.header, kind: 'column-toggles' });
  if (!r) return false;
  return toggleColumn(p, r.item.value.toggle);
}

// folder rules: a column set with a folder mask is chosen when such a folder is shown (F-668). With the setting
// "columnsAutoSwitch" off (Options > Custom columns > Switch column sets by folder; on when not set) no set is chosen by
// folder, and a panel showing one that was chosen that way goes back to its own view at the next folder.
let autoSeq = 0;
async function autoSwitch(p) {
  const vs = vsOf(p);
  if (vs.mode === 'tree') return;
  const all = theApp.setting('columnsAutoSwitch', true) === false ? [] : sets().filter((s) => (s.autoSwitch || '').trim());
  if (!all.length && !vs.auto) return;
  const seq = ++autoSeq;
  vs.autoSeq = seq;
  let i = -1;
  if (all.length) { try { i = await go.Views.AutoColumnSet(p.path, all.map((s) => s.autoSwitch)); } catch { i = -1; } }
  if (vs.autoSeq !== seq || p.vs !== vs) return;
  if (i >= 0) {
    if (vs.mode === 'custom' && vs.set === all[i].name) return;
    vs.mode = 'custom';
    vs.set = all[i].name;
    vs.auto = true;
    applyView(p);
  } else if (vs.auto) {
    vs.mode = vs.manual.mode;
    vs.set = vs.manual.set;
    vs.auto = false;
    applyView(p);
  }
}

// ---- thumbnails ----

// stopThumbs forgets a panel's thumbnails; tellGo stops the work in Go too (not needed when a new batch follows at
// once: Views.Thumbs stops the one before, and a separate stop could arrive after the new start).
function stopThumbs(p, tellGo = true) {
  const vs = vsOf(p);
  if (!vs.thumbs) return;
  clearTimeout(vs.thumbs.firstTimer);
  vs.thumbs = null;
  if (tellGo) go.Views.ThumbsStop(p.side).catch(() => {});
}

// requestThumbCells asks the engine for the field lines under the thumbnails (Thumbnails.FieldLines, F-679): each line
// is a column template, so [=tc.size.k] or [=img.width]×[=img.height] work as in a column set. No lines: nothing asked,
// and the tiles show the size or dimensions as before.
let thumbCellsSeq = 0;
async function requestThumbCells(p) {
  const vs = vsOf(p);
  const lines = thumbLines();
  if (vs.mode !== 'thumbs' || !p.path || !lines.length) { vs.thumbCells = null; vs.thumbCellsSig = ''; return; }
  const c = cfg();
  const sig = JSON.stringify([p.path, allId(p.all), lines, cellFormat(c, display), [...p.dirSizes].sort().join('|')]);
  if (sig === vs.thumbCellsSig) return;
  vs.thumbCellsSig = sig;
  if (!vs.thumbCells || vs.thumbCells.path !== p.path) vs.thumbCells = { path: p.path, lines, map: new Map() };
  vs.thumbCells.lines = lines;
  const seq = ++thumbCellsSeq;
  vs.thumbCellsSeq = seq;
  let r;
  try {
    r = await go.Views.Cells({
      path: p.path, columns: lines.map((template) => ({ title: '', template, width: 0, align: 'center' })), format: cellFormat(c, display),
      dirSizes: Object.fromEntries(p.dirSizes), sortCol: -1, desc: false, natural: true, sortMethod: '', sortLocale: '',
    });
  } catch (e) {
    if (vs.thumbCellsSeq === seq) p.app.flash(errText(e), 'err');
    return;
  }
  if (vs.thumbCellsSeq !== seq || p.vs !== vs || r.path !== p.path || vs.mode !== 'thumbs') return;
  vs.thumbCells = { path: r.path, lines, map: new Map(Object.entries(r.cells || {})) };
  p.renderRows();
}

async function startThumbs(p) {
  const vs = vsOf(p);
  requestThumbCells(p);
  if (vs.mode !== 'thumbs' || !p.path) return;
  const s = thumbSize();
  const key = JSON.stringify([p.path, allId(p.all), s, p.side]);
  if (vs.thumbs?.key === key) return;
  stopThumbs(p, false);
  const t = { key, gen: 0, urls: new Map(), dims: new Map(), icons: null };
  vs.thumbs = t;
  // Options > Thumbnails: which file types get one (empty: every type that can), files above the size limit keep their
  // icon, network folders only when allowed, and folders show a picture from inside them when asked
  const ov = (k, d) => theApp.config?.values?.[k] ?? d;
  const types = String(ov('Thumbnails.FileTypes', '')).trim();
  const typeOK = types ? compileMask(types) : () => true;
  const maxB = (Number(ov('Thumbnails.MaxFileMB', '200')) || 200) * 1048576;
  const vol = theApp.volumeFor(p.path);
  const netOK = ov('Thumbnails.ShowOnNet', '1') !== '0' || !(vol && (vol.kind === 'network' || vol.net));
  const folders = ov('Thumbnails.FolderThumb', '0') === '1';
  const names = !netOK ? [] : p.entries.filter((e) => !e.up && (e.dir ? folders : typeOK(e.name, false) && e.size <= maxB)).map((e) => e.name);
  try {
    const [icons, gen] = await Promise.all([
      go.Views.Icons(p.entries.map((e) => e.name), p.entries.map((e) => !!e.dir), Math.min(s, 128)),
      go.Views.Thumbs(p.side, p.path, names, s),
    ]);
    if (vs.thumbs !== t) return;
    t.icons = icons;
    t.gen = gen;
    for (const [n, th] of pending.get(gen) || []) addThumb(t, n, th);
    pending.delete(gen);
    p.renderRows();
  } catch (e) {
    p.app.flash(errText(e), 'err');
  }
}

const pending = new Map(); // gen → [[name, thumb]] that came before Thumbs() answered
function addThumb(t, name, th) {
  if (th.url) t.urls.set(name, th.url);
  if (th.width && th.height) t.dims.set(name, [th.width, th.height]);
}

let thumbRaf = 0;
const dirtyPanels = new Set();
function onThumb(th) {
  for (const p of theApp.panels) {
    const t = vsOf(p).thumbs;
    if (!t) continue;
    if (t.gen === 0 && th.key === p.side) { // Thumbs() has not answered yet: keep it for when it does
      if (!pending.has(th.gen)) pending.set(th.gen, []);
      pending.get(th.gen).push([th.name, th]);
      if (pending.size > 8) pending.delete(pending.keys().next().value);
      return;
    }
    if (t.gen !== th.gen) continue;
    addThumb(t, th.name, th);
    dirtyPanels.add(p);
  }
  if (!thumbRaf) thumbRaf = requestAnimationFrame(() => { thumbRaf = 0; for (const p of dirtyPanels) if (modeOf(p) === 'thumbs') p.renderRows(); dirtyPanels.clear(); });
}

function setThumbSize(n) {
  n = Math.max(32, Math.min(512, Math.round(n / 8) * 8));
  if (n === thumbSize()) return;
  saveCfg({ thumbSize: n });
  for (const p of theApp.panels) if (modeOf(p) === 'thumbs') { p.rebuild(); startThumbs(p); }
  theApp.flash(`Thumbnails: ${n} × ${n} pixels.`);
}

// ---- the tree in the panel (Ctrl+F8) ----

const treeParent = (n) => n.parentNode || null;
let treeSeq = 0;

async function readFolders(p, path) {
  const l = await go.FS.List(`views-tree-${p.side}-${++treeSeq}`, path);
  const sep = theApp.sepFor(l.path);
  const join = (n) => (l.path.endsWith(sep) || l.path.endsWith('/') ? l.path + n : l.path + sep + n);
  const cmp = nameComparer({ natural: !!theApp.setting('naturalSort', true), method: theApp.setting('sortMethod', ''), locale: theApp.setting('sortLocale', '') });
  // hidden folders stay out unless shown, except the ones on the way to the folder the panel was in (macOS hides
  // /private, which holds every temp folder), so the tree can always reach and select that folder
  // Options > Tree: "Show hidden folders in the tree" shows them whatever the panels do; "Show files in the tree" adds
  // the files of each folder after its subfolders
  const keep = vsOf(p).tree?.keep;
  const hidden = theApp.showHidden || treeOpt('Tree.ShowHidden', '0') === '1';
  const ok = (r) => hidden || !(r[3] & 4) || keep?.has(join(r[0]).toLowerCase());
  const dirs = (l.entries || []).filter((r) => (r[3] & 1) && ok(r)).map((r) => r[0]).sort(cmp).map((n) => ({ name: n, path: join(n) }));
  if (treeOpt('Tree.ShowFiles', '0') !== '1') return dirs;
  const files = (l.entries || []).filter((r) => !(r[3] & 1) && ok(r)).map((r) => r[0]).sort(cmp).map((n) => ({ name: n, path: join(n), file: true }));
  return dirs.concat(files);
}

const treeOpt = (k, d) => theApp.config?.values?.[k] ?? d;

function flatten(tree) {
  const out = [];
  const walk = (n) => { out.push(n); if (n.open && n.kids) for (const k of n.kids) walk(k); };
  walk(tree.root);
  tree.nodes = out;
}

async function expand(p, n) {
  if (!n.kids) {
    n.loading = true;
    try {
      const kids = await readFolders(p, n.path);
      n.kids = kids.map((k) => ({ ...k, depth: n.depth + 1, parentNode: n, open: false, kids: k.file ? [] : null }));
    } catch (e) {
      n.kids = [];
      p.app.flash(errText(e), 'err');
    }
    n.loading = false;
  }
  n.open = true;
}

async function enterTree(p) {
  const vs = vsOf(p);
  const startPath = p.path, startName = p.cur?.up ? null : p.cur?.name;
  let root;
  try { root = await go.FS.Root(p.path); } catch { root = splitPath(p.path)[0]?.path || p.path; }
  const chain = splitPath(startPath).map((x) => x.path).filter((x) => x.length > root.length);
  const tree = { root: { name: root, path: root, depth: 0, open: false, kids: null, parentNode: null }, nodes: [], at: 0, back: { path: startPath, focus: startName }, sync: 0, keep: new Set(chain.map((x) => x.toLowerCase())) };
  vs.prev = { mode: vs.mode, set: vs.set };
  stopThumbs(p);
  vs.mode = 'tree';
  vs.tree = tree;
  p.clearSelection();
  // open the way down to the folder the panel shows
  let node = tree.root;
  await expand(p, node);
  for (const want of chain) {
    const k = node.kids?.find((c) => c.path === want || c.path.toLowerCase() === want.toLowerCase());
    if (!k) break;
    node = k;
    if (want !== chain[chain.length - 1]) await expand(p, node);
  }
  flatten(tree);
  tree.at = Math.max(0, tree.nodes.indexOf(node));
  p.$.list.scrollLeft = 0;
  p.$.spacer.style.width = '';
  renderHeader(p);
  p.render();
  treeEnsureVisible(p, true);
  treeSync(p, true);
  p.app.panelChanged(p, { moved: false });
}

// leaveTree goes back to the list: into the folder chosen in the tree (Enter), or back where the panel was (Ctrl+F8).
async function leaveTree(p, { mode, set = '', toNode = false } = {}) {
  const vs = vsOf(p), tree = vs.tree;
  if (!tree) return false;
  const n = tree.nodes[tree.at];
  const prev = vs.prev || { mode: 'full', set: '' };
  vs.mode = mode || prev.mode;
  vs.set = vs.mode === 'custom' ? (set || prev.set) : '';
  if (vs.mode === 'custom' && !findSet(vs.set)) { vs.mode = 'full'; vs.set = ''; }
  if (mode) vs.manual = { mode: vs.mode, set: vs.set };
  vs.tree = null;
  vs.prev = null;
  clearTimeout(tree.syncTimer);
  p.$.rows.innerHTML = '';
  renderHeader(p);
  if (toNode && n?.file) await p.go(treeParent(n)?.path || tree.back.path, { focus: n.name });
  else if (toNode && n) await p.go(n.path);
  else await p.go(tree.back.path, { focus: tree.back.focus, history: false });
  applyView(p);
  save();
  return true;
}

// treeSync shows, under the tree, the folder above the chosen one with the cursor on it, so F5, F6 and F8 work on the
// folder chosen in the tree.
function treeSync(p, now = false) {
  const tree = vsOf(p).tree;
  if (!tree) return;
  clearTimeout(tree.syncTimer);
  const run = () => {
    const n = tree.nodes[tree.at];
    if (!n || vsOf(p).tree !== tree) return;
    const par = treeParent(n);
    if (!par) p.go(n.path, { history: false });
    else p.go(par.path, { focus: n.name, history: false }); // (a file in the tree: its folder, with the cursor on it)
  };
  if (now) run(); else tree.syncTimer = setTimeout(run, 150);
}

function renderTree(p) {
  const { list, rows, spacer } = p.$;
  const tree = vsOf(p).tree;
  if (!tree) { rows.innerHTML = ''; return; }
  const h = p.rowH;
  spacer.style.height = `${tree.nodes.length * h}px`;
  spacer.style.width = '1px';
  const first = Math.max(0, Math.floor(list.scrollTop / h) - 3);
  const last = Math.min(tree.nodes.length, first + Math.ceil((list.clientHeight || 600) / h) + 7);
  let html = '';
  for (let i = first; i < last; i++) {
    const n = tree.nodes[i];
    const tw = n.loading ? '…' : n.kids && n.kids.length === 0 ? '' : n.open ? '▾' : '▸';
    html += `<div class="mc-views-node${n.file ? '' : ' dir'}${i === tree.at ? ' cur' : ''}" data-t="${i}" style="padding-left:${8 + n.depth * 16}px"${i === tree.at && p.isActive ? ' aria-selected="true"' : ''}><span class="mc-views-tw" data-tw="${i}">${tw}</span><i class="ic"></i><b>${esc(n.depth === 0 ? n.path : n.name)}</b></div>`;
  }
  rows.style.transform = `translateY(${first * h}px)`;
  rows.style.width = '';
  rows.innerHTML = html;
}

function renderTreeStatus(p) {
  const tree = vsOf(p).tree;
  const n = tree?.nodes[tree.at];
  if (n) p.$.status.textContent = `Tree: ${n.path}  (Enter opens it, Ctrl+F8 goes back)`;
}

function treeEnsureVisible(p, center = false) {
  const tree = vsOf(p).tree, list = p.$.list, h = p.rowH;
  if (!tree) return;
  const y = tree.at * h, view = list.clientHeight;
  if (view) {
    if (center && (y < list.scrollTop || y + h > list.scrollTop + view)) list.scrollTop = Math.max(0, y - view / 2);
    else if (y < list.scrollTop) list.scrollTop = y;
    else if (y + h > list.scrollTop + view) list.scrollTop = y + h - view;
  }
  renderTree(p);
}

function treeMove(p, i) {
  const tree = vsOf(p).tree;
  i = Math.max(0, Math.min(tree.nodes.length - 1, i));
  if (i === tree.at) return;
  tree.at = i;
  treeEnsureVisible(p);
  renderTreeStatus(p);
  treeSync(p);
}

async function treeToggle(p, n, open) {
  const tree = vsOf(p).tree;
  if (open) { n.loading = true; renderTree(p); await expand(p, n); } else n.open = false;
  if (vsOf(p).tree !== tree) return;
  const cur = tree.nodes[tree.at];
  flatten(tree);
  tree.at = Math.max(0, tree.nodes.indexOf(cur));
  treeEnsureVisible(p);
}

function treeEnter(p) {
  return leaveTree(p, { toNode: true });
}

function treeKey(ev, combo) {
  const p = theApp.cur;
  if (!p || modeOf(p) !== 'tree') return false;
  const ae = document.activeElement;
  if (ae && ae !== document.body && ae !== p.$.list) return false;
  const tree = vsOf(p).tree;
  if (!tree) return false;
  const n = tree.nodes[tree.at];
  const page = Math.max(1, Math.floor(p.$.list.clientHeight / p.rowH) - 1);
  switch (combo) {
    case 'Up': treeMove(p, tree.at - 1); return true;
    case 'Down': treeMove(p, tree.at + 1); return true;
    case 'PgUp': treeMove(p, tree.at - page); return true;
    case 'PgDn': treeMove(p, tree.at + page); return true;
    case 'Home': treeMove(p, 0); return true;
    case 'End': treeMove(p, tree.nodes.length - 1); return true;
    case 'Left':
      if (n.open && n.kids?.length) treeToggle(p, n, false);
      else if (treeParent(n)) treeMove(p, tree.nodes.indexOf(treeParent(n)));
      return true;
    case 'Backspace':
      if (treeParent(n)) treeMove(p, tree.nodes.indexOf(treeParent(n)));
      return true;
    case 'Right':
      if (!n.open) treeToggle(p, n, true);
      else if (n.kids?.length) treeMove(p, tree.at + 1);
      return true;
    case 'Space': treeToggle(p, n, !n.open); return true;
    case 'Enter': treeEnter(p); return true;
    default: break;
  }
  // a letter jumps to the next folder whose name starts with it (quick search in the tree)
  if (ev.key && ev.key.length === 1 && !ev.ctrlKey && !ev.metaKey && !ev.altKey && ev.key !== ' ') {
    const c = ev.key.toLowerCase(), len = tree.nodes.length;
    for (let j = 1; j <= len; j++) {
      const k = (tree.at + j) % len;
      if (tree.nodes[k].name.toLowerCase().startsWith(c)) { treeMove(p, k); break; }
    }
    return true;
  }
  return false;
}

function treeMouse(p, ev) {
  if (modeOf(p) !== 'tree' || ev.button !== 0) return;
  const tree = vsOf(p).tree;
  const el = ev.target.closest('.mc-views-node');
  if (!tree || !el) return;
  const i = +el.dataset.t;
  if (ev.target.closest('[data-tw]')) { const n = tree.nodes[i]; treeToggle(p, n, !n.open); return; }
  treeMove(p, i);
  // Options > Tree > Expand folders on single click
  const n = tree.nodes[i];
  if (n && !n.file && !n.open && treeOpt('Tree.AutoExpand', '0') === '1') treeToggle(p, n, true);
}

// treeFollow moves the tree of panel p to a folder the other panel went to (Options > Tree > Tree follows the current
// folder), opening the way down to it; folders on a network volume or a server only with "Read network folders in the
// tree" on, as that reads every folder on the way.
async function treeFollow(p, path) {
  const tree = vsOf(p).tree;
  if (!tree || !path || treeOpt('Tree.FollowFolder', '1') !== '1') return;
  const v = theApp.volumeFor?.(path);
  const net = /^[a-z][a-z0-9+.-]*:\/\//i.test(path) || (v && (v.kind === 'network' || v.net));
  if (net && treeOpt('Tree.ShowNetwork', '0') !== '1') return;
  const root = tree.root.path;
  const low = (s) => (theApp.platform === 'windows' ? s.toLowerCase() : s);
  if (low(path) !== low(root) && !low(path).startsWith(low(root.endsWith(theApp.sepFor(root)) || root.endsWith('/') ? root : root + theApp.sepFor(root)))) return;
  const chain = splitPath(path).map((x) => x.path).filter((x) => x.length > root.length);
  let node = tree.root;
  if (!node.open) await expand(p, node);
  for (const want of chain) {
    const k = node.kids?.find((c) => !c.file && low(c.path) === low(want));
    if (!k) break;
    node = k;
    if (want !== chain[chain.length - 1] && !node.open) await expand(p, node);
  }
  if (vsOf(p).tree !== tree) return;
  flatten(tree);
  tree.at = Math.max(0, tree.nodes.indexOf(node));
  treeEnsureVisible(p, true);
  renderTreeStatus(p);
}

// rescanTree reads again every open folder of the tree of panel p (Commands > Refresh folder tree, cm_RescanTree,
// F-369), so folders made or deleted since they were opened show up or go away; open folders stay open, closed ones are
// read again when they are opened, and the cursor stays on its folder (or the nearest one above it that is left).
async function rescanTree(p) {
  const tree = vsOf(p).tree;
  if (!tree) return false;
  const low = (s) => (theApp.platform === 'windows' ? s.toLowerCase() : s);
  const at = tree.nodes[tree.at]?.path;
  const walk = async (n) => {
    if (n.file) return;
    if (!n.open) { n.kids = null; return; }
    let fresh;
    try { fresh = await readFolders(p, n.path); } catch (e) { p.app.flash(errText(e), 'err'); return; }
    n.kids = mergeKids(n, fresh, low);
    for (const k of n.kids) await walk(k);
  };
  await walk(tree.root);
  if (vsOf(p).tree !== tree) return false;
  flatten(tree);
  tree.at = cursorAfter(tree.nodes, at, low);
  treeEnsureVisible(p);
  renderTreeStatus(p);
  return true;
}

// ---- comments (Ctrl+Z) ----

async function editComment(app) {
  const p = app.cur, e = p.cur;
  if (!e || e.up) { app.flash('Put the cursor on a file or folder to comment on.'); return false; }
  let text;
  try { text = await go.Views.Comment(p.path, e.name); } catch (err) { app.flash(errText(err), 'err'); return false; }
  const r = await formDialog('Change comment',
    `<div class="mc-row"><label for="comment">Comment for ${esc(e.name)}:</label><textarea id="comment" class="mc-views-ta" rows="4" spellcheck="false" autofocus data-select="0,${text.length}">${esc(text)}</textarea></div>`,
    (box) => ({ text: box.querySelector('#comment').value }), // (a bare string would be read as a validation error)
    (box) => {
      const ta = box.querySelector('#comment');
      // Ctrl+Enter / Cmd+Enter saves, like the OK button (Enter makes a new line)
      ta.addEventListener('keydown', (ev) => { if (ev.key === 'Enter' && (ev.ctrlKey || ev.metaKey)) { ev.preventDefault(); box.requestSubmit(); } });
    },
    { kind: 'edit-comment' });
  if (r === null) return false;
  const ctext = r.text;
  try { await go.Views.SetComment(p.path, e.name, ctext); } catch (err) { app.flash(errText(err), 'err'); return false; }
  // (the reread brings descript.ion in or out of the list and new cells; the other panel may show the same folder)
  for (const q of app.panels) if (q !== p && q.path === p.path && isColumnMode(q)) requestCells(q, true);
  await p.reload();
  app.flash(ctext.trim() ? `Comment saved for ${e.name}.` : `Comment removed from ${e.name}.`);
  return true;
}

// ---- options dialogs ----

const opt = (list, v) => list.map(([k, l]) => `<option value="${esc(k)}"${k === v ? ' selected' : ''}>${esc(l)}</option>`).join('');

async function viewOptions(app) {
  const c = cfg();
  const known = DATE_FORMATS.some(([k]) => k === (c.dateFormat || ''));
  const html = `
    <div class="mc-row"><label for="sizeFormat">File sizes</label><select id="sizeFormat">${opt(SIZE_FORMATS, c.sizeFormat || 'bytes')}</select></div>
    <div class="mc-row"><label for="dateFormat">Dates</label><select id="dateFormat">${opt([...DATE_FORMATS, ['custom', 'Custom…']], known ? c.dateFormat || '' : 'custom')}</select></div>
    <div class="mc-row" id="mc-views-custom-date"${known ? ' hidden' : ''}><label for="datePicture">Custom date format (d, M, yyyy, H, hh, mm, ss, tt)</label><input id="datePicture" type="text" spellcheck="false" value="${esc(known ? '' : c.dateFormat)}"></div>
    <div class="mc-row"><label for="attrFormat">Attributes</label><select id="attrFormat">${opt(ATTR_FORMATS, c.attrFormat || '')}</select></div>
    <label class="mc-check"><input type="checkbox" id="separateExt"${c.namesWhole ? '' : ' checked'}> Show the extension in its own column</label>
    <label class="mc-check"><input type="checkbox" id="naturalSort"${app.setting('naturalSort', true) ? ' checked' : ''}> Natural sorting (file2 before file10)</label>
    <label class="mc-check"><input type="checkbox" id="dirsByName"${app.setting('dirsLikeFiles', false) ? '' : ' checked'}> Sort folders always by name</label>
    <div class="mc-row"><label for="sortMethod">Sort names</label><select id="sortMethod">${opt(SORT_METHODS, app.setting('sortMethod', '') || '')}</select></div>
    <div class="mc-row"><label for="sortLocale">Language for sorting (for example hu, de, sv; empty: the system's)</label><input id="sortLocale" type="text" spellcheck="false" value="${esc(app.setting('sortLocale', '') || '')}"></div>
    <div class="mc-row"><label for="thumbSize">Thumbnail size in pixels (32 to 512)</label><input id="thumbSize" type="text" inputmode="numeric" value="${thumbSize()}"></div>`;
  const r = await formDialog('View options', html, (box) => {
    const v = (id) => box.querySelector(`#${id}`);
    let date = v('dateFormat').value;
    if (date === 'custom') {
      date = v('datePicture').value.trim();
      if (!date) return 'Type a date format, for example dd.MM.yyyy HH:mm.';
    }
    const ts = parseInt(v('thumbSize').value, 10);
    if (!(ts >= 32 && ts <= 512)) return 'The thumbnail size must be between 32 and 512 pixels.';
    const loc = v('sortLocale').value.trim();
    if (loc && !validLocale(loc)) return `"${loc}" is not a language code. Type one like hu, de or en-GB, or leave it empty.`;
    return { sizeFormat: v('sizeFormat').value, dateFormat: date, attrFormat: v('attrFormat').value, namesWhole: !v('separateExt').checked, thumbSize: ts, natural: v('naturalSort').checked, dirsByName: v('dirsByName').checked, sortMethod: v('sortMethod').value, sortLocale: loc };
  }, (box) => {
    const sel = box.querySelector('#dateFormat');
    sel.addEventListener('change', () => { box.querySelector('#mc-views-custom-date').hidden = sel.value !== 'custom'; });
  }, { kind: 'view-options' });
  if (!r) return false;
  const { natural, dirsByName, sortMethod, sortLocale, ...views } = r;
  await saveCfg(views);
  await app.setSetting('naturalSort', natural);
  await app.setSetting('dirsLikeFiles', !dirsByName);
  // only written when changed from what the app has, so a settings file that never had them keeps the defaults
  if (sortMethod !== (app.setting('sortMethod', '') || '')) await app.setSetting('sortMethod', sortMethod);
  if (sortLocale !== (app.setting('sortLocale', '') || '')) await app.setSetting('sortLocale', sortLocale);
  refreshAll(app);
  return true;
}

function refreshAll(app) {
  for (const p of app.panels) {
    const vs = vsOf(p);
    if (vs.mode === 'tree') continue;
    vs.thumbs && (vs.thumbs.key = '');
    applyView(p);
  }
}

// the column set list (Configuration > Custom columns, cm_CustomColumnConfig)
async function customColumnConfig(app) {
  let list = sets().map((s) => ({ ...s, columns: (s.columns || []).map((c) => ({ ...c })) }));
  const ok = await modal((box, done) => {
    const draw = (sel = 0) => {
      box.innerHTML = `<h2>Custom columns</h2>
        <div class="mc-views-sets">
          <select id="sets" size="8" aria-label="Column sets">${list.map((s, i) => `<option value="${i}"${i === sel ? ' selected' : ''}>${esc(s.name)}${s.autoSwitch ? `  (for ${esc(s.autoSwitch)})` : ''}</option>`).join('')}</select>
          <div class="mc-views-side"><button type="button" data-a="new">New…</button><button type="button" data-a="copy">Copy…</button><button type="button" data-a="edit">Edit…</button>
          <button type="button" data-a="delete">Delete</button><button type="button" data-a="up">Up</button><button type="button" data-a="down">Down</button></div>
        </div>
        <p class="mc-note">${list.length ? 'Shift+F1 or a right-click on the column titles chooses a set for a panel.' : 'No column sets yet: New… makes one.'}</p>
        <p class="mc-err" hidden></p>
        <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
      box.querySelector('.cancel').onclick = () => done(false);
      box.querySelectorAll('[data-a]').forEach((b) => { b.onclick = () => act(b.dataset.a); });
      box.querySelector('#sets').addEventListener('dblclick', () => act('edit'));
    };
    const at = () => { const v = box.querySelector('#sets').value; return v === '' ? -1 : +v; };
    const act = async (a) => {
      const i = at();
      if (a === 'new') {
        const s = await editSet({ name: uniqueName(list, 'Columns'), autoSwitch: '', columns: [{ title: 'Size', template: '[=tc.size]', width: 90, align: 'right' }, { title: 'Date', template: '[=tc.writedate]', width: 130, align: 'left' }] }, list, -1);
        if (s) { list.push(s); draw(list.length - 1); }
      } else if (i < 0) {
        if (a !== 'up' && a !== 'down') { const e = box.querySelector('.mc-err'); e.hidden = false; e.textContent = 'Choose a column set first.'; }
      } else if (a === 'copy') {
        const s = await editSet({ ...list[i], name: uniqueName(list, `${list[i].name} copy`), columns: list[i].columns.map((c) => ({ ...c })) }, list, -1);
        if (s) { list.push(s); draw(list.length - 1); }
      } else if (a === 'edit') {
        const s = await editSet(list[i], list, i);
        if (s) { list[i] = s; draw(i); }
      } else if (a === 'delete') {
        list.splice(i, 1);
        draw(Math.min(i, list.length - 1));
      } else if (a === 'up' && i > 0) {
        [list[i - 1], list[i]] = [list[i], list[i - 1]];
        draw(i - 1);
      } else if (a === 'down' && i < list.length - 1) {
        [list[i + 1], list[i]] = [list[i], list[i + 1]];
        draw(i + 1);
      }
    };
    box.onsubmit = (ev) => { ev.preventDefault(); done(true); };
    draw();
  }, false, { kind: 'custom-columns' });
  if (!ok) return false;
  // panels showing a set that was renamed or deleted fall back to Full
  const names = new Set(list.map((s) => s.name));
  await saveCfg({ columnSets: list });
  for (const p of app.panels) {
    const vs = vsOf(p);
    if (vs.manual.mode === 'custom' && !names.has(vs.manual.set)) vs.manual = { mode: 'full', set: '' };
    if (vs.mode === 'custom' && !names.has(vs.set)) { vs.mode = vs.manual.mode; vs.set = vs.manual.set; vs.auto = false; }
  }
  save();
  refreshAll(app);
  for (const p of app.panels) autoSwitch(p);
  return true;
}

function uniqueName(list, base) {
  let n = base, k = 2;
  while (list.some((s) => s.name === n)) n = `${base} ${k++}`;
  return n;
}

let fieldCache = null;
async function fieldList() {
  if (!fieldCache) fieldCache = await go.Views.Fields().catch(() => []);
  return fieldCache || [];
}

// editSet is the "Modify custom columns view" dialog: the set's name, the folder mask that picks it, and one line per
// column (title, field contents, width, alignment). The name column always comes first and is not listed.
function editSet(set, list, index) {
  let cols = set.columns.map((c) => ({ ...c }));
  return modal((box, done) => {
    const read = () => {
      cols = cols.map((c, i) => ({
        title: box.querySelector(`#title-${i}`).value, template: box.querySelector(`#template-${i}`).value,
        width: parseInt(box.querySelector(`#width-${i}`).value, 10) || 0, align: box.querySelector(`#align-${i}`).value,
      }));
    };
    const draw = () => {
      const name = box.querySelector('#name')?.value ?? set.name, auto = box.querySelector('#auto')?.value ?? (set.autoSwitch || '');
      box.innerHTML = `<h2>${index < 0 ? 'New column set' : 'Edit column set'}</h2>
        <div class="mc-row"><label for="name">Name of the column set</label><input id="name" type="text" spellcheck="false" value="${esc(name)}" autofocus></div>
        <div class="mc-row"><label for="auto">Use it for these folders (optional, e.g. Pictures;*/photos/*)</label><input id="auto" type="text" spellcheck="false" value="${esc(auto)}"></div>
        <div class="mc-views-grid"><span>Title</span><span>Field contents</span><span>Width</span><span>Align</span><span></span>
        ${cols.map((c, i) => `<input id="title-${i}" type="text" spellcheck="false" value="${esc(c.title)}" aria-label="Title of column ${i + 1}">
          <span class="mc-views-tpl"><input id="template-${i}" type="text" spellcheck="false" value="${esc(c.template)}" aria-label="Contents of column ${i + 1}"><button type="button" data-f="${i}" title="Add a field">…</button></span>
          <input id="width-${i}" type="text" inputmode="numeric" value="${c.width || ''}" placeholder="auto" aria-label="Width of column ${i + 1}">
          <select id="align-${i}" aria-label="Alignment of column ${i + 1}">${opt([['left', 'Left'], ['right', 'Right'], ['center', 'Center']], c.align || 'left')}</select>
          <button type="button" data-del="${i}" title="Remove this column">−</button>`).join('')}
        </div>
        <div class="mc-views-gridbtns"><button type="button" data-add="1">+ Add column</button><span class="mc-note">Fields look like [=tc.size.k] or [=tc.writedate.dd.MM.yyyy]; text around them is shown as it is.</span></div>
        <p class="mc-err" hidden></p>
        <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
      box.querySelector('.cancel').onclick = () => done(null);
      box.querySelector('[data-add]').onclick = () => { read(); cols.push({ title: 'Column', template: '[=tc.size]', width: 90, align: 'left' }); draw(); box.querySelector(`#title-${cols.length - 1}`)?.focus(); };
      box.querySelectorAll('[data-del]').forEach((b) => { b.onclick = () => { read(); cols.splice(+b.dataset.del, 1); draw(); }; });
      box.querySelectorAll('[data-f]').forEach((b) => { b.onclick = () => chooseField(box, +b.dataset.f, b); });
    };
    const err = (t) => { const e = box.querySelector('.mc-err'); e.hidden = false; e.textContent = t; };
    box.onsubmit = async (ev) => {
      ev.preventDefault();
      read();
      const name = box.querySelector('#name').value.trim();
      if (!name) return err('Give the column set a name.');
      if (list.some((s, i) => i !== index && s.name === name)) return err(`There is already a column set called "${name}".`);
      if (!cols.length) return err('Add at least one column.');
      const problem = await go.Views.CheckColumns(cols).catch((e) => errText(e));
      if (problem) return err(problem);
      done({ name, autoSwitch: box.querySelector('#auto').value.trim(), columns: cols });
    };
    draw();
  }, null, { kind: 'edit-column-set' });
}

async function chooseField(box, i, anchor) {
  const fields = await fieldList();
  const items = fields.map((f) => ({ label: `[=${f.name}]`, detail: f.help + (f.units?.length ? ` (units: ${f.units.join(', ')})` : ''), value: f.name }));
  const r = await showList({ title: 'Fields', items, anchor, kind: 'column-fields' });
  if (!r) return;
  const input = box.querySelector(`#template-${i}`);
  if (!input) return;
  input.value = (input.value ? `${input.value} ` : '') + `[=${r.item.value}]`;
  input.focus();
}

// Configuration > Thumbnails: size and the cache
async function thumbnailsConfig(app) {
  let info = { dir: '', entries: 0, bytes: 0 };
  try { info = await go.Views.ThumbCache(); } catch { /* the size can still be set */ }
  const r = await formDialog('Thumbnails',
    `<div class="mc-row"><label for="thumbSize">Thumbnail size in pixels (32 to 512)</label><input id="thumbSize" type="text" inputmode="numeric" value="${thumbSize()}" autofocus></div>
     <div class="mc-row"><label for="thumbLines">Field lines under the picture, one per line (column templates such as [=tc.size.k] or [=img.width]×[=img.height]; empty: the size or dimensions)</label><textarea id="thumbLines" rows="3" spellcheck="false">${esc(thumbLines().join('\n'))}</textarea></div>
     <p class="mc-note">Cache: ${esc(info.dir || 'none')} — ${info.entries} ${info.entries === 1 ? 'picture' : 'pictures'}, ${esc(shortSize(info.bytes))}.</p>
     <div class="mc-btns mc-views-left"><button type="button" id="mc-views-clear">Delete thumbnail cache…</button></div>`,
    (box) => {
      const ts = parseInt(box.querySelector('#thumbSize').value, 10);
      if (!(ts >= 32 && ts <= 512)) return 'The thumbnail size must be between 32 and 512 pixels.';
      const lines = box.querySelector('#thumbLines').value.split(/\r?\n/).map((x) => x.trim()).filter(Boolean);
      if (lines.length > 4) return 'At most four field lines can be shown under a thumbnail.';
      return { thumbSize: ts, lines };
    },
    (box) => { box.querySelector('#mc-views-clear').onclick = async () => { if (await clearThumbCache(app)) box.querySelector('.mc-note').textContent = 'The thumbnail cache is empty.'; }; },
    { kind: 'thumbnails-config' });
  if (!r) return false;
  await setThumbLines(app, r.lines);
  setThumbSize(r.thumbSize);
  return true;
}

// setThumbLines keeps new field lines for the thumbnails (checked like column templates first) and draws the tiles again.
async function setThumbLines(app, lines) {
  if (JSON.stringify(lines) === JSON.stringify(thumbLines())) return true;
  if (lines.length) {
    let bad = '';
    try { bad = await go.Views.CheckColumns(lines.map((template, i) => ({ title: `line ${i + 1}`, template, width: 0, align: 'center' }))); } catch (e) { bad = errText(e); }
    if (bad) { app.flash(bad, 'err'); return false; }
  }
  saveCfg({ thumbLines: lines });
  for (const p of app.panels) if (modeOf(p) === 'thumbs') { p.rebuild(); requestThumbCells(p); }
  return true;
}

async function clearThumbCache(app) {
  const a = await messageDialog({ Title: 'Delete thumbnail cache', Message: 'Delete every saved thumbnail? They are made again when they are needed.', Buttons: ['Delete', 'Cancel'], Kind: 'clear-thumbs' });
  if (a !== 'Delete') return false;
  try { await go.Views.ClearThumbCache(); } catch (e) { app.flash(errText(e), 'err'); return false; }
  app.flash('The thumbnail cache was deleted.');
  for (const p of app.panels) if (modeOf(p) === 'thumbs') { vsOf(p).thumbs && (vsOf(p).thumbs.key = ''); startThumbs(p); }
  return true;
}

// ---- commands ----

const PANELS = { Src: (app) => app.cur, Left: (app) => app.left, Right: (app) => app.right, Trg: (app) => app.other };
const SIDE_WORD = { Src: '', Left: ' (left panel)', Right: ' (right panel)', Trg: ' (other panel)' };

for (const [pre, pick] of Object.entries(PANELS)) {
  const w = SIDE_WORD[pre];
  const view = (suffix, mode, label) => command({
    name: `cm_${pre}${suffix}`, label: label + w, menuKey: pre === 'Src', checked: (app) => modeOf(pick(app)) === mode,
    run: (app) => (mode === 'tree' ? toggleTree(pick(app)) : setMode(pick(app), mode)),
  });
  view('Short', 'brief', 'Brief');
  view('Long', 'full', 'Full');
  view('Brief', 'brief', 'Brief'); // (other names of Short and Long, F-079 / F-080)
  view('Full', 'full', 'Full');
  view('Comments', 'comments', 'Comments');
  view('Thumbs', 'thumbs', 'Thumbnails');
  view('Tree', 'tree', 'Tree');
  command({ name: `cm_${pre}CustomViewMenu`, menuKey: pre === 'Src', label: `Custom columns…${w}`, checked: (app) => modeOf(pick(app)) === 'custom', run: (app) => columnSetMenu(app, pick(app)) });
  for (let n = 1; n <= 9; n++) command({ name: `cm_${pre}CustomView${n}`, label: `Column set ${n}${w}`, run: (app) => customByNumber(pick(app), n) });
  command({ name: `cm_${pre}NextCustomView`, label: `Next column set${w}`, run: (app) => stepCustom(pick(app), 1) });
  command({ name: `cm_${pre}PrevCustomView`, label: `Previous column set${w}`, run: (app) => stepCustom(pick(app), -1) });
  if (pre !== 'Src') { // (the Src sort commands are the sorting feature's)
    const sort = (suffix, key, label) => command({ name: `cm_${pre}${suffix}`, label: label + w, checked: (app) => pick(app).sort.key === key, run: (app) => pick(app).sortBy(key) });
    sort('ByName', 'name', 'Sort by name');
    sort('ByExt', 'ext', 'Sort by extension');
    sort('ByDateTime', 'date', 'Sort by date');
    sort('BySize', 'size', 'Sort by size');
    command({ name: `cm_${pre}Unsorted`, label: `Unsorted${w}`, checked: (app) => pick(app).sort.key === 'none', run: (app) => pick(app).sortBy('none', false) });
    command({ name: `cm_${pre}NegOrder`, label: `Reverse order${w}`, run: (app) => { const p = pick(app); p.sortBy(p.sort.key, !p.sort.desc); } });
  }
  if (pre !== 'Trg') {
    for (let n = 1; n <= 99; n++) {
      command({
        name: `cm_${pre}SortByCol${n}`, label: `Sort by column ${n}${w}`,
        run: (app) => sortByColumn(pick(app), n),
      });
    }
  }
}

// ---- view modes (F-785, Show > View modes, cm_SrcViewModeList) ----
//
// A view mode is a named bundle of a panel's look: its view (Brief, Full, Comments, Thumbnails or a column set) and its
// sort, kept in the setting "viewModes" as [{name, mode, set, sort, folders}]. Choosing one puts the panel in it.
// With a folder mask ("folders", TC's syntax, as column sets have) and Options > View modes > Switch view modes
// automatically on, a panel takes the mode by itself in matching folders and goes back to its own view after them.
// A mode may also set the panel's icons ("icons": "on" / "off"), colours ("scheme": a colour scheme's name) and font
// size ("fontSize" in pixels) and font face ("fontFace", a font family name) while it is used (F-820, F-785);
// without them (modes saved before) the panel keeps its own.

const viewModes = () => (theApp.setting('viewModes', []) || []).filter((m) => m && m.name);
const VIEW_WORD = { brief: 'Brief', full: 'Full', comments: 'Comments', thumbs: 'Thumbnails' };
const lookWords = (m) => [m.icons === 'off' ? 'no icons' : m.icons === 'on' ? 'icons' : '', m.scheme ? `colours "${m.scheme}"` : '', +m.fontSize ? `${+m.fontSize} px font` : '', m.fontFace ? `font "${m.fontFace}"` : ''].filter(Boolean);
const describeMode = (m) => `${m.mode === 'custom' ? `columns "${m.set}"` : VIEW_WORD[m.mode] || m.mode}${m.sort?.key ? `, by ${m.sort.key}${m.sort.desc ? ' (reversed)' : ''}` : ''}${lookWords(m).length ? `, ${lookWords(m).join(', ')}` : ''}${m.folders ? `; on its own in ${m.folders}` : ''}`;

// a few common font faces offered in the view mode dialog (any installed font's name can be typed)
const FONT_FACES = ['Arial', 'Helvetica', 'Verdana', 'Tahoma', 'Segoe UI', 'Times New Roman', 'Georgia', 'Menlo', 'Monaco', 'Consolas', 'Courier New', 'SF Mono', 'DejaVu Sans', 'DejaVu Sans Mono', 'Ubuntu'];
const LOOK_VARS = ['--text', '--bg', '--sel', '--cur', '--cur-off', '--row-alt', '--font', '--mono', '--row-h'];

// modeLook puts a view mode's icons, colours and font size on its panel, and with null (or a mode without them) takes
// them off again, so the panel looks as the options say (F-820). Colours come from the built-in schemes and the
// light / dark ones of Options > Colours (Panel.schemeVars, features/config/layout.js).
// cleanFontFace is a font family name safe to put in a CSS font (no quotes, semicolons or braces), or ''.
export function cleanFontFace(s) {
  const f = String(s || '').replace(/["';{}<>\\]/g, '').replace(/\s+/g, ' ').trim();
  return f.slice(0, 80);
}

// modeFont is the CSS font of a panel in a mode with font size px (0: the usual size) and face (''; the usual face),
// built from base, the app's --font ("13px -apple-system, ..."); '' when the mode changes neither.
export function modeFont(base, px, face) {
  if (!px && !face) return '';
  const b = String(base || '').trim() || '13px -apple-system, "Segoe UI", system-ui, sans-serif';
  const m = b.match(/^(.*?)(\d+(?:\.\d+)?px)(?:\/\S+)?\s*(.*)$/);
  const pre = m ? m[1] : '', size = px ? `${px}px` : (m ? m[2] : '13px'), fam = (m ? m[3] : b) || 'system-ui, sans-serif';
  return `${pre}${size} ${face ? `"${face}", ` : ''}${fam}`;
}

export function modeLook(p, m) {
  const el = p.el;
  if (!el?.classList) return;
  const had = !!p.vs?.look;
  const icons = m?.icons === 'on' || m?.icons === 'off' ? m.icons : '';
  const vars = m?.scheme ? (Panel.schemeVars?.(m.scheme) || null) : null;
  const px = Math.round(+m?.fontSize || 0);
  const font = px >= 8 && px <= 40;
  const face = cleanFontFace(m?.fontFace);
  el.classList.toggle('mc-vm-noicons', icons === 'off');
  el.classList.toggle('mc-vm-icons', icons === 'on');
  for (const k of LOOK_VARS) el.style.removeProperty(k);
  el.style.font = ''; el.style.color = ''; el.style.backgroundColor = '';
  if (vars) {
    for (const [k, v] of Object.entries(vars)) el.style.setProperty(k, v);
    if (vars['--text']) el.style.color = vars['--text'];
    if (vars['--bg']) el.style.backgroundColor = vars['--bg'];
  }
  if (font || face) {
    const root = globalThis.getComputedStyle ? getComputedStyle(document.documentElement) : null;
    const f = modeFont(root?.getPropertyValue('--font'), font ? px : 0, face);
    el.style.setProperty('--font', f);
    el.style.font = f;
  }
  if (font) {
    const root = globalThis.getComputedStyle ? getComputedStyle(document.documentElement) : null;
    el.style.setProperty('--mono', `${px - 1}px ui-monospace, "SF Mono", Menlo, Consolas, monospace`);
    const ic = parseFloat(root?.getPropertyValue('--ic-size')) || 15; // (taller rows for large icons, as Options > Icons)
    el.style.setProperty('--row-h', `${Math.max(Math.round(px * 1.55), ic > 16 ? Math.round(ic + 6) : 0)}px`);
  }
  const look = !!(icons || vars || font || face);
  if (p.vs) p.vs.look = look;
  if (look || had) { try { p.measure(); } catch { /* not drawn yet */ } }
}

async function applyViewMode(p, m, { auto = false } = {}) {
  const vs = vsOf(p);
  if (vs.mode === 'tree') await leaveTree(p, {});
  let mode = m.mode, set = m.mode === 'custom' ? m.set : '';
  if (mode === 'custom' && !findSet(set)) { mode = 'full'; set = ''; }
  if (!auto) { vs.manual = { mode, set }; vs.modeAuto = null; }
  else if (!vs.modeAuto) vs.modeAuto = { name: m.name, back: { mode: vs.mode, set: vs.set, sort: { ...p.sort } } };
  else vs.modeAuto.name = m.name;
  vs.mode = mode;
  vs.set = set;
  vs.auto = false;
  vs.viewMode = m.name;
  modeLook(p, m);
  applyView(p);
  if (m.sort?.key) p.sortBy(m.sort.key, !!m.sort.desc);
  if (!auto) save();
  return true;
}

// moveViewModeUp is the list of view modes with the named one a place higher (the same list when it is first or absent).
export function moveViewModeUp(modes, name) {
  const i = modes.findIndex((m) => m.name === name);
  if (i <= 0) return [...modes];
  const out = [...modes];
  [out[i - 1], out[i]] = [out[i], out[i - 1]];
  return out;
}

// putViewMode is the list with m added at the end, or in place of the mode of the same name (editing it keeps its place).
export function putViewMode(modes, m) {
  const i = modes.findIndex((x) => x.name === m.name);
  return i < 0 ? [...modes, m] : modes.map((x, k) => (k === i ? m : x));
}

// replaceViewMode is the list with the mode called oldName changed to m in its place (a new name included); a mode
// of m's new name elsewhere in the list gives way to it.
export function replaceViewMode(modes, oldName, m) {
  const i = modes.findIndex((x) => x.name === oldName);
  if (i < 0) return putViewMode(modes, m);
  return modes.map((x, k) => (k === i ? m : x)).filter((x, k) => k === i || x.name !== m.name);
}

// readModeForm turns the view mode dialog's fields into a mode (keeping fields of the old one the dialog doesn't show);
// a string is the error to show.
export function readModeForm(f, old = {}) {
  const name = String(f.name || '').trim();
  if (!name) return 'Please give the view mode a name.';
  const [mode, set = ''] = String(f.view || 'full').split(':');
  const fs = String(f.fontSize ?? '').trim();
  let fontSize = 0;
  if (fs) {
    fontSize = parseInt(fs, 10);
    if (!(fontSize >= 8 && fontSize <= 40)) return 'The font size is a number from 8 to 40 (or empty: the usual size).';
  }
  const m = { ...old, name, mode, set: mode === 'custom' ? set : '', sort: f.sortKey ? { key: f.sortKey, desc: !!f.sortDesc } : {}, folders: String(f.folders || '').trim() };
  delete m.icons; delete m.scheme; delete m.fontSize; delete m.fontFace;
  if (f.icons === 'on' || f.icons === 'off') m.icons = f.icons;
  if (String(f.scheme || '').trim()) m.scheme = String(f.scheme).trim();
  if (fontSize) m.fontSize = fontSize;
  const face = cleanFontFace(f.fontFace);
  if (face) m.fontFace = face;
  return m;
}

// modeDialog asks for a view mode's settings: a new one (from the panel's view) or one to change.
function modeDialog(p, m, isNew) {
  const opt = (v, l, cur) => `<option value="${esc(v)}"${String(v) === String(cur) ? ' selected' : ''}>${esc(l)}</option>`;
  const viewNow = m.mode === 'custom' ? `custom:${m.set}` : m.mode;
  const views = [['brief', 'Brief'], ['full', 'Full'], ['comments', 'Comments'], ['thumbs', 'Thumbnails'], ...sets().map((s) => [`custom:${s.name}`, `Columns: ${s.name}`])];
  if (!views.some(([v]) => v === viewNow)) views.push([viewNow, m.mode === 'custom' ? `Columns: ${m.set} (missing)` : viewNow]);
  const sorts = [['', 'Keep the panel\'s sort'], ['name', 'Name'], ['ext', 'Extension'], ['size', 'Size'], ['date', 'Date']];
  if (m.sort?.key && !sorts.some(([v]) => v === m.sort.key)) sorts.push([m.sort.key, m.sort.key]);
  const schemes = [['', 'The usual colours'], ...(Panel.schemeNames?.() || []).map((n) => [n, n])];
  if (m.scheme && !schemes.some(([v]) => v === m.scheme)) schemes.push([m.scheme, m.scheme]);
  const html = `<div class="mc-row"><label for="vm-name">Name</label><input type="text" id="vm-name" value="${esc(m.name || '')}" autofocus></div>
    <div class="mc-row"><label for="vm-view">View</label><select id="vm-view">${views.map(([v, l]) => opt(v, l, viewNow)).join('')}</select></div>
    <div class="mc-row"><label for="vm-sort">Sort by</label><select id="vm-sort">${sorts.map(([v, l]) => opt(v, l, m.sort?.key || '')).join('')}</select></div>
    <label class="mc-check"><input type="checkbox" id="vm-desc"${m.sort?.desc ? ' checked' : ''}> Reversed</label>
    <div class="mc-row"><label for="vm-icons">Icons</label><select id="vm-icons">${[['', 'As the options say'], ['on', 'Shown'], ['off', 'Hidden']].map(([v, l]) => opt(v, l, m.icons || '')).join('')}</select></div>
    <div class="mc-row"><label for="vm-scheme">Colours</label><select id="vm-scheme">${schemes.map(([v, l]) => opt(v, l, m.scheme || '')).join('')}</select></div>
    <div class="mc-row"><label for="vm-font">Font size in pixels (empty: the usual size)</label><input type="text" id="vm-font" inputmode="numeric" value="${+m.fontSize || ''}"></div>
    <div class="mc-row"><label for="vm-face">Font face (empty: the usual font), e.g. Menlo or Arial</label><input type="text" id="vm-face" list="vm-faces" spellcheck="false" value="${esc(m.fontFace || '')}"><datalist id="vm-faces">${FONT_FACES.map((f) => `<option value="${esc(f)}">`).join('')}</datalist></div>
    <div class="mc-row"><label for="vm-folders">Use it by itself in these folders (optional mask, e.g. *\\Photos* or */Pictures/*)</label><input type="text" id="vm-folders" spellcheck="false" value="${esc(m.folders || '')}"></div>
    <p class="mc-note">Icons, colours, font size and font face apply to the panel while it is in this mode, and go back when it leaves it.</p>`;
  return formDialog(isNew ? 'New view mode' : `Change view mode "${m.name}"`, html, (box) => {
    const v = (id) => box.querySelector(id);
    return readModeForm({ name: v('#vm-name').value, view: v('#vm-view').value, sortKey: v('#vm-sort').value, sortDesc: v('#vm-desc').checked,
      icons: v('#vm-icons').value, scheme: v('#vm-scheme').value, fontSize: v('#vm-font').value, fontFace: v('#vm-face').value, folders: v('#vm-folders').value }, isNew ? {} : m);
  }, null, { okLabel: isNew ? 'Save' : 'OK', kind: 'view-mode-edit' });
}

async function viewModeList(app, p) {
  const vs = vsOf(p);
  const modes = viewModes();
  const items = modes.map((m, i) => ({ label: m.name, detail: `${vs.viewMode === m.name ? '✓ ' : ''}${describeMode(m)}`, value: { use: m }, key: i < 9 ? String(i + 1) : '' }));
  if (items.length) items.push({ sep: true });
  items.push({ label: 'Save the current view as a view mode…', value: { add: true } });
  if (modes.length) items.push({ label: 'Change a view mode…', value: { edit: true } });
  if (modes.length > 1) items.push({ label: 'Move a view mode up…', value: { up: true } });
  if (modes.length) items.push({ label: 'Remove a view mode…', value: { remove: true } });
  const r = await showList({ title: `View modes (${p.side} panel)`, items, anchor: p.$.header, kind: 'view-modes' });
  if (!r) return false;
  const v = r.item.value;
  if (v.use) { app.setActive(p); return applyViewMode(p, v.use); }
  if (v.up) {
    // the order counts: the first nine are Show > View mode 1…9 and the first matching folder mask wins (F-820)
    const r2 = await showList({ title: 'Move which view mode up?', items: modes.slice(1).map((m) => ({ label: m.name, detail: describeMode(m), value: m.name })), anchor: p.$.header, kind: 'view-modes-up' });
    if (!r2) return false;
    await app.setSetting('viewModes', moveViewModeUp(modes, r2.item.value));
    refreshMenu();
    app.flash(`View mode "${r2.item.value}" moved up.`);
    return true;
  }
  if (v.remove) {
    const r2 = await showList({ title: 'Remove which view mode?', items: modes.map((m) => ({ label: m.name, detail: describeMode(m), value: m.name })), anchor: p.$.header, kind: 'view-modes-remove' });
    if (!r2) return false;
    await app.setSetting('viewModes', modes.filter((m) => m.name !== r2.item.value));
    refreshMenu();
    app.flash(`View mode "${r2.item.value}" removed.`);
    return true;
  }
  if (v.edit) {
    const r2 = await showList({ title: 'Change which view mode?', items: modes.map((m) => ({ label: m.name, detail: describeMode(m), value: m.name })), anchor: p.$.header, kind: 'view-modes-edit' });
    if (!r2) return false;
    const old = modes.find((m) => m.name === r2.item.value);
    const m = await modeDialog(p, old, false);
    if (!m) return false;
    await app.setSetting('viewModes', replaceViewMode(modes, old.name, m));
    refreshMenu();
    // a panel in the mode takes the change at once
    for (const q of app.panels || []) if (vsOf(q).viewMode === old.name) await applyViewMode(q, m, { auto: !!vsOf(q).modeAuto });
    app.flash(`View mode "${m.name}" changed.`);
    return true;
  }
  const cur = vs.mode === 'tree' ? (vs.prev || { mode: 'full', set: '' }) : vs;
  const m = await modeDialog(p, { name: '', mode: cur.mode, set: cur.mode === 'custom' ? cur.set : '', sort: { ...p.sort }, folders: '' }, true);
  if (!m) return false;
  await app.setSetting('viewModes', putViewMode(modes, m));
  refreshMenu();
  vs.viewMode = m.name;
  if (lookWords(m).length) modeLook(p, m);
  app.flash(`View mode "${m.name}" saved.`);
  return true;
}

// autoViewMode puts a panel in the view mode whose folder mask matches its folder (Options > View modes > Switch view
// modes automatically), and back in its own view when it leaves such folders.
let modeSeq = 0;
async function autoViewMode(p) {
  const vs = vsOf(p);
  if (vs.mode === 'tree') return;
  const on = theApp.config?.values?.['ViewModes.AutoSwitch'] !== '0';
  const modes = on ? viewModes().filter((m) => (m.folders || '').trim()) : [];
  if (!modes.length && !vs.modeAuto) return;
  const seq = ++modeSeq;
  let i = -1;
  if (modes.length) { try { i = await go.Views.AutoColumnSet(p.path, modes.map((m) => m.folders)); } catch { i = -1; } }
  if (seq !== modeSeq || p.vs !== vs) return;
  if (i >= 0) { if (vs.viewMode !== modes[i].name || !vs.modeAuto) await applyViewMode(p, modes[i], { auto: true }); return; }
  if (vs.modeAuto) {
    const back = vs.modeAuto.back;
    vs.modeAuto = null;
    vs.viewMode = '';
    modeLook(p, null);
    vs.mode = back.mode;
    vs.set = back.set;
    applyView(p);
    if (back.sort?.key) p.sortBy(back.sort.key, !!back.sort.desc);
  }
}

// viewModeMenu is the Show menu's View modes submenu for the saved modes (MENUS format): the first nine, by name.
export const viewModeMenu = (modes) => (modes.length
  ? ['-', ['View mode', modes.slice(0, 9).map((m, k) => [m.name, `cm_SrcViewMode${k + 1}`])]] : []);

// viewModeByNumber puts the panel in the n-th view mode (cm_SrcViewMode1…9, ours: the Show > View modes submenu, F-819).
function viewModeByNumber(app, p, n) {
  const m = viewModes()[n - 1];
  if (!m) { app.flash(viewModes().length ? `There is no view mode ${n}.` : 'There are no view modes yet. Save one with Show > View modes….', 'err'); return false; }
  app.setActive(p);
  return applyViewMode(p, m);
}

for (const [pre, pick] of Object.entries(PANELS)) {
  for (let n = 1; n <= 9; n++) {
    command({
      name: `cm_${pre}ViewMode${n}`, label: `View mode ${n}${SIDE_WORD[pre]}`,
      checked: (app) => { const m = viewModes()[n - 1], p = pick(app); return !!m && !!p && vsOf(p).viewMode === m.name; },
      run: (app) => viewModeByNumber(app, pick(app), n),
    });
  }
  command({ name: `cm_${pre}ViewModeList`, label: `View modes…${SIDE_WORD[pre]}`, run: (app) => viewModeList(app, pick(app)), ...(pre === 'Src' ? { menu: 'Show', menuAfter: 'cm_SrcCustomViewMenu' } : {}) });
}

// sortByColumn sorts by column n of the panel's view (1 = name); again reverses (F-790).
function sortByColumn(p, n) {
  if (modeOf(p) === 'tree') return false;
  const cols = isColumnMode(p) ? columns(p) : { ext: !cfg().namesWhole && modeOf(p) === 'full', cols: modeOf(p) === 'full' ? columns(p).cols : [] };
  const keys = ['name'];
  if (cols.ext) keys.push('ext');
  cols.cols.forEach((c, k) => keys.push(c.sort || (isColumnMode(p) ? `col${k + 1}` : null)));
  const key = keys[n - 1];
  if (!key) { p.app.flash(`The view has no column ${n} to sort by.`); return false; }
  p.sortBy(key);
  return true;
}

function toggleTree(p) {
  if (modeOf(p) === 'tree') return leaveTree(p, {});
  return enterTree(p);
}

// inTreeView and setTreeView let the "separate tree" commands of features/reserved-more.js (board issue #38) put a
// panel into the tree view or take it out again, exactly as Ctrl+F8 does, without knowing how the view is kept.
export const inTreeView = (p) => !!p && modeOf(p) === 'tree';
export async function setTreeView(p, on) {
  if (!p || inTreeView(p) === !!on) return false;
  return toggleTree(p);
}

command({
  // ours (F-369): TC rescans its tree cache; here it is the tree in the panel (Ctrl+F8) that is read again
  name: 'cm_RescanTree', label: 'Refresh folder tree', menu: 'Commands', menuAfter: 'cm_CDtree',
  run: async (app) => {
    const ps = app.panels.filter((p) => inTreeView(p));
    if (!ps.length) { app.flash('No panel shows the folder tree (Ctrl+F8); the folder tree popup (Alt+F10) is always read fresh.'); return false; }
    for (const p of ps) await rescanTree(p);
    return true;
  },
});
command({ name: 'cm_EditComment', menuKey: true, label: 'Edit comment…', run: editComment });
command({
  name: 'cm_SwitchDirSort', label: 'Sort folders always by name', checked: (app) => !app.setting('dirsLikeFiles', false), menu: 'Show', menuAfter: 'cm_SrcNegOrder',
  run: async (app) => { await app.setSetting('dirsLikeFiles', !app.setting('dirsLikeFiles', false)); for (const p of app.panels) { p.rebuild(); app.panelChanged(p, { moved: false }); } },
});
command({
  // ours: TC has this only as a setting (Configuration > Display > Sorting method)
  name: 'cm_SwitchNaturalSort', label: 'Natural sorting (file2 before file10)', checked: (app) => !!app.setting('naturalSort', true), menu: 'Show', menuAfter: 'cm_SrcNegOrder',
  run: async (app) => { await app.setSetting('naturalSort', !app.setting('naturalSort', true)); for (const p of app.panels) { vsOf(p).cellsSig = ''; p.rebuild(); app.panelChanged(p, { moved: false }); } },
});
command({ name: 'cm_ViewOptions', label: 'View options…', menu: 'Show', menuAfter: 'cm_ZoomOut', run: viewOptions }); // ours
// Options > Custom columns > List column sets in the Show menu: the first nine sets by name (cm_SrcCustomView1…9)
addMenuItems('Show', () => (treeOpt('CustomColumns.ShowInShowMenu', '1') === '1' && sets().length
  ? ['-', ...sets().slice(0, 9).map((s, k) => [s.name || `Column set ${k + 1}`, `cm_SrcCustomView${k + 1}`])] : []));
// the saved view modes as a Show > View modes submenu (F-819), the first nine (cm_SrcViewMode1…9); none, no submenu
addMenuItems('Show', () => viewModeMenu(viewModes()));
command({ name: 'cm_ColumnsShowHide', label: 'Show/hide columns…', menu: 'Show', menuAfter: 'cm_SrcCustomViewMenu', run: (app) => showHideColumns(app, app.cur) }); // ours (F-672)
command({ name: 'cm_CustomColumnConfig', label: 'Custom columns…', menu: 'Configuration', menuAfter: 'cm_ConfigHotkeys', run: customColumnConfig });
command({ name: 'cm_ThumbnailsConfig', label: 'Thumbnails…', menu: 'Configuration', menuAfter: 'cm_ConfigHotkeys', run: thumbnailsConfig });
command({ name: 'cm_ClearThumbnailCache', label: 'Delete thumbnail cache…', menu: 'Configuration', menuAfter: 'cm_ThumbnailsConfig', run: clearThumbCache }); // ours (TC: a button on the Thumbnails page)

// The cursor keys, view-aware: in Brief, Left/Right move a column and PgUp/PgDn to the top / bottom of the column; in
// Thumbnails, Up/Down move a row of tiles. Registered in init() so they replace the plain ones of navigation.js.
function cursorCommands() {
  command({
    name: 'cm_CursorUp', label: 'Cursor up',
    run: (app) => { const p = app.cur; if (modeOf(p) === 'thumbs') { const c = thumbGeo(p).cols; if (p.cursor - c >= 0) p.setCursor(p.cursor - c); } else p.moveCursor(-1); },
  });
  command({
    name: 'cm_CursorDown', label: 'Cursor down',
    run: (app) => {
      const p = app.cur;
      if (modeOf(p) === 'thumbs') {
        const c = thumbGeo(p).cols, n = p.entries.length;
        if (Math.floor(p.cursor / c) < Math.floor((n - 1) / c)) p.setCursor(Math.min(n - 1, p.cursor + c));
      } else p.moveCursor(1);
    },
  });
  command({
    name: 'cm_CursorLeft', label: 'Cursor left',
    run: (app) => { const p = app.cur, m = modeOf(p); if (m === 'brief') p.setCursor(Math.max(0, p.cursor - briefGeo(p).rows)); else if (m === 'thumbs') p.moveCursor(-1); },
  });
  command({
    name: 'cm_CursorRight', label: 'Cursor right',
    run: (app) => { const p = app.cur, m = modeOf(p); if (m === 'brief') p.setCursor(Math.min(p.entries.length - 1, p.cursor + briefGeo(p).rows)); else if (m === 'thumbs') p.moveCursor(1); },
  });
  command({
    name: 'cm_PageUp', label: 'Page up',
    run: (app) => {
      const p = app.cur;
      if (modeOf(p) === 'brief') { const r = briefGeo(p).rows; const top = p.cursor - (p.cursor % r); p.setCursor(p.cursor === top ? Math.max(0, top - r) : top); } else p.moveCursor(-p.pageSize());
    },
  });
  command({
    name: 'cm_PageDown', label: 'Page down',
    run: (app) => {
      const p = app.cur;
      if (modeOf(p) === 'brief') {
        const r = briefGeo(p).rows, n = p.entries.length;
        const bottom = Math.min(n - 1, p.cursor - (p.cursor % r) + r - 1);
        p.setCursor(p.cursor === bottom ? Math.min(n - 1, bottom + r) : bottom);
      } else p.moveCursor(p.pageSize());
    },
  });
}

// ---- control mode ----

function viewInfo(p) {
  const vs = vsOf(p);
  const out = { side: p.side, mode: vs.mode, set: vs.set, auto: vs.auto, columns: [], rows: [] };
  if (isColumnMode(p)) out.columns = [...p.$.header.querySelectorAll('[data-vcol]')].map((s) => s.textContent);
  else if (vs.mode !== 'tree') out.columns = [...p.$.header.querySelectorAll('[data-col]')].map((s) => s.textContent);
  if (vs.mode === 'tree') {
    const t = vs.tree;
    out.tree = t ? { at: t.at, path: t.nodes[t.at]?.path ?? '', nodes: t.nodes.slice(0, 200).map((n) => ({ name: n.depth ? n.name : n.path, path: n.path, depth: n.depth, open: !!n.open })) } : null;
    return out;
  }
  for (const el of p.$.rows.children) {
    const i = +el.dataset.i;
    const e = p.entries[i];
    if (!e) continue;
    const r = { i, name: e.name };
    if (vs.mode === 'thumbs') {
      r.thumb = !!vs.thumbs?.urls.has(e.name);
      r.icon = !!vs.thumbs?.icons?.names?.[e.name];
      r.text = el.querySelector('.mc-views-sub')?.textContent || '';
    } else if (vs.mode === 'brief') {
      const s = el.style;
      r.left = parseInt(s.left, 10);
      r.top = parseInt(s.top, 10);
    } else r.cells = [...el.children].map((c) => c.textContent);
    out.rows.push(r);
  }
  out.rows.sort((a, b) => a.i - b.i);
  if (vs.mode === 'brief') { const g = briefGeo(p); out.brief = { rows: g.rows, columns: g.cols, columnWidth: g.colW }; }
  if (vs.mode === 'thumbs') { const g = thumbGeo(p); out.thumbs = { columns: g.cols, size: g.s, gen: vs.thumbs?.gen ?? 0, done: vs.thumbs?.urls.size ?? 0 }; }
  return out;
}

addControlCommand('views', async () => ({ ok: true, left: viewInfo(theApp.left), right: viewInfo(theApp.right), settings: cfg() }));

// ---- start ----

// tabSwitched draws a panel again when its view state is another object than last time: another folder tab became
// active (each tab has its own p.vs). The thumbnails the tab left was making are stopped; its tree is left as it is.
const seenVs = new WeakMap();
function tabSwitched(p) {
  const vs = vsOf(p), old = seenVs.get(p);
  seenVs.set(p, vs);
  if (!old || old === vs) return false;
  if (old.thumbs && old.mode !== 'tree') {
    clearTimeout(old.thumbs.firstTimer);
    old.thumbs = null;
    if (vs.mode !== 'thumbs') go.Views.ThumbsStop(p.side).catch(() => {});
  }
  if (vs.mode === 'tree') { renderHeader(p); p.render(); return true; }
  applyView(p);
  save();
  return true;
}

export function init(app) {
  Panel.STATE_KEYS.push('vs');
  addTabState('vs', { copy: copyView, save: viewToSaved, load: (saved) => viewFromSaved(saved, (name) => !!findSet(name)) });
  for (const p of app.panels) install(p);
  cursorCommands();
  addKeyHandler(treeKey);
  on('views-thumb', onThumb);
  // the views the panels had last time (the tree is not kept)
  const c = cfg();
  if (!app.demo) {
    for (const p of app.panels) {
      const saved = c[p.side];
      if (!saved || !MODES.includes(saved.mode) || saved.mode === 'tree') continue;
      if (saved.mode === 'custom' && !findSet(saved.set)) continue;
      const vs = vsOf(p);
      vs.mode = saved.mode;
      vs.set = saved.mode === 'custom' ? saved.set : '';
      vs.manual = { mode: vs.mode, set: vs.set };
      renderHeader(p);
    }
  }
  for (const p of app.panels) if (/^col\d+$/.test(p.sort.key || '') && !(vsOf(p).mode === 'custom' || vsOf(p).mode === 'comments')) p.sort = { key: 'name', desc: false };
  for (const p of app.panels) seenVs.set(p, vsOf(p));
  app.on('panel', (p, info = {}) => {
    const switched = tabSwitched(p);
    const vs = vsOf(p);
    if (vs.mode === 'tree') return;
    // the other panel's tree follows this panel's folder (Options > Tree > Tree follows the current folder)
    if (info.moved && p.other && vsOf(p.other).mode === 'tree') treeFollow(p.other, p.path);
    if (info.moved) { autoSwitch(p); autoViewMode(p); }
    if (switched) return; // (applyView told the panel again, which asked for the cells and thumbnails)
    if (isColumnMode(p)) requestCells(p);
    if (vs.mode === 'thumbs') startThumbs(p);
  });
  app.on('exchange', () => {
    for (const p of app.panels) seenVs.set(p, vsOf(p));
    for (const p of app.panels) {
      const vs = vsOf(p);
      if (vs.thumbs) vs.thumbs.key = '';
      vs.cellsSig = '';
      applyView(p);
    }
    save();
  });
}
