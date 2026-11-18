// Keys of favourite tab sets (F-775, TC: a hotkey per entry of the Favorite tabs menu). The keys are kept in the
// setting "favTabKeys" as {combo: favourite name}; a key opens its favourite tabs from anywhere in the file manager.
// The functions here are pure (no app, no DOM) and return a new map.

const norm = (s) => String(s || '').trim().toLowerCase();

// favKeyFor is the combo that opens the favourite tabs `name`, or ''.
export function favKeyFor(map, name) {
  for (const [k, v] of Object.entries(map || {})) if (norm(v) === norm(name)) return k;
  return '';
}

// favForKey is the favourite name the combo opens, or ''.
export const favForKey = (map, combo) => (combo && map && Object.prototype.hasOwnProperty.call(map, combo) ? map[combo] : '');

// setFavKey gives the favourite `name` the combo (one key per favourite, one favourite per key); '' removes its key.
export function setFavKey(map, name, combo) {
  const out = {};
  for (const [k, v] of Object.entries(map || {})) if (norm(v) !== norm(name) && k !== combo) out[k] = v;
  if (combo) out[combo] = String(name).trim();
  return out;
}

// renameFavKey moves the key of a renamed favourite to its new name.
export function renameFavKey(map, from, to) {
  const k = favKeyFor(map, from);
  return k ? setFavKey(setFavKey(map, from, ''), to, k) : { ...(map || {}) };
}

// dropFavKey removes the key of a deleted favourite.
export const dropFavKey = (map, name) => setFavKey(map, name, '');
