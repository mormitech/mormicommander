#!/usr/bin/env python3
"""Drive a mormiCommander started in control mode (see docs/control.md). This is the QA harness.

As a library:
    from mc_ctl import MC
    mc = MC.start("build/bin/mormicommander.app", "--demo")          # or args=["-L=/tmp/a", "-R=/tmp/b"]
    mc.key("Down", "Enter")                                            # real key presses through the app's own handler
    mc.run("cm_GoToParent")                                            # a command by name
    s = mc.state(); print(s["left"]["path"], s["left"]["cursorName"])
    mc.shot("/tmp/shot.png")
    mc.quit()

From the shell (the app has to be running with --control, and the control file in its default place or in
MORMICOMMANDER_CONTROL_FILE):
    scripts/mc_ctl.py state
    scripts/mc_ctl.py key Down Down Enter
    scripts/mc_ctl.py run cm_GoToParent
    scripts/mc_ctl.py cd left /tmp
    scripts/mc_ctl.py shot /tmp/x.png
    scripts/mc_ctl.py '{"cmd": "list", "side": "left", "count": 20}'
    scripts/mc_ctl.py --batch commands.json

Every app started with MC.start() gets a control file of its own, so several can run at once, and quit() ends only
the process it started.
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request


class MCError(Exception):
    pass


def default_control_file():
    return os.environ.get("MORMICOMMANDER_CONTROL_FILE") or os.path.join(tempfile.gettempdir(), "mormicommander-control.json")


def executable(app):
    """The program inside a .app bundle (started directly, so the process is ours and its output can be read)."""
    app = os.path.abspath(app)
    if app.endswith(".app"):
        name = os.path.splitext(os.path.basename(app))[0]
        return os.path.join(app, "Contents", "MacOS", name)
    return app


class MC:
    def __init__(self, file=None, proc=None, own_config=None):
        self.own_config = own_config  # a settings folder start() made, removed by quit()
        self.file = file or default_control_file()
        with open(self.file) as f:
            info = json.load(f)
        self.url, self.token, self.pid = info["url"], info["token"], info.get("pid")
        self.proc = proc

    @classmethod
    def start(cls, app="build/bin/mormicommander.app", *args, control_file=None, config_dir=None, timeout=30, log=None):
        """Starts the app in control mode with the given arguments and waits until it answers and has shown its panels.

        The app gets a settings folder of its own (config_dir, or a new empty one that quit() removes), so a test never
        reads or changes the user's real settings."""
        exe = executable(app)
        if not os.path.exists(exe):
            raise MCError("no program at %s: build it first (scripts/build.sh)" % exe)
        own_file = control_file is None
        if own_file:
            fd, control_file = tempfile.mkstemp(prefix="mc-control-", suffix=".json")
            os.close(fd)
        if os.path.exists(control_file):
            os.remove(control_file)
        argv = [exe] + list(args)
        if "--demo" not in args and "--mock" not in args and not any(a.startswith("--control") for a in args):
            argv.append("--control=0")
        argv += ["--control-file", control_file]
        own_config = None
        if config_dir is None:
            config_dir = own_config = tempfile.mkdtemp(prefix="mc-config-")
        out = open(log, "w") if log else subprocess.DEVNULL
        env = dict(os.environ, MORMICOMMANDER_REEXEC="1", MORMICOMMANDER_CONFIG_DIR=config_dir)
        proc = subprocess.Popen(argv, stdout=out, stderr=out, env=env)
        end = time.time() + timeout
        last = None
        while time.time() < end:
            if proc.poll() is not None:
                raise MCError("mormiCommander exited with code %s before answering" % proc.returncode)
            try:
                mc = cls(control_file, proc, own_config)
                mc.info()
                mc.wait_ready(max(1, end - time.time()))
                return mc
            except (OSError, KeyError, ValueError, MCError) as e:
                last = e
                time.sleep(0.2)
        proc.terminate()
        if own_config:
            shutil.rmtree(own_config, ignore_errors=True)
        raise MCError("mormiCommander did not answer in %d seconds (%s)" % (timeout, last))

    def wait_ready(self, timeout=20):
        """Waits until both panels show a folder."""
        end = time.time() + timeout
        while time.time() < end:
            try:
                s = self.cmd("state", timeout=5)
                if s["left"]["path"] and s["right"]["path"] and not s["busy"]:
                    return s
            except MCError:
                pass
            time.sleep(0.2)
        raise MCError("the panels did not show folders in %d seconds" % timeout)

    def _req(self, path, body=None, timeout=60):
        data = None if body is None else json.dumps(body).encode()
        req = urllib.request.Request(self.url + path, data, {"Authorization": "Bearer " + self.token, "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=timeout) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            raise MCError("HTTP %d: %s" % (e.code, e.read().decode(errors="replace").strip()))

    def info(self):
        return self._req("/info", timeout=10)

    def cmd(self, _name, /, timeout=60, **args):
        """One command; returns the answer, or raises MCError with what the app said."""
        res = self._req("/cmd", dict(args, cmd=_name), timeout=timeout)
        if isinstance(res, dict) and res.get("error"):
            raise MCError("%s: %s" % (_name, res["error"]))
        return res

    def batch(self, commands, keep_going=False):
        res = self._req("/batch" + ("?continue=1" if keep_going else ""), commands)
        for r in res:
            if isinstance(r, dict) and r.get("error") and not keep_going:
                raise MCError(r["error"])
        return res

    # ---- shortcuts ----
    def state(self):
        return self.cmd("state")

    def panel(self, side="active"):
        s = self.state()
        return s[s["active"] if side == "active" else side]

    def key(self, *keys, delay=0):
        """Presses keys, one after the other ("Ctrl+Right", "F5", "Shift+Down")."""
        return self.cmd("key", keys=list(keys), delay=delay)

    def type(self, text):
        return self.cmd("type", text=text)

    def run(self, name, **args):
        return self.cmd("run", name=name, args=args)

    def cd(self, side, path):
        return self.cmd("cd", side=side, path=path)

    def focus(self, side):
        return self.cmd("focus", side=side)

    def cursor(self, side, name=None, index=None):
        return self.cmd("cursor", side=side, name=name, index=index)

    def listing(self, side="active", start=0, count=500):
        return self.cmd("list", side=side, **{"from": start, "count": count})

    def names(self, side="active"):
        return [e["name"] for e in self.listing(side)["entries"]]

    def menu(self, name):
        """Does what choosing the menu item of a cm_* command does (the native menu can't be clicked from outside)."""
        return self.cmd("menu", name=name)

    def dialog(self):
        return self.cmd("dialog")["dialog"]

    def answer(self, button=None, **fields):
        """Answers the open page dialog: presses `button` (its text; "Esc" cancels) after filling fields."""
        return self.cmd("dialog_answer", button=button, fields=fields)

    def popup(self):
        return self.cmd("popup")["popup"]

    def choose(self, which, shift=False):
        """Chooses an item of the open popup list by index or label."""
        args = {"index": which} if isinstance(which, int) else {"label": which}
        return self.cmd("popup_choose", shift=shift, **args)

    def window(self, w, h, x=None, y=None, max=None):
        args = {"w": w, "h": h}
        if max is not None:
            args["max"] = max
        if x is not None:
            args.update(x=x, y=y)
        return self.cmd("window", **args)

    def shot(self, path, delay=0.3, dialogs=None):
        """A PNG of the window. The app makes it on macOS, with its open dialog windows laid over it (dialogs=False
        leaves them out); if that fails (permission), the window alone is made from here."""
        time.sleep(delay)
        path = os.path.abspath(path)
        args = {} if dialogs is None else {"dialogs": dialogs}
        try:
            return self.cmd("screenshot", path=path, **args)
        except MCError as first:
            wid = self.info().get("windowId", 0)
            if not wid or sys.platform != "darwin":
                raise MCError("no screenshot: %s" % first)
            os.makedirs(os.path.dirname(path), exist_ok=True)
            subprocess.run(["screencapture", "-x", "-o", "-l", str(wid), path], check=True)
            return {"ok": True, "path": path}

    def quit(self, timeout=10):
        """Quits the app without questions; a process started by start() that doesn't end is terminated."""
        try:
            self.cmd("quit", timeout=5)
        except (OSError, MCError):
            pass
        if self.proc:
            try:
                self.proc.wait(timeout)
            except subprocess.TimeoutExpired:
                self.proc.terminate()
                try:
                    self.proc.wait(5)
                except subprocess.TimeoutExpired:
                    self.proc.kill()
        if self.file and os.path.basename(self.file).startswith("mc-control-") and os.path.exists(self.file):
            os.remove(self.file)
        if self.own_config and os.path.basename(self.own_config).startswith("mc-config-"):
            shutil.rmtree(self.own_config, ignore_errors=True)

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        self.quit()


def main(argv):
    if len(argv) < 2:
        print(__doc__)
        return 2
    mc = MC()
    a = argv[1]
    if a == "--batch":
        with open(argv[2]) as f:
            print(json.dumps(mc.batch(json.load(f)), indent=1))
        return 0
    if a.lstrip().startswith("{"):
        body = json.loads(a)
        print(json.dumps(mc.cmd(body.pop("cmd"), **body), indent=1))
        return 0
    rest = argv[2:]
    if a == "key":
        res = mc.key(*rest)
    elif a == "run":
        res = mc.run(rest[0])
    elif a == "cd":
        res = mc.cd(rest[0], rest[1])
    elif a == "shot":
        res = mc.shot(rest[0])
    elif a == "type":
        res = mc.type(" ".join(rest))
    elif a == "list":
        res = mc.listing(rest[0] if rest else "active")
    else:
        res = mc.cmd(a)
    print(json.dumps(res, indent=1))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv))
    except MCError as e:
        print("mormicommander:", e, file=sys.stderr)
        sys.exit(1)
