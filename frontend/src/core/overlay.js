// Where floating boxes (popup lists, menus at the pointer) may be placed in the page. Normally that is the whole page;
// while a dialog shows in a window of its own (core/mirror.js), a popup opened from it is drawn inside that window, so
// it has to fit the dialog's area instead. mirror.js sets the area; popup.js asks for it.

let areaFn = null;

// setPopupArea(fn): fn() → {left, top, right, bottom} in page pixels, or null for the whole page.
export function setPopupArea(fn) { areaFn = fn; }

// popupArea is the rectangle a floating box has to stay in.
export function popupArea() {
  const r = areaFn?.();
  return r || { left: 0, top: 0, right: innerWidth, bottom: innerHeight };
}
