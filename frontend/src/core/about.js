// The About window (F-453): the app started with --about shows only this. Links open in the system's browser.

import { go } from './go.js';
import { esc } from './dialogs.js';
import logo from '../assets/mormitech-mark.svg';

const REPO = 'https://github.com/mormitech/mormicommander';
const COFFEE = '<svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="vertical-align:-2px;margin-right:4px"><path d="M4.5 9.5h12v5a5 5 0 0 1-5 5h-2a5 5 0 0 1-5-5zM16.5 11h1.5a2.5 2.5 0 0 1 0 5h-1.8M8 3.5c-.9 1 .9 1.8 0 2.8M12 3.5c-.9 1 .9 1.8 0 2.8"/></svg>';

export async function aboutHTML() {
  const v = await go.App.Version().catch(() => 'dev');
  return `<img class="logo" src="${logo}" alt="" width="120">
    <h1><span class="wordmark"><span class="wm-mormi">mormi</span><span class="wm-commander">Commander</span></span></h1>
    <p class="muted">Version ${esc(v)}</p>
    <p>A two-panel file manager for macOS, Windows and Linux, in the spirit of Total Commander.</p>
    <p class="muted">Free and open source, MIT license. No accounts, no ads, no telemetry.<br>Copyright © 2026 Mormitech and the mormiCommander contributors.</p>
    <p class="links"><a href="${REPO}">Website and source code</a><a href="${REPO}/issues">Report a problem</a><a href="${REPO}/blob/main/LICENSE">License</a><a href="https://mormitech.com/support-us/">${COFFEE}Buy me a coffee</a></p>
    <p class="muted" style="font-size:12px">Total Commander is a trademark of Christian Ghisler. mormiCommander is not related to it.</p>`;
}

export async function showAbout() {
  document.body.classList.add('about-mode');
  const el = document.getElementById('about');
  el.hidden = false;
  el.innerHTML = await aboutHTML();
  window.addEventListener('keydown', (ev) => { // the About window has no menu: Esc and Cmd+W (Ctrl+W) close it
    if (ev.key === 'Escape' || ((ev.metaKey || ev.ctrlKey) && (ev.key === 'w' || ev.key === 'W'))) { ev.preventDefault(); go.App.CloseWindow(); }
  });
}
