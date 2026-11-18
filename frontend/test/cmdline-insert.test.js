// @vitest-environment happy-dom
// Putting names and paths into the command line (src/features/cmdline.js, F-298, F-299, F-813, issue #58): Ctrl+Enter
// adds the name under the cursor, one after another with a space, quoted when it has spaces; Ctrl+Shift+Enter the full
// path; Ctrl+P the folder. The Commands menu lists the same commands (src/core/menus.js), so the menu gives the same
// result as the key. The app, the registry and the Go side are stand-ins; the command line and the commands are real.
import { beforeEach, expect, test, vi } from 'vitest';

const cmds = {};
const app = {
  platform: 'darwin',
  home: '/Users/me',
  cur: { path: '/Users/me/My Docs', cur: null, fullPath: (n) => `/Users/me/My Docs/${n}` },
  run: () => Promise.resolve(),
  flash: () => {},
};
vi.mock('../src/core/app.js', () => ({ app, LABELS: {} }));
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => d, setSetting: () => {} }));
vi.mock('../src/core/go.js', () => ({ go: {}, on: () => {}, errText: String }));
vi.mock('../src/core/registry.js', () => ({
  command: (c) => { cmds[c.name] = c; }, hasCommand: () => false, commandList: () => [], getCommand: (n) => cmds[n],
  onChange: () => {}, run: () => Promise.resolve(),
}));

await import('../src/features/cmdline.js');
const { cmdline } = await import('../src/core/cmdline.js');
const { MENUS } = await import('../src/core/menus.js');

beforeEach(() => {
  document.body.innerHTML = '<input id="cmd" type="text">';
});

test('Ctrl+Enter on two files gives both names, separated by a space, quoted when needed', () => {
  app.cur.cur = { name: 'a.txt' };
  cmds.cm_AddFilenameToCmdline.run(app);
  app.cur.cur = { name: 'b c.txt' };
  cmds.cm_AddFilenameToCmdline.run(app);
  expect(cmdline.text).toBe('a.txt "b c.txt" ');
});

test('Ctrl+Enter on ".." adds nothing', () => {
  app.cur.cur = { name: '..', up: true };
  cmds.cm_AddFilenameToCmdline.run(app);
  expect(cmdline.text).toBe('');
});

test('Ctrl+Shift+Enter adds the full path and Ctrl+P the folder, quoted for their spaces', () => {
  app.cur.cur = { name: 'x.txt' };
  cmds.cm_AddPathAndNameToCmdline.run(app);
  cmds.cm_AddPathToCmdline.run(app);
  expect(cmdline.text).toBe('"/Users/me/My Docs/x.txt" "/Users/me/My Docs" ');
});

test('the Commands menu lists the command line commands, showing their keys', () => {
  const commands = MENUS.find(([l]) => l === 'Commands')[1].filter(Array.isArray).map((it) => it[1]);
  for (const n of ['cm_FocusCmdLine', 'cm_ShowCmdLineHistory', 'cm_AddPathToCmdline', 'cm_AddFilenameToCmdline', 'cm_AddPathAndNameToCmdline']) {
    expect(commands).toContain(n);
  }
  for (const n of ['cm_AddPathToCmdline', 'cm_AddFilenameToCmdline', 'cm_AddPathAndNameToCmdline']) expect(cmds[n].menuKey).toBe(true);
});
