package main

// Find files (Alt+F7, cm_SearchFor; FEATURES.md section 24): the page's Find dialog talks to this bound object. A search
// runs in a goroutine over internal/find; results go to the page in batches as "find-results" events, progress as
// "find-progress" and the end as "find-done", so the list fills while the search goes on and Stop works at any time.
// The settings of the last search, the named saved searches (find.Store, searches.json) and the histories of the text
// fields (config.History in find.ini, TC's [SearchName] / [SearchIn] / [SearchText]) live in the settings folder.
//
// The dialog sends a FindRequest: plain fields, close to the controls of the dialog, which this file checks and turns
// into find.Options. Errors are sentences for the user. Feed to listbox (feature_find_feed.go), the file system used for
// remote and demo folders (feature_find_fs.go) and quick search / quick filter matching (feature_find_quick.go) are in
// files of their own.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/config"
	"mormicommander/internal/find"
	"mormicommander/internal/vfs"
)

// Find is what the page calls for Find files, Feed to listbox and quick search (window.go.main.Find).
type Find struct {
	app *App

	mu    sync.Mutex
	seq   int
	runs  map[int]*findRun
	store *find.Store
	hist  map[string]*config.History // "name", "in", "text"
	histF *config.File
	quick map[string]*findQuickList // quick search names per panel (feature_find_quick.go)
	arc   *vfs.ArchiveFS            // archives opened by searches that look inside archives
}

// findRun is one search: the job and what it found so far (for Feed to listbox, copy and save).
type findRun struct {
	id      int
	job     *find.Job
	mu      sync.Mutex
	results []find.Result
	done    bool
	cut     bool // more results than findMaxKept: the rest went to the page but is not kept here
}

// findMaxKept is how many results a search keeps for Feed to listbox and saving (the page gets all of them).
const findMaxKept = 500000

func init() {
	registerFeature("find", func(a *App) []exposed {
		f := &Find{app: a, runs: map[int]*findRun{}, quick: map[string]*findQuickList{}, arc: vfs.NewArchiveFS()}
		findFeeds.def = a.fs
		return []exposed{expose(f)}
	})
}

// FindRequest is the Find dialog's fields, as the page sends them.
type FindRequest struct {
	SearchFor string   `json:"searchFor"`
	NameRegex bool     `json:"nameRegex"`
	SearchIn  string   `json:"searchIn"`           // folders separated by ";" (quotes allowed)
	Selected  []string `json:"selected,omitempty"` // "Only search in selected folders/files": these full paths
	WithinRun int      `json:"withinRun,omitempty"`
	Depth     int      `json:"depth"` // -1 = all subfolders, 0 = the folder only, n = n levels below
	Archives  bool     `json:"archives"`

	TextOn        bool     `json:"textOn"`
	Text          string   `json:"text"`
	WholeWords    bool     `json:"wholeWords"`
	CaseSensitive bool     `json:"caseSensitive"`
	NotContaining bool     `json:"notContaining"`
	Hex           bool     `json:"hex"`
	TextRegex     bool     `json:"textRegex"`
	Encodings     []string `json:"encodings"` // "ansi", "oem", "utf8", "utf16le", "utf16be"
	ANSICodepage  string   `json:"ansiCodepage"`
	OEMCodepage   string   `json:"oemCodepage"`

	DateOn    bool   `json:"dateOn"`
	DateFrom  string `json:"dateFrom"` // YYYY-MM-DD, local time, whole days
	DateTo    string `json:"dateTo"`
	AgeOn     bool   `json:"ageOn"`
	AgeOlder  bool   `json:"ageOlder"` // "Older than" instead of "Not older than"
	AgeN      int    `json:"ageN"`
	AgeUnit   string `json:"ageUnit"`
	SizeOn    bool   `json:"sizeOn"`
	SizeOp    string `json:"sizeOp"`
	SizeValue int64  `json:"sizeValue"`
	SizeUnit  string `json:"sizeUnit"`

	AttrSet     string `json:"attrSet"`   // attribute letters that must be set (find.ParseAttr)
	AttrClear   string `json:"attrClear"` // and that must not be
	Perms       string `json:"perms"`     // "rw-r-x???": r/w/x must be set, - must not be, ? or space don't care
	Kind        string `json:"kind"`      // "", "files", "dirs"
	FollowLinks bool   `json:"followLinks"`
	SkipHidden  bool   `json:"skipHidden"`
	ExcludeDirs string `json:"excludeDirs"` // folder names separated by ";"

	DupOn      bool `json:"dupOn"`
	DupName    bool `json:"dupName"`
	DupSize    bool `json:"dupSize"`
	DupContent bool `json:"dupContent"`
}

// findEncodings maps the dialog's names to the engine's bits.
var findEncodings = []struct {
	name string
	bit  find.Encoding
}{{"ansi", find.EncANSI}, {"oem", find.EncOEM}, {"utf8", find.EncUTF8}, {"utf16le", find.EncUTF16LE}, {"utf16be", find.EncUTF16BE}}

var findAgeUnits = map[string]find.AgeUnit{"minutes": find.Minutes, "hours": find.Hours, "days": find.Days, "weeks": find.Weeks, "months": find.Months, "years": find.Years}

// findOptions checks a request and turns it into engine options. Search in may name a fed result list (find://…),
// which searches only the files of that list (F-544).
func (f *Find) findOptions(r FindRequest) (find.Options, error) {
	o := find.DefaultOptions()
	o.SearchFor = strings.TrimSpace(r.SearchFor)
	if o.SearchFor == "" {
		o.SearchFor = "*"
	}
	o.NameRegex = r.NameRegex
	o.Levels = r.Depth
	if o.Levels < -1 {
		o.Levels = find.Unlimited
	}
	o.SearchArchives = r.Archives
	o.FollowLinks = r.FollowLinks
	o.SkipHidden = r.SkipHidden
	switch r.Kind {
	case "files":
		o.Kind = find.KindFiles
	case "dirs":
		o.Kind = find.KindDirs
	}
	for _, d := range find.ParseSearchIn(r.ExcludeDirs) {
		o.ExcludeDirs = append(o.ExcludeDirs, strings.TrimRight(d, `/\`))
	}

	// where
	switch {
	case r.WithinRun > 0:
		f.mu.Lock()
		run := f.runs[r.WithinRun]
		f.mu.Unlock()
		if run == nil {
			return o, errors.New("The earlier results are gone. Start a new search.")
		}
		run.mu.Lock()
		for _, x := range run.results {
			o.Within = append(o.Within, x.Path)
		}
		run.mu.Unlock()
		if len(o.Within) == 0 {
			return o, errors.New("There are no results to search in.")
		}
	case len(r.Selected) > 0:
		o.SearchIn = append(o.SearchIn, r.Selected...)
	default:
		for _, p := range find.ParseSearchIn(r.SearchIn) {
			if findFeedPath(p) {
				paths, ok := findFeeds.paths(p)
				if !ok {
					return o, fmt.Errorf("The search results %s are no longer there.", p)
				}
				o.Within = append(o.Within, paths...)
				continue
			}
			o.SearchIn = append(o.SearchIn, f.cleanRoot(p))
		}
		if len(o.SearchIn) == 0 && len(o.Within) == 0 {
			return o, errors.New("Please enter a folder to search in.")
		}
	}

	// text
	if r.TextOn && r.Text != "" {
		c := find.ContentOptions{Text: r.Text, CaseSensitive: r.CaseSensitive, WholeWords: r.WholeWords, Regex: r.TextRegex,
			Hex: r.Hex, NotContaining: r.NotContaining, ANSICodepage: r.ANSICodepage, OEMCodepage: r.OEMCodepage}
		for _, e := range r.Encodings {
			for _, x := range findEncodings {
				if x.name == e {
					c.Encodings |= x.bit
				}
			}
		}
		if c.Hex {
			if _, err := find.ParseHex(c.Text); err != nil {
				return o, errors.New("The hex text is not valid: use pairs of hex digits, like 4D 5A 90 00.")
			}
			if c.Encodings == 0 {
				c.Encodings = find.DefaultEncodings
			}
		} else if c.Encodings == 0 {
			return o, errors.New("Please choose at least one character set for Find text.")
		}
		o.Content = c
	} else if r.TextOn {
		return o, errors.New("Please enter the text to find, or turn off Find text.")
	}

	// date
	if r.DateOn {
		from, err := findDay(r.DateFrom)
		if err != nil {
			return o, err
		}
		to, err := findDay(r.DateTo)
		if err != nil {
			return o, err
		}
		if !from.IsZero() && !to.IsZero() && from.After(to) {
			from, to = to, from
		}
		o.Date.From = from
		if !to.IsZero() {
			o.Date.To = to.AddDate(0, 0, 1).Add(-time.Nanosecond) // the whole last day
		}
		if from.IsZero() && to.IsZero() {
			return o, errors.New("Please enter at least one date for Date between.")
		}
	}
	if r.AgeOn {
		u, ok := findAgeUnits[r.AgeUnit]
		if !ok {
			u = find.Days
		}
		if r.AgeN <= 0 {
			return o, errors.New("Please enter a number above 0 for the age.")
		}
		if r.AgeOlder {
			o.Date.OlderThan = find.Age{N: r.AgeN, Unit: u}
		} else {
			o.Date.NotOlderThan = find.Age{N: r.AgeN, Unit: u}
		}
	}

	// size
	if r.SizeOn {
		op := find.SizeOp(r.SizeOp)
		if op != find.SizeEqual && op != find.SizeGreater && op != find.SizeLess {
			op = find.SizeGreater
		}
		if r.SizeValue < 0 {
			return o, errors.New("The file size can't be below 0.")
		}
		unit := find.SizeUnit(strings.ToUpper(r.SizeUnit))
		if unit == "" {
			unit = find.Bytes
		}
		o.Size = find.SizeFilter{Op: op, Value: r.SizeValue, Unit: unit}
	}

	// attributes and permissions
	o.AttrSet = find.ParseAttr(r.AttrSet)
	o.AttrClear = find.ParseAttr(r.AttrClear)
	if o.AttrSet&o.AttrClear != 0 {
		return o, errors.New("An attribute can't be both set and not set.")
	}
	mask, value, err := findPerms(r.Perms)
	if err != nil {
		return o, err
	}
	o.PermMask, o.PermValue = mask, value

	// duplicates
	if r.DupOn {
		if !r.DupName && !r.DupSize && !r.DupContent {
			return o, errors.New("For duplicate files, choose at least one of same name, same size or same content.")
		}
		o.Duplicates = find.DupOptions{Enabled: true, Name: r.DupName, Size: r.DupSize, Content: r.DupContent}
	}

	// the file system: the disk directly, unless a folder is remote or this is the demo
	remote := f.app.mock != nil
	for _, p := range append(append([]string(nil), o.SearchIn...), o.Within...) {
		if strings.Contains(p, "://") {
			remote = true
		}
	}
	if remote {
		o.FS = &findVFS{def: f.app.fs, demo: f.app.mock != nil}
	}
	if o.SearchArchives && f.app.mock == nil {
		o.Archives = findArchives{arc: f.arc}
	}
	return o, nil
}

// cleanRoot makes a typed start folder absolute ("~" is home); URLs stay as they are.
func (f *Find) cleanRoot(p string) string {
	if strings.Contains(p, "://") {
		return p
	}
	return vfs.For(p, f.app.fs).Clean(p)
}

// findDay reads a date of the dialog ("" is no date).
func findDay(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("The date %q is not valid; write it as year-month-day, like 2026-03-31.", s)
	}
	return t, nil
}

// findPerms reads the permissions field: nine places rwxrwxrwx; a letter must be set, "-" must not be, "?" or a space
// doesn't matter. An empty field is no filter.
func findPerms(s string) (mask, value fs.FileMode, err error) {
	if strings.Trim(s, "? ") == "" {
		return 0, 0, nil
	}
	if len(s) > 9 {
		return 0, 0, errors.New("Permissions have nine places, like rw-r--r-- (use ? where anything goes).")
	}
	for i := 0; i < len(s); i++ {
		bit := fs.FileMode(0o400) >> i
		switch c := s[i]; {
		case c == '?' || c == ' ':
		case c == '-':
			mask |= bit
		case strings.IndexByte("rwxRWXsStT", c) >= 0:
			mask |= bit
			value |= bit
		default:
			return 0, 0, fmt.Errorf("%q is not a permission letter; use r, w, x, - or ?.", string(c))
		}
	}
	return mask, value, nil
}

// findRequest turns saved options back into the dialog's fields (saved searches, the last search).
func findRequest(o find.Options) FindRequest {
	r := FindRequest{SearchFor: o.SearchFor, NameRegex: o.NameRegex, SearchIn: findJoinIn(o.SearchIn), Depth: o.Levels,
		Archives: o.SearchArchives, FollowLinks: o.FollowLinks, SkipHidden: o.SkipHidden, ExcludeDirs: strings.Join(o.ExcludeDirs, ";")}
	switch o.Kind {
	case find.KindFiles:
		r.Kind = "files"
	case find.KindDirs:
		r.Kind = "dirs"
	}
	c := o.Content
	r.TextOn, r.Text, r.WholeWords, r.CaseSensitive, r.NotContaining, r.Hex, r.TextRegex = c.Text != "", c.Text, c.WholeWords, c.CaseSensitive, c.NotContaining, c.Hex, c.Regex
	r.ANSICodepage, r.OEMCodepage = c.ANSICodepage, c.OEMCodepage
	enc := c.Encodings
	if enc == 0 {
		enc = find.DefaultEncodings
	}
	for _, x := range findEncodings {
		if enc&x.bit != 0 {
			r.Encodings = append(r.Encodings, x.name)
		}
	}
	if !o.Date.From.IsZero() || !o.Date.To.IsZero() {
		r.DateOn = true
		if !o.Date.From.IsZero() {
			r.DateFrom = o.Date.From.Local().Format("2006-01-02")
		}
		if !o.Date.To.IsZero() {
			r.DateTo = o.Date.To.Local().Format("2006-01-02")
		}
	}
	if a := o.Date.NotOlderThan; a.N != 0 {
		r.AgeOn, r.AgeN, r.AgeUnit = true, a.N, string(a.Unit)
	} else if a := o.Date.OlderThan; a.N != 0 {
		r.AgeOn, r.AgeOlder, r.AgeN, r.AgeUnit = true, true, a.N, string(a.Unit)
	}
	if o.Size.Active() {
		r.SizeOn, r.SizeOp, r.SizeValue, r.SizeUnit = true, string(o.Size.Op), o.Size.Value, string(o.Size.Unit)
	}
	r.AttrSet, r.AttrClear = o.AttrSet.String(), o.AttrClear.String()
	if o.PermMask != 0 {
		b := []byte("?????????")
		for i := range b {
			bit := fs.FileMode(0o400) >> i
			if o.PermMask&bit != 0 {
				if o.PermValue&bit != 0 {
					b[i] = "rwx"[i%3]
				} else {
					b[i] = '-'
				}
			}
		}
		r.Perms = string(b)
	}
	d := o.Duplicates
	r.DupOn, r.DupName, r.DupSize, r.DupContent = d.Enabled, d.Name, d.Size, d.Content
	return r
}

// findJoinIn writes start folders back as the Search in field (quoting folders that have a ";").
func findJoinIn(in []string) string {
	out := make([]string, len(in))
	for i, p := range in {
		if strings.Contains(p, ";") {
			p = `"` + p + `"`
		}
		out[i] = p
	}
	return strings.Join(out, ";")
}

// ---- starting and stopping ----

// findItem is one result as the page gets it (short keys: a search can find a lot).
type findItem struct {
	Path    string `json:"p"`
	Name    string `json:"n"`
	Dir     string `json:"d"`           // the folder it is in (for an archive member: the folder inside the archive)
	Size    int64  `json:"s"`           // bytes
	MTime   int64  `json:"t"`           // Unix milliseconds
	IsDir   bool   `json:"f,omitempty"` // a folder
	Group   int    `json:"g,omitempty"` // duplicate group, 1…n
	Archive bool   `json:"a,omitempty"` // inside an archive
}

func findItemOf(r find.Result) findItem {
	dir := filepath.Dir(r.Path)
	if strings.Contains(r.Path, "://") {
		dir = path.Dir(r.Path)
	}
	return findItem{Path: r.Path, Name: r.Name, Dir: dir, Size: r.Size, MTime: r.ModTime.UnixMilli(), IsDir: r.IsDir, Group: r.DupGroup, Archive: r.Archive != ""}
}

// FindDone is the end of a search, as the "find-done" event and Wait tell it.
type FindDone struct {
	ID       int              `json:"id"`
	Files    int              `json:"files"`
	Dirs     int              `json:"dirs"`
	Stopped  bool             `json:"stopped"`
	Error    string           `json:"error,omitempty"`
	Errors   int64            `json:"errors"`
	Problems []find.PathError `json:"problems,omitempty"` // the first few paths that could not be read
	Examined int64            `json:"examined"`
	Elapsed  int64            `json:"elapsed"` // milliseconds
	Groups   int              `json:"groups,omitempty"`
	Cut      bool             `json:"cut,omitempty"`
}

// Start checks the request and starts the search; the answer is the search's number for the events and Stop. The
// request becomes the "last search" and its texts go to the histories.
func (f *Find) Start(r FindRequest) (int, error) {
	o, err := f.findOptions(r)
	if err != nil {
		return 0, err
	}
	q, err := find.Compile(o)
	if err != nil {
		return 0, errors.New(findErrText(err))
	}
	f.remember(r, o)
	f.mu.Lock()
	f.seq++
	run := &findRun{id: f.seq}
	f.runs[run.id] = run
	// only the last few searches are kept (for Feed to listbox and search in results)
	for id := range f.runs {
		if id <= run.id-8 {
			if old := f.runs[id]; old.job != nil {
				old.job.Stop()
			}
			delete(f.runs, id)
		}
	}
	f.mu.Unlock()
	run.job = q.Start(context.Background())
	go f.pump(run)
	return run.id, nil
}

// pump sends a search's results, progress and end to the page.
func (f *Find) pump(run *findRun) {
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	var batch []findItem
	files, dirs := 0, 0
	flush := func() {
		if len(batch) > 0 {
			f.app.emit("find-results", map[string]any{"id": run.id, "items": batch})
			batch = nil
		}
	}
	ch := run.job.Results()
	groups := 0
	for {
		select {
		case r, ok := <-ch:
			if !ok {
				flush()
				st, err := run.job.Wait()
				d := FindDone{ID: run.id, Files: files, Dirs: dirs, Errors: st.Errors, Examined: st.Files, Elapsed: st.Elapsed.Milliseconds(), Groups: groups}
				if errors.Is(err, context.Canceled) {
					d.Stopped = true
				} else if err != nil {
					d.Error = findErrText(err)
				}
				probs := run.job.Errors()
				if len(probs) > 5 {
					probs = probs[:5]
				}
				d.Problems = probs
				run.mu.Lock()
				run.done = true
				d.Cut = run.cut
				run.mu.Unlock()
				f.app.emit("find-done", d)
				return
			}
			if r.IsDir {
				dirs++
			} else {
				files++
			}
			if r.DupGroup > groups {
				groups = r.DupGroup
			}
			run.mu.Lock()
			if len(run.results) < findMaxKept {
				run.results = append(run.results, r)
			} else {
				run.cut = true
			}
			run.mu.Unlock()
			batch = append(batch, findItemOf(r))
			if len(batch) >= 500 {
				flush()
			}
		case <-tick.C:
			flush()
			f.app.emit("find-progress", map[string]any{"id": run.id, "progress": run.job.Progress()})
		}
	}
}

// Stop ends a search; what it found so far stays.
func (f *Find) Stop(id int) {
	f.mu.Lock()
	run := f.runs[id]
	f.mu.Unlock()
	if run != nil && run.job != nil {
		run.job.Stop()
	}
}

// Running says whether a search is still going.
func (f *Find) Running(id int) bool {
	f.mu.Lock()
	run := f.runs[id]
	f.mu.Unlock()
	if run == nil {
		return false
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	return !run.done
}

// Results are the results a search kept (for control mode and tests).
func (f *Find) Results(id int) []findItem {
	f.mu.Lock()
	run := f.runs[id]
	f.mu.Unlock()
	if run == nil {
		return nil
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	out := make([]findItem, len(run.results))
	for i, r := range run.results {
		out[i] = findItemOf(r)
	}
	return out
}

// Forget drops a search's results (the dialog closed for good).
func (f *Find) Forget(id int) {
	f.mu.Lock()
	run := f.runs[id]
	delete(f.runs, id)
	f.mu.Unlock()
	if run != nil && run.job != nil {
		run.job.Stop()
	}
}

// findErrText turns an engine error into a sentence ("find: bad …" → "Bad …").
func findErrText(err error) string {
	t := strings.TrimPrefix(err.Error(), "find: ")
	if t == "" {
		return "The search failed."
	}
	t = strings.ToUpper(t[:1]) + t[1:]
	if !strings.HasSuffix(t, ".") {
		t += "."
	}
	return t
}

// ---- results as text (F-838) ----

// findText writes the results as text: one full path per line, with size and date when details is true.
func (f *Find) findText(paths []string, details bool) string {
	var b strings.Builder
	nl := "\n"
	if runtime.GOOS == "windows" {
		nl = "\r\n"
	}
	rs := make([]find.Result, 0, len(paths))
	for _, p := range paths {
		r := find.Result{Path: p}
		if details {
			if e, err := vfs.For(p, f.app.fs).Stat(p); err == nil {
				r.Size, r.ModTime, r.IsDir = e.Size, time.UnixMilli(e.MTime), e.Flags&vfs.FlagDir != 0
			}
		}
		rs = append(rs, r)
	}
	_ = find.WriteResults(&b, rs, find.WriteOptions{Details: details, Newline: nl})
	return b.String()
}

// CopyText puts the given result paths on the clipboard as text, one per line.
func (f *Find) CopyText(paths []string, details bool) error {
	if f.app.ctx == nil {
		return errors.New("The clipboard is not available.")
	}
	return f.app.clipboardSetText(f.findText(paths, details))
}

// SaveText writes the given result paths to a text file, one per line (an existing file is replaced only when
// overwrite is true).
func (f *Find) SaveText(file string, paths []string, details, overwrite bool) error {
	file = vfs.Local{}.Clean(file)
	if _, err := os.Stat(file); err == nil && !overwrite {
		return fmt.Errorf("%s is already there.", file)
	}
	if err := config.WriteFileAtomic(file, []byte(f.findText(paths, details)), 0o644); err != nil {
		return fmt.Errorf("Can't write %s: %v", file, err)
	}
	return nil
}

// ---- last search, histories and saved searches (F-537, F-542) ----

// FindSetup is what the dialog needs when it opens.
type FindSetup struct {
	Last      *FindRequest        `json:"last"`      // the last search's fields, or nil
	History   map[string][]string `json:"history"`   // "name", "in", "text": most recent first
	Saved     []string            `json:"saved"`     // names of saved searches
	Codepages []string            `json:"codepages"` // for the ANSI and OEM choices
	Platform  string              `json:"platform"`
}

// Setup gives the dialog its histories, the last search and the saved searches.
func (f *Find) Setup() FindSetup {
	s := FindSetup{History: map[string][]string{}, Codepages: find.Codepages(), Platform: runtime.GOOS}
	f.mu.Lock()
	f.loadHistLocked()
	for k, h := range f.hist {
		s.History[k] = h.Items()
	}
	f.mu.Unlock()
	if st, err := f.openStore(); err == nil {
		if o, ok := st.Last(); ok {
			r := findRequest(o)
			s.Last = &r
		}
		s.Saved = st.Names()
	}
	return s
}

func (f *Find) loadHistLocked() {
	if f.hist != nil {
		return
	}
	f.hist = map[string]*config.History{"name": config.NewHistory(20, false), "in": config.NewHistory(20, false), "text": config.NewHistory(20, false)}
	f.histF = config.NewFile()
	if dir := configDir(); dir != "" {
		if file, err := config.LoadFile(filepath.Join(dir, "find.ini")); err == nil {
			f.histF = file
		}
	}
	for k, sec := range findHistSections {
		f.hist[k].Load(f.histF, sec)
	}
}

var findHistSections = map[string]string{"name": "SearchName", "in": "SearchIn", "text": "SearchText"}

// remember keeps a started search: its texts in the histories, its options as the last search.
func (f *Find) remember(r FindRequest, o find.Options) {
	f.mu.Lock()
	f.loadHistLocked()
	if t := strings.TrimSpace(r.SearchFor); t != "" {
		f.hist["name"].Add(t)
	}
	if t := strings.TrimSpace(r.SearchIn); t != "" && len(r.Selected) == 0 && r.WithinRun == 0 && !findFeedPath(t) {
		f.hist["in"].Add(t)
	}
	if r.TextOn && r.Text != "" {
		f.hist["text"].Add(r.Text)
	}
	for k, sec := range findHistSections {
		f.hist[k].Save(f.histF, sec)
	}
	if dir := configDir(); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
		_ = f.histF.Save(filepath.Join(dir, "find.ini"))
	}
	f.mu.Unlock()
	if st, err := f.openStore(); err == nil {
		last := o
		if findFeedPath(r.SearchIn) || r.WithinRun > 0 || len(r.Selected) > 0 {
			last.SearchIn = nil // a result list or a selection means nothing next time
		}
		_ = st.SetLast(last)
	}
}

func (f *Find) openStore() (*find.Store, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.store != nil {
		return f.store, nil
	}
	dir := configDir()
	if dir == "" {
		return nil, errors.New("Saved searches need a settings folder, and there is none.")
	}
	st, err := find.OpenStore(filepath.Join(dir, "searches.json"))
	if err != nil {
		return nil, errors.New(findErrText(err))
	}
	f.store = st
	return st, nil
}

// Saved lists the names of the saved searches.
func (f *Find) Saved() ([]string, error) {
	st, err := f.openStore()
	if err != nil {
		return nil, err
	}
	return st.Names(), nil
}

// LoadSaved gives the fields of a saved search ("" is the last search).
func (f *Find) LoadSaved(name string) (FindRequest, error) {
	st, err := f.openStore()
	if err != nil {
		return FindRequest{}, err
	}
	if name == "" {
		o, ok := st.Last()
		if !ok {
			return FindRequest{}, errors.New("There is no last search yet.")
		}
		return findRequest(o), nil
	}
	o, ok := st.Get(name)
	if !ok {
		return FindRequest{}, fmt.Errorf("There is no saved search named %q.", name)
	}
	return findRequest(o), nil
}

// Save keeps the dialog's fields as a named search; keepSearchIn also keeps the Search in folders.
func (f *Find) Save(name string, r FindRequest, keepSearchIn bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("Please give the search a name.")
	}
	st, err := f.openStore()
	if err != nil {
		return err
	}
	r.Selected, r.WithinRun = nil, 0
	if findFeedPath(r.SearchIn) {
		r.SearchIn = ""
	}
	if strings.TrimSpace(r.SearchIn) == "" {
		r.SearchIn = "." // (only to pass the check; dropped below)
		keepSearchIn = false
	}
	o, err := f.findOptions(r)
	if err != nil {
		return err
	}
	if _, err := find.Compile(o); err != nil {
		return errors.New(findErrText(err))
	}
	if err := st.Save(name, o, keepSearchIn); err != nil {
		return errors.New(findErrText(err))
	}
	return nil
}

// DeleteSaved removes a saved search.
func (f *Find) DeleteSaved(name string) error {
	st, err := f.openStore()
	if err != nil {
		return err
	}
	return st.Delete(name)
}

// ---- helpers for the dialog ----

// Volumes are the folders for the Search in drive list: all local volumes, then each one.
func (f *Find) Volumes() []string {
	var out []string
	for _, v := range (&FS{app: f.app}).Volumes() {
		out = append(out, v.Path)
	}
	return out
}
