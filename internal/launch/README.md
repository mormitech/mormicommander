# internal/launch

Starting other programs for the file manager, with no UI: opening a file with its default app, "Open with…",
running programs with parameters and a start folder, the command line below the panels, the external viewer and
editor (F3/F4), a terminal in the current folder, "Reveal in Finder/Explorer", internal file associations and
shortcut files. Standard library plus `golang.org/x/sys/windows` (registry, ShellExecute) only.

Every action is first built as a `Command` (program, arguments or raw Windows parameters, folder, window state) and
then handed to `Launcher.Start`. Each `XxxCommand` method returns the Command without running it, which is what the
tests check for every OS from any machine (set `Launcher.OS` and a fake `Start`). File names are always separate
arguments or carefully quoted; `cmd /c start` is never used.

## API summary

| Area | API |
| --- | --- |
| Setup | `New()`; `Launcher{OS, Start, Output, LookPath, LookupEnv, TempDir, Terminal}`; `Command`; `ShowMode` (`ShowNormal`, `ShowMinimized`, `ShowMaximized`, `ShowHidden`); `Hide(*exec.Cmd)` (HideWindow on Windows, no-op elsewhere) |
| Default app | `OpenDefault(fileOrURL)` — macOS `open`, Windows `rundll32 url.dll,FileProtocolHandler`, Linux `xdg-open` |
| Folder / reveal | `OpenFolder(dir)` (cm_Explorer); `Reveal(path)` — `open -R`, `explorer /select,"…"`, FileManager1 over D-Bus (fallback: parent folder) |
| Open with | `AppsFor(path) []App` (default first; macOS Info.plist + LaunchServices, Linux .desktop + mimeapps.list, Windows registry); `AppsForMIME`; `OpenWith(path, app)`; `OpenWithApp(path, App)`; `OpenWithChooser(path)` (Windows OpenAs dialog, `ErrUnsupported` elsewhere) |
| Print | `Print(path)` — `lp` on macOS/Linux, the "print" verb through PowerShell (path in an env var) on Windows |
| Run | `Run(RunSpec{Program, Params, Dir, Show, Admin, Cleanup, Terminal, KeepOpen})`; documents open with their app, `.app` bundles via `open -a … --args`, console programs get a new console on Windows, `Admin` uses the `runas` verb (Windows) |
| Placeholders | `ExpandParams(params, ParamContext)` → `Expanded{Text, Prompt, ListFiles, HasFile}`: `%P %T %N %M %O %E %S %R %P%S %T%R %L %F %D %UL %UF %WL %WF %Z %Y %1 %% %ENV%`, leading `?`, lowercase = 8.3 names on Windows; `CommandFor(program, params, dir)` |
| Command line | `ParseLine(line, cwd)` → `LineEmpty/LineCd/LineInternal/LineRun` (cd, cd .., cd -, cd ~, cd /d, `D:`, cm_*/em_*, typed folder); `ExpandEnv` (%VAR% everywhere, $VAR/${VAR} on Unix); `RunLine(line, dir, keepOpen, LineAuto/LineTerminal/LineDetached)`; `CaptureLine(ctx, line, dir)`; `CompleteLine(line, cwd)`; `AppendName(line, name)`; `History` (`Add`, `Items`, `Load`/`Store` in `[Command line history]`) |
| Terminal | `OpenTerminal(dir)` (cm_ExecuteDOS): Terminal/iTerm/any app on macOS, Windows Terminal or cmd (or powershell/pwsh), $TERMINAL → desktop terminal → x-terminal-emulator → table of 18 terminals on Linux, or a custom template with `%D`; `RunInTerminal(dir, line, keepOpen)` (cmd /k or /c; self-deleting `.command` script on macOS; `sh -c` with the terminal's exec option on Linux) |
| Viewer / editor | `Tool{Program, Params}`, `ParseTool`, `Tool.String`; `OpenInTool(tool, file, ctx)` (%1 or appended file); `DefaultEditorCommand` (`open -t`, Notepad, default text/plain app or xdg-open); `CreateForEdit(dir, name)` (Shift+F4); `WatchFile(ctx, path, interval)` (saved-file detection for archive/FTP edits) |
| Associations | `LoadAssociations(*INIFile)`, `Associations.Store`, `Find`, `Lookup(name, isDir, verb)` (falls through to later masks), `Association.Command/SetCommand`; `ParseAction` (cm_* or program); `OpenAssociated(assoc, verb, ctx)` → `Result{Handled, Internal, Args}`; verbs `VerbOpen`, `VerbView`, `VerbEdit` and custom ones |
| Masks | `ParseMask`, `Mask.Match(name, isDir)`, `MatchMask` — TC syntax: `*.txt *.md;*.log | excl*`, `*.*`, `*.`, `dir\`, quoted masks, case-insensitive |
| Shortcuts | `ReadShortcut(path)` → `Shortcut{Kind: URL/Path/Command, Target, Args, Dir}` for `.url`, `.webloc` (XML or binary), `.desktop` (Link/Application), `.lnk` (local and network LinkInfo, relative path); `ParseLnk`; `IsShortcutName` |
| Helpers | `ExecKindOf(path)` (GUI vs console from PE/Mach-O/ELF headers); `MIMEType(path)`; `ParseINI`/`INIFile` (UTF-8/UTF-16, comments kept); `ParsePlist` (XML + bplist00); `ParseDesktopEntry`, `ExpandDesktopExec`; `SplitArgs`, `EscapeArg`, `QuoteWindowsArg`, `SplitWindowsArgs`, `ShellQuote`, `QuoteName` |

## FEATURES.md IDs backed

- Section 10 (command line): F-292, F-293, F-294, F-295, F-296, F-297, F-298, F-301 (recognition), F-302, F-303
  (Windows `runas`; `ErrUnsupported` on macOS/Linux), F-812, F-813.
- Section 28 (editor): F-591, F-592 and F-593 (engine side: `WatchFile`), F-595.
- Related: F-051 (Enter opens with the default app), F-153/F-214 (`CreateForEdit`), F-196/F-363/F-371
  (cm_ExecuteDOS), F-199/F-265 (Shift+Enter in a terminal), F-200/F-703/F-708 ("Open with", reveal), F-232/F-335
  (print), F-310 and F-814 (parameter placeholders and the `?` prompt), F-323 (internal associations), F-366
  (cm_Explorer), F-420/F-590 (external viewer/editor configuration), F-738 (smart Enter on shortcut files), F-075
  (shortcut targets).

## Not covered here

The UI wiring (dialogs, the command line widget, the parameter prompt box), cm_* command dispatch, and archive/FTP
re-packing after an edit belong to other packages; this package supplies the pieces they call.
