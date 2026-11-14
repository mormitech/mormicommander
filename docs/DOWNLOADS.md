# Downloads

This is the canonical download page of mormiCommander. **The only official files are the ones attached to the
[GitHub releases of mormitech/mormicommander](https://github.com/mormitech/mormicommander/releases)**, and the package
managers below install exactly those files (their checksums are computed from the published files by
`scripts/make-cask.mjs`, `scripts/make-scoop-manifest.mjs` and `scripts/make-winget-manifest.mjs`). mormiCommander has
no other download site, no mirror and no "pro", "unlocked" or "portable" edition anywhere else. Copies of file managers
have been used to spread malware before, so if you got the app from somewhere else, check it against this page or
download it again from the releases.

The first release, v0.9.0, came out before the checksum file was added, so it has no `SHA256SUMS.txt`; every release
after it has one. You can always build the app from source instead (see the [README](../README.md)).

## Which file

Each release `vX.Y.Z` has these files (X.Y.Z is the version):

| System | File | What it is |
| --- | --- | --- |
| macOS (Apple silicon) | `mormicommander-X.Y.Z-macos-arm64.zip` | The app; unzip and move `mormicommander.app` to Applications |
| Windows (x64) | `mormicommander-setup-X.Y.Z-windows-x64.exe` | Installer (per user, no admin rights needed) |
| Windows (x64) | `mormicommander-X.Y.Z-windows-x64.zip` | `mormicommander.exe` without an installer |
| Debian, Ubuntu (x64) | `mormicommander_X.Y.Z_amd64.deb` | `sudo apt install ./mormicommander_X.Y.Z_amd64.deb` |
| Linux (x64) | `mormicommander-X.Y.Z-linux-x64.tar.gz` | The binary, a desktop entry and `install.sh` |
| All | `SHA256SUMS.txt` | The SHA-256 checksum of every file above |

## With a package manager

These will install and update it from Mormitech's own tap and bucket once mormiCommander is added there (it is not
yet; until then, use the files above):

```
brew install mormitech/tap/mormicommander       # macOS and Linux
scoop bucket add mormitech https://github.com/mormitech/scoop-bucket
scoop install mormitech/mormicommander          # Windows
winget install mormitech.mormicommander        # Windows
```

## Check a download

`SHA256SUMS.txt` is made by the release workflow (`.github/workflows/build.yml`) from the files it builds, in the same
run that uploads them. Download it together with the file you want, put both in one folder, and run:

macOS:

```
shasum -a 256 -c SHA256SUMS.txt --ignore-missing
```

Linux:

```
sha256sum -c SHA256SUMS.txt --ignore-missing
```

Windows (PowerShell), then compare the result with the line for that file in `SHA256SUMS.txt`:

```
Get-FileHash .\mormicommander-setup-X.Y.Z-windows-x64.exe -Algorithm SHA256
```

"OK" (or the same hash) means the file is the one the release built. Anything else: do not run it, and please
[tell us](https://github.com/mormitech/mormicommander/issues).

## Not signed yet

The first releases are not signed with a paid certificate. macOS asks before opening a downloaded app (right-click >
Open the first time; the Homebrew cask clears the quarantine flag), and Windows SmartScreen may warn about an unknown
publisher. The checksum above is how you know the file is ours.
