// @vitest-environment happy-dom
// The AI settings (features/ai-settings.js, issue #146 step 2): the pure helpers (model filter, prices, context
// lengths, which options changed) and the dialog wired to a Go recorder: opening it fetches nothing, Show models lists
// the models from the address in the field, a click chooses one, Save sends the key to AI.SetAPIKey (never to the
// options) and the changed options to Configuration.Apply, and the checks keep the dialog open with a sentence.
import { afterEach, describe, expect, test, vi } from 'vitest';

const calls = [];
let values = {};
let hasKey = false;
let setKeyFails = null;
const MODELS = [
  { id: 'anthropic/claude-sonnet-4.5', name: 'Anthropic: Claude Sonnet 4.5', contextLength: 1000000, pricing: { prompt: '0.000003', completion: '0.000015' } },
  { id: 'openai/gpt-5-mini', name: 'OpenAI: GPT-5 Mini', contextLength: 400000, pricing: { prompt: '0.00000025', completion: '0.000002' } },
  { id: 'meta/llama-free', name: 'Meta: Llama (free)', contextLength: 8192, pricing: { prompt: '0', completion: '0' }, noTools: true },
];
const answers = {
  'Configuration.Values': () => values,
  'Configuration.Apply': (ch) => Object.keys(ch),
  'AI.HasAPIKey': () => hasKey,
  'AI.SetAPIKey': () => { if (setKeyFails) throw new Error(setKeyFails); return null; },
  'AI.ModelsFrom': () => MODELS,
  'DialogWindows.Open': () => { throw new Error('no windows here'); },
};
vi.mock('../src/core/go.js', () => {
  const go = new Proxy({}, {
    get: (_, obj) => new Proxy({}, {
      get: (_, m) => (...args) => {
        calls.push({ k: `${obj}.${m}`, args });
        const f = answers[`${obj}.${m}`];
        try { return Promise.resolve(f ? f(...args) : null); } catch (e) { return Promise.reject(e); }
      },
    }),
  });
  return { go, on: () => () => {}, errText: (e) => String(e?.message ?? e).replace(/^Error:\s*/, ''), onFileDrop: () => () => {}, setClipboardText: async () => true };
});
vi.mock('../src/core/settings.js', () => ({ getSetting: (k, d) => (k === 'dialogsInWindows' ? false : d), setSetting: () => Promise.resolve(), onSettingChange: () => () => {}, allSettings: () => ({}) }));
globalThis.requestAnimationFrame = (f) => setTimeout(f, 0);

const { filterModels, perMillion, priceText, contextText, optionChanges, openAISettings } = await import('../src/features/ai-settings.js');
const { getCommand } = await import('../src/core/registry.js');

const tick = (ms = 10) => new Promise((r) => setTimeout(r, ms));
const box = () => document.querySelector('.mc-ai-settings');
const called = (k) => calls.filter((c) => c.k === k);

afterEach(async () => {
  const b = box();
  if (b) { b.querySelector('.cancel').click(); await tick(); }
  calls.length = 0;
  values = {};
  hasKey = false;
  setKeyFails = null;
});

describe('helpers', () => {
  test('filterModels matches every word in the name or id', () => {
    expect(filterModels(MODELS, '').length).toBe(3);
    expect(filterModels(MODELS, 'claude').map((m) => m.id)).toEqual(['anthropic/claude-sonnet-4.5']);
    expect(filterModels(MODELS, 'OPENAI mini').map((m) => m.id)).toEqual(['openai/gpt-5-mini']);
    expect(filterModels(MODELS, 'openai claude')).toEqual([]);
    expect(filterModels(null, 'x')).toEqual([]);
  });
  test('prices per million tokens', () => {
    expect(perMillion('0.000003')).toBe('$3');
    expect(perMillion('0.00000025')).toBe('$0.25');
    expect(perMillion('0.0000001')).toBe('$0.1');
    expect(perMillion('0.0003')).toBe('$300');
    expect(perMillion('0')).toBe('$0');
    expect(perMillion('-1')).toBe('');
    expect(perMillion('')).toBe('');
    expect(perMillion(undefined)).toBe('');
    expect(priceText(MODELS[0].pricing)).toBe('$3 / $15 per M tokens');
    expect(priceText(MODELS[2].pricing)).toBe('free');
    expect(priceText({ prompt: '-1', completion: '-1' })).toBe('');
    expect(priceText(null)).toBe('');
  });
  test('context lengths', () => {
    expect(contextText(8192)).toBe('8K');
    expect(contextText(200000)).toBe('200K');
    expect(contextText(1048576)).toBe('1M');
    expect(contextText(1500000)).toBe('1.5M');
    expect(contextText(512)).toBe('512');
    expect(contextText(0)).toBe('');
  });
  test('only changed options are sent', () => {
    const saved = { 'AI.Provider': '0', 'AI.OpenRouterModel': '', 'AI.OpenRouterURL': 'U' };
    expect(optionChanges(saved, { provider: '0', model: ' ', url: 'U' })).toEqual({});
    expect(optionChanges(saved, { provider: '1', model: ' a/b ', url: 'U' })).toEqual({ 'AI.Provider': '1', 'AI.OpenRouterModel': 'a/b' });
    expect(optionChanges(saved, { provider: '0', model: '', url: '' })).toEqual({ 'AI.OpenRouterURL': '' });
  });
});

describe('the dialog', () => {
  test('is a command in the Configuration menu', () => {
    const c = getCommand('cm_AISettings');
    expect(c).toMatchObject({ label: 'AI settings…', menu: 'Configuration' });
  });

  test('choose OpenRouter, a key and a model from the list, and save', async () => {
    values = { 'AI.Provider': '0', 'AI.OpenRouterModel': '', 'AI.OpenRouterURL': 'SAVED-URL', 'AI.ClaudeModel': 'opus' };
    const done = openAISettings();
    await tick();
    const b = box();
    expect(b.querySelector('input[name=ai-provider]:checked').value).toBe('0');
    expect(b.querySelector('#ai-url').value).toBe('SAVED-URL');
    expect(b.querySelector('.mc-ai-keystate').textContent).toMatch(/No key is saved/);
    expect(b.querySelector('.mc-ai-remove').hidden).toBe(true);
    expect(called('AI.ModelsFrom')).toEqual([]); // (opening the dialog sends nothing over the network)

    b.querySelector('input[name=ai-provider][value="1"]').checked = true;
    b.querySelector('#ai-url').value = 'TYPED-URL';
    b.querySelector('#ai-url').dispatchEvent(new Event('input'));
    b.querySelector('.mc-ai-load').click();
    await tick();
    expect(called('AI.ModelsFrom')[0].args).toEqual(['openrouter', 'TYPED-URL']);
    const rows = () => [...b.querySelectorAll('.mc-ai-model')];
    expect(rows().length).toBe(3);
    expect(rows()[0].textContent).toMatch(/Anthropic: Claude Sonnet 4\.5/);
    expect(rows()[0].textContent).toMatch(/anthropic\/claude-sonnet-4\.5 · 1M context · \$3 \/ \$15 per M tokens/);
    expect(rows()[2].className).toMatch(/no-tools/);
    expect(rows()[2].textContent).toMatch(/free · can't use tools/);
    const filter = b.querySelector('#ai-model-filter');
    filter.value = 'gpt';
    filter.dispatchEvent(new Event('input'));
    expect(rows().length).toBe(1);
    expect(b.querySelector('.mc-ai-modelstate').textContent).toMatch(/^1 of 3 models\./);
    rows()[0].click();
    expect(b.querySelector('#ai-model').value).toBe('openai/gpt-5-mini');
    expect(b.querySelector('.mc-ai-model.on')).toBeTruthy();

    // No key yet: Save says so and stays open.
    b.requestSubmit();
    await tick();
    expect(b.querySelector('.mc-err').textContent).toMatch(/needs an API key/);
    expect(called('Configuration.Apply')).toEqual([]);

    b.querySelector('#ai-key').value = '  sk-or-v1-abc  ';
    b.requestSubmit();
    expect(await done).toBe(true);
    expect(called('AI.SetAPIKey')[0].args).toEqual(['openrouter', 'sk-or-v1-abc']);
    const applied = called('Configuration.Apply')[0].args[0];
    expect(applied).toEqual({ 'AI.Provider': '1', 'AI.OpenRouterModel': 'openai/gpt-5-mini', 'AI.OpenRouterURL': 'TYPED-URL' });
    expect(JSON.stringify(calls.filter((c) => c.k !== 'AI.SetAPIKey'))).not.toMatch(/sk-or/); // (the key goes to the keychain only)
  });

  test('arrow keys in the filter move through the list; Enter there does not save', async () => {
    values = { 'AI.Provider': '1', 'AI.OpenRouterModel': 'anthropic/claude-sonnet-4.5', 'AI.OpenRouterURL': '' };
    hasKey = true;
    openAISettings();
    await tick();
    const b = box();
    const filter = b.querySelector('#ai-model-filter');
    filter.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }));
    await tick();
    expect(called('AI.ModelsFrom').length).toBe(1);
    expect(b.querySelector('.mc-ai-model.on').textContent).toMatch(/Claude Sonnet/);
    filter.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true, cancelable: true }));
    expect(b.querySelector('#ai-model').value).toBe('openai/gpt-5-mini');
    filter.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true }));
    expect(b.querySelector('#ai-model').value).toBe('anthropic/claude-sonnet-4.5');
    expect(box()).toBeTruthy();
    expect(called('Configuration.Apply')).toEqual([]);
  });

  test('a saved key can be removed; a keychain error keeps the dialog open', async () => {
    values = { 'AI.Provider': '0', 'AI.OpenRouterModel': 'm', 'AI.OpenRouterURL': '' };
    hasKey = true;
    const done = openAISettings();
    await tick();
    const b = box();
    expect(b.querySelector('.mc-ai-keystate').textContent).toMatch(/A key is saved in the system keychain/);
    expect(b.querySelector('#ai-key').value).toBe(''); // (the key itself is never shown)
    b.querySelector('.mc-ai-remove').click();
    expect(b.querySelector('.mc-ai-keystate').textContent).toMatch(/will be removed/);
    setKeyFails = 'The API key can\'t be saved, because the system keychain can\'t be used: install secret-tool.';
    b.requestSubmit();
    await tick();
    expect(b.querySelector('.mc-err').textContent).toMatch(/install secret-tool/);
    setKeyFails = null;
    b.requestSubmit();
    expect(await done).toBe(true);
    expect(called('AI.SetAPIKey').at(-1).args).toEqual(['openrouter', '']);
    expect(called('Configuration.Apply')).toEqual([]); // (nothing else changed)
  });

  test('OpenRouter without a model is not saved; a failed list says why', async () => {
    values = { 'AI.Provider': '1', 'AI.OpenRouterModel': '', 'AI.OpenRouterURL': '' };
    hasKey = true;
    answers['AI.ModelsFrom'] = () => { throw new Error('The AI provider (x) could not be reached: no route'); };
    try {
      openAISettings();
      await tick();
      const b = box();
      b.querySelector('.mc-ai-load').click();
      await tick();
      expect(b.querySelector('.mc-ai-modelstate').textContent).toBe('The model list could not be fetched: The AI provider (x) could not be reached: no route');
      expect(b.querySelector('.mc-ai-models').hidden).toBe(true);
      b.requestSubmit();
      await tick();
      expect(b.querySelector('.mc-err').textContent).toMatch(/Choose a model/);
      expect(called('Configuration.Apply')).toEqual([]);
    } finally {
      answers['AI.ModelsFrom'] = () => MODELS;
    }
  });
});
