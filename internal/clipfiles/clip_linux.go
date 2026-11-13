// This file selects the command-line tool backend (xdg.go) as the Linux clipboard, using the real process runner and
// the real environment, which says whether the session is Wayland or X11 and which desktop is running.

package clipfiles

import "os"

func newSystemBackend() backend { return &xdgBackend{r: execRunner{}, getenv: os.Getenv} }
