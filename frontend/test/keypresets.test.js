// Tests of the keyboard presets of the first-run setup (core/keypresets.js) and how the registry lays them into the
// keymap (setPresetKeys): the default preset changes nothing, the Mac-native one keeps Shift+arrows selection (F-119),
// the user's own keys still win, and the setup page opens by itself only on a real first run.
import { beforeAll, expect, test } from 'vitest';

// registry.js uses requestAnimationFrame through busy.js; Node has none
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const reg = await import('../src/core/registry.js');
const { DEFAULT_KEYS, MAC_KEYS } = await import('../src/core/keymap.js');
const { normalizeCombo } = await import('../src/core/keys.js');
const kp = await import('../src/core/keypresets.js');

beforeAll(() => {
  reg.setApp({ name: 'app' });
  reg.setupKeys('darwin', { 'Ctrl+K': 'cm_Copy', J: 'cm_RenMov' });
});

test('the default preset is Total Commander and adds no keys', () => {
  expect(kp.DEFAULT_PRESET).toBe('tc');
  expect(kp.presetKeys('tc')).toEqual({});
  expect(kp.presetName(undefined)).toBe('tc');
  expect(kp.presetName('nonsense')).toBe('tc');
  expect(kp.presetName('constructor')).toBe('tc');
  const before = reg.allKeys();
  reg.setPresetKeys(kp.presetKeys('tc'));
  expect(reg.allKeys()).toEqual(before);
});

test('every preset key is a canonical combo that runs a known command', () => {
  const known = new Set([...Object.values(DEFAULT_KEYS), ...Object.values(MAC_KEYS), 'cm_ShowQuickSearch', 'cm_FocusCmdLine', 'cm_EjectVolume']);
  for (const [name, p] of Object.entries(kp.KEY_PRESETS)) {
    expect(p.label, name).toBeTruthy();
    for (const [combo, cmd] of Object.entries(p.keys)) {
      expect(normalizeCombo(combo), `${name}: ${combo}`).toBe(combo);
      expect(known.has(cmd), `${name}: ${combo} → ${cmd}`).toBe(true);
    }
  }
});

test('the Mac-native preset keeps Shift+arrows selection and adds Finder keys', () => {
  reg.setPresetKeys(kp.presetKeys('mac'));
  expect(reg.commandForKey('Shift+Up')).toBe('cm_SelUp');
  expect(reg.commandForKey('Shift+Down')).toBe('cm_SelDown');
  expect(reg.commandForKey('Shift+Left')).toBe('cm_SelLeft');
  expect(reg.commandForKey('Shift+Right')).toBe('cm_SelRight');
  expect(reg.commandForKey('Space')).toBe('cm_List');
  expect(reg.commandForKey('Cmd+O')).toBe('cm_Enter');
  expect(reg.commandForKey('Shift+Cmd+N')).toBe('cm_MkDir');
  expect(reg.commandForKey('F5')).toBe('cm_Copy'); // TC keys stay
  expect(reg.commandForKey('Ctrl+K')).toBe('cm_Copy'); // the user's keys stay on top
});

test('the Marta-like preset adds Mac keys but keeps Space selecting (board issue #22)', () => {
  reg.setPresetKeys(kp.presetKeys('marta'));
  expect(kp.KEY_PRESETS.marta.label).toBe('Marta-like');
  expect(reg.commandForKey('Cmd+O')).toBe('cm_Enter');
  expect(reg.commandForKey('Shift+Cmd+N')).toBe('cm_MkDir');
  expect(reg.commandForKey('Cmd+K')).toBe('cm_FtpConnect');
  expect(reg.commandForKey('Space')).toBe('cm_SelectToggle');
  expect(reg.commandForKey('Shift+Down')).toBe('cm_SelDown');
  expect(reg.commandForKey('F5')).toBe('cm_Copy');
  expect(reg.commandForKey('Ctrl+K')).toBe('cm_Copy'); // the user's keys stay on top
});

test('the vim preset adds letter keys, but the user keeps the last word', () => {
  reg.setPresetKeys(kp.presetKeys('vim'));
  expect(reg.commandForKey('K')).toBe('cm_CursorUp');
  expect(reg.commandForKey('Shift+G')).toBe('cm_GoToLastEntry');
  expect(reg.commandForKey('Shift+;')).toBe('cm_FocusCmdLine');
  expect(reg.commandForKey('J')).toBe('cm_RenMov'); // the user's own J wins over the preset's
  expect(reg.commandForKey('Space')).toBe('cm_SelectToggle'); // (the Mac preset's Space is gone again)
});

test('going back to Total Commander restores the default keys', () => {
  reg.setPresetKeys(kp.presetKeys('vim'));
  reg.setPresetKeys(kp.presetKeys('tc'));
  expect(reg.commandForKey('K')).toBe('');
  expect(reg.commandForKey('Space')).toBe('cm_SelectToggle');
});

test('the setup page opens by itself only on a first run outside control mode and the demo', () => {
  expect(kp.shouldShowSetup({ firstRun: true }, false)).toBe(true);
  expect(kp.shouldShowSetup({ firstRun: false }, false)).toBe(false);
  expect(kp.shouldShowSetup({ firstRun: true }, true)).toBe(false);
  expect(kp.shouldShowSetup({ firstRun: true, control: true }, false)).toBe(false);
  expect(kp.shouldShowSetup({ firstRun: true, demo: true }, false)).toBe(false);
  expect(kp.shouldShowSetup(null, false)).toBe(false);
});

test('the F5 test says whether fn is needed', () => {
  expect(kp.fnAdvice({ pressed: false, platform: 'darwin', fnKeys: 'media' })).toMatch(/hold fn/);
  expect(kp.fnAdvice({ pressed: true, platform: 'darwin', fnKeys: 'standard' })).toMatch(/without fn/);
  expect(kp.fnAdvice({ pressed: true, platform: 'darwin', fnKeys: 'media' })).toMatch(/hold fn/);
  expect(kp.fnAdvice({ pressed: true, platform: 'darwin', fnKeys: 'media' })).toMatch(/standard function keys/);
  expect(kp.fnAdvice({ pressed: true, platform: 'windows', fnKeys: '' })).toMatch(/F5 works/);
  expect(kp.fnAdvice({ pressed: false, platform: 'linux', fnKeys: '' })).toMatch(/Press F5/);
});
