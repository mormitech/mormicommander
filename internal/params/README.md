# internal/params — command lines, placeholders, start-up switches

UI-free engine for everything that turns typed text into an action, Total
Commander style. Standard library only. Nothing here starts a process: the
functions return descriptions (`Startup`, `Launch`, `Action`) that the app
layer carries out. All path and quoting rules are chosen by `Style`
(`StyleWindows` / `StylePOSIX`, zero = the build OS), so Windows behaviour is
tested on macOS too.

Backs: **F-292, F-293, F-294, F-297, F-298, F-301, F-302, F-812, F-813**
(command line, section 10), **F-295** (history logic), **F-296**
(completion), **F-305, F-307, F-310, F-311 (em_ detection), F-814** (button
bar, section 11), **F-448, F-449, F-450** (start menu tree, section 18),
**F-728 – F-734, F-852** (start-up parameters, section 41; F-735 single
instance uses `Startup.Absolute` + `Startup.Args` to forward to the running
window).

## Start-up parameters (main.go)

```go
s, err := params.ParseStartup(os.Args[1:]) // unknown "-" flags land in s.Unknown
if s.OpenInRunning && !s.NewInstance && otherInstanceRunning() {
    forward(s.Absolute(cwd, params.DefaultStyle).Args()) // re-parse there
}
left, right := s.Panels(activeIsRight) // "" = keep that side
// s.NewTab, s.ActivePanel ('L'/'R'), s.IniPath, s.FtpIniPath,
// s.Tool (ToolLister/ToolCompare/ToolSync) + s.Paths + s.Lister options
```

| Switch | Meaning |
|---|---|
| `a b` | left / right folder (positional) |
| `/L=` `/R=` (`--left=` `--right=`) | left / right folder |
| `/O` | send to the running instance |
| `/N` | always a new instance |
| `/T` | open in new tab(s) |
| `/S` | paths are source/target, not left/right |
| `/P=L` `/P=R` | active panel |
| `/I=` `/INI=` `/F=` | main ini / FTP ini |
| `/S=L[:opts]` | Lister only; opts `A S V N T1..T7 C<cp> P<plugin>` |
| `/S=C`, `/S=S` | (ours) Compare only, Sync only |

Switches accept `/`, `-` or `--` and any case. A `/` token counts as a switch
only if it is exactly a known switch (POSIX absolute paths start with `/`).
A trailing `"` left by Windows argv splitting (`"C:\dir\"`) is removed.

## Buttons and start-menu entries

```go
l, err := params.PrepareButton(params.Button{Command, Params, StartPath, Iconic}, params.Context{
    Source: params.Panel{Path, Cursor: params.Entry{Name, Dir}, Selected: []params.Entry{...}},
    Target: ..., SourceIsRight: ..., Ask: askUser, Dropped: droppedFiles,
})
if l.Canceled { return }
if l.Prompt { edited, ok := showEditBox(l.Line); if !ok { l.Cleanup(); return }; l.SetLine(edited) }
if l.Internal { runCommand(l.Program, l.Args) } else { start(l.Program, l.Args, l.Dir, l.Show, l.Elevate) }
// after the process exits: l.Cleanup() (deletes list files)
```

`Expand(params, ctx)` / `ExpandPath(field, ctx)` are the lower-level pieces.

| Placeholder | Value |
|---|---|
| `%P` / `%T` | source / target folder with trailing separator |
| `%N` / `%M` | name under cursor in source / target |
| `%O` / `%E` | cursor name without extension / extension only |
| `%S` / `%R` | selected names in source / target (cursor item if none) |
| `%P%S`, `%T%R`, `x%S.y` | glued text repeats for every name |
| `%L` `%l` `%D` | list file, full paths (UTF-8) |
| `%F` `%f` `%d` | list file, names only (UTF-8) |
| `%UL` `%UF` | list file UTF-8; `%WL` `%WF` UTF-16LE + BOM |
| `%Q` | anywhere: no automatic quoting |
| `%X` `%Y` `%Z` | following placeholders use left panel / right panel / back to source-target |
| `%[caption;default]` | ask the user (ctx.Ask) |
| `%NAME%`, `%$DESKTOP%` | environment / special folder |
| `%%` | a literal `%` |
| leading `?` | show the expanded text for editing first (F-814) |
| command `*prog` | run elevated (Windows) |
| `Iconic` 1 / -1 | run minimised / maximised (`ShowModeFromIconic`) |

Names with spaces are quoted (Windows: `"…"`, POSIX: `'…'` for any unsafe
character); text glued to a placeholder is quoted with it (`"%O.zip"`), and
placeholders already inside user quotes are escaped instead. List files end
lines in CRLF on Windows style; folders get a trailing separator.

### Editor / viewer / compare tool templates

`ExpandFileCommand(template, files, ctx)` expands the parameters of the F4
editor, the external F3 viewer, the external compare tool and associations:
`%1`, `%2` … are the files (quoted as needed, raw inside user `"…"` or with
`%Q`); all button placeholders still work; when the template references no
file (no `%1..%9`, `%N`, `%S`, `%L`, `%F`, `%D`, `%UL`, `%WL` …) the files
are appended quoted, as TC does.

`SplitFileTarget(path)` → `(dir, cursor)`: a start-up path naming an
existing file opens its folder with the cursor on that file.

`BuildMenu` / `FlattenMenu` convert TC's flat `[user]` list (`-Sub`, `--`,
`-`) to a tree and back.

## Command-line box

```go
a, err := params.ParseLine(text, params.LineContext{Cwd, Home}, shiftEnter)
switch a.Kind {
case params.ActChangeDir:   // a.Path (""=stay), a.Back, a.Filter, a.ClearFilter
case params.ActInternal:    // a.Command (cm_/em_), a.Args
case params.ActOpenTerminal:// "cmd"
case params.ActRun:         // a.Command+a.Args, or a.Shell with a.Line; a.KeepOpen, a.Elevate, a.Dir
}
```

Understands `cd x`, `cd ..`, `cd..`, `cd \`, `cd /`, `cd /d D:\x`, `D:`,
`cd ...` (two up), `cd ~`, `cd` (home on POSIX), `cd -` (back),
`cd *.txt` (filter), `cd *` / `cd *.*` (clear filter), `cd dir\*.doc`,
UNC and `ftp://` paths, `%VAR%` / `$VAR`. Lines with pipes, redirects,
`&&`, `;`, `$(...)` or POSIX globs run through the shell.

Helpers: `InsertName` (Ctrl+Enter), `Complete` (path completion), `History`
(`Add`, `Items`, `Older` = Ctrl+E, `Newer` = Ctrl+X), `Quote`, `SplitArgs`,
`JoinArgs`, `ExpandEnv`, `SpecialFolder`, `JoinPath`, `CleanPath`.

Note: FEATURES.md F-310 lists "%Z list file UTF-8"; TC's UTF-8 lists are
`%UL`/`%UF`, and `%Z` here returns to source/target after `%X`/`%Y`.
