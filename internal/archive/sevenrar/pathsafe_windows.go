// On Windows, entry names have to be made legal for NTFS/FAT: the characters
// < > : " | ? * are forbidden, names may not end in a dot or space, and device
// names such as CON or LPT1 cannot be files. pathsafe.go does the rewriting
// when windowsPaths is true.

package sevenrar

const windowsPaths = true
