import { beforeAll, expect, test } from 'vitest';

// registry.js uses requestAnimationFrame through busy.js; Node has none
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const reg = await import('../src/core/registry.js');
const { DEFAULT_KEYS, MAC_KEYS } = await import('../src/core/keymap.js');
const { normalizeCombo } = await import('../src/core/keys.js');

beforeAll(() => {
  reg.setApp({ name: 'app' });
  reg.command({ name: 'cm_TestOne', label: 'Test one', run: (app, args) => ({ app: app.name, args }) });
  reg.command({ name: 'cm_TestKeyed', label: 'Keyed', run: () => 1, keys: ['Ctrl+Alt+Shift+F11', 'F5'] });
  reg.setupKeys('linux', { 'Ctrl+K': 'cm_TestOne', F2: '' });
});

test('TC keys are mapped by default', () => {
  expect(reg.commandForKey('F5')).toBe('cm_Copy'); // a command's own key never takes a TC key
  expect(reg.commandForKey('Ctrl+Num+')).toBe('cm_SelectAll');
  expect(reg.commandForKey('Alt+F1')).toBe('cm_LeftOpenDrives');
});

test('user keys win, "" removes a key', () => {
  expect(reg.commandForKey('Ctrl+K')).toBe('cm_TestOne');
  expect(reg.commandForKey('F2')).toBe('');
});

test("a command's own free key is used", () => {
  expect(reg.commandForKey('Ctrl+Alt+Shift+F11')).toBe('cm_TestKeyed');
});

test('numpad fallback only when asked (empty command line)', () => {
  expect(reg.commandForKey('Shift+8')).toBe('');
  expect(reg.commandForKey('Shift+8', true)).toBe('cm_ExchangeSelection');
});

test('Mac keys only on macOS', () => {
  expect(reg.commandForKey('Cmd+C')).toBe('');
  reg.setupKeys('darwin', {});
  expect(reg.commandForKey('Cmd+C')).toBe('cm_CopyToClipboard');
  expect(reg.commandForKey('Shift+Cmd+.')).toBe('cm_SwitchHidSys');
  expect(reg.commandForKey('Ctrl+C')).toBe('cm_CopyToClipboard'); // TC keys stay on macOS
});

test('run passes the app and args; unknown names reject with NotReady', async () => {
  expect(await reg.run('cm_TestOne', { x: 1 })).toEqual({ app: 'app', args: { x: 1 } });
  await expect(reg.run('cm_Nope')).rejects.toBeInstanceOf(reg.NotReady);
});

test('every default key is written the canonical way and names a cm_ command', () => {
  for (const [k, v] of Object.entries({ ...DEFAULT_KEYS, ...MAC_KEYS })) {
    expect(v).toMatch(/^cm_\w+$/);
    expect(normalizeCombo(normalizeCombo(k))).toBe(normalizeCombo(k));
  }
});

test('the macOS Cmd aliases of F-285 run the same commands, and exist only on macOS', () => {
  const aliases = {
    'Cmd+C': 'cm_CopyToClipboard', 'Cmd+X': 'cm_CutToClipboard', 'Cmd+V': 'cm_PasteFromClipboard', 'Cmd+A': 'cm_SelectAll',
    'Cmd+T': 'cm_OpenNewTab', 'Cmd+W': 'cm_CloseCurrentTab', 'Cmd+Q': 'cm_Exit', 'Cmd+,': 'cm_Config',
    'Cmd+Backspace': 'cm_Delete', 'Shift+Cmd+.': 'cm_SwitchHidSys', 'Cmd+R': 'cm_RereadSource', 'Cmd+Up': 'cm_GoToParent',
    'Cmd+Down': 'cm_Enter', 'Cmd+I': 'cm_Properties', 'Cmd+L': 'cm_EditPath', 'Cmd+F': 'cm_SearchFor', 'Shift+Cmd+G': 'cm_EditPath',
  };
  reg.setupKeys('darwin', {});
  for (const [k, v] of Object.entries(aliases)) expect(reg.commandForKey(k), k).toBe(v);
  for (const k of ['Ctrl+C', 'Ctrl+X', 'Ctrl+V']) expect(reg.commandForKey(k), k).toMatch(/Clipboard$/); // TC keys too
  for (const plat of ['windows', 'linux']) {
    reg.setupKeys(plat, {});
    for (const k of Object.keys(aliases)) expect(reg.commandForKey(k), `${k} on ${plat}`).toBe('');
    expect(reg.commandForKey('Ctrl+X')).toBe('cm_CutToClipboard');
  }
});

test('recent runs are noted for control mode, newest last; clear empties the list', async () => {
  reg.clearRecentRuns();
  await reg.run('cm_TestOne');
  await reg.run('cm_Nope').catch(() => {});
  expect(reg.recentRuns()).toEqual([{ name: 'cm_TestOne', how: 'ran' }, { name: 'cm_Nope', how: 'notready' }]);
  reg.clearRecentRuns();
  expect(reg.recentRuns()).toEqual([]);
  for (let i = 0; i < 150; i++) await reg.run('cm_TestOne');
  expect(reg.recentRuns().length).toBe(100);
});
