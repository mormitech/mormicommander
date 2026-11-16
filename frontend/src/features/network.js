// Network neighbourhood (board issue #148; TC's cm_OpenNetwork, cm_NetConnect, cm_NetDisconnect; F-365, F-383, F-386).
//
// The Network button at the right end of the drive bar (or Net > Network neighbourhood) turns the panel into the
// network folder, network:// (Go: feature_network_fs.go), like Total Commander's Network Neighborhood. The first time,
// Go starts searching the local network, and keeps searching in the background while the app runs: servers appear in
// the panel as they answer, a banner over the list shows that it is still looking, and a panel that comes back to the
// folder shows at once what was found meanwhile. Enter on a server lists its shares (asking for a user name and
// password when the server wants them, with "Save the password" for the system keychain); Enter on a share connects
// it and shows its files, with ".." back to the shares. smb://server, nfs://server or afp://server typed in the path
// bar works the same way.
//
// Net > Connect network drive… types a share and maps it to stay (a drive letter on Windows, /Volumes on macOS, GVfs
// on Linux); Net > Disconnect network drive… unmounts one.
//
// The network is searched only when the user opens the folder (docs/privacy.md).

import './network.css';
import { command } from '../core/registry.js';
import { showList } from '../core/popup.js';
import { formDialog, inputDialog, esc } from '../core/dialogs.js';
import { go, on, errText } from '../core/go.js';
import { networkVolumes, typedAddress, hasShare, typedPath, isNetworkRoot, bannerText, netKey, isNetFolder, netParent } from './network-lib.js';
import { Panel } from '../core/panel.js';

let status = { searching: false, count: 0, note: '', opening: [] };

// ---- servers and shares being opened ----
// Go opens a server or share in the background, also when the panel has gone elsewhere meanwhile
// (feature_network_open.go), and says which ones are being opened ("network-opening"). Their rows get a spinner in every
// panel that shows them. pending remembers, per panel, the network folder it was asked to open last: when that one
// opens while the panel shows its parent again (the user went away and came back), the panel goes into it.
let opening = new Set();
const pending = new Map();

// askCreds asks for the user name and password of a server; null when cancelled.
function askCreds(server, prev = {}) {
  const chk = (id, on, label) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
  return formDialog(`Sign in to ${server}`,
    `<p>${esc(server)} wants a user name and password.</p>
     <div class="mc-row"><label for="net-user">User name:</label><input id="net-user" type="text" spellcheck="false" autocomplete="off" value="${esc(prev.user || '')}" autofocus></div>
     <div class="mc-row"><label for="net-pass">Password:</label><input id="net-pass" type="password" autocomplete="off"></div>
     <div class="mc-row"><label for="net-domain">Domain:</label><input id="net-domain" type="text" spellcheck="false" placeholder="(usually empty)"></div>
     ${chk('net-save', false, 'Save the password in the system keychain')}`,
    (box) => {
      const user = box.querySelector('#net-user').value.trim();
      if (!user) return 'Type a user name.';
      return { user, password: box.querySelector('#net-pass').value, domain: box.querySelector('#net-domain').value.trim(), save: box.querySelector('#net-save').checked };
    }, null, { okLabel: 'Sign in', kind: 'net-signin' });
}

// withAuth calls fn(req) and asks for credentials while the answer says needAuth; null when the user cancels.
async function withAuth(label, req, fn) {
  for (;;) {
    const r = await fn(req);
    if (!r.needAuth) return r;
    const c = await askCreds(label, { user: req.user });
    if (!c) return null;
    req = { ...req, ...c };
  }
}

function setOpening(list) {
  opening = new Set((list || []).map(netKey));
}

function repaintNet(app) {
  for (const p of app.panels) if (p.path && /^(network|smb|nfs|afp):\/\//i.test(p.path)) p.renderRows();
}

// openingEvent handles "network-opening": new spinners, and a panel back at the parent of a folder that just opened
// goes into it (one that failed says why, when the panel's own reading of it isn't waiting any more).
function openingEvent(app, ev) {
  if (!ev) return;
  setOpening(ev.opening);
  repaintNet(app);
  if (!ev.ended) return;
  const ended = netKey(ev.ended);
  for (const p of app.panels) {
    if (pending.get(p) !== ended) continue;
    pending.delete(p);
    if (p.loading || netKey(p.path) !== netParent(ended)) continue; // still reading it itself, or somewhere else
    if (ev.ok) p.go(ended);
    else if (ev.error) app.flash(ev.error, 'err');
  }
}

// ---- the banner over the network folder ----

function banner(p) {
  let el = p.el.querySelector('.mc-net-banner');
  if (!isNetworkRoot(p.path)) { el?.remove(); return; }
  if (!el) {
    el = document.createElement('div');
    el.className = 'mc-net-banner';
    el.setAttribute('role', 'status');
    el.innerHTML = '<span class="mc-net-spin" aria-hidden="true"></span><span class="mc-net-text"></span>';
    el.addEventListener('mousedown', (ev) => ev.stopPropagation());
    p.el.appendChild(el);
  }
  el.classList.toggle('searching', !!status.searching);
  el.classList.toggle('warn', !!status.note);
  el.querySelector('.mc-net-text').textContent = bannerText(status, (p.all || []).filter((e) => !e.up).length);
}

function openNetwork(app, panel = app.cur) {
  app.setActive(panel);
  return panel.go('network://');
}

// ---- Connect and Disconnect network drive ----

// mapOptions asks Windows users for the drive letter and reconnecting; elsewhere there is nothing to ask.
async function mapOptions(app, address) {
  if (app.platform !== 'windows') return {};
  const letters = await go.Network.FreeLetters().catch(() => []);
  if (!letters.length) { app.flash('No drive letter is free.', 'err'); return null; }
  return formDialog('Map network drive',
    `<p>${esc(address)}</p>
     <div class="mc-row"><label for="net-letter">Drive:</label><select id="net-letter">${letters.map((l) => `<option>${esc(l)}</option>`).join('')}</select></div>
     <label class="mc-check"><input type="checkbox" id="net-persist" checked> Reconnect at sign-in</label>`,
    (box) => ({ letter: box.querySelector('#net-letter').value, persistent: box.querySelector('#net-persist').checked }),
    null, { okLabel: 'Map', kind: 'net-map' });
}

// netConnect is TC's "Connect network drive": type a share and map it; a server alone opens in the panel, to pick a
// share there.
async function netConnect(app, panel = app.cur) {
  const v = await inputDialog({
    title: 'Connect network drive',
    label: 'Share (smb://server/share, \\\\server\\share, nfs://server/export, afp://server/share):',
    okLabel: 'Connect',
    kind: 'net-server',
    validate: (s) => typedAddress(s).error || '',
  });
  if (v == null) return false;
  const address = typedAddress(v).address;
  if (!hasShare(address)) { app.setActive(panel); return panel.go(typedPath(address)); }
  const opts = await mapOptions(app, address);
  if (!opts) return false;
  app.flash(`Connecting ${address}…`);
  let r;
  try {
    r = await withAuth(address, { address, ...opts }, (req) => go.Network.Map(req));
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
  if (!r) { app.flash(''); return false; }
  app.flash(`Connected: ${r.path}`);
  await app.loadVolumes();
  app.setActive(panel);
  return panel.go(r.path);
}

async function netDisconnect(app, panel = app.cur) {
  await app.loadVolumes();
  const vols = networkVolumes(app.volumes);
  if (!vols.length) { app.flash('No network drives are connected.'); return false; }
  const r = await showList({ title: 'Disconnect network drive', items: vols.map((v) => ({ label: v.name, detail: v.path, value: v })), anchor: panel.$.drives, kind: 'net-disconnect' });
  if (!r) return false;
  const v = r.item.value;
  try {
    await go.Network.Disconnect(v.path);
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
  // a panel showing the share goes home, so it doesn't sit on a folder that is gone
  const home = (app.volumes.find((x) => x.kind === 'home') || app.volumes[0])?.path;
  for (const p of app.panels) if (home && app.driveFor(p.path) === v) await p.go(home);
  await app.loadVolumes();
  app.flash(`Disconnected ${v.name}.`);
  return true;
}

command({ name: 'cm_OpenNetwork', label: 'Network neighbourhood', run: (app) => openNetwork(app) });
command({ name: 'cm_NetConnect', label: 'Connect network drive…', run: (app) => netConnect(app) });
command({ name: 'cm_NetDisconnect', label: 'Disconnect network drive…', run: (app) => netDisconnect(app) });

// a network folder being opened shows a spinner on its row instead of fading the list
Panel.quietLoadHooks.push((p, path) => {
  if (!isNetFolder(path)) return false;
  pending.set(p, netKey(path));
  return true;
});
Panel.rowClassHooks.push((p, e) => (!e.up && opening.size && p.path && opening.has(netKey(p.fullPath(e.name))) ? 'mc-net-opening' : ''));

export function init(app) {
  app.openNetwork = (panel) => openNetwork(app, panel); // the drive bar's Network button (core/panel.js)
  app.on('panel', (p) => {
    banner(p);
    if (pending.get(p) === netKey(p.path)) pending.delete(p); // it opened in this panel: nothing left to wait for
  });
  on('network-opening', (ev) => openingEvent(app, ev));
  // servers came or went: a panel showing the network folder reads it again (the cursor stays where it was)
  on('network-servers', (s) => {
    if (s) status = s;
    for (const p of app.panels) {
      if (!isNetworkRoot(p.path)) continue;
      if (!p.loading) p.reload();
      banner(p);
    }
  });
  // a server wants a user name and password: ask, then show the folder again
  let asking = false;
  on('network-auth', async (a) => {
    if (asking) return;
    asking = true;
    try {
      const c = await askCreds(a.server, { user: a.user });
      if (!c) return;
      await go.Network.SetCreds({ address: a.address, ...c });
      const p = app.cur;
      await p.go(a.path);
    } finally {
      asking = false;
    }
  });
  go.Network.Status().then((s) => { if (s) { status = s; setOpening(s.opening); repaintNet(app); } }).catch(() => {});
}
