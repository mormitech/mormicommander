// Start-up parameters the page applies (docs/FEATURES.md section 41): -P picks the panel, -S swaps the folders when the
// right panel is active, -T opens tabs, -S=L opens the Lister on the file, and with the standalone option (F-852) only
// the Lister window opens: the panels are not moved, so the run leaves the saved session as it was.
import { expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', async () => {
  const { F } = await import('../src/core/format.js');
  const go = {
    FS: { Stat: (p) => Promise.resolve(p.endsWith('.txt') ? { flags: 0, name: 'f.txt' } : { flags: F.DIR }), Parent: () => Promise.resolve('/d') },
    ConfigStart: { StartParams: () => Promise.resolve(null) },
  };
  return { go, on: () => () => {} };
});
vi.mock('../src/core/registry.js', () => ({ hasCommand: () => false }));

const { applyStart } = await import('../src/features/config/start.js');

function fakeApp() {
  const panel = (side) => ({ side, went: [], focused: '', go(p) { this.went.push(p); return Promise.resolve(true); }, focusName(n) { this.focused = n; } });
  const app = { left: panel('left'), right: panel('right'), runs: [], flashes: [] };
  app.cur = app.left;
  app.setActive = (p) => { app.cur = p; };
  app.run = (name, args) => { app.runs.push([name, args]); return Promise.resolve(true); };
  app.flash = (m) => app.flashes.push(m);
  return app;
}

test('-S=L with the standalone option runs the Lister on the file and leaves the panels alone', async () => {
  const app = fakeApp();
  await applyStart(app, { tool: 'lister', file1: '/d/f.txt', standalone: true });
  expect(app.runs).toEqual([['cm_List', { path: '/d/f.txt', standalone: true }]]);
  expect(app.left.went).toEqual([]);
  expect(app.right.went).toEqual([]);
});

test('-S=L without the option shows the file in the left panel and views it there (as before)', async () => {
  const app = fakeApp();
  await applyStart(app, { tool: 'lister', file1: '/d/f.txt', standalone: false, toolOpts: '' });
  expect(app.left.went).toEqual(['/d']);
  expect(app.left.focused).toBe('f.txt');
  expect(app.runs[0][0]).toBe('cm_List');
  expect(app.runs[0][1].standalone).toBe(true);
  expect(app.runs[0][1].path).toBeUndefined();
});

test('-S -P and plain folders', async () => {
  let app = fakeApp();
  await applyStart(app, { left: '/a', right: '/b', active: 'R' });
  expect(app.cur).toBe(app.right);
  app = fakeApp();
  app.cur = app.right;
  await applyStart(app, { left: '/a', right: '/b', sourceTarget: true });
  expect(app.right.went).toEqual(['/a']); // (-S without -P: the first folder goes to the active panel)
  expect(app.left.went).toEqual(['/b']);
});
