// Printing from the Lister (Ctrl+P, File > Print…, F-581 / F-843): what is printed and the question asked first. Pure, so
// it is unit-tested without a page; the view (viewer/view.js) asks and calls Go (feature_system_print.go).

// printerNote says where a print job goes; the system's print path has no dialog of its own (also used by Ctrl+F9).
export const printerNote = (app) => (app?.platform === 'windows'
  ? 'Windows prints it with the app that belongs to the file type.'
  : 'It goes to the default printer (lp); choose another default printer in the system settings.');

// printPlan: with text selected in a text mode ([start, end) byte offsets) only that text is printed, as shown;
// otherwise the file itself.
export function printPlan({ selection = null, textMode = false, name = '' } = {}) {
  if (selection && textMode && selection[1] > selection[0]) {
    return { kind: 'text', start: selection[0], end: selection[1], title: `${name} (selection)`, question: `Print the selected text of ${name}?` };
  }
  return { kind: 'file', question: `Print ${name}?` };
}
