# Keyboard

mormiCommander uses Total Commander's keys on every system. The full list of default keys is in
[`frontend/src/core/keymap.js`](../frontend/src/core/keymap.js); Help > Keyboard shortcuts (and the command browser) will
show the keys as they are set, including your changes.

## The rule

1. **Every Total Commander key works the same on macOS, Windows and Linux**, by the physical key: Ctrl is the Control
   key, Alt is Alt (Option on a Mac), Shift is Shift. F5 copies, Ctrl+T opens a tab, Alt+F7 finds files, everywhere.
2. **macOS also has the usual Cmd keys**: Cmd+C / Cmd+X / Cmd+V copy, cut and paste files, Cmd+A select all, Cmd+T and
   Cmd+W tabs, Cmd+Q quit, Cmd+, options, Cmd+Backspace delete to the Trash, Cmd+Shift+. hidden files, Cmd+R reread,
   Cmd+Up parent folder, Cmd+Down open, Cmd+I properties, Cmd+L or Cmd+Shift+G type a path, Cmd+F find files, Cmd+[ and
   Cmd+] back and forward. They run the same commands as the TC keys, which keep working too. Cmd+Z in the file panel
   takes back the last file operation; in text fields and the command line it undoes typing as usual. The Cmd keys
   do not exist on Windows and Linux.
3. **Keys are matched by where they are, not by what they type**, so Option+letter on a Mac (which types a special
   character) and other layouts such as Hungarian still work. On European keyboards Ctrl+< (the key next to the left
   Shift, í on a Hungarian keyboard) goes to the root, like Ctrl+\\. AltGr (on Windows and Linux) types characters and is
   never taken for Ctrl+Alt.
4. **Keys macOS keeps for itself:** Ctrl+Up / Ctrl+Down (Mission Control), Ctrl+Left / Ctrl+Right (Spaces), Ctrl+F2 /
   Ctrl+F3 (menu bar and Dock focus), F11 (show desktop), unless you turn them off in System Settings > Keyboard >
   Keyboard Shortcuts. The alternatives: Cmd+Option+Left / Cmd+Option+Right show the folder under the cursor in the
   left / right panel, Cmd+Option+Up opens it in a new tab. The function key bar at the bottom always works with the
   mouse.
5. **Function keys on a Mac laptop** need fn (fn+F5), unless "Use F1, F2, etc. keys as standard function keys" is on in
   System Settings > Keyboard.
6. **No numpad, no Ins key:** Space selects like Ins; Shift+= / Shift+- / Shift+8 work as Num+ / Num- / Num* (select
   group, unselect group, invert selection) while the command line is empty. They do whatever Num+ / Num- / Num* are
   set to; binding Shift+= (or the others) to a command of its own replaces the stand-in.
7. **Changing keys:** settings.json has a "keymap" object, for example `{"Ctrl+K": "cm_Copy", "F2": ""}` ("" takes a key
   away). Configuration > Keyboard shortcuts will edit it; "Restore defaults" brings back the TC keys.

## Typing

As in Total Commander, the keyboard stays with the file panel. Letters you type go into the command line under the
panels; Enter runs the command line when it has text (and opens the file under the cursor when it is empty); Backspace,
Left, Right and Delete edit it while it has text; Esc clears it. `cd folder` in the command line changes the folder of
the active panel; a command name such as `cm_GoToParent` runs that command.
