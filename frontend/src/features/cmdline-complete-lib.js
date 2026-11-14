// The plain rules of Tab completion in the command line (features/cmdline-complete.js), kept apart so vitest can test
// them: which word the caret is on, which folder and name start it names, which entries match, and how a match is
// written back (quoted when it needs to be, a separator after a folder).

// wordAt finds the word that ends at the caret: it starts after the last space outside quotes. quoted: the word
// started with a quote. raw: the word as typed; word: without its quotes.
export function wordAt(text, caret) {
  const before = text.slice(0, caret);
  let start = 0, inQ = false;
  for (let i = 0; i < before.length; i++) {
    const c = before[i];
    if (c === '"') inQ = !inQ;
    else if (c === ' ' && !inQ) start = i + 1;
  }
  const raw = before.slice(start);
  return { start, end: caret, raw, quoted: raw.startsWith('"'), word: raw.replace(/"/g, '') };
}

// splitWord splits a word into its folder part (up to and with the last separator) and the name start after it.
export function splitWord(word, windows = false) {
  const i = Math.max(word.lastIndexOf('/'), windows ? word.lastIndexOf('\\') : -1);
  return i < 0 ? { dir: '', name: word } : { dir: word.slice(0, i + 1), name: word.slice(i + 1) };
}

// matchEntries gives the entries whose names start with name, folders first, each group by name. Hidden entries
// count when the panels show them or the name start is a dot; ".." never does.
export function matchEntries(entries, name, { caseless = true, showHidden = false } = {}) {
  const norm = (s) => (caseless ? s.toLowerCase() : s);
  const want = norm(name);
  return entries
    .filter((e) => e.name !== '.' && e.name !== '..' && norm(e.name).startsWith(want) && (showHidden || name.startsWith('.') || !e.hidden))
    .sort((a, b) => (a.dir === b.dir ? a.name.localeCompare(b.name, undefined, { sensitivity: 'base', numeric: true }) : a.dir ? -1 : 1));
}

// needsQuotes: the path has a space or a character the shell would take for itself.
export const needsQuotes = (s, windows = false) => (windows ? /[\s&()^,;=!]/ : /[\s"'&|;<>()$`\\*?[\]{}]/).test(s);

// writeMatch is the word that takes the place of the typed one: the folder part, the name, a separator after a folder,
// in quotes when the path needs them (or the typed word had them).
export function writeMatch(dir, entry, { sep = '/', quoted = false, windows = false } = {}) {
  const path = dir + entry.name + (entry.dir ? sep : '');
  return quoted || needsQuotes(path, windows) ? `"${path.replace(/"/g, '')}"` : path;
}
