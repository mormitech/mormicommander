// @vitest-environment happy-dom
// Section 42 (issue #90): the file list rows carry labels for screen readers (F-764), and the Dock badge of running
// jobs (F-749) says how far they are.
import { expect, test, vi } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});

const { Panel } = await import('../src/core/panel.js');
const { dockLabel } = await import('../src/features/dockprogress-lib.js');

test('rows are options named by the file, its kind and selection; the cursor row is the active descendant (F-764)', () => {
  const list = document.createElement('div'), rows = document.createElement('div');
  Object.defineProperty(list, 'clientHeight', { value: 400 });
  const p = Object.create(Panel.prototype);
  Object.assign(p, {
    app: { platform: 'darwin' }, side: 'left', rowH: 20, cursor: 1, selected: new Set(['b.txt']),
    entries: [{ name: '..', up: true, dir: true }, { name: 'docs', dir: true }, { name: 'b.txt', base: 'b', ext: 'txt', size: 3 }],
    $: { list, rows },
  });
  Object.defineProperty(p, 'isActive', { value: true });
  p.renderRows();
  const r = [...rows.children];
  expect(r.map((x) => x.getAttribute('role'))).toEqual(['option', 'option', 'option']);
  expect(r.map((x) => x.getAttribute('aria-label'))).toEqual(['.. (parent folder)', 'docs, folder', 'b.txt, selected']);
  expect(r[1].id).toBe('mc-cur-left');
  expect(r[1].getAttribute('aria-selected')).toBe('true');
  expect(list.getAttribute('aria-activedescendant')).toBe('mc-cur-left');
});

test('the Dock badge shows the progress of the jobs at work, and nothing when none is (F-749)', () => {
  expect(dockLabel([])).toBe('');
  expect(dockLabel([{ state: 'done', bytes: 10, bytesDone: 10 }])).toBe('');
  expect(dockLabel([{ state: 'running', bytes: 200, bytesDone: 50 }, { state: 'queued', bytes: 200, bytesDone: 0 }])).toBe('12%');
  expect(dockLabel([{ state: 'running', bytes: 0, files: 4, filesDone: 3 }])).toBe('75%');
  expect(dockLabel([{ state: 'asking', bytes: 0, files: 0 }])).toBe('0%');
});
