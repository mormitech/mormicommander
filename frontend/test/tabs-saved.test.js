// Tests for tabs-saved.js (board issue #12): a saved tab comes back with its folder, caption, lock, sort and the
// folder a "locked, but folder changes allowed" tab was locked to, and only a lock folder that differs is saved.
import { describe, it, expect } from 'vitest';
import { tabFields, lockField, tabHistory, backFromHistory, sortFields, sortSaved, sameSort } from '../src/features/tabs-saved.js';

describe('tabFields', () => {
  it('restores a tab locked to another folder than it shows', () => {
    const x = { path: '/a/sub', caption: 'Work', lock: 2, lockPath: '/a', sort: 'size', desc: true };
    const t = tabFields(x);
    expect(t).toMatchObject({ path: '/a/sub', caption: 'Work', lock: 2, lockPath: '/a', sort: { key: 'size', desc: true } });
    expect(t.saved).toBe(x);
  });
  it('uses the tab folder as lock folder when none was saved (older files)', () => {
    expect(tabFields({ path: '/b', lock: 1 }).lockPath).toBe('/b');
  });
  it('gives an unlocked tab no lock folder and the name sort', () => {
    expect(tabFields({ path: '/c', lockPath: '/x' })).toMatchObject({ lock: 0, lockPath: '', sort: { key: 'name', desc: false } });
  });
});

describe('lockField', () => {
  it('saves the lock folder only for a locked tab that left it', () => {
    expect(lockField(2, '/a', '/a/sub')).toEqual({ lockPath: '/a' });
    expect(lockField(2, '/a', '/a')).toEqual({});
    expect(lockField(1, '', '/a')).toEqual({});
    expect(lockField(0, '/a', '/b')).toEqual({});
  });
});

// Board issue #112: every tab's folder history survives a restart, not only the active tab's.
describe('tabHistory', () => {
  it('saves nothing for the active tab (its history is the panel one)', () => {
    expect(tabHistory({ state: { back: ['/a'] } }, true)).toEqual([]);
  });
  it('saves the back list of a tab that was left, newest first', () => {
    expect(tabHistory({ state: { back: ['/old', '/new'] }, saved: { history: ['/x'] } }, false)).toEqual(['/new', '/old']);
  });
  it('keeps what a tab not shown since the restart was read back with', () => {
    const saved = { history: ['/n', '/o'] };
    const h = tabHistory({ state: null, saved }, false);
    expect(h).toEqual(['/n', '/o']);
    expect(h).not.toBe(saved.history);
    expect(tabHistory({ state: null, saved: null }, false)).toEqual([]);
  });
});

describe('backFromHistory', () => {
  const same = (a, b) => a.toLowerCase() === b.toLowerCase();
  it('gives the panel back list oldest first without the shown folder', () => {
    expect(backFromHistory(['/b', '/A', '', '/c'], '/a', same)).toEqual(['/c', '/b']);
  });
  it('gives an empty list for a tab saved without history (older files)', () => {
    expect(backFromHistory(undefined, '/a', same)).toEqual([]);
  });
});

// F-114 (board issue #103): the second sort key is saved with the tab and comes back
describe('second sort key', () => {
  it('saves and restores sort.then', () => {
    const so = { key: 'ext', desc: false, then: { key: 'size', desc: true } };
    const saved = sortSaved(so);
    expect(saved).toEqual({ sort: 'ext', desc: false, sort2: 'size', desc2: true });
    expect(sortFields(saved)).toEqual(so);
    expect(tabFields({ path: '/a', ...saved }).sort).toEqual(so);
  });
  it('saves no second key when there is none, and ignores one equal to the first', () => {
    expect(sortSaved({ key: 'name', desc: true })).toEqual({ sort: 'name', desc: true });
    expect(sortFields({ sort: 'size', sort2: 'size' })).toEqual({ key: 'size', desc: false });
  });
  it('compares sorts with the second key', () => {
    expect(sameSort({ key: 'ext', desc: false }, { key: 'ext', desc: false })).toBe(true);
    expect(sameSort({ key: 'ext', desc: false, then: { key: 'size', desc: true } }, { key: 'ext', desc: false })).toBe(false);
    expect(sameSort({ key: 'ext', then: { key: 'size', desc: true } }, { key: 'ext', desc: false, then: { key: 'size', desc: true } })).toBe(true);
  });
});
