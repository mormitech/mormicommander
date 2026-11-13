# internal/fileutil

UI-free engines for Total Commander's small file utilities in the Files menu:
checksums, split/combine and encode/decode. Standard library only (BLAKE2b and
BLAKE3 are implemented in the package). Every long operation takes a
`context.Context` (Cancel button) and an optional `ProgressFunc` that receives
`Progress{File, FileDone, FileTotal, Done, Total}`; a cancel returns an error
wrapping `ErrCanceled` and removes partial output. Existing output files are
never replaced unless `Overwrite` is set (the error wraps `os.ErrExist`, so the
UI can ask "Overwrite?" and retry).

## FEATURES.md IDs backed

| ID | Feature | API |
|----|---------|-----|
| F-332, F-689 | Create checksum files (cm_CRCCreate) | `CreateChecksums` |
| F-333, F-690 | Verify checksums (cm_CRCCheck) | `VerifyChecksumFile(s)`, `VerifyReport` |
| F-691 | Checksums in the Properties dialog | `HashFile` (several algorithms, one read), `MatchChecksum` (compare with a pasted digest) |
| F-693 | Checksum of selected files to clipboard | `HashFiles` + `ClipboardText` |
| F-328, F-687 | Split file (cm_FileSplit) | `SplitFile`, `SizePresets`, `ParseSize`, `PartName`, `PartCount` |
| F-329, F-688 | Combine files (cm_FileCombine) | `CombineFiles`, `FindParts`, `ReadCRCInfo` |
| F-330, F-692 | Encode file (cm_UUEncode): MIME base64, UUE, XXE (+BinHex) | `EncodeFile` |
| F-331, F-692 | Decode file (cm_UUDecode): MIME, UUE, XXE, BinHex, base64 | `DecodeFiles` |

F-763 (the same commands in the context menu) is UI wiring only.

## Checksums

- `Algorithm`: `CRC32, MD5, SHA1, SHA224, SHA256, SHA384, SHA512, SHA3_256,
  SHA3_512, BLAKE2b, BLAKE3`; `Algorithms` is the dialog order. Methods:
  `String()`, `Extension()` (`.sfv .md5 .sha .sha224 .sha256 .sha384 .sha512
  .sha3-256 .sha3-512 .blake2 .blake3`), `HexLen()`, `New()`.
  Helpers: `ParseAlgorithm(name)`, `AlgorithmForPath(path)`,
  `IsChecksumFile(path)` (enable "Verify" for the selection),
  `AlgorithmsForHexLen(n)`.
- `CreateChecksums(ctx, CreateOptions{Algorithms, BaseDir, Paths, OnePerFile,
  OutputName, OutputDir, Format, BSDStyle, Uppercase, Overwrite, Comment,
  Progress}) (*CreateResult, error)`.
  - Paths are files and folders (recursive), names are written relative to
    `BaseDir` (the panel folder).
  - One common file `<OutputName><ext>` (default name: the single selected
    item, else the folder name) or, with `OnePerFile`, `name.ext.md5` next to
    each file.
  - `Format`: `FormatDefault` (Unix on macOS/Linux, Windows on Windows),
    `FormatUnix` (LF, `/`; TC's "Unix format" checkbox), `FormatWindows`
    (CRLF, `\`).
  - Lines: GNU `digest *name` (TC/md5sum), BSD `SHA256 (name) = digest` with
    `BSDStyle`, SFV `name DIGEST` for CRC32 always.
  - Unreadable files land in `CreateResult.Failed` and the run continues.
- `VerifyChecksumFile(ctx, path, VerifyOptions{BaseDir, Progress})
  (*VerifyReport, error)`; `VerifyChecksumFiles` for several with one progress
  total. Reads GNU, BSD, SFV, mixed, BOM, CRLF, GNU-escaped names,
  "digest only" files (`image.iso.sha256`), Windows `\` paths on Unix; a
  64/128-digit digest without a telling extension is tried against every
  algorithm of that length. `VerifyReport` has `Entries` (each with `Status`
  `StatusOK/StatusFailed/StatusMissing/StatusError`, `Actual`, `Used`, `Err`),
  `BadLines`, counters, `AllOK()`, `Summary()` ("12 OK, 1 FAILED, 2 missing"),
  `Text()` (plain-text report for copy/save).
- `HashFile(ctx, path, []Algorithm, progress) (Digests, error)` — map of
  algorithm to lowercase hex; one read for all algorithms.
- `HashFiles(ctx, paths, alg, progress) ([]FileDigest, error)` and
  `ClipboardText(results, alg, uppercase) string` (single digest, or
  md5sum-style lines for several files).
- `MatchChecksum(ctx, path, digest, progress) (alg, ok, err)` — compare a file
  with a pasted digest, algorithm picked by length.
- Low level: `FormatLine`, `ParseChecksumFile`, `ParseChecksums`.

## Split and combine

- `SplitFile(ctx, SplitOptions{Source, TargetDir, PartSize, FreeSpace,
  BeforePart, NoCRCFile, Format, Overwrite, Progress}) (*SplitResult, error)`.
  Parts are `name.001`, `name.002`, ... (extension replaced, as TC does; four
  digits beyond 999) plus `name.crc` with `filename=`, `size=`, `crc32=`.
  `PartSize` is bytes or `SizeAuto` (needs `FreeSpace(dir)`, re-queried before
  each part). `BeforePart(index, path)` can prompt for the next disk; an error
  aborts. Existing parts are refused before anything is written.
- `SizePresets` (`Automatic`, 1.44 MB, 1.2 MB, 720 KB, 360 KB, 650/700 MB CD,
  4.7 GB DVD, 8.5 GB DVD DL, 25 GB BD, 4 GB FAT32) and `ParseSize("700M")`
  for the editable drop-down (binary units, preset names, decimal comma).
- `CombineFiles(ctx, CombineOptions{FirstPart | Parts, TargetDir, TargetName,
  CRCFile, NoCRCCheck, NextPart, Overwrite, Progress}) (*CombineResult,
  error)`. From any part or the `.crc` file it finds all consecutive parts
  (also 7-Zip style `archive.zip.001`); `Parts` joins an explicit list in the
  given order (TC: order of selection). With split info it restores the name
  and checks size and CRC32; a mismatch returns an error wrapping
  `ErrCombineMismatch` and keeps the output (`CombineResult.Checked` false).
  `NextPart(index, expected)` is asked for parts missing in the middle.
- `ReadCRCInfo` / `ParseCRCInfo` / `CRCInfo.Format` for the `.crc` file.

## Encode and decode

- `EncodeFile(ctx, EncodeOptions{Source, Encoding, Target, TargetDir,
  LinesPerPart, Format, Overwrite, Progress}) (*EncodeResult, error)`.
  `Encoding`: `EncodingMIME` (MIME message, base64 attachment, `.b64`),
  `EncodingUUE` (`.uue`), `EncodingXXE` (`.xxe`), `EncodingBinHex` (`.hqx`),
  `EncodingBase64` (bare). `LinesPerPart` splits UUE/XXE into
  `name.001.uue`, `name.002.uue`, ...
- `DecodeFiles(ctx, DecodeOptions{Sources, TargetDir, Overwrite, Progress})
  (*DecodeResult, error)`. Detects the format from content: MIME (all
  attachments, base64 / quoted-printable / x-uuencode, nested multiparts,
  RFC 2047/2231 names), UUE and XXE `begin` blocks (several per file),
  `begin-base64`, BinHex 4.0 (data fork; dropped resource fork reported), bare
  base64. Several `Sources` are read as one stream (multi-part posts); a single
  `name.001.uue` pulls in the following parts. Junk lines between parts are
  skipped and reported in `Warnings`, as is a missing `end`. Stored names are
  reduced to a plain file name in `TargetDir`; duplicates get " (2)". Returns
  `ErrNothingToDecode` when nothing was found.
