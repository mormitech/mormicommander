// Printing from the Lister (F-581 / F-843): the File menu has Print… with its key, Ctrl+P (Cmd+P on macOS) runs it, and
// what is printed: the selected text in a text mode, otherwise the file. The view's methods run on a stand-in view.
import { describe, it, expect, vi } from 'vitest';
import { printPlan, printerNote } from '../src/features/viewer/print.js';

const { ListerView } = await import('../src/features/viewer/view.js');

const stand = (o = {}) => ({
  app: { platform: 'linux' }, id: 1, info: { name: 'a.txt', path: '/tmp/a.txt' }, mode: 'text', follow: false, fit: true, flipH: false,
  variable: false, wrap: false, enc: '', asText: '', onNeighbor: null, onClose: null, internalOnly: false, sel: null,
  isTextMode: ListerView.prototype.isTextMode, selRange: ListerView.prototype.selRange, altKey: () => 'Alt+F3', ...o,
});

describe('printPlan', () => {
  it('prints the selected text in a text mode, else the file', () => {
    expect(printPlan({ selection: [10, 20], textMode: true, name: 'a.txt' })).toMatchObject({ kind: 'text', start: 10, end: 20, title: 'a.txt (selection)' });
    expect(printPlan({ selection: [10, 20], textMode: false, name: 'b.png' })).toMatchObject({ kind: 'file', question: 'Print b.png?' });
    expect(printPlan({ selection: null, textMode: true, name: 'a.txt' }).kind).toBe('file');
    expect(printPlan({ selection: [5, 5], textMode: true, name: 'a.txt' }).kind).toBe('file');
  });
  it('says where the job goes', () => {
    expect(printerNote({ platform: 'windows' })).toMatch(/Windows/);
    expect(printerNote({ platform: 'darwin' })).toMatch(/lp/);
  });
});

describe('Lister Print in the menu and on its key', () => {
  it('File menu has Print… with Ctrl+P, and Print selection… when text is selected', () => {
    const file = (v) => ListerView.prototype.menuItems.call(v, 'File');
    const it1 = file(stand()).find((x) => x.label === 'Print…');
    expect(it1).toBeTruthy();
    expect(it1.detail).toMatch(/P/);
    expect(it1.disabled).toBe(false);
    const v = stand({ sel: { a: 0, b: 4 }, print: vi.fn() });
    const it2 = file(v).find((x) => x.label === 'Print selection…');
    expect(it2).toBeTruthy();
    it2.run();
    expect(v.print).toHaveBeenCalledOnce();
  });
  it('Ctrl+P runs the same, and Cmd+P on macOS', () => {
    for (const [platform, combo] of [['linux', 'Ctrl+P'], ['windows', 'Ctrl+P'], ['darwin', 'Cmd+P']]) {
      const v = stand({ app: { platform }, print: vi.fn() });
      expect(ListerView.prototype.key.call(v, combo)).toBe(true);
      expect(v.print).toHaveBeenCalledOnce();
    }
  });
});
