// This file is the field registry behind custom columns. In Total Commander a custom column's content is a template
// such as "[=tc.size.k] / [=tc.writedate.dd.MM.yyyy]": each [=namespace.field.unit] placeholder names a field and an
// optional unit, and the rest is literal text. TC gets extra fields from content plugins (WDX DLLs); we replace those
// with a registry, so other engines (thumbnails, archives, media readers) can add fields under their own namespace with
// RegisterField and every column set can use them. The built-in "tc" namespace provides the fields TC itself has, plus
// the Unix ones (permissions, owner, group, link target) a cross-platform commander needs.

package listing

import (
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kind tells the sorter how to compare a field's values.
type Kind int

const (
	KindText   Kind = iota // compared with the panel's name collation
	KindNumber             // compared as integers (sizes, pixel counts, track numbers)
	KindTime               // compared as instants
)

// Value is one field of one item: the text to show plus the raw value for sorting.
type Value struct {
	Text  string
	Kind  Kind
	Num   int64
	Time  time.Time
	Empty bool // the item has no value for this field; empty values sort after real ones
}

// TextValue, NumValue and TimeValue build Values.
func TextValue(s string) Value { return Value{Text: s, Kind: KindText, Empty: s == ""} }

// NumValue builds a numeric value with its display text.
func NumValue(n int64, text string) Value { return Value{Text: text, Kind: KindNumber, Num: n} }

// TimeValue builds a time value with its display text; a zero time is Empty.
func TimeValue(t time.Time, text string) Value {
	return Value{Text: text, Kind: KindTime, Time: t, Empty: t.IsZero()}
}

// Field is a named value extractor. Unit is the text after the field name in a placeholder (for "[=tc.size.k]" it is
// "k"), and its meaning is up to the field; Units lists suggestions for a column editor.
type Field struct {
	Name  string
	Kind  Kind
	Help  string
	Units []string
	Get   func(it *Item, unit string, f *Format) Value
}

var (
	regMu  sync.RWMutex
	fields = map[string]Field{} // key "ns.name", lower case
)

// RegisterField adds or replaces the field ns.name. Names are case-insensitive, as in TC.
func RegisterField(ns string, fl Field) {
	regMu.Lock()
	defer regMu.Unlock()
	fields[strings.ToLower(ns+"."+fl.Name)] = fl
}

// LookupField finds a registered field by "ns.name".
func LookupField(qualified string) (Field, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	fl, ok := fields[strings.ToLower(qualified)]
	return fl, ok
}

// FieldNames lists every registered field as "ns.name", sorted, for the column editor's field menu.
func FieldNames() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]string, 0, len(fields))
	for k := range fields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ExtraField makes a field that reads it.Extra[key] as filled by an external provider. Ints become numbers, times
// become dates (formatted with the unit as picture), anything else is shown with its string form.
func ExtraField(name, key string, kind Kind) Field {
	return Field{Name: name, Kind: kind, Get: func(it *Item, unit string, f *Format) Value {
		return extraValue(it.Extra[key], kind, unit, f)
	}}
}

func extraValue(v any, kind Kind, unit string, f *Format) Value {
	switch x := v.(type) {
	case nil:
		return Value{Kind: kind, Empty: true}
	case int:
		return NumValue(int64(x), strconv.Itoa(x))
	case int64:
		return NumValue(x, strconv.FormatInt(x, 10))
	case time.Time:
		pic := unit
		if pic == "" {
			pic = f.Date
		}
		return TimeValue(x, FormatDate(x, pic, f.Location))
	case string:
		return Value{Text: x, Kind: KindText, Empty: x == ""}
	}
	return Value{Kind: kind, Empty: true}
}

func init() {
	sizeUnits := []string{"bytes", "k", "M", "G", "dynamic", "raw"}
	dateUnits := []string{DefaultDatePicture, DatePictureISO, DatePictureEurope, DatePictureUS, "yyyy-MM-dd", "HH:mm:ss"}
	reg := func(fl Field) { RegisterField("tc", fl) }

	reg(Field{Name: "name", Kind: KindText, Help: "name without extension (folders whole)",
		Get: func(it *Item, _ string, f *Format) Value {
			b, _ := SplitExtFor(it, f.DirExt)
			return TextValue(b)
		}})
	reg(Field{Name: "ext", Kind: KindText, Help: "extension without the dot",
		Get: func(it *Item, _ string, f *Format) Value {
			_, e := SplitExtFor(it, f.DirExt)
			return TextValue(e)
		}})
	reg(Field{Name: "fullname", Kind: KindText, Help: "name with extension",
		Get: func(it *Item, _ string, f *Format) Value {
			if f.LinkSuffix && it.LinkTarget != "" {
				return TextValue(it.Name + " -> " + it.LinkTarget)
			}
			return TextValue(it.Name)
		}})
	reg(Field{Name: "size", Kind: KindNumber, Units: sizeUnits, Help: "size; folders show <DIR> until calculated",
		Get: func(it *Item, unit string, f *Format) Value {
			u, ok := ParseSizeUnit(unit)
			if !ok {
				u = f.Size
			}
			g := *f
			g.Size = u
			n := it.Size
			if it.IsDir() {
				n = it.DirSize
			}
			v := NumValue(n, g.SizeText(it))
			v.Empty = it.IsDir() && it.DirSize < 0
			return v
		}})
	date := func(name, help string, get func(*Item) time.Time) {
		reg(Field{Name: name, Kind: KindTime, Units: dateUnits, Help: help,
			Get: func(it *Item, unit string, f *Format) Value {
				pic := unit
				if pic == "" {
					pic = f.Date
				}
				t := get(it)
				return TimeValue(t, FormatDate(t, pic, f.Location))
			}})
	}
	date("writedate", "last modification", func(it *Item) time.Time { return it.ModTime })
	date("creationdate", "creation (birth) time", func(it *Item) time.Time { return it.CreateTime })
	date("accessdate", "last access", func(it *Item) time.Time { return it.AccessTime })
	reg(Field{Name: "attributestr", Kind: KindText, Units: []string{"auto", "win", "winfull", "unix", "octal"},
		Help: "attribute string",
		Get: func(it *Item, unit string, f *Format) Value {
			st := f.AttrStyle
			switch strings.ToLower(unit) {
			case "win", "windows":
				st = AttrStyleWindows
			case "winfull":
				st = AttrStyleWindowsFull
			case "unix":
				st = AttrStyleUnix
			case "octal":
				st = AttrStyleOctal
			case "auto":
				st = AttrStyleAuto
			}
			if it.IsUp() {
				return TextValue("")
			}
			return TextValue(AttrString(it, st))
		}})
	reg(Field{Name: "permissions", Kind: KindText, Help: "Unix permissions (drwxr-xr-x)",
		Get: func(it *Item, _ string, _ *Format) Value {
			if it.IsUp() {
				return TextValue("")
			}
			return TextValue(AttrString(it, AttrStyleUnix))
		}})
	reg(Field{Name: "owner", Kind: KindText, Get: func(it *Item, _ string, _ *Format) Value { return TextValue(it.Owner) }})
	reg(Field{Name: "group", Kind: KindText, Get: func(it *Item, _ string, _ *Format) Value { return TextValue(it.Group) }})
	reg(Field{Name: "linktarget", Kind: KindText,
		Get: func(it *Item, _ string, _ *Format) Value { return TextValue(it.LinkTarget) }})
	reg(Field{Name: "comment", Kind: KindText, Help: "descript.ion comment",
		Get: func(it *Item, _ string, _ *Format) Value {
			return TextValue(strings.ReplaceAll(it.Comment, "\n", " "))
		}})
	reg(Field{Name: "path", Kind: KindText, Help: "folder of the file, with trailing separator",
		Get: func(it *Item, _ string, _ *Format) Value { return TextValue(withSep(it.Dir)) }})
	reg(Field{Name: "fullpath", Kind: KindText, Help: "folder and name",
		Get: func(it *Item, _ string, _ *Format) Value { return TextValue(withSep(it.Dir) + it.Name) }})
	reg(Field{Name: "filetype", Kind: KindText, Help: "file type description",
		Get: func(it *Item, _ string, _ *Format) Value { return TextValue(FileTypeName(it)) }})
	reg(Field{Name: "mime", Kind: KindText, Help: "MIME type guessed from the extension",
		Get: func(it *Item, _ string, _ *Format) Value {
			if it.IsDir() {
				return TextValue("inode/directory")
			}
			t := mime.TypeByExtension(strings.ToLower(path.Ext(it.Name)))
			if i := strings.IndexByte(t, ';'); i >= 0 {
				t = t[:i]
			}
			return TextValue(t)
		}})
}

func withSep(dir string) string {
	if dir == "" || strings.HasSuffix(dir, "/") || strings.HasSuffix(dir, `\`) {
		return dir
	}
	if strings.Contains(dir, `\`) && !strings.Contains(dir, "/") {
		return dir + `\`
	}
	if filepath.Separator == '\\' && !strings.Contains(dir, "/") {
		return dir + `\`
	}
	return dir + "/"
}

// fileTypes names common extensions the way Explorer's "Type" column does. Unknown extensions fall back to
// "EXT file", which is also what Windows shows.
var fileTypes = map[string]string{
	"txt": "Text document", "md": "Markdown document", "log": "Log file", "ini": "Configuration settings",
	"pdf": "PDF document", "doc": "Word document", "docx": "Word document", "xls": "Excel worksheet",
	"xlsx": "Excel worksheet", "ppt": "PowerPoint presentation", "pptx": "PowerPoint presentation",
	"odt": "OpenDocument text", "ods": "OpenDocument spreadsheet", "rtf": "Rich text document",
	"csv": "CSV file", "json": "JSON file", "xml": "XML document", "yaml": "YAML file", "yml": "YAML file",
	"html": "HTML document", "htm": "HTML document", "css": "CSS stylesheet", "js": "JavaScript file",
	"ts": "TypeScript file", "go": "Go source file", "c": "C source file", "h": "C header file",
	"cpp": "C++ source file", "py": "Python script", "sh": "Shell script", "rs": "Rust source file",
	"java": "Java source file", "jpg": "JPEG image", "jpeg": "JPEG image", "png": "PNG image", "gif": "GIF image",
	"bmp": "Bitmap image", "webp": "WebP image", "svg": "SVG image", "tif": "TIFF image", "tiff": "TIFF image",
	"heic": "HEIC image", "ico": "Icon", "mp3": "MP3 audio", "wav": "WAVE audio", "flac": "FLAC audio",
	"ogg": "Ogg audio", "m4a": "MPEG-4 audio", "mp4": "MPEG-4 video", "mkv": "Matroska video", "avi": "AVI video",
	"mov": "QuickTime movie", "webm": "WebM video", "zip": "ZIP archive", "7z": "7-Zip archive", "rar": "RAR archive",
	"tar": "TAR archive", "gz": "GZip archive", "tgz": "TAR.GZ archive", "bz2": "BZip2 archive", "xz": "XZ archive",
	"iso": "Disc image", "dmg": "Disk image", "exe": "Application", "com": "MS-DOS application",
	"bat": "Windows batch file", "cmd": "Windows command script", "msi": "Windows installer package",
	"dll": "Application extension", "lnk": "Shortcut", "app": "Application", "deb": "Debian package",
	"rpm": "RPM package", "ttf": "TrueType font", "otf": "OpenType font",
}

// FileTypeName describes the item's type: "File folder" for folders, a known description for common extensions,
// "EXT file" otherwise, and "File" without an extension.
func FileTypeName(it *Item) string {
	if it.IsDir() {
		return "File folder"
	}
	_, ext := SplitExt(it.Name)
	if ext == "" {
		return "File"
	}
	if d, ok := fileTypes[strings.ToLower(ext)]; ok {
		return d
	}
	return strings.ToUpper(ext) + " file"
}
