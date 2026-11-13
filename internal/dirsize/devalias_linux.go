// On Linux every mounted file system has its own device number and there is no split system/data layout like macOS
// has, so device numbers are used as they are for "stay on this volume". (Btrfs subvolumes get their own device
// numbers and are therefore treated as separate volumes, the same as "du -x" does.)
package dirsize

func canonicalDev(dev uint64) uint64 { return dev }
