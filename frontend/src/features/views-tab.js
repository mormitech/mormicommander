// The view of a folder tab (issue #128): each tab keeps its own view mode (Brief, Full, Comments, Thumbnails, a custom
// column set), like Total Commander, instead of the view belonging to the panel. The panel's view state is the object
// p.vs of features/views.js; a tab that is not active keeps its own p.vs in its snapshot. These pure helpers make a
// tab's own copy of that object (so two tabs never share and change one object), turn it into the fields saved with a
// tab ("view", "set" in [lefttabs]/[righttabs], .tab files and favourite tab sets) and back. They have no DOM and no
// app, so the tests run them in Node.

// The views a tab can be saved in; the tree in the panel (Ctrl+F8) is not kept, as for the panel itself.
export const TAB_VIEWS = ['brief', 'full', 'comments', 'custom', 'thumbs'];

// freshView is the view state of a panel that has not chosen a view yet.
export function freshView(mode = 'full', set = '') {
  return { mode, set, manual: { mode, set }, auto: false, cells: null, cellsSig: '', thumbs: null, tree: null, prev: null };
}

// copyView makes a tab's own copy of a view state: the same view and column set (a panel in the tree gives the view
// under the tree), without the caches (cells, thumbnails), which belong to the folder the original showed.
export function copyView(vs) {
  if (!vs) return freshView();
  const shown = vs.mode === 'tree' ? (vs.prev || vs.manual || { mode: 'full', set: '' }) : vs;
  const manual = vs.manual ? { mode: vs.manual.mode, set: vs.manual.set || '' } : { mode: shown.mode, set: shown.set || '' };
  const c = freshView(shown.mode || 'full', shown.mode === 'custom' ? shown.set || '' : '');
  c.manual = manual;
  c.auto = vs.mode === 'tree' ? false : !!vs.auto;
  return c;
}

// viewToSaved is what a tab saves of its view: the view the user chose (not one a folder rule switched to).
export function viewToSaved(vs) {
  if (!vs) return {};
  const m = vs.manual || vs;
  if (!TAB_VIEWS.includes(m.mode)) return {};
  return { view: m.mode, set: m.mode === 'custom' ? m.set || '' : '' };
}

// viewFromSaved is the view state for a tab read back from its saved fields, or null when they name none (tabs saved
// by older versions keep the panel's view) or one that can't be shown any more (an unknown view, a column set that was
// deleted). hasSet tells whether a custom column set exists.
export function viewFromSaved(saved, hasSet = () => true) {
  const mode = saved?.view;
  if (!TAB_VIEWS.includes(mode)) return null;
  const set = mode === 'custom' ? String(saved.set || '') : '';
  if (mode === 'custom' && !hasSet(set)) return null;
  return freshView(mode, set);
}
