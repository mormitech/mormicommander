// How a button runs (src/features/config/run.js, FEATURES.md section 11, audit #59): cm_/em_ commands go to the
// registry (with the default bar's aliases, F-304), "cd folder" changes the folder, a .bar file opens as a sub-bar
// (F-308), dropped files reach Go's Prepare (F-307), and a "?" prompt (F-814) runs the edited line or, on Cancel,
// nothing. The app, the dialogs and the Go side are stand-ins; runButton is the real one.
import { beforeEach, expect, test, vi } from 'vitest';

const calls = [];
let prepared = {};
let inputAnswer = null;
let listPick = null;
vi.mock('../src/core/go.js', () => ({
  errText: (e) => String(e),
  go: {
    ConfigBar: {
      Prepare: (btn, source, target, sourceIsRight, answers, dropped) => { calls.push(['prepare', btn.cmd, btn.params, dropped]); return Promise.resolve(prepared); },
      Run: (id, line, edited) => { calls.push(['run', id, line, edited]); return Promise.resolve(); },
      Discard: (id) => { calls.push(['discard', id]); },
      LoadBar: (file) => { calls.push(['loadbar', file]); return Promise.resolve({ path: `/bars/${file}`, buttons: [{ cmd: 'cm_SrcShort', tooltip: 'Brief' }, { sep: true }] }); },
    },
  },
}));
vi.mock('../src/core/dialogs.js', () => ({
  esc: (s) => s,
  inputDialog: (o) => { calls.push(['input', o.value]); return Promise.resolve(inputAnswer); },
  formDialog: () => Promise.resolve(null),
}));
vi.mock('../src/core/popup.js', () => ({
  showList: (o) => { calls.push(['list', o.items.length]); return Promise.resolve(listPick ? { item: o.items[0] } : null); },
}));
vi.mock('../src/features/config/util.js', () => ({
  panelCtx: () => ({ source: { path: '/a' }, target: { path: '/b' }, sourceIsRight: false }),
}));

const { runButton, realCommand } = await import('../src/features/config/run.js');

const app = {
  cur: { path: '/home/me', go: (p) => { calls.push(['go', p]); return Promise.resolve(true); } },
  resolvePath: (p, from) => (p.startsWith('/') ? p : `${from}/${p}`),
  run: (name, opts) => { calls.push(['cmd', name, opts?.params]); return Promise.resolve(true); },
  flash: () => {},
};

beforeEach(() => { calls.length = 0; prepared = {}; inputAnswer = null; listPick = null; });

test('internal commands run through the registry, with the default bar aliases', async () => {
  await runButton(app, { cmd: 'cm_GoToPrev' });
  await runButton(app, { cmd: 'em_backup' });
  expect(calls).toEqual([['cmd', 'cm_GoToPreviousDir', undefined], ['cmd', 'em_backup', undefined]]);
  expect(realCommand('cm_MultiRename')).toBe('cm_MultiRenameFiles');
});

test('separators and empty buttons do nothing', async () => {
  expect(await runButton(app, { sep: true, cmd: 'cm_List' })).toBe(false);
  expect(await runButton(app, { cmd: '  ' })).toBe(false);
  expect(calls).toEqual([]);
});

test('"cd folder" changes the folder of the active panel', async () => {
  await runButton(app, { cmd: 'cd "Docs"' });
  expect(calls).toEqual([['go', '/home/me/Docs']]);
});

test('a .bar file opens as a sub-bar and runs the button chosen', async () => {
  listPick = true;
  await runButton(app, { cmd: 'tools.bar' });
  expect(calls).toEqual([['loadbar', 'tools.bar'], ['list', 2], ['cmd', 'cm_SrcShort', undefined]]);
});

test('files dropped on a button go to Prepare and the program runs', async () => {
  prepared = { id: 'x1', program: '/usr/bin/open', line: '"/f/a.txt"' };
  await runButton(app, { cmd: '/usr/bin/open', params: '%P%N' }, { dropped: ['/f/a.txt'] });
  expect(calls).toEqual([['prepare', '/usr/bin/open', '%P%N', ['/f/a.txt']], ['run', 'x1', '"/f/a.txt"', false]]);
});

test('"?" shows the parameters first: OK runs the edited line, Cancel runs nothing', async () => {
  prepared = { id: 'p1', program: 'vim', line: 'a.txt', prompt: true };
  inputAnswer = 'b.txt';
  await runButton(app, { cmd: 'vim', params: '?%N' });
  expect(calls.slice(1)).toEqual([['input', 'a.txt'], ['run', 'p1', 'b.txt', true]]);

  calls.length = 0;
  inputAnswer = null;
  expect(await runButton(app, { cmd: 'vim', params: '?%N' })).toBe(false);
  expect(calls.slice(1)).toEqual([['input', 'a.txt'], ['discard', 'p1']]);
});

test('a cm_ command with parameters gets the filled-in line', async () => {
  prepared = { internal: true, program: 'cm_GoToNext', line: '/x' };
  await runButton(app, { cmd: 'cm_GoToNext', params: '%T' });
  expect(calls.at(-1)).toEqual(['cmd', 'cm_GoToNextDir', '/x']);
});
