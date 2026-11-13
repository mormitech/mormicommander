// This file writes multi-volume ("spanned" or "split") zip archives, the
// "Multiple disk archive" option of Total Commander's Pack dialog, used to fit
// a large archive onto size-limited media or e-mail attachments. We first
// build a normal zip in a temporary file and then cut it into parts named
// name.z01, name.z02, ... with the last part called name.zip, exactly the
// layout Info-ZIP, WinZip, 7-Zip and Total Commander produce and read. Cutting
// is not a plain byte split: the first part starts with the spanning
// signature, every local header and central directory record must lie
// entirely within one part, and the central directory must record for each
// file the part number it starts on and its offset within that part, so the
// directory is re-encoded with those values (using Zip64 fields when needed).
package ziparc

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"

	"mormicommander/internal/archive"
)

// MinVolumeSize is the smallest accepted volume size. Headers may not be
// split across parts, so a part must at least hold the largest header.
const MinVolumeSize = 64 << 10

type volWriter struct {
	ctx      context.Context
	mainPath string
	size     int64
	f        *os.File
	disk     int
	pos      int64
	names    []string
}

func (v *volWriter) open() error {
	name := volumeNameFor(v.mainPath, v.disk+1)
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	v.f, v.pos = f, 0
	v.names = append(v.names, name)
	return nil
}

func (v *volWriter) next() error {
	if err := v.f.Close(); err != nil {
		return err
	}
	v.disk++
	if v.disk >= 0xffff {
		return fmt.Errorf("ziparc: too many volumes")
	}
	return v.open()
}

// ensure starts a new volume when n bytes would not fit in the current one.
func (v *volWriter) ensure(n int64) error {
	if n > v.size {
		return fmt.Errorf("ziparc: volume size %d is too small for a %d byte header", v.size, n)
	}
	if v.pos+n > v.size {
		return v.next()
	}
	return nil
}

func (v *volWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		if err := v.ctx.Err(); err != nil {
			return written, err
		}
		room := v.size - v.pos
		if room == 0 {
			if err := v.next(); err != nil {
				return written, err
			}
			room = v.size
		}
		chunk := p
		if int64(len(chunk)) > room {
			chunk = chunk[:room]
		}
		n, err := v.f.Write(chunk)
		written += n
		v.pos += int64(n)
		if err != nil {
			return written, err
		}
		p = p[n:]
	}
	return written, nil
}

// finish closes the last part, gives it the main name and removes leftover
// parts of an older, longer set with the same name.
func (v *volWriter) finish() ([]string, error) {
	if err := v.f.Close(); err != nil {
		return nil, err
	}
	last := len(v.names) - 1
	if err := os.Rename(v.names[last], v.mainPath); err != nil {
		return nil, err
	}
	v.names[last] = v.mainPath
	for k := len(v.names); ; k++ {
		stale := volumeNameFor(v.mainPath, k)
		if os.Remove(stale) != nil {
			break
		}
	}
	return v.names, nil
}

func (v *volWriter) abort() {
	if v.f != nil {
		v.f.Close()
	}
	for _, n := range v.names {
		os.Remove(n)
	}
}

// splitVolumes cuts the finished single-file zip src into volumes of at most
// volSize bytes, the last one named mainPath. It returns the part names.
func splitVolumes(ctx context.Context, src, mainPath string, volSize int64) (names []string, err error) {
	if volSize < MinVolumeSize {
		return nil, fmt.Errorf("ziparc: volume size must be at least %d bytes", MinVolumeSize)
	}
	z, err := Open(src, archive.OpenOptions{})
	if err != nil {
		return nil, err
	}
	defer z.Close()
	recs := z.dir.records
	order := make([]int, len(recs))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return recs[order[a]].localOffset < recs[order[b]].localOffset })

	v := &volWriter{ctx: ctx, mainPath: mainPath, size: volSize}
	if err := v.open(); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			v.abort()
		}
	}()
	var sig [4]byte
	binary.LittleEndian.PutUint32(sig[:], sigDescriptor) // spanning marker
	if _, err := v.Write(sig[:]); err != nil {
		return nil, err
	}

	newDisk := make([]uint32, len(recs))
	newOff := make([]uint64, len(recs))
	for k, i := range order {
		r := &recs[i]
		start := z.dir.base + int64(r.localOffset)
		end := z.dir.cdStart
		if k+1 < len(order) {
			end = z.dir.base + int64(recs[order[k+1]].localOffset)
		}
		var h [localLen]byte
		if _, err := z.vol.ReadAt(h[:], start); err != nil {
			return nil, err
		}
		hdrLen := int64(localLen) + int64(binary.LittleEndian.Uint16(h[26:])) + int64(binary.LittleEndian.Uint16(h[28:]))
		if err := v.ensure(hdrLen); err != nil {
			return nil, err
		}
		newDisk[i], newOff[i] = uint32(v.disk), uint64(v.pos)
		if _, err := io.Copy(v, io.NewSectionReader(z.vol, start, end-start)); err != nil {
			return nil, err
		}
	}

	// Central directory, records kept whole.
	cdDisk, cdOff := uint32(v.disk), uint64(v.pos)
	var cdSize uint64
	recDisk := make([]int, len(recs))
	for i := range recs {
		enc := encodeCentral(&recs[i], newDisk[i], newOff[i])
		if err := v.ensure(int64(len(enc))); err != nil {
			return nil, err
		}
		if i == 0 {
			cdDisk, cdOff = uint32(v.disk), uint64(v.pos)
		}
		recDisk[i] = v.disk
		if _, err := v.Write(enc); err != nil {
			return nil, err
		}
		cdSize += uint64(len(enc))
	}

	comment := z.dir.commentRaw
	need64 := len(recs) >= 0xffff || cdSize >= 0xffffffff || cdOff >= 0xffffffff || cdDisk >= 0xffff
	trailer := int64(eocdLen + len(comment))
	if need64 {
		trailer += zip64EOCDLen + zip64LocLen
	}
	if err := v.ensure(trailer); err != nil {
		return nil, err
	}
	onDisk := 0
	for _, d := range recDisk {
		if d == v.disk {
			onDisk++
		}
	}
	disk := uint32(v.disk)
	if need64 {
		z64Off := uint64(v.pos)
		b := make([]byte, zip64EOCDLen+zip64LocLen)
		binary.LittleEndian.PutUint32(b, sigZip64EOCD)
		binary.LittleEndian.PutUint64(b[4:], zip64EOCDLen-12)
		binary.LittleEndian.PutUint16(b[12:], hostUnix<<8|45)
		binary.LittleEndian.PutUint16(b[14:], 45)
		binary.LittleEndian.PutUint32(b[16:], disk)
		binary.LittleEndian.PutUint32(b[20:], cdDisk)
		binary.LittleEndian.PutUint64(b[24:], uint64(onDisk))
		binary.LittleEndian.PutUint64(b[32:], uint64(len(recs)))
		binary.LittleEndian.PutUint64(b[40:], cdSize)
		binary.LittleEndian.PutUint64(b[48:], cdOff)
		l := b[zip64EOCDLen:]
		binary.LittleEndian.PutUint32(l, sigZip64Loc)
		binary.LittleEndian.PutUint32(l[4:], disk)
		binary.LittleEndian.PutUint64(l[8:], z64Off)
		binary.LittleEndian.PutUint32(l[16:], disk+1)
		if _, err := v.Write(b); err != nil {
			return nil, err
		}
	}
	e := make([]byte, eocdLen, eocdLen+len(comment))
	binary.LittleEndian.PutUint32(e, sigEOCD)
	binary.LittleEndian.PutUint16(e[4:], uint16(min(disk, 0xffff)))
	binary.LittleEndian.PutUint16(e[6:], uint16(min(cdDisk, 0xffff)))
	binary.LittleEndian.PutUint16(e[8:], uint16(min(onDisk, 0xffff)))
	binary.LittleEndian.PutUint16(e[10:], uint16(min(len(recs), 0xffff)))
	binary.LittleEndian.PutUint32(e[12:], uint32(min(cdSize, 0xffffffff)))
	binary.LittleEndian.PutUint32(e[16:], uint32(min(cdOff, 0xffffffff)))
	binary.LittleEndian.PutUint16(e[20:], uint16(len(comment)))
	e = append(e, comment...)
	if _, err := v.Write(e); err != nil {
		return nil, err
	}
	return v.finish()
}

// encodeCentral serializes a central directory record with a new starting
// disk and offset, adding a Zip64 block when a value does not fit.
func encodeCentral(r *record, disk uint32, off uint64) []byte {
	extra := stripExtra(r.extra, extraZip64)
	var z64 []byte
	u32, c32, o32 := uint32(r.usize), uint32(r.csize), uint32(off)
	if r.usize >= 0xffffffff {
		z64 = binary.LittleEndian.AppendUint64(z64, r.usize)
		u32 = 0xffffffff
	}
	if r.csize >= 0xffffffff {
		z64 = binary.LittleEndian.AppendUint64(z64, r.csize)
		c32 = 0xffffffff
	}
	if off >= 0xffffffff {
		z64 = binary.LittleEndian.AppendUint64(z64, off)
		o32 = 0xffffffff
	}
	if len(z64) > 0 {
		h := make([]byte, 4, 4+len(z64)+len(extra))
		binary.LittleEndian.PutUint16(h, extraZip64)
		binary.LittleEndian.PutUint16(h[2:], uint16(len(z64)))
		extra = append(append(h, z64...), extra...)
	}
	b := make([]byte, centralLen, centralLen+len(r.nameRaw)+len(extra)+len(r.commentRaw))
	binary.LittleEndian.PutUint32(b, sigCentral)
	binary.LittleEndian.PutUint16(b[4:], r.creatorVersion)
	binary.LittleEndian.PutUint16(b[6:], r.readerVersion)
	binary.LittleEndian.PutUint16(b[8:], r.flags)
	binary.LittleEndian.PutUint16(b[10:], r.method)
	binary.LittleEndian.PutUint16(b[12:], r.dosTime)
	binary.LittleEndian.PutUint16(b[14:], r.dosDate)
	binary.LittleEndian.PutUint32(b[16:], r.crc)
	binary.LittleEndian.PutUint32(b[20:], c32)
	binary.LittleEndian.PutUint32(b[24:], u32)
	binary.LittleEndian.PutUint16(b[28:], uint16(len(r.nameRaw)))
	binary.LittleEndian.PutUint16(b[30:], uint16(len(extra)))
	binary.LittleEndian.PutUint16(b[32:], uint16(len(r.commentRaw)))
	binary.LittleEndian.PutUint16(b[34:], uint16(disk))
	binary.LittleEndian.PutUint16(b[36:], r.internalAttr)
	binary.LittleEndian.PutUint32(b[38:], r.externalAttr)
	binary.LittleEndian.PutUint32(b[42:], o32)
	b = append(b, r.nameRaw...)
	b = append(b, extra...)
	return append(b, r.commentRaw...)
}
