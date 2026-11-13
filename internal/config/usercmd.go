// User-defined commands (F-311). Total Commander lets users define their own
// commands in usercmd.ini, one section per command named em_<something>, with
// the keys cmd, param, path, menu (title/tooltip), button (icon) and iconic,
// the same fields a button has. Once defined, an em_ command appears in the
// command list next to the built-in cm_ commands and can be put on buttons,
// bound to hotkeys and used in the Start menu or hotlist. This file reads and
// writes that file and converts between commands and buttons.

package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// UserCmdIniName is the file holding user commands, next to the main ini.
const UserCmdIniName = "usercmd.ini"

// UserCommand is one em_ command.
type UserCommand struct {
	Name    string // em_..., the section name
	Command string
	Params  string
	Path    string
	Menu    string // title shown in menus and tooltips
	Icon    string
	Iconic  int
}

// Button returns the command as a button that runs it by name.
func (u UserCommand) Button() Button {
	return Button{Icon: u.Icon, Command: u.Name, Tooltip: u.Menu}
}

// ValidUserCommandName reports whether name is a valid em_ name.
func ValidUserCommandName(name string) bool {
	if len(name) < 4 || !strings.EqualFold(name[:3], "em_") {
		return false
	}
	for _, c := range name[3:] {
		if c == '[' || c == ']' || c == '=' || c == ' ' || c == '\t' {
			return false
		}
	}
	return true
}

// LoadUserCommands reads every em_ section of a usercmd.ini document,
// sorted by name.
func LoadUserCommands(f *File) []UserCommand {
	var out []UserCommand
	for _, sec := range f.Sections() {
		if !ValidUserCommandName(sec) {
			continue
		}
		m := f.Map(sec)
		iconic, _ := strconv.Atoi(strings.TrimSpace(m["iconic"]))
		out = append(out, UserCommand{
			Name: sec, Command: m["cmd"], Params: m["param"], Path: m["path"],
			Menu: m["menu"], Icon: m["button"], Iconic: iconic,
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// SaveUserCommand writes (or replaces) one command's section.
func SaveUserCommand(f *File, u UserCommand) error {
	if !ValidUserCommandName(u.Name) {
		return fmt.Errorf("config: %q is not a valid em_ command name", u.Name)
	}
	kvs := []KV{{"cmd", u.Command}}
	add := func(k, v string) {
		if v != "" {
			kvs = append(kvs, KV{k, v})
		}
	}
	add("param", u.Params)
	add("path", u.Path)
	add("menu", u.Menu)
	add("button", u.Icon)
	if u.Iconic != 0 {
		kvs = append(kvs, KV{"iconic", strconv.Itoa(u.Iconic)})
	}
	f.ReplaceSection(u.Name, kvs)
	return nil
}

// DeleteUserCommand removes a command.
func DeleteUserCommand(f *File, name string) bool { return f.DeleteSection(name) }

// FindUserCommand looks a command up by name (case-insensitive).
func FindUserCommand(cmds []UserCommand, name string) (UserCommand, bool) {
	for _, c := range cmds {
		if strings.EqualFold(c.Name, name) {
			return c, true
		}
	}
	return UserCommand{}, false
}
