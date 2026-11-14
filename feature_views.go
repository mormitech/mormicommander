package main

// The panel views beyond the plain listing (FEATURES.md sections 4, 5, 32 and 33): the cells of the Full, Comments and
// custom column views rendered by internal/listing (size and date formats, column templates such as "[=tc.size.k]",
// sorting by a custom column), descript.ion comments (Ctrl+Z), and thumbnails from internal/thumbs, generated in the
// background and sent to the page as "views-thumb" events. frontend/src/features/views.js is the page's side.
//
// The panel itself still lists folders through FS.List; this object only adds what a view needs on top, so a slow
// field (image sizes, EXIF dates) never holds up showing a folder.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/config"
	lst "mormicommander/internal/listing"
	"mormicommander/internal/thumbs"
	"mormicommander/internal/vfs"
)

// Views is what the page calls for column views, comments and thumbnails (window.go.main.Views).
type Views struct {
	app *App

	mu      sync.Mutex
	tn      *thumbs.Thumbnailer
	pool    *thumbs.Pool
	batches map[string]*viewsBatch // the thumbnail batch per panel ("left", "right")
	seq     int
}

type viewsBatch struct {
	gen int
	b   *thumbs.Batch
	dir string
}

func init() {
	registerFeature("views", func(a *App) []exposed { return []exposed{expose(newViews(a))} })
	onShutdown(func(a *App) {
		if v := viewsOf(a); v != nil {
			v.close()
		}
	})
}

// (one Views per App; kept here so the shutdown hook finds it without a field in App)
var (
	viewsMu  sync.Mutex
	viewsAll = map[*App]*Views{}
)

func newViews(a *App) *Views {
	v := &Views{app: a, batches: map[string]*viewsBatch{}}
	viewsMu.Lock()
	viewsAll[a] = v
	viewsMu.Unlock()
	return v
}

func viewsOf(a *App) *Views {
	viewsMu.Lock()
	defer viewsMu.Unlock()
	return viewsAll[a]
}

func (v *Views) close() {
	v.mu.Lock()
	for k, b := range v.batches {
		b.b.Cancel()
		delete(v.batches, k)
	}
	p := v.pool
	v.pool = nil
	v.mu.Unlock()
	if p != nil {
		p.Close()
	}
}

// ---- column views ----

// ViewsFormat is how sizes, dates and attributes look (Configuration > Display, F-093, F-094).
type ViewsFormat struct {
	Size string `json:"size"` // "bytes", "k", "M", "G", "dynamic" (lst.ParseSizeUnit names)
	Date string `json:"date"` // a date picture such as "dd.MM.yyyy HH:mm"; "" is the default
	Attr string `json:"attr"` // "", "win", "winfull", "unix", "octal"
	// NoThousands leaves the thousands separator out of sizes (Configuration > Display, issue #93); false keeps it.
	NoThousands bool `json:"noThousands,omitempty"`
}

// ViewsColumn is one column of a column set as the page stores it (settings "views" → columnSets).
type ViewsColumn struct {
	Title    string `json:"title"`
	Template string `json:"template"`
	Width    int    `json:"width"`
	Align    string `json:"align"` // "left", "right", "center"
}

// ViewsCellsRequest asks for the cells of a folder in a column view.
type ViewsCellsRequest struct {
	Path     string           `json:"path"`
	Columns  []ViewsColumn    `json:"columns"`  // the columns after the name (the name column is drawn by the page)
	Format   ViewsFormat      `json:"format"`   // size, date and attribute formats
	DirSizes map[string]int64 `json:"dirSizes"` // folder sizes calculated so far (Space, Alt+Shift+Enter)
	SortCol  int              `json:"sortCol"`  // >= 0: also return the order of the entries sorted by this column
	Desc     bool             `json:"desc"`     // with SortCol: largest / last first
	Natural  bool             `json:"natural"`  // with SortCol: natural name order for ties
	// SortMethod and SortLocale are the panel's name order (core/sort.js): "" the default, "case" upper and lower case
	// apart, "code" by character code; SortLocale a language such as "hu" for its alphabet (empty: none).
	SortMethod string `json:"sortMethod"`
	SortLocale string `json:"sortLocale"`
}

// viewsCollation turns the request's sort settings into the listing's name collation. With no method and no language
// it is the collation the custom-column sort always had.
func viewsCollation(r ViewsCellsRequest) lst.Collation {
	c := lst.Collation{Natural: r.Natural}
	switch r.SortMethod {
	case "case":
		c.CaseSensitive = true
		c.Locale = r.SortLocale
	case "code":
		// code point order with case ignored: the zero Collation, no language rules
	default:
		c.Locale = r.SortLocale
	}
	return c
}

// ViewsCells is the answer: a row of cells per entry name, and the sorted order when asked.
type ViewsCells struct {
	Path   string              `json:"path"`
	Titles []string            `json:"titles"`
	Cells  map[string][]string `json:"cells"`
	Order  []string            `json:"order"` // names in the requested column's order (folders first), or nil
	Local  bool                `json:"local"` // the folder is on this computer (comments can be written)
}

var viewsAttrStyles = map[string]lst.AttrStyle{
	"": lst.AttrStyleAuto, "win": lst.AttrStyleWindows, "winfull": lst.AttrStyleWindowsFull,
	"unix": lst.AttrStyleUnix, "octal": lst.AttrStyleOctal,
}

func (f ViewsFormat) toListing() lst.Format {
	unit, _ := lst.ParseSizeUnit(f.Size)
	out := lst.Format{Size: unit, Date: f.Date, AttrStyle: viewsAttrStyles[f.Attr]}
	if f.NoThousands {
		out.Number = lst.NumberFormat{Thousands: "", Decimal: lst.DefaultNumberFormat.Decimal}
	}
	return out
}

func (c ViewsColumn) toListing() lst.Column {
	al := lst.AlignLeft
	switch c.Align {
	case "right":
		al = lst.AlignRight
	case "center":
		al = lst.AlignCenter
	}
	return lst.Column{Title: c.Title, Template: c.Template, Width: c.Width, Align: al}
}

// viewsLocalDir says whether p is a folder on this computer that the listing engine can read directly (not the demo,
// not an archive or a network URL).
func (v *Views) viewsLocalDir(p string) bool {
	if v.app.mock != nil || strings.Contains(p, "://") || !filepath.IsAbs(p) {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// viewsItems reads a folder as listing items: straight from the disk for local folders (with owners, creation and
// access dates), through the panel's file system for the rest.
func (v *Views) viewsItems(p string) ([]lst.Item, bool, error) {
	if v.viewsLocalDir(p) {
		items, err := lst.ReadDir(p)
		if err != nil {
			return nil, true, errors.New(listError(p, err))
		}
		return items, true, nil
	}
	fsys := vfs.For(p, v.app.fs)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	l, err := fsys.List(ctx, p)
	if err != nil {
		return nil, false, errors.New(listError(fsys.Clean(p), err))
	}
	items := make([]lst.Item, len(l.Entries))
	for i, e := range l.Entries {
		items[i] = viewsItemFromEntry(l.Path, e, i)
	}
	return items, false, nil
}

// viewsItemFromEntry turns a vfs entry into a listing item.
func viewsItemFromEntry(dir string, e vfs.Entry, order int) lst.Item {
	it := lst.Item{Name: e.Name, Size: e.Size, DirSize: -1, Mode: os.FileMode(e.Mode & 0o777), Dir: dir, Order: order}
	if e.MTime != 0 {
		it.ModTime = time.UnixMilli(e.MTime)
	}
	flags := map[int]lst.Attr{
		vfs.FlagDir: lst.AttrDir, vfs.FlagLink: lst.AttrLink, vfs.FlagHidden: lst.AttrHidden, vfs.FlagExec: lst.AttrExec,
		vfs.FlagBroken: lst.AttrBroken, vfs.FlagSystem: lst.AttrSystem, vfs.FlagReadOnly: lst.AttrReadOnly,
	}
	for f, a := range flags {
		if e.Flags&f != 0 {
			it.Attr |= a
		}
	}
	if it.IsDir() {
		it.Size = 0
		it.Mode |= os.ModeDir
	}
	return it
}

func viewsNeedsComments(cols []ViewsColumn) bool {
	for _, c := range cols {
		if strings.Contains(strings.ToLower(c.Template), "tc.comment") {
			return true
		}
	}
	return false
}

// Cells renders the columns of a column view (Full with a size or date format, Comments, a custom column set) for
// every entry of a folder, and the order of a sort by one of those columns.
func (v *Views) Cells(r ViewsCellsRequest) (ViewsCells, error) {
	set := lst.ColumnSet{Name: "view"}
	for _, c := range r.Columns {
		set.Columns = append(set.Columns, c.toListing())
	}
	cs, err := set.Compile()
	if err != nil {
		return ViewsCells{}, fmt.Errorf("A column can't be shown: %v", strings.TrimPrefix(err.Error(), "listing: "))
	}
	items, local, err := v.viewsItems(r.Path)
	if err != nil {
		return ViewsCells{}, err
	}
	for i := range items {
		if s, ok := r.DirSizes[items[i].Name]; ok && items[i].IsDir() {
			items[i].DirSize = s
		}
	}
	var comments *lst.Comments
	if local && viewsNeedsComments(r.Columns) {
		comments, _ = lst.LoadComments(r.Path) // a missing or unreadable descript.ion just means no comments
	}
	opt := lst.Options{
		Mode:      lst.ViewCustom,
		Custom:    cs,
		Format:    r.Format.toListing(),
		Collation: viewsCollation(r),
		Sort:      lst.DefaultSort,
		Filter:    lst.FilterOptions{ShowHidden: true, ShowSystem: true, Dir: r.Path},
		Comments:  comments,
	}
	if r.SortCol >= 0 && r.SortCol < len(cs.Columns) {
		opt.Sort = lst.SortSpec{Keys: []lst.SortKey{{By: lst.ByColumn, Column: r.SortCol, Desc: r.Desc}}, DirsByName: true}
	}
	view := lst.Build(items, opt)
	out := ViewsCells{Path: r.Path, Titles: view.Titles, Cells: make(map[string][]string, len(view.Rows)), Local: local}
	for _, row := range view.Rows {
		out.Cells[row.Item.Name] = row.Cells
	}
	if r.SortCol >= 0 && r.SortCol < len(cs.Columns) {
		out.Order = make([]string, len(view.Rows))
		for i, row := range view.Rows {
			out.Order[i] = row.Item.Name
		}
	}
	return out, nil
}

// ViewsField is one field for the column editor's field chooser.
type ViewsField struct {
	Name  string   `json:"name"` // "tc.size"
	Help  string   `json:"help"`
	Units []string `json:"units"`
}

// Fields lists every field a column template can use ([=tc.size.k], [=img.width], ...).
func (v *Views) Fields() []ViewsField {
	var out []ViewsField
	for _, n := range lst.FieldNames() {
		f, ok := lst.LookupField(n)
		if !ok {
			continue
		}
		out = append(out, ViewsField{Name: n, Help: f.Help, Units: f.Units})
	}
	return out
}

// CheckColumns says what is wrong with a column set ("" when every template can be shown).
func (v *Views) CheckColumns(cols []ViewsColumn) string {
	for _, c := range cols {
		if strings.TrimSpace(c.Template) == "" {
			return fmt.Sprintf("The column %q has no content.", c.Title)
		}
		if _, err := c.toListing().Compile(); err != nil {
			return fmt.Sprintf("The column %q can't be shown: %v.", c.Title, strings.TrimPrefix(err.Error(), "listing: "))
		}
	}
	return ""
}

// AutoColumnSet is the index of the first folder mask that matches folder (masks like "Pictures;*/photos/*", F-668),
// or -1 when none does: the column set with that mask is chosen automatically for the folder.
func (v *Views) AutoColumnSet(folder string, masks []string) int {
	sets := make([]config.ColumnSet, len(masks))
	for i, m := range masks {
		sets[i].AutoSwitch = m
	}
	return config.AutoColumnSet(sets, folder)
}

// ---- comments (descript.ion) ----

func (v *Views) viewsCommentDir(dir string) error {
	if !v.viewsLocalDir(dir) {
		return errors.New("Comments can only be kept in folders on this computer.")
	}
	return nil
}

// Comment is the descript.ion comment of a file ("" when it has none).
func (v *Views) Comment(dir, name string) (string, error) {
	if err := v.viewsCommentDir(dir); err != nil {
		return "", err
	}
	c, err := lst.LoadComments(dir)
	if err != nil {
		return "", fmt.Errorf("Can't read the comments of %s: %v", dir, err)
	}
	return c.Get(name), nil
}

// SetComment changes the comment of a file in descript.ion; an empty comment removes it (and the file when it is
// left empty).
func (v *Views) SetComment(dir, name, text string) error {
	if err := v.viewsCommentDir(dir); err != nil {
		return err
	}
	if name == "" || name == ".." || strings.ContainsAny(name, `/\`) {
		return errors.New("Choose a file or folder to comment on.")
	}
	c, err := lst.LoadComments(dir)
	if err != nil {
		return fmt.Errorf("Can't read the comments of %s: %v", dir, err)
	}
	c.Set(name, strings.TrimSpace(text))
	if err := c.Save(dir); err != nil {
		return fmt.Errorf("Can't save the comment: %v", err)
	}
	return nil
}

// ---- thumbnails ----

// viewsThumbDir is where thumbnails are cached: in the settings folder (so a test run with its own settings folder
// never touches the user's cache), else the system's cache folder.
func viewsThumbDir() string {
	if d := configDir(); d != "" {
		return filepath.Join(d, "thumbs")
	}
	d, _ := thumbs.DefaultCacheDir()
	return d
}

// thumbnailer starts the generator and its pool the first time thumbnails are needed.
func (v *Views) thumbnailer() (*thumbs.Thumbnailer, *thumbs.Pool) {
	if v.tn == nil {
		var cache *thumbs.Cache
		if d := viewsThumbDir(); d != "" && v.app.mock == nil {
			cache, _ = thumbs.OpenCache(d, 0) // no cache only means thumbnails are made again next time
		}
		v.tn = thumbs.New(thumbs.DefaultOptions(), cache)
		v.pool = thumbs.NewPool(v.tn, 0)
	}
	return v.tn, v.pool
}

// ViewsThumb is one finished thumbnail (event "views-thumb").
type ViewsThumb struct {
	Key    string `json:"key"`
	Gen    int    `json:"gen"`
	Name   string `json:"name"`
	URL    string `json:"url"`   // data: URL of the picture, "" when there is none (the page keeps the icon)
	Width  int    `json:"width"` // size of the original picture, 0 when unknown
	Height int    `json:"height"`
}

// viewsThumbKinds are the files worth asking for a picture; the rest get their icon at once.
var viewsThumbKinds = map[thumbs.Kind]bool{thumbs.KindImage: true, thumbs.KindVideo: true, thumbs.KindPDF: true}

// Thumbs starts making thumbnails of the named files of a folder for a panel (key) at size px; each one arrives as a
// "views-thumb" event with the returned generation number. A new call for the same panel stops the one before.
func (v *Views) Thumbs(key, dir string, names []string, size int) int {
	v.mu.Lock()
	defer v.mu.Unlock()
	if old := v.batches[key]; old != nil {
		old.b.Cancel()
		delete(v.batches, key)
	}
	v.seq++
	gen := v.seq
	if v.app.mock != nil || !v.viewsLocalDir(dir) {
		return gen // demo files and files in archives or on servers show their icons
	}
	tn, pool := v.thumbnailer()
	size = thumbs.ClampSize(size)
	if o := tn.Options(); o.Width != size || o.Height != size {
		tn.SetSize(size, size)
	}
	var paths []string
	for _, n := range names {
		if n == "" || n == ".." || strings.ContainsAny(n, `/\`) || !viewsThumbKinds[thumbs.KindOf(n)] {
			continue
		}
		paths = append(paths, filepath.Join(dir, n))
	}
	if len(paths) == 0 {
		return gen
	}
	b := pool.Submit(context.Background(), paths, func(r thumbs.Result) {
		t := ViewsThumb{Key: key, Gen: gen, Name: filepath.Base(r.Path)}
		if r.Err == nil && r.Thumb != nil {
			t.URL = r.Thumb.DataURL()
			t.Width, t.Height = r.Thumb.Meta.Width, r.Thumb.Meta.Height
		}
		v.app.emit("views-thumb", t)
	})
	v.batches[key] = &viewsBatch{gen: gen, b: b, dir: dir}
	return gen
}

// ThumbsFirst moves the named files (the ones on screen) to the front of a panel's thumbnail queue.
func (v *Views) ThumbsFirst(key string, names []string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	b := v.batches[key]
	if b == nil {
		return
	}
	paths := make([]string, 0, len(names))
	for _, n := range names {
		paths = append(paths, filepath.Join(b.dir, n))
	}
	b.b.Prioritize(paths...)
}

// ThumbsStop stops making thumbnails for a panel (it left the thumbnails view).
func (v *Views) ThumbsStop(key string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if b := v.batches[key]; b != nil {
		b.b.Cancel()
		delete(v.batches, key)
	}
}

// ViewsIcons are the file-type icons for the thumbnails view: an icon id per name, and an SVG data: URL per id.
type ViewsIcons struct {
	Names map[string]string `json:"names"`
	SVG   map[string]string `json:"svg"`
}

// Icons gives the file-type icon of each name (dirs[i] tells whether names[i] is a folder).
func (v *Views) Icons(names []string, dirs []bool, size int) ViewsIcons {
	out := ViewsIcons{Names: make(map[string]string, len(names)), SVG: map[string]string{}}
	size = thumbs.ClampSize(size)
	for i, n := range names {
		ic := thumbs.IconFor(n, i < len(dirs) && dirs[i])
		id := string(ic.Kind) + ":" + ic.Label
		out.Names[n] = id
		if _, ok := out.SVG[id]; !ok {
			out.SVG[id] = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(ic.SVG(size))
		}
	}
	return out
}

// ViewsCacheInfo describes the thumbnail cache for the thumbnails settings.
type ViewsCacheInfo struct {
	Dir     string `json:"dir"`
	Entries int    `json:"entries"`
	Bytes   int64  `json:"bytes"`
}

// ThumbCache tells where the thumbnail cache is and how big it is.
func (v *Views) ThumbCache() ViewsCacheInfo {
	v.mu.Lock()
	tn, _ := v.thumbnailer()
	v.mu.Unlock()
	info := ViewsCacheInfo{Dir: viewsThumbDir()}
	if c := tn.Cache(); c != nil {
		if s, err := c.Stats(); err == nil {
			info.Entries, info.Bytes = s.Entries, s.Bytes
		}
	}
	return info
}

// ClearThumbCache deletes every cached thumbnail ("Delete thumbnail cache").
func (v *Views) ClearThumbCache() error {
	v.mu.Lock()
	tn, _ := v.thumbnailer()
	v.mu.Unlock()
	c := tn.Cache()
	if c == nil {
		return nil
	}
	if err := c.Clear(); err != nil {
		return fmt.Errorf("Can't delete the thumbnail cache: %v", err)
	}
	return nil
}
