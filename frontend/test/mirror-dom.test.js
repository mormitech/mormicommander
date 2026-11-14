// The pure pieces shared by both sides of a dialog window (core/mirror-dom.js, core/mirror-events.js).
import { describe, expect, test } from 'vitest';
import { cleanClass, copyAttr, PAGE_ONLY_KINDS, isBigDialog } from '../src/core/mirror-dom.js';
import { keyTakenHere, keyMsg, mouseMsg } from '../src/core/mirror-events.js';

const el = (sel) => ({ matches: (q) => q.split(',').some((p) => sel.includes(p.trim())) });
const copy = { idOf: (n) => (n ? n.id : 0) };

describe('mirror-dom', () => {
  test('the owning page\'s own classes are not copied', () => {
    expect(cleanClass('mc-box mc-mirror-box mc-mirror-fixed primary')).toBe('mc-box primary');
    expect(cleanClass(undefined)).toBe('');
  });
  test('inline handlers, autofocus and javascript: links are not copied', () => {
    expect(copyAttr('onclick', 'x()')).toBe(null);
    expect(copyAttr('OnMouseDown', 'x()')).toBe(null);
    expect(copyAttr('autofocus', '')).toBe(null);
    expect(copyAttr('href', ' javascript:alert(1)')).toBe(null);
    expect(copyAttr('href', 'https://example.org')).toBe('https://example.org');
    expect(copyAttr('class', 'a mc-mirror-back b')).toBe('a b');
    expect(copyAttr('data-i', '3')).toBe('3');
  });
  test('the page-only kinds are documented and fixed', () => {
    expect(PAGE_ONLY_KINDS).toEqual(['lister']);
    expect(Object.isFrozen(PAGE_ONLY_KINDS)).toBe(true);
  });
  test('big dialogs are resizable tool windows', () => {
    expect(isBigDialog(420, 200)).toBe(false);
    expect(isBigDialog(900, 300)).toBe(true);
    expect(isBigDialog(500, 600)).toBe(true);
  });
});

describe('mirror-events', () => {
  test('the keys the window keeps from its browser', () => {
    const k = (key, target = el('input'), mods = {}) => keyTakenHere({ key, target, ...mods });
    expect(k('Enter')).toBe(true);
    expect(k('Enter', el('textarea'))).toBe(false);
    expect(k('Escape')).toBe(true);
    expect(k('F5')).toBe(true);
    expect(k('r', el('input'), { metaKey: true })).toBe(true);
    expect(k('a')).toBe(false);
    expect(k('a', el('input [data-keycapture]'))).toBe(true);
    expect(k('Tab', el('input [data-keycapture]'))).toBe(false);
    expect(k('Enter', el('textarea [data-enter-submits]'))).toBe(true);
    expect(k('Enter', el('textarea [data-enter-submits]'), { shiftKey: true })).toBe(false);
  });
  test('messages name elements by their number and carry the modifiers', () => {
    const t = { id: 7 };
    expect(keyMsg(copy, { key: 'a', code: 'KeyA', shiftKey: true, repeat: true }, 'keydown', t))
      .toEqual({ t: 'key', type: 'keydown', i: 7, key: 'a', code: 'KeyA', ctrl: false, shift: true, alt: false, meta: false, r: true });
    expect(keyMsg(copy, { key: 'a', code: 'KeyA' }, 'keyup', null).i).toBe(0);
    const m = mouseMsg(copy, { target: t, clientX: 5, clientY: 6, screenX: 50, screenY: 60, button: 2, buttons: 2, detail: 1, ctrlKey: true }, 'contextmenu');
    expect(m).toEqual({ t: 'mouse', type: 'contextmenu', i: 7, x: 5, y: 6, sx: 50, sy: 60, b: 2, bs: 2, d: 1, ctrl: true, shift: false, alt: false, meta: false });
  });
});
