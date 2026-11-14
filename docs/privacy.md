<!--
The privacy statement of mormiCommander: what the app sends over the network (nothing of its own), when it does go on
the network (only when the user asks), and the zero-network tests that check the promise on every build (the Go and page
tests, and the CI job that watches the running app: scripts/zero_network.sh). The README's
"Privacy" section is the short version and links here; frontend/test/readme.test.js keeps the links in place.
-->

# Privacy

mormiCommander is a file manager, and a file manager sees everything on your disk. So the rule is simple: **the app
makes no network connections of its own.**

- No telemetry, no usage statistics, no crash reports.
- No update checks in the background. **Help > Check for updates** opens the release page in your web browser, and
  only when you choose it; the app itself downloads nothing.
- No account, no sign-in, no licence server, no ads.
- No web fonts, scripts or styles from other sites: everything the window draws ships inside the app.

## When the app does go on the network

Only when you ask it to, and only to the place you name:

- **Remote locations you open.** Net > FTP connect (FTP, FTPS), SFTP and WebDAV connect to the server you typed in or
  saved, and only while you use that connection. A network share (SMB, NFS, AFP) that your system has mounted is just a
  folder to the app; the system does the networking for it.
- **AI search, when you start it.** The AI tab of Find files uses the provider you chose in **Configuration > AI
  settings** (issue #146). Either way the model gets **only mormiCommander's own tools**: no shell, no file reading or
  writing, and nothing that deletes, moves or overwrites. Nothing runs unless you start a search there.
  - **Claude Code** (the default): the app runs the `claude` command, already installed and signed in by you. That
    program sends your prompt, and the results of the tools it calls, to its provider; mormiCommander itself connects
    nowhere. The program reaches mormiCommander only through a local endpoint on 127.0.0.1 with a random token, for
    that one run.
  - **OpenRouter, or another OpenAI-compatible API**: the app itself sends HTTPS requests to the API address set in
    the AI settings (`https://openrouter.ai/api/v1` unless you change it; plain http is accepted only for a server on
    your own computer, such as a local model at `http://localhost`). It sends your API key (in the Authorization
    header), the model you chose, a short description of where you are (the folders of both panels, your platform,
    home folder and today's date), your prompt, the list of tools, and, as the model calls them, what the tools answer:
    file and folder names with their paths, sizes and dates. To OpenRouter only, it also names the app (mormiCommander
    and its GitHub page, nothing about you), which OpenRouter shows in its usage lists. What happens to the data there
    is up to that provider's own terms; OpenRouter passes it on to the company behind the model you chose.
  - **The model list** of the AI settings is fetched from the same address (`/models`) only when you press
    **Show models**; opening the settings sends nothing.
- **The OpenRouter API key** is kept in your system's secret store only (the login keychain on macOS, the Credential
  Manager on Windows, the Secret Service through `secret-tool` on Linux), under the name `ai.openrouter` of the
  service "mormiCommander", never in a settings file. The settings dialog can save, replace or remove it, but never
  shows it again. Where there is no secret store (Linux without `secret-tool`), the key can't be saved, and the
  dialog says why.
- **Links you click.** A web address in the About window, the Help menu or a viewed document opens in your web
  browser on a click. The app does not fetch it.

What the app does **not** do, even though it could look like it:

- **Viewing a file never contacts a server.** The Lister (F3) shows HTML and Markdown files with every remote picture,
  style sheet, font, frame and script removed, so a document that points at the web (a tracking pixel in a saved
  page, a badge in a README) stays silent. Pictures embedded in the file itself still show.
- **Thumbnails are made on your machine**, from the files themselves (on macOS, by the system's own QuickLook tool),
  and cached in the app's settings folder.
- **Control mode** (`--control`, off unless you start the app with it) and the hand-over that lets a second start
  open a window in the running app listen on 127.0.0.1 only, so nothing outside your computer can reach them.

## Saved passwords

Passwords of saved connections (Net > FTP connect, Ctrl+F) are never written in plain text, and stay on your computer:

- **By default** they are encrypted with a master password you choose (AES-256-GCM, with a key derived from it by
  PBKDF2-SHA256), in `ftpsites.json` in the settings folder. The master password itself is not stored; it is asked once
  per run, when a saved password is first needed.
- **With "Keep saved passwords in the system keychain"** (in the connection manager, off unless you turn it on) they go
  to your system's own secret store instead: the login keychain on macOS, the Credential Manager on Windows (kept on
  this computer, not roamed), or the Secret Service (GNOME Keyring, KWallet) through `secret-tool` on Linux. The
  settings file then keeps only a reference, never the password. Turning the option on or off moves the passwords
  already saved. The password is handed to the system store on standard input or in memory, never on a command line,
  so other programs can't read it from the process list. A password the keychain no longer has is asked when you
  connect.

A password you type without saving is kept in memory for that session only.

Programs you start from mormiCommander (opening a file with its app, the command line, your editor) are separate
programs with their own rules.

## The zero-network tests

You don't have to take this on trust. These tests run on every build and fail the build if the promise breaks; the
Go and page tests on macOS, Windows and Linux, the test of the whole running app on Linux:

| Test | What it checks |
| --- | --- |
| [`privacy_test.go`](../privacy_test.go) | Reads all of the Go code (its syntax tree, not a text search) and fails on anything that can open a connection, listen on a port or look up a host name outside the FTP, SFTP and WebDAV clients, which only the Net menu starts, the listeners on 127.0.0.1, and the one function of the AI client that sends a request (`send` in `internal/ai/chat_http.go`). It also follows that function back to its only callers: an AI run with the OpenRouter provider, and the model list. |
| [`privacy_session_test.go`](../privacy_session_test.go) | Runs the app's features the way the app builds them, in a temporary folder with settings of its own, through a scripted session: browse folders, view a text file, a Markdown file with remote pictures and an HTML page with remote styles, search by name and by text, copy files, make thumbnails. It records every HTTP request and host-name lookup, and on macOS and Linux it watches the process's open sockets ([`privacy_sockets_unix_test.go`](../privacy_sockets_unix_test.go)); any attempt to reach another machine fails the test. |
| [`frontend/test/privacy.test.js`](../frontend/test/privacy.test.js) | Checks the window's code: no script fetches anything, the pages and styles load nothing from other sites, and the only web addresses are links to the project's own sites. |
| [`frontend/test/viewer-noremote.test.js`](../frontend/test/viewer-noremote.test.js) | Puts Markdown and HTML files full of remote pictures, styles, fonts, frames and scripts through the Lister's own rendering and checks that nothing in the result would load from the network. |
| [`scripts/zero_network.sh`](../scripts/zero_network.sh) and [`scripts/zero_network.py`](../scripts/zero_network.py) (CI job `zero-network` in [`build.yml`](../.github/workflows/build.yml), Linux) | Starts the **built app** with its window on a virtual display, in control mode, with a temporary home and settings folder, and drives a session through the window: browse folders, the Lister on a text file and on Markdown, HTML and SVG files full of remote pictures, styles, scripts, fonts, frames and links, quick view, thumbnails of a folder of pictures, copying a folder, Find files by name and by text, the Options dialog (its AI page too) and the Ask AI dialog, opened and closed without starting a run. Meanwhile `strace` follows the app and every process it starts, the web view's WebKitWebProcess and WebKitNetworkProcess included, and records each `connect`, `sendto`, `sendmsg` and `sendmmsg`; `ss` snapshots every half second are a second witness. Any connection to another machine, anything sent to port 53 (a DNS lookup, even to the local resolver) or a socket open to the network fails the job with a list of the offending connections. Loopback (127.0.0.1, ::1) and Unix sockets (D-Bus, X11, Wayland) are allowed. |

Run them yourself from a copy of the source:

```sh
go test -run ZeroNetwork .
cd frontend && npx vitest run test/privacy.test.js test/viewer-noremote.test.js
# Linux, after scripts/build.sh (needs strace, iproute2, xvfb, xauth, dbus):
scripts/zero_network.sh --selftest && scripts/zero_network.sh build/bin/mormicommander
```

Each test also checks itself on made-up examples (a planted `http.Get`, a real connection to a listener of its own on
127.0.0.1, a remote `<img>`), so a test that stopped looking would fail rather than pass quietly. The whole-app test
starts with `--selftest`: a process two levels down the watched tree makes a connection attempt to 192.0.2.1 (an
address reserved for documentation, which reaches nobody) and sends a DNS query to it, next to a loopback connection
and a Unix socket; the job goes on only if the planted ones are reported, by the right process, and the loopback and
Unix ones are not. Its parser is unit-tested on sample
`strace` and `ss` lines ([`scripts/test_zero_network.py`](../scripts/test_zero_network.py)). Each run keeps its logs and
report as the `zero-network-logs` artifact of the CI run.

### What the tests do not cover

- **The whole-app test runs on Linux only**, where `strace` can follow a process tree. On macOS and Windows the window
  is the same code (the page and the Go side are shared), and the Go and page tests above run there.
- `strace` follows the app and the processes it starts. Services that the session bus starts on the app's behalf are
  not its children; the `ss` snapshots cover them too, but only connections that stay open for half a second or more.
- WebKitGTK's web process sandbox is switched off in the test (CI runners do not allow the user namespaces it needs);
  the app itself never switches it off.
- By design the session does not open remote locations, start an AI run, click links or choose Check for updates:
  those are the cases above where you ask for the network.

Found something that goes on the network when it should not? Please
[open an issue](https://github.com/mormitech/mormicommander/issues).
