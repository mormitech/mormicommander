// Package dirsize calculates how much space folders take, the way Total Commander does when the user presses Space on
// a folder, Alt+Shift+Enter for every folder in the panel, or Ctrl+L for the whole selection. A calculation walks the
// folder tree in parallel, can be cancelled at any moment (Esc or the Cancel button) and reports progress while it
// runs, because a home folder or a network share can take minutes to count. It reports both the logical size (the sum
// of the file lengths, what the size column shows) and the size on disk (what the files really occupy, which differs
// for small files, sparse files and compressed folders), counts files and folders, counts a file with several hard
// links only once so the total matches the free space it would release, and can stay on the volume it started on so
// that counting "/" does not wander into mounted disks and network shares. The package also reports volume details
// (label, file system, total and free space) for the drive bar, the drive popup and the drive info line above each
// panel. It has no UI dependencies, so it builds and tests on its own on every platform.
package dirsize

import (
	"errors"
	"fmt"
	"time"
)

// ErrCanceled is wrapped by the error a calculation returns when its context is cancelled. The partial result that
// comes with it is still valid as far as it got, so the UI can show "at least N bytes".
var ErrCanceled = errors.New("dirsize: calculation canceled")

// DefaultProgressInterval is how often Progress is called while a calculation runs. 150 ms keeps the dialog lively
// without spending noticeable time on UI updates.
const DefaultProgressInterval = 150 * time.Millisecond

// DefaultWorkers is how many folders are read at the same time. Parallel reads help a lot on SSDs and network shares
// and do little harm on spinning disks at this level.
const DefaultWorkers = 16

// maxListed caps how many errors and skipped mount points a Result keeps by name; the counts are always exact. A
// folder full of unreadable entries must not make a result use unbounded memory.
const maxListed = 100

// Options control a calculation. The zero value is a sensible default: hard links are counted once, symbolic links
// are not followed, mounted volumes are entered, and progress is reported every 150 ms if a callback is set.
type Options struct {
	// SameVolume stops the walk at mount points (and, on Windows, at volumes mounted into folders), like "du -x".
	// On macOS the system and data volumes of the startup disk count as one volume, since the user sees them as one.
	SameVolume bool
	// FollowSymlinks descends into symbolic links (and Windows junctions) that point to folders, and counts the
	// target size of links that point to files. Loops are detected, and each real folder and file is counted once (a
	// file reached a second time through a link counts as a hard link duplicate). The
	// calculation root itself is always followed, because pressing Space on a link to a folder means "that folder".
	FollowSymlinks bool
	// CountHardlinksTwice turns off hard link de-duplication. By default a file with several names is counted once
	// in Bytes and Allocated (each name still counts in Files), which is what deleting the folder would free.
	CountHardlinksTwice bool
	// Quick skips the per-file system calls that Windows needs to learn the real size on disk and the hard link
	// identity of a file; size on disk is then estimated by rounding up to the cluster size and hard links are not
	// de-duplicated. On macOS and Linux this information is free, so Quick changes nothing there.
	Quick bool
	// Workers is the number of folders read in parallel; 0 means DefaultWorkers.
	Workers int
	// ProgressInterval is the time between Progress calls; 0 means DefaultProgressInterval.
	ProgressInterval time.Duration
	// Progress, if set, is called from one goroutine at a time, every ProgressInterval and once more at the end
	// with Done set. It must return quickly.
	Progress func(Progress)
	// ItemDone, if set, is called as soon as each path of CalculateAll is finished, so the panel can fill in the
	// size column folder by folder during Alt+Shift+Enter instead of all at the end.
	ItemDone func(index int, r Result)
}

// PathError is one entry that could not be read. Unreadable folders are common (permissions, files that vanish while
// counting), so a calculation keeps going and reports them instead of stopping.
type PathError struct {
	Path string `json:"path"`
	Err  string `json:"err"`
}

func (e PathError) Error() string { return e.Path + ": " + e.Err }

// Result is what one calculation found. Counts do not include the calculated folder itself, so a folder with two
// files and no subfolders has Files 2 and Dirs 0.
type Result struct {
	Path  string `json:"path"`
	IsDir bool   `json:"isDir"`
	// Bytes is the logical size: the sum of file lengths (hard links once, see Options.CountHardlinksTwice).
	Bytes int64 `json:"bytes"`
	// Allocated is the size on disk: blocks or clusters the files occupy. It is smaller than Bytes for sparse and
	// compressed files and larger for many small files.
	Allocated int64 `json:"allocated"`
	Files     int64 `json:"files"`    // regular files (every name, including extra hard links)
	Dirs      int64 `json:"dirs"`     // subfolders
	Symlinks  int64 `json:"symlinks"` // symbolic links and junctions that were not followed (no size counted)
	Others    int64 `json:"others"`   // sockets, pipes, device files (no size counted)
	// HardlinkDupes is the number of file names whose data had already been counted under another name, and
	// HardlinkBytes the logical bytes that were therefore not counted again.
	HardlinkDupes int64 `json:"hardlinkDupes"`
	HardlinkBytes int64 `json:"hardlinkBytes"`
	// SkippedMounts lists folders that were not entered because they are on another volume (Options.SameVolume) or
	// because following them would loop; SkippedMountCount is the exact number.
	SkippedMounts     []string `json:"skippedMounts,omitempty"`
	SkippedMountCount int64    `json:"skippedMountCount"`
	// Errors lists entries that could not be read (at most 100); ErrorCount is the exact number.
	Errors     []PathError   `json:"errors,omitempty"`
	ErrorCount int64         `json:"errorCount"`
	Complete   bool          `json:"complete"` // false if the calculation was cancelled
	Duration   time.Duration `json:"duration"`

	rootErr error // the path itself could not be read; Calculate returns it
}

// Progress is a snapshot of a running calculation, for the "counting…" dialog and the status bar.
type Progress struct {
	Current    string        `json:"current"`    // the folder being read right now
	Item       int           `json:"item"`       // index of the path being calculated (CalculateAll)
	ItemPath   string        `json:"itemPath"`   // that path
	ItemsDone  int           `json:"itemsDone"`  // paths finished so far
	ItemsTotal int           `json:"itemsTotal"` // number of paths
	Bytes      int64         `json:"bytes"`      // running totals over all paths
	Allocated  int64         `json:"allocated"`
	Files      int64         `json:"files"`
	Dirs       int64         `json:"dirs"`
	Errors     int64         `json:"errors"`
	Elapsed    time.Duration `json:"elapsed"`
	Done       bool          `json:"done"`
}

// Summary is the result of CalculateAll: one Result per path, in the order given, and a Total over all of them.
// Total.Dirs counts the given folders themselves as well as their subfolders, and Total.Files counts the given files
// too, which is what the Ctrl+L dialog shows ("N files, M folders"). Hard links shared between different paths are
// counted once in Total as well.
type Summary struct {
	Items []Result `json:"items"`
	Total Result   `json:"total"`
}

// String gives a one-line description in Total Commander's Ctrl+L wording, for logs and tests.
func (r Result) String() string {
	return fmt.Sprintf("%s: %d bytes (%d on disk) in %d files, %d folders", r.Path, r.Bytes, r.Allocated, r.Files, r.Dirs)
}

func (o Options) workers() int {
	if o.Workers > 0 {
		return o.Workers
	}
	return DefaultWorkers
}

func (o Options) interval() time.Duration {
	if o.ProgressInterval > 0 {
		return o.ProgressInterval
	}
	return DefaultProgressInterval
}
