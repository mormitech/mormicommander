// This file implements the two password schemes zip files use. ZipCrypto
// (the "traditional PKWARE encryption") is weak but still what Windows
// Explorer and macOS Archive Utility understand, so we read it and can write
// it on request. WinZip AES (AE-1 and AE-2, 128/192/256-bit keys) is the
// modern standard used by 7-Zip, WinZip, WinRAR and Total Commander: keys are
// derived with PBKDF2-HMAC-SHA1 (1000 rounds), data is encrypted with AES in
// a little-endian counter mode, and an HMAC-SHA1 tag over the ciphertext
// detects both wrong passwords and tampering. Readers and writers here work
// as streams so large files never need to be held in memory.
package ziparc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"

	"mormicommander/internal/archive"
)

// ---- ZipCrypto ----

type zipCryptoKeys [3]uint32

func newZipCryptoKeys(password []byte) *zipCryptoKeys {
	k := &zipCryptoKeys{0x12345678, 0x23456789, 0x34567890}
	for _, b := range password {
		k.update(b)
	}
	return k
}

func crcByte(crc uint32, b byte) uint32 {
	return crc32.IEEETable[(crc^uint32(b))&0xff] ^ crc>>8
}

func (k *zipCryptoKeys) update(b byte) {
	k[0] = crcByte(k[0], b)
	k[1] = (k[1]+k[0]&0xff)*134775813 + 1
	k[2] = crcByte(k[2], byte(k[1]>>24))
}

func (k *zipCryptoKeys) stream() byte {
	t := k[2] | 2
	return byte((t * (t ^ 1)) >> 8)
}

func (k *zipCryptoKeys) decrypt(buf []byte) {
	for i, c := range buf {
		p := c ^ k.stream()
		k.update(p)
		buf[i] = p
	}
}

func (k *zipCryptoKeys) encrypt(buf []byte) {
	for i, p := range buf {
		c := p ^ k.stream()
		k.update(p)
		buf[i] = c
	}
}

// zipCryptoCheck decrypts the 12-byte encryption header with password and
// reports whether its check byte matches. On success it returns the keys
// positioned at the start of the data.
func zipCryptoCheck(header [12]byte, password []byte, check byte) (*zipCryptoKeys, bool) {
	k := newZipCryptoKeys(password)
	h := header
	k.decrypt(h[:])
	return k, h[11] == check
}

type zipCryptoReader struct {
	r io.Reader
	k *zipCryptoKeys
}

func (z *zipCryptoReader) Read(p []byte) (int, error) {
	n, err := z.r.Read(p)
	z.k.decrypt(p[:n])
	return n, err
}

type zipCryptoWriter struct {
	w   io.Writer
	k   *zipCryptoKeys
	buf []byte
}

// newZipCryptoWriter writes the encryption header (whose last byte is the
// high byte of the CRC, the check value readers compare against) and returns
// a writer that encrypts the data that follows.
func newZipCryptoWriter(w io.Writer, password []byte, crc uint32) (*zipCryptoWriter, error) {
	k := newZipCryptoKeys(password)
	var h [12]byte
	if _, err := rand.Read(h[:11]); err != nil {
		return nil, err
	}
	h[11] = byte(crc >> 24)
	k.encrypt(h[:])
	if _, err := w.Write(h[:]); err != nil {
		return nil, err
	}
	return &zipCryptoWriter{w: w, k: k}, nil
}

func (z *zipCryptoWriter) Write(p []byte) (int, error) {
	if cap(z.buf) < len(p) {
		z.buf = make([]byte, len(p))
	}
	b := z.buf[:len(p)]
	copy(b, p)
	z.k.encrypt(b)
	return z.w.Write(b)
}

// ---- WinZip AES ----

const aesAuthLen = 10

// winzipCTR is AES in counter mode with a little-endian counter starting at
// 1, which is what WinZip specifies and why crypto/cipher.NewCTR (big-endian)
// cannot be used.
type winzipCTR struct {
	block cipher.Block
	ctr   [aes.BlockSize]byte
	ks    [aes.BlockSize]byte
	pos   int
}

func newWinzipCTR(key []byte) (*winzipCTR, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &winzipCTR{block: b, pos: aes.BlockSize}, nil
}

func (c *winzipCTR) xor(dst, src []byte) {
	for i := range src {
		if c.pos == aes.BlockSize {
			for j := range c.ctr {
				c.ctr[j]++
				if c.ctr[j] != 0 {
					break
				}
			}
			c.block.Encrypt(c.ks[:], c.ctr[:])
			c.pos = 0
		}
		dst[i] = src[i] ^ c.ks[c.pos]
		c.pos++
	}
}

// aesKeys derives the encryption key, the HMAC key and the two-byte password
// verifier from a password and salt.
func aesKeys(password, salt []byte, keyLen int) (enc, mac, verify []byte, err error) {
	dk, err := pbkdf2.Key(sha1.New, string(password), salt, 1000, 2*keyLen+2)
	if err != nil {
		return nil, nil, nil, err
	}
	return dk[:keyLen], dk[keyLen : 2*keyLen], dk[2*keyLen:], nil
}

var errAESAuth = fmt.Errorf("%w: AES authentication code mismatch", archive.ErrChecksum)

// aesReader decrypts the ciphertext that follows salt and verifier and checks
// the authentication code when the end is reached.
type aesReader struct {
	r      io.Reader // positioned at the ciphertext
	remain int64     // ciphertext bytes left
	ctr    *winzipCTR
	mac    hash.Hash
	done   bool
}

func (a *aesReader) Read(p []byte) (int, error) {
	if a.remain <= 0 {
		if !a.done {
			a.done = true
			var tag [aesAuthLen]byte
			if _, err := io.ReadFull(a.r, tag[:]); err != nil {
				return 0, fmt.Errorf("%w: truncated AES data", archive.ErrChecksum)
			}
			if subtle.ConstantTimeCompare(tag[:], a.mac.Sum(nil)[:aesAuthLen]) != 1 {
				return 0, errAESAuth
			}
		}
		return 0, io.EOF
	}
	if int64(len(p)) > a.remain {
		p = p[:a.remain]
	}
	n, err := a.r.Read(p)
	if n > 0 {
		a.mac.Write(p[:n])
		a.ctr.xor(p[:n], p[:n])
		a.remain -= int64(n)
	}
	if err == io.EOF {
		if a.remain > 0 {
			return n, fmt.Errorf("%w: truncated AES data", archive.ErrChecksum)
		}
		err = nil
	}
	return n, err
}

// aesOpen checks the password against the verifier at the start of raw and
// returns the decrypting reader. ok is false for a wrong password.
func aesOpen(raw io.ReaderAt, csize int64, info *aesInfo, password []byte) (io.Reader, bool, error) {
	sl := info.saltLen()
	if info.strength < 1 || info.strength > 3 {
		return nil, false, fmt.Errorf("%w: unknown AES strength %d", archive.ErrUnsupportedMethod, info.strength)
	}
	if csize < int64(sl+2+aesAuthLen) {
		return nil, false, fmt.Errorf("%w: AES entry too short", archive.ErrFormat)
	}
	head := make([]byte, sl+2)
	if _, err := raw.ReadAt(head, 0); err != nil {
		return nil, false, err
	}
	enc, mac, verify, err := aesKeys(password, head[:sl], info.keyLen())
	if err != nil {
		return nil, false, err
	}
	if subtle.ConstantTimeCompare(verify, head[sl:]) != 1 {
		return nil, false, nil
	}
	ctr, err := newWinzipCTR(enc)
	if err != nil {
		return nil, false, err
	}
	body := int64(sl + 2)
	return &aesReader{
		r:      io.NewSectionReader(raw, body, csize-body),
		remain: csize - body - aesAuthLen,
		ctr:    ctr,
		mac:    hmac.New(sha1.New, mac),
	}, true, nil
}

// aesWriter encrypts a stream for a new entry; Close appends the
// authentication code.
type aesWriter struct {
	w   io.Writer
	ctr *winzipCTR
	mac hash.Hash
	buf []byte
}

// newAESWriter writes salt and verifier and returns the encrypting writer.
// The overhead added to the compressed size is saltLen+2+10 bytes.
func newAESWriter(w io.Writer, password []byte, strength byte) (*aesWriter, error) {
	info := aesInfo{strength: strength}
	salt := make([]byte, info.saltLen())
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	enc, mac, verify, err := aesKeys(password, salt, info.keyLen())
	if err != nil {
		return nil, err
	}
	ctr, err := newWinzipCTR(enc)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(append(salt, verify...)); err != nil {
		return nil, err
	}
	return &aesWriter{w: w, ctr: ctr, mac: hmac.New(sha1.New, mac)}, nil
}

func (a *aesWriter) Write(p []byte) (int, error) {
	if cap(a.buf) < len(p) {
		a.buf = make([]byte, len(p))
	}
	b := a.buf[:len(p)]
	a.ctr.xor(b, p)
	a.mac.Write(b)
	return a.w.Write(b)
}

func (a *aesWriter) Close() error {
	_, err := a.w.Write(a.mac.Sum(nil)[:aesAuthLen])
	return err
}

func strengthFor(e archive.Encryption) (byte, error) {
	switch e {
	case archive.EncryptAES128:
		return 1, nil
	case archive.EncryptAES192:
		return 2, nil
	case archive.EncryptAES256, archive.EncryptNone:
		return 3, nil
	}
	return 0, errors.New("ziparc: not an AES encryption")
}
