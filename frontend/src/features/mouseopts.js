// Mouse options (board issue #94): Options > Operation > Mouse selection mode, and Options > Drag & drop > Middle click
// on a folder. (The right-drag menu and the drag start distance are features/system-dnd.js's.)
//
// Mouse selection mode "Commander" (Total Commander's right-button mode): the right button selects. A right press
// toggles the row under it, dragging with it held selects (or unselects) every row it passes, and holding it still for
// a moment opens the right-click menu. "Standard" (the default) is the left-button selection the panels always had.
//
// Middle click on a folder (without moving: a middle drag still pans the list): open it in a new tab (the default), or
// in the other panel, or nothing.

import { Panel } from '../core/panel.js';

const opt = (app, k, d) => app.config?.values?.[k] ?? d;
const HOLD_MS = 600;

let rsel = null; // {p, on, last, timer, x, y, menuShown}

export function init(app) {
  const commander = () => opt(app, 'Operation.MouseMode', '1') === '0';

  Panel.rightPress = (p, ev, i) => {
    if (!commander()) return false;
    const e = p.entries[i];
    p.setCursor(i);
    const on = !!e && !e.up && !p.isSelected(e);
    if (e && !e.up) { p.select(e, on); p.selectionChanged(); }
    const args = { x: ev.clientX, y: ev.clientY, mouse: true, onRow: true };
    rsel = { p, on, last: i, x: ev.clientX, y: ev.clientY, menuShown: false, timer: 0 };
    rsel.timer = setTimeout(() => {
      if (!rsel || rsel.p !== p) return;
      rsel.menuShown = true;
      if (e && !e.up) { p.select(e, !on); p.selectionChanged(); } // (the press only meant the menu: undo the toggle)
      app.run('cm_ContextMenu', args, { quiet: true });
    }, HOLD_MS);
    return true;
  };

  const prevGate = Panel.contextGate;
  Panel.contextGate = (p, ev, args) => {
    if (commander()) return true; // (the menu opens by holding the button, never on the click)
    return prevGate ? prevGate(p, ev, args) : false;
  };

  document.addEventListener('pointermove', (ev) => {
    if (!rsel) return;
    if (!(ev.buttons & 2)) { clearTimeout(rsel.timer); rsel = null; return; }
    if (Math.hypot(ev.clientX - rsel.x, ev.clientY - rsel.y) > 4) clearTimeout(rsel.timer);
    const row = document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.('.row');
    if (!row || !rsel.p.el.contains(row)) return;
    const i = +row.dataset.i;
    if (i === rsel.last) return;
    const [a, b] = i > rsel.last ? [rsel.last + 1, i] : [i, rsel.last - 1];
    for (let k = a; k <= b; k++) rsel.p.select(rsel.p.entries[k], rsel.on);
    rsel.last = i;
    rsel.p.setCursor(i);
    rsel.p.selectionChanged();
  });
  document.addEventListener('pointerup', (ev) => {
    if (rsel && ev.button === 2) { clearTimeout(rsel.timer); rsel = null; }
  });

  Panel.middleClick = (p, ev) => {
    const what = opt(app, 'DragDrop.MiddleClick', '1');
    if (what === '0') return;
    const row = document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.('.row');
    if (!row || !p.el.contains(row)) return;
    const e = p.entries[+row.dataset.i];
    if (!e || e.up || !e.dir) return;
    app.setActive(p);
    p.setCursor(+row.dataset.i);
    if (what === '2') p.other.go(p.fullPath(e.name));
    else app.run('cm_OpenDirInNewTab');
  };
}
