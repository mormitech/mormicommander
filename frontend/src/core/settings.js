// The user's settings, as the page sees them: one object read from Go at start (settings.go), one key written back when
// it changes. Each feature uses its own top-level key ("keymap", "session", "hotlist", ...).

import { go } from './go.js';

let data = {};

export async function loadSettings() {
  try { data = (await go.Settings.All()) || {}; } catch { data = {}; }
  return data;
}

export const getSetting = (key, def) => (data[key] === undefined || data[key] === null ? def : data[key]);

export function setSetting(key, value) {
  data[key] = value;
  return go.Settings.Set(key, value).catch(() => {}); // (the browser preview has no Go side)
}

export const allSettings = () => ({ ...data });
