// Options > Misc: Beep on errors, Play a sound when long operations finish, and Check for updates (board issue #94).
// The sounds are made in the page (Web Audio), so nothing is played from a file. "Check for updates" never lets the
// program itself go online (docs/privacy.md): once a week at start it offers to open the releases page in the browser.

import { go, on as onGo } from '../../core/go.js';
import { messageDialog } from '../../core/dialogs.js';
import { on } from './util.js';

let audio = null;
// tone plays short notes: [[frequency, start s, length s], …]
function tone(notes, volume = 0.08) {
  try {
    audio ||= new (window.AudioContext || window.webkitAudioContext)();
    const t0 = audio.currentTime;
    for (const [f, at, len] of notes) {
      const o = audio.createOscillator(), g = audio.createGain();
      o.type = 'sine';
      o.frequency.value = f;
      g.gain.setValueAtTime(0, t0 + at);
      g.gain.linearRampToValueAtTime(volume, t0 + at + 0.01);
      g.gain.exponentialRampToValueAtTime(0.0001, t0 + at + len);
      o.connect(g).connect(audio.destination);
      o.start(t0 + at);
      o.stop(t0 + at + len + 0.02);
    }
  } catch { /* no sound here */ }
}
export const beep = () => tone([[220, 0, 0.18]], 0.1);
export const chime = () => tone([[660, 0, 0.25], [880, 0.12, 0.35]]);

const LONG_JOB_MS = 10000; // a job that worked this long gets the sound when it ends
const WEEK = 7 * 24 * 3600 * 1000;

export function initSounds(app) {
  const flash = app.flash.bind(app);
  app.flash = (msg, kind = '') => {
    if (kind === 'err' && on('Misc.BeepOnError')) beep();
    return flash(msg, kind);
  };
  onGo('job-done', (p) => { if (on('Misc.SoundOnDone') && (p?.elapsed || 0) >= LONG_JOB_MS) chime(); });
  app.on('ready', async () => {
    if (!on('Misc.CheckUpdates') || app.demo) return;
    const last = Number(app.setting('updateCheckAsked', 0)) || 0;
    if (Date.now() - last < WEEK) return;
    await app.setSetting('updateCheckAsked', Date.now());
    const b = await messageDialog({
      Title: 'Check for updates', Kind: 'check-updates',
      Message: 'mormiCommander does not go online by itself. Open the releases page in your browser to see whether there is a newer version?',
      Buttons: ['Open the releases page', 'Not now'],
    });
    if (b === 'Open the releases page') app.run('cm_CheckForUpdates');
  });
  return go;
}
