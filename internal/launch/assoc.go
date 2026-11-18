// Internal file associations (cm_InternalAssociate, F-323). Total Commander lets the user map file masks to
// commands inside the file manager, independent of the system: Enter, F3 and F4 on a matching file use the
// association before the system default. Changing the system's own associations is deliberately not done (F-322 is
// N/A); the internal ones cover the need without touching the OS.
//
// Associations are kept in an [Associations] INI section, in the spirit of wincmd.ini, one numbered entry each:
//
//	[Associations]
//	1=*.txt *.md *.log
//	1_desc=Text files
//	1_icon=/path/to/icon.png
//	1_open=code --wait "%1"
//	1_view=cm_List
//	1_edit="C:\Tools\npp.exe" -n1 %1
//	1_Print=lp "%1"
//
// The numbers give the order; the first entry whose mask matches wins. open, view and edit are the verbs of Enter,
// F3 and F4; any other suffix is an extra verb for the context menu. A command is either an internal command
// (cm_*/em_* with optional arguments), handed back to the caller to run, or a program followed by parameters with
// Total Commander placeholders (%1 for the file, %P, %N, %S, …). An entry without a placeholder gets the file appended.
package launch

import (
	"sort"
	"strconv"
	"strings"
)

// The standard verbs.
const (
	VerbOpen = "open" // Enter / double-click
	VerbView = "view" // F3
	VerbEdit = "edit" // F4
)

// AssociationsSection is the INI section associations are stored in.
const AssociationsSection = "Associations"

// Association maps a mask list to commands.
type Association struct {
	Mask        string     // Total Commander mask list, see ParseMask
	Description string     // shown in the configuration dialog
	Icon        string     // optional icon file
	Verbs       []VerbLine // commands in display order: open, view, edit first, then custom verbs
}

// VerbLine is one command of an association.
type VerbLine struct {
	Verb    string // "open", "view", "edit" or a custom name such as "Print"
	Command string // internal command or program with parameters
}

// Command returns the command for verb (matched without regard to case).
func (a Association) Command(verb string) (string, bool) {
	for _, v := range a.Verbs {
		if strings.EqualFold(v.Verb, verb) && v.Command != "" {
			return v.Command, true
		}
	}
	return "", false
}

// SetCommand sets (or with an empty command removes) the command for verb.
func (a *Association) SetCommand(verb, command string) {
	for i, v := range a.Verbs {
		if strings.EqualFold(v.Verb, verb) {
			if command == "" {
				a.Verbs = append(a.Verbs[:i], a.Verbs[i+1:]...)
			} else {
				a.Verbs[i].Command = command
			}
			return
		}
	}
	if command != "" {
		a.Verbs = append(a.Verbs, VerbLine{Verb: verb, Command: command})
	}
}

// Associations is the ordered list of internal associations.
type Associations struct {
	List []Association
}

// LoadAssociations reads the [Associations] section of an INI file. Entries without a mask are skipped.
func LoadAssociations(f *INIFile) *Associations {
	a := &Associations{}
	s := f.Section(AssociationsSection)
	if s == nil {
		return a
	}
	byNum := map[int]*Association{}
	var nums []int
	get := func(n int) *Association {
		if x, ok := byNum[n]; ok {
			return x
		}
		x := &Association{}
		byNum[n] = x
		nums = append(nums, n)
		return x
	}
	for _, kv := range s.Keys {
		if kv.Key == "" {
			continue
		}
		numText, field, hasField := strings.Cut(kv.Key, "_")
		n, err := strconv.Atoi(numText)
		if err != nil || n < 0 {
			continue
		}
		x := get(n)
		if !hasField {
			x.Mask = kv.Value
			continue
		}
		switch strings.ToLower(field) {
		case "desc":
			x.Description = kv.Value
		case "icon":
			x.Icon = kv.Value
		case "":
		default:
			verb := field
			if lf := strings.ToLower(field); lf == VerbOpen || lf == VerbView || lf == VerbEdit {
				verb = lf
			}
			x.SetCommand(verb, kv.Value)
		}
	}
	sort.Ints(nums)
	for _, n := range nums {
		x := byNum[n]
		if strings.TrimSpace(x.Mask) == "" {
			continue
		}
		x.Verbs = orderVerbs(x.Verbs)
		a.List = append(a.List, *x)
	}
	return a
}

// orderVerbs puts open, view and edit first, keeping custom verbs in file order after them.
func orderVerbs(v []VerbLine) []VerbLine {
	rank := func(s string) int {
		switch s {
		case VerbOpen:
			return 0
		case VerbView:
			return 1
		case VerbEdit:
			return 2
		}
		return 3
	}
	sort.SliceStable(v, func(i, j int) bool { return rank(v[i].Verb) < rank(v[j].Verb) })
	return v
}

// Store writes the list into the [Associations] section, replacing what was there, numbered from 1.
func (a *Associations) Store(f *INIFile) {
	s := f.EnsureSection(AssociationsSection)
	s.Keys = nil
	for i, x := range a.List {
		n := strconv.Itoa(i + 1)
		s.Set(n, x.Mask)
		if x.Description != "" {
			s.Set(n+"_desc", x.Description)
		}
		if x.Icon != "" {
			s.Set(n+"_icon", x.Icon)
		}
		for _, v := range x.Verbs {
			if v.Command != "" {
				s.Set(n+"_"+v.Verb, v.Command)
			}
		}
	}
}

// Find returns the first association whose mask matches name, or nil.
func (a *Associations) Find(name string, isDir bool) *Association {
	if a == nil {
		return nil
	}
	for i := range a.List {
		if ParseMask(a.List[i].Mask).Match(name, isDir) {
			return &a.List[i]
		}
	}
	return nil
}

// Lookup returns the command for verb of the first association that matches name and has that verb. Later
// associations are consulted when an earlier match has no command for the verb, so "*.*" can provide a fallback
// editor while "*.txt" only sets the opener.
func (a *Associations) Lookup(name string, isDir bool, verb string) (string, bool) {
	if a == nil {
		return "", false
	}
	for i := range a.List {
		if !ParseMask(a.List[i].Mask).Match(name, isDir) {
			continue
		}
		if c, ok := a.List[i].Command(verb); ok {
			return c, true
		}
	}
	return "", false
}

// Action is a parsed association command.
type Action struct {
	Internal string // the internal command (cm_*, em_*), when the command is one
	Args     string // the internal command's arguments
	Tool     Tool   // otherwise: the program and its parameters
}

// ParseAction splits an association command into an internal command or a program with parameters.
func ParseAction(command string) Action {
	command = strings.TrimSpace(command)
	first, rest := cutWord(command)
	if lf := strings.ToLower(first); strings.HasPrefix(lf, "cm_") || strings.HasPrefix(lf, "em_") {
		return Action{Internal: first, Args: strings.TrimSpace(rest)}
	}
	return Action{Tool: ParseTool(command)}
}

// Result says what OpenAssociated did.
type Result struct {
	Handled  bool   // an association matched and was carried out (or is an internal command to carry out)
	Internal string // the internal command the caller must run, when the association names one
	Args     string // its arguments
}

// OpenAssociated applies the internal association for verb to the file at ctx.Source.Dir/ctx.Source.Current (or
// ctx.File). When there is none, Result.Handled is false and the caller falls back to its default: the system's
// app for open, the built-in Lister for view, the configured editor for edit.
func (l *Launcher) OpenAssociated(a *Associations, verb string, ctx ParamContext) (Result, error) {
	file := ctx.File
	if file == "" {
		file = joinPath(l.OS, ctx.Source.Dir, ctx.Source.Current)
	}
	name := baseName(l.OS, file)
	command, ok := a.Lookup(name, false, verb)
	if !ok {
		return Result{}, nil
	}
	act := ParseAction(command)
	if act.Internal != "" {
		return Result{Handled: true, Internal: act.Internal, Args: act.Args}, nil
	}
	if err := l.OpenInTool(act.Tool, file, ctx); err != nil {
		return Result{}, err
	}
	return Result{Handled: true}, nil
}

// ParseAssociationLines reads the short form the Internal associations dialog keeps (cm_InternalAssociate): one
// association per line, "mask list = command", for Enter (the open verb). Empty lines and lines starting with "#" or
// ";" are skipped, as are lines without "=" or with an empty mask or command. The mask list is split at the first
// "=", so the command may hold "=" itself.
func ParseAssociationLines(text string) *Associations {
	a := &Associations{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		mask, command, ok := strings.Cut(line, "=")
		mask, command = strings.TrimSpace(mask), strings.TrimSpace(command)
		if !ok || mask == "" || command == "" {
			continue
		}
		a.List = append(a.List, Association{Mask: mask, Verbs: []VerbLine{{Verb: VerbOpen, Command: command}}})
	}
	return a
}
