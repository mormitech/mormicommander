// @vitest-environment happy-dom
// The Find files dialog's AI tab (features/find-ai.js, issue #146) wired into the real dialog (find.js): the tab is
// there, Enter in the prompt starts AI.Start with the dialog's context instead of a search, the run's progress fills
// the log, Stop stops the run, and a find_files call from that run searches through the dialog, so its results fill
// the dialog's list. Without the Go service the tab says AI is not available. Go is a recorder whose events the test
// sends itself; the dialog stays in the page (no dialog windows).
import { afterEach, expect, test, vi } from 'vitest';

const calls = [];
const handlers = new Map();
let aiMissing = false;
let providers = null; // what AI.Providers answers instead of Claude Code alone
const answers = {
  'Find.Setup': () => ({ history: {}, codepages: ['windows-1252', 'cp437'], saved: [], last: null }),
  'Find.Volumes': () => [],
  'Find.Start': () => 41,
  'AI.Providers': () => { if (aiMissing) throw new Error('AI.Providers is not available (is the Go feature registered?)'); return providers || { claude: { available: true, path: '/bin/claude', version: '2.1.0' } }; },
  'AI.Start': () => { if (aiMissing) throw new Error('AI.Start is not available (is the Go feature registered?)'); return 9; },
  'DialogWindows.Open': () => { throw new Error('no windows here'); },
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
  return {
    go,
    on: (ev, fn) => { handlers.set(ev, [...(handlers.get(ev) || []), fn]); return () => handlers.set(ev, (handlers.get(ev) || []).filter((x) => x !== fn)); },
    errText: (e) => String(e?.message ?? e).replace(/^Error:\s*/, ''),
    onFileDrop: () => () => {},
    setClipboardText: async () => true,
  };
});
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => (k === 'dialogsInWindows' ? false : d), setSetting: () => Promise.resolve(), onSettingChange: () => () => {}, allSettings: () => ({}) }));

const { app } = await import('../src/core/app.js');
await import('../src/features/find.js');
const { runTool } = await import('../src/core/aitools.js');
await import('../src/features/find-ai.js');
const { run: runCommand, setApp } = await import('../src/core/registry.js');

const left = { side: 'left', path: '/Users/me/Code', entries: [], selectedEntries: () => [], fullPath: (n) => n, $: { list: document.createElement('div') } };
const right = { side: 'right', path: '/Users/me/Documents', entries: [], selectedEntries: () => [], fullPath: (n) => n, $: { list: document.createElement('div') } };
Object.assign(app, { platform: 'darwin', home: '/Users/me', sep: '/', panels: [left, right], active: 0 });
app.setting = (k, d) => d;
app.setSetting = () => {};
setApp(app);
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const tick = (ms = 10) => new Promise((r) => setTimeout(r, ms));
const fire = (ev, ...args) => { for (const fn of handlers.get(ev) || []) fn(...args); };
const box = () => document.querySelector('.mc-find');
const key = (target, k, mods = {}) => target.dispatchEvent(new KeyboardEvent('keydown', { key: k, code: k, bubbles: true, cancelable: true, ...mods }));

async function open() {
  await runCommand('cm_SearchFor');
  await tick();
  box().querySelector('.mc-find-tab[data-tab=ai]').click();
  await tick();
}

afterEach(async () => {
  const b = box();
  if (b) { key(b, 'Escape'); key(b, 'Escape'); await tick(); }
  calls.length = 0;
  aiMissing = false;
  providers = null;
});

test('the AI tab: provider line, Enter starts a run with the context, progress fills the log, Stop stops it', async () => {
  await open();
  const b = box();
  expect(b.querySelector('.mc-find-ai-provider').textContent).toBe('Claude Code 2.1.0 found at /bin/claude');
  const prompt = b.querySelector('#ff-ai-prompt');
  prompt.value = 'pdfs from this week';
  key(prompt, 'Enter');
  await tick();
  const start = calls.find((c) => c.k === 'AI.Start');
  expect(start.args[0]).toMatchObject({ prompt: 'pdfs from this week', provider: '' }); // (Go uses the provider of the AI settings)
  expect(start.args[0].context).toMatch(/active panel \(left\) shows \/Users\/me\/Code/);
  expect(calls.some((c) => c.k === 'Find.Start')).toBe(false); // (not a normal search)
  expect(b.querySelector('.ff-cancel').textContent).toBe('Stop');
  expect(b.querySelector('.ff-start').disabled).toBe(true);

  fire('ai-progress', 9, { kind: 'tool', tool: 'mcp__mormi__find_files', args: { namePattern: '*.pdf', searchIn: '/Users/me/Documents' } });
  fire('ai-progress', 9, { kind: 'text', text: 'Found ' });
  fire('ai-progress', 9, { kind: 'text', text: 'them.' });
  fire('ai-progress', 8, { kind: 'text', text: 'another run' });
  const log = () => [...b.querySelectorAll('.mc-find-ai-log > div')].map((d) => `${d.className}: ${d.textContent}`);
  expect(log()).toEqual(['ai-note: Asking Claude…', 'ai-tool: Searching *.pdf in ~/Documents…', 'ai-text: Found them.']);

  b.querySelector('.ff-cancel').click();
  expect(calls.find((c) => c.k === 'AI.Stop').args).toEqual([9]);
  expect(box()).toBeTruthy(); // (Stop, not Cancel: the dialog stays)
  fire('ai-progress', 9, { kind: 'done', text: 'Found them.' });
  expect(log().at(-1)).toBe('ai-answer: Found them.');
  expect(b.querySelector('.ff-cancel').textContent).toBe('Cancel');
});

test('find_files from the tab\'s run searches through the dialog and fills its list', async () => {
  await open();
  const b = box();
  b.querySelector('#ff-ai-prompt').value = 'big pdfs';
  b.querySelector('.ff-start').click();
  await tick();
  const res = runTool('find_files', { namePattern: '*.pdf', searchIn: '~/Documents', sizeValue: 1, sizeUnit: 'MB' }, { run: 9 });
  await tick();
  const fs = calls.find((c) => c.k === 'Find.Start');
  expect(fs.args[0]).toMatchObject({ searchFor: '*.pdf', searchIn: '/Users/me/Documents', sizeOn: true, sizeValue: 1, sizeUnit: 'MB', depth: -1 });
  expect(b.querySelector('#ff-for').value).toBe('*.pdf'); // (the fields show what was searched)
  fire('find-results', { id: 41, items: [{ p: '/Users/me/Documents/a.pdf', n: 'a.pdf', d: '/Users/me/Documents', s: 2 << 20, t: 0 }] });
  fire('find-done', { id: 41, files: 1, dirs: 0 });
  const r = await res;
  expect(r.error).toBe('');
  expect(r.text).toMatch(/^Found 1 result \(1 file, 0 folders\)\. They are shown in the Find files dialog's result list\.\n\/Users\/me\/Documents\/a\.pdf {2}2\.0 MB/);
  await tick(20);
  expect(b.querySelectorAll('.mc-find-list .r').length).toBe(1);
  fire('ai-progress', 9, { kind: 'done', text: 'One.' });
});

test('without the Go service the tab says AI is not available', async () => {
  aiMissing = true;
  await open();
  const b = box();
  expect(b.querySelector('.mc-find-ai-provider').textContent).toBe('AI is not available in this build.');
  b.querySelector('#ff-ai-prompt').value = 'anything';
  b.querySelector('.ff-start').click();
  await tick();
  expect(b.querySelector('.mc-find-ai-log').textContent).toMatch(/AI is not available in this build\./);
  expect(b.querySelector('.ff-cancel').textContent).toBe('Cancel');
});

test('OpenRouter chosen: the provider line names it and the model, and AI settings… opens the settings', async () => {
  providers = { selected: 'openrouter', claude: { available: false }, openrouter: { available: true, model: 'anthropic/claude-sonnet-4.5', url: 'x' } };
  await open();
  const b = box();
  expect(b.querySelector('.mc-find-ai-provider').textContent).toBe('OpenRouter, model anthropic/claude-sonnet-4.5');
  b.querySelector('#ff-ai-prompt').value = 'pdfs';
  b.querySelector('.ff-start').click();
  await tick();
  expect(b.querySelector('.mc-find-ai-log').textContent).toMatch(/Asking anthropic\/claude-sonnet-4\.5…/);
  fire('ai-progress', 9, { kind: 'done', text: 'None.' });

  providers = { selected: 'openrouter', openrouter: { available: false, error: 'No model is chosen.' } };
  const before = calls.filter((c) => c.k === 'AI.Providers').length;
  b.querySelector('.mc-find-ai-settings').click();
  await tick();
  const dlg = [...document.querySelectorAll('.mc-box')].find((x) => x.querySelector('h2')?.textContent === 'AI settings');
  expect(dlg).toBeTruthy();
  expect(calls.some((c) => c.k === 'AI.ModelsFrom')).toBe(false); // (opening the settings sends nothing)
  dlg.querySelector('.cancel').click();
  await tick();
  expect(calls.filter((c) => c.k === 'AI.Providers').length).toBeGreaterThan(before);
  expect(b.querySelector('.mc-find-ai-provider').textContent).toBe('OpenRouter: No model is chosen.');
  expect(b.querySelector('.mc-find-ai-provider').className).toMatch(/bad/);
});
