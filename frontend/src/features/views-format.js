// How the column views of features/views.js (Full with a format, Comments, custom column sets) and Brief follow the
// Configuration > Display options (issue #93). The views feature has its own size / date / attribute formats (View >
// settings); when one of those is left at its default, the Display options (core/format.js "display") decide instead,
// so a size unit, thousands separator, date format, 12/24-hour clock and seconds chosen there show in every view, not
// only in the core's Full view. With the Display options at their defaults the result is exactly what the views sent
// before, so nothing changes for anyone who has not set them. These helpers are pure (no DOM, no app) so the tests run
// them in Node.

// The internal/listing size unit names for Display's sizeStyle 0..4 (bytes, kB, MB, GB, dynamic).
const SIZE_UNITS = ['bytes', 'k', 'M', 'G', 'dynamic'];

// The Go date picture the views sent when no date format was set (lst.DefaultDatePicture).
const DEFAULT_PICTURE = 'yyyy-MM-dd HH:mm';

// datePicture turns the Display date options into a date picture for internal/listing (FormatDate): the day part uses
// the same yyyy yy MM M dd d tokens as core/format.js date(); any other letters are quoted so the Go side copies them
// as text, the way the JS side does. "" means the default picture.
export function datePicture(display = {}) {
  const fmt = display.dateFormat || '';
  let day = 'yyyy-MM-dd';
  if (fmt) {
    day = '';
    const re = /yyyy|yy|MM|M|dd|d/g;
    let at = 0, m;
    const lit = (s) => (s && /[A-Za-z]/.test(s) && !s.includes("'") ? `'${s}'` : s);
    while ((m = re.exec(fmt))) {
      day += lit(fmt.slice(at, m.index)) + m[0];
      at = m.index + m[0].length;
    }
    day += lit(fmt.slice(at));
  }
  const time24 = display.time24 !== false;
  const sec = display.seconds ? ':ss' : '';
  const time = time24 ? `HH:mm${sec}` : `h:mm${sec} tt`;
  const pic = `${day} ${time}`;
  return pic === DEFAULT_PICTURE ? '' : pic;
}

// cellFormat is the "format" of a Views.Cells request: the views' own formats where set, else the Display options.
export function cellFormat(cfg = {}, display = {}) {
  const ownSize = cfg.sizeFormat && cfg.sizeFormat !== 'bytes' ? cfg.sizeFormat : '';
  const size = ownSize || SIZE_UNITS[Number(display.sizeStyle) || 0] || 'bytes';
  const f = { size, date: cfg.dateFormat || datePicture(display), attr: cfg.attrFormat || '' };
  if (display.thousands === false) f.noThousands = true;
  return f;
}

// shownName is a name as a view row shows it: a folder in [brackets] when Display asks for them (never "..").
export function shownName(e, name, display = {}) {
  return e.dir && !e.up && display.brackets ? `[${name}]` : name;
}

// extColumn says whether the Full column view has an Ext column: the views' own "extension in its own column" and the
// Display option must both be on.
export function extColumn(cfg = {}, display = {}) {
  return !cfg.namesWhole && display.separateExt !== false;
}
