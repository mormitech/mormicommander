// Native Quick Look (board issue #19, F-863): the pure part of features/quicklook.js, without DOM or Go, so the tests
// check it alone. quickLookPaths picks what Quick Look shows for a panel: the file or folder under the cursor, nothing
// on the ".." line. Paths inside archives or on servers are passed on too, and Go answers that Quick Look shows files
// on this computer only.

// quickLookPaths is the list of paths to show for a panel ({cur, fullPath(name)}), [] when there is nothing.
export function quickLookPaths(panel) {
  const e = panel?.cur;
  if (!e || e.up || !e.name) return [];
  return [panel.fullPath(e.name)];
}

// FOLLOW_DELAY is how long the cursor must rest before the open panel switches to its file (ms), so holding an arrow
// key does not load every file it passes.
export const FOLLOW_DELAY = 80;
