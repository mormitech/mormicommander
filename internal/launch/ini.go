// A small reader and writer for wincmd.ini-style INI files. Total Commander keeps its settings, its command line
// history and its internal associations in INI sections, and users move those files between machines, so the
// launcher reads and writes the same shape: [Section] headers, key=value lines, ';' comments. Section and key names
// are matched without regard to case, as Windows' own INI functions do. Files saved by Total Commander are often
// UTF-16 with a byte order mark, so that is recognised along with UTF-8; files are always written as UTF-8. Comment
// lines are kept in place so a hand-edited file survives a round trip.
package launch

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf16"
)

// INIKey is one line of a section: a key and its value, or (with an empty Key) a comment line kept verbatim in Value.
type INIKey struct {
	Key   string
	Value string
}

// INISection is one [Name] section with its lines in file order.
type INISection struct {
	Name string
	Keys []INIKey
}

// INIFile is a whole INI file, sections in file order.
type INIFile struct {
	Sections []*INISection
}

// ParseINI reads INI text (UTF-8, or UTF-16 with a byte order mark).
func ParseINI(data []byte) *INIFile {
	text := decodeText(data)
	f := &INIFile{}
	var cur *INISection
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			cur = f.EnsureSection(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
		case strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#"):
			if cur != nil {
				cur.Keys = append(cur.Keys, INIKey{Value: trimmed})
			}
		default:
			if cur == nil {
				continue // lines before the first section have no home
			}
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			cur.Keys = append(cur.Keys, INIKey{Key: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
		}
	}
	return f
}

// decodeText turns file bytes into a string, honouring UTF-8 and UTF-16 byte order marks.
func decodeText(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return string(data[3:])
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}), bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		le := data[0] == 0xFF
		data = data[2:]
		u := make([]uint16, len(data)/2)
		for i := range u {
			if le {
				u[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
			} else {
				u[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
			}
		}
		return string(utf16.Decode(u))
	}
	return string(data)
}

// Section returns the section with this name, or nil.
func (f *INIFile) Section(name string) *INISection {
	for _, s := range f.Sections {
		if strings.EqualFold(s.Name, name) {
			return s
		}
	}
	return nil
}

// EnsureSection returns the section with this name, adding an empty one at the end when there is none.
func (f *INIFile) EnsureSection(name string) *INISection {
	if s := f.Section(name); s != nil {
		return s
	}
	s := &INISection{Name: name}
	f.Sections = append(f.Sections, s)
	return s
}

// RemoveSection deletes the section with this name, if there is one.
func (f *INIFile) RemoveSection(name string) {
	out := f.Sections[:0]
	for _, s := range f.Sections {
		if !strings.EqualFold(s.Name, name) {
			out = append(out, s)
		}
	}
	f.Sections = out
}

// Get returns the value of key and whether it exists.
func (s *INISection) Get(key string) (string, bool) {
	for _, kv := range s.Keys {
		if kv.Key != "" && strings.EqualFold(kv.Key, key) {
			return kv.Value, true
		}
	}
	return "", false
}

// Set changes the value of key, or adds the key at the end of the section.
func (s *INISection) Set(key, value string) {
	for i, kv := range s.Keys {
		if kv.Key != "" && strings.EqualFold(kv.Key, key) {
			s.Keys[i].Value = value
			return
		}
	}
	s.Keys = append(s.Keys, INIKey{Key: key, Value: value})
}

// WriteTo writes the file as UTF-8 text with "\n" line endings (use Bytes("\r\n") for Windows line endings).
func (f *INIFile) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(f.Bytes("\n"))
	return int64(n), err
}

// Bytes renders the file with the given line ending.
func (f *INIFile) Bytes(eol string) []byte {
	var b strings.Builder
	for i, s := range f.Sections {
		if i > 0 {
			b.WriteString(eol)
		}
		b.WriteString("[" + s.Name + "]" + eol)
		for _, kv := range s.Keys {
			if kv.Key == "" {
				b.WriteString(kv.Value + eol)
				continue
			}
			b.WriteString(kv.Key + "=" + kv.Value + eol)
		}
	}
	return []byte(b.String())
}
