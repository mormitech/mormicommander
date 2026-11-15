// @vitest-environment happy-dom
// cm_VolumeId (Commands > Volume information, board issue #38) in features/system-menus.js: it shows the Properties box
// of the volume the active panel is on, with a note on where the label can be changed, and never renames anything
// itself. The registry, the Go side, the dialog and the app are stand-ins.
import { beforeEach, expect, test, vi } from 'vitest';

const cmds = new Map();
const goCalls = [];
let shown = null; // the html of the last dialog

vi.mock('../src/core/registry.js', () => ({ command: (c) => cmds.set(c.name, c), hasCommand: () => true, getCommand: () => null }));
vi.mock('../src/core/go.js', () => ({
  go: new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        goCalls.push(`${String(obj)}.${String(m)}`);
        if (obj === 'SystemFiles' && m === 'VolumeProps') return Promise.resolve({ name: 'Macintosh HD', root: '/', fileSystem: 'apfs', total: 1000, used: 400, available: 600 });
        return Promise.resolve(null);
      },
    }),
  }),
  errText: (e) => String(e),
}));
vi.mock('../src/core/popup.js', () => ({ showList: () => Promise.resolve(null) }));
vi.mock('../src/core/dialogs.js', () => ({
  esc: (s) => String(s),
  modal: (build, cancel) => {
    const box = document.createElement('form');
    build(box, () => {});
    shown = box.innerHTML;
    return Promise.resolve(cancel);
  },
}));
vi.mock('../src/core/control.js', () => ({ addControlCommand: () => {} }));
vi.mock('../src/core/app.js', () => ({ app: {} }));
vi.mock('../src/features/system.js', () => ({ anchorFor: () => null, revealLabel: () => 'Show in Finder', openWith: () => {} }));
vi.mock('../src/features/drivemaps.js', () => ({ mapFolder: () => {}, unmapFolder: () => {}, moveMap: () => {}, isMapped: () => false, mapIndex: () => -1, mapCount: () => 0 }));

await import('../src/features/system-menus.js');

const flashes = [];
const vol = { name: 'Macintosh HD', path: '/', kind: 'fixed' };
const app = {
  platform: 'darwin', demo: false, cur: { path: '/Users/me' },
  volumeFor: (p) => (p.startsWith('/') ? vol : null),
  flash: (m) => flashes.push(m),
};

beforeEach(() => { goCalls.length = 0; flashes.length = 0; shown = null; });

test('cm_VolumeId shows the volume of the active panel with a note on renaming', async () => {
  expect(cmds.get('cm_VolumeId').label).toBe('Volume information…');
  await cmds.get('cm_VolumeId').run(app);
  expect(goCalls).toEqual(['SystemFiles.VolumeProps']);
  expect(shown).toContain('Macintosh HD');
  expect(shown).toContain('apfs');
  expect(shown).toContain('Finder or in Disk Utility');
});

test('cm_VolumeId says so when the panel is not on a local volume', async () => {
  app.cur.path = 'ftp://host/x';
  expect(await cmds.get('cm_VolumeId').run(app)).toBe(false);
  expect(goCalls).toEqual([]);
  expect(flashes[0]).toMatch(/not on a volume/);
  app.cur.path = '/Users/me';
});
