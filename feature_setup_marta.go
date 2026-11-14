package main

// Coming from Marta (board issue #22, F-NEW-22): when Marta's settings folder is on this computer, the first-run setup
// page offers "Import from Marta", which copies Marta's favourite folders into a "Marta" submenu of the folder hotlist
// (FEATURES.md section 31). The keyboard side is the "Marta-like" preset of core/keypresets.js and the page
// docs/marta.md. This file is the Go half: it finds Marta's folder, reads the favourites (internal/marta) and adds
// them to the hotlist. Marta's own files are only read, never changed; folders the hotlist already has are skipped,
// and nothing happens unless the user presses the button.

import (
	"errors"
	"os"

	"mormicommander/internal/marta"
)

// martaSubmenu is the title of the hotlist submenu the imported favourites go into.
const martaSubmenu = "Marta"

// setupHome and setupHotlist are the home folder and the hotlist the import uses; tests replace them so the real
// Marta folder and the real folders.ini are never used.
var (
	setupHome    = os.UserHomeDir
	setupHotlist = func() *TabsHotlist { return thInstance }
)

// MartaInfo tells the setup page whether Marta's settings are on this computer and which favourites they have.
type MartaInfo struct {
	Found     bool             `json:"found"`     // Marta's settings folder is there
	Dir       string           `json:"dir"`       // that folder
	Favorites []marta.Favorite `json:"favorites"` // its favourite folders (may be empty)
	Error     string           `json:"error"`     // why the favourites could not be read, or ""
}

// Marta looks for Marta's settings (macOS only; elsewhere the answer is "not found"). The demo never shows it.
func (s *Setup) Marta() MartaInfo {
	info := MartaInfo{Favorites: []marta.Favorite{}}
	if setupGOOS != "darwin" || (s.app != nil && s.app.mock != nil) {
		return info
	}
	home, err := setupHome()
	if err != nil || home == "" {
		return info
	}
	info.Dir = marta.Dir(home)
	if info.Found = marta.Found(info.Dir); !info.Found {
		return info
	}
	favs, err := marta.Favorites(info.Dir, home)
	if err != nil {
		info.Error = "Marta's favourites could not be read: " + err.Error()
	}
	if favs != nil {
		info.Favorites = favs
	}
	return info
}

// ImportMarta adds Marta's favourite folders to the "Marta" submenu of the folder hotlist and says how many were
// added (folders already in the hotlist are not added again, so pressing the button twice adds nothing new).
func (s *Setup) ImportMarta() (int, error) {
	info := s.Marta()
	if !info.Found {
		return 0, errors.New("Marta's settings were not found on this computer.")
	}
	if info.Error != "" {
		return 0, errors.New(info.Error)
	}
	th := setupHotlist()
	if th == nil {
		return 0, errors.New("The folder hotlist is not available.")
	}
	folders := make([]THFolder, 0, len(info.Favorites))
	for _, f := range info.Favorites {
		folders = append(folders, THFolder{Title: f.Name, Folder: f.Path})
	}
	return th.ImportFolders(martaSubmenu, folders)
}
