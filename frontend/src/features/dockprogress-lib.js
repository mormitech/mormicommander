// The Dock badge of running jobs (F-749, Options > Misc > Show job progress on the Dock icon): what the badge says for
// the jobs of the transfer list. A pure function, so it tests without a page; features/config/windowopts.js shows it.

const LIVE = new Set(['queued', 'running', 'paused', 'asking']);

// dockLabel is "42%" for the jobs still at work (all of them together, by bytes when they know their bytes, else by
// files), and "" when none is.
export function dockLabel(list) {
  const live = (list || []).filter((p) => p && LIVE.has(p.state));
  if (!live.length) return '';
  const bytes = live.reduce((s, p) => s + (p.bytes || 0), 0);
  let f;
  if (bytes > 0) f = live.reduce((s, p) => s + (p.bytesDone || 0), 0) / bytes;
  else {
    const files = live.reduce((s, p) => s + (p.files || 0), 0);
    f = files > 0 ? live.reduce((s, p) => s + (p.filesDone || 0), 0) / files : 0;
  }
  return `${Math.floor(Math.max(0, Math.min(1, f)) * 100)}%`;
}
