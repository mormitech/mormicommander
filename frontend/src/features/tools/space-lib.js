// Whether Space on a folder also counts its size (TC's SpaceCalcSize, F-118 and F-797), kept free of the DOM and of
// Go so vitest can test it. The Options dialog stores the choice in settings.json as "countSpace"; a settings file
// that was copied from Total Commander (wincmd.ini saved as mormicommander.ini, or edited by hand) carries it as
// [Configuration] SpaceCalcSize, which the page sees as the option "Operation.SpaceCalcSize". features/tools/space.js
// uses it.

// countSpaceOn answers for the Space key. A choice stored in settings.json wins, as it always has; when there is none,
// the settings file's SpaceCalcSize counts, so a copied wincmd.ini with SpaceCalcSize=0 takes effect from the start.
// With neither, Space counts (the default, unchanged).
export function countSpaceOn(stored, iniValue) {
  if (stored !== undefined && stored !== null) return !!stored;
  if (iniValue === '0') return false;
  return true;
}
