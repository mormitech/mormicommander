// @vitest-environment happy-dom
// A played pointer drag (core/pointer.js, control mode's "pointer_drag") moves a real page dialog (core/dialogs.js
// modal) by its title, through the dialog's own pointer-capture code: the drag that was never tested by a script.
import { expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => (k === 'dialogsInWindows' ? false : d) }));

const { modal } = await import('../src/core/dialogs.js');
const { pointOf, dragGesture } = await import('../src/core/pointer.js');

test('dragging the title of a page dialog moves it; a double click on the title puts it back', async () => {
  let close;
  const p = modal((box, done) => { box.innerHTML = '<h2>Title</h2><input id="f">'; close = done; }, null, { kind: 'ptr-test' });
  const box = document.querySelector('.mc-box');
  const h2 = box.querySelector('h2');
  expect(box.classList.contains('mc-draggable')).toBe(true);
  box.getBoundingClientRect = () => ({ left: 300, top: 200, width: 300, height: 200, right: 600, bottom: 400, x: 300, y: 200 });
  h2.getBoundingClientRect = () => ({ left: 300, top: 200, width: 300, height: 30, right: 600, bottom: 230, x: 300, y: 200 });
  document.elementFromPoint = () => h2; // the title is under the press; afterwards capture decides where moves go
  const from = pointOf(h2);
  await dragGesture(document, { from, to: { x: from.x + 100, y: from.y + 50 }, steps: 4 });
  expect(box.style.transform).toBe('translate(100px, 50px)');
  expect(box.classList.contains('mc-dragging')).toBe(false);
  h2.dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
  expect(box.style.transform).toBe('');
  close('x');
  expect(await p).toBe('x');
});
