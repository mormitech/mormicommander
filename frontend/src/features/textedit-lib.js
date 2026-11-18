// The built-in text editor's plain logic (FEATURES.md F-594; the window is features/textedit.js): when F4 uses it,
// the status line, and the questions it asks before closing with unsaved changes or overwriting a file that changed.

// SAVE / DONT_SAVE / CANCEL: the answers of the question before closing with unsaved changes.
export const SAVE = 'Save';
export const DONT_SAVE = "Don't save";
export const CANCEL = 'Cancel';
export const OVERWRITE = 'Overwrite';

const remote = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');

// wantInternal says whether F4 / Shift+F4 open the built-in editor: Options > Viewer/Editor > "Use the built-in editor
// for F4 on text files" is on (it is off by default, so F4 runs the external editor as before) and the file is on
// this computer (not inside an archive or on a server: virtual says the panel shows such a place).
export function wantInternal(values, path, virtual = false) {
  return values?.['ViewerEditor.InternalEditor'] === '1' && !!path && !remote(path) && !virtual;
}

// lineCol gives the 1-based line and column of position pos in text.
export function lineCol(text, pos) {
  const before = String(text || '').slice(0, Math.max(0, pos | 0));
  const nl = before.lastIndexOf('\n');
  return { line: before.split('\n').length, col: before.length - nl };
}

// statusText is the editor's status line: where the cursor is, the line ends and encoding, and whether it changed.
export function statusText({ text, pos, crlf, bom, dirty }) {
  const { line, col } = lineCol(text, pos);
  return `Line ${line}, column ${col} · ${crlf ? 'CRLF' : 'LF'} · UTF-8${bom ? ' with BOM' : ''}${dirty ? ' · changed' : ''}`;
}

// closeQuestion is the messageDialog options for closing with unsaved changes; Esc keeps the editor open.
export function closeQuestion(name) {
  return { Title: 'Editor', Message: `Save the changes to ${name}?`, Buttons: [SAVE, DONT_SAVE, CANCEL], DefaultButton: SAVE, CancelButton: CANCEL, Kind: 'textedit-close' };
}

// changedQuestion is the messageDialog options when the file changed on disk since it was opened; the default keeps
// the file as it is (nothing is overwritten by Enter).
export function changedQuestion(name) {
  return {
    Title: 'Editor',
    Message: `${name} was changed by another program since it was opened.\n\nOverwrite it with your text?`,
    Buttons: [OVERWRITE, CANCEL], DefaultButton: CANCEL, CancelButton: CANCEL, Kind: 'textedit-changed',
  };
}

// isChangedError says whether Go refused a save because the file changed on disk (feature_text_edit.go).
export const isChangedError = (msg) => /changed on disk/i.test(String(msg || ''));
