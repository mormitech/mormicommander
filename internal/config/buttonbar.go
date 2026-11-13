// Button bar files (section 11 of FEATURES.md). Total Commander keeps each
// toolbar in its own .bar file so users can switch between bars (F-309) and
// open one bar from a button of another as a drop-down sub-bar (F-308). A
// .bar file is an INI file with one [Buttonbar] section: Buttoncount=N, then
// for every button N the keys buttonN (icon: "file,index" or an image path),
// cmdN (a cm_* or em_* command, a program, or another .bar file for a
// sub-bar), paramN (parameters with %P/%N... placeholders, see params.go),
// pathN (start folder), menuN (tooltip text) and iconicN (1 = run minimised,
// -1 = maximised). A button with neither icon nor command is a separator. The
// main bar and the vertical middle bar (F-314) are named in wincmd.ini under
// [Buttonbar] and [ButtonbarVertical]. Single buttons can be copied and
// pasted as text in the same clipboard format TC uses (F-313).

package config

import (
	"fmt"
	"strconv"
	"strings"
)

// SectionButtonbar is the section inside a .bar file, and also the
// wincmd.ini section that names the main bar file.
const (
	SectionButtonbar         = "Buttonbar"
	SectionButtonbarVertical = "ButtonbarVertical"
)

// Button is one toolbar button.
type Button struct {
	Icon    string // "file,index" or an image file; may contain variables
	Command string // cm_x, em_x, program, or a .bar file (sub-bar)
	Params  string
	Path    string // start folder
	Tooltip string // TC "menu" key
	Iconic  int    // 0 normal, 1 minimised, -1 maximised
}

// IsSeparator reports whether the button is a gap.
func (b Button) IsSeparator() bool {
	return strings.TrimSpace(b.Icon) == "" && strings.TrimSpace(b.Command) == ""
}

// IsSubBar reports whether clicking opens another bar.
func (b Button) IsSubBar() bool {
	head, _ := SplitCommand(b.Command)
	return strings.HasSuffix(strings.ToLower(head), ".bar")
}

// IconFile splits Icon into a file and a resource index ("x.dll,3").
func (b Button) IconFile() (string, int) {
	i := strings.LastIndex(b.Icon, ",")
	if i > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(b.Icon[i+1:])); err == nil {
			return strings.TrimSpace(b.Icon[:i]), n
		}
	}
	return strings.TrimSpace(b.Icon), 0
}

// Bar is a whole button bar.
type Bar struct {
	Buttons []Button
}

// ParseBar reads the [Buttonbar] section of a .bar document. Buttoncount
// decides how many buttons there are; missing numbers become separators.
func ParseBar(f *File) *Bar {
	m := f.Map(SectionButtonbar)
	n, _ := parseLeadingInt(m["buttoncount"])
	if n < 0 {
		n = 0
	}
	if n > 10000 {
		n = 10000
	}
	bar := &Bar{}
	for i := 1; i <= n; i++ {
		s := strconv.Itoa(i)
		iconic, _ := parseLeadingInt(m["iconic"+s])
		bar.Buttons = append(bar.Buttons, Button{
			Icon:    m["button"+s],
			Command: m["cmd"+s],
			Params:  m["param"+s],
			Path:    m["path"+s],
			Tooltip: m["menu"+s],
			Iconic:  iconic,
		})
	}
	return bar
}

// LoadBarFile reads a .bar file.
func LoadBarFile(path string) (*Bar, *File, error) {
	f, err := LoadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return ParseBar(f), f, nil
}

// Write stores the bar into the document's [Buttonbar] section.
func (bar *Bar) Write(f *File) {
	kvs := []KV{{"Buttoncount", strconv.Itoa(len(bar.Buttons))}}
	for i, b := range bar.Buttons {
		s := strconv.Itoa(i + 1)
		kvs = append(kvs, KV{"button" + s, b.Icon}, KV{"cmd" + s, b.Command})
		if b.IsSeparator() {
			continue
		}
		if b.Params != "" {
			kvs = append(kvs, KV{"param" + s, b.Params})
		}
		if b.Path != "" {
			kvs = append(kvs, KV{"path" + s, b.Path})
		}
		if b.Tooltip != "" {
			kvs = append(kvs, KV{"menu" + s, b.Tooltip})
		}
		if b.Iconic != 0 {
			kvs = append(kvs, KV{"iconic" + s, strconv.Itoa(b.Iconic)})
		}
	}
	f.ReplaceSection(SectionButtonbar, kvs)
}

// SaveBarFile writes the bar to path atomically, keeping comments and other
// sections of an existing file.
func (bar *Bar) SaveBarFile(path string) error {
	f, err := LoadFile(path)
	if err != nil {
		f = NewFile()
	}
	bar.Write(f)
	return f.Save(path)
}

func (bar *Bar) check(i int) error {
	if i < 0 || i >= len(bar.Buttons) {
		return fmt.Errorf("config: button %d out of range", i)
	}
	return nil
}

// Insert adds a button at index (-1 or past the end = append).
func (bar *Bar) Insert(index int, b Button) int {
	if index < 0 || index > len(bar.Buttons) {
		index = len(bar.Buttons)
	}
	bar.Buttons = append(bar.Buttons, Button{})
	copy(bar.Buttons[index+1:], bar.Buttons[index:])
	bar.Buttons[index] = b
	return index
}

// Remove deletes the button at index.
func (bar *Bar) Remove(index int) (Button, error) {
	if err := bar.check(index); err != nil {
		return Button{}, err
	}
	b := bar.Buttons[index]
	bar.Buttons = append(bar.Buttons[:index], bar.Buttons[index+1:]...)
	return b, nil
}

// Move moves a button from one index to another.
func (bar *Bar) Move(from, to int) error {
	b, err := bar.Remove(from)
	if err != nil {
		return err
	}
	bar.Insert(to, b)
	return nil
}

// ButtonClipboardHeader starts the text Total Commander puts on the
// clipboard when a button is copied (right-click > Copy, F-313). Users share
// buttons on forums in this form, so pasting one copied from TC works too.
const ButtonClipboardHeader = "TOTALCMD#BAR#DATA"

// ClipboardText encodes the button the way TC copies it: the header, then
// one line each for command, parameters, icon, tooltip, start folder and
// window mode, then "-1".
func (b Button) ClipboardText() string {
	iconic := ""
	if b.Iconic != 0 {
		iconic = strconv.Itoa(b.Iconic)
	}
	return strings.Join([]string{ButtonClipboardHeader, b.Command, b.Params, b.Icon, b.Tooltip, b.Path, iconic, "-1"}, "\r\n") + "\r\n"
}

// ParseButtonClipboard decodes text copied from a button (ours or TC's).
// Missing trailing lines are empty; text without the header is rejected.
func ParseButtonClipboard(text string) (Button, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != ButtonClipboardHeader {
		return Button{}, fmt.Errorf("config: clipboard does not hold a button")
	}
	get := func(i int) string {
		if i < len(lines) {
			return strings.TrimRight(lines[i], "\r")
		}
		return ""
	}
	b := Button{Command: get(1), Params: get(2), Icon: get(3), Tooltip: get(4), Path: get(5)}
	b.Iconic, _ = strconv.Atoi(strings.TrimSpace(get(6)))
	return b, nil
}

// ButtonForFile builds the button that dropping a program or file on the
// bar creates (F-306): the file is the command and its own icon, the tooltip
// is the file name, and the start folder is the file's folder.
func ButtonForFile(path string) Button {
	name := path
	dir := ""
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		name, dir = path[i+1:], path[:i]
		if dir == "" {
			dir = path[:1]
		}
	}
	return Button{Icon: path, Command: QuoteArg(path), Path: dir, Tooltip: name}
}
