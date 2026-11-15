// @vitest-environment happy-dom
// cm_SearchStandalone (Commands > Find files in a new window, board issue #38) opens the Find files dialog of find.js
// modeless, in a window of its own, whatever the FindFiles.Modeless option says; cm_SearchFor keeps following the
// option. The dialog is a stand-in that only records how it was asked to open.
import { expect, test, vi } from 'vitest';

const metas = [];
vi.mock('../src/core/go.js', () => ({
  go: new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) }),
  on: () => () => {},
  errText: (e) => String(e),
  onFileDrop: () => () => {},
  setClipboardText: async () => true,
}));
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => d, setSetting: () => Promise.resolve(), onSettingChange: () => () => {}, allSettings: () => ({}) }));
vi.mock('../src/core/dialogs.js', async (importOriginal) => ({
  ...(await importOriginal()),
  modal: (_build, cancel, meta) => { metas.push(meta); return Promise.resolve(cancel); },
}));

const { app } = await import('../src/core/app.js');
await import('../src/features/find.js');
const { getCommand } = await import('../src/core/registry.js');

const left = { side: 'left', path: '/Users/me', entries: [], selectedEntries: () => [], fullPath: (n) => n, $: { list: document.createElement('div') } };
Object.assign(app, { platform: 'darwin', panels: [left, left], active: 0 });

test('Find files in a new window opens the dialog modeless; Find files follows the option', async () => {
  await getCommand('cm_SearchStandalone').run(app, {});
  await getCommand('cm_SearchFor').run(app, {});
  expect(metas.map((m) => [m.kind, m.modeless])).toEqual([['find', true], ['find', false]]);
});
