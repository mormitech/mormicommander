package fileops

// Before a copy starts, TC checks that the target volume has room for it and warns when it hasn't, instead of
// stopping with "disk full" halfway. The check counts the bytes of the files that will really be written (a move on
// the same volume writes none) and asks Continue / Cancel when they don't fit; the user may know better (files that
// will overwrite others free their space). It can be turned off per job (Request.IgnoreSpace).

import (
	"mormicommander/internal/disk"
)

// freeSpace reports the bytes free on the volume of dir; the tests replace it.
var freeSpace = func(dir string) (int64, bool) {
	u, err := disk.UsageOf(existingAncestor(dir))
	if err != nil {
		return 0, false
	}
	return int64(u.Free), true
}

// checkSpace returns false when the user cancelled because the job doesn't fit.
func (j *job) checkSpace(need int64) bool {
	if j.req.IgnoreSpace || need <= 0 {
		return true
	}
	free, ok := freeSpace(j.req.Target)
	if !ok || free >= need {
		return true
	}
	a := j.ask(Conflict{Type: AskNoSpace, Target: j.req.Target, Need: need, Free: free,
		Choices: []Choice{ChoiceYes, ChoiceCancel}})
	return a.Choice == ChoiceYes
}
