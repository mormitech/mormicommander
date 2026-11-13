// BinHex 4.0 (.hqx) is the classic Macintosh text encoding. Total Commander's
// Decode command understands it because old Mac downloads still turn up in
// archives. A BinHex file stores a small header (name, type, creator), the
// data fork and the resource fork, each with a CRC, run-length compressed and
// written with a 64-character alphabet between two colons. Only the data
// fork is meaningful on today's file systems, so decoding writes the data
// fork and reports the size of any resource fork it had to drop. Encoding is
// provided too, so tests can round-trip and the feature is complete.

package fileutil

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	hqxTable  = "!\"#$%&'()*+,-012345689@ABCDEFGHIJKLMNPQRSTUVXYZ[`abcdefhijklmpqr"
	hqxHeader = "(This file must be converted with BinHex 4.0)"
	hqxRLE    = 0x90
)

var hqxRev = func() [256]int8 {
	var r [256]int8
	for i := range r {
		r[i] = -1
	}
	for i := 0; i < len(hqxTable); i++ {
		r[hqxTable[i]] = int8(i)
	}
	return r
}()

// crc16XModem is the CRC BinHex uses (CCITT polynomial, initial value 0).
func crc16XModem(crc uint16, p []byte) uint16 {
	for _, b := range p {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// binHexFile is a decoded BinHex file.
type binHexFile struct {
	Name     string
	Type     [4]byte
	Creator  [4]byte
	Flags    uint16
	Data     []byte
	RsrcSize int
}

// encodeBinHex writes data as a BinHex 4.0 text with the given file name.
func encodeBinHex(w io.Writer, name string, data []byte, nl string) error {
	if len(name) > 63 {
		name = name[:63]
	}
	if name == "" {
		name = "file"
	}
	var raw bytes.Buffer
	raw.WriteByte(byte(len(name)))
	raw.WriteString(name)
	raw.WriteByte(0)
	raw.WriteString("????????") // unknown type and creator
	raw.Write([]byte{0, 0})
	binary.Write(&raw, binary.BigEndian, uint32(len(data)))
	binary.Write(&raw, binary.BigEndian, uint32(0))
	binary.Write(&raw, binary.BigEndian, crc16XModem(0, raw.Bytes()))
	raw.Write(data)
	binary.Write(&raw, binary.BigEndian, crc16XModem(0, data))
	binary.Write(&raw, binary.BigEndian, uint16(0)) // empty resource fork CRC

	rle := hqxCompress(raw.Bytes())
	var b strings.Builder
	b.WriteString(hqxHeader + nl + ":")
	col := 1
	emit := func(c byte) {
		if col == 64 {
			b.WriteString(nl)
			col = 0
		}
		b.WriteByte(c)
		col++
	}
	for i := 0; i < len(rle); i += 3 {
		var g [3]byte
		n := copy(g[:], rle[i:])
		v := []byte{g[0] >> 2, (g[0]&3)<<4 | g[1]>>4, (g[1]&15)<<2 | g[2]>>6, g[2] & 63}
		chars := (n*8 + 5) / 6
		for _, x := range v[:chars] {
			emit(hqxTable[x])
		}
	}
	emit(':')
	b.WriteString(nl)
	_, err := io.WriteString(w, b.String())
	return err
}

func hqxCompress(p []byte) []byte {
	var out []byte
	for i := 0; i < len(p); {
		c := p[i]
		r := 1
		for i+r < len(p) && p[i+r] == c && r < 255 {
			r++
		}
		if c == hqxRLE {
			out = append(out, hqxRLE, 0)
			if r > 1 {
				out = append(out, hqxRLE, byte(r))
			}
		} else if r >= 3 {
			out = append(out, c, hqxRLE, byte(r))
		} else {
			for k := 0; k < r; k++ {
				out = append(out, c)
			}
		}
		i += r
	}
	return out
}

func hqxDecompress(p []byte) ([]byte, error) {
	out := make([]byte, 0, len(p))
	var prev byte
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c != hqxRLE {
			out = append(out, c)
			prev = c
			continue
		}
		if i+1 >= len(p) {
			// A trailing marker can be an artefact of the final padding.
			break
		}
		i++
		n := p[i]
		if n == 0 {
			out = append(out, hqxRLE)
			prev = hqxRLE
			continue
		}
		for k := 1; k < int(n); k++ {
			out = append(out, prev)
		}
	}
	return out, nil
}

// decodeBinHex parses BinHex text (the header line may be preceded by
// anything, e.g. mail headers).
func decodeBinHex(text string) (*binHexFile, error) {
	i := strings.Index(text, hqxHeader)
	if i < 0 {
		return nil, errors.New("no BinHex 4.0 header found")
	}
	rest := text[i+len(hqxHeader):]
	start := strings.IndexByte(rest, ':')
	if start < 0 {
		return nil, errors.New("BinHex data does not start with ':'")
	}
	rest = rest[start+1:]
	end := strings.IndexByte(rest, ':')
	if end < 0 {
		return nil, errors.New("BinHex data is not terminated with ':'")
	}
	var six []byte
	for j := 0; j < end; j++ {
		c := rest[j]
		if c == '\r' || c == '\n' || c == ' ' || c == '\t' {
			continue
		}
		if hqxRev[c] < 0 {
			return nil, fmt.Errorf("invalid BinHex character %q", c)
		}
		six = append(six, byte(hqxRev[c]))
	}
	raw := make([]byte, 0, len(six)*3/4+3)
	for j := 0; j+1 < len(six); j += 4 {
		var g [4]byte
		n := copy(g[:], six[j:])
		bs := []byte{g[0]<<2 | g[1]>>4, g[1]<<4 | g[2]>>2, g[2]<<6 | g[3]}
		raw = append(raw, bs[:n*6/8]...)
	}
	data, err := hqxDecompress(raw)
	if err != nil {
		return nil, err
	}
	if len(data) < 1 {
		return nil, errors.New("BinHex data is empty")
	}
	nlen := int(data[0])
	hlen := 1 + nlen + 1 + 4 + 4 + 2 + 4 + 4
	if len(data) < hlen+2 {
		return nil, errors.New("BinHex header is truncated")
	}
	hdr := data[:hlen]
	if got, want := crc16XModem(0, hdr), binary.BigEndian.Uint16(data[hlen:]); got != want {
		return nil, fmt.Errorf("BinHex header CRC error (%04X, expected %04X)", got, want)
	}
	f := &binHexFile{Name: string(hdr[1 : 1+nlen])}
	p := 1 + nlen + 1
	copy(f.Type[:], hdr[p:])
	copy(f.Creator[:], hdr[p+4:])
	f.Flags = binary.BigEndian.Uint16(hdr[p+8:])
	dlen := int(binary.BigEndian.Uint32(hdr[p+10:]))
	rlen := int(binary.BigEndian.Uint32(hdr[p+14:]))
	body := data[hlen+2:]
	if dlen < 0 || rlen < 0 || len(body) < dlen+2+rlen+2 {
		return nil, errors.New("BinHex data is truncated")
	}
	f.Data = body[:dlen]
	if got, want := crc16XModem(0, f.Data), binary.BigEndian.Uint16(body[dlen:]); got != want {
		return nil, fmt.Errorf("BinHex data CRC error (%04X, expected %04X)", got, want)
	}
	rsrc := body[dlen+2 : dlen+2+rlen]
	if got, want := crc16XModem(0, rsrc), binary.BigEndian.Uint16(body[dlen+2+rlen:]); got != want {
		return nil, fmt.Errorf("BinHex resource fork CRC error (%04X, expected %04X)", got, want)
	}
	f.RsrcSize = rlen
	return f, nil
}
