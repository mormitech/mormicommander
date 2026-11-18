// @vitest-environment happy-dom
// The command line and Commands menu leftovers of issues #58 and #62: the output window's rules (F-293), live folder
// suggestions (F-296: which word, the path completion on that word, Enter never running a suggestion), the TC-imported
// command line history merged into the app's own (F-295), and Commands > Open folder in new window (F-376). The app,
// the settings and Go are stand-ins.
import { beforeEach, expect, test, vi } from 'vitest';

const settings = {};
const calls = [];
const app = {
  platform: 'darwin', home: '/Users/me', showHidden: false, demo: false,
  cur: { path: '/Users/me' },
  config: { values: {} },
  setting: (k, d) => (k in settings ? settings[k] : d),
  setSetting: (k, v) => { settings[k] = v; },
  flash: (m, kind) => calls.push(['flash', m, kind || '']),
  resolvePath: (p, base) => (p.startsWith('/') ? p : `${base}/${p}`),
};
const cmds = {};
let openErr = null;
let iniHistory = [];
vi.mock('../src/core/app.js', () => ({ app, LABELS: {} }));
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => (k in settings ? settings[k] : d), setSetting: (k, v) => { settings[k] = v; } }));
vi.mock('../src/core/go.js', () => ({
  go: {
    FS: { List: async (_id, dir) => (dir === '/Users/me' ? { entries: [['Documents', 0, 0, 1], ['Downloads', 0, 0, 1], ['Desk.txt', 0, 0, 0]] } : { entries: [] }) },
    ConfigNewWindow: { Open: async (dir) => { calls.push(['open', dir]); if (openErr) throw new Error(openErr); } },
    Configuration: { CmdHistory: async () => iniHistory },
  },
  on: () => {}, errText: (e) => String(e?.message ?? e),
}));
vi.mock('../src/core/registry.js', () => ({
  command: (c) => { cmds[c.name] = c; }, hasCommand: () => false, commandList: () => [], getCommand: (n) => cmds[n],
  onChange: () => {}, run: () => Promise.resolve(),
}));

const { outputWanted, outputStatus } = await import('../src/features/cmdline-output-lib.js');
const { suggestWord } = await import('../src/features/cmdline-complete-lib.js');
const { attachPathComplete } = await import('../src/core/pathcomplete.js');
const { cmdline } = await import('../src/core/cmdline.js');
const { mergeCmdHistory, importCmdHistory } = await import('../src/features/config/cmdhist.js');
await import('../src/features/newwindow.js');
const { MENUS } = await import('../src/core/menus.js');

beforeEach(() => {
  document.body.innerHTML = '<input id="cmd" type="text">';
  calls.length = 0;
  for (const k of Object.keys(settings)) delete settings[k];
});

test('F-293: the output window only with the option on and plain Enter', () => {
  expect(outputWanted({}, false)).toBe(false); // off by default: a terminal as before
  expect(outputWanted({ 'Misc.CmdLineOutput': '1' }, false)).toBe(true);
  expect(outputWanted({ 'Misc.CmdLineOutput': '1' }, true)).toBe(false); // Shift+Enter: a terminal that stays open
});

test('F-293: the status line says how the program ended', () => {
  expect(outputStatus(null)).toMatch(/Running/);
  expect(outputStatus({ output: 'x', exitCode: 0 })).toBe('Done.');
  expect(outputStatus({ output: 'x', exitCode: 2 })).toMatch(/exit code 2/);
  expect(outputStatus({ output: '', stopped: true, exitCode: -1 })).toBe('Stopped. It printed nothing.');
  expect(outputStatus({ output: 'y', truncated: true, exitCode: -1 })).toMatch(/more than 1 MB/);
  expect(outputStatus({ error: 'Can\'t run' })).toBe('Can\'t run');
});

test('F-296: suggestions complete arguments and paths, not the program name', () => {
  expect(suggestWord('ls')).toBe(null);
  expect(suggestWord('cd Do')).toBe('Do');
  expect(suggestWord('ls ~/Doc')).toBe('~/Doc');
  expect(suggestWord('./bi')).toBe('./bi');
  expect(suggestWord('cd ')).toBe(null);
  expect(suggestWord('cd "My Do')).toBe('My Do');
});

const typeInto = async (input, text) => {
  input.value = text;
  input.setSelectionRange(text.length, text.length);
  input.dispatchEvent(new InputEvent('input', { inputType: 'insertText', data: text.slice(-1) }));
  await new Promise((r) => setTimeout(r, 0));
};

test('F-296: the suggestion is a folder filled in after the caret, selected; Enter runs only what was typed', async () => {
  const input = document.getElementById('cmd');
  input.focus();
  attachPathComplete(input, () => '/Users/me', { word: (t) => suggestWord(t) });
  await typeInto(input, 'cd Doc');
  expect(input.value).toBe('cd Documents');
  expect([input.selectionStart, input.selectionEnd]).toEqual([6, 12]);
  let ran = null;
  const { addCmdlineHandler } = await import('../src/core/cmdline.js');
  addCmdlineHandler(async (t) => { ran = t; return true; }, { first: true });
  await cmdline.submit();
  expect(ran).toBe('cd Doc');
  expect(settings.cmdHistory[0]).toBe('cd Doc');
});

test('F-296: without a word (the option off) nothing is suggested', async () => {
  const input = document.getElementById('cmd');
  input.focus();
  attachPathComplete(input, () => '/Users/me', { word: () => null });
  await typeInto(input, 'cd Doc');
  expect(input.value).toBe('cd Doc');
});

test('F-295: imported lines join the history once, after the own lines, without doubles', () => {
  expect(mergeCmdHistory(['make', 'ls'], ['git status', 'ls'], [], 50)).toEqual({ history: ['make', 'ls', 'git status'], seen: ['git status', 'ls'] });
  expect(mergeCmdHistory(['make'], ['git status'], ['git status'], 50)).toBe(null); // merged before: not again
  expect(mergeCmdHistory(['a', 'b'], ['c', 'd'], [], 3).history).toEqual(['a', 'b', 'c']);
  expect(mergeCmdHistory([], [], [], 50)).toBe(null);
});

test('F-295: importCmdHistory writes the merged history, and nothing with Remember command line history off', async () => {
  settings.cmdHistory = ['own'];
  iniHistory = ['tc one', 'tc two'];
  await importCmdHistory(app);
  expect(settings.cmdHistory).toEqual(['own', 'tc one', 'tc two']);
  settings.cmdHistory = ['own'];
  await importCmdHistory(app); // the same lines again: not brought back
  expect(settings.cmdHistory).toEqual(['own']);
  delete settings.cmdHistoryImported;
  app.config.values['Operation.SaveCommands'] = '0';
  await importCmdHistory(app);
  expect(settings.cmdHistory).toEqual(['own']);
  delete app.config.values['Operation.SaveCommands'];
});

test('F-376: Commands > Open folder in new window opens the active folder, and shows why when it cannot', async () => {
  const commands = MENUS.find(([l]) => l === 'Commands')[1].filter(Array.isArray).map((it) => it[1]);
  expect(commands).toContain('cm_OpenNewWindow');
  openErr = null;
  await cmds.cm_OpenNewWindow.run(app);
  expect(calls).toContainEqual(['open', '/Users/me']);
  openErr = 'Only one copy of mormiCommander may run';
  calls.length = 0;
  await cmds.cm_OpenNewWindow.run(app);
  expect(calls).toContainEqual(['flash', 'Only one copy of mormiCommander may run', 'err']);
});
