# internal/listing

UI-free engine that turns a raw folder listing into the rows a file panel draws: filtering, sorting, formatting,
column sets (Brief, Full, Comments, custom) and status line totals. It has no Wails dependency and cross-compiles for
macOS, Windows and Linux. Its dependencies beyond the standard library are `golang.org/x/text/collate`, used for
locale-aware name order, `golang.org/x/image` (image sizes of WebP, TIFF and BMP), `go4.org/media/heif` (the EXIF item
and size of HEIF/HEIC photos) and (on Linux) `golang.org/x/sys/unix` for the `statx` birth time.

## Typical use

```go
items, _ := listing.ReadDir(dir)            // or build []listing.Item from any VFS (archives, FTP, ...)
comments, _ := listing.LoadComments(dir)    // descript.ion, only needed for Comments view / tc.comment
view := listing.Build(items, listing.Options{
    Mode:      listing.ViewFull,
    Format:    listing.Format{Size: listing.SizeDynamic, Date: listing.DatePictureEurope},
    Collation: listing.Collation{Natural: true},
    Sort:      listing.DefaultSort.Toggle(listing.BySize, 0, false),
    Filter:    listing.FilterOptions{ShowUp: true, Dir: dir, ShowHidden: false},
    Comments:  comments,
})
// view.Titles, view.Rows[i].Cells, view.Totals
status := listing.StatusText(view.Count(isSelected), listing.SizeKB, listing.NumberFormat{})
```

## API summary

| Area | API |
|---|---|
| Items | `Item` (name, size, `DirSize` -1 until calculated, three dates, `Attr` bits, mode, owner/group, link target, comment, `Extra`), `Attr*` bits, `FromFileInfo`, `ReadDir` (file system order, no name sort), `UpItem`, `SplitExt`, `SplitExtFor`, `IsWindowsExecName` |
| Collation | `Collation{Natural, CaseSensitive, WordSort, Locale}`, `NewCollator`, `(*Collator).Compare` |
| Sorting | `SortBy` (`ByName`, `ByExt`, `ByDate`, `BySize`, `ByUnsorted`, `ByAttr`, `ByColumn`), `SortKey`, `SortSpec{Keys, DirsByName, MixDirs}`, `DefaultSort`, `SortSpec.Toggle` (click / Ctrl+click), `SortSpec.Reverse`, `Sort`, `Order` |
| Formatting | `Format`, `SizeUnit` (`SizeBytes`, `SizeKB`, `SizeMB`, `SizeGB`, `SizeDynamic`, `SizeRaw`), `FormatSize`, `ParseSizeUnit`, `NumberFormat`, `FormatDate` (Windows date pictures: `dd.MM.yyyy HH:mm`, `tt`, `'literal'`), `DatePicture*`, `AttrStyle` (auto, Windows `rahs`, Windows full `rahsce`, Unix `drwxr-xr-x`, octal), `AttrString` |
| Fields | `Field`, `Value`, `Kind`, `RegisterField`, `LookupField`, `FieldNames`, `ExtraField`, `FileTypeName` |
| Columns | `Column{Title, Template, Width, Align}`, `ColumnSet`, `Compile` / `MustCompile`, `CompiledColumn.Render` / `SortValue` / `Fields`, `ParseTemplate`, built-ins `BriefSet`, `FullSet`, `FullNameSet`, `CommentsSet`, `FolderRule`, `ChooseSet` |
| Filter | `FilterOptions{ShowHidden, ShowSystem, ShowUp, AtRoot, Mode, Match, FilterDirs, Ignore, IgnoreOff}`, `FilterMode` (`ShowAll`, `ShowPrograms`, `ShowCustom`), `Filter`, `Visible`, `Active` |
| Status | `Totals`, `Count`, `Totals.Add`, `StatusText` ("15 k / 17 k in 2 / 3 file(s), 1 / 2 dir(s)"), `ItemText` |
| Comments | `Comments`, `ParseComments`, `LoadComments`, `Get`, `Set`, `Rename`, `Bytes`, `Save`, `ApplyComments` (descript.ion, ANSI or UTF-8, TC multi-line marker; saved via `config.WriteFileAtomic`, which keeps the file mode) |
| Views | `ViewMode` (`ViewBrief`, `ViewFull`, `ViewComments`, `ViewCustom`), `Options`, `Build`, `View{Set, Titles, Rows, Totals}`, `View.Count`, `View.Find`, `SetFor`, `BriefLayout` (`Columns`, `Pos`, `Index`, `Move`) |
| Media | `ExifDate`, `ReadID3`, `ReadAudioTags`, `AudioTags`; fields read lazily and cached in `Item.Extra` |

### Column template fields

Placeholders are `[=namespace.field.unit]`; the unit is everything after the second dot, so date pictures may contain
dots (`[=tc.writedate.dd.MM.yyyy]`). Any other text is literal (`[=tc.size] bytes`). A column with one placeholder
sorts by the field's typed value (numbers, instants); empty values stay at the bottom in both directions.

- `tc.name`, `tc.ext`, `tc.fullname`, `tc.size` (units `bytes`, `k`, `M`, `G`, `dynamic`, `raw`), `tc.writedate`,
  `tc.creationdate`, `tc.accessdate` (unit = date picture), `tc.attributestr` (units `win`, `winfull`, `unix`, `octal`),
  `tc.permissions`, `tc.owner`, `tc.group`, `tc.linktarget`, `tc.comment`, `tc.path`, `tc.fullpath`, `tc.filetype`,
  `tc.mime`
- `img.width`, `img.height`, `img.size` (WxH, sorts by pixel count), `img.exifdate` (PNG, JPEG, GIF, WebP, TIFF, BMP
  headers and HEIF's image size property; EXIF DateTimeOriginal from JPEG, PNG eXIf, WebP EXIF, TIFF and the raw formats
  built on it, and HEIF/HEIC through `go4.org/media/heif`)
- `audio.title`, `audio.artist`, `audio.album`, `audio.year`, `audio.track`, `audio.genre` (ID3v1, ID3v2.2/2.3/2.4,
  and the Vorbis comments of FLAC and Ogg Vorbis/Opus files)
- Other engines add their own namespaces with `RegisterField` (the replacement for TC's WDX content plugins).

## FEATURES.md IDs backed

- Section 4 (views): F-079 Brief, F-080 Full, F-084 custom column sets, F-085 Comments, F-088 all / programs / custom
  filter, F-089 custom filter predicate, F-090 hidden/system toggle, F-091 `<DIR>` until folder size is calculated,
  F-092 extension column vs names whole, F-093 size display units, F-094 date format, F-098 folders first / always by
  name, F-102 ".." entry, F-785 (column set part of view modes), F-787 folders-by-name switch, F-788 ignore list toggle.
- Section 5 (sorting): F-107 name, F-108 extension, F-109 date (newest first), F-110 size (largest first), F-111
  unsorted, F-112 reverse, F-113 header-click toggle incl. custom columns, F-114 multi-key Ctrl+click, F-115 natural
  sort, F-116 case-sensitive / word sort / locale collation, F-117 per-tab sort state (`SortSpec` is plain JSON data),
  F-790 sort by column N.
- Section 32 (custom columns): F-665 column set model and field list, F-666 `[=tc.size]` placeholder syntax with
  several fields per column, F-667 units per field, F-668 folder rules (`ChooseSet`), F-670 sort by custom column,
  F-674 built-in tc fields. F-671 to F-673 (menus, header context menu, auto width) are UI work on top of this API.
- Status line totals (selected / total files, folders, bytes) used by sections 1 and 6.

## Notes

- Linux has no birth time in the classic stat call, so `creationdate` comes from `statx` (when the kernel and file
  system record one) and stays empty otherwise; Windows and macOS always fill it.
- Dotfiles count as hidden on macOS and Linux; on Windows only the hidden attribute counts. macOS `UF_HIDDEN` is honoured.
- Saving `descript.ion` marks it hidden on Windows, as Total Commander does. Names with quotes (Unix only) are
  written quoted with a quote doubled when it would otherwise end the name early, so every name round-trips.
