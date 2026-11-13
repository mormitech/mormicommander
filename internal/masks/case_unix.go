// This file sets the default case rule for Linux and the other Unix systems except macOS. Their usual file systems
// (ext4, XFS, Btrfs, UFS, ZFS) treat "Readme" and "README" as two different files, so a mask has to compare letters
// exactly or it would select files the user did not mean.

//go:build unix && !darwin

package masks

// DefaultCaseSensitive is the case rule CaseDefault resolves to on this platform.
const DefaultCaseSensitive = true

// lineEnding ends each line of a saved selection file.
const lineEnding = "\n"
