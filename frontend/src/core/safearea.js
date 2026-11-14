// The rounded corners of a macOS window cut into whatever touches the bottom edge (the command line, the function key
// buttons). While the window is not full screen the page keeps a strip of room at the bottom (style.css,
// html[data-platform="darwin"]:not(.mc-fullscreen)); in full screen there are no rounded corners and no strip.
//
// Full screen is told from the size: the page then fills the whole screen.

export const FULLSCREEN_CLASS = 'mc-fullscreen';

// isFullscreen says whether a window of inner size w×h fills a screen of size sw×sh (a pixel of slack for rounding).
export function isFullscreen(w, h, sw, sh) {
  return Math.abs(w - sw) <= 1 && Math.abs(h - sh) <= 1;
}

export function watchFullscreen(win = window) {
  const update = () => {
    const s = win.screen;
    win.document.documentElement.classList.toggle(FULLSCREEN_CLASS, !!s && isFullscreen(win.innerWidth, win.innerHeight, s.width, s.height));
  };
  win.addEventListener('resize', update);
  update();
}
