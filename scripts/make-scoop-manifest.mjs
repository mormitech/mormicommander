// Generates the Scoop manifest for a release (prints JSON).
//   node scripts/make-scoop-manifest.mjs [version] > ../scoop-bucket/bucket/mormicommander.json
// Scoop downloads the plain zip (mormicommander.exe + LICENSE) and adds a Start menu shortcut and the mormicommander command.
import { createHash } from 'node:crypto';
import { readFileSync } from 'node:fs';
import { join, resolve } from 'node:path';

const root = resolve(import.meta.dirname, '..');
const version = process.argv[2] ?? JSON.parse(readFileSync(join(root, 'wails.json'), 'utf8')).info.productVersion;
const repo = 'mormitech/mormicommander';
const file = (v) => `https://github.com/${repo}/releases/download/v${v}/mormicommander-${v}-windows-x64.zip`;
const url = file(version);
const res = await fetch(url);
if (!res.ok) throw new Error(`${url}: HTTP ${res.status} (is the release published?)`);
const hash = createHash('sha256').update(Buffer.from(await res.arrayBuffer())).digest('hex');

const manifest = {
  version,
  description: 'Two-panel file manager in the spirit of Total Commander',
  homepage: `https://github.com/${repo}`,
  license: 'MIT',
  architecture: { '64bit': { url, hash } },
  bin: 'mormicommander.exe',
  shortcuts: [['mormicommander.exe', 'mormiCommander']],
  checkver: 'github',
  autoupdate: { architecture: { '64bit': { url: file('$version') } } },
};
process.stdout.write(JSON.stringify(manifest, null, 4) + '\n');
