// The pure parts of the step 3 AI tools (issue #146): checking a value for an option against its type and range,
// the colour scheme checks (a text colour must stay readable on its background), the archive name of pack_files, the
// context and the log lines of the Ask dialog, its prompt history and the provider lines. Nothing here touches the
// page or Go, so test/aitools-more-lib.test.js runs it as it is; aitools-commands.js, aitools-settings.js,
// aitools-pack.js and ask.js do the work around it.

import { describeTool, toolName, tilde, providerLine, day } from './aitools-lib.js';
import { TEXT_FIELDS } from './aitools-allow.js';

// ---- option values ----

const TRUE = new Set(['1', 'true', 'yes', 'on']);
const FALSE = new Set(['0', 'false', 'no', 'off']);

// settingValue checks a value the model sends for an option and answers the option's text as Go keeps it ("1"/"0"
// for yes/no, a number, a text), or throws a sentence the model can act on. field is the schema's field: {name, type
// ("bool", "int", "string"), min, max, options: [{value, label}]}. A choice may be given by its number or its label.
export function settingValue(field, value) {
  const name = field.name;
  if (value === undefined || value === null) throw new Error(`Give the new value of ${name} (value).`);
  if (field.type === 'bool') {
    const s = String(value).trim().toLowerCase();
    if (TRUE.has(s)) return '1';
    if (FALSE.has(s)) return '0';
    throw new Error(`${name} is a yes/no option: give true or false.`);
  }
  if (field.type === 'int') {
    const opts = field.options || [];
    let n;
    if (typeof value === 'number') n = value;
    else {
      const s = String(value).trim();
      const byLabel = opts.find((o) => o.label.toLowerCase() === s.toLowerCase());
      n = byLabel ? byLabel.value : /^-?\d+$/.test(s) ? Number(s) : NaN;
    }
    if (!Number.isInteger(n)) throw new Error(`${name} needs a whole number${opts.length ? ` or one of: ${choiceList(opts)}` : ''}.`);
    if (opts.length && !opts.some((o) => o.value === n)) throw new Error(`${name} must be one of: ${choiceList(opts)}.`);
    if (field.min != null && n < field.min) throw new Error(`${name} must be at least ${field.min}.`);
    if (field.max != null && n > field.max) throw new Error(`${name} must be at most ${field.max}.`);
    return String(n);
  }
  const s = String(value).trim();
  if (s === '') return '';
  const [page, key] = String(name).split('.');
  const rule = TEXT_FIELDS[`${page}:${key}`];
  if (!rule) throw new Error(`${name} is a text option the AI may not change.`);
  if (!rule.re.test(s)) throw new Error(`${name} must be ${rule.hint}; got "${s.slice(0, 80)}".`);
  return s;
}

const choiceList = (opts) => opts.map((o) => `${o.value} (${o.label})`).join(', ');

// settingInfo is how an option is told to the model: its name, label, type, value (true/false for yes/no, a number,
// a text), what the value means for a choice, and the allowed range or choices.
export function settingInfo(field, text) {
  const v = field.type === 'bool' ? text === '1' : field.type === 'int' ? Number(text) : String(text ?? '');
  const out = { name: field.name, label: field.label, type: field.type === 'bool' ? 'boolean' : field.type === 'int' ? 'integer' : 'text', value: v };
  if (field.options?.length) {
    out.choices = field.options.map((o) => ({ value: o.value, label: o.label }));
    const o = field.options.find((x) => x.value === v);
    if (o) out.valueLabel = o.label;
  } else if (field.type === 'int') {
    if (field.min != null) out.min = field.min;
    if (field.max != null) out.max = field.max;
  }
  return out;
}

// ---- colours ----

// The colours of a scheme, as the Options dialog's Colours page edits them (features/config/options.js COLOR_KEYS, and
// Go's configSchemeColors in feature_config.go). "" means the theme's own colour.
export const COLOR_KEYS = [
  ['fore', 'Text'], ['back', 'Background'], ['back2', 'Second line background'], ['mark', 'Selected files'], ['cursor', 'Cursor'],
  ['cursorText', 'Text under the cursor'], ['inactiveCursor', 'Cursor in the inactive panel'], ['inactiveMark', 'Selected files in the inactive panel'],
  ['inactiveBack', 'Background of the inactive panel'],
];
const KEYS = new Set(COLOR_KEYS.map(([k]) => k));

// The theme's colours where a scheme leaves a colour empty (style.css: --text, --bg, --sel, --accent; the cursor is the
// accent at 34 %, the inactive cursor at 55 %, over the background).
export const THEME = {
  dark: { text: '#e8edf4', bg: '#1b2636', sel: '#ff7b72', accent: '#5aa9ff' },
  light: { text: '#1c2533', bg: '#f4f6fa', sel: '#c9252c', accent: '#1d6fd8' },
};

// MIN_CONTRAST is the least contrast (WCAG ratio, 1 to 21) a text colour must have on its background. 3 is WCAG's
// floor for large text: lower than that, a random scheme can leave names that are hard or impossible to read.
export const MIN_CONTRAST = 3;

// hexColor reads a colour as "#rrggbb" ("#abc" and "abc123" are taken too); "" stays "" (the theme's colour). Anything
// else throws: named colours and rgb() are not what the settings file keeps.
export function hexColor(v, what = 'A colour') {
  const s = String(v ?? '').trim().toLowerCase();
  if (s === '') return '';
  const m = s.match(/^#?([0-9a-f]{3}|[0-9a-f]{6})$/);
  if (!m) throw new Error(`${what} must be written as #rrggbb (like #1e2a3b), or "" for the theme's colour; got "${String(v).slice(0, 40)}".`);
  const h = m[1].length === 3 ? m[1].split('').map((c) => c + c).join('') : m[1];
  return `#${h}`;
}

const rgb = (hex) => [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
const toHex = (c) => `#${c.map((x) => Math.round(Math.max(0, Math.min(255, x))).toString(16).padStart(2, '0')).join('')}`;

// blend lays a colour over another with an opacity (0 to 1), as color-mix(… a%, transparent) over a background does.
export const blend = (fg, bg, a) => { const f = rgb(fg), b = rgb(bg); return toHex(f.map((x, i) => x * a + b[i] * (1 - a))); };

function luminance(hex) {
  const [r, g, b] = rgb(hex).map((x) => { const c = x / 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

// contrast is the WCAG contrast ratio of two colours: 1 (the same) to 21 (black on white).
export function contrast(a, b) {
  const x = luminance(a), y = luminance(b);
  return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05);
}

// effectiveColors fills a scheme's empty colours with what the panels really show (the theme's colours, the cursor
// over the background), so the pairs below can be measured.
export function effectiveColors(colors = {}, theme = 'dark') {
  const T = THEME[theme] || THEME.dark;
  const c = colors || {};
  const back = c.back || T.bg;
  const fore = c.fore || T.text;
  const mark = c.mark || T.sel;
  const inactiveBack = c.inactiveBack || back;
  return {
    fore, back, mark, back2: c.back2 || back,
    cursor: c.cursor || blend(T.accent, back, 0.34), cursorText: c.cursorText || fore,
    inactiveBack, inactiveCursor: c.inactiveCursor || blend(T.accent, inactiveBack, 0.55), inactiveMark: c.inactiveMark || mark,
  };
}

// The text-on-background pairs of the file lists: [text, background, what the user would not be able to read].
const PAIRS = [
  ['fore', 'back', 'the file names on the background'],
  ['fore', 'back2', 'the file names on the second line background'],
  ['mark', 'back', 'selected files on the background'],
  ['mark', 'back2', 'selected files on the second line background'],
  ['cursorText', 'cursor', 'the name under the cursor'],
  ['fore', 'inactiveBack', 'the file names in the inactive panel'],
  ['inactiveMark', 'inactiveBack', 'selected files in the inactive panel'],
  ['fore', 'inactiveCursor', 'the name under the cursor of the inactive panel'],
];

// schemeProblems lists what a change of one scheme would make hard to read: every pair (and every colour by file type
// on the background) whose contrast is below MIN_CONTRAST and that the change touched, so a pair that was already poor
// before does not block an unrelated change. before and after are {colors, filters}; theme is "light" or "dark".
export function schemeProblems(before, after, theme) {
  const b = effectiveColors(before?.colors, theme), a = effectiveColors(after?.colors, theme);
  const out = [];
  for (const [t, bg, what] of PAIRS) {
    if (a[t] === b[t] && a[bg] === b[bg]) continue;
    const r = contrast(a[t], a[bg]);
    if (r < MIN_CONTRAST) out.push({ what, text: a[t], background: a[bg], ratio: Math.round(r * 100) / 100 });
  }
  const old = new Map((before?.filters || []).map((f) => [`${f.mask}\n${f.color}`, true]));
  for (const f of after?.filters || []) {
    if (!f.color) continue;
    if (old.has(`${f.mask}\n${f.color}`) && a.back === b.back) continue;
    const r = contrast(f.color, a.back);
    if (r < MIN_CONTRAST) out.push({ what: `files matching ${f.mask}`, text: f.color, background: a.back, ratio: Math.round(r * 100) / 100 });
  }
  return out;
}

// problemText is the refusal the model reads for schemeProblems.
export function problemText(problems, theme) {
  const lines = problems.map((p) => `${p.what}: ${p.text} on ${p.background} has a contrast of ${p.ratio}`);
  return `The ${theme} colours were not changed: ${lines.join('; ')}. Text needs a contrast of at least ${MIN_CONTRAST} with its background (1 means invisible, 21 is black on white); choose a lighter or darker colour.`;
}

// newScheme makes the scheme a set_colors call asks for: colours replaced by key ("" = the theme's colour), the list of
// colours by file type replaced when filters is given, or a built-in preset's colours (as the Colours page's Use button
// does) before the colours. Bad keys and colours throw.
export function newScheme(old, { colors, filters, preset } = {}, builtin = []) {
  const s = { ...old, colors: { ...(old?.colors || {}) }, filters: (old?.filters || []).map((f) => ({ ...f })) };
  if (preset !== undefined && preset !== null && preset !== '') {
    const p = builtin.find((x) => x.name.toLowerCase() === String(preset).trim().toLowerCase());
    if (!p) throw new Error(`There is no colour preset called "${preset}". The presets are: ${builtin.map((x) => x.name).join(', ') || 'none'}.`);
    for (const [k] of COLOR_KEYS) s.colors[k] = p.colors?.[k] || '';
  }
  if (colors !== undefined && colors !== null) {
    if (typeof colors !== 'object' || Array.isArray(colors)) throw new Error('colors must be an object like {"fore": "#e0e0e0", "back": "#202020"}.');
    for (const [k, v] of Object.entries(colors)) {
      if (!KEYS.has(k)) throw new Error(`"${k}" is not a colour of a scheme. The colours are: ${COLOR_KEYS.map(([x, l]) => `${x} (${l})`).join(', ')}.`);
      s.colors[k] = hexColor(v, `The colour ${k}`);
    }
  }
  if (filters !== undefined && filters !== null) {
    if (!Array.isArray(filters)) throw new Error('filters must be a list like [{"mask": "*.jpg *.png", "color": "#4caf50"}].');
    if (filters.length > 100) throw new Error('At most 100 colours by file type can be set.');
    s.filters = filters.map((f, i) => {
      const mask = String(f?.mask ?? '').trim();
      if (!mask || mask.length > 300) throw new Error(`filters[${i}] needs a mask like "*.jpg *.png" (at most 300 characters).`);
      return { mask, color: hexColor(f?.color, `The colour of filters[${i}]`) };
    });
  }
  return s;
}

// schemeOut is a scheme as the model reads it.
export const schemeOut = (s) => ({ colors: { ...(s?.colors || {}) }, filters: (s?.filters || []).map((f) => ({ mask: f.mask, color: f.color || '' })) });

// ---- pack_files ----

// archiveName checks the archive name pack_files gets and finds its packer: the name's extension picks the packer
// (".zip", ".tar.gz", ".7z"), format can name it instead (then a name without that extension gets it). packers are
// go.VfsJobs.Packers() ({name, ext, single, available, note, label}). Answers {name, packer} or throws.
export function archiveName(raw, format, packers) {
  const name = String(raw ?? '').trim();
  if (!name) throw new Error('Give the name of the new archive (archive), like "photos.zip".');
  if (/[\\/:]/.test(name) || name === '.' || name === '..' || /[*?"<>|\0]/.test(name)) {
    throw new Error(`"${name}" is not a plain file name: give the archive's name only, without a folder or wildcards; the "where" argument picks the folder.`);
  }
  if (name.length > 200) throw new Error('The archive name is too long.');
  const low = name.toLowerCase();
  const byExt = [...packers].sort((a, b) => b.ext.length - a.ext.length).find((p) => low.endsWith(`.${p.ext.toLowerCase()}`) && low.length > p.ext.length + 1);
  let pk = byExt;
  let out = name;
  if (format !== undefined && format !== null && String(format).trim() !== '') {
    const f = String(format).trim().toLowerCase().replace(/^\./, '');
    const wanted = packers.find((p) => p.name.toLowerCase() === f || p.ext.toLowerCase() === f) || (f === 'tgz' ? packers.find((p) => p.name === 'tar.gz') : null);
    if (!wanted) throw new Error(`There is no packer called "${format}". The packers are: ${packers.map((p) => p.name).join(', ')}.`);
    if (byExt && byExt !== wanted) throw new Error(`The name ${name} ends in .${byExt.ext} but the format is ${wanted.name}: make them agree.`);
    if (!byExt) out = `${name}.${wanted.ext}`;
    pk = wanted;
  }
  if (!pk) {
    pk = packers.find((p) => p.name === 'zip') || packers[0];
    if (!pk) throw new Error('No packer is available in this build.');
    out = `${name}.${pk.ext}`;
  }
  if (!pk.available) throw new Error(pk.note || `The ${pk.label || pk.name} packer is not available on this computer.`);
  return { name: out, packer: pk };
}

// ---- the Ask dialog ----

const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

// panelLine tells one panel for the context: its folder, the name under the cursor and how many are selected.
function panelLine(p, which) {
  const cur = p.cursor ? `; the cursor is on ${p.cursor}` : '';
  return `The ${which} panel (${p.side}) shows ${p.path || 'nothing yet'}${cur}; ${p.selected} ${p.selected === 1 ? 'entry is' : 'entries are'} selected.`;
}

// askContext is what the page tells the model with a prompt from the Ask dialog (AIRequest.context; Go puts it before
// the prompt). active and other are {side, path, cursor, selected (a count)}; now is a Date.
export function askContext({ active, other, platform, home }, now = new Date()) {
  return [
    'The user is in the Ask AI dialog of mormiCommander, a two-panel file manager like Total Commander. You act only through the mormiCommander tools you were given: they read, navigate, select, search, change the view and the look, and pack copies of files into a new archive.',
    'None of the tools deletes, moves, renames or overwrites files, runs programs or a shell, or changes which program the app uses, and there is no other way to do those things. When the user asks for one of them, say that you cannot do it and tell them how to do it themselves in mormiCommander.',
    'Before changing what the user did not ask for, ask. After a change, say briefly what you changed, so the user can change it back.',
    panelLine(active, 'active'),
    panelLine(other, 'other'),
    `Platform: ${platform}. Home folder: ${home}. Today is ${WEEKDAYS[now.getDay()]}, ${day(now)}.`,
  ].join('\n');
}

const short = (s, n = 60) => (s.length > n ? s.slice(0, n - 1) + '…' : s);
const sideText = (s) => (s === 'left' || s === 'right' ? `the ${s} panel` : s === 'other' ? 'the other panel' : 'the active panel');

// describeAnyTool is the log line of a tool call in the Ask dialog: the tools of this step, then the others
// (aitools-lib.js describeTool). label(name) may give a command's label.
export function describeAnyTool(name, args, home = '', label = () => '') {
  const a = args || {};
  switch (toolName(name)) {
    case 'list_commands': return 'Looking up the commands…';
    case 'run_command': { const n = String(a.name || ''); const l = label(n); return `Running ${l ? `${l} (${n})` : n}…`; }
    case 'open_folder': return `Opening ${a.name ? short(String(a.name), 40) : 'the folder under the cursor'} in ${sideText(a.side)}…`;
    case 'set_panel_paths': {
      const parts = [a.left ? `left: ${short(tilde(String(a.left), home), 40)}` : '', a.right ? `right: ${short(tilde(String(a.right), home), 40)}` : ''].filter(Boolean);
      return `Opening ${parts.join(', ') || 'folders'}…`;
    }
    case 'list_settings': return a.page ? `Reading the ${a.page} options…` : 'Reading the options…';
    case 'get_setting': return `Reading ${a.name || 'an option'}…`;
    case 'set_setting': return `Setting ${a.name || 'an option'} to ${short(JSON.stringify(a.value ?? ''), 30)}…`;
    case 'get_colors': return 'Reading the colours…';
    case 'set_colors': return `Changing the ${a.scheme && a.scheme !== 'current' ? `${a.scheme} ` : ''}colours…`;
    case 'restore_colors': return 'Putting the colours back…';
    case 'pack_files': return `Packing into ${short(String(a.archive || 'an archive'), 40)}…`;
    default: return describeTool(name, args, home);
  }
}

// pushHistory adds a prompt to the history (newest last, no repeats, at most max).
export function pushHistory(list, prompt, max = 30) {
  const p = String(prompt ?? '').trim();
  const out = (Array.isArray(list) ? list : []).filter((x) => typeof x === 'string' && x && x !== p);
  if (p) out.push(p);
  return out.slice(-max);
}

// historyStep moves in the history with Up (-1) or Down (+1). at is the position now (list.length: the box shows the
// draft, not a history entry); answers {at, text}, text being the draft again past the newest entry.
export function historyStep(list, at, dir, draft = '') {
  const n = list.length;
  const next = Math.max(0, Math.min(n, at + dir));
  return { at: next, text: next >= n ? draft : list[next] };
}

// providerLines tells the state of every provider AI.Providers() answers: Claude Code with step 1's line, any other
// provider (step 2 adds "openrouter") in the same plain way. ok is true when at least one can be used.
export function providerLines(providers) {
  if (!providers || typeof providers !== 'object') return { ok: false, lines: [{ ok: false, text: 'AI is not available in this build.' }] };
  // Go names the provider Options > AI chose ("selected", step 2): a run with provider '' uses exactly that one, so its
  // line is the one that matters (providerLine knows both shapes).
  if (typeof providers.selected === 'string') { const l = providerLine(providers); return { ok: l.ok, lines: [l] }; }
  const lines = [];
  for (const [id, p] of Object.entries(providers)) {
    if (!p || typeof p !== 'object') continue;
    if (id === 'claude') { lines.push(providerLine(p)); continue; }
    const name = p.label || p.name || (id === 'openrouter' ? 'OpenRouter' : id);
    const model = p.model ? ` (${p.model})` : '';
    lines.push(p.available
      ? { ok: true, text: `${name}${model} is ready.` }
      : { ok: false, text: `${name} is not set up${p.error ? `: ${p.error}` : '.'}`, error: p.error || '' });
  }
  if (!lines.length) return { ok: false, lines: [{ ok: false, text: 'AI is not available in this build.' }] };
  return { ok: lines.some((l) => l.ok), lines };
}
