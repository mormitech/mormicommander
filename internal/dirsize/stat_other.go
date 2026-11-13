//go:build !darwin && !linux && !windows

// On other systems (BSDs, Plan 9, WebAssembly) the package still builds and counts logical sizes and entries, but it
// knows nothing about blocks, devices or inodes: size on disk equals the logical size, hard links are counted every
// time and "stay on this volume" cannot tell volumes apart. mormiCommander ships for macOS, Windows and Linux only;
// this file just keeps the package portable.
package dirsize

import "io/fs"

type statEnv struct{}

func newStatEnv(root string, quick bool) *statEnv { return &statEnv{} }

func fileStat(path string, fi fs.FileInfo, env *statEnv, wantID, allIDs, follow bool) entryStat {
	return entryStat{size: fi.Size(), alloc: fi.Size()}
}

func dirDev(path string, fi fs.FileInfo, env *statEnv) (uint64, bool) { return 0, false }

func dirID(path string, fi fs.FileInfo, env *statEnv) (fileID, bool) { return fileID{}, false }
