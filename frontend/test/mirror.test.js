// @vitest-environment happy-dom
// Dialogs in windows of their own (core/mirror.js + core/mirror-copy.js): the dialog stays in this page and a second
// document (standing in for the dialog window) builds a copy from the description, applies the patches, and sends the
// user's actions back. Go is replaced by a recorder, so this checks the mirror's logic end to end without a window.
import { describe, expect, test, vi, beforeAll, afterEach } from 'vitest';
import { Window } from 'happy-dom';

const calls = [];
const handlers = {};
let openReply = (req) => ({ name: `dialog-${calls.filter((c) => c.m === 'Open').length}`, w: req.w, h: req.h, sized: false });
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        calls.push({ obj, m, args });
        if (obj === 'DialogWindows' && m === 'Open') return new Promise((r) => r(openReply(args[0])));
        return Promise.resolve(null);
      },
    }),
  });
  return { go, on: (ev, fn) => { handlers[ev] = fn; return () => {}; }, errText: String };
});

const { mirrorDialog, mirrorAvailable } = await import('../src/core/mirror.js');
const { modal, currentDialog, answerDialog } = await import('../src/core/dialogs.js');
const { Copy } = await import('../src/core/mirror-copy.js');
const { Sequencer } = await import('../src/core/mirror-dom.js');

const tick = () => new Promise((r) => setTimeout(r, 0));
const pushesFor = (name) => calls.filter((c) => c.m === 'Push' && c.args[0] === name).map((c) => c.args[1]);

// win builds the stand-in for a dialog window from the last Open and keeps it in step with the Pushes.
function win(openIndex = -1) {
  const opens = calls.filter((c) => c.m === 'Open');
  const at = openIndex < 0 ? opens.length + openIndex : openIndex;
  const req = opens[at].args[0];
  const name = `dialog-${at + 1}`;
  const w2 = new Window();
  const copy = new Copy(w2.document);
  copy.applyEnv(req.payload.env);
  w2.document.body.append(copy.root(req.payload.tree));
  const seq = new Sequencer((m) => copy.apply(m));
  let seen = 0;
  const sync = () => { const all = pushesFor(name); for (; seen < all.length; seen++) seq.push(all[seen].q, all[seen]); };
  let q = 0;
  const input = (m) => handlers['dlg-input'](name, { q: ++q, ...m });
  return { req, name, w2, copy, sync, input, doc: w2.document };
}

function makeDialog(html, kind = 'test') {
  const back = document.createElement('div');
  back.className = 'mc-modal';
  const box = document.createElement('form');
  box.className = 'mc-box';
  box.innerHTML = html;
  back.append(box);
  document.body.append(back);
  const entry = { box, meta: { kind }, cancel: vi.fn() };
  return { back, box, entry };
}

const FORM = `<h2>Copy files</h2><p class="mc-msg">Copy 3 files to:</p><input id="to" value="/tmp">
  <label><input type="checkbox" id="verify"> Verify</label><select id="mode"><option>Ask</option><option>Overwrite</option><option>Skip</option></select>
  <input type="number" id="n" value="4"><textarea id="note">hi</textarea><div class="list"><p>a</p><p>b</p><p>c</p></div>
  <div class="mc-btns"><button type="button" class="primary">OK</button><button type="button">Cancel</button></div>`;

beforeAll(() => { expect(mirrorAvailable()).toBe(true); });
afterEach(() => { document.body.innerHTML = ''; });

describe('mirror: the copy follows the dialog', () => {
  test('the window gets the whole dialog, its title and its kind', async () => {
    const { back, box, entry } = makeDialog(FORM, 'copy');
    const s = mirrorDialog(entry, back, box, 0);
    expect(box.classList.contains('mc-mirror-box')).toBe(true);
    expect(back.classList.contains('mc-mirror-back')).toBe(true);
    await tick();
    const w = win();
    expect(w.req.kind).toBe('copy');
    expect(w.req.title).toBe('Copy files');
    expect(w.copy.box.innerHTML).toBe(box.innerHTML);
    expect(w.copy.box.classList.contains('mc-mirror-box')).toBe(false); // (the owning page's marks stay there)
    expect(w.copy.box.classList.contains('mc-window-box')).toBe(true);
    expect(s.name).toBe(w.name);
    s.close();
    expect(calls.some((c) => c.m === 'Close' && c.args[0] === w.name)).toBe(true);
  });

  test('changes become patches; known elements are moved, never rebuilt', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    const list = box.querySelector('.list');
    const copyA = w.copy.box.querySelector('.list p');
    // reorder, add, remove, change a text and an attribute
    list.append(list.firstElementChild); // a to the end
    const d = document.createElement('p');
    d.textContent = 'd';
    d.setAttribute('onclick', 'alert(1)');
    list.append(d);
    list.children[0].remove(); // b
    box.querySelector('.mc-msg').firstChild.data = 'Copy 2 files to:';
    box.querySelector('#to').setAttribute('title', 'target');
    box.querySelector('h2').textContent = 'Copy 2 files';
    await tick();
    s.flush();
    w.sync();
    const want = box.innerHTML.replace(' onclick="alert(1)"', '');
    expect(w.copy.box.innerHTML).toBe(want);
    expect(w.copy.box.querySelector('.list p:nth-child(2)')).toBe(copyA); // the same element, moved
    expect(calls.some((c) => c.m === 'SetTitle' && c.args[1] === 'Copy 2 files')).toBe(true);
    s.close();
  });

  test('values set by code (no mutation) reach the copy', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    box.querySelector('#to').value = '/home';
    box.querySelector('#verify').checked = true;
    box.querySelector('#mode').selectedIndex = 2;
    box.querySelector('#note').value = 'changed';
    await tick();
    w.sync();
    const $ = (q) => w.copy.box.querySelector(q);
    expect($('#to').value).toBe('/home');
    expect($('#verify').checked).toBe(true);
    expect($('#mode').selectedIndex).toBe(2);
    expect($('#note').value).toBe('changed');
    s.close();
  });

  test('patches that arrive out of order are applied in order', () => {
    const got = [];
    const q = new Sequencer((m) => got.push(m.n));
    q.push(2, { n: 2 });
    q.push(3, { n: 3 });
    expect(got).toEqual([]);
    q.push(1, { n: 1 });
    q.push(1, { n: 'again' }); // (an old one is dropped)
    expect(got).toEqual([1, 2, 3]);
  });
});

describe('mirror: what the user does in the window happens to the real dialog', () => {
  test('typing, choosing and ticking are replayed with their events and not echoed back', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const seen = [];
    box.addEventListener('input', (e) => seen.push(`input:${e.target.id}`));
    box.addEventListener('change', (e) => seen.push(`change:${e.target.id}`));
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    const id = (q) => w.copy.idOf(w.copy.box.querySelector(q));
    const before = pushesFor(w.name).length;
    w.input({ t: 'input', i: id('#to'), p: { v: '/var' } });
    w.input({ t: 'input', i: id('#n'), p: { v: '7' } });
    w.input({ t: 'input', i: id('#note'), p: { v: 'line 1\nline 2' } });
    w.input({ t: 'change', i: id('#mode'), p: { s: 1 } });
    expect(box.querySelector('#to').value).toBe('/var');
    expect(box.querySelector('#n').value).toBe('7');
    expect(box.querySelector('#note').value).toBe('line 1\nline 2');
    expect(box.querySelector('#mode').selectedIndex).toBe(1);
    expect(seen).toEqual(['input:to', 'input:n', 'input:note', 'change:mode']);
    await tick();
    s.flush();
    const echoed = pushesFor(w.name).slice(before).flatMap((m) => m.ops).filter((o) => o.o === 'p');
    expect(echoed).toEqual([]);
    // a checkbox goes as a click: the real one toggles itself and its value is sent back to the copy
    w.input({ t: 'mouse', type: 'click', i: id('#verify') });
    expect(box.querySelector('#verify').checked).toBe(true);
    expect(seen.at(-1)).toBe('change:verify');
    w.sync();
    expect(w.copy.box.querySelector('#verify').checked).toBe(true);
    s.close();
  });

  test('a click on a button runs the feature code in the owning page', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const clicked = vi.fn();
    box.querySelector('.primary').addEventListener('click', clicked);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    w.input({ t: 'mouse', type: 'click', i: w.copy.idOf(w.copy.box.querySelector('.primary')), x: 10, y: 10 });
    expect(clicked).toHaveBeenCalledTimes(1);
    s.close();
  });

  test('keys: Esc goes to the dialog\'s key handler, Enter submits through the form', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const keys = [];
    const onKey = (ev) => keys.push(ev.key);
    window.addEventListener('keydown', onKey, true);
    const submitted = vi.fn((e) => e.preventDefault());
    box.addEventListener('submit', submitted);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    const id = (q) => w.copy.idOf(w.copy.box.querySelector(q));
    w.input({ t: 'key', type: 'keydown', key: 'Escape', code: 'Escape', i: id('#to') });
    w.input({ t: 'key', type: 'keydown', key: 'Enter', code: 'Enter', i: id('#to') });
    expect(keys).toEqual(['Escape', 'Enter']);
    expect(submitted).toHaveBeenCalledTimes(1);
    // Enter in a text area is a new line, not a submit
    w.input({ t: 'key', type: 'keydown', key: 'Enter', code: 'Enter', i: id('#note') });
    expect(submitted).toHaveBeenCalledTimes(1);
    window.removeEventListener('keydown', onKey, true);
    s.close();
  });

  test('focus and scrolling go both ways', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    const id = (q) => w.copy.idOf(w.copy.box.querySelector(q));
    w.input({ t: 'focus', i: id('#note') });
    expect(document.activeElement).toBe(box.querySelector('#note'));
    w.input({ t: 'scroll', i: id('.list'), top: 30, left: 0 });
    expect(box.querySelector('.list').scrollTop).toBe(30);
    // the page moves the focus itself: the copy follows
    box.querySelector('#to').focus();
    await tick();
    w.sync();
    expect(w.doc.activeElement).toBe(w.copy.box.querySelector('#to'));
    s.close();
  });

  test('closing the window cancels the dialog', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    handlers['dlg-closed'](s.name);
    expect(entry.cancel).toHaveBeenCalledTimes(1);
    s.close();
  });

  test('the window\'s size is given to the hidden dialog once the user resizes it', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    w.input({ t: 'resize', w: 700, h: 500 });
    expect(box.classList.contains('mc-mirror-fixed')).toBe(true);
    expect(box.style.getPropertyValue('--mc-mirror-w')).toBe('700px');
    s.close();
  });
});

describe('mirror: popups, look, nested dialogs', () => {
  test('a popup list opened from the dialog is drawn in its window as a layer', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    const pop = document.createElement('div');
    pop.className = 'mc-popup';
    pop.innerHTML = '<div class="it on" data-i="0">One</div><div class="it" data-i="1">Two</div>';
    document.body.append(pop);
    await tick();
    w.sync();
    expect(w.copy.layers.size).toBe(1);
    const layer = [...w.copy.layers.values()][0];
    expect(layer.textContent).toBe('OneTwo');
    const chosen = vi.fn();
    pop.children[1].addEventListener('click', chosen);
    w.input({ t: 'mouse', type: 'click', i: w.copy.idOf(layer.querySelector('[data-i="1"]')) });
    expect(chosen).toHaveBeenCalled();
    pop.remove();
    await tick();
    w.sync();
    expect(w.copy.layers.size).toBe(0);
    expect(w.doc.querySelector('.mc-window-layer')).toBe(null);
    s.close();
  });

  test('theme and live colour changes reach the window', async () => {
    const { back, box, entry } = makeDialog(FORM);
    const s = mirrorDialog(entry, back, box, 0);
    await tick();
    const w = win();
    document.documentElement.dataset.theme = 'dark';
    const st = document.createElement('style');
    st.id = 'mc-config-colors';
    st.textContent = ':root { --bg: #123; }';
    document.head.append(st);
    await tick();
    w.sync();
    expect(w.doc.documentElement.dataset.theme).toBe('dark');
    expect(w.doc.head.querySelector('style[data-mirror="mc-config-colors"]').textContent).toBe(':root { --bg: #123; }');
    st.textContent = ':root { --bg: #456; }';
    await tick();
    w.sync();
    expect(w.doc.head.querySelector('style[data-mirror="mc-config-colors"]').textContent).toBe(':root { --bg: #456; }');
    st.remove();
    delete document.documentElement.dataset.theme;
    s.close();
  });

  test('modal(): a dialog goes to a window, a nested one to another, control mode still answers the real one', async () => {
    const order = [];
    const outer = modal((b, done) => {
      b.innerHTML = '<h2>Options</h2><label><input type="checkbox" id="Display.ShowHidden"> Hidden</label><button type="button" class="primary">OK</button>';
      b.querySelector('#Display\\.ShowHidden').addEventListener('change', () => order.push('live-apply'));
      b.querySelector('.primary').onclick = () => done('ok');
    }, null, { kind: 'options' });
    await tick();
    expect(currentDialog().kind).toBe('options');
    expect(currentDialog().window.name).toBe('dialog-' + calls.filter((c) => c.m === 'Open').length);
    const w1 = win();
    // Options live-apply through the window: the click toggles the real checkbox, whose change handler runs
    w1.input({ t: 'mouse', type: 'click', i: w1.copy.idOf(w1.copy.box.querySelector('input')) });
    expect(order).toEqual(['live-apply']);
    const inner = modal((b, done) => {
      b.innerHTML = '<h2>Sure?</h2><button type="button" class="primary">Yes</button>';
      b.querySelector('.primary').onclick = () => done('yes');
    }, 'no', { kind: 'confirm' });
    await tick();
    const w2 = win();
    expect(w2.name).not.toBe(w1.name);
    expect(currentDialog().kind).toBe('confirm');
    w2.input({ t: 'mouse', type: 'click', i: w2.copy.idOf(w2.copy.box.querySelector('.primary')) });
    expect(await inner).toBe('yes');
    expect(calls.some((c) => c.m === 'Close' && c.args[0] === w2.name)).toBe(true);
    answerDialog('OK'); // control mode on the real dialog
    expect(await outer).toBe('ok');
    expect(calls.some((c) => c.m === 'Close' && c.args[0] === w1.name)).toBe(true);
  });

  test('modal(): Esc in the window cancels; a floating box over a dialog window becomes a layer of it', async () => {
    const p = modal((b) => { b.innerHTML = '<h2>Find</h2><input id="q">'; }, 'cancelled', { kind: 'find' });
    await tick();
    const w = win();
    const opens = calls.filter((c) => c.m === 'Open').length;
    const menu = modal((b, done) => {
      b.style.position = 'fixed';
      b.innerHTML = '<button type="button">Item</button>';
      b.querySelector('button').onclick = () => done('item');
    }, null, { kind: 'menu' });
    await new Promise((r) => requestAnimationFrame(r));
    await tick();
    expect(calls.filter((c) => c.m === 'Open').length).toBe(opens); // no window of its own
    w.sync();
    expect(w.copy.layers.size).toBe(1);
    const item = [...w.copy.layers.values()][0].querySelector('button');
    w.input({ t: 'mouse', type: 'click', i: w.copy.idOf(item) });
    expect(await menu).toBe('item');
    w.input({ t: 'key', type: 'keydown', key: 'Escape', code: 'Escape', i: w.copy.idOf(w.copy.box.querySelector('#q')) });
    expect(await p).toBe('cancelled');
  });

  test('modal(): the kinds in PAGE_ONLY_KINDS and in the setting dialogsInPage stay in the page; dialogsInWindows off too', async () => {
    const before = calls.filter((c) => c.m === 'Open').length;
    let close;
    const a = modal((b, done) => { b.innerHTML = '<h2>Lister</h2>'; close = done; }, null, { kind: 'lister' });
    await tick();
    expect(currentDialog().window).toBe(null);
    close(1); await a;
    handlers.setting('dialogsInPage', ['about']);
    const b = modal((x, done) => { x.innerHTML = '<h2>About</h2>'; close = done; }, null, { kind: 'about' });
    await tick();
    expect(currentDialog().window).toBe(null);
    close(1); await b;
    handlers.setting('dialogsInWindows', false);
    const c = modal((x, done) => { x.innerHTML = '<h2>Any</h2>'; close = done; }, null, { kind: 'any' });
    await tick();
    expect(currentDialog().window).toBe(null);
    expect(document.querySelector('.mc-box').classList.contains('mc-draggable')).toBe(true);
    close(1); await c;
    expect(calls.filter((c2) => c2.m === 'Open').length).toBe(before);
    handlers.setting('dialogsInWindows', null);
    handlers.setting('dialogsInPage', null);
  });

  test('modal(): no dialog windows (an old Go side, the browser preview): the dialog stays in the page', async () => {
    const keep = openReply;
    openReply = () => { throw new Error('DialogWindows.Open is not available (is the Go feature registered?)'); };
    let close;
    const p = modal((b, done) => { b.innerHTML = '<h2>Copy</h2>'; close = done; }, null, { kind: 'copy' });
    await tick();
    await tick();
    const box = document.querySelector('.mc-box');
    expect(box.classList.contains('mc-mirror-box')).toBe(false);
    expect(box.classList.contains('mc-draggable')).toBe(true);
    expect(currentDialog().window).toBe(null);
    close(1); await p;
    expect(mirrorAvailable()).toBe(false); // (and later dialogs do not try again)
    openReply = keep;
  });
});
