// Package multirename is the engine behind the Multi-Rename Tool (Ctrl+M),
// the Total Commander dialog that renames many files at once. The user types
// a name mask and an extension mask built from placeholders such as [N] (the
// old name), [E] (the old extension), [C] (a counter) or [Y][M][D] (date
// parts), optionally adds search-and-replace pairs and a case conversion, and
// watches a live preview of the new names before pressing Start. This package
// does all of that without any UI: it parses the masks, computes the preview,
// flags conflicts (duplicate targets, characters the operating system will
// not accept, names that already exist on disk), performs the renames in a
// safe order with automatic rollback if anything fails, and writes an undo log
// so the last batch can be reversed later. It also loads replacement names
// from a text file or the clipboard, strips accents, and saves and loads named
// presets of all the settings. Keeping the logic here means it can be tested
// thoroughly with table-driven tests and the dialog stays a thin layer.
package multirename
