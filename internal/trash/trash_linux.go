package trash

// On Linux items go to the freedesktop.org trash with `gio trash` (GLib), or trash-cli's `trash-put` when gio is not
// installed. Both trash a symbolic link as the link itself and both take several paths at once, so MoveAll needs a
// single call. "--" ends the options, although the paths are absolute anyway.

import (
	"fmt"
	"os/exec"
	"strings"
)

const supported = true

func moveOne(path string) error { return moveMany([]string{path}) }

func moveMany(paths []string) error {
	for _, c := range [][]string{{"gio", "trash", "--"}, {"trash-put", "--"}} {
		bin, err := lookPath(c[0])
		if err != nil {
			continue
		}
		args := append(append([]string{}, c[1:]...), paths...)
		if out, err := runCommand(exec.Command(bin, args...)); err != nil {
			return fmt.Errorf("%s: %s", strings.Join(paths, ", "), strings.TrimSpace(string(out)))
		}
		return nil
	}
	return errNoTool
}
