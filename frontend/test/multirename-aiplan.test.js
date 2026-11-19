import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { applyRenamePlan, renamePlanEnabled } from '../src/features/multirename-lib.js';

const form = {
  nameMask: '[N]', extMask: '[E]', search: 'old', replace: 'new', caseSensitive: false, onlyFirst: false, includeExt: false,
  regex: true, subst: true, case: '', counterStart: 5, counterStep: 2, counterDigits: 3, removeAccents: true,
};

describe('AI rename plan (issue #33)', () => {
  it('is hidden unless a local model address is set', () => {
    expect(renamePlanEnabled(undefined)).toBe(false);
    expect(renamePlanEnabled({})).toBe(false);
    expect(renamePlanEnabled({ 'AI.LocalRenameURL': '  ' })).toBe(false);
    expect(renamePlanEnabled({ 'AI.LocalRenameURL': 'http://localhost:11434/v1' })).toBe(true);
  });

  it('fills in only the fields the plan set', () => {
    const out = applyRenamePlan(form, { nameMask: '[Y]-[M]-[D] [N]', case: 'lower', search: 'ignored', set: ['nameMask', 'case'] });
    expect(out.nameMask).toBe('[Y]-[M]-[D] [N]');
    expect(out.case).toBe('lower');
    expect(out.search).toBe('old');
    expect(out.regex).toBe(true);
    expect(out.subst).toBe(true);
    expect(out.counterStart).toBe(5);
    expect(out.removeAccents).toBe(true);
    expect(form.nameMask).toBe('[N]');
  });

  it('a literal search turns RegEx and Subst. off; unknown keys are ignored', () => {
    const out = applyRenamePlan(form, { search: ' ', replace: '_', counterStart: 99, set: ['search', 'replace', 'counterStart'] });
    expect(out).toMatchObject({ search: ' ', replace: '_', regex: false, subst: false, counterStart: 5 });
    const rx = applyRenamePlan({ ...form, regex: false }, { search: '(\\d+)', replace: '#$1', regex: true, set: ['search', 'replace', 'regex'] });
    expect(rx).toMatchObject({ regex: true, search: '(\\d+)' });
  });

  it('the tool only fills the form: the plan never reaches Execute', () => {
    const src = readFileSync(new URL('../src/features/multirename.js', import.meta.url), 'utf8');
    const fn = src.slice(src.indexOf('async function aiPlan'), src.indexOf('// ---- Undo ----'));
    expect(fn).toContain('go.AI.RenamePlan');
    expect(fn).not.toMatch(/Execute|startRename/);
  });

  it('is not an AI tool', () => {
    const allow = readFileSync(new URL('../src/features/aitools-allow.js', import.meta.url), 'utf8');
    expect(allow).not.toMatch(/RenamePlan|AI\.LocalRename/);
  });
});
