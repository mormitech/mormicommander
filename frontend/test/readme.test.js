// The README's promises, kept true by a test: the key table between the key-table markers must match the default
// keymap on the system it names (Cmd keys on macOS, the rest everywhere), and every command it lists must be one a
// feature really registers, so the README never shows a key that does something else or is "not ready yet". It also
// checks the not-affiliated line, the links to the zero-network test and the canonical download page, and that the
// release workflow writes the checksums the download page talks about.
import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, test } from 'vitest';

globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const reg = await import('../src/core/registry.js');
const { normalizeCombo } = await import('../src/core/keys.js');

const repo = resolve(import.meta.dirname, '..', '..');
const readme = readFileSync(join(repo, 'README.md'), 'utf8');

// keyTable returns the rows of the README key table as { keys: [...], commands: [...] }. A cell holds one or more
// `code` spans separated by " / "; keys and commands pair up in order (one command may serve several keys).
function keyTable(md) {
  const m = md.match(/<!-- key-table:start -->([\s\S]*?)<!-- key-table:end -->/);
  if (!m) return [];
  const codes = (cell) => [...cell.matchAll(/`([^`]+)`/g)].map((x) => x[1]);
  return m[1].split('\n').filter((l) => l.startsWith('|') && !/^\|\s*-/.test(l)).slice(1).map((l) => {
    const cells = l.split(/(?<!\\)\|/).slice(1, -1);
    return { keys: codes(cells[0]), commands: codes(cells[2] || '') };
  });
}

// registeredCommands collects the cm_* names the features register (command({ name: 'cm_X' }) or a helper such as
// sortCmd('cm_X', ...)). Read from the source, because the features need a window to load.
function registeredCommands() {
  const names = new Set();
  const walk = (dir) => {
    for (const n of readdirSync(dir)) {
      const p = join(dir, n);
      if (statSync(p).isDirectory()) walk(p);
      else if (n.endsWith('.js')) {
        for (const x of readFileSync(p, 'utf8').matchAll(/(?:name:\s*|[A-Za-z]Cmd\(\s*)['"](cm_\w+)['"]/g)) names.add(x[1]);
      }
    }
  };
  walk(join(repo, 'frontend', 'src', 'features'));
  return names;
}

const rows = keyTable(readme);

test('the key table is there and well formed', () => {
  expect(rows.length).toBeGreaterThan(30);
  for (const r of rows) {
    expect(r.keys.length).toBeGreaterThan(0);
    expect(r.commands.length).toBeGreaterThan(0);
    expect([1, r.keys.length]).toContain(r.commands.length);
    for (const c of r.commands) expect(c).toMatch(/^cm_\w+$/);
  }
});

describe.each(['linux', 'windows', 'darwin'])('on %s', (platform) => {
  test('every key in the table runs the command it names', () => {
    reg.setupKeys(platform, {});
    const wrong = [];
    for (const r of rows) {
      r.keys.forEach((k, i) => {
        if (k.includes('Cmd+') && platform !== 'darwin') return; // macOS-only keys
        const want = r.commands.length === 1 ? r.commands[0] : r.commands[i];
        const got = reg.commandForKey(normalizeCombo(k));
        if (got !== want) wrong.push(`${k}: ${got || '(none)'} instead of ${want}`);
      });
    }
    expect(wrong).toEqual([]);
  });
});

test('every command in the table is implemented by a feature', () => {
  const have = registeredCommands();
  expect(have.size).toBeGreaterThan(100);
  const missing = [...new Set(rows.flatMap((r) => r.commands))].filter((c) => !have.has(c));
  expect(missing).toEqual([]);
});

test('the parser reads cells the way the README writes them', () => {
  const md = '<!-- key-table:start -->\n| Key | What | Command |\n| --- | --- | --- |\n| `Ctrl+\\` | Root | `cm_GoToRoot` |\n| `F8` / `Delete` | Delete | `cm_Delete` |\n| `Alt+F1` / `Alt+F2` | Drives | `cm_LeftOpenDrives` / `cm_RightOpenDrives` |\n<!-- key-table:end -->';
  expect(keyTable(md)).toEqual([
    { keys: ['Ctrl+\\'], commands: ['cm_GoToRoot'] },
    { keys: ['F8', 'Delete'], commands: ['cm_Delete'] },
    { keys: ['Alt+F1', 'Alt+F2'], commands: ['cm_LeftOpenDrives', 'cm_RightOpenDrives'] },
  ]);
  expect(keyTable('no table here')).toEqual([]);
});

test('the README says it is not affiliated with Total Commander', () => {
  expect(readme).toMatch(/not affiliated with Ghisler Software GmbH/);
  expect(readme).toMatch(/Inspired by Total Commander/i);
  // "TC" stays out of the name and the title.
  expect(readme.split('\n')[0]).not.toMatch(/\bTC\b|Total Commander/);
  // ...and out of the app's own name, bundle id and file name, which the packages and the domain are made from.
  const w = JSON.parse(readFileSync(join(repo, 'wails.json'), 'utf8'));
  for (const v of [w.name, w.outputfilename, w.info.productName, w.info.bundleIdentifier, w.info.comments]) {
    expect(v).not.toMatch(/\bTC\b|total ?commander/i);
  }
});

test('the download page does not say there are no releases', () => {
  // v0.9.0 is out; a stale "no releases yet" would send people to unofficial copies.
  const page = readFileSync(join(repo, 'docs', 'DOWNLOADS.md'), 'utf8');
  for (const text of [readme, page]) expect(text).not.toMatch(/no releases yet/i);
});

test('the privacy statement links the zero-network test, and the test files exist', () => {
  for (const f of ['privacy_test.go', 'privacy_session_test.go', 'frontend/test/privacy.test.js', 'frontend/test/viewer-noremote.test.js']) {
    expect(readme).toContain(`(${f})`);
    expect(existsSync(join(repo, f))).toBe(true);
  }
  expect(readme).toMatch(/no telemetry/i);
  // docs/privacy.md is the full statement: the README links it, and it links every test file that exists.
  expect(readme).toContain('(docs/privacy.md)');
  const privacy = readFileSync(join(repo, 'docs', 'privacy.md'), 'utf8');
  for (const m of privacy.matchAll(/\]\(\.\.\/([^)]+)\)/g)) expect(existsSync(join(repo, m[1])), m[1]).toBe(true);
  for (const f of ['privacy_test.go', 'privacy_session_test.go', 'frontend/test/privacy.test.js', 'frontend/test/viewer-noremote.test.js']) {
    expect(privacy).toContain(`(../${f})`);
  }
});

test('there is one canonical download page, and releases carry checksums', () => {
  expect(readme).toContain('(docs/DOWNLOADS.md)');
  const page = readFileSync(join(repo, 'docs', 'DOWNLOADS.md'), 'utf8');
  expect(page).toContain('https://github.com/mormitech/mormicommander/releases');
  expect(page).toContain('SHA256SUMS.txt');
  const workflow = readFileSync(join(repo, '.github', 'workflows', 'build.yml'), 'utf8');
  const release = workflow.slice(workflow.indexOf('\n  release:'));
  expect(release).toMatch(/sha256sum .*> SHA256SUMS\.txt/);
  // the page names the files the workflow really makes
  for (const name of ['-macos-universal.zip', '-windows-x64.zip', '-linux-x64.tar.gz', '_amd64.deb', 'mormicommander-setup-']) {
    expect(page).toContain(name);
  }
  expect(readFileSync(join(repo, 'packaging', 'windows', 'installer.iss'), 'utf8')).toContain('OutputBaseFilename=mormicommander-setup-__VERSION__-windows-x64');
});
