// Where the cursor goes when a panel moves through its folder history (Alt+Left / Alt+Right, Cmd+[ / Cmd+], mouse
// buttons 4/5 and choosing a folder from the Alt+Down list). TC brings back the entry the cursor was on when the
// folder was last left (F-074); when that is not known but the folder being left lies inside the target, the cursor
// goes on the subfolder that leads to it, the same rule Backspace uses (board issue #126). Kept free of the DOM so it
// can be tested on its own.

// historyFocus returns the name to put the cursor on in target, or null for the default (the top).
// from: the folder being left; lastCursor: a Map of folder → name; sep: the path separator of target.
export function historyFocus(target, from, lastCursor, sep = '/') {
  const remembered = lastCursor?.get(target);
  if (remembered) return remembered;
  if (!target || !from || from === target) return null;
  const prefix = target.endsWith(sep) || target.endsWith('/') ? target : target + sep;
  if (!from.startsWith(prefix)) return null;
  const rest = from.slice(prefix.length);
  const i = rest.indexOf(sep);
  const name = i < 0 ? rest : rest.slice(0, i);
  return name || null;
}
