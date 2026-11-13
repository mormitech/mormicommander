// Tests for the permission helpers: ls text, octal, chmod expressions and the
// set/clear change model behind the dialog's checkbox grid.
package attrs

import (
	"io/fs"
	"testing"
)

func TestFormatPerm(t *testing.T) {
	tests := []struct {
		mode fs.FileMode
		want string
	}{
		{0o755, "rwxr-xr-x"},
		{0o644, "rw-r--r--"},
		{0, "---------"},
		{0o777 | fs.ModeSetuid, "rwsrwxrwx"},
		{0o644 | fs.ModeSetuid, "rwSr--r--"},
		{0o755 | fs.ModeSetgid, "rwxr-sr-x"},
		{0o744 | fs.ModeSetgid, "rwxr-Sr--"},
		{0o777 | fs.ModeSticky, "rwxrwxrwt"},
		{0o776 | fs.ModeSticky, "rwxrwxrwT"},
	}
	for _, tt := range tests {
		if got := FormatPerm(tt.mode); got != tt.want {
			t.Errorf("FormatPerm(%o) = %q, want %q", tt.mode, got, tt.want)
		}
		back, err := ParsePerm(tt.want)
		if err != nil || back != tt.mode&PermMask {
			t.Errorf("ParsePerm(%q) = %v, %v; want %v", tt.want, back, err, tt.mode&PermMask)
		}
	}
}

func TestFormatMode(t *testing.T) {
	tests := []struct {
		mode fs.FileMode
		want string
	}{
		{0o644, "-rw-r--r--"},
		{fs.ModeDir | 0o755, "drwxr-xr-x"},
		{fs.ModeSymlink | 0o777, "lrwxrwxrwx"},
		{fs.ModeNamedPipe | 0o600, "prw-------"},
		{fs.ModeSocket | 0o600, "srw-------"},
		{fs.ModeDevice | fs.ModeCharDevice | 0o666, "crw-rw-rw-"},
		{fs.ModeDevice | 0o660, "brw-rw----"},
		{fs.ModeDir | fs.ModeSticky | 0o777, "drwxrwxrwt"},
	}
	for _, tt := range tests {
		if got := FormatMode(tt.mode); got != tt.want {
			t.Errorf("FormatMode(%v) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestParsePermErrors(t *testing.T) {
	for _, s := range []string{"", "rwx", "rwxrwxrwxrwx", "rwxrwxrwq", "rwtr-xr-x", "rwxr-xr-s", "xwxr-xr-x"} {
		if _, err := ParsePerm(s); err == nil {
			t.Errorf("ParsePerm(%q) succeeded, want error", s)
		}
	}
	if m, err := ParsePerm("drwxr-xr-x"); err != nil || m != 0o755 {
		t.Errorf("ParsePerm with type letter = %v, %v", m, err)
	}
}

func TestOctal(t *testing.T) {
	tests := []struct {
		in   string
		want fs.FileMode
		ok   bool
	}{
		{"755", 0o755, true},
		{"0644", 0o644, true},
		{"4755", 0o755 | fs.ModeSetuid, true},
		{"2775", 0o775 | fs.ModeSetgid, true},
		{"1777", 0o777 | fs.ModeSticky, true},
		{"7777", 0o777 | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky, true},
		{"0o700", 0o700, true},
		{"7", 0o007, true},
		{"", 0, false},
		{"888", 0, false},
		{"12345", 0, false},
		{"rwx", 0, false},
	}
	for _, tt := range tests {
		got, err := ParseOctal(tt.in)
		if (err == nil) != tt.ok || (tt.ok && got != tt.want) {
			t.Errorf("ParseOctal(%q) = %v, %v; want %v ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
	}
	if s := FormatOctal(0o755 | fs.ModeSetuid); s != "4755" {
		t.Errorf("FormatOctal = %q", s)
	}
	if s := FormatOctal(0o644); s != "0644" {
		t.Errorf("FormatOctal = %q", s)
	}
	for u := uint32(0); u < 0o10000; u += 37 {
		if ToUnix(FromUnix(u)) != u {
			t.Fatalf("unix round trip failed for %o", u)
		}
	}
}

func TestParsePermAny(t *testing.T) {
	for in, want := range map[string]fs.FileMode{"644": 0o644, "rwxr-x---": 0o750, " 0700 ": 0o700, "-rw-------": 0o600} {
		got, err := ParsePermAny(in)
		if err != nil || got != want {
			t.Errorf("ParsePermAny(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
}

func TestApplyChmod(t *testing.T) {
	tests := []struct {
		start fs.FileMode
		expr  string
		isDir bool
		want  fs.FileMode
		ok    bool
	}{
		{0o644, "u+x", false, 0o744, true},
		{0o777, "go-w", false, 0o755, true},
		{0o600, "a+r", false, 0o644, true},
		{0o600, "+r", false, 0o644, true},
		{0o755, "o=", false, 0o750, true},
		{0o644, "u=rwx,g=rx,o=", false, 0o750, true},
		{0o640, "a+X", false, 0o640, true},
		{0o740, "a+X", false, 0o751, true},
		{0o640, "a+X", true, 0o751, true},
		{0o755, "u+s", false, 0o755 | fs.ModeSetuid, true},
		{0o755, "g+s", false, 0o755 | fs.ModeSetgid, true},
		{0o777, "+t", true, 0o777 | fs.ModeSticky, true},
		{0o777, "o+t", true, 0o777 | fs.ModeSticky, true},
		{0o777, "u+t", true, 0o777, true}, // sticky needs o or a
		{0o755 | fs.ModeSetuid, "u-s", false, 0o755, true},
		{0o755 | fs.ModeSetuid, "u=rwx", false, 0o755, true},
		{0o755 | fs.ModeSetgid, "g=rx", true, 0o755 | fs.ModeSetgid, true}, // folders keep setgid on =
		{0o740, "g=u", false, 0o770, true},
		{0o750, "o=g", false, 0o755, true},
		{0o644, "u+r-w", false, 0o444, true},
		{0o644, "700", false, 0o700, true},
		{0o644, "u+x,", false, 0, false},
		{0o644, "u", false, 0, false},
		{0o644, "u+q", false, 0, false},
		{0o644, "z+x", false, 0, false},
		{0o644, "", false, 0o644, true},
	}
	for _, tt := range tests {
		got, err := ApplyChmod(tt.start, tt.expr, tt.isDir)
		if (err == nil) != tt.ok {
			t.Errorf("ApplyChmod(%o, %q) error = %v, want ok=%v", tt.start, tt.expr, err, tt.ok)
			continue
		}
		if tt.ok && got != tt.want {
			t.Errorf("ApplyChmod(%o, %q, dir=%v) = %s, want %s", tt.start, tt.expr, tt.isDir, FormatPerm(got), FormatPerm(tt.want))
		}
	}
}

func TestPermChange(t *testing.T) {
	c := PermChange{Set: 0o100, Clear: 0o002}
	if got, _ := c.Apply(0o646, false); got != 0o744 {
		t.Errorf("Apply = %o", got)
	}
	if !(PermChange{}).IsZero() || c.IsZero() || (PermChange{Expr: "u+x"}).IsZero() {
		t.Error("IsZero wrong")
	}
	if got, _ := PermExact(0o750).Apply(0o7777, false); got != 0o750 {
		t.Errorf("PermExact apply = %v", got)
	}
	if err := (PermChange{Set: 0o100, Clear: 0o100}).Validate(); err == nil {
		t.Error("overlapping set/clear accepted")
	}
	if err := (PermChange{Expr: "u+?"}).Validate(); err == nil {
		t.Error("bad expression accepted")
	}
	got, err := PermChange{Set: 0o200, Expr: "a+X"}.Apply(0o400, true)
	if err != nil || got != 0o711 {
		t.Errorf("set then expr = %o, %v", got, err)
	}
	if len(PermBits) != 12 || PermBitName(fs.ModeSticky) != "sticky" || PermBitName(0o400) != "owner read" {
		t.Error("PermBits table wrong")
	}
}
