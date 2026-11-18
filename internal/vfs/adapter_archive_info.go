// This file answers TC's "Info on packed file" (Alt+Enter on an entry inside
// an archive, FEATURES.md F-847): unpacked and packed size, ratio, date, CRC
// and method of one entry, plus the archive's comment (F-622), which zips
// can also change. SetEncoding chooses the code page for names in old zips
// that are not marked as UTF-8 (F-848).
package vfs

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"mormicommander/internal/archive"
	"mormicommander/internal/archive/arcapi"
)

// PackedInfo describes one entry inside an archive.
type PackedInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`    // slash path inside the archive
	Archive    string `json:"archive"` // the panel path of the archive
	Format     string `json:"format"`
	IsDir      bool   `json:"isDir"`
	Size       int64  `json:"size"`   // unpacked, -1 when unknown
	Packed     int64  `json:"packed"` // stored size, -1 when unknown (7z solid blocks, tar streams)
	Ratio      int    `json:"ratio"`  // packed size in percent of the unpacked size, -1 when unknown
	MTime      int64  `json:"mtime"`  // Unix milliseconds, 0 when unknown
	CRC        string `json:"crc"`    // 8 hex digits, "" when the format stores none
	Method     string `json:"method"`
	Encrypted  bool   `json:"encrypted"`
	Encryption string `json:"encryption"`
	Comment    string `json:"comment"`        // the entry's own comment (zip)
	ArcComment string `json:"archiveComment"` // the archive's comment (zip), or the volume label (iso)
	CanComment bool   `json:"canComment"`     // the archive comment can be changed here
}

func ratioOf(size, packed int64) int {
	if size <= 0 || packed < 0 {
		return -1
	}
	return int((packed*100 + size/2) / size)
}

// PackedInfo describes the entry at p inside an archive.
func (a *ArchiveFS) PackedInfo(p string) (PackedInfo, error) {
	p = a.Clean(p)
	h, inner, err := a.locate(context.Background(), p, locFile)
	if err != nil {
		return PackedInfo{}, err
	}
	defer a.release(h)
	out := PackedInfo{Path: inner, Name: path.Base(inner), Archive: h.display, Format: h.be.format(), Size: -1, Packed: -1, Ratio: -1}
	if inner == "" {
		out.Name, out.IsDir = path.Base(strings.ReplaceAll(h.display, `\`, "/")), true
	}
	out.CanComment = out.Format == "zip" && h.be.writable() && !h.readOnly
	fill := func(e archive.Entry) {
		out.IsDir, out.Size, out.Packed = e.IsDir, e.Size, e.PackedSize
		if !e.ModTime.IsZero() {
			out.MTime = e.ModTime.UnixMilli()
		}
		if e.HasCRC {
			out.CRC = fmt.Sprintf("%08X", e.CRC32)
		}
		out.Method, out.Encrypted, out.Encryption, out.Comment = e.Method, e.Encrypted, e.Encryption, e.Comment
	}
	switch b := h.be.(type) {
	case *zipBackend:
		out.ArcComment = b.r.Comment()
		if inner != "" {
			e, err := b.r.Stat(inner)
			if err != nil {
				return PackedInfo{}, notExist(err, p)
			}
			fill(e)
		}
	case *isoBackend:
		out.ArcComment = b.r.Label()
		if inner != "" {
			e, err := b.r.Stat(inner)
			if err != nil {
				return PackedInfo{}, notExist(err, p)
			}
			fill(e)
			out.Packed = e.Size // (stored uncompressed)
		}
	case *tarBackend:
		if inner != "" {
			e, err := b.r.Stat(context.Background(), inner)
			if err != nil {
				return PackedInfo{}, notExist(err, p)
			}
			fillArc(&out, e)
			out.Method = b.r.Format().String()
		}
	case *sevenBackend:
		if inner != "" {
			e, err := b.a.Stat(inner)
			if err != nil {
				return PackedInfo{}, notExist(err, p)
			}
			out.IsDir, out.Size, out.Packed, out.Encrypted = e.IsDir, e.Size, e.PackedSize, e.Encrypted
			if !e.ModTime.IsZero() {
				out.MTime = e.ModTime.UnixMilli()
			}
			if e.HasCRC {
				out.CRC = fmt.Sprintf("%08X", e.CRC32)
			}
			out.Method = strings.ToUpper(b.a.Format().String())
			if e.Solid {
				out.Method += " (solid)"
			}
		}
	}
	if out.IsDir {
		out.Size, out.Packed = -1, -1
	}
	out.Ratio = ratioOf(out.Size, out.Packed)
	return out, nil
}

func fillArc(out *PackedInfo, e arcapi.Entry) {
	out.IsDir, out.Size, out.Packed = e.IsDir, e.Size, e.Packed
	if !e.ModTime.IsZero() {
		out.MTime = e.ModTime.UnixMilli()
	}
}

// ErrNoComment is returned by SetComment for archives that hold no comment.
var ErrNoComment = errors.New("only ZIP archives have a comment that can be changed")

// SetComment replaces the comment of the zip archive p is in ("" removes
// it). The archive is rewritten atomically like every other change.
func (a *ArchiveFS) SetComment(ctx context.Context, p, comment string) error {
	if len(comment) > 0xffff {
		return errors.New("the comment is longer than 65535 bytes")
	}
	return a.modify(ctx, p, locMissing, func(h *arcHandle, _ string) (arcChanges, error) {
		if _, ok := h.be.(*zipBackend); !ok {
			return arcChanges{}, ErrNoComment
		}
		return arcChanges{Comment: &comment}, nil
	})
}

// SetEncoding sets the code page for names in old zips that are not marked
// as UTF-8 ("" or "auto" guesses, "cp852", "cp437", ...). Archives already
// open are read again with it on their next use.
func (a *ArchiveFS) SetEncoding(enc string) error {
	enc = strings.TrimSpace(enc)
	if enc != "" && !strings.EqualFold(enc, "auto") {
		if _, ok := archive.LookupEncoding(enc); !ok {
			return fmt.Errorf("unknown encoding %q", enc)
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Encoding == enc {
		return nil
	}
	a.Encoding = enc
	for _, h := range a.handles {
		a.dropHandleLocked(h)
	}
	return nil
}
