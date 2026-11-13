// Follow tail ("auto-reload") for the Lister. Watching a growing log file
// is one of the most common reasons to open a viewer, so Total Commander's
// Lister can reload a file automatically and jump to its end, like tail -f.
// We poll instead of using OS change notifications: polling the size every
// fraction of a second is cheap, behaves the same on every platform and on
// network drives (where notifications are unreliable), and also notices log
// rotation, when the path suddenly names a new, shorter file. Each detected
// change is reported to a callback, which the UI uses to fetch the new rows
// and, when the user was looking at the end, scroll to the new end.

package lister

import (
	"context"
	"time"
)

// DefaultFollowInterval is how often FollowTail checks the file.
const DefaultFollowInterval = 500 * time.Millisecond

// FollowTail polls the viewer's file until ctx is cancelled and calls fn
// after every change (growth, truncation, replacement or a rewrite in
// place). The viewer is already reloaded when fn runs, so fn can call
// LastPage and Rows directly. Errors such as the file being deleted for a
// moment during rotation are passed to onErr (if not nil) and polling goes
// on. It returns ctx.Err() when stopped.
func (v *Viewer) FollowTail(ctx context.Context, interval time.Duration, fn func(Change), onErr func(error)) error {
	if interval <= 0 {
		interval = DefaultFollowInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			ch, err := v.Reload()
			if err != nil {
				if onErr != nil {
					onErr(err)
				}
				continue
			}
			if ch.Changed() && fn != nil {
				fn(ch)
			}
		}
	}
}

// AtEnd reports whether a view whose top row is at top and which shows
// rows rows is scrolled to the end, which is when follow mode should keep
// jumping to the new end after growth.
func (v *Viewer) AtEnd(top int64, rows int) (bool, error) {
	last, err := v.LastPage(rows)
	if err != nil {
		return false, err
	}
	return top >= last, nil
}
