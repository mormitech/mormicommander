// This file ties the engine together into the one call a panel makes: Build takes the raw entries of a folder plus
// the panel's settings (view mode, column set, sort, filter, formats) and returns the rows to draw, the header titles,
// and the status line totals. Brief view is a single-column set whose rows the UI flows into several screen columns;
// BriefLayout does that arithmetic (top to bottom, then left to right, as in TC) so cursor movement with the arrow keys
// can be computed without the UI knowing the rules.

package listing

import "sync"

// ViewMode is a panel display mode from the Show menu.
type ViewMode int

const (
	ViewBrief    ViewMode = iota // cm_SrcShort, Ctrl+F1
	ViewFull                     // cm_SrcLong, Ctrl+F2
	ViewComments                 // cm_SrcComments, Ctrl+Shift+F2
	ViewCustom                   // a user column set (Shift+F1 menu)
)

// Options are a panel's display settings.
type Options struct {
	Mode       ViewMode
	Custom     *CompiledSet // column set for ViewCustom; ignored otherwise
	NamesWhole bool         // Full view without a separate extension column (F-092)
	Format     Format
	Collation  Collation
	Sort       SortSpec
	Filter     FilterOptions
	Comments   *Comments // descript.ion for the folder; loaded by the caller when the view or a column needs it
}

// Row is one rendered line.
type Row struct {
	Item  Item     // the entry; Item.Order is its index in the raw listing
	Cells []string // one string per column of View.Set
}

// View is a ready-to-draw panel.
type View struct {
	Set    *CompiledSet
	Titles []string
	Rows   []Row
	Totals Totals // selection counts are zero; recount with View.Count after marking
	Sort   SortSpec
}

// The built-in sets are compiled on first use: package-level variables are initialised before init functions run,
// so compiling them eagerly would happen before the tc fields are registered.
var builtinSets = sync.OnceValue(func() map[ViewMode]*CompiledSet {
	return map[ViewMode]*CompiledSet{
		ViewBrief:     BriefSet.MustCompile(),
		ViewFull:      FullSet.MustCompile(),
		ViewComments:  CommentsSet.MustCompile(),
		viewFullWhole: FullNameSet.MustCompile(),
	}
})

const viewFullWhole ViewMode = -1

// SetFor returns the column set of a view mode. separateExt chooses between Full view with an extension column and
// Full view with names whole (F-092).
func SetFor(mode ViewMode, custom *CompiledSet, separateExt bool) *CompiledSet {
	sets := builtinSets()
	switch mode {
	case ViewBrief, ViewComments:
		return sets[mode]
	case ViewCustom:
		if custom != nil {
			return custom
		}
	}
	if !separateExt {
		return sets[viewFullWhole]
	}
	return sets[ViewFull]
}

// Build filters, sorts and renders a folder.
func Build(items []Item, opt Options) *View {
	set := SetFor(opt.Mode, opt.Custom, !opt.NamesWhole)
	vis := Filter(items, opt.Filter)
	if opt.Comments != nil {
		ApplyComments(vis, opt.Comments)
	}
	col := NewCollator(opt.Collation)
	f := opt.Format
	Sort(vis, opt.Sort, SortOptions{Collator: col, Format: &f, Set: set})
	v := &View{Set: set, Titles: set.Titles(), Rows: make([]Row, len(vis)), Sort: opt.Sort}
	for i := range vis {
		v.Rows[i] = Row{Item: vis[i], Cells: set.Render(&vis[i], &f)}
	}
	v.Totals = Count(vis, nil)
	return v
}

// Count recomputes totals with a selection.
func (v *View) Count(selected func(i int) bool) Totals {
	items := make([]Item, len(v.Rows))
	for i := range v.Rows {
		items[i] = v.Rows[i].Item
	}
	return Count(items, selected)
}

// Find returns the row index of name, or -1, so the cursor can stay on the same file after a re-sort or refresh.
func (v *View) Find(name string) int {
	for i := range v.Rows {
		if v.Rows[i].Item.Name == name {
			return i
		}
	}
	return -1
}

// BriefLayout maps item indexes to grid cells for Brief view, which fills columns top to bottom.
type BriefLayout struct {
	N    int // number of items
	Rows int // visible rows per column (at least 1)
}

// Columns is the number of screen columns needed.
func (b BriefLayout) Columns() int {
	r := b.rows()
	return (b.N + r - 1) / r
}

func (b BriefLayout) rows() int {
	if b.Rows < 1 {
		return 1
	}
	return b.Rows
}

// Pos returns the column and row of item i.
func (b BriefLayout) Pos(i int) (col, row int) { r := b.rows(); return i / r, i % r }

// Index returns the item at a cell, or -1 when the cell is past the end.
func (b BriefLayout) Index(col, row int) int {
	r := b.rows()
	if col < 0 || row < 0 || row >= r {
		return -1
	}
	i := col*r + row
	if i >= b.N {
		return -1
	}
	return i
}

// Move returns the cursor position after an arrow key in Brief view: up/down move by one item, left/right by one
// column (clamped to the first and last item, as TC does).
func (b BriefLayout) Move(i, dCol, dRow int) int {
	if b.N == 0 {
		return 0
	}
	j := i + dRow + dCol*b.rows()
	if j < 0 {
		j = 0
	}
	if j >= b.N {
		j = b.N - 1
	}
	return j
}
