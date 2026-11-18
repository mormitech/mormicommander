package attrs

import (
	"regexp"
	"strconv"
	"time"
)

// dateInName finds a date, with an optional time, in a file name: 2024-01-15, 2024_01_15, 20240115, then a time
// such as 09-30-00, 09.30.00, 093000 after a space, "_", "-" or "T" (IMG_20240115_093000.jpg, "Screenshot 2024-01-15
// at 09.30.00.png", 2024-01-15T09:30:00 notes.txt). The digits must not run on into more digits.
var dateInName = regexp.MustCompile(`(?:^|\D)((?:19|20)\d\d)([-_.]?)(\d\d)([-_.]?)(\d\d)(?:(?:[ _T-]|\s*at\s*)(\d\d)[-.:]?(\d\d)(?:[-.:]?(\d\d))?)?(?:\D|$)`)

// DateFromName gives the date and time written in a file name, in local time (F-340: Change attributes, modified date
// from the file name). ok is false when the name holds no valid date. Without a time in the name the time is midnight.
func DateFromName(name string) (t time.Time, ok bool) {
	for _, m := range dateInName.FindAllStringSubmatch(name, -1) {
		if m[2] != m[4] { // 2024-0115 is no date
			continue
		}
		n := func(s string) int { v, _ := strconv.Atoi(s); return v }
		y, mo, d := n(m[1]), n(m[3]), n(m[5])
		h, mi, s := 0, 0, 0
		if m[6] != "" {
			h, mi, s = n(m[6]), n(m[7]), n(m[8])
		}
		if mo < 1 || mo > 12 || d < 1 || d > 31 || h > 23 || mi > 59 || s > 59 {
			continue
		}
		t = time.Date(y, time.Month(mo), d, h, mi, s, 0, time.Local)
		if t.Day() != d { // 2023-02-30
			continue
		}
		return t, true
	}
	return time.Time{}, false
}
