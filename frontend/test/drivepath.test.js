// Where a drive button takes a panel (core/app.js drivePath, issue #125): the button of the drive the panel is already
// on goes to that drive's root, and only a different drive that the other panel is on takes the other panel's folder.
import { describe, expect, test, vi } from 'vitest';

vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, { get: () => new Proxy({}, { get: () => () => Promise.resolve(null) }) });
  return { go, on: () => () => {}, errText: String };
});

const { app } = await import('../src/core/app.js');

const hd = { name: 'Macintosh HD', path: '/', kind: 'disk' };
const home = { name: 'Home', path: '/Users/me', kind: 'home' };
const usb = { name: 'USB', path: '/Volumes/USB', kind: 'disk' };

// panels builds two linked panels in the given folders.
function panels(left, right) {
  const l = { path: left }, r = { path: right };
  l.other = r; r.other = l;
  return { l, r };
}

describe('drivePath', () => {
  app.platform = 'darwin';
  app.volumes = [hd, usb, home];

  test('own drive button goes to the drive root, not the other panel folder', () => {
    const { l, r } = panels('/private/tmp/x/A', '/');
    expect(app.drivePath(r, hd)).toBe('/');
    const b = panels('/private/tmp/x/A', '/private/tmp/x/B');
    expect(app.drivePath(b.l, hd)).toBe('/');
    expect(app.drivePath(l, hd)).toBe('/');
  });

  test('own Home button goes to home while both panels are inside it', () => {
    const { r } = panels('/Users/me/Downloads', '/Users/me/Documents');
    expect(app.drivePath(r, home)).toBe('/Users/me');
  });

  test('a different drive the other panel is on still takes its folder', () => {
    const { r } = panels('/Users/me/Downloads', '/Volumes/USB/stuff');
    expect(app.drivePath(r, home)).toBe('/Users/me/Downloads');
    const b = panels('/private/tmp/x/A', '/Users/me/Documents');
    expect(app.drivePath(b.r, hd)).toBe('/private/tmp/x/A');
  });

  test('a drive neither panel is on goes to its root', () => {
    const { r } = panels('/private/tmp', '/Users/me');
    expect(app.drivePath(r, usb)).toBe('/Volumes/USB');
  });
});
