// @vitest-environment happy-dom
// Tests of the Lister's table mode (viewer/table.js, board issue #26): the virtual-scrolling arithmetic (which rows to
// draw, which blocks to fetch), cell text and column widths, the row cursor keys, the status text, and a TableGrid on
// a happy-dom page fed by a fake Go side: it fetches only the blocks in view, draws NULLs, lists the tables of a
// database, switches tables, copies a row as tab-separated text, and marks the cell a search found. Nothing here talks to Go or opens a window.
import { describe, expect, test, vi } from 'vitest';

vi.mock('../src/core/dialogs.js', () => ({
  esc: (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c])),
}));

const { visibleRange, blocksFor, cellText, columnWidths, gridTemplate, moveCursor, rowsLabel, findFrom, FORMAT_NAMES, TableGrid, BLOCK } = await import('../src/features/viewer/table.js');

describe('table helpers', () => {
  test('visibleRange draws the rows in view and a few around them', () => {
    expect(visibleRange(0, 170, 17, 10000, 10)).toEqual({ first: 0, last: 20 });
    expect(visibleRange(1700, 170, 17, 10000, 10)).toEqual({ first: 90, last: 120 });
    expect(visibleRange(169983, 170, 17, 10000, 10)).toEqual({ first: 9989, last: 10000 });
    expect(visibleRange(0, 170, 17, 0)).toEqual({ first: 0, last: 0 });
  });

  test('blocksFor names the blocks that hold a range', () => {
    expect(blocksFor(0, 20, 200)).toEqual([0]);
    expect(blocksFor(190, 230, 200)).toEqual([0, 1]);
    expect(blocksFor(400, 400, 200)).toEqual([]);
    expect(blocksFor(399, 401, 200)).toEqual([1, 2]);
  });

  test('cellText keeps NULL apart and puts a cell on one line', () => {
    expect(cellText(null)).toBe(null);
    expect(cellText(undefined)).toBe(null);
    expect(cellText('a\r\nb\tc')).toBe('a b c');
    expect(cellText('')).toBe('');
  });

  test('columnWidths fit the header and the values, within limits', () => {
    const cols = [{ name: 'id', type: 'integer' }, { name: 'n', type: '' }, { name: 'long', type: 'text' }];
    expect(columnWidths(cols, [['1', null, 'x'.repeat(100)]])).toEqual([9, 4, 40]);
    expect(gridTemplate([9, 4], 10000)).toBe('6ch calc(9ch + 12px) calc(4ch + 12px)');
  });

  test('moveCursor moves by rows, pages and to the ends', () => {
    expect(moveCursor(5, 'Down', 10, 4)).toBe(6);
    expect(moveCursor(0, 'Up', 10, 4)).toBe(0);
    expect(moveCursor(0, 'PgDn', 10, 4)).toBe(3);
    expect(moveCursor(2, 'End', 10, 4)).toBe(9);
    expect(moveCursor(9, 'Ctrl+Home', 10, 4)).toBe(0);
    expect(moveCursor(9, 'Down', 10, 4)).toBe(9);
    expect(moveCursor(3, 'X', 10, 4)).toBe(null);
    expect(moveCursor(0, 'Down', 0, 4)).toBe(null);
  });

  test('rowsLabel says when only the first rows were read', () => {
    expect(rowsLabel({ rows: 1, truncated: false })).toBe('1 row');
    expect(rowsLabel({ rows: 3, truncated: false })).toBe('3 rows');
    expect(rowsLabel({ rows: 10000, truncated: true })).toMatch(/^first 10.000 rows$/);
    expect(rowsLabel({ rows: 10000, truncated: true, total: 0 })).toMatch(/^first 10.000 rows$/);
    expect(rowsLabel({ rows: 10000, truncated: true, total: 1234567 })).toMatch(/^first 10.000 of 1.234.567 rows$/);
    expect(FORMAT_NAMES.parquet).toBe('Parquet');
  });

  test('findFrom goes on after the found cell, or searches the cursor row first', () => {
    expect(findFrom(null, 5, 3, false)).toEqual({ row: 5, col: -1 });
    expect(findFrom(null, 5, 3, true)).toEqual({ row: 5, col: 3 });
    expect(findFrom({ row: 5, col: 1 }, 5, 3, false)).toEqual({ row: 5, col: 1 });
    expect(findFrom({ row: 5, col: 1 }, 5, 3, true)).toEqual({ row: 5, col: 1 });
    expect(findFrom({ row: 2, col: 1 }, 5, 3, false)).toEqual({ row: 5, col: -1 }); // the cursor moved since
  });
});

// fakeGo is a Go side with one 1,000-row table "big" and a 2-row table "small", recording the row requests.
function fakeGo() {
  const calls = [];
  const tables = {
    big: { columns: [{ name: 'n', type: 'INTEGER' }, { name: 'note', type: 'TEXT' }], rows: Array.from({ length: 1000 }, (_, i) => [String(i + 1), i % 3 ? `row <${i + 1}>` : null]) },
    small: { columns: [{ name: 'a', type: '' }], rows: [['x'], ['y']] },
  };
  let current = 'big';
  return {
    calls,
    api: {
      table: async (name) => {
        current = name || 'big';
        const t = tables[current];
        return { format: 'sqlite', columns: t.columns, rows: t.rows.length, truncated: false, limit: 10000, note: '', current,
          tables: [{ name: 'big', readable: true }, { name: 'small', readable: true }, { name: 'kv', readable: false }] };
      },
      rows: async (start, count) => { calls.push(start); return tables[current].rows.slice(start, start + count); },
    },
  };
}

describe('TableGrid', () => {
  test('draws the header, the first rows and NULLs, fetching only the first block', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    const g = fakeGo();
    const grid = new TableGrid(host, { api: g.api, rowHeight: 17 });
    await grid.load('');
    expect(host.querySelectorAll('.mc-tbl-h b')[1].textContent).toBe('note');
    expect(host.querySelectorAll('.mc-tbl-h i')[0].textContent).toBe('INTEGER');
    const rows = host.querySelectorAll('.mc-tbl-rows .mc-tbl-row');
    expect(rows.length).toBeGreaterThan(0);
    expect(rows.length).toBeLessThan(BLOCK);
    expect(rows[0].querySelector('.mc-tbl-null').textContent).toBe('NULL');
    expect(rows[1].textContent).toContain('row <2>');
    expect(rows[1].innerHTML).toContain('&lt;2&gt;');
    expect(g.calls).toEqual([0]);
    expect(host.querySelector('.mc-tbl-body').style.height).toBe(`${1000 * 17}px`);
    expect([...host.querySelectorAll('.mc-tbl-pick option')].map((o) => [o.value, o.disabled])).toEqual([['big', false], ['small', false], ['kv', true]]);
    expect(host.querySelector('.mc-tbl-bar').hidden).toBe(false);
  });

  test('the cursor keys move and the blocks further down load when reached', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    const g = fakeGo();
    const grid = new TableGrid(host, { api: g.api, rowHeight: 17 });
    await grid.load('');
    expect(grid.key('End')).toBe(true);
    expect(grid.cursor).toBe(999);
    grid.goToRow(450);
    expect(grid.cursor).toBe(450);
    await grid.block(2);
    expect(g.calls).toContain(400);
    expect(await grid.rowText(451)).toBe('452\trow <452>');
    expect(await grid.rowText(450)).toBe('451\t');
    expect(await grid.rowText(0)).toBe('1\t');
    expect(grid.key('Q')).toBe(false);
    expect(grid.state()).toMatchObject({ format: 'sqlite', rows: 1000, current: 'big', cursor: 450, columns: ['n', 'note'] });
  });

  test('a found cell is marked until the cursor moves', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    const g = fakeGo();
    const grid = new TableGrid(host, { api: g.api, rowHeight: 17 });
    await grid.load('');
    grid.showHit(1, 1);
    expect(grid.cursor).toBe(1);
    const hit = host.querySelectorAll('.mc-tbl-hit');
    expect(hit.length).toBe(1);
    expect(hit[0].textContent).toBe('row <2>');
    expect(grid.state().hit).toEqual({ row: 1, col: 1 });
    grid.key('Down');
    expect(host.querySelectorAll('.mc-tbl-hit').length).toBe(0);
    expect(grid.hit).toBe(null);
  });

  test('switching tables reads the other one and starts at its top', async () => {
    const host = document.createElement('div');
    document.body.append(host);
    const g = fakeGo();
    const grid = new TableGrid(host, { api: g.api, rowHeight: 17 });
    await grid.load('');
    grid.goToRow(10);
    await grid.load('small');
    expect(grid.cursor).toBe(0);
    expect(grid.state().rows).toBe(2);
    expect(host.querySelectorAll('.mc-tbl-rows .mc-tbl-row').length).toBe(2);
    grid.clear();
    expect(grid.state()).toBe(null);
    expect(host.querySelector('.mc-tbl-rows').textContent).toBe('');
  });
});
