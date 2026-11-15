package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mormicommander/internal/launch"
)

// printingLaunch is a SystemLaunch whose launcher pretends to be os and records the print commands instead of
// running them. out and fail are what the pretend lp answers.
func printingLaunch(a *App, osName, out string, fail bool) (*SystemLaunch, *[]launch.Command, *[]string) {
	var got []launch.Command
	var texts []string
	s := newSystemLaunch(a)
	s.newLauncher = func() *launch.Launcher {
		l := launch.New()
		l.OS = osName
		record := func(c launch.Command) {
			got = append(got, c)
			// the text file must still be there while the print system reads it
			if b, err := os.ReadFile(c.Args[len(c.Args)-1]); err == nil {
				texts = append(texts, string(b))
			}
		}
		l.Start = func(c launch.Command) error { got = append(got, c); return nil }
		l.Output = func(_ context.Context, c launch.Command) ([]byte, error) {
			record(c)
			if fail {
				return []byte(out), errors.New("exit status 1")
			}
			return []byte(out), nil
		}
		return l
	}
	return s, &got, &texts
}

func TestSystemPrintFile(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "a b.txt")
	os.WriteFile(doc, []byte("hello"), 0o644)
	s, got, _ := printingLaunch(NewApp(startOpts{}), "darwin", "request id is x-1", false)
	if err := s.Print(doc); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0].Path != "lp" || (*got)[0].Args[len((*got)[0].Args)-1] != doc {
		t.Fatalf("print command: %+v", *got)
	}
	if err := s.Print(dir); err == nil || !strings.Contains(err.Error(), "folder") {
		t.Errorf("a folder is not printed: %v", err)
	}
	if err := s.Print("ftp://host/a.txt"); err == nil {
		t.Error("a file on a server is not printed")
	}
	if err := s.Print(filepath.Join(dir, "missing.txt")); err == nil {
		t.Error("a missing file is not printed")
	}
	if len(*got) != 1 {
		t.Errorf("refused files still started commands: %+v", *got)
	}
}

func TestSystemPrintReportsLpError(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "a.txt")
	os.WriteFile(doc, []byte("hello"), 0o644)
	s, _, _ := printingLaunch(NewApp(startOpts{}), "linux", "lp: No default destination.", true)
	err := s.Print(doc)
	if err == nil || !strings.Contains(err.Error(), "No default destination") || !strings.Contains(err.Error(), "a.txt") {
		t.Errorf("lp's message is passed on: %v", err)
	}
}

func TestSystemPrintText(t *testing.T) {
	s, got, texts := printingLaunch(NewApp(startOpts{}), "darwin", "", false)
	if err := s.PrintText("File list of /tmp", "a.txt\nb.txt\n"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || len(*texts) != 1 || (*texts)[0] != "a.txt\nb.txt\n" {
		t.Fatalf("printed %+v with %q", *got, *texts)
	}
	c := (*got)[0]
	file := c.Args[len(c.Args)-1]
	if !strings.HasPrefix(filepath.Base(file), "mormicommander-File_list_of__tmp-") || !strings.HasSuffix(file, ".txt") {
		t.Errorf("temporary file name %q", file)
	}
	if len(c.Cleanup) != 1 || c.Cleanup[0] != file {
		t.Errorf("the temporary file is cleaned up after lp: %+v", c.Cleanup)
	}
	os.Remove(file) // (the pretend Output does not clean up)
	if err := s.PrintText("x", "  \n"); err == nil {
		t.Error("empty text is not printed")
	}
}

func TestSystemPrintTextWindowsKeepsFile(t *testing.T) {
	s, got, _ := printingLaunch(NewApp(startOpts{}), "windows", "", false)
	if err := s.PrintText("list", "a\n"); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 1 || (*got)[0].Path != "powershell.exe" || len((*got)[0].Cleanup) != 0 {
		t.Fatalf("windows print: %+v", *got)
	}
	for _, e := range (*got)[0].Env {
		if f, ok := strings.CutPrefix(e, "MORMI_PRINT_FILE="); ok {
			os.Remove(f)
		}
	}
}

func TestSystemPrintDemo(t *testing.T) {
	s, got, _ := printingLaunch(NewApp(startOpts{Demo: true}), "darwin", "", false)
	if err := s.Print("/x/a.txt"); err == nil {
		t.Error("the demo prints nothing")
	}
	if err := s.PrintText("x", "y"); err == nil {
		t.Error("the demo prints no text")
	}
	if len(*got) != 0 {
		t.Errorf("commands started in the demo: %+v", *got)
	}
}

func TestPrintTempPattern(t *testing.T) {
	for in, want := range map[string]string{
		"File list":      "mormicommander-File_list-*.txt",
		"":               "mormicommander-print-*.txt",
		"../..":          "mormicommander-print-*.txt",
		"a/b\\c:d*e?":    "mormicommander-a_b_c_d_e-*.txt",
		"report-2.final": "mormicommander-report-2.final-*.txt",
	} {
		if got := printTempPattern(in); got != want {
			t.Errorf("printTempPattern(%q) = %q, want %q", in, got, want)
		}
	}
}
