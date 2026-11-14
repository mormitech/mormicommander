// Tests for the transfer error list (board issue #15): the failures read from a job, the text of "Copy list" and
// "Save log", the jobs "Retry failed" starts, and the "Skip errors in background" rules (off: nothing changes).
import { expect, test } from 'vitest';
import { failuresOf, errorCount, listText, logText, logName, retryRequests, onErrorFor, listAtEnd, dirOf, baseOf } from '../src/core/joberrors.js';

const f = (source, target, reason = 'permission denied') => ({ source, target, reason, message: `Can't copy ${baseOf(source || target)}: ${reason}.` });

test('failures come from the engine list, or from plain sentences', () => {
  expect(failuresOf({ failed: [f('/a/x', '/b/x')] })).toEqual([f('/a/x', '/b/x')]);
  expect(failuresOf({ errors: ['Archive is damaged.'] })).toEqual([{ source: '', target: '', reason: '', message: 'Archive is damaged.' }]);
  expect(failuresOf(null)).toEqual([]);
  expect(errorCount({ errorCount: 150, errors: new Array(100).fill('x') })).toBe(150);
  expect(errorCount({ errors: ['x'] })).toBe(1);
});

test('paths split at either separator', () => {
  expect(dirOf('/a/b/c.txt')).toBe('/a/b');
  expect(dirOf('/c.txt')).toBe('/');
  expect(dirOf('C:\\x\\y.txt')).toBe('C:\\x');
  expect(dirOf('C:\\y.txt')).toBe('C:\\');
  expect(baseOf('C:\\x\\y.txt')).toBe('y.txt');
});

test('the copied list has one path and reason per line', () => {
  const list = [f('/a/locked.db', '/b/locked.db', 'the file is in use'), f('/a/secret', '/b/secret'), { source: '', target: '', reason: '', message: 'Archive is damaged.' }];
  expect(listText(list)).toBe('/a/locked.db: the file is in use\n/a/secret: permission denied\nArchive is damaged.');
});

test('the log has a heading, the summary and every failure with its paths', () => {
  const when = new Date(2026, 10, 14, 9, 5, 3);
  const p = { kind: 'copy', target: '/b', summary: 'Copied 98 of 100 files (1.0 MB) in 2s, 2 errors.', errorCount: 3 };
  const text = logText(p, [f('/a/x', '/b/x'), f('/a/y', '/b/y')], when);
  expect(text.split('\n')).toEqual([
    'Copy to /b, ended 2026-11-14 09:05:03', p.summary, '',
    "Can't copy x: permission denied.", '  source: /a/x', '  target: /b/x',
    "Can't copy y: permission denied.", '  source: /a/y', '  target: /b/y',
    '', '…and 1 more errors that were not listed.',
  ]);
  expect(logName(p, when)).toBe('copy-errors-2026-11-14.txt');
});

test('retry copies each failed file to where it was going, keeping the options', () => {
  const req = { kind: 'copy', sources: ['/a/d', '/a/e.txt'], target: '/b', rename: '', overwrite: 'skip', verify: true };
  const { requests, skipped } = retryRequests(req, [f('/a/d/x', '/b/d/x'), f('/a/d/sub/y', '/b/d/sub/y'), f('/a/d/z', '/b/d/z')]);
  expect(skipped).toBe(0);
  expect(requests).toEqual([
    { ...req, sources: ['/a/d/x', '/a/d/z'], target: '/b/d', rename: '' },
    { ...req, sources: ['/a/d/sub/y'], target: '/b/d/sub', rename: '' },
  ]);
});

test('retry keeps a renamed copy under its new name', () => {
  const req = { kind: 'copy', sources: ['/a/x.txt'], target: '/b', rename: 'y.txt' };
  expect(retryRequests(req, [f('/a/x.txt', '/b/y.txt')]).requests).toEqual([{ ...req, sources: ['/a/x.txt'], target: '/b', rename: 'y.txt' }]);
});

test('retry leaves out what a failed folder brings along, and what cannot be retried alone', () => {
  const req = { kind: 'move', sources: ['/a/d'], target: '/b' };
  const { requests, skipped } = retryRequests(req, [f('/a/d', '/b/d'), f('/a/d/x', '/b/d/x'), f('/a/k', '')]);
  expect(requests).toEqual([{ ...req, sources: ['/a/d'], target: '/b', rename: '' }]);
  expect(skipped).toBe(1); // the moved file whose original could not be removed
  expect(retryRequests(req, [{ source: '', target: '', reason: '', message: 'x' }])).toEqual({ requests: [], skipped: 1 });
});

test('a failure of the whole job retries the whole request', () => {
  const req = { kind: 'copy', sources: ['/a/x', '/a/y'], target: '/b/new' };
  expect(retryRequests(req, [f('', '/b/new'), f('/a/x', '/b/new/x')])).toEqual({ requests: [req], skipped: 0 });
});

test('retry of a delete deletes just the failed paths', () => {
  const req = { kind: 'delete', sources: ['/a/d', '/a/e'], onError: 'ask', confirmDirs: true };
  const { requests } = retryRequests(req, [f('/a/d/x', ''), f('/a/d', ''), f('/a/e', '')]);
  expect(requests).toEqual([{ ...req, sources: ['/a/d', '/a/e'] }]);
  expect(retryRequests(req, [])).toEqual({ requests: [], skipped: 0 });
});

test('with the setting off nothing changes; on, background jobs skip errors', () => {
  expect(onErrorFor({})).toBe('ask');
  expect(onErrorFor({ background: true })).toBe('ask');
  expect(onErrorFor({ ignoreErrors: true })).toBe('');
  expect(onErrorFor({ background: true, skipErrorsBackground: true })).toBe('');
  expect(onErrorFor({ background: false, skipErrorsBackground: true })).toBe('ask');
});

test('the list opens by itself only with the setting on and errors in a skipping or background job', () => {
  const failed = { state: 'failed', errorCount: 2 };
  expect(listAtEnd({ background: true, final: failed })).toBe(false);
  expect(listAtEnd({ skipErrorsBackground: true, background: true, final: failed })).toBe(true);
  expect(listAtEnd({ skipErrorsBackground: true, onError: '', final: failed })).toBe(true);
  expect(listAtEnd({ skipErrorsBackground: true, onError: 'ask', final: failed })).toBe(false);
  expect(listAtEnd({ skipErrorsBackground: true, background: true, final: { state: 'done' } })).toBe(false);
  expect(listAtEnd({ skipErrorsBackground: true, background: true, final: { state: 'cancelled', errorCount: 1 } })).toBe(false);
});
