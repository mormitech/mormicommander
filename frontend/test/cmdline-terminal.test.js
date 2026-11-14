// Tests of the terminal words of the command line (features/cmdline.js, terminalWord).
import { expect, test, vi } from 'vitest';
vi.mock('../src/core/go.js', () => ({ go: {}, errText: (e) => String(e), on: () => {} }));
const { terminalWord } = await import('../src/features/cmdline.js');

test('terminal and term open a terminal everywhere; cmd only on macOS and Linux', () => {
  for (const p of ['darwin', 'linux', 'windows']) {
    expect(terminalWord('terminal', p)).toEqual({ kind: 'terminal' });
    expect(terminalWord(' Term ', p)).toEqual({ kind: 'terminal' });
  }
  expect(terminalWord('cmd', 'darwin')).toEqual({ kind: 'terminal' });
  expect(terminalWord('cmd', 'linux')).toEqual({ kind: 'terminal' });
  expect(terminalWord('cmd', 'windows')).toBe(null); // Windows runs cmd.exe itself, as before
});

test('a shell name alone runs in a terminal on macOS and Linux; with arguments it does not', () => {
  expect(terminalWord('zsh', 'darwin')).toEqual({ kind: 'shell' });
  expect(terminalWord('bash', 'linux')).toEqual({ kind: 'shell' });
  expect(terminalWord('bash', 'windows')).toBe(null);
  expect(terminalWord('bash -c ls', 'linux')).toBe(null);
  expect(terminalWord('cmd /c dir', 'darwin')).toBe(null);
  expect(terminalWord('ls', 'darwin')).toBe(null);
});
