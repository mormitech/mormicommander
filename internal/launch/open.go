// Opening files, folders and web addresses with the system: Enter on a file (F-051), "Open with…" (F-200), Finder
// or Explorer in the current folder (cm_Explorer, F-366), "Reveal in Finder/Explorer/file manager" from the context
// menu (F-703) and printing through the associated app (F-335). Each OS has one well-known way to do these:
// macOS has /usr/bin/open, Linux has xdg-open and the FileManager1 D-Bus interface, and Windows has
// rundll32 url.dll,FileProtocolHandler, which hands the file to the shell like a double-click does. Windows'
// "cmd /c start" is never used: cmd would read characters such as & and ^ in a file name as commands.
package launch

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// safeArg keeps a relative path that starts with '-' from being read as an option by open or xdg-open.
func safeArg(p string) string {
	if strings.HasPrefix(p, "-") {
		return "./" + p
	}
	return p
}

// OpenDefaultCommand returns the command that opens a file, folder or URL with its default app.
func (l *Launcher) OpenDefaultCommand(target string) Command {
	switch {
	case l.darwin():
		return Command{Path: "open", Args: []string{safeArg(target)}}
	case l.windows():
		// FileProtocolHandler takes the rest of the command line as the file, so the name is not quoted.
		return Command{Path: "rundll32.exe", Params: "url.dll,FileProtocolHandler " + target}
	default:
		return Command{Path: "xdg-open", Args: []string{safeArg(target)}}
	}
}

// OpenDefault opens a file, folder or URL with the app the system uses for it.
func (l *Launcher) OpenDefault(target string) error { return l.start(l.OpenDefaultCommand(target)) }

// OpenFolderCommand returns the command that shows a folder in Finder, Explorer or the desktop's file manager.
func (l *Launcher) OpenFolderCommand(dir string) Command {
	if l.windows() {
		return Command{Path: "explorer.exe", Params: QuoteWindowsArg(dir)}
	}
	return l.OpenDefaultCommand(dir)
}

// OpenFolder shows a folder in the system file manager (cm_Explorer).
func (l *Launcher) OpenFolder(dir string) error { return l.start(l.OpenFolderCommand(dir)) }

// RevealCommand returns the command that opens the system file manager with path selected.
func (l *Launcher) RevealCommand(path string) Command {
	switch {
	case l.darwin():
		return Command{Path: "open", Args: []string{"-R", safeArg(path)}}
	case l.windows():
		return Command{Path: "explorer.exe", Params: "/select," + QuoteWindowsArg(path)}
	default:
		if l.has("dbus-send") {
			// Nautilus, Dolphin, Nemo, Thunar and others implement org.freedesktop.FileManager1.
			return Command{Path: "dbus-send", Args: []string{
				"--session", "--dest=org.freedesktop.FileManager1", "--type=method_call",
				"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
				"array:string:" + fileURI(path), "string:",
			}}
		}
		return Command{Path: "xdg-open", Args: []string{safeArg(parentDir(path))}}
	}
}

// Reveal opens the system file manager with path selected.
func (l *Launcher) Reveal(path string) error { return l.start(l.RevealCommand(path)) }

func parentDir(p string) string {
	d := filepath.Dir(p)
	if d == "" {
		return "."
	}
	return d
}

// fileURI turns an absolute path into a file:// URL with the special characters escaped.
func fileURI(p string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	return u.String()
}

// OpenWithCommand returns the command that opens path with a chosen application. app is, per platform:
//
//   - macOS: an application name ("TextEdit"), a path to a .app bundle, or a bundle id ("com.apple.TextEdit");
//   - Windows: a program path, or a command template that contains %1 (as found in the registry);
//   - Linux: a .desktop file path or id ("org.gnome.gedit.desktop"), a program name, or an Exec-style template with
//     %f/%F/%u/%U field codes.
func (l *Launcher) OpenWithCommand(path, app string) (Command, error) {
	switch {
	case l.darwin():
		if isBundleID(app) {
			return Command{Path: "open", Args: []string{"-b", app, safeArg(path)}}, nil
		}
		return Command{Path: "open", Args: []string{"-a", app, safeArg(path)}}, nil
	case l.windows():
		if strings.Contains(app, "%1") || strings.Contains(app, "%L") || strings.Contains(app, "%*") {
			prog, params := splitWindowsProgram(app)
			params = expandWindowsTemplate(params, path)
			return Command{Path: prog, Params: params}, nil
		}
		prog, params := splitWindowsProgram(app)
		if params != "" {
			params += " "
		}
		return Command{Path: prog, Params: params + QuoteWindowsArg(path)}, nil
	default:
		if strings.HasSuffix(app, ".desktop") {
			entry, location, err := findDesktopEntry(l, app)
			if err != nil {
				return Command{}, err
			}
			return l.desktopCommand(entry, location, []string{path})
		}
		if strings.Contains(app, "%") {
			argv, err := ExpandDesktopExec(app, []string{path}, "", "", "")
			if err != nil {
				return Command{}, err
			}
			if len(argv) == 0 {
				return Command{}, os.ErrInvalid
			}
			return Command{Path: argv[0], Args: argv[1:]}, nil
		}
		return Command{Path: app, Args: []string{path}}, nil
	}
}

// OpenWith opens path with a chosen application (see OpenWithCommand for what app may be).
func (l *Launcher) OpenWith(path, app string) error {
	c, err := l.OpenWithCommand(path, app)
	if err != nil {
		return err
	}
	return l.start(c)
}

// OpenWithApp opens path with an application from the AppsFor list.
func (l *Launcher) OpenWithApp(path string, app App) error {
	c, err := l.OpenWithAppCommand(path, app)
	if err != nil {
		return err
	}
	return l.start(c)
}

// OpenWithAppCommand returns the command OpenWithApp would run.
func (l *Launcher) OpenWithAppCommand(path string, app App) (Command, error) {
	switch {
	case l.darwin():
		return Command{Path: "open", Args: []string{"-a", app.Path, safeArg(path)}}, nil
	case l.windows():
		if app.Exec != "" {
			return l.OpenWithCommand(path, app.Exec)
		}
		return l.OpenWithCommand(path, app.Path)
	default:
		if app.Exec != "" {
			e := DesktopEntry{Name: app.Name, Exec: app.Exec, Terminal: app.Terminal, Icon: app.Icon}
			return l.desktopCommand(e, app.Path, []string{path})
		}
		return l.OpenWithCommand(path, app.Path)
	}
}

// desktopCommand builds the command for a desktop entry, wrapping it in a terminal when the entry asks for one.
func (l *Launcher) desktopCommand(e DesktopEntry, location string, files []string) (Command, error) {
	argv, err := ExpandDesktopExec(e.Exec, files, e.Name, e.Icon, location)
	if err != nil {
		return Command{}, err
	}
	if len(argv) == 0 {
		return Command{}, os.ErrInvalid
	}
	if e.Terminal {
		dir := ""
		if len(files) > 0 {
			dir = filepath.Dir(files[0])
		}
		return l.terminalArgvCommand(dir, argv)
	}
	return Command{Path: argv[0], Args: argv[1:]}, nil
}

// isBundleID recognises macOS bundle identifiers such as "com.apple.TextEdit".
func isBundleID(s string) bool {
	if strings.ContainsAny(s, "/ ") || strings.HasSuffix(strings.ToLower(s), ".app") {
		return false
	}
	return strings.Count(s, ".") >= 2
}

// splitWindowsProgram splits a Windows command template into the program (unquoted) and the raw rest.
func splitWindowsProgram(s string) (string, string) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) {
		if end := strings.IndexByte(s[1:], '"'); end >= 0 {
			return s[1 : end+1], strings.TrimSpace(s[end+2:])
		}
		return strings.Trim(s, `"`), ""
	}
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// expandWindowsTemplate fills a registry-style command template: %1 and %L are the file (quoted when the template
// does not quote it already), %* is all arguments (here: the file again), %% is a percent sign.
func expandWindowsTemplate(t, file string) string {
	var b strings.Builder
	for i := 0; i < len(t); i++ {
		if t[i] != '%' || i+1 >= len(t) {
			b.WriteByte(t[i])
			continue
		}
		switch t[i+1] {
		case '1', 'L', 'l', '*':
			inQuotes := strings.Count(t[:i], `"`)%2 == 1
			if inQuotes {
				b.WriteString(file)
			} else {
				b.WriteString(QuoteWindowsArg(file))
			}
			i++
		case '%':
			b.WriteByte('%')
			i++
		default:
			b.WriteByte('%')
		}
	}
	return b.String()
}

// OpenWithChooserCommand returns the command that shows the system's own "Open with" dialog. Only Windows has one
// that another program can call; elsewhere ErrUnsupported is returned and the UI shows the AppsFor list instead.
func (l *Launcher) OpenWithChooserCommand(path string) (Command, error) {
	if l.windows() {
		return Command{Path: "rundll32.exe", Params: "shell32.dll,OpenAs_RunDLL " + path}, nil
	}
	return Command{}, ErrUnsupported
}

// OpenWithChooser shows the system's "Open with" dialog for path (Windows only).
func (l *Launcher) OpenWithChooser(path string) error {
	c, err := l.OpenWithChooserCommand(path)
	if err != nil {
		return err
	}
	return l.start(c)
}

// PrintCommand returns the command that prints a file. macOS and Linux send it to CUPS with lp, which prints
// PDF, PostScript, images and text. Windows asks the file's associated app to print it through the "print" verb;
// the path goes to PowerShell in an environment variable, never inside the script text.
func (l *Launcher) PrintCommand(path string) Command {
	if l.windows() {
		return Command{
			Path:   "powershell.exe",
			Params: `-NoProfile -NonInteractive -Command "Start-Process -FilePath $env:MORMI_PRINT_FILE -Verb Print"`,
			Env:    []string{"MORMI_PRINT_FILE=" + path},
			Show:   ShowHidden,
		}
	}
	return Command{Path: "lp", Args: []string{"--", path}}
}

// Print prints a file through the system (cm_PrintFile).
func (l *Launcher) Print(path string) error { return l.start(l.PrintCommand(path)) }
