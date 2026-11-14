// The "Not responding" notice (issue #16, F-NEW-20): when a panel has been reading a folder for 3 seconds (a network
// share whose server is gone, a removable disk that went away), a notice over its file list says so, with a Cancel
// button. Esc does the same as Cancel (cm_ClearCmdLine already stops a slow read, F-741): the read is given up and the
// panel stays on the folder it showed before. The other panel, tabs and menus keep working all the while, because the
// read runs in Go off the page's thread and Go stops waiting for it at once (internal/hang).
//
// This feature only watches the panels' loading flag, so it adds the notice without changing how a folder is read.
// A feature that shows a notice of its own for a read it knows is slow but alive (the branch view counting the files
// of a big tree) sets the panel's ownLoadingNotice flag, and this notice stays away while it is set.

import { go } from '../core/go.js';
import { esc } from '../core/dialogs.js';
import { slowStep, slowText } from '../core/slowread.js';

export function init(app) {
  const state = new Map(); // panel → {since, el}
  const tick = () => {
    const now = Date.now();
    for (const p of app.panels) {
      const s = state.get(p) || { since: null, el: null };
      const r = slowStep(s.since, p.loading && !p.ownLoadingNotice, now);
      s.since = r.since;
      if (r.show && !s.el) s.el = show(p);
      else if (!r.show && s.el) { s.el.remove(); s.el = null; }
      state.set(p, s);
    }
  };
  setInterval(tick, 250);
}

// show puts the notice over a panel's file list and returns it.
function show(p) {
  const t = slowText(p.path);
  const el = document.createElement('div');
  el.className = 'slowread';
  el.setAttribute('role', 'status');
  el.innerHTML = `<div class="slowread-title">${esc(t.title)}</div><div class="slowread-hint">${esc(t.hint)}</div><button type="button">Cancel</button>`;
  // the list takes mouse presses for the cursor and selection; the notice keeps its own
  el.addEventListener('mousedown', (ev) => ev.stopPropagation());
  el.querySelector('button').addEventListener('click', () => {
    go.FS.CancelList(p.side).catch(() => {});
    p.$.list.focus();
  });
  el.style.top = `${p.$.list.scrollTop + 30}px`; // the list scrolls; the notice goes where the user is looking
  p.$.list.append(el);
  return el;
}
