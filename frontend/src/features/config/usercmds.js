// User commands (em_name in usercmd.ini, F-311): each becomes a command of the registry, so a button, a Start menu
// entry, a key (Options > Keyboard shortcuts) or the command line ("em_backup" + Enter) runs it. Running one is running
// its command like a button: a program with its parameters and start folder, a cm_ command, or "cd folder".

import { command, getCommand } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { runButton } from './run.js';
import { tm } from '../../core/i18n.js';

const known = new Map(); // em_ name (lower case) → the user command as last read

export async function registerUserCommands(app) {
  let list = [];
  try { list = (await go.ConfigBar.UserCommands()) || []; } catch { list = []; }
  const seen = new Set();
  for (const u of list) {
    const key = u.name.toLowerCase();
    seen.add(key);
    known.set(key, u);
    const label = u.menu || u.name;
    const c = getCommand(u.name);
    if (c?.mcConfigUser) { c.label = label; continue; } // already registered: the label follows, run reads `known`
    if (c) continue; // a feature's own command of that name wins
    command({
      name: u.name, label, mcConfigUser: true,
      run: (a) => {
        const cur = known.get(key);
        if (!cur) { a.flash(tm('The user command {name} was deleted.', { name: u.name })); return false; }
        return runButton(a, { cmd: cur.cmd, params: cur.params, path: cur.path, iconic: cur.iconic, tooltip: cur.menu || cur.name }, { title: cur.menu || cur.name });
      },
    });
  }
  for (const k of [...known.keys()]) if (!seen.has(k)) known.delete(k);
  return list;
}
