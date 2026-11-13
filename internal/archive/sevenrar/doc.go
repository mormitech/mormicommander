// Package sevenrar is mormiCommander's read-only engine for 7z and RAR
// archives, plus the archive format detector the whole app uses.
//
// Total Commander shows an archive as a folder when you press Enter on it,
// copies files out of it with F5, views them with F3, unpacks with Alt+F9 and
// tests with Alt+Shift+F9. This package does all of that for 7z (through the
// pure-Go github.com/bodgit/sevenzip) and RAR 1.5 to 7 (through the pure-Go
// github.com/nwaples/rardecode/v2), so no external 7z or unrar program is
// needed. Creating 7z or RAR archives is not part of it; that needs the
// external tools (FEATURES.md F-607, F-608).
//
// The UI-facing API is small: Detect/DetectFile say what any file is, Open
// returns an *Archive that can be browsed (ReadDir, Stat, Walk), read (Open),
// unpacked (Extract) and tested (Test). Multi-volume sets are opened from any
// of their volumes, encrypted archives ask for a password through a callback
// when it is needed, and extraction is protected against hostile entry names
// and links. See README.md for a walkthrough.
package sevenrar
