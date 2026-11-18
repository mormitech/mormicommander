// Live folder suggestions in the command line (F-296), with the option Misc > "Suggest folders while typing in the
// command line" (off by default, so typing works as before). While a path is typed as an argument ("cd Pro", "ls
// ~/Doc") or as a word with a separator, the first folder whose name starts with it is filled in after the caret,
// selected: typing on replaces it, Tab or Right takes it, Backspace removes it, and Enter runs only what was typed
// (core/cmdline.js drops the suggestion). It is the path fields' completion (core/pathcomplete.js) applied to the
// word at the caret; Tab completion of files and folders (features/cmdline-complete.js) works as before.

import { attachPathComplete } from '../core/pathcomplete.js';
import { suggestWord } from './cmdline-complete-lib.js';

export function init(app) {
  const input = document.getElementById('cmd');
  if (!input) return;
  attachPathComplete(input, () => app.cur?.path || '', {
    word: (text) => (app.config?.values?.['Misc.CmdLineSuggest'] === '1' ? suggestWord(text) : null),
  });
}
