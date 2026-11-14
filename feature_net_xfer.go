package main

// Copying and moving to, from and between servers (F-484, F-641, F-388): the file job engine (internal/fileops) works
// on the local disk only, so transfers that touch an FTP, SFTP or WebDAV panel run here, on top of the vfs router.
// Folders are copied with their structure; a file in the way is asked about like in TC's FTP copy, with "Resume" when
// the target is a shorter copy of the source (an aborted download or upload continues where it stopped: REST on FTP,
// offset writes on SFTP, an HTTP range on WebDAV). A move within one server is a server-side rename (F-638); across
// file systems it is a copy followed by deleting what was copied completely. A cancelled transfer leaves its partial
// file in place, so it can be resumed later.
//
// Events: "net-job" (progress), "net-job-ask" (a file is in the way: answer with AnswerJob), "net-job-done". The page
// side is runNetJob in frontend/src/features/net.js.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/ftp"
	"mormicommander/internal/remotefs"
	"mormicommander/internal/vfs"
)

// NetJobRequest is a transfer the page starts.
type NetJobRequest struct {
	Kind      string   `json:"kind"`      // "copy" or "move"
	Sources   []string `json:"sources"`   // full paths or URLs
	Target    string   `json:"target"`    // the destination folder
	Rename    string   `json:"rename"`    // a new name for a single source ("" keeps the name)
	Overwrite string   `json:"overwrite"` // "" asks; "overwrite", "skip", "resume", "older"
}

// NetJobAsk is the question about a file that is in the way.
type NetJobAsk struct {
	Source    string `json:"source"`
	Target    string `json:"target"`
	SrcSize   int64  `json:"srcSize"`
	SrcTime   int64  `json:"srcTime"`
	DstSize   int64  `json:"dstSize"`
	DstTime   int64  `json:"dstTime"`
	CanResume bool   `json:"canResume"`
}

// NetJobAnswer answers a NetJobAsk: overwrite, skip, resume, overwrite_all, skip_all, resume_all, older_all, rename
// (with NewName) or cancel.
type NetJobAnswer struct {
	Choice  string `json:"choice"`
	NewName string `json:"newName"`
}

// NetJobProgress is the state of a transfer.
type NetJobProgress struct {
	ID        int        `json:"id"`
	Kind      string     `json:"kind"`
	State     string     `json:"state"` // running, asking, done, failed, cancelled
	Files     int        `json:"files"`
	FilesDone int        `json:"filesDone"`
	Bytes     int64      `json:"bytes"`
	BytesDone int64      `json:"bytesDone"`
	Current   string     `json:"current"`
	Errors    []string   `json:"errors"`
	Ask       *NetJobAsk `json:"ask,omitempty"`
}

type netXfer struct {
	n      *Net
	ctx    context.Context
	cancel context.CancelFunc
	req    NetJobRequest
	answer chan NetJobAnswer

	mu       sync.Mutex
	p        NetJobProgress
	policy   string // the "… all" choice in force
	lastEmit time.Time
}

// netItem is one thing to copy.
type netItem struct {
	src, dst string
	dir      bool
	size     int64
	mtime    int64
	top      int // index of the source it belongs to
}

// StartJob starts a transfer and returns its number.
func (n *Net) StartJob(req NetJobRequest) (int, error) {
	if n.app != nil && n.app.mock != nil {
		return 0, errors.New("This is the demo: its files are not real, so they can't be copied or moved.")
	}
	if req.Kind != "copy" && req.Kind != "move" {
		return 0, fmt.Errorf("%q is not a transfer (copy or move).", req.Kind)
	}
	if len(req.Sources) == 0 {
		return 0, errors.New("Nothing is selected.")
	}
	if strings.TrimSpace(req.Target) == "" {
		return 0, errors.New("There is no target folder.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.mu.Lock()
	n.xferID++
	x := &netXfer{n: n, ctx: ctx, cancel: cancel, req: req, answer: make(chan NetJobAnswer, 1), policy: req.Overwrite}
	x.p = NetJobProgress{ID: n.xferID, Kind: req.Kind, State: "running", Errors: []string{}}
	n.xfers[x.p.ID] = x
	n.mu.Unlock()
	go x.run()
	return x.p.ID, nil
}

// CancelJob stops a transfer; what is done stays done, a partial file stays for a later resume.
func (n *Net) CancelJob(id int) bool {
	n.mu.Lock()
	x := n.xfers[id]
	n.mu.Unlock()
	if x == nil {
		return false
	}
	x.cancel()
	return true
}

// AnswerJob answers the question a transfer asks.
func (n *Net) AnswerJob(id int, a NetJobAnswer) error {
	n.mu.Lock()
	x := n.xfers[id]
	n.mu.Unlock()
	if x == nil {
		return errors.New("This transfer is over.")
	}
	select {
	case x.answer <- a:
	default:
	}
	return nil
}

// JobState is the state of a running transfer (control mode, tests).
func (n *Net) JobState(id int) (NetJobProgress, bool) {
	n.mu.Lock()
	x := n.xfers[id]
	done, ok := n.xferDone[id]
	n.mu.Unlock()
	if x == nil {
		return done, ok
	}
	return x.snapshot(), true
}

func (x *netXfer) snapshot() NetJobProgress {
	x.mu.Lock()
	defer x.mu.Unlock()
	p := x.p
	p.Errors = append([]string{}, x.p.Errors...)
	return p
}

func (x *netXfer) emit(event string, force bool) {
	x.mu.Lock()
	if !force && time.Since(x.lastEmit) < 150*time.Millisecond {
		x.mu.Unlock()
		return
	}
	x.lastEmit = time.Now()
	x.mu.Unlock()
	if x.n.app != nil {
		x.n.app.emit(event, x.snapshot())
	}
}

func (x *netXfer) fail(name string, err error) {
	x.mu.Lock()
	x.p.Errors = append(x.p.Errors, fmt.Sprintf("%s: %s", name, netErrText(err)))
	x.mu.Unlock()
}

// netErrText is an error without the engines' prefixes.
func netErrText(err error) string {
	s := err.Error()
	for _, p := range []string{"vfs: ", "ftp: ", "sftp: ", "webdav: ", "remotefs: "} {
		s = strings.ReplaceAll(s, p, "")
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "it is not there"
	case errors.Is(err, fs.ErrPermission):
		return "no permission"
	case errors.Is(err, ftp.ErrResumeASCII):
		return "resuming is not possible in ASCII mode; switch the transfer mode to binary (Ctrl+Shift+M)"
	}
	return s
}

func netBase(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// isLocal says whether p is on the local disk (not a server, not inside an archive).
func netIsLocalPath(r *vfs.Router, p string) bool {
	return !strings.Contains(p, "://") && r.Kind(p) == vfs.KindLocal
}

func (x *netXfer) run() {
	n := x.n
	r := n.router()
	defer func() {
		x.cancel()
		x.mu.Lock()
		switch {
		case x.p.State == "cancelled":
		case len(x.p.Errors) > 0:
			x.p.State = "failed"
		default:
			x.p.State = "done"
		}
		x.p.Ask = nil
		x.p.Current = ""
		x.mu.Unlock()
		x.emit("net-job-done", true)
		final := x.snapshot()
		n.mu.Lock()
		delete(n.xfers, final.ID)
		if n.xferDone == nil {
			n.xferDone = map[int]NetJobProgress{}
		}
		n.xferDone[final.ID] = final
		delete(n.xferDone, final.ID-50) // (keep the last few for control mode)
		n.mu.Unlock()
	}()
	target := r.Clean(x.req.Target)
	sameSession := func(a, b string) bool {
		ka, kb := netKeyFor(a), netKeyFor(b)
		if ka == "" && kb == "" {
			return netIsLocalPath(r, a) && netIsLocalPath(r, b)
		}
		return ka != "" && ka == kb
	}
	// plan
	var items []netItem
	moved := make([]bool, len(x.req.Sources))
	for i, src := range x.req.Sources {
		if x.ctx.Err() != nil {
			break
		}
		name := netBase(src)
		if x.req.Rename != "" && len(x.req.Sources) == 1 {
			name = x.req.Rename
		}
		dst := r.Join(target, name)
		if r.Clean(src) == r.Clean(dst) {
			x.fail(name, errors.New("it can't be copied onto itself"))
			moved[i] = true // (nothing to delete either)
			continue
		}
		e, err := r.Stat(src)
		if err != nil {
			x.fail(name, err)
			moved[i] = true
			continue
		}
		isDir := e.Flags&vfs.FlagDir != 0
		if isDir && strings.HasPrefix(r.Clean(dst)+"/", strings.TrimSuffix(r.Clean(src), "/")+"/") {
			x.fail(name, errors.New("a folder can't be copied into itself"))
			moved[i] = true
			continue
		}
		if x.req.Kind == "move" && sameSession(src, dst) {
			if _, err := r.Stat(dst); errors.Is(err, fs.ErrNotExist) {
				if err := r.Rename(x.ctx, src, dst); err == nil {
					moved[i] = true
					x.mu.Lock()
					x.p.Files++
					x.p.FilesDone++
					x.mu.Unlock()
					continue
				}
			}
		}
		if !isDir {
			items = append(items, netItem{src: src, dst: dst, size: e.Size, mtime: e.MTime, top: i})
			continue
		}
		items = append(items, netItem{src: src, dst: dst, dir: true, top: i})
		if err := x.walk(r, src, dst, i, &items, 0); err != nil {
			x.fail(name, err)
		}
	}
	x.mu.Lock()
	for _, it := range items {
		if !it.dir {
			x.p.Files++
			x.p.Bytes += it.size
		}
	}
	x.mu.Unlock()
	x.emit("net-job", true)
	// copy
	failed := make([]bool, len(x.req.Sources))
	var blocked []string // target folders that could not be made: what belongs inside them is not tried
	for _, it := range items {
		if x.ctx.Err() != nil {
			break
		}
		if netUnder(it.dst, blocked) {
			failed[it.top] = true
			continue
		}
		if it.dir {
			if !x.makeDir(r, it.dst) {
				blocked = append(blocked, it.dst)
				failed[it.top] = true
			}
			continue
		}
		if !x.copyFile(r, it) {
			failed[it.top] = true
		}
	}
	if x.ctx.Err() != nil {
		x.mu.Lock()
		x.p.State = "cancelled"
		x.mu.Unlock()
		return
	}
	// a move deletes the sources that were copied completely
	if x.req.Kind == "move" {
		for i, src := range x.req.Sources {
			if moved[i] || failed[i] {
				continue
			}
			if err := r.RemoveAll(x.ctx, src); err != nil {
				x.fail(netBase(src), fmt.Errorf("copied, but the original can't be deleted: %s", netErrText(err)))
			}
		}
	}
}

// makeDir makes the target folder dst, or merges into it when a folder of that name is already there (F-460: a
// folder copied onto a same-named folder is merged, never replaced). A file of that name in the way is one clear
// error, and it reports false so the folder's contents are not tried one by one.
func (x *netXfer) makeDir(r *vfs.Router, dst string) bool {
	err := r.Mkdir(x.ctx, dst)
	if err == nil {
		return true
	}
	if errors.Is(err, fs.ErrExist) {
		// the file systems answer "exists" for a file too, so look at what is there
		if e, serr := r.Stat(dst); serr != nil || e.Flags&vfs.FlagDir != 0 {
			return true // a folder (or it can't be told): merge, and let the files report their own problems
		}
		x.fail(netBase(dst), errors.New("a file of that name is in the way of the folder"))
		return false
	}
	x.fail(netBase(dst), err)
	return false
}

// netUnder says whether p lies inside one of the folders in dirs (both written the same way, as the router joins them).
func netUnder(p string, dirs []string) bool {
	for _, d := range dirs {
		d = strings.TrimRight(d, `/\`)
		if len(p) > len(d) && strings.HasPrefix(p, d) && (p[len(d)] == '/' || p[len(d)] == '\\') {
			return true
		}
	}
	return false
}

// walk adds the contents of the folder src (copied to dst) to items.
func (x *netXfer) walk(r *vfs.Router, src, dst string, top int, items *[]netItem, depth int) error {
	if depth > 64 {
		return errors.New("the folders are nested too deeply")
	}
	l, err := r.List(x.ctx, src)
	if err != nil {
		return err
	}
	dstLocal := netIsLocalPath(r, dst)
	for _, e := range l.Entries {
		// A listing comes from the server and is not trusted: a name such as "../x" or "a/../../x" (which an SFTP
		// server can send) would be joined into a target path outside the folder being copied, and on the local disk
		// the shared rule of the tree downloads also refuses names this system cannot store. Such a name fails the
		// copy, so it never ends looking complete when it is not.
		if !netPlainName(e.Name) {
			return fmt.Errorf("the server lists an item named %q, which is not a plain name", e.Name)
		}
		if dstLocal {
			if _, err := remotefs.LocalRel(e.Name); err != nil {
				return fmt.Errorf("the server lists an item named %q, which can't be stored safely on this disk", e.Name)
			}
		}
		s, d := r.Join(src, e.Name), r.Join(dst, e.Name)
		if e.Flags&vfs.FlagDir != 0 {
			if e.Flags&vfs.FlagLink != 0 && !strings.Contains(src, "://") {
				continue // a local link to a folder is not followed (loops)
			}
			*items = append(*items, netItem{src: s, dst: d, dir: true, top: top})
			if err := x.walk(r, s, d, top, items, depth+1); err != nil {
				return err
			}
			continue
		}
		*items = append(*items, netItem{src: s, dst: d, size: e.Size, mtime: e.MTime, top: top})
	}
	return nil
}

// netPlainName says whether name is one item of a folder: not empty, not "." or "..", and without a slash.
func netPlainName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsRune(name, '/')
}

// copyFile copies one file, asking when the target is there. It reports whether the file is done (or skipped by
// choice).
func (x *netXfer) copyFile(r *vfs.Router, it netItem) bool {
	name := netBase(it.src)
	x.mu.Lock()
	x.p.Current = it.src
	base := x.p.BytesDone
	x.mu.Unlock()
	x.emit("net-job", true)
	finish := func() {
		x.mu.Lock()
		x.p.FilesDone++
		x.p.BytesDone = base + it.size
		x.mu.Unlock()
	}
	srcLocal, dstLocal := netIsLocalPath(r, it.src), netIsLocalPath(r, it.dst)
	resume := false
	for {
		de, err := r.Stat(it.dst)
		if err != nil {
			break
		}
		if de.Flags&vfs.FlagDir != 0 {
			x.fail(name, errors.New("a folder of that name is in the target folder"))
			return false
		}
		canResume := de.Size > 0 && de.Size < it.size && (!srcLocal || (!dstLocal && netSchemes[netSchemeOf(it.dst)] != "webdav"))
		choice := x.decide(it, de, canResume)
		switch choice {
		case "overwrite":
		case "resume":
			resume = canResume
		case "skip":
			finish()
			return true
		case "cancel":
			x.cancel()
			return false
		default:
			if strings.HasPrefix(choice, "rename:") {
				it.dst = r.Join(r.Parent(it.dst), strings.TrimPrefix(choice, "rename:"))
				continue
			}
		}
		break
	}
	opt := vfs.TransferOptions{Resume: resume, PreserveTime: true, Progress: func(done, total int64) {
		x.mu.Lock()
		x.p.BytesDone = base + done
		x.mu.Unlock()
		x.emit("net-job", false)
	}}
	var err error
	switch {
	case !srcLocal && dstLocal:
		_, err = r.Download(x.ctx, it.src, it.dst, opt)
	case srcLocal && !dstLocal:
		_, err = r.Upload(x.ctx, it.src, it.dst, opt)
	default:
		sf, sp := r.Resolve(it.src)
		df, dp := r.Resolve(it.dst)
		_, err = vfs.CopyFile(x.ctx, sf, sp, df, dp, r.Local, opt)
	}
	if err != nil {
		if x.ctx.Err() == nil {
			x.fail(name, err)
		}
		return false
	}
	finish()
	return true
}

// decide answers "the target is there" from the policy in force or by asking.
func (x *netXfer) decide(it netItem, de vfs.Entry, canResume bool) string {
	x.mu.Lock()
	pol := x.policy
	x.mu.Unlock()
	switch pol {
	case "overwrite", "skip":
		return pol
	case "resume":
		if canResume {
			return "resume"
		}
		if de.Size == it.size {
			return "skip" // already complete
		}
		return "overwrite"
	case "older":
		if de.MTime < it.mtime {
			return "overwrite"
		}
		return "skip"
	}
	x.mu.Lock()
	x.p.State = "asking"
	x.p.Ask = &NetJobAsk{Source: it.src, Target: it.dst, SrcSize: it.size, SrcTime: it.mtime, DstSize: de.Size, DstTime: de.MTime, CanResume: canResume}
	x.mu.Unlock()
	x.emit("net-job-ask", true)
	var a NetJobAnswer
	select {
	case a = <-x.answer:
	case <-x.ctx.Done():
		a = NetJobAnswer{Choice: "cancel"}
	}
	x.mu.Lock()
	x.p.State, x.p.Ask = "running", nil
	x.mu.Unlock()
	switch a.Choice {
	case "overwrite_all", "skip_all", "resume_all", "older_all":
		p := strings.TrimSuffix(a.Choice, "_all")
		x.mu.Lock()
		x.policy = p
		x.mu.Unlock()
		return x.decide(it, de, canResume)
	case "rename":
		if n := strings.TrimSpace(a.NewName); n != "" && !strings.ContainsAny(n, `/\`) {
			return "rename:" + n
		}
		return "skip"
	case "overwrite", "skip", "cancel":
		return a.Choice
	case "resume":
		if canResume {
			return "resume"
		}
		return "overwrite"
	}
	return "cancel"
}

// OpenCopy downloads a server file into a new temporary folder and returns the local copy, so Enter on a remote file
// can open it with its app (TC downloads to a temp folder first). The copy is removed when the app ends.
func (n *Net) OpenCopy(p string) (string, error) {
	if !netIsRemote(p) {
		return "", errors.New("This file is not on a server.")
	}
	n.mu.Lock()
	if n.tempDir == "" {
		d, err := os.MkdirTemp("", "mormicommander-net-")
		if err != nil {
			n.mu.Unlock()
			return "", err
		}
		n.tempDir = d
	}
	root := n.tempDir
	n.mu.Unlock()
	dir, err := os.MkdirTemp(root, "f")
	if err != nil {
		return "", err
	}
	local := filepath.Join(dir, netBase(p))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := n.router().Download(ctx, p, local, vfs.TransferOptions{PreserveTime: true}); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("Can't download %s: %s", netBase(p), netErrText(err))
	}
	return local, nil
}
