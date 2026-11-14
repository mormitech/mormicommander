package keychain

// Tests of the stores without running anything: the commands the macOS and Linux stores would run are recorded by a
// fake runner and answered as the real programs answer, and the Windows blob is checked as data. The one test that
// really runs /usr/bin/security (keychain_darwin_test.go) works on a keychain file of its own.

import (
	"errors"
	"strings"
	"testing"
)

// fakeRun records the calls and answers each with the next result.
type fakeRun struct {
	calls   []fakeCall
	answers []runResult
}

type fakeCall struct {
	program string
	args    []string
	stdin   string
}

func (f *fakeRun) run(program string, args []string, stdin string) (runResult, error) {
	f.calls = append(f.calls, fakeCall{program, append([]string(nil), args...), stdin})
	if len(f.answers) == 0 {
		return runResult{}, nil
	}
	r := f.answers[0]
	f.answers = f.answers[1:]
	return r, nil
}

func TestValidName(t *testing.T) {
	for name, ok := range map[string]bool{"site.4f2a": true, "ai.anthropic": true, "a_b-C9": true, "": false, "a b": false,
		"a\"b": false, "a;b": false, "a\nb": false, "-X": true, strings.Repeat("x", 129): false} {
		if got := ValidName(name) == nil; got != ok {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, ok)
		}
	}
}

func TestFake(t *testing.T) {
	f := NewFake()
	if _, err := f.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty fake: %v", err)
	}
	f.Set("a", "1")
	f.Set("b", "2")
	if s, _ := f.Get("a"); s != "1" {
		t.Fatalf("got %q", s)
	}
	f.Delete("a")
	f.Delete("nothing")
	if n := f.Names(); len(n) != 1 || n[0] != "b" {
		t.Fatalf("names %v", n)
	}
	f.Fail = errors.New("locked")
	if err := f.Set("c", "3"); err == nil {
		t.Fatal("Fail not returned")
	}
	if err := f.Set("bad name", "x"); err == nil {
		t.Fatal("a bad name was taken")
	}
	var _ Keychain = f
}

func TestUnavailableWraps(t *testing.T) {
	err := Unavailable("no secret-tool")
	if !errors.Is(err, ErrUnavailable) || err.Error() != "no secret-tool" {
		t.Fatalf("%v", err)
	}
}

// The secret goes to "security -i" on standard input as hex; the command line is only "-i".
func TestMacSetKeepsSecretOffCommandLine(t *testing.T) {
	f := &fakeRun{}
	k := &macKeychain{service: Service, run: f.run}
	secret := `pä ss"w\ord -U ` + "\n" + `x`
	if err := k.Set("site.abc", secret); err != nil {
		t.Fatal(err)
	}
	c := f.calls[0]
	if c.program != "/usr/bin/security" || strings.Join(c.args, " ") != "-i" {
		t.Fatalf("command %s %v", c.program, c.args)
	}
	want := "add-generic-password -U -s mormiCommander -a site.abc -X 70c3a420737322775c6f7264202d55200a78\n"
	if c.stdin != want {
		t.Fatalf("stdin %q, want %q", c.stdin, want)
	}
	for _, a := range c.args {
		if strings.Contains(a, "ss") {
			t.Fatal("the secret is on the command line")
		}
	}
	// a keychain file (tests only) is the last word, quoted
	k.path = "/tmp/t.keychain-db"
	k.Set("site.abc", "x")
	if got := f.calls[1].stdin; got != "add-generic-password -U -s mormiCommander -a site.abc -X 78 \"/tmp/t.keychain-db\"\n" {
		t.Fatalf("with a keychain file: %q", got)
	}
	k.path = "/tmp/a\"b"
	if err := k.Set("site.abc", "x"); err == nil {
		t.Fatal("a quote in the keychain path was taken")
	}
	// any message from security means the command failed, whatever the exit code
	f.answers = []runResult{{stderr: "add-generic-password: returned -25308\n"}}
	k.path = ""
	if err := k.Set("site.abc", "x"); err == nil || !strings.Contains(err.Error(), "-25308") {
		t.Fatalf("failure not reported: %v", err)
	}
	if err := k.Set("site.abc", ""); err == nil {
		t.Fatal("an empty secret was taken")
	}
	if err := k.Set("site.abc", strings.Repeat("x", MaxSecretLen+1)); err == nil {
		t.Fatal("a too long secret was taken")
	}
	if err := k.Set("bad name", "x"); err == nil {
		t.Fatal("a bad name was taken")
	}
}

func TestMacGetAndDelete(t *testing.T) {
	f := &fakeRun{answers: []runResult{
		{stdout: "keychain: ...\n", stderr: "password: \"ab\"c d\"\n"},
		{stderr: "password: 0x70C3A420737322775C6F7264  \"p\\303\\244 ss\"w\\134ord\"\n"},
		{code: 44, stderr: "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.\n"},
		{code: 51, stderr: "security: User interaction is not allowed.\n"},
		{code: 44},
		{code: 0},
	}}
	k := &macKeychain{service: Service, run: f.run}
	if s, err := k.Get("site.a"); err != nil || s != `ab"c d` {
		t.Fatalf("quoted: %q %v", s, err)
	}
	if s, err := k.Get("site.a"); err != nil || s != `pä ss"w\ord` {
		t.Fatalf("hex: %q %v", s, err)
	}
	if _, err := k.Get("site.a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := k.Get("site.a"); err == nil || !strings.Contains(err.Error(), "User interaction") {
		t.Fatalf("other error: %v", err)
	}
	if strings.Join(f.calls[0].args, " ") != "find-generic-password -s mormiCommander -a site.a -g" || f.calls[0].stdin != "" {
		t.Fatalf("find command %v", f.calls[0].args)
	}
	if err := k.Delete("site.a"); err != nil {
		t.Fatalf("deleting a missing item: %v", err)
	}
	if err := k.Delete("site.a"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.calls[4].args, " ") != "delete-generic-password -s mormiCommander -a site.a" {
		t.Fatalf("delete command %v", f.calls[4].args)
	}
}

func TestMacParsePassword(t *testing.T) {
	for in, want := range map[string]string{"password: \"x\"\n": "x", "password: \n": "", "password:\n": "", "password: 0x6162630A  \"abc\\012\"\n": "abc\n",
		"password: \"  spaced  \"\r\n": "  spaced  "} {
		if got, err := macParsePassword(in); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "nothing here\n", "password: 0xZZ  \"\"\n", "password: odd\n"} {
		if _, err := macParsePassword(in); err == nil {
			t.Errorf("%q: no error", in)
		}
	}
}

// secret-tool gets the secret on standard input; the attributes are service and id.
func TestSecretTool(t *testing.T) {
	f := &fakeRun{answers: []runResult{
		{},                      // store
		{stdout: "s3 cret\n x"}, // lookup
		{code: 1},               // lookup: nothing
		{code: 1, stderr: "secret-tool: Cannot autolaunch D-Bus without X11 $DISPLAY\n"},
		{code: 1}, // clear: nothing
		{code: 1, stderr: "secret-tool: The name org.freedesktop.secrets was not provided\n"},
		{code: 1, stderr: "secret-tool: no service\n"}, // store fails
	}}
	k := &secretToolKeychain{service: Service, path: "/usr/bin/secret-tool", run: f.run}
	if err := k.Set("site.a", "s3 cret"); err != nil {
		t.Fatal(err)
	}
	c := f.calls[0]
	if c.program != "/usr/bin/secret-tool" || strings.Join(c.args, "|") != "store|--label=mormiCommander site.a|service|mormiCommander|id|site.a" || c.stdin != "s3 cret" {
		t.Fatalf("store %+v", c)
	}
	if s, err := k.Get("site.a"); err != nil || s != "s3 cret\n x" {
		t.Fatalf("lookup %q %v", s, err)
	}
	if strings.Join(f.calls[1].args, "|") != "lookup|service|mormiCommander|id|site.a" {
		t.Fatalf("lookup args %v", f.calls[1].args)
	}
	if _, err := k.Get("site.a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := k.Get("site.a"); err == nil || !strings.Contains(err.Error(), "D-Bus") {
		t.Fatalf("no D-Bus: %v", err)
	}
	if err := k.Delete("site.a"); err != nil {
		t.Fatalf("clear of nothing: %v", err)
	}
	if strings.Join(f.calls[4].args, "|") != "clear|service|mormiCommander|id|site.a" {
		t.Fatalf("clear args %v", f.calls[4].args)
	}
	if err := k.Delete("site.a"); err == nil {
		t.Fatal("clear failure not reported")
	}
	if err := k.Set("site.a", "x"); err == nil {
		t.Fatal("store failure not reported")
	}
	for _, c := range f.calls {
		for _, a := range c.args {
			if strings.Contains(a, "s3") {
				t.Fatal("the secret is on the command line")
			}
		}
	}
}

func TestWinBlob(t *testing.T) {
	if winTarget(Service, "site.a") != "mormiCommander:site.a" {
		t.Fatal(winTarget(Service, "site.a"))
	}
	for _, s := range []string{"", "abc", "pä ss", "𝄞 emoji 😀"} {
		b := winBlob(s)
		if got, err := winUnblob(b); err != nil || got != s {
			t.Errorf("%q: %q %v", s, got, err)
		}
	}
	if b := winBlob("Ab"); string(b) != "A\x00b\x00" {
		t.Fatalf("not UTF-16LE: %q", b)
	}
	if _, err := winUnblob([]byte{1, 2, 3}); err == nil {
		t.Fatal("odd blob taken")
	}
}
