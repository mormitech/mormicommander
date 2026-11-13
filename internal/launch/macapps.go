// Finding applications on macOS for "Open with". Every application bundle lists the document types it opens in its
// Info.plist (CFBundleDocumentTypes), either by file extension or, in modern apps, by Uniform Type Identifier
// (UTI) such as public.plain-text. UTIs form a tree (public.plain-text is a kind of public.text, which is a kind of
// public.data), so a file's extension is turned into its UTIs and all their parents, using a table of the system's
// common types plus every type the scanned applications declare themselves. The user's default handlers are read
// from the LaunchServices preferences. Doing this without LaunchServices' C API keeps the program free of cgo; the
// list may miss an app that only the system's database knows about, which is acceptable for a menu of choices.
package launch

import (
	"os"
	"path/filepath"
	"strings"
)

// builtinUTIs maps common extensions to the system's type identifiers.
var builtinUTIs = map[string]string{
	"txt": "public.plain-text", "text": "public.plain-text", "md": "net.daringfireball.markdown",
	"markdown": "net.daringfireball.markdown", "rtf": "public.rtf", "html": "public.html", "htm": "public.html",
	"xml": "public.xml", "json": "public.json", "csv": "public.comma-separated-values-text",
	"tsv": "public.tab-separated-values-text", "log": "com.apple.log", "pdf": "com.adobe.pdf",
	"jpg": "public.jpeg", "jpeg": "public.jpeg", "png": "public.png", "gif": "com.compuserve.gif",
	"tif": "public.tiff", "tiff": "public.tiff", "heic": "public.heic", "bmp": "com.microsoft.bmp",
	"svg": "public.svg-image", "webp": "org.webmproject.webp", "ico": "com.microsoft.ico",
	"mp3": "public.mp3", "m4a": "com.apple.m4a-audio", "wav": "com.microsoft.waveform-audio",
	"aiff": "public.aiff-audio", "flac": "org.xiph.flac", "mp4": "public.mpeg-4", "m4v": "com.apple.m4v-video",
	"mov": "com.apple.quicktime-movie", "avi": "public.avi", "mkv": "org.matroska.mkv",
	"zip": "public.zip-archive", "gz": "org.gnu.gnu-zip-archive", "tgz": "org.gnu.gnu-zip-tar-archive",
	"tar": "public.tar-archive", "bz2": "public.bzip2-archive", "7z": "org.7-zip.7-zip-archive",
	"dmg": "com.apple.disk-image-udif", "c": "public.c-source", "h": "public.c-header",
	"m": "public.objective-c-source", "cpp": "public.c-plus-plus-source", "cc": "public.c-plus-plus-source",
	"hpp": "public.c-plus-plus-header", "swift": "public.swift-source", "py": "public.python-script",
	"rb": "public.ruby-script", "pl": "public.perl-script", "php": "public.php-script", "sh": "public.shell-script",
	"zsh": "public.zsh-script", "bash": "public.bash-script", "js": "com.netscape.javascript-source",
	"java": "com.sun.java-source", "css": "public.css", "yaml": "public.yaml", "yml": "public.yaml",
	"go": "public.source-code", "rs": "public.source-code", "ts": "public.source-code",
}

// builtinConforms gives the parents of the built-in types.
var builtinConforms = map[string][]string{
	"public.plain-text": {"public.text"}, "net.daringfireball.markdown": {"public.plain-text"},
	"public.rtf": {"public.text"}, "public.html": {"public.text"}, "public.xml": {"public.text"},
	"public.json": {"public.text"}, "public.yaml": {"public.text"}, "public.css": {"public.text"},
	"public.comma-separated-values-text": {"public.delimited-values-text"},
	"public.tab-separated-values-text":   {"public.delimited-values-text"},
	"public.delimited-values-text":       {"public.text"}, "com.apple.log": {"public.plain-text"},
	"public.source-code": {"public.plain-text"}, "public.script": {"public.source-code"},
	"public.c-source": {"public.source-code"}, "public.c-header": {"public.source-code"},
	"public.objective-c-source": {"public.source-code"}, "public.c-plus-plus-source": {"public.source-code"},
	"public.c-plus-plus-header": {"public.source-code"}, "public.swift-source": {"public.source-code"},
	"com.sun.java-source": {"public.source-code"}, "public.python-script": {"public.script"},
	"public.ruby-script": {"public.script"}, "public.perl-script": {"public.script"},
	"public.php-script": {"public.script"}, "public.shell-script": {"public.script"},
	"public.zsh-script": {"public.shell-script"}, "public.bash-script": {"public.shell-script"},
	"com.netscape.javascript-source": {"public.script"},
	"public.text":                    {"public.data", "public.content"},
	"public.jpeg":                    {"public.image"}, "public.png": {"public.image"}, "com.compuserve.gif": {"public.image"},
	"public.tiff": {"public.image"}, "public.heic": {"public.image"}, "com.microsoft.bmp": {"public.image"},
	"public.svg-image": {"public.image"}, "org.webmproject.webp": {"public.image"}, "com.microsoft.ico": {"public.image"},
	"public.image": {"public.data", "public.content"},
	"public.mp3":   {"public.audio"}, "com.apple.m4a-audio": {"public.audio"}, "com.microsoft.waveform-audio": {"public.audio"},
	"public.aiff-audio": {"public.audio"}, "org.xiph.flac": {"public.audio"},
	"public.audio": {"public.audiovisual-content"}, "public.mpeg-4": {"public.movie"},
	"com.apple.m4v-video": {"public.movie"}, "com.apple.quicktime-movie": {"public.movie"},
	"public.avi": {"public.movie"}, "org.matroska.mkv": {"public.movie"},
	"public.movie":               {"public.audiovisual-content"},
	"public.audiovisual-content": {"public.data", "public.content"},
	"public.zip-archive":         {"public.archive"}, "org.gnu.gnu-zip-archive": {"public.archive"},
	"org.gnu.gnu-zip-tar-archive": {"public.archive"}, "public.tar-archive": {"public.archive"},
	"public.bzip2-archive": {"public.archive"}, "org.7-zip.7-zip-archive": {"public.archive"},
	"com.apple.disk-image-udif": {"public.disk-image"}, "public.disk-image": {"public.data"},
	"public.archive": {"public.data"}, "com.adobe.pdf": {"public.data", "public.composite-content"},
	"public.composite-content": {"public.content"}, "public.data": {"public.item"},
}

// broadUTIs are types so general that an app claiming them opens almost anything; such apps are listed last.
var broadUTIs = map[string]bool{
	"public.item": true, "public.data": true, "public.content": true, "public.text": true,
	"public.composite-content": true, "public.archive": true, "public.audiovisual-content": true,
}

// macDocType is one CFBundleDocumentTypes entry.
type macDocType struct {
	extensions []string
	utis       []string
	role       string
}

// macApp is what macAppsFor needs to know about one application bundle.
type macApp struct {
	path     string
	name     string
	bundleID string
	types    []macDocType
}

// readMacApp reads an application bundle's Info.plist and learns the types it declares into extUTIs/conforms.
func readMacApp(appPath string, extUTIs map[string][]string, conforms map[string][]string) (macApp, bool) {
	data, err := os.ReadFile(filepath.Join(appPath, "Contents", "Info.plist"))
	if err != nil {
		return macApp{}, false
	}
	v, err := ParsePlist(data)
	if err != nil {
		return macApp{}, false
	}
	info, ok := v.(map[string]any)
	if !ok {
		return macApp{}, false
	}
	a := macApp{path: appPath, bundleID: plistString(info, "CFBundleIdentifier")}
	a.name = plistString(info, "CFBundleDisplayName")
	if a.name == "" {
		a.name = plistString(info, "CFBundleName")
	}
	if a.name == "" {
		a.name = strings.TrimSuffix(filepath.Base(appPath), ".app")
	}
	for _, t := range plistArray(info, "CFBundleDocumentTypes") {
		m, ok := t.(map[string]any)
		if !ok {
			continue
		}
		dt := macDocType{role: plistString(m, "CFBundleTypeRole")}
		for _, e := range plistArray(m, "CFBundleTypeExtensions") {
			if s, ok := e.(string); ok {
				dt.extensions = append(dt.extensions, strings.ToLower(s))
			}
		}
		for _, u := range plistArray(m, "LSItemContentTypes") {
			if s, ok := u.(string); ok {
				dt.utis = append(dt.utis, s)
			}
		}
		a.types = append(a.types, dt)
	}
	for _, key := range []string{"UTExportedTypeDeclarations", "UTImportedTypeDeclarations"} {
		for _, d := range plistArray(info, key) {
			m, ok := d.(map[string]any)
			if !ok {
				continue
			}
			id := plistString(m, "UTTypeIdentifier")
			if id == "" {
				continue
			}
			for _, c := range plistStrings(m, "UTTypeConformsTo") {
				conforms[id] = appendUnique(conforms[id], c)
			}
			if tags, ok := m["UTTypeTagSpecification"].(map[string]any); ok {
				for _, ext := range plistStrings(tags, "public.filename-extension") {
					ext = strings.ToLower(ext)
					extUTIs[ext] = appendUnique(extUTIs[ext], id)
				}
			}
		}
	}
	return a, true
}

func plistString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func plistArray(m map[string]any, key string) []any {
	a, _ := m[key].([]any)
	return a
}

// plistStrings reads a value that may be one string or an array of strings.
func plistStrings(m map[string]any, key string) []string {
	switch v := m[key].(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// macAppDirs lists the folders searched for applications.
func macAppDirs(home string) []string {
	dirs := []string{"/Applications", "/Applications/Utilities", "/System/Applications", "/System/Applications/Utilities"}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// scanMacApps reads every .app bundle directly inside dirs.
func scanMacApps(dirs []string) ([]macApp, map[string][]string, map[string][]string) {
	extUTIs := map[string][]string{}
	conforms := map[string][]string{}
	var apps []macApp
	seen := map[string]bool{}
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".app") {
				continue
			}
			p := filepath.Join(d, e.Name())
			if seen[p] {
				continue
			}
			seen[p] = true
			if a, ok := readMacApp(p, extUTIs, conforms); ok {
				apps = append(apps, a)
			}
		}
	}
	return apps, extUTIs, conforms
}

// utisForExt returns the identifiers of an extension with all their parents, most specific first.
func utisForExt(ext string, extUTIs, conforms map[string][]string) []string {
	ext = strings.ToLower(ext)
	var start []string
	if u, ok := builtinUTIs[ext]; ok {
		start = append(start, u)
	}
	for _, u := range extUTIs[ext] {
		start = appendUnique(start, u)
	}
	var out []string
	seen := map[string]bool{}
	queue := start
	for len(queue) > 0 && len(out) < 64 {
		u := queue[0]
		queue = queue[1:]
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
		queue = append(queue, builtinConforms[u]...)
		queue = append(queue, conforms[u]...)
	}
	if len(out) == 0 {
		out = []string{"public.data", "public.item"}
	}
	return out
}

// macAppsFor picks the apps that open files with extension ext. defaultID is the bundle id of the user's default
// handler ("" when unknown).
func macAppsFor(ext string, apps []macApp, extUTIs, conforms map[string][]string, defaultID string) []App {
	utis := utisForExt(ext, extUTIs, conforms)
	rankOf := map[string]int{}
	for _, u := range utis {
		r := 0
		if broadUTIs[u] {
			r = 1
		}
		rankOf[u] = r
	}
	lext := strings.ToLower(ext)
	var out []App
	for _, a := range apps {
		best := -1
		for _, t := range a.types {
			if strings.EqualFold(t.role, "None") {
				continue
			}
			for _, e := range t.extensions {
				switch {
				case e == lext && lext != "":
					best = 0
				case e == "*" && best < 0:
					best = 1
				}
			}
			for _, u := range t.utis {
				if r, ok := rankOf[u]; ok && (best < 0 || r < best) {
					best = r
				}
			}
		}
		isDefault := defaultID != "" && strings.EqualFold(a.bundleID, defaultID)
		if best < 0 && !isDefault {
			continue
		}
		if best < 0 {
			best = 0
		}
		out = append(out, App{Name: a.name, Path: a.path, Default: isDefault, rank: best})
	}
	return out
}

// macDefaultHandler finds the bundle id the user chose for an extension (or one of its types) in the
// LaunchServices preferences.
func macDefaultHandler(lsPlist []byte, ext string, utis []string) string {
	v, err := ParsePlist(lsPlist)
	if err != nil {
		return ""
	}
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	handlers := plistArray(m, "LSHandlers")
	role := func(h map[string]any) string {
		for _, k := range []string{"LSHandlerRoleAll", "LSHandlerRoleViewer", "LSHandlerRoleEditor"} {
			if s := plistString(h, k); s != "" && s != "-" {
				return s
			}
		}
		return ""
	}
	for _, h := range handlers {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if plistString(hm, "LSHandlerContentTagClass") == "public.filename-extension" &&
			strings.EqualFold(plistString(hm, "LSHandlerContentTag"), ext) {
			if r := role(hm); r != "" {
				return r
			}
		}
	}
	for _, u := range utis {
		if broadUTIs[u] {
			continue
		}
		for _, h := range handlers {
			hm, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if strings.EqualFold(plistString(hm, "LSHandlerContentType"), u) {
				if r := role(hm); r != "" {
					return r
				}
			}
		}
	}
	return ""
}
