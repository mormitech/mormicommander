// This file unpacks the content of one zip entry: it finds the data after
// the local header, asks for the password when the entry is encrypted (and
// asks again after a wrong one, remembering the one that worked for the rest
// of the session, as Total Commander does), decrypts ZipCrypto or AES,
// decompresses Store, Deflate or BZip2 data, and verifies the size and CRC32
// when the caller reaches the end. Decompression errors are reported as
// checksum errors because, for the user, both mean "this file is damaged (or
// the password is wrong)".
package ziparc

import (
	"compress/bzip2"
	"compress/flate"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	"mormicommander/internal/archive"
)

// Open returns the unpacked content of a file entry.
func (z *Reader) Open(e archive.Entry) (io.ReadCloser, error) {
	if e.IsDir {
		return nil, fmt.Errorf("ziparc: %s is a folder", e.Path)
	}
	r, err := z.record(e)
	if err != nil {
		return nil, err
	}
	pos, err := z.dataPos(r)
	if err != nil {
		return nil, err
	}
	raw := io.NewSectionReader(z.vol, pos, int64(r.csize))
	var src io.Reader = raw
	checkCRC := true
	if r.encrypted() {
		if r.flags&flagStrong != 0 {
			return nil, fmt.Errorf("%w: PKWARE strong encryption", archive.ErrUnsupportedMethod)
		}
		if r.method == methodAES {
			if r.x.aes == nil {
				return nil, fmt.Errorf("%w: AES entry without AES header", archive.ErrFormat)
			}
			src, err = z.unlockAES(e, r, raw)
			checkCRC = r.x.aes.vendorVersion != 2
		} else {
			src, err = z.unlockZipCrypto(e, r, raw)
		}
		if err != nil {
			return nil, err
		}
	}
	var dec io.Reader
	var closer io.Closer
	switch m := r.realMethod(); m {
	case methodStore:
		dec = src
	case methodDeflate:
		fr := flate.NewReader(src)
		dec, closer = fr, fr
	case methodBzip2:
		dec = bzip2.NewReader(src)
	default:
		return nil, fmt.Errorf("%w: %s", archive.ErrUnsupportedMethod, methodName(m))
	}
	return &checkReader{
		r: dec, closer: closer, hash: crc32.NewIEEE(),
		want: r.crc, size: r.usize, checkCRC: checkCRC, encrypted: r.encrypted(), name: e.Path,
	}, nil
}

// checkReader verifies size and CRC32 at the end of the stream.
type checkReader struct {
	r         io.Reader
	closer    io.Closer
	hash      hash.Hash32
	want      uint32
	size      uint64
	n         uint64
	checkCRC  bool
	encrypted bool
	name      string
	err       error
}

func (c *checkReader) Read(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.r.Read(p)
	c.hash.Write(p[:n])
	c.n += uint64(n)
	if c.n > c.size {
		c.err = fmt.Errorf("%w: %s is longer than recorded", archive.ErrChecksum, c.name)
		return n, c.err
	}
	switch {
	case err == io.EOF:
		if c.n != c.size {
			c.err = fmt.Errorf("%w: %s is shorter than recorded", archive.ErrChecksum, c.name)
		} else if c.checkCRC && c.hash.Sum32() != c.want {
			if c.encrypted {
				c.err = fmt.Errorf("%w: CRC mismatch in %s (wrong password?)", archive.ErrChecksum, c.name)
			} else {
				c.err = fmt.Errorf("%w: CRC mismatch in %s", archive.ErrChecksum, c.name)
			}
		} else {
			c.err = io.EOF
		}
		return n, c.err
	case err != nil:
		if !errors.Is(err, archive.ErrChecksum) && !errors.Is(err, archive.ErrMissingVolume) {
			err = fmt.Errorf("%w: %s: %v", archive.ErrChecksum, c.name, err)
		}
		c.err = err
		return n, err
	}
	return n, nil
}

func (c *checkReader) Close() error {
	if c.closer != nil {
		return c.closer.Close()
	}
	return nil
}

// withPassword runs try with the remembered password first and then with
// passwords from the callback until try accepts one. try returns ok == false
// for a wrong password.
func (z *Reader) withPassword(name string, try func(pw []byte) (bool, error)) error {
	z.mu.Lock()
	pw, have := z.password, z.havePW
	z.mu.Unlock()
	if have {
		ok, err := try(pw)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	if z.opts.Password == nil {
		if have {
			return fmt.Errorf("%w: %s", archive.ErrBadPassword, name)
		}
		return fmt.Errorf("%w: %s", archive.ErrPasswordRequired, name)
	}
	for attempt := 1; ; attempt++ {
		s, err := z.opts.Password(z.path, name, attempt)
		if err != nil {
			return err
		}
		ok, err := try([]byte(s))
		if err != nil {
			return err
		}
		if ok {
			z.mu.Lock()
			z.password, z.havePW = []byte(s), true
			z.mu.Unlock()
			return nil
		}
	}
}

// SetPassword sets the password tried first for encrypted entries.
func (z *Reader) SetPassword(pw string) {
	z.mu.Lock()
	z.password, z.havePW = []byte(pw), true
	z.mu.Unlock()
}

func (z *Reader) unlockZipCrypto(e archive.Entry, r *record, raw *io.SectionReader) (io.Reader, error) {
	if r.csize < 12 {
		return nil, fmt.Errorf("%w: encrypted entry too short", archive.ErrFormat)
	}
	var header [12]byte
	if _, err := raw.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	// The check byte is the high byte of the CRC, or of the DOS time when
	// sizes and CRC follow the data in a descriptor.
	check := byte(r.crc >> 24)
	if r.flags&flagDescriptor != 0 {
		check = byte(r.dosTime >> 8)
	}
	var keys *zipCryptoKeys
	err := z.withPassword(e.Path, func(pw []byte) (bool, error) {
		k, ok := zipCryptoCheck(header, pw, check)
		if ok {
			keys = k
		}
		return ok, nil
	})
	if err != nil {
		return nil, err
	}
	return &zipCryptoReader{r: io.NewSectionReader(raw, 12, int64(r.csize)-12), k: keys}, nil
}

func (z *Reader) unlockAES(e archive.Entry, r *record, raw *io.SectionReader) (io.Reader, error) {
	var out io.Reader
	err := z.withPassword(e.Path, func(pw []byte) (bool, error) {
		rd, ok, err := aesOpen(raw, int64(r.csize), r.x.aes, pw)
		if ok {
			out = rd
		}
		return ok, err
	})
	return out, err
}
