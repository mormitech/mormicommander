// This file saves and loads named sync settings (FEATURES.md F-554).
// Total Commander keeps them in wincmd.ini under [SyncSettings] and offers
// them in a drop-down of the Synchronize dialog; we keep the same data as a
// small JSON file in the app's config folder. A profile remembers the two
// paths, the compare options and the state of the show buttons. The file is
// written to a temporary name and renamed into place, so a crash while
// saving never leaves a broken settings file behind.
package syncdirs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Profile is one saved set of sync settings.
type Profile struct {
	Name    string  `json:"name"`
	Left    string  `json:"left"`
	Right   string  `json:"right"`
	Options Options `json:"options"`
	Show    Show    `json:"show"`
}

type profileFile struct {
	Version  int       `json:"version"`
	Profiles []Profile `json:"profiles"`
}

// LoadProfiles reads the profiles saved in file. A missing file gives an
// empty list and no error.
func LoadProfiles(file string) ([]Profile, error) {
	b, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var pf profileFile
	if err := json.Unmarshal(b, &pf); err != nil {
		return nil, fmt.Errorf("syncdirs: %s: %w", file, err)
	}
	return pf.Profiles, nil
}

// SaveProfiles writes the profiles to file, sorted by name. Names must be
// unique and not empty.
func SaveProfiles(file string, ps []Profile) error {
	seen := map[string]bool{}
	for _, p := range ps {
		if p.Name == "" {
			return errors.New("syncdirs: profile without a name")
		}
		if seen[p.Name] {
			return fmt.Errorf("syncdirs: duplicate profile %q", p.Name)
		}
		seen[p.Name] = true
	}
	sorted := append([]Profile(nil), ps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	b, err := json.MarshalIndent(profileFile{Version: 1, Profiles: sorted}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".syncprofiles-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(append(b, '\n'))
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = replaceFile(tmp.Name(), file)
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
	}
	return werr
}

// PutProfile adds p to the list or replaces the profile with the same name.
func PutProfile(ps []Profile, p Profile) []Profile {
	for i := range ps {
		if ps[i].Name == p.Name {
			out := append([]Profile(nil), ps...)
			out[i] = p
			return out
		}
	}
	return append(append([]Profile(nil), ps...), p)
}

// DeleteProfile removes the profile with the given name, if present.
func DeleteProfile(ps []Profile, name string) []Profile {
	out := make([]Profile, 0, len(ps))
	for _, p := range ps {
		if p.Name != name {
			out = append(out, p)
		}
	}
	return out
}
