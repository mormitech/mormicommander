package trash

// On macOS ordinary files and folders go to the Trash through Finder, so Finder's "Put Back" works. The first call
// makes macOS ask whether the app may control Finder. Finder needs an alias to delete, and coercing a path to an
// alias resolves symbolic links, so asking Finder to delete a link would trash the file or folder it points to.
// Links therefore go through NSFileManager's trashItemAtURL instead (via AppleScriptObjC), which moves the link
// itself. Paths always travel as script arguments, never inside the script text.

import (
	"fmt"
	"os/exec"
	"strings"
)

const supported = true

// finderScript deletes every argument through Finder in a single call.
var finderScript = []string{
	"-e", "on run argv",
	"-e", "set items_ to {}",
	"-e", "repeat with p in argv",
	"-e", "set end of items_ to (POSIX file (contents of p) as alias)",
	"-e", "end repeat",
	"-e", `tell application "Finder" to delete items_`,
	"-e", "end run",
}

// linkScript trashes every argument as it is (no link is followed) through NSFileManager.
var linkScript = []string{
	"-e", `use framework "Foundation"`,
	"-e", "on run argv",
	"-e", "set fm to current application's NSFileManager's defaultManager()",
	"-e", "repeat with p in argv",
	"-e", "set u to current application's NSURL's fileURLWithPath:(contents of p)",
	"-e", "set {ok, err} to fm's trashItemAtURL:u resultingItemURL:(missing value) |error|:(reference)",
	"-e", "if ok as boolean is false then error ((contents of p) & \": \" & (err's localizedDescription() as text))",
	"-e", "end repeat",
	"-e", "end run",
}

func moveOne(path string) error { return moveMany([]string{path}) }

// moveMany sends the links in one NSFileManager call and everything else in one Finder call.
func moveMany(paths []string) error {
	var links, plain []string
	for _, p := range paths {
		if isSymlink(p) {
			links = append(links, p)
		} else {
			plain = append(plain, p)
		}
	}
	if len(links) > 0 {
		if err := osascript(linkScript, links); err != nil {
			return err
		}
	}
	if len(plain) > 0 {
		return osascript(finderScript, plain)
	}
	return nil
}

func osascript(script, paths []string) error {
	args := append(append([]string{}, script...), paths...)
	if out, err := runCommand(exec.Command("osascript", args...)); err != nil {
		return fmt.Errorf("%s: %s", strings.Join(paths, ", "), strings.TrimSpace(string(out)))
	}
	return nil
}
