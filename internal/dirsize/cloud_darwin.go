// Cloud storage folders hold files whose real home is a server: OneDrive, Google Drive, Dropbox and the like under
// ~/Library/CloudStorage (Apple's File Provider), iCloud Drive under ~/Library/Mobile Documents, and OneDrive's own
// mirror of its files in its group container (an "OneDrive….noindex" folder). Their size is not space on this disk in the
// usual sense (online-only files take none, and OneDrive shows the same files in two places), so the rule is: the
// folder the user started counting decides. Counting a cloud folder (or something inside one) gives the cloud
// content's logical size; counting a folder of the disk (the home folder, ~/Library) leaves the cloud folders inside
// it out, the way folders on other volumes are left out.
package dirsize

import (
	"os"
	"path/filepath"
	"strings"
)

// cloudPlaces gives the folders that are cloud storage as a whole, and OneDrive's group container, whose "OneDrive*.noindex"
// folders are cloud storage too. It is only asked for folders whose name matched, so reading HOME each time is cheap.
func cloudPlaces() (tops []string, oneDriveGC string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, ""
	}
	lib := filepath.Join(home, "Library")
	return []string{filepath.Join(lib, "CloudStorage"), filepath.Join(lib, "Mobile Documents")},
		filepath.Join(lib, "Group Containers", "UBF8T346G9.OneDriveSyncClientSuite")
}

// isCloudTop reports whether p is the top of a cloud storage folder. It is called for every folder of a walk, so
// the cheap name test comes first.
func isCloudTop(p string) bool {
	base := filepath.Base(p)
	noindex := strings.HasPrefix(base, "OneDrive") && strings.HasSuffix(base, ".noindex") // not its .Dbfs….noindex database
	if base != "CloudStorage" && base != "Mobile Documents" && !noindex {
		return false
	}
	tops, oneDriveGC := cloudPlaces()
	if noindex {
		return oneDriveGC != "" && strings.EqualFold(filepath.Dir(p), oneDriveGC)
	}
	for _, t := range tops {
		if strings.EqualFold(p, t) {
			return true
		}
	}
	return false
}

// inCloud reports whether p is a cloud storage folder or inside one.
func inCloud(p string) bool {
	for q := filepath.Clean(p); ; {
		if isCloudTop(q) {
			return true
		}
		up := filepath.Dir(q)
		if up == q {
			return false
		}
		q = up
	}
}
