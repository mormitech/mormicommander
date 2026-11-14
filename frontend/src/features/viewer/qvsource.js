// Where quick view (Ctrl+Q) reads the file under the other panel's cursor from (issue #141). A local file is opened as
// it is. A file inside an archive has no path on the disk, so it is unpacked to a temporary copy first (like F3 does),
// but only up to ARCHIVE_PREVIEW_LIMIT bytes (spec area 4: "preview files in archives up to 10 MB"), so scrolling
// through a big archive does not unpack every large entry. Files on servers are not previewed yet (issue #101).

export const ARCHIVE_PREVIEW_LIMIT = 10 * 1024 * 1024;

const urlLike = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');

// quickViewSource answers "local", "archive", "too-big" or "remote" for an entry of a panel whose place is described by
// info (FS.PathInfo of the panel's folder: {kind, remote}) and whose folder is dir.
export function quickViewSource(info, dir, entry) {
  if (urlLike(dir) || info?.remote) return 'remote';
  if (info?.kind === 'archive') return (entry?.size || 0) > ARCHIVE_PREVIEW_LIMIT ? 'too-big' : 'archive';
  return 'local';
}
