// This file implements "Find duplicate files" from the Advanced tab (F-534). Total
// Commander can group files that have the same name, the same size, the same content,
// or any combination; the result list then shows the groups one after the other. The
// search first collects every file that passes the normal filters, then narrows the
// candidates step by step, cheapest test first: equal name and size are free, a hash
// of the first 64 KiB weeds out most same-sized strangers, and only then is the whole
// file hashed. "Same content" finally compares the bytes of each file with the first
// file of its group, so even a hash collision cannot produce a false duplicate; with a
// named hash algorithm and Content off, the digest alone decides (faster, and the
// digest is shown in the results like TC's "compare by hash" option).

package find

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"sort"
	"strconv"
	"strings"
)

// DupOptions configures the duplicate search. If none of Name, Size, Content and Hash
// is set, Size+Content is used, the safest meaning of "duplicate".
type DupOptions struct {
	Enabled bool   `json:"enabled,omitempty"`
	Name    bool   `json:"name,omitempty"`    // same file name (ignoring case)
	Size    bool   `json:"size,omitempty"`    // same size
	Content bool   `json:"content,omitempty"` // same bytes, verified byte by byte
	Hash    string `json:"hash,omitempty"`    // "md5", "sha1", "sha256" or "crc32": group by this digest
}

// prefixLen is how much of each file the cheap first-pass hash reads.
const prefixLen = 64 << 10

type dupConfig struct {
	DupOptions
	newHash func() hash.Hash
}

func compileDup(d DupOptions) (*dupConfig, error) {
	c := &dupConfig{DupOptions: d}
	if !d.Name && !d.Size && !d.Content && d.Hash == "" {
		c.Size, c.Content = true, true
	}
	switch strings.ToLower(d.Hash) {
	case "":
		c.newHash = sha256.New
	case "md5":
		c.newHash = md5.New
	case "sha1":
		c.newHash = sha1.New
	case "sha256":
		c.newHash = sha256.New
	case "crc32":
		c.newHash = func() hash.Hash { return crc32.NewIEEE() }
	default:
		return nil, fmt.Errorf("find: unknown hash algorithm %q", d.Hash)
	}
	return c, nil
}

func (c *dupConfig) needContent() bool { return c.Content || c.Hash != "" }

// emitDuplicates groups the collected files and sends them out, group by group.
func (r *run) emitDuplicates() error {
	c := r.q.dup
	byKey := map[string][]*entry{}
	var keys []string
	for _, e := range r.dupFound {
		if c.needContent() && e.archive != "" {
			// Archive members can only be opened while their archive is being walked,
			// so they take part in name and size comparisons only.
			continue
		}
		var k strings.Builder
		if c.Name {
			k.WriteString(strings.ToLower(e.name))
		}
		k.WriteByte(0)
		if c.Size || c.needContent() {
			k.WriteString(strconv.FormatInt(e.size, 10))
		}
		key := k.String()
		if _, ok := byKey[key]; !ok {
			keys = append(keys, key)
		}
		byKey[key] = append(byKey[key], e)
	}
	var groups [][]*entry
	hashes := map[*entry]string{}
	for _, k := range keys {
		g := byKey[k]
		if len(g) < 2 {
			continue
		}
		if !c.needContent() {
			groups = append(groups, g)
			continue
		}
		sub, err := r.splitByContent(g, hashes)
		if err != nil {
			return err
		}
		groups = append(groups, sub...)
	}
	for _, g := range groups {
		sort.Slice(g, func(i, j int) bool { return g[i].path < g[j].path })
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i][0].path < groups[j][0].path })
	for i, g := range groups {
		for _, e := range g {
			res := e.result()
			res.DupGroup = i + 1
			if c.Hash != "" {
				res.Hash = hashes[e]
			}
			r.prog.Matches++
			if err := r.h.Result(res); err != nil {
				return err
			}
		}
	}
	return nil
}

// splitByContent splits one same-size group into groups of identical content.
func (r *run) splitByContent(g []*entry, hashes map[*entry]string) ([][]*entry, error) {
	c := r.q.dup
	// Pass 1: hash of the first 64 KiB (only worth it for bigger files).
	buckets := [][]*entry{g}
	if g[0].size > prefixLen {
		var err error
		if buckets, err = r.bucket(g, func(e *entry) (string, error) { return r.digest(e, prefixLen, sha256.New) }); err != nil {
			return nil, err
		}
	}
	// Pass 2: full digest.
	var out [][]*entry
	for _, b := range buckets {
		if len(b) < 2 {
			continue
		}
		full, err := r.bucket(b, func(e *entry) (string, error) {
			d, err := r.digest(e, -1, c.newHash)
			if err == nil {
				hashes[e] = d
			}
			return d, err
		})
		if err != nil {
			return nil, err
		}
		for _, f := range full {
			if len(f) < 2 {
				continue
			}
			if !c.Content {
				out = append(out, f)
				continue
			}
			// Pass 3: byte-by-byte against the first member, regrouping mismatches.
			for len(f) >= 2 {
				ref := f[0]
				same := []*entry{ref}
				var rest []*entry
				for _, e := range f[1:] {
					eq, err := r.sameBytes(ref, e)
					if err != nil {
						if r.ctx.Err() != nil {
							return nil, r.ctx.Err()
						}
						r.fail(e.path, err)
						continue
					}
					if eq {
						same = append(same, e)
					} else {
						rest = append(rest, e)
					}
				}
				if len(same) >= 2 {
					out = append(out, same)
				}
				f = rest
			}
		}
	}
	return out, nil
}

// bucket groups entries by a key function, dropping entries whose key failed.
func (r *run) bucket(g []*entry, key func(*entry) (string, error)) ([][]*entry, error) {
	m := map[string][]*entry{}
	var order []string
	for _, e := range g {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		k, err := key(e)
		if err != nil {
			if r.ctx.Err() != nil {
				return nil, r.ctx.Err()
			}
			r.fail(e.path, err)
			continue
		}
		if _, ok := m[k]; !ok {
			order = append(order, k)
		}
		m[k] = append(m[k], e)
	}
	out := make([][]*entry, 0, len(order))
	for _, k := range order {
		out = append(out, m[k])
	}
	return out, nil
}

// ctxReader makes long reads stop when the search is cancelled.
type ctxReader struct {
	r   *run
	src io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.r.ctx.Err(); err != nil {
		return 0, err
	}
	return c.src.Read(p)
}

// digest hashes the first limit bytes of a file (all of it if limit < 0).
func (r *run) digest(e *entry, limit int64, newHash func() hash.Hash) (string, error) {
	rc, err := e.open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var src io.Reader = ctxReader{r, rc}
	if limit >= 0 {
		src = io.LimitReader(src, limit)
	}
	h := newHash()
	if _, err := io.Copy(h, src); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sameBytes compares two files byte by byte.
func (r *run) sameBytes(a, b *entry) (bool, error) {
	ra, err := a.open()
	if err != nil {
		return false, err
	}
	defer ra.Close()
	rb, err := b.open()
	if err != nil {
		return false, err
	}
	defer rb.Close()
	ba := make([]byte, 256<<10)
	bb := make([]byte, 256<<10)
	for {
		if err := r.ctx.Err(); err != nil {
			return false, err
		}
		na, ea := io.ReadFull(ra, ba)
		nb, eb := io.ReadFull(rb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		aEnd := ea == io.EOF || ea == io.ErrUnexpectedEOF
		bEnd := eb == io.EOF || eb == io.ErrUnexpectedEOF
		if ea != nil && !aEnd {
			return false, ea
		}
		if eb != nil && !bEnd {
			return false, eb
		}
		if aEnd || bEnd {
			return aEnd == bEnd, nil
		}
	}
}
