// This file saves and loads named presets of the dialog settings (F-513).
// Total Commander keeps them in its ini file under names the user picks; we
// keep one small JSON file per preset in a folder the caller chooses
// (normally a "multirename" folder inside the app's config directory). The
// file name is derived from the preset name with unsafe characters escaped,
// so any name the user types, including slashes or colons, works on every
// system, and the real name is stored inside the file for listing. Loading
// starts from DefaultSettings, so a preset saved by an older version that
// lacks a newer field still gets a sensible value for it.

package multirename

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// presetFile is the on-disk form of a preset.
type presetFile struct {
	Name     string          `json:"name"`
	Settings json.RawMessage `json:"settings"`
}

// ErrPresetName is returned for an empty or blank preset name.
var ErrPresetName = errors.New("multirename: preset name is empty")

// presetPath maps a preset name to its file. Letters, digits, space, '-' and
// '_' are kept; everything else becomes %XX so distinct names never share a
// file, even on disks that ignore case.
func presetPath(dir, name string) string {
	var b strings.Builder
	for _, c := range []byte(name) {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '_', c == ' ':
			b.WriteByte(c)
		case c >= 'A' && c <= 'Z':
			// Escape upper case with a marker so "A" and "a" stay distinct
			// on case-insensitive disks.
			b.WriteByte('^')
			b.WriteByte(c + 'a' - 'A')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return filepath.Join(dir, b.String()+".json")
}

// SavePreset stores s under name, replacing an existing preset of that name.
func SavePreset(dir, name string, s Settings) error {
	if strings.TrimSpace(name) == "" {
		return ErrPresetName
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(presetFile{Name: name, Settings: raw}, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(presetPath(dir, name), data)
}

// LoadPreset reads the preset called name.
func LoadPreset(dir, name string) (Settings, error) {
	data, err := os.ReadFile(presetPath(dir, name))
	if err != nil {
		return Settings{}, err
	}
	return decodePreset(data)
}

func decodePreset(data []byte) (Settings, error) {
	var pf presetFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return Settings{}, fmt.Errorf("multirename: bad preset: %w", err)
	}
	s := DefaultSettings()
	if len(pf.Settings) > 0 {
		if err := json.Unmarshal(pf.Settings, &s); err != nil {
			return Settings{}, fmt.Errorf("multirename: bad preset: %w", err)
		}
	}
	return s, nil
}

// DeletePreset removes the preset called name. Deleting a preset that does
// not exist is not an error.
func DeletePreset(dir, name string) error {
	err := os.Remove(presetPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ListPresets returns the names of all presets in dir, sorted without regard
// to case. A missing folder simply has no presets.
func ListPresets(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var pf presetFile
		if json.Unmarshal(data, &pf) != nil || pf.Name == "" {
			continue
		}
		names = append(names, pf.Name)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := strings.ToLower(names[i]), strings.ToLower(names[j])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
	return names, nil
}
