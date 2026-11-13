// Tests for the passwd/group and dscl parsers behind the owner and group lists.
package attrs

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParseColonFile(t *testing.T) {
	in := `# comment
root:x:0:0:root:/root:/bin/bash
daemon:*:1:1::/:/usr/sbin/nologin
+nisuser::::::
bad:x:notanumber:0::/:
short:x
:x:5:5::/:
ann:x:1000:1000:Ann:/home/ann:/bin/zsh
`
	got := parseColonFile(strings.NewReader(in))
	want := []Account{{"root", 0}, {"daemon", 1}, {"ann", 1000}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseColonFile = %v, want %v", got, want)
	}
}

func TestParseDsclAndDedup(t *testing.T) {
	out := []byte("_www          70\nann           501\nbroken\nroot          0\nann           501\nodd x\n")
	got := dedupAccounts(parseDscl(out))
	want := []Account{{"_www", 70}, {"ann", 501}, {"root", 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dscl parse = %v, want %v", got, want)
	}
}

func TestUsersGroups(t *testing.T) {
	users, groups := Users(), Groups()
	if runtime.GOOS == "windows" {
		if users != nil || groups != nil {
			t.Error("Windows must not list accounts")
		}
		return
	}
	if len(users) == 0 || len(groups) == 0 {
		t.Fatalf("expected some accounts, got %d users %d groups", len(users), len(groups))
	}
	for i := 1; i < len(users); i++ {
		if users[i-1].Name >= users[i].Name {
			t.Fatalf("users not sorted/deduplicated at %d: %v %v", i, users[i-1], users[i])
		}
	}
}
