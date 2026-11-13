// This file implements the ignore list (Configuration > Display > Ignore list, toggled with cm_SwitchIgnoreList).
// Entries listed there disappear from the panels while the list is switched on. As in Total Commander, the list is a
// plain text file with one entry per line, and an entry is either a name mask such as "*.bak", "desktop.ini" or
// ".DS_Store", which hides matching entries in every folder, or a mask with a folder path such as
// "C:\Windows\*.dll" or "/home/me/tmp/*", which hides matching entries only in that folder. A trailing "\" or "/"
// limits an entry to folders (".git\"). Path entries use the same wildcards as names, but "*" and "?" stop at path
// separators, so "/tmp/*" hides the contents of /tmp and not of its subfolders. Blank lines and lines starting with
// "#" are skipped so the file can carry notes; TC has no comment syntax, but no real file name starts a line with "#"
// often enough to matter, and a name can still be listed as "[#]name".

package masks

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreList hides entries from the panels while Enabled is set.
type IgnoreList struct {
	Enabled bool
	names   []*Mask // matched against the entry name
	paths   []*Mask // matched against the full slash-separated path
	lines   []string
}

// ParseIgnoreList reads an ignore list from r. The list starts enabled.
func ParseIgnoreList(r io.Reader, c CaseRule) (*IgnoreList, error) {
	text, err := readText(r)
	if err != nil {
		return nil, err
	}
	l := &IgnoreList{Enabled: true}
	cs := c.Sensitive()
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		l.lines = append(l.lines, line)
		body := strings.TrimRight(line, `\/`)
		if body == "" {
			continue // a line that is only a separator would hide nothing useful
		}
		if strings.ContainsAny(body, `\/`) {
			l.paths = append(l.paths, compilePath(line, cs))
		} else {
			l.names = append(l.names, CompileMask(line, cs))
		}
	}
	return l, sc.Err()
}

// LoadIgnoreFile reads an ignore list file. A missing file gives an empty, enabled list, since "no ignore list
// configured yet" is the normal first-run state and not an error.
func LoadIgnoreFile(path string, c CaseRule) (*IgnoreList, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return &IgnoreList{Enabled: true}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseIgnoreList(f, c)
}

// Lines returns the active entries as read, for the configuration page.
func (l *IgnoreList) Lines() []string { return append([]string(nil), l.lines...) }

// Toggle switches the list on or off (cm_SwitchIgnoreList) and returns the new state.
func (l *IgnoreList) Toggle() bool {
	l.Enabled = !l.Enabled
	return l.Enabled
}

// Ignored says whether the entry name inside folder dir is hidden. dir is an OS path; it may be empty for virtual
// folders, in which case only name entries apply.
func (l *IgnoreList) Ignored(dir, name string, isDir bool) bool {
	if l == nil || !l.Enabled || name == ".." {
		return false
	}
	for _, m := range l.names {
		if (!m.dirOnly || isDir) && m.Match(name) {
			return true
		}
	}
	if len(l.paths) == 0 || dir == "" {
		return false
	}
	full := strings.TrimRight(filepath.ToSlash(dir), "/") + "/" + name
	for _, m := range l.paths {
		if (!m.dirOnly || isDir) && m.Match(full) {
			return true
		}
	}
	return false
}

// Filter returns the indexes of the entries in dir that stay visible, in order.
func (l *IgnoreList) Filter(dir string, entries []Entry) []int {
	out := make([]int, 0, len(entries))
	for i, e := range entries {
		if !l.Ignored(dir, e.Name, e.IsDir) {
			out = append(out, i)
		}
	}
	return out
}
