// On macOS the system thumbnailer is QuickLook, which knows HEIC photos,
// PDFs, videos, Office documents and anything else a QuickLook plug-in
// handles. We drive it through the stock qlmanage tool ("qlmanage -t -s N
// -o DIR FILE"), which writes FILE.png into DIR; this avoids cgo and keeps
// the binary cross-compilable. Each call gets its own temporary directory
// and a time limit, because a misbehaving plug-in must not stall the pool.

package thumbs

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// qlTimeout bounds one qlmanage run.
const qlTimeout = 15 * time.Second

func systemThumbnailer() SystemFunc {
	if _, err := exec.LookPath("qlmanage"); err != nil {
		return nil
	}
	return quickLook
}

func quickLook(ctx context.Context, path string, w, h int) (image.Image, error) {
	ctx, cancel := context.WithTimeout(ctx, qlTimeout)
	defer cancel()
	dir, err := os.MkdirTemp("", "mc-thumb-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	cmd := exec.CommandContext(ctx, "qlmanage", "-t", "-s", strconv.Itoa(max(w, h)), "-o", dir, path)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil && ctx.Err() != context.DeadlineExceeded {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: qlmanage: %v", ErrUnsupported, err)
	}
	f, err := os.Open(filepath.Join(dir, filepath.Base(path)+".png"))
	if err != nil {
		// qlmanage exits 0 even when no generator handled the file.
		return nil, fmt.Errorf("%w: quicklook produced no preview", ErrUnsupported)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%w: quicklook: %v", ErrUnsupported, err)
	}
	return img, nil
}
