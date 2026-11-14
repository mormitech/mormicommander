// pack_files, the AI layer's one tool that writes a file (issue #146, step 3): it packs copies of files into a NEW
// archive, the way the Pack dialog (Alt+F5, vfs.js) does, through the same job (core/jobs.js runJob, kind "pack",
// which Go's VfsJobs runs). It is safe under the owner's rule because it never touches what is there: the sources
// stay as they are ("move to archive" is never set), and an archive that already exists is refused, since packing to
// an existing name adds to that archive or replaces files in it. The archive goes into a local folder only (the other
// panel's folder by default, or the panel's own), so the existence check is a real one; no password, no parts, one
// archive for all the files.
//
// The check and the packing are a moment apart: a file of that name made in between by another program would be
// added to. That is the same window any "does it exist?" check has; the job itself is the Pack dialog's.

import { go } from '../core/go.js';
import { addAITool } from '../core/aitools.js';
import { app as theApp } from '../core/app.js';
import { runJob } from '../core/jobs.js';
import { F } from '../core/format.js';
import { side, nameList } from './aitools-lib.js';
import { archiveName } from './aitools-more-lib.js';

const isUrl = (p) => /^[a-z][a-z0-9+.-]*:\/\//i.test(p || '');

// exists answers whether something has that path (Stat answers or fails; a failure is "not there").
const exists = (path) => Promise.resolve(go.FS.Stat(path)).then((st) => !!st, () => false);

async function localFolder(path) {
  if (!path || isUrl(path)) return false;
  try { const st = await go.FS.Stat(path); return !!st && !!(st.flags & F.DIR); } catch { return false; }
}

const joinPath = (dir, name, sep) => (dir.endsWith(sep) || dir.endsWith('/') ? dir + name : dir + sep + name);

addAITool({
  name: 'pack_files',
  description: 'Packs copies of files and folders of a panel into a NEW archive (zip, 7z, tar, tar.gz, tar.bz2, tar.xz, tar.zst; gz and the like take one file '
    + 'only), as the Pack dialog does. The files stay where they are. By default the selected entries of the panel are packed (names picks entries by name '
    + 'instead), and the archive is made in the other panel\'s folder (where: "other", default) or in the panel\'s own folder (where: "same"). The archive '
    + 'name\'s extension picks the format, or format names it. If a file or folder with that name already exists the tool refuses: it never overwrites or '
    + 'adds to an archive. Only local folders can receive the archive. Answers the archive made, or why not.',
  schema: {
    type: 'object',
    properties: {
      side: { type: 'string', enum: ['left', 'right', 'active', 'other'], description: 'The panel whose files are packed (default: the active one).' },
      names: { type: 'array', items: { type: 'string' }, description: 'Names of entries in that panel\'s folder. Default: the selected entries.' },
      archive: { type: 'string', description: 'The new archive\'s file name (no folder), like "photos-2026.zip".' },
      format: { type: 'string', description: 'zip, 7z, tar, tar.gz, tar.bz2, tar.xz or tar.zst; default: from the name\'s extension, else zip.' },
      where: { type: 'string', enum: ['other', 'same'], description: '"other" (default): the other panel\'s folder; "same": the panel\'s own folder.' },
    },
    required: ['archive'],
  },
  run: async (args) => {
    const p = theApp.panel(side(args.side));
    if (!p?.path) throw new Error('That panel shows no folder yet.');
    const names = nameList(args.names);
    let items;
    if (names.length) {
      const missing = names.filter((n) => !p.entries.some((e) => e.name === n && !e.up));
      if (missing.length) throw new Error(`Not in ${p.path}: ${missing.slice(0, 10).join(', ')}.`);
      items = p.entries.filter((e) => !e.up && names.includes(e.name));
    } else {
      items = p.selectedEntries().filter((e) => !e.up);
      if (!items.length) throw new Error('Nothing is selected in that panel: select the files first (select_files) or give their names.');
    }
    const packers = (await go.VfsJobs.Packers()) || [];
    const { name, packer } = archiveName(args.archive, args.format, packers);
    if (packer.single && (items.length > 1 || items.some((e) => e.dir))) throw new Error(`${packer.label || packer.name} packs one file only; use zip or tar.gz for several files or folders.`);

    const where = String(args.where ?? 'other').trim().toLowerCase() === 'same' ? p : (p === theApp.left ? theApp.right : theApp.left);
    const dir = where.path;
    if (!(await localFolder(dir))) throw new Error(`The archive can only be made in a local folder; ${dir || 'that panel'} is not one (an archive, a server or a search result list).`);
    const target = joinPath(dir, name, theApp.sepFor ? theApp.sepFor(dir) : theApp.sep);
    if (await exists(target)) throw new Error(`${name} already exists in ${dir}. pack_files never overwrites or adds to an existing archive: choose another name.`);

    const req = {
      kind: 'pack', sources: items.map((e) => p.fullPath(e.name)), target, format: packer.name, level: 0, noPaths: false, noRecurse: false,
      move: false, separate: false, password: '', encryption: 'aes256', volumeSize: 0,
    };
    p.saveSelection?.();
    const final = await runJob(req, { title: 'Packing' });
    if (final?.state === 'done') {
      if (where.path === dir) where.focusName?.(name);
      return `Packed ${items.length} ${items.length === 1 ? 'entry' : 'entries'} into ${target}. The files themselves were not changed.`;
    }
    if (final?.state === 'cancelled') throw new Error('The packing was stopped by the user.');
    const errs = (final?.errors || []).slice(0, 5).join(' ');
    throw new Error(`The packing failed${errs ? `: ${errs}` : '.'}`);
  },
});
