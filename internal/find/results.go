// This file turns a result list into text, for "Copy results to clipboard" and "Save
// results to file" (F-838). Total Commander writes one full path per line; the
// optional details add size and date columns separated by tabs, which paste cleanly
// into a spreadsheet. Duplicate searches get an empty line between groups so the
// grouping survives the trip through the clipboard. The UI does the clipboard part;
// this code only produces the text.

package find

import (
	"bufio"
	"io"
	"strconv"
	"time"
)

// WriteOptions controls WriteResults.
type WriteOptions struct {
	Details bool   // add size and modification time columns
	Newline string // line ending; empty means "\n" ("\r\n" for Windows-style files)
}

// WriteResults writes the results as text, one per line.
func WriteResults(w io.Writer, results []Result, o WriteOptions) error {
	nl := o.Newline
	if nl == "" {
		nl = "\n"
	}
	bw := bufio.NewWriter(w)
	group := 0
	for i, r := range results {
		if r.DupGroup != 0 && r.DupGroup != group {
			if i > 0 {
				bw.WriteString(nl)
			}
			group = r.DupGroup
		}
		bw.WriteString(r.Path)
		if o.Details {
			bw.WriteByte('\t')
			if r.IsDir {
				bw.WriteString("<DIR>")
			} else {
				bw.WriteString(strconv.FormatInt(r.Size, 10))
			}
			bw.WriteByte('\t')
			bw.WriteString(r.ModTime.Local().Format(time.DateTime))
		}
		bw.WriteString(nl)
	}
	return bw.Flush()
}
