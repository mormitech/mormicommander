// Keeps progress boxes from opening on top of a question a job waits for. A job (a local file job in core/jobs.js, a
// server transfer in features/net.js, an archive job in features/vfs.js) opens its progress box a moment after it
// starts, so a small job doesn't flash one. When the engine asks something before that moment (a file is in the way,
// a folder is not empty, a file is read-only, the password of an archive), the box would open above the question,
// cover its buttons and take the keys and control mode (board issue #123). So every open question holds progress
// boxes back, and a box that is due waits until no question is open any more.
//
//   const release = holdProgress();   // while a question is on screen
//   ...; release();                   // once it is answered (calling it again does nothing)
//   const cancel = showWhenFree(openBox, 400);  // opens the box after 400 ms, or later once no question is open

let holds = 0;

// holdProgress starts a hold and returns the function that ends it.
export function holdProgress() {
  holds++;
  let ended = false;
  return () => { if (!ended) { ended = true; holds--; } };
}

// progressHeld tells whether a question is open, so a progress box must not open now.
export function progressHeld() { return holds > 0; }

// showWhenFree calls show() after delay milliseconds, or, while a question is open, as soon after that as none is
// (checked every retry milliseconds). It returns the function that cancels it (when the job ends first).
export function showWhenFree(show, delay = 400, retry = 300) {
  let timer = null;
  let cancelled = false;
  const tick = () => {
    if (cancelled) return;
    if (progressHeld()) { timer = setTimeout(tick, retry); return; }
    cancelled = true;
    show();
  };
  timer = setTimeout(tick, delay);
  return () => { cancelled = true; clearTimeout(timer); };
}
