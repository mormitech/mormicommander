package trash

// On macOS ordinary files and folders go to the Trash through Finder, so Finder's "Put Back" works. The first call
// makes macOS ask whether the app may control Finder. Finder needs an alias to delete, and coercing a path to an
// alias resolves symbolic links, so asking Finder to delete a link would trash the file or folder it points to.
// Links therefore go through NSFileManager's trashItemAtURL instead (via AppleScriptObjC), which moves the link
// itself. Paths always travel as script arguments, never inside the script text.
//
// A Finder call can hang: while macOS waits for the user to allow the automation (or when the prompt can't be shown
// at all) the call simply does not return, and a Trash job then sits at 0%. So the Finder call carries an AppleScript
// timeout (a minute plus a second per item), the helper is killed when the caller's context ends, and the two known
// failures (permission refused, no answer in time) are turned into plain messages. MoveDirect is the way around
// Finder: the same NSFileManager call used for links, which needs no automation permission (no "Put Back" though).

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const supported = true

const directSupported = true

// finderScript deletes every argument through Finder in a single call, giving Finder at most timeout seconds.
func finderScript(timeout int) []string {
	return []string{
		"-e", "on run argv",
		"-e", "set items_ to {}",
		"-e", "repeat with p in argv",
		"-e", "set end of items_ to (POSIX file (contents of p) as alias)",
		"-e", "end repeat",
		"-e", "with timeout of " + strconv.Itoa(timeout) + " seconds",
		"-e", `tell application "Finder" to delete items_`,
		"-e", "end timeout",
		"-e", "end run",
	}
}

// finderTimeout is how long one Finder call may take for n items, in seconds: a minute, plus a second per item for
// large selections, capped at ten minutes.
func finderTimeout(n int) int { return min(60+n, 600) }

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

// ErrFinderNotAllowed and ErrFinderTimeout are wrapped into the error of a Finder call that failed for these
// reasons, so callers can tell them apart (errors.Is) and offer MoveDirect.
var (
	ErrFinderNotAllowed = errors.New("macOS does not allow this app to control Finder (System Settings > Privacy & Security > Automation)")
	ErrFinderTimeout    = errors.New("Finder did not answer in time; macOS may be waiting for permission to let this app control Finder")
)

func moveOne(ctx context.Context, path string) error { return moveMany(ctx, []string{path}) }

func moveDirect(ctx context.Context, path string) error {
	return osascript(ctx, linkScript, []string{path})
}

// moveMany sends the links in one NSFileManager call and everything else in one Finder call.
func moveMany(ctx context.Context, paths []string) error {
	var links, plain []string
	for _, p := range paths {
		if isSymlink(p) {
			links = append(links, p)
		} else {
			plain = append(plain, p)
		}
	}
	if len(links) > 0 {
		if err := osascript(ctx, linkScript, links); err != nil {
			return err
		}
	}
	if len(plain) > 0 {
		return osascript(ctx, finderScript(finderTimeout(len(plain))), plain)
	}
	return nil
}

func osascript(ctx context.Context, script, paths []string) error {
	args := append(append([]string{}, script...), paths...)
	out, err := runCommand(command(ctx, "osascript", args...))
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return scriptError(strings.Join(paths, ", "), strings.TrimSpace(string(out)))
}

// scriptError turns osascript's output into an error, recognising the AppleEvent codes for "not authorised to send
// Apple events" (-1743) and "timed out" (-1712).
func scriptError(what, out string) error {
	switch {
	case strings.Contains(out, "(-1743)"):
		return fmt.Errorf("%s: %w", what, ErrFinderNotAllowed)
	case strings.Contains(out, "(-1712)"):
		return fmt.Errorf("%s: %w", what, ErrFinderTimeout)
	}
	return fmt.Errorf("%s: %s", what, out)
}
