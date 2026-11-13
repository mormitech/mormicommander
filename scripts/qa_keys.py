#!/usr/bin/env python3
"""QA of the Total Commander key layer (board issue #2): the whole key reference of FEATURES.md section 8, end to end.

Runs the built app through control mode (scripts/mc_ctl.py) in fresh temporary folders with a settings folder of its
own; never touches your files, your settings, the Trash or the system clipboard. Three parts:

  1. the key reference: the app's own list (control command "keyref", src/core/tckeys.js) is checked against the live
     keymap and the registered commands, and every key that is safe to press is pressed and must reach its command
     (control command "runs"); a dialog, popup or editor it opens is closed again and the panels are put back;
  2. typing goes to the command line ("cd .." works), quick search (Ctrl+Alt+letter, Up/Down, Esc) and quick filter
     (Ctrl+S) behave like TC, keyboard selection (Ins, Space with folder size, Num + with a mask);
  3. the options: quick search on plain letters, and Space that selects without counting the folder size.

Any failure fails the run (exit code 1), and a failing run blocks a version.

    scripts/qa_keys.py [path/to/mormicommander.app or binary] [--json results.json]
"""
import json
import os
import platform
import shutil
import sys
import tempfile
import traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mc_ctl import MC, MCError  # noqa: E402

MAC = platform.system() == "Darwin"
results = []

# Commands that switch a panel's look or state; after pressing their key the script runs these to put it back.
RESTORE = {
    "cm_SrcShort": ["cm_SrcLong"], "cm_SrcTree": ["cm_SrcLong"], "cm_SrcThumbs": ["cm_SrcLong"],
    "cm_SrcComments": ["cm_SrcLong"], "cm_SrcExecs": ["cm_SrcAllFiles"], "cm_SrcUserSpec": ["cm_SrcAllFiles"],
    "cm_SrcQuickView": ["cm_SrcQuickView"], "cm_SwitchHidSys": ["cm_SwitchHidSys"],
    "cm_SeparateTreeCycle": ["cm_SeparateTreeOff"], "cm_SrcUnsorted": ["cm_SrcByName"],
    "cm_SrcByExt": ["cm_SrcByName"], "cm_SrcByDateTime": ["cm_SrcByName"], "cm_SrcBySize": ["cm_SrcByName"],
}


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


def make_tree(root):
    L, R = os.path.join(root, "left"), os.path.join(root, "right")
    write(os.path.join(L, "alpha", "inner", "deep.txt"), b"deep" * 100)
    write(os.path.join(L, "alpha", "a1.txt"), b"a1")
    os.makedirs(os.path.join(L, "beta"))
    write(os.path.join(L, "gamma.txt"), b"0123456789")
    write(os.path.join(L, "garden.log"), b"g" * 30)
    write(os.path.join(L, "delta.log"), b"x" * 2000)
    write(os.path.join(L, "zed.txt"), b"z" * 50)
    os.makedirs(os.path.join(R, "r1"))
    write(os.path.join(R, "r2.txt"), b"r2")
    return L, R


class Sandbox:
    """The app on a temporary tree; tidy() puts it back to a known state between keys."""

    def __init__(self, app, settings=None):
        self.tmp = os.path.realpath(tempfile.mkdtemp(prefix="mc-qakeys-"))
        self.L, self.R = make_tree(os.path.join(self.tmp, "files"))
        self.config = os.path.join(self.tmp, "config")
        os.makedirs(self.config)
        if settings:
            with open(os.path.join(self.config, "settings.json"), "w") as f:
                json.dump(settings, f)
        self.mc = MC.start(app, "-L=" + self.L, "-R=" + self.R, config_dir=self.config)
        self.mc.window(1200, 760)

    def close(self):
        try:
            self.mc.quit()
        finally:
            shutil.rmtree(self.tmp, ignore_errors=True)

    def tidy(self):
        mc = self.mc
        for _ in range(6):
            s = mc.state()
            if s.get("dialog"):
                mc.cmd("dialog_answer", button="Esc")
            elif s.get("popup"):
                mc.cmd("popup_choose", index=None)
            elif mc.cmd("quicksearch").get("open"):
                mc.key("Esc")
            elif not str(s.get("focus", "")).endswith("-panel"):
                mc.key("Esc")
                if not str(mc.state().get("focus", "")).endswith("-panel"):
                    break  # (the focus stays elsewhere; focusing a panel below takes it back)
            else:
                break
        if mc.state().get("cmdline"):
            mc.key("Esc")
        for side, path in (("right", self.R), ("left", self.L)):
            mc.focus(side)
            if mc.state()[side]["path"] != path:
                mc.cd(side, path)
            mc.run("cm_ClearAll")
        mc.cursor("left", index=0)

    def safe_run(self, name):
        try:
            self.mc.run(name)
        except MCError:
            pass


def suite_reference(app):
    print("== the TC key reference (FEATURES.md section 8)")
    sb = Sandbox(app)
    mc = sb.mc
    try:
        ref = mc.cmd("keyref")["keys"]
        keymap = mc.cmd("keymap")["keymap"]
        registered = {c["name"] for c in mc.cmd("commands")["commands"]}
        check("F-282", len(ref) > 100, "%d keys in the reference" % len(ref))
        for r in ref:
            ids, key, want = r["id"], r["key"], r["command"]
            got = keymap.get(key, "")
            check(ids, got == want, "%s is mapped to %s%s" % (key, want, "" if got == want else " (got %s)" % (got or "nothing")))
            check(ids, want in registered, "%s is implemented" % want)
        for r in ref:
            if r["how"] != "press":
                continue
            ids, key, want = r["id"], r["key"], r["command"]

            def press(ids=ids, key=key, want=want):
                sb.tidy()
                mc.cmd("runs", clear=True)
                mc.key(key)
                names = [x["name"] for x in mc.cmd("runs")["runs"]]
                check(ids, want in names, "pressing %s runs %s%s" % (key, want, "" if want in names else " (ran: %s)" % (", ".join(names) or "nothing")))
                sb.tidy()
                for name in RESTORE.get(want, []):
                    sb.safe_run(name)
            attempt(ids, press)
    finally:
        sb.close()


def suite_typing(app):
    print("== command line, quick search, quick filter, selection")
    sb = Sandbox(app)
    mc, L = sb.mc, sb.L
    try:
        def cmdline():
            sb.tidy()
            s = mc.type("xyz")
            check("F-275 F-300", s["cmdline"] == "xyz", "letters typed in the panel go to the command line")
            mc.key("Esc")
            mc.cd("left", os.path.join(L, "alpha"))
            mc.type("cd ..")
            s = mc.state()
            check("F-300", s["cmdline"] == "cd ..", "the space after cd is typed, not taken by Space (got %r)" % s["cmdline"])
            s = mc.key("Enter")
            check("F-275 F-300", s["left"]["path"] == L and s["cmdline"] == "", '"cd .." + Enter goes up, like TC')
        attempt("F-300", cmdline)

        def quick_search():
            sb.tidy()
            s = mc.key("Ctrl+Alt+G")
            qs = mc.cmd("quicksearch")
            check("F-066 F-274", qs["open"] and qs["text"] == "g" and not qs["filter"], "Ctrl+Alt+G opens quick search with g")
            check("F-066", s["left"]["cursorName"] in ("gamma.txt", "garden.log"), "the cursor jumps to the first match (%s)" % s["left"]["cursorName"])
            first = s["left"]["cursorName"]
            s = mc.key("Down")
            check("F-066", s["left"]["cursorName"] in ("gamma.txt", "garden.log") and s["left"]["cursorName"] != first, "Down: the next match")
            s = mc.key("Up")
            check("F-066", s["left"]["cursorName"] == first, "Up: the previous match")
            mc.key("A")
            s = mc.key("R")
            check("F-066", mc.cmd("quicksearch")["text"] == "gar" and s["left"]["cursorName"] == "garden.log", "more letters narrow it")
            mc.key("Q")
            check("F-066", mc.cmd("quicksearch")["text"] == "gar", "a letter that matches nothing is not taken")
            s = mc.key("Esc")
            check("F-066", not mc.cmd("quicksearch")["open"] and s["left"]["cursorName"] == "garden.log" and s["cmdline"] == "", "Esc closes it, the cursor stays")
        attempt("F-066", quick_search)

        def quick_filter():
            sb.tidy()
            mc.key("Ctrl+S")
            check("F-068 F-250", mc.cmd("quicksearch")["filter"], "Ctrl+S opens the quick filter")
            for k in "TXT":
                mc.key(k)
            names = [n for n in mc.names("left") if n != ".."]
            files = [n for n in names if n not in ("alpha", "beta")]
            check("F-068", sorted(files) == ["gamma.txt", "zed.txt"], "only matching files are shown: %s" % names)
            mc.key("Esc")
            names = mc.names("left")
            check("F-068", "delta.log" in names and "garden.log" in names, "Esc removes the filter")
        attempt("F-068", quick_filter)

        def selection():
            sb.tidy()
            mc.cursor("left", name="gamma.txt")
            s = mc.key("Ins")
            check("F-118", s["left"]["selected"] == ["gamma.txt"] and s["left"]["cursorName"] != "gamma.txt", "Ins selects and moves down")
            mc.run("cm_ClearAll")
            mc.cursor("left", name="alpha")
            mc.key("Space")
            mc.cmd("wait", ms=300)
            row = [e for e in mc.listing("left")["entries"] if e["name"] == "alpha"][0]
            check("F-118", row["selected"] and row["dirSize"] is not None, "Space on a folder selects it and counts its size (%s)" % row["dirSize"])
            mc.run("cm_ClearAll")
            s = mc.key("Num+")
            check("F-120", s["dialog"] is not None, "Num + opens Select files")
            s = mc.cmd("dialog_answer", fields={"mask": "*.txt"})
            check("F-120", sorted(s["left"]["selected"]) == ["gamma.txt", "zed.txt"], "*.txt selects the matching files: %s" % s["left"]["selected"])
        attempt("F-118", selection)
    finally:
        sb.close()


def suite_options(app):
    print("== options: quick search on letters, Space without the folder size")
    sb = Sandbox(app, {"quickSearch": {"mode": "letter"}, "countSpace": False})
    mc = sb.mc
    try:
        def letters():
            sb.tidy()
            s = mc.key("Z")
            qs = mc.cmd("quicksearch")
            check("F-066", qs["open"] and qs["text"] == "z" and s["cmdline"] == "" and s["left"]["cursorName"] == "zed.txt", "letter mode: plain letters search")
            mc.key("Esc")
        attempt("F-066", letters)

        def space_only():
            sb.tidy()
            mc.cursor("left", name="alpha")
            mc.key("Space")
            mc.cmd("wait", ms=300)
            row = [e for e in mc.listing("left")["entries"] if e["name"] == "alpha"][0]
            check("F-797", row["selected"] and row["dirSize"] is None, "with the option off, Space selects without counting")
        attempt("F-797", space_only)
    finally:
        sb.close()


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
    for suite in (suite_reference, suite_typing, suite_options):
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
