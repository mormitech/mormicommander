package main

// Synchronize folders (cm_SyncDirs, FEATURES.md section 25) for the page: a thin layer over internal/syncdirs. The page
// opens a session, starts a compare (it runs in a goroutine and reports "syncdirs-progress" and "syncdirs-done"), reads
// the rows under its Show buttons, changes actions, and finally runs the plan ("syncdirs-exec", "syncdirs-ask" for the
// overwrite question, "syncdirs-exec-done"). All the rules (categories, default actions, what may be done to a row,
// the safe copy) live in the engine; this file only carries them between the page and the disk.
//
// Only folders on this computer can be synchronized for now: the engine's sides are file systems with writes, and the
// archive and network file systems of the panels don't offer that yet (F-555 stays open).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/syncdirs"
)

// SyncDirs is what the page calls for the Synchronize folders tool (window.go.main.SyncDirs).
type SyncDirs struct {
	app  *App
	mu   sync.Mutex
	next int
	sess map[int]*syncSession
}

type syncSession struct {
	mu          sync.Mutex
	left, right string
	lfs, rfs    *syncdirs.OSFS
	res         *syncdirs.Result
	cancel      context.CancelFunc // of the compare or sync in progress
	busy        bool
	answer      chan syncdirs.OverwriteDecision
}

func init() {
	registerFeature("syncdirs", func(a *App) []any { return []any{&SyncDirs{app: a, sess: map[int]*syncSession{}}} })
}

// SyncCompareRequest is what the Compare button sends: both folders and the options of the window.
type SyncCompareRequest struct {
	Left     string           `json:"left"`
	Right    string           `json:"right"`
	Options  syncdirs.Options `json:"options"`
	Selected []string         `json:"selected"` // top-level names for "only selected"
}

// SyncView is the list under the Show buttons, with the counts for the status line.
type SyncView struct {
	Rows     []*syncdirs.Item `json:"rows"`
	Summary  syncdirs.Summary `json:"summary"`
	Errors   []string         `json:"errors"`
	Compared bool             `json:"compared"`
	Left     string           `json:"left"`
	Right    string           `json:"right"`
}

// SyncExecRequest is what the Synchronize dialog sends.
type SyncExecRequest struct {
	Filter           syncdirs.PlanFilter `json:"filter"`
	ConfirmOverwrite bool                `json:"confirmOverwrite"`
	Trash            bool                `json:"trash"`
	// Left and Right are the folders the window shows; when given, they must be the compared pair, so a sync never
	// writes into folders the window no longer shows.
	Left  string `json:"left"`
	Right string `json:"right"`
}

func (s *SyncDirs) get(id int) (*syncSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.sess[id]
	if ss == nil {
		return nil, errors.New("This synchronize window is closed.")
	}
	return ss, nil
}

// idle returns the session when no compare or sync is running in it.
func (s *SyncDirs) idle(id int) (*syncSession, error) {
	ss, err := s.get(id)
	if err != nil {
		return nil, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if ss.busy {
		return nil, errors.New("Wait until the compare or the synchronization has finished.")
	}
	if ss.res == nil {
		return nil, errors.New("Compare the folders first.")
	}
	return ss, nil
}

// Open starts a session for one Synchronize window and returns its number.
func (s *SyncDirs) Open() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	s.sess[s.next] = &syncSession{}
	return s.next
}

// Close ends a session; a compare or sync still running in it is stopped.
func (s *SyncDirs) Close(id int) {
	s.mu.Lock()
	ss := s.sess[id]
	delete(s.sess, id)
	s.mu.Unlock()
	if ss != nil {
		ss.mu.Lock()
		if ss.cancel != nil {
			ss.cancel()
		}
		ss.mu.Unlock()
	}
}

// syncSamePair checks that the folders the window shows (when the page sends them) are the ones compared last.
func (s *SyncDirs) syncSamePair(ss *syncSession, left, right string) error {
	if left == "" && right == "" {
		return nil
	}
	ss.mu.Lock()
	cl, cr := ss.left, ss.right
	ss.mu.Unlock()
	l, errL := s.syncDir("left", left)
	r, errR := s.syncDir("right", right)
	if errL != nil || errR != nil || l != cl || r != cr {
		return errors.New("The folders changed since the last compare. Press Compare first.")
	}
	return nil
}

// syncDir checks that a path is a folder on this computer the engine can work on.
func (s *SyncDirs) syncDir(side, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("Choose the %s folder.", side)
	}
	if strings.Contains(p, "://") {
		return "", fmt.Errorf("The %s folder %s is not on this computer. Synchronize works with folders on this computer only.", side, p)
	}
	if strings.HasPrefix(p, "~") {
		if h, err := os.UserHomeDir(); err == nil {
			p = h + p[1:]
		}
	}
	p = filepath.Clean(p)
	fi, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("The %s folder %s is not there.", side, p)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("%s is not a folder.", p)
	}
	return p, nil
}

// Compare compares the two folders in the background. Progress comes as "syncdirs-progress" events, the end as
// "syncdirs-done" ({id, ok, cancelled, error}). Rows then gives the list.
func (s *SyncDirs) Compare(id int, r SyncCompareRequest) error {
	if s.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so they can't be synchronized.")
	}
	ss, err := s.get(id)
	if err != nil {
		return err
	}
	left, err := s.syncDir("left", r.Left)
	if err != nil {
		return err
	}
	right, err := s.syncDir("right", r.Right)
	if err != nil {
		return err
	}
	opts := r.Options
	opts.Selected = r.Selected
	if _, err := syncdirs.CompileFilter(opts.Filter, nil); err != nil {
		return fmt.Errorf("The file types to compare are not valid: %v", err)
	}
	ss.mu.Lock()
	if ss.busy {
		ss.mu.Unlock()
		return errors.New("A compare or synchronization is already running.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ss.busy, ss.cancel = true, cancel
	ss.left, ss.right = left, right
	ss.lfs, ss.rfs = syncdirs.DirFS(left), syncdirs.DirFS(right)
	ss.res = nil // the old result belongs to the old folders
	lfs, rfs := ss.lfs, ss.rfs
	ss.mu.Unlock()

	go func() {
		var last time.Time
		res, err := syncdirs.Compare(ctx, lfs, rfs, opts, func(p syncdirs.CompareProgress) {
			if time.Since(last) < 100*time.Millisecond {
				return
			}
			last = time.Now()
			s.app.emit("syncdirs-progress", map[string]any{"id": id, "dirs": p.Dirs, "files": p.Files, "current": p.Current})
		})
		cancelled := ctx.Err() != nil
		cancel()
		ss.mu.Lock()
		ss.busy, ss.cancel = false, nil
		if res != nil {
			ss.res = res
		}
		ss.mu.Unlock()
		done := map[string]any{"id": id, "ok": err == nil, "cancelled": cancelled, "error": ""}
		if err != nil && !cancelled {
			done["error"] = syncErrText(err)
		}
		s.app.emit("syncdirs-done", done)
	}()
	return nil
}

// syncErrText makes an engine error a sentence for the page.
func syncErrText(err error) string {
	var pe syncdirs.PathError
	if errors.As(err, &pe) {
		if pe.Path == "." {
			return fmt.Sprintf("The %s folder could not be read: %v", pe.Side, pe.Err)
		}
		return fmt.Sprintf("%s (%s side): %v", pe.Path, pe.Side, pe.Err)
	}
	return err.Error()
}

// Cancel stops the compare or the sync in progress; the rows compared so far stay.
func (s *SyncDirs) Cancel(id int) {
	if ss, err := s.get(id); err == nil {
		ss.mu.Lock()
		if ss.cancel != nil {
			ss.cancel()
		}
		ss.mu.Unlock()
	}
}

// Rows is the list under the Show buttons, in tree order, with the counts for the status line.
func (s *SyncDirs) Rows(id int, show syncdirs.Show) (*SyncView, error) {
	ss, err := s.get(id)
	if err != nil {
		return nil, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	v := &SyncView{Rows: []*syncdirs.Item{}, Errors: []string{}, Left: ss.left, Right: ss.right}
	if ss.res == nil || ss.busy {
		return v, nil
	}
	v.Compared = true
	v.Rows = ss.res.Visible(show)
	v.Summary = ss.res.Summary()
	for _, e := range ss.res.Errors {
		v.Errors = append(v.Errors, syncErrText(e))
	}
	return v, nil
}

// SetAction gives the rows an action (a folder row with tree also everything in it) and returns how many changed.
func (s *SyncDirs) SetAction(id int, paths []string, a syncdirs.Action, tree bool) (int, error) {
	ss, err := s.idle(id)
	if err != nil {
		return 0, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	n := 0
	for _, p := range paths {
		if tree {
			k, err := ss.res.SetActionTree(p, a)
			if err == nil {
				n += k
			}
			continue
		}
		if ss.res.SetAction(p, a) == nil {
			n++
		}
	}
	if n == 0 && len(paths) > 0 && a != syncdirs.NoAction {
		return 0, errors.New("That action is not possible for the chosen rows.")
	}
	return n, nil
}

// Reverse turns the copy direction of the rows around (the click on the arrow) and returns how many changed.
func (s *SyncDirs) Reverse(id int, paths []string) (int, error) {
	ss, err := s.idle(id)
	if err != nil {
		return 0, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	n := 0
	for _, p := range paths {
		if ss.res.ReverseAction(p) == nil {
			n++
		}
	}
	if n == 0 && len(paths) > 0 {
		return 0, errors.New("The copy direction of these rows can't be turned around.")
	}
	return n, nil
}

// Select selects or unselects the actions of rows (with tree a folder row takes everything in it; no paths = all).
func (s *SyncDirs) Select(id int, paths []string, on, tree bool) (int, error) {
	ss, err := s.idle(id)
	if err != nil {
		return 0, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if len(paths) == 0 {
		return ss.res.SelectTree("", on)
	}
	n := 0
	for _, p := range paths {
		if tree {
			k, _ := ss.res.SelectTree(p, on)
			n += k
		} else if ss.res.SetSelected(p, on) == nil {
			n++
		}
	}
	return n, nil
}

// Reset puts every row back to the action the compare proposed.
func (s *SyncDirs) Reset(id int) error {
	ss, err := s.idle(id)
	if err != nil {
		return err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.res.ResetActions()
	return nil
}

// Recompare reads one pair again (after the compare tool changed one of the files).
func (s *SyncDirs) Recompare(id int, path string) error {
	ss, err := s.idle(id)
	if err != nil {
		return err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.res.Recompare(context.Background(), path)
}

// Plan counts what Synchronize would do with the given checkboxes (for the Synchronize dialog).
func (s *SyncDirs) Plan(id int, f syncdirs.PlanFilter) (*syncdirs.Plan, error) {
	ss, err := s.idle(id)
	if err != nil {
		return nil, err
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	p := ss.res.Plan(f)
	p.Ops = nil // the page needs the counts only
	return p, nil
}

// Synchronize runs the plan in the background: "syncdirs-exec" events while it works, "syncdirs-ask" when a file would
// be replaced and the user wants to be asked (answer with Answer), "syncdirs-exec-done" at the end.
func (s *SyncDirs) Synchronize(id int, r SyncExecRequest) error {
	ss, err := s.idle(id)
	if err != nil {
		return err
	}
	if err := s.syncSamePair(ss, r.Left, r.Right); err != nil {
		return err
	}
	ss.mu.Lock()
	plan := ss.res.Plan(r.Filter)
	if len(plan.Ops) == 0 {
		ss.mu.Unlock()
		return errors.New("There is nothing to synchronize with these choices.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	ss.busy, ss.cancel = true, cancel
	ss.answer = make(chan syncdirs.OverwriteDecision, 1)
	lfs, rfs, answer := ss.lfs, ss.rfs, ss.answer
	ss.mu.Unlock()

	opts := syncdirs.ExecOptions{}
	var last time.Time
	opts.Progress = func(p syncdirs.Progress) {
		if p.Op != nil && time.Since(last) < 80*time.Millisecond && p.OpsDone < p.OpsTotal {
			return
		}
		last = time.Now()
		ev := map[string]any{"id": id, "opsDone": p.OpsDone, "opsTotal": p.OpsTotal, "bytesDone": p.BytesDone + p.FileDone,
			"bytesTotal": p.BytesTotal, "current": "", "kind": ""}
		if p.Op != nil {
			ev["current"], ev["kind"], ev["side"] = p.Op.Dst, p.Op.Kind, p.Op.Side
		}
		s.app.emit("syncdirs-exec", ev)
	}
	if r.ConfirmOverwrite {
		opts.ConfirmOverwrite = func(op *syncdirs.Op) syncdirs.OverwriteDecision {
			q := map[string]any{"id": id, "target": op.Dst, "side": op.Side}
			if it := op.Item; it != nil {
				src, dst := it.Left, it.Right
				if op.Side == syncdirs.LeftSide {
					src, dst = it.Right, it.Left
				}
				if src != nil {
					q["srcSize"], q["srcTime"] = src.Size, src.ModTime.UnixMilli()
				}
				if dst != nil {
					q["dstSize"], q["dstTime"] = dst.Size, dst.ModTime.UnixMilli()
				}
			}
			s.app.emit("syncdirs-ask", q)
			select {
			case d := <-answer:
				return d
			case <-ctx.Done():
				return syncdirs.CancelSync
			}
		}
	}
	if r.Trash {
		opts.Trash = syncdirs.SystemTrash
	}
	go func() {
		rep, err := syncdirs.Execute(ctx, plan, lfs, rfs, opts)
		cancelled := ctx.Err() != nil || errors.Is(err, context.Canceled)
		cancel()
		ss.mu.Lock()
		ss.busy, ss.cancel, ss.answer = false, nil, nil
		ss.mu.Unlock()
		done := map[string]any{"id": id, "cancelled": cancelled, "error": "", "done": 0, "skipped": 0, "bytes": 0, "errors": []string{}}
		if rep != nil {
			errs := []string{}
			for _, e := range rep.Errors {
				errs = append(errs, syncOpErrText(e))
			}
			done["done"], done["skipped"], done["bytes"], done["errors"] = rep.Done, rep.Skipped, rep.Bytes, errs
		}
		if err != nil && !cancelled {
			done["error"] = syncErrText(err)
		}
		s.app.emit("syncdirs-exec-done", done)
	}()
	return nil
}

// syncOpErrText says in words which operation failed and why.
func syncOpErrText(e syncdirs.OpError) string {
	verb := map[syncdirs.OpKind]string{syncdirs.OpMkdir: "Creating folder", syncdirs.OpCopyFile: "Copying", syncdirs.OpCopyLink: "Copying link",
		syncdirs.OpDelete: "Deleting", syncdirs.OpRemoveDir: "Removing folder"}[e.Op.Kind]
	if verb == "" {
		verb = e.Op.Kind.String()
	}
	return fmt.Sprintf("%s %s (%s side): %v", verb, e.Op.Dst, e.Op.Side, e.Err)
}

// Answer answers the overwrite question of a running sync: overwrite, skip, overwrite_all, skip_all or cancel.
func (s *SyncDirs) Answer(id int, choice string) error {
	ss, err := s.get(id)
	if err != nil {
		return err
	}
	d := map[string]syncdirs.OverwriteDecision{"overwrite": syncdirs.Overwrite, "skip": syncdirs.SkipFile, "overwrite_all": syncdirs.OverwriteAll,
		"skip_all": syncdirs.SkipAll, "cancel": syncdirs.CancelSync}
	dec, ok := d[choice]
	if !ok {
		return fmt.Errorf("unknown answer %q", choice)
	}
	ss.mu.Lock()
	ch := ss.answer
	ss.mu.Unlock()
	if ch == nil {
		return errors.New("Nothing is waiting for an answer.")
	}
	select {
	case ch <- dec:
	default:
	}
	return nil
}

func syncProfilesFile() (string, error) {
	d := configDir()
	if d == "" {
		return "", errors.New("There is no settings folder to keep the saved settings in.")
	}
	return filepath.Join(d, "syncdirs.json"), nil
}

// Profiles lists the saved synchronize settings (both folders, the file types and every option).
func (s *SyncDirs) Profiles() ([]syncdirs.Profile, error) {
	f, err := syncProfilesFile()
	if err != nil {
		return nil, err
	}
	ps, err := syncdirs.LoadProfiles(f)
	if ps == nil {
		ps = []syncdirs.Profile{}
	}
	return ps, err
}

// SaveProfile saves settings under their name, replacing settings of the same name.
func (s *SyncDirs) SaveProfile(p syncdirs.Profile) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return errors.New("Give the settings a name.")
	}
	f, err := syncProfilesFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	ps, err := syncdirs.LoadProfiles(f)
	if err != nil {
		return err
	}
	return syncdirs.SaveProfiles(f, syncdirs.PutProfile(ps, p))
}

// DeleteProfile removes saved settings by name.
func (s *SyncDirs) DeleteProfile(name string) error {
	f, err := syncProfilesFile()
	if err != nil {
		return err
	}
	ps, err := syncdirs.LoadProfiles(f)
	if err != nil {
		return err
	}
	return syncdirs.SaveProfiles(f, syncdirs.DeleteProfile(ps, name))
}
