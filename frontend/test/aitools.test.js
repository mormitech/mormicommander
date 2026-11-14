// The AI tool registry (core/aitools.js, issue #146): the list Go gets (once the app is ready, again a moment after a
// tool is added, in one call for tools added together), and the answers to "ai-tool" calls: a string as it is, an
// object as JSON, an unknown tool or a throw as an error. Go and the app are stand-ins that record what they get.
import { beforeEach, expect, test, vi } from 'vitest';

const calls = [];
const handlers = new Map();
let setToolsFails = false;
vi.mock('../src/core/go.js', () => ({
  go: {
    AI: {
      SetTools: (tools) => { calls.push(['SetTools', tools]); return setToolsFails ? Promise.reject(new Error('AI.SetTools is not available (is the Go feature registered?)')) : Promise.resolve(); },
      ToolReply: (id, json) => { calls.push(['ToolReply', id, JSON.parse(json)]); return Promise.resolve(); },
    },
  },
  on: (ev, fn) => { handlers.set(ev, [...(handlers.get(ev) || []), fn]); return () => {}; },
  errText: (e) => String(e?.message ?? e),
}));
const ready = [];
vi.mock('../src/core/app.js', () => ({ app: { on: (ev, fn) => { if (ev === 'ready') ready.push(fn); } } }));

const { addAITool, aiTools, runTool, onAIProgress } = await import('../src/core/aitools.js');

const tick = (ms = 40) => new Promise((r) => setTimeout(r, ms));
const fire = (ev, ...args) => { for (const fn of handlers.get(ev) || []) fn(...args); };

beforeEach(() => { calls.length = 0; setToolsFails = false; });

test('tools added before the app is ready go to Go once it is, in one call', async () => {
  addAITool({ name: 'echo', description: 'Says it back', schema: { type: 'object', properties: { s: { type: 'string' } } }, run: (a) => a.s });
  addAITool({ name: 'obj', run: () => ({ n: 2 }) });
  await tick();
  expect(calls).toEqual([]); // not ready yet
  ready.forEach((fn) => fn());
  await tick();
  expect(calls.length).toBe(1);
  expect(calls[0][1]).toEqual([
    { name: 'echo', description: 'Says it back', inputSchema: { type: 'object', properties: { s: { type: 'string' } } } },
    { name: 'obj', description: '', inputSchema: { type: 'object', properties: {} } },
  ]);
});

test('a tool added later sends the list again, once for several', async () => {
  addAITool({ name: 'later1', run: () => '' });
  addAITool({ name: 'later2', run: () => '' });
  await tick();
  expect(calls.filter((c) => c[0] === 'SetTools').length).toBe(1);
  expect(aiTools().map((t) => t.name)).toEqual(['echo', 'obj', 'later1', 'later2']);
});

test('a build without the AI service registers quietly', async () => {
  setToolsFails = true;
  addAITool({ name: 'quiet', run: () => '' });
  await tick();
  expect(calls[0][0]).toBe('SetTools'); // (and no unhandled rejection fails the run)
});

test('runTool: text, JSON, unknown tool, throw', async () => {
  expect(await runTool('echo', { s: 'hi' }, { run: 1 })).toEqual({ text: 'hi', error: '' });
  expect(await runTool('obj', {}, { run: 1 })).toEqual({ text: '{"n":2}', error: '' });
  expect((await runTool('nope', {}, {})).error).toMatch(/no tool called "nope"/);
  addAITool({ name: 'boom', run: async () => { throw new Error('That folder does not exist.'); } });
  expect(await runTool('boom', {}, {})).toEqual({ text: '', error: 'That folder does not exist.' });
});

test('an "ai-tool" event runs the tool with the run number and answers ToolReply(id, json)', async () => {
  let seen = null;
  addAITool({ name: 'ctx', run: (args, ctx) => { seen = { args, ctx }; return 'ok'; } });
  fire('ai-tool', 7, { run: 3, name: 'ctx', args: { a: 1 } });
  await tick();
  expect(seen).toEqual({ args: { a: 1 }, ctx: { run: 3 } });
  expect(calls.find((c) => c[0] === 'ToolReply')).toEqual(['ToolReply', 7, { text: 'ok', error: '' }]);
  calls.length = 0;
  fire('ai-tool', 8, { run: 3, name: 'missing', args: null });
  await tick();
  expect(calls[0][1]).toBe(8);
  expect(calls[0][2].error).toMatch(/missing/);
});

test('onAIProgress watches one run, or every run with 0', () => {
  const one = [], all = [];
  onAIProgress(5, (ev) => one.push(ev.kind));
  onAIProgress(0, (ev, run) => all.push(`${run}:${ev.kind}`));
  fire('ai-progress', 5, { kind: 'text', text: 'a' });
  fire('ai-progress', 6, { kind: 'done', text: 'b' });
  expect(one).toEqual(['text']);
  expect(all).toEqual(['5:text', '6:done']);
});
