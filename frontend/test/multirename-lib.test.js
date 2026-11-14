// The "[=?] Field…" menu of the Multi-Rename tool (board issue #7): built-in fields as before, then the media fields.
import { expect, test } from 'vitest';
import { fieldMenuItems } from '../src/features/multirename-lib.js';

test('built-in fields keep their [=tc.name] form', () => {
  expect(fieldMenuItems(['size', 'week'])).toEqual([
    { label: '[=tc.size]', text: '[=tc.size]', detail: 'size' },
    { label: '[=tc.week]', text: '[=tc.week]', detail: 'week' },
  ]);
});

test('media fields follow after a separator, by their own name', () => {
  const m = fieldMenuItems(['size'], ['img.exifdate', 'audio.artist', 'odd.field']);
  expect(m[1]).toEqual({ sep: true });
  expect(m[2]).toMatchObject({ label: '[=img.exifdate]', text: '[=img.exifdate]' });
  expect(m[2].detail).toContain('EXIF');
  expect(m[3].text).toBe('[=audio.artist]');
  expect(m[4].detail).toBe('odd.field');
});

test('no separator without both groups', () => {
  expect(fieldMenuItems([], ['img.width'])).toEqual([{ label: '[=img.width]', text: '[=img.width]', detail: 'Image width in pixels' }]);
  expect(fieldMenuItems()).toEqual([]);
});
