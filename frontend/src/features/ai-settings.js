// The AI settings (issue #146, step 2): Configuration > AI settings… (cm_AISettings), also opened from the provider
// line of the Find files dialog's AI tab. It chooses the provider a run uses (Claude Code, or OpenRouter / another
// OpenAI-compatible API), and for OpenRouter the API key, the model and the API address.
//
// The provider, model and address are ordinary options (AI.Provider, AI.OpenRouterModel, AI.OpenRouterURL on the AI
// page of the Options dialog) and are saved the same way (applyOptions). The API key is not: it goes straight to the
// system keychain through AI.SetAPIKey, and the dialog only learns whether one is saved (AI.HasAPIKey), never the key
// itself, so it is never in the page's memory after the field is cleared, nor in any settings file.
//
// The model list comes from the API (AI.ModelsFrom, GET {address}/models) and is fetched only when the user presses
// Show models: opening this dialog sends nothing over the network. The address is the one in the dialog's field, so a
// changed address can be tried before it is saved. The page never holds a web address of its own here (the default
// comes from Go's option), as frontend/test/privacy.test.js wants.

import { go, errText } from '../core/go.js';
import { command } from '../core/registry.js';
import { modal, esc } from '../core/dialogs.js';
import { applyOptions } from './config/util.js';
import './ai-settings.css';

const PROVIDERS = [
  { value: '0', label: 'Claude Code', note: 'the claude command, installed and signed in by you; its path and model are in Options > AI' },
  { value: '1', label: 'OpenRouter', note: 'or another OpenAI-compatible API; needs an API key and a model' },
];
const MAX_ROWS = 500; // rows the model list draws at once (OpenRouter lists a few hundred; a filter narrows them)

// ---- pure helpers (tested in test/ai-settings.test.js) ----

// filterModels keeps the models whose name or id contains every word of the query (any case).
export function filterModels(models, query) {
  const words = String(query ?? '').toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return models || [];
  return (models || []).filter((m) => {
    const hay = `${m.name || ''} ${m.id || ''}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}

// perMillion turns a price per token ("0.000003", as the API writes it) into dollars per million tokens ("$3",
// "$0.15"); "" when there is no usable price (missing, or negative: OpenRouter's routers price per request).
export function perMillion(perToken) {
  if (perToken === undefined || perToken === null || perToken === '') return '';
  const n = Number(perToken);
  if (!Number.isFinite(n) || n < 0) return '';
  const m = n * 1e6;
  if (m === 0) return '$0';
  const digits = m >= 100 ? 0 : m >= 1 ? 2 : m >= 0.01 ? 3 : 4;
  return '$' + m.toFixed(digits).replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');
}

// priceText is a model's price for the list: "free", "$3 / $15 per M tokens" (input / output), or "".
export function priceText(pricing) {
  if (!pricing) return '';
  const p = perMillion(pricing.prompt), c = perMillion(pricing.completion);
  if (!p && !c) return '';
  if (p === '$0' && c === '$0') return 'free';
  return `${p || '?'} / ${c || '?'} per M tokens`;
}

// contextText writes a context length short: 8192 → "8K", 200000 → "200K", 1048576 → "1M".
export function contextText(n) {
  n = Number(n) || 0;
  if (n <= 0) return '';
  if (n >= 1e6) return `${+(n / 1e6).toFixed(1)}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}K`;
  return String(n);
}

// optionChanges is what Save sends to applyOptions: the AI options of the form that differ from the saved values.
export function optionChanges(saved, form) {
  const out = {};
  const want = { 'AI.Provider': form.provider, 'AI.OpenRouterModel': form.model.trim(), 'AI.OpenRouterURL': form.url.trim() };
  for (const [k, v] of Object.entries(want)) if ((saved[k] ?? '') !== v) out[k] = v;
  return out;
}

// ---- the dialog ----

const notAvailable = (e) => /AI\.\w+ is not available/.test(errText(e));

// openAISettings shows the dialog; it answers true when the settings were saved.
export async function openAISettings() {
  let saved = {};
  try { saved = (await go.Configuration.Values()) || {}; } catch { saved = {}; }
  let hasKey = false;
  try { hasKey = !!(await go.AI.HasAPIKey('openrouter')); } catch { hasKey = false; }
  // (the dialog's keys go through meta.onKey: modal() keeps keydown events away from the fields' own listeners)
  const meta = { kind: 'ai-settings' };
  return modal((box, done) => { meta.onKey = build(box, done, saved, hasKey); }, false, meta);
}

// build fills the dialog and returns its key handler (true: the key was used).
function build(box, done, saved, hasKey) {
  const provider = saved['AI.Provider'] === '1' ? '1' : '0';
  box.classList.add('mc-ai-settings');
  box.innerHTML = `<h2>AI settings</h2>
    <fieldset class="mc-ai-providers"><legend>Provider</legend>
      ${PROVIDERS.map((p) => `<label><input type="radio" name="ai-provider" value="${p.value}"${p.value === provider ? ' checked' : ''}>
        <b>${esc(p.label)}</b> <span class="mc-ai-muted">(${esc(p.note)})</span></label>`).join('')}
    </fieldset>
    <fieldset class="mc-ai-or"><legend>OpenRouter</legend>
      <div class="mc-row"><label for="ai-key">API key</label>
        <div class="mc-ai-line"><input id="ai-key" type="password" spellcheck="false" autocomplete="off">
          <button type="button" class="mc-ai-remove">Remove key</button></div>
        <p class="mc-note mc-ai-keystate"></p></div>
      <div class="mc-row"><label for="ai-model">Model</label>
        <input id="ai-model" type="text" spellcheck="false" value="${esc(saved['AI.OpenRouterModel'] || '')}" placeholder="Choose from the list, or type a model id">
        <div class="mc-ai-line"><input id="ai-model-filter" type="text" spellcheck="false" placeholder="Filter the list (name or id)" aria-label="Filter the models">
          <button type="button" class="mc-ai-load">Show models</button></div>
        <div class="mc-ai-models" role="listbox" aria-label="Models" hidden></div>
        <p class="mc-note mc-ai-modelstate">The list is fetched from the API address below when you press Show models.</p></div>
      <div class="mc-row"><label for="ai-url">API address</label>
        <input id="ai-url" type="text" spellcheck="false" value="${esc(saved['AI.OpenRouterURL'] || '')}">
        <p class="mc-note">OpenRouter's address, or that of another OpenAI-compatible API (https, or http on this computer only). Empty means OpenRouter.</p></div>
    </fieldset>
    <p class="mc-note">What a run sends to the provider: your prompt, a short description of where you are (the panels' folders), and what the tools answer (file names, sizes, dates). The key stays in the system keychain.</p>
    <p class="mc-err" hidden></p>
    <div class="mc-btns"><button type="button" class="cancel">Cancel</button><button type="submit" class="primary">Save</button></div>`;

  const $ = (s) => box.querySelector(s);
  const keyIn = $('#ai-key'), modelIn = $('#ai-model'), filterIn = $('#ai-model-filter'), urlIn = $('#ai-url');
  const list = $('.mc-ai-models'), modelState = $('.mc-ai-modelstate');
  let removeKey = false;
  let models = null;     // the list once fetched
  let shown = [];        // the rows the filter lets through
  let loading = false;

  const showError = (msg) => { const e = $('.mc-err'); e.hidden = !msg; e.textContent = msg || ''; };

  function renderKeyState() {
    const st = $('.mc-ai-keystate');
    const rm = $('.mc-ai-remove');
    rm.hidden = !hasKey || removeKey;
    if (removeKey) st.textContent = 'The saved key will be removed when you save.';
    else st.textContent = hasKey ? 'A key is saved in the system keychain. Type a new one to replace it.' : 'No key is saved. Paste your key; it is kept in the system keychain only.';
    keyIn.placeholder = hasKey && !removeKey ? '(saved; type to replace)' : 'Paste your API key';
  }

  function renderList() {
    if (!models) return;
    shown = filterModels(models, filterIn.value);
    const cur = modelIn.value.trim();
    list.hidden = false;
    list.innerHTML = shown.slice(0, MAX_ROWS).map((m, i) => {
      const meta = [m.id, contextText(m.contextLength) && `${contextText(m.contextLength)} context`, priceText(m.pricing), m.noTools && "can't use tools"].filter(Boolean);
      return `<div class="mc-ai-model${m.id === cur ? ' on' : ''}${m.noTools ? ' no-tools' : ''}" role="option" data-i="${i}" aria-selected="${m.id === cur}"
        ${m.noTools ? 'title="This model can\'t call tools, so it can\'t search with mormiCommander."' : ''}>
        <span class="mc-ai-name">${esc(m.name || m.id)}</span><span class="mc-ai-meta">${esc(meta.join(' · '))}</span></div>`;
    }).join('') || '<div class="mc-ai-none">No model matches the filter.</div>';
    const more = shown.length > MAX_ROWS ? ` The first ${MAX_ROWS} are shown; type in the filter to narrow them.` : '';
    modelState.textContent = `${shown.length} of ${models.length} models.${more} Click one to choose it.`;
    list.querySelector('.on')?.scrollIntoView?.({ block: 'nearest' });
  }

  function choose(i) {
    const m = shown[i];
    if (!m) return;
    modelIn.value = m.id;
    renderList();
  }

  async function loadModels() {
    if (loading) return;
    loading = true;
    showError('');
    modelState.textContent = 'Fetching the model list…';
    try {
      models = (await go.AI.ModelsFrom('openrouter', urlIn.value.trim())) || [];
      renderList();
      filterIn.focus();
    } catch (e) {
      models = null;
      list.hidden = true;
      modelState.textContent = notAvailable(e) ? 'AI is not available in this build.' : `The model list could not be fetched: ${errText(e)}`;
    } finally {
      loading = false;
    }
  }

  $('.mc-ai-load').onclick = loadModels;
  $('.mc-ai-remove').onclick = () => { removeKey = true; keyIn.value = ''; renderKeyState(); };
  keyIn.addEventListener('input', () => { if (keyIn.value) { removeKey = false; renderKeyState(); } });
  filterIn.addEventListener('input', renderList);
  // (a changed address has its own list: the old one is put away until Show models fetches the new one)
  urlIn.addEventListener('input', () => { models = null; list.hidden = true; modelState.textContent = 'Press Show models to fetch the list from this address.'; });
  list.addEventListener('click', (ev) => { const row = ev.target.closest('.mc-ai-model'); if (row) choose(+row.dataset.i); });
  $('.cancel').onclick = () => done(false);

  box.onsubmit = async (ev) => {
    ev.preventDefault();
    showError('');
    const form = {
      provider: box.querySelector('input[name=ai-provider]:checked')?.value === '1' ? '1' : '0',
      model: modelIn.value, url: urlIn.value,
    };
    if (form.provider === '1' && !form.model.trim()) {
      showError('Choose a model for OpenRouter (Show models lists them), or choose Claude Code.');
      modelIn.focus();
      return;
    }
    const keyAfter = !!keyIn.value.trim() || (hasKey && !removeKey);
    if (form.provider === '1' && !keyAfter) {
      showError('OpenRouter needs an API key: paste it in the API key field.');
      keyIn.focus();
      return;
    }
    try {
      if (keyIn.value.trim()) await go.AI.SetAPIKey('openrouter', keyIn.value.trim());
      else if (removeKey) await go.AI.SetAPIKey('openrouter', '');
    } catch (e) {
      showError(notAvailable(e) ? 'AI is not available in this build.' : errText(e));
      return;
    }
    keyIn.value = '';
    try {
      await applyOptions(optionChanges(saved, form));
    } catch (e) {
      showError(errText(e));
      return;
    }
    done(true);
  };

  renderKeyState();

  // Up and Down in the filter move through the list (each step chooses that model); Enter there fetches the list when
  // it is not there yet, and never saves the dialog, since the filter is where one types to look, not to finish.
  return (ev) => {
    if (ev.target !== filterIn) return false;
    if (ev.key === 'Enter') { if (!models) loadModels(); return true; }
    if (ev.key !== 'ArrowDown' && ev.key !== 'ArrowUp') return false;
    const n = models ? Math.min(shown.length, MAX_ROWS) : 0;
    if (!n) return true;
    const down = ev.key === 'ArrowDown';
    const at = shown.findIndex((m) => m.id === modelIn.value.trim());
    choose(at < 0 || at >= n ? (down ? 0 : n - 1) : Math.max(0, Math.min(n - 1, at + (down ? 1 : -1))));
    return true;
  };
}

command({
  name: 'cm_AISettings',
  label: 'AI settings…',
  menu: 'Configuration',
  menuAfter: 'cm_ConfigDirHotlist',
  run: () => openAISettings(),
});
