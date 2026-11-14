// Package marta reads the favourite folders of Marta, the free macOS file manager, so people coming from it can bring
// them into mormiCommander's folder hotlist (board issue #22, F-NEW-22). Marta keeps its settings in
// ~/Library/Application Support/org.yanex.marta as ".marco" files, a small text format of its own: "key value" pairs,
// { } blocks, [ ] lists, "quoted strings", bare words and numbers. The favourites are in favorites.marco, and a
// "favorites" block in conf.marco is read too. Because the exact layout of those files is Marta's and not written
// down in full, the reader is tolerant: it parses the whole Marco grammar loosely and then takes every value that
// looks like a folder (an absolute path, a "~/" path or a file:// address), with the name next to it when there is one.
//
// This package only ever reads Marta's files; it never writes, moves or deletes anything in Marta's folder.
package marta

import (
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// DirName is the name of Marta's settings folder inside ~/Library/Application Support.
const DirName = "org.yanex.marta"

// The files that may hold favourites: Marta's own favourites file and the user's configuration.
const (
	FavoritesFile = "favorites.marco"
	ConfigFile    = "conf.marco"
)

// maxFileSize keeps a broken or huge file from being read whole (Marta's files are a few kilobytes).
const maxFileSize = 4 << 20

// Dir is Marta's settings folder for the home folder home (macOS layout).
func Dir(home string) string {
	return filepath.Join(home, "Library", "Application Support", DirName)
}

// Found says whether Marta's settings folder is there.
func Found(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

// Favorite is one favourite folder: the name Marta shows ("" when the file has none) and the folder.
type Favorite struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Favorites reads the favourite folders from Marta's settings folder dir. home expands "~". Folders are given once
// each, in the order the files list them (favorites.marco first). A missing file is not an error; when neither file
// is there, or neither can be read, the answer is empty with the first error.
func Favorites(dir, home string) ([]Favorite, error) {
	var out []Favorite
	seen := map[string]bool{}
	var firstErr error
	readAny := false
	add := func(list []Favorite) {
		for _, f := range list {
			if !seen[f.Path] {
				seen[f.Path] = true
				out = append(out, f)
			}
		}
	}
	for _, name := range []string{FavoritesFile, ConfigFile} {
		src, err := readSmall(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		readAny = true
		v := Parse(src)
		if name == FavoritesFile {
			add(Collect(v, home))
		} else {
			for _, fv := range findKey(v, "favorites") {
				add(Collect(fv, home))
			}
		}
	}
	if !readAny && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func readSmall(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileSize))
	return string(b), err
}

// ---- the Marco format ----

// Kind is what a parsed value is.
type Kind int

const (
	Scalar Kind = iota // a string, a bare word or a number
	Object             // { key value … }
	List               // [ value … ]
)

// Value is one parsed Marco value. An Object keeps its pairs in file order (a key may appear more than once).
type Value struct {
	Kind   Kind
	Text   string   // a Scalar's text, without quotes
	Quoted bool     // the Scalar was a "quoted string"
	Keys   []string // an Object's keys
	Items  []Value  // an Object's values (same order as Keys) or a List's items
}

// Get is the first value of key in an Object (nil when there is none).
func (v Value) Get(key string) *Value {
	for i, k := range v.Keys {
		if strings.EqualFold(k, key) {
			return &v.Items[i]
		}
	}
	return nil
}

// Parse reads a whole Marco file. It never fails: what it cannot make sense of is skipped. The file is read as the
// body of an object ("key value" pairs); a file that is only one { } block or [ ] list gives that value.
func Parse(src string) Value {
	p := &parser{toks: tokenize(src)}
	if len(p.toks) > 0 && (p.toks[0].kind == tokOpenList || p.toks[0].kind == tokOpenObj) {
		first := p.value()
		if p.pos >= len(p.toks) {
			return first
		}
		p.pos = 0
	}
	return p.object(false)
}

type tokKind int

const (
	tokWord tokKind = iota
	tokString
	tokOpenObj
	tokCloseObj
	tokOpenList
	tokCloseList
)

type token struct {
	kind tokKind
	text string
}

// tokenize splits the text into words, strings and brackets. Commas, colons and "=" between values are skipped, and
// "#", "//" and "--" at the start of a token begin a comment that runs to the end of the line.
func tokenize(src string) []token {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ',' || c == ';' || c == ':' || c == '=':
			i++
		case c == '#' || strings.HasPrefix(src[i:], "//") || strings.HasPrefix(src[i:], "--"):
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '{':
			toks = append(toks, token{kind: tokOpenObj})
			i++
		case c == '}':
			toks = append(toks, token{kind: tokCloseObj})
			i++
		case c == '[':
			toks = append(toks, token{kind: tokOpenList})
			i++
		case c == ']':
			toks = append(toks, token{kind: tokCloseList})
			i++
		case c == '"' || c == '\'':
			s, n := readString(src[i:], c)
			toks = append(toks, token{kind: tokString, text: s})
			i += n
		default:
			j := i
			for j < len(src) && !strings.ContainsRune(" \t\r\n{}[]\",;=", rune(src[j])) && !(src[j] == ':' && j+1 < len(src) && src[j+1] != '/') {
				j++
			}
			if j == i { // a lone character no rule takes
				j++
			}
			toks = append(toks, token{kind: tokWord, text: src[i:j]})
			i = j
		}
	}
	return toks
}

// readString reads a quoted string that starts at s[0] (the quote q) and gives its text and how many bytes it took.
func readString(s string, q byte) (string, int) {
	var b strings.Builder
	i := 1
	for i < len(s) {
		c := s[i]
		if c == q {
			return b.String(), i + 1
		}
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += n
	}
	return b.String(), len(s) // an unclosed string runs to the end
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) value() Value {
	if p.pos >= len(p.toks) {
		return Value{}
	}
	t := p.toks[p.pos]
	p.pos++
	switch t.kind {
	case tokOpenObj:
		return p.object(true)
	case tokOpenList:
		v := Value{Kind: List}
		for p.pos < len(p.toks) {
			if k := p.toks[p.pos].kind; k == tokCloseList || k == tokCloseObj {
				p.pos++
				break
			}
			v.Items = append(v.Items, p.value())
		}
		return v
	case tokCloseObj, tokCloseList: // a stray closing bracket: nothing
		return Value{}
	}
	return Value{Kind: Scalar, Text: t.text, Quoted: t.kind == tokString}
}

// object reads "key value" pairs up to the closing "}" (inner) or the end of the file.
func (p *parser) object(inner bool) Value {
	v := Value{Kind: Object}
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		if t.kind == tokCloseObj || t.kind == tokCloseList {
			p.pos++
			if inner {
				return v
			}
			continue
		}
		if t.kind == tokOpenObj || t.kind == tokOpenList { // a value without a key: keep it under ""
			v.Keys = append(v.Keys, "")
			v.Items = append(v.Items, p.value())
			continue
		}
		p.pos++
		key := t.text
		if p.pos >= len(p.toks) || p.toks[p.pos].kind == tokCloseObj {
			v.Keys = append(v.Keys, key)
			v.Items = append(v.Items, Value{})
			continue
		}
		v.Keys = append(v.Keys, key)
		v.Items = append(v.Items, p.value())
	}
	return v
}

// findKey gives every value stored under key (at any depth).
func findKey(v Value, key string) []Value {
	var out []Value
	for i, k := range v.Keys {
		if strings.EqualFold(k, key) {
			out = append(out, v.Items[i])
			continue
		}
		out = append(out, findKey(v.Items[i], key)...)
	}
	if v.Kind == List {
		for _, it := range v.Items {
			out = append(out, findKey(it, key)...)
		}
	}
	return out
}

// ---- finding the folders ----

// The keys whose value is taken as the folder or the name of a favourite block, best first.
var (
	pathKeys = []string{"path", "url", "location", "folder", "dir", "directory", "target", "uri"}
	nameKeys = []string{"name", "title", "label", "caption", "displayName"}
)

// Collect takes the favourite folders out of a parsed value: a block with a path key (and maybe a name key) is one
// favourite; "name" "/folder" pairs and lists of folders are taken too. home expands "~".
func Collect(v Value, home string) []Favorite {
	var out []Favorite
	seen := map[string]bool{}
	add := func(name, raw string) {
		p, ok := folderOf(raw, home)
		if !ok || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, Favorite{Name: strings.TrimSpace(name), Path: p})
	}
	var walk func(v Value, key string)
	walk = func(v Value, key string) {
		switch v.Kind {
		case Scalar:
			// a "Name" "/folder" pair: the key is the name unless it is just a field name
			name := key
			if isFieldKey(key) {
				name = ""
			}
			add(name, v.Text)
		case List:
			for _, it := range v.Items {
				walk(it, "")
			}
		case Object:
			if p := firstScalar(v, pathKeys); p != "" {
				if _, ok := folderOf(p, home); ok {
					add(firstScalar(v, nameKeys), p)
					return
				}
			}
			for i, k := range v.Keys {
				walk(v.Items[i], k)
			}
		}
	}
	walk(v, "")
	return out
}

func firstScalar(v Value, keys []string) string {
	for _, k := range keys {
		if x := v.Get(k); x != nil && x.Kind == Scalar && x.Text != "" {
			return x.Text
		}
	}
	return ""
}

func isFieldKey(k string) bool {
	for _, list := range [][]string{pathKeys, nameKeys} {
		for _, f := range list {
			if strings.EqualFold(k, f) {
				return true
			}
		}
	}
	return false
}

// folderOf turns a value into a clean absolute folder: "/x/y", "~", "~/x" and "file:///x" count; anything else (a
// word, a number, a web address) does not.
func folderOf(raw, home string) (string, bool) {
	s := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(s, "file://"):
		u, err := url.Parse(s)
		if err != nil || (u.Host != "" && u.Host != "localhost") || u.Path == "" {
			return "", false
		}
		s = u.Path
	case s == "~" || strings.HasPrefix(s, "~/"):
		if home == "" {
			return "", false
		}
		s = filepath.Join(home, strings.TrimPrefix(s, "~"))
	}
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "", false
	}
	return filepath.Clean(s), true
}
