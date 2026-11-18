// A small decoder of the Thrift compact protocol, which Parquet uses for its file footer and page headers. It decodes
// any struct into a generic tree (field id -> value) so the Parquet reader picks out only the fields it knows and
// skips the rest, as the protocol intends. Every length is checked against the bytes there are, and nesting is capped,
// so a damaged file gives an error instead of a crash or a huge allocation.

package table

import (
	"encoding/binary"
	"errors"
	"math"
)

var errThrift = errors.New("the file's metadata is damaged")

// tstruct is a decoded Thrift struct: field id -> value (int64, bool, float64, []byte, tstruct or []any).
type tstruct map[int16]any

func (s tstruct) i(id int16) (int64, bool) {
	v, ok := s[id].(int64)
	return v, ok
}

func (s tstruct) int(id int16) int64 { v, _ := s.i(id); return v }

func (s tstruct) bytes(id int16) []byte { v, _ := s[id].([]byte); return v }

func (s tstruct) str(id int16) string { return string(s.bytes(id)) }

func (s tstruct) sub(id int16) tstruct { v, _ := s[id].(tstruct); return v }

func (s tstruct) list(id int16) []any { v, _ := s[id].([]any); return v }

func (s tstruct) flag(id int16, def bool) bool {
	if v, ok := s[id].(bool); ok {
		return v
	}
	return def
}

// compact reads Thrift compact protocol values from b.
type compact struct {
	b     []byte
	p     int
	depth int
}

const thriftMaxDepth = 32

func (c *compact) byte() (byte, error) {
	if c.p >= len(c.b) {
		return 0, errThrift
	}
	c.p++
	return c.b[c.p-1], nil
}

func (c *compact) uvarint() (uint64, error) {
	v, n := binary.Uvarint(c.b[c.p:])
	if n <= 0 {
		return 0, errThrift
	}
	c.p += n
	return v, nil
}

func (c *compact) zigzag() (int64, error) {
	v, err := c.uvarint()
	return int64(v>>1) ^ -int64(v&1), err
}

// readStruct reads a struct up to its stop byte.
func (c *compact) readStruct() (tstruct, error) {
	if c.depth++; c.depth > thriftMaxDepth {
		return nil, errThrift
	}
	defer func() { c.depth-- }()
	s := tstruct{}
	var last int16
	for {
		h, err := c.byte()
		if err != nil {
			return nil, err
		}
		if h == 0 {
			return s, nil
		}
		id := last + int16(h>>4)
		if h>>4 == 0 {
			v, err := c.zigzag()
			if err != nil || v < math.MinInt16 || v > math.MaxInt16 {
				return nil, errThrift
			}
			id = int16(v)
		}
		last = id
		typ := h & 0x0f
		var v any
		switch typ {
		case 1:
			v = true
		case 2:
			v = false
		default:
			if v, err = c.value(typ); err != nil {
				return nil, err
			}
		}
		s[id] = v
	}
}

// value reads one value of compact type typ (booleans inside lists are one byte each).
func (c *compact) value(typ byte) (any, error) {
	switch typ {
	case 1, 2:
		b, err := c.byte()
		return b == 1, err
	case 3:
		b, err := c.byte()
		return int64(int8(b)), err
	case 4, 5, 6:
		return c.zigzag()
	case 7:
		if c.p+8 > len(c.b) {
			return nil, errThrift
		}
		c.p += 8
		return math.Float64frombits(binary.LittleEndian.Uint64(c.b[c.p-8:])), nil
	case 8:
		n, err := c.uvarint()
		if err != nil || n > uint64(len(c.b)-c.p) {
			return nil, errThrift
		}
		c.p += int(n)
		return c.b[c.p-int(n) : c.p], nil
	case 9, 10:
		h, err := c.byte()
		if err != nil {
			return nil, err
		}
		n := uint64(h >> 4)
		if n == 15 {
			if n, err = c.uvarint(); err != nil {
				return nil, err
			}
		}
		if n > uint64(len(c.b)-c.p) { // every element takes at least one byte
			return nil, errThrift
		}
		return c.elements(h&0x0f, int(n))
	case 11:
		n, err := c.uvarint()
		if err != nil || n > uint64(len(c.b)-c.p) {
			return nil, errThrift
		}
		if n == 0 {
			return []any{}, nil
		}
		kv, err := c.byte()
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, 2*n)
		for i := uint64(0); i < n; i++ {
			k, err := c.value(kv >> 4)
			if err != nil {
				return nil, err
			}
			v, err := c.value(kv & 0x0f)
			if err != nil {
				return nil, err
			}
			out = append(out, k, v)
		}
		return out, nil
	case 12:
		return c.readStruct()
	}
	return nil, errThrift
}

func (c *compact) elements(typ byte, n int) ([]any, error) {
	if c.depth++; c.depth > thriftMaxDepth {
		return nil, errThrift
	}
	defer func() { c.depth-- }()
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		v, err := c.value(typ)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// decodeThrift reads one struct from the start of b and says how many bytes it took.
func decodeThrift(b []byte) (tstruct, int, error) {
	c := &compact{b: b}
	s, err := c.readStruct()
	return s, c.p, err
}
