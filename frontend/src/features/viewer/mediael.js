// Picks which HTML element the Lister uses to draw an image, sound, film or document it has read into a blob: address
// (viewer/view.js). Pictures go in <img>, which never runs scripts, even for SVG; sound and film use <audio>/<video>.
// Anything else would go in an <embed>, and that is the one place where a file's own content could become a live page
// inside the app: a blob: address belongs to the app's own origin, so an HTML or SVG document embedded that way could
// run scripts and reach the bound Go methods through the Wails runtime (the partly fixed Wails advisory
// GHSA-2p5f-2gx8-mh8m, see docs/ARCHITECTURE.md, "Web view security"). So <embed> is kept for an allowlist of document
// types that cannot carry app-origin scripts (today only PDF, the one "document" kind the Go side reports), and any
// other type gets no element at all. This changes nothing for the files the Lister shows today; it only makes sure a
// future format cannot open that door by accident.

// EMBED_SAFE lists the document MIME types that may be drawn with <embed>.
export const EMBED_SAFE = new Set(['application/pdf']);

// mediaTag returns the tag name for a media kind and MIME type ('img', 'video', 'audio' or 'embed'), or '' when the
// file must not be drawn as live content.
export function mediaTag(kind, mime) {
  if (kind === 'image') return 'img';
  if (kind === 'video') return 'video';
  if (kind === 'audio') return 'audio';
  const type = String(mime || '').split(';')[0].trim().toLowerCase();
  return EMBED_SAFE.has(type) ? 'embed' : '';
}

// convertible says whether a picture type is one Go can convert (feature_imgconv.go: HEIC, AVIF, JPEG XL) when the web
// view can't draw it, as WebView2 on Windows and WebKitGTK on Linux often can't.
export function convertible(mime) {
  return /^image\/(heic|heif|avif|jxl)$/i.test(String(mime || '').split(';')[0].trim());
}
