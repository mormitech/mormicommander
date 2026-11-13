// Package archive is the format-neutral heart of mormiCommander's packer
// support. Total Commander lets the user press Enter on a zip, tar or 7z file
// and walk around inside it as if it were an ordinary folder, copy files out
// of it with F5, drop files into it, delete or rename entries, and test it for
// damage. To make that possible for many formats without the panel code caring
// which one it is looking at, every format implements the same small Archive
// interface (read side) and, when it can write, the Packer interface. This
// file defines those interfaces together with the Entry record that describes
// one file or folder inside an archive. The generic machinery that works on
// any Archive (extracting with zip-slip protection and overwrite questions,
// testing, nested archives, an fs.FS view) lives in the sibling files, and the
// concrete formats live in sub-packages such as internal/archive/ziparc that
// register themselves with Register.
package archive

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"time"
)

// DOS attribute bits as stored in zip files and on Windows file systems. They
// are kept separately from the Unix mode because Total Commander shows them in
// the attribute column ("rahs") and an archive made on Windows only has these.
const (
	AttrReadOnly uint8 = 0x01
	AttrHidden   uint8 = 0x02
	AttrSystem   uint8 = 0x04
	AttrDir      uint8 = 0x10
	AttrArchive  uint8 = 0x20
)

// Entry describes one file, folder or symbolic link inside an archive. The
// panel shows these exactly like files on disk, so the fields mirror what a
// directory listing needs, plus the packer-specific details that Total
// Commander's "Info on packed file" dialog shows (packed size, ratio, method,
// CRC).
type Entry struct {
	// ID is a format-specific handle for the record, stable while the archive
	// stays open. Implied folders (folders that only exist because files live
	// under them) have ID -1.
	ID int
	// Path is the cleaned, slash-separated path inside the archive without a
	// leading or trailing slash, for example "docs/readme.txt".
	Path string
	// Name is the last element of Path.
	Name string
	// RawName is the name as stored in the archive after character-set
	// decoding but before cleaning. It differs from Path when the stored name
	// used backslashes, was absolute, or contained ".." elements.
	RawName string
	// IsDir is true for folders, both explicit and implied.
	IsDir bool
	// Implied is true for folders that have no record of their own.
	Implied bool
	// Insecure is true when the stored name tried to leave the archive root
	// (absolute path or ".." elements). The entry is still listed under its
	// sanitized Path, and extraction always stays inside the target folder.
	Insecure bool
	// Size is the unpacked size in bytes.
	Size int64
	// PackedSize is the compressed size in bytes as stored in the archive,
	// including any encryption overhead.
	PackedSize int64
	// ModTime is the modification time. Formats that only store local DOS
	// time return it in time.Local.
	ModTime time.Time
	// Mode carries the permission bits and the type bits (fs.ModeDir,
	// fs.ModeSymlink). Archives made on Windows get a synthesized mode.
	Mode fs.FileMode
	// DOSAttrs holds the DOS/Windows attribute bits (AttrReadOnly, ...).
	DOSAttrs uint8
	// CRC32 is the stored checksum of the unpacked data when HasCRC is true.
	CRC32  uint32
	HasCRC bool
	// Method is a human-readable compression method name such as "Deflate".
	Method string
	// Encrypted is true when the entry needs a password; Encryption names the
	// scheme, for example "ZipCrypto" or "AES-256".
	Encrypted  bool
	Encryption string
	// Comment is the per-entry comment, if the format supports one.
	Comment string
}

// IsSymlink reports whether the entry is a symbolic link. Its target is the
// content returned by Archive.Open.
func (e Entry) IsSymlink() bool { return e.Mode&fs.ModeSymlink != 0 }

// Ratio returns the packed size as a percentage of the unpacked size, the way
// Total Commander shows it in the packed file info dialog. Empty files return
// 0 to avoid dividing by zero.
func (e Entry) Ratio() float64 {
	if e.Size <= 0 {
		return 0
	}
	return float64(e.PackedSize) * 100 / float64(e.Size)
}

// Archive is the read side every format implements. Implementations must be
// safe for concurrent Open calls, because the panel may preview one entry
// while a copy job extracts others.
type Archive interface {
	// Format returns the registered format name, for example "zip".
	Format() string
	// Path returns the file the archive was opened from (the first volume
	// for multi-volume sets), or the name given to OpenReaderAt.
	Path() string
	// Comment returns the archive comment, or "".
	Comment() string
	// Entries returns every entry: real records in archive order followed by
	// implied folders. The slice must not be modified.
	Entries() []Entry
	// ReadDir lists the direct children of dir ("" is the root), sorted by
	// name. It returns an error wrapping fs.ErrNotExist for unknown folders.
	ReadDir(dir string) ([]Entry, error)
	// Stat returns the entry for a path ("" is the root folder).
	Stat(name string) (Entry, error)
	// Open returns the unpacked content of a file entry. Reading it to the
	// end verifies the checksum; a mismatch is reported as ErrChecksum from
	// Read. Encrypted entries ask the OpenOptions.Password callback.
	Open(e Entry) (io.ReadCloser, error)
	// Close releases the underlying files.
	Close() error
}

// Method selects the compression method when packing.
type Method string

const (
	// MethodDefault lets the format choose (Deflate for zip).
	MethodDefault Method = ""
	// MethodStore keeps data uncompressed.
	MethodStore Method = "store"
	// MethodDeflate is the classic zip compression.
	MethodDeflate Method = "deflate"
)

// Encryption selects how a packer protects new entries with a password.
type Encryption int

const (
	// EncryptNone stores data in the clear. When a password is set anyway
	// the packer picks its strongest scheme (AES-256 for zip).
	EncryptNone Encryption = iota
	// EncryptZipCrypto is the legacy PKWARE scheme. It is weak but the only
	// one built-in unzippers (Windows Explorer, macOS Archive Utility) read.
	EncryptZipCrypto
	// EncryptAES128, EncryptAES192 and EncryptAES256 are the WinZip AES
	// schemes understood by 7-Zip, WinZip, WinRAR and Total Commander.
	EncryptAES128
	EncryptAES192
	EncryptAES256
)

// String returns the name shown in the UI.
func (e Encryption) String() string {
	switch e {
	case EncryptZipCrypto:
		return "ZipCrypto"
	case EncryptAES128:
		return "AES-128"
	case EncryptAES192:
		return "AES-192"
	case EncryptAES256:
		return "AES-256"
	}
	return "None"
}

// Packer is the write side. A single Update call applies a batch of changes
// (delete, rename, make folder, add, comment) and writes the result
// atomically: the new archive is built in a temporary file next to the old
// one and only renamed over it when everything succeeded, so a cancelled or
// failed update never damages the original. Creating a new archive is an
// Update on a path that does not exist yet.
type Packer interface {
	Update(ctx context.Context, archivePath string, ch Changes, opts PackOptions) (PackResult, error)
}

// Changes is the batch of edits applied by Packer.Update. They are applied in
// the order Delete, Rename, Mkdir, Add, comments.
type Changes struct {
	// Delete removes entries; a folder path removes everything under it.
	Delete []string
	// Rename moves entries; renaming a folder moves its whole subtree.
	Rename []Rename
	// Mkdir creates empty folders (parents are created as needed).
	Mkdir []string
	// Add packs local files and folders or in-memory content.
	Add []AddItem
	// Comment replaces the archive comment when non-nil.
	Comment *string
	// EntryComments sets per-entry comments, keyed by entry path.
	EntryComments map[string]string
}

// Rename moves one entry (or folder subtree) inside the archive.
type Rename struct {
	From, To string
}

// AddItem is one thing to pack. Either Source names a local file or folder
// (folders are packed recursively unless PackOptions.NoRecurse), or Open
// supplies content directly, which the UI uses for "copy from another archive
// into this one" and tests use for generated data.
type AddItem struct {
	// Source is a local file or folder path.
	Source string
	// Dest is the path inside the archive for this item. When empty, the
	// base name of Source is used, so the item lands in the archive root.
	Dest string
	// Open, when non-nil, provides file content instead of Source. Size,
	// ModTime and Mode describe that content; Mode may include fs.ModeDir to
	// add an empty folder.
	Open    func() (io.ReadCloser, error)
	Size    int64
	ModTime time.Time
	Mode    fs.FileMode
}

// PackOptions are the settings of Total Commander's Pack dialog that apply to
// a single archive.
type PackOptions struct {
	// Method and Level choose compression. Level runs from 1 (fastest) to 9
	// (best); 0 means the format default. Level is ignored for MethodStore.
	Method Method
	Level  int
	// Password, when non-empty, encrypts every new file with Encryption.
	Password   string
	Encryption Encryption
	// FlattenPaths stores only file names, without folders ("Pack path
	// names" switched off).
	FlattenPaths bool
	// NoRecurse packs only the files directly inside selected folders
	// ("Recursively pack subdirectories" switched off).
	NoRecurse bool
	// StoreSymlinks stores symbolic links as links instead of packing the
	// files they point to.
	StoreSymlinks bool
	// ExcludeMask skips local files whose names match this Total Commander
	// style mask (see ParseMask).
	ExcludeMask string
	// VolumeSize, when positive, splits a newly created archive into volumes
	// of at most this many bytes ("Multiple disk archive").
	VolumeSize int64
	// MoveSources deletes the packed local files and folders after the
	// archive was written and verified ("Move to archive").
	MoveSources bool
	// Policy and OnConflict decide what happens when an added path already
	// exists in the archive.
	Policy     ConflictPolicy
	OnConflict ConflictFunc
	// Progress, when non-nil, is called as data is packed.
	Progress ProgressFunc
}

// PackResult summarizes an Update.
type PackResult struct {
	Added, Replaced, Skipped, Deleted, Renamed int
	// Bytes is the number of unpacked bytes read from sources.
	Bytes int64
	// Volumes lists the files written for a multi-volume archive, in order.
	Volumes []string
	// Moved lists the local paths deleted because of MoveSources.
	Moved []string
}

// PasswordFunc asks the user for the password of an encrypted entry. attempt
// starts at 1 and grows after each wrong password. Returning an error (for
// example ErrCanceled when the user closes the dialog) stops the operation.
type PasswordFunc func(archivePath, entry string, attempt int) (string, error)

// OpenOptions configure how an archive is opened.
type OpenOptions struct {
	// Password is asked when an encrypted entry is opened. A password that
	// worked once is remembered for the rest of the session.
	Password PasswordFunc
	// Encoding selects the character set used for names that are not marked
	// as UTF-8 in old archives: "" or "auto" (UTF-8 if every name is valid
	// UTF-8, else the system OEM code page), or a name such as "cp852",
	// "cp437", "cp866", "windows-1250" or "shift_jis". See Encodings.
	Encoding string
}

// Progress is reported by long-running operations. Done and Total count
// unpacked bytes over the whole job; FileDone and FileTotal the current file.
type Progress struct {
	Op         string // "extract", "test", "pack" or "copy"
	Path       string // current entry or source path
	FileDone   int64
	FileTotal  int64
	Done       int64
	Total      int64
	Files      int // files finished so far
	TotalFiles int
}

// ProgressFunc receives progress reports. It is called from the worker
// goroutine and should return quickly; the UI layer throttles redraws.
type ProgressFunc func(Progress)

// Errors shared by all formats. Callers test them with errors.Is.
var (
	ErrFormat            = errors.New("archive: not a valid archive or archive is damaged")
	ErrPasswordRequired  = errors.New("archive: password required")
	ErrBadPassword       = errors.New("archive: wrong password")
	ErrChecksum          = errors.New("archive: checksum error, data is damaged")
	ErrUnsupportedMethod = errors.New("archive: unsupported compression method")
	ErrInsecurePath      = errors.New("archive: entry path leaves the target folder")
	ErrReadOnly          = errors.New("archive: format or archive cannot be modified")
	ErrMissingVolume     = errors.New("archive: volume of a multi-volume archive is missing")
	ErrCanceled          = errors.New("archive: canceled by user")
	ErrNotFound          = fs.ErrNotExist
	ErrExist             = fs.ErrExist
)
