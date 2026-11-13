// This file is the compare half of Synchronize Directories (FEATURES.md
// F-546 to F-549 and F-552). Compare walks both trees side by side, pairs up
// names (case-insensitively where the platform's file systems are), and
// sorts every pair into a Category using the options from the dialog:
// subfolders on or off, ignore empty folders, compare by content, ignore
// date, the two-second tolerance needed for FAT drives, the "ignore exact
// hour differences" option for daylight-saving and time-zone shifts,
// asymmetric mode, only-selected and the file-type filter.
//
// Names are paired after Unicode NFC normalisation, so a file that macOS
// stored in decomposed form ("e" + combining accent) pairs with the composed
// spelling a Linux or SMB server keeps; otherwise one sync would create a
// second, identical-looking file on the server. Named pipes, sockets and
// devices are listed but never opened, compared by content or copied: opening
// a pipe without a writer blocks forever. A cancelled compare returns the
// rows found so far together with the context error, so the list can show
// partial results as Total Commander does.
//
// The rules follow Total Commander. Without "by content" two files are equal
// when their dates match within the tolerance and their sizes match; a
// different date makes the newer side win, and the same date with a
// different size is "unequal". With "by content" files of the same size are
// read and compared byte by byte, and only a real content difference counts;
// the date then just picks the direction. Symbolic links are not followed
// unless asked: a link is compared by its target text, so a link to "/"
// never makes the engine read or copy the whole disk. When links are
// followed, folder loops are detected and skipped.
package syncdirs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Options are the settings of the Synchronize dialog. The zero value
// compares only the two top folders with exact dates; DefaultOptions gives
// the usual Total Commander defaults.
type Options struct {
	// Subdirs also compares everything in subfolders.
	Subdirs bool `json:"subdirs"`
	// IgnoreEmptyDirs hides folders that contain no files at any depth.
	IgnoreEmptyDirs bool `json:"ignoreEmptyDirs"`
	// ByContent compares the bytes of files that have the same size.
	ByContent bool `json:"byContent"`
	// IgnoreDate ignores modification times: without ByContent only sizes
	// are compared, and nothing is ever "newer".
	IgnoreDate bool `json:"ignoreDate"`
	// ToleranceSeconds is how far two times may differ and still count as
	// equal. 2 covers FAT and exFAT, which store times in two-second steps.
	ToleranceSeconds int `json:"toleranceSeconds"`
	// IgnoreHours ignores differences of exactly 1..IgnoreHours hours
	// (within ToleranceSeconds), which appear after a daylight-saving
	// change on FAT drives or when copying between time zones.
	IgnoreHours int `json:"ignoreHours"`
	// Asymmetric treats the left side as the master: by default every
	// difference is copied to the right and right-only files are marked for
	// deletion, so the right side ends up as a copy of the left.
	Asymmetric bool `json:"asymmetric"`
	// FollowSymlinks compares what links point to instead of the links
	// themselves. The engine still never writes or deletes inside a
	// followed folder link.
	FollowSymlinks bool `json:"followSymlinks"`
	// Case chooses case-sensitive or case-insensitive name matching.
	Case CaseMode `json:"case"`
	// Filter is a Total Commander filter string, see CompileFilter.
	Filter string `json:"filter"`
	// Templates resolves ">name" words in Filter to the "Search for" string
	// of a saved Find Files template. Nil means no templates are available.
	Templates func(name string) (string, bool) `json:"-"`
	// OnlySelected limits the compare to the top-level names in Selected
	// (the names selected in the panels). With an empty list everything is
	// compared.
	OnlySelected bool     `json:"onlySelected"`
	Selected     []string `json:"-"`
}

// DefaultOptions returns the options Total Commander starts with:
// subfolders on and the two-second tolerance.
func DefaultOptions() Options {
	return Options{Subdirs: true, ToleranceSeconds: 2}
}

// timeOrder returns 1 when l is newer, -1 when r is newer and 0 when the two
// times count as equal under the options.
func (o *Options) timeOrder(l, r time.Time) int {
	if o.IgnoreDate {
		return 0
	}
	d := l.Sub(r)
	ad := absDur(d)
	tol := time.Duration(o.ToleranceSeconds) * time.Second
	if ad <= tol {
		return 0
	}
	for h := 1; h <= o.IgnoreHours; h++ {
		if absDur(ad-time.Duration(h)*time.Hour) <= tol {
			return 0
		}
	}
	if d > 0 {
		return 1
	}
	return -1
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// CompareProgress is reported while comparing.
type CompareProgress struct {
	Dirs         int    `json:"dirs"`         // folders listed so far
	Files        int    `json:"files"`        // file pairs classified so far
	ContentBytes int64  `json:"contentBytes"` // bytes read for content compares
	Current      string `json:"current"`      // path being worked on
}

// PathError records a problem with one path that did not stop the compare,
// such as an unreadable subfolder.
type PathError struct {
	Side Side   `json:"side"`
	Path string `json:"path"`
	Err  error  `json:"-"`
}

func (e PathError) Error() string { return fmt.Sprintf("%s: %s: %v", e.Side, e.Path, e.Err) }

// Unwrap returns the underlying error.
func (e PathError) Unwrap() error { return e.Err }

// MarshalText lets the UI see the message.
func (e PathError) MarshalText() ([]byte, error) { return []byte(e.Error()), nil }

// Result is the outcome of Compare: the rows in tree order (each folder
// right before its contents, names sorted) plus any path errors.
type Result struct {
	Options Options     `json:"options"`
	Items   []*Item     `json:"items"`
	Errors  []PathError `json:"errors,omitempty"`

	left, right FS
	index       map[string]*Item
}

// maxFollowDepth limits nesting when links are followed on a file system
// that cannot resolve real paths, as a last guard against loops.
const maxFollowDepth = 64

// Compare compares the trees at the roots of left and right. progress may
// be nil. Cancelling ctx stops the walk and returns the rows compared so far
// (with their default actions) together with ctx.Err(); spec 9.3.5 wants
// partial results to stay visible after Esc. Other errors return a nil
// Result.
func Compare(ctx context.Context, left, right FS, opts Options, progress func(CompareProgress)) (*Result, error) {
	filt, err := CompileFilter(opts.Filter, opts.Templates)
	if err != nil {
		return nil, err
	}
	w := &walker{
		ctx:      ctx,
		l:        left,
		r:        right,
		o:        opts,
		filt:     filt,
		ci:       opts.Case.insensitive(),
		progress: progress,
		res:      &Result{Options: opts, left: left, right: right},
		stackL:   map[string]bool{},
		stackR:   map[string]bool{},
	}
	for _, s := range []struct {
		fs   FS
		side Side
	}{{left, LeftSide}, {right, RightSide}} {
		fi, err := s.fs.Stat(".")
		if err != nil {
			return nil, PathError{Side: s.side, Path: ".", Err: err}
		}
		if !fi.IsDir() {
			return nil, PathError{Side: s.side, Path: ".", Err: errors.New("not a folder")}
		}
	}
	w.push(left, w.stackL, ".")
	w.push(right, w.stackR, ".")
	werr := w.dir(node{".", true, false}, node{".", true, false}, 0, nil, false)
	if werr != nil && ctx.Err() == nil {
		return nil, werr
	}
	if opts.IgnoreEmptyDirs {
		w.res.pruneEmptyDirs()
	}
	for _, it := range w.res.Items {
		it.setDefault(opts.Asymmetric)
	}
	w.res.reindex()
	w.report("", true)
	return w.res, werr
}

type node struct {
	path   string // path on this side (existing or would-be)
	exists bool
	via    bool // reached through a followed folder link
}

type walker struct {
	ctx            context.Context
	l, r           FS
	o              Options
	filt           *Filter
	ci             bool
	res            *Result
	prog           CompareProgress
	progress       func(CompareProgress)
	lastReport     time.Time
	stackL, stackR map[string]bool
}

func (w *walker) report(cur string, force bool) {
	if w.progress == nil {
		return
	}
	w.prog.Current = cur
	if now := time.Now(); force || now.Sub(w.lastReport) >= 50*time.Millisecond {
		w.lastReport = now
		w.progress(w.prog)
	}
}

func joinPath(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return dir + "/" + name
}

// fold returns the pairing key of a name: NFC always, lower case when
// matching ignores case.
func (w *walker) fold(s string) string {
	s = norm.NFC.String(s)
	if w.ci {
		return strings.ToLower(s)
	}
	return s
}

// push records a folder's real path on the current walk stack and reports
// false when it is already there (a symlink loop).
func (w *walker) push(f FS, stack map[string]bool, p string) (string, bool) {
	if !w.o.FollowSymlinks {
		return "", true
	}
	rp, ok := f.(RealPather)
	if !ok {
		return "", true
	}
	real, err := rp.RealPath(p)
	if err != nil {
		return "", true
	}
	if stack[real] {
		return "", false
	}
	stack[real] = true
	return real, true
}

// specialMode holds the type bits of entries that are neither files, folders
// nor links: named pipes, sockets and devices.
const specialMode = fs.ModeNamedPipe | fs.ModeSocket | fs.ModeDevice | fs.ModeCharDevice

func entryFromInfo(e *Entry, fi fs.FileInfo) {
	e.ModTime = fi.ModTime()
	switch {
	case fi.IsDir():
		e.Kind = KindDir
		e.Size = 0
	case fi.Mode()&specialMode != 0:
		e.Kind = KindSpecial
		e.Size = 0
	default:
		e.Kind = KindFile
		e.Size = fi.Size()
	}
}

// statEntry builds an Entry for p on f, following a final link when the
// options say so.
func (w *walker) statEntry(f FS, p, name string, via bool) (*Entry, error) {
	fi, err := f.Lstat(p)
	if err != nil {
		return nil, err
	}
	e := &Entry{Name: name, Mode: fi.Mode(), ViaLink: via}
	entryFromInfo(e, fi)
	if fi.Mode()&fs.ModeSymlink != 0 {
		e.IsLink = true
		e.Kind = KindLink
		e.Size = 0
		if t, err := f.Readlink(p); err == nil {
			e.LinkTarget = t
		}
		if w.o.FollowSymlinks {
			if st, err := f.Stat(p); err == nil {
				entryFromInfo(e, st)
				e.Mode = st.Mode()
				e.Followed = true
			}
		}
	}
	return e, nil
}

func (w *walker) list(f FS, n node) ([]*Entry, error) {
	if !n.exists {
		return nil, nil
	}
	des, err := f.ReadDir(n.path)
	if err != nil {
		return nil, err
	}
	out := make([]*Entry, 0, len(des))
	for _, de := range des {
		e, err := w.statEntry(f, joinPath(n.path, de.Name()), de.Name(), n.via)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// keep applies subfolder, selection and filter rules to one listing and
// returns the kept entries plus, for folders, whether each matched an
// include folder pattern.
func (w *walker) keep(es []*Entry, dir string, depth int, sel map[string]bool, parentIncl bool) ([]*Entry, map[*Entry]bool) {
	out := es[:0]
	incl := map[*Entry]bool{}
	for _, e := range es {
		if e.Kind == KindDir && !w.o.Subdirs {
			continue
		}
		if depth == 0 && sel != nil && !sel[w.fold(e.Name)] {
			continue
		}
		rel := joinPath(dir, e.Name)
		if e.Kind == KindDir {
			enter, in := w.filt.Dir(rel, e.Name, parentIncl)
			if !enter {
				continue
			}
			incl[e] = in
		} else if !w.filt.File(rel, e.Name, parentIncl) {
			continue
		}
		out = append(out, e)
	}
	return out, incl
}

type pair struct{ l, r *Entry }

// pairUp matches names exactly first and then pairs the leftovers by folded
// name (NFC, and lower case when matching ignores case). Two names that
// differ only in case or normalisation on one side therefore pair with their
// exact twins and do not swallow each other.
func (w *walker) pairUp(ls, rs []*Entry) []pair {
	rByName := make(map[string]*Entry, len(rs))
	for _, e := range rs {
		rByName[e.Name] = e
	}
	used := map[*Entry]bool{}
	var out []pair
	var restL []*Entry
	for _, e := range ls {
		if r := rByName[e.Name]; r != nil {
			out = append(out, pair{e, r})
			used[r] = true
		} else {
			restL = append(restL, e)
		}
	}
	byFold := map[string][]*Entry{}
	for _, e := range rs {
		if !used[e] {
			k := w.fold(e.Name)
			byFold[k] = append(byFold[k], e)
		}
	}
	for _, e := range restL {
		k := w.fold(e.Name)
		if c := byFold[k]; len(c) > 0 {
			out = append(out, pair{e, c[0]})
			used[c[0]] = true
			byFold[k] = c[1:]
		} else {
			out = append(out, pair{e, nil})
		}
	}
	for _, e := range rs {
		if !used[e] {
			out = append(out, pair{nil, e})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := pairName(out[i]), pairName(out[j])
		fa, fb := strings.ToLower(a), strings.ToLower(b)
		if fa != fb {
			return fa < fb
		}
		return a < b
	})
	return out
}

func pairName(p pair) string {
	if p.l != nil {
		return p.l.Name
	}
	return p.r.Name
}

func (w *walker) dir(ln, rn node, depth int, parent *Item, parentIncl bool) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	w.prog.Dirs++
	w.report(ln.path, false)
	if depth > maxFollowDepth && (ln.via || rn.via) {
		if parent != nil {
			parent.Note = "folder nesting too deep (symlink loop?)"
		}
		return nil
	}
	ls, errL := w.list(w.l, ln)
	rs, errR := w.list(w.r, rn)
	if errL != nil || errR != nil {
		if depth == 0 {
			if errL != nil {
				return PathError{Side: LeftSide, Path: ".", Err: errL}
			}
			return PathError{Side: RightSide, Path: ".", Err: errR}
		}
		// Never compare a folder when one side could not be listed: the
		// missing listing would turn every name into a single and propose
		// copies or even deletions that are not real differences.
		if errL != nil {
			w.res.Errors = append(w.res.Errors, PathError{Side: LeftSide, Path: ln.path, Err: errL})
		}
		if errR != nil {
			w.res.Errors = append(w.res.Errors, PathError{Side: RightSide, Path: rn.path, Err: errR})
		}
		if parent != nil {
			parent.Note = "could not list folder contents"
		}
		return nil
	}
	var sel map[string]bool
	if depth == 0 && w.o.OnlySelected && len(w.o.Selected) > 0 {
		sel = map[string]bool{}
		for _, s := range w.o.Selected {
			sel[w.fold(s)] = true
		}
	}
	ls, inclL := w.keep(ls, ln.path, depth, sel, parentIncl)
	rs, inclR := w.keep(rs, rn.path, depth, sel, parentIncl)

	for _, p := range w.pairUp(ls, rs) {
		lname, rname := pairName(p), pairName(p)
		if p.l != nil {
			lname = p.l.Name
		}
		if p.r != nil {
			rname = p.r.Name
		}
		it := &Item{
			LeftPath:  joinPath(ln.path, lname),
			RightPath: joinPath(rn.path, rname),
			Left:      p.l,
			Right:     p.r,
			Depth:     depth,
			lvia:      ln.via,
			rvia:      rn.via,
		}
		it.Path = it.LeftPath
		incl := inclL[p.l] || inclR[p.r]
		switch {
		case p.l != nil && p.r != nil:
			ld, rd := p.l.Kind == KindDir, p.r.Kind == KindDir
			switch {
			case ld && rd:
				it.IsDir = true
				it.Category = Equal
				w.res.Items = append(w.res.Items, it)
				if err := w.descend(it, node{it.LeftPath, true, ln.via || p.l.Followed}, node{it.RightPath, true, rn.via || p.r.Followed}, depth, incl); err != nil {
					return err
				}
			case p.l.Kind != p.r.Kind:
				it.Category = Unequal
				it.TypeMismatch = true
				w.res.Items = append(w.res.Items, it)
			default:
				if err := w.classify(it); err != nil {
					return err
				}
				w.res.Items = append(w.res.Items, it)
			}
		case p.l != nil:
			it.Category = LeftOnly
			it.IsDir = p.l.Kind == KindDir
			if p.l.Kind == KindSpecial {
				it.Note = noteSpecial
			}
			w.res.Items = append(w.res.Items, it)
			if it.IsDir {
				if err := w.descend(it, node{it.LeftPath, true, ln.via || p.l.Followed}, node{it.RightPath, false, rn.via}, depth, incl); err != nil {
					return err
				}
			} else {
				w.prog.Files++
			}
		default:
			it.Category = RightOnly
			it.IsDir = p.r.Kind == KindDir
			if p.r.Kind == KindSpecial {
				it.Note = noteSpecial
			}
			w.res.Items = append(w.res.Items, it)
			if it.IsDir {
				if err := w.descend(it, node{it.LeftPath, false, ln.via}, node{it.RightPath, true, rn.via || p.r.Followed}, depth, incl); err != nil {
					return err
				}
			} else {
				w.prog.Files++
			}
		}
	}
	return nil
}

// descend enters a folder pair, guarding against symlink loops.
func (w *walker) descend(it *Item, ln, rn node, depth int, incl bool) error {
	var realL, realR string
	okL, okR := true, true
	if ln.exists {
		realL, okL = w.push(w.l, w.stackL, ln.path)
	}
	if rn.exists {
		realR, okR = w.push(w.r, w.stackR, rn.path)
	}
	if !okL || !okR {
		if realL != "" {
			delete(w.stackL, realL)
		}
		if realR != "" {
			delete(w.stackR, realR)
		}
		it.Note = "symlink loop: folder not entered"
		return nil
	}
	err := w.dir(ln, rn, depth+1, it, incl)
	if realL != "" {
		delete(w.stackL, realL)
	}
	if realR != "" {
		delete(w.stackR, realR)
	}
	return err
}

// classify sets the category of a pair of two non-folder entries of the
// same kind.
func (w *walker) classify(it *Item) error {
	w.prog.Files++
	w.report(it.Path, false)
	l, r := it.Left, it.Right
	tc := w.o.timeOrder(l.ModTime, r.ModTime)
	byTime := func() Category {
		switch tc {
		case 1:
			return LeftNewer
		case -1:
			return RightNewer
		}
		return Unequal
	}
	if l.Kind == KindSpecial {
		// Pipes, sockets and devices have no contents to compare or copy.
		it.Category = Unequal
		it.Note = noteSpecial
		return nil
	}
	if l.Kind == KindLink {
		if l.LinkTarget == r.LinkTarget {
			it.Category = Equal
		} else {
			it.Category = byTime()
		}
		return nil
	}
	if w.o.ByContent {
		same := false
		if l.Size == r.Size {
			var err error
			same, err = w.sameContent(it)
			if err != nil {
				if ctxErr := w.ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				it.Category = Unequal
				it.Note = "content compare failed: " + err.Error()
				return nil
			}
			it.ContentCompared = true
		}
		if same {
			it.Category = Equal
		} else {
			it.Category = byTime()
		}
		return nil
	}
	if tc == 0 {
		if l.Size == r.Size {
			it.Category = Equal
		} else {
			it.Category = Unequal
		}
		return nil
	}
	it.Category = byTime()
	return nil
}

const chunkSize = 256 << 10

// noteSpecial explains rows with a pipe, socket or device.
const noteSpecial = "pipe, socket or device: not compared or copied"

func (w *walker) sameContent(it *Item) (bool, error) {
	return compareFiles(w.ctx, w.l, it.LeftPath, w.r, it.RightPath, func(n int64) {
		w.prog.ContentBytes += n
		w.report(it.Path, false)
	})
}

// compareFiles reads two files in parallel chunks and reports whether their
// bytes are identical. It stops at the first difference.
func compareFiles(ctx context.Context, lf FS, lp string, rf FS, rp string, onBytes func(int64)) (bool, error) {
	a, err := lf.Open(lp)
	if err != nil {
		return false, err
	}
	defer a.Close()
	b, err := rf.Open(rp)
	if err != nil {
		return false, err
	}
	defer b.Close()
	ba := make([]byte, chunkSize)
	bb := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		na, ea := io.ReadFull(a, ba)
		nb, eb := io.ReadFull(b, bb)
		if onBytes != nil {
			onBytes(int64(na + nb))
		}
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		aEnd := ea == io.EOF || ea == io.ErrUnexpectedEOF
		bEnd := eb == io.EOF || eb == io.ErrUnexpectedEOF
		if ea != nil && !aEnd {
			return false, ea
		}
		if eb != nil && !bEnd {
			return false, eb
		}
		if aEnd || bEnd {
			return aEnd == bEnd, nil
		}
	}
}

// pruneEmptyDirs drops folder rows that have no file rows below them.
func (r *Result) pruneEmptyDirs() {
	hasFile := map[string]bool{}
	for _, it := range r.Items {
		if it.IsDir {
			continue
		}
		p := it.Path
		for {
			i := strings.LastIndexByte(p, '/')
			if i < 0 {
				break
			}
			p = p[:i]
			hasFile[p] = true
		}
	}
	out := r.Items[:0]
	for _, it := range r.Items {
		if it.IsDir && !hasFile[it.Path] {
			continue
		}
		out = append(out, it)
	}
	r.Items = out
}

func (r *Result) reindex() {
	r.index = make(map[string]*Item, len(r.Items))
	for _, it := range r.Items {
		r.index[it.Path] = it
	}
}

// Find returns the row with the given Path, or nil.
func (r *Result) Find(path string) *Item {
	return r.index[path]
}

// Recompare re-reads one file pair from disk and classifies it again, for
// example after the user edited one side in the compare tool (F-553). The
// row's action is reset to the new default.
func (r *Result) Recompare(ctx context.Context, path string) error {
	it := r.index[path]
	if it == nil {
		return fmt.Errorf("syncdirs: no item %q", path)
	}
	if it.IsDir {
		return fmt.Errorf("syncdirs: %q is a folder; compare again instead", path)
	}
	w := &walker{ctx: ctx, l: r.left, r: r.right, o: r.Options, res: r}
	var err error
	if it.Left, err = w.restat(r.left, it.LeftPath, it.lvia); err != nil {
		return err
	}
	if it.Right, err = w.restat(r.right, it.RightPath, it.rvia); err != nil {
		return err
	}
	it.TypeMismatch, it.ContentCompared, it.Note = false, false, ""
	switch {
	case it.Left == nil && it.Right == nil:
		r.remove(it)
		return nil
	case it.Right == nil:
		it.Category = LeftOnly
	case it.Left == nil:
		it.Category = RightOnly
	case it.Left.Kind != it.Right.Kind:
		it.Category = Unequal
		it.TypeMismatch = true
	default:
		if err := w.classify(it); err != nil {
			return err
		}
	}
	it.IsDir = (it.Left == nil || it.Left.Kind == KindDir) && (it.Right == nil || it.Right.Kind == KindDir)
	it.setDefault(r.Options.Asymmetric)
	return nil
}

func (w *walker) restat(f FS, p string, via bool) (*Entry, error) {
	e, err := w.statEntry(f, p, baseName(p), via)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

func (r *Result) remove(it *Item) {
	for i, x := range r.Items {
		if x == it {
			r.Items = append(r.Items[:i], r.Items[i+1:]...)
			break
		}
	}
	delete(r.index, it.Path)
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
