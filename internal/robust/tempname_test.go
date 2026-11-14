package robust

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTempStemAndShortName(t *testing.T) {
	long := strings.Repeat("é", 120) + ".txt" // 244 bytes
	if s := TempStem(long); len(s) > 64 || !utf8.ValidString(s) {
		t.Fatalf("TempStem: %d bytes, valid %v", len(s), utf8.ValidString(s))
	}
	s := ShortName(long, 100)
	if len(s) > 100 || !strings.HasSuffix(s, ".txt") || !utf8.ValidString(s) {
		t.Fatalf("ShortName: %q", s)
	}
	if ShortName("a.txt", 100) != "a.txt" {
		t.Fatal("a short name changed")
	}
}
