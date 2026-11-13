// When Total Commander splits a file it also writes a small text file with
// the same base name and the extension .crc. It records the original file
// name, its size and its CRC32, so that Combine can restore the right name
// and prove that the parts were joined correctly and none was damaged or left
// out. The format is three "key=value" lines; this file reads and writes it,
// accepting TC's files (CRLF, any key order, upper or lower case) as well.

package fileutil

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// CRCInfo is the content of a split info (.crc) file.
type CRCInfo struct {
	FileName string // original file name, without folder
	Size     int64
	CRC32    uint32
}

// Format returns the file content with the given line ending.
func (c CRCInfo) Format(unix bool) string {
	nl := newlineFor(unix)
	return fmt.Sprintf("filename=%s%ssize=%d%scrc32=%08X%s", c.FileName, nl, c.Size, nl, c.CRC32, nl)
}

// ReadCRCInfo parses a .crc split info file.
func ReadCRCInfo(path string) (CRCInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return CRCInfo{}, err
	}
	defer f.Close()
	return ParseCRCInfo(f)
}

// ParseCRCInfo parses split info from r. All three keys are required.
func ParseCRCInfo(r io.Reader) (CRCInfo, error) {
	var c CRCInfo
	var haveName, haveSize, haveCRC bool
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if first {
			line = strings.TrimPrefix(line, "\xef\xbb\xbf")
			first = false
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "filename":
			c.FileName = v
			haveName = v != ""
		case "size":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				return c, fmt.Errorf("split info: bad size %q", v)
			}
			c.Size, haveSize = n, true
		case "crc32", "crc":
			n, err := strconv.ParseUint(v, 16, 32)
			if err != nil {
				return c, fmt.Errorf("split info: bad CRC32 %q", v)
			}
			c.CRC32, haveCRC = uint32(n), true
		}
	}
	if err := sc.Err(); err != nil {
		return c, err
	}
	if !haveName || !haveSize || !haveCRC {
		return c, fmt.Errorf("split info: filename, size and crc32 are all required")
	}
	return c, nil
}
