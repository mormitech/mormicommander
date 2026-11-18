package find

import (
	"context"
	"errors"
	"io"
	"testing"
)

// A cloud placeholder (#30) is left out of content search only when SkipCloud is on, and then its contents are never
// opened (opening it is what would make the system download it). The entry is faked: a real one needs iCloud/OneDrive.
func TestSkipCloudNeverOpensPlaceholders(t *testing.T) {
	for _, skip := range []bool{false, true} {
		o := DefaultOptions()
		o.Content = ContentOptions{Text: "needle"}
		o.SkipCloud = skip
		o.SearchIn = []string{t.TempDir()}
		q, err := Compile(o)
		if err != nil {
			t.Fatal(err)
		}
		opened := false
		e := &entry{path: "/c/a.txt", name: "a.txt", rel: "a.txt", attr: AttrCloud, open: func() (io.ReadCloser, error) {
			opened = true
			return nil, errors.New("would download")
		}}
		ok, err := q.match(context.Background(), e)
		if skip && (ok || err != nil || opened) {
			t.Errorf("SkipCloud on: match=%v err=%v opened=%v, want a silent skip without opening", ok, err, opened)
		}
		if !skip && !opened {
			t.Error("SkipCloud off: the placeholder was not read (today's behaviour must stay)")
		}
		// an ordinary file is read either way
		opened = false
		e.attr = 0
		_, _ = q.match(context.Background(), e)
		if !opened {
			t.Errorf("SkipCloud=%v: an ordinary file was not read", skip)
		}
	}
}
