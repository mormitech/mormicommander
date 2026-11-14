// Tests of native Quick Look (board issue #19, F-863): what is shown for a panel, ⌘Y on macOS, and the Mac-native
// preset's Space and ⌘Y, which stay the built-in viewer unless the setting "quickLookSpace" is on.
import { beforeAll, expect, test } from 'vitest';

// registry.js uses requestAnimationFrame through busy.js; Node has none
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const reg = await import('../src/core/registry.js');
const { DEFAULT_KEYS, MAC_KEYS } = await import('../src/core/keymap.js');
const kp = await import('../src/core/keypresets.js');
const { quickLookPaths, FOLLOW_DELAY } = await import('../src/features/quicklook-lib.js');

beforeAll(() => { reg.setApp({ name: 'app' }); });

test('Quick Look shows the entry under the cursor, nothing on ".." or an empty panel', () => {
  const panel = (cur) => ({ cur, fullPath: (n) => `/tmp/x/${n}` });
  expect(quickLookPaths(panel({ name: 'a.txt' }))).toEqual(['/tmp/x/a.txt']);
  expect(quickLookPaths(panel({ name: 'sub', dir: true }))).toEqual(['/tmp/x/sub']);
  expect(quickLookPaths(panel({ name: '..', up: true }))).toEqual([]);
  expect(quickLookPaths(panel(undefined))).toEqual([]);
  expect(quickLookPaths(null)).toEqual([]);
  expect(FOLLOW_DELAY).toBeGreaterThan(0);
});

test('⌘Y opens Quick Look on macOS only and Space keeps its Total Commander meaning', () => {
  expect(MAC_KEYS['Cmd+Y']).toBe('cm_QuickLook');
  expect(DEFAULT_KEYS['Cmd+Y']).toBeUndefined();
  reg.setupKeys('darwin', {});
  reg.setPresetKeys(kp.presetKeys('tc'));
  expect(reg.commandForKey('Cmd+Y')).toBe('cm_QuickLook');
  expect(reg.commandForKey('Space')).toBe('cm_SelectToggle');
  reg.setupKeys('windows', {});
  expect(reg.commandForKey('Cmd+Y')).toBe('');
});

test('the Mac-native preset keeps the built-in viewer unless quickLookSpace is on', () => {
  reg.setupKeys('darwin', {});
  expect(kp.presetKeys('mac')).toEqual(kp.presetKeys('mac', { quickLookSpace: false }));
  reg.setPresetKeys(kp.presetKeys('mac'));
  expect(reg.commandForKey('Space')).toBe('cm_List');
  expect(reg.commandForKey('Cmd+Y')).toBe('cm_List');
  reg.setPresetKeys(kp.presetKeys('mac', { quickLookSpace: true }));
  expect(reg.commandForKey('Space')).toBe('cm_QuickLook');
  expect(reg.commandForKey('Cmd+Y')).toBe('cm_QuickLook');
  expect(reg.commandForKey('Shift+Up')).toBe('cm_SelUp'); // the rest of the preset stays
  // the setting changes only the Mac-native preset
  expect(kp.presetKeys('tc', { quickLookSpace: true })).toEqual({});
  expect(kp.presetKeys('vim', { quickLookSpace: true })).toEqual(kp.presetKeys('vim'));
  // the user's own keys still win
  reg.setupKeys('darwin', { Space: 'cm_SelectToggle' });
  expect(reg.commandForKey('Space')).toBe('cm_SelectToggle');
  reg.setPresetKeys({});
});
