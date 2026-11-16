package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// Which files the web view plays as they are, and which go through ffmpeg (board issue #150).
func TestStreamPlan(t *testing.T) {
	for _, c := range []struct {
		probe         string
		direct, video bool
	}{
		{`{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2"},"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}]}`, true, true},
		{`{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2"},"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"ac3"}]}`, false, true},
		{`{"format":{"format_name":"mpeg"},"streams":[{"codec_type":"video","codec_name":"mpeg2video"},{"codec_type":"audio","codec_name":"mp2"}]}`, false, true},
		{`{"format":{"format_name":"matroska,webm"},"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}]}`, false, true},
		{`{"format":{"format_name":"mp3"},"streams":[{"codec_type":"audio","codec_name":"mp3"},{"codec_type":"video","codec_name":"mjpeg","disposition":{"attached_pic":1}}]}`, true, false},
		{`{"format":{"format_name":"ogg"},"streams":[{"codec_type":"audio","codec_name":"vorbis"}]}`, false, false},
	} {
		var p probeInfo
		if err := json.Unmarshal([]byte(c.probe), &p); err != nil {
			t.Fatal(err)
		}
		if d, v := p.plan(); d != c.direct || v != c.video {
			t.Errorf("%s: direct %v video %v, want %v %v", c.probe, d, v, c.direct, c.video)
		}
	}
}

func TestHLSPlaylist(t *testing.T) {
	pl := hlsPlaylist(14.5)
	if !strings.Contains(pl, "seg0.ts") || !strings.Contains(pl, "seg2.ts") || strings.Contains(pl, "seg3.ts") || !strings.Contains(pl, "#EXTINF:2.500,") || !strings.HasSuffix(pl, "#EXT-X-ENDLIST\n") {
		t.Fatalf("playlist:\n%s", pl)
	}
	// each segment keeps its place in the whole film, so the player joins them without gaps
	a := strings.Join(segmentArgs("/f.mpg", 0, 12, 6, true, []string{"-c:v", "libx264"}), " ")
	if !strings.Contains(a, "setsar=1") {
		t.Fatalf("pictures are not made square: %s", a)
	}
	if b := strings.Join(segmentArgs("/f.mpg", 2034, 0, 6, true, nil), " "); !strings.Contains(b, "-skip_initial_bytes 2034 -i /f.mpg") {
		t.Fatalf("skip: %s", b)
	}
	if !strings.Contains(a, "-ss 12.000 -i /f.mpg -t 6.000") || !strings.Contains(a, "-output_ts_offset 12.000") || !strings.Contains(a, "-c:v libx264") {
		t.Fatalf("args: %s", a)
	}
	if a := strings.Join(segmentArgs("/f.ogg", 0, 0, 6, false, nil), " "); strings.Contains(a, "-ss") || !strings.Contains(a, "-vn") {
		t.Fatalf("sound args: %s", a)
	}
}
