// These tests cover mask parsing and rendering: every placeholder of the
// Total Commander Multi-Rename Tool, the character-range forms, inline
// counters, inline case switches, built-in fields and the literal-bracket
// escapes, plus the error messages for malformed masks.

package multirename

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var testTime = time.Date(2024, time.March, 7, 9, 5, 3, 0, time.UTC)

func newItem(name string) Item {
	return Item{Dir: "/photos/2024/trip", Name: name, Size: 5 << 20, ModTime: testTime}
}

// render compiles a name mask (extension mask "[E]") and renders it for the
// item at position pos in a list of n items.
func renderOne(t *testing.T, s Settings, items []Item, pos int) string {
	t.Helper()
	r, err := Compile(s)
	if err != nil {
		t.Fatalf("compile %+v: %v", s, err)
	}
	names, _ := r.NewNames(items)
	return names[pos]
}

func TestMaskPlaceholders(t *testing.T) {
	cases := []struct {
		mask, ext, name, want string
	}{
		{"[N]", "[E]", "holiday.jpg", "holiday.jpg"},
		{"[N]_[C]", "[E]", "file.txt", "file_1.txt"},
		{"[N1]", "[E]", "holiday.jpg", "h.jpg"},
		{"[N2-5]", "[E]", "holiday.jpg", "olid.jpg"},
		{"[N2,3]", "[E]", "holiday.jpg", "oli.jpg"},
		{"[N3-]", "[E]", "holiday.jpg", "liday.jpg"},
		{"[N-3,2]", "[E]", "holiday.jpg", "da.jpg"},
		{"[N-5-]", "[E]", "holiday.jpg", "liday.jpg"},
		{"[N-4-2]", "[E]", "holiday.jpg", "ida.jpg"},
		{"[N-4--2]", "[E]", "holiday.jpg", "ida.jpg"},
		{"[N2--2]", "[E]", "holiday.jpg", "olida.jpg"},
		{"[N-8,5]", "[E]", "abcdefghij", "cdefg"},
		{"[N20]", "[E]", "abc.txt", ".txt"},
		{"[N2-99]", "[E]", "abc.txt", "bc.txt"},
		{"[N-99,2]", "[E]", "abc.txt", "ab.txt"},
		{"[N]", "[E1]", "a.jpeg", "a.j"},
		{"[N]", "[E2-3]", "a.jpeg", "a.pe"},
		{"[N]", "", "a.jpeg", "a"},
		{"[N]", "bak", "a.jpeg", "a.bak"},
		{"[E]_[N]", "[N]", "a.jpeg", "jpeg_a.a"},
		{"[P]-[N]", "[E]", "x.png", "trip-x.png"},
		{"[G]-[P]-[N]", "[E]", "x.png", "2024-trip-x.png"},
		{"[P1-2]", "[E]", "x.png", "tr.png"},
		{"[Y][M][D]", "[E]", "x.png", "20240307.png"},
		{"[y]-[M]", "[E]", "x.png", "24-03.png"},
		{"[h][m][s]", "[E]", "x.png", "090503.png"},
		{"[d] [t]", "[E]", "x.png", "2024-03-07 09.05.03.png"},
		{"[[][N][]]", "[E]", "x.png", "[x].png"},
		{"a]b", "[E]", "x.png", "a]b.png"},
		{"[U][N]", "[E]", "Hello World.txt", "HELLO WORLD.txt"},
		{"[L][N]", "[U][E]", "Hello World.txt", "hello world.TXT"},
		{"[F][N]", "[E]", "hELLO wORLD don't.txt", "Hello World Don't.txt"},
		{"x[U][N2-][n]Y", "[E]", "abc.txt", "xBCY.txt"},
		{"[N1][F][N2-]", "[E]", "abc def.txt", "abc Def.txt"},
		{"[=tc.size]", "[E]", "x.png", "5242880.png"},
		{"[=size.m]MB", "[E]", "x.png", "5MB.png"},
		{"[=SIZE.K]", "[E]", "x.png", "5120.png"},
		{"[=weekday]", "[E]", "x.png", "Thursday.png"},
		{"[=monthname] [=day]", "[E]", "x.png", "March 07.png"},
		{"[=week]-[=dayofyear]", "[E]", "x.png", "10-067.png"},
		{"[=type]", "[E]", "x.png", "file.png"},
		{"[=fullname]", "", "x.png", "x.png"},
		{"[=index]of[=count]", "[E]", "x.png", "1of1.png"},
		{"", "[E]", "x.png", ".png"},
		{"[N]", "[E]", ".bashrc", ".bashrc"},
		{"[N]_new", "[E]", ".bashrc", ".bashrc_new"},
		{"[N]", "[E]", "noext", "noext"},
		{"[N]", "[E]", "árvíztűrő.txt", "árvíztűrő.txt"},
		{"[N2-4]", "[E]", "árvíztűrő.txt", "rví.txt"},
	}
	for _, c := range cases {
		s := DefaultSettings()
		s.NameMask, s.ExtMask = c.mask, c.ext
		got := renderOne(t, s, []Item{newItem(c.name)}, 0)
		if got != c.want {
			t.Errorf("mask %q / %q on %q = %q, want %q", c.mask, c.ext, c.name, got, c.want)
		}
	}
}

func TestMaskCounters(t *testing.T) {
	items := make([]Item, 12)
	for i := range items {
		items[i] = newItem("f.txt")
	}
	cases := []struct {
		mask                 string
		start, step, digits  int
		want0, want1, want11 string
	}{
		{"[C]", 1, 1, 1, "1", "2", "12"},
		{"[C]", 1, 1, 3, "001", "002", "012"},
		{"[C]", 0, 5, 2, "00", "05", "55"},
		{"[C10+5:3]", 1, 1, 1, "010", "015", "065"},
		{"[C+2]", 1, 1, 1, "1", "3", "23"},
		{"[C:4]", 7, 1, 1, "0007", "0008", "0018"},
		{"[C100]", 1, 1, 1, "100", "101", "111"},
		{"[C]", 1, 1, AutoDigits, "01", "02", "12"},
		{"[C:a]", 1, 1, 5, "01", "02", "12"},
		{"[C5+-1]", 1, 1, 2, "05", "04", "-06"},
		{"[Ca]", 1, 1, 1, "a", "b", "l"},
		{"[CA+2]", 1, 1, 1, "A", "C", "W"},
		{"[Cy]", 1, 1, 1, "y", "z", "aj"},
		{"[C]-[C]", 1, 1, 2, "01-01", "02-02", "12-12"},
	}
	for _, c := range cases {
		s := DefaultSettings()
		s.NameMask, s.ExtMask = c.mask, ""
		s.CounterStart, s.CounterStep, s.CounterDigits = c.start, c.step, c.digits
		r, err := Compile(s)
		if err != nil {
			t.Fatalf("%q: %v", c.mask, err)
		}
		names, _ := r.NewNames(items)
		if names[0] != c.want0 || names[1] != c.want1 || names[11] != c.want11 {
			t.Errorf("%q start=%d step=%d digits=%d: got %q %q %q, want %q %q %q",
				c.mask, c.start, c.step, c.digits, names[0], names[1], names[11], c.want0, c.want1, c.want11)
		}
	}
}

func TestCounterResetPerFolder(t *testing.T) {
	items := []Item{
		{Dir: "/a", Name: "x.txt"}, {Dir: "/a", Name: "y.txt"},
		{Dir: "/b", Name: "x.txt"}, {Dir: "/a", Name: "z.txt"},
	}
	s := DefaultSettings()
	s.NameMask = "[C]"
	s.CounterResetPerFolder = true
	r, _ := Compile(s)
	got, _ := r.NewNames(items)
	want := []string{"1.txt", "2.txt", "1.txt", "3.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
	s.CounterResetPerFolder = false
	r, _ = Compile(s)
	got, _ = r.NewNames(items)
	if got[2] != "3.txt" {
		t.Errorf("without reset got %v", got)
	}
}

func TestMaskErrors(t *testing.T) {
	cases := []struct{ mask, want string }{
		{"[N", "missing closing bracket"},
		{"[]", "empty placeholder"},
		{"[X]", "unknown placeholder [X]"},
		{"[Nx]", "bad character range"},
		{"[N2-x]", "bad character range"},
		{"[N2,-1]", "bad character count"},
		{"[N2;3]", "bad character range"},
		{"[C1+x]", "bad counter step"},
		{"[C:-1]", "bad counter width"},
		{"[Cab3]", "bad counter"},
		{"[CaB]", "mixed-case"},
		{"[=]", "field name missing"},
		{"[YM]", "unknown placeholder"},
	}
	for _, c := range cases {
		_, err := CompileMask(c.mask)
		var me *MaskError
		if !errors.As(err, &me) {
			t.Errorf("%q: want MaskError, got %v", c.mask, err)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: error %q does not mention %q", c.mask, err, c.want)
		}
	}
	s := DefaultSettings()
	s.ExtMask = "[Q]"
	_, err := Compile(s)
	var se *SettingsError
	if !errors.As(err, &se) || se.Field != "extMask" {
		t.Errorf("want SettingsError for extMask, got %v", err)
	}
}

func TestUnknownFieldWarns(t *testing.T) {
	s := DefaultSettings()
	s.NameMask = "[N][=exif.model]"
	it := newItem("a.jpg")
	r, _ := Compile(s)
	names, warns := r.NewNames([]Item{it})
	if names[0] != "a.jpg" || len(warns[0]) != 1 {
		t.Errorf("got %q %v", names[0], warns[0])
	}
	it.Fields = map[string]string{"EXIF.Model": "X100"}
	names, warns = r.NewNames([]Item{it})
	if names[0] != "aX100.jpg" || len(warns[0]) != 0 {
		t.Errorf("caller field: got %q %v", names[0], warns[0])
	}
	it.Fields = map[string]string{"size": "override"}
	s.NameMask = "[=tc.size]"
	r, _ = Compile(s)
	names, _ = r.NewNames([]Item{it})
	if names[0] != "5242880.jpg" {
		// "tc.size" is not the key "size" in Fields, so the built-in wins.
		t.Errorf("got %q", names[0])
	}
}

func TestFieldNames(t *testing.T) {
	names := FieldNames()
	if len(names) < 20 || names[0] > names[1] {
		t.Errorf("unexpected field list %v", names)
	}
}

func TestDirectoriesAndDateFormats(t *testing.T) {
	dir := Item{Dir: "/x", Name: "My.Project", IsDir: true, ModTime: testTime}
	s := DefaultSettings()
	s.NameMask = "[N]_old"
	if got := renderOne(t, s, []Item{dir}, 0); got != "My.Project_old" {
		t.Errorf("dir without split: %q", got)
	}
	s.SplitDirExt = true
	if got := renderOne(t, s, []Item{dir}, 0); got != "My_old.Project" {
		t.Errorf("dir with split: %q", got)
	}
	s = DefaultSettings()
	s.NameMask = "[d]_[t]"
	s.DateFormat, s.TimeFormat = "02.01.2006", "15h04"
	if got := renderOne(t, s, []Item{newItem("a.b")}, 0); got != "07.03.2024_09h05.b" {
		t.Errorf("custom formats: %q", got)
	}
	root := Item{Dir: "/", Name: "a.txt"}
	s = DefaultSettings()
	s.NameMask = "[G][P][N]"
	if got := renderOne(t, s, []Item{root}, 0); got != "a.txt" {
		t.Errorf("root parents: %q", got)
	}
}

func TestCharRangeApply(t *testing.T) {
	cases := []struct {
		spec, in, want string
	}{
		{"1", "", ""},
		{"0", "abc", "a"},
		{"3,0", "abcdef", ""},
		{"5-2", "abcdef", ""},
		{"-1", "abc", "c"},
		{"-1-", "abc", "c"},
		{"-3--1", "abcdef", "def"},
	}
	for _, c := range cases {
		r, err := parseRange(c.spec)
		if err != nil {
			t.Fatalf("%q: %v", c.spec, err)
		}
		if got := r.apply(c.in); got != c.want {
			t.Errorf("[N%s] on %q = %q, want %q", c.spec, c.in, got, c.want)
		}
	}
}

func TestAlphaCounterHelpers(t *testing.T) {
	for n := 1; n < 2000; n++ {
		if alphaToNum(numToAlpha(n)) != n {
			t.Fatalf("round trip failed at %d (%q)", n, numToAlpha(n))
		}
	}
	if numToAlpha(0) != "a" || alphaToNum("A") != 0 || numToAlpha(27) != "aa" {
		t.Error("edge cases")
	}
	if padInt(-5, 3) != "-005" || padInt(1234, 2) != "1234" {
		t.Error("padInt")
	}
}
