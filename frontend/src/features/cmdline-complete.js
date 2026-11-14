// Tab completion of file and folder names in the command line: Tab completes the word at the caret from the folder it
// names (the active panel's folder for a plain name); with more than one match, each further Tab goes on to the next
// (Shift+Tab back). A folder gets a separator after it, so the next Tab looks inside. With no match Tab does what it
// did before: the keyboard goes back to the panel (core/keyboard.js).

import { setCmdlineCompleter } from '../core/cmdline.js';
import { app as theApp } from '../core/app.js';
import { go } from '../core/go.js';
import { toEntry } from '../core/panel.js';
import { wordAt, splitWord, matchEntries, writeMatch } from './cmdline-complete-lib.js';

let last = null; // {value, caret, start, dir, quoted, matches, i}: the completion Tab made, for cycling

function put(input, value, start, word, last0) {
  input.value = value.slice(0, start) + word + value.slice(last0);
  const c = start + word.length;
  input.setSelectionRange(c, c);
  return c;
}

async function complete(input, back) {
  const app = theApp, p = app.cur;
  const windows = app.platform === 'windows';
  const value = input.value, caret = input.selectionStart ?? value.length;
  // a further Tab right after a completion with several matches: the next (or previous) one
  if (last && last.value === value && last.caret === caret && last.matches.length > 1) {
    last.i = (last.i + (back ? -1 : 1) + last.matches.length) % last.matches.length;
    const word = writeMatch(last.dir, last.matches[last.i], { sep: last.sep, quoted: last.quoted, windows });
    last.caret = put(input, value, last.start, word, caret);
    last.value = input.value;
    return true;
  }
  last = null;
  const w = wordAt(value, caret);
  const { dir, name } = splitWord(w.word, windows);
  if (!p?.path) return false;
  let folder = dir === '' ? p.path : dir.replace(/^~(?=[/\\]|$)/, app.home);
  folder = app.resolvePath(folder, p.path);
  let listing;
  try { listing = await go.FS.List('cmdline-complete', folder); } catch { return false; }
  if (input.value !== value) return true; // typed on meanwhile: leave it
  const entries = (listing.entries || []).map(toEntry);
  const matches = matchEntries(entries, name, { caseless: app.platform !== 'linux', showHidden: !!app.showHidden });
  if (!matches.length) return false;
  const sep = app.sepFor(listing.path || folder);
  const word = writeMatch(dir, matches[0], { sep, quoted: w.quoted, windows });
  const c = put(input, value, w.start, word, caret);
  if (matches.length > 1) last = { value: input.value, caret: c, start: w.start, dir, sep, quoted: w.quoted, matches, i: 0 };
  return true;
}

setCmdlineCompleter(complete);
