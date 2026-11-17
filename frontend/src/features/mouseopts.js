// Mouse options (board issue #94): Options > Operation > Mouse selection mode, and Options > Drag & drop > Middle click
// on a folder. (The right-drag menu and the drag start distance are features/system-dnd.js's.)
//
// Mouse selection mode "Commander" (Total Commander's right-button mode): the right button selects. A right press
// toggles the row under it, dragging with it held selects (or unselects) every row it passes, and holding it still for
// a moment opens the right-click menu. In the same mode a left drag that starts on a row that is not selected and
// goes up or down selects the rows it passes (F-135, TC style); a left drag that goes sideways, or starts on a
// selected row, still drags the files. "Standard" (the default) is the left-button selection the panels always had.
//
// Middle click on a folder (without moving: a middle drag still pans the list): open it in a new tab (the default), or
// in the other panel, or nothing.

import { Panel } from '../core/panel.js';

const opt = (app, k, d) => app.config?.values?.[k] ?? d;
const HOLD_MS = 600;

let rsel = null; // {p, on, last, timer, x, y, menuShown}
let lsel = null; // the left drag of Commander mode: {p, start, last, x, y, dx, dy, active}

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

  // left drag (Commander mode only): a press on a row that is not selected may become a drag selection
  for (const p of app.panels) {
    p.$.list.addEventListener('mousedown', (ev) => {
      lsel = null;
      if (ev.button !== 0 || ev.detail > 1 || ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.altKey || !commander()) return;
      const row = ev.target.closest?.('.row');
      if (!row) return;
      const i = +row.dataset.i, e = p.entries[i];
      if (!e || e.up || p.isSelected(e)) return;
      lsel = { p, start: i, last: i, x: ev.clientX, y: ev.clientY, dx: 0, dy: 0, active: false };
    });
    // the drag of the files starts first: up or down it becomes the selection instead (sideways it stays a drag)
    p.el.addEventListener('dragstart', (ev) => {
      if (!lsel || lsel.p !== p) return;
      if (Math.abs(lsel.dy) > Math.abs(lsel.dx)) {
        ev.preventDefault();
        ev.stopImmediatePropagation();
        lsel.active = true;
        dragSelect(lsel.start);
      } else lsel = null;
    }, true);
  }
  // dragSelect selects the rows from the start row to row i; rows the drag passed before and has left lose it again
  const dragSelect = (i) => {
    const { p, start, last } = lsel;
    const [a, b] = i > start ? [start, i] : [i, start];
    for (let k = Math.min(a, last); k <= Math.max(b, last); k++) {
      const e = p.entries[k];
      if (e && !e.up) p.select(e, k >= a && k <= b);
    }
    lsel.last = i;
    p.setCursor(i);
    p.selectionChanged();
  };

  document.addEventListener('pointermove', (ev) => {
    if (lsel) {
      if (!(ev.buttons & 1)) lsel = null;
      else {
        lsel.dx = ev.clientX - lsel.x;
        lsel.dy = ev.clientY - lsel.y;
        const row = lsel.active && document.elementFromPoint(ev.clientX, ev.clientY)?.closest?.('.row');
        if (row && lsel.p.el.contains(row) && +row.dataset.i !== lsel.last) dragSelect(+row.dataset.i);
      }
    }
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
    if (lsel && ev.button === 0) lsel = null;
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
