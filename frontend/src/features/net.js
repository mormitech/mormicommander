// The Net menu (FEATURES.md sections 15 and 30): FTP, FTPS, SFTP and WebDAV servers in a panel.
//
//   Ctrl+F        cm_FtpConnect        the connection manager: saved servers in folders; connect, new, duplicate,
//                                      edit, delete, folders, master password
//   Ctrl+N        cm_FtpNew            quick connect: type ftp://user@host:port/folder (sftp://, webdav:// too)
//   Ctrl+Shift+F  cm_FtpDisconnect     closes the connection of the panel and goes back to a local folder
//   Ctrl+Shift+M  cm_FtpTransferMode   binary → ASCII → automatic
//                 cm_FtpHiddenFiles    LIST -a (hidden files on FTP servers)
//                 cm_FtpSendCommand    a raw FTP command, the reply in a box
//
// A panel shows a server through the vfs router (feature_net*.go puts the connection layer on it), so typing a URL in
// the path bar works too. While a panel is on a server, a small bar above its path shows the kind of connection, the
// transfer mode (click to change), the protocol log and a Disconnect button (F-817). Questions only the user can
// answer while connecting (password, key passphrase, master password, SFTP host key) come from Go as "net-ask" events
// and are page dialogs of kind "net-<kind>", so control mode reads and answers them like any other.
//
// runNetJob copies or moves to, from and between servers with progress and the overwrite question (with Resume); it
// is exported (and set as app.netJob) for the copy/move commands: when a source or the target is a server URL
// (app.isRemote(path)), they hand the job to it instead of the local file job engine.

import './net.css';
import { command } from '../core/registry.js';
import { go, on, errText } from '../core/go.js';
import { modal, messageDialog, inputDialog, formDialog, infoDialog, esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { size, date, plural } from '../core/format.js';
import { track } from '../core/busy.js';
import { app as theApp } from '../core/app.js';

const SCHEMES = /^(ftp|ftps|ftpes|sftp|scp|webdav|webdavs|dav|davs):\/\//i;
export const isRemote = (p) => SCHEMES.test(p || '');
const kindOf = (p) => { const s = (/^([a-z]+):/i.exec(p || '') || [])[1]?.toLowerCase() || ''; return s.startsWith('ftp') ? 'ftp' : s === 'sftp' || s === 'scp' ? 'sftp' : s ? 'webdav' : ''; };
const keyOf = (p) => { const m = /^([a-z]+:\/\/[^/]*)/i.exec(p || ''); return m ? m[1].toLowerCase().replace(/^scp:/, 'sftp:') : ''; };
const base = (p) => p.replace(/[\\/]+$/, '').split(/[\\/]/).pop() || p;
const MODE_LABEL = { auto: 'Automatic', binary: 'Binary', ascii: 'ASCII' };
const KIND_LABEL = { ftp: 'FTP', sftp: 'SFTP', webdav: 'WebDAV' };

const opts = (app) => app.setting('net', {}) || {};
async function setOpt(app, key, value) { await app.setSetting('net', { ...opts(app), [key]: value }); }

// ---- questions from Go while connecting ----

const asks = new Map(); // id → close()

function answer(id, a) { asks.delete(id); go.Net.Answer(id, a).catch(() => {}); }

function showAsk(q) {
  if (q.kind === 'hostkey') return askHostKey(q);
  const two = q.kind === 'master-new';
  const secret = q.secret !== false;
  modal((box, done) => {
    asks.set(q.id, () => done(undefined));
    box.innerHTML = `<h2>${esc(q.title)}</h2>${q.message ? `<p class="mc-msg">${esc(q.message).replace(/\n/g, '<br>')}</p>` : ''}
      <div class="mc-row"><label for="value">${two ? 'Master password' : secret ? 'Password' : 'Answer'}</label><input id="value" type="${secret ? 'password' : 'text'}" class="mc-net-in" autofocus spellcheck="false"></div>
      ${two ? '<div class="mc-row"><label for="confirm">Type it again</label><input id="confirm" type="password" class="mc-net-in" spellcheck="false"></div>' : ''}
      <p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">OK</button></div>`;
    box.querySelector('.cancel').onclick = () => done(null);
    box.onsubmit = (ev) => {
      ev.preventDefault();
      const v = box.querySelector('#value').value;
      const err = box.querySelector('.mc-err');
      if (two && !v) { err.hidden = false; err.textContent = 'Type a master password.'; return; }
      if (two && v !== box.querySelector('#confirm').value) { err.hidden = false; err.textContent = 'The two passwords are not the same.'; return; }
      done(v);
    };
  }, null, { kind: `net-${q.kind}` }).then((v) => {
    if (v === undefined) return; // withdrawn by Go
    answer(q.id, v === null ? { ok: false } : { ok: true, value: v });
  });
}

function askHostKey(q) {
  modal((box, done) => {
    asks.set(q.id, () => done(undefined));
    const known = (q.known || []).map((k) => `<li><code>${esc(k)}</code></li>`).join('');
    box.innerHTML = `<h2>${esc(q.title)}</h2><p class="mc-msg">${esc(q.message)}</p>
      <dl class="mc-net-key"><dt>Server</dt><dd>${esc(q.host)}</dd><dt>Key</dt><dd>${esc(q.keyType)}</dd><dt>Fingerprint</dt><dd><code>${esc(q.fingerprint)}</code></dd>
      ${known ? `<dt>Saved before</dt><dd><ul>${known}</ul></dd>` : ''}</dl>
      <p class="mc-note">"Accept and save" remembers the key${q.saveFile ? ` in ${esc(q.saveFile)}` : ''}, so you are not asked again.</p>
      <div class="mc-btns"><button type="button" data-c="reject">Cancel</button><button type="button" data-c="once">Accept once</button><button type="submit" class="primary${q.status === 'changed' ? ' danger' : ''}" data-c="save">Accept and save</button></div>`;
    box.querySelectorAll('button[data-c]').forEach((b) => { b.onclick = (ev) => { ev.preventDefault(); done(b.dataset.c); }; });
    box.onsubmit = (ev) => { ev.preventDefault(); done(q.status === 'changed' ? 'reject' : 'save'); };
    // a changed key never gets Enter as "accept": the person has to choose it
    if (q.status === 'changed') setTimeout(() => box.querySelector('[data-c="reject"]').focus());
  }, 'reject', { kind: 'net-hostkey' }).then((c) => {
    if (c === undefined) return;
    answer(q.id, c === 'reject' ? { ok: false } : { ok: true, choice: c });
  });
}

// ---- connecting ----

// connecting runs fn (a Go connect call) with a "Connecting…" box that has a Cancel button (shown after a moment).
async function connecting(label, fn) {
  let close = () => {};
  let cancelled = false;
  let timer = 0;
  const show = () => {
    if (asks.size) { timer = setTimeout(show, 300); return; } // a question is open: it stays on top
    modal((box, done) => {
      close = () => done(true);
      box.innerHTML = `<h2>Connecting</h2><p class="mc-msg">Connecting to ${esc(label)}…</p><div class="mc-progress mc-net-busy"><i></i></div>
        <div class="mc-btns"><button type="button" class="cancel">Cancel</button></div>`;
      box.querySelector('.cancel').onclick = () => done(false);
      box.onsubmit = (ev) => ev.preventDefault();
    }, false, { kind: 'net-connecting' }).then((ok) => { if (!ok) { cancelled = true; go.Net.CancelConnect().catch(() => {}); } });
  };
  timer = setTimeout(show, 300);
  try {
    return await fn();
  } catch (e) {
    if (cancelled) return null;
    throw e;
  } finally {
    clearTimeout(timer);
    close();
  }
}

// openServer connects and shows the server in the panel (the active one by default); a site's local folder goes to
// the other panel.
async function openServer(app, call, label, panel = app.cur) {
  const r = await connecting(label, call);
  if (!r) return false;
  const ok = await panel.go(r.path);
  if (ok && r.localDir) await panel.other.go(r.localDir);
  if (ok) app.flash(`Connected to ${label}.`);
  return ok;
}

function remember(app, url) {
  const h = (app.setting('netHistory', []) || []).filter((x) => x !== url);
  h.unshift(url);
  app.setSetting('netHistory', h.slice(0, 20));
}

async function quickConnect(app, args = {}) {
  let url = args.url;
  if (!url) {
    const hist = app.setting('netHistory', []) || [];
    url = await formDialog('New FTP connection', `
      <div class="mc-row"><label for="url">Server address</label><input id="url" type="text" spellcheck="false" autofocus list="mc-net-hist" value="${esc(hist[0] || '')}" data-select="0,${(hist[0] || '').length}"></div>
      <datalist id="mc-net-hist">${hist.map((h) => `<option value="${esc(h)}">`).join('')}</datalist>
      <p class="mc-note">For example ftp://user@example.com:21/pub, sftp://user@host/home/user or webdavs://host/remote.php/dav. A name alone means FTP.</p>`,
    (box) => { const v = box.querySelector('#url').value.trim(); return v ? { url: v } : 'Type a server address.'; }, null, { okLabel: 'Connect', kind: 'net-quick' }).then((r) => r?.url);
    if (!url) return false;
  }
  const shown = url.replace(/^([a-z]+:\/\/[^:@/]*):[^@/]*@/i, '$1@'); // never keep a password in the history
  remember(app, shown);
  return openServer(app, () => go.Net.Connect(url), shown.replace(/^[a-z]+:\/\//i, ''));
}

// ---- disconnect ----

// localBefore is the last local folder in the panel's history (where Disconnect goes back to).
function localBefore(p) {
  for (let i = p.back.length - 1; i >= 0; i--) if (!isRemote(p.back[i])) return p.back[i];
  return theApp.home;
}

async function disconnect(app, path) {
  const key = keyOf(path);
  await go.Net.Disconnect(path);
  for (const p of app.panels) if (keyOf(p.path) === key) await p.go(localBefore(p));
  app.flash(`Disconnected from ${key.replace(/^[a-z]+:\/\//, '')}.`);
  refreshBars(app);
  return true;
}

// remotePanel is the panel a Net command works on: the active one when it shows a server, else the other one.
function remotePanel(app, kind) {
  for (const p of [app.cur, app.other]) if (isRemote(p.path) && (!kind || kindOf(p.path) === kind)) return p;
  return null;
}

// ---- the connection manager (Ctrl+F) ----

const PROTOCOLS = [['ftp', 'FTP'], ['ftpes', 'FTP with TLS (explicit, ftpes://)'], ['ftps', 'FTPS (implicit TLS, ftps://)'], ['sftp', 'SFTP (SSH)'], ['webdav', 'WebDAV (http)'], ['webdavs', 'WebDAV (https)']];
const DEFAULT_PORT = { ftp: 21, ftpes: 21, ftps: 990, sftp: 22, webdav: 80, webdavs: 443 };

async function manager(app, args = {}) {
  let list = await go.Net.Sites();
  const open = new Set(app.setting('netOpenFolders', null) ?? list.folders);
  let selected = args.select || app.setting('netLastSite', '') || '';
  return modal((box, done) => {
    box.classList.add('mc-net-mgrbox');
    box.innerHTML = `<h2>Connect to server</h2>
      <div class="mc-net-mgr">
        <select id="site" size="14" class="mc-net-list" aria-label="Saved connections"></select>
        <div class="mc-net-side">
          <button type="submit" class="primary" data-a="connect">Connect</button>
          <button type="button" data-a="new">New connection…</button>
          <button type="button" data-a="url">New URL…</button>
          <button type="button" data-a="copy">Duplicate…</button>
          <button type="button" data-a="folder">New folder…</button>
          <button type="button" data-a="edit">Edit…</button>
          <button type="button" data-a="delete">Delete</button>
          <button type="button" data-a="master">Master password…</button>
        </div>
      </div>
      <p class="mc-note mc-net-info"></p><p class="mc-err" hidden></p>
      <div class="mc-btns"><button type="button" class="cancel">Close</button></div>`;
    const sel = box.querySelector('#site'), info = box.querySelector('.mc-net-info'), err = box.querySelector('.mc-err');
    const showErr = (e) => { err.hidden = !e; err.textContent = e ? errText(e) : ''; };
    const cur = () => sel.value || '';
    const siteOf = (v) => (v.startsWith('s:') ? list.sites.find((s) => s.id === v.slice(2)) : null);
    const folderOf = (v) => (v.startsWith('f:') ? v.slice(2) : siteOf(v)?.folder || '');
    const saveOpen = () => app.setSetting('netOpenFolders', [...open]);

    const paint = () => {
      const rows = [];
      const depth = (f) => (f ? f.split('/').length : 0);
      const visible = (f) => { if (!f) return true; const parts = f.split('/'); for (let i = 1; i <= parts.length; i++) if (!open.has(parts.slice(0, i).join('/'))) return false; return true; };
      const pad = (d) => '   '.repeat(d);
      const addFolder = (f) => {
        const parentVisible = visible(f.split('/').slice(0, -1).join('/'));
        if (parentVisible) rows.push({ v: `f:${f}`, t: `${pad(depth(f) - 1)}${open.has(f) ? '▾' : '▸'} ${f.split('/').pop()}` });
        for (const sub of list.folders.filter((x) => x.startsWith(f + '/') && !x.slice(f.length + 1).includes('/'))) addFolder(sub);
        if (visible(f)) for (const s of list.sites.filter((x) => x.folder === f)) rows.push({ v: `s:${s.id}`, t: `${pad(depth(f))}${s.name}  —  ${s.host}` });
      };
      for (const f of list.folders.filter((x) => !x.includes('/'))) addFolder(f);
      for (const s of list.sites.filter((x) => !x.folder)) rows.push({ v: `s:${s.id}`, t: `${s.name}  —  ${s.host}` });
      sel.innerHTML = rows.length ? rows.map((r) => `<option value="${esc(r.v)}">${esc(r.t)}</option>`).join('') : '<option value="" disabled>No saved connections yet: use "New connection…".</option>';
      const want = selected && rows.find((r) => r.v === selected || r.v === `s:${selected}`);
      sel.value = want ? want.v : rows[0]?.v || '';
      update();
    };
    const update = () => {
      const s = siteOf(cur());
      const f = cur().startsWith('f:');
      info.textContent = s ? `${PROTOCOLS.find((p) => p[0] === s.protocol)?.[1] || s.protocol} · ${s.user || (s.protocol === 'sftp' ? 'your login name' : 'anonymous')}@${s.host}${s.port ? `:${s.port}` : ''}${s.remoteDir ? ` · ${s.remoteDir}` : ''}${s.hasPassword ? ' · password saved' : ''}${s.comment ? ` · ${s.comment}` : ''}`
        : f ? `Folder ${cur().slice(2)}` : list.hasMaster ? 'Saved passwords are protected by the master password.' : '';
      for (const b of box.querySelectorAll('[data-a="connect"], [data-a="copy"], [data-a="edit"]')) b.disabled = !s && !(f && b.dataset.a === 'edit');
      box.querySelector('[data-a="connect"]').disabled = !s;
      box.querySelector('[data-a="delete"]').disabled = !cur();
    };
    const reload = async (pick) => { list = await go.Net.Sites(); if (pick) selected = pick; paint(); sel.focus(); };
    const toggle = (f) => { if (open.has(f)) open.delete(f); else open.add(f); saveOpen(); selected = `f:${f}`; paint(); };

    const act = async (a) => {
      showErr('');
      const v = cur(), s = siteOf(v);
      try {
        switch (a) {
          case 'connect': {
            if (v.startsWith('f:')) { toggle(v.slice(2)); return; }
            if (!s) return;
            app.setSetting('netLastSite', s.id);
            done({ connect: s });
            return;
          }
          case 'new': { const id = await editSite(app, { folder: folderOf(v), protocol: 'ftp', passive: true, useAgent: true }, list); if (id) await reload(`s:${id}`); return; }
          case 'url': { done({ quick: true }); return; }
          case 'copy': { if (!s) return; const id = await go.Net.CopySite(s.id); await reload(`s:${id}`); const c = list.sites.find((x) => x.id === id); if (c && await editSite(app, c, list)) await reload(`s:${id}`); return; }
          case 'folder': {
            const parent = folderOf(v);
            const name = await inputDialog({ title: 'New folder', label: parent ? `Folder in ${parent}` : 'Folder name', kind: 'net-folder',
              validate: (t) => (!t.trim() ? 'Type a name.' : /[/\\]/.test(t) ? 'A folder name can\'t have / or \\ in it.' : '') });
            if (!name) return;
            const f = parent ? `${parent}/${name.trim()}` : name.trim();
            await go.Net.AddFolder(f);
            open.add(f); if (parent) open.add(parent); saveOpen();
            await reload(`f:${f}`);
            return;
          }
          case 'edit': {
            if (s) { if (await editSite(app, s, list)) await reload(v); return; }
            if (v.startsWith('f:')) {
              const f = v.slice(2), parent = f.split('/').slice(0, -1).join('/');
              const name = await inputDialog({ title: 'Rename folder', label: 'Folder name', value: f.split('/').pop(), kind: 'net-folder',
                validate: (t) => (!t.trim() ? 'Type a name.' : /[/\\]/.test(t) ? 'A folder name can\'t have / or \\ in it.' : '') });
              if (!name || name.trim() === f.split('/').pop()) return;
              const to = parent ? `${parent}/${name.trim()}` : name.trim();
              await go.Net.RenameFolder(f, to);
              if (open.delete(f)) open.add(to); saveOpen();
              await reload(`f:${to}`);
            }
            return;
          }
          case 'delete': {
            if (s) {
              const b = await messageDialog({ Title: 'Delete connection', Message: `Delete the saved connection "${s.name}"?`, Buttons: ['Delete', 'Cancel'], Danger: true, Kind: 'net-delete' });
              if (b !== 'Delete') return;
              await go.Net.DeleteSite(s.id);
            } else if (v.startsWith('f:')) {
              const f = v.slice(2), n = list.sites.filter((x) => x.folder === f || x.folder.startsWith(f + '/')).length;
              const b = await messageDialog({ Title: 'Delete folder', Message: n ? `Delete the folder "${f}" and the ${plural(n, 'connection')} in it?` : `Delete the folder "${f}"?`, Buttons: ['Delete', 'Cancel'], Danger: true, Kind: 'net-delete' });
              if (b !== 'Delete') return;
              await go.Net.DeleteFolder(f);
            } else return;
            selected = '';
            await reload();
            return;
          }
          case 'master': { await go.Net.ChangeMasterPassword(); await reload(); app.flash('The master password is set.'); return; }
        }
      } catch (e) { showErr(e); }
    };
    box.querySelectorAll('[data-a]').forEach((b) => { if (b.type !== 'submit') b.onclick = () => act(b.dataset.a); });
    box.querySelector('.cancel').onclick = () => done(null);
    box.onsubmit = (ev) => { ev.preventDefault(); act('connect'); };
    sel.addEventListener('change', () => { selected = cur(); update(); });
    sel.addEventListener('dblclick', () => act('connect'));
    sel.addEventListener('keydown', (ev) => {
      const v = cur();
      if (ev.key === 'Enter') { ev.preventDefault(); act('connect'); }
      else if (ev.key === 'Delete') { ev.preventDefault(); act('delete'); }
      else if (ev.key === 'Insert') { ev.preventDefault(); act('new'); }
      else if (ev.key === 'F2' || ev.key === 'F4') { ev.preventDefault(); act('edit'); }
      else if (ev.key === 'F7') { ev.preventDefault(); act('folder'); }
      else if (v.startsWith('f:') && ((ev.key === 'ArrowRight' && !open.has(v.slice(2))) || (ev.key === 'ArrowLeft' && open.has(v.slice(2))))) { ev.preventDefault(); toggle(v.slice(2)); }
    });
    paint();
    setTimeout(() => sel.focus());
  }, null, { kind: 'net-connect' }).then(async (r) => {
    if (!r) return false;
    if (r.quick) return quickConnect(app);
    return openServer(app, () => go.Net.ConnectSite(r.connect.id), r.connect.name);
  });
}

// editSite shows the connection details; it resolves with the site's ID when saved, or null.
async function editSite(app, s, list) {
  const folders = ['', ...list.folders];
  const opt = (v, label, cur) => `<option value="${esc(v)}"${v === cur ? ' selected' : ''}>${esc(label)}</option>`;
  const chk = (id, label, on) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
  const txt = (id, label, v, extra = '') => `<div class="mc-row"><label for="${id}">${esc(label)}</label><input id="${id}" type="text" class="mc-net-in" spellcheck="false" value="${esc(v ?? '')}" ${extra}></div>`;
  const html = () => `<div class="mc-net-form">
    ${txt('name', 'Connection name', s.name, 'autofocus')}
    <div class="mc-row"><label for="folder">Folder</label><select id="folder">${folders.map((f) => opt(f, f || '(top level)', s.folder || '')).join('')}</select></div>
    <div class="mc-row"><label for="protocol">Protocol</label><select id="protocol">${PROTOCOLS.map(([v, l]) => opt(v, l, s.protocol || 'ftp')).join('')}</select></div>
    <div class="mc-net-2">${txt('host', 'Server (name, address or URL)', s.host)}${txt('port', 'Port', s.port || '', 'inputmode="numeric"')}</div>
    <div class="mc-net-2">${txt('user', 'User name', s.user)}<div class="mc-row"><label for="password">Password</label><input id="password" type="password" class="mc-net-in" placeholder="${s.hasPassword ? 'saved (type to change)' : 'asked when connecting'}"></div></div>
    <div class="mc-net-row">${chk('anon', 'Anonymous login', !s.user && (s.protocol || 'ftp').startsWith('ftp') && !!s.id)}${chk('savePassword', 'Save the password (encrypted with the master password)', s.savePassword)}</div>
    <div class="mc-net-2">${txt('remoteDir', 'Remote folder', s.remoteDir, 'placeholder="the login folder"')}${txt('localDir', 'Local folder (other panel)', s.localDir)}</div>
    <fieldset class="mc-net-ftp"><legend>FTP</legend>
      <div class="mc-net-row">${chk('passive', 'Passive mode (through firewalls)', s.passive !== false)}${chk('tryTLS', 'Use TLS when the server offers it', s.tryTLS)}${chk('showHidden', 'Show hidden files (LIST -a)', s.showHidden)}${chk('noMLSD', 'Don\'t use MLSD', s.noMLSD)}</div>
      <div class="mc-net-3">
        <div class="mc-row"><label for="transfer">Transfer mode</label><select id="transfer">${opt('auto', 'Automatic', s.transfer || 'auto')}${opt('binary', 'Binary', s.transfer)}${opt('ascii', 'ASCII (text)', s.transfer)}</select></div>
        <div class="mc-row"><label for="encoding">Charset of names</label><select id="encoding">${opt('auto', 'Automatic (UTF-8)', s.encoding || 'auto')}${opt('utf-8', 'UTF-8', s.encoding)}${opt('latin1', 'Western (Latin-1)', s.encoding)}</select></div>
        ${txt('timeOffset', 'Time offset (minutes)', s.timeOffset || '', 'inputmode="numeric"')}
      </div>
      ${txt('listCommand', 'List command', s.listCommand, 'placeholder="LIST"')}
    </fieldset>
    <fieldset class="mc-net-sftp"><legend>SFTP</legend>
      ${txt('keyFile', 'Private key file', s.keyFile, 'placeholder="~/.ssh/id_ed25519 (the usual keys are tried anyway)"')}
      ${chk('useAgent', 'Use the keys of the SSH agent', s.useAgent !== false)}
    </fieldset>
    <div class="mc-net-2">${txt('keepalive', 'Keep alive every (seconds)', s.keepalive || '', 'inputmode="numeric" placeholder="off"')}${txt('comment', 'Comment', s.comment)}</div>
  </div>`;
  const read = (box) => {
    const $ = (id) => box.querySelector(`#${id}`);
    const num = (id, lo, hi, what) => { const t = $(id).value.trim(); if (!t) return 0; const n = Number(t); return Number.isInteger(n) && n >= lo && n <= hi ? n : `${what} must be a whole number from ${lo} to ${hi}.`; };
    const port = num('port', 1, 65535, 'The port'); if (typeof port === 'string') return port;
    const timeOffset = num('timeOffset', -1440, 1440, 'The time offset'); if (typeof timeOffset === 'string') return timeOffset;
    const keepalive = num('keepalive', 0, 3600, 'The keep-alive time'); if (typeof keepalive === 'string') return keepalive;
    if (!$('name').value.trim()) return 'Give the connection a name.';
    if (!$('host').value.trim()) return 'Type the server\'s name or address.';
    return {
      id: s.id || '', name: $('name').value.trim(), folder: $('folder').value, protocol: $('protocol').value, host: $('host').value.trim(), port,
      user: $('anon').checked ? '' : $('user').value.trim(), password: $('password').value, savePassword: $('savePassword').checked,
      keyFile: $('keyFile').value.trim(), useAgent: $('useAgent').checked, remoteDir: $('remoteDir').value.trim(), localDir: $('localDir').value.trim(),
      passive: $('passive').checked, tryTLS: $('tryTLS').checked, transfer: $('transfer').value, encoding: $('encoding').value, showHidden: $('showHidden').checked,
      listCommand: $('listCommand').value.trim(), timeOffset, keepalive, noMLSD: $('noMLSD').checked, comment: $('comment').value.trim(),
    };
  };
  const setup = (box) => {
    box.classList.add('mc-net-editbox');
    const $ = (id) => box.querySelector(`#${id}`);
    const sync = () => {
      const p = $('protocol').value, ftp = p.startsWith('ftp');
      box.querySelector('.mc-net-ftp').hidden = !ftp;
      box.querySelector('.mc-net-sftp').hidden = p !== 'sftp';
      $('tryTLS').closest('label').hidden = p !== 'ftp';
      $('anon').closest('label').hidden = !ftp;
      $('port').placeholder = String(DEFAULT_PORT[p] || '');
      $('user').disabled = ftp && $('anon').checked;
      $('password').disabled = ftp && $('anon').checked;
    };
    $('protocol').addEventListener('change', sync);
    $('anon').addEventListener('change', sync);
    $('password').addEventListener('input', () => { if ($('password').value) $('savePassword').checked = true; });
    sync();
  };
  for (;;) {
    const v = await formDialog(s.id ? 'Connection details' : 'New connection', html(), read, setup, { okLabel: 'Save', kind: 'net-site' });
    if (!v) return null;
    try {
      return await go.Net.SaveSite(v);
    } catch (e) {
      await messageDialog({ Title: 'Not saved', Message: errText(e), Kind: 'net-error' });
      s = { ...s, ...v, password: '', hasPassword: s.hasPassword };
    }
  }
}

// ---- the bar above a server panel (F-817) ----

const bars = new Map(); // panel → element

function bar(p) {
  let el = bars.get(p);
  if (el) return el;
  el = document.createElement('div');
  el.className = 'mc-net-bar';
  el.hidden = true;
  el.innerHTML = '<span class="mc-net-what"></span><button type="button" class="mc-net-mode" title="Change the transfer mode (Ctrl+Shift+M)"></button><button type="button" class="mc-net-log" title="Show the protocol log">Log</button><button type="button" class="mc-net-off" title="Disconnect (Ctrl+Shift+F)">Disconnect</button>';
  p.$.pathbar.before(el);
  el.querySelector('.mc-net-mode').onclick = () => { theApp.setActive(p); theApp.run('cm_FtpTransferMode'); };
  el.querySelector('.mc-net-log').onclick = () => showLog(theApp, p.path);
  el.querySelector('.mc-net-off').onclick = () => { theApp.setActive(p); disconnect(theApp, p.path).catch((e) => theApp.flash(errText(e), 'err')); };
  bars.set(p, el);
  return el;
}

async function paintBar(p) {
  const el = bar(p);
  if (!isRemote(p.path)) { el.hidden = true; return; }
  let st;
  try { st = await go.Net.Status(p.path); } catch { st = { remote: true, kind: kindOf(p.path) }; }
  if (!isRemote(p.path)) { el.hidden = true; return; }
  el.hidden = false;
  el.querySelector('.mc-net-what').textContent = `${KIND_LABEL[st.kind] || 'Server'}${st.name ? ` · ${st.name}` : ''}${st.connected === false ? ' · not connected' : ''}`;
  const mode = el.querySelector('.mc-net-mode');
  mode.hidden = st.kind !== 'ftp';
  mode.textContent = MODE_LABEL[st.mode] || 'Automatic';
  el.querySelector('.mc-net-log').hidden = !st.log;
}

function refreshBars(app) { for (const p of app.panels) paintBar(p); }

async function showLog(app, path) {
  const lines = await go.Net.Log(path).catch(() => []);
  infoDialog(`<h2>Protocol log</h2><p class="mc-note">${esc(keyOf(path))}</p><pre class="mc-net-logtext">${esc(lines.join('\n') || 'Nothing logged yet.')}</pre>`, 'mc-net-logbox', 'net-log');
  setTimeout(() => { const pre = document.querySelector('.mc-net-logtext'); if (pre) pre.scrollTop = pre.scrollHeight; });
}

// ---- transfers to and from servers ----

const jobs = new Map(); // id → {resolve, update, close}
let jobsListening = false;
const JOB_BUTTONS = { Overwrite: 'overwrite', Resume: 'resume', Skip: 'skip', 'Overwrite all': 'overwrite_all', 'Resume all': 'resume_all', 'Skip all': 'skip_all', 'Overwrite all older': 'older_all', 'Rename…': 'rename', Cancel: 'cancel' };

function listenJobs() {
  if (jobsListening) return;
  jobsListening = true;
  on('net-job', (p) => jobs.get(p.id)?.update(p));
  on('net-job-ask', (p) => { jobs.get(p.id)?.update(p); askOverwrite(p); });
  on('net-job-done', (p) => { const j = jobs.get(p.id); if (!j) return; jobs.delete(p.id); j.close(); j.resolve(p); });
}

async function askOverwrite(p) {
  const c = p.ask;
  const name = base(c.target);
  const buttons = Object.keys(JOB_BUTTONS).filter((b) => c.canResume || !b.startsWith('Resume'));
  const msg = `${name} is already in the target folder.\n\nNew: ${size(c.srcSize)}, ${date(c.srcTime)}\nThere: ${size(c.dstSize)}, ${date(c.dstTime)}${c.canResume ? '\n\nThe file there is shorter: "Resume" copies only the rest.' : ''}`;
  const b = await messageDialog({ Title: 'Overwrite?', Message: msg, Buttons: buttons, DefaultButton: c.canResume ? 'Resume' : 'Overwrite', CancelButton: 'Cancel', Kind: 'net-overwrite' });
  const a = { choice: JOB_BUTTONS[b] || 'cancel', newName: '' };
  if (a.choice === 'rename') {
    const v = await inputDialog({ title: 'Copy under another name', label: 'New name', value: name, kind: 'rename-target' });
    if (v === null) a.choice = 'skip'; else a.newName = v;
  }
  go.Net.AnswerJob(p.id, a).catch(() => {});
}

// runNetJob copies or moves to, from or between servers: {kind: 'copy'|'move', sources: [full paths or URLs],
// target: folder, rename: '', overwrite: ''}. It resolves with the final progress (state done, failed or cancelled);
// both panels are reread afterwards.
export function runNetJob(req, { title } = {}) {
  listenJobs();
  return track((async () => {
    const id = await go.Net.StartJob(req).catch((e) => { throw new Error(errText(e)); });
    let p = { id, kind: req.kind, state: 'running', files: 0, filesDone: 0, bytes: 0, bytesDone: 0, current: '' };
    let close = () => {};
    let paint = () => {};
    const result = new Promise((resolve) => jobs.set(id, { resolve, update: (np) => { p = np; paint(); }, close: () => close() }));
    const showTimer = setTimeout(() => {
      modal((box, done) => {
        close = () => done();
        box.innerHTML = `<h2>${esc(title || (req.kind === 'move' ? 'Moving' : 'Copying'))}</h2><p class="mc-msg"></p><div class="mc-progress"><i></i></div><p class="mc-note"></p>
          <div class="mc-btns"><button type="button" class="cancel">Cancel</button></div>`;
        const msg = box.querySelector('.mc-msg'), barEl = box.querySelector('.mc-progress i'), note = box.querySelector('.mc-note');
        paint = () => {
          msg.textContent = p.current ? base(p.current) : '';
          const frac = p.bytes > 0 ? p.bytesDone / p.bytes : p.files > 0 ? p.filesDone / p.files : 0;
          barEl.style.width = `${Math.round(Math.min(1, frac) * 100)}%`;
          note.textContent = `${p.filesDone} of ${plural(p.files, 'file')}${p.bytes ? ` · ${size(p.bytesDone)} of ${size(p.bytes)}` : ''}`;
        };
        paint();
        box.querySelector('.cancel').onclick = () => go.Net.CancelJob(id);
        box.onsubmit = (ev) => ev.preventDefault();
      }, undefined, { kind: 'net-job' }).then(() => go.Net.CancelJob(id).catch(() => {}));
    }, 400);
    const final = await result;
    clearTimeout(showTimer);
    close();
    await Promise.all(theApp.panels.map((pn) => pn.reload()));
    if (final.state === 'failed') theApp.flash(final.errors.slice(0, 3).join('\n') + (final.errors.length > 3 ? `\n…and ${final.errors.length - 3} more.` : ''), 'err');
    else if (final.state === 'cancelled') theApp.flash('The transfer was stopped. Copy again and choose "Resume" to finish a file.');
    return final;
  })());
}

// ---- the download list (F-381): server files queued to be fetched later, one after the other ----
// Kept in the setting "netDownloadList" as [{src, target}] (target: the local folder), so it survives a restart. The
// dialog (kind "net-download-list") adds the selection of the server panel (to the other panel's folder), starts,
// removes and reorders; Start runs the transfers one by one with runNetJob and drops each one that is done.

const dlList = (app) => (app.setting('netDownloadList', []) || []).filter((x) => x && x.src && x.target);
const dlSave = (app, list) => app.setSetting('netDownloadList', list);

// addToDownloadList queues the selection (or the file under the cursor) of the server panel; true when added.
function addToDownloadList(app) {
  const p = isRemote(app.cur.path) ? app.cur : remotePanel(app);
  if (!p) { app.flash('Files can be added to the download list from a panel that shows a server.'); return false; }
  const o = p === app.left ? app.right : app.left;
  if (isRemote(o.path)) { app.flash('The other panel must show a folder on this computer: the files are downloaded there.'); return false; }
  const items = p.targets();
  if (!items.length) { app.flash('No files selected.'); return false; }
  const list = dlList(app);
  const have = new Set(list.map((x) => `${x.src}\n${x.target}`));
  let n = 0;
  for (const e of items) {
    const it = { src: p.fullPath(e.name), target: o.path };
    if (have.has(`${it.src}\n${it.target}`)) continue;
    list.push(it);
    n++;
  }
  dlSave(app, list);
  app.flash(n ? `Added ${plural(n, 'item')} to the download list.` : 'Those files are on the download list already.');
  return n > 0;
}

// runDownloadList fetches the queued items in order; it stops at the first one that fails or is stopped.
async function runDownloadList(app) {
  let done = 0;
  for (;;) {
    const list = dlList(app);
    if (!list.length) break;
    const it = list[0];
    const final = await runNetJob({ kind: 'copy', sources: [it.src], target: it.target, rename: '', overwrite: '' }, { title: `Downloading ${base(it.src)}` });
    if (final.state !== 'done') { app.flash(`The download list stopped at ${base(it.src)}; it stays on the list.`, 'err'); return done; }
    dlSave(app, dlList(app).filter((x) => !(x.src === it.src && x.target === it.target)));
    done++;
  }
  if (done) app.flash(`Downloaded ${plural(done, 'item')} from the download list.`);
  return done;
}

async function downloadList(app) {
  const start = await modal((box, done) => {
    box.classList.add('mc-net-dlbox');
    box.innerHTML = `<h2>Download list</h2><p class="mc-msg"></p>
      <div class="mc-net-mgr"><select id="items" class="mc-net-list" size="10" aria-label="Files to download"></select>
        <div class="mc-net-side"><button type="button" class="add" title="The selection of the server panel, to the other panel's folder">Add selected</button>
          <button type="button" class="up">Up</button><button type="button" class="down">Down</button><button type="button" class="remove">Remove</button></div></div>
      <div class="mc-btns"><button type="button" class="close">Close</button><button type="submit" class="primary">Start</button></div>`;
    const $ = (q) => box.querySelector(q);
    const sel = $('#items');
    const paint = (at = sel.selectedIndex) => {
      const list = dlList(app);
      sel.innerHTML = list.map((x, i) => `<option value="${i}">${esc(base(x.src))}  →  ${esc(x.target)}</option>`).join('');
      if (list.length) sel.selectedIndex = Math.max(0, Math.min(list.length - 1, at < 0 ? 0 : at));
      $('.mc-msg').textContent = list.length ? `${plural(list.length, 'item')} to download. Start fetches them one after the other.`
        : 'The list is empty. Select files in a server panel and press "Add selected".';
      for (const c of ['.up', '.down', '.remove']) $(c).disabled = !list.length;
      box.querySelector('button[type=submit]').disabled = !list.length;
    };
    const move = (d) => {
      const list = dlList(app), i = sel.selectedIndex, j = i + d;
      if (i < 0 || j < 0 || j >= list.length) return;
      [list[i], list[j]] = [list[j], list[i]];
      dlSave(app, list);
      paint(j);
    };
    $('.add').onclick = () => { addToDownloadList(app); paint(dlList(app).length - 1); };
    $('.up').onclick = () => move(-1);
    $('.down').onclick = () => move(1);
    $('.remove').onclick = () => {
      const i = sel.selectedIndex;
      if (i < 0) return;
      dlSave(app, dlList(app).filter((_, k) => k !== i));
      paint(i);
    };
    $('.close').onclick = () => done(false);
    box.onsubmit = (ev) => { ev.preventDefault(); if (dlList(app).length) done(true); };
    paint(0);
  }, false, {
    kind: 'net-download-list',
    onKey: (ev) => {
      if (ev.key === 'Delete' && ev.target.id === 'items') { document.querySelector('.mc-net-dlbox .remove')?.click(); return true; }
      return false;
    },
  });
  if (!start) return false;
  return runDownloadList(app);
}

// ---- commands ----

command({ name: 'cm_FtpDownloadList', label: 'Download list…', run: (app) => downloadList(app) });
command({ name: 'cm_FtpAddToList', label: 'Add to download list', run: (app) => addToDownloadList(app) });

command({ name: 'cm_FtpConnect', label: 'FTP connect…', short: 'FTP', run: (app, args) => manager(app, args) });
command({ name: 'cm_FtpNew', label: 'New FTP connection…', run: (app, args) => quickConnect(app, args) });
command({
  name: 'cm_FtpDisconnect', label: 'Disconnect',
  run: (app, args = {}) => {
    const p = args.path ? { path: args.path } : remotePanel(app);
    if (!p) { app.flash('No panel shows a server.'); return false; }
    return disconnect(app, p.path);
  },
});
command({
  name: 'cm_FtpHiddenFiles', label: 'Show hidden FTP files', checked: (app) => !!opts(app).showHidden,
  run: async (app) => {
    const on = !opts(app).showHidden;
    await setOpt(app, 'showHidden', on);
    await go.Net.ReconnectFTP().catch(() => []);
    await Promise.all(app.panels.filter((p) => kindOf(p.path) === 'ftp').map((p) => p.reload()));
    app.flash(on ? 'Hidden files on FTP servers are shown (LIST -a).' : 'Hidden files on FTP servers are not asked for.');
  },
});
command({
  name: 'cm_FtpTransferMode', label: 'Transfer mode',
  run: async (app, args = {}) => {
    const p = remotePanel(app, 'ftp');
    let mode;
    if (p) {
      mode = args.mode ? (await go.Net.SetTransferMode(p.path, args.mode), args.mode) : await go.Net.CycleTransferMode(p.path);
    } else {
      // no FTP panel: the default for the next connections
      const order = ['binary', 'ascii', 'auto'];
      const cur = opts(app).transferMode || 'auto';
      mode = args.mode || order[(order.indexOf(cur) + 1) % order.length];
      await setOpt(app, 'transferMode', mode);
    }
    refreshBars(app);
    app.flash(`Transfer mode: ${MODE_LABEL[mode] || mode}${p ? '' : ' (for new FTP connections)'}.`);
    return mode;
  },
});
command({
  name: 'cm_FtpSendCommand', label: 'Send FTP command…',
  run: async (app, args = {}) => {
    const p = remotePanel(app, 'ftp');
    if (!p) { app.flash('Commands can be sent when a panel shows an FTP server.'); return false; }
    const line = args.command ?? await inputDialog({ title: 'Send FTP command', label: `Command for ${keyOf(p.path).replace(/^[a-z]+:\/\//, '')}`, value: app.setting('netLastCommand', ''), note: 'For example SITE HELP or SITE CHMOD 644 file.txt. The reply is also in the protocol log.', kind: 'net-command' });
    if (!line) return false;
    app.setSetting('netLastCommand', line);
    const reply = await go.Net.SendCommand(p.path, line);
    refreshBars(app);
    await messageDialog({ Title: 'Server reply', Message: reply || '(no reply)', Kind: 'net-reply' });
    await p.reload();
    return reply;
  },
});

addControlCommand('net_job', async (m) => { // starts a transfer without waiting for it: {req: {kind, sources, target, overwrite}}
  runNetJob(m.req || {}).catch((e) => theApp.flash(errText(e), 'err'));
  return { ok: true };
});

export function init(app) {
  app.isRemote = isRemote;
  app.netJob = runNetJob;
  on('net-ask', (q) => showAsk(q));
  on('net-ask-gone', (id) => { const close = asks.get(id); asks.delete(id); close?.(); });
  // TC does not reopen server connections at start: a panel left on a server starts in its last local folder
  const session = app.setting('session', null);
  for (const side of ['left', 'right']) {
    const s = session?.[side];
    if (s && isRemote(s.path)) s.path = '';
  }
  // Enter on a server file opens a downloaded copy with its app
  app.openHandlers.push(async (panel, entry, path) => {
    if (!isRemote(path)) return false;
    app.flash(`Downloading ${entry.name}…`);
    const local = await go.Net.OpenCopy(path);
    await go.FS.OpenFile(local);
    app.flash(`Opened a copy of ${entry.name}.`);
    return true;
  });
  app.on('panel', (p, info) => { if (info?.moved || isRemote(p.path)) paintBar(p); });
  app.on('exchange', () => refreshBars(app));
  for (const p of app.panels) bar(p);
}
