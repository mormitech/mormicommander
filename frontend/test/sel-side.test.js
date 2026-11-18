// @vitest-environment happy-dom
// Shift+Left / Shift+Right (board #127, spec area 3, 1.2): they are bound by default, and they move one side step with
// range marking, like Shift+Down over that many rows: a column in Brief (the panel's sideStep), a page in the Full view
// (no sideStep, so the page size), and they take back the marks when the run turns round.
import { beforeAll, describe, expect, test, vi } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});

const reg = await import('../src/core/registry.js');
await import('../src/features/selection.js');

// fakePanel is the part of core/panel.js the selection commands use.
function fakePanel(n, { sideStep, pageSize = 7 } = {}) {
  const p = {
    entries: Array.from({ length: n }, (_, i) => ({ name: `f${i}` })),
    cursor: 0,
    selected: new Set(),
    get cur() { return this.entries[this.cursor]; },
    isSelected(e) { return !!e && this.selected.has(e.name); },
    select(e, on = true) { if (on) this.selected.add(e.name); else this.selected.delete(e.name); },
    setCursor(i) { this.cursor = Math.max(0, Math.min(this.entries.length - 1, i)); },
    pageSize: () => pageSize,
    selectionChanged() {},
  };
  if (sideStep) p.sideStep = () => sideStep;
  return p;
}
const marked = (p) => [...p.selected].sort((a, b) => +a.slice(1) - +b.slice(1));

let app;
beforeAll(() => {
  app = { cur: null, setting: (_k, d) => d, emit() {} };
  reg.setApp(app);
  reg.setupKeys('linux', {});
});

test('Shift+Left and Shift+Right have commands by default', () => {
  expect(reg.commandForKey('Shift+Left')).toBe('cm_SelLeft');
  expect(reg.commandForKey('Shift+Right')).toBe('cm_SelRight');
  reg.setupKeys('darwin', {});
  expect(reg.commandForKey('Shift+Right')).toBe('cm_SelRight');
  reg.setupKeys('linux', {});
});

describe('Brief view: one column (sideStep = rows)', () => {
  test('Shift+Right marks the rows passed and lands one column on', async () => {
    const p = fakePanel(60, { sideStep: 20 });
    p.cursor = 1; app.cur = p;
    await reg.run('cm_SelRight');
    expect(p.cursor).toBe(21);
    expect(marked(p)).toEqual(Array.from({ length: 20 }, (_, i) => `f${i + 1}`));
  });
  test('Shift+Left after it takes the marks back', async () => {
    const p = fakePanel(60, { sideStep: 20 });
    p.cursor = 1; app.cur = p;
    await reg.run('cm_SelRight');
    await reg.run('cm_SelLeft');
    expect(p.cursor).toBe(1);
    expect(marked(p)).toEqual([]);
  });
  test('Shift+Left from a later column marks backwards and stops at the top', async () => {
    const p = fakePanel(60, { sideStep: 20 });
    p.cursor = 25; app.cur = p;
    await reg.run('cm_SelLeft');
    expect(p.cursor).toBe(5);
    expect(marked(p)).toEqual(Array.from({ length: 20 }, (_, i) => `f${i + 6}`));
    await reg.run('cm_SelLeft');
    expect(p.cursor).toBe(0);
  });
});

test('Full view (no sideStep): Shift+Right acts as Shift+PgDn', async () => {
  const a = fakePanel(60, { pageSize: 7 });
  a.cursor = 1; app.cur = a;
  await reg.run('cm_SelRight');
  const b = fakePanel(60, { pageSize: 7 });
  b.cursor = 1; app.cur = b;
  await reg.run('cm_SelPageDown');
  expect(a.cursor).toBe(b.cursor);
  expect(marked(a)).toEqual(marked(b));
  expect(a.cursor).toBe(8);
});

// F-850: in the Thumbnails view Up/Down move a row of tiles, and Shift+Up/Shift+Down select over that row (the panel's
// vertStep); with no vertStep (the other views) they move one entry as before.
describe('Thumbnails view: Shift+Up/Down move a row (vertStep = tiles per row)', () => {
  test('Shift+Down marks the row it leaves, Shift+Up takes it back', async () => {
    const p = fakePanel(30);
    p.vertStep = () => 5;
    app.cur = p;
    await reg.run('cm_SelDown');
    expect(p.cursor).toBe(5);
    expect(marked(p)).toEqual(['f0', 'f1', 'f2', 'f3', 'f4']);
    await reg.run('cm_SelUp');
    expect(p.cursor).toBe(0);
    expect(marked(p)).toEqual([]);
  });
  test('without vertStep Shift+Down moves one entry', async () => {
    const p = fakePanel(30);
    app.cur = p;
    await reg.run('cm_SelDown');
    expect(p.cursor).toBe(1);
    expect(marked(p)).toEqual(['f0']);
  });
});
