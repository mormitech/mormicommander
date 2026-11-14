// @vitest-environment happy-dom
// The Ask AI dialog (features/ask.js, issue #146 step 3): cm_AIAsk opens it with its key, the provider lines show
// every provider AI.Providers() answers, Enter starts AI.Start with provider "" and a context about both panels,
// the run's progress fills the log (tool calls as readable lines, the answer), Stop stops the run, closing stops it
// too, and Up brings back the last prompt. Go is a recorder whose events the test sends itself; the dialog stays in
// the page (no dialog windows).
import { afterEach, expect, test, vi } from 'vitest';

const calls = [];
const handlers = new Map();
let aiMissing = false;
const answers = {
  'AI.Providers': () => { if (aiMissing) throw new Error('AI.Providers is not available (is the Go feature registered?)'); return { claude: { available: true, path: '/bin/claude', version: '2.1.0' }, openrouter: { available: false, error: 'No API key is set.' } }; },
  'AI.Start': () => { if (aiMissing) throw new Error('AI.Start is not available (is the Go feature registered?)'); return 7; },
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
await import('../src/features/ask.js');
const { run: runCommand, setApp, setupKeys, commandForKey } = await import('../src/core/registry.js');

const store = {};
const left = { side: 'left', path: '/Users/me/Code', cur: { name: 'main.go' }, entries: [], selectedEntries: () => [{ name: 'a' }, { name: 'b' }], $: { list: document.createElement('div') } };
const right = { side: 'right', path: '/Users/me/Documents', cur: { up: true, name: '..' }, entries: [], selectedEntries: () => [], $: { list: document.createElement('div') } };
Object.assign(app, { platform: 'darwin', home: '/Users/me', sep: '/', panels: [left, right], active: 0 });
app.setting = (k, d) => store[k] ?? d;
app.setSetting = (k, v) => { store[k] = v; };
setApp(app);
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const tick = (ms = 10) => new Promise((r) => setTimeout(r, ms));
const fire = (ev, ...args) => { for (const fn of handlers.get(ev) || []) fn(...args); };
const box = () => document.querySelector('.mc-ask');
const key = (target, k, mods = {}) => target.dispatchEvent(new KeyboardEvent('keydown', { key: k, code: k === 'Enter' ? 'Enter' : k, bubbles: true, cancelable: true, ...mods }));

async function open() {
  await runCommand('cm_AIAsk');
  await tick();
}

afterEach(async () => {
  if (box()) { box().querySelector('.ask-close').click(); await tick(); }
  calls.length = 0;
  aiMissing = false;
});

test('cm_AIAsk has Ctrl+Shift+A on every platform, and Shift+Cmd+A on macOS', () => {
  for (const p of ['darwin', 'windows', 'linux']) {
    setupKeys(p, {});
    expect(commandForKey('Ctrl+Shift+A'), p).toBe('cm_AIAsk');
  }
  setupKeys('darwin', {});
  expect(commandForKey('Shift+Cmd+A')).toBe('cm_AIAsk');
});

test('the dialog: providers, Enter starts a run with the context, progress fills the log, Stop stops it', async () => {
  await open();
  const b = box();
  expect([...b.querySelectorAll('.mc-ask-provider > div')].map((d) => d.textContent)).toEqual(['Claude Code 2.1.0 found at /bin/claude', 'OpenRouter is not set up: No API key is set.']);
  expect(b.querySelector('.mc-ask-log').textContent).toMatch(/cannot delete, move, rename or overwrite files/);
  const prompt = b.querySelector('#ask-prompt');
  prompt.value = 'swap the panels';
  key(prompt, 'Enter');
  await tick();
  const start = calls.find((c) => c.k === 'AI.Start');
  expect(start.args[0]).toMatchObject({ prompt: 'swap the panels', provider: '' });
  expect(start.args[0].context).toMatch(/The active panel \(left\) shows \/Users\/me\/Code; the cursor is on main\.go; 2 entries are selected\./);
  expect(start.args[0].context).toMatch(/The other panel \(right\) shows \/Users\/me\/Documents; 0 entries are selected\./);
  expect(start.args[0].context).toMatch(/Platform: darwin/);
  expect(b.querySelector('.ask-start').disabled).toBe(true);
  fire('ai-progress', 7, { kind: 'tool', tool: 'mcp__mormi__run_command', args: { name: 'cm_Exchange' } });
  fire('ai-progress', 7, { kind: 'text', text: 'Swapped ' });
  fire('ai-progress', 7, { kind: 'text', text: 'them.' });
  fire('ai-progress', 99, { kind: 'text', text: 'another run' });
  const log = () => [...b.querySelectorAll('.mc-ask-log > div')].map((d) => `${d.className}: ${d.textContent}`);
  expect(log()).toEqual(['ai-note: › swap the panels', 'ai-tool: Running cm_Exchange…', 'ai-text: Swapped them.']);
  b.querySelector('.ask-stop').click();
  expect(calls.find((c) => c.k === 'AI.Stop').args).toEqual([7]);
  fire('ai-progress', 7, { kind: 'done', text: 'Swapped them.' });
  await tick();
  expect(log().at(-1)).toBe('ai-answer: Swapped them.');
  expect(b.querySelector('.ask-start').disabled).toBe(false);
  expect(b.querySelector('.ask-stop').disabled).toBe(true);
});

test('Shift+Enter does not start; Up brings back the last prompt; an error shows in the log', async () => {
  await open();
  const b = box();
  const prompt = b.querySelector('#ask-prompt');
  prompt.value = 'x';
  key(prompt, 'Enter', { shiftKey: true });
  await tick();
  expect(calls.some((c) => c.k === 'AI.Start')).toBe(false);
  prompt.value = '';
  key(prompt, 'ArrowUp');
  expect(prompt.value).toBe('swap the panels');
  key(prompt, 'ArrowDown');
  expect(prompt.value).toBe('');
  aiMissing = true;
  prompt.value = 'hello';
  key(prompt, 'Enter');
  await tick();
  expect(b.querySelector('.mc-ask-log .ai-error').textContent).toBe('AI is not available in this build.');
  expect(store.aiAskHistory).toEqual(['swap the panels', 'hello']);
});

test('closing the dialog stops a run that is still going', async () => {
  await open();
  const prompt = box().querySelector('#ask-prompt');
  prompt.value = 'make a dark theme';
  key(prompt, 'Enter');
  await tick();
  box().querySelector('.ask-close').click();
  await tick();
  expect(box()).toBe(null);
  expect(calls.filter((c) => c.k === 'AI.Stop').map((c) => c.args)).toEqual([[7]]);
});
