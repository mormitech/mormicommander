// Hotkey overrides (Configuration > Misc > Redefine hotkeys, F-431, and the
// hotkey rule at the top of FEATURES.md). The default keys come from the
// command registry; the settings file only stores what the user changed, in
// TC's format: section [Shortcuts] maps a key to a command, "CS+F1=cm_x",
// where the letters before "+" are the modifiers C (Ctrl), A (Alt) and S
// (Shift), and keys with the Windows key live in [ShortcutsWin] with the W
// left out. The Windows key is the same physical key as Cmd on a Mac and Super
// on Linux, so [ShortcutsWin] also holds our Cmd bindings. An empty value
// ("C+D=") removes a default binding without assigning a new one, which TC
// can't express but users ask for. Key names are normalised (Backspace, BkSp
// and Back are all "Back") so a TC file and our UI agree, and FromEventCode
// translates a browser KeyboardEvent.code to the same names, because the UI
// matches keys by physical code (rule 3 in FEATURES.md).

package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TC hotkey sections.
const (
	SectionShortcuts    = "Shortcuts"
	SectionShortcutsWin = "ShortcutsWin"
)

// Hotkey is a key with modifiers.
type Hotkey struct {
	Ctrl, Alt, Shift, Win bool // Win = Cmd on macOS, Super on Linux
	Key                   string
}

// String renders TC notation: modifiers in the order C, A, S, W, then "+",
// then the key ("CS+F1", "A+F7", "F5").
func (h Hotkey) String() string {
	mods := h.mods(true)
	if mods == "" {
		return h.Key
	}
	return mods + "+" + h.Key
}

func (h Hotkey) mods(withWin bool) string {
	var b strings.Builder
	if h.Ctrl {
		b.WriteByte('C')
	}
	if h.Alt {
		b.WriteByte('A')
	}
	if h.Shift {
		b.WriteByte('S')
	}
	if withWin && h.Win {
		b.WriteByte('W')
	}
	return b.String()
}

// Label renders a readable form like "Ctrl+Shift+F1" (on macOS the UI may
// prefer symbols; that's its business).
func (h Hotkey) Label() string {
	var parts []string
	if h.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if h.Alt {
		parts = append(parts, "Alt")
	}
	if h.Shift {
		parts = append(parts, "Shift")
	}
	if h.Win {
		parts = append(parts, "Cmd")
	}
	return strings.Join(append(parts, h.Key), "+")
}

// ParseHotkey parses TC notation ("CS+F1", "C++", "A+NUM+", "F5") and also
// the readable form ("Ctrl+Shift+F1", "Cmd+C", "Option+F7").
func ParseHotkey(s string) (Hotkey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Hotkey{}, fmt.Errorf("config: empty hotkey")
	}
	var h Hotkey
	// TC compact form: [CASW]+ "+" key.
	if i := strings.Index(s, "+"); i > 0 && i < len(s)-1 && isModLetters(s[:i]) {
		for _, c := range strings.ToUpper(s[:i]) {
			switch c {
			case 'C':
				h.Ctrl = true
			case 'A':
				h.Alt = true
			case 'S':
				h.Shift = true
			case 'W':
				h.Win = true
			}
		}
		h.Key = NormalizeKey(s[i+1:])
		if !ValidKey(h.Key) {
			return Hotkey{}, fmt.Errorf("config: bad hotkey %q", s)
		}
		return h, nil
	}
	// Readable form: words joined by "+"; the key is the rest.
	rest := s
	for {
		i := strings.Index(rest, "+")
		if i <= 0 || i == len(rest)-1 {
			break
		}
		switch strings.ToLower(rest[:i]) {
		case "ctrl", "control", "ctl":
			h.Ctrl = true
		case "alt", "option", "opt":
			h.Alt = true
		case "shift":
			h.Shift = true
		case "cmd", "command", "meta", "win", "super":
			h.Win = true
		default:
			goto done
		}
		rest = rest[i+1:]
	}
done:
	h.Key = NormalizeKey(rest)
	if !ValidKey(h.Key) {
		return Hotkey{}, fmt.Errorf("config: bad hotkey %q", s)
	}
	return h, nil
}

func isModLetters(s string) bool {
	if len(s) > 4 {
		return false
	}
	for _, c := range strings.ToUpper(s) {
		if !strings.ContainsRune("CASW", c) {
			return false
		}
	}
	return true
}

var keyAliases = map[string]string{
	"backspace": "Back", "back": "Back", "bksp": "Back", "bs": "Back",
	"delete": "Del", "del": "Del",
	"insert": "Ins", "ins": "Ins",
	"pageup": "PgUp", "pgup": "PgUp", "prior": "PgUp",
	"pagedown": "PgDn", "pgdn": "PgDn", "next": "PgDn",
	"enter": "Enter", "return": "Enter",
	"escape": "Esc", "esc": "Esc",
	"space": "Space", "spacebar": "Space", " ": "Space",
	"tab": "Tab", "home": "Home", "end": "End",
	"left": "Left", "right": "Right", "up": "Up", "down": "Down",
	"arrowleft": "Left", "arrowright": "Right", "arrowup": "Up", "arrowdown": "Down",
	"num+": "Num+", "add": "Num+", "numpadadd": "Num+",
	"num-": "Num-", "subtract": "Num-", "numpadsubtract": "Num-",
	"num*": "Num*", "multiply": "Num*", "numpadmultiply": "Num*",
	"num/": "Num/", "divide": "Num/", "numpaddivide": "Num/",
	"numenter": "NumEnter", "numpadenter": "NumEnter",
	"apps": "Apps", "contextmenu": "Apps", "menu": "Apps",
	"pause": "Pause", "scroll": "Scroll", "scrolllock": "Scroll",
	"plus": "+", "minus": "-",
}

// NormalizeKey returns the canonical name of a key: single letters upper
// case, F1..F24, Num0..Num9, and the names listed in keyAliases.
func NormalizeKey(k string) string {
	if k == " " {
		return "Space"
	}
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	low := strings.ToLower(k)
	if v, ok := keyAliases[low]; ok {
		return v
	}
	if len([]rune(k)) == 1 {
		return strings.ToUpper(k)
	}
	if low[0] == 'f' && len(low) <= 3 && allDigits(low[1:]) {
		return "F" + low[1:]
	}
	if strings.HasPrefix(low, "num") && len(low) == 4 && allDigits(low[3:]) {
		return "Num" + low[3:]
	}
	if strings.HasPrefix(low, "numpad") && len(low) == 7 && allDigits(low[6:]) {
		return "Num" + low[6:]
	}
	return k
}

// ValidKey reports whether k is a canonical key name: one character, F1..F24,
// Num0..Num9, Num., or a named key such as PgUp or Back.
func ValidKey(k string) bool {
	if len([]rune(k)) == 1 {
		return k != " "
	}
	if k == "Num." {
		return true
	}
	if strings.HasPrefix(k, "F") && allDigits(k[1:]) {
		n, _ := strconv.Atoi(k[1:])
		return n >= 1 && n <= 24
	}
	if strings.HasPrefix(k, "Num") && len(k) == 4 && allDigits(k[3:]) {
		return true
	}
	for _, v := range keyAliases {
		if v == k {
			return true
		}
	}
	return false
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

var eventCodes = map[string]string{
	"Backquote": "`", "Minus": "-", "Equal": "=", "BracketLeft": "[", "BracketRight": "]",
	"Backslash": `\`, "IntlBackslash": `\`, "Semicolon": ";", "Quote": "'", "Comma": ",",
	"Period": ".", "Slash": "/", "NumpadDecimal": "Num.",
}

// FromEventCode converts a browser KeyboardEvent.code ("KeyA", "Digit1",
// "ArrowLeft", "NumpadAdd", "F5") to a canonical key name, or "" for codes
// that are modifiers themselves.
func FromEventCode(code string) string {
	switch {
	case strings.HasPrefix(code, "Key") && len(code) == 4:
		return code[3:]
	case strings.HasPrefix(code, "Digit") && len(code) == 6:
		return code[5:]
	case strings.HasSuffix(code, "Left") && isModifierCode(code), strings.HasSuffix(code, "Right") && isModifierCode(code):
		return ""
	}
	if v, ok := eventCodes[code]; ok {
		return v
	}
	return NormalizeKey(code)
}

func isModifierCode(code string) bool {
	for _, m := range []string{"Shift", "Control", "Alt", "Meta", "OS"} {
		if strings.HasPrefix(code, m) {
			return true
		}
	}
	return false
}

// Binding is one stored override. An empty Command means "unbound".
type Binding struct {
	Key     Hotkey
	Command string
}

// Keymap holds the user's hotkey overrides.
type Keymap struct {
	bindings map[string]Binding // Hotkey.String() -> binding
}

// NewKeymap returns an empty keymap.
func NewKeymap() *Keymap { return &Keymap{bindings: map[string]Binding{}} }

// LoadKeymap reads [Shortcuts] and [ShortcutsWin]. Unparsable keys are
// skipped and reported.
func LoadKeymap(f *File) (*Keymap, []string) {
	k := NewKeymap()
	var bad []string
	for _, sec := range []string{SectionShortcuts, SectionShortcutsWin} {
		for _, kv := range f.Entries(sec) {
			h, err := ParseHotkey(kv.Key)
			if err != nil {
				bad = append(bad, sec+": "+kv.Key)
				continue
			}
			if sec == SectionShortcutsWin {
				h.Win = true
			}
			k.bindings[h.String()] = Binding{Key: h, Command: strings.TrimSpace(kv.Value)}
		}
	}
	return k, bad
}

// Save writes the overrides back, sorted by key for stable files.
func (k *Keymap) Save(f *File) {
	var plain, win []KV
	for _, b := range k.Bindings() {
		if b.Key.Win {
			h := b.Key
			h.Win = false
			win = append(win, KV{h.String(), b.Command})
		} else {
			plain = append(plain, KV{b.Key.String(), b.Command})
		}
	}
	f.ReplaceSection(SectionShortcuts, plain)
	if len(win) > 0 || f.HasSection(SectionShortcutsWin) {
		f.ReplaceSection(SectionShortcutsWin, win)
	}
}

// Bind assigns a command to a key (replacing any override of that key).
func (k *Keymap) Bind(h Hotkey, command string) {
	h.Key = NormalizeKey(h.Key)
	k.bindings[h.String()] = Binding{Key: h, Command: command}
}

// Unbind marks a key as having no command, hiding its default.
func (k *Keymap) Unbind(h Hotkey) { k.Bind(h, "") }

// Reset removes the override of a key so its default applies again.
func (k *Keymap) Reset(h Hotkey) {
	h.Key = NormalizeKey(h.Key)
	delete(k.bindings, h.String())
}

// RestoreDefaults drops every override ("Restore defaults", rule 6).
func (k *Keymap) RestoreDefaults() { k.bindings = map[string]Binding{} }

// Lookup returns the override for a key, if any.
func (k *Keymap) Lookup(h Hotkey) (Binding, bool) {
	h.Key = NormalizeKey(h.Key)
	b, ok := k.bindings[h.String()]
	return b, ok
}

// Bindings lists the overrides sorted by key text.
func (k *Keymap) Bindings() []Binding {
	out := make([]Binding, 0, len(k.bindings))
	for _, b := range k.bindings {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out
}

// Effective merges the overrides onto defaults (key text -> command, keys in
// any notation ParseHotkey accepts) and returns canonical key text ->
// command. Unbound keys are left out.
func (k *Keymap) Effective(defaults map[string]string) map[string]string {
	out := map[string]string{}
	for ks, cmd := range defaults {
		h, err := ParseHotkey(ks)
		if err != nil {
			continue
		}
		out[h.String()] = cmd
	}
	for ks, b := range k.bindings {
		if b.Command == "" {
			delete(out, ks)
		} else {
			out[ks] = b.Command
		}
	}
	return out
}

// KeysFor lists the keys that run command in the effective map, sorted.
func KeysFor(effective map[string]string, command string) []string {
	var out []string
	for ks, cmd := range effective {
		if strings.EqualFold(cmd, command) {
			out = append(out, ks)
		}
	}
	sort.Strings(out)
	return out
}
