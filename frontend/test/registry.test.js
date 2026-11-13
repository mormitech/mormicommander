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
