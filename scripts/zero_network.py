#!/usr/bin/env python3
"""Zero-network test of the built app (board issue #14, FEATURES.md F-875). Linux only.

Starts the built mormiCommander under a virtual display (xvfb-run) and a private D-Bus session, in control mode, with a
temporary HOME and settings folder, and drives a scripted session through control commands (scripts/mc_ctl.py): browse
folders, view a text file, a Markdown file and an HTML page full of remote pictures, styles, scripts and links, quick
view, thumbnails of a folder of pictures, copy files, Find files by name and by text, open and close the Options
dialog (its AI page too) and the Ask AI dialog (without starting a run).

Two independent watchers record every network attempt of the app and of every process it starts (GTK, and WebKitGTK's
WebKitWebProcess and WebKitNetworkProcess, which do the web view's loading):

1. strace -f on the whole process tree, for the system calls that reach another machine: connect, sendto, sendmsg,
   sendmmsg (and execve, to name the processes). Every internet address that is not loopback is a finding, and so is
   anything sent to port 53 (a DNS lookup, even to the local resolver on 127.0.0.53) or a connection to
   systemd-resolved's socket. Unix sockets (D-Bus, X11, Wayland, WebKit's own IPC) and netlink are allowed.
2. ss -tunap snapshots every half second: any TCP or UDP socket of a process of the tree that talks to another machine
   or to port 53, or that listens on anything but loopback.

The run fails with a report of the offending connections; it also fails when the app did not start, when the web view's
process was never seen (so the watch would not have covered it), or when a core step of the session failed.

    scripts/zero_network.py [--out DIR] [--keep] path/to/mormicommander     the session
    scripts/zero_network.py --selftest [--out DIR]                          proves the watchers see a planted connection

scripts/zero_network.sh is the wrapper CI calls (it checks the tools and sets the virtual display's environment). The
parsing functions are OS-independent and unit-tested by scripts/test_zero_network.py (python3 -m unittest).
"""
import argparse
import ipaddress
import json
import os
import re
import shutil
import struct
import subprocess
import sys
import tempfile
import threading
import time
import zlib

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mc_ctl import MC, MCError  # noqa: E402

# A TEST-NET-1 address (RFC 5737): never assigned, so the self-test's connect attempt reaches nobody.
SELFTEST_ADDR = "192.0.2.1"
SELFTEST_PORT = 9
TRACED = "execve,connect,sendto,sendmsg,sendmmsg"

# ---------------------------------------------------------------------------------------------------------------------
# Parsing (OS-independent)
# ---------------------------------------------------------------------------------------------------------------------

# "1234  12:00:00.123456 connect(...)", optionally "1234<comm>" (strace -Y) and without the time.
_LINE = re.compile(r'^(?P<pid>\d+)(?:<(?P<comm>[^>]*)>)?\s+(?:(?P<time>\d\d:\d\d:\d\d(?:\.\d+)?)\s+)?(?P<rest>.*)$')
_CALL = re.compile(r'^(?P<call>execve|connect|sendto|sendmsg|sendmmsg)\((?P<args>.*)$')
_RESUMED = re.compile(r'^<\.\.\. (?P<call>\w+) resumed>(?P<tail>.*)$')
_RESULT = re.compile(r'\)\s*=\s*(?P<ret>-?\d+|\?)(?:\s+(?P<err>E[A-Z0-9]+))?')
_SOCKADDR = re.compile(r'sa_family=(?P<fam>AF_\w+)(?P<body>[^{}]*)')
_V4 = re.compile(r'sin_port=htons\((?P<port>\d+)\).*?sin_addr=inet_addr\("(?P<addr>[^"]*)"\)')
_V6 = re.compile(r'sin6_port=htons\((?P<port>\d+)\).*?inet_pton\(AF_INET6,\s*"(?P<addr>[^"]*)"')
_UNIX = re.compile(r'sun_path=(?P<abstract>@?)"(?P<path>(?:[^"\\]|\\.)*)"')
_EXECVE = re.compile(r'^"(?P<exe>(?:[^"\\]|\\.)*)"')

# Unix sockets that are a host-name lookup in disguise (glibc's nss-resolve talks to systemd-resolved through it).
DNS_UNIX_SOCKETS = ("io.systemd.Resolve",)
ALLOWED_FAMILIES = {"AF_UNIX", "AF_LOCAL", "AF_NETLINK", "AF_UNSPEC"}


def is_local_address(addr):
    """True for loopback (127.0.0.0/8, ::1, ::ffff:127.x) and the unspecified address (which Linux treats as local)."""
    a = addr.strip().strip("[]").split("%", 1)[0]
    if a in ("*", ""):
        return True
    try:
        ip = ipaddress.ip_address(a)
    except ValueError:
        return False
    if isinstance(ip, ipaddress.IPv6Address) and ip.ipv4_mapped:
        ip = ip.ipv4_mapped
    return ip.is_loopback or ip.is_unspecified


def parse_sockaddrs(args):
    """Every socket address in a call's argument text: [{family, addr, port}] (port None where it has none)."""
    out = []
    for m in _SOCKADDR.finditer(args):
        fam, body = m.group("fam"), m.group("body")
        entry = {"family": fam, "addr": "", "port": None}
        if fam == "AF_INET":
            v = _V4.search(body)
            if v:
                entry.update(addr=v.group("addr"), port=int(v.group("port")))
        elif fam == "AF_INET6":
            v = _V6.search(body)
            if v:
                entry.update(addr=v.group("addr"), port=int(v.group("port")))
        elif fam in ("AF_UNIX", "AF_LOCAL"):
            v = _UNIX.search(body)
            if v:
                entry["addr"] = v.group("abstract") + v.group("path")
        out.append(entry)
    return out


def classify(family, addr, port):
    """None when the destination is allowed, else the reason it is a finding."""
    if family in ("AF_UNIX", "AF_LOCAL"):
        if any(s in addr for s in DNS_UNIX_SOCKETS):
            return "host-name lookup through systemd-resolved"
        return None
    if family in ALLOWED_FAMILIES:
        return None
    if family in ("AF_INET", "AF_INET6"):
        if port == 53:
            return "DNS lookup (port 53)"
        if not addr:
            return "internet address strace did not decode"
        if is_local_address(addr):
            return None
        return "connection to another machine"
    return "unexpected socket family %s" % family


def parse_strace(lines):
    """Reads strace -f output. Returns (events, procs): events are the decoded socket destinations of connect, sendto,
    sendmsg and sendmmsg ({pid, comm, time, call, family, addr, port, result, finding}); procs maps a pid to the
    program it ran (from successful execve calls; a call split by another process's line is joined back)."""
    events, procs, pending_exec = [], {}, {}
    for raw in lines:
        m = _LINE.match(raw.rstrip("\n"))
        if not m:
            continue
        pid, comm, when, rest = int(m.group("pid")), m.group("comm") or "", m.group("time") or "", m.group("rest")
        r = _RESUMED.match(rest)
        if r:
            if r.group("call") == "execve" and pid in pending_exec:
                res = _RESULT.search(r.group("tail"))
                exe = pending_exec.pop(pid)
                if res and res.group("ret") == "0":
                    procs[pid] = exe
            continue
        c = _CALL.match(rest)
        if not c:
            continue
        call, args = c.group("call"), c.group("args")
        res = _RESULT.search(args)
        unfinished = args.rstrip().endswith("<unfinished ...>")
        result = "unfinished" if unfinished or not res else (res.group("ret") + (" " + res.group("err") if res.group("err") else ""))
        if call == "execve":
            e = _EXECVE.match(args)
            if not e:
                continue
            if unfinished:
                pending_exec[pid] = e.group("exe")
            elif res and res.group("ret") == "0":
                procs[pid] = e.group("exe")
            continue
        for sa in parse_sockaddrs(args):
            events.append({"pid": pid, "comm": comm, "time": when, "call": call, "family": sa["family"], "addr": sa["addr"],
                           "port": sa["port"], "result": result, "finding": classify(sa["family"], sa["addr"], sa["port"])})
    return events, procs


def split_hostport(s):
    """ss's "127.0.0.1:53", "[::1]:8080", "*:*", "127.0.0.53%lo:53", "[fe80::1]%eth0:22" → (host, port or "*")."""
    host, _, port = s.rpartition(":")
    if not host:
        return s, "*"
    host = host.split("%", 1)[0].strip("[]")
    return host, port


_SS_USERS = re.compile(r'\("(?P<comm>[^"]*)",pid=(?P<pid>\d+),fd=(?P<fd>\d+)\)')


def parse_ss(text):
    """Reads `ss -tunapH` (no header). Returns [{netid, state, local, peer, procs: [(comm, pid)]}]."""
    out = []
    for line in text.splitlines():
        parts = line.split()
        if len(parts) < 6 or parts[0] not in ("tcp", "udp", "raw"):
            continue
        procs = [(m.group("comm"), int(m.group("pid"))) for m in _SS_USERS.finditer(line)]
        out.append({"netid": parts[0], "state": parts[1], "local": parts[4], "peer": parts[5], "procs": procs})
    return out


def classify_ss(entry):
    """None when the socket is allowed, else why it is a finding. A connected socket may only talk to loopback (and never
    to port 53); a listening TCP or unconnected UDP socket may only be bound to loopback."""
    lhost, _ = split_hostport(entry["local"])
    phost, pport = split_hostport(entry["peer"])
    if pport == "53":
        return "DNS lookup (port 53)"
    if pport != "*":
        return None if is_local_address(phost) else "connection to another machine"
    if lhost in ("*", "") or is_unspecified(lhost) or not is_local_address(lhost):
        return "socket open to every network (%s)" % entry["local"]
    return None


def is_unspecified(host):
    try:
        return ipaddress.ip_address(host.strip("[]")).is_unspecified
    except ValueError:
        return False


def ss_findings(entries, pids):
    """The entries of the given processes that are findings: [(entry, reason, comm, pid)]."""
    out = []
    for e in entries:
        mine = [(c, p) for c, p in e["procs"] if p in pids]
        if not mine:
            continue
        why = classify_ss(e)
        if why:
            out.append((e, why) + mine[0])
    return out


def proc_tree(root, stat_reader=None):
    """The pid root and all its descendants, from /proc/<pid>/stat (stat_reader: {pid: stat text}, for tests)."""
    stats = stat_reader if stat_reader is not None else _read_stats()
    children = {}
    for pid, text in stats.items():
        try:
            ppid = int(text.rsplit(")", 1)[1].split()[1])
        except (IndexError, ValueError):
            continue
        children.setdefault(ppid, []).append(pid)
    seen, todo = set(), [root]
    while todo:
        p = todo.pop()
        if p in seen:
            continue
        seen.add(p)
        todo.extend(children.get(p, []))
    return seen


def _read_stats():
    out = {}
    for d in os.listdir("/proc"):
        if d.isdigit():
            try:
                with open("/proc/%s/stat" % d) as f:
                    out[int(d)] = f.read()
            except OSError:
                pass
            # threads have their own ids too (ss names the thread group's id, so only processes are needed)
    return out


def describe_event(e, procs):
    who = procs.get(e["pid"]) or e["comm"] or "?"
    dest = e["addr"] + (":%s" % e["port"] if e["port"] is not None else "")
    t = (e["time"] + " ") if e["time"] else ""
    return "%spid %d (%s) %s %s %s -> %s: %s" % (t, e["pid"], os.path.basename(who), e["call"], e["family"], dest, e["result"], e["finding"])


# ---------------------------------------------------------------------------------------------------------------------
# Watching a process tree (Linux)
# ---------------------------------------------------------------------------------------------------------------------

def strace_flags():
    """strace's options, with --seccomp-bpf (only the traced calls stop the process: much faster) and -Y (command names
    next to pids) when this strace has them."""
    base = ["-f", "-qq", "-tt", "-s", "64", "-e", "trace=" + TRACED, "-e", "signal=none"]
    extra = []
    for flag in ("--seccomp-bpf", "-Y"):
        probe = ["strace"] + base + extra + [flag, "-o", os.devnull, "--", "true"]
        if subprocess.run(probe, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            extra.append(flag)
    return base + extra


class SSWatcher(threading.Thread):
    """Takes `ss -tunapH` snapshots every interval seconds and keeps the findings for the tree under root()."""

    def __init__(self, root, log_path, interval=0.5):
        super().__init__(daemon=True)
        self.root, self.interval, self.log_path = root, interval, log_path
        self.findings, self.snapshots, self.sockets_seen = [], 0, set()
        self.stop_event = threading.Event()

    def run(self):
        with open(self.log_path, "w") as log:
            while not self.stop_event.is_set():
                self.sample(log)
                self.stop_event.wait(self.interval)
            self.sample(log)

    def sample(self, log):
        r = self.root()
        if not r:
            return
        try:
            text = subprocess.run(["ss", "-tunapH"], capture_output=True, text=True, timeout=10).stdout
        except (OSError, subprocess.TimeoutExpired):
            return
        pids = proc_tree(r)
        self.snapshots += 1
        entries = parse_ss(text)
        for e in entries:
            if any(p in pids for _, p in e["procs"]):
                key = (e["netid"], e["local"], e["peer"], e["state"])
                if key not in self.sockets_seen:
                    self.sockets_seen.add(key)
                    log.write("%s %s %s %s -> %s %s\n" % (time.strftime("%H:%M:%S"), e["netid"], e["state"], e["local"], e["peer"], e["procs"]))
        for e, why, comm, pid in ss_findings(entries, pids):
            f = "%s %s %s -> %s (pid %d, %s): %s" % (e["netid"], e["state"], e["local"], e["peer"], pid, comm, why)
            if f not in self.findings:
                self.findings.append(f)

    def stop(self):
        self.stop_event.set()
        self.join(15)


# ---------------------------------------------------------------------------------------------------------------------
# Test files
# ---------------------------------------------------------------------------------------------------------------------

def png(w, h, rgb):
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
    row = b"\x00" + bytes(rgb) * w
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + \
        chunk(b"IDAT", zlib.compress(row * h)) + chunk(b"IEND", b"")


REMOTE_MD = """# Remote-heavy README

![badge](https://img.shields.io/badge/build-passing-green.svg)
![logo](https://example.com/logo.png "logo")
<img src="https://example.org/pixel.gif" width="1" height="1">
<link rel="stylesheet" href="https://example.com/style.css">
<iframe src="https://example.net/frame.html"></iframe>
<script src="https://example.com/track.js"></script>

See [the project site](https://example.com/) and <https://example.org/autolink>.

[ref]: https://example.net/reference
"""

REMOTE_HTML = """<!doctype html>
<html><head>
<meta charset="utf-8">
<link rel="dns-prefetch" href="//prefetch.example.com">
<link rel="preconnect" href="https://preconnect.example.com">
<link rel="prefetch" href="https://example.com/next.html">
<link rel="stylesheet" href="https://example.com/a.css">
<style>@import url("https://example.com/b.css"); @font-face { font-family: X; src: url(https://example.com/x.woff2); }
body { background: url(https://example.com/bg.png); font-family: X; }</style>
<script src="https://example.com/c.js"></script>
<script>fetch("https://example.com/beacon"); new Image().src = "https://example.com/i.gif";</script>
<meta http-equiv="refresh" content="5; url=https://example.com/refresh">
</head><body>
<img src="https://example.com/d.png" srcset="https://example.com/d2.png 2x">
<picture><source srcset="https://example.com/e.webp"><img src="https://example.com/e.png"></picture>
<video src="https://example.com/v.mp4" poster="https://example.com/p.png"></video>
<audio src="https://example.com/a.mp3"></audio>
<iframe src="https://example.com/frame.html"></iframe>
<object data="https://example.com/o.swf"></object><embed src="https://example.com/e.swf">
<svg><image href="https://example.com/s.png"/></svg>
<a href="https://example.com/link">a link</a>
<form action="https://example.com/post"><input type="image" src="https://example.com/btn.png"></form>
</body></html>
"""

REMOTE_SVG = """<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="64" height="64">
<rect width="64" height="64" fill="#3a7"/>
<image xlink:href="https://example.com/svg.png" width="64" height="64"/>
</svg>
"""


def make_tree(root):
    """The folders of the session: real files in a temporary folder (no --demo)."""
    left, right = os.path.join(root, "left"), os.path.join(root, "right")
    docs, images, sub = os.path.join(left, "docs"), os.path.join(left, "images"), os.path.join(left, "docs", "sub")
    for d in (docs, images, sub, right):
        os.makedirs(d, exist_ok=True)
    files = {
        os.path.join(docs, "notes.txt"): b"Plain text notes.\nhttps://example.com/in-text\n" * 50,
        os.path.join(docs, "readme.md"): REMOTE_MD.encode(),
        os.path.join(docs, "page.html"): REMOTE_HTML.encode(),
        os.path.join(sub, "deep.txt"): b"deep file, search me: example\n",
        os.path.join(sub, "other.css"): b"@import url(https://example.com/x.css);\n",
        os.path.join(left, "top.log"): b"x" * 4000,
        os.path.join(images, "logo.svg"): REMOTE_SVG.encode(),
    }
    for i, rgb in enumerate([(200, 40, 40), (40, 200, 40), (40, 40, 200), (220, 220, 40)]):
        files[os.path.join(images, "pic%d.png" % i)] = png(96, 64, rgb)
    for p, data in files.items():
        with open(p, "wb") as f:
            f.write(data)
    return left, right, docs, images


# ---------------------------------------------------------------------------------------------------------------------
# The session
# ---------------------------------------------------------------------------------------------------------------------

class Session:
    """Runs the steps; a step that fails is reported and the next one goes on (after closing what it left open)."""

    CORE = ("browse", "view text", "view markdown", "view html")

    def __init__(self, mc):
        self.mc, self.steps = mc, []

    def step(self, name, fn):
        t = time.time()
        try:
            note = fn() or ""
            self.steps.append({"step": name, "ok": True, "note": note, "s": round(time.time() - t, 1)})
            print("  ok    %-22s %s" % (name, note), flush=True)
        except Exception as e:  # noqa: BLE001
            self.steps.append({"step": name, "ok": False, "note": str(e), "s": round(time.time() - t, 1)})
            print("  FAIL  %-22s %s" % (name, e), flush=True)
            self.recover()

    def idle(self, timeout_ms=20000):
        """Control mode's own wait: until the app is idle (folders read, the page painted)."""
        self.mc._req("/cmd", {"cmd": "idle", "ms": 100, "timeout": timeout_ms}, timeout=timeout_ms / 1000 + 10)

    def wait_for(self, what, fn, timeout=20):
        end, last = time.time() + timeout, None
        while time.time() < end:
            try:
                v = fn()
                if v:
                    return v
            except MCError as e:
                last = e
            time.sleep(0.25)
        raise MCError("timed out waiting for %s%s" % (what, " (%s)" % last if last else ""))

    def dialog(self):
        try:
            return self.mc.dialog()
        except MCError:
            return None

    def close_dialog(self, *buttons):
        for b in list(buttons) + ["Esc", "Esc", "Esc"]:
            d = self.dialog()
            if not d:
                return
            try:
                self.mc.answer(b)
            except MCError:
                pass
            time.sleep(0.3)
        if self.dialog():
            raise MCError("the dialog %r did not close" % (self.dialog() or {}).get("kind"))

    def listers(self):
        try:
            ws = self.mc.cmd("windows")["windows"]
        except MCError:
            return []
        return [w["name"] for w in ws if w.get("kind") == "lister" and w.get("visible") and not w.get("spare")]

    def close_viewers(self):
        for name in self.listers():
            try:
                self.mc.cmd("viewer_close", window=name)
            except MCError:
                pass
        d = self.dialog()
        if d and d.get("kind") == "lister":
            self.close_dialog("Esc")

    def recover(self):
        try:
            self.close_viewers()
            self.close_dialog()
            self.mc.cmd("release")
        except Exception:  # noqa: BLE001
            pass

    # ---- steps ----
    def view(self, path):
        """Opens a file in the Lister (F3's command with a path), waits until it shows, closes it."""
        self.mc.run("cm_List", path=path)

        def shown():
            if self.listers():
                v = self.mc.cmd("viewer")
                lst = v.get("lister") or {}
                if v.get("open") and (lst.get("rows") or lst.get("docShown") or lst.get("mediaShown")):
                    return {"mode": lst.get("mode"), "docShown": lst.get("docShown"), "where": v.get("window")}
                return None
            d = self.dialog()
            if d and d.get("kind") == "lister":
                lst = self.mc.cmd("lister").get("lister") or {}
                return {"mode": lst.get("mode"), "docShown": lst.get("docShown"), "where": "page"}
            return None
        got = self.wait_for("the Lister to show %s" % os.path.basename(path), shown, 25)
        time.sleep(1.5)  # time for anything the rendered page would load
        self.close_viewers()
        self.wait_for("the Lister to close", lambda: not self.listers() and (self.dialog() or {}).get("kind") != "lister", 15)
        return "mode %s%s, in %s" % (got["mode"], ", rendered" if got["docShown"] else "", got["where"])


def drive(mc, tree):
    left, right, docs, images = tree
    s = Session(mc)

    def browse():
        mc.window(1200, 760)
        mc.focus("left")
        mc.cursor("left", name="docs")
        mc.key("Enter")
        s.wait_for("the docs folder", lambda: mc.panel("left")["path"].rstrip("/").endswith("docs"), 15)
        mc.cursor("left", name="sub")
        mc.key("Enter")
        s.wait_for("the sub folder", lambda: mc.panel("left")["path"].rstrip("/").endswith("sub"), 15)
        mc.run("cm_GoToParent")
        mc.run("cm_GoToParent")
        mc.cd("left", images)
        mc.key("Down", "Down", "End", "Home")
        mc.key("Tab")
        mc.cd("right", right)
        mc.key("Tab")
        mc.cd("left", left)
        return "%d entries in the left panel" % mc.panel("left")["count"]

    def quick_view():
        mc.cd("left", docs)
        mc.focus("left")
        mc.cursor("left", name="readme.md")
        mc.run("cm_SrcQuickView")
        s.idle()
        time.sleep(1.5)
        mc.cursor("left", name="page.html")
        s.idle()
        time.sleep(1.5)
        mc.cursor("left", name="notes.txt")
        s.idle()
        q = mc.cmd("lister").get("quickView") or {}
        mc.run("cm_SrcQuickView")
        s.idle()
        return "quick view showed %s" % (q.get("name") or q.get("path") or "?")

    def thumbnails():
        mc.cd("left", images)
        mc.focus("left")
        mc.run("cm_SrcThumbs")
        s.wait_for("thumbnails view", lambda: mc.cmd("views")["left"]["mode"] == "thumbs", 15)
        try:
            s.wait_for("thumbnails drawn", lambda: [r for r in mc.cmd("views")["left"].get("rows", []) if r.get("thumb")], 20)
            note = "thumbnails drawn"
        except MCError:
            note = "view switched; thumbnails not reported drawn"
        time.sleep(1)
        mc.run("cm_SrcLong")
        return note

    def copy():
        mc.cd("left", left)
        mc.cd("right", right)
        mc.focus("left")
        mc.cursor("left", name="docs")
        mc.key("F5")
        d = s.wait_for("the copy dialog", lambda: (s.dialog() or {}).get("kind") in ("copy",) and s.dialog(), 15)
        mc.answer("OK")
        target = os.path.join(right, "docs", "readme.md")
        s.wait_for("the copied files", lambda: os.path.exists(target) and not s.dialog(), 30)
        return "copied docs/ (%s)" % d.get("title", "")

    def find():
        mc.cd("left", left)
        mc.focus("left")
        mc.run("cm_SearchFor", fields={"ff-for": "*.md", "ff-in": left}, start=True)
        r1 = mc.cmd("find", wait=True, timeout=90)
        s.close_dialog("Esc")
        mc.run("cm_SearchFor", fields={"ff-for": "*", "ff-in": left, "ff-texton": True, "ff-text": "example"}, start=True)
        r2 = mc.cmd("find", wait=True, timeout=90)
        s.close_dialog("Esc")
        if not r1.get("count"):
            raise MCError("the name search found nothing: %s" % r1.get("status"))
        return "by name %s, by text %s" % (r1.get("count"), r2.get("count"))

    def settings():
        mc.run("cm_Config")
        s.wait_for("the Options dialog", lambda: (s.dialog() or {}).get("kind") == "config", 15)
        time.sleep(1)
        s.close_dialog("Cancel")
        mc.run("cm_Config", page="AI")
        s.wait_for("the Options dialog on its AI page", lambda: (s.dialog() or {}).get("kind") == "config", 15)
        time.sleep(1.5)
        s.close_dialog("Cancel")
        return "opened and cancelled twice (Layout, AI)"

    def ask_ai():
        mc.run("cm_AIAsk")
        s.wait_for("the Ask AI dialog", lambda: (s.dialog() or {}).get("kind") == "ai-ask", 15)
        time.sleep(2)  # it asks Go which providers can be used: nothing is sent anywhere
        s.close_dialog("Close")
        return "opened and closed, no run started"

    s.step("browse", browse)
    s.step("view text", lambda: s.view(os.path.join(docs, "notes.txt")))
    s.step("view markdown", lambda: s.view(os.path.join(docs, "readme.md")))
    s.step("view html", lambda: s.view(os.path.join(docs, "page.html")))
    s.step("view svg", lambda: s.view(os.path.join(images, "logo.svg")))
    s.step("quick view", quick_view)
    s.step("thumbnails", thumbnails)
    s.step("copy", copy)
    s.step("find files", find)
    s.step("options dialog", settings)
    s.step("ask ai dialog", ask_ai)
    s.step("linger", lambda: (time.sleep(5), "5 s idle at the end")[1])
    return s.steps


def app_env(root, config):
    home = os.path.join(root, "home")
    env = {k: v for k, v in os.environ.items() if not k.lower().endswith("_proxy")}
    env.update({
        "HOME": home,
        "XDG_CONFIG_HOME": os.path.join(home, ".config"),
        "XDG_CACHE_HOME": os.path.join(home, ".cache"),
        "XDG_DATA_HOME": os.path.join(home, ".local", "share"),
        "XDG_STATE_HOME": os.path.join(home, ".local", "state"),
        "XDG_RUNTIME_DIR": os.path.join(root, "run"),
        "MORMICOMMANDER_CONFIG_DIR": config,
        "MORMICOMMANDER_REEXEC": "1",
        "MORMICOMMANDER_ZOXIDE_DB": "none",
        "MORMICOMMANDER_NO_EDITOR": "1",
    })
    for d in ("XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR"):
        os.makedirs(env[d], mode=0o700, exist_ok=True)
    return env


def start_app(app, root, tree, out, attempt):
    """Starts the app under dbus-run-session, xvfb-run and strace; returns (mc, proc, strace_log, app_pid)."""
    left, right = tree[0], tree[1]
    config = os.path.join(root, "config-%d" % attempt)
    os.makedirs(config, exist_ok=True)
    control = os.path.join(root, "control-%d.json" % attempt)
    trace_log = os.path.join(out, "strace-%d.log" % attempt)
    argv = ["dbus-run-session", "--", "xvfb-run", "-a", "-s", "-screen 0 1400x900x24",
            "strace"] + strace_flags() + ["-o", trace_log, "--", os.path.abspath(app),
                                          "--control=0", "--control-file", control, "-L=" + left, "-R=" + right]
    with open(os.path.join(out, "argv-%d.txt" % attempt), "w") as f:
        f.write(" ".join(argv) + "\n")
    log = open(os.path.join(out, "app-%d.log" % attempt), "w")
    proc = subprocess.Popen(argv, stdout=log, stderr=subprocess.STDOUT, env=app_env(root, config), start_new_session=True)
    end, last = time.time() + 90, None
    while time.time() < end:
        if proc.poll() is not None:
            raise MCError("the app exited with code %s before answering (see app-%d.log)" % (proc.returncode, attempt))
        try:
            mc = MC(control, proc)
            info = mc.info()
            mc.wait_ready(max(1, end - time.time()))
            return mc, proc, trace_log, int(info.get("pid") or 0)
        except (OSError, KeyError, ValueError, MCError) as e:
            last = e
            time.sleep(0.5)
    stop_tree(proc)
    raise MCError("the app did not answer in 90 s (%s)" % last)


def stop_tree(proc, timeout=30):
    if proc.poll() is None:
        try:
            proc.wait(timeout)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, 15)
            try:
                proc.wait(10)
            except subprocess.TimeoutExpired:
                os.killpg(proc.pid, 9)
                proc.wait(10)


def report(events, procs, ss, steps, out, extra_problems):
    findings = [e for e in events if e["finding"]]
    fams = {}
    for e in events:
        k = e["family"] if e["family"] not in ("AF_INET", "AF_INET6") else e["family"] + (" loopback" if not e["finding"] else "")
        fams[k] = fams.get(k, 0) + 1
    progs = sorted({os.path.basename(p) for p in procs.values()})
    lines = ["== zero-network report",
             "processes watched (execve): %s" % (", ".join(progs) or "none recorded"),
             "socket destinations seen by strace: %s" % (", ".join("%s %d" % kv for kv in sorted(fams.items())) or "none"),
             "ss snapshots: %d, sockets of the tree seen: %d" % (ss.snapshots, len(ss.sockets_seen))]
    if steps is not None:
        lines.append("session: %d of %d steps passed" % (sum(1 for s in steps if s["ok"]), len(steps)))
        lines += ["  %s %s %s" % ("ok  " if s["ok"] else "FAIL", s["step"], s["note"]) for s in steps]
    if findings:
        lines.append("OUTBOUND ATTEMPTS (strace):")
        lines += ["  " + describe_event(e, procs) for e in findings]
    if ss.findings:
        lines.append("OUTBOUND SOCKETS (ss):")
        lines += ["  " + f for f in ss.findings]
    lines += ["PROBLEM: " + p for p in extra_problems]
    text = "\n".join(lines) + "\n"
    print(text, flush=True)
    with open(os.path.join(out, "report.txt"), "w") as f:
        f.write(text)
    with open(os.path.join(out, "report.json"), "w") as f:
        json.dump({"findings": findings, "ss_findings": ss.findings, "procs": procs, "steps": steps, "problems": extra_problems,
                   "families": fams}, f, indent=1)
    return findings


def run_session(app, out, keep):
    root = os.path.realpath(tempfile.mkdtemp(prefix="mc-zero-network-"))
    tree = make_tree(os.path.join(root, "files"))
    mc = proc = None
    app_pid = 0
    # ss watches the whole tree under dbus-run-session: the app and its processes, and also the services the private
    # session bus starts for it (strace covers only the app's own tree; see docs/privacy.md)
    ss = SSWatcher(lambda: proc.pid if proc is not None and proc.poll() is None else 0, os.path.join(out, "ss.log"))
    problems, steps, trace_log = [], None, None
    try:
        for attempt in range(1, 4):
            try:
                print("== starting the app (attempt %d)" % attempt, flush=True)
                mc, proc, trace_log, app_pid = start_app(app, root, tree, out, attempt)
                break
            except MCError as e:
                print("  " + str(e), flush=True)
                if attempt == 3:
                    problems.append("the app did not start: %s" % e)
        if mc:
            ss.start()
            print("== session (app pid %d)" % app_pid, flush=True)
            steps = drive(mc, tree)
            mc.quit(timeout=20)
            stop_tree(proc)
    finally:
        if ss.is_alive():
            ss.stop()
        if proc is not None and proc.poll() is None:
            stop_tree(proc, 5)
        if not keep:
            shutil.rmtree(root, ignore_errors=True)
    events, procs = [], {}
    if trace_log and os.path.exists(trace_log):
        with open(trace_log, errors="replace") as f:
            events, procs = parse_strace(f)
    elif not problems:
        problems.append("no strace log")
    progs = {os.path.basename(p) for p in procs.values()}
    if mc:
        if "WebKitWebProcess" not in progs:
            problems.append("WebKitWebProcess was never seen starting: the web view was not watched")
        if "WebKitNetworkProcess" not in progs:
            print("note: WebKitNetworkProcess was not seen starting (its loads would still go through the web process)")
        if not any(e["family"] in ("AF_UNIX", "AF_LOCAL") for e in events):
            problems.append("strace recorded no Unix socket connection at all (X11, D-Bus): the trace is not working")
    if steps is not None:
        problems += ["core step failed: %s (%s)" % (s["step"], s["note"]) for s in steps if not s["ok"] and s["step"] in Session.CORE]
    findings = report(events, procs, ss, steps, out, problems)
    if findings or ss.findings or problems:
        print("FAIL: see the report above (logs in %s)" % out)
        return 1
    print("PASS: no outbound connection, DNS lookup or open network socket from the app or its processes")
    return 0


# ---------------------------------------------------------------------------------------------------------------------
# Self-test: a planted connection inside the watched tree has to be reported
# ---------------------------------------------------------------------------------------------------------------------

GRANDCHILD = r"""
import json, os, socket, sys, time
out = {"pid": os.getpid()}
# allowed: a loopback connection to a listener of our own, and a Unix socket
srv = socket.socket(); srv.bind(("127.0.0.1", 0)); srv.listen(1)
c = socket.create_connection(srv.getsockname()); c.close(); srv.close()
u = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); name = "\0mc-zn-selftest-%d" % os.getpid()
us = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM); us.bind(name); us.listen(1); u.connect(name); u.close(); us.close()
# planted: a TCP connect attempt to TEST-NET-1, held open, and a "DNS query" to it
s = socket.socket(); s.setblocking(False)
out["connect"] = os.strerror(s.connect_ex((sys.argv[2], int(sys.argv[3]))))
d = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
try:
    d.sendto(b"\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01", (sys.argv[2], 53))
    out["dns"] = "sent"
except OSError as e:
    out["dns"] = str(e)
with open(sys.argv[1], "w") as f:
    json.dump(out, f)
time.sleep(float(sys.argv[4]))
"""


def selftest_child(result, hold):
    """The process strace starts: it starts the grandchild that makes the connections (so -f has to follow)."""
    subprocess.run([sys.executable, "-c", GRANDCHILD, result, SELFTEST_ADDR, str(SELFTEST_PORT), str(hold)], check=True)


def run_selftest(out):
    root = tempfile.mkdtemp(prefix="mc-zn-selftest-")
    result = os.path.join(root, "grandchild.json")
    trace_log = os.path.join(out, "selftest-strace.log")
    argv = ["strace"] + strace_flags() + ["-o", trace_log, "--", sys.executable, os.path.abspath(__file__),
                                          "--selftest-child", result, "--hold", "5"]
    print("== self-test: %s" % " ".join(argv), flush=True)
    proc = subprocess.Popen(argv, start_new_session=True)
    ss = SSWatcher(lambda: proc.pid if proc.poll() is None else 0, os.path.join(out, "selftest-ss.log"), interval=0.25)
    ss.start()
    try:
        proc.wait(60)
    finally:
        ss.stop()
        stop_tree(proc, 5)
    problems = []
    try:
        with open(result) as f:
            gc = json.load(f)
    except (OSError, ValueError) as e:
        gc = {}
        problems.append("the planted process did not run: %s" % e)
    with open(trace_log, errors="replace") as f:
        events, procs = parse_strace(f)
    findings = [e for e in events if e["finding"]]
    report(events, procs, ss, None, out, [])
    planted = [e for e in findings if e["addr"] == SELFTEST_ADDR and e["port"] == SELFTEST_PORT and e["call"] == "connect"]
    dns = [e for e in findings if e["port"] == 53]
    print("planted process: %s" % gc)
    if not planted:
        problems.append("strace did not report the planted connect to %s:%d" % (SELFTEST_ADDR, SELFTEST_PORT))
    elif gc.get("pid") and planted[0]["pid"] != gc["pid"]:
        problems.append("the planted connect was reported for pid %d, not the grandchild's %d" % (planted[0]["pid"], gc["pid"]))
    if gc.get("dns") == "sent" and not dns:
        problems.append("strace did not report the planted DNS query")
    if [e for e in findings if is_local_address(e["addr"]) and e["port"] != 53]:
        problems.append("a loopback connection was reported as a finding")
    if not [e for e in events if e["family"] == "AF_INET" and not e["finding"]]:
        problems.append("the allowed loopback connection was not seen at all")
    if not [e for e in events if e["family"] in ("AF_UNIX", "AF_LOCAL") and e["addr"].startswith("@mc-zn-selftest")]:
        problems.append("the allowed Unix socket connection was not seen at all")
    in_progress = gc.get("connect") == os.strerror(115)  # EINPROGRESS: the socket stays in SYN-SENT for ss to see
    if in_progress and not [f for f in ss.findings if (SELFTEST_ADDR + ":" + str(SELFTEST_PORT)) in f]:
        problems.append("ss did not see the planted socket in SYN-SENT")
    elif not in_progress:
        print("note: the planted connect ended at once (%s), so the ss check could not be tested" % gc.get("connect"))
    shutil.rmtree(root, ignore_errors=True)
    for p in problems:
        print("SELF-TEST PROBLEM: " + p)
    if problems:
        print("FAIL: the detector would miss connections")
        return 1
    print("PASS: the detector reported the planted connection%s and let loopback and Unix sockets pass" % (" (strace and ss)" if in_progress else " (strace)"))
    return 0


def main(argv):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("app", nargs="?", help="the built binary (build/bin/mormicommander)")
    ap.add_argument("--out", help="folder for logs and the report (default: a new temporary folder)")
    ap.add_argument("--keep", action="store_true", help="keep the temporary HOME, settings and test folders")
    ap.add_argument("--selftest", action="store_true", help="check that the watchers report a planted connection")
    ap.add_argument("--selftest-child", metavar="RESULT", help=argparse.SUPPRESS)
    ap.add_argument("--hold", type=float, default=5, help=argparse.SUPPRESS)
    a = ap.parse_args(argv)
    if a.selftest_child:
        selftest_child(a.selftest_child, a.hold)
        return 0
    if not sys.platform.startswith("linux"):
        print("zero_network.py runs on Linux only (strace, ss, /proc)", file=sys.stderr)
        return 2
    out = os.path.abspath(a.out or tempfile.mkdtemp(prefix="mc-zn-out-"))
    os.makedirs(out, exist_ok=True)
    if a.selftest:
        return run_selftest(out)
    if not a.app or not os.path.isfile(a.app):
        print("give the path of the built binary (build/bin/mormicommander)", file=sys.stderr)
        return 2
    return run_session(a.app, out, a.keep)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
