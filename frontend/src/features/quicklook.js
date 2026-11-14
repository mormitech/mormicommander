// Native Quick Look (board issue #19, F-863, macOS only, ours): ⌘Y opens the system's Quick Look panel, the one Finder
// opens with Space, on the file under the cursor, and while the panel is open it follows the cursor of the active
// panel. ⌘Y again or Esc closes it (Esc only while the panel is open, so Esc keeps its usual meaning otherwise).
//
// Space keeps its Total Commander meaning (select). With the Mac-native keyboard preset, Space and ⌘Y show the file in
// the built-in viewer as before; the setting "quickLookSpace" (the first-run setup page, off by default) makes them open
// Quick Look instead (core/keypresets.js). The panel itself is native code in Go (feature_quicklook.go).

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { addKeyHandler } from '../core/keyboard.js';
import { addControlCommand } from '../core/control.js';
import { quickLookPaths, FOLLOW_DELAY } from './quicklook-lib.js';

let open = false; // the panel was opened from here and has not been seen closed since
let off = []; // unsubscribe functions of the cursor following
let timer = 0;

function stopFollowing() {
  open = false;
  clearTimeout(timer);
  off.forEach((f) => f());
  off = [];
}

// follow switches the open panel to the file under the active panel's cursor, after a short rest.
function follow(app) {
  clearTimeout(timer);
  timer = setTimeout(() => {
    if (!open || !app.cur) return;
    go.QuickLook.Update(quickLookPaths(app.cur)).then((still) => { if (!still) stopFollowing(); }, () => {});
  }, FOLLOW_DELAY);
}

async function closeQuickLook() {
  stopFollowing();
  await go.QuickLook.Close().catch(() => {});
}

async function toggle(app) {
  // the user may have closed the panel with its own button: then ⌘Y opens it again
  if (open && (await go.QuickLook.IsOpen().catch(() => false))) return closeQuickLook();
  stopFollowing();
  const paths = quickLookPaths(app.cur);
  if (!paths.length) { app.flash('There is no file to show.'); return; }
  try {
    await go.QuickLook.Show(paths);
  } catch (err) {
    app.flash(errText(err), 'err');
    return;
  }
  open = true;
  const again = (p) => { if (!p || p === app.cur) follow(app); };
  off.push(app.on('cursor', again), app.on('panel', again), app.on('active', () => follow(app)));
}

command({
  name: 'cm_QuickLook', label: 'Quick Look', menu: 'Show', menuAfter: 'cm_SrcQuickInternalOnly',
  enabled: (app) => app.platform === 'darwin',
  checked: () => open,
  run: (app) => toggle(app),
});

export function init(app) {
  // Esc closes the panel, only while it is open
  addKeyHandler((ev, combo) => {
    if (!open || combo !== 'Esc') return false;
    closeQuickLook();
    return true;
  });
}

// Control mode: {"cmd": "quicklook_state"} tells whether the page thinks the panel is open (Go's "quicklook" asks the
// system).
addControlCommand('quicklook_state', async () => ({ ok: true, following: open }));
