// Account lists for the owner and group boxes of the attributes dialog. On
// macOS and Linux a user who may change ownership (usually root, or anyone
// changing the group to one they belong to) picks from a list instead of
// typing a name. The list is built from /etc/passwd and /etc/group, topped up
// with Directory Services on macOS and with the current user's own groups, so
// it is useful even on systems that keep accounts elsewhere (LDAP, NIS). The
// parsers live in this platform-neutral file so they are tested everywhere.
package attrs

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
)

// Account is a user or group with its numeric ID.
type Account struct {
	Name string
	ID   int
}

// Users returns the known user accounts sorted by name. It returns nil on
// Windows, where ownership is not changed from this dialog.
func Users() []Account { return accounts(false) }

// Groups returns the known groups sorted by name; nil on Windows.
func Groups() []Account { return accounts(true) }

func accounts(groups bool) []Account {
	if !platformCaps().Owner {
		return nil
	}
	var list []Account
	file := "/etc/passwd"
	if groups {
		file = "/etc/group"
	}
	if f, err := os.Open(file); err == nil {
		list = append(list, parseColonFile(f)...)
		f.Close()
	}
	list = append(list, extraAccounts(groups)...)
	if u, err := user.Current(); err == nil {
		if !groups {
			if id, err := strconv.Atoi(u.Uid); err == nil {
				list = append(list, Account{Name: u.Username, ID: id})
			}
		} else if gids, err := u.GroupIds(); err == nil {
			for _, g := range gids {
				id, err := strconv.Atoi(g)
				if err != nil {
					continue
				}
				list = append(list, Account{Name: groupName(id), ID: id})
			}
		}
	}
	return dedupAccounts(list)
}

// parseColonFile reads passwd- or group-style lines "name:x:id:...". Comment
// lines and NIS "+"/"-" entries are skipped.
func parseColonFile(r io.Reader) []Account {
	var list []Account
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '+' || line[0] == '-' {
			continue
		}
		f := strings.Split(line, ":")
		if len(f) < 3 || f[0] == "" {
			continue
		}
		id, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		list = append(list, Account{Name: f[0], ID: id})
	}
	return list
}

// parseDscl reads "name   id" lines as printed by "dscl . -list /Users UniqueID".
func parseDscl(out []byte) []Account {
	var list []Account
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 {
			continue
		}
		id, err := strconv.Atoi(f[1])
		if err != nil {
			continue
		}
		list = append(list, Account{Name: f[0], ID: id})
	}
	return list
}

// dedupAccounts removes repeated names (first one wins) and sorts by name.
func dedupAccounts(list []Account) []Account {
	seen := map[string]bool{}
	out := list[:0:0]
	for _, a := range list {
		if a.Name == "" || seen[a.Name] {
			continue
		}
		seen[a.Name] = true
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
