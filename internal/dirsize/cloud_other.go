//go:build !darwin

package dirsize

// Only macOS keeps cloud storage in folders of its own inside the home folder (see cloud_darwin.go). On Windows an
// online-only OneDrive file takes no clusters, so the size on disk is already right there.
func isCloudTop(p string) bool { return false }
func inCloud(p string) bool    { return false }

// InCloud: see cloud_darwin.go.
func InCloud(p string) bool { return false }
