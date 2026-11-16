// mergeMaps puts the folders you mapped to the drive bar (settings "driveMaps") after the real drives. A mapped
// network share ("network://DiskStation/z", "smb://nas.local/z") is mounted by the system when it is opened, and the
// mounted volume would then show a second button for the same share. So such a volume is marked hidden (the drive bar
// and the drive list skip it; disconnecting, ejecting and the trash warnings still know it) while a mapped button stands
// for its share, and the button keeps the volume's folder (mount) so it lights up while a panel is inside it.

// shareOfPath reads the server and share of a network panel path, or null for any other path.
export function shareOfPath(p) {
  const m = /^(network|smb|afp|nfs):\/\/([^/]+)\/([^/]+)\/?$/i.exec(String(p || ''));
  if (!m) return null;
  let host = m[2].replace(/^[^@]*@/, ''), share = m[3];
  try { host = decodeURIComponent(host); share = decodeURIComponent(share); } catch { /* as typed */ }
  return { host: host.replace(/:\d+$/, ''), share };
}

// sameServer: "DiskStation", "diskstation.local" and "DiskStation.local" are one server.
export function sameServer(a, b) {
  const short = (h) => String(h || '').toLowerCase().replace(/\.$/, '').replace(/\.local$/, '');
  return !!a && !!b && short(a) === short(b);
}

export function mergeMaps(vols, maps) {
  const out = [], buttons = maps.map((m) => ({ name: m.name || m.path, path: m.path, kind: 'map' }));
  for (const v of vols) {
    const b = v.kind === 'network' && v.share
      ? buttons.find((x) => { const s = shareOfPath(x.path); return s && s.share.toLowerCase() === v.share.toLowerCase() && sameServer(s.host, v.host); })
      : null;
    if (b) b.mount = v.path;
    out.push(b ? { ...v, hidden: true } : v);
  }
  return [...out, ...buttons];
}
