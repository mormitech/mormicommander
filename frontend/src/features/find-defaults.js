// The fields a new Find files search (Alt+F7) starts with when there is no last search to fill them from (the first
// search, or after the last search was forgotten): Options > Find files sets them (board issue #94). Once a search has
// been made, the dialog opens with that search's fields, as it always did, so the options never undo what the user
// chose there. (The result limit, Stop after this many results, applies to every search; Go reads it: feature_find.go.)
// Kept apart with no imports so the tests can run it in Node.

// findDefaults builds the request the dialog is filled from. opt(name) is the option's value as text ("1"/"0" for
// yes/no); a missing value gives the program's own default, which is also the option's default.
export function findDefaults(opt) {
  const on = (name, def) => { const v = opt(name); return v === undefined || v === null || v === '' ? def : v === '1'; };
  const depth = parseInt(opt('FindFiles.Depth'), 10);
  return {
    searchFor: '*',
    depth: Number.isFinite(depth) ? Math.max(-1, Math.min(99, depth)) : -1,
    archives: on('FindFiles.Archives', false),
    caseSensitive: on('FindFiles.CaseSensitive', false),
    // "Regular expressions" is for the text search: the name field starts with "*", which is not a regular expression
    textRegex: on('FindFiles.RegEx', false),
    skipHidden: !on('FindFiles.Hidden', true),
    followLinks: on('FindFiles.FollowLinks', false),
  };
}
