// Tests for the quick filter: beginning vs any-part, wildcards, alternatives, ".." and kept folders.

package masks

import (
	"reflect"
	"testing"
)

func TestQuickFilter(t *testing.T) {
	entries := []Entry{{Name: "..", IsDir: true}, {Name: "Reports", IsDir: true}, {Name: "report.doc"},
		{Name: "my report.txt"}, {Name: "data.csv"}}
	tests := []struct {
		text string
		opt  QuickFilterOptions
		want []int
	}{
		{"", QuickFilterOptions{}, []int{0, 1, 2, 3, 4}},
		{"rep", QuickFilterOptions{Case: CaseInsensitive}, []int{0, 1, 2}},
		{"rep", QuickFilterOptions{Case: CaseSensitive}, []int{0, 2}},
		{"rep", QuickFilterOptions{Case: CaseInsensitive, AnyPart: true}, []int{0, 1, 2, 3}},
		{"rep", QuickFilterOptions{Case: CaseSensitive, KeepFolders: true}, []int{0, 1, 2}},
		{"*.csv", QuickFilterOptions{}, []int{0, 4}},
		{"r?port", QuickFilterOptions{Case: CaseSensitive}, []int{0, 2}},
		{"my rep", QuickFilterOptions{}, []int{0, 3}},
		{"data;report", QuickFilterOptions{Case: CaseSensitive}, []int{0, 2, 4}},
	}
	for _, tt := range tests {
		q := NewQuickFilter(tt.text, tt.opt)
		if got := q.Filter(entries); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q %+v: got %v, want %v", tt.text, tt.opt, got, tt.want)
		}
		if q.Text() != tt.text || q.Active() != (tt.text != "") {
			t.Errorf("%q: Text/Active", tt.text)
		}
	}
	var nilQ *QuickFilter
	if !nilQ.Match("x", false) || nilQ.Text() != "" {
		t.Error("nil filter must match everything")
	}
}
