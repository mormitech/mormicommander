// Compare by content (cm_CompareFilesByContent, FEATURES.md F-321, F-181 and section 26, behaviour from docs/spec
// area 4 section 5 and area 3 section 3.5): two files side by side with their line numbers, differences highlighted
// down to the changed words, next/previous difference, ignore case / blanks / repeated lines, binary (hex) mode, an
// edit mode with copy left/right, undo and save, and an encoding choice per side. The diff, the edits and saving are
// internal/compare's (feature_cmpfiles.go); this file draws and takes the keys.
//
// Keys in the window: F8 / Ctrl+Down next difference, Shift+F8 / Ctrl+Up previous, F2 recompare, Tab switches the side,
// Ctrl+E edit mode, Enter (edit mode) edits the line, Ctrl+Delete deletes it, Alt+Right / Alt+Left copy the current
// difference to the right / left, Ctrl+S saves the side, Ctrl+Z / Ctrl+Y undo / redo, Ctrl+F find, F3 find again,
// Left / Right scroll sideways, Esc closes (asking about unsaved changes).
//
// The window is a dialog of kind "compare" for control mode; the control command "compare" tells its state.

import './comparefiles.css';
import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { esc, messageDialog, inputDialog } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { size, plural } from '../core/format.js';
import { openWindow } from './synccompare-ui.js';

const ROW_H = 18;     // px, the fixed height of a text or hex row (the list draws only what is in view)
const HEX_W = 16;     // bytes per hex row
const PAGE = 65536;   // bytes read at a time for the hex view

let current = null;   // the state of the open compare window, for control mode

const settings = (app) => app.setting('compare', {}) || {};
const saveSettings = (app, patch) => app.setSetting('compare', { ...settings(app), ...patch });
const baseName = (p) => String(p).split(/[\\/]/).pop();

// compareFiles opens the compare window for two files (or starts the external compare program when one is set).
// It resolves when the window is closed, with {saved: [left, right]} saying which side was saved.
export async function compareFiles(app, left, right, { force = false } = {}) {
  const ext = (settings(app).externalTool || '').trim();
  if (ext && !force) {
    await go.CompareFiles.ExternalStart(ext, left, right).catch((e) => { throw new Error(errText(e)); });
    return { saved: [false, false], external: true };
  }
  const same = await go.CompareFiles.Same(left, right).catch((e) => { throw new Error(errText(e)); });
  if (same) {
    const b = await messageDialog({ Title: 'Compare by content', Message: `The two files are identical.\n\n${left}\n${right}`, Buttons: ['OK', 'Show anyway'], Kind: 'compare-identical' });
    if (b !== 'Show anyway') return { saved: [false, false] };
  }
  const opts = { ignoreCase: false, whitespace: 'exact', ignoreRepeated: false, resyncLines: 1, binary: false, ansi: '', ...(settings(app).options || {}) };
  opts.binary = false; // binary is chosen per window, never remembered
  const view = await go.CompareFiles.Open(left, right, opts).catch((e) => { throw new Error(errText(e)); });
  const encodings = await go.CompareFiles.Encodings().catch(() => ['Auto', 'UTF-8']);
  return runWindow(app, view, encodings);
}

function runWindow(app, first, encodings) {
  const st = {
    v: first, id: first.id, row: 0, side: 0, block: -1, edit: false, x: 0, find: '', saved: [false, false],
    bytes: new Map(), // hex view pages: page index → {a, b}
    editing: null,    // {row, side, input}: the line being edited
  };
  current = st;
  let box, body, spacer, rowsEl, status, render;

  const call = async (fn) => {
    try {
      const v = await fn();
      if (v) setView(v);
      return true;
    } catch (e) {
      app.flash(errText(e), 'err');
      return false;
    }
  };
  const setView = (v) => {
    const modeChanged = v.binary !== st.v.binary;
    st.v = v;
    if (modeChanged) { st.bytes.clear(); st.row = 0; }
    st.row = Math.min(st.row, Math.max(0, rowCount() - 1));
    st.block = v.binary ? st.block : Math.min(st.block, v.blocks.length - 1);
    paintAll();
  };
  const rowCount = () => st.v.binary ? Math.ceil(Math.max(st.v.sizeA, st.v.sizeB) / HEX_W) || 1 : st.v.rows.length;
  // the difference blocks in row coordinates, for both modes
  const blocks = () => st.v.binary
    ? st.v.runs.map(([o, n]) => [Math.floor(o / HEX_W), Math.floor((o + Math.max(1, n) - 1) / HEX_W)])
    : st.v.blocks;
  const modified = () => !st.v.binary && (st.v.modified[0] || st.v.modified[1]);
  const opts = () => ({ ...st.v.options });

  // ---- drawing ----
  const hl = (text, spans) => {
    if (!spans?.length) return esc(text);
    let out = '', at = 0;
    for (const [a, b] of spans) { out += esc(text.slice(at, a)) + `<mark>${esc(text.slice(a, b))}</mark>`; at = b; }
    return out + esc(text.slice(at));
  };
  const hex = (b) => b === undefined ? '  ' : b.toString(16).padStart(2, '0');
  const chr = (b) => b === undefined ? ' ' : b >= 32 && b < 127 ? String.fromCharCode(b) : '·';
  const page = (i) => {
    if (st.bytes.has(i)) return st.bytes.get(i);
    st.bytes.set(i, null);
    go.CompareFiles.Bytes(st.id, i * PAGE, PAGE).then((r) => {
      const dec = (s) => { const t = atob(s || ''); const u = new Uint8Array(t.length); for (let k = 0; k < t.length; k++) u[k] = t.charCodeAt(k); return u; };
      st.bytes.set(i, { a: dec(r.a), b: dec(r.b) });
      render();
    }).catch((e) => app.flash(errText(e), 'err'));
    return null;
  };
  const byteAt = (off) => {
    const p = page(Math.floor(off / PAGE));
    if (!p) return [undefined, undefined, true];
    const k = off % PAGE;
    return [off < st.v.sizeA ? p.a[k] : undefined, off < st.v.sizeB ? p.b[k] : undefined, false];
  };
  const curBlock = () => { const bs = blocks(); return st.block >= 0 && st.block < bs.length ? bs[st.block] : null; };

  render = () => {
    const n = rowCount();
    spacer.style.height = `${n * ROW_H}px`;
    const top = body.scrollTop, h = body.clientHeight || 400;
    const from = Math.max(0, Math.floor(top / ROW_H) - 5), to = Math.min(n, Math.ceil((top + h) / ROW_H) + 5);
    const cb = curBlock();
    let html = '';
    for (let i = from; i < to; i++) {
      const inCur = cb && i >= cb[0] && i <= cb[1];
      const cls = ['mc-cmp-row'];
      if (i === st.row) cls.push('cur');
      if (inCur) cls.push('curblock');
      if (st.v.binary) {
        const base = i * HEX_W;
        let ha = '', hb = '', ca = '', cbb = '', loading = false, diff = false;
        for (let k = 0; k < HEX_W; k++) {
          const [a, b, wait] = byteAt(base + k);
          loading ||= wait;
          const d = !wait && a !== b && (a !== undefined || b !== undefined);
          diff ||= d;
          const o = d ? '<mark>' : '', c = d ? '</mark>' : '';
          ha += `${o}${hex(a)}${c} `; hb += `${o}${hex(b)}${c} `;
          ca += `${o}${esc(chr(a))}${c}`; cbb += `${o}${esc(chr(b))}${c}`;
        }
        if (diff) cls.push('k3');
        const off = base.toString(16).padStart(8, '0');
        html += loading ? `<div class="${cls.join(' ')}" data-i="${i}" style="top:${i * ROW_H}px"><span class="mc-cmp-n">${off}</span><span class="mc-cmp-t">…</span></div>`
          : `<div class="${cls.join(' ')} hex" data-i="${i}" style="top:${i * ROW_H}px"><span class="mc-cmp-n">${off}</span><span class="mc-cmp-t s0">${ha}  ${ca}</span><span class="mc-cmp-n">${off}</span><span class="mc-cmp-t s1">${hb}  ${cbb}</span></div>`;
        continue;
      }
      const r = st.v.rows[i];
      cls.push(`k${r.k}`);
      const sa = r.k === 3 ? r.sa : null, sb = r.k === 3 ? r.sb : null;
      html += `<div class="${cls.join(' ')}" data-i="${i}" style="top:${i * ROW_H}px">`
        + `<span class="mc-cmp-n">${r.l >= 0 ? r.l + 1 : ''}</span><span class="mc-cmp-t s0${r.l < 0 ? ' pad' : ''}${st.side === 0 && i === st.row ? ' focus' : ''}">${hl(r.a, sa)}</span>`
        + `<span class="mc-cmp-n">${r.r >= 0 ? r.r + 1 : ''}</span><span class="mc-cmp-t s1${r.r < 0 ? ' pad' : ''}${st.side === 1 && i === st.row ? ' focus' : ''}">${hl(r.b, sb)}</span></div>`;
    }
    rowsEl.innerHTML = html;
    rowsEl.style.setProperty('--cmp-x', st.x);
    if (st.editing) placeEditor();
  };

  const paintStatus = () => {
    const v = st.v, bs = blocks();
    let s;
    if (v.binary) {
      s = v.identical ? 'The files are identical.' : `${plural(v.diffBytes, 'differing byte')}${v.truncated ? ' (or more; only the first are listed)' : ''} · ${size(v.sizeA)} / ${size(v.sizeB)}`;
      if (!v.identical && v.sizeA !== v.sizeB) s += ' · the sizes differ';
    } else {
      s = v.identical ? 'No differences.' : `${plural(v.stats.blocks, 'difference')}`;
      s += ` · ${plural(v.lineCount[0], 'line')} / ${plural(v.lineCount[1], 'line')}`;
      if (st.edit) s += ` · editing the ${st.side ? 'right' : 'left'} side`;
    }
    if (st.block >= 0 && bs.length) s = `Difference ${st.block + 1} of ${bs.length} · ` + s;
    if (v.note) s += ` · ${v.note}`;
    status.textContent = s;
  };

  const paintTools = () => {
    const v = st.v, txt = !v.binary;
    const q = (sel) => box.querySelector(sel);
    q('#cmp-case').checked = !!v.options.ignoreCase;
    q('#cmp-ws').value = v.options.whitespace || 'exact';
    q('#cmp-rep').checked = !!v.options.ignoreRepeated;
    q('#cmp-bin').checked = !!v.binary;
    q('#cmp-edit').checked = st.edit;
    for (const id of ['#cmp-case', '#cmp-ws', '#cmp-rep', '#cmp-edit']) q(id).disabled = !txt;
    const bs = blocks();
    q('.b-next').disabled = !bs.length;
    q('.b-prev').disabled = !bs.length;
    for (const c of ['.b-toright', '.b-toleft']) q(c).disabled = !txt || !st.edit || !curBlock();
    q('.b-undo').disabled = !txt || !v.canUndo;
    q('.b-redo').disabled = !txt || !v.canRedo;
    q('.b-saveleft').disabled = !txt || !v.modified[0];
    q('.b-saveright').disabled = !txt || !v.modified[1];
    q('.b-saveleft').textContent = `Save left${v.modified[0] ? ' *' : ''}`;
    q('.b-saveright').textContent = `Save right${v.modified[1] ? ' *' : ''}`;
    for (const [i, id] of [[0, '#cmp-enc-left'], [1, '#cmp-enc-right']]) {
      const el = q(id);
      el.disabled = !txt;
      const name = txt ? String(v.encoding[i]).replace(/ BOM$/, '') : '';
      if (name && ![...el.options].some((o) => o.value === name)) el.add(new Option(name, name));
      el.value = name || 'Auto';
      el.title = txt ? `Encoding of the ${i ? 'right' : 'left'} file: ${v.encoding[i]}` : '';
    }
    if (document.activeElement !== q('#cmp-left')) q('#cmp-left').value = v.left;
    if (document.activeElement !== q('#cmp-right')) q('#cmp-right').value = v.right;
    box.classList.toggle('editing', st.edit);
    box.querySelector('h2').textContent = `Compare ${baseName(v.left)} and ${baseName(v.right)}`;
  };

  const paintAll = () => { paintTools(); paintStatus(); render(); };

  // ---- moving ----
  const showRow = (i, center = false) => {
    const top = i * ROW_H, h = body.clientHeight || 400;
    if (center) body.scrollTop = Math.max(0, top - h / 3);
    else if (top < body.scrollTop) body.scrollTop = top;
    else if (top + ROW_H > body.scrollTop + h) body.scrollTop = top + ROW_H - h;
  };
  const setRow = (i, center = false) => {
    st.row = Math.max(0, Math.min(rowCount() - 1, i));
    const bs = blocks();
    const b = bs.findIndex(([a, z]) => st.row >= a && st.row <= z);
    if (b >= 0) st.block = b;
    showRow(st.row, center);
    paintStatus();
    paintTools();
    render();
  };
  const goDiff = (d) => {
    const bs = blocks();
    if (!bs.length) { app.flash('No differences.'); return; }
    let i;
    if (d > 0) i = bs.findIndex(([a]) => a > st.row);
    else { i = -1; for (let k = bs.length - 1; k >= 0; k--) if (bs[k][1] < st.row) { i = k; break; } }
    if (i < 0) { app.flash(d > 0 ? 'This is the last difference.' : 'This is the first difference.'); return; }
    st.block = i;
    st.row = bs[i][0];
    showRow(st.row, true);
    paintAll();
  };

  // ---- editing ----
  const placeEditor = () => {
    const e = st.editing;
    const rowEl = rowsEl.querySelector(`.mc-cmp-row[data-i="${e.row}"]`);
    const cell = rowEl?.querySelector(`.s${e.side}`);
    if (!cell) { e.input.style.display = 'none'; return; }
    e.input.style.display = '';
    const br = body.getBoundingClientRect(), cr = cell.getBoundingClientRect();
    Object.assign(e.input.style, { left: `${cr.left - br.left + body.scrollLeft}px`, top: `${e.row * ROW_H}px`, width: `${cr.width}px`, height: `${ROW_H}px` });
    if (!body.contains(e.input)) body.append(e.input);
  };
  const startEdit = () => {
    if (st.v.binary) return;
    if (!st.edit) { st.edit = true; paintTools(); paintStatus(); }
    const r = st.v.rows[st.row];
    if (!r) return;
    const input = document.createElement('input');
    input.type = 'text';
    input.id = 'cmp-line';
    input.className = 'mc-cmp-input';
    input.spellcheck = false;
    input.value = st.side ? r.b : r.a;
    st.editing = { row: st.row, side: st.side, input };
    body.append(input);
    placeEditor();
    input.focus();
    input.select();
  };
  const stopEdit = () => { if (st.editing) { st.editing.input.remove(); st.editing = null; body.focus(); } };
  const commitEdit = async () => {
    const e = st.editing;
    if (!e) return;
    const text = e.input.value;
    const r = st.v.rows[e.row];
    stopEdit();
    if (r && text === (e.side ? r.b : r.a) && (e.side ? r.r : r.l) >= 0) return;
    await call(() => go.CompareFiles.SetLine(st.id, e.side, e.row, text));
  };

  // ---- actions ----
  const setOpts = (patch) => {
    const o = { ...opts(), ...patch };
    const { binary, ...keep } = o;
    saveSettings(app, { options: keep });
    return call(() => go.CompareFiles.SetOptions(st.id, o));
  };
  const askSave = async () => {
    if (!modified()) return true;
    for (const side of [0, 1]) {
      if (!st.v.modified[side]) continue;
      const file = side ? st.v.right : st.v.left;
      const b = await messageDialog({ Title: 'Compare by content', Message: `Save changes to ${file}?`, Buttons: ['Yes', 'No', 'Cancel'], DefaultButton: 'Yes', CancelButton: 'Cancel', Kind: 'compare-save' });
      if (b === 'Cancel') return false;
      if (b === 'Yes') {
        if (!await call(() => go.CompareFiles.Save(st.id, side))) return false;
        st.saved[side] = true;
      }
    }
    return true;
  };
  const save = async (side) => {
    if (await call(() => go.CompareFiles.Save(st.id, side))) { st.saved[side] = true; app.flash(`Saved ${baseName(side ? st.v.right : st.v.left)}.`); }
  };
  const recompare = async (newPaths = false) => {
    if (modified()) {
      const b = await messageDialog({ Title: 'Compare by content', Message: 'Comparing again reads both files from disk, and your unsaved changes are lost.', Buttons: ['Compare again', 'Cancel'], Kind: 'compare-discard' });
      if (b !== 'Compare again') return;
    }
    const l = newPaths ? box.querySelector('#cmp-left').value.trim() : st.v.left;
    const r = newPaths ? box.querySelector('#cmp-right').value.trim() : st.v.right;
    st.bytes.clear();
    if (await call(() => go.CompareFiles.Reopen(st.id, l, r))) { st.block = -1; app.flash(st.v.identical ? 'No differences.' : `${plural(blocks().length, 'difference')}.`); }
  };
  const copyBlock = (from) => {
    if (st.v.binary) return;
    if (!st.edit) { app.flash('Turn on Edit to change the files (Ctrl+E).'); return; }
    if (!curBlock()) { app.flash('Go to a difference first (F8).'); return; }
    const b = st.block;
    call(() => go.CompareFiles.CopyBlock(st.id, b, from));
  };
  const find = async (again) => {
    if (!again || !st.find) {
      const t = await inputDialog({ title: 'Find', label: `Find in the ${st.side ? 'right' : 'left'} file`, value: st.find, kind: 'compare-find' });
      if (t === null || t === '') return;
      st.find = t;
    }
    if (st.v.binary) { app.flash('Find works in text mode.'); return; }
    const needle = st.find.toLowerCase();
    const rows = st.v.rows, n = rows.length;
    for (let k = 1; k <= n; k++) {
      const i = (st.row + k) % n;
      if ((st.side ? rows[i].b : rows[i].a).toLowerCase().includes(needle)) { setRow(i, true); return; }
    }
    app.flash(`"${st.find}" is not there.`);
  };

  const close = async (done) => {
    if (st.editing) { stopEdit(); return; }
    if (!await askSave()) return;
    done({ saved: st.saved });
  };

  return openWindow({
    kind: 'compare',
    cls: 'mc-cmp-win',
    cancelValue: { saved: st.saved },
    build: (b, done) => {
      box = b;
      const encOpts = encodings.map((e) => `<option value="${esc(e)}">${esc(e)}</option>`).join('');
      box.innerHTML = `<h2></h2>
        <div class="mc-cmp-tools">
          <button type="button" class="b-next" title="Next difference (F8)">Next difference</button>
          <button type="button" class="b-prev" title="Previous difference (Shift+F8)">Previous difference</button>
          <button type="button" class="b-again" title="Compare again (F2)">Compare again</button>
          <span class="sep"></span>
          <label class="mc-check"><input type="checkbox" id="cmp-case">Ignore case</label>
          <label class="mc-check">Blanks <select id="cmp-ws"><option value="exact">Compare</option><option value="change">Ignore repeated</option><option value="all">Ignore all</option></select></label>
          <label class="mc-check"><input type="checkbox" id="cmp-rep">Ignore repeated lines</label>
          <label class="mc-check"><input type="checkbox" id="cmp-bin">Binary</label>
          <span class="sep"></span>
          <label class="mc-check" title="Edit mode (Ctrl+E)"><input type="checkbox" id="cmp-edit">Edit</label>
          <button type="button" class="b-toleft" title="Copy the difference to the left (Alt+Left)">‹ Copy to left</button>
          <button type="button" class="b-toright" title="Copy the difference to the right (Alt+Right)">Copy to right ›</button>
          <button type="button" class="b-undo" title="Undo (Ctrl+Z)">Undo</button>
          <button type="button" class="b-redo" title="Redo (Ctrl+Y)">Redo</button>
          <button type="button" class="b-saveleft" title="Save the left file">Save left</button>
          <button type="button" class="b-saveright" title="Save the right file">Save right</button>
        </div>
        <div class="mc-cmp-paths">
          <input type="text" id="cmp-left" spellcheck="false" title="Left file (Enter compares another file)"><select id="cmp-enc-left" title="Encoding of the left file">${encOpts}</select>
          <input type="text" id="cmp-right" spellcheck="false" title="Right file (Enter compares another file)"><select id="cmp-enc-right" title="Encoding of the right file">${encOpts}</select>
        </div>
        <div class="mc-cmp-body" tabindex="0"><div class="mc-cmp-spacer"></div><div class="mc-cmp-rows"></div></div>
        <div class="mc-cmp-foot"><span class="mc-cmp-status"></span><span class="mc-btns"><button type="button" class="b-close">Close</button></span></div>`;
      body = box.querySelector('.mc-cmp-body');
      spacer = box.querySelector('.mc-cmp-spacer');
      rowsEl = box.querySelector('.mc-cmp-rows');
      status = box.querySelector('.mc-cmp-status');
      body.setAttribute('autofocus', '');
      const on = (sel, fn) => { box.querySelector(sel).onclick = fn; };
      on('.b-next', () => { goDiff(1); body.focus(); });
      on('.b-prev', () => { goDiff(-1); body.focus(); });
      on('.b-again', () => recompare());
      on('.b-toleft', () => copyBlock(1));
      on('.b-toright', () => copyBlock(0));
      on('.b-undo', () => call(() => go.CompareFiles.Undo(st.id)));
      on('.b-redo', () => call(() => go.CompareFiles.Redo(st.id)));
      on('.b-saveleft', () => save(0));
      on('.b-saveright', () => save(1));
      on('.b-close', () => close(done));
      box.querySelector('#cmp-case').onchange = (ev) => setOpts({ ignoreCase: ev.target.checked });
      box.querySelector('#cmp-ws').onchange = (ev) => setOpts({ whitespace: ev.target.value });
      box.querySelector('#cmp-rep').onchange = (ev) => setOpts({ ignoreRepeated: ev.target.checked });
      box.querySelector('#cmp-bin').onchange = (ev) => { st.edit = false; stopEdit(); setOpts({ binary: ev.target.checked }); };
      box.querySelector('#cmp-edit').onchange = (ev) => { st.edit = ev.target.checked; if (!st.edit) stopEdit(); paintAll(); };
      for (const [side, id] of [[0, '#cmp-enc-left'], [1, '#cmp-enc-right']]) {
        box.querySelector(id).onchange = (ev) => call(() => go.CompareFiles.SetEncoding(st.id, side, ev.target.value, false)).then((ok) => { if (!ok) paintTools(); });
      }
      box.onsubmit = (ev) => {
        ev.preventDefault();
        const a = document.activeElement;
        if (st.editing && a === st.editing.input) commitEdit();
        else if (a?.id === 'cmp-left' || a?.id === 'cmp-right') recompare(true);
      };
      body.addEventListener('scroll', () => render());
      body.addEventListener('wheel', (ev) => { if (ev.shiftKey && Math.abs(ev.deltaY) > Math.abs(ev.deltaX)) { ev.preventDefault(); st.x = Math.max(0, st.x + Math.sign(ev.deltaY) * 4); render(); } }, { passive: false });
      rowsEl.addEventListener('mousedown', (ev) => {
        if (ev.target.closest('.mc-cmp-input')) return;
        const r = ev.target.closest('.mc-cmp-row');
        if (!r) return;
        const t = ev.target.closest('.mc-cmp-t');
        if (t) st.side = t.classList.contains('s1') ? 1 : 0;
        if (st.editing) commitEdit();
        setRow(+r.dataset.i);
      });
      rowsEl.addEventListener('dblclick', (ev) => { if (ev.target.closest('.mc-cmp-row') && !st.v.binary) startEdit(); });
      new ResizeObserver(() => render()).observe(body);
      requestAnimationFrame(() => {
        paintAll();
        if (!st.v.identical) { st.row = -1; goDiff(1); }
        body.focus();
      });
      // keep the window's done() for Esc handling below
      st.done = done;
    },
    onKey: (ev, combo) => {
      const inField = ev.target.matches?.('input[type=text], select');
      if (st.editing && ev.target === st.editing.input) {
        if (combo === 'Esc') { stopEdit(); return true; }
        if (combo === 'Enter') { commitEdit(); return true; }
        if (combo === 'Up' || combo === 'Down') { const d = combo === 'Up' ? -1 : 1; commitEdit().then(() => { setRow(st.row + d); startEdit(); }); return true; }
        return false;
      }
      if (combo === 'Esc') { close(st.done); return true; }
      if (inField && !/^(Shift\+)?F\d+$|^Ctrl\+/.test(combo)) return false;
      const page = Math.max(1, Math.floor((body.clientHeight || 400) / ROW_H) - 1);
      switch (combo) {
        case 'F8': case 'Ctrl+Down': goDiff(1); return true;
        case 'Shift+F8': case 'Ctrl+Up': goDiff(-1); return true;
        case 'F2': recompare(); return true;
        case 'Up': setRow(st.row - 1); return true;
        case 'Down': setRow(st.row + 1); return true;
        case 'PgUp': setRow(st.row - page); return true;
        case 'PgDn': setRow(st.row + page); return true;
        case 'Ctrl+Home': case 'Home': if (combo === 'Home' && st.x) { st.x = 0; render(); return true; } setRow(0); return true;
        case 'Ctrl+End': case 'End': setRow(rowCount() - 1); return true;
        case 'Left': st.x = Math.max(0, st.x - 8); render(); return true;
        case 'Right': st.x += 8; render(); return true;
        case 'Tab': case 'Shift+Tab':
          if (ev.target !== body) return false;
          st.side = 1 - st.side; paintStatus(); render(); return true;
        case 'Ctrl+E': box.querySelector('#cmp-edit').click(); return true;
        case 'Enter': if (ev.target === body && st.edit) { startEdit(); return true; } return false;
        case 'Ctrl+Delete': if (st.edit && !st.v.binary) { call(() => go.CompareFiles.DeleteLine(st.id, st.side, st.row)); } return true;
        case 'Alt+Right': copyBlock(0); return true;
        case 'Alt+Left': copyBlock(1); return true;
        case 'Ctrl+S': case 'Cmd+S': if (!st.v.binary && st.v.modified[st.side]) save(st.side); return true;
        case 'Ctrl+Z': case 'Cmd+Z': if (st.v.canUndo) call(() => go.CompareFiles.Undo(st.id)); return true;
        case 'Ctrl+Y': case 'Ctrl+Shift+Z': case 'Cmd+Shift+Z': if (st.v.canRedo) call(() => go.CompareFiles.Redo(st.id)); return true;
        case 'Ctrl+F': case 'Cmd+F': find(false); return true;
        case 'F3': find(true); return true;
        case 'Ctrl+B': box.querySelector('#cmp-bin').click(); return true;
      }
      return false;
    },
  }).finally(() => {
    go.CompareFiles.Close(st.id).catch(() => {});
    if (current === st) current = null;
  });
}

// pickPair finds the two files to compare (spec area 4, 5.1): two selected in the active panel; one selected in each
// panel; the file under the cursor (or the one selected) and the file of the same name in the other panel, or else the
// file under the cursor there.
export function pickPair(app) {
  const a = app.cur, b = app.other;
  const sa = a.selectedEntries(), sb = b.selectedEntries();
  const files = (list) => list.every((e) => !e.dir);
  let l, r;
  if (sa.length === 2) {
    if (!files(sa)) return { error: 'Please select two files to compare.' };
    return { left: a.fullPath(sa[0].name), right: a.fullPath(sa[1].name) };
  }
  if (sa.length === 1 && sb.length === 1) {
    if (!files(sa) || !files(sb)) return { error: 'Please select two files to compare.' };
    l = a.fullPath(sa[0].name); r = b.fullPath(sb[0].name);
  } else if (sa.length <= 1 && sb.length <= 1) {
    const e = sa[0] || (a.cur && !a.cur.up ? a.cur : null);
    if (!e) return { error: 'Please select two files to compare, or one file in each panel.' };
    if (e.dir) return { error: 'Please select two files to compare.' };
    const ci = app.platform !== 'linux';
    const twin = b.entries.find((x) => !x.up && !x.dir && (ci ? x.name.toLowerCase() === e.name.toLowerCase() : x.name === e.name))
      || (sb[0] && !sb[0].dir ? sb[0] : null) || (b.cur && !b.cur.up && !b.cur.dir ? b.cur : null);
    if (!twin) return { error: 'Please select two files to compare, or one file in each panel.' };
    l = a.fullPath(e.name); r = b.fullPath(twin.name);
  } else {
    return { error: 'Please select two files to compare, or one file in each panel.' };
  }
  return a.side === 'right' ? { left: r, right: l } : { left: l, right: r };
}

command({
  name: 'cm_CompareFilesByContent', label: 'Compare by content…', short: 'Compare',
  run: async (app, args = {}) => {
    let { left, right } = args;
    if (!left || !right) {
      const p = pickPair(app);
      if (p.error) { await messageDialog({ Title: 'Compare by content', Message: p.error, Kind: 'compare-pick' }); return false; }
      ({ left, right } = p);
    }
    await compareFiles(app, left, right, { force: !!args.internal });
    return true;
  },
});

command({
  // ours: the external compare program (F-563); empty = the built-in window
  name: 'cm_CompareToolSetup', label: 'Compare program…', menu: 'Configuration',
  run: async (app) => {
    const v = await inputDialog({
      title: 'Compare program', label: 'Program and parameters (%1 = left file, %2 = right file); empty = the built-in compare window',
      value: settings(app).externalTool || '', note: 'For example: "C:\\Program Files\\WinMerge\\WinMergeU.exe" %1 %2, or meld %1 %2', kind: 'compare-tool',
    });
    if (v === null) return false;
    saveSettings(app, { externalTool: v.trim() });
    app.flash(v.trim() ? 'Compare by content now starts that program.' : 'Compare by content now uses the built-in window.');
    return true;
  },
});

addControlCommand('compare', async () => {
  const st = current;
  if (!st) return { ok: true, open: false };
  const v = st.v;
  return {
    ok: true, open: true, left: v.left, right: v.right, binary: v.binary, identical: v.identical, note: v.note,
    options: v.options, edit: st.edit, row: st.row, side: st.side, block: st.block,
    blocks: v.binary ? v.runs.length : v.blocks.length, rows: v.binary ? 0 : v.rows.length,
    modified: v.modified, encoding: v.encoding, lineCount: v.lineCount, sizeA: v.sizeA, sizeB: v.sizeB, diffBytes: v.diffBytes,
    text: v.binary ? null : v.rows.slice(Math.max(0, st.row - 3), st.row + 4).map((r) => ({ k: r.k, l: r.l, r: r.r, a: r.a, b: r.b })),
  };
});
