// Folder completion in path fields (F-782): the path bar while it is edited, the copy and move dialog's target and
// Find files' "Search in". While a name is typed, the first folder of that place whose name starts with it is
// filled in after the caret, selected, so typing on replaces it; Tab or Right takes it (Right does so by itself, as
// it moves the caret to the end), Backspace removes it. Folders only; hidden ones only while the panels show them.
// Paths on servers and inside archives are completed too, from what the panel's file system lists.

import { go } from './go.js';
import { app } from './app.js';

const cache = new Map(); // folder → {at, names}
const TTL = 5000;
let seq = 0;

async function folderNames(dir) {
  const c = cache.get(dir);
  if (c && Date.now() - c.at < TTL) return c.names;
  const l = await go.FS.List(`complete-${++seq % 4}`, dir);
  const names = (l.entries || []).filter((r) => (r[3] & 1) && (app.showHidden || !(r[3] & 4))).map((r) => r[0]).sort((a, b) => a.localeCompare(b));
  cache.set(dir, { at: Date.now(), names });
  if (cache.size > 50) cache.delete(cache.keys().next().value);
  return names;
}

// split cuts typed text into the folder to list and the start of the name typed in it ('' folder: nothing to do).
function split(text, base) {
  const i = Math.max(text.lastIndexOf('/'), text.lastIndexOf('\\'));
  if (i < 0) return base ? { dir: base, prefix: text, head: '' } : { dir: '', prefix: '', head: '' };
  const head = text.slice(0, i + 1);
  let dir = head;
  if (/^~[\\/]/.test(dir)) dir = app.home + dir.slice(1);
  else if (!/^([a-z][a-z0-9+.-]*:\/\/|[\\/]|[a-z]:[\\/])/i.test(dir) && base) dir = app.resolvePath(dir, base);
  if (dir.length > 1 && !/^[a-z]:[\\/]$/i.test(dir) && !/:\/\/[^/]*\/$/.test(dir)) dir = dir.replace(/[\\/]+$/, '');
  return { dir, prefix: text.slice(i + 1), head };
}

// attachPathComplete makes input complete folder names. base() is the folder a relative path starts from ('' for
// none). opts.word(text), when given, picks the part of the text that is the path (the command line's last word,
// F-296), or null for no suggestion; without it the whole text is the path. Returns a function that takes the
// completion off again.
export function attachPathComplete(input, base = () => '', opts = {}) {
  let busy = 0;
  const win = () => app.platform === 'windows';
  const onInput = async (ev) => {
    if (ev.inputType && !ev.inputType.startsWith('insert')) return; // (deleting: no suggestion, so Backspace works)
    const caret = input.selectionStart;
    if (caret !== input.value.length || input.selectionEnd !== caret) return;
    const typed = input.value;
    const part = opts.word ? opts.word(typed) : typed;
    if (!part) return;
    const { dir, prefix } = split(part, base());
    if (!dir || !prefix) return;
    const my = ++busy;
    let names;
    try { names = await folderNames(dir); } catch { return; }
    if (my !== busy || input.value !== typed || document.activeElement !== input) return;
    const low = (s) => (win() || app.platform === 'darwin' ? s.toLowerCase() : s);
    const hit = names.find((n) => low(n).startsWith(low(prefix)) && n.length > prefix.length);
    if (!hit) return;
    input.value = typed + hit.slice(prefix.length);
    input.setSelectionRange(typed.length, input.value.length);
    input.dataset.mcCompleted = '1';
  };
  const onKey = (ev) => {
    if (ev.key === 'Tab' && !ev.shiftKey && input.dataset.mcCompleted && input.selectionEnd === input.value.length && input.selectionStart < input.selectionEnd) {
      ev.preventDefault();
      ev.stopPropagation();
      input.setSelectionRange(input.value.length, input.value.length);
      delete input.dataset.mcCompleted;
    } else if (!['Shift', 'Control', 'Alt', 'Meta'].includes(ev.key)) delete input.dataset.mcCompleted;
  };
  input.addEventListener('input', onInput);
  input.addEventListener('keydown', onKey, true);
  return () => { input.removeEventListener('input', onInput); input.removeEventListener('keydown', onKey, true); };
}
