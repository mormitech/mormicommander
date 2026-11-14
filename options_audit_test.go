// The Options audit (board issue #94): every option of the Options dialog must either be read by the program or be
// listed in internal/config/noeffect.go as having no effect yet, so the dialog can mark it (and hide it with
// Misc.HideNoEffect). "Read by the program" means its "Page.Field" name appears as a quoted string in the page's code
// (frontend/src, where features read options by name or bridge them to their own settings), or it is read as a field
// ".Page.Field" in the Go code outside internal/config (also through a variable holding the page, like
// "op := cfg.Settings.Operation" and then "op.SaveHistory"). internal/config is left out because it only defines and
// stores the options: the few it looks at itself (history and tab sections of mormicommander.ini, the ignore list
// loader) are about parts of the file the program does not use, as the folder history and tabs live in folders.ini.
//
// The test fails both ways: an option nobody reads that is not on the list (a new option added without wiring it up),
// and an option on the list that is now read (wired up, but still marked as doing nothing). It reads the source files
// only; nothing is started.

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mormicommander/internal/config"
)

// optionSources reads the code the audit searches: the page's JavaScript, and the Go code outside internal/config
// without tests.
func optionSources(t *testing.T) (js, goSrc string) {
	t.Helper()
	var jsB, goB strings.Builder
	read := func(root string, keep func(path string) bool, into *strings.Builder) {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", ".worktrees", ".git", "build":
					if path != root {
						return filepath.SkipDir
					}
				}
				if filepath.ToSlash(path) == "internal/config" {
					return filepath.SkipDir
				}
				return nil
			}
			if !keep(path) {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			into.Write(b)
			into.WriteByte('\n')
			return nil
		})
		if err != nil {
			t.Fatalf("reading %s: %v", root, err)
		}
	}
	read(filepath.Join("frontend", "src"), func(p string) bool { return strings.HasSuffix(p, ".js") }, &jsB)
	read(".", func(p string) bool { return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") }, &goB)
	return jsB.String(), goB.String()
}

// pageAlias finds a variable that holds one page of the settings: "op := cfg.Settings.Operation".
var pageAlias = regexp.MustCompile(`(?m)\b(\w+)\s*:?=\s*[\w.]*Settings\.(\w+)\s*$`)

// optionReader answers whether the program's code reads an option.
type optionReader struct {
	js, goSrc string
	aliases   map[string][]string // page → variables holding it
}

func newOptionReader(js, goSrc string) *optionReader {
	r := &optionReader{js: js, goSrc: goSrc, aliases: map[string][]string{}}
	for _, m := range pageAlias.FindAllStringSubmatch(goSrc, -1) {
		r.aliases[m[2]] = append(r.aliases[m[2]], m[1])
	}
	return r
}

func (r *optionReader) reads(name string) bool {
	q := regexp.QuoteMeta(name)
	if regexp.MustCompile("['\"`]" + q + "['\"`]").MatchString(r.js) {
		return true
	}
	if regexp.MustCompile(`\.` + q + `\b`).MatchString(r.goSrc) {
		return true
	}
	page, field, _ := strings.Cut(name, ".")
	for _, v := range r.aliases[page] {
		if regexp.MustCompile(`\b` + regexp.QuoteMeta(v) + `\.` + regexp.QuoteMeta(field) + `\b`).MatchString(r.goSrc) {
			return true
		}
	}
	return false
}

func TestOptionReaderFindsReads(t *testing.T) {
	r := newOptionReader("x = opt('Layout.ButtonBarVertical');", "op := cfg.Settings.Operation\nif op.SavePath {}\n_ = s.Display.ShowSeconds")
	for name, want := range map[string]bool{
		"Layout.ButtonBarVertical": true, "Layout.ButtonBar": false, // a longer name is not a read of a shorter one
		"Operation.SavePath": true, "Operation.SaveHistory": false, "Display.ShowSeconds": true, "Display.ShowSystem": false,
	} {
		if got := r.reads(name); got != want {
			t.Errorf("reads(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestOptionsEitherReadOrMarkedNoEffect(t *testing.T) {
	js, goSrc := optionSources(t)
	if !strings.Contains(js, "'Display.ShowHidden'") {
		t.Fatal("the page's code was not found (run the test from the repository root)")
	}
	r := newOptionReader(js, goSrc)
	var unmarked, stale []string
	for _, f := range config.Fields() {
		read := r.reads(f.Name)
		switch {
		case !read && !f.NoEffect:
			unmarked = append(unmarked, f.Name)
		case read && f.NoEffect:
			stale = append(stale, f.Name)
		}
	}
	sort.Strings(unmarked)
	sort.Strings(stale)
	if len(unmarked) > 0 {
		t.Errorf("options nobody reads: wire them up or add them to internal/config/noeffect.go:\n\t%s", strings.Join(unmarked, "\n\t"))
	}
	if len(stale) > 0 {
		t.Errorf("options marked as having no effect that are now read: remove them from internal/config/noeffect.go:\n\t%s", strings.Join(stale, "\n\t"))
	}
}

// The options this change wires to the features that do the work must stay wired (and so not be marked).
func TestOptionsWiredInIssue94(t *testing.T) {
	for _, name := range []string{
		"Refresh.AutoRefresh", "Refresh.OnlyForeground", "Refresh.RefreshNetwork", "Refresh.ExcludeFolders",
		"DragDrop.ConfirmDrop", "DragDrop.SpringLoaded", "DragDrop.SpringDelayMs", "Hotlist.HistorySize",
		"Log.Enabled", "Log.File", "FolderTabs.Show", "FolderTabs.MultiLine", "FolderTabs.CloseButton",
		"FolderTabs.DoubleClickOpen", "FolderTabs.NewNextToCur", "FolderTabs.ConfirmCloseAll", "FolderTabs.LockedIcon",
		"FolderTabs.MaxTitleChars", "Misc.HideNoEffect",
	} {
		f, ok := config.LookupSetting(name)
		if !ok {
			t.Errorf("%s: no such option", name)
			continue
		}
		if f.NoEffect {
			t.Errorf("%s is marked as having no effect", name)
		}
	}
}
