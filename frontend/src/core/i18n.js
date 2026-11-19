// Translations of the interface's words (Options > Language, board issues #94 and #113). The language file (read by Go,
// feature_lang.go: .lng lines "English text=Translated text" or a .json object, optionally in sections named by
// language) gives a table English → translation; t() looks a text up in it. Without a file, and for every text the
// file lacks, t() gives back the English text unchanged, so the default interface is exactly the English one.
// What a dialog answers stays the English word (the buttons only show a translation), so the program's own logic never
// sees one.
//
// t('Copy {what} to:', { what }) fills in {name} placeholders after the lookup; the translation may move them around.
// The template of all the texts (lang/template.lng) is made by `npm run lang:template` in frontend/, which collects
// what t() is asked while the menus are built (collect mode, below) plus the literal t('…') texts in the source.

let table = null; // Map English → translation, or null
let collected = null; // Set of the texts asked while collecting, or null

export function setTranslations(map) {
  const entries = map ? Object.entries(map).filter(([k, v]) => k && typeof v === 'string' && v !== '') : [];
  table = entries.length ? new Map(entries) : null;
}

// fill puts vars into the {name} placeholders of s (a placeholder without a value stays as it is).
function fill(s, vars) {
  return s.replace(/\{(\w+)\}/g, (m, k) => (Object.hasOwn(vars, k) ? String(vars[k]) : m));
}

// t is the translation of an English text (the text itself when there is none). A trailing "…" is kept, so "Pack" also
// translates "Pack…".
export function t(s, vars) {
  if (typeof s !== 'string' || !s) return s;
  collected?.add(s);
  let out = s;
  if (table) {
    const hit = table.get(s);
    if (hit !== undefined) out = hit;
    else if (s.endsWith('…')) { const h = table.get(s.slice(0, -1)); if (h !== undefined) out = `${h}…`; }
  }
  return vars ? fill(out, vars) : out;
}

export const tr = t; // (the older name, used by the menus, popups and dialogs)

export const translating = () => !!table;

// translateTree translates a freshly built piece of a dialog in place (#113): every text node and title / placeholder /
// aria-label whose whole text (without the spaces around it) is a text of the language file. It is called on the
// dialog's own words right after they are built, before names or other data are filled in, so it changes nothing but
// those words; without a language file it does nothing at all. An element marked data-notr is left alone with its
// contents.
const TR_ATTRS = ['title', 'placeholder', 'aria-label'];
export function translateTree(root) {
  if ((!table && !collected) || !root?.ownerDocument) return root; // (collect mode reads the words for the template)
  const doc = root.ownerDocument;
  const skip = (n) => !!n.parentElement?.closest('[data-notr], script, style, textarea');
  const walker = doc.createTreeWalker(root, 4 /* NodeFilter.SHOW_TEXT */);
  for (let n = walker.nextNode(); n; n = walker.nextNode()) {
    const s = n.nodeValue, core = s.trim();
    if (!core || !/[A-Za-z]/.test(core) || skip(n)) continue;
    const x = t(core);
    if (x !== core) n.nodeValue = s.replace(core, () => x);
  }
  for (const el of [root, ...root.querySelectorAll(TR_ATTRS.map((a) => `[${a}]`).join(','))]) {
    if (!el.getAttribute || el.closest?.('[data-notr]')) continue;
    for (const a of TR_ATTRS) {
      const v = el.getAttribute(a)?.trim();
      if (!v) continue;
      const x = t(v);
      if (x !== v) el.setAttribute(a, x);
    }
  }
  return root;
}

// htmlTexts finds the words of HTML templates in source code: the text between tags and the title / placeholder /
// aria-label values that have no ${} in them (for the template and the test of the files translated with translateTree).
export function htmlTexts(src) {
  const out = [];
  const keep = (s) => {
    s = s.replace(/\s+/g, ' ').trim().replace(/&gt;/g, '>').replace(/&lt;/g, '<').replace(/&quot;/g, '"').replace(/&amp;/g, '&');
    if (/[A-Za-z]{2}/.test(s) && !/[${}=;|]|&&/.test(s)) out.push(s);
  };
  for (const m of src.matchAll(/>([^<>`]+)</g)) keep(m[1]);
  for (const m of src.matchAll(/\b(?:title|placeholder|aria-label)="([^"$`]+)"/g)) keep(m[1]);
  return out;
}

// Collect mode, for the template: every text t() is asked from now on is kept until stopCollecting() returns them.
export function startCollecting() { collected = new Set(); }
export function stopCollecting() { const s = collected ? [...collected] : []; collected = null; return s; }

// lngLine is one "English=Translation" line of a .lng file ("=" in the English text written "\=").
export const lngLine = (en, tr2 = '') => `${en.replace(/=/g, '\\=')}=${tr2}`;

// literalTexts finds the texts of t('…') / tr('…') / tt('…') calls with a plain string (no ${} inside) in source code.
export function literalTexts(src) {
  const out = [];
  const re = /\b(?:t|tr|tt)\(\s*(?:'((?:[^'\\\n]|\\.)*)'|"((?:[^"\\\n]|\\.)*)"|`((?:[^`\\$]|\\.)*)`)/g;
  for (const m of src.matchAll(re)) {
    const s = (m[1] ?? m[2] ?? m[3]).replace(/\\(.)/g, (x, c) => (c === 'n' ? '\n' : c));
    if (s) out.push(s);
  }
  return out;
}
