# internal/config

Settings engine for mormiCommander. It reads and writes Total Commander style
`wincmd.ini` files without losing comments, blank lines, key order or unknown
sections, and maps them onto typed Go models that the Configuration dialog,
the hotlist, the Start menu, the button bar and the hotkey system can use
directly. UI-free; standard library only.

## API summary

| Area | Main API | File |
|---|---|---|
| INI reader/writer (comment/order preserving, duplicate sections, CRLF, encodings) | `File`, `ParseString`, `Parse`, `LoadFile`, `(*File).Get/Set/GetInt/GetBool/Delete/Entries/ReplaceSection/RenameSection/Save` | ini.go, encoding.go |
| Encodings (UTF-8 with/without BOM, UTF-16 LE/BE, Windows-1252/1250 fallback) | `Decode`, `Encode`, `Encoding` | encoding.go |
| Atomic save (temp file + fsync + rename, keeps mode, follows symlinks, optional .bak) | `WriteFileAtomic`, `WriteFileAtomicBackup` | atomic.go, sync_unix.go, sync_other.go |
| Settings file location (user config dir, portable mode, `/i=` `/f=` overrides) | `Locate`, `LocateOptions`, `Locations`, `ParseConfigArgs` | paths.go |
| Whole configuration (open, sync, save, reload, export/restore) | `Open`, `(*Config).Save/Reload/Export/Restore/Expand/LoadButtonBar/LoadUserCommands/ActiveColors` | config.go |
| Typed Configuration dialog model (one struct per page, field metadata, search) | `Settings`, `Defaults`, `LoadSettings`, `(*Settings).Save/Get/Set/Diff`, `Fields`, `Pages`, `LookupSetting`, `SearchSettings` | settings.go |
| `%COMMANDER_PATH%`, `%COMMANDER_INI%` and environment variable expansion | `Vars`, `DefaultVars`, `(Vars).Expand/Contract` | expand.go |
| Numbered menus (`menuN`/`cmdN`, `-` submenus, `--` end, separators) | `MenuItem`, `MenuPath`, `ParseNumberedMenu`, `NumberedMenuKVs`, `MenuInsert/Remove/Move/Walk`, `Accelerator` | menu.go |
| Directory hotlist (nested submenus, separators, target paths, filter) | `Hotlist`, `LoadHotlist`, `(*Hotlist).Save/AddFolder/Insert/Remove/Move/Filter/Action`, `ParseHotlistCommand` | hotlist.go |
| Folder / command-line / dialog histories, frequently used folders | `History`, `NewHistory`, `Frequent`, `NewFrequent` | history.go |
| Custom column sets and column templates | `ColumnSet`, `Column`, `LoadColumnSets`, `SaveColumnSets`, `AutoColumnSet`, `ParseTemplate`, `RenderTemplate`, `BuiltinFields` | columns.go |
| User Start menu (with per-item hotkeys) | `StartMenu`, `LoadStartMenu`, `(*StartMenu).Save/Hotkeys/Action`, `NewStartItem` | startmenu.go |
| Button bar `.bar` files, sub-bars, clipboard format | `Bar`, `Button`, `ParseBar`, `LoadBarFile`, `(*Bar).SaveBarFile/Insert/Remove/Move`, `ParseButtonClipboard`, `ButtonForFile`, `DefaultBar` | buttonbar.go |
| Button / Start menu parameter placeholders (`%P %N %S %L ...`) | `ExpandParams`, `ParamContext`, `WriteListFile` | params.go |
| User commands (`usercmd.ini`, `em_*`) | `UserCommand`, `LoadUserCommands`, `SaveUserCommand`, `FindUserCommand` | usercmd.go |
| Hotkey overrides (`[Shortcuts]`, macOS Cmd mapping) | `Hotkey`, `ParseHotkey`, `Keymap`, `LoadKeymap`, `(*Keymap).Bind/Unbind/Reset/Effective`, `KeysFor` | keymap.go |
| Colour schemes, file-type colours, dark mode | `Color`, `ColorScheme`, `LoadColorScheme`, `SaveColorScheme`, `NamedSchemes`, `BuiltinSchemes` | colors.go |
| TC file masks (`*.exe *.com | *.bak`, folder masks) | `Mask`, `ParseMask`, `MatchMask` | mask.go |
| View modes, tooltip rules, external tools, ignore list | `ViewMode`, `TooltipRule`, `ExternalTool`, `IgnoreList` and their Load/Save | lists.go |
| Folder tabs, `.tab` files, favourite tab sets, panel state, window position | `TabSet`, `TabFile`, `FavoriteTabs`, `PanelState`, `WindowPos` | tabs.go |
| Custom main menu `.mnu` files | `ParseMenuFile`, `FormatMenuFile` | menufile.go |
| Import of a real TC `wincmd.ini` (+ `.bar`, `usercmd.ini`, path rewriting) | `ImportTC`, `ImportOptions`, `AllImport`, `ImportReport` | importtc.go |
| Path comparison per platform | (internal) | pathcase_windows.go, pathcase_other.go |

## FEATURES.md IDs backed

- Section 17 (Configuration dialog pages): F-412 to F-440, F-444 to F-447, F-820 to F-828.
- Section 18 (Start menu): F-448, F-449, F-450.
- Section 31 (Directory hotlist and history): F-652, F-653, F-655, F-656, F-658, F-660 to F-664, F-849.
- Section 32 (Custom columns and column sets): F-665 to F-668, F-674.
- Section 11 (Button bar files, parameters, user commands): F-8, F-304, F-306, F-308 to F-311, F-313, F-314, F-814.
- Section 24 (Find files history/settings): F-16.

## Testing

```
go vet ./internal/config/
go test ./internal/config/
GOOS=windows go build ./internal/config/...
GOOS=linux go build ./internal/config/...
```

All tests use `t.TempDir()`; the TC import test uses an in-test Windows-1252,
CRLF fixture.

Values with CR or LF cannot be stored: `Set` ignores them (`SafeValue` tells), histories skip them, and rendering turns any left in `ReplaceSection` data into spaces, so a folder name with a newline can never add INI lines.
