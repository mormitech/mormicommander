package main

// The Lister (F3, FEATURES.md section 27) and the quick view panel (Ctrl+Q): a thin layer over internal/lister. The page
// opens a file and gets a session number; it then asks for screenfuls of rows by byte offset ("the top row starts at
// byte N, give me 40 rows"), which is how a file of many gigabytes opens and scrolls at once. Searching, going to a
// line and following a growing file run here, off the page's thread, and can be stopped (Esc). Images, audio and video
// are read in chunks the page turns into a blob URL, so no local web server is needed. frontend/src/features/viewer.js
// and viewer/view.js are the page's side.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/dirsize"
	"mormicommander/internal/lister"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Lister is what the page calls for the viewer (window.go.main.Lister).
type Lister struct {
	app      *App
	mu       sync.Mutex
	seq      int
	sessions map[int]*listerSession
	sizes    map[string]context.CancelFunc // folder size counts in progress, by key
}

// listerSession is one open file: the Lister window and the quick view panel each have their own.
type listerSession struct {
	mu      sync.Mutex
	path    string
	file    *lister.File
	raw     *lister.Viewer // the file itself
	view    *lister.Viewer // what is shown: raw, or the file converted to text (HTML or RTF as text)
	asText  string         // "", "html" or "rtf"
	opts    lister.Options // the options of raw
	work    context.CancelFunc
	workSeq int
	follow  context.CancelFunc
}

// listerDocLimit caps what is read whole: HTML/RTF converted to text, documents rendered by the page.
const listerDocLimit = 16 << 20

// listerChunkLimit caps one chunk of a media file sent to the page.
const listerChunkLimit = 8 << 20

func init() {
	registerFeature("lister", func(a *App) []any {
		l := &Lister{app: a, sessions: map[int]*listerSession{}, sizes: map[string]context.CancelFunc{}}
		listerInstance = l
		return []any{l}
	})
	onShutdown(func(a *App) {
		if listerInstance != nil {
			listerInstance.closeAll()
		}
	})
}

var listerInstance *Lister

// ListerOptions are the display options the page chooses.
type ListerOptions struct {
	Mode     string `json:"mode"`     // "text", "binary" or "hex"
	Encoding string `json:"encoding"` // "" = detect; an encoding id, or "ansi", "oem", "unicode", "utf-8"
	Wrap     bool   `json:"wrap"`     // text mode: wrap long lines
	Width    int    `json:"width"`    // text mode: wrap column; binary mode: bytes per row
	TabWidth int    `json:"tabWidth"` // text mode: tab stops
	AsText   string `json:"asText"`   // "", "html" or "rtf": show the file converted to text
}

// ListerInfo describes an open file.
type ListerInfo struct {
	ID           int                `json:"id"`
	Path         string             `json:"path"`
	Name         string             `json:"name"`
	Size         int64              `json:"size"`
	MTime        int64              `json:"mtime"`
	Encoding     string             `json:"encoding"`     // the encoding in effect
	EncodingName string             `json:"encodingName"` // its menu text
	Detected     string             `json:"detected"`     // what detection found
	Binary       bool               `json:"binary"`       // the data does not look like text
	Suggest      string             `json:"suggest"`      // how to show it first: text, binary, hex, media, html, markdown, rtf
	Media        *lister.MediaInfo  `json:"media"`        // image/audio/video details (nil for other files)
	OffsetDigits int                `json:"offsetDigits"` // hex digits of offsets in hex mode
	DataStart    int64              `json:"dataStart"`    // first byte after a byte order mark
	ViewSize     int64              `json:"viewSize"`     // size of what is shown (differs from Size for HTML/RTF as text)
	Options      ListerOptionsShown `json:"options"`
}

// ListerOptionsShown are the options in effect, with defaults filled in.
type ListerOptionsShown struct {
	Mode     string `json:"mode"`
	Wrap     bool   `json:"wrap"`
	Width    int    `json:"width"`
	TabWidth int    `json:"tabWidth"`
	AsText   string `json:"asText"`
}

// ListerRow is one screen row, in short field names because a page has many of them.
type ListerRow struct {
	O int64   `json:"o"`           // first byte of the row
	E int64   `json:"e"`           // first byte after the row
	T string  `json:"t"`           // the text to draw (hex mode: the character column)
	M []int32 `json:"m"`           // byte offset (from O) of every character of T
	L bool    `json:"l"`           // the row starts a line
	H string  `json:"h,omitempty"` // hex mode: the hex byte column
}

// ListerPage is a screenful of rows and where it is in the file.
type ListerPage struct {
	Top           int64       `json:"top"`
	Rows          []ListerRow `json:"rows"`
	Size          int64       `json:"size"`
	Percent       float64     `json:"percent"` // position of the end of the page
	AtEnd         bool        `json:"atEnd"`
	Line          int64       `json:"line"`  // line number of the top row (0 when not known quickly)
	Lines         int64       `json:"lines"` // lines found so far
	LinesComplete bool        `json:"linesComplete"`
}

// ListerQuery is one search.
type ListerQuery struct {
	Pattern       string `json:"pattern"`
	Hex           bool   `json:"hex"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"caseSensitive"`
	WholeWords    bool   `json:"wholeWords"`
	Backward      bool   `json:"backward"`
	Encoding      string `json:"encoding"` // "" = the encoding shown; "utf-8" or "unicode" to search the text encoded that way
}

// ListerMatch is what a search found.
type ListerMatch struct {
	Found  bool  `json:"found"`
	Offset int64 `json:"offset"`
	Length int64 `json:"length"`
}

// ListerChange is what a reload found.
type ListerChange struct {
	Changed   bool  `json:"changed"`
	Grown     bool  `json:"grown"`
	Truncated bool  `json:"truncated"`
	Replaced  bool  `json:"replaced"`
	Size      int64 `json:"size"`
}

func listerMode(s string) lister.Mode {
	switch s {
	case "binary":
		return lister.ModeBinary
	case "hex":
		return lister.ModeHex
	}
	return lister.ModeText
}

// listerEncoding resolves the encoding names the page uses ("" = detect).
func listerEncoding(s string) (lister.Encoding, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return "", nil
	case "ansi":
		return lister.DefaultANSI(), nil
	case "oem", "ascii", "dos":
		return lister.DefaultOEM(), nil
	case "unicode", "utf-16", "utf16":
		return lister.UTF16LE, nil
	}
	e, ok := lister.LookupEncoding(s)
	if !ok {
		return "", fmt.Errorf("The encoding %q is not known.", s)
	}
	return e, nil
}

func (o ListerOptions) engine() (lister.Options, error) {
	enc, err := listerEncoding(o.Encoding)
	if err != nil {
		return lister.Options{}, err
	}
	return lister.Options{Mode: listerMode(o.Mode), Encoding: enc, Wrap: o.Wrap, Width: o.Width, TabWidth: o.TabWidth}, nil
}

func (l *Lister) session(id int) (*listerSession, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.sessions[id]
	if s == nil {
		return nil, errors.New("This file is no longer open in the viewer.")
	}
	return s, nil
}

// listerOpenError says in plain words why a file can't be viewed.
func listerOpenError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("The file %s is not there.", path)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("You don't have permission to read %s.", path)
	}
	return fmt.Errorf("Can't open %s: %v", path, err)
}

// Open opens a file for viewing and returns its session; Close ends it.
func (l *Lister) Open(path string, o ListerOptions) (ListerInfo, error) {
	if l.app.mock != nil {
		return ListerInfo{}, errors.New("This is the demo: its files are not real, so they can't be viewed.")
	}
	if strings.Contains(path, "://") {
		return ListerInfo{}, errors.New("Viewing files inside archives or on servers is not ready yet.")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return ListerInfo{}, listerOpenError(path, err)
	}
	if fi.IsDir() {
		return ListerInfo{}, fmt.Errorf("%s is a folder, not a file.", path)
	}
	opt, err := o.engine()
	if err != nil {
		return ListerInfo{}, err
	}
	v, f, err := lister.OpenViewer(path, opt)
	if err != nil {
		return ListerInfo{}, listerOpenError(path, err)
	}
	s := &listerSession{path: path, file: f, raw: v, view: v, opts: opt}
	if err := s.setAsText(o.AsText); err != nil {
		f.Close()
		return ListerInfo{}, err
	}
	l.mu.Lock()
	l.seq++
	id := l.seq
	l.sessions[id] = s
	l.mu.Unlock()
	return s.info(id, true), nil
}

// info describes the session; probe also reads the media header and the suggested view.
func (s *listerSession) info(id int, probe bool) ListerInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	det := s.raw.Detection()
	enc := s.view.Encoding()
	o := s.view.Options()
	info := ListerInfo{
		ID: id, Path: s.path, Name: filepath.Base(s.path), Size: s.raw.Size(), Encoding: string(enc), EncodingName: enc.Info().Name,
		Detected: string(det.Encoding), Binary: det.Binary, OffsetDigits: s.view.OffsetDigits(), DataStart: s.view.DataStart(),
		ViewSize: s.view.Size(),
		Options:  ListerOptionsShown{Mode: o.Mode.String(), Wrap: o.Wrap, Width: o.Width, TabWidth: o.TabWidth, AsText: s.asText},
	}
	if st := s.file.Info(); st != nil {
		info.MTime = st.ModTime().UnixMilli()
	}
	if probe {
		sample := make([]byte, lister.SampleSize)
		n, _ := s.file.ReadAt(sample, 0)
		info.Suggest = viewKindName(lister.SuggestView(s.path, sample[:n]))
		if m, err := lister.ProbeMedia(s.file, s.file.Size()); err == nil {
			info.Media = m
			if info.Suggest != "media" && m.Kind != "" && (m.Kind == "image" || m.Kind == "audio" || m.Kind == "video") && det.Binary {
				info.Suggest = "media"
			}
		}
	}
	return info
}

func viewKindName(k lister.ViewKind) string {
	switch k {
	case lister.ViewBinary:
		return "binary"
	case lister.ViewHex:
		return "hex"
	case lister.ViewMedia:
		return "media"
	case lister.ViewHTML:
		return "html"
	case lister.ViewRTF:
		return "rtf"
	case lister.ViewMarkdown:
		return "markdown"
	}
	return "text"
}

// setAsText switches between the file itself and the file converted to text (HTML or RTF). Callers hold no lock.
func (s *listerSession) setAsText(kind string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if kind != "html" && kind != "rtf" {
		kind = ""
	}
	if kind == s.asText {
		return nil
	}
	if kind == "" {
		s.view, s.asText = s.raw, ""
		return nil
	}
	data, err := s.readAllLocked()
	if err != nil {
		return err
	}
	var text string
	if kind == "html" {
		text = lister.HTMLToText(listerDecodeHTML(data, s.raw.Encoding(), s.opts.Encoding != ""))
	} else if text, err = lister.RTFToText(data); err != nil {
		return errors.New("This file is not an RTF document.")
	}
	opt := s.opts
	opt.Encoding = lister.UTF8
	opt.Mode = lister.ModeText
	v, err := lister.NewViewer(lister.NewBytesSource([]byte(text)), opt)
	if err != nil {
		return err
	}
	s.view, s.asText = v, kind
	return nil
}

// listerDecodeHTML decodes an HTML file: the encoding the user chose, else the one its <meta> names, else the detected one.
func listerDecodeHTML(data []byte, enc lister.Encoding, chosen bool) string {
	if !chosen {
		if e, ok := lister.HTMLCharset(data[:min(len(data), 4096)]); ok {
			enc = e
		}
	}
	if b, n := lister.BOM(data); n > 0 {
		enc, data = b, data[n:]
	}
	return lister.DecodeString(enc, data)
}

func (s *listerSession) readAllLocked() ([]byte, error) {
	size := s.file.Size()
	if size > listerDocLimit {
		return nil, fmt.Errorf("The file is too large to convert here (more than %d MB).", listerDocLimit>>20)
	}
	b := make([]byte, size)
	n, err := s.file.ReadAt(b, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:n], nil
}

// SetOptions changes the mode, encoding, wrap, widths or the converted-text view, and returns the new state.
func (l *Lister) SetOptions(id int, o ListerOptions) (ListerInfo, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerInfo{}, err
	}
	opt, err := o.engine()
	if err != nil {
		return ListerInfo{}, err
	}
	s.mu.Lock()
	encChanged := opt.Encoding != s.opts.Encoding
	s.opts = opt
	s.raw.SetOptions(opt)
	if s.asText != "" {
		if encChanged && s.asText == "html" {
			s.asText = "rebuild" // the HTML is decoded again in the new encoding below
		} else {
			to := opt
			to.Encoding, to.Mode = lister.UTF8, lister.ModeText
			if opt.Mode != lister.ModeText {
				to.Mode = opt.Mode
			}
			s.view.SetOptions(to)
		}
	}
	s.mu.Unlock()
	if err := s.setAsText(o.AsText); err != nil {
		return ListerInfo{}, err
	}
	return s.info(id, false), nil
}

// Move scrolls and returns the page to show. how: "top" (re-align top), "rows" (arg rows down, negative = up), "home",
// "end", "offset" (the row containing byte arg), "percent" (arg %), "reveal" (scroll only if byte arg is not on the page).
func (l *Lister) Move(id int, top int64, how string, arg float64, rows int) (ListerPage, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerPage{}, err
	}
	s.mu.Lock()
	v := s.view
	s.mu.Unlock()
	if rows < 1 {
		rows = 1
	}
	top = max(0, min(top, v.Size()))
	clamp := true
	switch how {
	case "rows":
		if n := int(arg); n > 0 {
			top, err = v.Forward(top, n)
		} else if n < 0 {
			top, err = v.Back(top, -n)
		} else {
			top, err = v.RowStart(top)
		}
	case "home":
		top, err = v.RowStart(0)
	case "end":
		top, err = v.LastPage(rows)
		clamp = false
	case "offset":
		top, err = v.RowStart(max(0, min(int64(arg), v.Size())))
	case "percent":
		top, err = v.RowStart(v.OffsetAtPercent(max(0, min(arg, 100))))
	case "reveal":
		off := max(0, min(int64(arg), v.Size()))
		page, perr := v.Rows(top, rows)
		if perr != nil {
			return ListerPage{}, perr
		}
		visible := len(page) > 0 && off >= page[0].Offset && (off < page[len(page)-1].End || (len(page) < rows && off <= page[len(page)-1].End))
		if !visible {
			top, err = v.RowStart(off)
			if err == nil {
				top, err = v.Back(top, min(3, rows/3)) // a few lines of context above the match, as Total Commander does
			}
		}
	default:
		top, err = v.RowStart(top)
	}
	if err != nil {
		return ListerPage{}, err
	}
	if clamp {
		if last, lerr := v.LastPage(rows); lerr == nil && top > last {
			top = last
		}
	}
	return listerPage(v, top, rows)
}

func listerPage(v *lister.Viewer, top int64, rows int) (ListerPage, error) {
	rs, err := v.Rows(top, rows)
	if err != nil {
		return ListerPage{}, err
	}
	p := ListerPage{Top: top, Size: v.Size(), Rows: make([]ListerRow, len(rs))}
	for i, r := range rs {
		m := r.Map
		if m == nil {
			m = []int32{}
		}
		p.Rows[i] = ListerRow{O: r.Offset, E: r.End, T: r.Text, M: m, L: r.LineStart, H: r.Hex}
	}
	end := top
	if len(rs) > 0 {
		end = rs[len(rs)-1].End
	}
	p.Percent = v.PercentAt(end)
	p.AtEnd, _ = v.AtEnd(top, rows)
	if v.Size() == 0 {
		p.Percent, p.AtEnd = 100, true
	}
	if v.Options().Mode == lister.ModeText {
		// The line number is a nicety: it is only shown when it can be found quickly.
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		if n, err := v.Lines().LineAt(ctx, top); err == nil {
			p.Line = n
		}
		cancel()
		p.Lines, p.LinesComplete = v.Lines().Known()
	}
	return p, nil
}

// startWork cancels the search or go-to of the session that is running and returns the context of a new one.
func (s *listerSession) startWork() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if s.work != nil {
		s.work()
	}
	s.workSeq++
	seq := s.workSeq
	s.work = cancel
	s.mu.Unlock()
	return ctx, func() {
		cancel()
		s.mu.Lock()
		if s.workSeq == seq {
			s.work = nil
		}
		s.mu.Unlock()
	}
}

// Find searches from byte from (forward: at or after it; backward: before it). Progress comes as "lister-progress"
// events {id, pos, size}; Cancel stops it.
func (l *Lister) Find(id int, q ListerQuery, from int64) (ListerMatch, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerMatch{}, err
	}
	s.mu.Lock()
	v := s.view
	s.mu.Unlock()
	enc := v.Encoding()
	if q.Encoding != "" {
		if enc, err = listerEncoding(q.Encoding); err != nil {
			return ListerMatch{}, err
		}
	}
	sr, err := lister.Compile(lister.Query{Pattern: q.Pattern, Hex: q.Hex, Regex: q.Regex, CaseSensitive: q.CaseSensitive,
		WholeWords: q.WholeWords, Backward: q.Backward, Encoding: enc})
	if err != nil {
		if errors.Is(err, lister.ErrEmptyPattern) {
			return ListerMatch{}, errors.New("Type the text to search for.")
		}
		return ListerMatch{}, fmt.Errorf("The search text is not valid: %v", strings.TrimPrefix(err.Error(), "lister: "))
	}
	var last time.Time
	sr.Progress = func(pos, size int64) {
		if time.Since(last) > 120*time.Millisecond {
			last = time.Now()
			l.app.emit("lister-progress", map[string]any{"id": id, "pos": pos, "size": size})
		}
	}
	ctx, done := s.startWork()
	defer done()
	m, found, err := sr.Find(ctx, v.Source(), max(0, min(from, v.Size())), v.DataStart())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return ListerMatch{}, errors.New("Search stopped.")
		}
		return ListerMatch{}, err
	}
	return ListerMatch{Found: found, Offset: m.Offset, Length: m.Length}, nil
}

// Cancel stops the search or go-to-line of a session.
func (l *Lister) Cancel(id int) {
	if s, err := l.session(id); err == nil {
		s.mu.Lock()
		if s.work != nil {
			s.work()
		}
		s.mu.Unlock()
	}
}

// LineOffset is the byte where line n (1-based) starts.
func (l *Lister) LineOffset(id int, n int64) (int64, error) {
	s, err := l.session(id)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 0, errors.New("Line numbers start at 1.")
	}
	s.mu.Lock()
	v := s.view
	s.mu.Unlock()
	ctx, done := s.startWork()
	defer done()
	off, err := v.Lines().LineOffset(ctx, n)
	switch {
	case errors.Is(err, lister.ErrBeyondEnd):
		total, cerr := v.Lines().Count(ctx) // (the scan already reached the end, so this is quick)
		if cerr != nil {
			return 0, fmt.Errorf("Line %d is past the end of the file.", n)
		}
		return 0, fmt.Errorf("The file has only %d lines.", total)
	case errors.Is(err, context.Canceled):
		return 0, errors.New("Stopped.")
	}
	return off, err
}

// Text is the selection [start, end) as text in the encoding shown (asHex: as hex pairs).
func (l *Lister) Text(id int, start, end int64, asHex bool) (string, error) {
	s, err := l.session(id)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	v := s.view
	s.mu.Unlock()
	var t string
	if asHex {
		t, err = v.HexText(start, end, 0)
	} else {
		t, err = v.Text(start, end, 0)
	}
	if errors.Is(err, lister.ErrTooLarge) {
		return "", errors.New("The selection is too large to copy.")
	}
	return t, err
}

// Copy puts the selection [start, end) on the clipboard, as text or as hex pairs, and returns its length in bytes.
func (l *Lister) Copy(id int, start, end int64, asHex bool) (int64, error) {
	t, err := l.Text(id, start, end, asHex)
	if err != nil {
		return 0, err
	}
	if l.app.ctx == nil {
		return 0, errors.New("There is no window to copy from.")
	}
	if err := wruntime.ClipboardSetText(l.app.ctx, t); err != nil {
		return 0, fmt.Errorf("Can't copy to the clipboard: %v", err)
	}
	return end - start, nil
}

// SaveRange writes the bytes [start, end) of what is shown to a new file (Save selection as / Save as). An existing
// file is replaced only when overwrite is true.
func (l *Lister) SaveRange(id int, start, end int64, dest string, overwrite bool) (int64, error) {
	s, err := l.session(id)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	v := s.view
	s.mu.Unlock()
	if same, _ := listerSameFile(dest, s.path); same {
		return 0, errors.New("A file can't be saved over itself.")
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(dest, flags, 0o666)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return 0, fmt.Errorf("%s is already there.", dest)
		}
		return 0, fmt.Errorf("Can't create %s: %v", dest, err)
	}
	ctx, done := s.startWork()
	defer done()
	n, err := v.CopyRange(ctx, f, max(0, start), min(end, v.Size()))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dest)
		return 0, fmt.Errorf("Can't save %s: %v", dest, err)
	}
	return n, nil
}

func listerSameFile(a, b string) (bool, error) {
	fa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(fa, fb), nil
}

// Reload reads the file again from disk (F2) and says what changed.
func (l *Lister) Reload(id int) (ListerChange, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerChange{}, err
	}
	c, err := s.raw.Reload()
	if err != nil {
		return ListerChange{}, listerOpenError(s.path, err)
	}
	if c.Changed() {
		s.rebuildText()
	}
	return ListerChange{Changed: c.Changed(), Grown: c.Grown, Truncated: c.Truncated, Replaced: c.Replaced, Size: c.NewSize}, nil
}

// rebuildText converts the file to text again after it changed on disk.
func (s *listerSession) rebuildText() {
	s.mu.Lock()
	kind := s.asText
	if kind != "" {
		s.asText = "rebuild"
	}
	s.mu.Unlock()
	if kind != "" {
		if err := s.setAsText(kind); err != nil {
			s.mu.Lock()
			s.view, s.asText = s.raw, ""
			s.mu.Unlock()
		}
	}
}

// Follow turns "follow the file" on or off: while on, the file is checked twice a second and every change comes as a
// "lister-change" event {id, grown, truncated, replaced, size}.
func (l *Lister) Follow(id int, on bool) error {
	s, err := l.session(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.follow != nil {
		s.follow()
		s.follow = nil
	}
	if !on {
		s.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.follow = cancel
	raw := s.raw
	s.mu.Unlock()
	go raw.FollowTail(ctx, 0, func(c lister.Change) {
		s.rebuildText()
		l.app.emit("lister-change", map[string]any{"id": id, "grown": c.Grown, "truncated": c.Truncated, "replaced": c.Replaced, "size": c.NewSize})
	}, nil)
	return nil
}

// Media reads the image, audio or video details of the file (nil error and nil info for other files).
func (l *Lister) Media(id int) (*lister.MediaInfo, error) {
	s, err := l.session(id)
	if err != nil {
		return nil, err
	}
	m, err := lister.ProbeMedia(s.file, s.file.Size())
	if errors.Is(err, lister.ErrUnknownMedia) {
		return nil, nil
	}
	return m, err
}

// Chunk reads up to n bytes of the file from off (the page builds images and videos from these).
func (l *Lister) Chunk(id int, off int64, n int) ([]byte, error) {
	s, err := l.session(id)
	if err != nil {
		return nil, err
	}
	n = max(0, min(n, listerChunkLimit))
	b := make([]byte, n)
	got, err := s.file.ReadAt(b, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:got], nil
}

// Document is the whole file decoded as text, for the rendered HTML and Markdown views.
func (l *Lister) Document(id int) (string, error) {
	s, err := l.session(id)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readAllLocked()
	if err != nil {
		return "", err
	}
	return listerDecodeHTML(data, s.raw.Encoding(), s.opts.Encoding != ""), nil
}

// Encodings lists the encodings of the Encoding menu.
func (l *Lister) Encodings() []lister.EncodingInfo { return lister.Encodings() }

// Close ends a session.
func (l *Lister) Close(id int) {
	l.mu.Lock()
	s := l.sessions[id]
	delete(l.sessions, id)
	l.mu.Unlock()
	if s != nil {
		s.close()
	}
}

func (s *listerSession) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.work != nil {
		s.work()
	}
	if s.follow != nil {
		s.follow()
	}
	s.file.Close()
}

func (l *Lister) closeAll() {
	l.mu.Lock()
	list := l.sessions
	l.sessions = map[int]*listerSession{}
	sizes := l.sizes
	l.sizes = map[string]context.CancelFunc{}
	l.mu.Unlock()
	for _, s := range list {
		s.close()
	}
	for _, c := range sizes {
		c()
	}
}

// Sessions is the number of open sessions (control mode checks that closing the viewer frees the file).
func (l *Lister) Sessions() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.sessions)
}

// FolderSize counts a folder (F3 on a folder, the folder line of quick view). key names the caller: a new count for
// the same key stops the one before.
func (l *Lister) FolderSize(key, path string) (dirsize.Result, error) {
	if l.app.mock != nil {
		return dirsize.Result{}, errors.New("This is the demo: its folders are not real, so they can't be counted.")
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.mu.Lock()
	if old := l.sizes[key]; old != nil {
		old()
	}
	l.sizes[key] = cancel
	l.mu.Unlock()
	defer cancel()
	r, err := dirsize.Calculate(ctx, path, dirsize.Options{})
	if err != nil && ctx.Err() != nil {
		return r, errors.New("Stopped counting.")
	}
	if err != nil {
		return r, listerOpenError(path, err)
	}
	return r, nil
}

// CancelFolderSize stops a folder count.
func (l *Lister) CancelFolderSize(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c := l.sizes[key]; c != nil {
		c()
		delete(l.sizes, key)
	}
}
