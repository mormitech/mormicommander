// The command tools of the AI layer (issue #146, step 3): list_commands and run_command over the page's command
// registry, so a model can do what a user does with keys (Tab, Ctrl+U, Backspace, sorting, view modes, tabs,
// selection), and two navigation helpers, open_folder and set_panel_paths. run_command runs only the commands on the
// reviewed allow-list of aitools-allow.js; anything else, a command added later included, is refused with a sentence
// that says why. The arguments a command gets are plain values only (no objects, no functions), so a model cannot
// hand a command a callback or an option bag it was never reviewed with.
//
// A command that opens a dialog would make run_command wait for the user; the allow-list leaves those out, and the
// tool answers after a few seconds anyway ("still running") rather than hanging the model's turn.

import { addAITool } from '../core/aitools.js';
import { app as theApp } from '../core/app.js';
import { commandList, getCommand, hasCommand, keysFor, run } from '../core/registry.js';
import { ALLOWED_COMMANDS, ALLOWED_FAMILIES, commandVerdict } from './aitools-allow.js';
import { side, expandHome } from './aitools-lib.js';

const RUN_WAIT = 15_000; // ms run_command waits for a command before it answers that it is still running
const SIDE = { type: 'string', enum: ['left', 'right', 'active', 'other'], description: 'Which panel; "active" (the default) is the panel the user works in, "other" the other one.' };

// plainArgs keeps the arguments of run_command that are plain values (text, numbers, yes/no), at most 10 of them.
export function plainArgs(a) {
  if (!a || typeof a !== 'object' || Array.isArray(a)) return {};
  const out = {};
  for (const [k, v] of Object.entries(a).slice(0, 10)) {
    if (/^\w{1,40}$/.test(k) && (typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean')) out[k] = typeof v === 'string' ? v.slice(0, 1000) : v;
  }
  return out;
}

const panelsNow = () => theApp.panels.map((p) => `${p.side}${p === theApp.cur ? ' (active)' : ''}: ${p.path}`).join('; ');

// ---- list_commands ----

addAITool({
  name: 'list_commands',
  description: 'Lists the mormiCommander commands run_command can run: navigation (parent, root, home, back, forward), switching and swapping '
    + 'the panels, folder tabs, view modes (brief, full, thumbnails, tree), sorting, hidden files, the quick filter, refresh, branch view, '
    + 'compare folders (selects the differences), selection commands and the bars. Each entry has the name, what it does, its keyboard '
    + 'shortcut and, for toggles, whether it is on. Numbered families (tab N, sort by column N, column set N) are listed once with <N>. '
    + 'Commands that delete, move, rename, copy, edit, run programs or change settings are not listed and cannot be run. filter narrows the list by words in the name or text.',
  schema: { type: 'object', properties: { filter: { type: 'string', description: 'Words that must appear in the command\'s name or text, like "sort" or "tab".' } } },
  run: (args) => {
    const words = String(args.filter ?? '').toLowerCase().split(/\s+/).filter(Boolean);
    const hit = (s) => words.every((w) => s.toLowerCase().includes(w));
    const out = [];
    for (const c of commandList()) {
      if (!Object.hasOwn(ALLOWED_COMMANDS, c.name)) continue;
      const text = c.label && c.label !== c.name ? c.label : ALLOWED_COMMANDS[c.name];
      if (!hit(`${c.name} ${text} ${ALLOWED_COMMANDS[c.name]}`)) continue;
      const keys = keysFor(c.name).slice(0, 2);
      const e = { name: c.name, does: text };
      if (keys.length) e.keys = keys.join(', ');
      if (c.checked) { try { e.on = !!c.checked(theApp); } catch { /* not ready */ } }
      if (c.enabled) { try { if (!c.enabled(theApp)) e.unavailableNow = true; } catch { /* not ready */ } }
      out.push(e);
    }
    for (const f of ALLOWED_FAMILIES) if (hit(`${f.name} ${f.label}`) && commandList().some((c) => f.re.test(c.name))) out.push({ name: f.name, does: f.label });
    return { commands: out, note: 'Run one with run_command {name}. Anything not listed here is refused.' };
  },
});

// ---- run_command ----

addAITool({
  name: 'run_command',
  description: 'Runs one mormiCommander command by name, like pressing its key: e.g. cm_FocusTrg (switch panel, Tab), cm_Exchange (swap panels, Ctrl+U), '
    + 'cm_GoToParent, cm_SrcLong (full view), cm_SrcByDateTime (sort by date), cm_SwitchHidSys (hidden files), cm_CompareDirs (selects the differences '
    + 'between the panels), cm_SelectAll. Only the commands list_commands lists can run; any other name is refused, and nothing that deletes, '
    + 'moves, renames, copies, edits, runs a program or changes settings can run. args is for the few commands that take one, like cm_QuickFilter '
    + '{"text": "*.pdf"}; only plain values are passed. Answers what ran and the folders the panels show afterwards.',
  schema: {
    type: 'object',
    properties: {
      name: { type: 'string', description: 'The command, like "cm_Exchange".' },
      args: { type: 'object', description: 'Optional plain arguments for the command, like {"text": "*.pdf"} for cm_QuickFilter.' },
    },
    required: ['name'],
  },
  run: async (args) => {
    const name = String(args.name ?? '').trim();
    const v = commandVerdict(name, hasCommand(name));
    if (!v.ok) throw new Error(v.reason);
    const c = getCommand(name);
    if (c.enabled && !c.enabled(theApp)) throw new Error(`${name} (${c.label}) cannot run right now: it does not apply to what the panels show (for example nothing is selected, or there is no closed tab).`);
    let timer = 0;
    const late = new Promise((resolve) => { timer = setTimeout(() => resolve('late'), RUN_WAIT); });
    try {
      const r = await Promise.race([run(name, plainArgs(args.args)), late]);
      if (r === 'late') return `${name} (${c.label}) is still running, perhaps waiting for the user. Panels: ${panelsNow()}.`;
      return `Ran ${name} (${c.label}). Panels: ${panelsNow()}.`;
    } finally {
      clearTimeout(timer);
    }
  },
});

// ---- open_folder ----

addAITool({
  name: 'open_folder',
  description: 'Opens a folder of a panel\'s list in that panel, like Enter on a folder: the folder under the cursor, or the one named. ".." goes to the parent '
    + 'folder. Files are never opened or run: naming a file is refused. To show any folder by its path, use set_panel_path.',
  schema: {
    type: 'object',
    properties: { side: SIDE, name: { type: 'string', description: 'The name of a folder in the panel\'s list (not a path). Default: the entry under the cursor.' } },
  },
  run: async (args) => {
    const p = theApp.panel(side(args.side));
    if (!p?.path) throw new Error('That panel shows no folder yet.');
    const want = String(args.name ?? '').trim();
    const e = want ? p.entries.find((x) => x.name === want) : p.cur;
    if (want === '..' || e?.up) {
      const parent = p.parent; // (what the panel's ".." row leads to, as Backspace has it)
      if (!parent || parent === p.path) throw new Error(`${p.path} has no parent folder.`);
      if (!(await p.go(parent))) throw new Error(`Could not open ${parent}.`);
      return `The ${p.side} panel shows ${p.path}.`;
    }
    if (!e) throw new Error(want ? `There is no "${want}" in ${p.path}.` : 'The cursor is on nothing.');
    if (!e.dir) throw new Error(`${e.name} is a file. open_folder opens folders only; files are never opened or run by the AI.`);
    const before = theApp.lastMessage;
    if (!(await p.go(p.fullPath(e.name)))) throw new Error(theApp.lastMessage !== before ? theApp.lastMessage : `Could not open ${e.name}.`);
    return `The ${p.side} panel shows ${p.path}.`;
  },
});

// ---- set_panel_paths ----

async function show(p, raw) {
  const path0 = expandHome(raw, theApp.home, theApp.sep);
  const path = p.path ? theApp.resolvePath(path0, p.path) : path0;
  const before = theApp.lastMessage;
  if (!(await p.go(path))) throw new Error(theApp.lastMessage !== before ? theApp.lastMessage : `Could not open ${path}.`);
  return p.path;
}

addAITool({
  name: 'set_panel_paths',
  description: 'Shows folders in both panels at once (like typing them in the path bars): left and/or right. "~" is the home folder; a relative path is taken '
    + 'from that panel\'s current folder. Answers the folders the panels show afterwards; a folder that cannot be opened is reported and that panel stays where it was.',
  schema: {
    type: 'object',
    properties: {
      left: { type: 'string', description: 'The folder for the left panel.' },
      right: { type: 'string', description: 'The folder for the right panel.' },
    },
  },
  run: async (args) => {
    const jobs = [['left', args.left], ['right', args.right]].filter(([, v]) => String(v ?? '').trim());
    if (!jobs.length) throw new Error('Give left, right or both: the folders to show.');
    const lines = [], errors = [];
    for (const [s, v] of jobs) {
      try { lines.push(`The ${s} panel shows ${await show(theApp.panel(s), String(v))}.`); } catch (e) { errors.push(`${s}: ${e.message}`); }
    }
    if (errors.length && !lines.length) throw new Error(errors.join(' '));
    return [...lines, ...errors.map((e) => `Not opened: ${e}`)].join(' ');
  },
});
