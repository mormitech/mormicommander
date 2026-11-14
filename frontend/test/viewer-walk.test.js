// N and P in the Lister (issue #142): files viewed from a temporary copy (inside an archive, Find files results, a fed
// list) walk their list of full paths, each unpacked when it is reached; local folders keep walking their names.
import { describe, it, expect } from 'vitest';
import { makeWalker, walkList } from '../src/features/viewer/walk.js';

const resolve = (n, d) => `${d}/${n}`;

describe('Lister N / P walker', () => {
  it('walks the names of a local folder as before', async () => {
    const w = makeWalker({ path: '/d/a.txt', names: ['a.txt', 'b.dat'], dir: '/d' }, { resolve, localCopy: async () => { throw new Error('no'); } });
    const n = await w.step('/d/a.txt', 1);
    expect(n.path).toBe('/d/b.dat');
    expect(await w.step('/d/b.dat', 1)).toBeNull();
    expect(await w.step('/d/a.txt', -1)).toBeNull();
  });

  it('walks the files of an archive, unpacking each once', async () => {
    const calls = [];
    const localCopy = async (p) => { calls.push(p); return `/tmp/c${calls.length}/${p.split('/').pop()}`; };
    const sources = ['/x/t.tgz/a.txt', '/x/t.tgz/bin.dat', '/x/t.tgz/nums.txt'];
    const req = { path: '/tmp/c0/a.txt', names: ['a.txt'], dir: '/tmp/c0', sources, at: 0 };
    expect(walkList(req)).toEqual(['a.txt', 'bin.dat', 'nums.txt']);
    const w = makeWalker(req, { resolve, localCopy });
    let cur = req.path;
    let n = await w.step(cur, 1);
    expect(n).toMatchObject({ path: '/tmp/c1/bin.dat', name: 'bin.dat' });
    n.done(); cur = n.path;
    n = await w.step(cur, 1);
    expect(n.name).toBe('nums.txt');
    n.done(); cur = n.path;
    expect(await w.step(cur, 1)).toBeNull();
    n = await w.step(cur, -1);
    expect(n.path).toBe('/tmp/c1/bin.dat'); // (the copy made before)
    n.done(); cur = n.path;
    n = await w.step(cur, -1);
    expect(n.path).toBe('/tmp/c0/a.txt'); // (the file shown first)
    expect(calls).toEqual(['/x/t.tgz/bin.dat', '/x/t.tgz/nums.txt']);
  });

  it('walks Find files results in different folders and does not move past a file that failed', async () => {
    const sources = ['/a/1.txt', '/b/2.txt', '/c/3.txt'];
    const w = makeWalker({ path: '/a/1.txt', names: ['1.txt'], dir: '/a', sources, at: 0 }, { resolve, localCopy: async (p) => p });
    const n = await w.step('/a/1.txt', 1);
    expect(n.path).toBe('/b/2.txt');
    // (not done: the view could not open it) the next N tries the same file again
    expect((await w.step('/a/1.txt', 1)).path).toBe('/b/2.txt');
    expect(await w.step('/a/1.txt', -1)).toBeNull();
  });
});
