// Info on packed file (F-847), archive comments (F-622) and the packers an archive converts to (F-628).
import { expect, test } from 'vitest';
import { packedInfoRows, packedInfoHTML, convertTargets } from '../src/features/archive-info-lib.js';

const zipInfo = { name: 'b.txt', path: 'docs/b.txt', archive: '/tmp/photos.zip', format: 'zip', isDir: false, size: 12345, packed: 2345,
  ratio: 19, mtime: Date.UTC(2024, 4, 17), crc: '0A1B2C3D', method: 'Deflate', encrypted: true, encryption: 'AES-256', comment: '',
  archiveComment: 'Backup <2024>', canComment: true };

test('the info window shows sizes, ratio, date, CRC and method', () => {
  const rows = Object.fromEntries(packedInfoRows(zipInfo, () => 'DATE'));
  expect(rows).toMatchObject({ Name: 'b.txt', Archive: 'photos.zip', Folder: 'docs', Type: 'ZIP archive', Size: '12 345 bytes',
    'Packed size': '2 345 bytes', Ratio: '19%', Date: 'DATE', 'CRC-32': '0A1B2C3D', Method: 'Deflate', Encrypted: 'AES-256',
    'Archive comment': 'Backup <2024>' });
  expect(rows.Comment).toBeUndefined();
});

test('unknown values are left out, and an ISO shows its volume label', () => {
  const rows = Object.fromEntries(packedInfoRows({ name: 'x', path: 'x', archive: 'a.7z', format: '7z', size: 5, packed: -1, ratio: -1, crc: '' }));
  expect(rows['Packed size']).toBeUndefined();
  expect(rows.Ratio).toBeUndefined();
  expect(rows['CRC-32']).toBeUndefined();
  expect(Object.fromEntries(packedInfoRows({ name: 'f', path: 'f', archive: 'd.iso', format: 'iso', archiveComment: 'DISC' }))['Volume label']).toBe('DISC');
});

test('the table escapes what the archive stores', () => {
  const html = packedInfoHTML(zipInfo, () => 'd');
  expect(html).toContain('Backup &lt;2024&gt;');
  expect(html).not.toContain('<2024>');
});

test('convert offers the available packers', () => {
  const list = [{ name: 'zip', available: true }, { name: 'gz', available: true, single: true }, { name: '7z', available: false }];
  expect(convertTargets(list).map((p) => p.name)).toEqual(['zip', 'gz']);
  expect(convertTargets(list, { single: false }).map((p) => p.name)).toEqual(['zip']);
});
