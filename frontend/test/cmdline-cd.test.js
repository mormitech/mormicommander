// @vitest-environment happy-dom
// The command line's "cd" (src/features/cmdline.js, board issue #130): "cd\" and "cd.." work without the space as in
// Total Commander, and environment variables ("cd $HOME", "cd ${HOME}/x", "cd %HOME%") are expanded by asking Go.
// The app, the registry's run and the Go side are stand-ins; the command line and the handler are the real ones.
import { beforeEach, expect, test, vi } from 'vitest';

const went = [];
const ran = [];
vi.mock('../src/core/app.js', () => ({
  app: {
    platform: 'darwin',
    home: '/Users/me',
    cur: { path: '/Users/me/A', go: (p) => { went.push(p); return Promise.resolve(); } },
    resolvePath: (p, from) => (p.startsWith('/') ? p : `${from}/${p}`),
    run: () => Promise.resolve(),
    flash: () => {},
  },
}));
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => d, setSetting: () => {} }));
vi.mock('../src/core/go.js', () => ({
  go: { SystemLaunch: { ExpandEnv: (s) => Promise.resolve(s.replace(/\$\{HOME\}|\$HOME|%HOME%/g, '/Users/me')) } },
}));
vi.mock('../src/core/registry.js', () => ({
  command: () => {}, hasCommand: () => false, commandList: () => [],
  run: (name) => { ran.push(name); return Promise.resolve(); },
}));

const { cdArg } = await import('../src/features/cmdline.js');
const { cmdline } = await import('../src/core/cmdline.js');

beforeEach(() => {
  document.body.innerHTML = '<input id="cmd" type="text">';
  went.length = 0;
  ran.length = 0;
});

async function enter(text) {
  document.getElementById('cmd').value = text;
  return cmdline.submit();
}

test('cd\\ and cd.. without a space are cd commands; other words are not', () => {
  expect(cdArg('cd\\')).toBe('\\');
  expect(cdArg('cd..')).toBe('..');
  expect(cdArg('cd/')).toBe('/');
  expect(cdArg('cd ..')).toBe('..');
  expect(cdArg('cd')).toBe('');
  expect(cdArg('cdrecord x')).toBe(null);
  expect(cdArg('cd.sh')).toBe(null);
});

test('cd\\ goes to the root and cd.. to the parent', async () => {
  expect(await enter('cd\\')).toBe(true);
  expect(await enter('cd..')).toBe(true);
  expect(ran).toEqual(['cm_GoToRoot', 'cm_GoToParent']);
});

test('environment variables are expanded', async () => {
  await enter('cd $HOME');
  await enter('cd ${HOME}/Documents');
  await enter('cd %HOME%');
  await enter('cd sub');
  expect(went).toEqual(['/Users/me', '/Users/me/Documents', '/Users/me', '/Users/me/A/sub']);
});
