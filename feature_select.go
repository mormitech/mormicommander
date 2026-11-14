package main

// Selection and masks (FEATURES.md sections 6 and 13): the page keeps which rows are selected, and asks this object
// which rows a mask, an extension, a name, a selection file, the clipboard or a folder compare picks. The matching
// rules are internal/masks (TC wildcards, "|" excludes, "<regex", saved filters, NFC names, case rule), the clipboard
// text formats are internal/clipfiles. Every method answers with row indexes into the entries the page sent, so the
// page never needs its own copy of the mask rules.
//
// MORMICOMMANDER_CLIPBOARD_FILE, when set, makes the copy-names and load-from-clipboard commands use that text file
// instead of the system clipboard, so tests never touch the user's clipboard.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/clipfiles"
	"mormicommander/internal/masks"
	"mormicommander/internal/vfs"
)

// Select is what the page calls for selecting by mask, by name, from files and the clipboard, and for folder compare
// (window.go.main.Select).
type Select struct {
	app    *App
	mu     sync.Mutex
	cancel context.CancelFunc // the folder compare in progress
}

// SelectEntry is one row of a panel as the selection commands need it.
type SelectEntry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`  // bytes; for folders the calculated size, or -1
	MTime int64  `json:"mtime"` // Unix milliseconds
	Attr  string `json:"attr"`  // the Attr column as the panel shows it
}

// SelectCompareResult is the answer of a folder compare: the names to select on each side.
type SelectCompareResult struct {
	Left      []string `json:"left"`
	Right     []string `json:"right"`
	Differs   bool     `json:"differs"`
	Cancelled bool     `json:"cancelled"`
	Warning   string   `json:"warning"` // a folder that could not be read, if any
}

func init() {
	registerFeature("select", func(a *App) []exposed { return []exposed{expose(&Select{app: a})} })
}

// selKind turns "files", "folders" or "both" into a masks.Kind.
func selKind(k string) masks.Kind {
	switch k {
	case "files":
		return masks.KindFiles
	case "folders":
		return masks.KindFolders
	}
	return masks.KindBoth
}

// selEntries turns page rows into masks entries.
func selEntries(es []SelectEntry) []masks.Entry {
	out := make([]masks.Entry, len(es))
	for i, e := range es {
		out[i] = masks.Entry{Name: e.Name, IsDir: e.Dir, Size: e.Size, Attr: e.Attr}
		if e.MTime != 0 {
			out[i].ModTime = time.UnixMilli(e.MTime)
		}
	}
	return out
}

// selNew makes a fresh selection over the page rows.
func selNew(es []SelectEntry) *masks.Selection {
	return masks.NewSelection(selEntries(es), masks.SelectionOptions{})
}

// selIndexes is the rows a fresh selection ended up with; never nil, so the page always gets an array.
func selIndexes(s *masks.Selection) []int {
	if out := s.Selected(); out != nil {
		return out
	}
	return []int{}
}

// selErr turns an engine error into a sentence for the dialog.
func selErr(err error) error {
	var re *masks.RegexpError
	if errors.As(err, &re) {
		return err
	}
	t := strings.TrimPrefix(err.Error(), "masks: ")
	if t != "" {
		t = strings.ToUpper(t[:1]) + t[1:]
	}
	if !strings.HasSuffix(t, ".") {
		t += "."
	}
	return errors.New(t)
}

// selParse parses a mask expression; filters are the saved filters by name (written as ">name").
func selParse(expr string, filters map[string]string) (*masks.List, error) {
	l, err := masks.Parse(expr, masks.Options{Resolve: func(name string) (string, bool) {
		for k, v := range filters {
			if strings.EqualFold(k, name) {
				return v, true
			}
		}
		return "", false
	}})
	if err != nil {
		return nil, selErr(err)
	}
	return l, nil
}

// Check says whether a mask expression is valid: "" when it is, otherwise the reason.
func (s *Select) Check(expr string, filters map[string]string) string {
	if _, err := selParse(expr, filters); err != nil {
		return err.Error()
	}
	return ""
}

// Group returns the rows of kind ("files", "folders" or "both") that match the mask expression (Num +, Num -).
func (s *Select) Group(entries []SelectEntry, expr string, kind string, filters map[string]string) ([]int, error) {
	l, err := selParse(expr, filters)
	if err != nil {
		return nil, err
	}
	sel := selNew(entries)
	sel.SelectGroup(l, selKind(kind))
	return selIndexes(sel), nil
}

// Same returns the rows of kind with the same extension ("ext"), base name ("name") or full name ("nameext") as row cur.
func (s *Select) Same(entries []SelectEntry, cur int, mode string, kind string) []int {
	sel := selNew(entries)
	k := selKind(kind)
	switch mode {
	case "ext":
		sel.SelectSameExt(cur, true, k)
	case "name":
		sel.SelectSameName(cur, true, k)
	default:
		sel.SelectSameNameExt(cur, true, k)
	}
	return selIndexes(sel)
}

// Names returns the rows whose names are in names (same case rule and Unicode normalisation as the masks).
func (s *Select) Names(entries []SelectEntry, names []string) []int {
	sel := selNew(entries)
	sel.SelectNames(names, true)
	return selIndexes(sel)
}

// selApply returns the rows that the lines of a selection list pick in folder dir.
func selApply(entries []SelectEntry, lines []string, dir string) []int {
	sel := selNew(entries)
	sel.ApplyList(lines, dir, false)
	return selIndexes(sel)
}

// LoadFile reads a selection file (one name, full path or mask per line) and returns the rows it picks in folder dir.
func (s *Select) LoadFile(path, dir string, entries []SelectEntry) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("The file %s is not there.", path)
		}
		return nil, fmt.Errorf("The file %s could not be read: %v", path, selCause(err))
	}
	defer f.Close()
	lines, err := masks.ReadSelection(f)
	if err != nil {
		return nil, fmt.Errorf("The file %s could not be read: %v", path, selCause(err))
	}
	return selApply(entries, lines, dir), nil
}

// LoadClip reads names from the clipboard text and returns the rows they pick in folder dir; no text picks nothing.
func (s *Select) LoadClip(dir string, entries []SelectEntry) ([]int, error) {
	text, err := selReadText()
	if err != nil {
		if errors.Is(err, clipfiles.ErrEmpty) {
			return []int{}, nil
		}
		return nil, fmt.Errorf("The clipboard could not be read: %v", selCause(err))
	}
	return selApply(entries, clipfiles.ParseNameList(text), dir), nil
}

// Exists says whether a file or folder is at path.
func (s *Select) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// SaveFile writes names to a selection file, one per line; with full set each line is the full path in folder dir.
func (s *Select) SaveFile(path, dir string, names []string, full bool) error {
	var b strings.Builder
	if runtime.GOOS == "windows" {
		b.WriteString("\xEF\xBB\xBF") // TC reads UTF-8 with a byte order mark on Windows
	}
	if err := masks.WriteSelection(&b, dir, names, full); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("The file %s could not be written: %v", path, selCause(err))
	}
	return nil
}

// CopyNames puts the rows on the clipboard as text: cmd is the TC command (cm_CopyNamesToClip, cm_CopyFullNamesToClip,
// cm_CopyDetailsToClip, cm_CopyFullDetailsToClip, cm_CopyNetNamesToClip, cm_CopyUnixNamesToClip,
// cm_CopyURLNamesToClip, and our cm_CopyFullNamesQuotedToClip). It returns the text it copied.
func (s *Select) CopyNames(cmd, dir string, entries []SelectEntry) (string, error) {
	kind, ok := clipfiles.CommandKind(cmd)
	if !ok {
		return "", fmt.Errorf("%s is not a copy-names command.", cmd)
	}
	fsys := vfs.For(dir, s.app.fs)
	list := make([]clipfiles.Entry, len(entries))
	for i, e := range entries {
		list[i] = clipfiles.Entry{Path: fsys.Join(dir, e.Name), Name: e.Name, Size: e.Size, IsDir: e.Dir, Attr: e.Attr}
		if e.MTime != 0 {
			list[i].ModTime = time.UnixMilli(e.MTime)
		}
	}
	// Details: a folder whose size was calculated shows it, as in the panel; the others show <DIR>.
	var text []string
	for _, e := range list {
		f := clipfiles.Format{DateLayout: "2006-01-02 15:04"}
		if e.IsDir && e.Size >= 0 && (kind == clipfiles.Details || kind == clipfiles.FullDetails) {
			f.DirSizeText = fmt.Sprint(e.Size)
		}
		text = append(text, clipfiles.FormatText(kind, []clipfiles.Entry{e}, f))
	}
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	out := strings.Join(text, nl)
	if err := selWriteText(out); err != nil {
		return "", fmt.Errorf("The names could not be copied to the clipboard: %v", selCause(err))
	}
	return out, nil
}

// CopyPath puts a folder path on the clipboard (cm_CopySrcPathToClip, cm_CopyTrgPathToClip) and returns it.
func (s *Select) CopyPath(dir string) (string, error) {
	p := dir
	if !strings.Contains(dir, "://") {
		p = clipfiles.FolderPath(dir, false)
	}
	if err := selWriteText(p); err != nil {
		return "", fmt.Errorf("The path could not be copied to the clipboard: %v", selCause(err))
	}
	return p, nil
}

// ReadClipText returns the clipboard text ("" when there is none), for tests and the command line.
func (s *Select) ReadClipText() string {
	t, _ := selReadText()
	return t
}

// Compare compares two folders the way Shift+F2 does and returns the names to select on each side: files that are
// newer or missing on the other side, and files of the same date with a different size. With subdirs, folders on
// both sides are compared through all their subfolders and selected when anything inside differs.
func (s *Select) Compare(leftDir, rightDir string, left, right []SelectEntry, subdirs, ignoreHour bool) (SelectCompareResult, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		cancel()
		s.cancel = nil
		s.mu.Unlock()
	}()

	opt := masks.DefaultCompareOptions()
	opt.IgnoreHourShift = ignoreHour
	opt.Subdirs = subdirs
	le, re := selEntries(left), selEntries(right)
	var res masks.CompareResult
	if subdirs {
		res = selCompareTrees(le, re, selVFS{ctx: ctx, fs: vfs.For(leftDir, s.app.fs), root: leftDir},
			selVFS{ctx: ctx, fs: vfs.For(rightDir, s.app.fs), root: rightDir}, opt)
	} else {
		res = masks.CompareLists(le, re, opt)
	}
	out := SelectCompareResult{Left: []string{}, Right: []string{}, Differs: res.Differs()}
	if ctx.Err() != nil {
		out.Cancelled = true
		return out, nil
	}
	for i, m := range res.LeftMarks() {
		if m {
			out.Left = append(out.Left, le[i].Name)
		}
	}
	for i, m := range res.RightMarks() {
		if m {
			out.Right = append(out.Right, re[i].Name)
		}
	}
	if res.Err != nil {
		out.Warning = "Some folders could not be read: " + selCause(res.Err).Error()
	}
	return out, nil
}

// CancelCompare stops the folder compare in progress.
func (s *Select) CancelCompare() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
}

// selCompareTrees compares the rows both panels show (so filters and hidden files count as they do on screen) and
// looks into the folders present on both sides through the two file systems: such a folder is selected when anything
// in its sub-tree is newer, missing or of another size. A folder on one side only is selected like a missing file.
func selCompareTrees(le, re []masks.Entry, lfs, rfs selVFS, opt masks.CompareOptions) masks.CompareResult {
	flat := masks.CompareLists(le, re, opt) // Subdirs set: one-sided folders are marked, shared ones are Same
	l, r, tree, err := masks.CompareTrees(lfs, ".", rfs, ".", opt)
	if err != nil {
		flat.Err = err
		return flat
	}
	dirStatus := func(list []masks.Entry, st []masks.CompareStatus) map[string]masks.CompareStatus {
		m := map[string]masks.CompareStatus{}
		for i, e := range list {
			if e.IsDir && st[i] == masks.StatusDirDiffers {
				m[e.Name] = st[i]
			}
		}
		return m
	}
	ld, rd := dirStatus(l, tree.Left), dirStatus(r, tree.Right)
	for i, e := range le {
		if st, ok := ld[e.Name]; ok && e.IsDir {
			flat.Left[i] = st
		}
	}
	for i, e := range re {
		if st, ok := rd[e.Name]; ok && e.IsDir {
			flat.Right[i] = st
		}
	}
	flat.Err = tree.Err
	return flat
}

// selCause strips the "open /path:" part of an OS error, which the sentence around it already names.
func selCause(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// ---- the clipboard, or the test file in its place ----

func selWriteText(t string) error {
	if p := os.Getenv("MORMICOMMANDER_CLIPBOARD_FILE"); p != "" {
		return os.WriteFile(p, []byte(t), 0o644)
	}
	return clipfiles.WriteText(t)
}

func selReadText() (string, error) {
	if p := os.Getenv("MORMICOMMANDER_CLIPBOARD_FILE"); p != "" {
		b, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && len(b) == 0) {
			return "", clipfiles.ErrEmpty
		}
		return string(b), err
	}
	return clipfiles.ReadText()
}

// ---- a panel file system seen as an io/fs.FS, for the recursive compare ----

// selVFS shows folder root of a panel file system (local, demo, archive, FTP…) as an io/fs.FS that can list folders.
// Links to folders are listed as files, so a link loop can never make the compare walk forever.
type selVFS struct {
	ctx  context.Context
	fs   vfs.FS
	root string
}

func (v selVFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
}

func (v selVFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := v.ctx.Err(); err != nil {
		return nil, err
	}
	p := v.root
	if name != "." && name != "" {
		for _, part := range strings.Split(name, "/") {
			p = v.fs.Join(p, part)
		}
	}
	l, err := v.fs.List(v.ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, 0, len(l.Entries))
	for _, e := range l.Entries {
		if e.Name == "." || e.Name == ".." {
			continue
		}
		out = append(out, selDirEntry{e})
	}
	return out, nil
}

type selDirEntry struct{ e vfs.Entry }

func (d selDirEntry) Name() string { return d.e.Name }
func (d selDirEntry) IsDir() bool  { return d.e.Flags&vfs.FlagDir != 0 && d.e.Flags&vfs.FlagLink == 0 }
func (d selDirEntry) Type() fs.FileMode {
	if d.IsDir() {
		return fs.ModeDir
	}
	return 0
}
func (d selDirEntry) Info() (fs.FileInfo, error) { return d, nil }
func (d selDirEntry) Size() int64                { return d.e.Size }
func (d selDirEntry) Mode() fs.FileMode          { return fs.FileMode(d.e.Mode&0o777) | d.Type() }
func (d selDirEntry) ModTime() time.Time         { return time.UnixMilli(d.e.MTime) }
func (d selDirEntry) Sys() any                   { return nil }

var _ fs.ReadDirFS = selVFS{}
