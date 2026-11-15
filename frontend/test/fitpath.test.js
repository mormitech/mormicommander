// @vitest-environment happy-dom
// Long paths are shortened at the start, not the end (src/core/fitpath.js, issue #129): the path bar hides leading
// folders behind a "…" so the current folder's name stays visible, and the prompt keeps the end of the path. happy-dom
// lays nothing out, so a stand-in layout makes every character 8 px wide (hidden elements take no room).
import { beforeAll, describe, expect, test, vi } from 'vitest';

// panel.js reaches the Wails runtime through go.js; the runtime starts a 50 ms timer on load that could fire after this
// quick file's environment is gone ("window is not defined"), so the bridge is a stand-in here, as in sel-side.test.js.
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});
import { crumbCut, tailCut, fitCrumbs, fitText, ELLIPSIS } from '../src/core/fitpath.js';
import { splitPath } from '../src/core/panel.js';

const CH = 8;
const textW = (el) => (el.hidden ? 0 : [...el.childNodes].reduce((a, n) => a + (n.nodeType === 3 ? n.textContent.length * CH : textW(n)), 0));
const boxW = new WeakMap();

beforeAll(() => {
  const P = globalThis.HTMLElement.prototype;
  Object.defineProperty(P, 'scrollWidth', { configurable: true, get() { return textW(this); } });
  Object.defineProperty(P, 'clientWidth', { configurable: true, get() { return boxW.get(this) ?? textW(this); } });
  P.getBoundingClientRect = function () { const width = textW(this); return { width, height: 16, x: 0, y: 0, left: 0, top: 0, right: width, bottom: 16 }; };
});

// bar builds a path bar's crumbs box as panel.js draws it, `w` px wide.
function bar(path, w) {
  const box = document.createElement('span');
  box.className = 'crumbs';
  for (const p of splitPath(path)) {
    const s = document.createElement('span');
    s.dataset.p = p.path;
    s.textContent = p.label;
    box.append(s);
  }
  boxW.set(box, w);
  document.body.append(box);
  return box;
}

const shown = (box) => [...box.children].filter((s) => !s.hidden).map((s) => s.textContent).join('');

describe('crumbCut', () => {
  test('nothing is hidden when it fits', () => {
    expect(crumbCut([10, 20, 30], 60, 5)).toBe(0);
    expect(crumbCut([100], 10, 5)).toBe(0);
  });
  test('the fewest leading parts are hidden, counting the "…"', () => {
    expect(crumbCut([10, 20, 30], 59, 5)).toBe(1);   // 5 + 20 + 30 = 55
    expect(crumbCut([10, 20, 30], 40, 5)).toBe(2);   // 5 + 30 = 35
  });
  test('the last part always stays', () => {
    expect(crumbCut([10, 20, 300], 40, 5)).toBe(2);
  });
});

describe('tailCut', () => {
  test('0 when the text fits', () => expect(tailCut(50, true, () => true)).toBe(0));
  test('the smallest cut that fits', () => expect(tailCut(50, false, (i) => 1 + 50 - i <= 20)).toBe(31));
  test('one character stays', () => expect(tailCut(50, false, () => false)).toBe(49));
});

describe('fitCrumbs (issue #129)', () => {
  const deep = '/private/tmp/claude-501/-Users-jpzwei-Code/4e99420a-31b4-47d4-a1ec-5d2793595b36/scratchpad/kbtest/A/sub1/a long folder.d';

  test('a path that fits is left alone', () => {
    const box = bar('/Users/me/Documents', 400);
    fitCrumbs(box);
    expect(shown(box)).toBe('/Users/me/Documents');
    expect(box.querySelector('.cut')).toBeNull();
  });

  test('a long path loses its start and keeps the current folder visible', () => {
    const box = bar(deep, 300);
    fitCrumbs(box);
    const text = shown(box);
    expect(text.startsWith(ELLIPSIS)).toBe(true);
    expect(text.endsWith('sub1/a long folder.d')).toBe(true);
    expect(box.scrollWidth).toBeLessThanOrEqual(300);
    // the "…" goes to the deepest folder it hides; the hidden parts stay in the page
    const cut = box.querySelector('.cut');
    const firstShown = [...box.querySelectorAll('[data-p]:not(.cut)')].find((s) => !s.hidden);
    expect(deep.startsWith(cut.dataset.p)).toBe(true);
    expect(firstShown.dataset.p.startsWith(cut.dataset.p + '/')).toBe(true);
    expect(box.querySelectorAll('[data-p]').length).toBe(splitPath(deep).length + 1);
  });

  test('fitting again after a resize starts over', () => {
    const box = bar(deep, 300);
    fitCrumbs(box);
    boxW.set(box, 2000);
    fitCrumbs(box);
    expect(shown(box)).toBe(deep);
    expect(box.querySelector('.cut')).toBeNull();
  });

  test('a single folder name wider than the bar is cut at its start', () => {
    const box = bar('/tmp/' + 'x'.repeat(60) + 'END', 200);
    fitCrumbs(box);
    const text = shown(box);
    expect(text.startsWith(ELLIPSIS)).toBe(true);
    expect(text.endsWith('END')).toBe(true);
    expect(box.scrollWidth).toBeLessThanOrEqual(200);
    boxW.set(box, 2000);
    fitCrumbs(box);
    expect(shown(box)).toBe('/tmp/' + 'x'.repeat(60) + 'END');
  });
});

describe('fitText (the prompt)', () => {
  test('keeps the end of the path with "…" in front, and no gap', () => {
    const pr = document.createElement('label');
    const bdi = document.createElement('bdi');
    pr.append(bdi);
    boxW.set(pr, 200);
    const full = '/private/tmp/scratchpad/i129/deep/A/sub1/a very long folder name.d>';
    fitText(bdi, full, pr);
    expect(bdi.textContent.startsWith(ELLIPSIS)).toBe(true);
    expect(full.endsWith(bdi.textContent.slice(1))).toBe(true);
    expect(bdi.textContent.length * CH).toBe(200);   // as many characters as fit, so no empty room after the "…"
    expect(bdi.dataset.label).toBe(full);
  });
});
