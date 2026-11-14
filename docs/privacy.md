<!--
The privacy statement of mormiCommander: what the app sends over the network (nothing of its own), when it does go on
the network (only when the user asks), and the zero-network tests that check the promise on every build. The README's
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
- **AI search, when you start it.** The AI tab of Find files runs the AI program you chose (step 1: the Claude Code
  command, `claude`, already installed and signed in by you). That program sends your prompt, and the results of the
  tools it calls (file names, sizes, dates), to its provider; mormiCommander itself still connects nowhere. The program
  reaches mormiCommander only through a local endpoint on 127.0.0.1 with a random token, for that one run, and gets
  **only mormiCommander's own tools**: no shell, no file reading or writing, and nothing that deletes, moves or
  overwrites (issue #146). Nothing runs unless you start a search there.
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

You don't have to take this on trust. These tests run on every build, on macOS, Windows and Linux, and fail the build
if the promise breaks:

| Test | What it checks |
| --- | --- |
| [`privacy_test.go`](../privacy_test.go) | Reads all of the Go code (its syntax tree, not a text search) and fails on anything that can open a connection, listen on a port or look up a host name outside the FTP, SFTP and WebDAV clients, which only the Net menu starts, and the two listeners on 127.0.0.1. |
| [`privacy_session_test.go`](../privacy_session_test.go) | Runs the app's features the way the app builds them, in a temporary folder with settings of its own, through a scripted session: browse folders, view a text file, a Markdown file with remote pictures and an HTML page with remote styles, search by name and by text, copy files, make thumbnails. It records every HTTP request and host-name lookup, and on macOS and Linux it watches the process's open sockets ([`privacy_sockets_unix_test.go`](../privacy_sockets_unix_test.go)); any attempt to reach another machine fails the test. |
| [`frontend/test/privacy.test.js`](../frontend/test/privacy.test.js) | Checks the window's code: no script fetches anything, the pages and styles load nothing from other sites, and the only web addresses are links to the project's own sites. |
| [`frontend/test/viewer-noremote.test.js`](../frontend/test/viewer-noremote.test.js) | Puts Markdown and HTML files full of remote pictures, styles, fonts, frames and scripts through the Lister's own rendering and checks that nothing in the result would load from the network. |

Run them yourself from a copy of the source:

```sh
go test -run ZeroNetwork .
cd frontend && npx vitest run test/privacy.test.js test/viewer-noremote.test.js
```

Each test also checks itself on made-up examples (a planted `http.Get`, a real connection to a listener of its own on
127.0.0.1, a remote `<img>`), so a test that stopped looking would fail rather than pass quietly.

### What the tests do not cover yet

The run-time test drives the app's Go side directly; it does not start the whole app with its window. A test that
launches the built app in control mode, runs the same session through the window and watches the whole process
(including the web view) for outbound connections is planned. Until then the window's side is covered by reading its
code (`privacy.test.js`) and by running the Lister's rendering on hostile files (`viewer-noremote.test.js`).

Found something that goes on the network when it should not? Please
[open an issue](https://github.com/mormitech/mormicommander/issues).
