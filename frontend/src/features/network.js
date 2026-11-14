// Network neighbourhood (board issue #148; TC's cm_OpenNetwork, cm_NetConnect, cm_NetDisconnect; F-365, F-383, F-386).
//
// The Network button at the end of the drive bar (or Net > Network neighbourhood…) searches the local network for file
// servers and lists them after the ones used before; "Type a server…" takes smb://, \\server, nfs:// or afp:// by hand.
// Picking a server lists its shares (asking for a user name and password when the server wants them, with "Save the
// password" for the system keychain); picking a share offers Open (shown in the panel; a mount Open needed goes away
// when the app ends) and Map (mounted to stay: a drive letter on Windows, /Volumes on macOS, GVfs on Linux).
// Net > Connect network drive… types a share directly; Net > Disconnect network drive… unmounts one.
//
// The network is searched only when the user asks (docs/privacy.md); the work is in Go (feature_network.go).

import { command } from '../core/registry.js';
import { showList } from '../core/popup.js';
import { formDialog, inputDialog, esc } from '../core/dialogs.js';
import { go, errText } from '../core/go.js';
import { serverItems, shareItems, networkVolumes, typedAddress, hasShare, shareAddress } from './network-lib.js';

const TYPE = { label: 'Type a server…', detail: 'smb://server, \\\\server, nfs://server or afp://server', value: { typed: true } };

// askCreds asks for the user name and password of a server; null when cancelled.
function askCreds(app, server, prev = {}) {
  const chk = (id, on, label) => `<label class="mc-check"><input type="checkbox" id="${id}"${on ? ' checked' : ''}> ${esc(label)}</label>`;
  return formDialog(`Sign in to ${server}`,
    `<p>${esc(server)} wants a user name and password${prev.tried ? ' (the last ones were refused)' : ''}.</p>
     <div class="mc-row"><label for="net-user">User name:</label><input id="net-user" type="text" spellcheck="false" autocomplete="off" value="${esc(prev.user || '')}" autofocus></div>
     <div class="mc-row"><label for="net-pass">Password:</label><input id="net-pass" type="password" autocomplete="off"></div>
     <div class="mc-row"><label for="net-domain">Domain:</label><input id="net-domain" type="text" spellcheck="false" placeholder="(usually empty)" value="${esc(prev.domain || '')}"></div>
     ${chk('net-save', !!prev.save, 'Save the password in the system keychain')}`,
    (box) => {
      const user = box.querySelector('#net-user').value.trim();
      if (!user) return 'Type a user name.';
      return { user, password: box.querySelector('#net-pass').value, domain: box.querySelector('#net-domain').value.trim(), save: box.querySelector('#net-save').checked };
    }, null, { okLabel: 'Sign in', kind: 'net-signin' });
}

// withAuth calls fn(req) and asks for credentials while the answer says needAuth; null when the user cancels.
async function withAuth(app, label, req, fn) {
  let tried = false;
  for (;;) {
    const r = await fn(req);
    if (!r.needAuth) return r;
    const c = await askCreds(app, label, { user: req.user, domain: req.domain, save: req.save, tried });
    if (!c) return null;
    req = { ...req, ...c };
    tried = true;
  }
}

// chooseServer shows the servers (searching the network first); returns the server or null.
async function chooseServer(app, panel) {
  app.flash('Searching the local network for servers…');
  let found;
  try {
    found = await go.Network.Discover();
  } catch (e) {
    app.flash(errText(e), 'err');
    return null;
  }
  const items = [TYPE, ...serverItems(found.servers || [])];
  if (found.note && items.length === 1) app.flash(found.note);
  else app.flash(items.length > 1 ? '' : 'No servers found on the local network; type one.');
  const r = await showList({ title: 'Network', items, anchor: panel.$.drives, index: items.length > 1 ? 1 : 0, kind: 'network', filter: true });
  if (!r) return null;
  if (r.item.value.typed) return typeServer(app);
  return r.item.value;
}

// typeServer asks for a server address; a typed share goes straight to the share's actions.
async function typeServer(app) {
  const v = await inputDialog({
    title: 'Network server',
    label: 'Server (smb://server, \\\\server, nfs://server, afp://server; a share may follow):',
    okLabel: 'Connect',
    kind: 'net-server',
    validate: (s) => typedAddress(s).error || '',
  });
  if (v == null) return null;
  const address = typedAddress(v).address;
  return { name: address, host: '', address, kind: /^nfs:/i.test(address) ? 'nfs' : /^afp:/i.test(address) ? 'afp' : 'smb', typed: true, direct: hasShare(address) };
}

// chooseShare lists a server's shares and returns the address of the one picked (null: cancelled).
async function chooseShare(app, panel, server) {
  if (server.direct) return server.address;
  if (server.kind === 'afp') return server.address; // AFP lists no shares: macOS asks in its own window
  app.flash(`Asking ${server.name} for its shares…`);
  let r;
  try {
    r = await withAuth(app, server.name, { address: server.address, user: server.user || '' }, (req) => go.Network.Shares(req).then((x) => { server.req = req; return x; }));
  } catch (e) {
    app.flash(errText(e), 'err');
    return null;
  }
  app.flash('');
  if (!r) return null;
  if (!r.shares?.length) { app.flash(`${server.name} shares no folders with you.`); return null; }
  const pick = await showList({ title: `Shares on ${server.name}`, items: shareItems(r.shares), anchor: panel.$.drives, kind: 'network-shares', filter: true });
  if (!pick) return null;
  return shareAddress(server, pick.item.value.name);
}

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

// connect opens (or maps) the share at address and shows it in panel.
async function connect(app, panel, address, how, base = {}) {
  let opts = {};
  if (how === 'map') {
    opts = await mapOptions(app, address);
    if (!opts) return false;
  }
  app.flash(how === 'map' ? `Mapping ${address}…` : `Connecting to ${address}…`);
  let r;
  try {
    r = await withAuth(app, address, { ...base, address, ...opts, password: base.password || '' },
      (req) => (how === 'map' ? go.Network.Map(req) : go.Network.Open(req)));
  } catch (e) {
    app.flash(errText(e), 'err');
    return false;
  }
  if (!r) { app.flash(''); return false; }
  app.flash(how === 'map' ? `Mapped: ${r.path}` : '');
  await app.loadVolumes();
  app.setActive(panel);
  return panel.go(r.path);
}

// shareActions offers Open and Map for a chosen share.
async function shareActions(app, panel, address, base) {
  const items = [
    { label: 'Open', detail: 'show it in this panel', value: 'open' },
    { label: app.platform === 'windows' ? 'Map network drive…' : 'Map (stay connected)', detail: app.platform === 'windows' ? 'to a drive letter' : 'it stays in the drive bar until you disconnect it', value: 'map' },
  ];
  const r = await showList({ title: address.replace(/^[a-z]+:\/\//, ''), items, anchor: panel.$.drives, kind: 'network-share' });
  if (!r) return false;
  return connect(app, panel, address, r.item.value, base);
}

async function openNetwork(app, panel = app.cur) {
  const server = await chooseServer(app, panel);
  if (!server) return false;
  const address = await chooseShare(app, panel, server);
  if (!address) return false;
  const base = server.req ? { user: server.req.user, password: server.req.password, domain: server.req.domain, save: server.req.save } : { user: server.user || '' };
  return shareActions(app, panel, address, base);
}

// netConnect is TC's "Connect network drive": type a share, then map it (or pick a share when only a server is typed).
async function netConnect(app, panel = app.cur) {
  const server = await typeServer(app);
  if (!server) return false;
  if (!server.direct) {
    const address = await chooseShare(app, panel, server);
    if (!address) return false;
    const base = server.req ? { user: server.req.user, password: server.req.password, domain: server.req.domain, save: server.req.save } : {};
    return connect(app, panel, address, 'map', base);
  }
  return connect(app, panel, server.address, 'map');
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

command({ name: 'cm_OpenNetwork', label: 'Network neighbourhood…', run: (app) => openNetwork(app) });
command({ name: 'cm_NetConnect', label: 'Connect network drive…', run: (app) => netConnect(app) });
command({ name: 'cm_NetDisconnect', label: 'Disconnect network drive…', run: (app) => netDisconnect(app) });

// The drive bar's Network button (core/panel.js draws it) opens the neighbourhood for its own panel.
export function init(app) {
  app.openNetwork = (panel) => openNetwork(app, panel);
}
