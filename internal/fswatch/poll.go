// Polling backend. Native change notifications are unreliable or missing on network shares (a change made by another
// machine on an SMB or NFS share usually produces no local event), on some FUSE filesystems and on platforms without a
// native backend, so those folders are reread every PollInterval instead. Each pass lists the folder and compares
// every entry's size, modification time and mode with the previous pass; the names that differ are reported. A
// folder that cannot be read once (a share that hiccups) produces a soft error and polling continues; a folder that
// no longer exists ends the backend so the watcher can report it as gone.
package fswatch

import (
	"errors"
	"io/fs"
	"os"
	"sync"
	"time"
)

type pollStat struct {
	size  int64
	mtime int64
	mode  fs.FileMode
}

type pollBackend struct {
	path     string
	interval time.Duration
	report   func(raw)
	quit     chan struct{}
	done     chan struct{}
	once     sync.Once
}

// startPollBackend takes an initial snapshot synchronously, so changes made right after Watch returns are seen.
func startPollBackend(path string, opts Options, report func(raw)) (backend, error) {
	snap, err := pollSnapshot(path)
	if err != nil {
		return nil, err
	}
	b := &pollBackend{path: path, interval: opts.PollInterval, report: report,
		quit: make(chan struct{}), done: make(chan struct{})}
	go b.loop(snap)
	return b, nil
}

func (b *pollBackend) stop() {
	b.once.Do(func() { close(b.quit) })
	<-b.done
}

func (b *pollBackend) loop(prev map[string]pollStat) {
	defer close(b.done)
	t := time.NewTicker(b.interval)
	defer t.Stop()
	failing := false
	for {
		select {
		case <-b.quit:
			return
		case <-t.C:
		}
		cur, err := pollSnapshot(b.path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, errNotDir) {
				b.report(raw{Terminal: true, Err: err})
				return
			}
			if !failing {
				b.report(raw{Soft: err})
				failing = true
			}
			continue
		}
		// The share may have come back after failing; prev still holds the last good listing, so the diff below
		// covers whatever changed meanwhile.
		failing = false
		for _, n := range diffSnapshots(prev, cur) {
			select {
			case <-b.quit:
				return
			default:
			}
			b.report(raw{Name: n})
		}
		prev = cur
	}
}

var errNotDir = errors.New("not a directory")

// pollSnapshot lists a folder with one Lstat per entry. Entries that vanish between the listing and the Lstat are
// skipped; they will show up as removed in the next pass if they really are gone.
func pollSnapshot(path string) (map[string]pollStat, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errNotDir
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	ents, err := f.ReadDir(-1)
	if err != nil && len(ents) == 0 {
		return nil, err
	}
	snap := make(map[string]pollStat, len(ents))
	for _, de := range ents {
		info, err := de.Info()
		if err != nil {
			continue
		}
		snap[de.Name()] = pollStat{size: info.Size(), mtime: info.ModTime().UnixNano(), mode: info.Mode()}
	}
	return snap, nil
}

// diffSnapshots returns the names that were added, removed or changed between two snapshots.
func diffSnapshots(prev, cur map[string]pollStat) []string {
	var out []string
	for n, s := range cur {
		if p, ok := prev[n]; !ok || p != s {
			out = append(out, n)
		}
	}
	for n := range prev {
		if _, ok := cur[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}
