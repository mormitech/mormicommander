// Makes an HTML file safe to draw inside the Lister (rendered HTML view, F-571): no scripts, no plugins, no frames, no
// forms that could send anything, and nothing loaded from the network. The file is parsed with DOMParser (which never
// runs scripts), then cleaned element by element with an allowlist of tags; every event attribute goes, and addresses
// survive only when they point inside the page (#name) or carry their data inline (data:image/...). Styles stay, minus
// @import and url(...) of anything but inline data. The result is drawn into a shadow root, so its styles can't touch
// the app.

const DROP = new Set(['script', 'noscript', 'iframe', 'frame', 'frameset', 'object', 'embed', 'applet', 'link', 'meta', 'base',
  'template', 'portal', 'audio', 'video', 'source', 'track', 'canvas', 'dialog']);
const UNWRAP = new Set(['form', 'html', 'body', 'head']);
const URL_ATTRS = new Set(['href', 'src', 'srcset', 'action', 'formaction', 'poster', 'background', 'xlink:href', 'cite', 'longdesc', 'data', 'ping', 'lowsrc', 'dynsrc']);

const okData = (u) => /^data:image\/(png|jpe?g|gif|webp|bmp|svg\+xml|avif|x-icon)[;,]/i.test(u.trim());

// cleanCSS removes what could load something from the network.
export function cleanCSS(css) {
  return String(css)
    .replace(/@import[^;]*;?/gi, '')
    .replace(/url\(\s*(['"]?)(.*?)\1\s*\)/gi, (m, q, u) => (okData(u) ? m : 'none'))
    .replace(/expression\s*\(/gi, '(')
    .replace(/-moz-binding[^;]*;?/gi, '')
    .replace(/behavior\s*:[^;]*;?/gi, '');
}

function clean(el) {
  for (const child of [...el.children]) {
    const tag = child.localName;
    if (DROP.has(tag)) { child.remove(); continue; }
    if (tag === 'input' || tag === 'button' || tag === 'select' || tag === 'textarea') child.setAttribute('disabled', '');
    for (const a of [...child.attributes]) {
      const n = a.name.toLowerCase();
      if (n.startsWith('on')) { child.removeAttribute(a.name); continue; }
      if (n === 'style') { child.setAttribute('style', cleanCSS(a.value)); continue; }
      if (n === 'srcdoc' || n === 'formtarget' || n === 'target') { child.removeAttribute(a.name); continue; }
      if (URL_ATTRS.has(n)) {
        const v = a.value.trim();
        const inPage = (n === 'href' || n === 'xlink:href') && v.startsWith('#');
        const keepLink = n === 'href' && tag === 'a'; // links are kept to show where they point; clicks are caught by the viewer
        if (!(inPage || keepLink || (n === 'src' && okData(v)))) child.removeAttribute(a.name);
      }
    }
    if (tag === 'style') child.textContent = cleanCSS(child.textContent);
    clean(child);
    if (UNWRAP.has(tag)) child.replaceWith(...child.childNodes);
  }
}

// sanitizeHTML returns {css, html}: the document's style sheets and its body, both cleaned.
export function sanitizeHTML(source) {
  const doc = new DOMParser().parseFromString(String(source), 'text/html');
  const css = [...doc.querySelectorAll('style')].map((s) => cleanCSS(s.textContent)).join('\n');
  doc.querySelectorAll('style').forEach((s) => s.remove());
  clean(doc.body);
  return { css, html: doc.body.innerHTML };
}
