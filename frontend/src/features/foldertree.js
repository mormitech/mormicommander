// An expanding folder tree widget, shared by the Alt+F10 popup (cm_CDtree with Options > Tree > "Folder tree popup
// shows the whole drive as an expanding tree", F-171) and the separate tree pane (Layout.SeparateTreePane, F-025).
// It reads one folder's subfolders at a time, when that folder is opened, so a big drive or a server opens at once.
//
// Keys (key(ev) answers whether it used the key): Up/Down/PgUp/PgDn/Home/End move, Right opens a folder (or goes to
// its first subfolder), Left closes it (or goes to the folder above), Space opens or closes, Enter chooses (onGo),
// typing filters the folders read so far (when filter is on), Backspace takes a letter back (or goes up), Esc clears
// the filter (and is left to the caller when there is none). The mouse: a click chooses a row (onPick), a click on the
// arrow opens or closes it, a double click chooses it as Enter does.

import './foldertree.css';
import { go, errText } from '../core/go.js';
import { esc } from '../core/dialogs.js';
import { F } from '../core/format.js';
import { dirNames, joinPath } from './reserved-more-lib.js';
import { makeNode, setKids, visibleNodes, chainBelow, findKid, samePath, nameMatches } from './foldertree-lib.js';

let seq = 0;

// folderReader is the default reader: the folders of path through the app's file systems (local, archives, servers).
export function folderReader(app, { hidden = () => false } = {}) {
  return async (path) => {
    const l = await go.FS.List(`foldertree-${++seq}`, path);
    const here = l.path || path;
    const sep = app.sepFor(here);
    return dirNames(l.entries, { showHidden: app.showHidden || hidden(), dirFlag: F.DIR, hiddenFlag: F.HIDDEN }).map((n) => ({ name: n, path: joinPath(here, n, sep) }));
  };
}

// rootOf is the top of the tree for a folder: its drive or volume root (the server's root for a server).
export async function rootOf(path) {
  try { const r = await go.FS.Root(path); if (r) return r; } catch { /* below */ }
  const m = /^([a-z][a-z0-9+.-]*:\/\/[^/]*\/)/i.exec(path) || /^([a-z]:\\)/i.exec(path);
  return m ? m[1] : (String(path).startsWith('/') ? '/' : path);
}

export class FolderTree {
  // el: the element the tree draws into; read(path) → [{name, path}]; onGo(node, {shift}) for Enter / double click;
  // onPick(node) for a click; onMove(node) after the highlight moved; onFilter(query) after the filter changed;
  // onError(text) when a folder can't be read.
  constructor({ el, read, onGo = () => {}, onPick = null, onMove = null, onFilter = null, onError = null, filter = true, ignoreCase = false }) {
    Object.assign(this, { el, read, onGo, onPick, onMove, onFilter, onError, filter, ignoreCase });
    this.root = null;
    this.nodes = [];
    this.at = 0;
    this.query = '';
    el.classList.add('mc-ft');
    el.addEventListener('mousedown', (ev) => this.mouse(ev));
    el.addEventListener('dblclick', (ev) => { const i = this.rowAt(ev); if (i >= 0) this.onGo(this.nodes[i], { shift: ev.shiftKey }); });
  }

  get cur() { return this.nodes[this.at] || null; }

  // openTo starts the tree at rootPath and opens the way down to path, with the highlight on it.
  async openTo(rootPath, path) {
    this.root = makeNode(rootPath, rootPath, null);
    this.query = '';
    await this.reveal(path);
  }

  // reveal opens the way from the root down to path (as far as it exists) and path itself, and puts the highlight on
  // it; false when path is not under the root. Folders read before are not read again.
  async reveal(path) {
    if (!this.root) return false;
    const chain = chainBelow(this.root.path, path, this.ignoreCase);
    if (!chain.length && !samePath(this.root.path, path, this.ignoreCase)) return false;
    let node = this.root;
    await this.expand(node);
    for (const want of chain) {
      const k = findKid(node, want, this.ignoreCase);
      if (!k) break;
      node = k;
      await this.expand(node); // (the folder itself opens too, so its subfolders show at once)
    }
    this.refresh(node);
    return true;
  }

  async expand(n) {
    if (!n.kids) {
      n.loading = true;
      try { setKids(n, await this.read(n.path)); } catch (e) { n.kids = []; this.onError?.(errText(e)); }
      n.loading = false;
    }
    n.open = true;
  }

  // refresh works out the rows again, keeping the highlight on keep (or the folder it was on).
  refresh(keep = this.cur) {
    this.nodes = visibleNodes(this.root, this.query);
    const i = keep ? this.nodes.indexOf(keep) : -1;
    this.at = i >= 0 ? i : Math.min(this.at, Math.max(0, this.nodes.length - 1));
    this.render();
  }

  render() {
    const html = this.nodes.map((n, i) => {
      const tw = n.loading ? '…' : n.kids && n.kids.length === 0 ? '' : n.open && !this.query ? '▾' : '▸';
      return `<div class="mc-ft-row${i === this.at ? ' on' : ''}" data-i="${i}" role="treeitem" aria-level="${n.depth + 1}"${n.kids?.length ? ` aria-expanded="${n.open}"` : ''}${i === this.at ? ' aria-selected="true"' : ''} style="padding-left:${6 + n.depth * 14}px" title="${esc(n.path)}"><span class="tw" data-tw="1">${tw}</span><span class="nm">${esc(n.depth === 0 ? n.path : n.name)}</span></div>`;
    }).join('');
    this.el.innerHTML = html;
    this.el.setAttribute('role', 'tree');
    this.el.querySelector('.mc-ft-row.on')?.scrollIntoView?.({ block: 'nearest' });
  }

  move(i) {
    i = Math.max(0, Math.min(this.nodes.length - 1, i));
    if (i === this.at) return;
    this.at = i;
    this.render();
    this.onMove?.(this.cur);
  }

  async toggle(n, open) {
    if (!n) return;
    if (open) { n.loading = !n.kids; this.render(); await this.expand(n); } else n.open = false;
    this.refresh(this.cur);
  }

  setQuery(q) {
    this.query = q;
    this.refresh(this.cur);
    // the highlight goes to the first matching folder when its own folder does not match
    if (q && !(this.cur?.depth > 0 && nameMatches(this.cur.name, q))) {
      const i = this.nodes.findIndex((n) => n.depth > 0 && nameMatches(n.name, q));
      if (i >= 0) { this.at = i; this.render(); }
    }
    this.onFilter?.(q);
  }

  page() {
    const row = this.el.querySelector('.mc-ft-row');
    const h = row?.offsetHeight || 20;
    return Math.max(1, Math.floor((this.el.clientHeight || 300) / h) - 1);
  }

  key(ev) {
    const n = this.cur;
    const plain = !ev.ctrlKey && !ev.metaKey && !ev.altKey;
    switch (ev.key) {
      case 'ArrowUp': this.move(this.at - 1); return true;
      case 'ArrowDown': this.move(this.at + 1); return true;
      case 'PageUp': this.move(this.at - this.page()); return true;
      case 'PageDown': this.move(this.at + this.page()); return true;
      case 'Home': this.move(0); return true;
      case 'End': this.move(this.nodes.length - 1); return true;
      case 'ArrowRight':
        if (!n) return true;
        if (!n.open || this.query) { if (this.query) this.setQuery(''); this.toggle(n, true); } else if (n.kids?.length) this.move(this.at + 1);
        return true;
      case 'ArrowLeft':
        if (!n) return true;
        if (n.open && n.kids?.length && !this.query) this.toggle(n, false);
        else if (n.parent) this.move(this.nodes.indexOf(n.parent));
        return true;
      case 'Enter': if (n) this.onGo(n, { shift: ev.shiftKey }); return true;
      case 'Escape':
        if (this.query) { this.setQuery(''); return true; }
        return false;
      case 'Backspace':
        if (this.query) this.setQuery(this.query.slice(0, -1));
        else if (n?.parent) this.move(this.nodes.indexOf(n.parent));
        return true;
      default: break;
    }
    if (ev.key === ' ' && plain && !this.query) { if (n) this.toggle(n, !n.open); return true; }
    if (this.filter && plain && ev.key.length === 1) { this.setQuery(this.query + ev.key); return true; }
    return false;
  }

  rowAt(ev) {
    const d = ev.target?.closest?.('.mc-ft-row');
    return d ? +d.dataset.i : -1;
  }

  mouse(ev) {
    if (ev.button !== 0) return;
    const i = this.rowAt(ev);
    if (i < 0) return;
    const n = this.nodes[i];
    if (ev.target.closest('[data-tw]')) {
      ev.preventDefault();
      this.at = i;
      const open = !n.open || !!this.query;
      if (this.query) { this.query = ''; this.onFilter?.(''); }
      this.toggle(n, open);
      return;
    }
    this.at = i;
    this.render();
    this.onPick?.(n);
  }
}
