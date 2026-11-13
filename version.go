package main

import (
	_ "embed"
	"encoding/json"
)

// wails.json is the single source of the version: scripts/build.sh puts it in the macOS Info.plist and the Windows file
// properties, CI names the packages after it, and it is read from here for the About box, --version and the control
// mode's "info". (The name is from the Wails v2 days; Wails v3 itself does not read the file.)
//
//go:embed wails.json
var projectJSON []byte

func version() string {
	var p struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if json.Unmarshal(projectJSON, &p) != nil || p.Info.ProductVersion == "" {
		return "dev"
	}
	return p.Info.ProductVersion
}
