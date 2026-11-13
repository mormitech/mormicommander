// This file says that names on Windows are compared without regard to case.
// NTFS and FAT treat "Photo.JPG" and "photo.jpg" as the same file, so the
// duplicate check and the rename order must fold case, and a rename that only
// changes case is a rename of a file onto itself rather than a conflict.

package multirename

const defaultFoldCase = true
