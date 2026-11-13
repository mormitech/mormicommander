// Encoding is TC's cm_UUEncode ("Encode file (MIME, UUE, XXE format)"): it
// turns any file into plain text that can be pasted into an e-mail or a
// forum, either as a MIME message with a base64 attachment, as UUE or as XXE.
// UUE and XXE output can be cut into several files of a given number of
// lines, as TC offers for size-limited mail systems. BinHex can be written
// too, mainly for completeness and round-trip tests.
//
// Encoded files are written next to the source by default with the
// extensions .b64 (MIME), .uue, .xxe and .hqx; split UUE/XXE output is named
// name.001.uue, name.002.uue and so on.

package fileutil

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Encoding is a text encoding for binary files.
type Encoding int

const (
	// EncodingMIME is a MIME message with one base64 attachment.
	EncodingMIME Encoding = iota + 1
	// EncodingUUE is classic uuencode.
	EncodingUUE
	// EncodingXXE is xxencode.
	EncodingXXE
	// EncodingBinHex is BinHex 4.0.
	EncodingBinHex
	// EncodingBase64 is a bare base64 text without MIME headers; it is only
	// produced by the decoder's detection ("begin-base64" blocks and plain
	// base64 files).
	EncodingBase64
)

func (e Encoding) String() string {
	switch e {
	case EncodingMIME:
		return "MIME"
	case EncodingUUE:
		return "UUE"
	case EncodingXXE:
		return "XXE"
	case EncodingBinHex:
		return "BinHex"
	case EncodingBase64:
		return "Base64"
	}
	return fmt.Sprintf("Encoding(%d)", int(e))
}

// Extension returns the default extension for encoded output.
func (e Encoding) Extension() string {
	switch e {
	case EncodingMIME, EncodingBase64:
		return ".b64"
	case EncodingUUE:
		return ".uue"
	case EncodingXXE:
		return ".xxe"
	case EncodingBinHex:
		return ".hqx"
	}
	return ".txt"
}

// EncodeOptions configures EncodeFile.
type EncodeOptions struct {
	Source   string
	Encoding Encoding
	// Target is the output path (default: source + extension, in TargetDir
	// or the source's folder). For split output the part number is inserted
	// before the extension.
	Target    string
	TargetDir string
	// LinesPerPart splits UUE/XXE output into files of at most this many
	// encoded lines (0 = one file). The "begin" and "end" lines do not count.
	LinesPerPart int
	Format       LineFormat
	Overwrite    bool
	Progress     ProgressFunc
}

// EncodeResult lists the files EncodeFile wrote.
type EncodeResult struct {
	Files []string
}

// fileMode returns the permission digits for a "begin" line: the real ones
// on Unix, 644 on Windows where they do not exist.
func fileMode(path string) string {
	if runtime.GOOS == "windows" {
		return "644"
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "644"
	}
	return fmt.Sprintf("%o", fi.Mode().Perm())
}

// EncodeFile encodes a file.
func EncodeFile(ctx context.Context, opts EncodeOptions) (*EncodeResult, error) {
	size, err := fileSize(opts.Source)
	if err != nil {
		return nil, err
	}
	if opts.LinesPerPart < 0 {
		return nil, errors.New("lines per part cannot be negative")
	}
	if opts.LinesPerPart > 0 && opts.Encoding != EncodingUUE && opts.Encoding != EncodingXXE {
		return nil, errors.New("only UUE and XXE output can be split into parts")
	}
	target := opts.Target
	if target == "" {
		dir := opts.TargetDir
		if dir == "" {
			dir = filepath.Dir(opts.Source)
		}
		target = filepath.Join(dir, filepath.Base(opts.Source)+opts.Encoding.Extension())
	}
	if a, _ := filepath.Abs(target); a != "" {
		if s, _ := filepath.Abs(opts.Source); s == a {
			return nil, errors.New("the encoded file would overwrite the source")
		}
	}
	nl := newlineFor(opts.Format.unix())
	name := filepath.Base(opts.Source)

	in, err := os.Open(opts.Source)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	t := newTracker(ctx, opts.Progress, size)
	res := &EncodeResult{}
	fail := func(err error) (*EncodeResult, error) {
		removeAll(res.Files)
		return nil, err
	}

	switch opts.Encoding {
	case EncodingMIME, EncodingBase64:
		out, err := createOutput(target, opts.Overwrite)
		if err != nil {
			return nil, err
		}
		res.Files = append(res.Files, target)
		bw := bufio.NewWriter(out)
		if opts.Encoding == EncodingMIME {
			ct := mime.FormatMediaType("application/octet-stream", map[string]string{"name": name})
			cd := mime.FormatMediaType("attachment", map[string]string{"filename": name})
			fmt.Fprintf(bw, "MIME-Version: 1.0%sContent-Type: %s%sContent-Transfer-Encoding: base64%sContent-Disposition: %s%s%s",
				nl, ct, nl, nl, cd, nl, nl)
		}
		lw := &lineWrapper{w: bw, width: 76, nl: nl}
		enc := base64.NewEncoder(base64.StdEncoding, lw)
		_, err = t.copyN(enc, in, -1, opts.Source, 0, size)
		if err == nil {
			err = enc.Close()
		}
		if err == nil {
			err = lw.finish()
		}
		if err == nil {
			err = bw.Flush()
		}
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fail(err)
		}
	case EncodingBinHex:
		data, err := io.ReadAll(io.LimitReader(in, size+1))
		if err != nil {
			return nil, err
		}
		if err := t.check(); err != nil {
			return nil, err
		}
		out, err := createOutput(target, opts.Overwrite)
		if err != nil {
			return nil, err
		}
		res.Files = append(res.Files, target)
		err = encodeBinHex(out, name, data, nl)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fail(err)
		}
		t.done = size
		t.report(opts.Source, size, size)
	case EncodingUUE, EncodingXXE:
		codec := uuCodec
		if opts.Encoding == EncodingXXE {
			codec = xxCodec
		}
		if err := t.encodeUU(in, codec, target, name, fileMode(opts.Source), opts, size, nl, res); err != nil {
			return fail(err)
		}
	default:
		return nil, fmt.Errorf("unknown encoding %v", opts.Encoding)
	}
	return res, nil
}

// encodedPartName inserts a part number before the extension:
// "x.uue" -> "x.001.uue".
func encodedPartName(target string, n int) string {
	ext := filepath.Ext(target)
	return fmt.Sprintf("%s.%03d%s", strings.TrimSuffix(target, ext), n, ext)
}

func (t *tracker) encodeUU(in io.Reader, codec *lineCodec, target, name, mode string, opts EncodeOptions, size int64, nl string, res *EncodeResult) error {
	var out *os.File
	var bw *bufio.Writer
	part, lines := 0, 0
	closeOut := func() error {
		if out == nil {
			return nil
		}
		err := bw.Flush()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		out = nil
		return err
	}
	open := func() error {
		if err := closeOut(); err != nil {
			return err
		}
		part++
		p := target
		if opts.LinesPerPart > 0 {
			p = encodedPartName(target, part)
		}
		f, err := createOutput(p, opts.Overwrite)
		if err != nil {
			return err
		}
		res.Files = append(res.Files, p)
		out, bw, lines = f, bufio.NewWriter(f), 0
		return nil
	}
	if err := open(); err != nil {
		return err
	}
	fmt.Fprintf(bw, "begin %s %s%s", mode, name, nl)
	buf := make([]byte, uuLineBytes*1024)
	line := make([]byte, 0, 64)
	var done int64
	for {
		if err := t.check(); err != nil {
			closeOut()
			return err
		}
		n, rerr := io.ReadFull(in, buf)
		for i := 0; i < n; i += uuLineBytes {
			if opts.LinesPerPart > 0 && lines >= opts.LinesPerPart {
				if err := open(); err != nil {
					return err
				}
			}
			end := min(i+uuLineBytes, n)
			line = codec.encodeLine(line[:0], buf[i:end])
			bw.Write(line)
			bw.WriteString(nl)
			lines++
		}
		done += int64(n)
		t.done += int64(n)
		t.report(opts.Source, done, size)
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			break
		}
		if rerr != nil {
			closeOut()
			return rerr
		}
	}
	fmt.Fprintf(bw, "%s%send%s", codec.terminator(), nl, nl)
	return closeOut()
}

// lineWrapper inserts a line break every width bytes.
type lineWrapper struct {
	w     io.Writer
	width int
	col   int
	nl    string
}

func (l *lineWrapper) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		k := min(l.width-l.col, len(p))
		if _, err := l.w.Write(p[:k]); err != nil {
			return 0, err
		}
		l.col += k
		p = p[k:]
		if l.col == l.width {
			if _, err := io.WriteString(l.w, l.nl); err != nil {
				return 0, err
			}
			l.col = 0
		}
	}
	return n, nil
}

func (l *lineWrapper) finish() error {
	if l.col > 0 {
		_, err := io.WriteString(l.w, l.nl)
		l.col = 0
		return err
	}
	return nil
}
