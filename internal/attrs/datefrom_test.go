package attrs

import (
	"testing"
	"time"
)

func TestDateFromName(t *testing.T) {
	at := func(y, mo, d, h, mi, s int) time.Time {
		return time.Date(y, time.Month(mo), d, h, mi, s, 0, time.Local)
	}
	cases := map[string]time.Time{
		"IMG_20240115_093000.jpg":               at(2024, 1, 15, 9, 30, 0),
		"Screenshot 2024-01-15 at 09.30.00.png": at(2024, 1, 15, 9, 30, 0),
		"2024-01-15T09:30:00 notes.txt":         at(2024, 1, 15, 9, 30, 0),
		"report 2023_12_31.pdf":                 at(2023, 12, 31, 0, 0, 0),
		"VID-20221005-WA0001.mp4":               at(2022, 10, 5, 0, 0, 0),
		"scan 1999.05.06 1230.tif":              at(1999, 5, 6, 12, 30, 0),
	}
	for name, want := range cases {
		got, ok := DateFromName(name)
		if !ok || !got.Equal(want) {
			t.Errorf("%s = %v, %v; want %v", name, got, ok, want)
		}
	}
	for _, name := range []string{"notes.txt", "2023-02-30.txt", "20241315.jpg", "2024-0115.txt", "id 120240115.txt", "1850-01-01.txt"} {
		if got, ok := DateFromName(name); ok {
			t.Errorf("%s: no date expected, got %v", name, got)
		}
	}
}
