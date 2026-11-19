// @vitest-environment happy-dom
// Translations (issue #113): t() gives the English text back unchanged without a language file (so the default menus
// are exactly what they were), falls back to English for texts a file lacks, fills in {placeholders}; the built-in
// Hungarian file (lang/hu.lng) covers the converted slice: the whole menu bar, the function key bar and every literal
// t('…') text of the converted files. `npm run lang:template` runs this file with MC_WRITE_LANG_TEMPLATE=1 to write
// lang/template.lng from the same collection.
import { afterEach, beforeAll, expect, test, vi } from 'vitest';
import { readFileSync, readdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../../docs/keyboard.md?raw', () => ({ default: '' }));

const root = resolve(__dirname, '../..');
let i18n, menus, fkeys;
beforeAll(async () => {
  vi.useFakeTimers();
  globalThis.requestAnimationFrame = () => 0;
  for (const load of Object.values(import.meta.glob('../src/features/*.js'))) await load();
  i18n = await import('../src/core/i18n.js');
  menus = await import('../src/core/menus.js');
  fkeys = await import('../src/core/fkeys.js');
  optionsMod = await import('../src/features/config/options.js');
  jobsMod = await import('../src/core/jobs.js');
  document.body.innerHTML = '<div id="panels"><div id="left"></div><div id="right"></div></div><div id="fkeys"></div>';
  // panels that answer every question with "nothing", so the menu's enabled/checked tests can run
  const { app } = await import('../src/core/app.js');
  const panel = (side) => new Proxy({ side, path: '/tmp', entries: [], selected: new Set(), cur: null, mode: 'files', view: 'full' },
    { get: (o, k) => (k in o ? o[k] : typeof k === 'string' ? () => [] : undefined) });
  const [l, r] = [panel('left'), panel('right')];
  for (const [k, v] of Object.entries({ panels: [l, r], cur: l, other: r, left: l, right: r })) {
    try { Object.defineProperty(app, k, { value: v, configurable: true, writable: true }); } catch { /* (a fixed property) */ }
  }
  vi.useRealTimers();
});
afterEach(() => i18n.setTranslations(null));

// readLng reads a .lng file the way Go does (feature_lang.go), without sections: English → translation.
function readLng(text) {
  const out = {};
  for (let line of text.split(/\r?\n/)) {
    line = line.trim();
    if (!line || line[0] === ';' || line[0] === '#' || (line[0] === '[' && line.endsWith(']'))) continue; // (a "[hu]" section line, as in Go)
    const m = /^((?:[^=\\]|\\=|\\(?!=))*)=(.*)$/.exec(line);
    const un = (x) => x.replace(/\\n/g, '\n');
    if (m && m[2].trim()) out[un(m[1].replace(/\\=/g, '=')).trim()] = un(m[2]).trim();
  }
  return out;
}

// The files whose literal t('…') texts belong to the converted slice.
const SLICE_FILES = ['src/core/dialogs.js', 'src/core/panel.js', 'src/features/fileops.js', 'src/features/config/options.js',
  'src/core/jobs.js', 'src/features/views.js'];

// The Options dialog's pages and options come from Go; options-texts.json lists their words (feature_lang_test.go keeps
// it in step with internal/config).
const OPTION_TEXTS = JSON.parse(readFileSync(resolve(__dirname, 'options-texts.json'), 'utf8'));

// Texts of the slice that reach t() through a variable (dialog titles and buttons, choices), so no literal finds them.
const EXTRA = ['OK', 'Cancel', 'Yes', 'No', 'Delete', 'Wipe', 'Delete permanently', 'Copy', 'Move', 'Make the folder?', 'Make it',
  'Trash', 'Recycle Bin', 'trash', 'Move to Trash', 'Move to Recycle Bin', 'Move to trash', 'file', 'files', 'folder', 'folders',
  'Ask', 'Overwrite all', 'Skip all', 'Overwrite all older', 'Auto-rename copied files', 'Auto-rename target files', 'Copy all larger',
  'Copy all smaller', 'Verify after copying', 'Read every copy back and compare it with the original', 'Keep file dates', 'Copy attributes',
  'Copy permissions', 'Copy owner and group (needs administrator rights)', 'Copy alternate data streams',
  'Copy extended attributes and resource forks', 'Copy extended attributes', 'Copy links as links (off: copy what they point to)',
  'Big file mode (large buffer, no system cache)', 'Don\'t stop on files that can\'t be read (list them at the end)',
  'Move files that are overwritten to the trash', 'The old file or link goes to the trash before the new one takes its place (local folders)',
  'Overwrite', 'Skip', 'Rename', 'Retry', 'Close', 'Apply', 'New folder', 'Show in Finder', 'Show in Explorer', 'Open in the file manager'];

// The texts of the Options dialog's own pages, notes and lists (options.js), which reach t() through a variable.
let optionsMod;
function optionsDialogTexts() {
  const m = optionsMod;
  return [...Object.values(m.EXTRA_PAGES).map((x) => x.title), ...Object.values(m.NOTES), ...m.COLOR_KEYS.map(([, l]) => l), ...m.ICONIC.map((o) => o.label),
    'given on the command line (-I=)', 'next to the program (portable)', 'in your settings folder', 'Import the folder hotlist',
    'Add the imported hotlist to yours (in a submenu), or replace yours with it?', 'Add', 'Replace', 'Settings imported', 'Restore settings?',
    'Restore', 'Save', 'Open', 'Import Total Commander settings', 'Back up settings', 'Restore settings from a backup'];
}

// The words of the job windows (core/jobs.js) that reach t() through a variable: kinds, states, titles, buttons.
let jobsMod;
function jobTexts() {
  const w = jobsMod.JOB_WORDS;
  return [...Object.values(w.VERB), ...Object.values(w.KIND), ...Object.values(w.STATE), 'Job', 'Working', 'Moving to the Trash', 'Moving to the Recycle Bin',
    'Converting archives', 'error', 'errors', 'job', 'jobs', 'item', 'items', 'line', 'lines', 'Read-only file', 'Hidden or system file', 'Problem',
    'Not enough space', 'Folder is not empty', 'Overwrite', 'Overwrite all', 'Skip', 'Skip all', 'Start anyway', 'All', 'Overwrite all older', 'Rename…',
    'Compare', 'Auto-rename', 'Auto-rename all', 'Rename target', 'Rename all targets', 'Copy all larger', 'Copy all smaller', 'Append', 'Resume', 'Continue',
    'Copy under another name', 'Move under another name', 'New name', 'Type a name.', 'A name can\'t contain / or \\.', 'That name is taken: type another one.',
    'Fewer options', 'Name', 'Ext', 'Size', 'Date', 'Attr'];
}

// app.flash() shows its message translated (#113), so the plain-text messages of every source file belong to the slice.
function flashTexts() {
  const out = [];
  const walk = (dir) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const f = resolve(dir, e.name);
      if (e.isDirectory()) walk(f);
      else if (f.endsWith('.js')) out.push(...i18n.literalTexts(readFileSync(f, 'utf8').replace(/\bflash\(/g, 't(')));
    }
  };
  walk(resolve(__dirname, '../src'));
  return out;
}

// Find files, Multi-Rename and Synchronize folders are translated with translateTree (their HTML) and through the
// dialogs, menus and messages they use, so their words are read from the source: the text of their HTML templates and
// their single-quoted strings that read like words for people (key names and the like left out).
// Part 3 (#113) added the Lister (in-page and window), the text editor, the tools (properties, checksums, split and
// combine, attributes, links, occupied space), the server connections and their bar, the AI dialogs, the tabs and
// hotlist dialogs, the command palette and Compare by content.
const TREE_FILES = ['src/features/find.js', 'src/features/multirename.js', 'src/features/syncdirs.js',
  'src/features/viewer.js', 'src/features/viewer/view.js', 'src/viewer-window.js', 'src/features/textedit.js',
  'src/features/tools/common.js', 'src/features/tools/props.js', 'src/features/tools/checksums.js', 'src/features/tools/split.js',
  'src/features/tools/attrs.js', 'src/features/tools/links.js', 'src/features/tools/space.js', 'src/features/tools/xattr.js',
  'src/features/net.js', 'src/features/net-attrs.js', 'src/features/network.js', 'src/features/ai-settings.js', 'src/features/ask.js',
  'src/features/tabs.js', 'src/features/hotlist.js', 'src/features/palette.js', 'src/features/comparefiles.js'];
const NOT_WORDS = new Set(['[data-b]', '[data-r]', 'Playing this kind', 'Esc', 'Ins', 'Num+', 'Num-', 'Space', 'Down', 'Apps', 'Enter', '^Aa', 'GB', 'KB', 'MB', 'TB', 'ANSI', 'UTF-8', 'Hex', 'RegEx']);
function uiStrings(src) {
  const out = [];
  for (const m of src.matchAll(/'((?:[^'\\\n]|\\.)*)'/g)) {
    const s = m[1].replace(/\\(.)/g, (x, c) => (c === 'n' ? '\n' : c));
    if (/^[A-Z[(…]/.test(s) && /[a-z]{2}/.test(s) && !/^[A-Z][a-z]+[A-Z]|^(Arrow|Page|Ctrl|Cmd|Alt|Shift|Escape|Enter|Tab|Home|End|Insert|Delete$|Backspace)|^[A-Za-z]+\.[A-Za-z]+$|[${}]|<\/|^\[data-|\/\/ |' \+/.test(s)) out.push(s);
  }
  return out;
}
// The words of tn(n, 'word', 'words') calls (a number and a word, translated), in every source file.
function pluralTexts() {
  const out = [];
  const walk = (dir) => {
    for (const e of readdirSync(dir, { withFileTypes: true })) {
      const f = resolve(dir, e.name);
      if (e.isDirectory()) walk(f);
      else if (f.endsWith('.js')) {
        for (const m of readFileSync(f, 'utf8').matchAll(/\btn\([^,()]+(?:\([^()]*\))?[^,()]*,\s*'([^']+)'(?:,\s*'([^']+)')?/g)) out.push(m[1], m[2] || `${m[1]}s`);
      }
    }
  };
  walk(resolve(__dirname, '../src'));
  return out;
}

// Texts of part 3 that reach t() through a variable: the Lister's modes, the link kinds and checksum states of the
// tools, the transfer modes, the keyboard presets, the kinds of a server item.
const DYNAMIC = ['Text', 'Binary', 'Hex', 'Image/Multimedia', 'HTML/Markdown', 'Table', 'File', 'Edit', 'Options', 'Image', 'Encoding',
  'Symbolic link', 'Junction', 'Hard link', 'Shortcut file', 'Web location file', 'Desktop link file', 'Windows shortcut', 'Finder alias',
  'Get Info in Finder', 'Windows properties…', 'File manager properties…', 'OK', 'FAILED', 'Not found', 'Not readable',
  'Automatic', 'ASCII', 'Total Commander', 'Mac-native', 'Marta-like', 'Vim', 'Folder',
  'Symbolic link to a folder', 'Symbolic link to a file', 'Symbolic link to a folder (target missing)', 'Symbolic link to a file (target missing)',
  'Left', 'Right', 'Left, shown', 'Right, shown', 'Command', 'Tab', 'Hotlist'];

// The Go error messages (frontend/test/error-texts.json, kept in step by feature_lang_test.go): shown translated by
// tText (core/i18n.js) where an error reaches the user.
const ERROR_TEXTS = JSON.parse(readFileSync(resolve(__dirname, 'error-texts.json'), 'utf8'));

function treeTexts() {
  const out = ['lowercase', 'UPPERCASE'];
  for (const f of TREE_FILES) {
    const src = readFileSync(resolve(__dirname, '..', f), 'utf8');
    out.push(...i18n.htmlTexts(src), ...uiStrings(src));
  }
  return out.filter((x) => !NOT_WORDS.has(x) && !/\/\/ |' \+|\('/.test(x)); // (not code that happens to sit between > and <)
}

// sliceTexts: what t() is asked while the menu bar is built (the texts of every menu, also the items features add),
// the function key bar's words and the literal texts of the converted files.
function sliceTexts() {
  i18n.startCollecting();
  menus.menuSpec();
  for (const s of Object.values(fkeys.SHORT)) i18n.t(s);
  const got = new Set([...i18n.stopCollecting(), ...EXTRA, ...menus.SYSTEM_MENU_WORDS, ...OPTION_TEXTS, ...optionsDialogTexts(), ...jobTexts(), ...flashTexts(), ...treeTexts(), ...pluralTexts(), ...DYNAMIC, ...ERROR_TEXTS]);
  for (const f of SLICE_FILES) for (const s of i18n.literalTexts(readFileSync(resolve(__dirname, '..', f), 'utf8'))) got.add(s);
  return [...got].filter((s) => /[A-Za-z]/.test(s) && !/^cm_|^em_/.test(s));
}

test('without a language file every text stays the English one, so the menus are unchanged', () => {
  const before = JSON.stringify(menus.menuSpec());
  expect(i18n.translating()).toBe(false);
  expect(i18n.t('Copy…')).toBe('Copy…');
  expect(i18n.t('Copy {what} to:', { what: '2 files' })).toBe('Copy 2 files to:');
  i18n.setTranslations({});
  expect(i18n.translating()).toBe(false);
  i18n.setTranslations({ Files: '' }); // (an empty translation, as in the template, counts as none)
  expect(i18n.translating()).toBe(false);
  expect(JSON.stringify(menus.menuSpec())).toBe(before);
});

test('a text the file lacks falls back to English; a trailing … and placeholders are kept', () => {
  i18n.setTranslations({ Files: 'Fájlok', Pack: 'Tömörítés', 'Copy {what} to:': '{what} másolása ide:' });
  expect(i18n.t('Files')).toBe('Fájlok');
  expect(i18n.t('Mark')).toBe('Mark');
  expect(i18n.t('Pack…')).toBe('Tömörítés…');
  expect(i18n.t('Copy {what} to:', { what: '3 fájl' })).toBe('3 fájl másolása ide:');
  expect(i18n.t('Move {what} to:', { what: 'x' })).toBe('Move x to:');
  const spec = menus.menuSpec();
  expect(spec[0].label).toBe('Fájlok');
  expect(spec[1].label).toBe('Mark');
});

test('literalTexts finds plain t() and tr() texts, not ones built with ${}', () => {
  expect(i18n.literalTexts("t('A b') + tr(\"C\") + t(`D`) + t(`E ${x}`) + get('no') + tt('It\\'s\\n')")).toEqual(['A b', 'C', 'D', "It's\n"]);
  expect(i18n.lngLine('Target = source', 'x')).toBe('Target \\= source=x');
  expect(readLng('Target \\= source=Cél = forrás\n; c\nA=\n[hu]\nB=b')).toEqual({ 'Target = source': 'Cél = forrás', B: 'b' });
});

test('the Hungarian file covers the converted slice and makes the menus Hungarian', () => {
  const hu = readLng(readFileSync(resolve(root, 'lang/hu.lng'), 'utf8'));
  const texts = sliceTexts();
  expect(texts.length).toBeGreaterThan(200);
  const missing = texts.filter((s) => hu[s] === undefined && !(s.endsWith('…') && hu[s.slice(0, -1)] !== undefined));
  if (process.env.MC_LANG_MISSING) writeFileSync(process.env.MC_LANG_MISSING, JSON.stringify(missing, null, 1));
  expect(missing).toEqual([]);
  i18n.setTranslations(hu);
  const spec = menus.menuSpec();
  expect(spec.map((m) => m.label).slice(0, 8)).toEqual(['Fájl', 'Kijelölés', 'Parancsok', 'Hálózat', 'Nézet', 'Beállítások', 'Indítás', 'Súgó']);
  expect(i18n.t('Copy {what} to:', { what: '3 fájl' })).toBe('3 fájl másolása ide:');
  // every placeholder of a text is in its translation
  for (const [en, tr] of Object.entries(hu)) {
    const ph = (s) => (s.match(/\{\w+\}/g) || []).sort().join();
    expect(ph(tr), en).toBe(ph(en));
  }
});

test('translateTree translates a dialog\'s own words in place, and nothing without a language file', () => {
  const box = document.createElement('div');
  const html = '<h2>Find files</h2><label> Search for: <input title="Choose a folder" placeholder="Search options" value="Find files"></label>'
    + '<span data-notr="">Find files</span><option>files only</option><b>Unknown words</b>';
  box.innerHTML = html;
  i18n.translateTree(box);
  expect(box.innerHTML).toBe(html);
  i18n.setTranslations({ 'Find files': 'Fájlkeresés', 'Search for:': 'Keresett név:', 'Choose a folder': 'Mappa választása', 'Search options': 'Keresés', 'files only': 'csak fájlok' });
  i18n.translateTree(box);
  expect(box.querySelector('h2').textContent).toBe('Fájlkeresés');
  expect(box.querySelector('label').firstChild.nodeValue).toBe(' Keresett név: '); // (the spaces around it stay)
  const input = box.querySelector('input');
  expect([input.title, input.placeholder, input.value]).toEqual(['Mappa választása', 'Keresés', 'Find files']); // (a value is data)
  expect(box.querySelector('[data-notr]').textContent).toBe('Find files');
  expect(box.querySelector('option').textContent).toBe('csak fájlok');
  expect(box.querySelector('b').textContent).toBe('Unknown words');
  expect(i18n.htmlTexts('x = `<label title="Tip">Search for:</label><b>${n} left</b><i> F3 &gt; view </i>`')).toEqual(['Search for:', 'F3 > view', 'Tip']);
});

test('a flash message is shown translated, and control mode still reads the English one', async () => {
  const { app } = await import('../src/core/app.js');
  document.body.insertAdjacentHTML('beforeend', '<div id="flash" hidden></div>');
  i18n.setTranslations({ 'The settings were saved.': 'A beállítások mentve.' });
  app.flash('The settings were saved.');
  expect(document.getElementById('flash').textContent).toBe('A beállítások mentve.');
  expect(app.lastMessage).toBe('The settings were saved.');
  i18n.setTranslations(null);
  app.flash('The settings were saved.');
  expect(document.getElementById('flash').textContent).toBe('The settings were saved.');
});

test('a translated menu carries its English name, so Go still finds Help; English menus carry none', () => {
  expect(menus.menuSpec().some((m) => 'key' in m)).toBe(false);
  i18n.setTranslations({ Help: 'Súgó' });
  const help = menus.menuSpec().find((m) => m.key === 'Help');
  expect(help?.label).toBe('Súgó');
  expect(menus.SYSTEM_MENU_WORDS).toContain('Select All');
});

test('the Hungarian file translates the Options labels from Go and the job words', () => {
  const hu = readLng(readFileSync(resolve(root, 'lang/hu.lng'), 'utf8'));
  i18n.setTranslations(hu);
  expect(i18n.t('Show hidden files')).toBe('Rejtett fájlok megjelenítése');
  expect(i18n.t('{done} of {files}', { done: 2, files: '5 fájl' })).toBe('2 / 5 fájl');
  expect(i18n.t('[N] Name')).toBe('[N] Név'); // (a line that starts with "[" but doesn't end with "]" is no section)
  for (const s of OPTION_TEXTS) if (/[a-z]{2}/.test(s)) expect(hu[s], s).toBeTruthy();
});

test('tm and tn keep the English text and give the translation; a Msg flash keeps the English one for control mode', async () => {
  const { app } = await import('../src/core/app.js');
  if (!document.getElementById('flash')) document.body.insertAdjacentHTML('beforeend', '<div id="flash" hidden></div>');
  const m = i18n.tm('Copied {what}.', { what: i18n.tn(3, 'name') });
  expect([m.en, m.text, String(m)]).toEqual(['Copied 3 names.', 'Copied 3 names.', 'Copied 3 names.']); // (no language file: English)
  const { plural } = await import('../src/core/format.js');
  expect(i18n.tn(1234, 'file').en).toBe(plural(1234, 'file')); // (the same text plural() gave before)
  i18n.setTranslations({ 'Copied {what}.': 'Másolva: {what}.', names: 'név', '{what} could not be read: {error}': '{what} nem olvasható: {error}' });
  const h = i18n.tm('Copied {what}.', { what: i18n.tn(3, 'name') });
  expect([h.en, h.text]).toEqual(['Copied 3 names.', 'Másolva: 3 név.']);
  app.flash(h);
  expect(document.getElementById('flash').textContent).toBe('Másolva: 3 név.');
  expect(app.lastMessage).toBe('Copied 3 names.');
  expect(i18n.t(h)).toBe('Másolva: 3 név.');
  const j = i18n.tjoin([h, 'x']);
  expect([j.en, j.text]).toEqual(['Copied 3 names.\nx', 'Másolva: 3 név.\nx']);
});

test('tText translates Go messages by their verbs, the most specific text first, and nothing without a language file', () => {
  expect(i18n.tText('Can\'t copy a.txt: it is not there.')).toBe('Can\'t copy a.txt: it is not there.');
  i18n.setTranslations({
    'Can\'t %s %s: %v': 'Nem sikerült (%s): %s: %v',
    'Can\'t %s %s: it is not there.': '%[2]s nem létezik (%[1]s).',
    'The folder %s is not there.': 'A(z) %s mappa nem létezik.',
    'Done.': 'Kész.',
  });
  expect(i18n.tText('Done.')).toBe('Kész.');
  expect(i18n.tText('The folder /tmp/x is not there.')).toBe('A(z) /tmp/x mappa nem létezik.');
  expect(i18n.tText('Can\'t copy a.txt: it is not there.')).toBe('a.txt nem létezik (copy).');
  expect(i18n.tText('Can\'t copy a.txt: disk full')).toBe('Nem sikerült (copy): a.txt: disk full');
  expect(i18n.tText('Something else happened.')).toBe('Something else happened.');
  const e = i18n.te('The folder /a is not there.');
  expect([e.en, e.text]).toEqual(['The folder /a is not there.', 'A(z) /a mappa nem létezik.']);
});

test('the Hungarian file translates every Go error message with the same verbs', () => {
  const hu = readLng(readFileSync(resolve(root, 'lang/hu.lng'), 'utf8'));
  const verbs = (s) => (s.match(/%(?:\[\d+\])?[-+# 0-9.]*[a-zA-Z]/g) || []).length;
  for (const en of ERROR_TEXTS) {
    expect(hu[en], en).toBeTruthy();
    expect(verbs(hu[en]), en).toBe(verbs(en));
  }
  i18n.setTranslations(hu);
  expect(i18n.tText('The folder /tmp/x is not there.')).toBe('A(z) /tmp/x mappa nem létezik.');
});

test.runIf(process.env.MC_WRITE_LANG_TEMPLATE)('write lang/template.lng', () => {
  const texts = sliceTexts().sort((a, b) => a.localeCompare(b));
  const head = ['; mormiCommander language file template (made by `npm run lang:template` in frontend/).',
    '; Write the translation after each "=". Lines left empty stay English. Keep {placeholders} as they are;',
    '; an "=" in the English text is written "\\=". Save as UTF-8, e.g. as xx.lng in the settings folder, and choose it',
    '; in Options > Language. A [xx] line starts a section used only when the interface language is xx.', ''];
  writeFileSync(resolve(root, 'lang/template.lng'), `${head.join('\n')}${texts.map((s) => i18n.lngLine(s.replace(/\n/g, '\\n'))).join('\n')}\n`);
});
