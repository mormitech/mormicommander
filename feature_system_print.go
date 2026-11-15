package main

// Printing (board issue #38): Ctrl+F9 prints the file under the cursor (cm_PrintFile, F-232, F-335) and Files > Print
// file list prints the list of the current folder (cm_PrintDir, F-280, F-334). Both go through the system's own print
// path in internal/launch (PrintCommand): CUPS's lp on macOS and Linux, the associated app's "print" verb on Windows.
// The page asks before it calls either method, because lp sends the job to the default printer without a dialog of
// its own. Nothing here changes or removes a file of the user's; the file list is written to a temporary file of ours.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"mormicommander/internal/launch"
)

// printTimeout bounds how long the page waits for lp to accept a job. lp only hands the file to the print queue, so
// this is generous; it keeps a stuck print system from holding the page's call forever.
const printTimeout = 30 * time.Second

// Print prints a file on this computer through the system (cm_PrintFile).
func (s *SystemLaunch) Print(path string) error {
	if s.app.mock != nil {
		return errors.New("This is the demo: its files are not real, so they can't be printed.")
	}
	if strings.Contains(path, "://") {
		return errors.New("Only files on this computer can be printed.")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return systemErr("print", path, err)
	}
	if fi.IsDir() {
		return errors.New("A folder can't be printed. Put the cursor on a file.")
	}
	l := s.launcher()
	return systemPrint(l, l.PrintCommand(path), path)
}

// PrintText prints a piece of text, such as the file list of a folder (cm_PrintDir). The text is written to a new
// temporary file named after name, which is removed again once the print system has taken it (macOS, Linux); on
// Windows the printing app may still be reading it when PowerShell returns, so the file is left to the system's
// cleaning of the temporary folder.
func (s *SystemLaunch) PrintText(name, text string) error {
	if s.app.mock != nil {
		return errors.New("This is the demo: nothing is printed.")
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("There is nothing to print.")
	}
	l := s.launcher()
	dir := os.TempDir()
	if l.TempDir != "" {
		dir = l.TempDir
	}
	f, err := os.CreateTemp(dir, printTempPattern(name))
	if err != nil {
		return fmt.Errorf("Can't write the text to print: %v", err)
	}
	_, werr := f.WriteString(text)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(f.Name())
		return fmt.Errorf("Can't write the text to print: %v", errors.Join(werr, cerr))
	}
	c := l.PrintCommand(f.Name())
	if l.OS != "windows" {
		c.Cleanup = []string{f.Name()}
	}
	return systemPrint(l, c, name)
}

// printTempPattern is the os.CreateTemp pattern of a text to print: the name with anything that is not a plain
// letter, digit, dot or dash replaced, so the file name is safe everywhere, and ".txt" so Windows prints it as text.
func printTempPattern(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
		if b.Len() >= 40 {
			break
		}
	}
	base := strings.Trim(b.String(), "._")
	if base == "" {
		base = "print"
	}
	return "mormicommander-" + base + "-*.txt"
}

// systemPrint runs a print command. On macOS and Linux it waits for lp, so that a missing printer or a file type CUPS
// can't print comes back as a message instead of being lost; on Windows the print verb is started and left to run.
func systemPrint(l *launch.Launcher, c launch.Command, what string) error {
	if l.OS == "windows" {
		if err := l.Start(c); err != nil {
			return systemErr("print", what, err)
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), printTimeout)
	defer cancel()
	out, err := l.Output(ctx, c)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("Can't print %s: %s", printName(what), msg)
	}
	return nil
}

// printName is the last part of a path, for messages.
func printName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 && i < len(p)-1 {
		return p[i+1:]
	}
	return p
}
