package dirsize

// Space is the one place that turns the system's answer into a size and free space (board #102: the drive bar in
// internal/disk asked statfs and GetDiskFreeSpaceEx with copies of this arithmetic). It is the quick question: no
// label, no file system name, no mount table, so the drive bar can ask it for every mount.

// Usage is a volume's size and the bytes this user can still write.
type Usage struct {
	Total     uint64 `json:"total"`
	Available uint64 `json:"available"`
}
