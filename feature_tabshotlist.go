package main

// Folder tabs, the directory hotlist and the folder histories (FEATURES.md sections 2 and 31). The page owns the tab
// bar, the popups and the dialogs (frontend/src/features/tabs.js and hotlist.js); this file keeps what has to last:
// the hotlist tree, the tab sets of both panels, saved .tab files, favourite tab sets, the folder history of each side
// and the "frequently used folders" counts. All of it goes through the settings engine in internal/config and is
// stored the way Total Commander stores it in wincmd.ini ([DirMenu], [lefttabs], [righttabs], [LeftHistory],
// [RightHistory], [FrequentDirs], [FavoriteTab <name>]), in a file of its own, folders.ini in the settings folder,
// so a hotlist can be copied from or to a real wincmd.ini by hand, and the Configuration feature can later move these
// sections into its own wincmd.ini without changing the page.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"mormicommander/internal/config"
	"mormicommander/internal/launch"
)

// TabsHotlist is what the page calls for tabs, the hotlist and the histories (window.go.main.TabsHotlist).
type TabsHotlist struct {
	app *App

	mu        sync.Mutex
	path      string // folders.ini, "" when there is no settings folder (nothing is saved then)
	file      *config.File
	hot       *config.Hotlist
	freq      *config.Frequent
	saveTimer *time.Timer
}

// THItem is one hotlist entry as the page sees it: an entry ("cmd"), a submenu ("menu") or a separator ("sep").
type THItem struct {
	Kind   string   `json:"kind"`
	Title  string   `json:"title"`           // with TC's "&" before the hotkey letter
	Label  string   `json:"label,omitempty"` // the title as shown (no "&"); filled by Hotlist, ignored by SetHotlist
	Key    string   `json:"key,omitempty"`   // the hotkey letter, lower case; filled by Hotlist
	Cmd    string   `json:"cmd,omitempty"`
	Target string   `json:"target,omitempty"`
	Items  []THItem `json:"items,omitempty"`
}

// THAction is what running a hotlist entry does, decoded and with its variables expanded.
type THAction struct {
	Kind    string `json:"kind"` // "cd", "tabs", "internal", "user", "program" or "" (nothing)
	Path    string `json:"path"`
	Filter  string `json:"filter"`
	Target  string `json:"target"`
	Command string `json:"command"`
	Args    string `json:"args"`
}

// THMatch is one result of a hotlist search.
type THMatch struct {
	Title      string `json:"title"`
	Breadcrumb string `json:"breadcrumb"`
	Cmd        string `json:"cmd"`
	Target     string `json:"target"`
}

// THTab is one folder tab: its folder, custom caption, lock mode (0 not locked, 1 locked, 2 locked but folder
// changes allowed), sort and view (issue #128: "" = none saved, the tab shows the panel's view; Set is the custom
// column set of the "custom" view).
type THTab struct {
	Path    string `json:"path"`
	Caption string `json:"caption"`
	Lock    int    `json:"lock"`
	Sort    string `json:"sort"`
	Desc    bool   `json:"desc"`
	View    string `json:"view,omitempty"`
	Set     string `json:"set,omitempty"`
}

// THTabSet is the tabs of one panel and which one is active.
type THTabSet struct {
	Tabs   []THTab `json:"tabs"`
	Active int     `json:"active"`
}

// THTabs is both panels' tabs and the folder history of each side's active tab (newest first).
type THTabs struct {
	Left         THTabSet `json:"left"`
	Right        THTabSet `json:"right"`
	LeftHistory  []string `json:"leftHistory"`
	RightHistory []string `json:"rightHistory"`
	HistoryMax   int      `json:"historyMax"`
}

// THTabFile is the content of a .tab file: the tabs of the panel that was active and of the other one.
type THTabFile struct {
	Active   THTabSet `json:"active"`
	Inactive THTabSet `json:"inactive"`
}

// THFavorite is a named pair of tab sets (favourite tabs).
type THFavorite struct {
	Name  string   `json:"name"`
	Left  THTabSet `json:"left"`
	Right THTabSet `json:"right"`
}

var thInstance *TabsHotlist

func init() {
	registerFeature("tabshotlist", func(a *App) []exposed {
		thInstance = thOpen(a, thFilePath())
		return []exposed{expose(thInstance)}
	})
	onShutdown(func(a *App) {
		if thInstance != nil {
			thInstance.flush()
		}
	})
}

func thFilePath() string {
	if dir := configDir(); dir != "" {
		return filepath.Join(dir, "folders.ini")
	}
	return ""
}

// thOpen reads folders.ini; a missing or broken file starts empty rather than stopping the app.
func thOpen(a *App, path string) *TabsHotlist {
	t := &TabsHotlist{app: a, path: path, file: config.NewFile(), freq: config.NewFrequent()}
	if path != "" {
		if f, err := config.LoadFile(path); err == nil {
			t.file = f
		}
	}
	t.hot = config.LoadHotlist(t.file)
	t.freq.Load(t.file, config.SectionFrequentDirs)
	return t
}

// save writes the file now (the caller holds mu).
func (t *TabsHotlist) save() error {
	if t.saveTimer != nil {
		t.saveTimer.Stop()
		t.saveTimer = nil
	}
	if t.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return fmt.Errorf("The settings folder could not be made: %v", err)
	}
	if err := t.file.Save(t.path); err != nil {
		return fmt.Errorf("The folder settings could not be saved: %v", err)
	}
	return nil
}

// saveSoon writes the file a moment later, so a burst of folder visits is one write (the caller holds mu).
func (t *TabsHotlist) saveSoon() {
	if t.saveTimer != nil || t.path == "" {
		return
	}
	t.saveTimer = time.AfterFunc(3*time.Second, func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.saveTimer = nil
		t.save()
	})
}

func (t *TabsHotlist) flush() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.saveTimer != nil {
		t.save()
	}
}

func (t *TabsHotlist) vars() config.Vars { return config.DefaultVars(configDir(), t.path) }

// ---- the hotlist ----

func thFromItems(items []*config.MenuItem) []THItem {
	out := make([]THItem, 0, len(items))
	for _, it := range items {
		switch it.Kind {
		case config.MenuSeparator:
			out = append(out, THItem{Kind: "sep"})
		case config.MenuSubmenu:
			out = append(out, THItem{Kind: "menu", Title: it.Title, Label: config.StripAccelerator(it.Title), Key: thKey(it.Title), Items: thFromItems(it.Children)})
		default:
			out = append(out, THItem{Kind: "cmd", Title: it.Title, Label: config.StripAccelerator(it.Title), Key: thKey(it.Title), Cmd: it.Field("cmd"), Target: it.Field("path")})
		}
	}
	return out
}

// thKey is the "&" hotkey letter of a title, or "".
func thKey(title string) string {
	if r := config.Accelerator(title); r != 0 {
		return string(r)
	}
	return ""
}

func thToItems(items []THItem) []*config.MenuItem {
	out := make([]*config.MenuItem, 0, len(items))
	for _, it := range items {
		switch it.Kind {
		case "sep":
			out = append(out, config.NewSeparator())
		case "menu":
			m := config.NewHotlistSubmenu(thOneLine(it.Title))
			m.Children = thToItems(it.Items)
			out = append(out, m)
		default:
			m := &config.MenuItem{Kind: config.MenuCommand, Title: thOneLine(it.Title)}
			m.SetField("cmd", thOneLine(it.Cmd))
			m.SetField("path", thOneLine(it.Target))
			out = append(out, m)
		}
	}
	return out
}

// thOneLine keeps a value on one line (a line break could not be stored in the ini file).
func thOneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(s))
}

// Hotlist returns the whole hotlist tree.
func (t *TabsHotlist) Hotlist() []THItem {
	t.mu.Lock()
	defer t.mu.Unlock()
	return thFromItems(t.hot.Items)
}

// SetHotlist replaces the hotlist with the tree the Configure dialog made, and saves it.
func (t *TabsHotlist) SetHotlist(items []THItem) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hot.Items = thToItems(items)
	t.hot.Save(t.file)
	return t.save()
}

// AddFolder adds a folder at the end of the hotlist (title "" uses the folder's name); target is the other panel's
// folder for an entry that changes both panels, or "".
func (t *TabsHotlist) AddFolder(title, folder, target string) error {
	if strings.TrimSpace(folder) == "" {
		return errors.New("There is no folder to add.")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.hot.AddFolder(thOneLine(title), thOneLine(folder), thOneLine(target))
	t.hot.Save(t.file)
	return t.save()
}

// AddCommand adds an entry with any command (cd <folder>, cd <file.tab>, cm_…, a program) at the end of the hotlist.
func (t *TabsHotlist) AddCommand(title, cmd string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	it := &config.MenuItem{Kind: config.MenuCommand, Title: thOneLine(title)}
	it.SetField("cmd", thOneLine(cmd))
	if _, err := t.hot.Insert(nil, -1, it); err != nil {
		return err
	}
	t.hot.Save(t.file)
	return t.save()
}

// thOpens says whether a hotlist entry opens folder (the engine's own path comparison: case matters on Linux only).
func thOpens(it *config.MenuItem, folder string) bool {
	if it.Kind != config.MenuCommand {
		return false
	}
	one := &config.Hotlist{Items: []*config.MenuItem{it}}
	return one.Contains(folder)
}

// Contains says whether some entry of the hotlist opens the folder.
func (t *TabsHotlist) Contains(folder string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.hot.Contains(folder)
}

// RemoveFolder removes the first entry (at any depth) that opens the folder; false when there was none.
func (t *TabsHotlist) RemoveFolder(folder string) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	var found config.MenuPath
	config.MenuWalk(t.hot.Items, func(p config.MenuPath, _ []string, it *config.MenuItem) {
		if found == nil && thOpens(it, folder) {
			found = append(config.MenuPath(nil), p...)
		}
	})
	if found == nil {
		return false, nil
	}
	if _, err := t.hot.Remove(found); err != nil {
		return false, err
	}
	t.hot.Save(t.file)
	return true, t.save()
}

// Resolve decodes an entry's command and target folder (cd, .tab files, cm_*, em_*, programs) with $HOME, %VAR%,
// ${VAR}, ~ and %COMMANDER_PATH% expanded.
func (t *TabsHotlist) Resolve(cmd, target string) THAction {
	it := &config.MenuItem{Kind: config.MenuCommand}
	it.SetField("cmd", cmd)
	it.SetField("path", target)
	a := (&config.Hotlist{}).Action(it, t.vars())
	kind := map[config.ActionKind]string{config.ActionChangeDir: "cd", config.ActionTabSet: "tabs", config.ActionInternal: "internal",
		config.ActionUser: "user", config.ActionProgram: "program"}[a.Kind]
	path, target := a.Path, a.Target
	if a.Kind == config.ActionChangeDir {
		path, target = thLocalSeparators(path), thLocalSeparators(target)
	}
	return THAction{Kind: kind, Path: path, Filter: a.Filter, Target: target, Command: a.Command, Args: a.Args}
}

// thLocalSeparators gives a local folder path the platform's separators: "cd ~/src" on Windows expands to
// C:\Users\me/src, which the panel would show and store with mixed separators. Server addresses (ftp://...) keep
// their slashes; on macOS and Linux nothing changes.
func thLocalSeparators(p string) string {
	if strings.Contains(p, "://") {
		return p
	}
	return filepath.FromSlash(p)
}

// Expand expands the variables in a path ($HOME, %VAR%, ${VAR}, ~, %COMMANDER_PATH%).
func (t *TabsHotlist) Expand(s string) string { return t.vars().Expand(s) }

// Filter searches the hotlist: entries whose title, command or target contain every word of the query.
func (t *TabsHotlist) Filter(query string) []THMatch {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []THMatch{}
	for _, m := range t.hot.Filter(query) {
		out = append(out, THMatch{Title: config.StripAccelerator(m.Item.Title), Breadcrumb: m.Breadcrumb, Cmd: m.Item.Field("cmd"), Target: m.Item.Field("path")})
	}
	return out
}

// MatchMask says, for each name, whether it matches a Total Commander file mask such as "*.txt *.md | *.bak" (a
// hotlist entry "cd folder/*.txt" shows only the matching files).
func (t *TabsHotlist) MatchMask(mask string, names []string, dirs []bool) []bool {
	m := config.ParseMask(mask)
	out := make([]bool, len(names))
	for i, n := range names {
		out[i] = m.Match(n, n, i < len(dirs) && dirs[i])
	}
	return out
}

// ReadTC reads the [DirMenu] hotlist of a Total Commander wincmd.ini (for importing it).
func (t *TabsHotlist) ReadTC(path string) ([]THItem, error) {
	f, err := config.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("The file %s could not be read: %v", filepath.Base(path), err)
	}
	if !f.HasSection(config.SectionDirMenu) {
		return nil, fmt.Errorf("%s has no folder hotlist (no [DirMenu] section).", filepath.Base(path))
	}
	return thFromItems(config.LoadHotlist(f).Items), nil
}

// RunProgram starts a program or opens a document or web address from a hotlist entry, in the folder dir.
func (t *TabsHotlist) RunProgram(program, params, dir string) error {
	if t.app != nil && t.app.mock != nil {
		return errors.New("Programs are not started in the demo.")
	}
	l := launch.New()
	if strings.Contains(program, "://") && params == "" {
		return l.OpenDefault(program)
	}
	if err := l.Run(launch.RunSpec{Program: program, Params: params, Dir: dir}); err != nil {
		return fmt.Errorf("%s could not be started: %v", program, err)
	}
	return nil
}

// ---- tabs ----

// thSortOrder is TC's sortorder number for a sort key (the <n>_options of a tab start with "sortorder|negative").
var thSortOrder = map[string]int{"name": 0, "ext": 1, "size": 2, "date": 3, "none": 4}

func thTabsTo(s THTabSet) config.TabSet {
	ts := config.TabSet{Active: s.Active}
	for _, tb := range s.Tabs {
		o, ok := thSortOrder[tb.Sort]
		if !ok {
			o = 0
		}
		neg := 0
		if tb.Desc {
			neg = 1
		}
		lock := config.LockMode(tb.Lock)
		if lock < config.TabUnlocked || lock > config.TabLockedChangeOK {
			lock = config.TabUnlocked
		}
		ts.Tabs = append(ts.Tabs, config.Tab{Path: thOneLine(tb.Path), Caption: thOneLine(tb.Caption), Options: fmt.Sprintf("%d|%d", o, neg), Lock: lock,
			View: thOneLine(tb.View), ColumnSet: thOneLine(tb.Set)})
	}
	if ts.Active < 0 || ts.Active >= len(ts.Tabs) {
		ts.Active = 0
	}
	return ts
}

func thTabsFrom(ts config.TabSet) THTabSet {
	out := THTabSet{Tabs: []THTab{}, Active: ts.Active}
	for _, tb := range ts.Tabs {
		x := THTab{Path: tb.Path, Caption: tb.Caption, Lock: int(tb.Lock), Sort: "name", View: tb.View, Set: tb.ColumnSet}
		parts := strings.Split(tb.Options, "|")
		if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
			for k, v := range thSortOrder {
				if v == n {
					x.Sort = k
				}
			}
		}
		if len(parts) > 1 && strings.TrimSpace(parts[1]) == "1" {
			x.Desc = true
		}
		out.Tabs = append(out.Tabs, x)
	}
	return out
}

// Tabs returns the saved tabs of both panels and their folder histories.
func (t *TabsHotlist) Tabs() THTabs {
	t.mu.Lock()
	defer t.mu.Unlock()
	lh, rh := config.NewHistory(0, true), config.NewHistory(0, true)
	lh.Max, rh.Max = 200, 200
	lh.Load(t.file, config.SectionLeftHistory)
	rh.Load(t.file, config.SectionRightHistory)
	return THTabs{
		Left: thTabsFrom(config.LoadTabSet(t.file, config.SectionLeftTabs)), Right: thTabsFrom(config.LoadTabSet(t.file, config.SectionRightTabs)),
		LeftHistory: lh.Items(), RightHistory: rh.Items(),
	}
}

// thHistory turns a newest-first folder list into the engine's history: no repeats, at most max entries.
func thHistory(items []string, max int) *config.History {
	h := config.NewHistory(max, true)
	for i := len(items) - 1; i >= 0; i-- {
		h.Add(items[i])
	}
	return h
}

// SaveTabs stores both panels' tabs and folder histories (written a moment later, so quick changes are one write).
func (t *TabsHotlist) SaveTabs(s THTabs) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	config.SaveTabSet(t.file, config.SectionLeftTabs, thTabsTo(s.Left))
	config.SaveTabSet(t.file, config.SectionRightTabs, thTabsTo(s.Right))
	thHistory(s.LeftHistory, s.HistoryMax).Save(t.file, config.SectionLeftHistory)
	thHistory(s.RightHistory, s.HistoryMax).Save(t.file, config.SectionRightHistory)
	t.saveSoon()
	return nil
}

// TrimHistory returns a newest-first folder history without repeats and with at most max entries (the history size
// setting).
func (t *TabsHotlist) TrimHistory(items []string, max int) []string {
	out := thHistory(items, max).Items()
	if out == nil {
		out = []string{}
	}
	return out
}

// SaveTabFile writes a .tab file with the tabs of the active panel (and of the other one when it has any).
func (t *TabsHotlist) SaveTabFile(path string, f THTabFile) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("No file name was given.")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("The folder for %s could not be made: %v", filepath.Base(path), err)
	}
	if err := config.SaveTabFile(path, config.TabFile{Active: thTabsTo(f.Active), Inactive: thTabsTo(f.Inactive)}); err != nil {
		return fmt.Errorf("The tabs could not be saved to %s: %v", filepath.Base(path), err)
	}
	return nil
}

// LoadTabFile reads a .tab file.
func (t *TabsHotlist) LoadTabFile(path string) (THTabFile, error) {
	tf, err := config.LoadTabFile(t.vars().Expand(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return THTabFile{}, fmt.Errorf("The tab file %s is not there.", filepath.Base(path))
		}
		return THTabFile{}, fmt.Errorf("The tab file %s could not be read: %v", filepath.Base(path), err)
	}
	if len(tf.Active.Tabs) == 0 {
		return THTabFile{}, fmt.Errorf("The tab file %s has no tabs in it.", filepath.Base(path))
	}
	return THTabFile{Active: thTabsFrom(tf.Active), Inactive: thTabsFrom(tf.Inactive)}, nil
}

// TabDir is the folder where tab files are kept by default (tabs in the settings folder).
func (t *TabsHotlist) TabDir() string {
	if d := configDir(); d != "" {
		return filepath.Join(d, "tabs")
	}
	return ""
}

// ChooseFile shows the system's Save or Open dialog for a file of the given extension ("tab", "ini"); "" means
// cancelled. In control mode the answer comes from the queue (kinds "savefile" and "openfile").
func (t *TabsHotlist) ChooseFile(save bool, title, start, ext string) (string, error) {
	kind := "openfile"
	if save {
		kind = "savefile"
	}
	if v, ok := t.app.mockAnswer(kind, title); ok {
		return v, nil
	}
	if t.app.ctx == nil {
		return "", nil
	}
	dir, name := "", ""
	if start != "" {
		if st, err := os.Stat(start); err == nil && st.IsDir() {
			dir = start
		} else {
			dir, name = filepath.Dir(start), filepath.Base(start)
		}
		if dir != "" {
			os.MkdirAll(dir, 0o755)
		}
	}
	filters := []fileFilter{}
	if ext != "" {
		filters = append(filters, fileFilter{DisplayName: strings.ToUpper(ext) + " files (*." + ext + ")", Pattern: "*." + ext})
	}
	if save {
		return t.app.saveFileDialog(title, dir, name, filters)
	}
	return t.app.openFileDialog(title, dir, filters)
}

// ---- favourite tabs ----

// Favorites lists the favourite tab sets, sorted by name.
func (t *TabsHotlist) Favorites() []THFavorite {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []THFavorite{}
	for _, f := range config.LoadFavoriteTabs(t.file) {
		out = append(out, THFavorite{Name: f.Name, Left: thTabsFrom(f.Left), Right: thTabsFrom(f.Right)})
	}
	return out
}

func thFavName(name string) (string, error) {
	name = strings.TrimSpace(thOneLine(name))
	if name == "" {
		return "", errors.New("Please give the favourite tabs a name.")
	}
	if strings.ContainsAny(name, "[]") {
		return "", errors.New("A name of favourite tabs cannot contain [ or ].")
	}
	return name, nil
}

// SaveFavorite stores (or replaces) a favourite tab set.
func (t *TabsHotlist) SaveFavorite(f THFavorite) error {
	name, err := thFavName(f.Name)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	config.SaveFavoriteTabs(t.file, config.FavoriteTabs{Name: name, Left: thTabsTo(f.Left), Right: thTabsTo(f.Right)})
	return t.save()
}

// DeleteFavorite removes a favourite tab set.
func (t *TabsHotlist) DeleteFavorite(name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !config.DeleteFavoriteTabs(t.file, name) {
		return fmt.Errorf("There are no favourite tabs named %s.", name)
	}
	return t.save()
}

// RenameFavorite gives a favourite tab set another name.
func (t *TabsHotlist) RenameFavorite(from, to string) error {
	to, err := thFavName(to)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, f := range config.LoadFavoriteTabs(t.file) {
		if !strings.EqualFold(f.Name, to) || strings.EqualFold(from, to) {
			continue
		}
		return fmt.Errorf("There are already favourite tabs named %s.", to)
	}
	for _, f := range config.LoadFavoriteTabs(t.file) {
		if f.Name == from {
			config.DeleteFavoriteTabs(t.file, from)
			f.Name = to
			config.SaveFavoriteTabs(t.file, f)
			return t.save()
		}
	}
	return fmt.Errorf("There are no favourite tabs named %s.", from)
}

// ---- frequently used folders ----

// Visit counts a visit of a folder for the frequently used folders list.
func (t *TabsHotlist) Visit(folder string) {
	if folder == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.freq.Visit(folder)
	t.freq.Save(t.file, config.SectionFrequentDirs)
	t.saveSoon()
}

// Frequent returns up to n of the most used folders, most used first.
func (t *TabsHotlist) Frequent(n int) []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n <= 0 {
		n = 20
	}
	out := t.freq.Top(n)
	if out == nil {
		out = []string{}
	}
	return out
}

// ForgetFrequent removes one folder from the frequently used folders.
func (t *TabsHotlist) ForgetFrequent(folder string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.freq.Forget(folder)
	t.freq.Save(t.file, config.SectionFrequentDirs)
	return t.save()
}

// ClearFrequent empties the frequently used folders.
func (t *TabsHotlist) ClearFrequent() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.freq.Clear()
	t.freq.Save(t.file, config.SectionFrequentDirs)
	return t.save()
}
