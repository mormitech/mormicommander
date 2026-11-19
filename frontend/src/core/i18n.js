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
let patterns = null; // [regex, translation] of the texts with Go verbs (%s, %v, %d, %q…), made when first needed

export function setTranslations(map) {
  const entries = map ? Object.entries(map).filter(([k, v]) => k && typeof v === 'string' && v !== '') : [];
  table = entries.length ? new Map(entries) : null;
  patterns = null;
}

// Msg is a message with both its English and its translated text (tm below): app.flash shows the translation and keeps
// the English for control mode; anywhere else it reads as the translated text.
export class Msg {
  constructor(en, text) { this.en = en; this.text = text; }
  toString() { return this.text; }
}

// fill puts vars into the {name} placeholders of s (a placeholder without a value stays as it is). A Msg value gives
// its English text (en) or its translation.
function fill(s, vars, en = false) {
  return s.replace(/\{(\w+)\}/g, (m, k) => {
    if (!Object.hasOwn(vars, k)) return m;
    const v = vars[k];
    return v instanceof Msg ? (en ? v.en : v.text) : String(v);
  });
}

// t is the translation of an English text (the text itself when there is none). A trailing "…" is kept, so "Pack" also
// translates "Pack…".
export function t(s, vars) {
  if (s instanceof Msg) return s.text;
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

// tm is t() as a Msg: the English text with its placeholders filled in (for control mode and the program's logic) and
// the translated one (shown). Without a language file both are the same English text.
export function tm(s, vars = {}) {
  return new Msg(fill(s, vars, true), t(s, vars));
}

// tn is "3 files" as a Msg: the number and the English word, and the number and the translated word (Hungarian keeps
// the singular after a number: "files=fájl"). Used as a value of tm()'s placeholders.
const group = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }); // (as plural() in core/format.js)
export function tn(n, word, many = `${word}s`, num = group.format(n)) {
  const w = n === 1 ? word : many;
  return new Msg(`${num} ${w}`, `${num} ${t(w)}`);
}

// tjoin joins texts and Msgs into one Msg (lines of a message, parts of a status line).
export const tjoin = (parts, sep = '\n') => new Msg(parts.map((x) => (x instanceof Msg ? x.en : String(x))).join(sep), parts.map(String).join(sep));

// te is a text that may come from Go (an error) as a Msg: the English text and its translation (tText below), for use
// as a value of tm()'s placeholders.
export const te = (s) => new Msg(String(s ?? ''), tText(String(s ?? '')));

// tText translates a whole message shown to the user that may come from Go (an error): the language file's text for it,
// else the most specific text with Go verbs (%s, %v, %d, %q…, as written in the Go source; frontend/test/error-texts.json lists
// them) that matches it, with the parts the verbs stand for put into the translation's verbs (in order, or by %[n]s).
// The program itself keeps the English text; without a language file the text comes back unchanged.
export function tText(s) {
  if (s instanceof Msg) return s.text;
  if (typeof s !== 'string' || !s || !table) return t(s);
  const hit = t(s);
  if (hit !== s) return hit;
  if (!patterns) {
    patterns = [];
    for (const [en, x] of table) {
      if (!/%[-+# 0-9.]*[a-zA-Z]/.test(en.replace(/%%/g, ''))) continue;
      const parts = en.split(/(%%|%[-+# 0-9.]*[a-zA-Z])/);
      const re = parts.map((p, i) => (i % 2 ? (p === '%%' ? '%' : '([\\s\\S]*?)') : p.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))).join('');
      patterns.push([new RegExp(`^${re}$`), x, parts.filter((p, i) => i % 2 === 0).join('').length]);
    }
    patterns.sort((a, b) => b[2] - a[2]); // (the most specific text first: "Can't %s %s: it is not there." before "Can't %s %s: %v")
  }
  for (const [re, x] of patterns) {
    const m = re.exec(s);
    if (!m) continue;
    let i = 0;
    return x.replace(/%%|%(?:\[(\d+)\])?[-+# 0-9.]*[a-zA-Z]/g, (v, n) => (v === '%%' ? '%' : (m[n ? +n : ++i] ?? '')));
  }
  return s;
}

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
    s = s.replace(/\s+/g, ' ').trim().replace(/&gt;/g, '>').replace(/&lt;/g, '<').replace(/&quot;/g, '"').replace(/&amp;/g, '&').replace(/\\'/g, "'");
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

// literalTexts finds the texts of t('…') / tr('…') / tt('…') / tm('…') calls with a plain string (no ${} inside) in source code.
export function literalTexts(src) {
  const out = [];
  const re = /\b(?:t|tr|tt|tm)\(\s*(?:'((?:[^'\\\n]|\\.)*)'|"((?:[^"\\\n]|\\.)*)"|`((?:[^`\\$]|\\.)*)`)/g;
  for (const m of src.matchAll(re)) {
    const s = (m[1] ?? m[2] ?? m[3]).replace(/\\(.)/g, (x, c) => (c === 'n' ? '\n' : c));
    if (s) out.push(s);
  }
  return out;
}
