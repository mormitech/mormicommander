// The keys of the mask box in the Select files / Unselect files dialog (Num+ / Num-), kept apart from the dialog so
// they can be tested without a page. Alt+Down opens or closes the list of masks used before; while the list is open,
// Up/Down move the highlight and Enter takes the highlighted mask; while it is closed, Up/Down step through the masks
// used before, like Total Commander's combo box. The dialog routes these keys through modal()'s meta.onKey, because
// modal() stops key events before they reach the elements inside it.

// maskKey answers what a key does in the mask box, or null when the key is not for it (it then types as usual).
// state: {listOpen, count (entries in the list), on (highlighted entry, -1 for none), at (history step, -1 for none),
// historyLength}. The answer is {do: 'toggle'}, {do: 'highlight', index}, {do: 'pick', index} or {do: 'step', at}.
export function maskKey(ev, state) {
  const down = ev.key === 'ArrowDown', up = ev.key === 'ArrowUp';
  if (down && ev.altKey) return { do: 'toggle' };
  if (state.listOpen && state.count > 0) {
    if (down || up) {
      const want = state.on < 0 ? 0 : state.on + (down ? 1 : -1);
      return { do: 'highlight', index: Math.max(0, Math.min(state.count - 1, want)) };
    }
    if (ev.key === 'Enter' && state.on >= 0) return { do: 'pick', index: state.on };
    return null;
  }
  if ((down || up) && state.historyLength > 0) {
    return { do: 'step', at: Math.max(0, Math.min(state.historyLength - 1, state.at + (down ? 1 : -1))) };
  }
  return null;
}
