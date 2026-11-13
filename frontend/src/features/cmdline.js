// The command line's own commands (FEATURES.md section 10): Enter runs it; "cd <folder>" changes the active panel's
// folder (F-062, F-297); a cm_* name runs that command (F-301); Ctrl+E brings back the one before; Ctrl+Enter,
// Ctrl+Shift+Enter and Ctrl+P put names and paths into it. Running programs is a feature of its own: it adds a handler
// with addCmdlineHandler and gets every line that isn't "cd" or a cm_* name.

import { command, hasCommand, commandList, run } from '../core/registry.js';
import { cmdline, addCmdlineHandler } from '../core/cmdline.js';
import { app as theApp } from '../core/app.js';

const quote = (s) => (/[\s"'&|;<>()$`\\]/.test(s) && theApp.platform !== 'windows') || /\s/.test(s) ? `"${s.replace(/"/g, '\\"')}"` : s;

// cd: "cd" alone goes home, "cd .." up, "cd /" or "cd \" to the root, "cd ~/x" and relative paths work.
addCmdlineHandler(async (text) => {
  const m = /^cd(?:\s+(.*))?$/i.exec(text);
  if (!m) return false;
  const app = theApp, p = app.cur;
  let arg = (m[1] || '').trim().replace(/^"(.*)"$/, '$1');
  if (!arg) arg = app.home;
  if (arg === '\\' || arg === '/') return run('cm_GoToRoot').then(() => true);
  if (arg === '..') return run('cm_GoToParent').then(() => true);
  await p.go(app.resolvePath(arg, p.path));
  return true;
});

addCmdlineHandler(async (text) => {
  if (!/^cm_\w+$/i.test(text)) return false;
  // the name as typed, or with other upper/lower case ("cm_gotoparent" works too)
  const name = hasCommand(text) ? text : commandList().find((c) => c.name.toLowerCase() === text.toLowerCase())?.name || text;
  await theApp.run(name);
  return true;
});

command({
  name: 'cm_ExecuteCmdLine', label: 'Run the command line',
  run: async (app, { shift } = {}) => {
    const text = cmdline.text.trim();
    if (!await cmdline.submit({ shift })) app.flash(`Running programs from the command line is not ready yet ("${text}").`);
  },
});
command({ name: 'cm_FocusCmdLine', label: 'Command line', run: () => cmdline.focus() });
command({ name: 'cm_CmdLineHistoryPrev', label: 'Previous command line', run: () => cmdline.historyStep(-1) });
command({ name: 'cm_CmdLineHistoryNext', label: 'Next command line', run: () => cmdline.historyStep(1) });
command({
  name: 'cm_AddFilenameToCmdline', label: 'Name to command line',
  run: (app) => { const e = app.cur.cur; if (e && !e.up) cmdline.add(quote(e.name)); },
});
command({
  name: 'cm_AddPathAndNameToCmdline', label: 'Path and name to command line',
  run: (app) => { const e = app.cur.cur; if (e && !e.up) cmdline.add(quote(app.cur.fullPath(e.name))); },
});
command({ name: 'cm_AddPathToCmdline', label: 'Path to command line', run: (app) => cmdline.add(quote(app.cur.path)) });
