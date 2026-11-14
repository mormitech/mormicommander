// The zero-network test, page half (the Go half is privacy_test.go at the repository root). The README promises that
// mormiCommander makes no network connections of its own; on the page side that means: no script asks for anything
// over the network (no fetch, XMLHttpRequest, WebSocket, EventSource or beacons), the HTML pages and styles load
// nothing from other sites (no CDNs, no web fonts), and the only web addresses in the code are links the user clicks,
// which open in the system's browser (core/go.js), to an allowed list of sites. The Wails runtime's own calls to the
// app (bound Go methods) stay inside the process and are not in src/.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative, resolve } from 'node:path';
import { expect, test } from 'vitest';

const root = resolve(import.meta.dirname, '..');

// files lists the files under dir (recursively) whose names end with one of exts.
function files(dir, exts) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...files(p, exts));
    else if (exts.some((e) => name.endsWith(e))) out.push(p);
  }
  return out;
}

// stripComments drops // and /* */ comments, so a comment that talks about fetch is not taken for a call. Good enough
// for this code base (it does not try to understand strings that contain //).
const stripComments = (s) => s.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:'"`\\])\/\/.*$/gm, '$1');

const sources = files(join(root, 'src'), ['.js']);
const styles = files(join(root, 'src'), ['.css']);
const pages = ['index.html', 'viewer.html', 'dialog.html'].map((p) => join(root, p));

// The sites a link in the app may point at (opened in the browser, only on a click).
const LINK_HOSTS = ['github.com', 'mormitech.com'];

// What would make the page itself talk to the network.
const NETWORK_CALLS = [/\bfetch\s*\(/, /\bXMLHttpRequest\b/, /\bWebSocket\b/, /\bEventSource\b/, /\bsendBeacon\b/, /\bimportScripts\b/, /\bRTCPeerConnection\b/];

function networkCallsIn(code) {
  const c = stripComments(code);
  return NETWORK_CALLS.filter((re) => re.test(c)).map((re) => re.source);
}

function remoteLoadsIn(markupOrCss) {
  const hits = [];
  for (const re of [/\b(?:src|href|srcset|poster)\s*=\s*["']?\s*(?:https?:)?\/\/[^"'\s>]+/gi, /url\(\s*["']?\s*(?:https?:)?\/\/[^)]+\)/gi, /@import\s+(?:url\()?\s*["']?(?:https?:)?\/\/[^;]+/gi]) {
    hits.push(...(markupOrCss.match(re) || []));
  }
  return hits;
}

const hostOf = (u) => { try { return new URL(u).hostname; } catch { return ''; } };
const allowedLink = (u) => LINK_HOSTS.some((h) => hostOf(u) === h || hostOf(u).endsWith('.' + h));

test('the scan sees the code', () => {
  expect(sources.length).toBeGreaterThan(50);
  expect(styles.length).toBeGreaterThan(5);
});

test('no script talks to the network', () => {
  const found = sources.flatMap((f) => networkCallsIn(readFileSync(f, 'utf8')).map((what) => `${relative(root, f)}: ${what}`));
  expect(found).toEqual([]);
});

test('pages and styles load nothing from other sites', () => {
  const found = [...pages, ...styles].flatMap((f) => remoteLoadsIn(readFileSync(f, 'utf8')).map((what) => `${relative(root, f)}: ${what}`));
  expect(found).toEqual([]);
});

test('the only web addresses are links to the allowed sites', () => {
  const found = [];
  for (const f of sources) {
    const code = stripComments(readFileSync(f, 'utf8'));
    for (const u of code.match(/https?:\/\/[^\s'"`<>)\\]+/g) || []) {
      if (u.startsWith('http://www.w3.org/')) continue; // XML namespaces (SVG), never loaded
      if (!allowedLink(u)) found.push(`${relative(root, f)}: ${u}`);
    }
  }
  expect(found).toEqual([]);
});

test('the checks catch what they are there to catch', () => {
  expect(networkCallsIn("fetch('https://example.com/ping')")).not.toEqual([]);
  expect(networkCallsIn('const x = new XMLHttpRequest();')).not.toEqual([]);
  expect(networkCallsIn('navigator.sendBeacon(u, d)')).not.toEqual([]);
  expect(networkCallsIn('// we never fetch(anything)\nconst a = 1;')).toEqual([]);
  expect(remoteLoadsIn('<script src="https://cdn.example.com/x.js"></script>')).not.toEqual([]);
  expect(remoteLoadsIn('<link href="//fonts.example.com/css" rel="stylesheet">')).not.toEqual([]);
  expect(remoteLoadsIn("body { background: url('https://example.com/a.png') }")).not.toEqual([]);
  expect(remoteLoadsIn('@import "https://example.com/a.css";')).not.toEqual([]);
  expect(remoteLoadsIn('<script type="module" src="/src/main.js"></script>')).toEqual([]);
  expect(remoteLoadsIn('a { background: url(data:image/png;base64,AAAA) }')).toEqual([]);
  expect(allowedLink('https://github.com/mormitech/mormicommander/issues')).toBe(true);
  expect(allowedLink('https://mormitech.com/support-us/')).toBe(true);
  expect(allowedLink('https://evilgithub.com/')).toBe(false);
});
