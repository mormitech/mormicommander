// The user's settings, as the page sees them: one object read from Go at start (settings.go), one key written back when
// it changes. Each feature uses its own top-level key ("keymap", "session", "hotlist", ...).
//
// Every window of the app (the file manager, each Lister window) has a copy; Go tells all of them about every change
// (the "setting" event, settings.go), so a change made in one window is seen in the others. onSettingChange(fn) is
// told (key, value) after such a change.

import { go, on } from './go.js';

let data = {};
const watchers = new Set();

on('setting', (key, value) => {
  if (typeof key !== 'string') return;
  if (value === null || value === undefined) delete data[key]; else data[key] = value;
  for (const fn of watchers) { try { fn(key, value); } catch (e) { console.error(e); } }
});

export function onSettingChange(fn) { watchers.add(fn); return () => watchers.delete(fn); }

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
