// The option and colour tools of the AI layer (issue #146, step 3): list_settings, get_setting and set_setting over
// the Options dialog's fields, and get_colors, set_colors and restore_colors over the colour schemes of the Colours
// page. Only the options on the allow-list of aitools-allow.js can be read or changed (how things look, sorting,
// quick search, folder tabs, the viewer's display); options that name programs or paths, turn off confirmations or
// the trash, touch the network or change the AI are refused. A value is checked against the option's type, range and
// choices before it is applied (aitools-more-lib.js settingValue), and the answer gives the old and the new value, so
// the model can tell the user what changed and put it back.
//
// A change goes the way the Options dialog applies one (features/config/options.js save): Go's Configuration.Apply
// saves the settings file, the features that keep an option of their own get it (util.js BRIDGE), and the screen is
// drawn again (layout.js applyAll). Colours go through Configuration.SetColors like the Colours page's. Every colour
// change first keeps the colours it replaces, so restore_colors can put back the last change or everything the AI
// changed in this session; and a change that would make text hard to read on its background is refused.

import { go } from '../core/go.js';
import { addAITool } from '../core/aitools.js';
import { app as theApp } from '../core/app.js';
import { loadSchema, loadValues, currentValue, BRIDGE, isDark } from './config/util.js';
import { applyAll, loadColors } from './config/layout.js';
import { ALLOWED_SETTINGS, settingVerdict } from './aitools-allow.js';
import { settingValue, settingInfo, newScheme, schemeProblems, problemText, schemeOut, COLOR_KEYS, THEME, MIN_CONTRAST } from './aitools-more-lib.js';

const HISTORY_MAX = 20; // colour changes restore_colors can undo one by one

// colorHistory: the light and dark schemes as they were before each colour change of the AI in this session, oldest
// first, as Go's Configuration.Colors() answered them.
const colorHistory = [];

async function fieldOf(name) {
  const schema = await loadSchema();
  return (schema?.fields || []).find((f) => f.name === name) || null;
}

async function allowedField(name) {
  const n = String(name ?? '').trim();
  const f = await fieldOf(n);
  const v = settingVerdict(n, f);
  if (!v.ok) throw new Error(v.reason);
  return f;
}

// redraw reads the options and colours again and puts them on screen, as the Options dialog does after a change.
async function redraw(changed = []) {
  await loadValues(theApp);
  await loadColors();
  applyAll(theApp.startTheme);
  theApp.emit('config', changed);
  theApp.emit('settings');
}

async function applyOption(name, text) {
  const changed = (await go.Configuration.Apply({ [name]: text })) || [];
  await BRIDGE[name]?.set(theApp, text);
  await redraw(changed);
}

// ---- options ----

addAITool({
  name: 'list_settings',
  description: 'Lists the options of mormiCommander\'s Options dialog that the AI may read and change, with their current values: the layout (bars, panels '
    + 'side by side or on top of each other), how file lists look and sort, icons, fonts and sizes, dark mode and how colours are used, quick search, '
    + 'the folder tab bar, the built-in viewer, tooltips, thumbnails and the tree. Each option has its name (Page.Field), label, type, value and the '
    + 'allowed range or choices. Options about programs, paths, confirmations, the trash, the network, file operations or the AI are not listed and cannot be changed. '
    + 'page narrows the list to one page, like "Display".',
  schema: { type: 'object', properties: { page: { type: 'string', description: 'One page: Layout, Display, Icons, Font, Colors, QuickSearch, FolderTabs, Lister, Tooltips, Thumbnails, Tree, CustomColumns or ViewModes.' } } },
  run: async (args) => {
    const schema = await loadSchema();
    await loadValues(theApp);
    const want = String(args.page ?? '').trim().toLowerCase();
    const pages = [];
    for (const [page, x] of Object.entries(ALLOWED_SETTINGS)) {
      if (want && page.toLowerCase() !== want) continue;
      const options = [];
      for (const f of schema.fields.filter((ff) => ff.name.startsWith(`${page}.`))) {
        if (settingVerdict(f.name, f).ok) options.push(settingInfo(f, currentValue(theApp, f.name)));
      }
      if (options.length) pages.push({ page, about: x.why, options });
    }
    if (want && !pages.length) throw new Error(`"${args.page}" is not a page the AI may change. The pages are: ${Object.keys(ALLOWED_SETTINGS).join(', ')}.`);
    return { pages, note: 'Change one with set_setting {name, value}. Colours themselves are changed with set_colors.' };
  },
});

addAITool({
  name: 'get_setting',
  description: 'Reads one option of the Options dialog by its name (Page.Field, like Display.ShowHidden or Font.PanelSize): its label, type, current value '
    + 'and the allowed range or choices. Only the options list_settings lists can be read.',
  schema: { type: 'object', properties: { name: { type: 'string', description: 'The option, like Colors.DarkMode.' } }, required: ['name'] },
  run: async (args) => {
    const f = await allowedField(args.name);
    await loadValues(theApp);
    return settingInfo(f, currentValue(theApp, f.name));
  },
});

addAITool({
  name: 'set_setting',
  description: 'Changes one option of the Options dialog, as the user would there; the change shows at once and is saved. name is Page.Field (like '
    + 'Colors.DarkMode, Display.SortDirsByName, Font.PanelSize); value is true/false for a yes/no option, a whole number within the range '
    + 'for a number, the number or the label of a choice (Colors.DarkMode: 0 never, 1 always, 2 follow the system), or a text where allowed (font names, '
    + 'the date format). The value is checked first; a bad one is refused with the reason. Answers the old and the new value, so the change can be put '
    + 'back with another set_setting. Only the options list_settings lists can be changed.',
  schema: {
    type: 'object',
    properties: {
      name: { type: 'string', description: 'The option, like Display.ShowHidden.' },
      value: { description: 'The new value: true/false, a number, a choice\'s number or label, or a text.' },
    },
    required: ['name', 'value'],
  },
  run: async (args) => {
    const f = await allowedField(args.name);
    const text = settingValue(f, args.value);
    await loadValues(theApp);
    const before = currentValue(theApp, f.name);
    const old = settingInfo(f, before);
    if (before === text) return { name: f.name, label: f.label, unchanged: true, value: old.value, ...(old.valueLabel ? { valueLabel: old.valueLabel } : {}) };
    await applyOption(f.name, text);
    const now = settingInfo(f, currentValue(theApp, f.name));
    const out = { name: f.name, label: f.label, old: old.value, new: now.value };
    if (old.valueLabel) out.oldLabel = old.valueLabel;
    if (now.valueLabel) out.newLabel = now.valueLabel;
    if (f.name === 'Colors.DarkMode' && (theApp.startTheme === 'light' || theApp.startTheme === 'dark')) {
      out.note = `mormiCommander was started with the ${theApp.startTheme} theme fixed, so dark mode follows this option after a restart.`;
    }
    return out;
  },
});

// ---- colours ----

const KEY_TEXT = Object.fromEntries(COLOR_KEYS);
const which = (v) => {
  const s = String(v ?? 'current').trim().toLowerCase();
  if (s === 'light' || s === 'dark') return [s];
  if (s === 'both') return ['light', 'dark'];
  if (s === '' || s === 'current') return [isDark() ? 'dark' : 'light'];
  throw new Error('scheme must be "current" (the one shown now), "light", "dark" or "both".');
};

async function colors() {
  const c = await go.Configuration.Colors();
  if (!c?.light || !c?.dark) throw new Error('The colours could not be read.');
  return c;
}

async function saveColors(light, dark) {
  await go.Configuration.SetColors(light, dark);
  await redraw([]);
}

addAITool({
  name: 'get_colors',
  description: 'Reads mormiCommander\'s colour schemes, as the Colours page of the Options dialog shows them: the light and the dark scheme (the app shows one '
    + 'of them, by the dark mode option), each with its colours (fore: text, back: background, back2: every second line, mark: selected files, cursor, '
    + 'cursorText, inactiveCursor, inactiveMark, inactiveBack) as #rrggbb ("" means the theme\'s own colour) and its colours by file type (filters: '
    + 'a mask like "*.jpg *.png" and a colour). Also tells which scheme is shown now, the built-in presets, and whether restore_colors has something to put back.',
  schema: { type: 'object', properties: {} },
  run: async () => {
    const c = await colors();
    return {
      showing: isDark() ? 'dark' : 'light',
      light: schemeOut(c.light), dark: schemeOut(c.dark),
      colorNames: KEY_TEXT,
      themeColors: { light: { text: THEME.light.text, background: THEME.light.bg }, dark: { text: THEME.dark.text, background: THEME.dark.bg } },
      presets: (c.builtin || []).map((s) => s.name),
      canRestore: colorHistory.length > 0,
      note: `Dark mode is the option Colors.DarkMode (set_setting). Text colours must keep a contrast of at least ${MIN_CONTRAST} with their background.`,
    };
  },
});

addAITool({
  name: 'set_colors',
  description: 'Changes the colours of a scheme, as the Colours page of the Options dialog does; the change shows at once and is saved. scheme: "current" '
    + '(default, the one shown now), "light", "dark" or "both". colors: the colours to change by name (fore, back, back2, mark, cursor, cursorText, '
    + 'inactiveCursor, inactiveMark, inactiveBack) as "#rrggbb", or "" for the theme\'s colour; colours not named stay. filters: a new list of colours '
    + 'by file type [{mask, color}] that replaces the old one. preset: the name of a built-in preset whose colours are taken first. A change that '
    + `leaves text with a contrast below ${MIN_CONTRAST} on its background (names, selected files, the cursor, a file type's colour) is refused with the `
    + 'pairs that fail. The colours before the change are kept: restore_colors puts them back. For a random scheme, pick colours that keep the text readable.',
  schema: {
    type: 'object',
    properties: {
      scheme: { type: 'string', enum: ['current', 'light', 'dark', 'both'], description: 'Which scheme to change (default "current").' },
      colors: { type: 'object', description: 'Colours by name, like {"fore": "#e6e6e6", "back": "#1d1f21", "mark": "#ffcc00"}.', additionalProperties: { type: 'string' } },
      filters: { type: 'array', description: 'Colours by file type, replacing the list: [{"mask": "*.jpg *.png", "color": "#4caf50"}].', items: { type: 'object', properties: { mask: { type: 'string' }, color: { type: 'string' } } } },
      preset: { type: 'string', description: 'A built-in preset (get_colors lists them), applied before colors.' },
    },
  },
  run: async (args) => {
    if (args.colors == null && args.filters == null && !args.preset) throw new Error('Give colors, filters or a preset to change.');
    const targets = which(args.scheme);
    const c = await colors();
    const next = { light: c.light, dark: c.dark };
    for (const w of targets) {
      const s = newScheme(c[w], { colors: args.colors, filters: args.filters, preset: args.preset }, c.builtin || []);
      const problems = schemeProblems(c[w], s, w);
      if (problems.length) throw new Error(problemText(problems, w));
      next[w] = s;
    }
    if (JSON.stringify(next) === JSON.stringify({ light: c.light, dark: c.dark })) return 'The colours are already like that; nothing changed.';
    colorHistory.push({ light: c.light, dark: c.dark });
    if (colorHistory.length > HISTORY_MAX) colorHistory.splice(1, 1); // (the first entry stays: it is how the session began)
    await saveColors(next.light, next.dark);
    const shown = isDark() ? 'dark' : 'light';
    return {
      changed: targets,
      ...Object.fromEntries(targets.map((w) => [w, schemeOut(next[w])])),
      showing: shown,
      note: `${targets.includes(shown) ? '' : `The ${shown} scheme is shown now, so the change shows when the theme changes. `}restore_colors puts the previous colours back.`,
    };
  },
});

addAITool({
  name: 'restore_colors',
  description: 'Puts back the colours as they were before the AI changed them in this session: all of them (default), or only the last set_colors change '
    + 'with {"last": true}. Answers what was restored; when the AI changed no colours, says so.',
  schema: { type: 'object', properties: { last: { type: 'boolean', description: 'Undo only the last colour change (default: every change of this session).' } } },
  run: async (args) => {
    if (!colorHistory.length) return 'The AI has not changed any colours in this session; there is nothing to put back.';
    const all = !(args.last === true || args.last === 'true');
    const snap = all ? colorHistory[0] : colorHistory[colorHistory.length - 1];
    await saveColors(snap.light, snap.dark);
    if (all) colorHistory.length = 0; else colorHistory.pop();
    return all ? 'The colours are back as they were before the AI changed them.' : `The last colour change was undone${colorHistory.length ? `; ${colorHistory.length} earlier ${colorHistory.length === 1 ? 'change remains' : 'changes remain'}` : ''}.`;
  },
});

// (for the tests: forget the colour history between them)
export function _resetColorHistory() { colorHistory.length = 0; }
