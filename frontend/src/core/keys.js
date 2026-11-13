// Key combos as text: "F5", "Ctrl+Shift+F5", "Alt+Left", "Num+", "Ctrl+\". The keymap, the menus, the docs and control
// mode all use this one spelling.
//
// The rule (docs/FEATURES.md, "Hotkey rule for macOS"): keys are matched by the physical key (KeyboardEvent.code), so
// Option+letter on a Mac (which types a special character) and non-US layouts such as Hungarian still give "Alt+A" or
// "Ctrl+\". Ctrl is the Control key on every system, Alt is Alt/Option, Cmd is the Mac's Command key. Modifiers are
// always written in the order Ctrl, Alt, Shift, Cmd.

const CODE_NAMES = {
  ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right', PageUp: 'PgUp', PageDown: 'PgDn',
  Home: 'Home', End: 'End', Insert: 'Ins', Delete: 'Delete', Backspace: 'Backspace', Enter: 'Enter', NumpadEnter: 'Enter',
  Tab: 'Tab', Escape: 'Esc', Space: 'Space', Backslash: '\\', IntlBackslash: '<', IntlRo: '\\', IntlYen: '\\',
  NumpadAdd: 'Num+', NumpadSubtract: 'Num-', NumpadMultiply: 'Num*', NumpadDivide: 'Num/', NumpadDecimal: 'Num.',
  Minus: '-', Equal: '=', BracketLeft: '[', BracketRight: ']', Semicolon: ';', Quote: "'", Backquote: '`', Comma: ',',
  Period: '.', Slash: '/', ContextMenu: 'Apps',
};

const KEY_NAMES = { // KeyboardEvent.key → our name, for events without a usable code
  ArrowUp: 'Up', ArrowDown: 'Down', ArrowLeft: 'Left', ArrowRight: 'Right', PageUp: 'PgUp', PageDown: 'PgDn', Insert: 'Ins',
  Escape: 'Esc', ' ': 'Space', ContextMenu: 'Apps', Del: 'Delete', Esc: 'Esc',
};

const MODS = ['Ctrl', 'Alt', 'Shift', 'Cmd'];
const MOD_ALIASES = { ctrl: 'Ctrl', control: 'Ctrl', alt: 'Alt', option: 'Alt', opt: 'Alt', shift: 'Shift', cmd: 'Cmd', command: 'Cmd', meta: 'Cmd', super: 'Cmd', win: 'Cmd' };
const NAME_ALIASES = { del: 'Delete', delete: 'Delete', ins: 'Ins', insert: 'Ins', pageup: 'PgUp', pgup: 'PgUp', pagedown: 'PgDn', pgdn: 'PgDn',
  escape: 'Esc', esc: 'Esc', return: 'Enter', enter: 'Enter', space: 'Space', spacebar: 'Space', backspace: 'Backspace', tab: 'Tab',
  up: 'Up', down: 'Down', left: 'Left', right: 'Right', home: 'Home', end: 'End', apps: 'Apps', menu: 'Apps',
  'num +': 'Num+', 'num -': 'Num-', 'num *': 'Num*', 'num /': 'Num/', plus: '+' };

// keyName is our name of the key of an event, or '' for a lone modifier.
export function keyName(ev) {
  const c = ev.code || '';
  if (/^Key[A-Z]$/.test(c)) return c.slice(3);
  if (/^Digit\d$/.test(c)) return c.slice(5);
  if (/^Numpad\d$/.test(c)) return 'Num' + c.slice(6);
  if (/^F\d{1,2}$/.test(c)) return c;
  if (CODE_NAMES[c]) return CODE_NAMES[c];
  const k = ev.key || '';
  if (['Shift', 'Control', 'Alt', 'Meta', 'AltGraph', 'CapsLock', 'OS', 'Fn', 'Dead', 'Unidentified', ''].includes(k)) return '';
  if (KEY_NAMES[k]) return KEY_NAMES[k];
  if (/^F\d{1,2}$/.test(k)) return k;
  if (k.length === 1) return k.toUpperCase();
  return k;
}

// comboFromEvent is the combo of a key event ("" for a lone modifier). AltGr (Ctrl+Alt on Windows, typed on many
// European layouts for \ @ [ ] and the like) is not taken as Ctrl+Alt (F-290).
export function comboFromEvent(ev) {
  const name = keyName(ev);
  if (!name) return '';
  const altGr = ev.getModifierState?.('AltGraph');
  const mods = [];
  if (ev.ctrlKey && !altGr) mods.push('Ctrl');
  if (ev.altKey && !altGr) mods.push('Alt');
  if (ev.shiftKey) mods.push('Shift');
  if (ev.metaKey) mods.push('Cmd');
  return [...mods, name].join('+');
}

// normalizeCombo spells any way of writing a combo the canonical way: "shift+ctrl+f5" → "Ctrl+Shift+F5". The last part
// is the key; "Ctrl++" is Ctrl with the + key, "Num +" the numpad plus.
export function normalizeCombo(s) {
  if (!s) return '';
  let str = String(s).trim();
  const parts = [];
  // split on + but keep a trailing "+" key (Ctrl++, Num+)
  while (str.length) {
    const i = str.indexOf('+');
    if (i <= 0 || i === str.length - 1) { parts.push(str); break; }
    const head = str.slice(0, i);
    if (/^num\s*$/i.test(head)) { parts.push(str); break; } // "Num+…" is a key, not a modifier
    parts.push(head);
    str = str.slice(i + 1);
  }
  const mods = new Set();
  let key = '';
  for (const p of parts) {
    const low = p.trim().toLowerCase();
    if (MOD_ALIASES[low] && p !== parts[parts.length - 1]) { mods.add(MOD_ALIASES[low]); continue; }
    key = NAME_ALIASES[low] || (/^f\d{1,2}$/i.test(p.trim()) ? p.trim().toUpperCase() : /^num\s*[+\-*/.]$/i.test(p.trim()) ? 'Num' + p.trim().slice(-1) : p.trim().length === 1 ? p.trim().toUpperCase() : p.trim());
  }
  return [...MODS.filter((m) => mods.has(m)), key].join('+');
}

// splitCombo parts a combo into its modifiers and its key ("Ctrl+Num+" → {Ctrl}, "Num+"; "Ctrl++" → {Ctrl}, "+").
export function splitCombo(combo) {
  let c = normalizeCombo(combo);
  const mods = new Set();
  for (let again = true; again;) {
    again = false;
    for (const m of MODS) {
      if (c.startsWith(m + '+') && c.length > m.length + 1) { mods.add(m); c = c.slice(m.length + 1); again = true; }
    }
  }
  return { mods, name: c };
}

const NAME_TO_CODE = Object.fromEntries(Object.entries(CODE_NAMES).filter(([c]) => c !== 'NumpadEnter' && c !== 'IntlRo' && c !== 'IntlYen').map(([c, n]) => [n, c]));
const NAME_TO_KEY = { Up: 'ArrowUp', Down: 'ArrowDown', Left: 'ArrowLeft', Right: 'ArrowRight', PgUp: 'PageUp', PgDn: 'PageDown', Ins: 'Insert',
  Esc: 'Escape', Space: ' ', Apps: 'ContextMenu', 'Num+': '+', 'Num-': '-', 'Num*': '*', 'Num/': '/', 'Num.': '.' };

// eventInit is the KeyboardEvent a combo stands for, so control mode can press keys through the same handler as the
// keyboard (only code, key and the modifiers matter to it).
export function eventInit(combo) {
  const { mods, name } = splitCombo(combo);
  const LONE = { Ctrl: ['Control', 'ControlLeft'], Alt: ['Alt', 'AltLeft'], Shift: ['Shift', 'ShiftLeft'], Cmd: ['Meta', 'MetaLeft'] };
  if (LONE[name]) { // a modifier pressed on its own ("Shift" held to see the shifted function keys)
    mods.add(name);
    return { key: LONE[name][0], code: LONE[name][1], ctrlKey: mods.has('Ctrl'), altKey: mods.has('Alt'), shiftKey: mods.has('Shift'), metaKey: mods.has('Cmd'), bubbles: true, cancelable: true };
  }
  let code = '', key = name;
  if (/^[A-Z]$/.test(name)) { code = 'Key' + name; key = mods.has('Shift') ? name : name.toLowerCase(); }
  else if (/^\d$/.test(name)) { code = 'Digit' + name; }
  else if (/^Num\d$/.test(name)) { code = 'Numpad' + name.slice(3); key = name.slice(3); }
  else if (/^F\d{1,2}$/.test(name)) { code = name; }
  else { code = NAME_TO_CODE[name] || ''; key = NAME_TO_KEY[name] || name; }
  return { key, code, ctrlKey: mods.has('Ctrl'), altKey: mods.has('Alt'), shiftKey: mods.has('Shift'), metaKey: mods.has('Cmd'), bubbles: true, cancelable: true };
}

// label is a combo as the platform writes it in menus and tooltips (macOS: ⌃⌥⇧⌘ symbols for the modifiers).
export function label(combo, platform) {
  const c = normalizeCombo(combo);
  if (platform !== 'darwin') return c;
  const sym = { Ctrl: '⌃', Alt: '⌥', Shift: '⇧', Cmd: '⌘' };
  const parts = c.split('+');
  const key = parts.pop() || '+';
  return parts.map((m) => sym[m] || m).join('') + key;
}

// printable says whether the event types a character (no Ctrl, Alt or Cmd; AltGr is allowed: it types characters).
export function printable(ev) {
  const altGr = ev.getModifierState?.('AltGraph');
  return ev.key?.length === 1 && !ev.metaKey && ((!ev.ctrlKey && !ev.altKey) || altGr);
}
