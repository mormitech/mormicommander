// These tests cover two review findings from issue #91: a counter width
// with no upper bound (typing [C:2000000000] in the live preview used to
// build a 2 GB string), and Execute trusting a stale Errors count after
// names were edited by hand and CheckEntries was run again.
package multirename

import (
	"errors"
	"strings"
	"testing"
)

func TestCounterWidthIsBounded(t *testing.T) {
	if _, err := CompileMask("[C:2000000000]"); err == nil {
		t.Error("a 2-billion-digit counter width was accepted")
	}
	if _, err := CompileMask("[C:256]"); err == nil {
		t.Error("a counter width above the cap was accepted")
	}
	if _, err := CompileMask("[C:255]"); err != nil {
		t.Errorf("the largest allowed width was refused: %v", err)
	}
	// A huge width from the dialog's Digits field is clamped, not honoured.
	if got := len(padInt(7, 1<<31)); got != MaxCounterDigits {
		t.Errorf("padInt length = %d, want %d", got, MaxCounterDigits)
	}
	if got := padInt(-7, 3); got != "-007" {
		t.Errorf("padInt(-7, 3) = %q", got)
	}
}

func TestExecuteRechecksEditedEntries(t *testing.T) {
	_, items := makeFiles(t, "a.txt", "b.txt")
	s := DefaultSettings()
	s.NameMask = "[N]_new"
	res := previewOrFail(t, items, s)
	// The user edits the second new name by hand so it clashes with the
	// first, and the dialog reruns CheckEntries without touching Errors.
	res.Entries[1].NewName = res.Entries[0].NewName
	CheckEntries(res.Entries, PreviewOptions{CaseRule: CaseRuleSensitive})
	if res.Entries[1].OK() && res.Entries[0].OK() {
		t.Fatal("CheckEntries did not flag the clash")
	}
	if _, err := Execute(res, ExecOptions{}); !errors.Is(err, ErrHasConflicts) {
		t.Fatalf("want ErrHasConflicts, got %v", err)
	}
	if res.Errors == 0 {
		t.Error("Errors was not refreshed")
	}
	// Fixing the name by hand clears the block again.
	res.Entries[1].NewName = strings.Replace(res.Entries[0].NewName, "a", "c", 1)
	CheckEntries(res.Entries, PreviewOptions{CaseRule: CaseRuleSensitive})
	if res.HasErrors() {
		t.Fatalf("still blocked after the fix: %+v", res.Entries)
	}
}
