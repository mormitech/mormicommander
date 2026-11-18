// "Info on packed file" (Alt+Enter on an entry inside an archive, FEATURES.md F-847) and the archive comment (F-622),
// the page's plain logic: the rows of the info window from Go's FS.PackedInfo answer, and which packers an archive
// can be converted to (F-628). vfs.js shows them.

const esc = (s) => String(s ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const baseName = (p) => String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || String(p || '');
const num = (n) => String(n).replace(/\B(?=(\d{3})+(?!\d))/g, ' ');

// packedInfoRows gives [label, text] pairs for an info answer; unknown values are left out. dateText formats Unix ms.
export function packedInfoRows(info, dateText = (ms) => new Date(ms).toLocaleString()) {
  const rows = [['Name', info.name || '']];
  rows.push(['Archive', baseName(info.archive)]);
  if (info.path && info.path.includes('/')) rows.push(['Folder', info.path.slice(0, info.path.lastIndexOf('/'))]);
  rows.push(['Type', `${String(info.format || '').toUpperCase()} archive`]);
  if (!info.isDir) {
    if (info.size >= 0) rows.push(['Size', `${num(info.size)} bytes`]);
    if (info.packed >= 0) rows.push(['Packed size', `${num(info.packed)} bytes`]);
    if (info.ratio >= 0) rows.push(['Ratio', `${info.ratio}%`]);
  }
  if (info.mtime) rows.push(['Date', dateText(info.mtime)]);
  if (info.crc) rows.push(['CRC-32', info.crc]);
  if (info.method) rows.push(['Method', info.method]);
  if (info.encrypted) rows.push(['Encrypted', info.encryption || 'Yes']);
  if (info.comment) rows.push(['Comment', info.comment]);
  if (info.archiveComment) rows.push([info.format === 'iso' ? 'Volume label' : 'Archive comment', info.archiveComment]);
  return rows.filter(([, v]) => v !== '');
}

// packedInfoHTML is the table of the info window.
export function packedInfoHTML(info, dateText) {
  const body = packedInfoRows(info, dateText).map(([k, v]) => `<tr><th>${esc(k)}</th><td>${esc(v).replace(/\n/g, '<br>')}</td></tr>`).join('');
  return `<table class="mc-vfs-info">${body}</table>`;
}

// convertTargets lists the packers an archive can be converted to: the available ones, without single-file packers
// unless asked (they hold one file only).
export function convertTargets(packers, { single = true } = {}) {
  return (packers || []).filter((p) => p.available && (single || !p.single));
}
