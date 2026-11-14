package netbrowse

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func discover(ctx context.Context) ([]Server, string) { return discoverDNSSD(ctx) }

// mountedAt says where t is mounted now, "" when it isn't.
func mountedAt(ctx context.Context, t Target) string {
	out, err := exec.CommandContext(ctx, "mount").Output()
	if err != nil {
		return ""
	}
	return matchMount(ParseMountTable(string(out)), t)
}

// Mount mounts t under /Volumes the way Finder does (AppleScript "mount volume"). Without a user name macOS may show
// its own sign-in window.
func Mount(ctx context.Context, t Target, c Creds, _ MountOptions) (Mounted, error) {
	if t.Share == "" && t.Kind != "afp" {
		return Mounted{}, errors.New("choose a share first")
	}
	if p := mountedAt(ctx, t); p != "" {
		return Mounted{Path: p}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute) // the user may be typing in macOS's sign-in window
	defer cancel()
	cmd := exec.CommandContext(ctx, "osascript", "-")
	cmd.Stdin = strings.NewReader(MountScript(t, c))
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := lastLine(string(out), err)
		switch {
		case strings.Contains(msg, "(-128)"):
			return Mounted{}, errors.New("cancelled")
		case strings.Contains(msg, "(-5000)") || strings.Contains(msg, "(-5023)") || strings.Contains(strings.ToLower(msg), "password"):
			return Mounted{}, ErrAuth
		}
		return Mounted{}, fmt.Errorf("macOS could not connect to %s: %s", t.URL(), msg)
	}
	if p := mountedAt(context.Background(), t); p != "" {
		return Mounted{Path: p, Made: true}, nil
	}
	return Mounted{}, fmt.Errorf("%s was connected, but its folder can't be found under /Volumes", t.URL())
}

// Unmount disconnects the share mounted at path.
func Unmount(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "diskutil", "unmount", path).CombinedOutput()
	if err == nil {
		return nil
	}
	if out2, err2 := exec.CommandContext(ctx, "umount", path).CombinedOutput(); err2 == nil {
		return nil
	} else if len(out2) > 0 {
		out = out2
	}
	return fmt.Errorf("%s can't be disconnected: %s", path, lastLine(string(out), err))
}

// FreeLetters is for Windows only.
func FreeLetters() []string { return nil }
