// The Lister's table mode (key T, board issue #26 / F-NEW-10): CSV, TSV, JSON Lines, Parquet and SQLite files as a
// scrollable grid with a header of column names and types, in the Lister window and in quick view. Go (feature_lister_table.go
// over internal/table) reads the first 10,000 rows of the file, opened read-only; the page asks for the grid's shape
// (go.Lister.Table) and then fetches rows in blocks as they scroll into view (go.Lister.TableRows), drawing only the
// rows on screen (virtual scrolling), so a grid of 10,000 rows scrolls as lightly as one of ten. A SQLite database
// gets a list of its tables above the grid. The pure helpers at the top are unit-tested (test/viewer-table.test.js);
// TableGrid is the part on the page.

import { esc } from '../../core/dialogs.js';
import { bytes as fmtCount } from '../../core/format.js';
import { tm, tn } from '../../core/i18n.js';

export const BLOCK = 200; // rows fetched in one call
const OVERSCAN = 10; // rows drawn above and below the visible ones
const MIN_CH = 4;
const MAX_CH = 40;

// visibleRange is the rows [first, last) to draw for a scroll position, with a few extra on each side.
export function visibleRange(scrollTop, viewHeight, rowHeight, total, overscan = OVERSCAN) {
  if (!total || rowHeight <= 0) return { first: 0, last: 0 };
  const first = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
  const last = Math.min(total, Math.ceil((scrollTop + Math.max(0, viewHeight)) / rowHeight) + overscan);
  return { first, last: Math.max(first, last) };
}

// blocksFor lists the block numbers that hold rows [first, last).
export function blocksFor(first, last, block = BLOCK) {
  const out = [];
  if (last <= first) return out;
  for (let b = Math.floor(first / block); b <= Math.floor((last - 1) / block); b++) out.push(b);
  return out;
}

// cellText is what a cell shows: NULL for a missing value, and one line (line breaks and tabs as spaces).
export function cellText(v) {
  if (v === null || v === undefined) return null;
  return String(v).replace(/[\r\n\t]+/g, ' ');
}

// columnWidths picks each column's width in characters from its name, its type and the sample rows, between 4 and 40.
export function columnWidths(columns, rows) {
  return columns.map((c, i) => {
    let w = Math.max(String(c.name || '').length, String(c.type || '').length + 2);
    for (const r of rows || []) {
      const t = cellText(r?.[i]);
      w = Math.max(w, t === null ? 4 : t.length);
      if (w >= MAX_CH) break;
    }
    return Math.max(MIN_CH, Math.min(MAX_CH, w));
  });
}

// gridTemplate is the CSS grid-template-columns of a row: the row number column, then the columns.
export function gridTemplate(widths, total) {
  const num = Math.max(3, String(total || 0).length) + 1;
  return [`${num}ch`, ...widths.map((w) => `calc(${w}ch + 12px)`)].join(' ');
}

// moveCursor is where the row cursor goes for a key: Up/Down one row, PgUp/PgDn a page, Home/End the ends; null for
// keys that do not move it.
export function moveCursor(cursor, key, total, pageRows) {
  if (!total) return null;
  const page = Math.max(1, pageRows - 1);
  const to = {
    Up: cursor - 1, Down: cursor + 1, PgUp: cursor - page, 'Shift+Space': cursor - page, PgDn: cursor + page, Space: cursor + page,
    Home: 0, 'Ctrl+Home': 0, 'Ctrl+Up': 0, End: total - 1, 'Ctrl+End': total - 1, 'Ctrl+Down': total - 1,
  }[key];
  if (to === undefined) return null;
  return Math.max(0, Math.min(total - 1, to));
}

// rowsLabel is the row count as the status line says it: "1,234 rows", or "first 10,000 of 25,000 rows" when there are
// more ("first 10,000 rows" when how many more is not known).
export function rowsLabel(shape) {
  if (!shape) return '';
  if (shape.truncated) return shape.total > shape.rows ? `first ${fmtCount(shape.rows)} of ${fmtCount(shape.total)} rows` : `first ${fmtCount(shape.rows)} rows`;
  return `${fmtCount(shape.rows)} ${shape.rows === 1 ? 'row' : 'rows'}`;
}

// rowsLabelMsg is rowsLabel as a Msg (core/i18n.js): its English text and its translation, for the status line.
export function rowsLabelMsg(shape) {
  if (!shape) return '';
  if (shape.truncated) {
    return shape.total > shape.rows ? tm('first {rows} of {total} rows', { rows: fmtCount(shape.rows), total: fmtCount(shape.total) })
      : tm('first {rows} rows', { rows: fmtCount(shape.rows) });
  }
  return tn(shape.rows, 'row', 'rows', fmtCount(shape.rows));
}

// formatName is the menu text of a table format.
export const FORMAT_NAMES = { csv: 'CSV', tsv: 'TSV', jsonl: 'JSON Lines', sqlite: 'SQLite', parquet: 'Parquet' };

// findFrom is the cell a search in the grid goes on from (go.Lister.TableFind looks from the cell after it, or before
// it backward): the cell found last when it is in the cursor row, else just before the cursor row's first cell (just
// after its last one backward), so the cursor row itself is searched first.
export function findFrom(hit, cursor, width, backward) {
  if (hit && hit.row === cursor) return { row: hit.row, col: hit.col };
  return { row: cursor, col: backward ? width : -1 };
}

// TableGrid draws a table in a host element. api is {table(name), rows(start, count)} (the Go calls of one session);
// onChange runs when what the status line shows changed (with an error when a call failed); focus gives the keys back
// to the view after a table was picked from the list.
export class TableGrid {
  constructor(host, { api, rowHeight = 17, onChange = () => {}, focus = () => {} }) {
    this.host = host;
    this.api = api;
    this.rowHeight = rowHeight;
    this.onChange = onChange;
    this.shape = null;
    this.blocks = new Map(); // block number -> rows, or a Promise while it loads
    this.cursor = 0;
    this.hit = null; // the cell a search found: { row, col }
    this.gen = 0;
    host.innerHTML = `
      <div class="mc-tbl-bar" hidden><label>Table <select class="mc-tbl-pick"></select></label></div>
      <div class="mc-tbl-note" hidden></div>
      <div class="mc-tbl-scroll" tabindex="-1">
        <div class="mc-tbl-head"></div>
        <div class="mc-tbl-body"><div class="mc-tbl-rows"></div></div>
      </div>`;
    const $ = (s) => host.querySelector(s);
    this.$ = { bar: $('.mc-tbl-bar'), pick: $('.mc-tbl-pick'), note: $('.mc-tbl-note'), scroll: $('.mc-tbl-scroll'), head: $('.mc-tbl-head'), body: $('.mc-tbl-body'), rows: $('.mc-tbl-rows') };
    this.$.scroll.addEventListener('scroll', () => this.draw());
    this.$.pick.addEventListener('change', () => this.load(this.$.pick.value).catch((e) => this.onChange(e)).finally(() => focus()));
    this.$.rows.addEventListener('mousedown', (ev) => {
      const r = ev.target.closest('.mc-tbl-row');
      if (r) { this.cursor = +r.dataset.r; this.hit = null; this.draw(); this.onChange(); }
    });
  }

  setRowHeight(h) { this.rowHeight = h; this.layout(); }

  // load reads the grid (name: a table of a SQLite database, '' = the first).
  async load(name = '') {
    const gen = ++this.gen;
    const shape = await this.api.table(name);
    if (gen !== this.gen) return;
    this.shape = shape;
    this.blocks = new Map();
    this.cursor = 0;
    this.hit = null;
    const first = shape.rows ? await this.block(0) : [];
    if (gen !== this.gen) return;
    this.widths = columnWidths(shape.columns || [], first);
    const tables = shape.tables || [];
    this.$.bar.hidden = shape.format !== 'sqlite' || !tables.length;
    this.$.pick.innerHTML = tables.map((t) => `<option value="${esc(t.name)}"${t.name === shape.current ? ' selected' : ''}${t.readable ? '' : ' disabled'}>${esc(t.name)}</option>`).join('');
    this.$.note.hidden = !shape.note;
    this.$.note.textContent = shape.note || '';
    const tpl = gridTemplate(this.widths, shape.rows);
    this.host.style.setProperty('--mc-tbl-cols', tpl);
    this.$.head.innerHTML = `<div class="mc-tbl-row mc-tbl-hrow"><span class="mc-tbl-num">#</span>${(shape.columns || []).map((c) =>
      `<span class="mc-tbl-h" title="${esc(c.name)}${c.type ? ` (${esc(c.type)})` : ''}"><b>${esc(c.name)}</b><i>${esc(c.type || '')}</i></span>`).join('')}</div>`;
    this.$.scroll.scrollTop = 0;
    this.$.scroll.scrollLeft = 0;
    this.layout();
    this.onChange();
  }

  // reload reads the file again (F2), keeping the table of a database.
  reload() { return this.load(this.shape?.current || ''); }

  block(b) {
    const have = this.blocks.get(b);
    if (have) return Promise.resolve(have);
    const gen = this.gen;
    const p = this.api.rows(b * BLOCK, BLOCK).then((rows) => {
      if (gen === this.gen) this.blocks.set(b, rows || []);
      return rows || [];
    }, (e) => { this.blocks.delete(b); throw e; });
    this.blocks.set(b, p);
    return p;
  }

  layout() {
    if (!this.shape) return;
    this.$.body.style.height = `${this.shape.rows * this.rowHeight}px`;
    this.draw();
  }

  pageRows() { return Math.max(1, Math.floor((this.$.scroll.clientHeight - this.rowHeight * 1.6) / this.rowHeight)); }

  // draw shows the rows in view, fetching the blocks that are not there yet.
  draw() {
    const s = this.shape;
    if (!s) return;
    const sc = this.$.scroll;
    const { first, last } = visibleRange(sc.scrollTop, sc.clientHeight, this.rowHeight, s.rows);
    const missing = blocksFor(first, last).filter((b) => !Array.isArray(this.blocks.get(b)));
    if (missing.length) {
      const gen = this.gen;
      Promise.all(missing.map((b) => this.block(b))).then(() => { if (gen === this.gen) this.draw(); }, (e) => this.onChange(e));
    }
    const html = [];
    for (let i = first; i < last; i++) {
      const blk = this.blocks.get(Math.floor(i / BLOCK));
      const row = Array.isArray(blk) ? blk[i % BLOCK] : undefined;
      const hitCol = this.hit && this.hit.row === i ? this.hit.col : -1;
      const cells = (s.columns || []).map((_, c) => {
        if (!row) return '<span></span>';
        const t = cellText(row[c]);
        const hit = c === hitCol ? ' mc-tbl-hit' : '';
        return t === null ? `<span class="mc-tbl-null${hit}">NULL</span>` : `<span${hit ? ` class="${hit.trim()}"` : ''}>${esc(t)}</span>`;
      }).join('');
      html.push(`<div class="mc-tbl-row${i === this.cursor ? ' mc-tbl-cur' : ''}" data-r="${i}"><span class="mc-tbl-num">${i + 1}</span>${cells}</div>`);
    }
    this.$.rows.style.transform = `translateY(${first * this.rowHeight}px)`;
    this.$.rows.innerHTML = html.join('');
  }

  // showHit marks the cell a search found, moves the cursor to its row and scrolls the cell into view.
  showHit(row, col) {
    this.hit = { row, col };
    this.goToRow(row, true);
    const el = this.$.rows.querySelector('.mc-tbl-hit');
    if (!el) return;
    const sc = this.$.scroll, r = el.getBoundingClientRect(), b = sc.getBoundingClientRect();
    const num = this.$.rows.querySelector('.mc-tbl-cur .mc-tbl-num')?.getBoundingClientRect().width || 0;
    if (r.left < b.left + num || r.right > b.right) sc.scrollLeft += r.left - b.left - num - 8;
  }

  // goToRow moves the cursor to row i (0-based) and scrolls it into view (keepHit: leave the found cell marked).
  goToRow(i, keepHit = false) {
    if (!this.shape?.rows) return;
    if (!keepHit) this.hit = null;
    this.cursor = Math.max(0, Math.min(this.shape.rows - 1, i));
    const sc = this.$.scroll;
    const top = this.cursor * this.rowHeight;
    const head = this.$.head.offsetHeight || this.rowHeight;
    if (top < sc.scrollTop) sc.scrollTop = top;
    else if (top + this.rowHeight > sc.scrollTop + sc.clientHeight - head) sc.scrollTop = top + this.rowHeight - sc.clientHeight + head;
    this.draw();
    this.onChange();
  }

  // key handles a navigation key; true when it was used.
  key(k) {
    if (k === 'Left' || k === 'Right') { this.$.scroll.scrollLeft += k === 'Left' ? -60 : 60; return true; }
    const to = moveCursor(this.cursor, k, this.shape?.rows || 0, this.pageRows());
    if (to === null) return false;
    this.goToRow(to);
    return true;
  }

  // rowText is a row as tab-separated text (for copying).
  async rowText(i = this.cursor) {
    const rows = await this.block(Math.floor(i / BLOCK));
    const r = rows[i % BLOCK];
    return r ? r.map((v) => (v === null || v === undefined ? '' : String(v))).join('\t') : '';
  }

  state() {
    const s = this.shape;
    return s ? { format: s.format, columns: (s.columns || []).map((c) => c.name), types: (s.columns || []).map((c) => c.type), rows: s.rows, truncated: s.truncated,
      current: s.current || '', tables: (s.tables || []).map((t) => t.name), cursor: this.cursor, hit: this.hit, total: s.total || 0, note: s.note || '',
      shown: [...this.$.rows.querySelectorAll('.mc-tbl-row')].length } : null;
  }

  clear() {
    this.gen++;
    this.shape = null;
    this.hit = null;
    this.blocks = new Map();
    this.$.head.textContent = '';
    this.$.rows.textContent = '';
    this.$.body.style.height = '0px';
    this.$.bar.hidden = true;
    this.$.note.hidden = true;
  }
}
