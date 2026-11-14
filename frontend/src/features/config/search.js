// Word matching for the search box of the Options dialog (F-827). The dialog matches page titles here and asks Go
// (Configuration.SearchWords) for the options; both follow the same rule. By default a search word may be any part
// of a word, as it always was ("two" also finds "network"); with "Whole words" ticked (board issue #109) it must
// stand on its own: no letter or digit right before or after it. This mirrors config.ContainsWord on the Go side.

const WORD_CHAR = /[\p{L}\p{N}]/u;

// isWordChar tells whether ch (one character, or undefined at the start / end of the text) is part of a word.
const isWordChar = (ch) => ch !== undefined && WORD_CHAR.test(ch);

// containsWord reports whether hay contains word; with whole, only as a word of its own. Every place it occurs is
// tried, so "network and two" still has the whole word "two".
export function containsWord(hay, word, whole = false) {
  if (!whole || !word) return hay.includes(word);
  for (let from = 0; ;) {
    const i = hay.indexOf(word, from);
    if (i < 0) return false;
    const before = Array.from(hay.slice(0, i)).at(-1); // (whole characters, so letters outside the BMP count too)
    const after = Array.from(hay.slice(i + word.length, i + word.length + 2))[0];
    if (!isWordChar(before) && !isWordChar(after)) return true;
    from = i + 1;
  }
}

// searchWords splits a query into lower-case words (empty for a blank query).
export const searchWords = (q) => String(q || '').toLowerCase().split(/\s+/).filter(Boolean);

// matchesAll reports whether text contains every one of words (already lower case), case-insensitively.
export function matchesAll(text, words, whole = false) {
  const hay = String(text || '').toLowerCase();
  return words.every((w) => containsWord(hay, w, whole));
}
