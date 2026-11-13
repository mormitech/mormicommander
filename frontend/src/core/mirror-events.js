// What a dialog window (dialog-window.js) sends back to the page that owns the dialog (core/mirror.js replays it), and
// which keys the window keeps from its own browser defaults. Pure functions over an event and the Copy
// (mirror-copy.js), so the tests check them without a window.

const mods = (ev) => ({ ctrl: !!ev.ctrlKey, shift: !!ev.shiftKey, alt: !!ev.altKey, meta: !!ev.metaKey });

// mouseMsg: a mouse event at its place in the dialog (the window's dialog starts at its top left corner), on the element
// the copy knows by number.
export function mouseMsg(copy, ev, type) {
  return { t: 'mouse', type, i: copy.idOf(ev.target), x: ev.clientX, y: ev.clientY, sx: ev.screenX, sy: ev.screenY, b: ev.button, bs: ev.buttons, d: ev.detail, ...mods(ev) };
}

// keyMsg: a key on the focused element of the copy.
export function keyMsg(copy, ev, type, focused) {
  const m = { t: 'key', type, i: copy.idOf(focused), key: ev.key, code: ev.code, ...mods(ev) };
  if (type === 'keydown') m.r = !!ev.repeat;
  return m;
}

// keyTakenHere tells whether the window's browser must not do what a key does by default: Enter (outside text areas:
// the owning page submits), Esc (it cancels there), F-keys and the browser's own shortcuts (reload, print, find), and
// every key in a key-capture field (Options > Keyboard shortcuts: the owning page writes the key's name into it, the
// window must not type the character as well).
export function keyTakenHere(ev) {
  const k = ev.key;
  const t = ev.target;
  if (t?.matches?.('[data-keycapture]') && k !== 'Tab') return true;
  const area = t?.matches?.('textarea, [contenteditable=""], [contenteditable="true"]');
  if (k === 'Enter' && !area) return true;
  if (k === 'Escape' || /^F\d{1,2}$/.test(k)) return true;
  return !!((ev.ctrlKey || ev.metaKey) && /^[rpfgj]$/i.test(k));
}
