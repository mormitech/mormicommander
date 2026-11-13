// Package lister is the UI-free engine behind mormiCommander's Lister, the
// built-in file viewer opened with F3 (and the quick view panel, Ctrl+Q). It
// does everything that does not need a window: opening files of any size
// lazily, guessing and switching the character encoding, laying out text,
// binary and hex rows by byte offset, word wrap, line numbers and "go to
// line", searching forwards and backwards (text, hex, regex, case, whole
// words), copying selections as text or hex, following a growing log file,
// stepping to the next or previous file, converting HTML and RTF to text, and
// reading image/audio/video metadata for multimedia mode. The UI binds these
// calls and draws the rows it gets back; see README.md for the wiring guide.
package lister
