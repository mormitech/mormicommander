// The pure parts of the AI tools (features/aitools-lib.js, issue #146): find_files arguments → the Find files request
// (with the dialog's defaults, "~", sizes, dates and ages), select_files' patterns, names and modes, and the texts
// for the model and the AI tab's log.
import { describe, expect, test } from 'vitest';
import {
  side, selectMode, nameList, maskExpr, maskKind, newSelection, findRequest, findSummary, describeTool, toolName,
  expandHome, tilde, addDays, providerLine, findContext, humanSize,
} from '../src/features/aitools-lib.js';

const env = { activePath: '/Users/me/Code', home: '/Users/me', sep: '/', today: '2026-11-14' };

describe('find_files → FindRequest', () => {
  test('defaults are those of a search from the dialog, in the active panel\'s folder', () => {
    const r = findRequest({}, env);
    expect(r).toMatchObject({
      searchFor: '*', nameRegex: false, searchIn: '/Users/me/Code', depth: -1, textOn: false, text: '',
      encodings: ['ansi', 'utf8', 'utf16le'], ansiCodepage: 'windows-1252', oemCodepage: 'cp437',
      dateOn: false, ageOn: false, sizeOn: false, kind: '', skipHidden: true, excludeDirs: '', dupOn: false, selected: [], withinRun: 0,
    });
  });
  test('names, text, regex flags, depth, kind, hidden, skipped folders', () => {
    const r = findRequest({ namePattern: '*.pdf;*.docx', text: 'invoice', textRegex: true, caseSensitive: true, depth: 0, kind: 'files',
      includeHidden: true, excludeDirs: 'node_modules;.git', nameRegex: false }, env);
    expect(r).toMatchObject({ searchFor: '*.pdf;*.docx', textOn: true, text: 'invoice', textRegex: true, caseSensitive: true, depth: 0,
      kind: 'files', skipHidden: false, excludeDirs: 'node_modules;.git' });
    expect(findRequest({ kind: 'everything' }, env).kind).toBe('');
    expect(() => findRequest({ depth: -3 }, env)).toThrow(/depth/);
  });
  test('"~" is the home folder; folders with spaces are quoted', () => {
    expect(findRequest({ searchIn: '~/Documents; ~' }, env).searchIn).toBe('/Users/me/Documents;/Users/me');
    expect(findRequest({ searchIn: '~/My Files' }, env).searchIn).toBe('"/Users/me/My Files"');
  });
  test('sizes: op, unit, fractions in bytes', () => {
    expect(findRequest({ sizeValue: 1, sizeUnit: 'MB' }, env)).toMatchObject({ sizeOn: true, sizeOp: '>', sizeValue: 1, sizeUnit: 'MB' });
    expect(findRequest({ sizeOp: '<', sizeValue: 500 }, env)).toMatchObject({ sizeOn: true, sizeOp: '<', sizeValue: 500, sizeUnit: 'B' });
    expect(findRequest({ sizeValue: 1.5, sizeUnit: 'kb' }, env)).toMatchObject({ sizeValue: 1536, sizeUnit: 'B' });
    expect(() => findRequest({ sizeValue: 1, sizeUnit: 'PB' }, env)).toThrow(/sizeUnit/);
    expect(() => findRequest({ sizeValue: -1 }, env)).toThrow(/sizeValue/);
  });
  test('one age stays an age in days, as the dialog\'s Not older than / Older than', () => {
    expect(findRequest({ newerThanDays: 7 }, env)).toMatchObject({ ageOn: true, ageOlder: false, ageN: 7, ageUnit: 'days', dateOn: false });
    expect(findRequest({ olderThanDays: 30 }, env)).toMatchObject({ ageOn: true, ageOlder: true, ageN: 30, ageUnit: 'days' });
  });
  test('dates, and ages with dates or with each other, become one range of days', () => {
    expect(findRequest({ dateFrom: '2026-01-01', dateTo: '2026-03-31' }, env)).toMatchObject({ dateOn: true, dateFrom: '2026-01-01', dateTo: '2026-03-31', ageOn: false });
    expect(findRequest({ dateFrom: '2026-01-01' }, env)).toMatchObject({ dateOn: true, dateFrom: '2026-01-01', dateTo: '' });
    expect(findRequest({ newerThanDays: 30, olderThanDays: 7 }, env)).toMatchObject({ dateOn: true, dateFrom: '2026-10-15', dateTo: '2026-11-06', ageOn: false });
    expect(findRequest({ dateFrom: '2026-01-01', newerThanDays: 10 }, env)).toMatchObject({ dateFrom: '2026-11-04' });
    expect(() => findRequest({ dateFrom: '2026-13-01' }, env)).toThrow(/YYYY-MM-DD/);
    expect(() => findRequest({ dateFrom: '2026-05-01', dateTo: '2026-04-01' }, env)).toThrow(/nothing to find/);
  });
  test('addDays crosses months and years', () => {
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
    expect(addDays('2026-12-31', 1)).toBe('2027-01-01');
  });
});

describe('select_files', () => {
  test('side and mode', () => {
    expect(side('LEFT')).toBe('left');
    expect(side('right')).toBe('right');
    expect(side()).toBe('active');
    expect(side('source')).toBe('active');
    expect(selectMode('add')).toBe('add');
    expect(selectMode('remove')).toBe('remove');
    expect(selectMode('whatever')).toBe('replace');
  });
  test('patterns become mask expressions; masks with spaces are quoted', () => {
    expect(maskExpr('*.pdf; *.docx ;')).toBe('*.pdf;*.docx');
    expect(maskExpr('My file*.txt;*.md')).toBe('"My file*.txt";*.md');
    expect(maskExpr('<^IMG_\\d+')).toBe('<^IMG_\\d+');
    expect(maskExpr('')).toBe('');
  });
  test('a pattern selects files, unless it names folders or markDirectories is on', () => {
    expect(maskKind('*.pdf')).toBe('files');
    expect(maskKind('build/;*.o')).toBe('both');
    expect(maskKind('*.pdf', true)).toBe('both');
  });
  test('names: a list or a string, never ".."', () => {
    expect(nameList(['a.txt', ' b.txt ', '..', ''])).toEqual(['a.txt', 'b.txt']);
    expect(nameList('a.txt;b.txt')).toEqual(['a.txt', 'b.txt']);
    expect(nameList(undefined)).toEqual([]);
  });
  test('replace, add, remove; ".." never selected', () => {
    const cur = new Set(['a', 'b']);
    expect([...newSelection(cur, ['c', '..'], 'replace')]).toEqual(['c']);
    expect([...newSelection(cur, ['c'], 'add')].sort()).toEqual(['a', 'b', 'c']);
    expect([...newSelection(cur, ['a', 'z'], 'remove')]).toEqual(['b']);
    expect([...cur]).toEqual(['a', 'b']); // (the panel's own set is left alone)
  });
});

describe('texts', () => {
  test('findSummary: the count, then at most the limit of results with size and date', () => {
    const t = new Date(2026, 10, 2, 14, 3).getTime();
    const items = [{ p: '/a/x.pdf', s: 1536, t }, { p: '/a/sub', s: 0, t, f: true }, { p: '/a/y.pdf', s: 10, t }];
    const s = findSummary(items, { errors: 0 }, { limit: 2, where: 'They are in the list.' });
    expect(s.split('\n')).toEqual([
      'Found 3 results (2 files, 1 folder). They are in the list. The first 2 are listed.',
      '/a/x.pdf  1.5 KB  2026-11-02 14:03',
      '/a/sub/  folder  2026-11-02 14:03',
    ]);
    expect(findSummary([], null)).toBe('Found nothing.');
    expect(findSummary([], { stopped: true })).toMatch(/stopped/);
    expect(findSummary([], { error: 'Bad mask.' })).toBe('The search failed: Bad mask.');
  });
  test('humanSize', () => {
    expect(humanSize(512)).toBe('512 B');
    expect(humanSize(1024 * 1024 * 3.25)).toBe('3.3 MB');
    expect(humanSize(1024 * 200)).toBe('200 KB');
  });
  test('tool calls as short lines', () => {
    expect(toolName('mcp__mormi__find_files')).toBe('find_files');
    expect(toolName('find_files')).toBe('find_files');
    expect(describeTool('mcp__mormi__find_files', { namePattern: '*.pdf', searchIn: '/Users/me/Documents' }, '/Users/me')).toBe('Searching *.pdf in ~/Documents…');
    expect(describeTool('find_files', { text: 'tax' }, '/Users/me')).toBe('Searching files containing "tax" in the panel\'s folder…');
    expect(describeTool('select_files', { pattern: '*.jpg', side: 'left' })).toBe('Selecting *.jpg in the left panel…');
    expect(describeTool('select_files', { names: ['a', 'b'], mode: 'remove' })).toBe('Unselecting a, b in the active panel…');
    expect(describeTool('set_panel_path', { path: '/Users/me/x' }, '/Users/me')).toBe('Opening ~/x in the active panel…');
    expect(describeTool('get_state', {})).toBe('Looking at the panels…');
  });
  test('home folder: expand and shorten', () => {
    expect(expandHome('~', '/h')).toBe('/h');
    expect(expandHome('~/a', '/h')).toBe('/h/a');
    expect(expandHome('~\\a', 'C:\\Users\\me', '\\')).toBe('C:\\Users\\me\\a');
    expect(expandHome('/x', '/h')).toBe('/x');
    expect(tilde('/h/a', '/h')).toBe('~/a');
    expect(tilde('/hx/a', '/h')).toBe('/hx/a');
  });
  test('provider line', () => {
    expect(providerLine({ available: true, path: '/opt/homebrew/bin/claude', version: '2.0.31 (Claude Code)' }).text).toBe('Claude Code 2.0.31 found at /opt/homebrew/bin/claude');
    expect(providerLine({ available: false, error: 'not found' })).toMatchObject({ ok: false, error: 'not found' });
    expect(providerLine({ available: false }).text).toMatch(/install it from claude\.com\/claude-code, or set its path in Options > AI/);
    expect(providerLine(null).text).toBe('AI is not available in this build.');
  });
  test('the context for a prompt from the dialog', () => {
    const c = findContext({ active: { side: 'left', path: '/a' }, other: { side: 'right', path: '/b' }, platform: 'darwin', home: '/h', searchIn: '/a' }, new Date(2026, 10, 14));
    expect(c).toMatch(/Find files dialog/);
    expect(c).toMatch(/active panel \(left\) shows \/a/);
    expect(c).toMatch(/other panel \(right\) shows \/b/);
    expect(c).toMatch(/Today is Saturday, 2026-11-14/);
    expect(c).not.toMatch(/Search in/);
  });
});
