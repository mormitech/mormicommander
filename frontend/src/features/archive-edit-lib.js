// Editing a file inside an archive (F4 on an archive entry, FEATURES.md F-592), the page's plain logic: the question
// asked when the editor saved the temporary copy, the note shown when F4 opened one, and which panels show the
// archive and so are read again after it was updated. Go watches the copy (feature_archive_edit.go) and sends
// "archive-edit-changed"; vfs.js asks with these and answers through ListerEdit.AnswerArchiveEdit.

const baseName = (p) => String(p || '').replace(/[\\/]+$/, '').split(/[\\/]/).pop() || String(p || '');

// The two answers of the question; the first is the default (Enter).
export const UPDATE = 'Update';
export const DONT_UPDATE = "Don't update";

// updateQuestion is the messageDialog options for a change c = {name, archive}.
export function updateQuestion(c) {
  return {
    Title: 'Edit inside archive',
    Message: `${c.name} was changed in the editor.\n\nUpdate it in ${baseName(c.archive)}?`,
    Buttons: [UPDATE, DONT_UPDATE],
    DefaultButton: UPDATE,
    CancelButton: DONT_UPDATE,
    Kind: 'archive-edit',
  };
}

// editNote is the flash message after F4 (r: Go's ListerEditResult; name: the entry's name), or '' for nothing.
export function editNote(r, name) {
  if (r?.note) return r.note;
  if (r?.watching) return `Editing a copy of ${name}. Each time you save it, you are asked whether to update the archive.`;
  return '';
}

// panelsShowing lists the panels whose folder is the archive or a folder inside it.
export function panelsShowing(panels, archive) {
  const a = String(archive || '').replace(/[\\/]+$/, '');
  if (!a) return [];
  return (panels || []).filter((p) => {
    const path = String(p?.path || '');
    return path === a || path.startsWith(a + '/') || path.startsWith(a + '\\');
  });
}
