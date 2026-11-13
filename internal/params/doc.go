// Package params holds everything mormiCommander needs to turn text typed by
// the user into something that can actually run, in the way Total Commander
// does it. That covers four related jobs. First, the program's own start-up
// parameters (/O /N /T /L= /R= /P= /S /S=L /I= /F= and friends) are parsed
// into a plain struct so main.go can decide which folders to open and whether
// to hand them to an already running window. Second, the parameter field of
// button-bar buttons and start-menu (user menu) entries is expanded: the
// placeholders %P %N %T %M %O %E %S %R %L %F %D %WL %UL and the rest are
// replaced by paths and names from the two panels, names with spaces are
// quoted, list files are written to the temp folder, a leading "?" asks the
// user to confirm the final text, and %[caption;default] asks for a value.
// Third, environment variables (%HOME%, $HOME, ${HOME}, %$DESKTOP%) are
// expanded. Fourth, the command-line box under the panels is understood:
// "cd" and its variants change the panel folder instead of starting a shell,
// cm_ and em_ names run internal commands, "cmd" opens a terminal, and
// everything else becomes a program launch, with or without a shell.
//
// The package is UI-free and does not start any process itself; it only
// returns descriptions (Launch, Action, Startup) that the app layer acts on.
// All path handling is style-aware (Windows or POSIX) instead of relying on
// the host OS, so the Windows rules are tested on a Mac as well.
package params
