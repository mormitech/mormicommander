// Package links is the UI-free engine behind Total Commander's link features:
// Files > Create symbolic link / Create hard link (cm_CreateSymbolicLink,
// cm_CreateHardLink, Ctrl+Shift+F5 cm_CreateShortcut), NTFS junctions on
// Windows, reading shortcut files (.lnk, .url, .webloc, .desktop and macOS
// Finder aliases) so that Enter and Ctrl+PgDn can follow them, resolving the
// target of a link through a whole chain of links, and detecting link loops so
// that recursive copy, search and size calculation never run forever.
//
// Everything here works on plain paths and returns plain values, so the
// panels, the dialogs and the file operation queue can wire it up later
// without this package knowing anything about the UI. Platform differences
// live in files with the _unix.go, _windows.go and _other.go suffixes; the
// shortcut parsers are pure Go and work on every platform, because a panel on
// macOS may well be showing a folder full of Windows .lnk files.
package links
