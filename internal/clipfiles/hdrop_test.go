// These tests check the Windows CF_HDROP and "Preferred DropEffect" encoders byte by byte, since a wrong offset or a
// missing terminator would make Explorer paste nothing, and they check that the decoder also reads the old ANSI form
// and survives truncated or corrupt blocks from other programs.

package clipfiles

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestHDROPRoundTrip(t *testing.T) {
	for _, paths := range [][]string{
		{`C:\a.txt`},
		{`C:\dir\one.txt`, `D:\ünï cödé\😀.png`, `\\srv\share\x`},
	} {
		b := EncodeHDROP(paths)
		if off := binary.LittleEndian.Uint32(b); off != 20 {
			t.Fatalf("pFiles = %d, want 20", off)
		}
		if wide := binary.LittleEndian.Uint32(b[16:]); wide != 1 {
			t.Fatalf("fWide = %d, want 1", wide)
		}
		if tail := b[len(b)-4:]; !reflect.DeepEqual(tail, []byte{0, 0, 0, 0}) {
			t.Fatalf("block must end with two UTF-16 NULs, got % x", tail)
		}
		got, err := DecodeHDROP(b)
		if err != nil || !reflect.DeepEqual(got, paths) {
			t.Fatalf("DecodeHDROP = %q, %v; want %q", got, err, paths)
		}
	}
}

func TestHDROPExactBytes(t *testing.T) {
	b := EncodeHDROP([]string{"A"})
	want := []byte{20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 'A', 0, 0, 0, 0, 0}
	if !reflect.DeepEqual(b, want) {
		t.Fatalf("EncodeHDROP = % x\nwant          % x", b, want)
	}
}

func TestHDROPDecodeVariants(t *testing.T) {
	ansi := append(make([]byte, 20), []byte("C:\\x.txt\x00D:\\y\x00\x00")...)
	binary.LittleEndian.PutUint32(ansi, 20)
	got, err := DecodeHDROP(ansi)
	if err != nil || !reflect.DeepEqual(got, []string{`C:\x.txt`, `D:\y`}) {
		t.Fatalf("ANSI DecodeHDROP = %q, %v", got, err)
	}

	tests := []struct {
		name string
		b    []byte
	}{
		{"short", []byte{1, 2, 3}},
		{"bad offset", func() []byte { b := EncodeHDROP([]string{"x"}); binary.LittleEndian.PutUint32(b, 9999); return b }()},
		{"offset in header", func() []byte { b := EncodeHDROP([]string{"x"}); binary.LittleEndian.PutUint32(b, 4); return b }()},
	}
	for _, tt := range tests {
		if _, err := DecodeHDROP(tt.b); err == nil {
			t.Errorf("%s: want error", tt.name)
		}
	}

	// A block cut off before its terminators still yields the complete name.
	b := EncodeHDROP([]string{"abc"})
	got, err = DecodeHDROP(b[:len(b)-4])
	if err != nil || !reflect.DeepEqual(got, []string{"abc"}) {
		t.Fatalf("truncated DecodeHDROP = %q, %v", got, err)
	}
}

func TestDropEffect(t *testing.T) {
	tests := []struct {
		b    []byte
		want Op
	}{
		{EncodeDropEffect(Copy), Copy},
		{EncodeDropEffect(Cut), Cut},
		{[]byte{5, 0, 0, 0}, Copy}, // Explorer's COPY|LINK
		{[]byte{2, 0, 0, 0}, Cut},
		{[]byte{3, 0, 0, 0}, Cut},
		{[]byte{2}, Copy}, // too short
		{nil, Copy},
	}
	for _, tt := range tests {
		if got := DecodeDropEffect(tt.b); got != tt.want {
			t.Errorf("DecodeDropEffect(% x) = %v, want %v", tt.b, got, tt.want)
		}
	}
	if b := EncodeDropEffect(Cut); binary.LittleEndian.Uint32(b) != 2 {
		t.Fatalf("cut must be DROPEFFECT_MOVE (2), got % x", b)
	}
}
