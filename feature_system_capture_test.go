package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mormicommander/internal/launch"
)

// F-293: which command lines the output window takes, and that StopCapture stops a running one.
func TestSystemLineCaptures(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	s, _ := recordingLaunch(NewApp(startOpts{}))
	for _, line := range []string{"cd sub", "cm_Copy", "", "sub"} {
		if s.LineCaptures(line, dir) {
			t.Errorf("%q must not be captured", line)
		}
	}
	if runtime.GOOS != "windows" && !s.LineCaptures("ls | wc -l", dir) {
		t.Error("a shell command is captured")
	}
}

func TestSystemRunLineCapturedStop(t *testing.T) {
	s := newSystemLaunch(NewApp(startOpts{}))
	started := make(chan struct{})
	s.newLauncher = func() *launch.Launcher {
		l := launch.New()
		l.Capture = func(ctx context.Context, c launch.Command, limit int) (launch.CaptureResult, error) {
			close(started)
			select {
			case <-ctx.Done():
				return launch.CaptureResult{Output: []byte("partial\xff"), ExitCode: -1}, nil
			case <-time.After(10 * time.Second):
				return launch.CaptureResult{}, nil
			}
		}
		return l
	}
	if s.StopCapture("x") {
		t.Error("nothing runs yet")
	}
	go func() { <-started; s.StopCapture("x") }()
	r, err := s.RunLineCaptured("x", "sleep 100", t.TempDir())
	if err != nil || !r.Stopped || r.Output != "partial�" || r.ExitCode != -1 {
		t.Errorf("result %+v %v", r, err)
	}
	if s.StopCapture("x") {
		t.Error("the run is over: nothing to stop")
	}
}
