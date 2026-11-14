// Where quick view (Ctrl+Q) and F3 read the file under the cursor from. A local file is opened as it is. A file inside
// an archive has no path on the disk, so it is unpacked to a temporary copy first (issue #141), but quick view does
// that only up to ARCHIVE_PREVIEW_LIMIT bytes (spec area 4: "preview files in archives up to 10 MB"), so scrolling
// through a big archive does not unpack every large entry. A file on a server (FTP, SFTP, WebDAV) is downloaded to a
// temporary copy the same way (issue #101); quick view does that only up to SERVER_PREVIEW_LIMIT bytes, smaller than
// the archive limit because every cursor move over a large server file would otherwise start a long download. Search
// results fed to a panel (find://) are left as they were.

export const ARCHIVE_PREVIEW_LIMIT = 10 * 1024 * 1024;
export const SERVER_PREVIEW_LIMIT = 2 * 1024 * 1024;

const urlLike = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');
const feedLike = (p) => /^find:\/\//i.test(String(p || '').trim());

// quickViewSource answers "local", "archive", "too-big", "server", "server-too-big" or "remote" (a place quick view
// does not preview: search results fed to a panel) for an entry of a panel whose place is described by info
// (FS.PathInfo of the panel's folder, or the panel's own copy of it: {kind, remote}) and whose folder is dir.
export function quickViewSource(info, dir, entry) {
  if (info?.kind === 'results' || feedLike(dir)) return 'remote';
  if (urlLike(dir) || info?.remote) return (entry?.size || 0) > SERVER_PREVIEW_LIMIT ? 'server-too-big' : 'server';
  if (info?.kind === 'archive') return (entry?.size || 0) > ARCHIVE_PREVIEW_LIMIT ? 'too-big' : 'archive';
  return 'local';
}

// viewsFromCopy says whether F3 on a file of a panel (info as above, dir its folder) goes through FS.LocalCopy rather
// than opening the panel path itself: inside an archive, on a server and in fed search results (whose names stand for
// real files, which LocalCopy hands back as they are), yes, as the F3 key in vfs.js already does; a local folder, no.
export function viewsFromCopy(info, dir) {
  if (urlLike(dir) || info?.remote) return true;
  return !!info && info.kind !== 'local';
}

// downloads says whether getting that copy means a download from a server (so the page says "Downloading…").
export function downloads(info, dir) {
  if (info?.kind === 'results' || feedLike(dir)) return false;
  return urlLike(dir) || !!info?.remote;
}
