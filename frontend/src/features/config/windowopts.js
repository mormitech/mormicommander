// Options > Misc, window part: "Keep the window on top of other windows" (F-748) and "Show job progress on the Dock
// icon" (F-749). Both are off by default, and then nothing is sent to the window at all.

import { go } from '../../core/go.js';
import { onJobs, jobList } from '../../core/jobs.js';
import { dockLabel } from '../dockprogress-lib.js';
import { on } from './util.js';

let onTop = false;
let badge = '';

// applyWindowOpts follows the options after they are read or changed.
export function applyWindowOpts() {
  const top = on('Misc.AlwaysOnTop');
  if (top !== onTop) {
    onTop = top;
    go.App.SetAlwaysOnTop(top).catch(() => {});
  }
  paintBadge();
}

function paintBadge() {
  const label = on('Misc.DockProgress') ? dockLabel(jobList()) : '';
  if (label === badge) return;
  badge = label;
  go.App.SetDockBadge(label).catch(() => {});
}

export function initWindowOpts() {
  onJobs(paintBadge);
}
