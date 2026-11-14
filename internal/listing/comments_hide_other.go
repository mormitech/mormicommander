// This file is the non-Windows side of hiding descript.ion after a save. Unix and macOS have no hidden attribute
// that a plain name like descript.ion could carry (hidden there means a leading dot), so nothing is done.

//go:build !windows

package listing

func hideCommentFile(string) {}
