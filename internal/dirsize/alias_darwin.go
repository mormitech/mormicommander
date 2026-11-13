// On macOS the data volume of the startup disk is mounted at /System/Volumes/Data, and its folders also appear inside
// "/" through firmlinks (/Users, /Applications, /Library, /private and so on are the same folders). A walk that went into
// /System/Volumes/Data as well would count every user file twice, so that folder is skipped as an alias, whatever the
// SameVolume option says, unless the calculation was started inside it (then it is the folder the user asked about).
package dirsize

import "strings"

func isAliasDir(p, root string) bool {
	if p != dataVolume {
		return false
	}
	return !(root == dataVolume || strings.HasPrefix(root, dataVolume+"/"))
}
