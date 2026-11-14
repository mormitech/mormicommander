// Middle-button panning of a scrolling element (the file list of each panel): while the middle mouse button is held,
// moving the mouse drags the content with it, as a hand would (mostly up and down; across too when the list is wider
// than the panel). The pointer shows a "grab" hand while the button is down. The same on every system: the browser's
// own middle-click autoscroll (Windows, Linux) is turned off for the element, so it never fights the panning.
//
// Pointer events with pointer capture, so the drag keeps going when the mouse leaves the list or the window.

export const PAN_CLASS = 'mc-panning'; // on <html> while a pan is going on (the cursor rule in style.css)

// attachMiddlePan makes el pannable with the middle button. Returns a function that removes the handlers.
export function attachMiddlePan(el, doc = el.ownerDocument) {
  let pan = null; // { id, x, y, left, top } while the middle button is down

  const end = () => {
    if (!pan) return;
    try { el.releasePointerCapture?.(pan.id); } catch { /* already released */ }
    pan = null;
    doc.documentElement.classList.remove(PAN_CLASS);
  };
  const down = (ev) => {
    if (ev.button !== 1 || ev.pointerType === 'touch') return;
    ev.preventDefault(); // no autoscroll, no paste of the selection (Linux)
    pan = { id: ev.pointerId, x: ev.clientX, y: ev.clientY, left: el.scrollLeft, top: el.scrollTop };
    try { el.setPointerCapture?.(ev.pointerId); } catch { /* the pointer is gone already */ }
    doc.documentElement.classList.add(PAN_CLASS);
  };
  const move = (ev) => {
    if (!pan || ev.pointerId !== pan.id) return;
    if (!(ev.buttons & 4)) { end(); return; } // the button was let go where no pointerup reached us
    el.scrollTop = pan.top - (ev.clientY - pan.y);
    el.scrollLeft = pan.left - (ev.clientX - pan.x);
  };
  const up = (ev) => { if (pan && ev.pointerId === pan.id && ev.button === 1) end(); };
  const cancel = (ev) => { if (pan && ev.pointerId === pan.id) end(); };
  // Chromium (WebView2) and WebKitGTK start autoscroll on the mousedown, so that is cancelled too
  const mdown = (ev) => { if (ev.button === 1) ev.preventDefault(); };
  const aux = (ev) => { if (ev.button === 1) ev.preventDefault(); };

  el.addEventListener('pointerdown', down);
  el.addEventListener('pointermove', move);
  el.addEventListener('pointerup', up);
  el.addEventListener('pointercancel', cancel);
  el.addEventListener('lostpointercapture', cancel);
  el.addEventListener('mousedown', mdown);
  el.addEventListener('auxclick', aux);
  return () => {
    end();
    el.removeEventListener('pointerdown', down);
    el.removeEventListener('pointermove', move);
    el.removeEventListener('pointerup', up);
    el.removeEventListener('pointercancel', cancel);
    el.removeEventListener('lostpointercapture', cancel);
    el.removeEventListener('mousedown', mdown);
    el.removeEventListener('auxclick', aux);
  };
}
