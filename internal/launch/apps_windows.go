// The "Open with" list on Windows, read from the registry the way Explorer builds its own menu: the user's choice
// (FileExts\<.ext>\UserChoice) or the class default (HKCR\<.ext>) is the default; the ProgIDs under OpenWithProgids
// and the programs under OpenWithList (per user and per machine) are the other candidates. Each candidate's
// shell\open\command gives the command template with %1, which OpenWithAppCommand fills in. Entries that only have
// a COM handler (DelegateExecute, no command) or are marked NoOpenWith are left out.
package launch

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const fileExtsKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\`

func platformApps(l *Launcher, path string) ([]App, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return nil, nil
	}
	var apps []App
	seen := map[string]bool{}
	add := func(app App, ok bool) {
		key := strings.ToLower(app.Exec)
		if !ok || app.Exec == "" || seen[key] {
			return
		}
		seen[key] = true
		apps = append(apps, app)
	}
	defaultProgID := regString(registry.CURRENT_USER, fileExtsKey+ext+`\UserChoice`, "ProgId")
	if defaultProgID == "" {
		defaultProgID = regString(registry.CLASSES_ROOT, ext, "")
	}
	if defaultProgID != "" {
		app, ok := progIDApp(defaultProgID)
		app.Default = true
		add(app, ok)
	}
	for _, p := range regValueNames(registry.CLASSES_ROOT, ext+`\OpenWithProgids`) {
		add(progIDApp(p))
	}
	for _, v := range regValueNames(registry.CURRENT_USER, fileExtsKey+ext+`\OpenWithProgids`) {
		add(progIDApp(v))
	}
	// OpenWithList holds program file names as values a, b, c … (per user) and as subkeys (per machine).
	for _, v := range regValueNames(registry.CURRENT_USER, fileExtsKey+ext+`\OpenWithList`) {
		if v == "MRUList" {
			continue
		}
		add(applicationApp(regString(registry.CURRENT_USER, fileExtsKey+ext+`\OpenWithList`, v)))
	}
	for _, exe := range regSubkeys(registry.CLASSES_ROOT, ext+`\OpenWithList`) {
		add(applicationApp(exe))
	}
	return apps, nil
}

func platformAppsForMIME(l *Launcher, mimeType string) ([]App, error) { return nil, ErrUnsupported }

// progIDApp describes the application behind a ProgID such as "txtfile" or "VSCode.txt".
func progIDApp(progID string) (App, bool) {
	if regHasValue(registry.CLASSES_ROOT, progID, "NoOpenWith") {
		return App{}, false
	}
	cmd := regString(registry.CLASSES_ROOT, progID+`\shell\open\command`, "")
	if cmd == "" {
		return App{}, false
	}
	prog, _ := splitWindowsProgram(cmd)
	name := regString(registry.CLASSES_ROOT, `Applications\`+filepath.Base(prog), "FriendlyAppName")
	if name == "" {
		name = regString(registry.CLASSES_ROOT, progID, "")
	}
	if name == "" || strings.HasPrefix(name, "@") {
		name = strings.TrimSuffix(filepath.Base(prog), filepath.Ext(prog))
	}
	return App{Name: name, Path: prog, Exec: cmd}, true
}

// applicationApp describes a program registered under HKCR\Applications\<exe>.
func applicationApp(exe string) (App, bool) {
	if exe == "" {
		return App{}, false
	}
	key := `Applications\` + exe
	if regHasValue(registry.CLASSES_ROOT, key, "NoOpenWith") {
		return App{}, false
	}
	cmd := regString(registry.CLASSES_ROOT, key+`\shell\open\command`, "")
	if cmd == "" {
		return App{}, false
	}
	prog, _ := splitWindowsProgram(cmd)
	name := regString(registry.CLASSES_ROOT, key, "FriendlyAppName")
	if name == "" || strings.HasPrefix(name, "@") {
		name = strings.TrimSuffix(exe, filepath.Ext(exe))
	}
	return App{Name: name, Path: prog, Exec: cmd}, true
}

func regString(root registry.Key, path, name string) string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	s, typ, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	if typ == registry.EXPAND_SZ {
		if x, err := registry.ExpandString(s); err == nil {
			s = x
		}
	}
	return s
}

func regHasValue(root registry.Key, path, name string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetValue(name, nil)
	return err == nil
}

func regValueNames(root registry.Key, path string) []string {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	return names
}

func regSubkeys(root registry.Key, path string) []string {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	return names
}
