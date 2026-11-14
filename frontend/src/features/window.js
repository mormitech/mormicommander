// The window and the app: quit (Alt+F4, asks while files are still being copied), minimize, maximize, About, website,
// text size, panels side by side or on top of each other.

import { command } from '../core/registry.js';
import { go } from '../core/go.js';
import { messageDialog } from '../core/dialogs.js';

const SITE = 'https://github.com/mormitech/mormicommander';

command({
  name: 'cm_Exit', label: 'Quit',
  run: async (app) => {
    const busy = await go.Jobs.Running().catch(() => false);
    if (busy) {
      const b = await messageDialog({ Title: 'Quit mormiCommander?', Message: 'Files are still being copied, moved or deleted. Quit anyway? The job stops where it is.', Buttons: ['Quit', 'Cancel'], DefaultButton: 'Cancel', Kind: 'quit' });
      if (b !== 'Quit') return false;
    }
    await app.flushSession?.().catch(() => {}); // (the last change of the session is still waiting a moment: #131)
    await go.App.Quit();
    return true;
  },
});
command({ name: 'cm_Minimize', label: 'Minimize', run: () => go.App.Minimize() });
command({ name: 'cm_Maximize', label: 'Maximize', run: () => go.App.ToggleMaximize() });
command({ name: 'cm_Restore', label: 'Restore window size', run: () => go.App.ToggleMaximize() });
command({ name: 'cm_About', label: 'About mormiCommander', run: () => go.App.ShowAbout() });
command({ name: 'cm_VisitHomepage', label: 'Website', run: () => go.App.OpenURL(SITE) });
command({ name: 'cm_CheckForUpdates', label: 'Check for updates', run: () => go.App.OpenURL(`${SITE}/releases/latest`) });

// Text size: the font of the panels, saved (F-101). The row height follows it.
function zoom(app, d) {
  const px = Math.max(10, Math.min(22, (app.setting('fontSize', 13) || 13) + d));
  app.setSetting('fontSize', px);
  applyFont(app, px);
}
function applyFont(app, px) {
  const root = document.documentElement.style;
  root.setProperty('--font', `${px}px -apple-system, "Segoe UI", system-ui, sans-serif`);
  root.setProperty('--mono', `${px - 1}px ui-monospace, "SF Mono", Menlo, Consolas, monospace`);
  root.setProperty('--row-h', `${Math.round(px * 1.55)}px`);
  for (const p of app.panels) { p.measure(); p.render(); p.ensureVisible(); }
}
command({ name: 'cm_ZoomIn', label: 'Larger text', run: (app) => zoom(app, 1) });
command({ name: 'cm_ZoomOut', label: 'Smaller text', run: (app) => zoom(app, -1) });

command({
  name: 'cm_HorizontalPanels', label: 'Panels on top of each other', checked: () => document.getElementById('panels').classList.contains('horizontal'),
  run: (app) => {
    const on = document.getElementById('panels').classList.toggle('horizontal');
    app.setSetting('horizontal', on);
    for (const p of app.panels) p.renderRows();
  },
});

export function init(app) {
  const px = app.setting('fontSize', 13);
  if (px !== 13) applyFont(app, px);
  if (app.setting('horizontal', false)) document.getElementById('panels').classList.add('horizontal');
}
