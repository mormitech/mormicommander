// Work in progress, for control mode: every command run and every folder read is tracked here, so a script can wait
// until the app is idle (the effect of a key press is on the screen) before it reads the state or takes a screenshot.
//
// A command that waits for the user (a dialog, a popup list, the overwrite question) is not work in progress while it
// waits: dialogs.js and popup.js wrap the wait in waitForUser(), so a script sees the dialog at once and can answer it,
// instead of waiting for an idle that only comes after the answer.

let count = 0; // tracked promises that have not settled
let asking = 0; // of those, how many are waiting for the user right now

// frames waits for two painted frames, or 60 ms when the window is hidden (a hidden or covered window gets no animation
// frames on macOS, and a script must not hang on that).
function frames() {
  return new Promise((resolve) => {
    let done = false;
    const end = () => { if (!done) { done = true; resolve(); } };
    requestAnimationFrame(() => requestAnimationFrame(end));
    setTimeout(end, 60);
  });
}
let waiters = [];

const working = () => count - asking > 0;

function wake() {
  if (working()) return;
  const w = waiters;
  waiters = [];
  // two frames: the change is painted, not just in the DOM
  if (w.length) frames().then(() => w.forEach((f) => f()));
}

export function track(promise) {
  count++;
  const done = () => { count--; wake(); };
  Promise.resolve(promise).then(done, done);
  return promise;
}

// waitForUser marks the time until promise settles as waiting for the user, not as work (see above).
export function waitForUser(promise) {
  asking++;
  wake();
  const done = () => { asking--; };
  Promise.resolve(promise).then(done, done);
  return promise;
}

export const isBusy = () => working();

// idle resolves once nothing is in progress (or after timeout ms, so a stuck job can't hang a script).
export function idle(timeout = 15000) {
  if (!working()) return frames();
  return new Promise((resolve) => {
    const t = setTimeout(resolve, timeout);
    waiters.push(() => { clearTimeout(t); resolve(); });
  });
}
