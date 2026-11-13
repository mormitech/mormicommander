package fileops

// TC can write a log file of its file operations (Configuration > Operation > Log file): one line per file copied,
// moved, deleted or skipped, and one per error, so that after a long background job the user can see what happened
// to which file. The queue hands every such event to Queue.Log as an Entry; FileLogger is a ready Log function that
// appends TC-style lines to a file. Lines are written as they happen, under a lock, so jobs running side by side
// don't mix up their lines.

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// Entry is one logged event.
type Entry struct {
	Time   time.Time `json:"time"`
	Job    int       `json:"job"`
	Kind   Kind      `json:"kind"`
	Action string    `json:"action"` // copy, move, delete, trash, wipe, rename, mkdir, rmdir, verify, skip, warning, error, done
	Source string    `json:"source,omitempty"`
	Target string    `json:"target,omitempty"`
	Err    string    `json:"err,omitempty"` // the error, or the message of an "error" entry
}

// String is the entry as one log line: "2026-11-13 10:04:05 [3] copy: /a/x.txt -> /b/x.txt".
func (e Entry) String() string {
	s := fmt.Sprintf("%s [%d] %s:", e.Time.Format("2006-01-02 15:04:05"), e.Job, e.Action)
	if e.Source != "" {
		s += " " + e.Source
	}
	if e.Target != "" {
		s += " -> " + e.Target
	}
	if e.Err != "" {
		s += " (" + e.Err + ")"
	}
	return s
}

// FileLogger opens (or creates) a log file for appending and returns a function for Queue.Log and one that closes
// the file.
func FileLogger(path string) (log func(Entry), closeLog func() error, err error) {
	f, err := os.OpenFile(lp(path), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	var mu sync.Mutex
	log = func(e Entry) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintln(f, e.String())
	}
	closeLog = func() error {
		mu.Lock()
		defer mu.Unlock()
		return f.Close()
	}
	return log, closeLog, nil
}
