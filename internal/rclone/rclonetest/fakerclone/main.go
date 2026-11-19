// Command fakerclone plays rclone for the tests: no network, no real remotes. Every folder under $FAKE_RCLONE_ROOT is
// a remote of that name ("box:docs/a.txt" is $FAKE_RCLONE_ROOT/box/docs/a.txt); arguments that start with "/" (or a
// drive letter on Windows) are local paths. It knows the commands internal/rclone uses (listremotes, lsjson, cat,
// rcat, copyto, moveto, deletefile, rmdir, purge, mkdir) and appends each call's arguments as one tab-separated line
// to $FAKE_RCLONE_LOG when that is set. $FAKE_RCLONE_FAIL names a command that fails with an error message.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	args := os.Args[1:]
	if lp := os.Getenv("FAKE_RCLONE_LOG"); lp != "" {
		if f, err := os.OpenFile(lp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			fmt.Fprintln(f, strings.Join(args, "\t"))
			f.Close()
		}
	}
	var flags = map[string]bool{}
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags[strings.SplitN(a, "=", 2)[0]] = true
			continue
		}
		pos = append(pos, a)
	}
	if len(pos) == 0 {
		die(1, "no command")
	}
	cmd, pos := pos[0], pos[1:]
	if cmd == os.Getenv("FAKE_RCLONE_FAIL") {
		die(1, "Failed to "+cmd+": couldn't connect: 403 Forbidden")
	}
	root := os.Getenv("FAKE_RCLONE_ROOT")
	arg := func(i int) string {
		if i >= len(pos) {
			die(1, "missing argument")
		}
		return resolve(root, pos[i])
	}
	switch cmd {
	case "listremotes":
		es, _ := os.ReadDir(root)
		for _, e := range es {
			if e.IsDir() {
				if flags["--long"] {
					fmt.Printf("%-20s s3\n", e.Name()+":")
				} else {
					fmt.Println(e.Name() + ":")
				}
			}
		}
	case "lsjson":
		p := arg(0)
		fi, err := os.Stat(p)
		if err != nil {
			die(3, "error listing: directory not found")
		}
		if flags["--stat"] {
			out(item(filepath.Base(p), fi))
			return
		}
		if !fi.IsDir() {
			out([]any{item(fi.Name(), fi)})
			return
		}
		es, _ := os.ReadDir(p)
		list := []any{}
		names := []string{}
		for _, e := range es {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, n := range names {
			if fi, err := os.Stat(filepath.Join(p, n)); err == nil {
				list = append(list, item(n, fi))
			}
		}
		out(list)
	case "cat":
		f, err := os.Open(arg(0))
		if err != nil {
			die(3, "directory not found")
		}
		io.Copy(os.Stdout, f)
		f.Close()
	case "rcat":
		p := arg(0)
		os.MkdirAll(filepath.Dir(p), 0o755)
		f, err := os.Create(p)
		if err != nil {
			die(1, err.Error())
		}
		io.Copy(f, os.Stdin)
		f.Close()
	case "copyto", "moveto":
		src, dst := arg(0), arg(1)
		fi, err := os.Stat(src)
		if err != nil {
			die(3, "directory not found")
		}
		os.MkdirAll(filepath.Dir(dst), 0o755)
		if cmd == "moveto" {
			if err := os.Rename(src, dst); err != nil {
				die(1, err.Error())
			}
			return
		}
		b, err := os.ReadFile(src)
		if err != nil {
			die(1, err.Error())
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			die(1, err.Error())
		}
		os.Chtimes(dst, fi.ModTime(), fi.ModTime())
		if flags["--use-json-log"] {
			s, _ := json.Marshal(map[string]any{"level": "info", "msg": "stats", "stats": map[string]any{"bytes": len(b), "totalBytes": len(b)}})
			fmt.Fprintln(os.Stderr, string(s))
		}
	case "deletefile":
		p := arg(0)
		fi, err := os.Stat(p)
		if err != nil {
			die(4, "object not found")
		}
		if fi.IsDir() {
			die(1, "is a directory not a file")
		}
		os.Remove(p)
	case "rmdir":
		if err := os.Remove(arg(0)); err != nil {
			if os.IsNotExist(err) {
				die(3, "directory not found")
			}
			die(1, "directory not empty")
		}
	case "purge":
		p := arg(0)
		if _, err := os.Stat(p); err != nil {
			die(3, "directory not found")
		}
		os.RemoveAll(p)
	case "mkdir":
		if err := os.MkdirAll(arg(0), 0o755); err != nil {
			die(1, err.Error())
		}
	default:
		die(1, "unknown command "+cmd)
	}
}

func resolve(root, a string) string {
	if i := strings.Index(a, ":"); i > 1 && !strings.ContainsAny(a[:i], `/\`) {
		return filepath.Join(root, a[:i], filepath.FromSlash(a[i+1:]))
	}
	return a
}

func item(name string, fi fs.FileInfo) map[string]any {
	m := map[string]any{"Path": name, "Name": name, "Size": fi.Size(), "ModTime": fi.ModTime().UTC().Format(time.RFC3339Nano), "IsDir": fi.IsDir()}
	if fi.IsDir() {
		m["Size"] = -1
	}
	return m
}

func out(v any) {
	b, _ := json.Marshal(v)
	os.Stdout.Write(b)
	fmt.Println()
}

func die(code int, msg string) {
	fmt.Fprintf(os.Stderr, "2026/11/19 03:00:00 ERROR : %s\n", msg)
	os.Exit(code)
}
