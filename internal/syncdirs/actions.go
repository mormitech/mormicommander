// This file holds everything between comparing and synchronizing: the
// default arrow Total Commander proposes for each row, the rules for which
// actions a row allows, the user's overrides from the right-click menu and
// arrow clicks (FEATURES.md F-550), the show/hide filter buttons (F-549) and
// the counts shown under the list.
//
// Defaults follow Total Commander. In normal (symmetric) mode the newer file
// or the only copy is copied to the other side and "unequal" rows get no
// action, because the engine cannot know which side the user wants. In
// asymmetric mode the left side is the master: every difference is copied
// left to right and names that exist only on the right are marked for
// deletion. The two asymmetric actions that destroy newer or unique data on
// the right (overwriting a newer right file, deleting a right-only entry)
// start out unselected, as spec area-5 9.3.3 decides: they are shown, but
// Plan leaves them out until the user selects them. A row never gets an
// action that would write or delete inside a followed folder link, a
// file/folder clash can only be deleted, and a pipe, socket or device is
// never copied.
package syncdirs

import (
	"fmt"
	"strings"
)

func defaultAction(it *Item, asym bool) Action {
	var a Action
	switch it.Category {
	case Equal:
		a = NoAction
	case LeftNewer, LeftOnly:
		a = CopyLeftToRight
	case RightNewer:
		a = CopyRightToLeft
		if asym {
			a = CopyLeftToRight
		}
	case RightOnly:
		a = CopyRightToLeft
		if asym {
			a = DeleteRight
		}
	case Unequal:
		if asym && !it.TypeMismatch {
			a = CopyLeftToRight
		}
	}
	if !it.CanApply(a) {
		return NoAction
	}
	return a
}

// setDefault gives the row its default action and selection state.
func (it *Item) setDefault(asym bool) {
	it.Default = defaultAction(it, asym)
	it.defUnselected = asym && ((it.Category == RightNewer && it.Default == CopyLeftToRight) ||
		(it.Category == RightOnly && it.Default == DeleteRight))
	it.Action = it.Default
	it.Unselected = it.defUnselected
}

// CanApply reports whether action a is allowed for the row.
func (it *Item) CanApply(a Action) bool {
	switch a {
	case NoAction:
		return true
	case CopyLeftToRight:
		return it.Left != nil && it.Left.Kind != KindSpecial && !it.TypeMismatch && writableDest(it.Right, it.rvia)
	case CopyRightToLeft:
		return it.Right != nil && it.Right.Kind != KindSpecial && !it.TypeMismatch && writableDest(it.Left, it.lvia)
	case DeleteLeft:
		return it.Left != nil && !it.lvia
	case DeleteRight:
		return it.Right != nil && !it.rvia
	case DeleteBoth:
		return it.Left != nil && it.Right != nil && !it.lvia && !it.rvia
	}
	return false
}

// writableDest says whether a copy may write to the destination entry: not
// inside a followed folder link, and not over a followed link (that would
// either write through it or silently replace it).
func writableDest(dst *Entry, via bool) bool {
	if via {
		return false
	}
	if dst != nil && dst.Followed {
		return false
	}
	return true
}

// SetAction overrides the action of one row. Choosing an action is an
// explicit decision, so the row also becomes selected.
func (r *Result) SetAction(path string, a Action) error {
	it := r.index[path]
	if it == nil {
		return fmt.Errorf("syncdirs: no item %q", path)
	}
	if !it.CanApply(a) {
		return fmt.Errorf("syncdirs: action %s not possible for %q", a, path)
	}
	it.Action = a
	it.Unselected = false
	return nil
}

// SetSelected selects or unselects the action of one row. An unselected
// row keeps its action (the arrow stays visible) but Plan skips it.
func (r *Result) SetSelected(path string, on bool) error {
	it := r.index[path]
	if it == nil {
		return fmt.Errorf("syncdirs: no item %q", path)
	}
	it.Unselected = !on
	return nil
}

// SelectTree selects or unselects the row at path and every row below it
// (an empty path means all rows) and returns how many rows changed.
func (r *Result) SelectTree(path string, on bool) (int, error) {
	if path != "" && r.index[path] == nil {
		return 0, fmt.Errorf("syncdirs: no item %q", path)
	}
	n := 0
	for _, it := range r.Items {
		if path != "" && it.Path != path && !strings.HasPrefix(it.Path, path+"/") {
			continue
		}
		if it.Unselected == on {
			it.Unselected = !on
			n++
		}
	}
	return n, nil
}

// SetActionTree sets action a on the row at path and on every row below it
// where the action is possible, as Total Commander does when the user picks
// an action for a folder. An empty path means all rows. Rows where the
// action is impossible keep their current action. Rows that get the action
// also become selected. It returns how many rows were changed.
func (r *Result) SetActionTree(path string, a Action) (int, error) {
	if path != "" && r.index[path] == nil {
		return 0, fmt.Errorf("syncdirs: no item %q", path)
	}
	n := 0
	for _, it := range r.Items {
		if path != "" && it.Path != path && !strings.HasPrefix(it.Path, path+"/") {
			continue
		}
		if it.CanApply(a) && (it.Action != a || it.Unselected) {
			it.Action = a
			it.Unselected = false
			n++
		}
	}
	return n, nil
}

// ReverseAction swaps the copy direction of one row (the arrow click in the
// list). Rows without a copy action get the copy towards the side that is
// missing or older, when possible.
func (r *Result) ReverseAction(path string) error {
	it := r.index[path]
	if it == nil {
		return fmt.Errorf("syncdirs: no item %q", path)
	}
	var want Action
	switch it.Action {
	case CopyLeftToRight:
		want = CopyRightToLeft
	case CopyRightToLeft:
		want = CopyLeftToRight
	default:
		want = CopyLeftToRight
		if it.Left == nil || it.Category == RightNewer {
			want = CopyRightToLeft
		}
	}
	if !it.CanApply(want) {
		return fmt.Errorf("syncdirs: action %s not possible for %q", want, path)
	}
	it.Action = want
	it.Unselected = false
	return nil
}

// ResetActions puts every row back to its default action and selection.
func (r *Result) ResetActions() {
	for _, it := range r.Items {
		it.Action = it.Default
		it.Unselected = it.defUnselected
	}
}

// Show holds the filter buttons above the list. A row is shown when the
// button for its category is on and, in addition, the duplicates button
// (names on both sides) or the singles button (names on one side) matching
// it is on.
type Show struct {
	LeftToRight bool `json:"leftToRight"` // left newer and left-only rows
	Equal       bool `json:"equal"`
	Unequal     bool `json:"unequal"`
	RightToLeft bool `json:"rightToLeft"` // right newer and right-only rows
	Duplicates  bool `json:"duplicates"`
	Singles     bool `json:"singles"`
}

// ShowAll has every button on.
func ShowAll() Show { return Show{true, true, true, true, true, true} }

// DefaultShow is Total Commander's initial state: equal files hidden.
func DefaultShow() Show { return Show{true, false, true, true, true, true} }

// Matches reports whether a single row passes the buttons, ignoring
// the folder logic of Visible.
func (s Show) Matches(it *Item) bool {
	var cat bool
	switch it.Category {
	case LeftNewer, LeftOnly:
		cat = s.LeftToRight
	case RightNewer, RightOnly:
		cat = s.RightToLeft
	case Equal:
		cat = s.Equal
	case Unequal:
		cat = s.Unequal
	}
	if !cat {
		return false
	}
	if it.Duplicate() {
		return s.Duplicates
	}
	return s.Singles
}

// Visible returns the rows to display under the buttons, in tree order.
// A folder row is kept when anything below it is visible (so the tree
// stays readable) or, when it has nothing below it, when it matches itself.
func (r *Result) Visible(s Show) []*Item {
	shown := map[string]bool{}
	hasChild := map[string]bool{}
	for _, it := range r.Items {
		p := it.Path
		vis := !it.IsDir && s.Matches(it)
		for {
			i := strings.LastIndexByte(p, '/')
			if i < 0 {
				break
			}
			p = p[:i]
			hasChild[p] = true
			if vis {
				shown[p] = true
			}
		}
	}
	var out []*Item
	for _, it := range r.Items {
		switch {
		case !it.IsDir:
			if s.Matches(it) {
				out = append(out, it)
			}
		case shown[it.Path]:
			out = append(out, it)
		case !hasChild[it.Path] && s.Matches(it):
			out = append(out, it)
		}
	}
	return out
}

// Summary counts the rows per category, as shown in the status line of the
// tool. Folders present on both sides only add to Dirs; a folder that exists
// on one side only also counts as LeftOnly or RightOnly, because it is a
// difference the sync will act on.
type Summary struct {
	Equal      int `json:"equal"`
	LeftNewer  int `json:"leftNewer"`
	RightNewer int `json:"rightNewer"`
	LeftOnly   int `json:"leftOnly"`
	RightOnly  int `json:"rightOnly"`
	Unequal    int `json:"unequal"`
	Dirs       int `json:"dirs"`
	Errors     int `json:"errors"`
}

// AllEqual reports whether no file differs (what F-551 checks after a
// sync and a re-compare). Folder rows that exist on one side only count as
// differences too.
func (s Summary) AllEqual() bool {
	return s.LeftNewer+s.RightNewer+s.LeftOnly+s.RightOnly+s.Unequal == 0
}

// Summary returns the counts for the result.
func (r *Result) Summary() Summary {
	var s Summary
	for _, it := range r.Items {
		if it.IsDir {
			s.Dirs++
			switch it.Category {
			case LeftOnly:
				s.LeftOnly++
			case RightOnly:
				s.RightOnly++
			}
			continue
		}
		switch it.Category {
		case Equal:
			s.Equal++
		case LeftNewer:
			s.LeftNewer++
		case RightNewer:
			s.RightNewer++
		case LeftOnly:
			s.LeftOnly++
		case RightOnly:
			s.RightOnly++
		case Unequal:
			s.Unequal++
		}
	}
	s.Errors = len(r.Errors)
	return s
}
