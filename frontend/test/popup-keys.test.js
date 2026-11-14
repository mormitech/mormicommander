// @vitest-environment happy-dom
// Popup list keys added in board issue #111 (src/core/popup.js): Right opens a submenu item and Left goes back (only
// when the caller asks), and the typed filter (only when the caller asks). Without those options the list keys exactly
// as before: letters jump to and choose items, the arrows do nothing.
import { afterEach, describe, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});

const { showList, popupMatch, currentPopup, closePopup } = await import('../src/core/popup.js');

const press = (key, more = {}) => window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...more }));
const shownLabels = () => [...document.querySelectorAll('.mc-popup .it')].filter((d) => !d.hidden).map((d) => d.firstElementChild.textContent);
const items = () => [
  { label: 'Alpha', detail: '/a', value: 1 },
  { label: 'Work ▸', value: 2, sub: true },
  { sep: true },
  { label: 'Beta', detail: '/b/src', value: 3 },
  { label: 'Work › Src', detail: '/src', value: 4, only: true },
];

afterEach(() => { closePopup(); document.body.innerHTML = ''; });

describe('popupMatch', () => {
  test('every word, in the label or the detail, ignoring case', () => {
    expect(popupMatch({ label: 'Projects', detail: '/home/me/src' }, 'proj SRC')).toBe(true);
    expect(popupMatch({ label: 'Projects', detail: '/home/me/src' }, 'proj lib')).toBe(false);
  });
  test('an empty query keeps all but the "only while filtering" items; separators never match', () => {
    expect(popupMatch({ label: 'x' }, '')).toBe(true);
    expect(popupMatch({ label: 'x', only: true }, '  ')).toBe(false);
    expect(popupMatch({ label: 'x', only: true }, 'x')).toBe(true);
    expect(popupMatch({ sep: true }, '')).toBe(false);
  });
});

describe('without the new options', () => {
  test('a letter still chooses the only item that starts with it', async () => {
    const r = showList({ items: items() });
    press('b');
    expect((await r).item.value).toBe(3);
  });
  test('Right and Left do nothing (no item is marked as a submenu)', async () => {
    const r = showList({ items: items().map(({ sub, ...it }) => it), index: 1 });
    press('ArrowRight');
    press('ArrowLeft');
    expect(currentPopup()).not.toBeNull();
    press('Enter');
    expect((await r).item.value).toBe(2);
  });
});

describe('Right and Left for submenus', () => {
  test('Right chooses a submenu item, not a plain one', async () => {
    const r = showList({ items: items(), index: 0, left: true });
    press('ArrowRight');
    expect(currentPopup()).not.toBeNull();
    press('ArrowDown');
    press('ArrowRight');
    expect((await r).item.value).toBe(2);
  });
  test('Left closes like Esc when the caller asks', async () => {
    const r = showList({ items: items(), left: true });
    press('ArrowLeft');
    expect(await r).toBeNull();
  });
});

describe('typed filter', () => {
  test('letters filter instead of choosing; submenu entries show while typing', async () => {
    const r = showList({ items: items(), filter: true });
    expect(shownLabels()).toEqual(['Alpha', 'Work ▸', 'Beta']);
    press('s'); press('r'); press('c');
    expect(currentPopup().filter).toBe('src');
    expect(shownLabels()).toEqual(['Beta', 'Work › Src']);
    expect(document.querySelector('.mc-popup .q').textContent).toBe('Filter: src');
    press('ArrowDown');
    press('Enter');
    expect((await r).item.value).toBe(4);
  });
  test('Backspace takes a letter back; Esc clears the filter first, then closes', async () => {
    const r = showList({ items: items(), filter: true });
    press('z'); press('z');
    expect(shownLabels()).toEqual([]);
    press('Backspace');
    expect(currentPopup().filter).toBe('z');
    press('Escape');
    expect(currentPopup().filter).toBe('');
    expect(shownLabels()).toEqual(['Alpha', 'Work ▸', 'Beta']);
    press('Escape');
    expect(await r).toBeNull();
  });
  test('Enter with nothing left chooses nothing', async () => {
    showList({ items: items(), filter: true });
    press('q');
    press('Enter');
    expect(currentPopup()).not.toBeNull();
  });
});
