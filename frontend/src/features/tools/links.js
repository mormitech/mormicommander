// Links (internal/links): Create symbolic link / Create hard link (cm_CreateSymbolicLink, cm_CreateHardLink, F-179,
// F-337) open TC's dialog with the new link's name (in the other panel) and what it points to; with several items
// selected they make one link per item in a folder. Ctrl+Shift+F5 (cm_CreateShortcut, F-178) makes links of the
// selection in the other panel and lets the user pick the kind: symbolic link, hard link, junction (Windows) or a
// shortcut file (.url / .webloc / .desktop).

import { command } from '../../core/registry.js';
import { go } from '../../core/go.js';
import { formDialog, messageDialog } from '../../core/dialogs.js';
import { plural } from '../../core/format.js';
import { esc, runTool, askOverwrite, needLocal, targets, showError, failureList, reloadAll, deselect, pref, setPref, baseName, joinPath, otherDir } from './common.js';
import { onServer, remoteSymlink } from '../net-attrs.js';
import { tm, tn } from '../../core/i18n.js';

let kindsCache = null;
const linkKinds = async () => (kindsCache ||= await go.Tools.LinkKinds().catch(() => [{ kind: 'symlink', label: 'Symbolic link', ext: '' }]));

// makeLinks creates one link per source in dir and asks once before replacing names that are already there.
async function makeLinks(app, p, entries, sources, dir, kind, relative) {
  let res;
  try { res = await runTool('Creating links', (id) => go.Tools.CreateLinks(id, { kind, sources, destDir: dir, relative, overwrite: false })); } catch (e) { await showError(e.message); return; }
  const there = res.filter((r) => r.exists);
  if (there.length) {
    const ok = there.length === 1 ? await askOverwrite(there[0].link)
      : (await messageDialog({ Title: 'Overwrite?', Message: tm('{what} already in {dir}:\n{names}\n\nReplace them?', { what: tn(there.length, 'name'), dir, names: there.slice(0, 8).map((r) => baseName(r.link)).join('\n') + (there.length > 8 ? '\n…' : '') }), Buttons: ['Overwrite', 'Skip them', 'Cancel'], Kind: 'tools-overwrite' })) === 'Overwrite';
    if (ok) {
      try {
        const again = await runTool('Creating links', (id) => go.Tools.CreateLinks(id, { kind, sources: there.map((r) => r.source), destDir: dir, relative, overwrite: true }));
        const by = new Map(again.map((r) => [r.source, r]));
        res = res.map((r) => by.get(r.source) || r);
      } catch (e) { await showError(e.message); return; }
    }
  }
  await reloadAll(app, res.length === 1 && !res[0].error && !res[0].exists ? res[0].link : '');
  const made = res.filter((r) => !r.error && !r.exists);
  const failed = res.filter((r) => r.error).map((r) => ({ path: r.source, error: r.error }));
  const okSources = new Set(made.map((r) => r.source));
  deselect(p, entries.filter((e) => okSources.has(p.fullPath(e.name))).map((e) => e.name));
  if (failed.length) await failureList('Create links', failed, `${plural(made.length, 'link')} made, ${failed.length} not:`);
  else app.flash(made.length === 1 ? `Made ${baseName(made[0].link)}.` : `Made ${plural(made.length, 'link')} in ${dir}.`);
}

// linkDialog: TC's "Create symbolic link" / "Create hard link" dialog for one item.
async function linkDialog(app, kind) {
  // on an SFTP panel: a symbolic link on the server (features/net-attrs.js, F-633)
  if (kind === 'symlink' && !app.demo && onServer(app)) return remoteSymlink(app);
  if (!needLocal(app)) return;
  const { panel: p, entries, paths } = targets(app);
  if (!paths.length) { app.flash('Nothing is selected.'); return; }
  const kinds = await linkKinds();
  const info = kinds.find((k) => k.kind === kind);
  if (!info) { await showError('This kind of link can\'t be made on this system.'); return; }
  const title = kind === 'hardlink' ? 'Create hard link' : 'Create symbolic link';
  const relRow = kind === 'symlink' ? `<label class="mc-check"><input type="checkbox" id="relative"${pref(app, 'linkRelative', false) ? ' checked' : ''}> Store the path relative to the link</label>` : '';
  if (paths.length > 1) {
    const ans = await formDialog(title, `<div class="mc-row"><label for="folder">${esc(tm('Make {what} in the folder', { what: kind === 'hardlink' ? tn(paths.length, 'hard link') : tn(paths.length, 'symbolic link') }))}</label><input type="text" id="folder" spellcheck="false" value="${esc(otherDir(app))}" autofocus></div>${relRow}`,
      (box) => { const v = box.querySelector('#folder').value.trim(); return v ? { folder: v, relative: !!box.querySelector('#relative')?.checked } : 'Type the folder for the links.'; }, null, { kind: 'create-link', okLabel: 'Create' });
    if (!ans) return;
    if (kind === 'symlink') setPref(app, 'linkRelative', ans.relative);
    await makeLinks(app, p, entries, paths, ans.folder, kind, ans.relative);
    return;
  }
  const html = `<div class="mc-row"><label for="link">New link</label><input type="text" id="link" spellcheck="false" value="${esc(joinPath(app, otherDir(app), entries[0].name))}" autofocus></div>
    <div class="mc-row"><label for="target">Points to${kind === 'hardlink' ? ' (a file on the same drive)' : ''}</label><input type="text" id="target" spellcheck="false" value="${esc(paths[0])}"></div>${relRow}`;
  const values = await formDialog(title, html, (box) => {
    const link = box.querySelector('#link').value.trim(), target = box.querySelector('#target').value.trim();
    if (!link) return 'Type the name of the new link.';
    if (!target) return 'Type what the link points to.';
    return { link: app.resolvePath(link, p.path), target: app.resolvePath(target, p.path), relative: !!box.querySelector('#relative')?.checked };
  }, null, { kind: 'create-link', okLabel: 'Create' });
  if (!values) return;
  if (kind === 'symlink') setPref(app, 'linkRelative', values.relative);
  let r;
  try {
    r = await go.Tools.CreateLink({ kind, ...values, overwrite: false });
    if (r.exists) {
      if (!(await askOverwrite(values.link))) return;
      r = await go.Tools.CreateLink({ kind, ...values, overwrite: true });
    }
  } catch (e) { await showError(String(e?.message ?? e), title); return; }
  if (r.error) { await showError(r.error, title); return; }
  await reloadAll(app, r.link);
  deselect(p, [entries[0].name]);
  app.flash(tm('Made {name}.', { name: baseName(r.link) }));
}

command({ name: 'cm_CreateSymbolicLink', label: 'Create symbolic link…', enabled: (app) => !app.demo, run: (app) => linkDialog(app, 'symlink') });
command({ name: 'cm_CreateHardLink', label: 'Create hard link…', enabled: (app) => !app.demo, run: (app) => linkDialog(app, 'hardlink') });

command({
  name: 'cm_CreateShortcut', label: 'Create links…', short: 'Link',
  enabled: (app) => !app.demo,
  run: async (app) => {
    if (!needLocal(app)) return;
    const { panel: p, entries, paths } = targets(app);
    if (!paths.length) { app.flash('Nothing is selected.'); return; }
    const kinds = await linkKinds();
    const cur = pref(app, 'linkKind', 'symlink');
    const what = paths.length === 1 ? entries[0].name : plural(paths.length, 'item');
    const html = `<div class="mc-row"><label for="folder">Make links to ${esc(what)} in the folder</label><input type="text" id="folder" spellcheck="false" value="${esc(otherDir(app))}" autofocus></div>
      <div class="mc-row"><label for="kind">Kind</label><select id="kind">${kinds.map((k) => `<option value="${esc(k.kind)}"${k.kind === cur ? ' selected' : ''}>${esc(k.label)}</option>`).join('')}</select></div>
      <label class="mc-check"><input type="checkbox" id="relative"${pref(app, 'linkRelative', false) ? ' checked' : ''}> Store the path relative to the link (symbolic links)</label>`;
    const ans = await formDialog('Create links', html, (box) => {
      const folder = box.querySelector('#folder').value.trim();
      if (!folder) return 'Type the folder for the links.';
      return { folder: app.resolvePath(folder, p.path), kind: box.querySelector('#kind').value, relative: box.querySelector('#relative').checked };
    }, (box) => {
      const sync = () => { box.querySelector('#relative').disabled = box.querySelector('#kind').value !== 'symlink'; };
      box.querySelector('#kind').addEventListener('change', sync);
      sync();
    }, { kind: 'create-links', okLabel: 'Create' });
    if (!ans) return;
    setPref(app, 'linkKind', ans.kind);
    if (ans.kind === 'symlink') setPref(app, 'linkRelative', ans.relative);
    await makeLinks(app, p, entries, paths, ans.folder, ans.kind, ans.kind === 'symlink' && ans.relative);
  },
});
