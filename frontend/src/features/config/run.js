// Running a button, a Start menu entry or a user command (em_): Total Commander gives all three the same four fields
// (command, parameters, start folder, window state). Internal commands (cm_*, em_*) run through the registry; "cd path"
// changes the folder; a .bar file opens as a drop-down sub-bar (F-308); anything else is a program, started by Go after
// internal/params filled in the placeholders (F-310): %[question;default] asks first, a leading "?" shows the filled-in
// parameters for editing before anything runs (F-814), and files dropped on a button are added (F-307).

import { go, errText } from '../../core/go.js';
import { inputDialog, formDialog, esc } from '../../core/dialogs.js';
import { showList } from '../../core/popup.js';
import { panelCtx } from './util.js';

// The default bar of internal/config names a few commands differently from our registry; TC itself knows both.
export const ALIASES = { cm_GoToPrev: 'cm_GoToPreviousDir', cm_GoToNext: 'cm_GoToNextDir', cm_MultiRename: 'cm_MultiRenameFiles', cm_Back: 'cm_GoToPreviousDir', cm_Forward: 'cm_GoToNextDir' };
export const realCommand = (name) => ALIASES[name] || name;

const isInternal = (cmd) => /^(cm|em)_\w+$/i.test(cmd);

// askQuestions shows the %[...] questions of a button in one form; null when cancelled.
async function askQuestions(list, title) {
  const html = list.map((q, i) => `<div class="mc-row"><label for="mc-config-ask-${i}">${esc(q.caption || 'Value')}</label><input id="mc-config-ask-${i}" type="text" spellcheck="false" value="${esc(q.default || '')}"${i === 0 ? ' autofocus' : ''}></div>`).join('');
  return formDialog(title, html, (box) => {
    const out = {};
    list.forEach((q, i) => { out[q.caption] = box.querySelector(`#mc-config-ask-${i}`).value; });
    return out;
  }, null, { okLabel: 'Run', kind: 'button-ask' });
}

// openSubBar shows the buttons of another .bar file as a drop-down list and runs the one chosen.
export async function openSubBar(app, file, anchor) {
  let bar;
  try { bar = await go.ConfigBar.LoadBar(file); } catch (e) { app.flash(errText(e), 'err'); return false; }
  const items = (bar.buttons || []).map((b) => (b.sep ? { sep: true } : { label: b.tooltip || b.cmd, detail: b.subBar ? '▸' : '', value: b }));
  if (!items.length) { app.flash('This button bar is empty.'); return false; }
  const r = await showList({ title: bar.path.split(/[\\/]/).pop(), items, anchor, kind: 'subbar' });
  if (!r) return false;
  return runButton(app, r.item.value, { anchor });
}

// runButton runs a button-like {cmd, params, path, iconic}.
export async function runButton(app, btn, { dropped = [], anchor = null, title = '' } = {}) {
  if (!btn || btn.sep) return false;
  let cmd = String(btn.cmd || '').trim();
  if (!cmd) return false;
  if (btn.subBar || /\.bar("?\s|"?$)/i.test(cmd)) return openSubBar(app, cmd, anchor);
  const cd = cmd.match(/^cd\s+(.+)$/i);
  if (cd && !btn.params) {
    const target = cd[1].trim().replace(/^"|"$/g, '');
    return app.cur.go(app.resolvePath(target, app.cur.path));
  }
  if (isInternal(cmd) && !btn.params && !dropped.length) return app.run(realCommand(cmd));
  const ctx = panelCtx(app);
  let answers = {};
  let r;
  for (let round = 0; round < 3; round++) {
    r = await go.ConfigBar.Prepare({ ...btn, cmd, sep: false, subBar: false }, ctx.source, ctx.target, ctx.sourceIsRight, answers, dropped);
    if (!r.ask?.length) break;
    const got = await askQuestions(r.ask, title || btn.tooltip || cmd);
    if (!got) return false;
    answers = { ...answers, ...got };
  }
  if (!r || r.ask?.length || r.canceled) return false;
  if (r.internal) return app.run(realCommand(r.program), { params: r.line });
  let line = r.line, edited = false;
  if (r.prompt) {
    const v = await inputDialog({ title: title || btn.tooltip || 'Run program', label: `Parameters for ${r.program}`, value: r.line, okLabel: 'Run', kind: 'button-params' });
    if (v === null) { go.ConfigBar.Discard(r.id); return false; }
    line = v;
    edited = true;
  }
  await go.ConfigBar.Run(r.id, line, edited);
  return true;
}
