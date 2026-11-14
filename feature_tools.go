package main

// The Files-menu tools (FEATURES.md sections 12, 34, 35 and the link items of 12/42): change attributes, properties,
// occupied space, checksums, split/combine, encode/decode and links. This file holds what all of them share: the bound
// object Tools (window.go.main.Tools), cancellable operations and their progress events, and the check that a path is a
// real file on this computer. The tools themselves are in feature_tools_*.go, over the UI-free engines internal/attrs,
// internal/dirsize, internal/fileutil and internal/links.
//
// A long operation is one blocking call from the page (Wails runs every call on its own goroutine) that takes an id
// chosen by the page. While it runs, Go sends "tools-progress" events ({id, ...}) and the page's Cancel button calls
// Tools.Cancel(id). That keeps the page side a plain `await` with a progress box, the same shape for every tool.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"
)

// Tools is what the page calls for the Files-menu tools (window.go.main.Tools).
type Tools struct {
	app *App
	mu  sync.Mutex
	ops map[string]context.CancelFunc // running operations by the page's id
}

func init() {
	registerFeature("tools", func(a *App) []exposed { return []exposed{expose(&Tools{app: a, ops: map[string]context.CancelFunc{}})} })
}

// Cancel stops the operation with this id; what is done stays done (the engines remove half-written output).
func (t *Tools) Cancel(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if c := t.ops[id]; c != nil {
		c()
	}
}

// toolsBegin registers an operation and returns its context and the function that ends it.
func (t *Tools) toolsBegin(id string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	if id != "" {
		if old := t.ops[id]; old != nil {
			old()
		}
		t.ops[id] = cancel
	}
	t.mu.Unlock()
	return ctx, func() {
		cancel()
		t.mu.Lock()
		delete(t.ops, id)
		t.mu.Unlock()
	}
}

// toolsProgress returns a function that sends progress for an operation, at most about ten times a second (the
// last call before the end is not lost: the page shows the final numbers from the result anyway).
func (t *Tools) toolsProgress(id string) func(data map[string]any) {
	var mu sync.Mutex
	var last time.Time
	return func(data map[string]any) {
		mu.Lock()
		now := time.Now()
		if now.Sub(last) < 100*time.Millisecond {
			mu.Unlock()
			return
		}
		last = now
		mu.Unlock()
		data["id"] = id
		t.app.emit("tools-progress", data)
	}
}

// toolsLocal checks that every path is a file or folder on this computer: the tools work on real files only, not
// on the demo's fictional ones, inside archives or on FTP.
func (t *Tools) toolsLocal(paths ...string) error {
	if t.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so this tool can't work on them.")
	}
	for _, p := range paths {
		if p == "" {
			return errors.New("Nothing is selected.")
		}
		if strings.Contains(p, "://") {
			return errors.New("This tool works on files on this computer only, not in archives or on servers.")
		}
		if _, err := os.Lstat(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%s is not there, or it is inside an archive (this tool works on files on this computer only).", p)
			}
			return toolsErr(err)
		}
	}
	return nil
}

// toolsLocalDir checks that a folder for output is a real folder on this computer.
func (t *Tools) toolsLocalDir(dir string) error {
	if err := t.toolsLocal(dir); err != nil {
		return err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a folder.", dir)
	}
	return nil
}

// toolsErr turns an engine error into a sentence for the user.
func toolsErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(toolsErrText(err))
}

// toolsErrText says in plain words what went wrong.
func toolsErrText(err error) string {
	var pe *fs.PathError
	path := ""
	if errors.As(err, &pe) {
		path = pe.Path
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		path = le.New
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "Stopped."
	case errors.Is(err, fs.ErrPermission):
		if path != "" {
			return "You don't have permission to change " + path + "."
		}
		return "You don't have permission to do that."
	case errors.Is(err, fs.ErrNotExist):
		if path != "" {
			return path + " is not there."
		}
		return "A file is not there any more."
	case errors.Is(err, fs.ErrExist):
		if path != "" {
			return path + " is already there."
		}
		return "The file is already there."
	}
	t := err.Error()
	if t == "" {
		return "Something went wrong."
	}
	return strings.ToUpper(t[:1]) + t[1:]
}

// toolsMS turns a time into Unix milliseconds for the page (0 for no time).
func toolsMS(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
