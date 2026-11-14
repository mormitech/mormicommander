// The pure parts of the AI tools (issue #146): reading the arguments a model sends, turning find_files arguments into
// the Find files dialog's request (feature_find.go FindRequest), the selection rules of select_files, and the short
// texts the AI tab and the model read. Nothing here touches the page or Go, so the tests run it as it is
// (test/aitools-lib.test.js); aitools-basic.js and find-ai.js do the work around it.

// ---- arguments ----

// side reads a panel side: "left", "right", or anything else (missing, "active", "source") for the active panel.
// "other" / "target" are accepted too, as app.panel() knows them.
export function side(v) {
  const s = String(v ?? '').trim().toLowerCase();
  return ['left', 'right', 'other', 'target'].includes(s) ? s : 'active';
}

// mode reads select_files' mode: "replace" (the default), "add" or "remove".
export function selectMode(v) {
  const m = String(v ?? '').trim().toLowerCase();
  return m === 'add' || m === 'remove' ? m : 'replace';
}

// names reads a list of names: an array, or one string with names separated by ";" or new lines (models do both).
export function nameList(v) {
  const arr = Array.isArray(v) ? v : typeof v === 'string' ? v.split(/[;\n]/) : [];
  return arr.map((x) => String(x ?? '').trim()).filter((x) => x && x !== '..');
}

// maskExpr turns select_files' pattern ("*.pdf;*.docx", the contract's ";" list) into a mask expression of the
// Select files dialog (internal/masks: masks separated by ";" or spaces, those with spaces in double quotes). A mask
// with a space is quoted, so "My file*.txt" stays one mask instead of two; a regular expression ("<…") and an
// expression that already has quotes are passed as they are. "" is no pattern.
export function maskExpr(pattern) {
  const p = String(pattern ?? '').trim();
  if (!p || p.startsWith('<') || p.includes('"')) return p;
  return p.split(';').map((m) => m.trim()).filter(Boolean).map((m) => (/\s/.test(m) ? `"${m}"` : m)).join(';');
}

// maskKind is the kind of rows a pattern selects, the way the Select files dialog (Num +) decides it: files only,
// unless the option "markDirectories" is on; a mask that ends in "/" or "\" names folders, so then folders count too.
export function maskKind(expr, markDirs = false) {
  if (markDirs) return 'both';
  return expr.split(/[;\s|]/).some((m) => /[\\/]"?$/.test(m)) ? 'both' : 'files';
}

// newSelection is the set of names selected after select_files: the matched names replace the selection, are added to
// it, or are taken out of it. ".." is never selected, whatever matched.
export function newSelection(current, matched, mode) {
  const out = mode === 'replace' ? new Set() : new Set(current);
  for (const n of matched) {
    if (n === '..') continue;
    if (mode === 'remove') out.delete(n); else out.add(n);
  }
  out.delete('..');
  return out;
}

// ---- dates and paths ----

const pad = (n) => String(n).padStart(2, '0');

// day writes a date as YYYY-MM-DD in local time (as the dialog's date fields have it).
export const day = (d) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

// addDays moves a YYYY-MM-DD date by n days (n may be negative).
export function addDays(ymd, n) {
  const [y, m, d] = ymd.split('-').map(Number);
  return day(new Date(y, m - 1, d + n));
}

const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;

// validDay checks a YYYY-MM-DD date and answers it, or throws a sentence the model can act on.
function validDay(v, field) {
  const s = String(v ?? '').trim();
  if (!s) return '';
  if (!DAY_RE.test(s) || addDays(s, 0) !== s) throw new Error(`${field} must be a date written as YYYY-MM-DD, like 2026-03-31; got "${s}".`);
  return s;
}

// expandHome turns "~" and "~/x" into the home folder (Find does not know "~"; models write it often).
export function expandHome(p, home, sep = '/') {
  p = String(p ?? '').trim().replace(/^"(.*)"$/, '$1');
  if (!home) return p;
  if (p === '~') return home;
  if (p.startsWith('~/') || p.startsWith('~\\')) return (home.endsWith(sep) ? home : home + sep) + p.slice(2);
  return p;
}

// tilde shortens a path in the home folder to "~/…", for the progress lines.
export function tilde(p, home) {
  if (!home || home === '/' || !p) return p;
  if (p === home) return '~';
  return p.startsWith(home) && /[\\/]/.test(p[home.length] || '') ? '~' + p.slice(home.length) : p;
}

// ---- find_files → FindRequest ----

const SIZE_UNITS = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3, TB: 1024 ** 4 };

const int = (v) => (v === undefined || v === null || v === '' ? null : Number.isFinite(+v) ? Math.trunc(+v) : NaN);
const bool = (v) => v === true || v === 'true' || v === 1;

// findRequest turns find_files arguments into a FindRequest with the defaults of a search started from the dialog
// (the encodings and code pages the dialog ticks, sizes in KB, age in days, duplicates off). env: {activePath, home,
// sep, today (YYYY-MM-DD)}. Hidden files are skipped unless includeHidden is true: a search of the home folder
// that walks every dot-folder is slow and rarely what a prompt means. Bad arguments throw a sentence for the model.
export function findRequest(args = {}, env = {}) {
  const a = args || {};
  const sep = env.sep || '/';
  const searchIn = String(a.searchIn ?? '').split(';').map((p) => expandHome(p, env.home, sep)).filter(Boolean)
    .map((p) => (/[;\s]/.test(p) && !p.startsWith('"') ? `"${p}"` : p)).join(';') || env.activePath || '';
  const depth = int(a.depth);
  if (Number.isNaN(depth) || (depth !== null && depth < -1)) throw new Error('depth must be -1 (all subfolders), 0 (the folder only) or a number of levels.');
  const text = String(a.text ?? '');
  const r = {
    searchFor: String(a.namePattern ?? '').trim() || '*', nameRegex: bool(a.nameRegex), searchIn,
    selected: [], withinRun: 0, depth: depth ?? -1, archives: false,
    textOn: text !== '', text, wholeWords: false, caseSensitive: bool(a.caseSensitive), notContaining: false,
    hex: false, textRegex: bool(a.textRegex), encodings: ['ansi', 'utf8', 'utf16le'], ansiCodepage: 'windows-1252', oemCodepage: 'cp437',
    dateOn: false, dateFrom: '', dateTo: '',
    ageOn: false, ageOlder: false, ageN: 1, ageUnit: 'days',
    sizeOn: false, sizeOp: '>', sizeValue: 0, sizeUnit: 'KB',
    attrSet: '', attrClear: '', perms: '', kind: a.kind === 'files' || a.kind === 'dirs' ? a.kind : '',
    followLinks: false, skipHidden: !bool(a.includeHidden), excludeDirs: String(a.excludeDirs ?? '').trim(),
    dupOn: false, dupName: true, dupSize: true, dupContent: false,
  };
  if (!r.searchIn) throw new Error('searchIn is needed: the folders to search in, separated by ";".');

  // size: a fraction ("1.5 MB") is turned into bytes, since the request holds whole numbers only
  if (a.sizeValue !== undefined && a.sizeValue !== null && a.sizeValue !== '') {
    const v = +a.sizeValue;
    if (!Number.isFinite(v) || v < 0) throw new Error('sizeValue must be a number of at least 0.');
    const op = ['>', '<', '='].includes(a.sizeOp) ? a.sizeOp : '>';
    let unit = String(a.sizeUnit ?? 'B').toUpperCase();
    if (!SIZE_UNITS[unit]) throw new Error('sizeUnit must be B, KB, MB, GB or TB.');
    let n = v;
    if (!Number.isInteger(v)) { n = Math.round(v * SIZE_UNITS[unit]); unit = 'B'; }
    Object.assign(r, { sizeOn: true, sizeOp: op, sizeValue: n, sizeUnit: unit });
  }

  // dates: the request has one date range and one age, never both; one age alone stays an age (to the minute, like
  // the dialog's), anything more becomes a range of whole days counted from today
  let from = validDay(a.dateFrom, 'dateFrom'), to = validDay(a.dateTo, 'dateTo');
  const older = int(a.olderThanDays), newer = int(a.newerThanDays);
  for (const [v, f] of [[older, 'olderThanDays'], [newer, 'newerThanDays']]) {
    if (Number.isNaN(v) || (v !== null && v < 0)) throw new Error(`${f} must be a whole number of days.`);
  }
  const ages = (older !== null) + (newer !== null);
  if (ages === 1 && !from && !to) {
    Object.assign(r, { ageOn: true, ageOlder: older !== null, ageN: Math.max(older ?? newer, older !== null ? 0 : 1), ageUnit: 'days' });
  } else if (ages || from || to) {
    const today = env.today || day(new Date());
    if (newer !== null) { const f = addDays(today, -newer); if (!from || f > from) from = f; }
    if (older !== null) { const t = addDays(today, -older - 1); if (!to || t < to) to = t; }
    if (from && to && from > to) throw new Error(`The dates leave nothing to find: from ${from} to ${to}.`);
    Object.assign(r, { dateOn: true, dateFrom: from, dateTo: to });
  }
  return r;
}

// ---- texts ----

const group = new Intl.NumberFormat('en-US');
const count = (n, word, many = word + 's') => `${group.format(n)} ${n === 1 ? word : many}`;

// humanSize writes a size for the model and the log: "512 B", "1.4 KB", "3.2 MB".
export function humanSize(n) {
  if (!(n >= 0)) return '';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0, v = n;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return i === 0 ? `${v} B` : `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
}

const stamp = (ms) => { if (!ms) return ''; const d = new Date(ms); return `${day(d)} ${pad(d.getHours())}:${pad(d.getMinutes())}`; };

// findSummary is what find_files answers: the count, then up to `limit` results, one per line (path, size, date;
// folders end with a separator and say "folder"). items are the Find results ({p, s, t, f}); done is the find-done
// (stopped, error, cut, errors) or null; where says where the list is shown, when it is.
export function findSummary(items, done, { limit = 200, where = '' } = {}) {
  if (done?.error) return `The search failed: ${done.error}`;
  const files = items.filter((x) => !x.f).length, dirs = items.length - files;
  let t = items.length ? `Found ${count(items.length, 'result')} (${count(files, 'file')}, ${count(dirs, 'folder')})` : 'Found nothing';
  if (done?.stopped) t += '; the search was stopped before it finished';
  if (done?.cut) t += '; there were more results than the search keeps';
  if (done?.errors) t += `; ${count(done.errors, 'folder or file', 'folders or files')} could not be read`;
  t += '.';
  if (where && items.length) t += ` ${where}`;
  if (items.length > limit) t += ` The first ${limit} are listed.`;
  const lines = items.slice(0, limit).map((x) => (x.f ? `${x.p}${/[\\/]$/.test(x.p) ? '' : '/'}  folder  ${stamp(x.t)}` : `${x.p}  ${humanSize(x.s)}  ${stamp(x.t)}`));
  return lines.length ? `${t}\n${lines.join('\n')}` : t;
}

// toolName drops the prefix the CLI puts before a tool of an MCP server ("mcp__mormi__find_files" → "find_files").
export const toolName = (n) => String(n ?? '').replace(/^mcp__[^_]+(?:_[^_]+)*?__/, '');

const short = (s, n = 60) => (s.length > n ? s.slice(0, n - 1) + '…' : s);

// describeTool is the line the AI tab's log shows when the model calls a tool: "Searching *.pdf in ~/Documents…".
export function describeTool(name, args, home = '') {
  const a = args || {};
  const where = (s) => (s === 'active' || !s ? 'the active panel' : `the ${s} panel`);
  switch (toolName(name)) {
    case 'find_files': {
      const what = String(a.namePattern || '').trim() || (a.kind === 'dirs' ? 'folders' : 'files');
      const text = a.text ? ` containing "${short(String(a.text), 30)}"` : '';
      const dirs = String(a.searchIn || '').split(';').map((p) => tilde(p.trim(), home)).filter(Boolean).join(', ');
      return `Searching ${short(what, 40)}${text} in ${dirs ? short(dirs) : 'the panel\'s folder'}…`;
    }
    case 'select_files': {
      const what = String(a.pattern || '').trim() || nameList(a.names).join(', ') || 'files';
      return `${selectMode(a.mode) === 'remove' ? 'Unselecting' : 'Selecting'} ${short(what, 50)} in ${where(side(a.side))}…`;
    }
    case 'set_panel_path': return `Opening ${short(tilde(String(a.path || ''), home))} in ${where(side(a.side))}…`;
    case 'get_state': return 'Looking at the panels…';
    default: return `Using ${toolName(name)}…`;
  }
}

const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

// findContext is the situation the page tells the model with a prompt from the Find files dialog (AIRequest.context;
// Go puts it before the prompt). now is a Date.
export function findContext({ active, other, platform, home, searchIn }, now = new Date()) {
  const lines = [
    'The user is in the Find files dialog of mormiCommander, a two-panel file manager. The results of find_files fill this dialog\'s result list, where the user can go to a file, view it or feed the list to a panel.',
    `The active panel (${active.side}) shows ${active.path}.`,
    `The other panel (${other.side}) shows ${other.path}.`,
  ];
  if (searchIn && searchIn !== active.path) lines.push(`The dialog's "Search in" field says ${searchIn}.`);
  lines.push(`Platform: ${platform}. Home folder: ${home}. Today is ${WEEKDAYS[now.getDay()]}, ${day(now)}.`);
  return lines.join('\n');
}

// providerLine is the AI tab's line about the provider, from AI.Providers(): {selected: "claude"|"openrouter", claude,
// openrouter}. It names the provider a run will use (the one chosen in the AI settings) and its model, or says what
// is missing. name is how the log speaks of it ("Asking Claude…"). An object with "available" is read as the Claude
// entry alone (the shape of step 1).
export function providerLine(p) {
  if (!p) return { ok: false, text: 'AI is not available in this build.', name: 'the AI' };
  if ('available' in p) p = { selected: 'claude', claude: p };
  if (p.selected === 'openrouter') {
    const o = p.openrouter || {};
    // (another OpenAI-compatible server is named by its host; OpenRouter's own address needs no mention)
    let host = '';
    try { host = o.url ? new URL(o.url).host : ''; } catch { host = ''; }
    const label = !host || host === 'openrouter.ai' || host.endsWith('.openrouter.ai') ? 'OpenRouter' : `OpenAI-compatible API at ${host}`;
    if (o.available) return { ok: true, text: `${label}, model ${o.model}`, name: o.model };
    return { ok: false, text: `${label}: ${o.error || 'not set up yet.'}`, error: o.error || '', name: label };
  }
  const c = p.claude;
  if (!c) return { ok: false, text: 'AI is not available in this build.', name: 'the AI' };
  if (c.available) {
    const v = String(c.version || '').replace(/\s*\(Claude Code\)\s*$/i, '').trim();
    return { ok: true, text: `Claude Code${v ? ' ' + v : ''} found at ${c.path}`, name: 'Claude' };
  }
  return { ok: false, text: 'Claude Code was not found: install it from claude.com/claude-code, or set its path in Options > AI.', error: c.error || '', name: 'Claude' };
}
