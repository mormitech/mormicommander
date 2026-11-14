// Issue #93: the Configuration > Display options (brackets, the extension column, size and date formats, seconds) also
// apply in Brief and the column views (src/features/views-format.js). With the Display options at their defaults the
// views send exactly what they sent before.
import { describe, expect, test } from 'vitest';
import { datePicture, cellFormat, shownName, extColumn } from '../src/features/views-format.js';

const DEFAULT_DISPLAY = { brackets: false, separateExt: true, sizeStyle: 0, thousands: true, dateFormat: '', time24: true, seconds: false };
const DEFAULT_VIEWS = { sizeFormat: 'bytes', dateFormat: '', attrFormat: '', namesWhole: false };

describe('views follow the Display options', () => {
  test('default Display options change nothing', () => {
    expect(cellFormat(DEFAULT_VIEWS, DEFAULT_DISPLAY)).toEqual({ size: 'bytes', date: '', attr: '' });
    expect(cellFormat({}, {})).toEqual({ size: 'bytes', date: '', attr: '' });
    expect(extColumn(DEFAULT_VIEWS, DEFAULT_DISPLAY)).toBe(true);
    expect(shownName({ dir: true }, 'src', DEFAULT_DISPLAY)).toBe('src');
  });

  test('size unit and thousands separator come from Display when the views leave them at default', () => {
    expect(cellFormat(DEFAULT_VIEWS, { ...DEFAULT_DISPLAY, sizeStyle: 1 }).size).toBe('k');
    expect(cellFormat(DEFAULT_VIEWS, { ...DEFAULT_DISPLAY, sizeStyle: 4 }).size).toBe('dynamic');
    expect(cellFormat(DEFAULT_VIEWS, { ...DEFAULT_DISPLAY, thousands: false }).noThousands).toBe(true);
  });

  test("the views' own formats win over Display", () => {
    const f = cellFormat({ ...DEFAULT_VIEWS, sizeFormat: 'M', dateFormat: 'dd.MM.yyyy HH:mm' }, { ...DEFAULT_DISPLAY, sizeStyle: 1, seconds: true });
    expect(f.size).toBe('M');
    expect(f.date).toBe('dd.MM.yyyy HH:mm');
  });

  test('date picture: seconds, 12-hour clock and a custom day format', () => {
    expect(datePicture(DEFAULT_DISPLAY)).toBe('');
    expect(datePicture({ ...DEFAULT_DISPLAY, seconds: true })).toBe('yyyy-MM-dd HH:mm:ss');
    expect(datePicture({ ...DEFAULT_DISPLAY, time24: false })).toBe('yyyy-MM-dd h:mm tt');
    expect(datePicture({ ...DEFAULT_DISPLAY, dateFormat: 'dd.MM.yy' })).toBe('dd.MM.yy HH:mm');
    // letters that are not day tokens stay text on the Go side, as core/format.js date() copies them
    expect(datePicture({ ...DEFAULT_DISPLAY, dateFormat: 'd/M/yyyy at' })).toBe("d/M/yyyy' at' HH:mm");
  });

  test('brackets around folders, never around ".." or files', () => {
    const d = { ...DEFAULT_DISPLAY, brackets: true };
    expect(shownName({ dir: true }, 'src', d)).toBe('[src]');
    expect(shownName({ dir: true, up: true }, '..', d)).toBe('..');
    expect(shownName({ dir: false }, 'a.txt', d)).toBe('a.txt');
  });

  test('the Ext column needs both the views option and the Display option', () => {
    expect(extColumn(DEFAULT_VIEWS, { ...DEFAULT_DISPLAY, separateExt: false })).toBe(false);
    expect(extColumn({ ...DEFAULT_VIEWS, namesWhole: true }, DEFAULT_DISPLAY)).toBe(false);
  });
});
