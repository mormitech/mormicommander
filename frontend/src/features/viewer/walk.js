// N and P in the Lister: which file comes next. Both Lister forms (the window of its own, src/viewer-window.js, and the
// dialog over the panels, features/viewer.js) use this, so they walk the same way.
//
// A Lister is opened with one of two lists. In a local folder it gets "names" (files of one folder, "dir"), and the
// next file is simply that folder's next name. For files that are viewed from a temporary copy (inside an archive, a
// Find files result, a list fed to the panel) it gets "sources" instead: the full paths of the files to walk, each
// possibly inside an archive or anywhere on the disk, with "at" the place of the file shown first. The next file is then
// unpacked through FS.LocalCopy ("path" is the file shown first, already a local copy) (a local file comes back as it is) when it is reached. Copies made once are kept in
// the walker's cache, so going back and forth does not unpack the same file again.

const fileOf = (path) => path.slice(Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\')) + 1);

// walkList is the list the control mode shows ("list"): the names N and P go through.
export function walkList(req) {
  return req?.sources?.length ? req.sources.map(fileOf) : req?.names || [];
}

// makeWalker gives a walker for one Lister request. deps: resolve(name, dir) builds a path in a folder, and
// localCopy(path) gives a local file for any path (FS.LocalCopy). Its step(curPath, d) gives {path, name} for the next
// (d = 1) or previous (d = -1) file, or null at either end of the list; it throws when the file can't be unpacked.
// The caller calls done() of the answer once the file is shown, so a file that could not be opened is not counted.
export function makeWalker(req, { resolve, localCopy }) {
  const sources = req?.sources || [];
  const names = req?.names || [];
  let at = Number.isInteger(req?.at) ? req.at : sources.indexOf(req?.path);
  const copies = new Map();
  if (at >= 0 && at < sources.length && req.path) copies.set(sources[at], req.path); // (the file shown first)
  return {
    async step(curPath, d) {
      if (at >= 0 && at < sources.length) {
        const j = at + d;
        if (j < 0 || j >= sources.length) return null;
        const src = sources[j];
        let path = copies.get(src);
        if (!path) {
          path = (await localCopy(src)) || src;
          copies.set(src, path);
        }
        return { path, name: fileOf(src), done: () => { at = j; } };
      }
      const i = names.indexOf(fileOf(curPath));
      const j = i + d;
      if (i < 0 || j < 0 || j >= names.length) return null;
      return { path: resolve(names[j], req.dir), name: names[j], done: () => {} };
    },
  };
}
