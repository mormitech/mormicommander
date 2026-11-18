// The command line's output window (F-293). With the option Misc > "Show command line output in a window instead of a
// terminal" on, a console program or shell command typed on the command line runs without a terminal and what it
// prints shows here: scrollable text that can be selected and copied, a Copy button, Stop (the program gets no input,
// so one that waits for it, or never ends, is stopped here) and Close (which stops it too). Go caps the output at 1 MB
// and stops the program there (internal/launch/capture.go). GUI programs, documents, folders and cm_* names go on as
// before (features/system.js asks SystemLaunch.LineCaptures first).

import { modal, esc } from '../core/dialogs.js';
import { go, errText } from '../core/go.js';
import { copyText } from './tools/common.js';
import { outputStatus } from './cmdline-output-lib.js';

let seq = 0;

// showOutputWindow runs line in dir and shows its output; it resolves when the window is closed.
export function showOutputWindow(app, line, dir) {
  const id = `cmdout-${Date.now()}-${++seq}`;
  let result = null;
  let running = true;
  return modal((box, done) => {
    box.classList.add('mc-cmdout');
    box.innerHTML = `<h2>${esc(line)}</h2>
      <pre class="mc-cmdout-text" tabindex="0" aria-label="Output"></pre>
      <p class="mc-note mc-cmdout-status"></p>
      <div class="mc-btns"><button type="button" class="mc-cmdout-copy">Copy</button><button type="button" class="mc-cmdout-stop">Stop</button><button type="submit" class="primary" autofocus>Close</button></div>`;
    const text = box.querySelector('.mc-cmdout-text');
    const status = box.querySelector('.mc-cmdout-status');
    const stopBtn = box.querySelector('.mc-cmdout-stop');
    const show = () => {
      status.textContent = outputStatus(result);
      stopBtn.disabled = !running;
    };
    show();
    const stop = () => { if (running) go.SystemLaunch.StopCapture(id).catch(() => {}); };
    stopBtn.onclick = stop;
    box.querySelector('.mc-cmdout-copy').onclick = async () => {
      const ok = await copyText(text.textContent);
      app.flash(ok ? 'Output copied.' : 'The output could not be copied.', ok ? '' : 'err');
    };
    box.onsubmit = (ev) => { ev.preventDefault(); done(); };
    go.SystemLaunch.RunLineCaptured(id, line, dir).then((r) => { result = r; }, (e) => { result = { error: errText(e) }; })
      .finally(() => {
        running = false;
        if (result?.output) text.textContent = result.output;
        show();
        text.scrollTop = text.scrollHeight;
      });
  }, undefined, { kind: 'cmdoutput' }).finally(() => {
    // closed in any way (Close, Esc, the window's close button) while it still runs: the program must not run on unseen
    if (running) go.SystemLaunch.StopCapture(id).catch(() => {});
  });
}
