// The readers of the line-based formats: CSV and TSV (the first row names the columns) and JSON Lines (one JSON object
// per line; every key becomes a column, in the order the keys are first met). Both read from a plain io.Reader, so the
// caller decides how the bytes are decoded, and stop after MaxRows rows.

package table

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// ReadDelimited reads CSV (comma ',') or TSV (comma '\t'). Quotes are taken loosely and rows may have any number of
// fields, as real files are often a little broken: a row longer than the header gets extra columns named by number.
func ReadDelimited(r io.Reader, comma rune) (*Table, error) {
	cr := csv.NewReader(r)
	cr.Comma = comma
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true
	format := "csv"
	if comma == '\t' {
		format = "tsv"
	}
	t := &Table{Format: format}
	head, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return t, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the first row can't be read: %v", err)
	}
	for i, h := range head {
		if i == 0 {
			h = trimBOM(h)
		}
		t.Columns = append(t.Columns, Column{Name: clip(h)})
	}
	bad := 0
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) && !errors.Is(pe.Err, csv.ErrFieldCount) {
				bad++
				if bad > 100 {
					return nil, fmt.Errorf("the file does not look like %s: %v", format, err)
				}
				continue
			}
			return nil, err
		}
		if len(t.Rows) == MaxRows {
			t.Truncated = true
			break
		}
		for len(t.Columns) < len(rec) {
			t.Columns = append(t.Columns, Column{Name: "Column " + strconv.Itoa(len(t.Columns)+1)})
		}
		row := make([]*string, len(rec))
		for i, f := range rec {
			row[i] = strp(clip(f))
		}
		t.Rows = append(t.Rows, row)
	}
	if bad > 0 {
		t.Note = fmt.Sprintf("%d rows could not be read and were left out.", bad)
	}
	inferTypes(t)
	return t, nil
}

func trimBOM(s string) string {
	if len(s) >= 3 && s[:3] == "\xef\xbb\xbf" {
		return s[3:]
	}
	return s
}

// ReadJSONL reads JSON Lines. A line that is not a JSON object (a number, an array, broken JSON) is shown in a column
// named "value"; blank lines are skipped.
func ReadJSONL(r io.Reader) (*Table, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	t := &Table{Format: "jsonl"}
	index := map[string]int{}
	types := []string{}
	col := func(name string) int {
		i, ok := index[name]
		if !ok {
			i = len(t.Columns)
			index[name] = i
			t.Columns = append(t.Columns, Column{Name: clip(name)})
			types = append(types, "")
		}
		return i
	}
	bad := 0
	first := true
	for {
		line, err := br.ReadBytes('\n')
		if first {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			first = false
		}
		if s := bytes.TrimSpace(line); len(s) > 0 {
			if len(t.Rows) == MaxRows {
				t.Truncated = true
				break
			}
			row := []*string{}
			set := func(i int, v string, typ string) {
				for len(row) <= i {
					row = append(row, nil)
				}
				row[i] = strp(clip(v))
				types[i] = mergeJSON(types[i], typ)
			}
			var obj map[string]json.RawMessage
			if s[0] == '{' && json.Unmarshal(s, &obj) == nil {
				for _, k := range keyOrder(s) {
					raw, ok := obj[k]
					if !ok {
						continue
					}
					v, typ := jsonCell(raw)
					if typ == "null" {
						col(k) // a null shows as an empty (NULL) cell
						continue
					}
					set(col(k), v, typ)
				}
			} else {
				if !json.Valid(s) {
					bad++
				}
				v, typ := jsonCell(s)
				set(col("value"), v, typ)
			}
			t.Rows = append(t.Rows, row)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	for i := range t.Columns {
		if types[i] == "" {
			types[i] = "text"
		}
		t.Columns[i].Type = types[i]
	}
	if bad > 0 {
		t.Note = fmt.Sprintf("%d lines are not valid JSON; they are shown as text in the \"value\" column.", bad)
	}
	return t, nil
}

// jsonCell turns one JSON value into the text of a cell and its type: a string without quotes, other values as JSON.
func jsonCell(raw []byte) (string, string) {
	raw = bytes.TrimSpace(raw)
	if !json.Valid(raw) {
		return string(raw), "text"
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s, "text"
		}
	case 't', 'f':
		return string(raw), "boolean"
	case 'n':
		return "", "null"
	case '{':
		return string(raw), "object"
	case '[':
		return string(raw), "array"
	default:
		if isInteger(string(raw)) {
			return string(raw), "integer"
		}
		return string(raw), "number"
	}
	return string(raw), "text"
}

// mergeJSON combines the JSON types of a column like merge does, where mixed kinds become "mixed".
func mergeJSON(have, next string) string {
	switch {
	case next == "" || have == next:
		return have
	case have == "":
		return next
	case (have == "integer" && next == "number") || (have == "number" && next == "integer"):
		return "number"
	}
	return "mixed"
}

// keyOrder lists the top-level keys of a JSON object in the order they are written (a Go map forgets it).
func keyOrder(obj []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(obj))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		k, ok := tok.(string)
		if !ok {
			return keys
		}
		keys = append(keys, k)
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}
