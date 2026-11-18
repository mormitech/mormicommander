// F-492: Options > Copy/Delete > "Show a summary when a copy or move ends" (off by default) makes a foreground copy or
// move say how it went. Tests summaryWanted of src/core/jobs.js.
import { describe, expect, test } from 'vitest';
import { summaryWanted } from '../src/core/jobs.js';

describe('summaryWanted', () => {
  test('off by default', () => {
    expect(summaryWanted({ kind: 'copy' }, {})).toBe(false);
    expect(summaryWanted({ kind: 'copy' }, undefined)).toBe(false);
  });
  test('on: copy and move only', () => {
    const v = { 'CopyDelete.ShowSummary': '1' };
    expect(summaryWanted({ kind: 'copy' }, v)).toBe(true);
    expect(summaryWanted({ kind: 'move' }, v)).toBe(true);
    expect(summaryWanted({ kind: 'delete' }, v)).toBe(false);
    expect(summaryWanted(null, v)).toBe(false);
  });
});
