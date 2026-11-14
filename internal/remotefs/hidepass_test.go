package remotefs

import (
	"net/url"
	"strings"
	"testing"
)

func TestHidePassword(t *testing.T) {
	for in, want := range map[string]string{
		"ftp://bob:s3cret@host:21/pub": "ftp://bob:****@host:21/pub",
		"ftp://bob@host/pub":           "ftp://bob@host/pub",
		"ftp://host/a:b@c":             "ftp://host/a:b@c",
		"sftp://bob:p@ss:w@host":       "sftp://bob:****@host",
		"no url":                       "no url",
	} {
		if got := HidePassword(in); got != want {
			t.Errorf("HidePassword(%q) = %q, want %q", in, got, want)
		}
	}
	_, err := url.Parse("ftp://bob:s3cret@host:99x/")
	if msg := HideURLErrorPassword(err).Error(); strings.Contains(msg, "s3cret") || !strings.Contains(msg, "bob:****@host") {
		t.Errorf("url error: %s", msg)
	}
}
