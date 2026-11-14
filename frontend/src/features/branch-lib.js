// The pure parts of the branch view (branch.js): which panel paths are branch views, the path of a folder's branch
// view, how a name of the flat list maps to the real file, and the words of its notices. No DOM here, so the unit
// tests load it without a page.

const BRANCH_RE = /^branch(sel)?:\/\//i;
const SCHEME_RE = /^[a-z][a-z0-9+.-]*:\/\//i;

// isBranchPath: branch://<folder> (every file under the folder) or branchsel://N (the selected files and folders).
export const isBranchPath = (p) => BRANCH_RE.test(String(p || '').trim());

// isSelectionBranch: the branch view of a selection (Ctrl+Shift+B).
export const isSelectionBranch = (p) => /^branchsel:\/\//i.test(String(p || '').trim());

// canBranch says whether a folder can be shown as a branch view: folders on this computer only (no archives, servers
// or search results; their names are not real files the walk could read).
export const canBranch = (p) => !!p && !SCHEME_RE.test(String(p).trim());

// branchPathFor is the branch view of a folder: the scheme in front of the real path (branch:///Users/me,
// branch://C:\Users\me). Go's feature_branch.go reads it back the same way.
export const branchPathFor = (folder) => `branch://${folder}`;

// sepsFor: the characters that separate folders in the names Go sends (only "/" on macOS and Linux, where "\" may be
// part of a file name; both on Windows).
const sepsFor = (platform) => (platform === 'windows' ? /[\\/]/ : /\//);

// leafName is the file name of a name of the flat list ("c.txt" of "sub/deeper/c.txt").
export function leafName(name, platform) {
  const parts = String(name).split(sepsFor(platform));
  return parts[parts.length - 1];
}

// dirPart is the folder part of a name, with its separator ("sub/deeper/" of "sub/deeper/c.txt"; "" for "c.txt").
export function dirPart(name, platform) {
  const n = String(name);
  return n.slice(0, n.length - leafName(n, platform).length);
}

// firstPart is the top folder of a name ("sub" of "sub/deeper/c.txt"), or the name itself for a file of the folder:
// where the cursor goes when the branch view is left, so it lands on the folder the file came from.
export function firstPart(name, platform) {
  return String(name).split(sepsFor(platform))[0];
}

// realPath joins a name of the flat list to the folder the view lists, which gives the real file.
export function realPath(base, name, sep) {
  if (!base) return '';
  return base.endsWith(sep) || base.endsWith('/') ? base + name : base + sep + name;
}

// progressText is what the notice says while a branch view is read.
export function progressText(files, folder, fmt = (n) => String(n)) {
  return {
    title: files > 0 ? `Reading the subfolders… ${fmt(files)} ${files === 1 ? 'file' : 'files'} so far` : 'Reading the subfolders…',
    hint: folder ? `Esc or Stop stays in ${folder}.` : 'Esc or Stop stops reading.',
  };
}

// doneText is the message after a branch view was read (info: Go's BranchInfo).
export function doneText(info, fmt = (n) => String(n)) {
  const files = info?.files || 0;
  let t = `${fmt(files)} ${files === 1 ? 'file' : 'files'} from all subfolders. Ctrl+B or ".." goes back.`;
  if (info?.truncated) t = `Only the first ${fmt(info.limit)} files are shown: the folder has more. Ctrl+B or ".." goes back.`;
  if (info?.skipped) t += ` ${fmt(info.skipped)} ${info.skipped === 1 ? 'folder' : 'folders'} could not be read.`;
  return t;
}

// pickWalk matches the listings Go is running (Branch.Progress) to a panel that is reading: the one for the folder
// the panel shows (a reread), else one that is not the other panel's reread. null when there is none.
export function pickWalk(walks, panelPath, otherPath) {
  if (!walks?.length) return null;
  return walks.find((w) => w.path === panelPath) || walks.find((w) => w.path !== otherPath) || null;
}
