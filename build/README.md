# Build files

What `scripts/build.sh` puts into the programs (Wails v3 has no build step of its own here; the script calls `go build`):

* `appicon.png` - the icon, 1024 x 1024. The macOS bundle's `iconfile.icns` is made from it at build time, and the
  Linux window and the .deb icons use it too.
* `darwin/Info.plist` - the Info.plist of `build/bin/mormicommander.app`; the `@VERSION@`-style values are filled in
  from `wails.json`, the one place of the version.
* `windows/icon.ico`, `windows/wails.exe.manifest` - the icon and the manifest linked into `mormicommander.exe` (through
  a `.syso` file made by `wails3 generate syso`, with the file properties from `wails.json`).
* `bin/` - the output (not committed).
