// Options > Misc "Keep the window on top of other windows" (F-748) and "Show job progress on the Dock icon" (F-749):
// with both off (the default) nothing is sent to the window; turning them on and off sends the change once.
import { expect, test, vi } from 'vitest';

const calls = [];
const values = {};
let jobs = [];
let jobsFn = null;
vi.mock('../src/core/go.js', () => ({
  go: { App: { SetAlwaysOnTop: (v) => { calls.push(['top', v]); return Promise.resolve(); }, SetDockBadge: (v) => { calls.push(['badge', v]); return Promise.resolve(); } } },
}));
vi.mock('../src/core/jobs.js', () => ({ onJobs: (f) => { jobsFn = f; }, jobList: () => jobs }));
vi.mock('../src/features/config/util.js', () => ({ on: (n) => values[n] === '1' }));

const { applyWindowOpts, initWindowOpts } = await import('../src/features/config/windowopts.js');

test('nothing is sent while both options are off', () => {
  initWindowOpts();
  applyWindowOpts();
  jobs = [{ state: 'running', bytes: 10, bytesDone: 5 }];
  jobsFn();
  expect(calls).toEqual([]);
});

test('always on top follows the option, and the badge follows the jobs while its option is on', () => {
  values['Misc.AlwaysOnTop'] = '1';
  applyWindowOpts();
  applyWindowOpts();
  expect(calls).toEqual([['top', true]]);
  values['Misc.DockProgress'] = '1';
  applyWindowOpts();
  expect(calls.at(-1)).toEqual(['badge', '50%']);
  jobs = [{ state: 'done', bytes: 10, bytesDone: 10 }];
  jobsFn();
  expect(calls.at(-1)).toEqual(['badge', '']);
  values['Misc.AlwaysOnTop'] = '0';
  applyWindowOpts();
  expect(calls.at(-1)).toEqual(['top', false]);
});
