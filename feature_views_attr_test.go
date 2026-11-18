package main

import (
	"testing"

	lst "mormicommander/internal/listing"
	"mormicommander/internal/vfs"
)

// F-686: the archive bit read on Windows reaches the custom-column engine's attribute string.
func TestViewsItemFromEntryArchive(t *testing.T) {
	it := viewsItemFromEntry("/x", vfs.Entry{Name: "a.txt", Flags: vfs.FlagArchive | vfs.FlagWinAttrs | vfs.FlagReadOnly}, 0)
	if got := lst.AttrString(&it, lst.AttrStyleWindows); got != "ra--" {
		t.Fatalf("attr = %q, want ra--", got)
	}
	it = viewsItemFromEntry("/x", vfs.Entry{Name: "b.txt", Flags: vfs.FlagWinAttrs}, 0)
	if got := lst.AttrString(&it, lst.AttrStyleWindows); got != "----" {
		t.Fatalf("attr = %q, want ----", got)
	}
}
