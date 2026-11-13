// Package fileutil holds the engines behind Total Commander's small "file
// utility" commands in the Files menu: creating and verifying checksum files
// (cm_CRCCreate, cm_CRCCheck), splitting a big file into parts and joining
// them again (cm_FileSplit, cm_FileCombine), and encoding or decoding a file
// as UUE, XXE, MIME base64 or BinHex text (cm_UUEncode, cm_UUDecode). The
// package knows nothing about the user interface: every operation takes plain
// paths and an options struct, reports progress through a callback, honours a
// context for cancellation and returns a result the dialogs can show. Keeping
// the logic here means it can be tested thoroughly in temporary folders and
// wired to the UI and the control mode later without touching the details.
package fileutil
