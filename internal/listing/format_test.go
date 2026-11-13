// Tests for size, date and attribute formatting.
package listing

import (
	"io/fs"
	"testing"
	"time"
)

func TestFormatSize(t *testing.T) {
	eu := NumberFormat{Thousands: ".", Decimal: ","}
	tests := []struct {
		n    int64
		unit SizeUnit
		nf   NumberFormat
		want string
	}{
		{0, SizeBytes, NumberFormat{}, "0"},
		{999, SizeBytes, NumberFormat{}, "999"},
		{1234567, SizeBytes, NumberFormat{}, "1,234,567"},
		{1234567, SizeBytes, eu, "1.234.567"},
		{1234567, SizeBytes, NumberFormat{Thousands: " ", Decimal: "."}, "1 234 567"},
		{1234567, SizeRaw, NumberFormat{}, "1234567"},
		{0, SizeKB, NumberFormat{}, "0 k"},
		{1, SizeKB, NumberFormat{}, "1 k"},
		{1024, SizeKB, NumberFormat{}, "1 k"},
		{1025, SizeKB, NumberFormat{}, "2 k"},
		{1234567, SizeKB, NumberFormat{}, "1,206 k"},
		{1234567, SizeMB, NumberFormat{}, "1.2 M"},
		{1234567, SizeMB, eu, "1,2 M"},
		{1, SizeMB, NumberFormat{}, "0.1 M"},
		{10 << 20, SizeMB, NumberFormat{}, "10.0 M"},
		{10 << 20, SizeGB, NumberFormat{}, "0.01 G"},
		{5 << 30, SizeGB, NumberFormat{}, "5.00 G"},
		{999, SizeDynamic, NumberFormat{}, "999 b"},
		{1240, SizeDynamic, NumberFormat{}, "1.21 k"},
		{12390, SizeDynamic, NumberFormat{}, "12.1 k"},
		{123900, SizeDynamic, NumberFormat{}, "121 k"},
		{1234567, SizeDynamic, NumberFormat{}, "1.18 M"},
		{10234, SizeDynamic, NumberFormat{}, "9.99 k"},
		{10239, SizeDynamic, NumberFormat{}, "10.0 k"},
		{3 << 40, SizeDynamic, NumberFormat{}, "3.00 T"},
	}
	for _, tt := range tests {
		if got := FormatSize(tt.n, tt.unit, tt.nf); got != tt.want {
			t.Errorf("FormatSize(%d, %d) = %q, want %q", tt.n, tt.unit, got, tt.want)
		}
	}
}

func TestParseSizeUnit(t *testing.T) {
	tests := map[string]SizeUnit{"bytes": SizeBytes, "K": SizeKB, "Mbytes": SizeMB, "g": SizeGB, "bkMG": SizeDynamic, "raw": SizeRaw}
	for in, want := range tests {
		got, ok := ParseSizeUnit(in)
		if !ok || got != want {
			t.Errorf("ParseSizeUnit(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseSizeUnit("parsecs"); ok {
		t.Error("unknown unit accepted")
	}
}

func TestSizeText(t *testing.T) {
	f := &Format{}
	up := UpItem("/x")
	dir := Item{Name: "d", Attr: AttrDir, DirSize: -1}
	sized := Item{Name: "d", Attr: AttrDir, DirSize: 2048}
	file := Item{Name: "f", Size: 1500}
	for _, c := range []struct {
		it   Item
		want string
	}{{up, "<UP--DIR>"}, {dir, "<DIR>"}, {sized, "2,048"}, {file, "1,500"}} {
		if got := f.SizeText(&c.it); got != c.want {
			t.Errorf("SizeText(%s) = %q want %q", c.it.Name, got, c.want)
		}
	}
	g := &Format{Size: SizeKB, DirLabel: "[DIR]"}
	if got := g.SizeText(&dir); got != "[DIR]" {
		t.Errorf("custom dir label: %q", got)
	}
	if got := g.SizeText(&sized); got != "2 k" {
		t.Errorf("kb dir: %q", got)
	}
}

func TestFormatDate(t *testing.T) {
	tm := time.Date(2026, 3, 7, 14, 5, 9, 123456789, time.UTC)
	tests := []struct {
		pic, want string
	}{
		{"", "2026-03-07 14:05"},
		{DatePictureISO, "2026-03-07 14:05:09"},
		{DatePictureEurope, "07.03.2026 14:05"},
		{DatePictureUS, "03/07/2026 02:05 PM"},
		{DatePictureTCShort, "07.03.26 14:05"},
		{"d/M/yy h:m t", "7/3/26 2:5 P"},
		{"ddd, dd MMM yyyy", "Sat, 07 Mar 2026"},
		{"dddd MMMM", "Saturday March"},
		{"HH:mm:ss.fff", "14:05:09.123"},
		{"yyyy 'at' HH'h'", "2026 at 14h"},
		{"hh tt", "02 PM"},
	}
	for _, tt := range tests {
		if got := FormatDate(tm, tt.pic, time.UTC); got != tt.want {
			t.Errorf("FormatDate(%q) = %q want %q", tt.pic, got, tt.want)
		}
	}
	if FormatDate(time.Time{}, "", nil) != "" {
		t.Error("zero time not empty")
	}
	midnight := time.Date(2026, 1, 1, 0, 30, 0, 0, time.UTC)
	if got := FormatDate(midnight, "h:mm tt", time.UTC); got != "12:30 AM" {
		t.Errorf("midnight 12h = %q", got)
	}
}

func TestAttrString(t *testing.T) {
	tests := []struct {
		name  string
		it    Item
		style AttrStyle
		want  string
	}{
		{"win none", Item{}, AttrStyleWindows, "----"},
		{"win ra", Item{Attr: AttrReadOnly | AttrArchive}, AttrStyleWindows, "ra--"},
		{"win all", Item{Attr: AttrReadOnly | AttrArchive | AttrHidden | AttrSystem}, AttrStyleWindows, "rahs"},
		{"win full", Item{Attr: AttrArchive | AttrCompressed | AttrEncrypted}, AttrStyleWindowsFull, "-a--ce"},
		{"unix file", Item{Mode: 0o644}, AttrStyleUnix, "-rw-r--r--"},
		{"unix dir", Item{Mode: fs.ModeDir | 0o755, Attr: AttrDir}, AttrStyleUnix, "drwxr-xr-x"},
		{"unix link", Item{Mode: fs.ModeSymlink | 0o777, Attr: AttrLink}, AttrStyleUnix, "lrwxrwxrwx"},
		{"unix setuid", Item{Mode: fs.ModeSetuid | 0o755}, AttrStyleUnix, "-rwsr-xr-x"},
		{"unix setuid no x", Item{Mode: fs.ModeSetuid | 0o644}, AttrStyleUnix, "-rwSr--r--"},
		{"unix sticky", Item{Mode: fs.ModeDir | fs.ModeSticky | 0o777, Attr: AttrDir}, AttrStyleUnix, "drwxrwxrwt"},
		{"unix pipe", Item{Mode: fs.ModeNamedPipe | 0o600}, AttrStyleUnix, "prw-------"},
		{"octal", Item{Mode: 0o755}, AttrStyleOctal, "0755"},
		{"octal setgid sticky", Item{Mode: fs.ModeSetgid | fs.ModeSticky | 0o750}, AttrStyleOctal, "3750"},
	}
	for _, tt := range tests {
		if got := AttrString(&tt.it, tt.style); got != tt.want {
			t.Errorf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestGroupDigits(t *testing.T) {
	for in, want := range map[string]string{"1": "1", "123": "123", "1234": "1,234", "-1234567": "-1,234,567", "123456": "123,456"} {
		if got := GroupDigits(in, ","); got != want {
			t.Errorf("GroupDigits(%q) = %q want %q", in, got, want)
		}
	}
}
