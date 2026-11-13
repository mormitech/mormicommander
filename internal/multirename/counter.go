// This file implements the counter used by the [C] placeholder. The dialog
// has three counter fields (start, step and number of digits) and a "reset
// per folder" switch for branch views; a single [C] can override them inline
// as [C10+5:3]. Besides decimal numbers we accept a letter start such as
// [Ca] or [CA], which counts a, b, ..., z, aa, ab like spreadsheet columns,
// and a width of "a" ([C:a] or Digits set to AutoDigits) which pads every
// number to the width of the largest one so the names sort correctly.

package multirename

import (
	"strconv"
	"strings"
)

// AutoDigits as the Digits setting pads counters to the width of the largest
// value in the list.
const AutoDigits = -1

// MaxCounterDigits caps the counter width. No file system accepts a name part
// longer than 255 bytes, and without a cap typing [C:2000000000] in the live
// preview would build a 2 GB string and take the whole app down.
const MaxCounterDigits = 255

// counter renders the counter for the current item (whose 0-based position
// in its counter group is c.index) from the inline overrides of one [C]
// placeholder and the dialog settings.
func (c *renderCtx) counter(cs *counterSpec) string {
	s := c.settings
	start, step, digits := s.CounterStart, s.CounterStep, s.CounterDigits
	alpha, upper, auto := false, false, digits == AutoDigits
	if cs != nil {
		if cs.start != nil {
			start, alpha, upper = *cs.start, cs.alpha, cs.upper
		}
		if cs.step != nil {
			step = *cs.step
		}
		if cs.digits != nil {
			digits, auto = *cs.digits, false
		}
		if cs.auto {
			auto = true
		}
	}
	v := start + step*c.index
	if alpha {
		txt := numToAlpha(v)
		if upper {
			txt = strings.ToUpper(txt)
		}
		return txt
	}
	if auto {
		last := start + step*(c.total-1)
		digits = max(len(strconv.Itoa(abs(start))), len(strconv.Itoa(abs(last))))
	}
	return padInt(v, digits)
}

// padInt formats v with at least digits digits, keeping a minus sign in
// front of the zeros.
func padInt(v, digits int) string {
	neg := v < 0
	s := strconv.Itoa(abs(v))
	digits = min(digits, MaxCounterDigits)
	if len(s) < digits {
		s = strings.Repeat("0", digits-len(s)) + s
	}
	if neg {
		s = "-" + s
	}
	return s
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// alphaToNum converts a lower-case letter counter ("a", "z", "aa") to its
// bijective base-26 value (1, 26, 27). It returns 0 for invalid input.
func alphaToNum(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'a' || c > 'z' {
			return 0
		}
		n = n*26 + int(c-'a'+1)
		if n > 1<<40 {
			return 0
		}
	}
	return n
}

// numToAlpha is the inverse of alphaToNum. Values below 1 cannot be written
// as letters and come out as "a" so a negative step never produces an empty
// name part.
func numToAlpha(n int) string {
	if n < 1 {
		return "a"
	}
	var b []byte
	for n > 0 {
		n--
		b = append([]byte{byte('a' + n%26)}, b...)
		n /= 26
	}
	return string(b)
}
