// Translations of the interface's words (Options > Language, board issue #94). A language file holds lines of
// "English text=Translated text", optionally in sections named by language ("[hu]"); the section of Options > Language
// > Interface language is used, and lines outside any section always. Without a file everything stays English. The
// menu bar, the function key buttons, the popup lists and the dialogs' titles and buttons go through tr(); what a
// dialog answers stays the English word, so the program's own logic never sees a translation.

let table = null; // Map English → translation, or null

export function setTranslations(map) {
  table = map && Object.keys(map).length ? new Map(Object.entries(map)) : null;
}

// tr is the translation of an English text (the text itself when there is none). A trailing "…" is kept.
export function tr(s) {
  if (!table || typeof s !== 'string' || !s) return s;
  const hit = table.get(s);
  if (hit !== undefined) return hit;
  if (s.endsWith('…')) { const h = table.get(s.slice(0, -1)); if (h !== undefined) return `${h}…`; }
  return s;
}

export const translating = () => !!table;
