// The simple built-in text editor (FEATURES.md F-594, our addition: TC uses Notepad). A window over the panels with
// the text, a status line and Save / Close. Go reads and writes the file (feature_text_edit.go): local UTF-8 text files
// only, line ends and byte order mark kept, and a save never overwrites changes another program made without asking.
//
//   cm_EditInternal   Files > Edit with the built-in editor: the file under the cursor (registered in viewer.js,
//                     after Shift+F4, so it sits after it in the menu)
//   F4 / Shift+F4     use it too when Options > Viewer/Editor > "Use the built-in editor for F4 on text files" is on
//                     (off by default; a file it can't take, binary or too large, then goes to the external editor)
//
// Keys in the editor: Ctrl+S (Cmd+S on macOS) saves, Esc closes (asking first when there are unsaved changes).

import './textedit.css';
import { go, errText } from '../core/go.js';
import { modal, esc, messageDialog } from '../core/dialogs.js';
import { lineCol, closeQuestion, changedQuestion, isChangedError, SAVE, DONT_SAVE, OVERWRITE } from './textedit-lib.js';
import { translateTree, tm, tText } from '../core/i18n.js';

// openTextEditor shows the file path in the built-in editor and resolves when it is closed: {shown: true}, or
// {shown: false, why} when the file can't be edited there (Go's reason), so F4 can hand it to the external editor.
export async function openTextEditor(app, path) {
  let f;
  try {
    f = await go.ListerEdit.ReadText(path);
  } catch (e) {
    return { shown: false, why: errText(e) };
  }
  let saved = f.text;
  let onKey = () => false;
  let onClose = () => {};
  await modal((box, done) => {
    box.classList.add('mc-textedit');
    box.innerHTML = `<h2 data-notr>${esc(f.name)}</h2>
      <textarea id="te-text" class="mc-textedit-text" spellcheck="false" autocomplete="off" wrap="off" autofocus></textarea>
      <div class="mc-textedit-bar"><span class="mc-textedit-status"></span><span class="mc-err" hidden></span></div>
      <div class="mc-btns"><button type="button" class="te-close">Close</button><button type="button" class="primary te-save">Save</button></div>`;
    translateTree(box);
    const ta = box.querySelector('#te-text');
    const status = box.querySelector('.mc-textedit-status');
    const err = box.querySelector('.mc-err');
    ta.value = f.text;
    const dirty = () => ta.value !== saved;
    // (statusText of textedit-lib.js, with its words translated, #113)
    const show = () => {
      const { line, col } = lineCol(ta.value, ta.selectionStart);
      status.textContent = [tm('Line {line}, column {col}', { line, col }), f.crlf ? 'CRLF' : 'LF', f.bom ? tm('UTF-8 with BOM') : 'UTF-8', ...(dirty() ? [tm('changed')] : [])].join(' · ');
    };
    for (const ev of ['input', 'keyup', 'click', 'select']) ta.addEventListener(ev, show);
    setTimeout(() => { ta.focus(); ta.setSelectionRange(0, 0); ta.scrollTop = 0; show(); }, 0);
    let busy = false;
    const save = async () => {
      if (busy) return false;
      busy = true;
      err.hidden = true;
      const text = ta.value;
      try {
        let r;
        try {
          r = await go.ListerEdit.WriteText({ ...f, text: '' }, text, false);
        } catch (e) {
          if (!isChangedError(errText(e))) throw e;
          if ((await messageDialog({ ...changedQuestion(f.name), Message: tm('{name} was changed by another program since it was opened.\n\nOverwrite it with your text?', { name: f.name }) })) !== OVERWRITE) return false;
          r = await go.ListerEdit.WriteText({ ...f, text: '' }, text, true);
        }
        f = { ...f, mtime: r.mtime, size: r.size };
        saved = text;
        app.flash?.(`Saved ${f.name}.`);
        return true;
      } catch (e) {
        err.textContent = tText(errText(e));
        err.hidden = false;
        return false;
      } finally {
        busy = false;
        show();
        ta.focus();
      }
    };
    const close = async () => {
      if (!dirty()) { done(true); return; }
      const a = await messageDialog({ ...closeQuestion(f.name), Message: tm('Save the changes to {name}?', { name: f.name }) });
      if (a === DONT_SAVE) done(true);
      else if (a === SAVE && await save()) done(true);
      else ta.focus();
    };
    box.querySelector('.te-save').onclick = () => save();
    box.querySelector('.te-close').onclick = () => close();
    box.onsubmit = (ev) => ev.preventDefault(); // (Enter is a new line, not a button)
    onClose = close;
    onKey = (ev) => {
      const mod = ev.ctrlKey || ev.metaKey;
      if (mod && !ev.altKey && !ev.shiftKey && ev.key.toLowerCase() === 's') { save(); return true; }
      if (ev.key === 'Escape') { close(); return true; }
      return false;
    };
  }, true, { kind: 'textedit', onKey: (ev) => onKey(ev), onClose: () => onClose() });
  // the panels show the new size and time
  for (const p of app.panels || []) {
    const dir = path.slice(0, Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')));
    if (p.path === dir) p.reload?.();
  }
  return { shown: true };
}
