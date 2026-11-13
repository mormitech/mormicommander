// Total Commander's key reference (src/core/tckeys.js, FEATURES.md section 8) against the default keymap: on every
// system each TC key must run its TC command, and the list must be well formed. The same list is pressed key by key in
// the real app by scripts/qa_keys.py.
import { describe, expect, test } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const reg = await import('../src/core/registry.js');
const { TC_KEY_REFERENCE, PRESS, MAP } = await import('../src/core/tckeys.js');
const { normalizeCombo } = await import('../src/core/keys.js');

test('the reference is well formed', () => {
  expect(TC_KEY_REFERENCE.length).toBeGreaterThan(100);
  const seen = new Set();
  for (const r of TC_KEY_REFERENCE) {
    expect(r.id).toMatch(/^F-\d{3}$/);
    expect(r.command).toMatch(/^cm_\w+$/);
    expect([PRESS, MAP]).toContain(r.how);
    expect(normalizeCombo(r.key)).toBe(r.key); // written the canonical way
    expect(seen.has(r.key)).toBe(false); // each key once
    seen.add(r.key);
  }
});

test('keys that would leave the sandbox are never pressed', () => {
  const pressed = new Set(TC_KEY_REFERENCE.filter((r) => r.how === PRESS).map((r) => r.command));
  for (const c of ['cm_Exit', 'cm_Minimize', 'cm_Edit', 'cm_PrintFile', 'cm_DeletePermanent', 'cm_CopyToClipboard', 'cm_CutToClipboard', 'cm_PasteFromClipboard', 'cm_RunKeepOpen', 'cm_ListExternal', 'cm_HelpIndex']) {
    expect(pressed.has(c)).toBe(false);
  }
});

describe.each(['linux', 'windows', 'darwin'])('on %s', (platform) => {
  test('every TC key runs its TC command', () => {
    reg.setupKeys(platform, {});
    const wrong = TC_KEY_REFERENCE.filter((r) => reg.commandForKey(r.key) !== r.command).map((r) => `${r.key}: ${reg.commandForKey(r.key) || '(none)'} instead of ${r.command}`);
    expect(wrong).toEqual([]);
  });
});
