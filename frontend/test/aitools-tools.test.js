// @vitest-environment happy-dom
// The step 3 AI tools as the model calls them (core/aitools.js runTool, issue #146): run_command runs an allowed
// command and refuses the rest; set_setting checks the value, applies it the Options dialog's way and answers the old
// and the new value; set_colors refuses a scheme whose text would vanish, saves through Configuration.SetColors, and
// restore_colors puts the colours back; pack_files refuses an archive that exists and never moves the files. Go is a
// stand-in that keeps the options and colours in memory; the job runner is a recorder.
import { beforeEach, expect, test, vi } from 'vitest';

const calls = [];
const values = {};
let colors = null;
const existing = new Set();
const FIELDS = [
  { name: 'Font.PanelSize', pageId: 'Font', type: 'int', label: 'File list font size', min: 6, max: 72 },
  { name: 'Colors.DarkMode', pageId: 'Colors', type: 'int', label: 'Dark mode', min: 0, max: 2, options: [{ value: 0, label: 'Never' }, { value: 1, label: 'Always' }, { value: 2, label: 'Follow system' }] },
  { name: 'Display.SortNatural', pageId: 'Display', type: 'bool', label: 'Natural sorting (numbers by value)' },
  { name: 'Display.ShowSystem', pageId: 'Display', type: 'bool', label: 'Show system files', noEffect: true },
  { name: 'CopyDelete.UseTrash', pageId: 'CopyDelete', type: 'bool', label: 'Delete to trash' },
  { name: 'ViewerEditor.Editor', pageId: 'ViewerEditor', type: 'string', label: 'Editor for F4' },
];
const scheme = (name, c = {}) => ({ name, colors: { fore: '', back: '', back2: '', mark: '', cursor: '', cursorText: '', inactiveCursor: '', inactiveMark: '', inactiveBack: '', ...c }, inverseCursor: false, inverseSelection: false, filters: [{ mask: '*.txt', color: '' }] });
const answers = {
  'Configuration.Schema': () => ({ pages: [], fields: FIELDS }),
  'Configuration.Values': () => ({ ...values }),
  'Configuration.Apply': (ch) => { const changed = Object.keys(ch).filter((k) => values[k] !== ch[k]); Object.assign(values, ch); return changed; },
  'Configuration.Colors': () => JSON.parse(JSON.stringify(colors)),
  'Configuration.SetColors': (light, dark) => { colors = { ...colors, light: JSON.parse(JSON.stringify(light)), dark: JSON.parse(JSON.stringify(dark)) }; return null; },
  'VfsJobs.Packers': () => [{ name: 'zip', ext: 'zip', available: true, label: 'ZIP' }, { name: 'gz', ext: 'gz', single: true, available: true, label: 'GZ' }],
  'FS.Stat': (p) => {
    if (p === '/out' || p === '/src') return { name: p.slice(1), path: p, flags: 1 };
    if (existing.has(p)) return { name: p, path: p, flags: 0 };
    throw new Error(`${p}: no such file or folder`);
  },
};
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        calls.push({ k: `${obj}.${m}`, args });
        const f = answers[`${obj}.${m}`];
        try { return Promise.resolve(f ? f(...args) : null); } catch (e) { return Promise.reject(e); }
      },
    }),
  });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e).replace(/^Error:\s*/, ''), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
const store = {};
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => store[k] ?? d, setSetting: (k, v) => { store[k] = v; return Promise.resolve(); }, onSettingChange: () => () => {}, allSettings: () => store }));
const jobs = [];
vi.mock('../src/core/jobs.js', () => ({ runJob: (req) => { jobs.push(req); return Promise.resolve({ state: 'done', errors: [] }); }, jobBackends: [], holdProgress: () => {} }));

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);
const { app } = await import('../src/core/app.js');
const reg = await import('../src/core/registry.js');
const { runTool } = await import('../src/core/aitools.js');
await import('../src/features/aitools-commands.js');
const { _resetColorHistory } = await import('../src/features/aitools-settings.js');
await import('../src/features/aitools-pack.js');

const entry = (name, dir = false) => ({ name, dir });
const panel = (side, path, entries = [], sel = []) => ({
  side, path, entries, parent: '/', cur: entries[0] || null, selected: new Set(sel),
  selectedEntries() { return entries.filter((e) => this.selected.has(e.name)); },
  fullPath: (n) => `${path}/${n}`, go: async function go(p) { this.path = p; return true; }, focusName: () => true, saveSelection: () => {},
});
let left, right;
beforeEach(() => {
  calls.length = 0;
  jobs.length = 0;
  existing.clear();
  for (const k of Object.keys(store)) delete store[k];
  Object.assign(values, { 'Font.PanelSize': '13', 'Colors.DarkMode': '2', 'Display.SortNatural': '1', 'Display.ShowSystem': '0', 'CopyDelete.UseTrash': '1', 'ViewerEditor.Editor': '' });
  colors = { light: scheme('Colors'), dark: scheme('ColorsDark'), builtin: [{ name: 'Classic blue', colors: { fore: '#00ffff', back: '#000080', mark: '#ffff00', cursor: '#008080', cursorText: '#000000' } }] };
  _resetColorHistory();
  left = panel('left', '/src', [entry('a.txt'), entry('b.txt'), entry('pics', true)], ['a.txt', 'pics']);
  right = panel('right', '/out');
  Object.assign(app, { platform: 'darwin', home: '/Users/me', sep: '/', panels: [left, right], active: 0, startTheme: '' });
  app.setting = (k, d) => store[k] ?? d;
  app.setSetting = (k, v) => { store[k] = v; };
  app.sepFor = () => '/';
  reg.setApp(app);
  document.documentElement.dataset.theme = 'dark';
});

const tool = async (name, args) => runTool(name, args, { run: 1 });

// ---- run_command ----

const ran = [];
reg.command({ name: 'cm_Exchange', label: 'Swap panels', run: () => { ran.push('cm_Exchange'); return true; } });
reg.command({ name: 'cm_Delete', label: 'Delete…', run: () => { ran.push('cm_Delete'); return true; } });
reg.command({ name: 'cm_QuickFilter', label: 'Quick filter', run: (a, args) => { ran.push(['cm_QuickFilter', args]); return true; } });
reg.command({ name: 'cm_ReopenClosedTab', label: 'Reopen closed tab', enabled: () => false, run: () => { ran.push('reopen'); } });
reg.command({ name: 'cm_SomethingNew', label: 'Something new', run: () => { ran.push('new'); } });

test('run_command runs an allowed command, with plain arguments only', async () => {
  ran.length = 0;
  const r = await tool('run_command', { name: 'cm_Exchange' });
  expect(r.error).toBe('');
  expect(r.text).toMatch(/^Ran cm_Exchange \(Swap panels\)\. Panels: left \(active\): \/src; right: \/out\.$/);
  await tool('run_command', { name: 'cm_QuickFilter', args: { text: '*.pdf', evil: { run: 1 }, f: () => 1 } });
  expect(ran).toEqual(['cm_Exchange', ['cm_QuickFilter', { text: '*.pdf' }]]);
});

test('run_command refuses dangerous, unreviewed, unknown and unavailable commands, and runs none of them', async () => {
  ran.length = 0;
  expect((await tool('run_command', { name: 'cm_Delete' })).error).toMatch(/cm_Delete cannot be run by the AI: it deletes files/);
  expect((await tool('run_command', { name: 'cm_SomethingNew' })).error).toMatch(/not on the list/);
  expect((await tool('run_command', { name: 'cm_Nope' })).error).toMatch(/no command called cm_Nope/);
  expect((await tool('run_command', { name: 'cm_ReopenClosedTab' })).error).toMatch(/cannot run right now/);
  expect(ran).toEqual([]);
});

test('list_commands lists only allowed commands that exist', async () => {
  const r = JSON.parse((await tool('list_commands', {})).text);
  const names = r.commands.map((c) => c.name);
  expect(names).toContain('cm_Exchange');
  expect(names).not.toContain('cm_Delete');
  expect(names).not.toContain('cm_SomethingNew');
  expect(r.commands.find((c) => c.name === 'cm_ReopenClosedTab').unavailableNow).toBe(true);
});

test('open_folder opens folders and refuses files', async () => {
  left.cur = left.entries[2];
  expect((await tool('open_folder', {})).text).toBe('The left panel shows /src/pics.');
  left.path = '/src';
  expect((await tool('open_folder', { name: 'a.txt' })).error).toMatch(/a\.txt is a file\. open_folder opens folders only/);
  expect((await tool('open_folder', { name: 'zzz' })).error).toMatch(/There is no "zzz"/);
});

test('set_panel_paths shows folders in both panels', async () => {
  expect((await tool('set_panel_paths', { left: '~/Downloads', right: 'sub' })).text).toBe('The left panel shows /Users/me/Downloads. The right panel shows /out/sub.');
  expect((await tool('set_panel_paths', {})).error).toMatch(/Give left, right or both/);
});

// ---- options ----

test('set_setting checks the value, applies it and answers the old and the new value', async () => {
  const r = await tool('set_setting', { name: 'Font.PanelSize', value: 16 });
  expect(r.error).toBe('');
  expect(JSON.parse(r.text)).toMatchObject({ name: 'Font.PanelSize', old: 13, new: 16 });
  expect(calls.find((c) => c.k === 'Configuration.Apply').args[0]).toEqual({ 'Font.PanelSize': '16' });
  const d = JSON.parse((await tool('set_setting', { name: 'Colors.DarkMode', value: 'always' })).text);
  expect(d).toMatchObject({ old: 2, new: 1, oldLabel: 'Follow system', newLabel: 'Always' });
  expect(JSON.parse((await tool('set_setting', { name: 'Display.SortNatural', value: true })).text)).toMatchObject({ unchanged: true, value: true });
});

test('set_setting refuses bad values and options off the list, and applies nothing', async () => {
  expect((await tool('set_setting', { name: 'Font.PanelSize', value: 500 })).error).toMatch(/at most 72/);
  expect((await tool('set_setting', { name: 'Colors.DarkMode', value: 7 })).error).toMatch(/one of/);
  expect((await tool('set_setting', { name: 'CopyDelete.UseTrash', value: false })).error).toMatch(/CopyDelete page cannot be read or changed/);
  expect((await tool('set_setting', { name: 'ViewerEditor.Editor', value: 'vim' })).error).toMatch(/cannot be read or changed/);
  expect((await tool('set_setting', { name: 'Display.ShowSystem', value: true })).error).toMatch(/no effect yet/);
  expect((await tool('set_setting', { name: 'Nope.Nothing', value: 1 })).error).toMatch(/no option called/);
  expect(calls.some((c) => c.k === 'Configuration.Apply')).toBe(false);
  expect(values['CopyDelete.UseTrash']).toBe('1');
});

test('get_setting and list_settings show allowed options only', async () => {
  expect(JSON.parse((await tool('get_setting', { name: 'Colors.DarkMode' })).text)).toMatchObject({ value: 2, valueLabel: 'Follow system' });
  expect((await tool('get_setting', { name: 'CopyDelete.UseTrash' })).error).not.toBe('');
  const all = JSON.parse((await tool('list_settings', {})).text);
  const names = all.pages.flatMap((p) => p.options.map((o) => o.name));
  expect(names.sort()).toEqual(['Colors.DarkMode', 'Display.SortNatural', 'Font.PanelSize']);
});

// ---- colours ----

test('set_colors refuses text that would vanish on its background, and saves nothing', async () => {
  const r = await tool('set_colors', { colors: { fore: '#202020', back: '#202020' } });
  expect(r.error).toMatch(/The dark colours were not changed: the file names on the background: #202020 on #202020 has a contrast of 1/);
  expect(calls.some((c) => c.k === 'Configuration.SetColors')).toBe(false);
});

test('set_colors saves a readable scheme; restore_colors puts the colours back, the last change or all', async () => {
  const before = JSON.parse(JSON.stringify(colors));
  const r = await tool('set_colors', { scheme: 'dark', colors: { fore: '#e6e6e6', back: '#1d1f21', mark: '#ffcc00' } });
  expect(r.error).toBe('');
  expect(colors.dark.colors).toMatchObject({ fore: '#e6e6e6', back: '#1d1f21', mark: '#ffcc00' });
  expect(colors.light).toEqual(before.light);
  await tool('set_colors', { scheme: 'both', preset: 'Classic blue' });
  expect(colors.light.colors.back).toBe('#000080');
  expect((await tool('restore_colors', { last: true })).text).toMatch(/last colour change was undone; 1 earlier change remains/);
  expect(colors.dark.colors.back).toBe('#1d1f21');
  expect(colors.light).toEqual(before.light);
  await tool('set_colors', { filters: [{ mask: '*.jpg', color: '#4caf50' }] });
  expect((await tool('restore_colors', {})).text).toMatch(/back as they were/);
  expect({ light: colors.light, dark: colors.dark }).toEqual({ light: before.light, dark: before.dark });
  expect((await tool('restore_colors', {})).text).toMatch(/nothing to put back/);
});

test('get_colors tells both schemes and which one is shown', async () => {
  const r = JSON.parse((await tool('get_colors', {})).text);
  expect(r).toMatchObject({ showing: 'dark', presets: ['Classic blue'], canRestore: false });
  expect(r.dark.colors.fore).toBe('');
});

// ---- pack_files ----

test('pack_files packs the selection into a new archive in the other panel\'s folder, never moving the files', async () => {
  const r = await tool('pack_files', { archive: 'stuff' });
  expect(r.error).toBe('');
  expect(r.text).toBe('Packed 2 entries into /out/stuff.zip. The files themselves were not changed.');
  expect(jobs).toEqual([expect.objectContaining({ kind: 'pack', sources: ['/src/a.txt', '/src/pics'], target: '/out/stuff.zip', format: 'zip', move: false, separate: false, password: '', volumeSize: 0 })]);
});

test('pack_files refuses an archive that already exists, and starts no job', async () => {
  existing.add('/out/stuff.zip');
  expect((await tool('pack_files', { archive: 'stuff.zip' })).error).toMatch(/stuff\.zip already exists in \/out\. pack_files never overwrites or adds to an existing archive/);
  existing.add('/src/here.zip');
  expect((await tool('pack_files', { archive: 'here.zip', where: 'same' })).error).toMatch(/already exists in \/src/);
  expect(jobs).toEqual([]);
});

test('pack_files refuses paths, one-file packers for several files, non-local folders and an empty selection', async () => {
  expect((await tool('pack_files', { archive: '../up.zip' })).error).toMatch(/plain file name/);
  expect((await tool('pack_files', { archive: 'x.gz' })).error).toMatch(/packs one file only/);
  right.path = 'sftp://host/dir';
  expect((await tool('pack_files', { archive: 'x.zip' })).error).toMatch(/only be made in a local folder/);
  right.path = '/out';
  left.selected = new Set();
  expect((await tool('pack_files', { archive: 'x.zip' })).error).toMatch(/Nothing is selected/);
  expect((await tool('pack_files', { archive: 'x.zip', names: ['nope'] })).error).toMatch(/Not in \/src: nope/);
  expect(jobs).toEqual([]);
  expect((await tool('pack_files', { archive: 'one.gz', names: ['b.txt'] })).text).toMatch(/into \/out\/one\.gz/);
});
