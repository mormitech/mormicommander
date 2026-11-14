// One Lister view: the part that shows a file, used by the Lister window (F3) and by the quick view panel (Ctrl+Q). The
// Go side (feature_lister.go over internal/lister) lays the file out; this keeps only "the byte offset of the top row"
// and asks for one screenful of rows at a time, so a file of any size opens and scrolls at once (F-579). Selections and
// search matches are byte ranges too; every row says which byte each of its characters came from.
//
// Modes (TC's keys): 1 text, 2 binary, 3 hex, 4 image/multimedia, 5 HTML/Markdown rendered (other files: HTML as text),
// 6/U Unicode, 7/8 UTF-8, A ANSI, S ASCII (DOS), V variable-width font, W wrap. See key() for the rest.

import { go, on, errText } from '../../core/go.js';
import { track } from '../../core/busy.js';
import { esc, messageDialog, inputDialog, formDialog, infoDialog } from '../../core/dialogs.js';
import { showList } from '../../core/popup.js';
import { size as fmtSize, bytes as fmtBytes } from '../../core/format.js';
import { label as keyLabel } from '../../core/keys.js';
import { markdownToHTML } from './markdown.js';
import { sanitizeHTML } from './sanitize.js';
import { quickMenuItems } from './quickmenu.js';

export const MODE_LABELS = { text: 'Text', binary: 'Binary', hex: 'Hex', media: 'Image/Multimedia', html: 'HTML/Markdown' };
const MEDIA_LIMIT = 256 * 1024 * 1024; // larger media files are not read into the page
const CHUNK = 4 * 1024 * 1024;
const WORD = /[\p{L}\p{N}_]/u;
const MAC_ALIASES = { 'Cmd+C': 'Ctrl+C', 'Cmd+A': 'Ctrl+A', 'Cmd+F': 'Ctrl+F', 'Cmd+G': 'F3', 'Shift+Cmd+G': 'Shift+F3', 'Cmd+S': 'Ctrl+S',
  'Cmd+L': 'Ctrl+G', 'Cmd+=': 'Ctrl+=', 'Cmd+-': 'Ctrl+-', 'Cmd+Up': 'Ctrl+Home', 'Cmd+Down': 'Ctrl+End' };

// The settings of the Lister ("lister" in settings.json), with Total Commander's defaults.
export const LISTER_DEFAULTS = {
  defaultMode: 'auto', // auto, text, binary, hex
  defaultCharset: 'auto', // auto, ansi, oem, utf-8, unicode
  autoHex: true, // binary files open in hex mode (TC opens them as text)
  showMedia: true, // images, audio and video open in mode 4 (TC's BmpStartup / Multimedia)
  renderHTML: true, // HTML and Markdown open rendered
  wrap: true,
  tabWidth: 8,
  binaryWidth: 75,
  font: 'fixed', // fixed or variable (V)
  fontSize: 12,
  fitImage: true,
  viewer: '', // external viewer for F3 ('' = Lister); Alt+F3 then opens the other one
  findHistory: [],
};

export function listerSettings(app) { return { ...LISTER_DEFAULTS, ...(app.setting('lister', {}) || {}) }; }
export function saveListerSettings(app, changes) { return app.setSetting('lister', { ...(app.setting('lister', {}) || {}), ...changes }); }

// applyListerOptions gives an open view new display options (wrap, font, font size, fit) at once.
export function applyListerOptions(v, lo) {
  Object.assign(v, { wrap: !!lo.wrap, variable: lo.font === 'variable', fontSize: +lo.fontSize || 12, fit: lo.fitImage !== false });
  v.applyFont();
  v.layout();
  if (v.id && v.isTextMode()) v.queue(() => v.applyOptions());
}

// Every view by session id, for the events Go sends.
const views = new Map();
on('lister-progress', (d) => views.get(d?.id)?.progress(d));
on('lister-change', (d) => views.get(d?.id)?.changed(d));

// The last search, shared by every view like in Total Commander (F3 in another Lister repeats it).
let lastQuery = null;

const b64ToBytes = (s) => {
  const bin = atob(s || '');
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
};

// normMedia gives lister.MediaInfo (Go field names) the page's spelling.
const normMedia = (m) => (m ? {
  kind: m.Kind, format: m.Format, mime: m.MIME, width: m.Width, height: m.Height, bitDepth: m.BitDepth, frames: m.Frames, duration: m.Duration,
  sampleRate: m.SampleRate, channels: m.Channels, bitrate: m.Bitrate, orientation: m.Orientation, tags: (m.Tags || []).map((t) => ({ name: t.Name, value: t.Value })),
} : null);

const hexOffset = (n, digits) => n.toString(16).toUpperCase().padStart(digits, '0');

export class ListerView {
  constructor(app, { quick = false, internalOnly = false, onNeighbor = null, onClose = null, onOpen = null } = {}) {
    this.app = app;
    this.quick = quick;
    this.internalOnly = internalOnly;
    this.onNeighbor = onNeighbor; // (delta) => opens the next/previous file (Lister window only)
    this.onClose = onClose;
    this.onOpen = onOpen; // (info) after a file was opened
    const s = listerSettings(app);
    this.wrap = !!s.wrap;
    this.variable = s.font === 'variable';
    this.fontSize = +s.fontSize || 12;
    this.fit = s.fitImage !== false;
    this.id = 0;
    this.info = null;
    this.mode = 'text';
    this.enc = ''; // '' = what detection found
    this.asText = ''; // '', 'html' or 'rtf' in text mode
    this.top = 0;
    this.page = null;
    this.rows = 30;
    this.cols = 80;
    this.sel = null; // {a, b}: anchor and focus, byte offsets
    this.match = null; // {o, l}
    this.zoom = 1;
    this.rot = 0;
    this.flipH = false;
    this.follow = false;
    this.searching = false;
    this.loading = false;
    this.msg = '';
    this.msgTimer = 0;
    this.chain = Promise.resolve();
    this.blobURL = '';
    this.mediaFor = 0;
    this.docFor = 0;
    this.folder = null;
    this.build();
  }

  // ---- building ----

  build() {
    const el = document.createElement('div');
    el.className = 'mc-viewer' + (this.quick ? ' mc-viewer-quick' : '');
    el.tabIndex = 0;
    el.innerHTML = `
      <div class="mc-viewer-main">
        <div class="mc-viewer-text"><div class="mc-viewer-rows"></div></div>
        <div class="mc-viewer-sb" title="Position in the file"><div class="mc-viewer-thumb"></div></div>
        <div class="mc-viewer-media" hidden></div>
        <div class="mc-viewer-doc" hidden></div>
        <div class="mc-viewer-note" hidden></div>
      </div>
      <div class="mc-viewer-status"><span class="mc-viewer-st-l"></span><span class="mc-viewer-st-r"></span></div>`;
    const $ = (s) => el.querySelector(s);
    this.el = el;
    this.$ = { main: $('.mc-viewer-main'), text: $('.mc-viewer-text'), rows: $('.mc-viewer-rows'), sb: $('.mc-viewer-sb'), thumb: $('.mc-viewer-thumb'),
      media: $('.mc-viewer-media'), doc: $('.mc-viewer-doc'), note: $('.mc-viewer-note'), stl: $('.mc-viewer-st-l'), str: $('.mc-viewer-st-r') };
    this.applyFont();
    this.ro = new ResizeObserver(() => this.layout());
    this.ro.observe(this.$.main);
    this.$.text.addEventListener('wheel', (ev) => this.onWheel(ev), { passive: false });
    this.$.media.addEventListener('wheel', (ev) => { if (ev.ctrlKey || ev.metaKey) { ev.preventDefault(); this.zoomBy(ev.deltaY < 0 ? 1.25 : 0.8); } }, { passive: false });
    this.$.text.addEventListener('mousedown', (ev) => this.onMouseDown(ev));
    this.$.text.addEventListener('dblclick', (ev) => this.onDblClick(ev));
    this.$.text.addEventListener('scroll', () => { this.$.text.scrollTop = 0; }); // vertical position is ours (byte offset), never the element's
    this.$.sb.addEventListener('mousedown', (ev) => this.onScrollbar(ev));
    this.$.doc.addEventListener('click', (ev) => {
      const a = ev.composedPath().find((n) => n.localName === 'a');
      if (!a) return;
      ev.preventDefault(); // a link must never take the app's page somewhere else
      const h = a.getAttribute('href') || '';
      if (h.startsWith('#') && h.length > 1) {
        const root = this.$.doc.querySelector('.mc-viewer-dochost')?.shadowRoot;
        const t = root?.getElementById(decodeURIComponent(h.slice(1))) || root?.querySelector(`[name="${CSS.escape(decodeURIComponent(h.slice(1)))}"]`);
        t?.scrollIntoView();
      } else if (h) this.note(`Link: ${h}`);
    });
    el.addEventListener('mousedown', (ev) => { if (!ev.target.closest('input, select, button, video, audio')) setTimeout(() => this.focus(), 0); });

    // quick view has no menu bar: a right-click opens the Lister's menus as one list (video and audio keep their own menu)
    if (this.quick) el.addEventListener('contextmenu', (ev) => {
      if (ev.target.closest?.('video, audio')) return;
      ev.preventDefault();
      this.quickMenu({ x: ev.clientX, y: ev.clientY });
    });
  }

  focus() { this.el.focus({ preventScroll: true }); }

  applyFont() {
    this.el.classList.toggle('mc-viewer-variable', this.variable);
    this.el.style.setProperty('--mc-viewer-fs', `${this.fontSize}px`);
    // whole pixels per row, so the number of rows that fit is exact
    this.lh = Math.ceil(this.fontSize * 1.4);
    this.el.style.setProperty('--mc-viewer-lh', `${this.lh}px`);
    this.measured = false;
  }

  // measure finds the size of one character cell.
  measure() {
    const probe = document.createElement('span');
    probe.className = 'mc-viewer-probe';
    probe.textContent = 'MMMMMMMMMM';
    this.$.rows.append(probe);
    const r = probe.getBoundingClientRect();
    probe.remove();
    if (r.width > 0) {
      this.cw = r.width / 10;
      this.measured = true;
    }
  }

  // layout fits rows and columns to the space; when they change the page is fetched again.
  layout() {
    if (!this.el.isConnected) return;
    if (!this.measured) this.measure();
    if (!this.measured) return;
    const t = this.$.text;
    const h = t.clientHeight;
    if (!h || !t.clientWidth) return; // hidden (image or HTML mode)
    const rows = Math.max(1, Math.floor((h - 4) / this.lh));
    const cols = Math.max(20, Math.floor((t.clientWidth - 12) / this.cw));
    const colsChanged = cols !== this.cols;
    const rowsChanged = rows !== this.rows;
    this.rows = rows;
    this.cols = cols;
    if (!this.id || !this.isTextMode()) return;
    if (colsChanged && this.wrap && this.mode === 'text') this.queue(() => this.applyOptions());
    else if (rowsChanged || colsChanged) this.refresh('top');
  }

  isTextMode() { return this.mode === 'text' || this.mode === 'binary' || this.mode === 'hex'; }

  engineOptions() {
    const s = listerSettings(this.app);
    const textMode = this.isTextMode() ? this.mode : 'text';
    return {
      mode: textMode, encoding: this.enc, wrap: this.wrap, tabWidth: +s.tabWidth || 8,
      width: textMode === 'binary' ? (+s.binaryWidth || 75) : this.cols,
      asText: textMode === 'text' ? this.asText : '',
    };
  }

  // queue runs Go calls one after another, so fast key repeats each start from the latest position.
  queue(fn) {
    const id = this.id;
    const p = this.chain.then(() => (this.id === id ? fn() : undefined)).catch((e) => this.note(errText(e), true));
    this.chain = p;
    return track(p);
  }

  // ---- opening ----

  // open shows a file. mode: 'auto' (per settings) or a mode to force. Errors reject (the caller tells the user).
  async open(path, mode = 'auto') {
    const s = listerSettings(this.app);
    const charset = s.defaultCharset === 'auto' ? '' : s.defaultCharset;
    const enc = charset;
    const info = await track(go.Lister.Open(path, { mode: 'text', encoding: enc, wrap: this.wrap, width: this.cols, tabWidth: +s.tabWidth || 8, asText: '' }));
    const old = this.id;
    if (old) { views.delete(old); go.Lister.Close(old).catch(() => {}); }
    this.dropMedia();
    info.media = normMedia(info.media);
    this.id = info.id;
    this.info = info;
    views.set(info.id, this);
    this.enc = enc;
    this.asText = '';
    this.top = 0;
    this.page = null;
    this.sel = null;
    this.match = null;
    this.zoom = 1;
    this.rot = 0;
    this.flipH = false;
    this.follow = false;
    this.folder = null;
    this.chain = Promise.resolve();
    this.onOpen?.(info);
    await this.setMode(mode === 'auto' ? this.autoMode(info, s) : mode, true);
  }

  autoMode(info, s) {
    const forced = s.defaultMode !== 'auto' ? s.defaultMode : '';
    switch (info.suggest) {
      case 'media': if (!this.internalOnly && s.showMedia) return 'media'; break;
      case 'html': case 'markdown': if (!this.internalOnly && s.renderHTML && !forced) return 'html'; break;
      case 'rtf': if (!forced) { this.asText = 'rtf'; return 'text'; } break;
    }
    if (forced) return forced;
    if (info.binary || info.suggest === 'binary' || info.suggest === 'media') return s.autoHex ? 'hex' : 'text';
    return info.suggest === 'hex' ? 'hex' : 'text';
  }

  // setMode switches between the five modes.
  async setMode(m, initial = false) {
    if (!this.id) return;
    if ((m === 'media' || m === 'html') && this.internalOnly) { this.note('This view shows text, binary and hex only.'); return; }
    if (m === 'media') {
      if (this.info.suggest === 'rtf') { this.asText = 'rtf'; m = 'text'; } else if (!this.info.media) {
        if (!initial) this.note('This is not an image or media file, so it is shown as text.');
        m = 'text';
      }
    }
    if (m === 'html' && !['html', 'markdown'].includes(this.info.suggest)) { this.asText = 'html'; m = 'text'; } // HTML as text (TC's key 5)
    this.mode = m;
    this.showPane();
    if (m === 'media') { await track(this.showMedia()); this.status(); return; }
    if (m === 'html') { await track(this.showDoc()); this.status(); return; }
    await this.queue(() => this.applyOptions());
  }

  showPane() {
    const textual = this.isTextMode();
    this.$.text.hidden = !textual;
    this.$.sb.hidden = !textual;
    this.$.media.hidden = this.mode !== 'media';
    this.$.doc.hidden = this.mode !== 'html';
    this.$.note.hidden = true;
    this.el.dataset.mode = this.mode;
    if (textual) requestAnimationFrame(() => this.layout());
  }

  async applyOptions() {
    const info = await go.Lister.SetOptions(this.id, this.engineOptions());
    Object.assign(this.info, { encoding: info.encoding, encodingName: info.encodingName, offsetDigits: info.offsetDigits, dataStart: info.dataStart, viewSize: info.viewSize, options: info.options });
    this.asText = info.options.asText;
    await this.move('top');
  }

  // ---- paging ----

  async move(how, arg = 0) {
    const id = this.id;
    const page = await go.Lister.Move(id, this.top, how, arg, this.rows);
    if (id !== this.id) return;
    this.page = page;
    this.top = page.top;
    this.render();
    this.status();
  }

  refresh(how = 'top', arg = 0) { return this.queue(() => this.move(how, arg)); }

  scrollRows(n) { if (this.isTextMode()) this.refresh('rows', n); }

  // ---- drawing ----

  selRange() {
    if (!this.sel || this.sel.a === this.sel.b) return null;
    return [Math.min(this.sel.a, this.sel.b), Math.max(this.sel.a, this.sel.b)];
  }

  classAt(off, sr) {
    if (sr && off >= sr[0] && off < sr[1]) return 'mc-viewer-sel';
    if (this.match && off >= this.match.o && off < this.match.o + this.match.l) return 'mc-viewer-hit';
    return '';
  }

  render() {
    const p = this.page;
    const box = this.$.rows;
    if (!p) { box.textContent = ''; return; }
    const sr = this.selRange();
    const hex = this.mode === 'hex';
    const digits = this.info.offsetDigits || 8;
    const html = [];
    for (let i = 0; i < p.rows.length; i++) {
      const r = p.rows[i];
      const touched = (sr && sr[0] < r.e && sr[1] > r.o) || (this.match && this.match.o < r.e && this.match.o + this.match.l > r.o);
      if (hex) {
        const parts = (r.h || '').trim().split(/\s+/).filter(Boolean);
        const chars = Array.from(r.t);
        let hx = '', cs = '';
        for (let j = 0; j < parts.length; j++) {
          const c = touched ? this.classAt(r.o + j, sr) : '';
          hx += `${j === 8 ? ' ' : ''}${j ? ' ' : ''}<span data-b="${j}"${c ? ` class="${c}"` : ''}>${parts[j]}</span>`;
          cs += `<span data-b="${j}"${c ? ` class="${c}"` : ''}>${esc(chars[j] ?? ' ')}</span>`;
        }
        const pad = Math.max(0, 16 - parts.length) * 3 + (parts.length <= 8 ? 1 : 0);
        html.push(`<div class="mc-viewer-ln" data-i="${i}"><span class="mc-viewer-off">${hexOffset(r.o, digits)}:</span> <span class="mc-viewer-hex">${hx}${' '.repeat(pad)}</span>  <span class="mc-viewer-chr">${cs}</span></div>`);
        continue;
      }
      if (!touched) { html.push(`<div class="mc-viewer-ln" data-i="${i}"><span data-r="0">${esc(r.t)}</span></div>`); continue; }
      const chars = Array.from(r.t);
      let out = '', run = '', cls = null, start = 0;
      for (let k = 0; k <= chars.length; k++) {
        const c = k < chars.length ? this.classAt(r.o + r.m[k], sr) : null;
        if (c !== cls || k === chars.length) {
          if (run || (k === chars.length && cls !== null)) out += `<span data-r="${start}"${cls ? ` class="${cls}"` : ''}>${esc(run)}</span>`;
          run = '';
          cls = c;
          start = k;
        }
        if (k < chars.length) run += chars[k];
      }
      // a selected line break shows as a selected cell at the end of the row
      const textEnd = this.rowEndText(r);
      if (sr && textEnd < r.e && sr[0] <= textEnd && sr[1] >= r.e) out += '<span class="mc-viewer-sel mc-viewer-eol"> </span>';
      html.push(`<div class="mc-viewer-ln" data-i="${i}">${out}</div>`);
    }
    box.innerHTML = html.join('');
    box.classList.toggle('mc-viewer-wrap', this.wrap && this.mode === 'text');
    this.thumb();
  }

  thumb() {
    const size = this.info?.viewSize || 0;
    const trackH = this.$.sb.clientHeight;
    const th = Math.max(24, Math.min(trackH, size ? trackH * Math.min(1, (this.rows * Math.max(this.cols, 16)) / size) : trackH));
    this.$.thumb.style.height = `${th}px`;
    const frac = size ? Math.min(1, this.top / size) : 0;
    this.$.thumb.style.transform = `translateY(${(trackH - th) * (this.page?.atEnd ? 1 : frac)}px)`;
  }

  status() {
    if (!this.info) { this.$.stl.textContent = this.folder ? 'Folder' : ''; this.$.str.textContent = ''; return; }
    const parts = [MODE_LABELS[this.mode] + (this.asText === 'html' ? ' (HTML as text)' : this.asText === 'rtf' ? ' (RTF as text)' : '')];
    if (this.isTextMode() && this.mode !== 'hex') parts.push(this.asText ? 'UTF-8' : this.info.encodingName || this.info.encoding);
    if (this.mode === 'text' && this.wrap) parts.push('Wrap');
    if (this.variable && this.mode !== 'hex' && this.isTextMode()) parts.push('Variable width');
    if (this.follow) parts.push('Following');
    if (this.mode === 'media' && this.info.media) {
      const m = this.info.media;
      parts.push(m.format + (m.width ? ` ${m.width}×${m.height}` : ''));
      if (this.mode === 'media' && m.kind === 'image') parts.push(this.fit ? 'Fit to window' : `${Math.round(this.zoom * 100)}%`);
    }
    const sr = this.selRange();
    if (sr && this.isTextMode()) parts.push(`${fmtBytes(sr[1] - sr[0])} bytes selected`);
    this.$.stl.textContent = this.msg || parts.join(' · ');
    this.$.stl.classList.toggle('mc-viewer-msg', !!this.msg);
    const right = [];
    const p = this.page;
    if (p && this.isTextMode()) {
      if (this.mode === 'text' && p.line) right.push(`Line ${fmtBytes(p.line)}${p.linesComplete ? ` of ${fmtBytes(p.lines)}` : ''}`);
      if (this.mode !== 'text') right.push(`Offset ${hexOffset(p.top, this.info.offsetDigits || 8)}`);
      right.push(`${Math.round(p.percent)}%`);
    }
    right.push(fmtSize(this.info.size));
    this.$.str.textContent = right.join(' · ');
  }

  // note shows a short message in the status line (errors stay longer).
  note(text, err = false) {
    this.msg = text;
    clearTimeout(this.msgTimer);
    this.status();
    this.msgTimer = setTimeout(() => { this.msg = ''; this.status(); }, err ? 6000 : 3000);
  }

  // ---- media and documents ----

  dropMedia() {
    if (this.blobURL) URL.revokeObjectURL(this.blobURL);
    this.blobURL = '';
    this.mediaFor = 0;
    this.docFor = 0;
    this.$.media.textContent = '';
    this.$.doc.textContent = '';
  }

  async showMedia() {
    const id = this.id;
    const m = this.info.media;
    if (this.mediaFor === id) { this.transform(); return; }
    this.$.media.textContent = '';
    if (this.info.size > MEDIA_LIMIT) {
      this.$.media.innerHTML = `<p class="mc-viewer-big">${esc(this.info.name)} is too large to show here (${esc(fmtSize(this.info.size))}).<br>Press ${esc(this.altKey())} to open it with another app.</p>`;
      this.mediaFor = id;
      return;
    }
    const parts = [];
    this.loading = true;
    try {
      for (let off = 0; off < this.info.size; off += CHUNK) {
        const b = b64ToBytes(await go.Lister.Chunk(id, off, CHUNK));
        if (this.id !== id) return;
        parts.push(b);
        if (!b.length) break;
        if (this.info.size > CHUNK) { this.msg = `Loading… ${Math.round(Math.min(100, ((off + CHUNK) / this.info.size) * 100))}%`; this.status(); }
      }
    } finally {
      this.loading = false;
      this.msg = '';
    }
    if (this.id !== id) return;
    this.blobURL = URL.createObjectURL(new Blob(parts, { type: m.mime || 'application/octet-stream' }));
    this.mediaFor = id;
    let el;
    if (m.kind === 'image') {
      el = document.createElement('img');
      el.alt = this.info.name;
      el.draggable = false;
      el.addEventListener('load', () => this.transform());
    } else if (m.kind === 'video') {
      el = document.createElement('video');
      el.controls = true;
    } else if (m.kind === 'audio') {
      el = document.createElement('audio');
      el.controls = true;
    } else {
      el = document.createElement('embed');
      el.type = m.mime;
    }
    el.className = 'mc-viewer-mediael';
    el.src = this.blobURL;
    this.$.media.append(el);
    this.transform();
  }

  transform() {
    const img = this.$.media.querySelector('img');
    if (!img) return;
    this.$.media.classList.toggle('mc-viewer-fit', this.fit);
    const turned = this.rot % 180 !== 0;
    if (this.fit) {
      const W = this.$.media.clientWidth - 8, H = this.$.media.clientHeight - 8;
      const w = img.naturalWidth || 1, h = img.naturalHeight || 1;
      const bw = turned ? h : w, bh = turned ? w : h;
      const k = Math.min(1, W / bw, H / bh); // TC's "fit": larger images shrink, smaller ones stay as they are
      img.style.width = `${w * k}px`;
      img.style.height = `${h * k}px`;
    } else {
      img.style.width = `${(img.naturalWidth || 0) * this.zoom}px`;
      img.style.height = `${(img.naturalHeight || 0) * this.zoom}px`;
    }
    img.style.transform = `rotate(${this.rot}deg)${this.flipH ? ' scaleX(-1)' : ''}`;
    img.style.margin = turned ? `${Math.abs(parseFloat(img.style.width) - parseFloat(img.style.height)) / 2}px ${-Math.abs(parseFloat(img.style.width) - parseFloat(img.style.height)) / 2}px` : '';
    this.status();
  }

  zoomBy(k) {
    if (this.mode !== 'media') return;
    const img = this.$.media.querySelector('img');
    if (!img) return;
    if (this.fit) { this.zoom = (parseFloat(img.style.width) || img.naturalWidth) / (img.naturalWidth || 1); this.fit = false; }
    this.zoom = Math.max(0.05, Math.min(32, this.zoom * k));
    this.transform();
  }

  async showDoc() {
    const id = this.id;
    if (this.docFor === id) return;
    const text = await go.Lister.Document(id);
    if (this.id !== id) return;
    const md = this.info.suggest === 'markdown';
    const { css, html } = sanitizeHTML(md ? markdownToHTML(text) : text);
    this.$.doc.textContent = '';
    const host = document.createElement('div');
    host.className = 'mc-viewer-dochost' + (md ? ' mc-viewer-md' : '');
    const root = host.attachShadow({ mode: 'open' });
    const base = md
      ? ':host{display:block;color:var(--text);font:14px/1.55 -apple-system,"Segoe UI",system-ui,sans-serif;padding:14px 22px;max-width:920px}pre,code{font:12px ui-monospace,Menlo,Consolas,monospace;background:color-mix(in srgb,var(--text) 8%,transparent);border-radius:4px}pre{padding:8px 10px;overflow:auto}code{padding:1px 4px}pre code{padding:0;background:none}a{color:var(--accent)}table{border-collapse:collapse}td,th{border:1px solid var(--line);padding:3px 8px}blockquote{margin:0;padding-left:12px;border-left:3px solid var(--line);color:var(--muted)}img{max-width:100%}h1,h2{border-bottom:1px solid var(--line);padding-bottom:4px}'
      : ':host{display:block;background:#fff;color:#000;font:15px/1.4 "Times New Roman",serif;padding:10px 14px;min-height:100%}img{max-width:100%}';
    root.innerHTML = `<style>${base}\n${css}</style><div class="mc-viewer-docbody">${html}</div>`;
    this.$.doc.append(host);
    this.$.doc.scrollTop = 0;
    this.docFor = id;
  }

  // showFolder is quick view on a folder: its name and, once counted, its size (spec area 4, section 2).
  async showFolder(name, path) {
    this.close();
    this.folder = { name, path };
    this.mode = 'text';
    this.$.text.hidden = this.$.sb.hidden = this.$.media.hidden = this.$.doc.hidden = true;
    const n = this.$.note;
    n.hidden = false;
    n.innerHTML = `<div class="mc-viewer-folder"><b>${esc(name)}</b><span>Folder</span><span class="mc-viewer-count">Counting…</span></div>`;
    this.status();
    try {
      const r = await track(go.Lister.FolderSize('quickview', path));
      if (this.folder?.path !== path) return;
      n.querySelector('.mc-viewer-count').textContent = `${fmtSize(r.bytes)} (${fmtBytes(r.bytes)} bytes) in ${fmtBytes(r.files)} ${r.files === 1 ? 'file' : 'files'} and ${fmtBytes(r.dirs)} ${r.dirs === 1 ? 'folder' : 'folders'}`;
    } catch (e) {
      if (this.folder?.path === path) n.querySelector('.mc-viewer-count').textContent = errText(e);
    }
  }

  // showNote replaces the view with a sentence (quick view on "..", or a file that can't be read).
  showNote(text) {
    this.close();
    this.$.text.hidden = this.$.sb.hidden = this.$.media.hidden = this.$.doc.hidden = true;
    this.$.note.hidden = false;
    this.$.note.innerHTML = `<p>${esc(text)}</p>`;
    this.status();
  }

  // ---- reading the mouse ----

  rowAt(y) {
    const lines = this.$.rows.children;
    if (!lines.length) return -1;
    for (let i = 0; i < lines.length; i++) {
      const r = lines[i].getBoundingClientRect();
      if (y < r.bottom) return i;
    }
    return lines.length - 1;
  }

  // offsetAt turns a point into a byte offset of the shown data.
  offsetAt(x, y) {
    const p = this.page;
    if (!p || !p.rows.length) return this.top;
    const box = this.$.text.getBoundingClientRect();
    if (y < box.top) return p.rows[0].o;
    if (y > box.bottom) return p.rows[p.rows.length - 1].e;
    const i = this.rowAt(y);
    const r = p.rows[i];
    const line = this.$.rows.children[i];
    if (this.mode === 'hex') {
      const el = document.elementFromPoint(x, Math.min(Math.max(y, line.getBoundingClientRect().top + 1), line.getBoundingClientRect().bottom - 1))?.closest('[data-b]');
      if (el && line.contains(el)) return r.o + +el.dataset.b;
      return x < line.querySelector('.mc-viewer-hex')?.getBoundingClientRect().left ? r.o : r.e;
    }
    const lr = line.getBoundingClientRect();
    const cy = Math.min(Math.max(y, lr.top + 1), lr.bottom - 1);
    let node = null, off = 0;
    if (document.caretRangeFromPoint) {
      const rg = document.caretRangeFromPoint(x, cy);
      if (rg) { node = rg.startContainer; off = rg.startOffset; }
    } else if (document.caretPositionFromPoint) {
      const cp = document.caretPositionFromPoint(x, cy);
      if (cp) { node = cp.offsetNode; off = cp.offset; }
    }
    const span = node && (node.nodeType === 3 ? node.parentElement : node)?.closest?.('[data-r]');
    if (!span || !line.contains(span)) {
      const last = line.lastElementChild?.getBoundingClientRect();
      return last && x >= last.right ? this.rowEndText(r) : r.o;
    }
    const k = +span.dataset.r + Array.from((node.nodeType === 3 ? node.textContent : span.textContent).slice(0, node.nodeType === 3 ? off : 0)).length;
    return k < r.m.length ? r.o + r.m[k] : this.rowEndText(r);
  }

  // rowEndText is where the text of a row ends: its end, without the line break of a line's last row.
  rowEndText(r) {
    const rows = this.page.rows;
    const next = rows[rows.indexOf(r) + 1];
    if (next && !next.l) return r.e; // a wrapped row goes on in the next row
    if (!r.m.length) return r.o;
    const chars = Array.from(r.t);
    return Math.min(r.e, r.o + r.m[r.m.length - 1] + this.charBytes(chars[chars.length - 1]));
  }

  // charBytes is how many bytes a character takes in the encoding shown.
  charBytes(ch) {
    const cp = ch ? ch.codePointAt(0) : 32;
    const enc = this.asText ? 'utf-8' : this.info?.encoding || 'utf-8';
    if (enc === 'utf-8') return cp < 0x80 ? 1 : cp < 0x800 ? 2 : cp < 0x10000 ? 3 : 4;
    if (enc.startsWith('utf-16')) return cp < 0x10000 ? 2 : 4;
    if (/^(shift_jis|euc-jp|gbk|gb18030|big5|euc-kr)$/.test(enc)) return cp < 0x80 ? 1 : 2;
    return 1;
  }

  onMouseDown(ev) {
    if (ev.button !== 0 || !this.page) return;
    ev.preventDefault();
    this.focus();
    const pos = this.offsetAt(ev.clientX, ev.clientY);
    if (ev.shiftKey && this.sel) this.sel.b = pos; else this.sel = { a: pos, b: pos };
    this.render();
    this.status();
    let auto = 0, lastY = ev.clientY, lastX = ev.clientX;
    const move = (e) => {
      lastY = e.clientY; lastX = e.clientX;
      this.sel.b = this.offsetAt(e.clientX, e.clientY);
      this.render();
      this.status();
    };
    const tick = () => {
      const box = this.$.text.getBoundingClientRect();
      if (lastY < box.top) this.refresh('rows', -1).then(() => { this.sel.b = this.page.rows[0]?.o ?? this.sel.b; this.render(); });
      else if (lastY > box.bottom) this.refresh('rows', 1).then(() => { this.sel.b = this.page.rows.at(-1)?.e ?? this.sel.b; this.render(); });
      else if (lastX) { /* inside: nothing to scroll */ }
    };
    auto = setInterval(tick, 60);
    const up = () => {
      clearInterval(auto);
      window.removeEventListener('mousemove', move);
      window.removeEventListener('mouseup', up);
      if (this.sel && this.sel.a === this.sel.b) { this.sel = null; this.render(); this.status(); }
    };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }

  onDblClick(ev) {
    if (!this.page || this.mode === 'hex') return;
    const i = this.rowAt(ev.clientY);
    const r = this.page.rows[i];
    if (!r) return;
    const pos = this.offsetAt(ev.clientX, ev.clientY);
    const chars = Array.from(r.t);
    let k = r.m.findIndex((m) => r.o + m >= pos);
    if (k < 0) k = chars.length - 1;
    if (r.o + r.m[k] > pos && k > 0) k--;
    if (!chars[k] || !WORD.test(chars[k])) return;
    let s = k, e = k;
    while (s > 0 && WORD.test(chars[s - 1])) s--;
    while (e + 1 < chars.length && WORD.test(chars[e + 1])) e++;
    this.sel = { a: r.o + r.m[s], b: e + 1 < r.m.length ? r.o + r.m[e + 1] : this.rowEndText(r) };
    this.render();
    this.status();
  }

  onWheel(ev) {
    ev.preventDefault();
    if (ev.ctrlKey || ev.metaKey) { this.fontBy(ev.deltaY < 0 ? 1 : -1); return; }
    if (Math.abs(ev.deltaX) > Math.abs(ev.deltaY)) { this.$.text.scrollLeft += ev.deltaX; return; }
    this.wheelAcc = (this.wheelAcc || 0) + (ev.deltaMode === 1 ? ev.deltaY * this.lh : ev.deltaY);
    const n = Math.trunc(this.wheelAcc / this.lh);
    if (!n) return;
    this.wheelAcc -= n * this.lh;
    this.scrollRows(n);
  }

  onScrollbar(ev) {
    ev.preventDefault();
    const sb = this.$.sb.getBoundingClientRect();
    const th = this.$.thumb.getBoundingClientRect();
    if (ev.target !== this.$.thumb) { // a click beside the thumb pages, like any scroll bar
      this.scrollRows(ev.clientY < th.top ? -(this.rows - 1) : this.rows - 1);
      return;
    }
    const grab = ev.clientY - th.top;
    let pending = 0;
    const move = (e) => {
      const pct = Math.max(0, Math.min(100, ((e.clientY - grab - sb.top) / Math.max(1, sb.height - th.height)) * 100));
      if (pending) return;
      pending = 1;
      this.refresh(pct >= 100 ? 'end' : 'percent', pct).finally(() => { pending = 0; });
    };
    const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up); };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
  }

  // ---- actions ----

  fontBy(d) {
    this.fontSize = Math.max(8, Math.min(32, this.fontSize + d));
    saveListerSettings(this.app, { fontSize: this.fontSize });
    this.applyFont();
    this.layout();
    if (!this.wrap || this.mode !== 'text') this.refresh('top');
  }

  setEncoding(enc) {
    if (!this.id) return;
    this.enc = enc;
    if (this.mode === 'html') { this.docFor = 0; this.setMode('html'); return; }
    if (!this.isTextMode()) this.mode = 'text';
    this.showPane();
    this.queue(() => this.applyOptions());
  }

  toggleWrap() {
    this.wrap = !this.wrap;
    saveListerSettings(this.app, { wrap: this.wrap });
    this.$.text.scrollLeft = 0;
    if (this.isTextMode()) this.queue(() => this.applyOptions());
    else this.status();
  }

  toggleVariable() {
    this.variable = !this.variable;
    saveListerSettings(this.app, { font: this.variable ? 'variable' : 'fixed' });
    this.applyFont();
    this.layout();
    if (this.isTextMode()) this.queue(() => this.applyOptions());
  }

  toggleFit() {
    this.fit = !this.fit;
    if (!this.fit) this.zoom = 1;
    saveListerSettings(this.app, { fitImage: this.fit });
    this.transform();
  }

  rotate(d) { if (this.mode === 'media') { this.rot = (this.rot + d + 360) % 360; this.transform(); } }

  selectAll() {
    if (!this.info || !this.isTextMode()) return;
    this.sel = { a: this.mode === 'text' ? this.info.dataStart : 0, b: this.info.viewSize };
    this.render();
    this.status();
  }

  async copy(asHex = false) {
    const sr = this.selRange();
    if (!sr) { this.note('Nothing is selected. Drag over the text, or press Ctrl+A to select all.'); return; }
    await this.queue(async () => {
      const n = await go.Lister.Copy(this.id, sr[0], sr[1], asHex);
      this.note(`Copied ${fmtBytes(n)} bytes${asHex ? ' as hex' : ''}.`);
    });
  }

  async reload() {
    if (!this.id) return;
    await this.queue(async () => {
      const c = await go.Lister.Reload(this.id);
      this.info.size = c.size || this.info.size;
      if (!c.changed) { this.note('The file has not changed.'); return; }
      const info = await go.Lister.SetOptions(this.id, this.engineOptions());
      Object.assign(this.info, { viewSize: info.viewSize, offsetDigits: info.offsetDigits, dataStart: info.dataStart });
      if (this.mode === 'media' || this.mode === 'html') {
        this.dropMedia();
        this.info.media = normMedia(await go.Lister.Media(this.id));
        await (this.mode === 'media' ? this.showMedia() : this.showDoc());
      } else await this.move(this.page?.atEnd && c.grown ? 'end' : 'top');
      this.note(c.truncated ? 'The file got shorter.' : c.replaced ? 'The file was replaced.' : 'File read again.');
    });
  }

  async toggleFollow() {
    if (!this.id) return;
    this.follow = !this.follow;
    await track(go.Lister.Follow(this.id, this.follow));
    this.status();
  }

  // changed: the followed file changed on disk.
  changed(d) {
    if (!this.follow) return;
    this.info.size = d.size;
    this.queue(async () => {
      const info = await go.Lister.SetOptions(this.id, this.engineOptions());
      this.info.viewSize = info.viewSize;
      if (this.isTextMode()) await this.move(this.page?.atEnd || d.truncated ? 'end' : 'top');
    });
  }

  progress(d) {
    if (!this.searching) return;
    this.msg = `Searching… ${d.size ? Math.round((d.pos / d.size) * 100) : 0}% (Esc stops)`;
    this.status();
  }

  // cancelWork stops a search in progress; true when there was one (Esc then does not close the window).
  cancelWork() {
    if (!this.searching) return false;
    go.Lister.Cancel(this.id).catch(() => {});
    return true;
  }

  async find() {
    if (!this.id) return;
    if (!this.isTextMode()) { await this.setMode('text'); }
    const s = listerSettings(this.app);
    const q = lastQuery || {};
    const sr = this.selRange();
    let initial = q.pattern || '';
    if (sr && sr[1] - sr[0] <= 200) { try { initial = await go.Lister.Text(this.id, sr[0], sr[1], !!q.hex); } catch { /* keep the last text */ } }
    const hist = (s.findHistory || []).map((h) => `<option value="${esc(h)}"></option>`).join('');
    const chk = (id, label, on) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${label}</label>`;
    const html = `<div class="mc-row"><label for="mc-viewer-find">Search for:</label><input id="mc-viewer-find" type="text" spellcheck="false" autofocus list="mc-viewer-find-hist" value="${esc(initial)}" data-select="0,${initial.length}"><datalist id="mc-viewer-find-hist">${hist}</datalist></div>
      <div class="mc-viewer-findopts">
        ${chk('mc-viewer-find-words', 'Whole words only', q.wholeWords)}${chk('mc-viewer-find-case', 'Case sensitive', q.caseSensitive)}
        ${chk('mc-viewer-find-hex', 'Hex', q.hex)}${chk('mc-viewer-find-back', 'Search backwards', q.backward)}
        ${chk('mc-viewer-find-regex', 'Regular expressions', q.regex)}
      </div>
      <div class="mc-row"><label for="mc-viewer-find-enc">Search the text as</label><select id="mc-viewer-find-enc">
        <option value="">Shown (${esc(this.info.encodingName || this.info.encoding)})</option><option value="utf-8"${q.encoding === 'utf-8' ? ' selected' : ''}>UTF-8</option><option value="unicode"${q.encoding === 'unicode' ? ' selected' : ''}>Unicode (UTF-16)</option></select></div>`;
    const res = await formDialog('Find', html, (box) => {
      const v = (id) => box.querySelector(`#${id}`);
      const pattern = v('mc-viewer-find').value;
      if (!pattern) return 'Type the text to search for.';
      return { pattern, wholeWords: v('mc-viewer-find-words').checked, caseSensitive: v('mc-viewer-find-case').checked, hex: v('mc-viewer-find-hex').checked,
        backward: v('mc-viewer-find-back').checked, regex: v('mc-viewer-find-regex').checked, encoding: v('mc-viewer-find-enc').value };
    }, null, { okLabel: 'Find', kind: 'lister-find' });
    this.focus();
    if (!res) return;
    lastQuery = res;
    const h = [res.pattern, ...(s.findHistory || []).filter((x) => x !== res.pattern)].slice(0, 20);
    saveListerSettings(this.app, { findHistory: h });
    await this.findNext(res.backward);
  }

  // findNext repeats the last search forwards (F3) or backwards (Shift+F3).
  async findNext(backward = false) {
    if (!this.id) return;
    if (!lastQuery) { await this.find(); return; }
    if (!this.isTextMode()) await this.setMode('text');
    const q = { ...lastQuery, backward };
    const sr = this.selRange();
    const at = this.match ? [this.match.o, this.match.o + this.match.l] : sr;
    const from = backward ? (at ? at[0] : this.page?.rows.at(-1)?.e ?? this.top) : at ? (at[1] > at[0] ? at[1] : at[0] + 1) : this.top;
    this.searching = true;
    this.msg = 'Searching…';
    this.status();
    let m;
    try {
      m = await track(go.Lister.Find(this.id, q, from));
    } catch (e) {
      this.searching = false;
      this.msg = '';
      this.status();
      const t = errText(e);
      if (t !== 'Search stopped.') await messageDialog({ Title: 'Find', Message: t, Kind: 'lister-error' });
      else this.note(t);
      this.focus();
      return;
    }
    this.searching = false;
    this.msg = '';
    if (!m.found) {
      this.status();
      await messageDialog({ Title: 'Find', Message: `"${q.pattern}" was not found${backward ? ' before this point' : ''}.`, Kind: 'lister-notfound' });
      this.focus();
      return;
    }
    this.match = { o: m.offset, l: m.length };
    this.sel = { a: m.offset, b: m.offset + m.length };
    await this.refresh('reveal', m.offset);
    this.revealColumn();
  }

  // revealColumn scrolls sideways (wrap off) so the match is in view.
  revealColumn() {
    const el = this.$.rows.querySelector('.mc-viewer-hit, .mc-viewer-sel');
    if (!el) return;
    const t = this.$.text, r = el.getBoundingClientRect(), b = t.getBoundingClientRect();
    if (r.left < b.left || r.right > b.right) t.scrollLeft += r.left - b.left - b.width / 3;
  }

  async goTo() {
    if (!this.id) return;
    if (!this.isTextMode()) await this.setMode('text');
    const text = this.mode === 'text';
    const hint = text ? 'Line number, or an offset (0x1F40), or a percentage (50%):' : this.mode === 'hex' ? 'Offset in hex (1F40), or a percentage (50%):' : 'Offset (8000 or 0x1F40), or a percentage (50%):';
    const v = await inputDialog({ title: 'Go to', label: hint, value: '', kind: 'lister-goto', okLabel: 'Go',
      validate: (s) => (/^\s*(\d+(\.\d+)?\s*%|(0x|\$)[0-9a-f]+|[0-9a-f]+)\s*$/i.test(s) ? '' : 'Type a line number, an offset or a percentage.') });
    this.focus();
    if (v == null) return;
    const s = v.trim();
    let m;
    if ((m = s.match(/^(\d+(?:\.\d+)?)\s*%$/))) { await this.refresh(+m[1] >= 100 ? 'end' : 'percent', +m[1]); return; }
    if ((m = s.match(/^(?:0x|\$)([0-9a-f]+)$/i))) { await this.refresh('offset', parseInt(m[1], 16)); return; }
    if (this.mode === 'hex') { await this.refresh('offset', parseInt(s, 16)); return; }
    if (!/^\d+$/.test(s)) { this.note('Type a line number, an offset or a percentage.', true); return; }
    if (text) {
      await this.queue(async () => {
        this.searching = true;
        try {
          const off = await go.Lister.LineOffset(this.id, parseInt(s, 10));
          await this.move('offset', off);
        } finally { this.searching = false; }
      });
    } else await this.refresh('offset', parseInt(s, 10));
  }

  async saveAs() {
    if (!this.id) return;
    const sr = this.selRange();
    const sep = this.app.sepFor(this.info.path);
    const dir = this.info.path.slice(0, this.info.path.lastIndexOf(sep) + 1);
    const dot = this.info.name.lastIndexOf('.');
    const base = dot > 0 ? this.info.name.slice(0, dot) : this.info.name;
    const ext = dot > 0 ? this.info.name.slice(dot) : '';
    const suggestion = dir + (sr ? `${base} (selection)${this.asText ? '.txt' : ext}` : `${base} (copy)${ext}`);
    const dest = await inputDialog({ title: sr ? 'Save selection as' : 'Save as', label: 'Save to:', value: suggestion, kind: 'lister-saveas', okLabel: 'Save',
      select: [dir.length, suggestion.length - (sr && this.asText ? 4 : ext.length)] });
    this.focus();
    if (!dest) return;
    const path = this.app.resolvePath(dest, dir.replace(/[\\/]$/, '') || dir);
    let overwrite = false;
    try {
      await go.FS.Stat(path);
      const a = await messageDialog({ Title: 'Save as', Message: `${path} is already there. Replace it?`, Buttons: ['Replace', 'Cancel'], DefaultButton: 'Cancel', Kind: 'lister-replace' });
      this.focus();
      if (a !== 'Replace') return;
      overwrite = true;
    } catch { /* not there: fine */ }
    await this.queue(async () => {
      const [a, b] = sr || [0, this.info.viewSize];
      const n = await go.Lister.SaveRange(this.id, a, b, path, overwrite);
      this.note(`Saved ${fmtBytes(n)} bytes to ${path}.`);
    });
  }

  async mediaInfo() {
    if (!this.id) return;
    const m = this.info.media || normMedia(await track(go.Lister.Media(this.id)));
    if (!m) { this.note('This is not an image or media file.'); return; }
    const row = (k, v) => (v ? `<tr><th>${esc(k)}</th><td>${esc(v)}</td></tr>` : '');
    const dur = m.duration ? `${(m.duration / 1e9).toFixed(1)} s` : '';
    let html = `<h2>${esc(this.info.name)}</h2><table class="mc-viewer-info">${row('Format', m.format)}${row('Type', m.mime)}
      ${row('Size', m.width ? `${m.width} × ${m.height} pixels` : '')}${row('Bit depth', m.bitDepth ? `${m.bitDepth} bits` : '')}${row('Frames', m.frames > 1 ? String(m.frames) : '')}
      ${row('Length', dur)}${row('Sample rate', m.sampleRate ? `${m.sampleRate} Hz` : '')}${row('Channels', m.channels ? String(m.channels) : '')}${row('Bit rate', m.bitrate ? `${Math.round(m.bitrate / 1000)} kbit/s` : '')}
      ${row('Orientation', m.orientation > 1 ? String(m.orientation) : '')}${row('File size', `${fmtSize(this.info.size)} (${fmtBytes(this.info.size)} bytes)`)}`;
    for (const t of m.tags || []) html += row(t.name, t.value);
    html += '</table>';
    await infoDialog(html, 'mc-viewer-infobox', 'lister-info');
    this.focus();
  }

  async encodingMenu(anchor) {
    const list = await track(go.Lister.Encodings()).catch(() => []);
    const cur = this.enc ? (this.info?.encoding || '') : '';
    const items = [{ label: `${this.enc ? '' : '✓ '}Auto-detect`, value: '', detail: '' }];
    for (const e of list) items.push({ label: `${e.ID === cur ? '✓ ' : ''}${e.Name}`, detail: e.Group, value: e.ID });
    const r = await showList({ title: 'Encoding', items, anchor, index: Math.max(0, items.findIndex((i) => i.value === cur)), kind: 'lister-encoding' });
    this.focus();
    if (r) this.setEncoding(r.item.value);
  }

  altKey() { return keyLabel('Alt+F3', this.app.platform); }

  // ---- keys ----

  // key handles a key combo; true when it was used. Close and next/previous file are the Lister window's.
  key(combo) {
    // TC's Ctrl keys work everywhere; on macOS the usual Command keys are added (F-285)
    const m = (this.app.platform === 'darwin' && MAC_ALIASES[combo]) || combo;
    const textual = this.isTextMode();
    switch (m) {
      case '1': this.asText = ''; this.setMode('text'); return true;
      case '2': this.setMode('binary'); return true;
      case '3': this.setMode('hex'); return true;
      case '4': this.setMode('media'); return true;
      case '5': this.setMode('html'); return true;
      case '6': case 'U': this.setEncoding('unicode'); return true;
      case '7': case '8': this.setEncoding('utf-8'); return true;
      case 'A': this.setEncoding('ansi'); return true;
      case 'S': this.setEncoding('oem'); return true;
      case 'V': this.toggleVariable(); return true;
      case 'W': this.toggleWrap(); return true;
      case 'F': if (this.mode === 'media') this.toggleFit(); return true;
      case 'L': this.rotate(-90); return true;
      case 'R': this.rotate(90); return true;
      case 'I': this.mediaInfo(); return true;
      case '+': case '=': case 'Shift+=': case 'Num+': if (this.mode === 'media') this.zoomBy(1.25); else this.fontBy(1); return true;
      case '-': case 'Num-': if (this.mode === 'media') this.zoomBy(0.8); else this.fontBy(-1); return true;
      case 'Ctrl+=': case 'Ctrl+Shift+=': this.fontBy(1); return true;
      case 'Ctrl+-': this.fontBy(-1); return true;
      case 'F2': this.reload(); return true;
      case 'F7': case 'Ctrl+F': this.find(); return true;
      case 'F3': this.findNext(false); return true;
      case 'Shift+F3': this.findNext(true); return true;
      case 'Ctrl+G': this.goTo(); return true;
      case 'Ctrl+C': case 'Ctrl+Ins': this.copy(false); return true;
      case 'Ctrl+A': this.selectAll(); return true;
      case 'Ctrl+S': this.saveAs(); return true;
      case 'N': case 'P':
        if (!this.onNeighbor) return false;
        this.onNeighbor(m === 'N' ? 1 : -1);
        return true;
    }
    if (this.mode === 'html') {
      const d = this.$.doc;
      const step = { Up: -40, Down: 40, PgUp: -(d.clientHeight - 40), PgDn: d.clientHeight - 40, Space: d.clientHeight - 40, 'Shift+Space': -(d.clientHeight - 40) }[m];
      if (step) { d.scrollTop += step; return true; }
      if (m === 'Home' || m === 'Ctrl+Home' || m === 'Ctrl+Up') { d.scrollTop = 0; return true; }
      if (m === 'End' || m === 'Ctrl+End' || m === 'Ctrl+Down') { d.scrollTop = d.scrollHeight; return true; }
      return false;
    }
    if (this.mode === 'media') {
      const v = this.$.media.querySelector('video, audio');
      if (m === 'Space' && v) { if (v.paused) v.play().catch(() => {}); else v.pause(); return true; }
      return false;
    }
    if (!textual) return false;
    switch (m) {
      case 'Up': this.scrollRows(-1); return true;
      case 'Down': this.scrollRows(1); return true;
      case 'PgUp': case 'Shift+Space': this.scrollRows(-(this.rows - 1)); return true;
      case 'PgDn': case 'Space': this.scrollRows(this.rows - 1); return true;
      case 'Home': case 'Ctrl+Home': case 'Ctrl+Up': this.$.text.scrollLeft = 0; this.refresh('home'); return true;
      case 'End': case 'Ctrl+End': case 'Ctrl+Down': this.refresh('end'); return true;
      case 'Left': if (!(this.wrap && this.mode === 'text')) this.$.text.scrollLeft -= this.cw * 4; return true;
      case 'Right': if (!(this.wrap && this.mode === 'text')) this.$.text.scrollLeft += this.cw * 4; return true;
    }
    return false;
  }

  // ---- menus (the Lister window's menu bar) ----

  menuItems(which) {
    const k = (c) => keyLabel(c, this.app.platform);
    const ck = (on, label) => `${on ? '✓ ' : ''}${label}`;
    const textual = this.isTextMode();
    const media = this.mode === 'media';
    switch (which) {
      case 'File': return [
        { label: 'Next file', detail: 'N', run: () => this.onNeighbor?.(1), disabled: !this.onNeighbor },
        { label: 'Previous file', detail: 'P', run: () => this.onNeighbor?.(-1), disabled: !this.onNeighbor },
        { sep: true },
        { label: 'Reload', detail: k('F2'), run: () => this.reload() },
        { label: ck(this.follow, 'Follow file (auto-reload)'), run: () => this.toggleFollow() },
        { sep: true },
        { label: this.selRange() ? 'Save selection as…' : 'Save as…', detail: k('Ctrl+S'), run: () => this.saveAs() },
        { label: 'Open with another app', detail: this.altKey(), run: () => this.app.run('cm_ListExternal', { path: this.info?.path, external: true }) },
        { sep: true },
        { label: 'Close', detail: 'Esc', run: () => this.onClose?.(), disabled: !this.onClose },
      ];
      case 'Edit': return [
        { label: 'Copy', detail: k('Ctrl+C'), run: () => this.copy(false), disabled: !textual },
        { label: 'Copy as hex', run: () => this.copy(true), disabled: !textual },
        { label: 'Select all', detail: k('Ctrl+A'), run: () => this.selectAll(), disabled: !textual },
        { sep: true },
        { label: 'Find…', detail: 'F7', run: () => this.find() },
        { label: 'Find next', detail: 'F3', run: () => this.findNext(false) },
        { label: 'Find previous', detail: k('Shift+F3'), run: () => this.findNext(true) },
        { sep: true },
        { label: 'Go to…', detail: k('Ctrl+G'), run: () => this.goTo() },
      ];
      case 'Options': return [
        { label: ck(this.mode === 'text' && !this.asText, 'Text'), detail: '1', run: () => { this.asText = ''; this.setMode('text'); } },
        { label: ck(this.mode === 'binary', 'Binary'), detail: '2', run: () => this.setMode('binary') },
        { label: ck(this.mode === 'hex', 'Hex'), detail: '3', run: () => this.setMode('hex') },
        { label: ck(media, 'Image/Multimedia'), detail: '4', run: () => this.setMode('media'), disabled: this.internalOnly },
        { label: ck(this.mode === 'html' || this.asText === 'html', 'HTML/Markdown'), detail: '5', run: () => this.setMode('html') },
        { sep: true },
        { label: ck(this.info?.encoding === 'utf-16le' && !!this.enc, 'Unicode'), detail: '6 / U', run: () => this.setEncoding('unicode') },
        { label: ck(this.info?.encoding === 'utf-8' && !!this.enc, 'UTF-8'), detail: '7 / 8', run: () => this.setEncoding('utf-8') },
        { label: ck(this.enc === 'ansi', 'ANSI (Windows charset)'), detail: 'A', run: () => this.setEncoding('ansi') },
        { label: ck(this.enc === 'oem', 'ASCII (DOS charset)'), detail: 'S', run: () => this.setEncoding('oem') },
        { label: ck(!this.enc, 'Auto-detect encoding'), run: () => this.setEncoding('') },
        { sep: true },
        { label: ck(this.variable, 'Variable width font'), detail: 'V', run: () => this.toggleVariable() },
        { label: ck(this.wrap, 'Wrap text'), detail: 'W', run: () => this.toggleWrap() },
        { label: 'Larger font', detail: k('Ctrl+='), run: () => this.fontBy(1) },
        { label: 'Smaller font', detail: k('Ctrl+-'), run: () => this.fontBy(-1) },
        { sep: true },
        { label: 'Lister options…', run: () => this.app.run('cm_ListerConfig') },
      ];
      case 'Image': return [
        { label: ck(this.fit, 'Fit image to window'), detail: 'F', run: () => this.toggleFit(), disabled: !media },
        { label: 'Zoom in', detail: '+', run: () => this.zoomBy(1.25), disabled: !media },
        { label: 'Zoom out', detail: '-', run: () => this.zoomBy(0.8), disabled: !media },
        { label: 'Original size', run: () => { this.fit = false; this.zoom = 1; this.transform(); }, disabled: !media },
        { sep: true },
        { label: 'Rotate left', detail: 'L', run: () => this.rotate(-90), disabled: !media },
        { label: 'Rotate right', detail: 'R', run: () => this.rotate(90), disabled: !media },
        { label: ck(this.flipH, 'Mirror'), run: () => { this.flipH = !this.flipH; this.transform(); }, disabled: !media },
        { sep: true },
        { label: 'Image and media info…', detail: 'I', run: () => this.mediaInfo() },
      ];
    }
    return [];
  }

  async menu(which, anchor) {
    if (which === 'Encoding') { await this.encodingMenu(anchor); return; }
    const items = this.menuItems(which);
    const r = await showList({ title: '', items, anchor, kind: 'lister-menu' });
    this.focus();
    if (r) r.item.run();
  }

  // quickMenu is the quick view panel's right-click menu (F-082 / F-589): the Lister menus in one list.
  quickItems() {
    if (!this.id) return [];
    const encoding = { label: 'Encoding…', run: () => this.encodingMenu(this.lastMenuAt || this.el) };
    return quickMenuItems({ File: this.menuItems('File'), Edit: this.menuItems('Edit'), Options: this.menuItems('Options'), Image: this.menuItems('Image') },
      { media: this.mode === 'media', encoding });
  }

  async quickMenu(at) {
    const items = this.quickItems();
    if (!items.length) return;
    this.lastMenuAt = at;
    const r = await showList({ title: '', items, anchor: at, kind: 'lister-quickmenu' });
    this.focus();
    if (r) r.item.run();
  }

  // ---- state for control mode ----

  state() {
    const p = this.page;
    const sr = this.selRange();
    return {
      id: this.id, path: this.info?.path ?? null, name: this.info?.name ?? null, mode: this.mode, asText: this.asText, encoding: this.info?.encoding ?? '',
      encodingName: this.info?.encodingName ?? '', forcedEncoding: this.enc, wrap: this.wrap, variable: this.variable, fontSize: this.fontSize,
      top: this.top, percent: p?.percent ?? 0, atEnd: p?.atEnd ?? false, line: p?.line ?? 0, rows: p ? p.rows.map((r) => (this.mode === 'hex' ? `${hexOffset(r.o, this.info.offsetDigits || 8)}: ${r.h} ${r.t}` : r.t)) : [],
      selection: sr ? { start: sr[0], end: sr[1] } : null, match: this.match ? { offset: this.match.o, length: this.match.l } : null,
      size: this.info?.size ?? 0, suggest: this.info?.suggest ?? '', media: this.info?.media ? { kind: this.info.media.kind, format: this.info.media.format, width: this.info.media.width, height: this.info.media.height } : null,
      mediaShown: !!this.$.media.querySelector('img, video, audio, embed'), mediaLoaded: !!this.$.media.querySelector('img')?.naturalWidth, docShown: !!this.$.doc.firstChild, fit: this.fit, zoom: this.zoom, rotate: this.rot,
      follow: this.follow, searching: this.searching, status: this.$.stl.textContent, statusRight: this.$.str.textContent, folder: this.folder?.path ?? null,
      note: this.$.note.hidden ? '' : this.$.note.textContent.trim(),
    };
  }

  // select sets the selection by bytes (control mode, for checks of copy and save).
  select(start, end) { this.sel = { a: start, b: end }; this.render(); this.status(); }

  // ---- ending ----

  close() {
    if (this.id) {
      views.delete(this.id);
      go.Lister.Close(this.id).catch(() => {});
    }
    this.id = 0;
    this.info = null;
    this.page = null;
    this.folder = null;
    this.$.rows.textContent = '';
    this.dropMedia();
  }

  destroy() {
    this.close();
    if (this.quick) go.Lister.CancelFolderSize('quickview').catch(() => {});
    this.ro.disconnect();
    clearTimeout(this.msgTimer);
    this.el.remove();
  }
}
