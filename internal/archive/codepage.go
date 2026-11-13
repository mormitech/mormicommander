// This file decodes file names from old archives that were not written in
// UTF-8. Before about 2010, zip tools on Windows stored names in the OEM code
// page of the machine (CP437 in the US, CP852 in Hungary, CP866 in Russia,
// Shift-JIS in Japan) without saying so, which is why a Hungarian "árvíztűrő"
// often shows up as box-drawing garbage. Total Commander lets the user pick
// the code page for such archives; Encodings lists the ones we support,
// LookupEncoding finds one by name, and DefaultOEMEncoding guesses the right
// one for this computer (from GetOEMCP on Windows, from the locale elsewhere),
// which is what the "auto" setting uses.
package archive

import (
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

// encodingTable maps canonical lower-case names to decoders. Aliases are
// resolved by normalizeEncodingName.
var encodingTable = map[string]encoding.Encoding{
	"cp437":        charmap.CodePage437,
	"cp850":        charmap.CodePage850,
	"cp852":        charmap.CodePage852,
	"cp855":        charmap.CodePage855,
	"cp858":        charmap.CodePage858,
	"cp860":        charmap.CodePage860,
	"cp862":        charmap.CodePage862,
	"cp863":        charmap.CodePage863,
	"cp865":        charmap.CodePage865,
	"cp866":        charmap.CodePage866,
	"windows-874":  charmap.Windows874,
	"windows-1250": charmap.Windows1250,
	"windows-1251": charmap.Windows1251,
	"windows-1252": charmap.Windows1252,
	"windows-1253": charmap.Windows1253,
	"windows-1254": charmap.Windows1254,
	"windows-1255": charmap.Windows1255,
	"windows-1256": charmap.Windows1256,
	"windows-1257": charmap.Windows1257,
	"windows-1258": charmap.Windows1258,
	"iso-8859-1":   charmap.ISO8859_1,
	"iso-8859-2":   charmap.ISO8859_2,
	"iso-8859-5":   charmap.ISO8859_5,
	"iso-8859-7":   charmap.ISO8859_7,
	"iso-8859-15":  charmap.ISO8859_15,
	"koi8-r":       charmap.KOI8R,
	"koi8-u":       charmap.KOI8U,
	"mac-roman":    charmap.Macintosh,
	"shift_jis":    japanese.ShiftJIS,
	"euc-jp":       japanese.EUCJP,
	"gbk":          simplifiedchinese.GBK,
	"gb18030":      simplifiedchinese.GB18030,
	"big5":         traditionalchinese.Big5,
	"euc-kr":       korean.EUCKR,
}

// Encodings returns the names accepted by LookupEncoding, sorted, plus
// "utf-8". The UI offers these in the "archive name encoding" setting.
func Encodings() []string {
	out := []string{"utf-8"}
	for k := range encodingTable {
		out = append(out, k)
	}
	sort.Strings(out[1:])
	return out
}

func normalizeEncodingName(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "_", "-")
	switch {
	case n == "utf8":
		return "utf-8"
	case strings.HasPrefix(n, "ibm"):
		return "cp" + strings.TrimPrefix(n, "ibm")
	case strings.HasPrefix(n, "oem"):
		return "cp" + strings.TrimLeft(strings.TrimPrefix(n, "oem"), "- ")
	case strings.HasPrefix(n, "cp125") && len(n) == 6:
		return "windows-" + n[2:]
	case n == "cp874":
		return "windows-874"
	case n == "shift-jis" || n == "sjis" || n == "cp932":
		return "shift_jis"
	case n == "cp936":
		return "gbk"
	case n == "cp950":
		return "big5"
	case n == "cp949":
		return "euc-kr"
	case n == "latin1":
		return "iso-8859-1"
	case n == "latin2":
		return "iso-8859-2"
	case n == "macintosh" || n == "mac":
		return "mac-roman"
	}
	return n
}

// LookupEncoding returns the decoder for a code page name. It accepts the
// names listed by Encodings and common aliases ("ibm852", "oem852",
// "cp1250", "sjis", "cp932"...). The boolean is false for unknown names and
// for "utf-8", which needs no decoder.
func LookupEncoding(name string) (encoding.Encoding, bool) {
	e, ok := encodingTable[normalizeEncodingName(name)]
	return e, ok
}

// Decoder turns raw name bytes from an archive into a Go string.
type Decoder func(raw []byte) string

// NewNameDecoder returns the decoder for an OpenOptions.Encoding value.
// For "" and "auto" the caller passes all non-UTF-8-flagged raw names of the
// archive: when every one of them is valid UTF-8 (macOS and Linux zip tools
// write UTF-8 without setting the flag) they are taken as UTF-8, otherwise
// the system OEM code page is used. Unknown names fall back to CP437, the
// code page the zip specification prescribes.
func NewNameDecoder(setting string, samples [][]byte) Decoder {
	n := normalizeEncodingName(setting)
	if n == "utf-8" {
		return decodeUTF8
	}
	if n == "" || n == "auto" {
		allUTF8 := true
		for _, s := range samples {
			if !utf8.Valid(s) {
				allUTF8 = false
				break
			}
		}
		if allUTF8 {
			return decodeUTF8
		}
		n = DefaultOEMEncoding()
	}
	enc, ok := encodingTable[n]
	if !ok {
		enc = charmap.CodePage437
	}
	dec := enc.NewDecoder()
	return func(raw []byte) string {
		if isASCII(raw) {
			return string(raw)
		}
		out, err := dec.Bytes(raw)
		if err != nil {
			return strings.ToValidUTF8(string(raw), "?")
		}
		return string(out)
	}
}

func decodeUTF8(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	return strings.ToValidUTF8(string(raw), "�")
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

// oemForLanguage maps a locale language (and for Chinese, region) to the
// Windows OEM code page used by that locale, mirroring what GetOEMCP returns
// on a Windows machine set to that language.
func oemForLanguage(locale string) string {
	l := strings.ToLower(locale)
	if i := strings.IndexAny(l, ".@"); i >= 0 {
		l = l[:i]
	}
	lang, region, _ := strings.Cut(strings.ReplaceAll(l, "-", "_"), "_")
	switch lang {
	case "hu", "cs", "sk", "pl", "hr", "sl", "ro", "sq", "bs", "sr_latn":
		return "cp852"
	case "ru", "uk", "be", "bg", "mk", "kk", "ky", "tt", "sr":
		return "cp866"
	case "de", "fr", "es", "it", "pt", "nl", "da", "sv", "fi", "is", "ca", "eu", "gl", "ga", "af":
		return "cp850"
	case "nb", "nn", "no":
		return "cp865"
	case "he", "iw":
		return "cp862"
	case "ja":
		return "shift_jis"
	case "ko":
		return "euc-kr"
	case "zh":
		if region == "tw" || region == "hk" || region == "mo" {
			return "big5"
		}
		return "gbk"
	case "th":
		return "windows-874"
	case "tr", "az":
		return "windows-1254"
	case "el":
		return "windows-1253"
	case "ar", "fa", "ur":
		return "windows-1256"
	case "lt", "lv", "et":
		return "windows-1257"
	case "vi":
		return "windows-1258"
	}
	return "cp437"
}

// oemForCodePage maps a Windows code page number to our encoding name.
func oemForCodePage(cp uint32) string {
	switch cp {
	case 932:
		return "shift_jis"
	case 936:
		return "gbk"
	case 949:
		return "euc-kr"
	case 950:
		return "big5"
	case 874:
		return "windows-874"
	case 65001:
		return "utf-8"
	}
	if cp >= 1250 && cp <= 1258 {
		return "windows-" + itoa(int(cp))
	}
	name := "cp" + itoa(int(cp))
	if _, ok := encodingTable[name]; ok {
		return name
	}
	return "cp437"
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	return string(b[n:])
}
