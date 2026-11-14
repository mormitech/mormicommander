// Pure helpers of the network neighbourhood (features/network.js, board issue #148).

const KIND = { smb: 'SMB', afp: 'AFP', nfs: 'NFS' };
const SOURCE = { recent: 'used before', bonjour: 'Bonjour', avahi: 'found', netbios: 'found', typed: 'typed' };

// serverItems are the popup lines of the servers: the name, then protocol, address and where it came from.
export function serverItems(servers) {
  return servers.map((s) => ({
    label: s.name || s.host,
    detail: [KIND[s.kind] || s.kind, s.address.replace(/^[a-z]+:\/\//, ''), s.user ? `as ${s.user}${s.saved ? ' (password saved)' : ''}` : '', SOURCE[s.source] || ''].filter(Boolean).join(' · '),
    value: s,
  }));
}

// shareItems are the popup lines of a server's shares (an NFS export shows who may mount it).
export const shareItems = (shares) => shares.map((s) => ({ label: s.name, detail: s.comment || '', value: s }));

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

// shareAddress is the address of one share of a server: the server's address with the share as its path.
export function shareAddress(server, share) {
  const base = String(server.address || '').replace(/\/+$/, '');
  const name = String(share || '').replace(/^\/+/, '');
  return `${base}/${name.split('/').map(encodeURIComponent).join('/')}`;
}
