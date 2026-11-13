// File properties information for the Properties window (Alt+Enter). Besides
// what Stat returns for a single entry, that window needs two things that take
// more work: the totals of a folder or a multiple selection (how many files
// and folders, how many bytes, and how much disk space they really occupy,
// counting hard-linked files once, as du does) and a human type line such as
// "PNG image", "Mach-O executable" or "Symbolic link to folder". The type is
// worked out from the extension first and, for unknown extensions, from the
// first bytes of the file, which is how Finder and file(1) avoid calling every
// binary just "file".
package attrs

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Totals sums up a folder tree or a multiple selection.
type Totals struct {
	Files      int64 // regular files and other non-folder entries
	Folders    int64 // folders below the selection (selected folders count too)
	Links      int64 // symbolic links (not followed)
	Size       int64 // logical bytes, hard links counted once
	SizeOnDisk int64 // allocated bytes, hard links counted once
	Errors     int   // entries that could not be read
}

// ComputeTotals walks the given paths without following symbolic links and
// adds everything up. progress, when not nil, is called now and then with the
// running totals so a dialog can update while a big tree is counted. The walk
// stops with ctx.Err() when ctx is canceled; the totals so far are returned.
func ComputeTotals(ctx context.Context, paths []string, progress func(Totals)) (Totals, error) {
	var t Totals
	type fileID struct{ dev, ino uint64 }
	seen := map[fileID]bool{}
	visited := 0
	var walk func(path string) error
	walk = func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := Stat(path)
		if err != nil {
			t.Errors++
			return nil
		}
		visited++
		if progress != nil && visited%256 == 0 {
			progress(t)
		}
		switch info.Kind {
		case KindDir:
			t.Folders++
			if info.SizeOnDisk > 0 {
				t.SizeOnDisk += info.SizeOnDisk
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				t.Errors++
				return nil
			}
			for _, e := range entries {
				if err := walk(filepath.Join(path, e.Name())); err != nil {
					return err
				}
			}
			return nil
		case KindSymlink:
			t.Links++
			return nil
		}
		t.Files++
		if info.Links > 1 && info.Inode != 0 {
			id := fileID{info.Device, info.Inode}
			if seen[id] {
				return nil
			}
			seen[id] = true
		}
		t.Size += info.Size
		if info.SizeOnDisk >= 0 {
			t.SizeOnDisk += info.SizeOnDisk
		}
		return nil
	}
	for _, p := range paths {
		if err := walk(p); err != nil {
			return t, err
		}
	}
	if progress != nil {
		progress(t)
	}
	return t, nil
}

// TypeInfo is the type line of the Properties window.
type TypeInfo struct {
	Kind        Kind
	Description string // e.g. "PNG image", "Folder", "Symbolic link to file"
	MIME        string // e.g. "image/png"; empty for folders and specials
	Executable  bool   // a program or script the system can run
}

// categories maps a MIME main type to a word used in descriptions.
var categories = map[string]string{
	"image": "image", "video": "video", "audio": "audio", "text": "text document", "font": "font",
}

// knownTypes names common extensions mime.TypeByExtension does not know on
// every platform, or knows only by an unhelpful name.
var knownTypes = map[string]struct{ desc, mime string }{
	".go":   {"Go source file", "text/x-go"},
	".md":   {"Markdown document", "text/markdown"},
	".sh":   {"Shell script", "text/x-shellscript"},
	".py":   {"Python script", "text/x-python"},
	".js":   {"JavaScript file", "text/javascript"},
	".json": {"JSON document", "application/json"},
	".zip":  {"ZIP archive", "application/zip"},
	".7z":   {"7-Zip archive", "application/x-7z-compressed"},
	".rar":  {"RAR archive", "application/vnd.rar"},
	".tar":  {"TAR archive", "application/x-tar"},
	".gz":   {"GZIP archive", "application/gzip"},
	".tgz":  {"TAR GZIP archive", "application/gzip"},
	".xz":   {"XZ archive", "application/x-xz"},
	".bz2":  {"BZIP2 archive", "application/x-bzip2"},
	".pdf":  {"PDF document", "application/pdf"},
	".exe":  {"Windows program", "application/vnd.microsoft.portable-executable"},
	".dll":  {"Windows library", "application/vnd.microsoft.portable-executable"},
	".bat":  {"Windows batch file", "application/x-bat"},
	".cmd":  {"Windows command script", "application/x-bat"},
	".lnk":  {"Windows shortcut", "application/x-ms-shortcut"},
	".dmg":  {"Disk image", "application/x-apple-diskimage"},
	".iso":  {"Disc image", "application/x-iso9660-image"},
	".txt":  {"Text document", "text/plain"},
}

// DescribeType works out the type line for an entry. For regular files it
// may read up to 512 bytes of the file.
func DescribeType(info Info) TypeInfo {
	ti := TypeInfo{Kind: info.Kind}
	switch info.Kind {
	case KindDir:
		ti.Description = "Folder"
		switch strings.ToLower(filepath.Ext(info.Name)) {
		case ".app":
			ti.Description = "Application bundle"
			ti.Executable = true
		case ".bundle", ".framework", ".plugin", ".kext":
			ti.Description = "Bundle"
		}
		return ti
	case KindSymlink:
		ti.Description = "Symbolic link"
		switch {
		case info.LinkBroken:
			ti.Description += " (target missing)"
		default:
			if fi, err := os.Stat(info.Path); err == nil {
				if fi.IsDir() {
					ti.Description += " to folder"
				} else {
					ti.Description += " to file"
				}
			}
		}
		return ti
	case KindFile:
	default:
		ti.Description = info.Kind.String()
		return ti
	}

	ext := strings.ToLower(filepath.Ext(info.Name))
	var head []byte
	if f, err := os.Open(info.Path); err == nil {
		buf := make([]byte, 512)
		n, _ := io.ReadFull(f, buf)
		head = buf[:n]
		f.Close()
	}
	isUnixExec := info.Mode&0o111 != 0 && !info.WinNative
	if d, ok := magicDescription(head); ok {
		ti.Description, ti.MIME = d.desc, d.mime
		// A Windows program is a program wherever it sits; ELF and Mach-O
		// binaries and scripts only run when their execute bit is set.
		ti.Executable = d.exec && (isUnixExec || d.mime == "application/vnd.microsoft.portable-executable")
		if ti.Executable || d.binary {
			return ti
		}
	}
	if k, ok := knownTypes[ext]; ok {
		ti.Description, ti.MIME = k.desc, k.mime
	} else if mt := mime.TypeByExtension(ext); ext != "" && mt != "" {
		ti.MIME = stripParams(mt)
		ti.Description = describeMIME(ext, ti.MIME)
	} else if ti.Description == "" {
		if len(head) > 0 {
			ti.MIME = stripParams(http.DetectContentType(head))
		}
		switch {
		case ext != "":
			ti.Description = strings.ToUpper(ext[1:]) + " file"
		case strings.HasPrefix(ti.MIME, "text/"):
			ti.Description = "Text document"
		default:
			ti.Description = "File"
		}
	}
	switch ext {
	case ".exe", ".com", ".bat", ".cmd":
		ti.Executable = info.WinNative
	}
	if isUnixExec {
		ti.Executable = true
	}
	return ti
}

type magicType struct {
	desc, mime   string
	exec, binary bool
}

// magicDescription recognises executables and scripts by their first bytes.
func magicDescription(head []byte) (magicType, bool) {
	switch {
	case bytes.HasPrefix(head, []byte("\x7fELF")):
		return magicType{"ELF executable", "application/x-elf", true, true}, true
	case len(head) >= 4 && (bytes.Equal(head[:4], []byte{0xcf, 0xfa, 0xed, 0xfe}) ||
		bytes.Equal(head[:4], []byte{0xce, 0xfa, 0xed, 0xfe}) ||
		bytes.Equal(head[:4], []byte{0xfe, 0xed, 0xfa, 0xcf}) ||
		bytes.Equal(head[:4], []byte{0xfe, 0xed, 0xfa, 0xce})):
		return magicType{"Mach-O executable", "application/x-mach-binary", true, true}, true
	case len(head) >= 8 && bytes.Equal(head[:4], []byte{0xca, 0xfe, 0xba, 0xbe}) && head[7] < 20:
		// Universal (fat) Mach-O; Java class files share the magic but
		// have a large version number in bytes 6-7.
		return magicType{"Mach-O universal executable", "application/x-mach-binary", true, true}, true
	case bytes.HasPrefix(head, []byte("MZ")):
		return magicType{"Windows program", "application/vnd.microsoft.portable-executable", true, true}, true
	case bytes.HasPrefix(head, []byte("#!")):
		line := head
		if i := bytes.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		desc := "Script"
		for _, k := range []struct{ word, name string }{
			{"python", "Python script"}, {"node", "JavaScript script"}, {"perl", "Perl script"},
			{"ruby", "Ruby script"}, {"bash", "Shell script"}, {"zsh", "Shell script"}, {"sh", "Shell script"},
		} {
			if bytes.Contains(line, []byte(k.word)) {
				desc = k.name
				break
			}
		}
		return magicType{desc, "text/x-script", true, false}, true
	}
	return magicType{}, false
}

func stripParams(mt string) string {
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		return strings.TrimSpace(mt[:i])
	}
	return mt
}

// describeMIME builds "PNG image" style text from an extension and MIME type.
func describeMIME(ext, mt string) string {
	main, _, _ := strings.Cut(mt, "/")
	word := categories[main]
	if word == "" {
		word = "file"
	}
	return strings.ToUpper(strings.TrimPrefix(ext, ".")) + " " + word
}
