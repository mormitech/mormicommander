// The pure parts of file tags (features/tags.js, issue #28): the settings with their defaults, Finder's tag colours,
// the marks drawn after a name, the tag list of a selection for the Tags dialog and what the dialog's choices change.
// No DOM here, so frontend/test/tags-lib.test.js tests it alone.
//
// A tag is {name, color}: color is Finder's number (0 none, 1 grey, 2 green, 3 purple, 4 blue, 5 yellow, 6 red,
// 7 orange); Linux tags (user.xdg.tags) have no colour. Names are compared without regard to case, as Finder does.

// DEFAULTS: the marks are off until the user turns them on (Show > File tags), so the panels look as before. names:
// the tag names beside the coloured dots, not only in the tooltip.
export const DEFAULTS = { enabled: false, names: false };

export function tagsConfig(saved) {
  const s = saved && typeof saved === 'object' ? saved : {};
  const c = { ...DEFAULTS };
  if (typeof s.enabled === 'boolean') c.enabled = s.enabled;
  if (typeof s.names === 'boolean') c.names = s.names;
  return c;
}

// COLORS: Finder's tag colours by number, with their names (index 0: no colour).
export const COLORS = [
  ['None', ''], ['Gray', '#8e8e93'], ['Green', '#34c759'], ['Purple', '#af52de'],
  ['Blue', '#0a84ff'], ['Yellow', '#ffcc00'], ['Red', '#ff3b30'], ['Orange', '#ff9500'],
];

// colorOf is a tag's CSS colour ('' for none).
export const colorOf = (t) => (t && t.color > 0 && t.color < 8 ? COLORS[t.color][1] : '');

// defaultColor: Finder's colour tags ("Red", "blue" …) keep their colour; other names get none.
export function defaultColor(name) {
  const i = COLORS.findIndex(([n], k) => k > 0 && n.toLowerCase() === String(name || '').trim().toLowerCase());
  return i > 0 ? i : 0;
}

const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

// tagText lists the names: "Red, Work".
export const tagText = (list) => (list || []).map((t) => t.name).join(', ');

// marksHTML is what goes after a name: a dot per tag (hollow for a tag without colour; at most 4, Finder shows
// 3), and with names on the names too. '' for no tags.
export function marksHTML(list, cfg = DEFAULTS) {
  if (!list?.length) return '';
  const dots = list.slice(0, 4).map((t) => {
    const c = colorOf(t);
    return c ? `<i class="mc-tags-dot" style="background:${c}"></i>` : '<i class="mc-tags-dot none"></i>';
  }).join('');
  const names = cfg.names ? `<span class="mc-tags-names">${esc(tagText(list))}</span>` : '';
  return `${dots}${names}`;
}

export const hasTag = (list, name) => (list || []).some((t) => t.name.toLowerCase() === String(name).toLowerCase());

// summary is the tags of a selection for the Tags dialog: [{name, color, count}] where count is how many of the n
// items have it: the selection's tags in the order met, then extra (tags seen elsewhere in the folder) and, when
// colours is true, Finder's colour tags that are not there yet (so one click adds one), both with count 0.
export function summary(lists, { colours = false, extra = [] } = {}) {
  const out = [];
  const find = (name) => out.find((t) => t.name.toLowerCase() === name.toLowerCase());
  const add = (t, n) => {
    const have = find(t.name);
    if (have) { have.count += n; if (!have.color && t.color) have.color = t.color; return; }
    out.push({ name: t.name, color: t.color || 0, count: n });
  };
  for (const l of lists) for (const t of l || []) add(t, 1);
  for (const t of extra) add(t, 0);
  if (colours) for (const i of [6, 7, 5, 2, 4, 3, 1]) add({ name: COLORS[i][0], color: i }, 0);
  return out;
}

// parseNames splits typed text into tag names (comma-separated; empty ones dropped, duplicates once).
export function parseNames(text) {
  const out = [];
  for (const s of String(text || '').split(/[,\n]/)) {
    const n = s.trim();
    if (n && !out.some((o) => o.toLowerCase() === n.toLowerCase())) out.push(n);
  }
  return out;
}

// changes turns the dialog's choices into what Go is asked to do: rows [{name, color, count}] with state true (all
// get it), false (none has it) or 'mixed' (left as it is), n the number of items, typed the new names and color the
// colour chosen for them. Returns {add: [{name, color}], remove: [names]}.
export function changes(rows, states, n, typed = [], color = 0) {
  const add = [], remove = [];
  rows.forEach((r, i) => {
    const s = states[i];
    if (s === true && r.count < n) add.push({ name: r.name, color: r.color || 0 });
    if (s === false && r.count > 0) remove.push(r.name);
  });
  for (const name of typed) {
    if (add.some((a) => a.name.toLowerCase() === name.toLowerCase())) continue;
    const row = rows.find((r) => r.name.toLowerCase() === name.toLowerCase());
    add.push({ name, color: color || row?.color || defaultColor(name) });
    const k = remove.findIndex((x) => x.toLowerCase() === name.toLowerCase());
    if (k >= 0) remove.splice(k, 1);
  }
  return { add, remove };
}

// folderTags is every tag found in a Lookup answer ({path: [tags]}), each once, sorted by name.
export function folderTags(map) {
  const out = [];
  for (const l of Object.values(map || {})) {
    for (const t of l || []) if (!hasTag(out, t.name)) out.push({ name: t.name, color: t.color || 0 });
  }
  return out.sort((a, b) => a.name.localeCompare(b.name));
}
