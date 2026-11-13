// @vitest-environment happy-dom
// The mask box of Select files (Num+): Alt+Down opens the list of masks used before, Up/Down move in it or step through
// the history (src/features/select-mask-keys.js). modal() stops key events before they reach the elements inside a
// dialog, so the keys must come through meta.onKey; the last test checks that route on a real modal() (board #92).
import { describe, expect, test, vi } from 'vitest';
import { maskKey } from '../src/features/select-mask-keys.js';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});

const k = (key, altKey = false) => ({ key, altKey });
const closed = (historyLength, at = -1) => ({ listOpen: false, count: 0, on: -1, at, historyLength });
const opened = (count, on = -1) => ({ listOpen: true, count, on, at: -1, historyLength: count });

describe('list closed: Up/Down step through the history', () => {
  test('Down from the start takes the newest mask', () => expect(maskKey(k('ArrowDown'), closed(3))).toEqual({ do: 'step', at: 0 }));
  test('Down goes to older masks and stops at the oldest', () => {
    expect(maskKey(k('ArrowDown'), closed(3, 1))).toEqual({ do: 'step', at: 2 });
    expect(maskKey(k('ArrowDown'), closed(3, 2))).toEqual({ do: 'step', at: 2 });
  });
  test('Up goes back and stops at the newest', () => {
    expect(maskKey(k('ArrowUp'), closed(3, 2))).toEqual({ do: 'step', at: 1 });
    expect(maskKey(k('ArrowUp'), closed(3, -1))).toEqual({ do: 'step', at: 0 });
  });
  test('no history: the keys are left alone', () => expect(maskKey(k('ArrowDown'), closed(0))).toBeNull());
  test('other keys type as usual', () => {
    expect(maskKey(k('a'), closed(3))).toBeNull();
    expect(maskKey(k('Enter'), closed(3))).toBeNull();
  });
});

describe('Alt+Down and the open list', () => {
  test('Alt+Down toggles the list, open or closed', () => {
    expect(maskKey(k('ArrowDown', true), closed(0))).toEqual({ do: 'toggle' });
    expect(maskKey(k('ArrowDown', true), opened(2, 1))).toEqual({ do: 'toggle' });
  });
  test('Down highlights the first entry, then the next, and stops at the last', () => {
    expect(maskKey(k('ArrowDown'), opened(3))).toEqual({ do: 'highlight', index: 0 });
    expect(maskKey(k('ArrowDown'), opened(3, 0))).toEqual({ do: 'highlight', index: 1 });
    expect(maskKey(k('ArrowDown'), opened(3, 2))).toEqual({ do: 'highlight', index: 2 });
  });
  test('Up moves back and stops at the first', () => {
    expect(maskKey(k('ArrowUp'), opened(3, 2))).toEqual({ do: 'highlight', index: 1 });
    expect(maskKey(k('ArrowUp'), opened(3, 0))).toEqual({ do: 'highlight', index: 0 });
  });
  test('Enter takes the highlighted entry; with none it submits the dialog as usual', () => {
    expect(maskKey(k('Enter'), opened(3, 1))).toEqual({ do: 'pick', index: 1 });
    expect(maskKey(k('Enter'), opened(3))).toBeNull();
  });
  test('an open but empty list ("No masks used yet") with no history leaves Up/Down alone', () => {
    expect(maskKey(k('ArrowDown'), { listOpen: true, count: 0, on: -1, at: -1, historyLength: 0 })).toBeNull();
  });
});

describe('modal() routes keys to meta.onKey', () => {
  test('a key on the input reaches onKey; a listener on the input itself never runs', async () => {
    const { modal } = await import('../src/core/dialogs.js');
    let input;
    const seen = [];
    const inputSaw = [];
    const p = modal((box) => {
      box.innerHTML = '<input id="mask" autofocus>';
      input = box.querySelector('#mask');
      input.addEventListener('keydown', (ev) => inputSaw.push(ev.key));
    }, null, { kind: 'select-group', onKey: (ev) => { seen.push(ev.key); return ev.key === 'ArrowDown'; } });
    const down = new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true });
    input.dispatchEvent(down);
    expect(seen).toEqual(['ArrowDown']);
    expect(down.defaultPrevented).toBe(true);
    expect(inputSaw).toEqual([]);
    const a = new KeyboardEvent('keydown', { key: 'a', bubbles: true, cancelable: true });
    input.dispatchEvent(a);
    expect(a.defaultPrevented).toBe(false); // not used by the dialog: the character still types
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }));
    await expect(p).resolves.toBeNull();
  });
});
