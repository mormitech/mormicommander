// The command registry: every action of the app is a named command (cm_* like Total Commander's), and keys, menus, the
// function key bar, the command line ("cm_Copy" + Enter) and control mode all run commands by name. A feature adds
// commands with command({...}) when its module is imported (see docs/ARCHITECTURE.md):
//
//   command({
//     name: 'cm_MkDir',                 // the TC name (or one of ours, marked "ours" in FEATURES.md)
//     label: 'New folder…',             // menu and command-browser text: plain words, "…" when a dialog follows
//     short: 'New folder',              // optional: the function key bar text
//     run: async (app, args) => {...},  // does it; args come from control mode or other commands ({} from keys)
//     enabled: (app) => true,           // optional: greyed out in menus when false
//     checked: (app) => bool,           // optional: a toggle; the menu shows a check mark
//     keys: ['Ctrl+Alt+N'],             // optional: extra default keys (TC's keys are already in keymap.js)
//   });
//
// The keymap is DEFAULT_KEYS (+ MAC_KEYS on macOS) + keys declared by commands, with the user's changes on top.

import { DEFAULT_KEYS, MAC_KEYS, NUMPAD_FALLBACK } from './keymap.js';
import { normalizeCombo } from './keys.js';
import { track } from './busy.js';

const commands = new Map();
const listeners = new Set(); // called when commands or keys change (menus, function key bar)

export function command(def) {
  if (!def?.name || typeof def.run !== 'function') throw new Error('a command needs a name and a run function');
  if (commands.has(def.name)) console.warn(`command ${def.name} registered twice; the last one wins`);
  commands.set(def.name, { label: def.name, ...def });
  changed();
}

export const getCommand = (name) => commands.get(name);
export const hasCommand = (name) => commands.has(name);
export const commandList = () => [...commands.values()];

// onChange calls fn whenever commands or keys change.
export function onChange(fn) { listeners.add(fn); return () => listeners.delete(fn); }
let pending = false;
function changed() {
  if (pending) return;
  pending = true;
  queueMicrotask(() => { pending = false; listeners.forEach((f) => f()); });
}

// ---- keys ----
let platform = 'darwin';
let userKeys = {};
let keymap = new Map();

// setupKeys builds the keymap for the platform with the user's changes ({combo: commandName or ""}).
export function setupKeys(plat, user = {}) {
  platform = plat;
  userKeys = {};
  for (const [k, v] of Object.entries(user || {})) userKeys[normalizeCombo(k)] = v;
  rebuildKeys();
}

function rebuildKeys() {
  const m = new Map();
  const put = (k, v) => m.set(normalizeCombo(k), v);
  for (const [k, v] of Object.entries(DEFAULT_KEYS)) put(k, v);
  if (platform === 'darwin') for (const [k, v] of Object.entries(MAC_KEYS)) put(k, v);
  for (const c of commands.values()) for (const k of c.keys || []) if (!m.has(normalizeCombo(k))) put(k, c.name);
  for (const [k, v] of Object.entries(userKeys)) { if (v) m.set(k, v); else m.delete(k); }
  keymap = m;
}

// bindKey changes one key for this session (Configuration saves it in settings "keymap"); "" removes the key.
export function bindKey(combo, name) {
  userKeys[normalizeCombo(combo)] = name || '';
  rebuildKeys();
  changed();
}

export const userKeymap = () => ({ ...userKeys });

// commandForKey is the command a combo runs ("" when none). numpadFallback: also try Shift+= as Num+ etc.
export function commandForKey(combo, numpadFallback = false) {
  if (commands.size && keymap.size === 0) rebuildKeys();
  const c = keymap.get(combo);
  if (c) return c;
  if (numpadFallback && NUMPAD_FALLBACK[combo]) return keymap.get(NUMPAD_FALLBACK[combo]) || '';
  return '';
}

// keysFor lists the combos that run a command, TC keys first.
export function keysFor(name) {
  if (keymap.size === 0) rebuildKeys();
  const out = [];
  for (const [k, v] of keymap) if (v === name) out.push(k);
  return out;
}

export const allKeys = () => { if (keymap.size === 0) rebuildKeys(); return new Map(keymap); };

// ---- running ----

// The last commands asked for, newest last, so control mode can check that a key reached the command it should
// ("runs"): {name, how: 'ran' | 'disabled' | 'notready'}. Only names are kept, never arguments.
const recent = [];
const RECENT_MAX = 100;
function note(name, how) {
  recent.push({ name, how });
  if (recent.length > RECENT_MAX) recent.splice(0, recent.length - RECENT_MAX);
}
export const recentRuns = () => recent.slice();
export function clearRecentRuns() { recent.length = 0; }

export class NotReady extends Error {}

let app = null;
let before = null; // a hook: called with the name before any command runs (for "last command" and the like)
export function setApp(a, hook) { app = a; before = hook || null; }

// run runs a command by name. Unknown names reject with NotReady (the UI says "… is not ready yet").
export function run(name, args = {}) {
  const c = commands.get(name);
  note(name, !c ? 'notready' : c.enabled && !c.enabled(app) ? 'disabled' : 'ran');
  if (!c) return Promise.reject(new NotReady(`${name} is not ready yet.`));
  if (c.enabled && !c.enabled(app)) return Promise.resolve(false);
  before?.(name, args);
  return track(Promise.resolve().then(() => c.run(app, args ?? {})));
}

// rebuild keys once all features have registered (commands' own keys)
onChange(rebuildKeys);
