// The pure parts of the step 3 AI tools (features/aitools-more-lib.js, issue #146): values for options checked
// against type, range and choices; the colour checks (contrast, a text colour equal to its background, a pair that
// was already poor not blocking an unrelated change); the scheme a set_colors call makes; the archive name of
// pack_files; the Ask dialog's context, log lines, prompt history and provider lines.
import { expect, test } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  settingValue, settingInfo, hexColor, contrast, blend, effectiveColors, schemeProblems, problemText, newScheme, archiveName, askContext,
  describeAnyTool, pushHistory, historyStep, providerLines, COLOR_KEYS, MIN_CONTRAST,
} from '../src/features/aitools-more-lib.js';

const bool = { name: 'Display.ShowHidden', type: 'bool', label: 'Show hidden files' };
const size = { name: 'Font.PanelSize', type: 'int', label: 'File list font size', min: 6, max: 72 };
const dark = { name: 'Colors.DarkMode', type: 'int', label: 'Dark mode', min: 0, max: 2, options: [{ value: 0, label: 'Never' }, { value: 1, label: 'Always' }, { value: 2, label: 'Follow system' }] };
const font = { name: 'Font.PanelFont', type: 'string', label: 'File list font' };
const date = { name: 'Display.DateFormat', type: 'string', label: 'Date format' };

test('settingValue: yes/no, numbers within the range, choices by number or label', () => {
  for (const v of [true, 'true', 'on', '1', 1, 'yes']) expect(settingValue(bool, v)).toBe('1');
  for (const v of [false, 'false', 'off', '0', 0, 'no']) expect(settingValue(bool, v)).toBe('0');
  expect(() => settingValue(bool, 'maybe')).toThrow(/true or false/);
  expect(settingValue(size, 14)).toBe('14');
  expect(settingValue(size, '20')).toBe('20');
  expect(() => settingValue(size, 5)).toThrow(/at least 6/);
  expect(() => settingValue(size, 100)).toThrow(/at most 72/);
  expect(() => settingValue(size, 13.5)).toThrow(/whole number/);
  expect(() => settingValue(size, 'big')).toThrow(/whole number/);
  expect(settingValue(dark, 1)).toBe('1');
  expect(settingValue(dark, 'always')).toBe('1');
  expect(settingValue(dark, 'Follow system')).toBe('2');
  expect(() => settingValue(dark, 3)).toThrow(/one of: 0 \(Never\), 1 \(Always\), 2 \(Follow system\)/);
  expect(() => settingValue(size, undefined)).toThrow(/Give the new value/);
});

test('settingValue: texts only where allowed, and only in their pattern', () => {
  expect(settingValue(font, 'JetBrains Mono')).toBe('JetBrains Mono');
  expect(settingValue(font, '')).toBe('');
  expect(() => settingValue(font, '/usr/bin/evil')).toThrow(/font name/);
  expect(() => settingValue(font, 'x; rm -rf ~')).toThrow(/font name/);
  expect(settingValue(date, 'dd.MM.yyyy')).toBe('dd.MM.yyyy');
  expect(() => settingValue(date, '%Y $(id)')).toThrow(/date pattern/);
  expect(() => settingValue({ name: 'ViewerEditor.Editor', type: 'string' }, 'vim')).toThrow(/may not change/);
  expect(settingValue({ name: 'Colors.SelectionBgColor', type: 'string' }, '#334455')).toBe('#334455');
  expect(() => settingValue({ name: 'Colors.SelectionBgColor', type: 'string' }, 'url(x)')).toThrow(/#rrggbb/);
});

test('settingInfo: typed value, choice label, range', () => {
  expect(settingInfo(bool, '1')).toEqual({ name: 'Display.ShowHidden', label: 'Show hidden files', type: 'boolean', value: true });
  expect(settingInfo(size, '13')).toMatchObject({ type: 'integer', value: 13, min: 6, max: 72 });
  expect(settingInfo(dark, '2')).toMatchObject({ value: 2, valueLabel: 'Follow system', choices: dark.options });
});

test('colours: hex forms, contrast, blending', () => {
  expect(hexColor('#ABC')).toBe('#aabbcc');
  expect(hexColor('1e2a3b')).toBe('#1e2a3b');
  expect(hexColor('')).toBe('');
  expect(() => hexColor('red')).toThrow(/#rrggbb/);
  expect(() => hexColor('rgb(1,2,3)')).toThrow(/#rrggbb/);
  expect(contrast('#000000', '#ffffff')).toBeCloseTo(21, 0);
  expect(contrast('#123456', '#123456')).toBe(1);
  expect(blend('#ffffff', '#000000', 0.5)).toBe('#808080');
  expect(effectiveColors({}, 'light')).toMatchObject({ fore: '#1c2533', back: '#f4f6fa', cursorText: '#1c2533' });
});

test('the colour keys are the Options dialog\'s', () => {
  const src = readFileSync(resolve(__dirname, '../src/features/config/options.js'), 'utf8');
  const block = src.slice(src.indexOf('const COLOR_KEYS = ['), src.indexOf('];', src.indexOf('const COLOR_KEYS = [')));
  expect([...block.matchAll(/\['(\w+)', '([^']+)'\]/g)].map((m) => [m[1], m[2]])).toEqual(COLOR_KEYS);
});

const scheme = (colors = {}, filters = []) => ({ name: 'Colors', colors: { fore: '', back: '', back2: '', mark: '', cursor: '', cursorText: '', inactiveCursor: '', inactiveMark: '', inactiveBack: '', ...colors }, inverseCursor: false, inverseSelection: false, filters });

test('schemeProblems: text equal to its background, or too faint, is a problem; a readable scheme is not', () => {
  const before = scheme();
  const same = schemeProblems(before, scheme({ fore: '#336699', back: '#336699' }), 'dark');
  expect(same.some((p) => p.what === 'the file names on the background' && p.ratio === 1)).toBe(true);
  expect(problemText(same, 'dark')).toMatch(/not changed.*at least 3/);
  expect(schemeProblems(before, scheme({ fore: '#777777', back: '#666666' }), 'dark').length).toBeGreaterThan(0);
  expect(schemeProblems(before, scheme({ fore: '#e6e6e6', back: '#1d1f21', mark: '#ffcc00', cursor: '#3a5f8a', cursorText: '#ffffff' }), 'dark')).toEqual([]);
  // the theme's own text on a new background counts too
  expect(schemeProblems(before, scheme({ back: '#e8edf0' }), 'dark').length).toBeGreaterThan(0);
  // a colour by file type that vanishes on the background
  expect(schemeProblems(before, scheme({}, [{ mask: '*.jpg', color: '#1c2636' }]), 'dark')[0].what).toBe('files matching *.jpg');
  expect(MIN_CONTRAST).toBe(3);
});

test('schemeProblems: a pair that was already poor does not block a change elsewhere', () => {
  const poor = scheme({ inactiveMark: '#222222', inactiveBack: '#202020' });
  expect(schemeProblems(scheme(), poor, 'dark').length).toBeGreaterThan(0);
  expect(schemeProblems(poor, { ...poor, colors: { ...poor.colors, mark: '#ffcc00' } }, 'dark')).toEqual([]);
});

test('newScheme: colours by key, "" for the theme, filters replaced, presets, bad input refused', () => {
  const old = scheme({ fore: '#111111' }, [{ mask: '*.txt', color: '#ff0000' }]);
  const s = newScheme(old, { colors: { back: '#FFF', fore: '' } });
  expect(s.colors).toMatchObject({ back: '#ffffff', fore: '' });
  expect(s.filters).toEqual(old.filters);
  expect(old.colors.fore).toBe('#111111'); // (the old scheme is left as it was)
  expect(newScheme(old, { filters: [{ mask: '*.jpg *.png', color: '#4caf50' }] }).filters).toEqual([{ mask: '*.jpg *.png', color: '#4caf50' }]);
  const builtin = [{ name: 'Classic blue', colors: { fore: '#00ffff', back: '#000080' } }];
  expect(newScheme(old, { preset: 'classic blue' }, builtin).colors).toMatchObject({ fore: '#00ffff', back: '#000080', mark: '' });
  expect(() => newScheme(old, { preset: 'Nope' }, builtin)).toThrow(/presets are: Classic blue/);
  expect(() => newScheme(old, { colors: { text: '#fff' } })).toThrow(/"text" is not a colour/);
  expect(() => newScheme(old, { colors: { fore: 'blue' } })).toThrow(/#rrggbb/);
  expect(() => newScheme(old, { filters: [{ mask: '', color: '#fff' }] })).toThrow(/needs a mask/);
});

const PACKERS = [
  { name: 'zip', ext: 'zip', available: true, label: 'ZIP' }, { name: 'tar', ext: 'tar', available: true, label: 'TAR' },
  { name: 'tar.gz', ext: 'tar.gz', available: true, label: 'TGZ' }, { name: 'gz', ext: 'gz', single: true, available: true, label: 'GZ' },
  { name: '7z', ext: '7z', available: false, label: '7Z', note: 'Packing to 7z needs 7-Zip installed.' },
];

test('archiveName: the extension picks the packer, format adds it, paths and conflicts are refused', () => {
  expect(archiveName('photos.zip', '', PACKERS)).toMatchObject({ name: 'photos.zip', packer: { name: 'zip' } });
  expect(archiveName('backup.tar.gz', undefined, PACKERS).packer.name).toBe('tar.gz'); // (not "gz")
  expect(archiveName('notes', 'tar.gz', PACKERS).name).toBe('notes.tar.gz');
  expect(archiveName('notes', 'tgz', PACKERS).name).toBe('notes.tar.gz');
  expect(archiveName('notes', null, PACKERS).name).toBe('notes.zip');
  expect(() => archiveName('a.zip', 'tar', PACKERS)).toThrow(/make them agree/);
  expect(() => archiveName('../x.zip', '', PACKERS)).toThrow(/plain file name/);
  expect(() => archiveName('/tmp/x.zip', '', PACKERS)).toThrow(/plain file name/);
  expect(() => archiveName('C:x.zip', '', PACKERS)).toThrow(/plain file name/);
  expect(() => archiveName('*.zip', '', PACKERS)).toThrow(/plain file name/);
  expect(() => archiveName('', '', PACKERS)).toThrow(/Give the name/);
  expect(() => archiveName('a.7z', '', PACKERS)).toThrow(/7-Zip/);
  expect(() => archiveName('a', 'rar', PACKERS)).toThrow(/no packer called "rar"/);
});

test('askContext: both panels, the platform and the date, and that nothing destructive exists', () => {
  const t = askContext({
    active: { side: 'left', path: '/Users/me/Code', cursor: 'main.go', selected: 3 },
    other: { side: 'right', path: '/Users/me/Documents', cursor: '', selected: 1 },
    platform: 'darwin', home: '/Users/me',
  }, new Date(2026, 10, 14));
  expect(t).toMatch(/The active panel \(left\) shows \/Users\/me\/Code; the cursor is on main\.go; 3 entries are selected\./);
  expect(t).toMatch(/The other panel \(right\) shows \/Users\/me\/Documents; 1 entry is selected\./);
  expect(t).toMatch(/Platform: darwin\. Home folder: \/Users\/me\. Today is Saturday, 2026-11-14\./);
  expect(t).toMatch(/None of the tools deletes, moves, renames or overwrites files, runs programs or a shell/);
});

test('describeAnyTool: readable lines for the new tools, step 1\'s for the old', () => {
  expect(describeAnyTool('mcp__mormi__run_command', { name: 'cm_Exchange' }, '', () => 'Swap panels')).toBe('Running Swap panels (cm_Exchange)…');
  expect(describeAnyTool('set_setting', { name: 'Colors.DarkMode', value: 1 })).toBe('Setting Colors.DarkMode to 1…');
  expect(describeAnyTool('set_panel_paths', { left: '/Users/me/Downloads' }, '/Users/me')).toBe('Opening left: ~/Downloads…');
  expect(describeAnyTool('pack_files', { archive: 'a.zip' })).toBe('Packing into a.zip…');
  expect(describeAnyTool('restore_colors', {})).toBe('Putting the colours back…');
  expect(describeAnyTool('get_state', {})).toBe('Looking at the panels…');
});

test('prompt history: newest last, no repeats, capped; Up and Down walk it back to the draft', () => {
  let h = pushHistory([], 'a');
  h = pushHistory(h, 'b');
  h = pushHistory(h, 'a');
  expect(h).toEqual(['b', 'a']);
  expect(pushHistory(['x', 'y', 'z'], 'w', 3)).toEqual(['y', 'z', 'w']);
  expect(pushHistory(h, '   ')).toEqual(['b', 'a']);
  expect(historyStep(['b', 'a'], 2, -1, 'draft')).toEqual({ at: 1, text: 'a' });
  expect(historyStep(['b', 'a'], 1, -1)).toEqual({ at: 0, text: 'b' });
  expect(historyStep(['b', 'a'], 0, -1)).toEqual({ at: 0, text: 'b' });
  expect(historyStep(['b', 'a'], 1, 1, 'draft')).toEqual({ at: 2, text: 'draft' });
});

test('providerLines: Claude Code as in step 1, other providers in the same plain way', () => {
  const r = providerLines({ claude: { available: true, path: '/bin/claude', version: '2.1.0 (Claude Code)' }, openrouter: { available: false, error: 'No API key is set.' } });
  expect(r.ok).toBe(true);
  expect(r.lines.map((l) => l.text)).toEqual(['Claude Code 2.1.0 found at /bin/claude', 'OpenRouter is not set up: No API key is set.']);
  expect(providerLines({ openrouter: { available: true, model: 'some/model' } }).lines[0].text).toBe('OpenRouter (some/model) is ready.');
  expect(providerLines(null)).toEqual({ ok: false, lines: [{ ok: false, text: 'AI is not available in this build.' }] });
});
