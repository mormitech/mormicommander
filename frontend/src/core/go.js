// The Go side, as the page sees it. Wails puts every bound object at window.go.main.<Type> (see features.go); calling
// go.FS.List('left', '/tmp') is window.go.main.FS.List(...). The generated frontend/wailsjs folder is not used, so
// features that add Go objects never fight over generated files.
//
// on(event, fn) listens to events from Go (runtime.EventsEmit); it returns a function that stops listening.

const missing = (obj, m) => () => Promise.reject(new Error(`${obj}.${m} is not available (is the Go feature registered?)`));

export const go = new Proxy({}, {
  get: (_, obj) => new Proxy({}, {
    get: (_, m) => {
      const o = globalThis.window?.go?.main?.[obj];
      return o && typeof o[m] === 'function' ? o[m].bind(o) : missing(obj, m);
    },
  }),
});

export function on(event, fn) {
  const rt = globalThis.window?.runtime;
  if (!rt) return () => {};
  return rt.EventsOn(event, fn);
}

// errText turns what Go rejected with into a sentence for the UI.
export function errText(e) {
  const t = String(e?.message ?? e ?? '').replace(/^Error:\s*/, '');
  return t.charAt(0).toUpperCase() + t.slice(1);
}
