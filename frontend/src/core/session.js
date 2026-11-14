// The session saver: the folders, sorts and active panel are written a moment after a change, so a run of quick
// changes (holding Backspace, several sorts in a row) is written once. Each change is also staged on the Go side at
// once (Settings.Stage), and Go writes a staged value the page did not get to write when the app ends; flush() writes a
// waiting value now (cm_Exit calls it before quitting). Without these, changes made in the last moment before quitting
// were lost (board issue #131).
//
//   const saver = sessionSaver({ write: (v) => setSetting('session', v), stage: (v) => go.Settings.Stage('session', v) });
//   saver.schedule(() => snapshot());   saver.flush();

export function sessionSaver({ write, stage = () => {}, delay = 400 }) {
  let timer = 0;
  let snap = null;
  const fire = () => {
    timer = 0;
    const f = snap;
    snap = null;
    return f ? write(f()) : undefined;
  };
  return {
    // schedule writes snapFn() after the delay (a newer call replaces it) and stages its value now.
    schedule(snapFn) {
      snap = snapFn;
      try { Promise.resolve(stage(snapFn())).catch(() => {}); } catch { /* (the browser preview has no Go side) */ }
      clearTimeout(timer);
      timer = setTimeout(fire, delay);
    },
    // flush writes a waiting value at once; it returns the write's promise (or undefined when nothing waited).
    flush() {
      if (!timer) return undefined;
      clearTimeout(timer);
      return fire();
    },
    get pending() { return timer !== 0; },
  };
}
