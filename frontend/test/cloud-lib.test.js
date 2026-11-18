import { describe, it, expect } from 'vitest';
import { isCloudOnly, cloudRowClass } from '../src/features/cloud-lib.js';
import { F } from '../src/core/format.js';

describe('cloud placeholder marks (#30)', () => {
  it('reads the flag Go sets', () => {
    expect(F.CLOUD).toBe(1024);
    expect(isCloudOnly({ name: 'a', flags: F.CLOUD })).toBe(true);
    expect(isCloudOnly({ name: 'a', flags: F.HIDDEN | F.WINATTRS })).toBe(false);
    expect(isCloudOnly({ name: '..', up: true, flags: F.CLOUD })).toBe(false);
    expect(isCloudOnly(null)).toBe(false);
  });
  it('marks rows only when the option is on (off by default keeps today\'s look)', () => {
    const e = { name: 'a', flags: F.CLOUD | F.DIR };
    expect(cloudRowClass(e, false)).toBe('');
    expect(cloudRowClass(e, true)).toBe('mc-cloud');
    expect(cloudRowClass({ name: 'b', flags: 0 }, true)).toBe('');
  });
});
