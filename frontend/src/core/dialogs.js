// Dialogs drawn by the page (never alert()). One modal() helper: a box over the page, Esc = cancel, keys don't reach the
// panels while it is open, focus comes back afterwards. Every open dialog is listed, so control mode can read it
// ("dialog") and answer it ("dialog_answer") by pressing its buttons and filling its fields, the same as a person.
//
// With the setting "dialogsInWindows" (on unless false) every dialog shows in a native window of its own: the box is
// still built here, in this page, and stays here invisibly while the page is dimmed; a window shows a live copy of it
// and sends back what the user does there (core/mirror.js, feature_dialog_window.go). Nothing changes for the code
// that builds a dialog or for control mode. A dialog drawn as a floating box (position: fixed, a menu at the pointer)
// is drawn inside the dialog window below it when there is one, else in the page; the kinds in PAGE_ONLY_KINDS
// (mirror-dom.js) and in the setting "dialogsInPage" (a list of kinds) always stay in the page.
//
//   await messageDialog({Title, Message, Buttons: ['Overwrite', 'Skip', 'Cancel'], DefaultButton, CancelButton}) → label
//   await inputDialog({title, label, value, select: [0, 4], okLabel}) → text or null
//   await formDialog(title, html, read, setup) → what read(box) returns, or null
//   infoDialog(html) → a box with a close button

import './dialogs.css';
import { waitForUser } from './busy.js';
import { getSetting } from './settings.js';
import { mirrorDialog, mirrorAvailable, now } from './mirror.js';
import { PAGE_ONLY_KINDS } from './mirror-dom.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
export { esc };

const open = []; // the dialogs on screen, last = top

// Dialogs in the page (dialogsInWindows off, or a kind that stays in the page) can be moved like windows: drag the
// title (or any empty part of the box). The box is shifted with a transform and kept inside the app window. Where a kind of dialog was left is remembered for the
// session, so Options or the copy dialog open where you put them last; double-clicking the title puts it back in the
// middle. Popup menus drawn as boxes (position: fixed, placed at the pointer) are not draggable.
const placed = new Map(); // dialog kind → {x, y} offset from the centred position
const KEEP = 40; // pixels of the box that always stay inside the window, so it can be grabbed again

function draggable(box, kind) {
  if (getComputedStyle(box).position === 'fixed') return;
  box.classList.add('mc-draggable');
  let off = { x: 0, y: 0 };
  const apply = () => {
    // Clamp against the box's centred position so a dialog never ends up out of reach, also after the window shrank.
    box.style.transform = '';
    const r = box.getBoundingClientRect();
    const minX = KEEP - r.right, maxX = innerWidth - KEEP - r.left;
    const minY = -r.top, maxY = innerHeight - KEEP - r.top;
    off = { x: Math.min(maxX, Math.max(minX, off.x)), y: Math.min(maxY, Math.max(minY, off.y)) };
    box.style.transform = off.x || off.y ? `translate(${off.x}px, ${off.y}px)` : '';
  };
  if (kind && placed.has(kind)) { off = { ...placed.get(kind) }; apply(); }
  const isHandle = (t) => t === box || t.closest('h2, .mc-drag') && !t.closest('input, select, textarea, button, a, [contenteditable]');
  box.addEventListener('pointerdown', (ev) => {
    if (ev.button !== 0 || !isHandle(ev.target)) return;
    ev.preventDefault();
    const start = { x: ev.clientX - off.x, y: ev.clientY - off.y };
    box.setPointerCapture(ev.pointerId);
    box.classList.add('mc-dragging');
    const move = (e) => { off = { x: e.clientX - start.x, y: e.clientY - start.y }; apply(); };
    const up = () => {
      box.removeEventListener('pointermove', move);
      box.removeEventListener('pointerup', up);
      box.removeEventListener('pointercancel', up);
      box.classList.remove('mc-dragging');
      if (kind) placed.set(kind, off);
    };
    box.addEventListener('pointermove', move);
    box.addEventListener('pointerup', up);
    box.addEventListener('pointercancel', up);
  });
  box.addEventListener('dblclick', (ev) => {
    if (!ev.target.closest('h2, .mc-drag')) return;
    off = { x: 0, y: 0 };
    apply();
    if (kind) placed.delete(kind);
  });
}

// modal shows a box; build fills it and gets done(value) to close it with an answer. meta: {kind} for control mode,
// and optionally onKey(ev) → true when the dialog handles a key itself (keys don't reach elements inside otherwise), and
// onClose() for the close button of its dialog window (else that button does what Esc does: see requestClose below).
// windowsWanted tells whether a dialog of this kind goes to a window of its own.
function windowsWanted(kind) {
  if (!mirrorAvailable() || getSetting('dialogsInWindows', true) === false) return false;
  if (PAGE_ONLY_KINDS.includes(kind)) return false;
  const extra = getSetting('dialogsInPage', []);
  return !(Array.isArray(extra) && extra.includes(kind));
}

export function modal(build, cancelValue, meta = {}) {
  const t0 = now();
  return waitForUser(new Promise((resolve) => {
    const prevFocus = document.activeElement;
    const back = document.createElement('div');
    back.className = 'mc-modal';
    back.setAttribute('role', 'dialog');
    back.setAttribute('aria-modal', 'true');
    const box = document.createElement('form');
    box.className = 'mc-box';
    box.autocomplete = 'off';
    back.append(box);
    let closed = false;
    const entry = { box, meta, done: null };
    const done = (v) => {
      if (closed) return;
      closed = true;
      entry.mirror?.close();
      entry.layerOf?.removeLayer(back);
      back.remove();
      open.splice(open.indexOf(entry), 1);
      window.removeEventListener('keydown', key, true);
      prevFocus?.focus?.();
      resolve(v);
    };
    entry.done = done;
    entry.cancel = () => done(cancelValue);
    // requestClose: the close button of the dialog's window. It goes the way Esc goes, so a dialog that handles Esc
    // itself (Compare by content asks to save changes) does the same here; meta.onClose replaces that when given.
    entry.requestClose = () => {
      if (closed) return;
      if (meta.onClose) { meta.onClose(); return; }
      const ae = document.activeElement;
      const target = ae && box.contains(ae) ? ae : box;
      const ev = new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', bubbles: true, cancelable: true, composed: true, view: window });
      target.dispatchEvent(ev);
      // (nothing took the key although this dialog is on top: cancel it, as before)
      if (!closed && !ev.defaultPrevented && open[open.length - 1] === entry) done(cancelValue);
    };
    const key = (ev) => {
      if (open[open.length - 1] !== entry) return;
      // meta.onKey(ev) → true: the dialog used the key itself (F2 Queue in the copy dialog, a dropdown's Esc)
      if (meta.onKey?.(ev)) { ev.preventDefault(); ev.stopImmediatePropagation(); return; }
      if (ev.key === 'Escape') { ev.preventDefault(); ev.stopImmediatePropagation(); done(cancelValue); return; }
      ev.stopImmediatePropagation(); // the panels' keys are not for it while a dialog is open
    };
    window.addEventListener('keydown', key, true);
    build(box, done);
    const below = open[open.length - 1];
    open.push(entry);
    document.body.append(back);
    // where it shows: a window of its own, a layer of the dialog window below it (a floating box), or the page
    let mode = 'page';
    if (windowsWanted(meta.kind)) {
      if (getComputedStyle(box).position !== 'fixed') mode = 'window';
      else if (below?.mirror?.alive) mode = 'layer';
    }
    if (mode === 'page') draggable(box, meta.kind);
    const first = box.querySelector('[autofocus]') || box.querySelector('.primary') || box.querySelector('button');
    first?.focus();
    if (first?.select && first.dataset.select) { const [a, b] = first.dataset.select.split(',').map(Number); first.setSelectionRange(a, b); }
    if (mode === 'window') entry.mirror = mirrorDialog(entry, back, box, t0, () => { if (!closed) draggable(box, meta.kind); });
    if (mode === 'layer') {
      // (placed by its builder in a moment, often in an animation frame: the layer is made after that)
      const layerOf = below.mirror;
      back.classList.add('mc-mirror-layer');
      requestAnimationFrame(() => { if (!closed && layerOf.alive) { entry.layerOf = layerOf; layerOf.addLayer(back); } });
    }
  }));
}

export const dialogOpen = () => open.length > 0;

// currentDialog describes the top dialog for control mode: title, message, buttons, fields.
export function currentDialog() {
  const d = open[open.length - 1];
  if (!d) return null;
  const box = d.box;
  const fields = {};
  box.querySelectorAll('input, select, textarea').forEach((el) => {
    const id = el.id || el.name;
    if (!id) return;
    fields[id] = el.type === 'checkbox' || el.type === 'radio' ? el.checked : el.value;
  });
  return {
    kind: d.meta.kind || '',
    title: box.querySelector('h2')?.textContent || '',
    message: box.querySelector('.mc-msg')?.textContent || '',
    buttons: [...box.querySelectorAll('button')].filter((b) => !b.classList.contains('mc-close')).map((b) => b.textContent.trim()),
    fields,
    error: box.querySelector('.mc-err:not([hidden])')?.textContent || '',
    window: d.mirror ? d.mirror.info() : d.layerOf ? { layerOf: d.layerOf.name || '' } : null, // a dialog window shows it (mirror.js)
  };
}

// answerDialog fills fields ({id: value}) and presses a button (by its text, case-insensitive). With no button the form
// is submitted (Enter); button "Esc" cancels.
export function answerDialog(button, fields = {}) {
  const d = open[open.length - 1];
  if (!d) throw new Error('no dialog is open');
  for (const [id, v] of Object.entries(fields)) {
    const el = d.box.querySelector(`#${CSS.escape(id)}`) || d.box.querySelector(`[name="${CSS.escape(id)}"]`);
    if (!el) throw new Error(`the dialog has no field "${id}"`);
    if (el.type === 'checkbox' || el.type === 'radio') el.checked = !!v;
    else el.value = v;
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }
  if (button === 'Esc') { d.cancel(); return; }
  if (!button) { d.box.requestSubmit(); return; }
  const b = [...d.box.querySelectorAll('button')].find((x) => x.textContent.trim().toLowerCase() === String(button).toLowerCase());
  if (!b) throw new Error(`the dialog has no button "${button}"`);
  b.click();
}

// messageDialog: a title, a message and buttons; the answer is the label of the button (CancelButton for Esc).
export function messageDialog(o) {
  const buttons = o.Buttons?.length ? o.Buttons : ['OK'];
  const cancel = o.CancelButton || (buttons.includes('Cancel') ? 'Cancel' : buttons.includes('No') ? 'No' : buttons.at(-1));
  return modal((box, done) => {
    box.innerHTML = `<h2>${esc(o.Title)}</h2><p class="mc-msg">${esc(o.Message).replace(/\n/g, '<br>')}</p><div class="mc-btns"></div>`;
    const row = box.querySelector('.mc-btns');
    for (const b of buttons) {
      const el = document.createElement('button');
      el.type = 'button';
      el.textContent = b;
      if (b === (o.DefaultButton || buttons[0])) el.className = 'primary';
      if (o.Danger && b === (o.DefaultButton || buttons[0])) el.classList.add('danger');
      el.onclick = () => done(b);
      row.append(el);
    }
    box.onsubmit = (ev) => { ev.preventDefault(); done(o.DefaultButton || buttons[0]); };
  }, cancel, { kind: o.Kind || 'message' });
}

// inputDialog asks for one line of text. select: [start, end] of the text to select (default: all). The answer is the
// text, or null when cancelled. validate(text) may return an error sentence to keep the dialog open.
export function inputDialog({ title, label = '', value = '', select, okLabel = 'OK', note = '', validate, kind = 'input' }) {
  return modal((box, done) => {
    const sel = select ? select.join(',') : `0,${value.length}`;
    box.innerHTML = `<h2>${esc(title)}</h2>
      <div class="mc-row"><label for="mc-input">${esc(label)}</label><input id="mc-input" type="text" spellcheck="false" value="${esc(value)}" autofocus data-select="${sel}"></div>
      ${note ? `<p class="mc-note">${esc(note)}</p>` : ''}<p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">${esc(okLabel)}</button></div>`;
    box.querySelector('.cancel').onclick = () => done(null);
    box.onsubmit = (ev) => {
      ev.preventDefault();
      const v = box.querySelector('#mc-input').value;
      const err = validate?.(v);
      if (err) { const e = box.querySelector('.mc-err'); e.hidden = false; e.textContent = err; return; }
      done(v);
    };
  }, null, { kind });
}

// formDialog shows html fields; read(box) returns the answer, or a string to show as an error and keep the dialog open.
export function formDialog(title, html, read, setup, { okLabel = 'OK', kind = 'form' } = {}) {
  return modal((box, done) => {
    box.innerHTML = `<h2>${esc(title)}</h2>${html}<p class="mc-err" hidden></p><div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">${esc(okLabel)}</button></div>`;
    box.querySelector('.cancel').onclick = () => done(null);
    box.onsubmit = (ev) => {
      ev.preventDefault();
      const v = read(box);
      if (typeof v === 'string') { const e = box.querySelector('.mc-err'); e.hidden = false; e.textContent = v; return; }
      done(v);
    };
    setup?.(box, done);
  }, null, { kind });
}

// infoDialog shows html with a close button (×, Esc, or a click beside it).
export function infoDialog(html, cls = '', kind = 'info') {
  return modal((box, done) => {
    box.classList.add('mc-info');
    if (cls) box.classList.add(cls);
    box.innerHTML = `<button type="button" class="mc-close" aria-label="Close" title="Close (Esc)">×</button>${html}`;
    box.querySelector('.mc-close').onclick = () => done();
    box.onsubmit = (ev) => { ev.preventDefault(); done(); };
    box.parentElement.addEventListener('mousedown', (ev) => { if (ev.target === ev.currentTarget) done(); });
  }, undefined, { kind });
}
