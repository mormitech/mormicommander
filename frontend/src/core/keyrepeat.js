// A faster repeat for held cursor keys (option Operation.CursorRepeat, "Held Up/Down key repeat speed"; 0, the
// default, leaves the system's repeat alone). The system still decides when a held key starts repeating (its "delay
// until repeat"); from the first repeat on, mormiCommander repeats the key's command itself at the chosen pace and
// skips the system's repeats, until the key is let go, another key is pressed or the window loses the keyboard.

export const REPEAT_KEYS = new Set(['Up', 'Down', 'PageUp', 'PageDown', 'Shift+Up', 'Shift+Down', 'Shift+PageUp', 'Shift+PageDown']);
// the time between two steps for each speed (ms): 1 Fast, 2 Faster, 3 Fastest. The quickest system setting on a Mac
// repeats every 30 ms.
export const REPEAT_MS = [0, 30, 18, 10];

export const repeatInterval = (speed) => REPEAT_MS[Math.max(0, Math.min(3, Number(speed) || 0))] || 0;

let timer = 0, held = '', heldCode = '', busy = false;

export function stopRepeat() {
  if (timer) clearTimeout(timer);
  timer = 0;
  held = '';
}

// takeRepeat is called for a key event the system repeats (ev.repeat) with its combo. It returns true when the event is
// taken (our own repeat runs, or already runs, for it): step(combo) runs the key's command once.
export function takeRepeat(ev, combo, speed, step) {
  const ms = repeatInterval(speed);
  if (!ms || !ev.repeat || !REPEAT_KEYS.has(combo)) return false;
  if (held === combo) return true; // the system's repeat: ours is running
  stopRepeat();
  held = combo;
  const code = ev.code;
  const tick = async () => {
    if (held !== combo) return;
    if (!busy) { // a slow step (a big folder) is not stacked up: the next one waits for it
      busy = true;
      try { await step(combo); } catch { /* the command reports its own errors */ } finally { busy = false; }
    }
    if (held === combo) timer = setTimeout(tick, ms);
  };
  heldCode = code;
  tick();
  return true;
}

let wired = false;
// watchRepeatEnd stops the repeat when the key goes up, another key goes down or the window loses the keyboard.
export function watchRepeatEnd(win = window) {
  if (wired) return;
  wired = true;
  win.addEventListener('keyup', (ev) => { if (held && (ev.code === heldCode || ev.key === 'Shift')) stopRepeat(); }, true);
  win.addEventListener('keydown', (ev) => { if (held && !ev.repeat) stopRepeat(); }, true);
  win.addEventListener('blur', stopRepeat);
  win.document?.addEventListener('visibilitychange', () => { if (win.document.visibilityState !== 'visible') stopRepeat(); });
}
