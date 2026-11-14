// The right-click menu of the quick view panel (Ctrl+Q, F-082 / F-589). The Lister window has a menu bar (File, Edit,
// Options, Image, Encoding); the quick view panel has no room for one, so until now its modes and commands were reached
// only by keys after Tab moved into the view. A right-click in the panel now opens one flat list made of the same
// Lister menu items: the view modes first (1-5, so HTML/Markdown rendered is a click away), then copying and searching
// (Find with case, whole words, hex, regex and backwards), the image commands when an image or video is shown, and the
// text options last. Items that only make sense in the Lister window (next/previous file, close, save) are left out.
// Pure: it only rearranges item lists, so it is unit-tested without a page.

// Labels of Lister items the quick view menu leaves out: the panel follows the cursor and closes with Ctrl+Q.
const SKIP = new Set(['Next file', 'Previous file', 'Close', 'Save as…', 'Save selection as…', 'Open with another app']);

const plain = (label) => String(label || '').replace(/^✓ /, '');

// tidy drops separators at the ends and separators that follow each other.
export function tidy(items) {
  const out = [];
  for (const it of items) {
    if (it.sep && (!out.length || out[out.length - 1].sep)) continue;
    out.push(it);
  }
  while (out.length && out[out.length - 1].sep) out.pop();
  return out;
}

// quickMenuItems builds the quick view menu from the Lister menus: menus is {File, Edit, Options, Image} as returned
// by ListerView.menuItems, media says an image or video is shown, encoding is an extra item that opens the encoding
// list (or null).
export function quickMenuItems(menus, { media = false, encoding = null } = {}) {
  const keep = (list) => (list || []).filter((it) => it.sep || !SKIP.has(plain(it.label)));
  const options = keep(menus.Options);
  const cut = options.findIndex((it) => it.sep); // the modes are the Options menu's first group
  const modes = cut < 0 ? options : options.slice(0, cut);
  const rest = cut < 0 ? [] : options.slice(cut + 1);
  const items = [...modes, { sep: true }, ...keep(menus.Edit), { sep: true }];
  if (media) items.push(...keep(menus.Image).filter((it) => !it.disabled), { sep: true });
  if (encoding) items.push(encoding);
  items.push(...rest, { sep: true }, ...keep(menus.File).filter((it) => /^(✓ )?(Reload|Follow file)/.test(it.label || '')));
  return tidy(items);
}
