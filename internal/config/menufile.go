// Custom main menu files (F-828). Total Commander can replace its whole main
// menu with a user .mnu file, written in a small resource-script-like
// language: POPUP "&Files" opens a menu, MENUITEM "&Copy", cm_Copy adds an
// entry that runs a command (with optional parameters after the command),
// MENUITEM SEPARATOR draws a line, and END_POPUP closes the menu. Lines
// starting with ";" are comments. This file parses such a file into the same
// MenuItem tree the Start menu uses (field "cmd", optional "param") and
// writes it back, so users can carry their TC menu files over and our menu
// editor can save one. Choosing "default" in the UI simply means not loading
// a file.

package config

import (
	"fmt"
	"strings"
)

// ParseMenuFile parses .mnu text. Errors carry the line number.
func ParseMenuFile(text string) ([]*MenuItem, error) {
	var root []*MenuItem
	stack := []*[]*MenuItem{&root}
	for n, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
			continue
		}
		word, rest := SplitCommand(line)
		cur := stack[len(stack)-1]
		switch strings.ToUpper(word) {
		case "POPUP":
			title, _, err := quotedTitle(rest)
			if err != nil {
				return nil, fmt.Errorf("config: menu line %d: %v", n+1, err)
			}
			it := &MenuItem{Kind: MenuSubmenu, Title: title}
			*cur = append(*cur, it)
			stack = append(stack, &it.Children)
		case "END_POPUP", "ENDPOPUP", "END":
			if len(stack) == 1 {
				return nil, fmt.Errorf("config: menu line %d: END_POPUP without POPUP", n+1)
			}
			stack = stack[:len(stack)-1]
		case "MENUITEM":
			if strings.EqualFold(strings.TrimSpace(rest), "SEPARATOR") {
				*cur = append(*cur, &MenuItem{Kind: MenuSeparator})
				continue
			}
			title, after, err := quotedTitle(rest)
			if err != nil {
				return nil, fmt.Errorf("config: menu line %d: %v", n+1, err)
			}
			after = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(after), ","))
			cmd, param := SplitCommand(after)
			it := &MenuItem{Kind: MenuCommand, Title: title}
			it.SetField("cmd", cmd)
			it.SetField("param", param)
			*cur = append(*cur, it)
		default:
			return nil, fmt.Errorf("config: menu line %d: unknown keyword %q", n+1, word)
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("config: menu file ends inside a POPUP")
	}
	return root, nil
}

// quotedTitle reads a "..." title ("" inside means one quote).
func quotedTitle(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, `"`) {
		return "", "", fmt.Errorf("title must be in double quotes")
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		if s[i] == '"' {
			if i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte('"')
				i++
				continue
			}
			return b.String(), s[i+1:], nil
		}
		b.WriteByte(s[i])
	}
	return "", "", fmt.Errorf("unterminated title")
}

// FormatMenuFile writes a tree as .mnu text with two-space indentation.
func FormatMenuFile(items []*MenuItem) string {
	var b strings.Builder
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	var walk func([]*MenuItem, string)
	walk = func(list []*MenuItem, ind string) {
		for _, it := range list {
			switch it.Kind {
			case MenuSeparator:
				b.WriteString(ind + "MENUITEM SEPARATOR\n")
			case MenuSubmenu:
				b.WriteString(ind + "POPUP " + q(it.Title) + "\n")
				walk(it.Children, ind+"  ")
				b.WriteString(ind + "END_POPUP\n")
			default:
				line := ind + "MENUITEM " + q(it.Title) + ", " + it.Field("cmd")
				if p := it.Field("param"); p != "" {
					line += " " + p
				}
				b.WriteString(line + "\n")
			}
		}
	}
	walk(items, "")
	return b.String()
}
