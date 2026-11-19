// Commands > Open folder in new window (cm_OpenNewWindow, F-376): a second mormiCommander window starts with the
// active panel's folder on its left (Go: feature_newwindow.go, which starts the program again with /N and the same
// settings files). With Options > Operation > "Allow only one copy of the program" on, Go starts nothing and says why;
// the sentence is shown here.

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { tm } from '../core/i18n.js';

command({
  name: 'cm_OpenNewWindow', label: 'Open folder in new window',
  run: async (app) => {
    const dir = app.cur?.path;
    if (!dir) return;
    try {
      await go.ConfigNewWindow.Open(dir);
      app.flash(tm('Opening {path} in a new window…', { path: dir }));
    } catch (e) { app.flash(errText(e), 'err'); }
  },
});
