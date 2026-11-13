// System default code pages. Total Commander's "ANSI" key (A) means the
// Windows code page of the system and "ASCII" (S) the DOS/OEM code page that
// console programs use, for example Windows-1250 and 852 on a Hungarian
// Windows. On Windows we ask the system (locale_windows.go); on macOS and
// Linux there is no such setting, so we derive the pair from the language
// part of LC_ALL / LC_CTYPE / LANG, which gives the code pages a user of that
// language most likely meets in old files.

package lister

import "strings"

// localeCodepages maps a language (and for Chinese, the region) to the
// usual Windows ANSI and DOS OEM code pages.
var localeCodepages = map[string][2]Encoding{
	"hu": {Win1250, DOS852}, "cs": {Win1250, DOS852}, "sk": {Win1250, DOS852},
	"pl": {Win1250, DOS852}, "sl": {Win1250, DOS852}, "hr": {Win1250, DOS852},
	"ro": {Win1250, DOS852}, "sq": {Win1250, DOS852}, "bs": {Win1250, DOS852},
	"ru": {Win1251, DOS866}, "uk": {Win1251, DOS866}, "be": {Win1251, DOS866},
	"bg": {Win1251, DOS866}, "sr": {Win1251, DOS855}, "mk": {Win1251, DOS866},
	"kk": {Win1251, DOS866},
	"el": {Win1253, DOS437}, "tr": {Win1254, dos857}, "he": {Win1255, DOS862},
	"ar": {Win1256, DOS437}, "fa": {Win1256, DOS437}, "lt": {Win1257, DOS437},
	"lv": {Win1257, DOS437}, "et": {Win1257, DOS437}, "vi": {Win1258, DOS437},
	"th": {Win874, Win874}, "ja": {ShiftJIS, ShiftJIS}, "ko": {EUCKR, EUCKR},
	"zh_cn": {GBK, GBK}, "zh_sg": {GBK, GBK}, "zh_tw": {Big5, Big5}, "zh_hk": {Big5, Big5},
	"zh": {GBK, GBK}, "pt_br": {Win1252, DOS850}, "pt": {Win1252, DOS860},
	"fr_ca": {Win1252, DOS863}, "da": {Win1252, DOS865}, "nb": {Win1252, DOS865},
	"no": {Win1252, DOS865}, "nn": {Win1252, DOS865}, "is": {Win1252, DOS850},
	"de": {Win1252, DOS850}, "fr": {Win1252, DOS850}, "es": {Win1252, DOS850},
	"it": {Win1252, DOS850}, "nl": {Win1252, DOS850}, "sv": {Win1252, DOS850},
	"fi": {Win1252, DOS850},
}

// dos857 stands in for the Turkish DOS code page 857, which x/text
// does not provide; 850 shows the same Latin letters for most text.
const dos857 = DOS850

// codepagesForLocale parses a POSIX locale string like "hu_HU.UTF-8" and
// returns the matching ANSI and OEM code pages (US defaults if unknown).
func codepagesForLocale(loc string) (ansi, oem Encoding) {
	loc = strings.ToLower(loc)
	if i := strings.IndexAny(loc, ".@"); i >= 0 {
		loc = loc[:i]
	}
	loc = strings.ReplaceAll(loc, "-", "_")
	if p, ok := localeCodepages[loc]; ok {
		return p[0], p[1]
	}
	if i := strings.IndexByte(loc, '_'); i >= 0 {
		if p, ok := localeCodepages[loc[:i]]; ok {
			return p[0], p[1]
		}
	}
	return Win1252, DOS437
}

// EncodingForKey maps the Lister's charset keys to an encoding: A = ANSI
// (system Windows code page), S = ASCII (DOS/OEM code page), U = UTF-16
// ("Unicode" in Total Commander) and 8 = UTF-8. ok is false for other keys.
func EncodingForKey(key rune) (enc Encoding, ok bool) {
	switch key {
	case 'a', 'A':
		return DefaultANSI(), true
	case 's', 'S':
		return DefaultOEM(), true
	case 'u', 'U':
		return UTF16LE, true
	case '8':
		return UTF8, true
	}
	return "", false
}
