// Package ziparc is mormiCommander's built-in zip packer, the equivalent of
// Total Commander's internal ZIP support: it opens zip files (and the many
// formats that are zip inside, such as .jar, .apk, .docx, .xlsx, .epub and
// self-extracting .exe files) as folders, unpacks and tests them, and creates
// or updates them with Store or Deflate at levels 1 to 9, ZipCrypto or AES
// encryption, archive and file comments, and multi-volume output. It needs no
// external tools. Importing the package registers the "zip" format with
// internal/archive, so the panel code only ever talks to archive.Archive and
// archive.Packer.
package ziparc

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"mormicommander/internal/archive"
)

// Extensions handled as zip when the user presses Enter.
var Extensions = []string{
	"zip", "zipx", "jar", "war", "ear", "apk", "aab", "xapk", "ipa", "appx", "msix",
	"docx", "docm", "xlsx", "xlsm", "pptx", "pptm", "odt", "ods", "odp", "odg",
	"epub", "cbz", "xpi", "crx", "whl", "nupkg", "vsix", "kmz", "sketch",
}

func init() {
	archive.Register(Format())
}

// partExtensions lets any part of a multi-volume set be opened by name:
// name.z01 ... name.z99 and name.zip.001 ... name.zip.099.
func partExtensions() []string {
	var out []string
	for i := 1; i <= 99; i++ {
		out = append(out, fmt.Sprintf("z%02d", i), fmt.Sprintf("zip.%03d", i))
	}
	return out
}

// Format returns the registration record for zip.
func Format() archive.Format {
	return archive.Format{
		Name:         "zip",
		Description:  "ZIP archive",
		Extensions:   append(append([]string(nil), Extensions...), partExtensions()...),
		Magic:        magic,
		Probe:        probe,
		Open:         openAny,
		OpenReaderAt: openReaderAtAny,
		Packer:       Packer{},
		Caps: archive.Caps{
			Write: true, Encrypt: true, Volumes: true, Comments: true, Levels: true,
			Methods:     []archive.Method{archive.MethodStore, archive.MethodDeflate},
			Encryptions: []archive.Encryption{archive.EncryptZipCrypto, archive.EncryptAES128, archive.EncryptAES192, archive.EncryptAES256},
		},
	}
}

// openAny and openReaderAtAny adapt the typed constructors to the registry
// signatures without turning a nil *Reader into a non-nil interface.
func openAny(p string, o archive.OpenOptions) (archive.Archive, error) {
	r, err := Open(p, o)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func openReaderAtAny(ra io.ReaderAt, size int64, name string, o archive.OpenOptions) (archive.Archive, error) {
	r, err := OpenReaderAt(ra, size, name, o)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func magic(h []byte) bool {
	if len(h) < 4 {
		return false
	}
	for _, sig := range [][]byte{
		{'P', 'K', 3, 4}, {'P', 'K', 5, 6}, {'P', 'K', 7, 8}, {'P', 'K', '0', '0'},
	} {
		if bytes.HasPrefix(h, sig) {
			return true
		}
	}
	return false
}

// probe finds archives with data in front (self-extractors) by their end
// record.
func probe(r io.ReaderAt, size int64) bool {
	_, err := findEOCD(r, size)
	return err == nil
}

// Open opens a zip file. For split sets any part may be given; all parts
// must sit in the same folder.
func Open(path string, opts archive.OpenOptions) (*Reader, error) {
	names, numbered, err := discoverVolumes(path)
	if err != nil {
		return nil, err
	}
	vol, err := openVolumes(names)
	if err != nil {
		return nil, err
	}
	dir, err := readDirectory(vol, numbered)
	if err == nil && !dir.spanned && !numbered && len(names) > 1 {
		// Stray .z01 files next to a normal zip: ignore them.
		vol.Close()
		names = names[len(names)-1:]
		if vol, err = openVolumes(names); err != nil {
			return nil, err
		}
		dir, err = readDirectory(vol, false)
	}
	if err != nil {
		vol.Close()
		return nil, err
	}
	return newArchive(names[len(names)-1], vol, dir, opts), nil
}

// OpenReaderAt opens a zip held in memory or inside another archive.
func OpenReaderAt(r io.ReaderAt, size int64, name string, opts archive.OpenOptions) (*Reader, error) {
	vol := singleVolume(r, size, name)
	dir, err := readDirectory(vol, false)
	if err != nil {
		return nil, err
	}
	if dir.spanned {
		return nil, archive.ErrMissingVolume
	}
	return newArchive(name, vol, dir, opts), nil
}

// IsZip reports whether the file looks like a zip archive by content.
func IsZip(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return false
	}
	h := make([]byte, 4)
	if n, _ := f.ReadAt(h, 0); n == 4 && magic(h) {
		return true
	}
	return probe(f, st.Size())
}
