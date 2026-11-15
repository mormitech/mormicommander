//go:build !darwin && !linux && !windows

// mormiCommander ships on macOS, Windows and Linux only. On any other system
// the package still builds: UsageOf reports an error and Volumes offers just
// "/", so the drive bar has one button and the drive info line stays empty.

package disk

// Volumes lists only "/" on this platform.
func Volumes() []Volume { return []Volume{{Name: "Root", Path: "/", Kind: "fixed"}} }
