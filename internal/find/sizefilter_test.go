// These tests cover SizeFilter.Match, in particular values so large that
// multiplying them by the unit would overflow int64 (issue #91). Such a
// filter must behave as "bigger than any file", not wrap around to a
// negative limit that matches everything or nothing.
package find

import (
	"math"
	"testing"
)

func TestSizeFilterMatchBasics(t *testing.T) {
	cases := []struct {
		f    SizeFilter
		size int64
		want bool
	}{
		{SizeFilter{Op: SizeEqual, Value: 3, Unit: KB}, 3072, true},
		{SizeFilter{Op: SizeEqual, Value: 3, Unit: KB}, 4095, true},
		{SizeFilter{Op: SizeEqual, Value: 3, Unit: KB}, 4096, false},
		{SizeFilter{Op: SizeEqual, Value: 3, Unit: KB}, 3071, false},
		{SizeFilter{Op: SizeGreater, Value: 3, Unit: KB}, 3072, false},
		{SizeFilter{Op: SizeGreater, Value: 3, Unit: KB}, 3073, true},
		{SizeFilter{Op: SizeLess, Value: 3, Unit: KB}, 3071, true},
		{SizeFilter{Op: SizeLess, Value: 3, Unit: KB}, 3072, false},
		{SizeFilter{Op: SizeEqual, Value: 10}, 10, true},
		{SizeFilter{Op: SizeGreater, Value: 0}, 0, false},
		{SizeFilter{Op: SizeGreater, Value: 0}, 1, true},
		{SizeFilter{Op: SizeLess, Value: 0}, 0, false},
		{SizeFilter{Op: SizeGreater, Value: -1}, 0, true},
		{SizeFilter{}, 123, true},
		{SizeFilter{Op: SizeEqual, Value: 1, Unit: "PB"}, 1, false},
	}
	for _, c := range cases {
		if got := c.f.Match(c.size); got != c.want {
			t.Errorf("%+v.Match(%d) = %v, want %v", c.f, c.size, got, c.want)
		}
	}
}

func TestSizeFilterMatchNoOverflow(t *testing.T) {
	huge := int64(math.MaxInt64 / 1024) // times 1 TB overflows int64
	sizes := []int64{0, 1, 1 << 40, math.MaxInt64}
	for _, size := range sizes {
		if (SizeFilter{Op: SizeGreater, Value: huge, Unit: TB}).Match(size) {
			t.Errorf("> %d TB matched %d bytes", huge, size)
		}
		if !(SizeFilter{Op: SizeLess, Value: huge, Unit: TB}).Match(size) {
			t.Errorf("< %d TB did not match %d bytes", huge, size)
		}
		if (SizeFilter{Op: SizeEqual, Value: huge, Unit: TB}).Match(size) {
			t.Errorf("= %d TB matched %d bytes", huge, size)
		}
		if !(SizeFilter{Op: SizeGreater, Value: math.MinInt64, Unit: TB}).Match(size) {
			t.Errorf("> MinInt64 TB did not match %d bytes", size)
		}
	}
}
