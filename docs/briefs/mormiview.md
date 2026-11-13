# Brief from the mormiView session (conventions to copy)

All paths under /Users/jpzwei/Code/mormiview.

## Notes for mormiCommander agents (from the orchestrator)

- Péter explicitly asked for a QA agent for this project, so QA through **control mode** (HTTP commands that act directly in the app) is wanted here. Still: never simulate OS mouse/keyboard input, and never kill app instances you did not start (no blanket `pkill`).
- Version bumps: feature branches do NOT bump the version or edit the CHANGELOG version header (parallel branches would conflict). They add their lines under `## Unreleased`. The integrator bumps the minor version in wails.json once per round and turns `## Unreleased` into a dated `## x.y.z – YYYY-MM-DD` section.
- Releases, PRs to GitHub, pushing, packaging uploads: never. Local commits only.

## (1) UI

- Theming: CSS custom properties on :root in frontend/src/style.css (top ~15 lines: --bg --panel --line --text --muted --accent --accent-text --btn --bad --view --bar, plus wordmark --wm-mormi/--wm-product). Dark mode = `@media (prefers-color-scheme: dark)` redefining the same tokens; `color-scheme: light dark`. A `--theme light|dark` flag forces it via Wails mac Appearance / windows.Options.Theme (main.go macAppearance/windowsTheme), for screenshots. Two-tone wordmark: "mormi" in --wm-mormi, product name in --wm-product (purple). Logo inverts in dark (`filter:invert(1)`).
- Dialogs: frontend/src/dialogs.js + dialogs.css. One `modal(build, cancelValue)` promise helper (form box, role=dialog, Esc = cancel, captures keydown so app shortcuts don't fire, restores focus, autofocus/.primary). `messageDialog` mirrors Wails' MessageDialog shape (Title/Message/Buttons/DefaultButton/CancelButton) so web and desktop share call sites; `infoDialog` for About-like boxes. Classes prefixed `mv-` (use your own prefix, e.g. `mc-`).
- Tooltips: frontend/src/tooltip.js `initTips()` — any [title] shows a custom dark bubble after 1 s (title moved to data-tip while shown); native tooltips not used.
- Menus: native Wails menu built in Go, about.go `appMenu()`: macOS gets an app menu (About/Hide/Quit), File/Edit/…, `menu.WindowMenu()`, and Help titled "Help​" (zero-width space, else macOS injects its Search field). Win/Linux: Help menu with F1 + About. Shortcuts via keys.CmdOrCtrl. Menu items stored on App and enabled/disabled via an update function; menu_bar_test.go tests the menu structure. Page-side menu actions just `app.emit("menu-xyz")`.
- Keyboard: one window keydown listener in frontend/src/main.js; dialogs capture and stopImmediatePropagation.
- Window state: winstate.go — window.json {x,y,w,h} in configDir, atomic write (tmp+rename), min size check, onScreen check for vanished monitors, not saved when fullscreen/maximised/minimised; loaded in main.go before wails.Run, saved in OnBeforeClose.
- Wails v2 = one window per process: new windows/About are spawned as new processes of the same binary (`--about`, `--cascade N` to offset).

## (2) Go side

- app.go: single `App` struct bound via Bind; exported methods are the JS API; mutex `mu`; heavily commented fields. Files split by feature (one feature = one .go + _test.go).
- configdir.go: `os.UserConfigDir()/mormiview`; `--demo/--mock` uses a fresh temp dir so screenshots never show personal settings. `underHome()` to avoid leaking usernames.
- reexec.go + reexec_darwin.go/reexec_other.go: on macOS, if started through a symlink (Homebrew bin), syscall.Exec the real path (env guard) — otherwise no bundle identity and Open/Save dialogs collapse. packaging/macos/<app>: shell launcher the cask links, does `open -a <app>`.
- Platform splits by filename suffix: *_windows.go / *_other.go (hide_windows.go sets HideWindow on helper exec.Cmd so no console flashes), windowid_darwin.go. Build tag `webkit2_41` on Linux.
- Testing: go vet + go test in CI on all 3 OSes; logic lives in Go so it's testable; tests never exec the test binary (window spawning is stubbed).
- Control mode (control.go, docs/control.md, scripts/mormiview_ctl.py): `--control PORT` (0=free) → 127.0.0.1 HTTP JSON, bearer token in a 0600 control file in TMPDIR, refuses Origin/foreign Host; system dialogs answered from a queue. Used for demo screenshots (scripts/demo_screenshots.py, `screencapture -l <windowId>`). Never synthetic OS input; commands act directly.
- macOS launches the app hidden when opened with a file → call wruntime.WindowShow + Show at startup/domready/open-file. macOS also delivers argv files as OnFileOpen — guard against spawning a window for it (a fork bomb happened once). `<APP>_TRACE=file` timeline tracing.

## (3) Build/release

- Version only in wails.json info.productVersion; dated CHANGELOG sections `## x.y.z – YYYY-MM-DD` (en dash) with ### Added/Changed/Fixed.
- CI: .github/workflows/build.yml — matrix mac/ubuntu/windows; npm ci && npm run build (dist must exist before go vet/test because of embed), go vet, go test, wails build; tag v* must match wails.json; packages: mac zip + dmg (ad-hoc codesign), Windows Inno Setup + zip, Linux tar.gz + .deb/.rpm; release on tag. Copy almost verbatim.
- Vendors: scripts/make-cask.mjs, make-scoop-manifest.mjs, make-winget-manifest.mjs.
- PRs titled "<What, in plain words> (x.y.z)", squash-merged.

## (4) Wording/style

- "image", never "picture" in UI text. Brand: "mormi" lowercase, product word capitalised: **mormiCommander** in display, `mormicommander` for identifiers/files/package IDs.
- Plain, everyday English in UI and comments; "…" (ellipsis char) on menu items that open dialogs; Cmd on Mac / Ctrl elsewhere spelled out per platform in help text.
- No username in screenshots (use /tmp paths).
- Code comments are full plain-English sentences explaining why; Go files open with a paragraph comment describing the feature.

Best files to copy: main.go, about.go (menu), winstate.go(+test), configdir.go, reexec*.go, hide_*.go, control.go, frontend/src/{style.css top, dialogs.js/css, tooltip.js}, frontend/vite.config.js, wails.json, .github/workflows/build.yml, packaging/*, scripts/make-*.mjs, CHANGELOG.md header.
