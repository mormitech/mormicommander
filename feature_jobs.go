package main

// File jobs (copy, move, delete, trash) for the page: a thin layer over internal/fileops. The page starts a job, gets
// "job", "job-ask" and "job-done" events (see fileops.Event*), and answers questions with Answer. frontend/src/core/jobs.js
// is the page's side: progress box, overwrite question, runJob() promise.

import (
	"errors"

	"mormicommander/internal/fileops"
)

// Jobs is what the page calls for file jobs (window.go.main.Jobs).
type Jobs struct {
	app *App
	q   *fileops.Queue
}

func init() {
	registerFeature("jobs", func(a *App) []any {
		j := &Jobs{app: a}
		j.q = fileops.NewQueue(func(event string, data any) { a.emit(event, data) })
		a.jobs = j
		return []any{j}
	})
	onShutdown(func(a *App) {
		if a.jobs != nil {
			for _, p := range a.jobs.q.List() {
				a.jobs.q.Cancel(p.ID)
			}
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
	return j.q.Start(r)
}

// Cancel stops a job; what is done stays done.
func (j *Jobs) Cancel(id int) bool { return j.q.Cancel(id) }

// Answer answers the question a job asks (a file is in the way).
func (j *Jobs) Answer(id int, a fileops.Answer) error { return j.q.Answer(id, a) }

// List is the state of every job, oldest first.
func (j *Jobs) List() []fileops.Progress { return j.q.List() }

// Running says whether a job is still at work (the page asks before quitting).
func (j *Jobs) Running() bool {
	for _, p := range j.q.List() {
		if p.State == fileops.StateRunning || p.State == fileops.StateAsking {
			return true
		}
	}
	return false
}

// Forget drops finished jobs from the list.
func (j *Jobs) Forget() { j.q.Forget() }
