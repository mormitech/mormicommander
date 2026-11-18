// Pure helpers of the tree in the panel (Ctrl+F8, features/views.js) for Commands > Refresh folder tree
// (cm_RescanTree, F-369, issue #62): the tree reads each folder once, when it is opened, so folders made or deleted
// later only show after a rescan. These merge a fresh read of a folder into the nodes the tree already has, keeping what
// was open, and pick where the cursor goes when its folder is gone. No DOM and no app, so the tests run them in Node.

// mergeKids turns the fresh list of a folder ([{name, path, file?}]) into the child nodes of `parent`, reusing the old
// node of each entry that is still there (so an open subfolder stays open and keeps its children until they are read
// again) and making new, closed nodes for the new entries. Entries that are gone are dropped.
export function mergeKids(parent, fresh, low = (s) => s) {
  const old = new Map((parent.kids || []).map((k) => [low(k.path), k]));
  return (fresh || []).map((f) => {
    const was = old.get(low(f.path));
    if (was && !!was.file === !!f.file) return Object.assign(was, { name: f.name, path: f.path, depth: parent.depth + 1, parentNode: parent });
    return { ...f, depth: parent.depth + 1, parentNode: parent, open: false, kids: f.file ? [] : null };
  });
}

// cursorAfter finds the node the cursor goes to after a rescan: the node with the path it was on, or else the nearest
// folder above it that is still in the tree (the root at the latest). nodes is the flattened tree.
export function cursorAfter(nodes, path, low = (s) => s) {
  if (!nodes.length) return 0;
  let want = path || '';
  for (;;) {
    const i = nodes.findIndex((n) => low(n.path) === low(want));
    if (i >= 0) return i;
    const up = want.replace(/[\\/]+$/, '').replace(/[^\\/]+$/, '');
    if (!up || up === want) return 0;
    want = up.length > 1 ? up.replace(/[\\/]+$/, '') || up : up;
    if (/^[a-z]:$/i.test(want)) want += '\\';
  }
}
