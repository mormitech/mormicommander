// The basic AI tools (issue #146): get_state (what the panels show), select_files (select by mask or by name, like
// Num + and the name lists) and set_panel_path (a panel goes to a folder). They are registered with core/aitools.js;
// a model reaches them through the Go service AI. find_files belongs to the Find files dialog (find-ai.js).
//
// select_files picks rows the way the Select files dialog does: the masks are matched in Go by internal/masks
// (Select.Group, Select.Names in feature_select.go), with the panel's rows as selection.js builds them, so the case
// rule, Unicode normalisation and the "folder/" masks are the same. A pattern selects files only, as Num + does,
// unless the option markDirectories is on or a mask names folders ("build/"); names select whatever has that name.

import { go } from '../core/go.js';
import { addAITool } from '../core/aitools.js';
import { app as theApp } from '../core/app.js';
import { rows } from './selection.js';
import { side, selectMode, nameList, maskExpr, maskKind, newSelection, expandHome } from './aitools-lib.js';

const SIDE = { type: 'string', enum: ['left', 'right', 'active'], description: 'Which panel; "active" (the default) is the panel the user works in.' };
const MAX_NAMES = 500; // selected names listed per panel in get_state (a selection of 20,000 files is told as a count)

const panelOf = (s) => theApp.panel(side(s));

function panelState(p) {
  const list = p.entries.filter((e) => !e.up);
  const sel = p.selectedEntries().map((e) => e.name);
  return {
    side: p.side, path: p.path, active: p === theApp.cur,
    cursor: p.cur?.name ?? '',
    files: list.filter((e) => !e.dir).length, folders: list.filter((e) => e.dir).length,
    selectedCount: sel.length,
    selected: sel.slice(0, MAX_NAMES),
    ...(sel.length > MAX_NAMES ? { selectedListCut: true } : {}),
    ...(p.filterLabel ? { filter: p.filterLabel } : {}),
  };
}

addAITool({
  name: 'get_state',
  description: 'Tells what mormiCommander shows: for both panels (left, right) the folder, whether it is the active panel, the name under the cursor, the numbers of files and folders, and the selected names; also the platform and the home folder.',
  schema: { type: 'object', properties: {} },
  run: () => ({ platform: theApp.platform, home: theApp.home, panels: theApp.panels.map(panelState) }),
});

addAITool({
  name: 'select_files',
  description: 'Selects files in a panel, as the Select files dialog (Num +) does. Give a pattern (wildcards * and ?, several separated by ";", like "*.pdf;*.docx"; a mask ending in "/" means folders) or a list of names in the panel\'s folder, or both. mode "replace" (default) makes these the selection, "add" adds them, "remove" unselects them. Answers how many entries are selected afterwards.',
  schema: {
    type: 'object',
    properties: {
      side: SIDE,
      pattern: { type: 'string', description: 'Wildcard masks separated by ";", e.g. "*.jpg;*.png".' },
      names: { type: 'array', items: { type: 'string' }, description: 'Names of entries in the panel\'s folder (not paths).' },
      mode: { type: 'string', enum: ['replace', 'add', 'remove'], description: 'replace (default), add or remove.' },
    },
  },
  run: async (args) => {
    const p = panelOf(args.side);
    if (!p?.path) throw new Error('That panel shows no folder yet.');
    const expr = maskExpr(args.pattern);
    const names = nameList(args.names);
    if (!expr && !names.length) throw new Error('Give a pattern or a list of names to select.');
    const r = rows(theApp, p);
    const idx = new Set();
    if (expr) {
      const filters = theApp.setting('selectFilters', {}) || {};
      for (const i of (await go.Select.Group(r, expr, maskKind(expr, !!theApp.setting('markDirectories', false)), filters)) || []) idx.add(i);
    }
    if (names.length) for (const i of (await go.Select.Names(r, names)) || []) idx.add(i);
    const matched = [...idx].map((i) => p.entries[i]).filter((e) => e && !e.up).map((e) => e.name);
    const mode = selectMode(args.mode);
    const next = newSelection(p.selected, matched, mode);
    p.selected = next;
    p.selectionChanged();
    const sel = p.selectedEntries().length;
    return `${matched.length} ${matched.length === 1 ? 'entry' : 'entries'} matched; ${sel} selected now in the ${p.side} panel (${p.path}).`;
  },
});

addAITool({
  name: 'set_panel_path',
  description: 'Shows a folder in a panel (like typing it in the path bar). "~" is the home folder; a relative path is taken from the panel\'s current folder. Answers the folder the panel shows afterwards.',
  schema: {
    type: 'object',
    properties: { side: SIDE, path: { type: 'string', description: 'The folder to show.' } },
    required: ['path'],
  },
  run: async (args) => {
    const p = panelOf(args.side);
    const raw = expandHome(args.path, theApp.home, theApp.sep);
    if (!raw) throw new Error('Give the folder to show (path).');
    const path = p.path ? theApp.resolvePath(raw, p.path) : raw;
    const before = theApp.lastMessage;
    if (!(await p.go(path))) {
      const why = theApp.lastMessage !== before ? theApp.lastMessage : '';
      throw new Error(why || `Could not open ${path}.`);
    }
    return `The ${p.side} panel shows ${p.path}.`;
  },
});
