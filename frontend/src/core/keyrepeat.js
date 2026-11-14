// A faster repeat for held cursor keys (option Operation.CursorRepeat, "Held Up/Down key repeat speed"; 0, the
// default, leaves the system's repeat alone). From the first repeat on, mormiCommander repeats the key's command itself
// at the chosen pace and skips the system's repeats, until the key is let go, another key is pressed or the window
// loses the keyboard.
//
// When the repeat starts: Fast waits for the system's own first repeat (its "delay until repeat"). Faster and Fastest
// also start on their own after a shorter wait (REPEAT_DELAY_MS), a little shorter for each; whichever comes first wins,
// so a system set to repeat sooner still does.

export const REPEAT_KEYS = new Set(['Up', 'Down', 'PageUp', 'PageDown', 'Shift+Up', 'Shift+Down', 'Shift+PageUp', 'Shift+PageDown']);
// the time between two steps for each speed (ms): 1 Fast, 2 Faster, 3 Fastest. The quickest system setting on a Mac
// repeats every 30 ms.
export const REPEAT_MS = [0, 30, 18, 10];
// the wait before the repeat starts on its own (ms; 0: wait for the system's first repeat)
export const REPEAT_DELAY_MS = [0, 0, 300, 230];

const level = (speed) => Math.max(0, Math.min(3, Number(speed) || 0));
export const repeatInterval = (speed) => REPEAT_MS[level(speed)] || 0;
export const repeatDelay = (speed) => REPEAT_DELAY_MS[level(speed)] || 0;

let timer = 0, held = '', heldCode = '', busy = false;
let pending = 0; // the wait of armRepeat, before the repeat starts on its own

export function stopRepeat() {
  if (timer) clearTimeout(timer);
  if (pending) clearTimeout(pending);
  timer = 0;
  pending = 0;
  held = '';
}

function start(combo, code, ms, step) {
  stopRepeat();
  held = combo;
  heldCode = code;
  const tick = async () => {
    if (held !== combo) return;
    if (!busy) { // a slow step (a big folder) is not stacked up: the next one waits for it
      busy = true;
      try { await step(combo); } catch { /* the command reports its own errors */ } finally { busy = false; }
    }
    if (held === combo) timer = setTimeout(tick, ms);
  };
  tick();
}

// armRepeat is called for the first press of a key (not a repeat), after its command ran once. With Faster or Fastest
// the repeat starts on its own after the shorter wait, unless the key goes up (or the system repeats it) first.
export function armRepeat(ev, combo, speed, step) {
  const ms = repeatInterval(speed), delay = repeatDelay(speed);
  if (!ms || !delay || ev.repeat || !REPEAT_KEYS.has(combo)) return;
  stopRepeat();
  const code = ev.code;
  heldCode = code;
  pending = setTimeout(() => { pending = 0; start(combo, code, ms, step); }, delay);
}

// takeRepeat is called for a key event the system repeats (ev.repeat) with its combo. It returns true when the event is
// taken (our own repeat runs, or already runs, for it): step(combo) runs the key's command once.
export function takeRepeat(ev, combo, speed, step) {
  const ms = repeatInterval(speed);
  if (!ms || !ev.repeat || !REPEAT_KEYS.has(combo)) return false;
  if (held === combo) return true; // the system's repeat: ours is running
  start(combo, ev.code, ms, step); // the system's first repeat came before our own wait was over (or there is none)
  return true;
}

let wired = false;
// watchRepeatEnd stops the repeat (or its wait) when the key goes up, another key goes down or the window loses the
// keyboard.
export function watchRepeatEnd(win = window) {
  if (wired) return;
  wired = true;
  win.addEventListener('keyup', (ev) => { if ((held || pending) && (ev.code === heldCode || ev.key === 'Shift')) stopRepeat(); }, true);
  win.addEventListener('keydown', (ev) => { if ((held || pending) && !ev.repeat) stopRepeat(); }, true);
  win.addEventListener('blur', stopRepeat);
  win.document?.addEventListener('visibilitychange', () => { if (win.document.visibilityState !== 'visible') stopRepeat(); });
}
