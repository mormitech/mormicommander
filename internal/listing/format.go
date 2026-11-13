// This file formats the three kinds of value a file row shows: sizes, dates and attribute strings. Total Commander lets
// the user pick the size display (plain bytes with thousands separators, kilobytes, megabytes, gigabytes, or a
// "dynamic" unit that changes with the size), a date/time picture, and on Unix-like systems a permission string instead
// of Windows' "rahs" letters. The same formatters back the Full view and the custom column fields ([=tc.size.k]), so a
// size looks identical wherever it appears.

package listing

import (
	"io/fs"
	"math"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// SizeUnit selects how a byte count is shown.
type SizeUnit int

const (
	SizeBytes   SizeUnit = iota // "1,234,567"
	SizeKB                      // "1,206 k" (rounded up, so a 1-byte file is "1 k", like TC)
	SizeMB                      // "1.2 M"
	SizeGB                      // "0.01 G"
	SizeDynamic                 // "999 b", "1.21 k", "12.1 k", "121 k", "1.18 M", ... (3 significant digits)
	SizeRaw                     // "1234567": no separators, for scripts and copy-to-clipboard
)

// NumberFormat holds the separators used when printing numbers.
type NumberFormat struct {
	Thousands string // group separator, "," by default; "" for none
	Decimal   string // decimal point, "." by default
}

// DefaultNumberFormat is used when a Format leaves NumberFormat empty.
var DefaultNumberFormat = NumberFormat{Thousands: ",", Decimal: "."}

// Format collects every display option the formatters need. The zero value is usable and gives TC's defaults on the
// current platform.
type Format struct {
	Size       SizeUnit
	Number     NumberFormat // zero value means DefaultNumberFormat
	Date       string       // date picture, see FormatDate; "" means DefaultDatePicture
	Location   *time.Location
	AttrStyle  AttrStyle
	DirExt     bool   // show the extension of folder names in the ext column instead of keeping the name whole
	DirLabel   string // text in the size column of a folder without a computed size; "" means "<DIR>"
	UpLabel    string // text in the size column of the ".." entry; "" means "<UP--DIR>" (TC's label)
	LinkSuffix bool   // append " -> target" to link names in the name field
}

func (f *Format) num() NumberFormat {
	if f.Number == (NumberFormat{}) {
		return DefaultNumberFormat
	}
	return f.Number
}

// GroupDigits inserts the thousands separator into a decimal integer string (which may start with '-').
func GroupDigits(digits, sep string) string {
	if sep == "" {
		return digits
	}
	neg := strings.HasPrefix(digits, "-")
	if neg {
		digits = digits[1:]
	}
	if len(digits) <= 3 {
		if neg {
			return "-" + digits
		}
		return digits
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		b.WriteString(sep)
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// formatFloat prints v with prec decimals, grouping the integer part.
func formatFloat(v float64, prec int, nf NumberFormat) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	ip, fp, _ := strings.Cut(s, ".")
	out := GroupDigits(ip, nf.Thousands)
	if fp != "" {
		out += nf.Decimal + fp
	}
	return out
}

// FormatSize renders n bytes in the given unit.
func FormatSize(n int64, unit SizeUnit, nf NumberFormat) string {
	if nf == (NumberFormat{}) {
		nf = DefaultNumberFormat
	}
	const k = 1024.0
	switch unit {
	case SizeRaw:
		return strconv.FormatInt(n, 10)
	case SizeKB:
		return GroupDigits(strconv.FormatInt(ceilDiv(n, 1024), 10), nf.Thousands) + " k"
	case SizeMB:
		return formatFloat(roundUp(float64(n)/(k*k), 1), 1, nf) + " M"
	case SizeGB:
		return formatFloat(roundUp(float64(n)/(k*k*k), 2), 2, nf) + " G"
	case SizeDynamic:
		if n < 1024 {
			return strconv.FormatInt(n, 10) + " b"
		}
		v := float64(n)
		units := []string{"k", "M", "G", "T", "P", "E"}
		i := -1
		for v >= 1024 && i < len(units)-1 {
			v /= 1024
			i++
		}
		prec := 0
		switch {
		case v < 10:
			prec = 2
		case v < 100:
			prec = 1
		}
		// Rounding can push 9.999 to "10.00"; re-pick the precision for the rounded value.
		r := math.Round(v*math.Pow10(prec)) / math.Pow10(prec)
		if prec == 2 && r >= 10 {
			prec = 1
		} else if prec == 1 && r >= 100 {
			prec = 0
		}
		return formatFloat(v, prec, nf) + " " + units[i]
	}
	return GroupDigits(strconv.FormatInt(n, 10), nf.Thousands)
}

func ceilDiv(n, d int64) int64 {
	if n <= 0 {
		return 0
	}
	return (n + d - 1) / d
}

// roundUp rounds v up to prec decimals so a non-empty file never displays as zero.
func roundUp(v float64, prec int) float64 {
	p := math.Pow10(prec)
	r := math.Ceil(v*p-1e-9) / p
	if r < 0 {
		return 0
	}
	return r
}

// ParseSizeUnit reads a unit name as used in custom column fields and the configuration: "bytes"/"b", "kbytes"/"k",
// "mbytes"/"m", "gbytes"/"g", "dynamic"/"d"/"bkm"/"bkmg", "raw". Matching ignores case.
func ParseSizeUnit(s string) (SizeUnit, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "default":
		return SizeBytes, false
	case "b", "bytes", "byte":
		return SizeBytes, true
	case "k", "kb", "kbytes", "kilobytes":
		return SizeKB, true
	case "m", "mb", "mbytes", "megabytes":
		return SizeMB, true
	case "g", "gb", "gbytes", "gigabytes":
		return SizeGB, true
	case "d", "dyn", "dynamic", "bkm", "bkmg", "bkmgt", "float":
		return SizeDynamic, true
	case "raw", "n", "number":
		return SizeRaw, true
	}
	return SizeBytes, false
}

// SizeText is the size column of an item: the byte count for files, the computed size or "<DIR>" for folders, and
// TC's "<UP--DIR>" for "..".
func (f *Format) SizeText(it *Item) string {
	if it.IsUp() {
		if f.UpLabel != "" {
			return f.UpLabel
		}
		return "<UP--DIR>"
	}
	if it.IsDir() {
		if it.DirSize >= 0 {
			return FormatSize(it.DirSize, f.Size, f.num())
		}
		if f.DirLabel != "" {
			return f.DirLabel
		}
		return "<DIR>"
	}
	return FormatSize(it.Size, f.Size, f.num())
}

// DefaultDatePicture is the date format used when none is configured: unambiguous everywhere and it sorts as text.
const DefaultDatePicture = "yyyy-MM-dd HH:mm"

// Date pictures matching common TC/system settings.
const (
	DatePictureISO     = "yyyy-MM-dd HH:mm:ss"
	DatePictureEurope  = "dd.MM.yyyy HH:mm"
	DatePictureUS      = "MM/dd/yyyy hh:mm tt"
	DatePictureUK      = "dd/MM/yyyy HH:mm"
	DatePictureTCShort = "dd.MM.yy HH:mm"
)

// FormatDate renders t with a Windows-style date picture, the syntax TC and the Windows control panel use:
//
//	d dd ddd dddd   day of month (1, 01), weekday (Mon, Monday)
//	M MM MMM MMMM   month (1, 01, Jan, January)
//	yy yyyy         year (26, 2026)
//	h hh / H HH     hour 12-hour / 24-hour
//	m mm            minute
//	s ss            second
//	f..fffffffff    fractions of a second
//	t tt            A/P, AM/PM
//	'text'          literal text; any other character is copied as is
//
// A zero time renders as an empty string, so a source without creation dates leaves the column blank.
func FormatDate(t time.Time, picture string, loc *time.Location) string {
	if t.IsZero() {
		return ""
	}
	if picture == "" {
		picture = DefaultDatePicture
	}
	if loc != nil {
		t = t.In(loc)
	}
	var b strings.Builder
	p := picture
	for len(p) > 0 {
		c := p[0]
		if c == '\'' {
			end := strings.IndexByte(p[1:], '\'')
			if end < 0 {
				b.WriteString(p[1:])
				break
			}
			b.WriteString(p[1 : 1+end])
			p = p[end+2:]
			continue
		}
		n := 1
		for n < len(p) && p[n] == c {
			n++
		}
		switch c {
		case 'd':
			switch {
			case n == 1:
				b.WriteString(strconv.Itoa(t.Day()))
			case n == 2:
				b.WriteString(pad2(t.Day()))
			case n == 3:
				b.WriteString(t.Weekday().String()[:3])
			default:
				b.WriteString(t.Weekday().String())
			}
		case 'M':
			switch {
			case n == 1:
				b.WriteString(strconv.Itoa(int(t.Month())))
			case n == 2:
				b.WriteString(pad2(int(t.Month())))
			case n == 3:
				b.WriteString(t.Month().String()[:3])
			default:
				b.WriteString(t.Month().String())
			}
		case 'y':
			if n <= 2 {
				b.WriteString(pad2(t.Year() % 100))
			} else {
				b.WriteString(strconv.Itoa(t.Year()))
			}
		case 'h':
			h := t.Hour() % 12
			if h == 0 {
				h = 12
			}
			b.WriteString(padN(h, n))
		case 'H':
			b.WriteString(padN(t.Hour(), n))
		case 'm':
			b.WriteString(padN(t.Minute(), n))
		case 's':
			b.WriteString(padN(t.Second(), n))
		case 'f':
			if n > 9 {
				n = 9
			}
			frac := strconv.Itoa(t.Nanosecond() + 1e9)[1:]
			b.WriteString(frac[:n])
		case 't':
			ap := "AM"
			if t.Hour() >= 12 {
				ap = "PM"
			}
			if n == 1 {
				ap = ap[:1]
			}
			b.WriteString(ap)
		default:
			b.WriteString(p[:n])
		}
		p = p[n:]
	}
	return b.String()
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func padN(n, width int) string {
	if width >= 2 {
		return pad2(n)
	}
	return strconv.Itoa(n)
}

// DateText formats the item's modification time with the configured picture.
func (f *Format) DateText(it *Item) string { return FormatDate(it.ModTime, f.Date, f.Location) }

// AttrStyle selects how attributes are written.
type AttrStyle int

const (
	AttrStyleAuto        AttrStyle = iota // Windows letters on Windows, Unix permissions elsewhere
	AttrStyleWindows                      // "rahs" with '-' for unset bits, as in TC's Full view
	AttrStyleWindowsFull                  // "rahsce": adds compressed and encrypted
	AttrStyleUnix                         // "drwxr-xr-x"
	AttrStyleOctal                        // "0755"
)

func (s AttrStyle) resolve() AttrStyle {
	if s != AttrStyleAuto {
		return s
	}
	if runtime.GOOS == "windows" {
		return AttrStyleWindows
	}
	return AttrStyleUnix
}

// AttrString renders an item's attributes in the given style.
func AttrString(it *Item, style AttrStyle) string {
	switch style.resolve() {
	case AttrStyleWindows, AttrStyleWindowsFull:
		full := style.resolve() == AttrStyleWindowsFull
		b := []byte("----")
		if full {
			b = []byte("------")
		}
		set := func(i int, a Attr, c byte) {
			if it.Attr&a != 0 {
				b[i] = c
			}
		}
		set(0, AttrReadOnly, 'r')
		set(1, AttrArchive, 'a')
		set(2, AttrHidden, 'h')
		set(3, AttrSystem, 's')
		if full {
			set(4, AttrCompressed, 'c')
			set(5, AttrEncrypted, 'e')
		}
		return string(b)
	case AttrStyleOctal:
		m := it.Mode
		v := uint32(m.Perm())
		if m&fs.ModeSetuid != 0 {
			v |= 0o4000
		}
		if m&fs.ModeSetgid != 0 {
			v |= 0o2000
		}
		if m&fs.ModeSticky != 0 {
			v |= 0o1000
		}
		s := strconv.FormatUint(uint64(v), 8)
		for len(s) < 4 {
			s = "0" + s
		}
		return s
	}
	return unixPermString(it)
}

// unixPermString writes ls -l style permissions. Unlike fs.FileMode.String it uses one type letter (d, l, p, s, c, b)
// and the s/S/t/T conventions for setuid, setgid and sticky, which is what users of ls and mc expect to read.
func unixPermString(it *Item) string {
	m := it.Mode
	b := []byte("----------")
	switch {
	case m&fs.ModeSymlink != 0 || it.Attr&AttrLink != 0:
		b[0] = 'l'
	case m&fs.ModeDir != 0 || it.Attr&AttrDir != 0:
		b[0] = 'd'
	case m&fs.ModeNamedPipe != 0:
		b[0] = 'p'
	case m&fs.ModeSocket != 0:
		b[0] = 's'
	case m&fs.ModeCharDevice != 0:
		b[0] = 'c'
	case m&fs.ModeDevice != 0:
		b[0] = 'b'
	}
	const rwx = "rwxrwxrwx"
	p := m.Perm()
	for i := 0; i < 9; i++ {
		if p&(1<<uint(8-i)) != 0 {
			b[1+i] = rwx[i]
		}
	}
	special := func(i int, on bool, lower, upper byte) {
		if !on {
			return
		}
		if b[i] == 'x' {
			b[i] = lower
		} else {
			b[i] = upper
		}
	}
	special(3, m&fs.ModeSetuid != 0, 's', 'S')
	special(6, m&fs.ModeSetgid != 0, 's', 'S')
	special(9, m&fs.ModeSticky != 0, 't', 'T')
	return string(b)
}

// AttrText is the attribute column of an item in the configured style.
func (f *Format) AttrText(it *Item) string {
	if it.IsUp() {
		return ""
	}
	return AttrString(it, f.AttrStyle)
}
