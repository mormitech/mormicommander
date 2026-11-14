// Archives that can't be changed (board #143), kept apart from features/fileops.js so the rule can be tested without a
// page. A 7z is only read here, and an archive entered from inside another archive (or opened from a server) is a
// temporary copy, so F5/F6 into them, F7 and F8 in them can't work. Go's FS.PathInfo marks such a folder readOnly and
// gives the reason as readOnlyWhy; fileops shows that sentence before its dialogs and questions (TC: "This archive
// type cannot be modified") instead of asking first and failing with a vague status line afterwards.

// archiveRefusal returns the sentence to show when the folder described by info (FS.PathInfo's answer, or null) is
// inside an archive that can't be changed, and "" when it can be changed or is not in an archive at all. Read-only
// server folders are left to the server, which says why itself.
export function archiveRefusal(info) {
  if (!info || info.kind !== 'archive' || !info.readOnly) return '';
  return info.readOnlyWhy || "This archive can't be changed.";
}
