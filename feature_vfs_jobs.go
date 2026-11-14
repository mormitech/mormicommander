package main

// Jobs that involve archives or servers: F5/F6/F8 out of, into and inside archives (and FTP/SFTP folders), Pack
// (Alt+F5, Alt+Shift+F5), Unpack (Alt+F9) and Test (Alt+Shift+F9). internal/fileops copies between local folders
// only, so the page's runJob (core/jobs.js) first offers every job to VfsJobs.Start: it answers 0 for a plain local
// copy (which then goes to the fileops queue as before) and takes everything else. A VfsJobs job sends the same
// "job", "job-ask" and "job-done" events with a fileops.Progress, so the progress box and the overwrite question of
// core/jobs.js serve both; job numbers start at vfsJobBase so they never meet the fileops queue's.
//
// How the work is done, so that each archive is rewritten once and the overwrite question works the same everywhere:
//   - out of an archive (or a server) into a local folder: unpack/download into a hidden staging folder inside the
//     target (same volume), then move each file into place, asking when a file is in the way;
//   - into an archive: the local files (staged first when they come from another archive or a server) are added in one
//     rewrite (vfs.Router.AddLocal) after the question for names already in the archive;
//   - inside one archive (F6): a rename in the archive; delete (F8): the archive's own delete;
//   - "move": the sources are removed only after their copy succeeded and nothing of them was skipped.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/fileops"
	"mormicommander/internal/vfs"
)

const vfsJobBase = 1 << 30

// VfsJobs is what the page calls for jobs on archives and servers (window.go.main.VfsJobs).
type VfsJobs struct {
	app  *App
	mu   sync.Mutex
	seq  int
	jobs map[int]*vfsJob
	// started, when set, sees each job as it is registered (tests: a short job can be over before anyone looks it up).
	started func(*vfsJob)
}

func init() {
	registerFeature("vfsjobs", func(a *App) []exposed { return []exposed{expose(&VfsJobs{app: a, jobs: map[int]*vfsJob{}})} })
}

// VfsRequest is a job for VfsJobs.Start: the fields of fileops.Request the copy dialog fills, plus the options of the
// Pack and Unpack dialogs.
type VfsRequest struct {
	Kind      string         `json:"kind"` // copy, move, delete, trash, wipe, pack, unpack, test
	Sources   []string       `json:"sources"`
	Target    string         `json:"target"` // copy/move/unpack: a folder; pack: the archive (may hold * with separate)
	Rename    string         `json:"rename"`
	Overwrite fileops.Policy `json:"overwrite"`
	// Pack dialog
	Format     string `json:"format"`
	Level      int    `json:"level"`
	Password   string `json:"password"` // pack: encrypt with it; unpack/test: tried before asking
	Encryption string `json:"encryption"`
	NoPaths    bool   `json:"noPaths"` // pack: don't store folder names; unpack: don't create them
	NoRecurse  bool   `json:"noRecurse"`
	VolumeSize int64  `json:"volumeSize"`
	Move       bool   `json:"move"`
	Separate   bool   `json:"separate"` // pack: one archive per item; unpack: one subfolder per archive
	// Unpack dialog
	Mask string `json:"mask"`
}

// vfsReport is one archive's line in the Test result.
type vfsReport struct {
	Archive  string   `json:"archive"`
	OK       bool     `json:"ok"`
	Files    int      `json:"files"`
	Problems []string `json:"problems,omitempty"`
}

// vfsProgress is a fileops.Progress plus the Test report.
type vfsProgress struct {
	fileops.Progress
	Report []vfsReport `json:"report,omitempty"`
}

type vfsJob struct {
	js       *VfsJobs
	req      VfsRequest
	ctx      context.Context
	cancel   context.CancelFunc
	answers  chan fileops.Answer
	mu       sync.Mutex
	p        vfsProgress
	policy   fileops.Policy
	start    time.Time
	lastEmit time.Time
}

// Start starts a job on archives or servers and returns its number, or 0 when every path is a plain local one (the
// page then hands the job to Jobs.Start).
func (js *VfsJobs) Start(req VfsRequest) (int, error) {
	r := vfsRouter(js.app)
	if r == nil {
		if js.app.mock != nil && (req.Kind == "pack" || req.Kind == "unpack" || req.Kind == "test") {
			return 0, errors.New("This is the demo: its files are not real, so they can't be packed or unpacked.")
		}
		return 0, nil
	}
	switch req.Kind {
	case "copy", "move", "delete", "trash", "wipe":
		local := req.Kind == "delete" || req.Kind == "trash" || req.Kind == "wipe" || vfsPathKind(r, req.Target) == "local"
		for _, s := range req.Sources {
			local = local && vfsIsLocal(r, s)
		}
		if local {
			return 0, nil
		}
	case "pack", "unpack", "test":
	default:
		return 0, fmt.Errorf("Unknown job %q.", req.Kind)
	}
	if len(req.Sources) == 0 {
		return 0, errors.New("Nothing is selected.")
	}
	// an archive that can't be changed says so before any question is asked (board #143)
	if req.Kind == "copy" || req.Kind == "move" {
		if why := vfsArchiveReadOnly(r, req.Target, true); why != "" {
			return 0, errors.New(why)
		}
	}
	if req.Kind == "delete" || req.Kind == "trash" {
		for _, s := range req.Sources {
			if why := vfsArchiveReadOnly(r, s, false); why != "" {
				return 0, errors.New(why)
			}
		}
	}
	if req.Kind != "delete" && req.Kind != "trash" && req.Kind != "wipe" && req.Kind != "test" && strings.TrimSpace(req.Target) == "" {
		return 0, errors.New("No target is given.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	js.mu.Lock()
	js.seq++
	id := vfsJobBase + js.seq
	job := &vfsJob{js: js, req: req, ctx: ctx, cancel: cancel, answers: make(chan fileops.Answer, 1), policy: req.Overwrite, start: time.Now()}
	job.p.ID, job.p.Kind, job.p.State, job.p.Target, job.p.ETA = id, fileops.Kind(req.Kind), fileops.StateRunning, req.Target, -1
	js.jobs[id] = job
	if js.started != nil {
		js.started(job)
	}
	js.mu.Unlock()
	go job.run()
	return id, nil
}

// Cancel stops a job; what is done stays done (archives are only replaced when a rewrite completed).
func (js *VfsJobs) Cancel(id int) bool {
	js.mu.Lock()
	job := js.jobs[id]
	js.mu.Unlock()
	if job == nil {
		return false
	}
	job.cancel()
	return true
}

// Answer answers the question a job asks (a file is in the way).
func (js *VfsJobs) Answer(id int, a fileops.Answer) error {
	js.mu.Lock()
	job := js.jobs[id]
	js.mu.Unlock()
	if job == nil {
		return errors.New("That job is over.")
	}
	select {
	case job.answers <- a:
	default:
	}
	return nil
}

// Running says whether a job on archives or servers is still at work.
func (js *VfsJobs) Running() bool {
	js.mu.Lock()
	defer js.mu.Unlock()
	return len(js.jobs) > 0
}

// Packers lists the Pack dialog's packers, with what each one can do.
func (js *VfsJobs) Packers() []vfsPacker { return vfsPackers() }

// ---- job plumbing ----

func (job *vfsJob) emit(event string) {
	job.mu.Lock()
	job.p.Elapsed = time.Since(job.start).Milliseconds()
	p := job.p
	p.Errors = append([]string(nil), job.p.Errors...)
	job.lastEmit = time.Now()
	job.mu.Unlock()
	job.js.app.emit(event, p)
}

// update changes the progress and sends it, at most five times a second.
func (job *vfsJob) update(fn func(p *vfsProgress)) {
	job.mu.Lock()
	fn(&job.p)
	due := time.Since(job.lastEmit) > 200*time.Millisecond
	job.mu.Unlock()
	if due {
		job.emit(fileops.EventProgress)
	}
}

func (job *vfsJob) fail(msg string) {
	job.mu.Lock()
	if len(job.p.Errors) < 100 {
		job.p.Errors = append(job.p.Errors, msg)
	}
	job.mu.Unlock()
}

func (job *vfsJob) progressFn() vfsProgressFunc {
	var base int64 // bytes of earlier steps
	var last int64
	return func(name string, done, total int64, files int) {
		if done < last {
			base += last
		}
		last = done
		job.update(func(p *vfsProgress) {
			if name != "" {
				p.Current = name
			}
			p.BytesDone = base + done
			if total > 0 && base+total > p.Bytes {
				p.Bytes = base + total
			}
		})
	}
}

func (job *vfsJob) password() vfsPasswordFunc {
	return func(arc, entry string, attempt int) (string, bool) {
		if attempt == 1 && job.req.Password != "" && job.req.Kind != "pack" {
			return job.req.Password, true
		}
		return vfsAskPassword(job.js.app, arc, entry, attempt)
	}
}

func (job *vfsJob) run() {
	defer job.cancel()
	var err error
	switch job.req.Kind {
	case "copy", "move":
		err = job.runCopy()
	case "delete", "trash", "wipe":
		err = job.runDelete()
	case "pack":
		err = job.runPack()
	case "unpack":
		err = job.runUnpack()
	case "test":
		err = job.runTest()
	}
	job.mu.Lock()
	switch {
	case job.ctx.Err() != nil:
		job.p.State = fileops.StateCancelled
	case err != nil:
		job.p.Errors = append(job.p.Errors, vfsSentence(err))
		job.p.State = fileops.StateFailed
	case len(job.p.Errors) > 0:
		job.p.State = fileops.StateFailed
	default:
		job.p.State = fileops.StateDone
	}
	if job.p.Summary == "" {
		job.p.Summary = vfsSummary(&job.p)
	}
	job.p.Ask, job.p.Current, job.p.ETA = nil, "", 0
	job.mu.Unlock()
	job.emit(fileops.EventDone)
	job.js.mu.Lock()
	delete(job.js.jobs, job.p.ID)
	job.js.mu.Unlock()
}

// vfsSentence makes an error a sentence for the job's error list.
func vfsSentence(err error) string {
	t := vfsErrText(err)
	if t == "" {
		return ""
	}
	t = strings.ToUpper(t[:1]) + t[1:]
	if !strings.HasSuffix(t, ".") {
		t += "."
	}
	return t
}

func vfsSummary(p *vfsProgress) string {
	verb := map[fileops.Kind]string{"copy": "Copied", "move": "Moved", "delete": "Deleted", "trash": "Deleted", "wipe": "Deleted",
		"pack": "Packed", "unpack": "Unpacked", "test": "Tested"}[p.Kind]
	s := fmt.Sprintf("%s %d file(s)", verb, p.FilesDone)
	if p.Skipped > 0 {
		s += fmt.Sprintf(", skipped %d", p.Skipped)
	}
	if n := len(p.Errors); n > 0 {
		s += fmt.Sprintf(", %d error(s)", n)
	}
	return s + "."
}

// ask asks the page a question and waits for the answer (cancel when the job is stopped).
func (job *vfsJob) ask(c fileops.Conflict) fileops.Answer {
	for {
		select {
		case <-job.answers: // (drop a stale answer)
			continue
		default:
		}
		break
	}
	job.mu.Lock()
	job.p.State, job.p.Ask = fileops.StateAsking, &c
	job.mu.Unlock()
	job.emit(fileops.EventAsk)
	var a fileops.Answer
	select {
	case a = <-job.answers:
	case <-job.ctx.Done():
		a = fileops.Answer{Choice: fileops.ChoiceCancel}
	}
	job.mu.Lock()
	job.p.State, job.p.Ask = fileops.StateRunning, nil
	job.mu.Unlock()
	job.emit(fileops.EventProgress)
	if a.Choice == fileops.ChoiceCancel {
		job.cancel()
	}
	return a
}

// decision is what to do with one file that is in the way.
type vfsDecision struct {
	act     string // "overwrite", "skip", "rename" (to name), "rename_target"
	name    string
	stopped bool
}

// resolve decides one conflict: by the job's standing policy, else by asking.
func (job *vfsJob) resolve(source, target string, sSize, dSize, sTime, dTime int64, freeName func(string) string) vfsDecision {
	name := vfsBase(target)
	for {
		switch job.policy {
		case fileops.Overwrite:
			return vfsDecision{act: "overwrite"}
		case fileops.Skip:
			return vfsDecision{act: "skip"}
		case fileops.Older:
			if sTime > dTime {
				return vfsDecision{act: "overwrite"}
			}
			return vfsDecision{act: "skip"}
		case fileops.Larger:
			if sSize > dSize {
				return vfsDecision{act: "overwrite"}
			}
			return vfsDecision{act: "skip"}
		case fileops.Smaller:
			if sSize < dSize {
				return vfsDecision{act: "overwrite"}
			}
			return vfsDecision{act: "skip"}
		case fileops.AutoRenameSource:
			return vfsDecision{act: "rename", name: freeName(name)}
		case fileops.AutoRenameTarget:
			if freeName != nil {
				return vfsDecision{act: "rename_target", name: freeName(name)}
			}
			return vfsDecision{act: "rename", name: freeName(name)}
		}
		a := job.ask(fileops.Conflict{Type: fileops.AskExists, Source: source, Target: target, SrcSize: sSize, DstSize: dSize,
			SrcTime: sTime, DstTime: dTime, Choices: []fileops.Choice{fileops.ChoiceOverwrite, fileops.ChoiceSkip,
				fileops.ChoiceOverwriteAll, fileops.ChoiceSkipAll, fileops.ChoiceOlderAll, fileops.ChoiceRename,
				fileops.ChoiceAutoRename, fileops.ChoiceAutoRenameAll, fileops.ChoiceCancel}})
		switch a.Choice {
		case fileops.ChoiceOverwrite:
			return vfsDecision{act: "overwrite"}
		case fileops.ChoiceSkip:
			return vfsDecision{act: "skip"}
		case fileops.ChoiceOverwriteAll:
			job.policy = fileops.Overwrite
		case fileops.ChoiceSkipAll:
			job.policy = fileops.Skip
		case fileops.ChoiceOlderAll:
			job.policy = fileops.Older
		case fileops.ChoiceLargerAll:
			job.policy = fileops.Larger
		case fileops.ChoiceSmallerAll:
			job.policy = fileops.Smaller
		case fileops.ChoiceAutoRenameAll:
			job.policy = fileops.AutoRenameSource
		case fileops.ChoiceRenameTargetAll:
			job.policy = fileops.AutoRenameTarget
		case fileops.ChoiceAutoRename:
			return vfsDecision{act: "rename", name: freeName(name)}
		case fileops.ChoiceRenameTarget:
			return vfsDecision{act: "rename_target", name: freeName(name)}
		case fileops.ChoiceRename:
			n := strings.TrimSpace(a.NewName)
			if n == "" || n == "." || n == ".." || strings.ContainsAny(n, `/\`) {
				job.fail(fmt.Sprintf("%s: %q is not a valid name; the file was skipped.", name, a.NewName))
				return vfsDecision{act: "skip"}
			}
			return vfsDecision{act: "rename", name: n}
		default:
			return vfsDecision{act: "skip", stopped: true}
		}
	}
}

// vfsBase is the last element of a local, archive or server path.
func vfsBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// vfsDir is everything before the last element of a path.
func vfsDir(p string) string {
	p = strings.TrimRight(p, `/\`)
	i := strings.LastIndexAny(p, `/\`)
	switch {
	case i < 0:
		return ""
	case i == 0:
		return p[:1]
	case i == 2 && p[1] == ':':
		return p[:3]
	}
	return p[:i]
}

// newName is the name a source gets in the target: its own, or the dialog's new name or mask.
func (job *vfsJob) newName(src string) string {
	name := vfsBase(src)
	rn := strings.TrimSpace(job.req.Rename)
	if rn == "" {
		return name
	}
	if len(job.req.Sources) == 1 && !strings.ContainsAny(rn, "*?") {
		return rn
	}
	if n := fileops.ApplyMask(rn, name); n != "" && n != "." && n != ".." {
		return n
	}
	return name
}

// ---- staging: getting files out of archives and servers ----

// stage puts local copies of sources into the local folder dir, each under its own name: archive entries are
// unpacked by the archive engine (one pass per archive), server files downloaded, local files copied. It returns the
// staged path per source ("" where it failed; the reason is in the job's errors).
func (job *vfsJob) stage(sources []string, dir string) []string {
	r := vfsRouter(job.js.app)
	out := make([]string, len(sources))
	type group struct {
		file, base string
		idx        []int
		paths      []string
	}
	groups := map[string]*group{}
	var order []string
	for i, s := range sources {
		s = r.Clean(s)
		if r.Kind(s) == vfs.KindArchive {
			if ai, err := r.Archives.Info(s); err == nil && ai.Inner != "" {
				base := ""
				if i := strings.LastIndexByte(ai.Inner, '/'); i >= 0 {
					base = ai.Inner[:i]
				}
				key := ai.File + "\x00" + base
				g := groups[key]
				if g == nil {
					g = &group{file: ai.File, base: base}
					groups[key] = g
					order = append(order, key)
				}
				g.idx = append(g.idx, i)
				g.paths = append(g.paths, ai.Inner)
				continue
			}
		}
		if err := job.ctx.Err(); err != nil {
			return out
		}
		dst := filepath.Join(dir, vfsBase(s))
		var err error
		if vfsIsLocal(r, s) {
			err = vfsCopyLocalTree(job.ctx, s, dst)
		} else {
			err = job.downloadTree(r, s, dst)
		}
		if err != nil {
			job.fail(fmt.Sprintf("%s: %s", vfsBase(s), vfsErrText(err)))
			continue
		}
		out[i] = dst
	}
	for _, key := range order {
		g := groups[key]
		if job.ctx.Err() != nil {
			return out
		}
		_, problems, err := vfsExtract(job.ctx, g.file, dir, vfsExtractOpts{Paths: g.paths, Base: g.base,
			Password: job.password(), Progress: job.progressFn()})
		for _, p := range problems {
			job.fail(p)
		}
		if err != nil {
			if job.ctx.Err() == nil {
				job.fail(fmt.Sprintf("%s: %s", vfsBase(g.file), vfsErrText(err)))
			}
			continue
		}
		for k, i := range g.idx {
			p := filepath.Join(dir, filepath.FromSlash(vfsBase(g.paths[k])))
			if _, err := os.Lstat(p); err == nil {
				out[i] = p
			}
		}
	}
	return out
}

// downloadTree copies a file or folder from any panel place to the local path dst.
func (job *vfsJob) downloadTree(r *vfs.Router, src, dst string) error {
	if err := job.ctx.Err(); err != nil {
		return err
	}
	e, err := r.Stat(src)
	if err != nil {
		return err
	}
	if e.Flags&vfs.FlagDir == 0 {
		job.update(func(p *vfsProgress) { p.Current = e.Name; p.Bytes += e.Size })
		_, err := r.Download(job.ctx, src, dst, vfs.TransferOptions{PreserveTime: true, Progress: func(done, total int64) {}})
		if err == nil {
			job.update(func(p *vfsProgress) { p.BytesDone += e.Size })
		}
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	l, err := r.List(job.ctx, src)
	if err != nil {
		return err
	}
	for _, c := range l.Entries {
		if err := job.downloadTree(r, r.Join(src, c.Name), filepath.Join(dst, c.Name)); err != nil {
			return err
		}
	}
	return nil
}

// vfsCopyLocalTree copies a local file or folder (links as links) to dst.
func vfsCopyLocalTree(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		t, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(t, dst)
	case fi.IsDir():
		if err := os.MkdirAll(dst, fi.Mode().Perm()|0o700); err != nil {
			return err
		}
		des, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, d := range des {
			if err := vfsCopyLocalTree(ctx, filepath.Join(src, d.Name()), filepath.Join(dst, d.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}

// merge moves the staged file or folder src into the local folder dstDir as name, asking when a file is in the way.
// shown is where the file really comes from (the entry in the archive, or the file on the server): the "Replace
// file?" question names it rather than the hidden staging folder the file waits in. It reports whether everything
// of it arrived (nothing skipped or failed).
func (job *vfsJob) merge(src, shown, dstDir, name string) bool {
	if job.ctx.Err() != nil {
		return false
	}
	d := filepath.Join(dstDir, name)
	sfi, err := os.Lstat(src)
	if err != nil {
		job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
		return false
	}
	dfi, derr := os.Lstat(d)
	if sfi.IsDir() {
		if derr != nil {
			if err := os.Rename(src, d); err != nil {
				job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
				return false
			}
			n := vfsCountFiles(d)
			job.update(func(p *vfsProgress) { p.FilesDone += n; p.Files += n })
			return true
		}
		if !dfi.IsDir() {
			job.fail(fmt.Sprintf("%s: a file with this name is in the way of the folder.", d))
			return false
		}
		des, err := os.ReadDir(src)
		if err != nil {
			job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
			return false
		}
		ok := true
		for _, de := range des {
			ok = job.merge(filepath.Join(src, de.Name()), vfsShownJoin(job, shown, de.Name()), d, de.Name()) && ok
		}
		return ok
	}
	job.update(func(p *vfsProgress) { p.Current = name; p.Files++ })
	if derr == nil {
		if dfi.IsDir() {
			job.fail(fmt.Sprintf("%s: a folder with this name is in the way.", d))
			return false
		}
		dec := job.resolve(shown, d, sfi.Size(), dfi.Size(), sfi.ModTime().UnixMilli(), dfi.ModTime().UnixMilli(),
			func(n string) string { return fileops.UniqueName(dstDir, n) })
		switch dec.act {
		case "skip":
			job.update(func(p *vfsProgress) { p.Skipped++ })
			return false
		case "rename":
			job.update(func(p *vfsProgress) { p.Files-- })
			return job.merge(src, shown, dstDir, dec.name)
		case "rename_target":
			if err := os.Rename(d, filepath.Join(dstDir, dec.name)); err != nil {
				job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
				return false
			}
		}
	}
	if err := os.Rename(src, d); err != nil {
		job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
		return false
	}
	job.update(func(p *vfsProgress) { p.FilesDone++ })
	return true
}

// vfsShownJoin adds name to a shown source path (an archive, server or local path), for the question merge asks.
func vfsShownJoin(job *vfsJob, dir, name string) string {
	if r := vfsRouter(job.js.app); r != nil {
		return r.Join(dir, name)
	}
	return filepath.Join(dir, name)
}

// vfsCountFiles counts the files in a local tree.
func vfsCountFiles(dir string) int {
	n := 0
	filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// ---- copy and move ----

func (job *vfsJob) runCopy() error {
	r := vfsRouter(job.js.app)
	target := r.Clean(job.req.Target)
	move := job.req.Kind == "move"
	sources := make([]string, len(job.req.Sources))
	for i, s := range job.req.Sources {
		sources[i] = r.Clean(s)
		if sources[i] == target || strings.HasPrefix(target, sources[i]+"/") || strings.HasPrefix(target, sources[i]+`\`) {
			if vfsPathKind(r, sources[i]) != "archive" || vfsPathKind(r, target) != "local" {
				return fmt.Errorf("%s can't be copied into itself", vfsBase(sources[i]))
			}
		}
	}
	tkind := vfsPathKind(r, target)
	// F6 inside one archive or on one server: a rename there.
	if move && tkind != "local" {
		var rest []string
		for _, s := range sources {
			if job.ctx.Err() != nil {
				return nil
			}
			if vfsSamePlace(r, s, target) {
				err := vfs.Rename(job.ctx, r, s, r.Join(target, job.newName(s)))
				if err == nil {
					job.update(func(p *vfsProgress) { p.FilesDone++; p.Files++ })
					continue
				}
				if !errors.Is(err, vfs.ErrCrossFS) && !errors.Is(err, vfs.ErrUnsupported) {
					job.fail(fmt.Sprintf("%s: %s", vfsBase(s), vfsErrText(err)))
					continue
				}
			}
			rest = append(rest, s)
		}
		sources = rest
		if len(sources) == 0 {
			return nil
		}
	}
	var arrived []bool
	if tkind == "local" {
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		stage, err := os.MkdirTemp(target, ".mc-copy-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		staged := job.stage(sources, stage)
		arrived = make([]bool, len(sources))
		for i, s := range staged {
			if s == "" || job.ctx.Err() != nil {
				continue
			}
			arrived[i] = job.merge(s, sources[i], target, job.newName(sources[i]))
		}
	} else {
		var err error
		arrived, err = job.copyInto(r, sources, target)
		if err != nil {
			return err
		}
	}
	if move && job.ctx.Err() == nil {
		for i, s := range sources {
			if !arrived[i] {
				continue
			}
			var err error
			if vfsIsLocal(r, s) {
				err = os.RemoveAll(s)
			} else {
				err = vfs.RemoveAll(job.ctx, r, s)
			}
			if err != nil {
				job.fail(fmt.Sprintf("%s was copied but could not be removed: %s", vfsBase(s), vfsErrText(err)))
			}
		}
	}
	return nil
}

// vfsSamePlace says whether two paths are in the same archive file or on the same server (so a move is a rename).
func vfsSamePlace(r *vfs.Router, a, b string) bool {
	if r.Kind(a) == vfs.KindLocal {
		return false
	}
	ka, kb := vfsPathKind(r, a), vfsPathKind(r, b)
	if ka != kb || ka == "local" {
		return false
	}
	if ka == "archive" {
		ia, err1 := r.Archives.Info(a)
		ib, err2 := r.Archives.Info(b)
		return err1 == nil && err2 == nil && ia.File == ib.File && ia.Inner != ""
	}
	return r.Root(a) == r.Root(b)
}

// copyInto copies sources into a folder in an archive or on a server: the files are staged locally when needed, the
// names already there are asked about, and then everything is added at once (one archive rewrite).
func (job *vfsJob) copyInto(r *vfs.Router, sources []string, target string) ([]bool, error) {
	arrived := make([]bool, len(sources))
	if why := vfsArchiveReadOnly(r, target, true); why != "" {
		return arrived, errors.New(why)
	}
	if c := r.Caps(target); !c.Write && vfsPathKind(r, target) == "archive" {
		return arrived, errors.New("this archive can't be changed (its format is read-only here); unpack it or pack a new one with Alt+F5")
	}
	needStage := false
	for _, s := range sources {
		if !vfsIsLocal(r, s) || job.newName(s) != vfsBase(s) {
			needStage = true
		}
	}
	locals := append([]string(nil), sources...)
	var stage string
	if needStage {
		var err error
		if stage, err = vfsTempDir("stage"); err != nil {
			return arrived, err
		}
		defer os.RemoveAll(stage)
		locals = job.stage(sources, stage)
		for i, l := range locals {
			if l == "" {
				continue
			}
			if n := job.newName(sources[i]); n != filepath.Base(l) {
				nl := filepath.Join(stage, n)
				if err := os.Rename(l, nl); err != nil {
					job.fail(fmt.Sprintf("%s: %s", n, vfsErrText(err)))
					locals[i] = ""
					continue
				}
				locals[i] = nl
			}
		}
	}
	if job.ctx.Err() != nil {
		return arrived, nil
	}
	var add []string
	var idx []int
	for i, l := range locals {
		if l == "" {
			continue
		}
		fi, err := os.Lstat(l)
		if err != nil {
			job.fail(fmt.Sprintf("%s: %s", vfsBase(l), vfsErrText(err)))
			continue
		}
		name := filepath.Base(l)
		dst := r.Join(target, name)
		if e, err := r.Stat(dst); err == nil {
			isDir := e.Flags&vfs.FlagDir != 0
			if isDir != fi.IsDir() {
				job.fail(fmt.Sprintf("%s: a %s with this name is already there.", name, map[bool]string{true: "folder", false: "file"}[isDir]))
				continue
			}
			if !isDir {
				taken := func(n string) bool { _, err := r.Stat(r.Join(target, n)); return err == nil }
				dec := job.resolve(l, dst, fi.Size(), e.Size, fi.ModTime().UnixMilli(), e.MTime, func(n string) string {
					return vfsFreeName(n, taken)
				})
				if dec.stopped {
					return arrived, nil
				}
				switch dec.act {
				case "skip":
					job.update(func(p *vfsProgress) { p.Skipped++ })
					continue
				case "rename", "rename_target":
					if dec.act == "rename_target" {
						if err := vfs.Rename(job.ctx, r, dst, r.Join(target, dec.name)); err != nil {
							job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
							continue
						}
						break
					}
					if stage == "" {
						var err error
						if stage, err = vfsTempDir("stage"); err != nil {
							return arrived, err
						}
						defer os.RemoveAll(stage)
					}
					sub, err := os.MkdirTemp(stage, "r")
					if err == nil {
						nl := filepath.Join(sub, dec.name)
						if err = vfsCopyLocalTree(job.ctx, l, nl); err == nil {
							l = nl
						}
					}
					if err != nil {
						job.fail(fmt.Sprintf("%s: %s", name, vfsErrText(err)))
						continue
					}
				}
			}
		}
		add = append(add, l)
		idx = append(idx, i)
	}
	if len(add) == 0 || job.ctx.Err() != nil {
		return arrived, nil
	}
	n := 0
	for _, a := range add {
		n += max(1, vfsCountFiles(a))
	}
	job.update(func(p *vfsProgress) { p.Files += n; p.Current = vfsBase(target) })
	if err := r.AddLocal(job.ctx, target, add); err != nil {
		return arrived, err
	}
	job.update(func(p *vfsProgress) { p.FilesDone += n })
	for _, i := range idx {
		arrived[i] = true
	}
	return arrived, nil
}

// vfsFreeName is "name(2).ext", "name(3).ext"... the first one taken says is free.
func vfsFreeName(name string, taken func(string) bool) string {
	if !taken(name) {
		return name
	}
	base, ext := vfs.SplitExt(name)
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s(%d)", base, i)
		if ext != "" {
			n += "." + ext
		}
		if !taken(n) {
			return n
		}
	}
}

// ---- delete ----

func (job *vfsJob) runDelete() error {
	r := vfsRouter(job.js.app)
	job.update(func(p *vfsProgress) { p.Files = len(job.req.Sources) })
	for _, s := range job.req.Sources {
		if job.ctx.Err() != nil {
			return nil
		}
		s = r.Clean(s)
		job.update(func(p *vfsProgress) { p.Current = vfsBase(s) })
		var err error
		if vfsIsLocal(r, s) {
			err = os.RemoveAll(s)
		} else {
			err = vfs.RemoveAll(job.ctx, r, s)
		}
		if err != nil {
			job.fail(fmt.Sprintf("%s: %s", vfsBase(s), vfsErrText(err)))
			continue
		}
		job.update(func(p *vfsProgress) { p.FilesDone++ })
	}
	return nil
}

// ---- pack ----

// packTargets pairs each archive to write with its sources: one archive, or one per item ("create separate archives").
func (job *vfsJob) packTargets(ext string) [][2]any {
	t := job.req.Target
	if !job.req.Separate {
		return [][2]any{{t, job.req.Sources}}
	}
	var out [][2]any
	for _, s := range job.req.Sources {
		name := vfsBase(s)
		stem := name
		if fi, err := os.Stat(s); err != nil || !fi.IsDir() {
			if b, e := vfs.SplitExt(name); e != "" {
				stem = b
			}
		}
		var at string
		if strings.Contains(vfsBase(t), "*") {
			at = vfsDir(t) + vfsSepOf(t) + strings.ReplaceAll(vfsBase(t), "*", stem)
		} else {
			at = vfsDir(t) + vfsSepOf(t) + stem + "." + ext
		}
		out = append(out, [2]any{at, []string{s}})
	}
	return out
}

func vfsSepOf(p string) string {
	if strings.Contains(p, "://") || !strings.Contains(p, `\`) {
		return "/"
	}
	return `\`
}

func (job *vfsJob) runPack() error {
	r := vfsRouter(job.js.app)
	pk, ok := vfsPackerFor(job.req.Format)
	if !ok {
		return fmt.Errorf("mormiCommander has no packer called %q", job.req.Format)
	}
	for _, pair := range job.packTargets(pk.Ext) {
		if job.ctx.Err() != nil {
			return nil
		}
		target, sources := r.Clean(pair[0].(string)), pair[1].([]string)
		if strings.Contains(vfsBase(target), "*") {
			return errors.New("the archive name can only hold * when separate archives are made")
		}
		allLocal := true
		for _, s := range sources {
			allLocal = allLocal && vfsIsLocal(r, s)
		}
		locals := sources
		if !allLocal {
			stage, err := vfsTempDir("stage")
			if err != nil {
				return err
			}
			defer os.RemoveAll(stage)
			locals = nil
			for _, l := range job.stage(sources, stage) {
				if l != "" {
					locals = append(locals, l)
				}
			}
			if len(locals) == 0 {
				continue
			}
		}
		localTarget := target
		remote := vfsPathKind(r, vfsDir(target)) != "local"
		if remote {
			if job.req.VolumeSize > 0 {
				return errors.New("a multi-volume archive can only be written to a local folder")
			}
			tmp, err := vfsTempDir("pack")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)
			localTarget = filepath.Join(tmp, vfsBase(target))
			if _, err := r.Stat(target); err == nil { // add to the archive that is there
				if _, err := r.Download(job.ctx, target, localTarget, vfs.TransferOptions{PreserveTime: true}); err != nil {
					job.fail(fmt.Sprintf("%s: %s", vfsBase(target), vfsErrText(err)))
					continue
				}
			}
		} else if err := os.MkdirAll(filepath.Dir(localTarget), 0o755); err != nil {
			return err
		}
		n := 0
		for _, l := range locals {
			n += max(1, vfsCountFiles(l))
		}
		job.update(func(p *vfsProgress) { p.Files += n; p.Current = vfsBase(target) })
		err := vfsPack(job.ctx, localTarget, locals, vfsPackOpts{Format: pk.Name, Level: job.req.Level, Password: job.req.Password,
			Encryption: job.req.Encryption, NoPaths: job.req.NoPaths, NoRecurse: job.req.NoRecurse, VolumeSize: job.req.VolumeSize,
			Move: job.req.Move && allLocal, Progress: job.progressFn()})
		if err != nil {
			if job.ctx.Err() == nil {
				job.fail(fmt.Sprintf("%s: %s", vfsBase(target), vfsErrText(err)))
			}
			continue
		}
		if remote {
			if _, err := r.Upload(job.ctx, localTarget, target, vfs.TransferOptions{PreserveTime: true}); err != nil {
				job.fail(fmt.Sprintf("%s: %s", vfsBase(target), vfsErrText(err)))
				continue
			}
		}
		job.update(func(p *vfsProgress) { p.FilesDone += n })
		if job.req.Move && !allLocal {
			for _, s := range sources {
				if err := vfs.RemoveAll(job.ctx, r, r.Clean(s)); err != nil {
					job.fail(fmt.Sprintf("%s was packed but could not be removed: %s", vfsBase(s), vfsErrText(err)))
				}
			}
		}
	}
	return nil
}

// ---- unpack and test ----

// localArchive gives a local file for an archive anywhere (archives in archives and on servers are copied out).
func (job *vfsJob) localArchive(r *vfs.Router, src string) (string, error) {
	src = r.Clean(src)
	if vfsIsLocal(r, src) {
		return src, nil
	}
	if vfsPathKind(r, src) == "archive" {
		if ai, err := r.Archives.Info(src); err == nil && ai.Inner == "" {
			return ai.File, nil // the archive file itself (local, or the temp copy of a remote or nested one)
		}
	}
	tmp, err := vfsTempDir("arc")
	if err != nil {
		return "", err
	}
	local := filepath.Join(tmp, vfsBase(src))
	if _, err := r.Download(job.ctx, src, local, vfs.TransferOptions{PreserveTime: true}); err != nil {
		return "", err
	}
	return local, nil
}

// archives picks the archives out of the sources (later volumes of a set are left out); none is an error.
func (job *vfsJob) archives(r *vfs.Router) ([][2]string, error) {
	var out [][2]string
	for _, s := range job.req.Sources {
		if job.ctx.Err() != nil {
			break
		}
		if e, err := r.Stat(r.Clean(s)); err == nil && e.Flags&vfs.FlagDir != 0 && vfsPathKind(r, s) != "archive" {
			continue
		}
		local, err := job.localArchive(r, s)
		if err != nil {
			job.fail(fmt.Sprintf("%s: %s", vfsBase(s), vfsErrText(err)))
			continue
		}
		if fi, err := os.Stat(local); err != nil || !fi.Mode().IsRegular() || vfsEngineOf(local) == "" || vfsLaterVolume(local) {
			continue
		}
		out = append(out, [2]string{s, local})
	}
	if len(out) == 0 && job.ctx.Err() == nil && len(job.p.Errors) == 0 {
		return nil, errors.New("no supported archive selected")
	}
	return out, nil
}

func (job *vfsJob) runUnpack() error {
	r := vfsRouter(job.js.app)
	arcs, err := job.archives(r)
	if err != nil {
		return err
	}
	target := r.Clean(job.req.Target)
	localTarget := vfsPathKind(r, target) == "local"
	for _, a := range arcs {
		if job.ctx.Err() != nil {
			return nil
		}
		src, file := a[0], a[1]
		dest := target
		if job.req.Separate {
			dest = r.Join(target, vfsArchiveStem(vfsBase(src)))
		}
		job.update(func(p *vfsProgress) { p.Current = vfsBase(src) })
		opts := vfsExtractOpts{Mask: job.req.Mask, NoPaths: job.req.NoPaths, Password: job.password(), Progress: job.progressFn()}
		var stage string
		if localTarget {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return err
			}
			if stage, err = os.MkdirTemp(dest, ".mc-unpack-"); err != nil {
				return err
			}
		} else if stage, err = vfsTempDir("unpack"); err != nil {
			return err
		}
		_, problems, err := vfsExtract(job.ctx, file, stage, opts)
		for _, p := range problems {
			job.fail(fmt.Sprintf("%s: %s", vfsBase(src), p))
		}
		if err != nil {
			os.RemoveAll(stage)
			if job.ctx.Err() == nil {
				job.fail(fmt.Sprintf("%s: %s", vfsBase(src), vfsErrText(err)))
			}
			continue
		}
		names, _ := vfsTopNames(stage)
		if localTarget {
			for _, n := range names {
				job.merge(filepath.Join(stage, n), vfsShownJoin(job, src, n), dest, n)
			}
		} else {
			paths := make([]string, len(names))
			for i, n := range names {
				paths[i] = filepath.Join(stage, n)
			}
			if job.req.Separate {
				if err := vfs.Mkdir(job.ctx, r, dest); err != nil && !errors.Is(err, os.ErrExist) {
					job.fail(fmt.Sprintf("%s: %s", vfsBase(dest), vfsErrText(err)))
					os.RemoveAll(stage)
					continue
				}
			}
			n := vfsCountFiles(stage)
			if err := r.AddLocal(job.ctx, dest, paths); err != nil {
				job.fail(fmt.Sprintf("%s: %s", vfsBase(src), vfsErrText(err)))
			} else {
				job.update(func(p *vfsProgress) { p.FilesDone += n; p.Files += n })
			}
		}
		os.RemoveAll(stage)
	}
	return nil
}

func (job *vfsJob) runTest() error {
	r := vfsRouter(job.js.app)
	arcs, err := job.archives(r)
	if err != nil {
		return err
	}
	bad := 0
	for _, a := range arcs {
		if job.ctx.Err() != nil {
			return nil
		}
		src, file := a[0], a[1]
		job.update(func(p *vfsProgress) { p.Current = vfsBase(src) })
		files, problems, err := vfsTest(job.ctx, file, job.password(), job.progressFn())
		if job.ctx.Err() != nil {
			return nil
		}
		if err != nil {
			problems = append(problems, vfsSentence(err))
		}
		rep := vfsReport{Archive: src, OK: len(problems) == 0, Files: files, Problems: problems}
		if !rep.OK {
			bad++
			for _, p := range problems {
				job.fail(fmt.Sprintf("%s: %s", vfsBase(src), p))
			}
		}
		job.update(func(p *vfsProgress) { p.Report = append(p.Report, rep); p.FilesDone += files; p.Files += files })
	}
	job.mu.Lock()
	if bad == 0 {
		job.p.Summary = "No errors found in the archive(s)."
	} else {
		job.p.Summary = fmt.Sprintf("%d archive(s) OK, %d with errors.", len(arcs)-bad, bad)
	}
	job.mu.Unlock()
	return nil
}
