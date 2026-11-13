// Tests for splitting files into parts and combining them again: part names
// and sizes, the .crc info file, automatic part sizes with disk changes,
// finding parts, detecting damaged or missing parts, and size parsing.

package fileutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"1457664", 1457664, true},
		{"1.44 MB", Size1_44M, true},
		{"1.44mb", Size1_44M, true},
		{"700 MB CD", SizeCD700, true},
		{"4.7 GB DVD", SizeDVD, true},
		{"Automatic", SizeAuto, true},
		{"auto", SizeAuto, true},
		{"100k", 100 << 10, true},
		{"100 KB", 100 << 10, true},
		{"2M", 2 << 20, true},
		{"1,5 GB", 3 << 29, true},
		{"1.5g", 3 << 29, true},
		{"1TiB", 1 << 40, true},
		{"10 bytes", 10, true},
		{"", 0, false},
		{"MB", 0, false},
		{"0", 0, false},
		{"12 parsecs", 0, false},
		{"1.2.3M", 0, false},
	}
	for _, tt := range tests {
		got, err := ParseSize(tt.in)
		if (err == nil) != tt.ok || got != tt.want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d ok=%v", tt.in, got, err, tt.want, tt.ok)
		}
	}
	for _, p := range SizePresets {
		if got, err := ParseSize(p.Name); err != nil || got != p.Size {
			t.Errorf("preset %q parses to %d, %v", p.Name, got, err)
		}
	}
}

func TestPartNamesAndCount(t *testing.T) {
	if got := PartName("/x", "/src/movie.mkv", 1); got != filepath.Join("/x", "movie.001") {
		t.Errorf("PartName = %s", got)
	}
	if got := PartName("/x", "/src/noext", 1000); got != filepath.Join("/x", "noext.1000") {
		t.Errorf("PartName = %s", got)
	}
	if got := PartName("/x", "/src/.hidden", 2); got != filepath.Join("/x", ".hidden.002") {
		t.Errorf("PartName = %s", got)
	}
	if got := CRCFileName("/x", "/src/a.tar.gz"); got != filepath.Join("/x", "a.tar.crc") {
		t.Errorf("CRCFileName = %s", got)
	}
	counts := []struct {
		size, part int64
		want       int
	}{{0, 10, 1}, {1, 10, 1}, {10, 10, 1}, {11, 10, 2}, {100, 10, 10}, {5, 0, 0}}
	for _, c := range counts {
		if got := PartCount(c.size, c.part); got != c.want {
			t.Errorf("PartCount(%d,%d) = %d", c.size, c.part, got)
		}
	}
}

func writeData(t *testing.T, dir, name string, n int) (string, []byte) {
	t.Helper()
	data := pattern251(n)
	for i := range data {
		data[i] ^= byte(i / 251) // make parts differ from each other
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, data
}

func TestSplitAndCombine(t *testing.T) {
	tests := []struct {
		size, part int64
		parts      int
	}{
		{0, 10, 1},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{1000, 100, 10},
		{3<<20 + 5, 1 << 20, 4},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_by_%d", tt.size, tt.part), func(t *testing.T) {
			src := t.TempDir()
			out := t.TempDir()
			p, data := writeData(t, src, "file.bin", int(tt.size))
			res, err := SplitFile(context.Background(), SplitOptions{Source: p, TargetDir: out, PartSize: tt.part, Format: FormatUnix})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Parts) != tt.parts {
				t.Fatalf("parts = %v", res.Parts)
			}
			var joined []byte
			for i, part := range res.Parts {
				if want := PartName(out, p, i+1); part != want {
					t.Errorf("part %d = %s, want %s", i+1, part, want)
				}
				b, _ := os.ReadFile(part)
				if i < len(res.Parts)-1 && int64(len(b)) != tt.part {
					t.Errorf("part %d is %d bytes", i+1, len(b))
				}
				joined = append(joined, b...)
			}
			if !bytes.Equal(joined, data) {
				t.Fatal("parts do not add up to the source")
			}
			wantCRC := fmt.Sprintf("filename=file.bin\nsize=%d\ncrc32=%08X\n", tt.size, crc32.ChecksumIEEE(data))
			if got := readFile(t, res.CRCFile); got != wantCRC {
				t.Errorf(".crc = %q, want %q", got, wantCRC)
			}

			dst := t.TempDir()
			cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0], TargetDir: dst})
			if err != nil {
				t.Fatal(err)
			}
			if !cr.Checked || cr.Target != filepath.Join(dst, "file.bin") || cr.Size != tt.size {
				t.Errorf("combine result = %+v", cr)
			}
			if got, _ := os.ReadFile(cr.Target); !bytes.Equal(got, data) {
				t.Error("combined file differs from the source")
			}
		})
	}
}

func TestSplitRefusals(t *testing.T) {
	dir := t.TempDir()
	p, _ := writeData(t, dir, "a.bin", 100)
	if _, err := SplitFile(context.Background(), SplitOptions{Source: p, PartSize: 0}); err == nil {
		t.Error("zero part size accepted")
	}
	if _, err := SplitFile(context.Background(), SplitOptions{Source: p, PartSize: SizeAuto}); err == nil {
		t.Error("auto without FreeSpace accepted")
	}
	if _, err := SplitFile(context.Background(), SplitOptions{Source: dir, PartSize: 10}); err == nil {
		t.Error("splitting a folder accepted")
	}
	if _, err := SplitFile(context.Background(), SplitOptions{Source: p, PartSize: 10, TargetDir: filepath.Join(dir, "nope")}); err == nil {
		t.Error("missing target folder accepted")
	}
	// An existing part blocks the split up front, nothing is written.
	os.WriteFile(filepath.Join(dir, "a.005"), []byte("keep"), 0o644)
	_, err := SplitFile(context.Background(), SplitOptions{Source: p, PartSize: 10})
	if !errors.Is(err, os.ErrExist) {
		t.Errorf("err = %v, want exists", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.001")); !os.IsNotExist(err) {
		t.Error("a part was written although the split was refused")
	}
	if _, err := SplitFile(context.Background(), SplitOptions{Source: p, PartSize: 10, Overwrite: true}); err != nil {
		t.Errorf("overwrite: %v", err)
	}
	// A source that itself looks like a part must not be overwritten.
	q, _ := writeData(t, dir, "b.001", 100)
	if _, err := SplitFile(context.Background(), SplitOptions{Source: q, PartSize: 10, Overwrite: true}); err == nil {
		t.Error("split would overwrite its own source")
	}
}

func TestSplitNoCRCAndCancel(t *testing.T) {
	dir := t.TempDir()
	p, data := writeData(t, dir, "a.bin", 3<<20)
	res, err := SplitFile(context.Background(), SplitOptions{Source: p, PartSize: 1 << 20, NoCRCFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.CRCFile != "" || res.Info.CRC32 != crc32.ChecksumIEEE(data) {
		t.Errorf("result = %+v", res)
	}
	if _, err := os.Stat(CRCFileName(dir, p)); !os.IsNotExist(err) {
		t.Error(".crc written although NoCRCFile was set")
	}
	cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[1], TargetName: "joined.bin"})
	if err != nil {
		t.Fatal(err)
	}
	if cr.Checked || cr.Info != nil || len(cr.Parts) != 3 {
		t.Errorf("combine without .crc = %+v", cr)
	}
	if got, _ := os.ReadFile(cr.Target); !bytes.Equal(got, data) {
		t.Error("combined data differs")
	}

	out := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	_, err = SplitFile(ctx, SplitOptions{Source: p, TargetDir: out, PartSize: 1 << 20, Progress: func(Progress) {
		n++
		if n == 2 {
			cancel()
		}
	}})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("err = %v", err)
	}
	if left, _ := os.ReadDir(out); len(left) != 0 {
		t.Errorf("cancelled split left %d files", len(left))
	}
}

func TestSplitAutomatic(t *testing.T) {
	dir := t.TempDir()
	out := t.TempDir()
	p, data := writeData(t, dir, "a.bin", 300<<10)
	// Simulate three disks with different free space; BeforePart sees the
	// disk change requests in order.
	free := []int64{autoReserve + 100<<10, autoReserve + 150<<10, autoReserve + 1<<20}
	var asked []int
	disk := -1
	res, err := SplitFile(context.Background(), SplitOptions{
		Source: p, TargetDir: out, PartSize: SizeAuto,
		BeforePart: func(i int, path string) error { asked = append(asked, i); disk++; return nil },
		FreeSpace:  func(string) (int64, error) { return free[disk], nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Parts) != 3 || fmt.Sprint(asked) != "[1 2 3]" {
		t.Fatalf("parts %v, asked %v", res.Parts, asked)
	}
	sizes := []int64{100 << 10, 150 << 10, 50 << 10}
	for i, part := range res.Parts {
		if s, _ := fileSize(part); s != sizes[i] {
			t.Errorf("part %d is %d bytes, want %d", i+1, s, sizes[i])
		}
	}
	cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0], TargetDir: t.TempDir()})
	if err != nil || !cr.Checked {
		t.Fatalf("combine: %v %+v", err, cr)
	}
	if got, _ := os.ReadFile(cr.Target); !bytes.Equal(got, data) {
		t.Error("combined data differs")
	}

	// A full disk stops the split with a clear error and cleans up.
	out2 := t.TempDir()
	_, err = SplitFile(context.Background(), SplitOptions{
		Source: p, TargetDir: out2, PartSize: SizeAuto,
		FreeSpace: func(string) (int64, error) { return 10, nil },
	})
	if err == nil || !strings.Contains(err.Error(), "free space") {
		t.Errorf("err = %v", err)
	}
	// A BeforePart error (user pressed Cancel at the disk prompt) aborts.
	stop := errors.New("user stopped")
	_, err = SplitFile(context.Background(), SplitOptions{
		Source: p, TargetDir: t.TempDir(), PartSize: 100 << 10,
		BeforePart: func(i int, _ string) error {
			if i == 2 {
				return stop
			}
			return nil
		},
	})
	if !errors.Is(err, stop) {
		t.Errorf("err = %v", err)
	}
}

func TestCombineChecks(t *testing.T) {
	setup := func(t *testing.T) (string, *SplitResult, []byte) {
		dir := t.TempDir()
		p, data := writeData(t, dir, "orig.dat", 1000)
		out := t.TempDir()
		res, err := SplitFile(context.Background(), SplitOptions{Source: p, TargetDir: out, PartSize: 300})
		if err != nil {
			t.Fatal(err)
		}
		return out, res, data
	}

	t.Run("damaged part", func(t *testing.T) {
		out, res, _ := setup(t)
		b, _ := os.ReadFile(res.Parts[1])
		b[5] ^= 1
		os.WriteFile(res.Parts[1], b, 0o644)
		cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0]})
		if !errors.Is(err, ErrCombineMismatch) || !strings.Contains(err.Error(), "CRC32") {
			t.Fatalf("err = %v", err)
		}
		if cr == nil || cr.Checked {
			t.Fatalf("result = %+v", cr)
		}
		if _, err := os.Stat(filepath.Join(out, "orig.dat")); err != nil {
			t.Error("the joined file should be kept for rescue")
		}
	})

	t.Run("missing middle part", func(t *testing.T) {
		out, res, _ := setup(t)
		os.Remove(res.Parts[2])
		_, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0]})
		if err == nil || !strings.Contains(err.Error(), "orig.003 is missing") {
			t.Fatalf("err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(out, "orig.dat")); !os.IsNotExist(err) {
			t.Error("incomplete output should be removed")
		}
	})

	t.Run("missing part from another disk", func(t *testing.T) {
		out, res, data := setup(t)
		elsewhere := filepath.Join(t.TempDir(), "orig.003")
		os.Rename(res.Parts[2], elsewhere)
		var asked []string
		cr, err := CombineFiles(context.Background(), CombineOptions{
			FirstPart: res.Parts[0],
			NextPart: func(i int, expected string) (string, error) {
				asked = append(asked, fmt.Sprintf("%d:%s", i, filepath.Base(expected)))
				if i == 3 {
					return elsewhere, nil
				}
				return expected, nil
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(asked) != "[3:orig.003]" || !cr.Checked {
			t.Errorf("asked %v, result %+v", asked, cr)
		}
		if got, _ := os.ReadFile(filepath.Join(out, "orig.dat")); !bytes.Equal(got, data) {
			t.Error("combined data differs")
		}
	})

	t.Run("start from any part or the crc file", func(t *testing.T) {
		_, res, data := setup(t)
		for _, start := range []string{res.Parts[2], res.CRCFile} {
			dst := t.TempDir()
			cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: start, TargetDir: dst})
			if err != nil || !cr.Checked {
				t.Fatalf("start %s: %v", start, err)
			}
			if got, _ := os.ReadFile(cr.Target); !bytes.Equal(got, data) {
				t.Errorf("start %s: data differs", start)
			}
		}
	})

	t.Run("explicit list in chosen order", func(t *testing.T) {
		_, res, data := setup(t)
		dst := t.TempDir()
		cr, err := CombineFiles(context.Background(), CombineOptions{Parts: res.Parts, TargetDir: dst, NoCRCCheck: true, TargetName: "x.dat"})
		if err != nil || cr.Checked {
			t.Fatalf("%v %+v", err, cr)
		}
		if got, _ := os.ReadFile(filepath.Join(dst, "x.dat")); !bytes.Equal(got, data) {
			t.Error("data differs")
		}
		rev := []string{res.Parts[1], res.Parts[0]}
		cr, err = CombineFiles(context.Background(), CombineOptions{Parts: rev, TargetDir: dst, TargetName: "y.dat", NoCRCCheck: true})
		if err != nil || cr.Size != 600 {
			t.Fatalf("%v %+v", err, cr)
		}
		got, _ := os.ReadFile(filepath.Join(dst, "y.dat"))
		if !bytes.Equal(got[:300], data[300:600]) {
			t.Error("explicit order not kept")
		}
	})

	t.Run("refuses to overwrite", func(t *testing.T) {
		out, res, _ := setup(t)
		os.WriteFile(filepath.Join(out, "orig.dat"), []byte("keep"), 0o644)
		_, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0]})
		if !errors.Is(err, os.ErrExist) {
			t.Errorf("err = %v", err)
		}
		if readFile(t, filepath.Join(out, "orig.dat")) != "keep" {
			t.Error("existing file was touched")
		}
		_, err = CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0], TargetName: "orig.002", Overwrite: true})
		if err == nil {
			t.Error("combining onto one of the parts was allowed")
		}
	})

	t.Run("hostile name in crc file", func(t *testing.T) {
		out, res, _ := setup(t)
		os.WriteFile(res.CRCFile, []byte(strings.Replace(readFile(t, res.CRCFile), "filename=orig.dat", `filename=..\..\evil.dat`, 1)), 0o644)
		cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: res.Parts[0]})
		if err != nil {
			t.Fatal(err)
		}
		if cr.Target != filepath.Join(out, "evil.dat") {
			t.Errorf("target = %s", cr.Target)
		}
	})

	t.Run("not a part", func(t *testing.T) {
		dir := t.TempDir()
		p, _ := writeData(t, dir, "x.txt", 10)
		if _, err := CombineFiles(context.Background(), CombineOptions{FirstPart: p}); err == nil {
			t.Error("combining x.txt accepted")
		}
		if _, err := CombineFiles(context.Background(), CombineOptions{}); err == nil {
			t.Error("empty options accepted")
		}
	})
}

func TestFindPartsSevenZipStyleAndWide(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"arc.zip.001", "arc.zip.002", "arc.zip.003"} {
		os.WriteFile(filepath.Join(dir, n), []byte(n), 0o644)
	}
	parts, err := FindParts(filepath.Join(dir, "arc.zip.002"))
	if err != nil || len(parts) != 3 || filepath.Base(parts[0]) != "arc.zip.001" {
		t.Fatalf("parts = %v, %v", parts, err)
	}
	cr, err := CombineFiles(context.Background(), CombineOptions{FirstPart: parts[0]})
	if err != nil || filepath.Base(cr.Target) != "arc.zip" {
		t.Fatalf("%v %+v", err, cr)
	}
	if readFile(t, cr.Target) != "arc.zip.001arc.zip.002arc.zip.003" {
		t.Error("joined content wrong")
	}

	// Beyond 999 parts the number grows to four digits.
	wide := t.TempDir()
	os.WriteFile(filepath.Join(wide, "w.998"), []byte("a"), 0o644)
	os.WriteFile(filepath.Join(wide, "w.999"), []byte("b"), 0o644)
	os.WriteFile(filepath.Join(wide, "w.1000"), []byte("c"), 0o644)
	parts, err = FindParts(filepath.Join(wide, "w.998"))
	if err != nil || len(parts) != 3 || filepath.Base(parts[2]) != "w.1000" {
		t.Errorf("wide parts = %v %v", parts, err)
	}
}

func TestCRCInfo(t *testing.T) {
	good := []string{
		"filename=a.bin\nsize=10\ncrc32=0000ABCD\n",
		"\xef\xbb\xbfFILENAME=a.bin\r\nSize=10\r\nCRC32=abcd\r\n",
		"crc32=ABCD\nsize=10\nfilename=a.bin",
	}
	for _, g := range good {
		c, err := ParseCRCInfo(strings.NewReader(g))
		if err != nil || c.FileName != "a.bin" || c.Size != 10 || c.CRC32 != 0xABCD {
			t.Errorf("ParseCRCInfo(%q) = %+v, %v", g, c, err)
		}
	}
	bad := []string{"", "filename=a\nsize=1\n", "filename=a\nsize=x\ncrc32=1\n", "filename=a\nsize=1\ncrc32=zz\n", "filename=\nsize=1\ncrc32=1\n", "filename=a\nsize=-1\ncrc32=1\n"}
	for _, b := range bad {
		if _, err := ParseCRCInfo(strings.NewReader(b)); err == nil {
			t.Errorf("ParseCRCInfo(%q) accepted", b)
		}
	}
	c := CRCInfo{FileName: "x", Size: 5, CRC32: 0x1F}
	if got := c.Format(false); got != "filename=x\r\nsize=5\r\ncrc32=0000001F\r\n" {
		t.Errorf("Format = %q", got)
	}
}
