// The command palette's matching and ranking (features/palette-lib.js, board #21, F-857 / F-373): "sync" finds
// Synchronize folders, spread-out letters match, recent use ranks first, the recent list stays short and unique, and the
// hotlist tree and folder histories flatten the way the palette lists them. The Ctrl+Shift+P key is checked against
// the real keymap too, so it never takes a Total Commander key.
import { describe, expect, test } from 'vitest';
import { matchWord, scoreItem, rankItems, noteRecent, commandWords, uniqueFolders, flattenHotlist, RECENT_MAX } from '../src/features/palette-lib.js';
import { DEFAULT_KEYS, MAC_KEYS } from '../src/core/keymap.js';
import { KEY_PRESETS } from '../src/core/keypresets.js';
import { normalizeCombo } from '../src/core/keys.js';

const cmd = (name, label, extra = {}) => ({ key: `cmd:${name}`, group: 'cmd', label, detail: name, words: commandWords(name), ...extra });
const items = [
  cmd('cm_Copy', 'Copy…'),
  cmd('cm_SyncDirs', 'Synchronize folders…'),
  cmd('cm_SearchFor', 'Find files…'),
  cmd('cm_DirectoryHotlist', 'Folder hotlist'),
  cmd('cm_SrcByName', 'Sort by name'),
  { key: 'dir:/Users/me/sync', group: 'dir', label: '/Users/me/sync' },
  { key: 'page:Layout', group: 'page', label: 'Options: Layout' },
];

describe('matching', () => {
  test('substrings beat spread-out letters, word starts beat the middle', () => {
    expect(matchWord('sync', 'Synchronize folders…')).toBeGreaterThan(matchWord('sync', 'Async thing'));
    expect(matchWord('fold', 'Synchronize folders')).toBeGreaterThan(matchWord('sydi', 'Sync dirs'));
    expect(matchWord('sydi', 'Sync dirs')).toBeGreaterThan(0);
    expect(matchWord('xyz', 'Sync dirs')).toBe(-1);
    expect(matchWord('', 'anything')).toBe(0);
  });

  test('every word of the query must match', () => {
    expect(scoreItem('sort name', items[4])).toBeGreaterThan(0);
    expect(scoreItem('sort zebra', items[4])).toBe(-1);
  });

  test('the cm_* name finds a command too', () => {
    expect(scoreItem('SyncDirs', items[1])).toBeGreaterThan(0);
    expect(scoreItem('cm_searchfor', items[2])).toBeGreaterThan(0);
    expect(commandWords('cm_SyncDirs')).toEqual(['cm_SyncDirs', 'SyncDirs', 'sync dirs']);
  });
});

describe('ranking', () => {
  test('typing "sync" lists Synchronize folders first', () => {
    const r = rankItems(items, 'sync');
    expect(r[0].key).toBe('cmd:cm_SyncDirs');
    expect(r.map((x) => x.key)).toContain('dir:/Users/me/sync');
    expect(r.map((x) => x.key)).not.toContain('cmd:cm_Copy');
  });

  test('nothing typed: recent items first, newest first, then the rest in order', () => {
    const r = rankItems(items, '', ['page:Layout', 'cmd:cm_SearchFor', 'gone:x']);
    expect(r.slice(0, 3).map((x) => x.key)).toEqual(['page:Layout', 'cmd:cm_SearchFor', 'cmd:cm_Copy']);
    expect(r).toHaveLength(items.length);
  });

  test('recent use lifts an item among matches', () => {
    expect(rankItems(items, 'sync')[0].key).toBe('cmd:cm_SyncDirs');
    expect(rankItems(items, 'sync', ['dir:/Users/me/sync'])[0].key).toBe('dir:/Users/me/sync');
  });

  test('disabled items sink below usable ones that match as well', () => {
    const two = [cmd('cm_A', 'Pack files', { disabled: true }), cmd('cm_B', 'Pack files')];
    expect(rankItems(two, 'pack').map((x) => x.key)).toEqual(['cmd:cm_B', 'cmd:cm_A']);
  });

  test('limit cuts the list', () => {
    expect(rankItems(items, '', [], 2)).toHaveLength(2);
  });
});

describe('recent list', () => {
  test('newest first, once each, at most RECENT_MAX', () => {
    expect(noteRecent(['a', 'b', 'c'], 'b')).toEqual(['b', 'a', 'c']);
    expect(noteRecent(undefined, 'a')).toEqual(['a']);
    expect(noteRecent([1, 'x'], 'a')).toEqual(['a', 'x']);
    const long = Array.from({ length: 50 }, (_, i) => `k${i}`);
    expect(noteRecent(long, 'new')).toHaveLength(RECENT_MAX);
  });
});

describe('sources', () => {
  test('folders once each, ignoring a trailing separator and (by default) case', () => {
    expect(uniqueFolders(['/a/b/', '/a/b', '/A/B', '', '/c'])).toEqual(['/a/b/', '/c']);
    expect(uniqueFolders(['/a', '/A'], true)).toEqual(['/a', '/A']);
    expect(uniqueFolders(['/'])).toEqual(['/']);
  });

  test('the hotlist tree flattens with submenu trails, without separators', () => {
    const tree = [
      { kind: 'cmd', title: 'Home', cmd: 'cd /Users/me' },
      { kind: 'sep' },
      { kind: 'menu', title: 'Work', items: [{ kind: 'cmd', title: 'Sites', cmd: 'cd "/srv/my sites"' }, { kind: 'menu', title: 'Deep', items: [{ kind: 'cmd', cmd: 'cd /x' }] }] },
    ];
    const f = flattenHotlist(tree);
    expect(f.map((x) => [x.label, x.trail])).toEqual([['Home', ''], ['Sites', 'Work'], ['/x', 'Work › Deep']]);
    expect(f[1].entry.cmd).toBe('cd "/srv/my sites"');
    expect(flattenHotlist(null)).toEqual([]);
  });
});

describe('the palette key', () => {
  test('Ctrl+Shift+P and Shift+Cmd+P are free in the default keymaps and the presets', () => {
    const taken = (map) => Object.keys(map || {}).map(normalizeCombo);
    const keys = ['Ctrl+Shift+P', 'Shift+Cmd+P'].map(normalizeCombo);
    for (const k of keys) {
      expect(taken(DEFAULT_KEYS)).not.toContain(k);
      expect(taken(MAC_KEYS)).not.toContain(k);
      for (const p of Object.values(KEY_PRESETS)) expect(taken(p.keys)).not.toContain(k);
    }
  });
});

test('assignTarget names the command of a command item only (F-373)', async () => {
  const { assignTarget } = await import('../src/features/palette-lib.js');
  expect(assignTarget({ key: 'cmd:cm_Copy', group: 'cmd' })).toBe('cm_Copy');
  expect(assignTarget({ key: 'cmd:em_backup', group: 'cmd' })).toBe('em_backup');
  expect(assignTarget({ key: 'dir:/tmp', group: 'dir' })).toBe('');
  expect(assignTarget({ key: 'page:Layout', group: 'page' })).toBe('');
  expect(assignTarget({ key: 'cmd:not a name', group: 'cmd' })).toBe('');
  expect(assignTarget(undefined)).toBe('');
});
