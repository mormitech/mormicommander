// The zero-network test, Lister part (docs/privacy.md). Viewing a Markdown or HTML file must never contact a server, even
// when the file points at pictures, style sheets, fonts or frames on the web: the Lister turns Markdown into HTML
// (viewer/markdown.js) and then cleans every HTML page (viewer/sanitize.js) before drawing it. This test runs that same
// path on files full of remote addresses and checks that nothing in the result would make the web view load anything
// from another machine. Links stay (they show where they point; a click opens the browser, and only on a click).
//
// The DOM here is happy-dom, set up by hand so the test itself never goes on the network: unlike a browser's
// DOMParser, happy-dom would fetch a <link rel="stylesheet"> it parses, so file loading is switched off and a fetch
// interceptor answers every request with an empty response instead of sending it.
import { afterAll, describe, expect, test } from 'vitest';
import { Window } from 'happy-dom';

const offline = new Window({
  url: 'http://localhost/',
  settings: {
    disableJavaScriptFileLoading: true,
    disableJavaScriptEvaluation: true,
    disableCSSFileLoading: true,
    disableIframePageLoading: true,
    handleDisabledFileLoadingAsSuccess: true,
    navigation: { disableMainFrameNavigation: true, disableChildFrameNavigation: true, disableChildPageNavigation: true },
    fetch: {
      interceptor: {
        beforeAsyncRequest: async ({ window }) => new window.Response(''),
        beforeSyncRequest: () => ({ status: 200, statusText: 'OK', ok: true, url: 'http://localhost/', redirected: false, headers: new offline.Headers(), body: Buffer.from('') }),
      },
    },
  },
});
globalThis.DOMParser = offline.DOMParser;
globalThis.document = offline.document;
const { markdownToHTML } = await import('../src/features/viewer/markdown.js');
const { sanitizeHTML, cleanCSS } = await import('../src/features/viewer/sanitize.js');

// remoteLoads lists what in cleaned output would load something from the network when drawn: any address attribute
// other than a link's href, and any url(...) or @import in styles, that is not inline data or an in-page #anchor.
function remoteLoads({ css, html }) {
  const hits = [];
  const box = document.createElement('div');
  box.innerHTML = html;
  for (const el of box.querySelectorAll('*')) {
    for (const a of el.attributes) {
      const n = a.name.toLowerCase();
      const v = a.value.trim();
      if (n === 'href' && el.localName === 'a') continue; // a link, followed only on a click (outside the view)
      if (n === 'style') { hits.push(...cssLoads(v).map((x) => `${el.localName} style: ${x}`)); continue; }
      if (['src', 'srcset', 'href', 'poster', 'background', 'data', 'action', 'formaction', 'xlink:href', 'ping', 'lowsrc', 'dynsrc'].includes(n)) {
        if (v.startsWith('#') || /^data:image\//i.test(v)) continue;
        hits.push(`${el.localName} ${n}=${v}`);
      }
    }
    if (['script', 'iframe', 'frame', 'object', 'embed', 'link', 'meta', 'base', 'video', 'audio', 'source', 'portal'].includes(el.localName)) {
      hits.push(`<${el.localName}>`);
    }
  }
  hits.push(...cssLoads(css));
  return hits;
}

function cssLoads(css) {
  const hits = [];
  for (const m of String(css).matchAll(/url\(\s*(['"]?)(.*?)\1\s*\)/gi)) if (!/^data:image\//i.test(m[2].trim())) hits.push(m[0]);
  for (const m of String(css).matchAll(/@import[^;]*/gi)) hits.push(m[0]);
  return hits;
}

afterAll(() => offline.happyDOM.close());

const viewMarkdown = (md) => sanitizeHTML(markdownToHTML(md));

describe('viewing a file never contacts a server', () => {
  test('Markdown with remote images', () => {
    const md = [
      '# Read me',
      '',
      '![logo](https://example.com/logo.png)',
      '![tracker](http://tracker.example.net/pixel.gif?id=123 "pixel")',
      '![protocol-relative](//cdn.example.org/a.png)',
      '![local](images/local.png)',
      '![inline](data:image/png;base64,iVBORw0KGgo=)',
      '',
      'See [the site](https://example.com/) and <img src="https://example.com/raw.png"> inline.',
      '',
      '<iframe src="https://example.com/embed"></iframe>',
    ].join('\n');
    const out = viewMarkdown(md);
    expect(remoteLoads(out)).toEqual([]);
    expect(out.html).toContain('href="https://example.com/"'); // the link stays, to be clicked
    expect(out.html).toContain('data:image/png;base64'); // inline pictures still show
  });

  test('HTML with remote styles, fonts, pictures, frames and scripts', () => {
    const page = `<!doctype html><html><head>
      <link rel="stylesheet" href="https://cdn.example.com/site.css">
      <link rel="preconnect" href="https://fonts.example.com">
      <meta http-equiv="refresh" content="0; url=https://example.com/">
      <base href="https://example.com/">
      <style>@import url("https://example.com/more.css"); @font-face { src: url(https://fonts.example.com/f.woff2) }
        body { background: url('https://example.com/bg.jpg') } .ok { background: url(data:image/png;base64,AAAA) }</style>
      <script src="https://example.com/track.js"></script>
    </head><body background="https://example.com/b.gif">
      <img src="https://example.com/a.png" srcset="https://example.com/a2.png 2x" onerror="fetch('https://x')">
      <picture><source srcset="https://example.com/p.webp"><img src="//example.com/p.png"></picture>
      <video poster="https://example.com/poster.jpg" src="https://example.com/v.mp4"></video>
      <iframe src="https://example.com/frame"></iframe><object data="https://example.com/o.swf"></object>
      <form action="https://example.com/send"><button formaction="https://example.com/b">Go</button></form>
      <a href="https://example.com/link" ping="https://example.com/ping">a link</a>
      <div style="background-image: url(https://example.com/div.png)">styled</div>
      <svg><image href="https://example.com/s.png"></image><use xlink:href="https://example.com/s.svg#x"></use></svg>
      <a href="#top">top</a>
    </body></html>`;
    expect(remoteLoads(sanitizeHTML(page))).toEqual([]);
  });

  test('cleanCSS keeps inline data and drops everything remote', () => {
    expect(cssLoads(cleanCSS('a{background:url(https://e.com/x.png)} @import "https://e.com/y.css"; b{background:url(data:image/gif;base64,R0)}'))).toEqual([]);
    expect(cleanCSS('b{background:url(data:image/gif;base64,R0)}')).toContain('data:image/gif');
  });

  test('the check itself catches remote loads', () => {
    expect(remoteLoads({ css: '', html: '<img src="https://example.com/a.png">' })).not.toEqual([]);
    expect(remoteLoads({ css: 'a{background:url(https://example.com/a.png)}', html: '' })).not.toEqual([]);
    expect(remoteLoads({ css: '', html: '<div style="background:url(//e.com/a.png)"></div>' })).not.toEqual([]);
    expect(remoteLoads({ css: '', html: '<a href="https://example.com/">x</a>' })).toEqual([]);
  });
});
