// @vitest-environment happy-dom
// Server transfers in the background transfer window (F-382) and the server bar's Abort button (F-817), issue #63.
// Both are behind Options > FTP settings that are off by default ('FTP.ListTransfers', 'FTP.AbortButton'). Go is a
// stand-in: StartJob hands out a number, events are sent by the test, and nothing ever connects anywhere.
import { beforeEach, expect, test, vi } from 'vitest';

const calls = [];
const handlers = new Map();
const answers = { 'Net.StartJob': 7, 'Net.RunningOn': 0, 'Net.AbortOn': 1, 'Net.Status': { remote: true, kind: 'ftp', connected: true, mode: 'binary' }, 'Jobs.List': [], 'DialogWindows.Open': { name: 'w' } };
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: (_, svc) => new Proxy({}, { get: (__, m) => (...a) => { const k = `${String(svc)}.${String(m)}`; calls.push([k, ...a]); return Promise.resolve(answers[k] ?? null); } }) });
  return {
    go,
    on: (ev, fn) => { handlers.set(ev, fn); return () => handlers.delete(ev); },
    errText: (e) => String(e?.message ?? e),
    onFileDrop: () => () => {},
    setClipboardText: async () => true,
  };
});

const { app } = await import('../src/core/app.js');
const jobs = await import('../src/core/jobs.js');
const net = await import('../src/features/net.js');
const tick = () => new Promise((r) => setTimeout(r, 0));
const emit = (ev, p) => handlers.get(ev)?.(p);

beforeEach(() => {
  calls.length = 0;
  app.config = { values: {} };
  app.panels = [];
  answers['Net.RunningOn'] = 0;
  document.body.innerHTML = '';
});

test('both options are off by default', () => {
  expect(net.listTransfersWanted({})).toBe(false);
  expect(net.listTransfersWanted(undefined)).toBe(false);
  expect(net.abortWanted({})).toBe(false);
  expect(net.listTransfersWanted({ 'FTP.ListTransfers': '1' })).toBe(true);
  expect(net.abortWanted({ 'FTP.AbortButton': '1' })).toBe(true);
});

test('a tracked server transfer is listed with progress and can be stopped from the transfer window', async () => {
  const cancel = vi.fn();
  const t = jobs.trackJob(-3, { req: { kind: 'copy', sources: ['ftp://h/a.bin'], target: '/tmp' }, title: 'Copying', cancel });
  t.update({ id: 3, kind: 'copy', state: 'running', files: 1, filesDone: 0, bytes: 100, bytesDone: 40, current: 'ftp://h/a.bin', target: '/tmp' });
  const row = jobs.jobList().find((j) => j.id === -3);
  expect(row).toMatchObject({ state: 'running', bytesDone: 40, server: true, sources: ['ftp://h/a.bin'] });
  const open = jobs.showTransfers();
  await tick(); await tick();
  const box = document.querySelector('.mc-jobs-transfers');
  expect(box).toBeTruthy();
  const r = box.querySelector('[data-id="-3"]');
  expect(r.textContent).toContain('Copy (server)');
  expect(r.textContent).toContain('40%');
  r.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
  expect(box.querySelector('.pause').disabled).toBe(true); // a server transfer can't be paused
  expect(box.querySelector('.stop').disabled).toBe(false);
  box.querySelector('.stop').click();
  expect(cancel).toHaveBeenCalledTimes(1);
  t.done({ id: 3, kind: 'copy', state: 'cancelled', files: 1, filesDone: 0, bytes: 100, bytesDone: 40, errors: [] });
  expect(jobs.jobList().find((j) => j.id === -3).state).toBe('cancelled');
  expect(box.querySelector('.stop').disabled).toBe(true);
  box.querySelector('.clear').click();
  await tick();
  expect(jobs.jobList().find((j) => j.id === -3)).toBeUndefined();
  box.querySelector('.close').click();
  await open;
});

test('runNetJob lists the transfer only with the option on', async () => {
  answers['Net.StartJob'] = 7;
  const off = net.runNetJob({ kind: 'copy', sources: ['ftp://h/x'], target: '/tmp' });
  await tick();
  expect(jobs.jobList().some((j) => j.id === -7)).toBe(false);
  emit('net-job-done', { id: 7, kind: 'copy', state: 'done', files: 1, filesDone: 1, bytes: 1, bytesDone: 1, errors: [] });
  await off;

  app.config.values['FTP.ListTransfers'] = '1';
  answers['Net.StartJob'] = 8;
  const on = net.runNetJob({ kind: 'copy', sources: ['ftp://h/y'], target: '/tmp' });
  await tick();
  expect(jobs.jobList().find((j) => j.id === -8)).toMatchObject({ state: 'running', server: true });
  emit('net-job', { id: 8, kind: 'copy', state: 'running', files: 2, filesDone: 1, bytes: 10, bytesDone: 5, current: 'ftp://h/y/a', errors: [] });
  expect(jobs.jobList().find((j) => j.id === -8).bytesDone).toBe(5);
  emit('net-job-done', { id: 8, kind: 'copy', state: 'done', files: 2, filesDone: 2, bytes: 10, bytesDone: 10, errors: [] });
  const final = await on;
  expect(final.state).toBe('done');
  expect(jobs.jobList().find((j) => j.id === -8).state).toBe('done');
});

test('Background in the progress box closes it without stopping the transfer', async () => {
  app.config.values['FTP.ListTransfers'] = '1';
  answers['Net.StartJob'] = 9;
  const run = net.runNetJob({ kind: 'copy', sources: ['ftp://h/z'], target: '/tmp' });
  await new Promise((r) => setTimeout(r, 450)); // the box shows after a moment
  const bg = document.querySelector('.mc-modal .bg');
  expect(bg).toBeTruthy();
  bg.click();
  await tick();
  expect(document.querySelector('.mc-modal .bg')).toBeNull();
  expect(calls.some(([k]) => k === 'Net.CancelJob')).toBe(false);
  emit('net-job-done', { id: 9, kind: 'copy', state: 'done', files: 1, filesDone: 1, bytes: 1, bytesDone: 1, errors: [] });
  await run;
});

test('without the option the progress box has no Background button', async () => {
  answers['Net.StartJob'] = 10;
  const run = net.runNetJob({ kind: 'copy', sources: ['ftp://h/w'], target: '/tmp' });
  await new Promise((r) => setTimeout(r, 450));
  expect(document.querySelector('.mc-modal .cancel')).toBeTruthy();
  expect(document.querySelector('.mc-modal .bg')).toBeNull();
  emit('net-job-done', { id: 10, kind: 'copy', state: 'done', files: 1, filesDone: 1, bytes: 1, bytesDone: 1, errors: [] });
  await run;
});

test('the server bar shows Abort only with the option on; it stops the transfers of that server', async () => {
  const pbtns = document.createElement('span');
  document.body.append(pbtns);
  const p = { path: 'ftp://h/pub', $: { pbtns }, reload: async () => {} };
  app.panels = [p];
  net.init(app);
  const el = () => document.querySelector('.mc-net-abort');
  app.emit('panel', p, { moved: true });
  await tick(); await tick();
  expect(el().hidden).toBe(true); // the bar looks as before

  app.config.values['FTP.AbortButton'] = '1';
  app.emit('panel', p, { moved: true });
  await tick(); await tick();
  expect(el().hidden).toBe(false);
  expect(el().disabled).toBe(true); // nothing runs

  answers['Net.RunningOn'] = 1;
  app.emit('panel', p, { moved: true });
  await tick(); await tick();
  expect(el().disabled).toBe(false);
  calls.length = 0;
  el().click();
  await tick();
  expect(calls).toContainEqual(['Net.AbortOn', 'ftp://h/pub']);
});
