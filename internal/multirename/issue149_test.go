// Review follow-ups from issue #149: search and replace and the duplicate check compare names in NFC like masks and
// Find do, and a name ending in a dot keeps it with the default mask.
package multirename

import (
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestSearchReplaceMatchesEitherUnicodeForm(t *testing.T) {
	nfd := norm.NFD.String("café.txt")
	s := DefaultSettings()
	s.Search = norm.NFC.String("é") // typed on a keyboard
	s.Replace = "e"
	r, err := Compile(s)
	if err != nil {
		t.Fatal(err)
	}
	names, _ := r.NewNames([]Item{{Dir: "/x", Name: nfd}})
	if names[0] != "cafe.txt" {
		t.Fatalf("NFC search on an NFD name: %q, want cafe.txt", names[0])
	}
	// no match: the name comes back byte for byte
	s.Search = "zzz"
	r, _ = Compile(s)
	if names, _ := r.NewNames([]Item{{Dir: "/x", Name: nfd}}); names[0] != nfd {
		t.Fatalf("an unmatched name changed form: %q", names[0])
	}
}

func TestDuplicateCheckFoldsUnicodeForm(t *testing.T) {
	entries := []Entry{
		{Item: Item{Dir: "/x", Name: "a"}, NewName: norm.NFC.String("café")},
		{Item: Item{Dir: "/x", Name: "b"}, NewName: norm.NFD.String("café")},
	}
	CheckEntries(entries, PreviewOptions{CaseRule: CaseRuleInsensitive, SkipDiskCheck: true})
	if entries[0].OK() || entries[1].OK() {
		t.Fatal("two new names that differ only in Unicode form were not reported as duplicates on a folding disk")
	}
}

func TestTrailingDotKept(t *testing.T) {
	r, err := Compile(DefaultSettings())
	if err != nil {
		t.Fatal(err)
	}
	if names, _ := r.NewNames([]Item{{Dir: "/x", Name: "file."}}); names[0] != "file." {
		t.Fatalf("[N].[E] on file.: %q", names[0])
	}
}
