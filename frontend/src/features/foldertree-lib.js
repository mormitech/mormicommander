// The pure parts of the expanding folder tree (features/foldertree.js) that the Alt+F10 popup (cm_CDtree with
// Tree.CDTreeExpand, F-171) and the separate tree pane (Layout.SeparateTreePane, F-025) share: the nodes, which of
// them show (the open ones, or with a typed filter the matching ones and the folders above them), and the way from the
// root down to a folder. Nothing here reads the disk; the widget asks for one folder's subfolders at a time, when the
// user opens it (lazy loading), so even a big drive opens at once.

// pathParts is a path and the folders above it, top first ("/a/b" → "/", "/a", "/a/b"), the way the path bar cuts it
// (core/panel.js splitPath; kept here so the tree does not load the panel code).
export function pathParts(p) {
  p = String(p || '');
  const m = /^([a-z][a-z0-9+.-]*:\/\/[^/]*\/?)(.*)$/i.exec(p);
  let head = '', rest = p, sep = '/';
  if (m) { head = m[1].endsWith('/') ? m[1] : `${m[1]}/`; rest = m[2]; }
  else if (/^[a-z]:\\/i.test(p) || (p.includes('\\') && !p.includes('/'))) { sep = '\\'; const i = p.indexOf('\\'); head = p.slice(0, i + 1); rest = p.slice(i + 1); }
  else if (p.startsWith('/')) { head = '/'; rest = p.slice(1); }
  const out = head ? [head] : [];
  let acc = head;
  for (const n of rest.split(sep).filter(Boolean)) {
    acc = acc && !acc.endsWith(sep) && !acc.endsWith('/') ? acc + sep + n : acc + n;
    out.push(acc);
  }
  return out;
}

// makeNode is a folder of the tree: kids null means not read yet, [] means read and empty.
export function makeNode(name, path, parent = null) {
  return { name, path, depth: parent ? parent.depth + 1 : 0, parent, kids: null, open: false, loading: false };
}

// setKids puts the subfolders read for a node ([{name, path}]) under it.
export function setKids(n, list) {
  n.kids = (Array.isArray(list) ? list : []).map((k) => makeNode(k.name, k.path, n));
  return n.kids;
}

// filterWords is the typed filter as lower-case words.
const filterWords = (q) => String(q || '').toLowerCase().split(/\s+/).filter(Boolean);

// nameMatches says whether a folder name has every typed word.
export function nameMatches(name, query) {
  const words = filterWords(query);
  const n = String(name || '').toLowerCase();
  return words.every((w) => n.includes(w));
}

// visibleNodes is the tree as the rows show it, top to bottom. Without a filter: the root and, below each open
// folder, its subfolders. With a filter: every folder read so far whose name matches, with the folders above it so
// it stays in its place in the tree (the root always shows, so the user sees where they are).
export function visibleNodes(root, query = '') {
  const out = [];
  if (!root) return out;
  if (!filterWords(query).length) {
    const walk = (n) => { out.push(n); if (n.open && n.kids) for (const k of n.kids) walk(k); };
    walk(root);
    return out;
  }
  const keep = (n) => {
    let any = n.depth > 0 && nameMatches(n.name, query);
    const kids = [];
    for (const k of n.kids || []) { const r = keep(k); if (r) { any = true; kids.push(...r); } }
    return any || n.depth === 0 ? [n, ...kids] : null;
  };
  return keep(root) || [root];
}

// samePath compares paths the way the system does (Windows ignores case), without a trailing separator.
export function samePath(a, b, ignoreCase = false) {
  const t = (s) => { let x = String(s || ''); if (x.length > 1 && /[\\/]$/.test(x) && !/^[a-z]:\\$/i.test(x)) x = x.slice(0, -1); return ignoreCase ? x.toLowerCase() : x; };
  return t(a) === t(b);
}

// chainBelow is the list of folders from just below root down to path ([] when path is the root or not under it).
export function chainBelow(root, path, ignoreCase = false) {
  const parts = pathParts(path);
  const i = parts.findIndex((x) => samePath(x, root, ignoreCase));
  if (i < 0) return [];
  return parts.slice(i + 1);
}

// findKid is the subfolder of n with that path.
export function findKid(n, path, ignoreCase = false) {
  return (n?.kids || []).find((k) => samePath(k.path, path, ignoreCase)) || null;
}
