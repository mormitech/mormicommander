package main

// Versions from snapshots (board #34, F-874, Files > Versions from snapshots…, cm_SnapshotVersions): lists the copies
// of the file or folder under the cursor that the file system's snapshots keep (ZFS .zfs/snapshot, Btrfs snapper
// .snapshots, macOS APFS local snapshots and Time Machine backups, Windows shadow copies; internal/snapshots), and the
// page opens the chosen one in the other panel for compare or copy back. Snapshots are only read: nothing here
// creates, deletes, mounts or restores one, and the page refuses copying, moving, renaming, deleting and new folders
// inside a snapshot (fileops-readonly.js snapshotRefusal). Not in the AI tool allow-list.

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"mormicommander/internal/disk"
	"mormicommander/internal/snapshots"
)

// snapshotTimeout bounds one lookup (a stale mount above the folder, or a slow tmutil / vssadmin).
var snapshotTimeout = 20 * time.Second

// snapshotEnv is how the lookup reaches the system (a variable so tests use fake layouts and fake programs).
var snapshotEnv = func() snapshots.Env {
	return snapshots.Env{
		GOOS:     runtime.GOOS,
		Stat:     os.Stat,
		ReadDir:  os.ReadDir,
		ReadFile: os.ReadFile,
		Run: func(ctx context.Context, name string, args ...string) (string, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			hideWindow(cmd)
			out, err := cmd.CombinedOutput()
			return string(out), err
		},
		VolumeRoot: disk.VolumeRootOf,
	}
}

var reScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// SnapshotVersions lists the versions of the file or folder at path kept by file system snapshots, newest first.
func (f *FS) SnapshotVersions(path string) (snapshots.Result, error) {
	if f.app != nil && f.app.mock != nil {
		return snapshots.Result{}, errors.New("This is the demo: its files have no snapshots.")
	}
	return findSnapshotVersions(path)
}

func findSnapshotVersions(path string) (snapshots.Result, error) {
	if strings.TrimSpace(path) == "" || reScheme.MatchString(path) || (!filepath.IsAbs(path) && !strings.HasPrefix(path, `\\?\`)) {
		return snapshots.Result{}, errors.New("Snapshots are only kept for files and folders on this computer's disks.")
	}
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()
	env := snapshotEnv()
	done := make(chan snapshots.Result, 1)
	go func() { done <- snapshots.Find(ctx, env, path) }()
	select {
	case r := <-done:
		return r, nil
	case <-ctx.Done():
		return snapshots.Result{}, errors.New("Looking for snapshots took too long: a disk or a program did not answer.")
	}
}
