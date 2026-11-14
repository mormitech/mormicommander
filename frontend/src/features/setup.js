// First-run setup (board issue #6, F-NEW-1, ours): one page shown the very first time the app starts on a computer,
// with the keyboard preset (Total Commander, Mac-native, Marta-like, vim), a "press F5" test that says whether fn is
// needed, and on macOS a button that opens System Settings > Privacy & Security > Full Disk Access. When Marta's
// settings folder is on the Mac, a "Coming from Marta" box offers "Import from Marta", which copies Marta's favourite
// folders into a "Marta" submenu of the folder hotlist (board issue #22; feature_setup_marta.go, setup-lib.js). The choice is saved in
// settings.json ("keyPreset", and "setupDone" once the page was shown); Configuration > First-run setup… opens the
// page again at any time.
//
// It changes nothing for an existing installation: the page opens by itself only when the app found no settings file
// and no saved window at start (Setup.Info in feature_setup.go), never in control mode or the demo, and the default
// preset is Total Commander's keys exactly as they are. The presets themselves are in core/keypresets.js.

import './setup.css';
import { command, setPresetKeys } from '../core/registry.js';
import { go, errText } from '../core/go.js';
import { modal, esc } from '../core/dialogs.js';
import { addControlCommand } from '../core/control.js';
import { app as theApp } from '../core/app.js';
import { martaOffer, martaDone } from './setup-lib.js';
import { KEY_PRESETS, DEFAULT_PRESET, presetName, presetKeys, shouldShowSetup, fnAdvice } from '../core/keypresets.js';

// applyPreset lays the chosen preset's keys into the keymap (the menus and the function key bar follow by themselves).
// The setting "quickLookSpace" (board issue #19, off by default) makes the Mac-native preset's Space and ⌘Y open Quick
// Look instead of the built-in viewer.
function applyPreset(name) { setPresetKeys(presetKeys(name, { quickLookSpace: !!theApp.setting('quickLookSpace', false) })); }

// showSetup opens the setup page. The answer is the chosen preset, or null when it was closed without OK.
async function showSetup(app) {
  const platform = app.platform;
  const mac = platform === 'darwin';
  let fnKeys = mac ? await go.Setup.FnKeys().catch(() => '') : '';
  const martaInfo = mac ? await go.Setup.Marta().catch(() => null) : null;
  const marta = martaOffer(martaInfo);
  const current = presetName(app.setting('keyPreset', DEFAULT_PRESET));
  let pressed = false;
  let say = () => {};
  const qlBefore = !!app.setting('quickLookSpace', false);
  let qlAfter = qlBefore;
  const answer = await modal((box, done) => {
    const presets = Object.entries(KEY_PRESETS).map(([id, p]) => `<label class="mc-setup-preset"><input type="radio" name="keyPreset" id="preset-${id}" value="${id}"${id === current ? ' checked' : ''}>
        <span><b>${esc(p.label)}</b>${id === DEFAULT_PRESET ? ' (default)' : ''}<br><span class="mc-note">${esc(p.note)}</span></span></label>`).join('');
    box.innerHTML = `<h2>Welcome to mormiCommander</h2>
      <p class="mc-note">A few choices before you start. You can come back here any time: Configuration > First-run setup….</p>
      <fieldset class="mc-setup-part"><legend>Keyboard</legend>${presets}
        ${mac ? `<label class="mc-setup-ql"><input type="checkbox" id="setup-ql"${qlBefore ? ' checked' : ''}> With the Mac-native keys, Space and ⌘Y open Quick Look instead of the built-in viewer (⌘Y opens Quick Look with the other keys too)</label>` : ''}</fieldset>
      <fieldset class="mc-setup-part"><legend>Function keys</legend>
        <p class="mc-setup-fn" id="setup-fn" aria-live="polite"></p>
        ${mac ? '<button type="button" class="setup-keyboard">Open Keyboard settings</button>' : ''}
      </fieldset>
      ${mac ? `<fieldset class="mc-setup-part"><legend>Full Disk Access</legend>
        <p class="mc-note">macOS asks before an app may read some folders (Desktop, Documents, Downloads, other disks…). To see every folder without being asked, add mormiCommander in Privacy & Security > Full Disk Access.</p>
        <button type="button" class="setup-fda">Open Full Disk Access settings</button></fieldset>` : ''}
      ${marta.show ? `<fieldset class="mc-setup-part"><legend>Coming from Marta</legend>
        <p class="mc-note" id="setup-marta" aria-live="polite">${esc(marta.text)}</p>
        ${marta.canImport ? '<button type="button" class="setup-marta">Import from Marta</button>' : ''}</fieldset>` : ''}
      <div class="mc-btns"><button type="button" class="cancel">Later</button><button type="submit" class="primary">OK</button></div>`;
    const fnLine = box.querySelector('#setup-fn');
    say = () => {
      fnLine.textContent = fnAdvice({ pressed, platform, fnKeys });
      fnLine.classList.toggle('mc-setup-ok', pressed);
    };
    say();
    const openPane = (call) => call().catch((e) => app.flash(errText(e), 'err'));
    box.querySelector('.setup-keyboard')?.addEventListener('click', () => openPane(() => go.Setup.OpenKeyboardSettings()));
    box.querySelector('.setup-fda')?.addEventListener('click', () => openPane(() => go.Setup.OpenFullDiskAccess()));
    const martaBtn = box.querySelector('.setup-marta');
    martaBtn?.addEventListener('click', async () => {
      martaBtn.disabled = true;
      const line = box.querySelector('#setup-marta');
      try {
        const added = await go.Setup.ImportMarta();
        line.textContent = martaDone(added, martaInfo.favorites.length);
      } catch (e) {
        line.textContent = errText(e);
        martaBtn.disabled = false;
      }
    });
    box.querySelector('.cancel').onclick = () => done(null);
    box.onsubmit = (ev) => {
      ev.preventDefault();
      qlAfter = !!box.querySelector('#setup-ql')?.checked;
      done(presetName(box.querySelector('input[name="keyPreset"]:checked')?.value));
    };
  }, null, {
    kind: 'first-run-setup',
    // the F5 test: the key is the dialog's own while it is open (also from its dialog window, which passes keys on)
    onKey: (ev) => {
      if (ev.type !== 'keydown' || (ev.code !== 'F5' && ev.key !== 'F5')) return false;
      pressed = true;
      say();
      // the user may have just changed the setting in System Settings: ask again
      if (mac) go.Setup.FnKeys().then((k) => { fnKeys = k; say(); }).catch(() => {});
      return true;
    },
  });
  // shown once by itself: closing it with "Later" or Esc does not bring it back at the next start
  if (!app.setting('setupDone', false)) await app.setSetting('setupDone', true);
  if (answer && qlAfter !== qlBefore) {
    await app.setSetting('quickLookSpace', qlAfter);
    if (answer === current) applyPreset(answer);
  }
  if (answer && answer !== current) {
    await app.setSetting('keyPreset', answer);
    applyPreset(answer);
    app.flash(`Keyboard: ${KEY_PRESETS[answer].label}.`);
  }
  return answer;
}

command({
  name: 'cm_FirstRunSetup', label: 'First-run setup…', menu: 'Configuration', menuAfter: 'cm_ConfigHotkeys',
  run: (app) => showSetup(app),
});

export async function init(app) {
  const name = presetName(app.setting('keyPreset', DEFAULT_PRESET));
  if (name !== DEFAULT_PRESET) applyPreset(name);
  const info = await go.Setup.Info().catch(() => null);
  if (!shouldShowSetup(info, app.setting('setupDone', false))) return;
  // once the panels show their folders, so the page opens over a ready window
  app.on('ready', () => { app.run('cm_FirstRunSetup').catch((e) => console.error(e)); });
}

// Control mode: {"cmd": "setup"} tells the chosen preset and whether the page was shown.
addControlCommand('setup', async () => ({
  ok: true, preset: presetName(theApp.setting('keyPreset', DEFAULT_PRESET)), done: !!theApp.setting('setupDone', false),
}));
