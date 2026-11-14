// The tools an AI model can use (issue #146): features add them here, the page tells Go the list, and Go relays the
// model's calls back as "ai-tool" events. The tools run in the page because that is where the panels, the selection,
// the settings and the Find files dialog live; the Go service AI (feature_ai.go) only carries the calls to and from
// the model's process, so the model works through mormiCommander's own commands and never runs shell commands.
//
//   addAITool({name: 'get_state', description: '…', schema: {type: 'object', properties: {}}, run: async (args, ctx) => …});
//
// run gets the model's arguments and ctx = {run} (the number of the AI run that called it, as AI.Start answered it);
// what it returns is the text the model reads (an object is sent as JSON). A throw becomes the tool's error, which
// the model reads too, so messages are written as sentences it can act on.
//
// A build without the Go service (or the browser preview) has no AI.SetTools: registering then fails quietly, and
// the features that start runs ask AI.Providers() and say AI is not available.

import { go, on, errText } from './go.js';
import { app } from './app.js';

const tools = new Map(); // name → {name, description, schema, run}
let started = false; // the app is ready: the list may go to Go
let timer = 0;

// addAITool adds (or replaces) a tool. The list goes to Go a moment later, so tools added together go in one call.
export function addAITool(t) {
  if (!t?.name || typeof t.run !== 'function') throw new Error('addAITool needs a name and a run function');
  tools.set(t.name, t);
  schedule();
  return () => { if (tools.get(t.name) === t) { tools.delete(t.name); schedule(); } };
}

// aiTools is the list as Go gets it (AITool: name, description, inputSchema).
export function aiTools() {
  return [...tools.values()].map((t) => ({
    name: t.name,
    description: t.description || '',
    inputSchema: t.schema || { type: 'object', properties: {} },
  }));
}

function schedule() {
  if (!started) return;
  clearTimeout(timer);
  timer = setTimeout(register, 30);
}

function register() {
  timer = 0;
  try { Promise.resolve(go.AI.SetTools(aiTools())).catch(() => {}); } catch { /* no AI service in this build */ }
}

// startAITools sends the list to Go now and whenever it changes later. It runs when the app is ready (below).
export function startAITools() {
  if (started) return;
  started = true;
  register();
}

// runTool runs one call and answers what goes back to Go: {text, error}.
export async function runTool(name, args, ctx) {
  const t = tools.get(name);
  if (!t) return { text: '', error: `There is no tool called "${name}".` };
  try {
    const r = await t.run(args && typeof args === 'object' ? args : {}, ctx);
    const text = r === undefined || r === null ? '' : typeof r === 'string' ? r : JSON.stringify(r);
    return { text, error: '' };
  } catch (e) {
    return { text: '', error: errText(e) || 'The tool failed.' };
  }
}

// onAIProgress watches the "ai-progress" events of one run (run 0 or null: every run); fn gets (ev, run), ev being
// {kind: "text"|"tool"|"done"|"error", text, tool, args, error}. It returns a function that stops watching.
export function onAIProgress(run, fn) {
  return on('ai-progress', (r, ev) => { if (!run || r === run) fn(ev || {}, r); });
}

on('ai-tool', async (id, call) => {
  const c = call || {};
  const res = await runTool(c.name, c.args, { run: c.run || 0 });
  try { await go.AI.ToolReply(id, JSON.stringify(res)); } catch { /* the run is gone */ }
});

app.on?.('ready', startAITools);
