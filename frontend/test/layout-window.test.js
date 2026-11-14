// @vitest-environment happy-dom
// The window layout (board issue #37): panels on top of each other with Ctrl+Shift+H and back, saved (F-003); the bar
// between the panels — dragging, double-click for half and half, a right-click list of fixed shares (F-008); the text
// size with Ctrl+= / Ctrl++ / Ctrl+-, saved (F-101). The app, the settings and the popup list are stand-ins; the
// registry, the keymap, window.js and splitter.js are the real ones.
import { beforeEach, expect, test, vi } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const settings = {};
const fakePanel = (side) => ({ side, el: null, measure: vi.fn(), render: vi.fn(), ensureVisible: vi.fn(), renderRows: vi.fn() });
const app = {
  platform: 'linux',
  split: 0.5,
  panels: [fakePanel('left'), fakePanel('right')],
  setting: (k, d) => (settings[k] === undefined ? d : settings[k]),
  setSetting: (k, v) => { settings[k] = v; },
  saveSession: vi.fn(),
};
app.left = app.panels[0];
app.right = app.panels[1];
vi.mock('../src/core/app.js', () => ({ app }));
vi.mock('../src/core/go.js', () => ({ go: {}, on: () => {}, errText: String }));
vi.mock('../src/core/dialogs.js', () => ({ messageDialog: async () => '', esc: String }));
let popupAnswer = null;
const popupCalls = [];
vi.mock('../src/core/popup.js', () => ({ showList: async (o) => { popupCalls.push(o); return popupAnswer; } }));

const reg = await import('../src/core/registry.js');
const splitter = await import('../src/core/splitter.js');
const win = await import('../src/features/window.js');

reg.setApp(app);
reg.setupKeys('linux', {});

beforeEach(() => {
  for (const k of Object.keys(settings)) delete settings[k];
  document.body.innerHTML = '<main id="panels"><div class="panel" data-side="left"></div><div id="splitter" role="separator"></div><div class="panel" data-side="right"></div></main>';
  app.left.el = document.querySelector('[data-side="left"]');
  app.right.el = document.querySelector('[data-side="right"]');
  document.documentElement.removeAttribute('style');
  app.saveSession.mockClear();
  popupCalls.length = 0;
  popupAnswer = null;
});

const panels = () => document.getElementById('panels');
const bar = () => document.getElementById('splitter');

test('Ctrl+Shift+H, Ctrl+= / Ctrl++ / Ctrl+- are the keys of the layout commands', () => {
  expect(reg.commandForKey('Ctrl+Shift+H')).toBe('cm_HorizontalPanels');
  expect(reg.commandForKey('Ctrl+=')).toBe('cm_ZoomIn');
  expect(reg.commandForKey('Ctrl+Shift+=')).toBe('cm_ZoomIn');
  expect(reg.commandForKey('Ctrl+-')).toBe('cm_ZoomOut');
  reg.setupKeys('darwin', {});
  expect(reg.commandForKey('Cmd+=')).toBe('cm_ZoomIn');
  expect(reg.commandForKey('Shift+Cmd+=')).toBe('cm_ZoomIn');
  expect(reg.commandForKey('Cmd+-')).toBe('cm_ZoomOut');
  reg.setupKeys('linux', {});
});

test('the panels go on top of each other and back, the setting is saved and the bar turns with them', async () => {
  win.init(app);
  expect(bar().getAttribute('aria-orientation')).toBe('vertical');
  await reg.run('cm_HorizontalPanels');
  expect(panels().classList.contains('horizontal')).toBe(true);
  expect(settings.horizontal).toBe(true);
  expect(bar().getAttribute('aria-orientation')).toBe('horizontal');
  expect(reg.getCommand('cm_HorizontalPanels').checked()).toBe(true);
  await reg.run('cm_HorizontalPanels');
  expect(panels().classList.contains('horizontal')).toBe(false);
  expect(settings.horizontal).toBe(false);
  expect(bar().getAttribute('aria-orientation')).toBe('vertical');
});

test('a saved arrangement comes back at start', () => {
  settings.horizontal = true;
  win.init(app);
  expect(panels().classList.contains('horizontal')).toBe(true);
  expect(bar().getAttribute('aria-orientation')).toBe('horizontal');
});

test('the text size grows and shrinks a step at a time, within limits, and is saved', async () => {
  expect(win.nextFontSize(13, 1)).toBe(14);
  expect(win.nextFontSize(13, -1)).toBe(12);
  expect(win.nextFontSize(22, 1)).toBe(22);
  expect(win.nextFontSize(10, -1)).toBe(10);
  expect(win.nextFontSize(undefined, 1)).toBe(14);
  await reg.run('cm_ZoomIn');
  expect(settings.fontSize).toBe(14);
  expect(document.documentElement.style.getPropertyValue('--row-h')).toBe(`${Math.round(14 * 1.55)}px`);
  expect(app.left.measure).toHaveBeenCalled();
  await reg.run('cm_ZoomOut');
  expect(settings.fontSize).toBe(13);
  expect(document.documentElement.style.getPropertyValue('--row-h')).toBe(`${Math.round(13 * 1.55)}px`);
});

test('shares are kept between 10% and 90%; a missing one is half and half', () => {
  expect(splitter.clampSplit(0.3)).toBe(0.3);
  expect(splitter.clampSplit(0.01)).toBe(0.1);
  expect(splitter.clampSplit(5)).toBe(0.9);
  expect(splitter.clampSplit(undefined)).toBe(0.5);
  expect(splitter.clampSplit(0)).toBe(0.5);
  expect(splitter.clampSplit('x')).toBe(0.5);
});

test('the right-click list offers 50/50, 60/40, 70/30 ... with the current share marked', () => {
  const items = splitter.splitItems(0.6);
  const labels = items.map((i) => i.label.replace('✓ ', ''));
  expect(labels.slice(0, 3)).toEqual(['50/50', '60/40', '70/30']);
  expect(labels).toContain('30/70');
  expect(items.filter((i) => i.label.startsWith('✓')).map((i) => i.value)).toEqual([0.6]);
  expect(splitter.splitItems(0.5, true)[0].detail).toBe('top/bottom');
  expect(splitter.splitItems(0.5, false)[0].detail).toBe('left/right');
});

test('double-click on the bar gives half and half, and is saved', () => {
  splitter.initSplitter(0.3);
  expect(app.split).toBe(0.3);
  expect(app.left.el.style.flex.startsWith('0.3 1 0')).toBe(true);
  bar().dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
  expect(app.split).toBe(0.5);
  expect(app.left.el.style.flex).toBe(app.right.el.style.flex);
  expect(app.saveSession).toHaveBeenCalled();
});

test('choosing a share in the right-click list applies and saves it; Esc changes nothing', async () => {
  splitter.initSplitter(0.5);
  popupAnswer = { item: { value: 0.7 } };
  bar().dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: 10, clientY: 20 }));
  await new Promise((r) => setTimeout(r, 0));
  expect(popupCalls[0].kind).toBe('splitter');
  expect(popupCalls[0].anchor).toEqual({ x: 10, y: 20 });
  expect(app.split).toBe(0.7);
  expect(app.saveSession).toHaveBeenCalledTimes(1);
  popupAnswer = null;
  bar().dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true }));
  await new Promise((r) => setTimeout(r, 0));
  expect(app.split).toBe(0.7);
  expect(app.saveSession).toHaveBeenCalledTimes(1);
});

test('dragging the bar shares the width by where the mouse is let go', () => {
  splitter.initSplitter(0.5);
  panels().getBoundingClientRect = () => ({ left: 0, top: 0, width: 1000, height: 600 });
  bar().dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0, clientX: 500, clientY: 300 }));
  window.dispatchEvent(new MouseEvent('mousemove', { clientX: 300, clientY: 300 }));
  window.dispatchEvent(new MouseEvent('mouseup', { clientX: 300, clientY: 300 }));
  expect(app.split).toBeCloseTo(0.3);
  expect(app.saveSession).toHaveBeenCalled();
});
