// Tests for the Windows attribute bit set: TC's letter column, parsing and the
// attrib-style "+r -h" change notation.
package attrs

import (
	"reflect"
	"testing"
)

func TestWinAttrString(t *testing.T) {
	tests := []struct {
		a          WinAttr
		short, lng string
	}{
		{0, "----", "------"},
		{AttrArchive, "-a--", "-a----"},
		{AttrReadOnly | AttrHidden | AttrSystem, "r-hs", "r-hs--"},
		{AttrArchive | AttrCompressed, "-a--", "-a--c-"},
		{AttrEncrypted | AttrDirectory, "----", "-----e"},
	}
	for _, tt := range tests {
		if got := tt.a.String(); got != tt.short {
			t.Errorf("String(%#x) = %q, want %q", tt.a, got, tt.short)
		}
		if got := tt.a.LongString(); got != tt.lng {
			t.Errorf("LongString(%#x) = %q, want %q", tt.a, got, tt.lng)
		}
		back, err := ParseWinAttr(tt.lng)
		if err != nil || back != tt.a&(AttrReadOnly|AttrArchive|AttrHidden|AttrSystem|AttrCompressed|AttrEncrypted) {
			t.Errorf("ParseWinAttr(%q) = %#x, %v", tt.lng, back, err)
		}
	}
}

func TestParseWinAttr(t *testing.T) {
	tests := []struct {
		in   string
		want WinAttr
		ok   bool
	}{
		{"ra", AttrReadOnly | AttrArchive, true},
		{"-A-R", AttrReadOnly | AttrArchive, true},
		{"h s", AttrHidden | AttrSystem, true},
		{"", 0, true},
		{"rx", 0, false},
	}
	for _, tt := range tests {
		got, err := ParseWinAttr(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseWinAttr(%q) = %#x, %v", tt.in, got, err)
		}
	}
}

func TestParseWinAttrChange(t *testing.T) {
	tests := []struct {
		in   string
		want WinAttrChange
		ok   bool
	}{
		{"+r -h", WinAttrChange{Set: AttrReadOnly, Clear: AttrHidden}, true},
		{"+r+a-s", WinAttrChange{Set: AttrReadOnly | AttrArchive, Clear: AttrSystem}, true},
		{"+ra", WinAttrChange{Set: AttrReadOnly | AttrArchive}, true},
		{"+r-r", WinAttrChange{Clear: AttrReadOnly}, true}, // last one wins
		{"r", WinAttrChange{}, false},
		{"+q", WinAttrChange{}, false},
		{"", WinAttrChange{}, true},
	}
	for _, tt := range tests {
		got, err := ParseWinAttrChange(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseWinAttrChange(%q) = %+v, %v", tt.in, got, err)
		}
		if tt.ok {
			again, err := ParseWinAttrChange(got.String())
			if err != nil || again != got {
				t.Errorf("round trip of %q via %q = %+v, %v", tt.in, got.String(), again, err)
			}
		}
	}
}

func TestWinAttrChangeApplyAndNames(t *testing.T) {
	c := WinAttrChange{Set: AttrHidden, Clear: AttrArchive}
	if got := c.Apply(AttrArchive | AttrReadOnly); got != AttrReadOnly|AttrHidden {
		t.Errorf("Apply = %#x", got)
	}
	if !(WinAttrChange{}).IsZero() || c.IsZero() {
		t.Error("IsZero wrong")
	}
	if got := (AttrReadOnly | AttrCompressed).Names(); !reflect.DeepEqual(got, []string{"read-only", "compressed"}) {
		t.Errorf("Names = %v", got)
	}
	if !(AttrReadOnly | AttrHidden).Has(AttrHidden) || AttrReadOnly.Has(AttrHidden) || AttrReadOnly.Has(0) {
		t.Error("Has wrong")
	}
}
