package main

// File tags (issue #28, F-869): the tags the system's own file manager shows, read from and written to the files
// themselves — Finder tags on macOS, user.xdg.tags on Linux (internal/tags does the work). Windows has no file tags:
// Info says so and the page leaves the commands disabled there.
//
// The page asks only for the rows on screen (Lookup, in batches) and only while Show > File tags is on (off by
// default). Change adds and removes tags on the selection; every change that did something goes into the undo journal
// as one entry, so Cmd+Z (Files > Undo last file operation) puts the old tags back. Custom columns get the field
// [=mc.tags] (the tag names, comma-separated), which works whether or not the marks are shown.

import (
	"errors"
	"path/filepath"
	"strings"

	lst "mormicommander/internal/listing"
	"mormicommander/internal/tags"
	"mormicommander/internal/undo"
)

// Tags is what the page calls for file tags (window.go.main.Tags).
type Tags struct{ app *App }

func init() {
	registerFeature("tags", func(a *App) []exposed { return []exposed{expose(&Tags{app: a})} })
	lst.RegisterField("mc", lst.Field{Name: "tags", Kind: lst.KindText, Help: "file tags (Finder tags, user.xdg.tags)",
		Get: func(it *lst.Item, _ string, _ *lst.Format) lst.Value {
			if it.IsUp() || !tagsLocal(it.Dir) {
				return lst.TextValue("")
			}
			l, _ := tags.Read(filepath.Join(it.Dir, it.Name))
			return lst.TextValue(tagNames(l))
		}})
}

// tagsLocal says whether a path is on this computer's disks (tags are never read from servers or archives).
func tagsLocal(p string) bool { return p != "" && !strings.Contains(p, "://") && filepath.IsAbs(p) }

func tagNames(l []tags.Tag) string {
	names := make([]string, len(l))
	for i, t := range l {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

// TagsInfo says whether this system has file tags and how they are kept.
type TagsInfo struct {
	Supported bool   `json:"supported"`
	Style     string `json:"style"`  // "finder", "xdg" or ""
	Colors    bool   `json:"colors"` // tags have colours (Finder)
}

// Info says whether this system has file tags (never in the demo, whose files are not real).
func (t *Tags) Info() TagsInfo {
	s := tags.System()
	if t.app != nil && t.app.mock != nil {
		s = tags.None
	}
	return TagsInfo{Supported: s != tags.None, Style: string(s), Colors: s.HasColors()}
}

// Lookup returns the tags of the paths that have any, by path.
func (t *Tags) Lookup(paths []string) map[string][]tags.Tag {
	out := map[string][]tags.Tag{}
	if !t.Info().Supported {
		return out
	}
	for _, p := range paths {
		if !tagsLocal(p) {
			continue
		}
		if l, err := tags.Read(p); err == nil && len(l) > 0 {
			out[p] = l
		}
	}
	return out
}

// TagsResult is what Change did.
type TagsResult struct {
	Changed int            `json:"changed"`
	Failed  []ToolsFailure `json:"failed"`
}

// Change adds the tags of add to every path and takes the names of remove away. A tag named like one of Finder's
// colour tags gets that colour when add gives none; on Linux tags have no colour. What changed is one undo step.
func (t *Tags) Change(paths []string, add []tags.Tag, remove []string) (TagsResult, error) {
	res := TagsResult{Failed: []ToolsFailure{}}
	info := t.Info()
	if !info.Supported {
		return res, errors.New("This system has no file tags.")
	}
	style := tags.Style(info.Style)
	var adds []tags.Tag
	for _, a := range add {
		name := tags.Clean(style, a.Name)
		if name == "" {
			continue
		}
		c := a.Color
		if !style.HasColors() {
			c = 0
		} else if c <= 0 || c > 7 {
			c = tags.DefaultColor(name)
		}
		adds = append(adds, tags.Tag{Name: name, Color: c})
	}
	var items []undo.Item
	for _, p := range paths {
		if !tagsLocal(p) {
			res.Failed = append(res.Failed, ToolsFailure{Path: p, Error: "Tags can only be set on this computer's disks."})
			continue
		}
		before, err := tags.Read(p)
		if err != nil {
			res.Failed = append(res.Failed, ToolsFailure{Path: p, Error: err.Error()})
			continue
		}
		after, changed := tags.Change(before, adds, remove)
		if !changed {
			continue
		}
		if err := tags.Write(p, after); err != nil {
			res.Failed = append(res.Failed, ToolsFailure{Path: p, Error: err.Error()})
			continue
		}
		res.Changed++
		items = append(items, undo.Item{From: p, To: p, TagsBefore: before, TagsAfter: after})
	}
	if len(items) > 0 && t.app != nil && t.app.undo != nil {
		t.app.undo.record(undo.Tags, items...)
	}
	return res, nil
}
