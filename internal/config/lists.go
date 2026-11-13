// Smaller list-type settings that don't fit a single key: view modes
// (F-820: a named combination of view, column set and sort that can switch
// on automatically for matching folders), tooltip rules (F-821: which fields
// a tooltip shows for which file types), external tools (our replacement for
// TC's Windows-only plugin DLLs, F-425: command-line helpers for packing,
// viewing and extra columns) and the ignore list (F-428: a text file with one
// mask per line whose matches are hidden from the panels). The first three
// are stored as numbered records, NameN/MaskN/..., in their own sections; the
// ignore list is a separate text file named on the Ignore list page, exactly
// like TC's ignore.txt.

package config

import (
	"os"
	"strconv"
	"strings"
)

// Section names.
const (
	SectionViewModes     = "ViewModes"
	SectionTooltips      = "Tooltips"
	SectionExternalTools = "ExternalTools"
)

// loadRecords reads numbered records whose first field must be present.
func loadRecords(f *File, section string, fields []string) []map[string]string {
	m := f.Map(section)
	var out []map[string]string
	for i := 1; ; i++ {
		n := strconv.Itoa(i)
		if _, ok := m[strings.ToLower(fields[0])+n]; !ok {
			break
		}
		r := map[string]string{}
		for _, fl := range fields {
			r[fl] = m[strings.ToLower(fl)+n]
		}
		out = append(out, r)
	}
	return out
}

func saveRecords(f *File, section string, fields []string, recs []map[string]string) {
	var kvs []KV
	for i, r := range recs {
		n := strconv.Itoa(i + 1)
		for j, fl := range fields {
			if v := r[fl]; v != "" || j == 0 {
				kvs = append(kvs, KV{fl + n, v})
			}
		}
	}
	f.ReplaceSection(section, kvs)
}

// ViewMode is a named panel look.
type ViewMode struct {
	Name       string
	View       string // brief, full, thumbnails, tree, comments, custom
	ColumnSet  string // when View is "custom"
	SortColumn int
	SortDesc   bool
	AutoSwitch string // folder mask; empty = manual only
}

var viewModeFields = []string{"Name", "View", "Columns", "Sort", "Desc", "Auto"}

// LoadViewModes reads [ViewModes].
func LoadViewModes(f *File) []ViewMode {
	var out []ViewMode
	for _, r := range loadRecords(f, SectionViewModes, viewModeFields) {
		sort, _ := strconv.Atoi(r["Sort"])
		out = append(out, ViewMode{Name: r["Name"], View: r["View"], ColumnSet: r["Columns"],
			SortColumn: sort, SortDesc: r["Desc"] == "1", AutoSwitch: r["Auto"]})
	}
	return out
}

// SaveViewModes writes [ViewModes].
func SaveViewModes(f *File, modes []ViewMode) {
	var recs []map[string]string
	for _, v := range modes {
		desc := ""
		if v.SortDesc {
			desc = "1"
		}
		recs = append(recs, map[string]string{"Name": v.Name, "View": v.View, "Columns": v.ColumnSet,
			"Sort": strconv.Itoa(v.SortColumn), "Desc": desc, "Auto": v.AutoSwitch})
	}
	saveRecords(f, SectionViewModes, viewModeFields, recs)
}

// AutoViewMode returns the index of the first mode whose folder mask
// matches, or -1.
func AutoViewMode(modes []ViewMode, folder string) int {
	for i, v := range modes {
		cs := ColumnSet{AutoSwitch: v.AutoSwitch}
		if cs.MatchFolder(folder) {
			return i
		}
	}
	return -1
}

// TooltipRule says which fields to show in tooltips for matching files.
type TooltipRule struct {
	Mask   string
	Fields string // column template, lines separated by "\n" (literal)
}

var tooltipFields = []string{"Mask", "Fields"}

// LoadTooltipRules reads [Tooltips].
func LoadTooltipRules(f *File) []TooltipRule {
	var out []TooltipRule
	for _, r := range loadRecords(f, SectionTooltips, tooltipFields) {
		out = append(out, TooltipRule{Mask: r["Mask"], Fields: r["Fields"]})
	}
	return out
}

// SaveTooltipRules writes [Tooltips].
func SaveTooltipRules(f *File, rules []TooltipRule) {
	var recs []map[string]string
	for _, t := range rules {
		recs = append(recs, map[string]string{"Mask": t.Mask, "Fields": t.Fields})
	}
	saveRecords(f, SectionTooltips, tooltipFields, recs)
}

// TooltipFor returns the template lines of the first rule matching a file.
func TooltipFor(rules []TooltipRule, name, fullPath string, isDir bool) ([]string, bool) {
	for _, r := range rules {
		if ParseMask(r.Mask).Match(name, fullPath, isDir) {
			return splitBackslashN(r.Fields), true
		}
	}
	return nil, false
}

// ExternalTool is a configured command-line helper.
type ExternalTool struct {
	Name    string
	Kind    string // "packer", "unpacker", "viewer", "editor", "field"
	Mask    string // file types it handles
	Command string
	Args    string // with %-placeholders (params.go)
}

var toolFields = []string{"Name", "Kind", "Mask", "Cmd", "Args"}

// LoadExternalTools reads [ExternalTools].
func LoadExternalTools(f *File) []ExternalTool {
	var out []ExternalTool
	for _, r := range loadRecords(f, SectionExternalTools, toolFields) {
		out = append(out, ExternalTool{Name: r["Name"], Kind: r["Kind"], Mask: r["Mask"], Command: r["Cmd"], Args: r["Args"]})
	}
	return out
}

// SaveExternalTools writes [ExternalTools].
func SaveExternalTools(f *File, tools []ExternalTool) {
	var recs []map[string]string
	for _, t := range tools {
		recs = append(recs, map[string]string{"Name": t.Name, "Kind": t.Kind, "Mask": t.Mask, "Cmd": t.Command, "Args": t.Args})
	}
	saveRecords(f, SectionExternalTools, toolFields, recs)
}

// ToolFor returns the first tool of a kind that handles a file name.
func ToolFor(tools []ExternalTool, kind, name string) (ExternalTool, bool) {
	for _, t := range tools {
		if strings.EqualFold(t.Kind, kind) && MatchMask(t.Mask, name, false) {
			return t, true
		}
	}
	return ExternalTool{}, false
}

// IgnoreList is a parsed ignore file.
type IgnoreList struct {
	Masks []Mask
}

// ParseIgnoreList reads ignore file text: one mask per line; blank lines
// and lines starting with ";" or "#" are skipped.
func ParseIgnoreList(text string) IgnoreList {
	var l IgnoreList
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		// A full path is one pattern even if it contains spaces.
		if strings.ContainsAny(line, `/\`) && !strings.Contains(line, "|") {
			l.Masks = append(l.Masks, Mask{Include: []string{line}})
			continue
		}
		l.Masks = append(l.Masks, ParseMask(line))
	}
	return l
}

// LoadIgnoreList reads an ignore file (decoded like an ini file). A missing
// file gives an empty list and no error.
func LoadIgnoreList(path string) (IgnoreList, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return IgnoreList{}, nil
	}
	if err != nil {
		return IgnoreList{}, err
	}
	text, _ := Decode(data, EncCP1252)
	return ParseIgnoreList(text), nil
}

// Hidden reports whether a file should be hidden. A pattern that is a full
// path hides exactly that file or folder.
func (l IgnoreList) Hidden(name, fullPath string, isDir bool) bool {
	for _, m := range l.Masks {
		if len(m.Include) == 1 && len(m.Exclude) == 0 && isAbsLike(m.Include[0]) {
			if fullPath != "" && pathsEqual(strings.TrimRight(m.Include[0], `/\`), fullPath) {
				return true
			}
			continue
		}
		if m.Match(name, fullPath, isDir) {
			return true
		}
	}
	return false
}

func isAbsLike(p string) bool {
	if strings.ContainsAny(p, "*?") {
		return false
	}
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\\`) || (len(p) > 2 && p[1] == ':')
}
