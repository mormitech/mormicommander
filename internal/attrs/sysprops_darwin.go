// On macOS the native properties window is Finder's Get Info, opened through
// a tiny AppleScript run by osascript.
package attrs

func openSystemProperties(path string) error {
	return startDetached(finderInfoCommand(path))
}
