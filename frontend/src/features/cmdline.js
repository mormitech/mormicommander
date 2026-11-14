// The command line's own commands (FEATURES.md section 10): Enter runs it; "cd <folder>" changes the active panel's
// folder (F-062, F-297); a cm_* name runs that command (F-301); Ctrl+E brings back the one before; Ctrl+Enter,
// Ctrl+Shift+Enter and Ctrl+P put names and paths into it. A server URL alone (sftp://host/x, ftp://host) opens in the
// active panel like "cd" with it (F-636). Running programs is a feature of its own: it adds a handler
// with addCmdlineHandler and gets every line that isn't "cd" or a cm_* name.

import { command, hasCommand, commandList, run } from '../core/registry.js';
import { cmdline, addCmdlineHandler } from '../core/cmdline.js';
import { app as theApp } from '../core/app.js';
import { go, errText } from '../core/go.js';

const quote = (s) => (/[\s"'&|;<>()$`\\]/.test(s) && theApp.platform !== 'windows') || /\s/.test(s) ? `"${s.replace(/"/g, '\\"')}"` : s;

// cd: "cd" alone goes home, "cd .." up, "cd /" or "cd \" to the root, "cd ~/x" and relative paths work. As in Total
// Commander, "cd\" and "cd.." also work without the space, and environment variables are expanded ("cd $HOME",
// "cd ${HOME}/x", "cd %HOME%"; board issue #130).
export const cdArg = (text) => {
  const m = /^cd(?:\s+(.*)|((?:\\|\/|\.\.).*))?$/i.exec(text);
  return m ? (m[1] ?? m[2] ?? '') : null;
};

// expandEnv asks Go for the values of the environment variables in s; s stays as it is when Go can't answer.
async function expandEnv(s) {
  if (!/[$%]/.test(s)) return s;
  try { return (await go.SystemLaunch.ExpandEnv(s)) || s; } catch { return s; }
}

// terminalWord: a line that asks for a terminal in the current folder. "terminal" and "term" everywhere, "cmd" on
// macOS and Linux (as on Windows, where "cmd" itself already opens one): {kind: 'terminal'}. A shell's name alone
// ("zsh", "bash"...) on macOS and Linux: {kind: 'shell'}, run in a terminal that stays open, since a shell needs a
// terminal window of its own. Anything else: null.
export const SHELLS = ['sh', 'bash', 'zsh', 'fish', 'dash', 'ksh', 'tcsh', 'csh', 'nu', 'pwsh', 'xonsh', 'elvish'];
export function terminalWord(text, platform) {
  const t = text.trim();
  if (/^(terminal|term)$/i.test(t)) return { kind: 'terminal' };
  if (platform === 'windows') return null;
  if (/^cmd$/i.test(t)) return { kind: 'terminal' };
  if (SHELLS.includes(t)) return { kind: 'shell' };
  return null;
}

addCmdlineHandler(async (text) => {
  const w = terminalWord(text, theApp.platform);
  if (!w) return false;
  const app = theApp, p = app.cur;
  if (w.kind === 'terminal') { await app.run('cm_ExecuteDOS'); return true; }
  try {
    await go.SystemLaunch.RunLine(text.trim(), p.path, true);
    app.flash(`Started ${text.trim()} in a terminal.`);
  } catch (e) { app.flash(errText(e), 'err'); }
  return true;
});

addCmdlineHandler(async (text) => {
  const raw = cdArg(text);
  if (raw === null) return false;
  const app = theApp, p = app.cur;
  let arg = await expandEnv(raw.trim().replace(/^"(.*)"$/, '$1'));
  if (!arg) arg = app.home;
  if (arg === '\\' || arg === '/') return run('cm_GoToRoot').then(() => true);
  if (arg === '..') return run('cm_GoToParent').then(() => true);
  await p.go(app.resolvePath(arg, p.path));
  return true;
});

// serverUrlArg: a line that is only a server URL ("sftp://bob@host/home/bob", "ftp://example.com", also in quotes)
// is the URL; it opens in the active panel like "cd <url>" (F-636), instead of being run as a program. Anything else:
// null.
export function serverUrlArg(text) {
  const t = text.trim().replace(/^"(.*)"$/, '$1');
  return /^(ftp|ftps|ftpes|sftp|scp|webdav|webdavs|dav|davs):\/\/[^\s/]+(\/.*)?$/i.test(t) && (!/\s/.test(t) || /^".*"$/.test(text.trim())) ? t : null;
}

addCmdlineHandler(async (text) => {
  const url = serverUrlArg(text);
  if (url === null) return false;
  const app = theApp, p = app.cur;
  await p.go(app.resolvePath(url, p.path));
  return true;
}, { first: true });

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
