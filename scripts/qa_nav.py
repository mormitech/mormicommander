#!/usr/bin/env python3
"""QA of the core: window, panels, navigation, selection keys, sorting, command line basics, control mode.

Runs the built app through control mode (scripts/mc_ctl.py) in fresh temporary folders, and in demo mode; never touches
your files or your settings. Each check names the FEATURES.md IDs it covers. Exit code 0 when everything passed.

    scripts/qa_nav.py [path/to/mormicommander.app or binary] [--json results.json]
"""
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import time
import traceback
import urllib.error
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mc_ctl import MC, MCError, executable  # noqa: E402

MAC = platform.system() == "Darwin"
results = []


def check(ids, ok, note=""):
    results.append({"ids": ids, "ok": bool(ok), "note": note})
    print(("  ok    " if ok else "  FAIL  ") + ids + ("  " + note if note else ""), flush=True)
    return ok


def attempt(ids, fn):
    """Runs one check function; an exception counts as a failure of those IDs."""
    try:
        fn()
    except Exception as e:  # noqa: BLE001
        check(ids, False, "error: %s" % e)
        traceback.print_exc()


def write(p, data):
    os.makedirs(os.path.dirname(p), exist_ok=True)
    with open(p, "wb") as f:
        f.write(data)


LONG = "a very long file name that will not fit into the name column at all.txt"


def make_tree(root):
    L, R = os.path.join(root, "left"), os.path.join(root, "right")
    write(os.path.join(L, "alpha", "inner", "deep.txt"), b"deep")
    write(os.path.join(L, "alpha", "a1.txt"), b"a1")
    os.makedirs(os.path.join(L, "beta"))
    write(os.path.join(L, "gamma.txt"), b"0123456789")
    write(os.path.join(L, "delta.log"), b"x" * 2000)
    write(os.path.join(L, ".hidden_file"), b"h")
    write(os.path.join(L, "zed.txt"), b"z" * 50)
    write(os.path.join(L, LONG), b"long")
    os.symlink(os.path.join(L, "alpha"), os.path.join(L, "link_dir"))
    os.symlink(os.path.join(L, "alpha", "a1.txt"), os.path.join(L, "link_file"))
    os.makedirs(os.path.join(R, "r1"))
    write(os.path.join(R, "r2.txt"), b"r2")
    return L, R


def suite_real(app):
    print("== real folders (temporary)")
    tmp = os.path.realpath(tempfile.mkdtemp(prefix="mc-qa-"))
    mc = None
    try:
        L, R = make_tree(tmp)
        mc = MC.start(app, "-L=" + L, "-R=" + R)
        mc.window(1200, 760)
        st = mc.state

        def first():
            s = st()
            check("F-001", s["active"] == "left" and s["left"]["path"] == L and s["right"]["path"] == R, "two panels, left active")
            d = mc.cmd("dom", selector=".panel.active")
            check("F-001", d["count"] == 1, "one highlighted panel")
            s = mc.cmd("click", selector='.panel[data-side="right"] .list')
            check("F-001", s["active"] == "right", "clicking a panel makes it active")
            mc.cmd("click", selector='.panel[data-side="left"] .list')
            check("F-102", mc.names("left")[0] == "..", '".." line on top')
        attempt("F-001", first)

        def cursor():
            mc.cursor("left", index=0)
            s = mc.key("Down")
            check("F-049", s["left"]["cursor"] == 1 and s["left"]["cursorName"] == "alpha", "Down")
            s = mc.key("Down", "Down", "Up")
            check("F-049", s["left"]["cursor"] == 2, "Up")
            s = mc.key("End")
            last = s["left"]["count"]
            check("F-049", s["left"]["cursor"] == last, "End goes to the last entry")
            s = mc.key("Home")
            check("F-049", s["left"]["cursor"] == 0, "Home goes to the first entry")
            s = mc.key("PgDn")
            check("F-049", s["left"]["cursor"] == last, "PgDn (short list: to the end)")
            s = mc.key("PgUp")
            check("F-049", s["left"]["cursor"] == 0, "PgUp")
        attempt("F-049", cursor)

        def tab():
            mc.cursor("left", name="beta")
            s = mc.key("Tab")
            check("F-002 F-272", s["active"] == "right", "Tab switches to the right panel")
            s = mc.key("Tab")
            check("F-002 F-272", s["active"] == "left" and s["left"]["cursorName"] == "beta", "Tab back; cursor kept")
            s = mc.key("Ctrl+I")
            check("F-243", s["active"] == "right", "Ctrl+I focuses the other panel")
            mc.key("Ctrl+I")
            s = mc.run("cm_FocusRight")
            ok = s["active"] == "right"
            s = mc.run("cm_FocusLeft")
            ok = ok and s["active"] == "left"
            s = mc.run("cm_FocusSrc")
            check("F-766", ok and s["active"] == "left", "cm_FocusLeft / cm_FocusRight / cm_FocusSrc")
        attempt("F-002", tab)

        def enter_parent():
            mc.cursor("left", name="alpha")
            s = mc.key("Enter")
            check("F-051", s["left"]["path"] == os.path.join(L, "alpha") and s["left"]["cursorName"] == "..", "Enter opens a folder")
            s = mc.key("Backspace")
            check("F-052", s["left"]["path"] == L and s["left"]["cursorName"] == "alpha", "Backspace: parent, cursor on the folder we came from")
            mc.key("Enter")
            mc.cursor("left", index=0)
            s = mc.key("Enter")
            check("F-052", s["left"]["path"] == L and s["left"]["cursorName"] == "alpha", 'Enter on ".." goes up too')
            s = mc.key("Enter")
            mc.cursor("left", name="a1.txt")
            mc.key("Backspace")
            s = mc.key("Enter")
            # Restoring the cursor of a folder you come back to (F-074) is the setting "rememberCursor", off by
            # default: re-entering a folder puts the cursor on "..". suite_remember_cursor checks it switched on.
            check("F-074", s["left"]["cursorName"] == "..", 'back in a folder, the cursor is on ".." (rememberCursor off)')
            s = mc.key("Ctrl+PgUp")
            check("F-077 F-260", s["left"]["path"] == L and s["left"]["cursorName"] == "alpha", "Ctrl+PgUp goes to the parent")
            mc.cursor("left", name="beta")
            s = mc.key("Ctrl+PgDn")
            check("F-078 F-260", s["left"]["path"] == os.path.join(L, "beta"), "Ctrl+PgDn opens the folder under the cursor")
            mc.key("Backspace")
            mc.cursor("left", name="link_file")
            s = mc.key("Ctrl+PgDn")
            check("F-076", s["left"]["path"] == os.path.join(L, "alpha") and s["left"]["cursorName"] == "a1.txt", "Ctrl+PgDn on a link to a file: its folder, cursor on it")
            mc.cd("left", L)
            mc.cursor("left", name="link_dir")
            s = mc.key("Ctrl+PgDn")
            check("F-076", s["left"]["path"] == os.path.join(L, "alpha"), "Ctrl+PgDn on a link to a folder: the target folder")
            mc.cd("left", L)
            mc.cursor("left", name="link_dir")
            s = mc.key("Enter")
            check("F-051", s["left"]["path"] == os.path.join(L, "link_dir"), "Enter on a link to a folder opens it")
            mc.cd("left", L)
        attempt("F-051", enter_parent)

        def root_history():
            s = mc.key("Ctrl+\\")
            check("F-053 F-261", s["left"]["path"] == "/", "Ctrl+\\ goes to the root")
            s = mc.key("Alt+Left")
            check("F-054 F-266", s["left"]["path"] == L, "Alt+Left goes back")
            s = mc.key("Alt+Right")
            check("F-054 F-266", s["left"]["path"] == "/", "Alt+Right goes forward")
            mc.key("Alt+Left")
            s = mc.key("Ctrl+<")
            check("F-778 F-808", s["left"]["path"] == "/", "Ctrl+< (key next to left Shift) goes to the root")
            mc.key("Alt+Left")
            s = mc.key("Alt+Down")
            pop = s["popup"]
            labels = [i.get("label") for i in (pop or {}).get("items", [])]
            check("F-055 F-267", pop is not None and pop["kind"] == "history" and "/" in labels, "Alt+Down lists visited folders")
            s = mc.choose("/")
            check("F-055", s["left"]["path"] == "/" and s["popup"] is None, "choosing one goes there")
            mc.key("Alt+Left")
            s = mc.cmd("click", selector='.panel[data-side="left"] .hist')
            check("F-056", s["popup"] is not None and s["popup"]["kind"] == "history", "the arrow on the path bar opens the same list")
            s = mc.choose(None)
            check("F-056", s["popup"] is None, "Esc-like close")
            mc.cd("left", L)
            mc.cd("left", "/")
            s = mc.cmd("click", selector='.panel[data-side="left"] .list', button=3)
            ok = s["left"]["path"] == L
            s = mc.cmd("click", selector='.panel[data-side="left"] .list', button=4)
            check("F-073", ok and s["left"]["path"] == "/", "mouse buttons 4 and 5 go back and forward")
            mc.cd("left", L)
        attempt("F-053", root_history)

        def transfer():
            mc.cursor("left", name="beta")
            s = mc.key("Ctrl+Right")
            check("F-007 F-259 F-367", s["right"]["path"] == os.path.join(L, "beta") and s["active"] == "left", "Ctrl+Right: folder under cursor opens in the right panel")
            mc.cursor("left", name="gamma.txt")
            s = mc.key("Ctrl+Right")
            check("F-007", s["right"]["path"] == L and s["right"]["cursorName"] == "gamma.txt", "on a file: the current folder, cursor on the file")
            mc.focus("right")
            mc.cursor("right", name="alpha")
            s = mc.key("Ctrl+Left")
            check("F-007 F-259", s["left"]["path"] == os.path.join(L, "alpha") and s["active"] == "right", "Ctrl+Left from the right panel")
            mc.focus("left")
            mc.cd("left", L)
            mc.cd("right", R)
        attempt("F-007", transfer)

        def swap_match():
            mc.key("Ctrl+F6")  # left sorted by size
            s = mc.key("Ctrl+U")
            check("F-004 F-252 F-362", s["left"]["path"] == R and s["right"]["path"] == L and s["right"]["sort"]["key"] == "size" and s["left"]["sort"]["key"] == "name", "Ctrl+U swaps folders and sort")
            s = mc.key("Ctrl+U")
            mc.key("Ctrl+F3")
            check("F-004", s["left"]["path"] == L and s["right"]["path"] == R, "Ctrl+U again swaps back")
            s = mc.cmd("menu", name="cm_Exchange")
            ok = s["left"]["path"] == R and s["right"]["path"] == L
            s = mc.cmd("menu", name="cm_Exchange")
            check("F-362", ok and s["left"]["path"] == L, "Commands > Swap panels does what Ctrl+U does")
            s = mc.cmd("menu", name="cm_MatchSrc")
            check("F-006 F-057 F-361", s["right"]["path"] == L, "Commands > Target = source")
            menus = mc.cmd("menus")["menus"]
            items = [i for m in menus if m["label"] == "Commands" for i in m["items"] if i.get("cmd") == "cm_MatchSrc"]
            check("F-006", items and not items[0]["disabled"], "the menu entry is there and enabled")
            mc.cd("right", R)
        attempt("F-004", swap_match)

        def hidden():
            s = mc.key("Ctrl+H")
            check("F-090 F-242", s["showHidden"] and ".hidden_file" in mc.names("left"), "Ctrl+H shows hidden files")
            menus = mc.cmd("menus")["menus"]
            hid = [i for m in menus if m["label"] == "Show" for i in m["items"] if i.get("cmd") == "cm_SwitchHidSys"]
            check("F-409", hid and hid[0].get("checked") is True, "Show > Hidden files has a check mark")
            s = mc.cmd("menu", name="cm_SwitchHidSys")
            check("F-090 F-409", not s["showHidden"] and ".hidden_file" not in mc.names("left"), "the menu entry hides them again")
            if MAC:
                s = mc.key("Shift+Cmd+.")
                ok = s["showHidden"]
                s = mc.key("Shift+Cmd+.")
                check("F-242 F-285", ok and not s["showHidden"], "Cmd+Shift+. toggles hidden files on macOS")
        attempt("F-090", hidden)

        def reread():
            mc.cursor("left", name="gamma.txt")
            write(os.path.join(L, "new.txt"), b"n")
            s = mc.key("Ctrl+R")
            check("F-064 F-249", "new.txt" in mc.names("left") and s["left"]["cursorName"] == "gamma.txt", "Ctrl+R shows a new file; cursor stays")
            os.remove(os.path.join(L, "new.txt"))
            s = mc.key("F2")
            check("F-147 F-202", "new.txt" not in mc.names("left") and s["left"]["cursorName"] == "gamma.txt", "F2 rereads too")
            s = mc.cmd("menu", name="cm_RereadSource")
            check("F-407", s["left"]["path"] == L, "Show > Reread")
        attempt("F-064", reread)

        def goto_cmds():
            s = mc.run("cm_GoToLastEntry")
            ok = s["left"]["cursor"] == s["left"]["count"]
            s = mc.run("cm_GoToFirstEntry")
            ok = ok and s["left"]["cursor"] == 0
            s = mc.run("cm_GoToFirstFile")
            names = mc.names("left")
            ok = ok and names[s["left"]["cursor"]] == LONG
            mc.cursor("left", name="gamma.txt")
            mc.key("Ins")
            mc.cursor("left", name="zed.txt")
            mc.key("Ins")
            mc.run("cm_GoToFirstEntry")
            s = mc.run("cm_GoToNextSelected")
            ok = ok and s["left"]["cursorName"] == "gamma.txt"
            s = mc.run("cm_GoToNextSelected")
            ok = ok and s["left"]["cursorName"] == "zed.txt"
            s = mc.run("cm_GoToPrevSelected")
            ok = ok and s["left"]["cursorName"] == "gamma.txt"
            mc.run("cm_ClearAll")
            s = mc.run("cm_GoToFirstEntry")
            s2 = mc.run("cm_GoToNextSelected")
            check("F-779", ok and s2["left"]["cursor"] == 0, "first/last entry, first file, next/previous selected")
        attempt("F-779", goto_cmds)

        def selection():
            mc.cursor("left", name="delta.log")
            s = mc.key("Ins")
            check("F-118 F-271", s["left"]["selected"] == ["delta.log"] and s["left"]["cursorName"] == "gamma.txt", "Ins selects and goes down")
            check("F-015 F-137", re.match(r"^2 KB of [\d.]+ KB in 1 of 5 files selected, 0 of 3 folders$", s["left"]["status"]) is not None, "status line: " + s["left"]["status"])
            s = mc.key("Space")
            check("F-118 F-271", sorted(s["left"]["selected"]) == ["delta.log", "gamma.txt"] and s["left"]["cursorName"] == "gamma.txt", "Space selects without moving")
            mc.run("cm_ClearAll")
            mc.cursor("left", name="delta.log")
            s = mc.key("Shift+Down", "Shift+Down")
            check("F-119", sorted(s["left"]["selected"]) == ["delta.log", "gamma.txt"], "Shift+Down selects a range")
            s = mc.key("Shift+Up")
            check("F-119", s["left"]["selected"] == ["delta.log"], "Shift+Up takes back the last one")
            mc.key("Shift+End")
            s = st()
            check("F-119", len(s["left"]["selected"]) >= 4, "Shift+End selects to the end")
            s = mc.key("Ctrl+A")
            files = [e["name"] for e in mc.listing("left")["entries"] if not e["dir"]]
            check("F-122 F-235", sorted(s["left"]["selected"]) == sorted(files), "Ctrl+A selects all files")
            s = mc.key("Ctrl+Num-")
            check("F-123 F-270", s["left"]["selected"] == [], "Ctrl+Num- unselects all")
            s = mc.key("Ctrl+Num+")
            check("F-270", sorted(s["left"]["selected"]) == sorted(files), "Ctrl+Num+ selects all")
            mc.run("cm_SaveSelection")
            mc.run("cm_ClearAll")
            s = mc.key("Num/")
            check("F-131", sorted(s["left"]["selected"]) == sorted(files), "Num / brings back the saved selection")
            mc.run("cm_ClearAll")
            mc.cursor("left", name="zed.txt")
            mc.key("Ins")
            s = mc.key("Num*")
            check("F-124", sorted(s["left"]["selected"]) == sorted(f for f in files if f != "zed.txt"), "Num * inverts (files)")
            s = mc.key("Shift+Num*")
            dirs = [e["name"] for e in mc.listing("left")["entries"] if e["dir"] and not e["up"]]
            check("F-125 F-805", sorted(s["left"]["selected"]) == sorted(dirs + ["zed.txt"]), "Shift+Num * inverts with folders")
            mc.run("cm_ClearAll")
            s = mc.key("Shift+8")
            check("F-288", sorted(s["left"]["selected"]) == sorted(files), "Shift+8 = Num * while the command line is empty")
            mc.run("cm_ClearAll")
        attempt("F-118", selection)

        def sorting():
            s = mc.cmd("click", selector='.panel[data-side="left"] .header [data-col="size"]')
            names = [n for n in mc.names("left") if n not in ("..", "alpha", "beta", "link_dir")]
            check("F-014 F-113", s["left"]["sort"] == {"key": "size", "desc": True} and names[0] == "delta.log", "clicking Size sorts largest first")
            s = mc.cmd("click", selector='.panel[data-side="left"] .header [data-col="size"]')
            check("F-014 F-113", s["left"]["sort"]["desc"] is False, "again: reversed")
            d = mc.cmd("dom", selector='.panel[data-side="left"] .header .sorted')
            check("F-113", d["count"] == 1 and "Size" in d["texts"][0], "the header marks the sort column")
            s = mc.key("Ctrl+F3")
            check("F-107", s["left"]["sort"] == {"key": "name", "desc": False}, "Ctrl+F3 by name")
            s = mc.key("Ctrl+F3")
            check("F-107", s["left"]["sort"]["desc"] is True, "Ctrl+F3 again: descending")
            s = mc.key("Ctrl+F4")
            files = [n for n in mc.names("left") if n not in ("..", "alpha", "beta", "link_dir")]
            check("F-108", s["left"]["sort"]["key"] == "ext" and files[0] == "link_file", "Ctrl+F4 by extension (no extension first)")
            s = mc.key("Ctrl+F5")
            check("F-109", s["left"]["sort"] == {"key": "date", "desc": True}, "Ctrl+F5 by date, newest first")
            s = mc.key("Ctrl+F6")
            check("F-110", s["left"]["sort"] == {"key": "size", "desc": True}, "Ctrl+F6 by size, largest first")
            s = mc.key("Ctrl+F7")
            check("F-111", s["left"]["sort"]["key"] == "none", "Ctrl+F7 unsorted")
            mc.key("Ctrl+F3")
            s = mc.run("cm_SrcNegOrder")
            check("F-112", s["left"]["sort"] == {"key": "name", "desc": True}, "cm_SrcNegOrder reverses")
            mc.run("cm_SrcNegOrder")
        attempt("F-014", sorting)

        def pathbar():
            s = mc.cmd("click", selector='.panel[data-side="left"] .pathbar')
            d = mc.cmd("dom", selector='.panel[data-side="left"] .pathbar input')
            check("F-011", d["count"] == 1, "clicking the path bar edits it")
            mc.key("Esc")
            d = mc.cmd("dom", selector='.panel[data-side="left"] .pathbar input')
            check("F-011", d["count"] == 0 and st()["left"]["path"] == L, "Esc cancels")
            mc.run("cm_EditPath")
            mc.type(R)
            s = mc.key("Enter")
            check("F-011 F-061", s["left"]["path"] == R, "typing a path and Enter goes there")
            mc.cd("left", os.path.join(L, "alpha", "inner"))
            s = mc.cmd("click", selector='.panel[data-side="left"] .crumbs span[data-p="%s"]' % L)
            check("F-012", s["left"]["path"] == L, "clicking a part of the path goes there")
        attempt("F-011", pathbar)

        def cmdline():
            s = mc.type("xyz")
            check("F-275 F-300", s["cmdline"] == "xyz", "letters typed in the panel go to the command line")
            s = mc.key("Esc")
            check("F-273", s["cmdline"] == "", "Esc clears it")
            mc.type("cd alpha")
            s = mc.key("Enter")
            check("F-062 F-297 F-812", s["left"]["path"] == os.path.join(L, "alpha") and s["cmdline"] == "", "cd <folder> + Enter")
            mc.type("cd ..")
            s = mc.key("Enter")
            check("F-062", s["left"]["path"] == L, "cd ..")
            mc.type("cd /")
            s = mc.key("Enter")
            check("F-062", s["left"]["path"] == "/", "cd /")
            mc.cd("left", os.path.join(L, "alpha"))
            mc.type("cm_GoToParent")
            s = mc.key("Enter")
            check("F-301", s["left"]["path"] == L, "a cm_* name runs that command")
            mc.cursor("left", name="gamma.txt")
            mc.key("Ctrl+Enter")
            mc.cursor("left", name="delta.log")
            s = mc.key("Ctrl+Enter")
            check("F-185 F-262 F-813", s["cmdline"] == "gamma.txt delta.log ", "Ctrl+Enter adds names: %r" % s["cmdline"])
            mc.key("Esc")
            mc.cursor("left", name=LONG)
            s = mc.key("Ctrl+Enter")
            check("F-298", s["cmdline"] == '"%s" ' % LONG, "names with spaces are quoted")
            mc.key("Esc")
            mc.cursor("left", name="gamma.txt")
            s = mc.key("Ctrl+Shift+Enter")
            check("F-186 F-262", s["cmdline"] == os.path.join(L, "gamma.txt") + " ", "Ctrl+Shift+Enter adds the full path")
            mc.key("Esc")
            s = mc.key("Ctrl+P")
            check("F-187 F-247", s["cmdline"] == L + " ", "Ctrl+P adds the folder path")
            mc.key("Esc")
        attempt("F-062", cmdline)

        def drives():
            s = mc.key("Alt+F1")
            pop = s["popup"]
            check("F-058 F-174 F-219", pop is not None and pop["kind"] == "drives" and "left" in pop["title"] and len(pop["items"]) >= 1, "Alt+F1 lists the drives for the left panel")
            first = pop["items"][0]["label"]
            same = [i for i in pop["items"] if i.get("label", "").lower().startswith(first[0].lower())]
            s = mc.key(first[0].upper())
            if len(same) > 1:
                s = mc.key("Enter")
            check("F-058 F-783", s["popup"] is None and s["left"]["path"] == "/", "typing the first letter picks the drive (%s)" % first)
            s = mc.key("Alt+F2")
            check("F-175 F-219", s["popup"] is not None and "right" in s["popup"]["title"], "Alt+F2 for the right panel")
            mc.key("Esc")
            s = mc.cmd("click", selector='.panel[data-side="right"] .drivebar button')
            check("F-059", s["right"]["path"] == "/" and s["active"] == "right", "a drive button switches its panel")
            check("F-016", re.match(r"^\[.+\]\s+.+ free of .+$", s["right"]["driveInfo"] or "") is not None, "drive info: " + s["right"]["driveInfo"])
            mc.focus("left")
            mc.cd("left", L)
            mc.cd("right", R)
        attempt("F-058", drives)

        def window_bits():
            info = mc.info()
            check("F-017", info.get("title") == "mormiCommander – " + L, "title: " + str(info.get("title")))
            menus = [m["label"] for m in mc.cmd("menus")["menus"]]
            check("F-010", menus == ["Files", "Mark", "Commands", "Net", "Show", "Configuration", "Start", "Help"], "menus: %s" % menus)
            s = mc.cmd("click", selector='#fkeys button[data-key="F7"]')
            check("F-023", "not ready yet" in s["message"] or s["dialog"] is not None, "a function key button runs its key's command: " + s["message"])
            mc.cmd("key", keys=["Shift"], hold=True)
            d = mc.cmd("dom", selector="#fkeys button")
            shifted = " ".join(d["texts"])
            mc.cmd("release")
            d2 = mc.cmd("dom", selector="#fkeys button")
            check("F-023", "Rename" in shifted and "Move" in " ".join(d2["texts"]) and "Rename" not in " ".join(d2["texts"]), "labels change while Shift is held")
            lay = mc.cmd("layout")
            s = mc.key("Ctrl+Shift+H")
            lay2 = mc.cmd("layout")
            mc.key("Ctrl+Shift+H")
            lay3 = mc.cmd("layout")
            check("F-003", lay2["horizontal"] and lay2["left"]["y"] < lay2["right"]["y"] and not lay3["horizontal"], "Ctrl+Shift+H stacks the panels and back")
            mc.cmd("drag", selector="#splitter", dx=-200)
            lay4 = mc.cmd("layout")
            check("F-008", lay4["left"]["w"] < lay4["right"]["w"] - 300, "dragging the splitter: %d / %d" % (lay4["left"]["w"], lay4["right"]["w"]))
            mc.cmd("click", selector="#splitter", double=True)
            lay5 = mc.cmd("layout")
            check("F-008", abs(lay5["left"]["w"] - lay5["right"]["w"]) < 4, "double-click: half and half")
            mc.key("Ctrl+=")
            big = mc.cmd("layout")["rowHeight"]
            saved = mc.cmd("settings")["settings"].get("fontSize")
            mc.key("Ctrl+-")
            check("F-101", big > lay["rowHeight"] and saved == 14 and mc.cmd("layout")["rowHeight"] == lay["rowHeight"], "Ctrl+= / Ctrl+- change the text size (%s → %s) and save it" % (lay["rowHeight"], big))
        attempt("F-010", window_bits)

        def control():
            with open(mc.file) as f:
                info = json.load(f)
            ok = info["url"].startswith("http://127.0.0.1:") and len(info["token"]) == 32
            mode = oct(os.stat(mc.file).st_mode & 0o777)
            try:
                urllib.request.urlopen(urllib.request.Request(info["url"] + "/info"), timeout=5)
                refused = False
            except urllib.error.HTTPError as e:
                refused = e.code == 401
            check("F-723", ok and refused and mode == "0o600", "control file (%s), no token refused" % mode)
            png = os.path.join(tmp, "shot.png")
            mc.shot(png)
            with open(png, "rb") as f:
                check("F-725", f.read(8) == b"\x89PNG\r\n\x1a\n", "screenshot is a PNG")
            out = subprocess.run([executable(app), "--version"], capture_output=True, text=True, timeout=20).stdout.strip()
            check("F-759", out == "mormiCommander " + mc.info()["version"], "--version: " + out)
        attempt("F-723", control)
    finally:
        if mc:
            mc.quit()
        shutil.rmtree(tmp, ignore_errors=True)


def suite_start_options(app):
    print("== start options")
    tmp = os.path.realpath(tempfile.mkdtemp(prefix="mc-qa-"))
    cfg = os.path.realpath(tempfile.mkdtemp(prefix="mc-qa-config-"))
    try:
        L, R = make_tree(tmp)
        mc = MC.start(app, "/L=" + R, "/R=" + L, "/P=R", "--theme", "dark", config_dir=cfg)
        try:
            s = mc.state()
            check("F-728", s["left"]["path"] == R and s["right"]["path"] == L, "/L= and /R= open those folders")
            check("F-732", s["active"] == "right", "/P=R makes the right panel active")
            check("F-021", mc.cmd("layout")["theme"] == "dark", "--theme dark")
            mc.window(900, 600, 120, 140)
            mc.cd("left", os.path.join(L, "alpha"))
            mc.cmd("drag", selector="#splitter", dx=-150)
            split = mc.cmd("layout")["split"]
            mc.key("Ctrl+Shift+H")
            time.sleep(0.6)  # (the session is saved a moment after a change)
        finally:
            mc.quit()
        mc = MC.start(app, config_dir=cfg)
        try:
            info = mc.info()
            s = mc.state()
            lay = mc.cmd("layout")
            check("F-018", abs(info["w"] - 900) <= 2 and abs(info["h"] - 600) <= 2 and abs(info["x"] - 120) <= 2 and abs(info["y"] - 140) <= 30,
                  "window comes back where it was: %s" % {k: info[k] for k in "xywh"})
            check("F-018", abs(lay["split"] - split) < 0.01, "the splitter comes back (%.2f)" % lay["split"])
            check("F-003 F-410", lay["horizontal"], "panels on top of each other stay so after a restart")
            check("session", s["left"]["path"] == os.path.join(L, "alpha") and s["right"]["path"] == L and s["active"] == "right", "the folders and the active panel come back")
            mc.key("Ctrl+Shift+H")
            mc.window(900, 600, max=True)
            time.sleep(0.5)
        finally:
            mc.quit()
        mc = MC.start(app, config_dir=cfg)
        try:
            time.sleep(0.8)
            check("F-018", mc.info()["maximised"], "a maximised window opens maximised")
        finally:
            mc.quit()
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
        shutil.rmtree(cfg, ignore_errors=True)


def suite_remember_cursor(app):
    """F-074 with the setting "rememberCursor" on (it is off by default): coming back into a folder puts the cursor
    on the entry it was on when the folder was left. The setting is written to a settings folder of the test's own."""
    print("== remember cursor (setting on)")
    tmp = os.path.realpath(tempfile.mkdtemp(prefix="mc-qa-"))
    cfg = os.path.realpath(tempfile.mkdtemp(prefix="mc-qa-config-"))
    try:
        L, R = make_tree(tmp)
        with open(os.path.join(cfg, "settings.json"), "w") as f:
            json.dump({"rememberCursor": True}, f)
        mc = MC.start(app, "-L=" + L, "-R=" + R, config_dir=cfg)
        try:
            def remember():
                mc.focus("left")
                mc.cd("left", os.path.join(L, "alpha"))
                mc.cursor("left", name="a1.txt")
                s = mc.key("Backspace")
                ok = s["left"]["path"] == L and s["left"]["cursorName"] == "alpha"
                s = mc.key("Enter")
                check("F-074", ok and s["left"]["cursorName"] == "a1.txt", "back in a folder, the cursor is where it was (rememberCursor on)")
            attempt("F-074", remember)
        finally:
            mc.quit()
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
        shutil.rmtree(cfg, ignore_errors=True)


def suite_demo(app):
    print("== demo")
    mc = MC.start(app, "--demo")
    try:
        s = mc.state()
        check("F-727", s["demo"] and s["left"]["path"] == "/Users/demo", "--demo shows the fictional home folder")
        mc.cursor("left", name="notes.txt")
        s = mc.key("Enter")
        check("F-051", "demo" in s["message"], "Enter on a file asks the system to open it (the demo refuses): " + s["message"])
        mc.type("cd ~/Documents")
        s = mc.key("Enter")
        ok = s["left"]["path"] == "/Users/demo/Documents"
        mc.cd("left", "/")
        mc.type("cd ~")
        s = mc.key("Enter")
        check("F-063", ok and s["left"]["path"] == "/Users/demo", "~ is the home folder")
        t = time.time()
        s = mc.cd("left", "/Users/demo/Projects/Big folder")
        took = time.time() - t
        p = s["left"]
        check("F-742", p["count"] == 100000 and p["renderedRows"] < 200, "100,000 files, %d rows in the page, read in %.1f s" % (p["renderedRows"], took))
        t = time.time()
        s = mc.key("End")
        p = s["left"]
        check("F-742", p["cursorName"] == "file100000.dat" and p["firstRenderedRow"] > 99000 and time.time() - t < 2, "End jumps to the last of 100,000 at once")
        s = mc.cmd("scroll", side="left", row=50000)
        check("F-742", 49990 <= s["left"]["firstRenderedRow"] <= 50000, "scrolling to the middle draws the middle rows")
        mc.window(1200, 760)
        mc.cd("left", "/Users/demo")
        mc.shot(os.path.join(tempfile.gettempdir(), "mc-qa-demo.png"))
        try:
            mc.key("Alt+F4")
        except (OSError, MCError):
            pass
        try:
            mc.proc.wait(10)
        except subprocess.TimeoutExpired:
            pass
        check("F-221 F-336", mc.proc.poll() is not None, "Alt+F4 quits")
    finally:
        mc.quit()


def main(argv):
    app = "build/bin/mormicommander.app" if MAC else "build/bin/mormicommander"
    out = None
    args = argv[1:]
    if "--json" in args:
        i = args.index("--json")
        out = args[i + 1]
        del args[i:i + 2]
    if args:
        app = args[0]
    for suite in (suite_real, suite_start_options, suite_remember_cursor, suite_demo):
        try:
            suite(app)
        except (MCError, OSError) as e:
            check(suite.__name__, False, "suite stopped: %s" % e)
    failed = [r for r in results if not r["ok"]]
    print("\n%d checks, %d failed" % (len(results), len(failed)))
    for r in failed:
        print("  FAIL", r["ids"], r["note"])
    if out:
        with open(out, "w") as f:
            json.dump(results, f, indent=1)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
