// Tests for tabs-saved.js (board issue #12): a saved tab comes back with its folder, caption, lock, sort and the
// folder a "locked, but folder changes allowed" tab was locked to, and only a lock folder that differs is saved.
import { describe, it, expect } from 'vitest';
import { tabFields, lockField } from '../src/features/tabs-saved.js';

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
