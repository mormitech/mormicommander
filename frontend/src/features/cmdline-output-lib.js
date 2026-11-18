// The plain rules of the command line's output window (features/cmdline-output.js, F-293), kept apart so vitest can
// test them: whether a line goes to the window, and the sentence under the output that says how the program ended.

// outputWanted: the option Misc > "Show command line output in a window instead of a terminal" is on (off by default,
// so lines open in a terminal as before), and the line was run with Enter: Shift+Enter always asks for a terminal that
// stays open.
export function outputWanted(values, shift) {
  return !shift && values?.['Misc.CmdLineOutput'] === '1';
}

// outputStatus is the line under the output: still running, stopped, cut at the size cap, or the exit code.
export function outputStatus(r) {
  if (!r) return 'Running… (it gets no input; Stop ends it)';
  if (r.error) return r.error;
  const parts = [];
  if (r.stopped) parts.push('Stopped.');
  else if (r.truncated) parts.push('Stopped: it printed more than 1 MB; only the first 1 MB is shown.');
  else if (r.exitCode === 0) parts.push('Done.');
  else if (r.exitCode > 0) parts.push(`Ended with exit code ${r.exitCode}.`);
  else parts.push('Ended.');
  if (!r.output) parts.push('It printed nothing.');
  return parts.join(' ');
}
