package robust

import (
	"path/filepath"
	"unicode/utf8"
)

// tempStemMax is how much of a file's name goes into a temporary name next to it. Names may be up to 255 bytes, so a
// temporary name made of the whole name plus a prefix and a random suffix fails with "name too long" for long names.
const tempStemMax = 64

// TempStem is the part of name that goes into a temporary name next to it: the whole name when it is short, else its
// first 64 bytes, cut on a character boundary.
func TempStem(name string) string {
	return cutUTF8(name, tempStemMax)
}

// ShortName keeps a name within max bytes and keeps its extension (".txt"), so a temporary copy is still recognised
// by its type. Short names come back unchanged.
func ShortName(name string, max int) string {
	if len(name) <= max {
		return name
	}
	ext := filepath.Ext(name)
	if len(ext) > max/2 {
		ext = ""
	}
	return cutUTF8(name[:len(name)-len(ext)], max-len(ext)) + ext
}

// cutUTF8 cuts s to at most max bytes without splitting a character.
func cutUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
