// Reading the column list out of a CREATE TABLE statement, as the SQLite reader needs it: each column's name and
// declared type, whether it is stored in the record (generated VIRTUAL columns are not), and whether it is the
// "INTEGER PRIMARY KEY" column that SQLite keeps as the rowid instead of in the record. It is a small tokenizer, not a
// full SQL parser: it only has to understand the statements SQLite itself saved in the schema table.

package table

import "strings"

// schemaCol is one column of a CREATE TABLE statement.
type schemaCol struct {
	name, typ string
	stored    bool // the value is in the record
	rowid     bool // INTEGER PRIMARY KEY: the value is the rowid
	real      bool // REAL affinity: whole numbers are stored as integers but are floating point values
	pk        int  // place in the primary key, from 1 (0 = not part of it); a WITHOUT ROWID record starts with these
}

// token is one SQL token; quoted is true for a quoted identifier or string, whose text is already unquoted.
type token struct {
	text   string
	quoted bool
}

func (t token) is(word string) bool { return !t.quoted && strings.EqualFold(t.text, word) }

// tokenize splits SQL into tokens, leaving out spaces and comments.
func tokenize(sql string) []token {
	var out []token
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '-' && i+1 < len(sql) && sql[i+1] == '-':
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(sql) && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return out
			}
			i += end + 4
		case c == '"' || c == '\'' || c == '`' || c == '[':
			closer := c
			if c == '[' {
				closer = ']'
			}
			var b strings.Builder
			j := i + 1
			for j < len(sql) {
				if sql[j] == closer {
					if closer != ']' && j+1 < len(sql) && sql[j+1] == closer {
						b.WriteByte(closer) // a doubled quote stands for one
						j += 2
						continue
					}
					break
				}
				b.WriteByte(sql[j])
				j++
			}
			out = append(out, token{text: b.String(), quoted: true})
			i = j + 1
		case c == '(' || c == ')' || c == ',' || c == ';':
			out = append(out, token{text: string(c)})
			i++
		case isWordByte(c):
			j := i
			for j < len(sql) && isWordByte(sql[j]) {
				j++
			}
			out = append(out, token{text: sql[i:j]})
			i = j
		default:
			out = append(out, token{text: string(c)})
			i++
		}
	}
	return out
}

func isWordByte(c byte) bool {
	return c == '_' || c == '$' || c == '.' || c >= 0x80 || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// columnStops are the words that end a column's type and start its constraints.
var columnStops = map[string]bool{"CONSTRAINT": true, "PRIMARY": true, "NOT": true, "NULL": true, "UNIQUE": true, "CHECK": true,
	"DEFAULT": true, "COLLATE": true, "REFERENCES": true, "GENERATED": true, "AS": true}

// tableConstraints are the words that start a table constraint instead of a column.
var tableConstraints = map[string]bool{"CONSTRAINT": true, "PRIMARY": true, "UNIQUE": true, "CHECK": true, "FOREIGN": true}

// parseCreateTable reads the columns of a CREATE TABLE statement. ok is false when there is no column list (CREATE
// TABLE ... AS SELECT) or the statement can't be read.
func parseCreateTable(sql string) (cols []schemaCol, withoutRowid, ok bool) {
	toks := tokenize(sql)
	open := -1
	for i, t := range toks {
		if t.is("AS") && open < 0 {
			return nil, false, false
		}
		if !t.quoted && t.text == "(" {
			open = i
			break
		}
	}
	if open < 0 {
		return nil, false, false
	}
	// split the list at its top-level commas
	var defs [][]token
	var cur []token
	depth, close := 0, -1
	for i := open + 1; i < len(toks) && close < 0; i++ {
		t := toks[i]
		switch {
		case !t.quoted && t.text == "(":
			depth++
		case !t.quoted && t.text == ")":
			if depth == 0 {
				close = i
				continue
			}
			depth--
		case !t.quoted && t.text == "," && depth == 0:
			defs = append(defs, cur)
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if close < 0 {
		return nil, false, false
	}
	defs = append(defs, cur)
	for i := close + 1; i+1 < len(toks); i++ {
		if toks[i].is("WITHOUT") && toks[i+1].is("ROWID") {
			withoutRowid = true
		}
	}
	var tablePK []string
	for _, d := range defs {
		if len(d) == 0 {
			continue
		}
		if !d[0].quoted && tableConstraints[strings.ToUpper(d[0].text)] {
			if pk := primaryKeyColumns(d); pk != nil {
				tablePK = pk
			}
			continue
		}
		c := schemaCol{name: d[0].text, stored: true}
		var typ []string
		j := 1
		for ; j < len(d); j++ {
			if !d[j].quoted && columnStops[strings.ToUpper(d[j].text)] {
				break
			}
			typ = append(typ, d[j].text)
		}
		c.typ = joinType(typ)
		c.real = realAffinity(c.typ)
		generated, storedGen := false, false
		for k := j; k < len(d); k++ {
			switch {
			case d[k].is("AS"):
				generated = true
			case d[k].is("STORED"):
				storedGen = true
			case d[k].is("PRIMARY") && k+1 < len(d) && d[k+1].is("KEY"):
				desc := k+2 < len(d) && d[k+2].is("DESC")
				c.rowid = strings.EqualFold(c.typ, "INTEGER") && !desc
				c.pk = 1
			}
		}
		if generated && !storedGen {
			c.stored = false
		}
		cols = append(cols, c)
	}
	for k, name := range tablePK {
		for i := range cols {
			if strings.EqualFold(cols[i].name, name) && cols[i].pk == 0 {
				cols[i].pk = k + 1
			}
		}
	}
	if len(tablePK) == 1 {
		for i := range cols {
			if strings.EqualFold(cols[i].name, tablePK[0]) && strings.EqualFold(cols[i].typ, "INTEGER") {
				cols[i].rowid = true
			}
		}
	}
	if withoutRowid {
		for i := range cols {
			cols[i].rowid = false
		}
	}
	return cols, withoutRowid, len(cols) > 0
}

// primaryKeyColumns lists the columns of a table constraint "PRIMARY KEY (a, b)" (nil for other constraints).
func primaryKeyColumns(d []token) []string {
	for i := 0; i+2 < len(d); i++ {
		if d[i].is("PRIMARY") && d[i+1].is("KEY") && d[i+2].text == "(" {
			var names []string
			expectName := true
			for _, t := range d[i+3:] {
				if !t.quoted && t.text == ")" {
					break
				}
				if !t.quoted && t.text == "," {
					expectName = true
					continue
				}
				if expectName {
					names = append(names, t.text)
					expectName = false
				}
			}
			return names
		}
	}
	return nil
}

// joinType writes a type back as SQL would: words with spaces, "(10, 2)" kept tight.
func joinType(words []string) string {
	var b strings.Builder
	for i, w := range words {
		if i > 0 && w != "(" && w != ")" && w != "," && words[i-1] != "(" && words[i-1] != "," {
			b.WriteByte(' ')
		}
		b.WriteString(w)
		if w == "," {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// realAffinity applies SQLite's rules for column affinity: a type with INT in it is an integer, one with CHAR, CLOB or
// TEXT is text, BLOB (or no type) is a blob, and REAL, FLOA or DOUB make it REAL.
func realAffinity(typ string) bool {
	u := strings.ToUpper(typ)
	if strings.Contains(u, "INT") || strings.Contains(u, "CHAR") || strings.Contains(u, "CLOB") || strings.Contains(u, "TEXT") || strings.Contains(u, "BLOB") {
		return false
	}
	return strings.Contains(u, "REAL") || strings.Contains(u, "FLOA") || strings.Contains(u, "DOUB")
}
