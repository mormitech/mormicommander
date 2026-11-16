// Pure helpers of the network neighbourhood (features/network.js, board issue #148).

// networkVolumes are the volumes that can be disconnected: network shares, not stale-free local disks.
export const networkVolumes = (volumes) => (volumes || []).filter((v) => v.kind === 'network');

// typedAddress tidies a typed server: blanks trimmed; a bare name stays (Go reads it as SMB).
export function typedAddress(s) {
  const t = String(s || '').trim();
  if (!t) return { error: 'Type a server, for example smb://nas or \\\\nas.' };
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(t) && !/^(smb|cifs|afp|nfs):\/\//i.test(t)) return { error: 'Only smb://, afp:// and nfs:// servers can be browsed here.' };
  return { address: t };
}

// hasShare says whether a typed address already names a share (smb://nas/media, \\nas\media, nfs://nas/export).
export function hasShare(address) {
  const t = String(address || '').trim();
  if (/^(\\\\|\/\/)/.test(t)) return t.replace(/^[\\/]+/, '').split(/[\\/]/).filter(Boolean).length > 1;
  const m = t.match(/^[a-z]+:\/\/[^/]+(\/.*)?$/i);
  if (m) return !!(m[1] && m[1].replace(/\//g, ''));
  return t.split('/').filter(Boolean).length > 1;
}

// isNetworkRoot says whether a panel shows the network folder itself (the list of servers).
export const isNetworkRoot = (p) => p === 'network://';

// typedPath turns a typed server into the panel path that lists its shares: \\nas or //nas → smb://nas, nas → smb://nas.
export function typedPath(address) {
  const t = String(address || '').trim();
  if (/^(\\\\|\/\/)/.test(t)) return `smb://${t.replace(/^[\\/]+/, '').replace(/\\/g, '/')}`;
  if (/^[a-z]+:\/\//i.test(t)) return t;
  return `smb://${t}`;
}

// bannerText is the line over the network folder: still searching, how many servers, or why it can't search.
export function bannerText(status, shown) {
  if (status.note) return status.note;
  const typeHint = 'For a server that isn\'t listed, type smb://server in the path bar.';
  if (status.searching) return shown ? `Searching the local network… ${shown} found so far.` : 'Searching the local network…';
  return shown ? `Still listening for more servers. ${typeHint}` : `No servers found yet; still listening. ${typeHint}`;
}

// netKey is a network folder's path as Go names it in "network-opening": without a trailing "/" (but "network://"
// stays as it is), so the panel's path and a row's full path compare equal.
export function netKey(p) {
  const s = String(p || '');
  if (s === 'network://' || s === 'network:') return 'network://';
  return s.replace(/\/+$/, '');
}

// isNetFolder says whether p is a network folder that has to be opened (a server's shares or a share): network://Server,
// network://Server/share, smb://host, nfs://host/export… — not the list of servers itself.
export function isNetFolder(p) {
  const k = netKey(p);
  return /^(network|smb|nfs|afp):\/\/./i.test(k) && k !== 'network://';
}

// netParent is the folder a network folder is listed in: network://Server/share → network://Server, network://Server
// → network://, smb://host/share → smb://host, smb://host → network:// (as Go's Parent).
export function netParent(p) {
  const k = netKey(p);
  const m = k.match(/^([a-z]+:\/\/)(.*)$/i);
  if (!m) return '';
  const rest = m[2];
  const i = rest.lastIndexOf('/');
  return i < 0 ? 'network://' : m[1] + rest.slice(0, i);
}
