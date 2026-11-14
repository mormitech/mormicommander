// Tests for features/git-lib.js: the settings defaults (off), the status of an entry, the row classes per setting,
// treating ignored files as hidden, and the branch badge text and tooltip.
import { describe, it, expect } from 'vitest';
import { DEFAULTS, gitConfig, statusOf, statusWord, rowClasses, hidesIgnored, branchText, branchTitle } from '../src/features/git-lib.js';

describe('gitConfig', () => {
  it('is off by default and keeps the current look', () => {
    expect(DEFAULTS.enabled).toBe(false);
    expect(gitConfig(undefined)).toEqual(DEFAULTS);
    expect(gitConfig('junk')).toEqual(DEFAULTS);
  });
  it('takes valid values and drops the rest', () => {
    expect(gitConfig({ enabled: true, column: false, branch: false, ignored: 'hide' })).toEqual({ enabled: true, column: false, branch: false, ignored: 'hide' });
    expect(gitConfig({ enabled: 'yes', ignored: 'vanish' })).toEqual(DEFAULTS);
  });
});

describe('statusOf', () => {
  const r = { repo: true, files: { a: 'M', b: '!' }, all: '' };
  it('reads the letter of an entry', () => {
    expect(statusOf(r, { name: 'a' })).toBe('M');
    expect(statusOf(r, { name: 'c' })).toBe('');
  });
  it('gives every entry the folder-wide status', () => {
    expect(statusOf({ repo: true, files: {}, all: '!' }, { name: 'x' })).toBe('!');
  });
  it('never marks ".." and nothing outside a repository', () => {
    expect(statusOf({ repo: true, files: {}, all: '!' }, { name: '..', up: true })).toBe('');
    expect(statusOf({ repo: false, files: { a: 'M' } }, { name: 'a' })).toBe('');
    expect(statusOf(null, { name: 'a' })).toBe('');
  });
  it('says letters in words', () => {
    expect(statusWord('?')).toBe('not tracked');
    expect(statusWord('Z')).toBe('');
  });
});

describe('rowClasses', () => {
  const on = { ...DEFAULTS, enabled: true };
  it('gives the letter class when the column is on', () => {
    expect(rowClasses('M', on)).toBe('mc-git-m');
    expect(rowClasses('?', on)).toBe('mc-git-new');
    expect(rowClasses('M', { ...on, column: false })).toBe('');
    expect(rowClasses('', on)).toBe('');
  });
  it('dims ignored files only in dim mode', () => {
    expect(rowClasses('!', on)).toBe('mc-git-ign');
    expect(rowClasses('!', { ...on, ignored: 'dim' })).toBe('mc-git-ign mc-git-dim');
    expect(rowClasses('!', { ...on, column: false, ignored: 'dim' })).toBe('mc-git-dim');
  });
  it('hides ignored files only in hide mode', () => {
    expect(hidesIgnored('!', { ...on, ignored: 'hide' })).toBe(true);
    expect(hidesIgnored('!', on)).toBe(false);
    expect(hidesIgnored('M', { ...on, ignored: 'hide' })).toBe(false);
  });
});

describe('branch badge', () => {
  it('shows branch, changes and ahead / behind', () => {
    expect(branchText({ repo: true, branch: 'main' })).toBe('main');
    expect(branchText({ repo: true, branch: 'main', dirty: true, ahead: 2, behind: 1 })).toBe('main ● ↑2 ↓1');
    expect(branchText({ repo: true, detached: true, oid: 'abc1234' })).toBe('(abc1234)');
    expect(branchText({ repo: false, branch: 'main' })).toBe('');
    expect(branchText(null)).toBe('');
  });
  it('explains itself in the tooltip', () => {
    const t = branchTitle({ repo: true, branch: 'dev', oid: 'abc1234', root: '/r', upstream: 'origin/dev', ahead: 1, behind: 0, dirty: false });
    expect(t).toContain('Git branch dev at abc1234');
    expect(t).toContain('Repository: /r');
    expect(t).toContain('Nothing to commit.');
    expect(t).toContain('Upstream origin/dev: 1 commit ahead.');
    expect(branchTitle({ repo: true, branch: 'x', dirty: true })).toContain('No upstream branch.');
    expect(branchTitle({ repo: true, branch: 'x', upstream: 'o/x' })).toContain('up to date');
    expect(branchTitle({ repo: false })).toBe('');
  });
});
