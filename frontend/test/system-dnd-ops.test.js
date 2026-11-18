// @vitest-environment happy-dom
// Drag and drop operations (F-694, F-851): the modifier keys held at a drop pick copy, move or link by Total
// Commander's rules everywhere, and also by Finder's on macOS (Option copies, Cmd moves, Cmd+Option links).
import { describe, expect, test, vi } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, onFileDrop: () => {}, errText: String };
});

const { opFor } = await import('../src/features/system-dnd.js');

describe('opFor', () => {
  const k = (s) => ({ ctrlKey: s.includes('c'), shiftKey: s.includes('s'), altKey: s.includes('a'), metaKey: s.includes('m') });
  test('TC rules', () => {
    for (const plat of ['windows', 'linux', 'darwin']) {
      expect(opFor(k(''), plat)).toBe('copy');
      expect(opFor(k('s'), plat)).toBe('move');
      expect(opFor(k('cs'), plat)).toBe('link');
    }
    expect(opFor(k('a'), 'windows')).toBe('link');
    expect(opFor(k('m'), 'windows')).toBe('copy');
  });
  test('Finder rules on macOS', () => {
    expect(opFor(k('a'), 'darwin')).toBe('copy');
    expect(opFor(k('m'), 'darwin')).toBe('move');
    expect(opFor(k('ma'), 'darwin')).toBe('link');
  });
});
