# Coming from Marta

This page is for people moving from [Marta](https://marta.sh), the free macOS file manager, to mormiCommander. It covers
bringing your favourite folders over, the Marta-like keyboard preset, and where the everyday actions are. Nothing on
this page changes Marta: mormiCommander only reads Marta's settings, and Marta keeps working as before.

## Bringing your favourites over

When Marta's settings folder (`~/Library/Application Support/org.yanex.marta`) is on your Mac, the first-run setup page
shows a **Coming from Marta** box with an **Import from Marta** button. It copies your Marta favourites into the folder
hotlist (Ctrl+D), in a submenu called **Marta**.

- The setup page opens by itself on the first start; you can open it again at any time with Configuration > First-run
  setup….
- The favourites are read from `favorites.marco`, and from a `favorites` block in `conf.marco` if you have one. Every
  folder written as an absolute path, a `~/` path or a `file://` address is taken, with its name when Marta has one.
- Folders that are already in your hotlist are skipped, so pressing the button twice adds nothing new.
- Marta's files are only read: nothing in Marta's folder is written, moved or deleted.
- The imported entries are ordinary hotlist entries: rename, move or delete them in Ctrl+D > Configure….

## The Marta-like keyboard preset

The setup page's **Keyboard** box has a **Marta-like** preset. Like every preset it keeps all of Total Commander's
keys (F3–F8, Tab, Ctrl+arrows…) and adds a few Mac keys on top. Your own changes in Configuration > Keyboard shortcuts
always win over a preset.

| Key | Does |
| --- | --- |
| ⌘O | Open the file or folder under the cursor |
| ⇧⌘N | New folder |
| ⌘Y | Show the file (the viewer) |
| ⌘K | Connect to a server (FTP, SFTP, WebDAV) |
| ⌥⌘⌫ | Delete at once, without the Trash |
| ⌘E | Eject the disk |

Space and Insert still select files, and Shift with the arrows selects too. If you would rather have Space show the
file the way Finder does, pick the **Mac-native** preset instead.

## Where the everyday actions are

The keys below work in every preset. The first column says what you want to do; the second is the mormiCommander key
(on a Mac laptop the function keys may need fn, see [keyboard.md](keyboard.md)).

| What you want to do | mormiCommander |
| --- | --- |
| Open a file or folder | Enter, ⌘↓ |
| Go to the parent folder | Backspace, ⌘↑ |
| Back / forward in the folder history | ⌘[ / ⌘] |
| Type a path to go to | ⌘L, ⇧⌘G |
| Switch to the other panel | Tab |
| Swap the two panels | Ctrl+U |
| Favourite folders | Ctrl+D (the folder hotlist) |
| New tab / close tab / next tab | ⌘T / ⌘W / Ctrl+Tab |
| View a file | F3 |
| Edit a file | F4 |
| Copy to the other panel | F5 |
| Move to the other panel, or rename | F6 |
| Rename in place | Shift+F6 |
| New folder | F7 (⇧⌘N in the Marta-like preset) |
| Delete (to the Trash by default) | F8, ⌘⌫ |
| Delete at once | Shift+F8, Shift+Delete |
| Select / unselect | Space, Insert, Shift+arrows |
| Select all | ⌘A |
| Filter the list as you type | Ctrl+S |
| Find files | ⌘F, Alt+F7 |
| Show hidden files | ⇧⌘. |
| Reload the folder | ⌘R |
| Copy the full path of the selected files | ⌥⌘C |
| Properties | ⌘I |
| Rename many files at once | Ctrl+M |
| Compare the two folders | Shift+F2 |
| Every command, by name | ⇧⌘P (the command palette) |
| Settings | ⌘, |

Marta's Lua plugins and themes have no counterpart to import. The command palette (⇧⌘P) finds any command by name and
shows its key, which is the quickest way to learn the rest.
