// This file sets the defaults for Windows. NTFS and FAT compare names case-insensitively, as Total Commander does,
// and text files that Windows tools read (Notepad, TC's own "Load selection from file") expect CRLF line endings.

package masks

// DefaultCaseSensitive is the case rule CaseDefault resolves to on this platform.
const DefaultCaseSensitive = false

// lineEnding ends each line of a saved selection file.
const lineEnding = "\r\n"
