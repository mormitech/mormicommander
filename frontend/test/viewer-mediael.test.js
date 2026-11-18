// Tests for viewer/mediael.js, the choice of element the Lister draws a media file with. The point is the safety rule
// behind the Wails iframe advisory (docs/ARCHITECTURE.md, "Web view security"): pictures, sound and film keep their
// usual elements, PDF keeps its <embed>, and any other type, above all HTML and SVG, never ends up in an <embed> where
// it could run scripts with the app's own origin.
import { describe, expect, test } from 'vitest';
import { mediaTag, EMBED_SAFE, convertible } from '../src/features/viewer/mediael.js';

describe('mediaTag', () => {
  test('keeps the elements used today', () => {
    expect(mediaTag('image', 'image/png')).toBe('img');
    expect(mediaTag('image', 'image/svg+xml')).toBe('img'); // SVG in <img> never runs scripts
    expect(mediaTag('video', 'video/mp4')).toBe('video');
    expect(mediaTag('audio', 'audio/mpeg')).toBe('audio');
    expect(mediaTag('document', 'application/pdf')).toBe('embed');
    expect(mediaTag('document', 'Application/PDF; charset=binary')).toBe('embed');
  });

  test('never embeds a type that could run scripts', () => {
    for (const mime of ['text/html', 'image/svg+xml', 'application/xhtml+xml', 'text/xml', 'application/xml',
      'application/octet-stream', '', undefined]) {
      expect(mediaTag('document', mime)).toBe('');
      expect(mediaTag(undefined, mime)).toBe('');
    }
  });

  test('the allowlist holds only PDF', () => {
    expect([...EMBED_SAFE]).toEqual(['application/pdf']);
  });
});

// HEIC, AVIF and JPEG XL pictures the web view can't draw are converted by Go (issue #25); other types never are.
describe('convertible', () => {
  test('only the formats Go converts', () => {
    for (const m of ['image/heic', 'image/HEIF', 'image/avif', 'image/jxl', 'image/avif; x=1']) expect(convertible(m)).toBe(true);
    for (const m of ['image/png', 'image/jpeg', 'image/svg+xml', 'video/mp4', '', undefined, null]) expect(convertible(m)).toBe(false);
  });
});
