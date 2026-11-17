// The window and the app: quit (Alt+F4, asks while files are still being copied), minimize, maximize, About, website,
// text size, panels side by side or on top of each other.

import { command } from '../core/registry.js';
import { go } from '../core/go.js';
import { messageDialog } from '../core/dialogs.js';
import { syncSplitter, applySplit } from '../core/splitter.js';

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
// nextFontSize is the panel font size one step larger (d = 1) or smaller (d = -1), kept between 10 and 22 pixels.
export function nextFontSize(cur, d) {
  return Math.max(10, Math.min(22, (Number(cur) || 13) + d));
}
function zoom(app, d) {
  const px = nextFontSize(app.setting('fontSize', 13), d);
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
    syncSplitter();
    for (const p of app.panels) p.renderRows();
  },
});

// Panel sizes by command (F-009): cm_50Percent … cm_80Percent give the left (or top) panel that share, cm_20Percent …
// cm_40Percent the other way round. cm_100Percent shows the active panel alone, the whole width; again (or making the
// hidden panel active, with Tab) brings both back at the share they had.
function setShare(app, r) {
  endSolo(app);
  applySplit(r);
  app.saveSession();
  return true;
}
for (const n of [20, 30, 40, 50, 60, 70, 80]) {
  command({ name: `cm_${n}Percent`, label: `Panel sizes ${n}/${100 - n}`, checked: (app) => !solo && Math.abs((app.split ?? 0.5) - n / 100) < 0.005, run: (app) => setShare(app, n / 100) });
}
let solo = null; // the panel shown alone by cm_100Percent
function endSolo(app) {
  if (!solo) return;
  solo = null;
  document.getElementById('panels').classList.remove('mc-solo-left', 'mc-solo-right');
  for (const p of app.panels) p.renderRows();
}
command({
  name: 'cm_100Percent', label: 'Active panel alone', checked: () => !!solo,
  run: (app) => {
    if (solo) { endSolo(app); return true; }
    solo = app.cur;
    document.getElementById('panels').classList.add(`mc-solo-${solo.side}`);
    solo.renderRows();
    return true;
  },
});

export function init(app) {
  app.on('active', (p) => { if (solo && p !== solo) endSolo(app); });
  const px = app.setting('fontSize', 13);
  if (px !== 13) applyFont(app, px);
  if (app.setting('horizontal', false)) document.getElementById('panels').classList.add('horizontal');
  syncSplitter();
}
