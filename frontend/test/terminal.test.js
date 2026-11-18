// @vitest-environment happy-dom
// The embedded terminal pane (issue #31, F-856): the settings, the decoding of Go's output, when a panel folder is sent
// as a cd (never while the user has typed something, never twice, never for a remote folder), the commands and their
// menu entries, and that the AI tools may run none of them. xterm.js and Go are stand-ins that record what they get.
import { beforeAll, beforeEach, expect, test, vi } from 'vitest';
import { termConfig, clampHeight, b64ToBytes, lineDirty, followTarget, TERM_DEFAULTS, MIN_HEIGHT } from '../src/features/terminal-lib.js';
import { commandVerdict } from '../src/features/aitools-allow.js';

const calls = [];
const handlers = {};
vi.mock('../src/core/go.js', () => {
  const answers = { Available: true, Start: 7, Cd: true, Busy: false };
  const go = new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        if (obj === 'Terminal') calls.push([m, ...args]);
        return Promise.resolve(obj === 'Terminal' ? answers[m] ?? null : null);
      },
    }),
  });
  return { go, on: (ev, fn) => { handlers[ev] = fn; return () => {}; }, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

const written = [];
let dataFn = null;
vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    constructor() { this.cols = 100; this.rows = 20; }
    loadAddon() {} open() {} focus() {} reset() {} attachCustomKeyEventHandler() {}
    onData(fn) { dataFn = fn; }
    write(d) { written.push(d); }
  },
}));
vi.mock('@xterm/addon-fit', () => ({ FitAddon: class { fit() {} } }));
vi.mock('@xterm/xterm/css/xterm.css', () => ({}));

let reg, app, menus;
const panel = { path: '/Users/x', side: 'left', $: { list: { focus() {} } }, targets: () => [{ name: 'a b.txt' }], fullPath: (n) => `/Users/x/${n}` };

beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  globalThis.ResizeObserver = class { observe() {} };
  document.body.innerHTML = '<div id="app"><main id="panels"></main><div id="cmdline"></div></div>';
  await import('../src/features/terminal.js');
  reg = await import('../src/core/registry.js');
  menus = await import('../src/core/menus.js');
  ({ app } = await import('../src/core/app.js'));
  reg.setApp(app);
  app.panels = [panel];
  app.active = 0;
  (await import('../src/features/terminal.js')).init(app);
  vi.useRealTimers();
});
beforeEach(() => { calls.length = 0; });

const tick = () => new Promise((r) => setTimeout(r, 0));
const moveTo = async (path) => { panel.path = path; app.emit('panel', panel, { moved: true }); await tick(); await tick(); };

test('settings: following is on and the height has a default and a floor', () => {
  expect(termConfig(undefined)).toEqual(TERM_DEFAULTS);
  expect(termConfig({ follow: false, height: 10 })).toEqual({ follow: false, height: TERM_DEFAULTS.height });
  expect(termConfig({ height: 300.4 }).height).toBe(300);
  expect(clampHeight(10, 800)).toBe(MIN_HEIGHT);
  expect(clampHeight(5000, 800)).toBe(680);
});

test('Go output is decoded byte by byte (a read may end inside a UTF-8 character)', () => {
  expect([...b64ToBytes(btoa('hi\xc3'))]).toEqual([104, 105, 0xc3]);
  expect(b64ToBytes('').length).toBe(0);
});

test('the line is dirty after typing and clean after Enter, Ctrl+C or Ctrl+U', () => {
  expect(lineDirty(false, 'ls')).toBe(true);
  expect(lineDirty(true, '\r')).toBe(false);
  expect(lineDirty(true, 'x\x03')).toBe(false);
  expect(lineDirty(false, '\x1b[A')).toBe(true); // an arrow may bring back a history line
  expect(lineDirty(false, 'a\rb')).toBe(true);
  expect(lineDirty(true, '')).toBe(true);
});

test('followTarget sends a local folder once, only with a running shell and an empty line', () => {
  const base = { open: true, id: 1, follow: true, path: '/a', lastSent: '/b', dirty: false, isLocal: (p) => !p.includes('://') };
  expect(followTarget(base)).toBe('/a');
  expect(followTarget({ ...base, lastSent: '/a' })).toBe('');
  expect(followTarget({ ...base, dirty: true })).toBe('');
  expect(followTarget({ ...base, follow: false })).toBe('');
  expect(followTarget({ ...base, open: false })).toBe('');
  expect(followTarget({ ...base, id: 0 })).toBe('');
  expect(followTarget({ ...base, path: 'ftp://host/x' })).toBe('');
});

test('the pane opens with the shell in the panel folder, follows the panel and inserts names without Enter', async () => {
  expect(document.getElementById('terminal-pane')).toBeNull(); // nothing is made before the user opens it
  expect(await app.run('cm_TerminalPane')).toBe(true);
  const pane = document.getElementById('terminal-pane');
  expect(pane.hidden).toBe(false);
  expect(calls.find((c) => c[0] === 'Start')).toEqual(['Start', '/Users/x', 100, 20]);
  expect(calls.some((c) => c[0] === 'Write')).toBe(false); // the app types nothing by itself

  await moveTo('/Users/x/Docs');
  expect(calls.filter((c) => c[0] === 'Cd')).toEqual([['Cd', 7, '/Users/x/Docs']]);
  await moveTo('/Users/x/Docs');
  expect(calls.filter((c) => c[0] === 'Cd').length).toBe(1); // the same folder is not sent again
  await moveTo('ftp://server/pub');
  expect(calls.filter((c) => c[0] === 'Cd').length).toBe(1); // a remote folder is not sent

  dataFn('git sta'); // the user is typing: following waits
  await moveTo('/tmp');
  expect(calls.filter((c) => c[0] === 'Cd').length).toBe(1);
  dataFn('\x15');
  await moveTo('/var');
  expect(calls.filter((c) => c[0] === 'Cd').at(-1)).toEqual(['Cd', 7, '/var']);

  handlers['terminal-data'](7, btoa('hello'));
  expect(written.at(-1)).toEqual(new Uint8Array([104, 101, 108, 108, 111]));

  calls.length = 0;
  await app.run('cm_TerminalInsertNames');
  expect(calls.filter((c) => c[0] === 'Insert')).toEqual([['Insert', 7, ['a b.txt']]]);
  await app.run('cm_TerminalInsertPaths');
  expect(calls.filter((c) => c[0] === 'Insert').at(-1)).toEqual(['Insert', 7, ['/Users/x/a b.txt']]);

  await app.run('cm_TerminalFollow', { on: false });
  calls.length = 0;
  await moveTo('/opt');
  expect(calls.some((c) => c[0] === 'Cd')).toBe(false);
  await app.run('cm_TerminalFollow', { on: true });

  expect(await app.run('cm_TerminalPane')).toBe(true);
  expect(pane.hidden).toBe(true);
  expect(calls.some((c) => c[0] === 'Close' && c[1] === 7)).toBe(true);
});

test('the commands are registered, in the Commands menu with their keys, and Ctrl+` moves to the terminal', () => {
  for (const n of ['cm_TerminalPane', 'cm_TerminalFocus', 'cm_TerminalFollow', 'cm_TerminalInsertNames', 'cm_TerminalInsertPaths']) expect(reg.hasCommand(n)).toBe(true);
  expect(reg.commandForKey('Ctrl+`')).toBe('cm_TerminalFocus');
  const cmds = menus.menuSpec().find((m) => m.label === 'Commands');
  const names = JSON.stringify(cmds);
  for (const n of ['cm_TerminalPane', 'cm_TerminalFollow', 'cm_TerminalInsertNames']) expect(names).toContain(n);
});

test('the AI tools may run none of the terminal commands', () => {
  for (const n of ['cm_TerminalPane', 'cm_TerminalFocus', 'cm_TerminalFollow', 'cm_TerminalInsertNames', 'cm_TerminalInsertPaths']) expect(commandVerdict(n).ok).toBe(false);
});
