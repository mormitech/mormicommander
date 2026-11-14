// jobchoices.js holds the button lists of the file job questions and the rule that picks the buttons to show: the
// engine sends the choices that fit the question (a failed move to the macOS Trash also offers "without Finder",
// issue #36), and only the buttons for those choices appear, in the order given here. Kept apart from jobs.js so the
// rule can be tested without the page.

// ERROR_BUTTONS are the buttons of the error question, [label, choice]. "Trash without Finder" and "All without
// Finder" come only with a failed move to the macOS Trash (the Finder call was refused or hung); "Delete permanently"
// with any failed move to the trash.
export const ERROR_BUTTONS = [['Retry', 'retry'], ['Skip', 'skip'], ['Skip all', 'skip_all'],
  ['Trash without Finder', 'trash_direct'], ['All without Finder', 'trash_direct_all'], ['Delete permanently', 'delete'], ['Cancel', 'cancel']];

// offeredButtons keeps the [label, choice] buttons whose choice the engine offers, in their own order.
export function offeredButtons(buttons, choices) {
  const offered = new Set(choices || []);
  return buttons.filter(([, c]) => offered.has(c));
}
