// @vitest-environment happy-dom
// Modeless dialogs in windows (core/dialogs.js meta.modeless, board issue #110): Find files with the option
// FindFiles.Modeless leaves the main window usable, as TC's Find files does. The page is not dimmed, keys aimed at the
// page reach it, a modal dialog the page opens meanwhile takes the page's keys, a dialog opened from the modeless one
// goes over it, and the page gets its focus back when its window is activated. Go is replaced by a recorder, as in
// mirror.test.js; nothing here opens a window.
import { describe, expect, test, vi, afterEach } from 'vitest';

// (see mirror.test.js: happy-dom may drop a MutationObserver's callback otherwise)
globalThis.WeakRef = class StrongRef { constructor(v) { this.v = v; } deref() { return this.v; } };

const calls = [];
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        calls.push({ obj, m, args });
        if (obj === 'DialogWindows' && m === 'Open') {
          const n = calls.filter((c) => c.m === 'Open').length;
          return Promise.resolve({ name: `dialog-${n}`, w: args[0].w, h: args[0].h, sized: false });
        }
        return Promise.resolve(null);
      },
    }),
  });
  return { go, on: () => () => {}, errText: String };
});

const { modal, dialogOpen, keyOwner, keyOwnerBox, focusDialog } = await import('../src/core/dialogs.js');

const tick = () => new Promise((r) => setTimeout(r, 0));
const lastOpen = () => calls.filter((c) => c.m === 'Open').at(-1).args[0];
const key = (target, k = 'Escape') => {
  const ev = new KeyboardEvent('keydown', { key: k, code: k, bubbles: true, cancelable: true });
  target.dispatchEvent(ev);
  return ev;
};

// a stand-in for the panels: a focusable list in the page
function panel() {
  const list = document.createElement('div');
  list.tabIndex = 0;
  list.className = 'panel-list';
  document.body.append(list);
  list.focus();
  return list;
}

afterEach(() => { document.body.innerHTML = ''; });

describe('keyOwner', () => {
  const el = (inside) => ({ contains: (t) => t === inside });
  test('with no detached dialog the top dialog has every key, as before', () => {
    const a = { back: el('a') }, b = { back: el('b') };
    expect(keyOwner([a, b], 'a')).toBe(b);
    expect(keyOwner([a, b], 'page')).toBe(b);
    expect(keyOwner([], 'page')).toBe(null);
  });
  test('a detached dialog has only the keys aimed at it (or at a dialog opened from it)', () => {
    const find = { back: el('find'), detached: true };
    expect(keyOwner([find], 'page')).toBe(null); // the panels' key
    expect(keyOwner([find], 'find')).toBe(find);
    const copy = { back: el('copy') }; // a modal dialog the page opened meanwhile
    expect(keyOwner([find, copy], 'page')).toBe(copy);
    expect(keyOwner([find, copy], 'find')).toBe(find);
    const save = { back: el('save'), detached: true, under: find }; // Save search, opened from Find files
    expect(keyOwner([find, copy, save], 'find')).toBe(save);
    expect(keyOwner([find, copy, save], 'save')).toBe(save);
    expect(keyOwner([find, copy, save], 'page')).toBe(copy);
  });
});

describe('modal(): meta.modeless', () => {
  test('the page is not dimmed or blocked, its keys reach it, keys in the dialog go to the dialog', async () => {
    const list = panel();
    let pageKeys = 0;
    const count = () => { pageKeys++; };
    window.addEventListener('keydown', count);
    const p = modal((b) => { b.innerHTML = '<h2>Find files</h2><input id="ff-for">'; }, 'closed', { kind: 'find', modeless: true });
    await tick();
    const back = document.querySelector('.mc-modal');
    expect(back.classList.contains('mc-modeless')).toBe(true);
    expect(back.getAttribute('aria-modal')).toBe('false');
    expect(lastOpen()).toMatchObject({ kind: 'find', modeless: true, over: '' });
    expect(dialogOpen()).toBe(false);
    const ev = key(list, 'ArrowDown');
    expect(ev.defaultPrevented).toBe(false);
    expect(pageKeys).toBe(1);
    expect(keyOwnerBox(list)).toBe(null);
    const input = back.querySelector('#ff-for');
    expect(keyOwnerBox(input)).toBe(back.firstChild);
    key(input);
    expect(await p).toBe('closed');
    expect(pageKeys).toBe(1); // (the dialog's Esc did not reach the page)
    window.removeEventListener('keydown', count);
  });

  test('a modal dialog the page opens meanwhile takes the page keys; the modeless one keeps its own', async () => {
    const list = panel();
    const find = modal((b) => { b.innerHTML = '<h2>Find files</h2><input id="ff-for">'; }, 'find-closed', { kind: 'find', modeless: true });
    await tick();
    const findInput = document.querySelector('#ff-for');
    list.focus();
    const copy = modal((b) => { b.innerHTML = '<h2>Copy</h2><input id="to">'; }, 'copy-closed', { kind: 'copy' });
    await tick();
    expect(lastOpen().modeless).toBeUndefined();
    expect(document.querySelectorAll('.mc-modal')[1].classList.contains('mc-modeless')).toBe(false);
    expect(dialogOpen()).toBe(true);
    key(findInput); // Esc typed in the Find files window closes it, not the copy dialog
    expect(await find).toBe('find-closed');
    expect(dialogOpen()).toBe(true);
    key(document.body);
    expect(await copy).toBe('copy-closed');
  });

  test('a dialog opened from the modeless one is detached too and goes over its window', async () => {
    panel();
    let finish;
    const find = modal((b, done) => { b.innerHTML = '<h2>Find files</h2><input id="ff-for">'; finish = done; }, null, { kind: 'find', modeless: true });
    await tick();
    const findName = `dialog-${calls.filter((c) => c.m === 'Open').length}`;
    document.querySelector('#ff-for').focus();
    const save = modal((b) => { b.innerHTML = '<h2>Save search</h2><input id="ff-save-name" autofocus>'; }, 'save-closed', { kind: 'find-save' });
    await tick();
    expect(lastOpen()).toMatchObject({ kind: 'find-save', modeless: true, over: findName });
    expect(dialogOpen()).toBe(false);
    expect(document.activeElement.id).toBe('ff-save-name');
    key(document.querySelector('#ff-for')); // a key in the Find files window goes to Save search, which is above it
    expect(await save).toBe('save-closed');
    expect(document.activeElement.id).toBe('ff-for');
    finish('done');
    expect(await find).toBe('done');
  });

  test('the page takes its focus back when its window is activated; closing does not steal it from the panels', async () => {
    const list = panel();
    let finish;
    const p = modal((b, done) => { b.innerHTML = '<h2>Find files</h2><input id="ff-for" autofocus>'; finish = done; }, null, { kind: 'find', modeless: true });
    await tick();
    expect(document.activeElement.id).toBe('ff-for'); // (the hidden copy has it while its window has the user)
    window.dispatchEvent(new Event('focus'));
    expect(document.activeElement).toBe(list);
    const other = document.createElement('input');
    document.body.append(other);
    other.focus(); // the user works on in the page
    finish('x');
    await p;
    expect(document.activeElement).toBe(other);
  });

  test('Alt+F7 again: focusDialog brings the dialog window to the front', async () => {
    let finish, box;
    const p = modal((b, done) => { b.innerHTML = '<h2>Find files</h2>'; finish = done; box = b; }, null, { kind: 'find', modeless: true });
    await tick();
    focusDialog(box);
    expect(calls.at(-1)).toMatchObject({ obj: 'DialogWindows', m: 'Focus' });
    finish(1);
    await p;
  });

  test('without meta.modeless a dialog blocks the page as always', async () => {
    const list = panel();
    const p = modal((b) => { b.innerHTML = '<h2>Copy</h2>'; }, 'c', { kind: 'copy' });
    await tick();
    expect(dialogOpen()).toBe(true);
    expect(document.querySelector('.mc-modal').classList.contains('mc-modeless')).toBe(false);
    const ev = key(list, 'ArrowDown');
    expect(ev.defaultPrevented).toBe(false); // (stopped, not prevented: the panels never see it)
    key(list);
    expect(await p).toBe('c');
  });
});
