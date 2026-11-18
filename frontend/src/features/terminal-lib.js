// The pure parts of the embedded terminal pane (features/terminal.js, issue #31): its settings, decoding what Go
// sends, whether the user has typed something not yet sent with Enter, and when a panel's folder is sent as a cd.

export const TERM_DEFAULTS = { follow: true, height: 240 };
export const MIN_HEIGHT = 80;

// termConfig is the "terminal" setting with the defaults filled in.
export function termConfig(raw) {
  const r = raw && typeof raw === 'object' ? raw : {};
  const h = Number(r.height);
  return {
    follow: typeof r.follow === 'boolean' ? r.follow : TERM_DEFAULTS.follow,
    height: Number.isFinite(h) && h >= MIN_HEIGHT ? Math.round(h) : TERM_DEFAULTS.height,
  };
}

// clampHeight keeps the pane between MIN_HEIGHT and what leaves the panels some room (avail is the height of the panels
// and the pane together).
export function clampHeight(h, avail) {
  const most = Math.max(MIN_HEIGHT, Math.round(avail - 120));
  return Math.min(most, Math.max(MIN_HEIGHT, Math.round(h)));
}

// b64ToBytes turns the base64 text of a "terminal-data" event into bytes for xterm.js.
export function b64ToBytes(s) {
  const bin = atob(String(s || ''));
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

// lineDirty tells, from the text just typed, whether the prompt now holds something the user typed and has not sent:
// Enter, Ctrl+C, Ctrl+U and Ctrl+D end or clear the line; anything after them (even an arrow key, which may bring
// back a history line) makes it dirty again. Following the panel waits while the line is dirty, so a cd never mixes
// into what the user is typing.
export function lineDirty(wasDirty, data) {
  const s = String(data ?? '');
  if (!s) return wasDirty;
  const end = Math.max(s.lastIndexOf('\r'), s.lastIndexOf('\x03'), s.lastIndexOf('\x15'), s.lastIndexOf('\x04'));
  if (end < 0) return true;
  return end < s.length - 1;
}

// followTarget is the folder to send as a cd, or '' when nothing is to be sent: the pane must be open with a running
// shell, following turned on, the folder local and not the one sent last, and the user's line empty.
export function followTarget({ open, id, follow, path, lastSent, dirty, isLocal }) {
  if (!open || !id || !follow || dirty || !path) return '';
  if (isLocal && !isLocal(path)) return '';
  if (path === lastSent) return '';
  return path;
}
