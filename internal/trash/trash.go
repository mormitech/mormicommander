// Package trash moves files to the Trash (macOS, Linux) or the Recycle Bin (Windows), so a delete can be undone in the
// system's own way. The path is always passed as a separate argument or environment variable, never inside script
// text, so no file name can be read as a command. Platform parts are in trash_<os>.go.
package trash

// Supported says whether this platform has a way to move files to the trash at all.
func Supported() bool { return supported }
