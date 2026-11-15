// The app object: the two panels, which one is active, app-wide state (hidden files, volumes), messages to the user,
// and small events features listen to. Every command gets it as its first argument:
//
//   command({name: 'cm_MatchSrc', label: 'Target = source', run: (app) => app.other.go(app.cur.path)});
//
// app.cur is the active ("source") panel, app.other the other ("target") one; app.left / app.right by side.
// Events: app.on('panel', (panel, {moved}) => …) after a folder was read or re-sorted; 'cursor' (panel); 'selection'
// (panel); 'active' (panel); 'ready' once start-up is done.

import { go, errText } from './go.js';
import { run as runCommand, NotReady, getCommand, keysFor } from './registry.js';
import { getSetting, setSetting } from './settings.js';
import { label as keyLabel } from './keys.js';
import { fitText } from './fitpath.js';
import { sessionSaver } from './session.js';

const listeners = new Map();
let flashTimer = 0;
let promptObserved = false; // the prompt is fitted again when the command line's width changes (updateTitle)
const session = sessionSaver({
  write: (v) => setSetting('session', v),
  stage: (v) => go.Settings.Stage('session', v),
});

export const app = {
  platform: 'darwin',
  sep: '/',
  home: '/',
  demo: false,
  panels: [],
  active: 0,
  showHidden: false,
  volumes: [],
  lastMessage: '',
  // openHandlers decide what Enter on a file does before the system opens it (archives as folders, .url files...):
  // app.openHandlers.push(async (panel, entry, path) => true when handled)
  openHandlers: [],

  get cur() { return this.panels[this.active]; },
  get other() { return this.panels[1 - this.active]; },
  get left() { return this.panels[0]; },
  get right() { return this.panels[1]; },

  // panel finds a panel by "left", "right", "active"/"source", "other"/"target" (default: active).
  panel(which) {
    switch (which) {
      case 'left': return this.panels[0];
      case 'right': return this.panels[1];
      case 'other': case 'target': return this.other;
      default: return this.cur;
    }
  },

  on(event, fn) {
    if (!listeners.has(event)) listeners.set(event, new Set());
    listeners.get(event).add(fn);
    return () => listeners.get(event).delete(fn);
  },

  emit(event, ...args) {
    for (const fn of listeners.get(event) || []) {
      try { fn(...args); } catch (e) { console.error(e); }
    }
  },

  setting: getSetting,
  setSetting,

  // setActive makes a panel the active one (and gives it the keyboard focus).
  setActive(p, { focus = true } = {}) {
    const i = typeof p === 'number' ? p : this.panels.indexOf(p);
    if (i < 0) return;
    const changed = i !== this.active;
    this.active = i;
    for (const panel of this.panels) {
      panel.el.classList.toggle('active', panel === this.cur);
      panel.renderRows();
    }
    if (focus && !document.querySelector('.mc-modal:not(.mc-modeless)')) { // (a modeless Find files window does not count)
      const ae = document.activeElement;
      if (!ae || ae === document.body || ae.closest?.('.panel') || ae.id === 'cmd') this.cur.$.list.focus({ preventScroll: true });
    }
    if (changed) { this.updateTitle(); this.emit('active', this.cur); this.saveSession(); }
  },

  // run runs a command and turns a failure into a message (never an alert). opts.quiet: no "not ready" message.
  async run(name, args = {}, opts = {}) {
    try {
      return await runCommand(name, args);
    } catch (e) {
      if (e instanceof NotReady) {
        if (!opts.quiet) this.flash(`${commandTitle(name)} is not ready yet.`);
      } else {
        console.error(e);
        this.flash(errText(e), 'err');
      }
      return false;
    }
  },

  // flash shows a short message above the function keys for a few seconds.
  flash(msg, kind = '') {
    const el = document.getElementById('flash');
    this.lastMessage = msg;
    if (!el) return;
    el.textContent = msg;
    el.className = kind;
    el.hidden = false;
    clearTimeout(flashTimer);
    flashTimer = setTimeout(() => { el.hidden = true; }, kind === 'err' ? 6000 : 3000);
  },

  // sepFor is the separator of a path: "\" for Windows paths, "/" for the rest (URLs and the demo too).
  sepFor(p) { return /^[a-z]:\\/i.test(p || '') || (p || '').startsWith('\\\\') ? '\\' : '/'; },

  // resolvePath makes a typed path absolute: "~" is home, a relative path is taken from the folder `from`.
  resolvePath(p, from) {
    p = p.trim();
    if (p === '~' || p.startsWith('~/') || p.startsWith('~\\')) return p;
    if (/^[a-z][a-z0-9+.-]*:\/\//i.test(p) || p.startsWith('/') || /^[a-z]:[\\/]?/i.test(p) || p.startsWith('\\\\')) return p;
    if (this.platform === 'windows' && p.startsWith('\\')) return from.slice(0, 2) + p; // "\x" is the root of this drive
    const sep = this.sepFor(from);
    return from.endsWith(sep) || from.endsWith('/') ? from + p : from + sep + p;
  },

  // volumeFor is the volume a path is on (the longest mount path that contains it), for the drive bar.
  volumeFor(p) {
    let best = null;
    for (const v of this.volumes) {
      if (v.kind === 'home' || v.kind === 'map') continue;
      const root = v.path;
      const within = p === root || p.startsWith(root.endsWith('/') || root.endsWith('\\') ? root : root + this.sepFor(root));
      const winWithin = this.platform === 'windows' && p.toLowerCase().startsWith(root.toLowerCase());
      if ((within || winWithin) && (!best || root.length > best.path.length)) best = v;
    }
    return best;
  },

  // driveFor is the drive bar button to highlight for a path: the most specific of Home and your mapped folders that
  // contains it (so inside a mapped folder its button lights up), else the volume it is on.
  driveFor(p) {
    let best = null;
    const win = this.platform === 'windows';
    for (const v of this.volumes) {
      if (v.kind !== 'home' && v.kind !== 'map') continue;
      const root = v.path;
      const a = win ? p.toLowerCase() : p, r = win ? root.toLowerCase() : root;
      const inside = a === r || a.startsWith(r.endsWith('/') || r.endsWith('\\') ? r : r + this.sepFor(root));
      if (inside && (!best || root.length > best.path.length)) best = v;
    }
    return best || this.volumeFor(p);
  },

  // drivePath is where a drive button (or a drive list entry) takes panel p, as in Total Commander: when the other panel
  // is on that same drive button (the one lit up there), p goes to the other panel's folder; otherwise to the
  // button's own folder. So with the left panel in ~/Downloads, Home on the right opens ~/Downloads, while Macintosh HD
  // opens /; on Windows, with the left panel in d:\games, d opens d:\games and c opens c:\.
  // The button of the drive p is already on (the one lit up on p's own bar) always takes p to that drive's root, also
  // in Total Commander: following the other panel only applies when switching to a different drive.
  drivePath(p, v) {
    const o = p.other;
    if (p.path && this.driveFor(p.path) === v) return v.path;
    // Options > Operation > Go to root when changing drive: always the drive's root, not the other panel's folder
    if (this.config?.values?.['Operation.AlwaysToRoot'] === '1') return v.path;
    if (o && o.path && this.driveFor(o.path) === v) return o.path;
    return v.path;
  },

  // loadVolumes reads the drives from Go and adds the folders you mapped to the drive bar (settings "driveMaps",
  // features/drivemaps.js) after them, so the real drives keep their places and keys.
  async loadVolumes() {
    let vols;
    try { vols = (await go.FS.Volumes()) || []; } catch { vols = []; }
    const maps = (getSetting('driveMaps', []) || []).filter((m) => m && m.path).map((m) => ({ name: m.name || m.path, path: m.path, kind: 'map' }));
    this.volumes = [...vols, ...maps];
    for (const p of this.panels) p.renderDrives(this.volumes);
  },

  // panelChanged is called by a panel after it read a folder or re-sorted (features listen to 'panel').
  panelChanged(p, info = {}) {
    if (info.moved) {
      p.renderDrives(this.volumes);
      if (/^(network|smb|nfs|afp):\/\//i.test(p.path)) p.renderDriveInfo({ name: 'Network' }); // the Network panel (#148)
      else go.FS.VolumeOf(p.path).then((v) => p.renderDriveInfo(v), () => p.renderDriveInfo(null));
    }
    if (p === this.cur) this.updateTitle();
    this.emit('panel', p, info);
    this.saveSession();
  },

  updateTitle() {
    const p = this.cur;
    if (!p?.path) return;
    // (a long path loses its start, not its end: fitText cuts it to fit, issue #129; the prompt is also right-to-left
    // with a <bdi> as a fallback, so it still loses the start where nothing measures)
    const pr = document.getElementById('prompt');
    const bdi = document.createElement('bdi');
    pr.replaceChildren(bdi);
    fitText(bdi, p.path + '>', pr);
    pr.title = p.path;
    if (!promptObserved && typeof ResizeObserver !== 'undefined' && pr.parentElement) {
      promptObserved = true;
      new ResizeObserver(() => { const b = pr.querySelector('bdi'); if (b && pr.title) fitText(b, pr.title + '>', pr); }).observe(pr.parentElement);
    }
    go.App.SetTitle(`mormiCommander – ${p.path}`).catch(() => {});
  },

  // saveSession remembers the folders, sorts and the active panel, so the next start comes back to them (written a
  // moment later, staged on the Go side at once: core/session.js).
  saveSession() {
    if (this.demo || !this.panels.length) return;
    const s = (p) => ({ path: p.path, sort: p.sort });
    session.schedule(() => ({ left: s(this.left), right: s(this.right), active: this.active, showHidden: this.showHidden, split: this.split ?? 0.5 }));
  },

  // flushSession writes a session change that is still waiting (before quitting, board issue #131).
  flushSession() { return Promise.resolve(session.flush()); },

  keyLabel(name) {
    const k = keysFor(name)[0];
    return k ? keyLabel(k, this.platform) : '';
  },
};

// commandTitle is a command's label without "…", for messages ("Copy (F5) is not ready yet.").
export function commandTitle(name) {
  const c = getCommand(name);
  const label = (c?.label || LABELS[name] || name).replace(/…$/, '');
  const k = app.keyLabel(name);
  return k ? `${label} (${k})` : label;
}

// Labels of commands that no feature registered yet, from the menu layout (menus.js fills this).
export const LABELS = {};
