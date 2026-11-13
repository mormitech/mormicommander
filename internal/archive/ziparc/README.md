# internal/archive/ziparc

The built-in ZIP packer (Total Commander's internal ZIP). Importing the
package registers format `"zip"` with `internal/archive`, so the UI normally
only uses `archive.Open`, `archive.Lookup("zip").Packer` and the generic
functions. It needs no external tools.

## API

- `Open(path, archive.OpenOptions) (*Reader, error)` opens a zip, any part of
  a split set (`x.z01`, `x.zip`), or numbered parts (`x.zip.001`).
- `OpenReaderAt(r, size, name, opts)` opens a zip held in memory or inside
  another archive.
- `*Reader` implements `archive.Archive` and `archive.Sectioner`, plus
  `Volumes()`, `Spanned()` and `SetPassword(pw)` (for example after the user
  typed it in the Pack dialog).
- `Packer{}.Update(ctx, path, archive.Changes, archive.PackOptions)` creates
  or updates archives (see `archive.Packer`).
- `Format()`, `Extensions`, `IsZip(path)`, `MinVolumeSize`.

## What is supported

- Reading: Store, Deflate, BZip2. Deflate64, LZMA, PPMd, Zstandard and XZ
  entries are listed but `Open` returns `ErrUnsupportedMethod` with the
  method name.
- Encryption: ZipCrypto read and write (Explorer and macOS compatible), and
  WinZip AES-128/192/256 (AE-1/AE-2) read and write, with HMAC checking. PKWARE
  "strong encryption" is reported as unsupported. Updating an archive copies
  untouched encrypted entries raw, so no password is needed for that.
- Names: UTF-8 flag, Info-ZIP Unicode path/comment fields, unflagged UTF-8
  (macOS), and legacy code pages through `OpenOptions.Encoding`. New and
  renamed entries are written as flagged UTF-8, and untouched legacy names
  keep their bytes.
- Metadata: DOS, Unix (0x5455) and NTFS (0x000a) timestamps, Unix permissions
  and symlinks, DOS attributes, per-entry and archive comments.
- Zip64 end records and sizes (more than 65535 entries, more than 4 GB),
  self-extracting archives and other data in front of the zip, and `.jar`,
  `.apk`, `.docx`, `.epub` and similar files by extension or content.
- Multi-volume: `PackOptions.VolumeSize` writes `x.z01 ... x.zip` the way
  Info-ZIP/WinZip/TC do: spanning signature, headers never split across parts,
  per-part offsets in the central directory. These sets are read back, and
  Info-ZIP `zip -s` sets are read too. Spanned sets are read-only for
  `Update` (`ErrReadOnly`), the same as in TC. An archive that fits in one
  volume stays a normal single file.
- Levels 1-9 (0 means 6), Store or Deflate.

## Tests

`go test ./internal/archive/...` creates everything in `t.TempDir()`. When
installed, Info-ZIP `zip`/`unzip`, `bzip2` and `bsdtar` cross-check
interoperability: unzip -t on our archives, ZipCrypto both ways, AES through
libarchive, Info-ZIP split sets, joining our split sets with `zip -s 0`.

## FEATURES.md IDs backed

F-599, F-600, F-601, F-602, F-603, F-611, F-617, F-618, F-619, F-621,
F-622, F-847, F-848, and the zip side of F-596, F-598, F-612-F-616, F-620,
F-625 and F-628. F-627 (SFX) is N/A, but SFX archives can be read.
