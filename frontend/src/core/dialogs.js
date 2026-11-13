// Dialogs drawn by the page (never alert()). One modal() helper: a box over the page, Esc = cancel, keys don't reach the
// panels while it is open, focus comes back afterwards. Every open dialog is listed, so control mode can read it
// ("dialog") and answer it ("dialog_answer") by pressing its buttons and filling its fields, the same as a person.
//
//   await messageDialog({Title, Message, Buttons: ['Overwrite', 'Skip', 'Cancel'], DefaultButton, CancelButton}) → label
//   await inputDialog({title, label, value, select: [0, 4], okLabel}) → text or null
//   await formDialog(title, html, read, setup) → what read(box) returns, or null
//   infoDialog(html) → a box with a close button

import './dialogs.css';
import { waitForUser } from './busy.js';

const esc = (s) => String(s ?? '').replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
export { esc };

const open = []; // the dialogs on screen, last = top

// modal shows a box; build fills it and gets done(value) to close it with an answer.
export function modal(build, cancelValue, meta = {}) {
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
      back.remove();
      open.splice(open.indexOf(entry), 1);
      window.removeEventListener('keydown', key, true);
      prevFocus?.focus?.();
      resolve(v);
    };
    entry.done = done;
    entry.cancel = () => done(cancelValue);
    const key = (ev) => {
      if (open[open.length - 1] !== entry) return;
      if (ev.key === 'Escape') { ev.preventDefault(); ev.stopImmediatePropagation(); done(cancelValue); return; }
      ev.stopImmediatePropagation(); // the panels' keys are not for it while a dialog is open
    };
    window.addEventListener('keydown', key, true);
    build(box, done);
    open.push(entry);
    document.body.append(back);
    const first = box.querySelector('[autofocus]') || box.querySelector('.primary') || box.querySelector('button');
    first?.focus();
    if (first?.select && first.dataset.select) { const [a, b] = first.dataset.select.split(',').map(Number); first.setSelectionRange(a, b); }
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
