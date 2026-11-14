// The pure parts of the git status feature (features/git.js): the settings with their defaults, the text and tooltip
// of the branch badge in the path bar, the status of one entry and the row classes it gives. No DOM here, so
// frontend/test/git-lib.test.js tests it alone.
//
// Status letters come from Go (internal/gitstat): M changed, A added, D deleted, R renamed, U conflict, ? untracked,
// ! ignored.

// DEFAULTS: the feature is off until the user turns it on (Show > Git status), so nothing changes for anyone who does
// not want it. column: the status letter beside each name; branch: the badge in the path bar; ignored: what happens to
// ignored files — "show" (like any file), "dim" (drawn faded) or "hide" (treated as hidden files, so they show only
// while hidden files are shown).
export const DEFAULTS = { enabled: false, column: true, branch: true, ignored: 'show' };
export const IGNORED_MODES = [['show', 'Show them like other files'], ['dim', 'Show them dimmed'], ['hide', 'Treat them as hidden files']];

// gitConfig fills a saved "git" setting up with the defaults and drops values that make no sense.
export function gitConfig(saved) {
  const s = saved && typeof saved === 'object' ? saved : {};
  const c = { ...DEFAULTS };
  if (typeof s.enabled === 'boolean') c.enabled = s.enabled;
  if (typeof s.column === 'boolean') c.column = s.column;
  if (typeof s.branch === 'boolean') c.branch = s.branch;
  if (IGNORED_MODES.some(([v]) => v === s.ignored)) c.ignored = s.ignored;
  return c;
}

const WORDS = { M: 'changed', A: 'added', D: 'deleted', R: 'renamed', U: 'conflict', '?': 'not tracked', '!': 'ignored' };
const CLASS = { M: 'm', A: 'a', D: 'd', R: 'r', U: 'u', '?': 'new', '!': 'ign' };

// statusOf is the letter of one entry of the folder ('' for none, and always '' for "..").
export function statusOf(r, e) {
  if (!r || !r.repo || !e || e.up) return '';
  return (r.files && r.files[e.name]) || r.all || '';
}

// statusWord says a letter in words (for tooltips and control mode).
export const statusWord = (code) => WORDS[code] || '';

// rowClasses is what a row with status code gets: "mc-git-<kind>" for the letter beside the name (only when the
// column is on) and "mc-git-dim" for an ignored file in "dim" mode. Joined with spaces; '' for nothing.
export function rowClasses(code, cfg) {
  if (!code) return '';
  const out = [];
  if (cfg.column && CLASS[code]) out.push(`mc-git-${CLASS[code]}`);
  if (code === '!' && cfg.ignored === 'dim') out.push('mc-git-dim');
  return out.join(' ');
}

// hidesIgnored: whether an entry with this code is treated as a hidden file.
export const hidesIgnored = (code, cfg) => code === '!' && cfg.ignored === 'hide';

// branchText is the badge in the path bar: the branch (or the commit when detached), a dot when something is changed,
// and ↑ / ↓ with the commits ahead of and behind the upstream. '' outside a repository.
export function branchText(r) {
  if (!r || !r.repo) return '';
  let t = r.detached ? `(${r.oid || 'detached'})` : r.branch || '(no branch)';
  if (r.dirty) t += ' ●';
  if (r.ahead) t += ` ↑${r.ahead}`;
  if (r.behind) t += ` ↓${r.behind}`;
  return t;
}

// branchTitle is the badge's tooltip, in words.
export function branchTitle(r) {
  if (!r || !r.repo) return '';
  const lines = [];
  lines.push(r.detached ? `Git: not on a branch (at ${r.oid || 'no commit yet'})` : `Git branch ${r.branch || '?'}${r.oid ? ` at ${r.oid}` : ', no commit yet'}`);
  if (r.root) lines.push(`Repository: ${r.root}`);
  lines.push(r.dirty ? 'There are changes that are not committed.' : 'Nothing to commit.');
  if (r.upstream) {
    const ab = [];
    if (r.ahead) ab.push(`${r.ahead} commit${r.ahead === 1 ? '' : 's'} ahead`);
    if (r.behind) ab.push(`${r.behind} commit${r.behind === 1 ? '' : 's'} behind`);
    lines.push(`Upstream ${r.upstream}: ${ab.length ? ab.join(', ') : 'up to date'}.`);
  } else if (!r.detached) lines.push('No upstream branch.');
  return lines.join('\n');
}
