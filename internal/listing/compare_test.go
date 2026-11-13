// Tests for the name collation rules: natural numbers, case handling, word sort and locale order.
package listing

import "testing"

func TestCollatorCompare(t *testing.T) {
	tests := []struct {
		name string
		c    Collation
		a, b string
		want int
	}{
		{"plain less", Collation{}, "apple", "banana", -1},
		{"case-insensitive equal", Collation{}, "Apple", "apple", 0},
		{"case-insensitive alpha before case", Collation{}, "apple", "Banana", -1},
		{"case-sensitive upper first", Collation{CaseSensitive: true}, "Apple", "apple", -1},
		{"case-sensitive still alphabetical", Collation{CaseSensitive: true}, "apple", "Banana", -1},
		{"string sort digits", Collation{}, "file10", "file2", -1},
		{"natural digits", Collation{Natural: true}, "file2", "file10", -1},
		{"natural equal values leading zero", Collation{Natural: true}, "file7", "file007", -1},
		{"natural multi runs", Collation{Natural: true}, "v1.10.2", "v1.9.9", 1},
		{"natural huge numbers", Collation{Natural: true}, "a99999999999999999999", "a100000000000000000000", -1},
		{"prefix shorter first", Collation{}, "abc", "abcd", -1},
		{"word sort ignores hyphen", Collation{WordSort: true}, "co-op", "coop", 0},
		{"no word sort hyphen counts", Collation{}, "co-op", "coop", -1},
		{"unicode fold", Collation{}, "Éclair", "éclair", 0},
		{"locale german umlaut", Collation{Locale: "de"}, "Äpfel", "Banane", -1},
		{"code point umlaut after z", Collation{}, "Äpfel", "Zebra", 1},
		{"locale natural", Collation{Locale: "en", Natural: true}, "file2", "file10", -1},
		{"locale case-sensitive lower first (CLDR)", Collation{Locale: "en", CaseSensitive: true}, "a", "A", -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewCollator(tt.c).Compare(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			if back := NewCollator(tt.c).Compare(tt.b, tt.a); back != -tt.want {
				t.Fatalf("Compare(%q, %q) = %d, want %d (antisymmetry)", tt.b, tt.a, back, -tt.want)
			}
		})
	}
}
