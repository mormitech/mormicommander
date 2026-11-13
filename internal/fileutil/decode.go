// Decoding is TC's cm_UUDecode ("Decode file (MIME, UUE, XXE, BinHex)"). The
// user selects an encoded file, often a saved e-mail or news post, and the
// command finds the encoded data in it and writes the original file(s) back.
// The format is detected from the content, not the extension: a MIME message
// (every attachment is saved, base64 and quoted-printable are handled, also
// nested multiparts), "begin 644 name" blocks in UUE or XXE (told apart line
// by line; several blocks in one file give several files), "begin-base64"
// blocks, BinHex 4.0, and finally a bare base64 text. A file split into
// several parts can be decoded by selecting all parts in order, or by
// selecting name.001.uue so the following parts are found automatically.
// Junk lines between parts (headers, signatures) are skipped. File names come
// from the encoded data, so they are reduced to a plain name inside the
// target folder; a name used twice in one run gets " (2)" added.

package fileutil

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DecodeOptions configures DecodeFiles.
type DecodeOptions struct {
	// Sources are the encoded files, in order. One file named like
	// "name.001.uue" pulls in name.002.uue, name.003.uue, ... by itself.
	Sources []string
	// TargetDir is where decoded files go (default: first source's folder).
	TargetDir string
	Overwrite bool
	Progress  ProgressFunc
}

// DecodedFile describes one file written by DecodeFiles.
type DecodedFile struct {
	Path       string
	StoredName string // the name as found in the encoded data
	Encoding   Encoding
	Size       int64
	// DroppedResourceFork is the size of a BinHex resource fork that could
	// not be kept.
	DroppedResourceFork int
}

// DecodeResult reports what DecodeFiles did.
type DecodeResult struct {
	Files    []DecodedFile
	Sources  []string
	Warnings []string
}

// ErrNothingToDecode is returned when no encoded data was found.
var ErrNothingToDecode = errors.New("no encoded data (MIME, UUE, XXE, BinHex or base64) found")

var (
	reBegin       = regexp.MustCompile(`^begin(-base64)?\s+([0-7]{3,4})\s+(.+)$`)
	reHeaderLine  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*:`)
	reEncodedPart = regexp.MustCompile(`^(.*)\.(\d{3,})(\.[^.]+)$`)
)

// FindEncodedParts returns the consecutive split parts starting at a path
// like "name.001.uue" (name.002.uue, ...). Any other path is returned alone.
func FindEncodedParts(path string) []string {
	m := reEncodedPart.FindStringSubmatch(path)
	if m == nil {
		return []string{path}
	}
	n, _ := strconv.Atoi(m[2])
	width := len(m[2])
	var parts []string
	for ; ; n++ {
		p := fmt.Sprintf("%s.%0*d%s", m[1], width, n, m[3])
		if !exists(p) {
			break
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return []string{path}
	}
	return parts
}

// decoder carries the state of one DecodeFiles run.
type decoder struct {
	t       *tracker
	opts    DecodeOptions
	dir     string
	res     *DecodeResult
	used    map[string]bool
	created []string
	srcAbs  map[string]bool
}

// countingReader reports progress and checks for cancellation as data is
// read from the sources.
type countingReader struct {
	r io.Reader
	t *tracker
}

func (c *countingReader) Read(p []byte) (int, error) {
	if err := c.t.check(); err != nil {
		return 0, err
	}
	n, err := c.r.Read(p)
	c.t.done += int64(n)
	c.t.report("", 0, 0)
	return n, err
}

func (d *decoder) open() (*bufio.Reader, func(), error) {
	var rs []io.Reader
	var fs []*os.File
	closeAll := func() {
		for _, f := range fs {
			f.Close()
		}
	}
	for _, s := range d.res.Sources {
		f, err := os.Open(s)
		if err != nil {
			closeAll()
			return nil, nil, err
		}
		fs = append(fs, f)
		rs = append(rs, f)
	}
	d.t.done = 0
	return bufio.NewReaderSize(&countingReader{r: io.MultiReader(rs...), t: d.t}, 256<<10), closeAll, nil
}

// DecodeFiles decodes the given encoded file(s).
func DecodeFiles(ctx context.Context, opts DecodeOptions) (*DecodeResult, error) {
	if len(opts.Sources) == 0 {
		return nil, errors.New("no file selected")
	}
	sources := opts.Sources
	if len(sources) == 1 {
		sources = FindEncodedParts(sources[0])
	}
	var total int64
	srcAbs := map[string]bool{}
	for _, s := range sources {
		n, err := fileSize(s)
		if err != nil {
			return nil, err
		}
		total += n
		a, _ := filepath.Abs(s)
		srcAbs[a] = true
	}
	dir := opts.TargetDir
	if dir == "" {
		dir = filepath.Dir(sources[0])
	}
	d := &decoder{
		t:      newTracker(ctx, opts.Progress, total),
		opts:   opts,
		dir:    dir,
		res:    &DecodeResult{Sources: sources},
		used:   map[string]bool{},
		srcAbs: srcAbs,
	}
	err := d.run()
	if err != nil {
		removeAll(d.created)
		return nil, err
	}
	return d.res, nil
}

func (d *decoder) run() error {
	r, closeAll, err := d.open()
	if err != nil {
		return err
	}
	head, _ := r.Peek(64 << 10)
	if err := d.t.check(); err != nil {
		closeAll()
		return err
	}
	kind := detectEncoding(head)
	switch kind {
	case EncodingBinHex:
		err = d.decodeBinHex(r)
	case EncodingMIME:
		err = d.decodeMIME(r)
		if err == nil && len(d.res.Files) == 0 {
			// A mail without attachments may still carry uuencoded text.
			closeAll()
			if r, closeAll, err = d.open(); err != nil {
				return err
			}
			err = d.decodeUU(r, "")
		}
	case EncodingUUE:
		err = d.decodeUU(r, "")
	case EncodingBase64:
		err = d.decodePlainBase64(r)
	default:
		err = ErrNothingToDecode
	}
	closeAll()
	if err == nil && len(d.res.Files) == 0 {
		err = ErrNothingToDecode
	}
	return err
}

// detectEncoding guesses the format from the start of the data.
func detectEncoding(head []byte) Encoding {
	if bytes.Contains(head, []byte(hqxHeader)) {
		return EncodingBinHex
	}
	lines := strings.Split(strings.ReplaceAll(string(head), "\r\n", "\n"), "\n")
	// MIME: the data starts with header lines and one of them is about
	// content or MIME.
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) && reHeaderLine.MatchString(lines[i]) {
		for ; i < len(lines) && lines[i] != ""; i++ {
			l := strings.ToLower(lines[i])
			if strings.HasPrefix(l, "content-") || strings.HasPrefix(l, "mime-version:") {
				return EncodingMIME
			}
			if !reHeaderLine.MatchString(lines[i]) && lines[i][0] != ' ' && lines[i][0] != '\t' {
				break
			}
		}
	}
	for _, l := range lines {
		if reBegin.MatchString(strings.TrimRight(l, " \t\r")) {
			return EncodingUUE
		}
	}
	// Bare base64: nothing but alphabet characters, one block per line
	// (spaces inside a line mean ordinary text such as "hello world").
	text := strings.TrimSpace(string(head))
	if text == "" {
		return 0
	}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if strings.Trim(l, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=") != "" {
			return 0
		}
	}
	return EncodingBase64
}

// create opens an output file for a stored name.
func (d *decoder) create(stored string, enc Encoding) (*os.File, *DecodedFile, error) {
	name, err := safeFileName(stored)
	if err != nil {
		return nil, nil, err
	}
	name = d.unique(name)
	p := filepath.Join(d.dir, name)
	if a, _ := filepath.Abs(p); d.srcAbs[a] {
		name = d.unique(name + ".decoded")
		p = filepath.Join(d.dir, name)
	}
	f, err := createOutput(p, d.opts.Overwrite)
	if err != nil {
		return nil, nil, err
	}
	d.created = append(d.created, p)
	d.res.Files = append(d.res.Files, DecodedFile{Path: p, StoredName: stored, Encoding: enc})
	return f, &d.res.Files[len(d.res.Files)-1], nil
}

func (d *decoder) unique(name string) string {
	key := strings.ToLower(name)
	if !d.used[key] {
		d.used[key] = true
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if !d.used[strings.ToLower(n)] {
			d.used[strings.ToLower(n)] = true
			return n
		}
	}
}

// last returns the entry of the file being written. It is looked up fresh
// each time because the slice may have grown since the file was created.
func (d *decoder) last() *DecodedFile { return &d.res.Files[len(d.res.Files)-1] }

// decodeUU scans for begin/end blocks. defaultName is used for a
// "begin-base64" block without a name (never happens in practice).
func (d *decoder) decodeUU(r io.Reader, defaultName string) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var (
		out       *bufio.Writer
		f         *os.File
		inBlock   bool
		b64       bool
		codec     *lineCodec
		pending   []string
		b64rest   string
		ended     bool
		blockName string
		size      int64
		junk      int
		buf       []byte
	)
	closeBlock := func(complete bool) error {
		if f == nil {
			return nil
		}
		err := out.Flush()
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		d.last().Size = size
		if !complete {
			d.res.Warnings = append(d.res.Warnings, fmt.Sprintf("%s: the encoded data ends without an \"end\" line, so the file may be incomplete (is a part missing?)", blockName))
		}
		if junk > 0 {
			d.res.Warnings = append(d.res.Warnings, fmt.Sprintf("%s: skipped %d line%s that were not encoded data", blockName, junk, plural(junk)))
		}
		f, out, inBlock = nil, nil, false
		return err
	}
	write := func(p []byte) error {
		size += int64(len(p))
		_, err := out.Write(p)
		return err
	}
	flushPending := func(c *lineCodec) error {
		for _, l := range pending {
			var n int
			var ok bool
			buf, n, ok = c.decodeLine(buf[:0], l)
			if !ok {
				junk++
				continue
			}
			if n == 0 {
				ended = true
				continue
			}
			if err := write(buf); err != nil {
				return err
			}
		}
		pending = nil
		return nil
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if !inBlock {
			m := reBegin.FindStringSubmatch(strings.TrimRight(line, " \t"))
			if m == nil {
				continue
			}
			b64 = m[1] != ""
			blockName = m[3]
			if blockName == "" {
				blockName = defaultName
			}
			enc := EncodingUUE
			if b64 {
				enc = EncodingBase64
			}
			var err error
			if f, _, err = d.create(blockName, enc); err != nil {
				return err
			}
			out = bufio.NewWriter(f)
			inBlock, codec, pending, b64rest, ended, size, junk = true, nil, nil, "", false, 0, 0
			continue
		}
		trimmed := strings.TrimSpace(line)
		if b64 {
			if trimmed == "====" {
				if err := closeBlock(true); err != nil {
					return err
				}
				continue
			}
			b64rest += strings.Join(strings.Fields(trimmed), "")
			k := len(b64rest) / 4 * 4
			if k > 0 {
				dec, err := base64.StdEncoding.DecodeString(b64rest[:k])
				if err != nil {
					junk++
					b64rest = ""
					continue
				}
				if err := write(dec); err != nil {
					return err
				}
				b64rest = b64rest[k:]
			}
			continue
		}
		if trimmed == "end" {
			if codec == nil {
				codec = uuCodec
			}
			if err := flushPending(codec); err != nil {
				return err
			}
			if codec == xxCodec {
				d.last().Encoding = EncodingXXE
			}
			if err := closeBlock(true); err != nil {
				return err
			}
			continue
		}
		if ended {
			// Lines after the zero-length line but before "end" are ignored.
			continue
		}
		if codec == nil {
			uuOK, xxOK := uuCodec.validFor(line), xxCodec.validFor(line)
			switch {
			case uuOK && !xxOK:
				codec = uuCodec
			case xxOK && !uuOK:
				codec = xxCodec
			case uuOK && xxOK:
				pending = append(pending, line)
				continue
			default:
				junk++
				continue
			}
			if err := flushPending(codec); err != nil {
				return err
			}
		}
		var n int
		var ok bool
		buf, n, ok = codec.decodeLine(buf[:0], line)
		if !ok {
			junk++
			continue
		}
		if n == 0 {
			ended = true
			continue
		}
		if err := write(buf); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		if f != nil {
			f.Close()
		}
		return err
	}
	if inBlock {
		if codec == nil {
			codec = uuCodec
		}
		if !b64 {
			if err := flushPending(codec); err != nil {
				return err
			}
			if codec == xxCodec {
				d.last().Encoding = EncodingXXE
			}
		}
		return closeBlock(false)
	}
	return nil
}

func (d *decoder) decodeBinHex(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	bh, err := decodeBinHex(string(data))
	if err != nil {
		return err
	}
	f, df, err := d.create(bh.Name, EncodingBinHex)
	if err != nil {
		return err
	}
	_, err = f.Write(bh.Data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	df.Size = int64(len(bh.Data))
	df.DroppedResourceFork = bh.RsrcSize
	if bh.RsrcSize > 0 {
		d.res.Warnings = append(d.res.Warnings, fmt.Sprintf("%s: the Macintosh resource fork (%d bytes) was not kept", bh.Name, bh.RsrcSize))
	}
	return err
}

func (d *decoder) decodePlainBase64(r io.Reader) error {
	name := stem(d.res.Sources[0])
	f, df, err := d.create(name, EncodingBase64)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, base64.NewDecoder(base64.StdEncoding, &spaceFilter{r: r}))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	df.Size = n
	if err != nil {
		return fmt.Errorf("the base64 data is damaged: %w", err)
	}
	return nil
}

// spaceFilter drops spaces and tabs, which the base64 decoder would reject
// (it already ignores line breaks).
type spaceFilter struct{ r io.Reader }

func (s *spaceFilter) Read(p []byte) (int, error) {
	for {
		n, err := s.r.Read(p)
		k := 0
		for _, c := range p[:n] {
			if c != ' ' && c != '\t' {
				p[k] = c
				k++
			}
		}
		if k > 0 || err != nil {
			return k, err
		}
	}
}

func (d *decoder) decodeMIME(r io.Reader) error {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return fmt.Errorf("the MIME headers cannot be read: %w", err)
	}
	return d.mimeEntity(textprotoHeader(msg.Header), msg.Body, 0)
}

// header is the small subset of a MIME header the walker needs; both
// mail.Header and multipart.Part's textproto header provide Get.
type header interface{ Get(string) string }

type textprotoHeader mail.Header

func (h textprotoHeader) Get(k string) string { return mail.Header(h).Get(k) }

var wordDec = new(mime.WordDecoder)

func (d *decoder) mimeEntity(h header, body io.Reader, depth int) error {
	if depth > 20 {
		return errors.New("the MIME message is nested too deeply")
	}
	ctype := h.Get("Content-Type")
	mt, params, err := mime.ParseMediaType(ctype)
	if err != nil {
		mt, params = "text/plain", map[string]string{}
	}
	if strings.HasPrefix(mt, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return errors.New("a multipart MIME section has no boundary")
		}
		mr := multipart.NewReader(body, boundary)
		for {
			p, err := mr.NextRawPart()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("the MIME message is damaged: %w", err)
			}
			if err := d.mimeEntity(p.Header, p, depth+1); err != nil {
				return err
			}
		}
	}
	name := ""
	if _, dp, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		name = dp["filename"]
	}
	if name == "" {
		name = params["name"]
	}
	if dn, err := wordDec.DecodeHeader(name); err == nil {
		name = dn
	}
	cte := strings.ToLower(strings.TrimSpace(h.Get("Content-Transfer-Encoding")))
	if name == "" {
		// Message text without a file name is not an attachment; the only
		// exception is a single-part message that is itself the encoded
		// file, as TC writes it.
		if depth > 0 || strings.HasPrefix(mt, "text/") || mt == "message/rfc822" {
			return nil
		}
		name = stem(d.res.Sources[0])
	}
	var src io.Reader
	switch cte {
	case "base64":
		src = base64.NewDecoder(base64.StdEncoding, &spaceFilter{r: body})
	case "quoted-printable":
		src = quotedprintable.NewReader(body)
	case "x-uuencode", "uuencode", "x-uue":
		return d.decodeUU(body, name)
	default:
		src = body
	}
	f, df, err := d.create(name, EncodingMIME)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, src)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	df.Size = n
	if err != nil {
		return fmt.Errorf("%s: the encoded data is damaged: %w", name, err)
	}
	return nil
}
