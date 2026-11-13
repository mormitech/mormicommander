// Telling console programs from window (GUI) programs. Total Commander gives a console program typed on the command
// line its own console window, while a window program just starts. The answer is in the program file: a Windows
// .exe says so in its PE header (the subsystem field), a macOS program that links AppKit or lives in a .app bundle
// is a window program, and a Linux ELF program that links an X11, Wayland, GTK, Qt or SDL library is one too.
// Scripts (#!) and batch files are console programs. Only the standard library's debug/pe, debug/macho and
// debug/elf readers are used, so this works for program files of any platform on any platform.
package launch

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"os"
	"path/filepath"
	"strings"
)

// ExecKind is what kind of program a file is.
type ExecKind int

const (
	KindUnknown ExecKind = iota // not a program, or a format we cannot read
	KindGUI                     // a window program
	KindConsole                 // a console program or script
)

func (k ExecKind) String() string {
	switch k {
	case KindGUI:
		return "gui"
	case KindConsole:
		return "console"
	}
	return "unknown"
}

// guiLibraries are fragments of library names whose presence makes an ELF or Mach-O program a window program.
var guiLibraries = []string{
	"libx11", "libxcb.so", "libwayland-client", "libgtk", "libgdk", "libqt", "libsdl", "libglfw", "libfltk",
	"libwx_", "libgio-2", "appkit.framework", "cocoa.framework", "uikit.framework", "swiftui.framework",
}

// ExecKindOf inspects the file at path.
func ExecKindOf(path string) ExecKind {
	fi, err := os.Stat(path)
	if err != nil {
		return KindUnknown
	}
	if fi.IsDir() {
		if strings.HasSuffix(strings.TrimRight(path, "/"), ".app") {
			return KindGUI
		}
		return KindUnknown
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bat", ".cmd", ".ps1", ".vbs":
		return KindConsole
	}
	if strings.Contains(filepath.ToSlash(path), ".app/Contents/MacOS/") {
		return KindGUI
	}
	f, err := os.Open(path)
	if err != nil {
		return KindUnknown
	}
	defer f.Close()
	head := make([]byte, 4)
	n, _ := f.ReadAt(head, 0)
	head = head[:n]
	switch {
	case bytes.HasPrefix(head, []byte("#!")):
		return KindConsole
	case bytes.HasPrefix(head, []byte("MZ")):
		pf, err := pe.NewFile(f)
		if err != nil {
			return KindUnknown
		}
		defer pf.Close()
		var sub uint16
		switch h := pf.OptionalHeader.(type) {
		case *pe.OptionalHeader32:
			sub = h.Subsystem
		case *pe.OptionalHeader64:
			sub = h.Subsystem
		default:
			return KindUnknown
		}
		switch sub {
		case pe.IMAGE_SUBSYSTEM_WINDOWS_GUI:
			return KindGUI
		case pe.IMAGE_SUBSYSTEM_WINDOWS_CUI, pe.IMAGE_SUBSYSTEM_POSIX_CUI, pe.IMAGE_SUBSYSTEM_OS2_CUI:
			return KindConsole
		}
		return KindUnknown
	case bytes.HasPrefix(head, []byte("\x7fELF")):
		ef, err := elf.NewFile(f)
		if err != nil {
			return KindUnknown
		}
		defer ef.Close()
		if ef.Type != elf.ET_EXEC && ef.Type != elf.ET_DYN {
			return KindUnknown
		}
		libs, _ := ef.ImportedLibraries()
		return kindFromLibs(libs)
	}
	if mf, err := macho.NewFile(f); err == nil {
		defer mf.Close()
		if mf.Type != macho.TypeExec {
			return KindUnknown
		}
		libs, _ := mf.ImportedLibraries()
		return kindFromLibs(libs)
	}
	if ff, err := macho.NewFatFile(f); err == nil {
		defer ff.Close()
		if len(ff.Arches) == 0 || ff.Arches[0].Type != macho.TypeExec {
			return KindUnknown
		}
		libs, _ := ff.Arches[0].ImportedLibraries()
		return kindFromLibs(libs)
	}
	return KindUnknown
}

func kindFromLibs(libs []string) ExecKind {
	for _, lib := range libs {
		ll := strings.ToLower(lib)
		for _, g := range guiLibraries {
			if strings.Contains(ll, g) {
				return KindGUI
			}
		}
	}
	return KindConsole
}
