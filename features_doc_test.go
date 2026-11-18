// Tests for docs/FEATURES.md, the Total Commander parity checklist. The file is edited by hand and by many agents, so
// these tests keep it readable by tools: every item line has a known status and an F-NNN ID, no ID is used twice, and
// the items checked by hand for board issue #103 keep the mark that audit gave them and their "audit #103" note. That
// stops a later pass from marking them partly done again just because their ID shows up somewhere in the code; when one
// of them really changes, update its mark, its note and the table below together.
package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

var featureLine = regexp.MustCompile(`^- \[( |~|x|N/A)\] (F-\d{3}) `)

// readFeatures returns the status of every item in docs/FEATURES.md by ID, and the item lines by ID.
func readFeatures(t *testing.T) (map[string]string, map[string]string) {
	t.Helper()
	b, err := os.ReadFile("docs/FEATURES.md")
	if err != nil {
		t.Fatal(err)
	}
	status, lines := map[string]string{}, map[string]string{}
	for n, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		m := featureLine.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("FEATURES.md line %d is not \"- [status] F-NNN ...\": %q", n+1, line)
			continue
		}
		if _, dup := status[m[2]]; dup {
			t.Errorf("FEATURES.md line %d: %s is used twice", n+1, m[2])
		}
		status[m[2]], lines[m[2]] = m[1], line
	}
	return status, lines
}

func TestFeaturesFormat(t *testing.T) {
	status, _ := readFeatures(t)
	if len(status) < 800 {
		t.Fatalf("only %d items read from FEATURES.md; the format check is not seeing the list", len(status))
	}
}

// TestFeaturesAudit103 pins the marks the hand audit of board issue #103 gave.
func TestFeaturesAudit103(t *testing.T) {
	want := map[string]string{
		"F-135": "~", "F-136": "~", "F-352": "~", "F-353": "~", "F-555": "~", "F-466": "~", "F-114": "~",
		"F-672": "~", "F-673": " ", "F-785": "~", "F-819": "~", "F-040": "~", "F-775": "~", "F-700": " ",
		"F-698": "~", "F-828": " ", "F-852": "~", "F-543": "~", "F-545": "~", "F-564": "~", "F-846": "~",
		"F-797": "~",
	}
	status, lines := readFeatures(t)
	for id, st := range want {
		got, ok := status[id]
		switch {
		case !ok:
			t.Errorf("%s is missing from FEATURES.md", id)
		case got == "x":
			// QA checked it in the real app: that supersedes the audit
		case got != st:
			t.Errorf("%s is [%s], the #103 audit found [%s]; update the audit note and this table if it really changed", id, got, st)
		case !strings.Contains(lines[id], "— audit #103: "):
			t.Errorf("%s lost its \"audit #103\" note", id)
		}
	}
}
