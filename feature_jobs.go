package main

// File jobs (copy, move, delete, trash, wipe) for the page: a thin layer over internal/fileops. The page starts a job,
// gets "job", "job-ask" and "job-done" events (see fileops.Event*), and answers questions with Answer. Running jobs
// can be paused, resumed, slowed down and cancelled from the background transfer window. frontend/src/core/jobs.js is
// the page's side: progress box, the questions, the background list, runJob() promise. The instant operations of the
// same area (make folder, rename in place, reading the copy dialog's target) are in feature_fileops.go.
//
// The end-of-job error list (board issue #15) uses SkipErrors, for a job sent to the background with "Skip errors in
// background" on, and SaveErrorLog, which writes the list the page made to a file the user picks.

import (
	"errors"
	"os"
	"strings"

	"mormicommander/internal/fileops"
)

// Jobs is what the page calls for file jobs (window.go.main.Jobs).
type Jobs struct {
	app *App
	q   *fileops.Queue
}

func init() {
	registerFeature("jobs", func(a *App) []exposed {
		j := &Jobs{app: a}
		j.q = fileops.NewQueue(func(event string, data any) { a.emit(event, data) })
		// what the jobs do also feeds the undo journal (feature_undo.go); a.undo is set before any job can start
		j.q.Log = func(e fileops.Entry) { a.undo.observe(e); jobLog(a, e) }
		a.jobs = j
		return []exposed{expose(j)}
	})
	onShutdown(func(a *App) {
		if a.jobs != nil {
			a.jobs.q.CancelAll()
		}
	})
	registerControl("jobs", func(a *App, m map[string]any) (map[string]any, error) {
		if a.jobs == nil {
			return nil, errors.New("no job queue")
		}
		return map[string]any{"ok": true, "jobs": a.jobs.q.List()}, nil
	})
}

// Start starts a file job and returns its number. Demo files are not real, so the demo starts none.
func (j *Jobs) Start(r fileops.Request) (int, error) {
	if j.app.mock != nil {
		return 0, errors.New("This is the demo: its files are not real, so they can't be copied, moved or deleted.")
	}
	id, err := j.app.undo.startTracked(r, j.q.Start)
	if err != nil {
		return 0, errors.New(fileopsSentence(err.Error()))
	}
	if r.Kind == fileops.Copy || r.Kind == fileops.Move {
		go func() { copyJobComments(r, j.q.Wait(id)) }()
	}
	return id, nil
}

// Cancel stops a job; what is done stays done.
func (j *Jobs) Cancel(id int) bool { return j.q.Cancel(id) }

// CancelAll stops every job.
func (j *Jobs) CancelAll() { j.q.CancelAll() }

// Pause holds a running job after the block it is writing.
func (j *Jobs) Pause(id int) bool { return j.q.Pause(id) }

// Resume lets a paused job go on.
func (j *Jobs) Resume(id int) bool { return j.q.Resume(id) }

// SetSpeedLimit slows a job down to bytesPerSecond (0 = no limit); it takes effect at once.
func (j *Jobs) SetSpeedLimit(id int, bytesPerSecond int64) bool {
	return j.q.SetSpeedLimit(id, bytesPerSecond)
}

// RunNow takes a waiting job out of the queue and starts it at once.
func (j *Jobs) RunNow(id int) bool { return j.q.RunNow(id) }

// Answer answers the question a job asks (a file is in the way).
func (j *Jobs) Answer(id int, a fileops.Answer) error { return j.q.Answer(id, a) }

// List is the state of every job, oldest first.
func (j *Jobs) List() []fileops.Progress { return j.q.List() }

// Running says whether a job is still at work (the page asks before quitting).
func (j *Jobs) Running() bool {
	for _, p := range j.q.List() {
		switch p.State {
		case fileops.StateRunning, fileops.StateAsking, fileops.StatePaused, fileops.StateQueued:
			return true
		}
	}
	return false
}

// Forget drops finished jobs from the list.
func (j *Jobs) Forget() { j.q.Forget() }

// SkipErrors makes a running job note failed files and go on, instead of asking Retry / Skip (see Queue.SkipErrors).
func (j *Jobs) SkipErrors(id int) bool { return j.q.SkipErrors(id) }

// SaveErrorLog asks where to save a job's error list and writes text there. It returns the path written, or "" when
// the user cancelled. In control mode the answer comes from the queue ("save-file"), never a system dialog.
func (j *Jobs) SaveErrorLog(name, text string) (string, error) {
	path, ok := j.app.mockAnswer("save-file", "Save error log")
	if !ok {
		var err error
		path, err = j.app.saveFileDialog("Save error log", "", name, []fileFilter{{DisplayName: "Text files (*.txt, *.log)", Pattern: "*.txt;*.log"}})
		if err != nil {
			return "", errors.New(fileopsSentence(err.Error()))
		}
	}
	if path == "" {
		return "", nil
	}
	if err := writeErrorLog(path, text); err != nil {
		return "", errors.New(fileopsSentence(err.Error()))
	}
	return path, nil
}

// writeErrorLog writes the error list to path, with a final line break and the system's own line breaks on Windows
// (so Notepad shows the lines).
func writeErrorLog(path, text string) error {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if os.PathSeparator == '\\' {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	return os.WriteFile(path, []byte(text), 0o644)
}
