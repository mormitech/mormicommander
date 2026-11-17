package main

// Streaming sound and films in the Lister (F3, board issue #150). Before this the page read a media file whole into a
// blob: address, so a film took long to start, files over 256 MB were not shown at all, and formats the web view
// can't play (MPEG-2 .mpg and .ts TV recordings, .mkv, .avi, .wmv...) showed as hex.
//
// Now a small web server on 127.0.0.1 (a random port, started the first time it is needed) serves the file the Lister
// has open, at an address with a random token the page gets from Lister.Stream. Nothing else can be fetched: the
// address names no path, and the token is dropped when the Lister closes the file. Two ways:
//
//   - "file": a file the web view plays itself (MP4/MOV with H.264 or HEVC, MP3, AAC/M4A, WAV, FLAC) is served as it
//     is, with HTTP ranges (http.ServeContent), so playing starts at once and seeking reads only what is needed.
//   - "hls": anything else is played through ffmpeg (if it is installed) as an HLS stream: a playlist of 6-second
//     segments for the whole play time, each one made by ffmpeg only when the player asks for it (H.264 with the
//     hardware encoder on macOS, AAC sound), so seeking anywhere works without converting the whole film first. The web
//     view on macOS plays HLS itself.
//
// ffprobe tells which of the two a file needs; without ffmpeg only the first way is there.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// streamSegment is the length of one HLS segment, in seconds.
const streamSegment = 6.0

// ListerStream is where the page plays the open file from. URL is "" when it can't be streamed (Error says why).
type ListerStream struct {
	URL   string  `json:"url"`
	HLS   bool    `json:"hls"`   // converted on the fly (ffmpeg)
	Error string  `json:"error"` // why there is no URL
	Secs  float64 `json:"secs"`  // play time, when known
}

type mediaStream struct {
	sid      int // the Lister session it belongs to
	path     string
	mime     string
	hls      bool
	video    bool // (an HLS stream of a sound file has no picture)
	duration float64
	skip     int64           // bytes ffmpeg skips at the start (see mpegSkip)
	ctx      context.Context // cancelled when the Lister lets the file go: running ffmpegs end
	cancel   context.CancelFunc
	slots    chan struct{} // at most a few ffmpegs at a time per stream
}

type mediaServer struct {
	mu      sync.Mutex
	base    string // "http://127.0.0.1:port"
	streams map[string]*mediaStream
	start   error
}

var mediaSrv = &mediaServer{streams: map[string]*mediaStream{}}

// ensure starts the server once.
func (m *mediaServer) ensure() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.base != "" || m.start != nil {
		return m.start
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		m.start = err
		return err
	}
	m.base = "http://" + ln.Addr().String()
	srv := &http.Server{Handler: http.HandlerFunc(m.serve), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln) //nolint:errcheck // (ends with the app)
	return nil
}

func (m *mediaServer) add(st *mediaStream) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	m.mu.Lock()
	m.streams[tok] = st
	m.mu.Unlock()
	return tok
}

// dropSession forgets the streams of a Lister session and stops their ffmpegs.
func (m *mediaServer) dropSession(sid int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tok, st := range m.streams {
		if st.sid == sid {
			st.cancel()
			delete(m.streams, tok)
		}
	}
}

func (m *mediaServer) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// /m/<token>/<what>
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/m/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	m.mu.Lock()
	st := m.streams[parts[0]]
	m.mu.Unlock()
	if st == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*") // (the page's origin is the app's own scheme)
	w.Header().Set("Cache-Control", "no-store")
	switch what := parts[1]; {
	case !st.hls && what == "file":
		f, err := os.Open(st.path)
		if err != nil {
			http.Error(w, "can't open the file", http.StatusNotFound)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "can't read the file", http.StatusInternalServerError)
			return
		}
		if st.mime != "" {
			w.Header().Set("Content-Type", st.mime)
		}
		http.ServeContent(w, r, "", fi.ModTime(), f)
	case st.hls && what == "index.m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write([]byte(hlsPlaylist(st.duration)))
	case st.hls && strings.HasPrefix(what, "seg") && strings.HasSuffix(what, ".ts"):
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(what, "seg"), ".ts"))
		if err != nil || n < 0 || float64(n)*streamSegment >= st.duration+streamSegment {
			http.NotFound(w, r)
			return
		}
		st.segment(w, r, n)
	default:
		http.NotFound(w, r)
	}
}

// hlsPlaylist lists the segments of a film of secs seconds (the last one shorter).
func hlsPlaylist(secs float64) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MEDIA-SEQUENCE:0\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(streamSegment)))
	for i := 0; float64(i)*streamSegment < secs; i++ {
		d := math.Min(streamSegment, secs-float64(i)*streamSegment)
		if d < 0.05 {
			break
		}
		fmt.Fprintf(&b, "#EXTINF:%.3f,\nseg%d.ts\n", d, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

// segment makes segment n with ffmpeg and sends it as it is made.
func (st *mediaStream) segment(w http.ResponseWriter, r *http.Request, n int) {
	select {
	case st.slots <- struct{}{}:
		defer func() { <-st.slots }()
	case <-r.Context().Done():
		return
	case <-st.ctx.Done():
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-st.ctx.Done(): // the Lister let the file go
			cancel()
		case <-ctx.Done():
		}
	}()
	start := float64(n) * streamSegment
	cmd := exec.CommandContext(ctx, ffmpegPath("ffmpeg"), segmentArgs(st.path, st.skip, start, streamSegment, st.video, videoEncoder())...)
	hideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "can't start ffmpeg", http.StatusInternalServerError)
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
	if err := cmd.Start(); err != nil {
		http.Error(w, "can't start ffmpeg", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	buf := make([]byte, 256<<10)
	wrote := false
	for {
		k, rerr := out.Read(buf)
		if k > 0 {
			wrote = true
			if _, werr := w.Write(buf[:k]); werr != nil {
				break
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if rerr != nil {
			break
		}
	}
	if err := cmd.Wait(); err != nil && !wrote && ctx.Err() == nil {
		http.Error(w, "ffmpeg: "+strings.TrimSpace(stderr.String()), http.StatusInternalServerError)
	}
}

// segmentArgs are ffmpeg's arguments for one segment: secs seconds from start, as MPEG-TS with H.264 and AAC, with the
// timestamps of their place in the whole film (so the player joins the segments without gaps). Interlaced TV pictures
// are deinterlaced (only the interlaced frames), and pictures stored with non-square pixels (most TV: 720x576 shown as
// 4:3 or 16:9) are scaled to square ones, so the film shows at its real shape. skip: bytes to leave out at the start.
func segmentArgs(path string, skip int64, start, secs float64, video bool, enc []string) []string {
	a := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if skip > 0 {
		a = append(a, "-skip_initial_bytes", strconv.FormatInt(skip, 10))
	}
	if start > 0 {
		a = append(a, "-ss", fmt.Sprintf("%.3f", start))
	}
	a = append(a, "-i", path, "-t", fmt.Sprintf("%.3f", secs), "-map", "0:a:0?")
	if video {
		a = append(a, "-map", "0:v:0", "-vf", "yadif=mode=0:parity=-1:deint=1,scale='if(gt(sar,0),trunc(iw*sar/2)*2,iw)':ih,setsar=1,format=yuv420p")
		a = append(a, enc...)
		a = append(a, "-force_key_frames", "expr:eq(n,0)")
	} else {
		a = append(a, "-vn")
	}
	a = append(a, "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-sn", "-dn",
		"-output_ts_offset", fmt.Sprintf("%.3f", start), "-muxdelay", "0", "-muxpreload", "0", "-f", "mpegts", "pipe:1")
	return a
}

var encOnce sync.Once
var encArgs []string

// videoEncoder is the H.264 encoder ffmpeg is asked for: Apple's hardware encoder on macOS (fast, little CPU), else
// x264 at a fast preset.
func videoEncoder() []string {
	encOnce.Do(func() {
		encArgs = []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "21"}
		if runtime.GOOS == "darwin" {
			if out, err := exec.Command(ffmpegPath("ffmpeg"), "-hide_banner", "-encoders").Output(); err == nil && strings.Contains(string(out), "h264_videotoolbox") {
				encArgs = []string{"-c:v", "h264_videotoolbox", "-b:v", "8M", "-maxrate", "12M", "-bufsize", "16M", "-allow_sw", "1"}
			}
		}
	})
	return encArgs
}

type limitedWriter struct {
	w interface{ Write([]byte) (int, error) }
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		_, _ = l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}

// ffmpegPath finds ffmpeg or ffprobe: on the PATH, or where Homebrew and the usual installers put it (an app started
// from the Dock has a short PATH on macOS). "" when it is not installed.
func ffmpegPath(name string) string {
	if p, found, off := ffmpegConfigured(name); off {
		return "" // (Options > Plugins > Use configured external tools off)
	} else if found {
		return p
	}
	ffOnce.Do(func() { ffFound = map[string]string{} })
	ffMu.Lock()
	defer ffMu.Unlock()
	if p, ok := ffFound[name]; ok {
		return p
	}
	p, err := exec.LookPath(name)
	if err != nil {
		p = ""
		for _, dir := range []string{"/opt/homebrew/bin", "/usr/local/bin", "/opt/local/bin", "/usr/bin"} {
			c := filepath.Join(dir, name)
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				p = c
				break
			}
		}
	}
	ffFound[name] = p
	return p
}

var (
	ffOnce  sync.Once
	ffMu    sync.Mutex
	ffFound map[string]string
)

// probeInfo is what ffprobe says about a file.
type probeInfo struct {
	Format struct {
		Name     string `json:"format_name"`
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		Type     string `json:"codec_type"`
		Codec    string `json:"codec_name"`
		Disposal struct {
			Picture int `json:"attached_pic"`
		} `json:"disposition"`
	} `json:"streams"`
}

// mpegSkip finds where an MPEG program stream really starts. A later part of a recording split into pieces (name_003.mpg)
// starts in the middle of the stream, and ffmpeg then takes the picture data for MP3 sound and finds no film at all;
// from the pack before the first sequence header on, it reads it right. 0 for a file that starts properly.
func mpegSkip(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	b := make([]byte, 8<<20)
	n, _ := io.ReadFull(f, b)
	b = b[:n]
	seq := bytes.Index(b, []byte("\x00\x00\x01\xb3"))
	if seq < 0 {
		return 0
	}
	pack := bytes.LastIndex(b[:seq], []byte("\x00\x00\x01\xba"))
	if pack <= 0 {
		return 0
	}
	return int64(pack)
}

func ffprobe(ctx context.Context, path string, skip int64) (*probeInfo, error) {
	bin := ffmpegPath("ffprobe")
	if bin == "" {
		return nil, errors.New("ffprobe is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	args := []string{"-v", "error"}
	if skip > 0 {
		args = append(args, "-skip_initial_bytes", strconv.FormatInt(skip, 10))
	}
	cmd := exec.CommandContext(ctx, bin, append(args, "-show_entries", "format=format_name,duration:stream=codec_type,codec_name:stream_disposition=attached_pic", "-of", "json", path)...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var p probeInfo
	if err := json.Unmarshal(out, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// plan says how a probed file is played: as it is (the web view plays its container and codecs), or through HLS.
// video: it has a picture (a cover image in a sound file does not count).
func (p *probeInfo) plan() (direct, video bool) {
	direct = true
	for _, s := range p.Streams {
		switch s.Type {
		case "video":
			if s.Disposal.Picture == 1 {
				continue
			}
			video = true
			if s.Codec != "h264" && s.Codec != "hevc" {
				direct = false
			}
		case "audio":
			switch {
			case s.Codec == "aac", s.Codec == "mp3", s.Codec == "alac", s.Codec == "flac", strings.HasPrefix(s.Codec, "pcm_"):
			default:
				direct = false
			}
		}
	}
	f := p.Format.Name
	if !(strings.Contains(f, "mp4") || strings.Contains(f, "mov") || f == "mp3" || f == "wav" || f == "flac" || f == "aac") {
		direct = false
	}
	return direct, video
}

// directMIME are the files the web view plays as they are, when ffprobe can't be asked.
var directMIME = map[string]bool{"video/mp4": true, "video/quicktime": true, "audio/mpeg": true, "audio/mp4": true, "audio/wav": true, "audio/flac": true}

// Stream gives the page an address to play the open sound or film from (see the top of this file).
func (l *Lister) Stream(id int) (ListerStream, error) {
	s, err := l.session(id)
	if err != nil {
		return ListerStream{}, err
	}
	s.mu.Lock()
	path := s.path
	s.mu.Unlock()
	info := s.info(id, true)
	if info.Media == nil || (info.Media.Kind != "video" && info.Media.Kind != "audio") {
		return ListerStream{Error: "This is not a sound or film file."}, nil
	}
	if err := mediaSrv.ensure(); err != nil {
		return ListerStream{Error: "Can't start the player: " + err.Error()}, nil
	}
	st := &mediaStream{sid: id, path: path, mime: info.Media.MIME, video: info.Media.Kind == "video", slots: make(chan struct{}, 3)}
	if info.Media.Format == "MPEG" {
		st.skip = mpegSkip(path)
	}
	if p, err := ffprobe(context.Background(), path, st.skip); err == nil {
		direct, video := p.plan()
		st.video = video
		st.duration, _ = strconv.ParseFloat(p.Format.Duration, 64)
		if !direct {
			if st.duration <= 0 {
				return ListerStream{Error: "ffmpeg can't tell how long this file plays, so it can't be played here."}, nil
			}
			st.hls = true
		}
	} else if !directMIME[info.Media.MIME] {
		return ListerStream{Error: "Playing this kind of file needs ffmpeg (for example: brew install ffmpeg)."}, nil
	}
	st.ctx, st.cancel = context.WithCancel(context.Background())
	tok := mediaSrv.add(st)
	r := ListerStream{HLS: st.hls, Secs: st.duration}
	if st.hls {
		r.URL = mediaSrv.base + "/m/" + tok + "/index.m3u8"
	} else {
		r.URL = mediaSrv.base + "/m/" + tok + "/file"
	}
	return r, nil
}
