package main

// Actions on duplicate search results (board issue #32, F-862): after a Find files search for duplicate files, the
// page picks one file to keep in every group (newest, oldest or first path) and can then replace the other copies with
// hard links to the kept file. Moving the copies to the trash goes through the ordinary job system (kind "trash") and
// needs nothing here except Missing, which tells the page which results are gone afterwards.
//
// Replacing a file with a hard link throws its own data away, so LinkDuplicates is careful: both files must be plain
// local files of the same size whose contents are byte for byte the same right now (a search by name or size alone
// does not prove that), and the link is made under a temporary name and renamed over the copy (internal/links), so a
// failure never leaves the copy missing. Pairs that are already the same file are reported as done with nothing saved.

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"mormicommander/internal/links"
)

// FindDupPair asks for Dup to become a hard link to Keep.
type FindDupPair struct {
	Keep string `json:"keep"`
	Dup  string `json:"dup"`
}

// FindDupResult is the outcome for one pair. Error is a sentence for the user ("" when the link was made or the files
// were already linked); Saved is the space the link freed, in bytes.
type FindDupResult struct {
	Dup     string `json:"dup"`
	Linked  bool   `json:"linked"`
	Already bool   `json:"already,omitempty"` // the two names were already the same file
	Saved   int64  `json:"saved"`
	Error   string `json:"error,omitempty"`
}

// LinkDuplicates replaces every Dup with a hard link to its Keep, one pair at a time; a pair that can't be done is
// skipped with its reason and the rest go on.
func (f *Find) LinkDuplicates(pairs []FindDupPair) []FindDupResult {
	out := make([]FindDupResult, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, findLinkDup(p))
	}
	return out
}

func findLinkDup(p FindDupPair) FindDupResult {
	res := FindDupResult{Dup: p.Dup}
	fail := func(msg string) FindDupResult { res.Error = msg; return res }
	if p.Keep == "" || p.Dup == "" {
		return fail("A file to keep and a copy are both needed.")
	}
	if strings.Contains(p.Keep, "://") || strings.Contains(p.Dup, "://") {
		return fail("Hard links can only be made for files on this computer.")
	}
	ki, err := os.Lstat(p.Keep)
	if err != nil {
		return fail(toolsErrText(err))
	}
	di, err := os.Lstat(p.Dup)
	if err != nil {
		return fail(toolsErrText(err))
	}
	if !ki.Mode().IsRegular() || !di.Mode().IsRegular() {
		return fail("Only plain files are replaced with hard links (not folders or symbolic links).")
	}
	if os.SameFile(ki, di) {
		res.Linked, res.Already = true, true
		return res
	}
	if ki.Size() != di.Size() {
		return fail("The files are not the same size any more, so the copy was kept.")
	}
	same, err := findSameContent(p.Keep, p.Dup)
	if err != nil {
		return fail("The files could not be compared: " + toolsErrText(err))
	}
	if !same {
		return fail("The contents are different, so the copy was kept.")
	}
	if err := links.CreateHardLink(p.Keep, p.Dup, links.CreateOptions{Overwrite: true}); err != nil {
		return fail(toolsLinkErr(err))
	}
	res.Linked, res.Saved = true, di.Size()
	return res
}

// findSameContent tells whether two files hold the same bytes.
func findSameContent(a, b string) (bool, error) {
	fa, err := os.Open(a)
	if err != nil {
		return false, err
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false, err
	}
	defer fb.Close()
	ra, rb := bufio.NewReaderSize(fa, 1<<16), bufio.NewReaderSize(fb, 1<<16)
	ba, bb := make([]byte, 1<<16), make([]byte, 1<<16)
	for {
		na, ea := io.ReadFull(ra, ba)
		nb, eb := io.ReadFull(rb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false, nil
		}
		aEnd := ea == io.EOF || errors.Is(ea, io.ErrUnexpectedEOF)
		bEnd := eb == io.EOF || errors.Is(eb, io.ErrUnexpectedEOF)
		if ea != nil && !aEnd {
			return false, ea
		}
		if eb != nil && !bEnd {
			return false, eb
		}
		if aEnd || bEnd {
			return aEnd && bEnd, nil
		}
	}
}

// Missing returns the paths that are no longer there (after moving duplicates to the trash, the page drops them from
// the result list). Paths it can't check for another reason are not reported.
func (f *Find) Missing(paths []string) []string {
	out := []string{}
	for _, p := range paths {
		if strings.Contains(p, "://") {
			continue
		}
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			out = append(out, p)
		}
	}
	return out
}
