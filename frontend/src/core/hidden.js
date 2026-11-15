// The small rules around "show hidden files" (Ctrl+H, cm_SwitchHidSys; board issue #4, F-090, F-242, F-NEW-21), kept
// here as plain functions so they can be tested in Node:
//   - the marker the status line of each panel shows while hidden files are shown ("Hidden: shown");
//   - whether a new Find files search (Alt+F7) includes hidden files: as the last search did (the default, which is
//     how it always worked), or as the panel shows them when the option Find files > "New searches include hidden
//     files when the panel shows them" is on;
//   - the menu label of a command that shows its key in the menu ("Hidden files (Ctrl+H)").

export const HIDDEN_MARK = 'Hidden: shown';

// hiddenMarkHTML is the marker for the status line: empty while hidden files are hidden.
export function hiddenMarkHTML(show) {
  return show ? `<span class="mc-hidden-mark" title="Hidden files are shown">${HIDDEN_MARK}</span>` : '';
}

// findIncludeHidden says whether "Include hidden files" starts ticked in a new search. last is the last search's
// request (or null); followPanel is the option; showHidden is whether the panels show hidden files now; fresh is
// what a search starts with when there is no last search (Options > Find files > Search hidden files, on by default).
export function findIncludeHidden({ last = null, followPanel = false, showHidden = false, fresh = true } = {}) {
  if (followPanel) return !!showHidden;
  if (!last) return !!fresh;
  return !last.skipHidden;
}

// menuLabel adds the key to a menu label, as "Hidden files (Ctrl+H)"; with no key the label stays as it is.
export function menuLabel(label, key) {
  return key ? `${label} (${key})` : label;
}
