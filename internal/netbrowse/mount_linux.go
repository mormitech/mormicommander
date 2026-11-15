package netbrowse

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func discover(ctx context.Context) ([]Server, string) { return discoverAvahi(ctx) }

func watch(ctx context.Context, emit func(Event)) string { return watchAvahi(ctx, emit) }

// gioLocalPath is the local folder GVfs shows a mounted location under, "" when it isn't mounted.
func gioLocalPath(ctx context.Context, uri string) string {
	out, err := exec.CommandContext(ctx, "gio", "info", uri).Output()
	if err != nil {
		return ""
	}
	return ParseGioLocalPath(string(out))
}

// Mount mounts t through GVfs (gio mount), which needs no administrator rights; the share then shows under
// /run/user/<uid>/gvfs. gio's questions for the user name, domain and password are answered on its input.
func Mount(ctx context.Context, t Target, c Creds, _ MountOptions) (Mounted, error) {
	if t.Share == "" {
		return Mounted{}, errors.New("choose a share first")
	}
	if _, err := exec.LookPath("gio"); err != nil {
		return Mounted{}, errors.New("connecting needs gio (GVfs, part of GNOME and most desktops); without it, mount the share with your system's tools")
	}
	uri := t.URL()
	if p := gioLocalPath(ctx, uri); p != "" {
		return Mounted{Path: p}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := gioMount(ctx, uri, c); err != nil {
		return Mounted{}, err
	}
	if p := gioLocalPath(context.Background(), uri); p != "" {
		return Mounted{Path: p, Made: true}, nil
	}
	return Mounted{}, fmt.Errorf("%s was connected, but GVfs shows no local folder for it (is gvfs-fuse installed?)", uri)
}

// gioMount runs `gio mount uri` and answers its questions. A second password question means the password was wrong.
func gioMount(ctx context.Context, uri string, c Creds) error {
	args := []string{"mount", uri}
	if c.User == "" {
		args = []string{"mount", "-a", uri} // anonymous (guest)
	}
	cmd := exec.CommandContext(ctx, "gio", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	var said strings.Builder
	authFailed := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		r := bufio.NewReader(stdout)
		var line strings.Builder
		passwords := 0
		for {
			b, err := r.ReadByte()
			if err != nil {
				return
			}
			said.WriteByte(b)
			if b == '\n' {
				line.Reset()
				continue
			}
			line.WriteByte(b)
			l := line.String()
			if !strings.HasSuffix(l, ": ") {
				continue
			}
			switch {
			case strings.HasPrefix(l, "User"):
				io.WriteString(stdin, c.User+"\n")
			case strings.HasPrefix(l, "Domain"):
				io.WriteString(stdin, c.Domain+"\n") // empty takes gio's default
			case strings.HasPrefix(l, "Password"):
				passwords++
				if c.User == "" || passwords > 1 {
					authFailed = true
					cmd.Process.Kill()
					return
				}
				io.WriteString(stdin, c.Password+"\n")
			default:
				continue
			}
			line.Reset()
		}
	}()
	<-done // all output read before Wait, as os/exec asks
	stdin.Close()
	err = cmd.Wait()
	if authFailed {
		return ErrAuth
	}
	if err != nil {
		msg := lastLine(said.String(), err)
		if strings.Contains(strings.ToLower(msg), "already mounted") {
			return nil
		}
		return fmt.Errorf("gio could not connect to %s: %s", uri, msg)
	}
	return nil
}

// Unmount disconnects a share: GVfs folders through gio, kernel mounts (CIFS, NFS from fstab with "user") through
// umount.
func Unmount(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if uri := GVfsURI(filepath.Base(path)); uri != "" && strings.Contains(path, "/gvfs/") {
		out, err := exec.CommandContext(ctx, "gio", "mount", "-u", uri).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s can't be disconnected: %s", path, lastLine(string(out), err))
		}
		return nil
	}
	out, err := exec.CommandContext(ctx, "umount", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s can't be disconnected (it may need administrator rights): %s", path, lastLine(string(out), err))
	}
	return nil
}

// FreeLetters is for Windows only.
func FreeLetters() []string { return nil }
