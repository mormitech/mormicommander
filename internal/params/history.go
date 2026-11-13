// Command-line history. Total Commander remembers the commands typed into
// the command-line box (the dropdown on Alt+F8 / Ctrl+Down) and lets the
// user step through them with Ctrl+E (older) and Ctrl+X (newer). The newest
// command is first, a repeated command moves to the top instead of being
// stored twice, and the list is capped. Saving and loading is left to the
// ini layer: Items returns the list in the order to store it, and
// NewHistory accepts it back.
package params

import "strings"

// History is the command-line history, newest first.
type History struct {
	items []string
	max   int
	pos   int // -1 = not browsing; else index of the shown item
	draft string
}

// NewHistory returns a history holding at most max items (0 means 50),
// starting with items (newest first).
func NewHistory(max int, items []string) *History {
	if max <= 0 {
		max = 50
	}
	h := &History{max: max, pos: -1}
	for i := len(items) - 1; i >= 0; i-- {
		h.Add(items[i])
	}
	return h
}

// Add records a command; blank commands are ignored.
func (h *History) Add(cmd string) {
	cmd = strings.TrimSpace(cmd)
	h.pos = -1
	if cmd == "" {
		return
	}
	out := []string{cmd}
	for _, it := range h.items {
		if it != cmd {
			out = append(out, it)
		}
	}
	if len(out) > h.max {
		out = out[:h.max]
	}
	h.items = out
}

// Items returns a copy of the history, newest first.
func (h *History) Items() []string { return append([]string(nil), h.items...) }

// Older returns the next older command (Ctrl+E). current is the text in
// the box, restored when the user steps back past the newest entry.
func (h *History) Older(current string) (string, bool) {
	if h.pos+1 >= len(h.items) {
		return "", false
	}
	if h.pos < 0 {
		h.draft = current
	}
	h.pos++
	return h.items[h.pos], true
}

// Newer returns the next newer command (Ctrl+X); stepping past the newest
// returns the text that was in the box before browsing started.
func (h *History) Newer() (string, bool) {
	if h.pos < 0 {
		return "", false
	}
	h.pos--
	if h.pos < 0 {
		return h.draft, true
	}
	return h.items[h.pos], true
}

// ResetBrowse ends browsing (the user typed something).
func (h *History) ResetBrowse() { h.pos = -1 }
