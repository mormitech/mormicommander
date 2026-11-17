// Options > Operation: what comes back after a restart (board issue #94). Each is on by default, which is what the
// program always did:
//   Remember folders on exit       the panels open where they were (main.js; off: the start folders, home)
//   Remember folder history        Alt+Left and the Alt+Down list keep the folders of last time (features/tabs.js)
//   Remember command line history  the commands typed last time stay in the command line history
//   Remember panel layout and views  the sort of each panel, the share between them and their views
//   Save settings on exit          the session (folders, sorts, the active panel) is written as it changes and at the
//                                  end; off keeps the one saved last (core/app.js saveSession)

import { applySplit } from '../../core/splitter.js';
import { on } from './util.js';

export function initSessionOpts(app) {
  if (app.demo) return;
  if (!on('Operation.SaveCommands')) app.setSetting('cmdHistory', []);
  if (!on('Operation.SavePanels')) {
    for (const p of app.panels) p.sort = { key: 'name', desc: false };
    applySplit(0.5);
    app.setSetting('views', { ...(app.setting('views', {}) || {}), left: undefined, right: undefined });
  }
}
