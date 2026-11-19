// The trash folder view (F-501, ours as commands): cm_GoToTrash opens the trash folder of the home volume in the
// panel (~/.Trash on macOS, ~/.local/share/Trash/files on Linux, the user's folder in the Recycle Bin on Windows), and
// cm_RestoreFromTrash puts the selected items of a trash folder back where they came from. Go works out each item's
// old place from the trash's own record (.trashinfo, $I), or on macOS from the undo journal for items this app
// trashed; it never puts anything back over an existing file and only takes items out of a trash folder
// (feature_trash_restore.go, internal/trash). Nothing is deleted here, and neither command is in the AI allow-list.

import { command } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { messageDialog } from '../core/dialogs.js';
import { plural } from '../core/format.js';
import { restoreReport } from './trash-lib.js';
import { tm } from '../core/i18n.js';

const trashWord = (app) => (app.platform === 'windows' ? 'Recycle Bin' : app.platform === 'darwin' ? 'Trash' : 'trash');

async function goToTrash(app) {
  const dir = await go.Jobs.TrashFolder().catch(() => '');
  if (!dir) { app.flash(tm('This system has no {trash} mormiCommander can open.', { trash: tm(trashWord(app)) }), 'err'); return false; }
  return app.cur.go(dir);
}

async function restoreFromTrash(app) {
  const src = app.cur;
  const T = trashWord(app);
  const items = src.targets();
  if (!items.length) { app.flash('No files selected.'); return false; }
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(src.path || '')) { app.flash(tm('Open the {trash} folder first.', { trash: tm(T) }), 'err'); return false; }
  const paths = items.map((e) => src.fullPath(e.name));
  const found = (await go.Jobs.TrashOriginals(paths).catch(() => [])) || [];
  const known = found.filter((r) => r.original);
  if (!known.length) {
    const mac = app.platform === 'darwin' ? ' For items Finder put in the Trash, use Put Back in Finder.' : '';
    await messageDialog({ Title: 'Put back', Message: `It is not known where ${items.length === 1 ? 'this item' : 'these items'} came from: only items in the ${T} can be put back.${mac}`, Kind: 'trash-restore' });
    return false;
  }
  const shown = known.slice(0, 5).map((r) => `  ${r.original}`).join('\n') + (known.length > 5 ? `\n  … and ${known.length - 5} more` : '');
  const left = found.length - known.length;
  const b = await messageDialog({
    Title: 'Put back',
    Message: `Put ${plural(known.length, 'item')} back where ${known.length === 1 ? 'it' : 'they'} came from?\n\n${shown}` +
      `${left ? `\n\n${plural(left, 'item')} with no known old place stay${left === 1 ? 's' : ''} in the ${T}.` : ''}\n\nNothing is replaced: an item whose old place is taken stays in the ${T}.`,
    Buttons: ['Put back', 'Cancel'], DefaultButton: 'Put back', Kind: 'trash-restore',
  });
  if (b !== 'Put back') return false;
  let res;
  try { res = await go.Jobs.RestoreFromTrash(known.map((r) => r.path)); } catch (e) { app.flash(errText(e), 'err'); return false; }
  await Promise.all(app.panels.map((p) => p.reload()));
  const r = restoreReport(res, T);
  app.flash(r.text, r.err ? 'err' : undefined);
  return !r.err;
}

// (registered before undo.js, whose entries then go in right after Wipe, ahead of these two)
command({ name: 'cm_GoToTrash', label: 'Go to the trash folder', menu: 'Files', menuAfter: 'cm_Wipe', run: (app) => goToTrash(app) });
command({ name: 'cm_RestoreFromTrash', label: 'Put back from the trash…', menu: 'Files', menuAfter: 'cm_GoToTrash', run: (app) => restoreFromTrash(app) });
