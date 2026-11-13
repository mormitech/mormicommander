// Package disk lists the drives and volumes for the drive bar and the drive popup (Alt+F1 / Alt+F2), and tells the
// free space of the volume a folder is on (the drive info line above each panel). It has no UI dependencies, so it
// cross-compiles and tests on its own. Platform parts live in volumes_<os>.go and usage_*.go.
package disk

// Volume is one drive or mounted volume.
type Volume struct {
	Name  string `json:"name"`  // what the user calls it: "Macintosh HD", "USB stick", "C:"
	Path  string `json:"path"`  // where it is mounted: "/", "/Volumes/USB stick", `C:\`
	Label string `json:"label"` // the volume label where the system has one (Windows), else ""
	Kind  string `json:"kind"`  // "fixed", "removable", "network", "cdrom" or "home"
	Total uint64 `json:"total"` // bytes
	Free  uint64 `json:"free"`  // bytes the user can still write
}

// Usage is the size and free space of the volume a path lives on.
type Usage struct {
	Total uint64 `json:"total"`
	Free  uint64 `json:"free"`
}
