// The frecency jump (F-867, board issue #27, ours): Ctrl+Alt+J (or Commands > Jump to a frequently used folder) opens a
// popup of the folders used most often and most recently, best first. Typing part of a folder name narrows it (the
// rules are in frecency-lib.js), Enter opens the folder in the active panel and Shift+Enter in a new tab. The list comes
// from Go (feature_frecency.go): the app's own frequently used folders, plus zoxide's folders when zoxide is installed
// and the setting frecencyZoxide is on (on by default; the last row of the popup switches it). zoxide's database is
// only read, so nothing new is stored. It is a new command; the folder history and the frequently used folders popup
// (Alt+Down, Alt+Shift+Down) work as before.

import { command } from '../core/registry.js';
import { go } from '../core/go.js';
import { showList } from '../core/popup.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { openInNewTab } from './tabs.js';
import { frecencyItems, frecencyItemMatch } from './frecency-lib.js';

const useZoxide = () => !!theApp.setting('frecencyZoxide', true);

async function showJump(p, { query = '' } = {}) {
  for (;;) {
    const zox = useZoxide();
    const list = await go.Frecency.List(zox, 500);
    if (list.problem) theApp.flash(list.problem, 'err');
    const items = frecencyItems(list.items);
    if (!items.length) items.push({ label: 'No folders counted yet: open a few folders first', disabled: true });
    items.push({ sep: true }, {
      label: `${zox ? '✓ ' : ''}Include zoxide's folders`, value: 'zoxide',
      detail: zox ? (list.zoxide ? `${list.imported} found` : 'zoxide not found') : 'off',
    });
    const r = await showList({
      title: 'Jump to a folder (type part of its name)', items, kind: 'frecency', filter: true, match: frecencyItemMatch, query,
      anchor: p.$.pathbar,
    });
    if (!r) return false;
    if (r.item.value === 'zoxide') { theApp.setSetting('frecencyZoxide', !zox); query = ''; continue; }
    return r.shift ? openInNewTab(p, r.item.value) : p.go(r.item.value);
  }
}

command({
  name: 'cm_FrecencyJump', label: 'Jump to a frequently used folder', short: 'Jump', keys: ['Ctrl+Alt+J'],
  menu: 'Commands', menuAfter: 'cm_FrequentlyUsedDirs',
  run: (app, args) => showJump(app.panel(args.panel || 'active'), { query: args.query ? String(args.query) : '' }),
});

// {"cmd": "frecency"} returns the ranked list the popup would show (for QA)
addControlCommand('frecency', async () => ({ ok: true, ...(await go.Frecency.List(useZoxide(), 500)) }));
