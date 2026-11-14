// @vitest-environment happy-dom
// Tests of core/midpan.js: middle-button panning of the file list with a grab cursor.
import { expect, test } from 'vitest';
import { attachMiddlePan, PAN_CLASS } from '../src/core/midpan.js';

const ptr = (el, type, o) => {
  const ev = new MouseEvent(type, { bubbles: true, cancelable: true, ...o });
  Object.defineProperty(ev, 'pointerId', { value: 1 });
  Object.defineProperty(ev, 'pointerType', { value: 'mouse' });
  el.dispatchEvent(ev);
  return ev;
};

function list() {
  const el = document.createElement('div');
  document.body.appendChild(el);
  let top = 500, left = 0;
  Object.defineProperty(el, 'scrollTop', { get: () => top, set: (v) => { top = Math.max(0, v); } });
  Object.defineProperty(el, 'scrollLeft', { get: () => left, set: (v) => { left = Math.max(0, v); } });
  return el;
}

test('holding the middle button and moving drags the content with the mouse', () => {
  const el = list();
  const off = attachMiddlePan(el);
  const d = ptr(el, 'pointerdown', { button: 1, buttons: 4, clientX: 100, clientY: 300 });
  expect(d.defaultPrevented).toBe(true);
  expect(document.documentElement.classList.contains(PAN_CLASS)).toBe(true);
  ptr(el, 'pointermove', { buttons: 4, clientX: 100, clientY: 260 }); // mouse up 40px: content follows, list scrolls down
  expect(el.scrollTop).toBe(540);
  ptr(el, 'pointermove', { buttons: 4, clientX: 100, clientY: 400 });
  expect(el.scrollTop).toBe(400);
  ptr(el, 'pointerup', { button: 1, buttons: 0, clientX: 100, clientY: 400 });
  expect(document.documentElement.classList.contains(PAN_CLASS)).toBe(false);
  ptr(el, 'pointermove', { buttons: 0, clientX: 100, clientY: 0 });
  expect(el.scrollTop).toBe(400);
  off();
});

test('the left button does not pan; a move with the button already up ends the pan', () => {
  const el = list();
  attachMiddlePan(el);
  ptr(el, 'pointerdown', { button: 0, buttons: 1, clientX: 0, clientY: 300 });
  ptr(el, 'pointermove', { buttons: 1, clientX: 0, clientY: 200 });
  expect(el.scrollTop).toBe(500);
  expect(document.documentElement.classList.contains(PAN_CLASS)).toBe(false);
  ptr(el, 'pointerdown', { button: 1, buttons: 4, clientX: 0, clientY: 300 });
  ptr(el, 'pointermove', { buttons: 0, clientX: 0, clientY: 200 });
  expect(el.scrollTop).toBe(500);
  expect(document.documentElement.classList.contains(PAN_CLASS)).toBe(false);
});

test('the middle mousedown is cancelled (no autoscroll)', () => {
  const el = list();
  attachMiddlePan(el);
  expect(ptr(el, 'mousedown', { button: 1 }).defaultPrevented).toBe(true);
  expect(ptr(el, 'mousedown', { button: 0 }).defaultPrevented).toBe(false);
});
