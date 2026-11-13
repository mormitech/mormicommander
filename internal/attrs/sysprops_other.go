//go:build !darwin && !windows

// On Linux and the BSDs the native properties window belongs to the desktop's
// file manager, reached through the freedesktop.org FileManager1 D-Bus
// interface. Without a session bus or a supporting file manager the call
// fails, and the caller should fall back to mormiCommander's own dialog.
package attrs

func openSystemProperties(path string) error {
	return startDetached(fileManagerPropertiesCommand(path))
}
