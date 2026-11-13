// The Go side, as the page sees it, and the only file that knows the shell is Wails (v3, through @wailsio/runtime).
// Every bound object is a v3 service (main.go, features.go); calling go.FS.List('left', '/tmp') calls the Go method
// main.FS.List by name, so no generated binding files are needed and features that add Go objects never fight over
// them. The same objects are at window.go.main.<Type>.<Method> as before (handy in the developer console).
//
// on(event, fn) listens to events from Go (App.emit); fn gets the event's values as its arguments, and on returns a
// function that stops listening. onFileDrop(fn) gets files dropped from Finder / Explorer as fn(x, y, paths), and
// setClipboardText(text) puts text on the system's clipboard (true when it worked).

import { Call, Clipboard, Events } from '@wailsio/runtime';

const unknown = /unknown bound method name/i;

// call runs main.<obj>.<m>(...args) and turns "unknown method" into the sentence the features have always seen.
function call(obj, m, args) {
  return Call.ByName(`main.${obj}.${m}`, ...args).catch((e) => {
    if (unknown.test(String(e?.message ?? e))) throw new Error(`${obj}.${m} is not available (is the Go feature registered?)`);
    throw e;
  });
}

export const go = new Proxy({}, {
  get: (_, obj) => new Proxy({}, {
    get: (_, m) => (typeof m === 'string' ? (...args) => call(obj, m, args) : undefined),
  }),
});

if (globalThis.window) {
  window.go = window.go || {};
  window.go.main = go;
  // Wails v2 sent a click on a web link to the system's browser by itself; v3 would load the page in the window (and
  // hand it the bound methods). So links to other sites (the About box, the help) open in the browser from here.
  document.addEventListener('click', (ev) => {
    const a = ev.target?.closest?.('a[href]');
    if (!a || ev.defaultPrevented) return;
    const url = new URL(a.getAttribute('href'), location.href);
    if (url.origin === location.origin || !/^(https?|mailto):$/.test(url.protocol)) return;
    ev.preventDefault();
    go.App.OpenURL(url.href);
  });
}

export function on(event, fn) {
  if (!globalThis.window) return () => {};
  return Events.On(event, (ev) => {
    const d = ev?.data;
    fn(...(Array.isArray(d) ? d : d === undefined || d === null ? [] : [d]));
  });
}

// onFileDrop listens to files dropped from other apps. Wails only reports drops on elements marked as drop targets, so
// the whole page is one; the "filedrop" event comes from window.go (Go), with the point and the paths.
export function onFileDrop(fn) {
  if (!globalThis.document) return () => {};
  document.documentElement.setAttribute('data-file-drop-target', '');
  return on('filedrop', (x, y, paths) => fn(x, y, paths || []));
}

export async function setClipboardText(text) {
  try { await Clipboard.SetText(text); return true; } catch { return false; }
}

// errText turns what Go rejected with into a sentence for the UI.
export function errText(e) {
  const t = String(e?.message ?? e ?? '').replace(/^Error:\s*/, '');
  return t.charAt(0).toUpperCase() + t.slice(1);
}
