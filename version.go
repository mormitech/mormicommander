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

// bundleID is the app's reverse-DNS identity (com.mormitech.mormicommander), also from wails.json: scripts/build.sh puts
// it in the macOS Info.plist as CFBundleIdentifier and in the Windows manifest as the assembly name. macOS ties the
// app's permissions (Full Disk Access and the like) and the signature to it. The settings folder does not depend on it
// (see configdir.go). It is empty when wails.json has none.
func bundleID() string {
	var p struct {
		Info struct {
			BundleIdentifier string `json:"bundleIdentifier"`
		} `json:"info"`
	}
	if json.Unmarshal(projectJSON, &p) != nil {
		return ""
	}
	return p.Info.BundleIdentifier
}
