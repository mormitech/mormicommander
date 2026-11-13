// Package ftp is mormiCommander's FTP client engine, the part of Total
// Commander's "FTP connect" feature that has nothing to do with windows and
// buttons. It opens a control connection (plain, explicit TLS through AUTH
// TLS, or implicit TLS on port 990), logs in (anonymously or with a user
// name, optionally through a SOCKS, HTTP CONNECT or FTP-style proxy), and
// then offers everything a file panel needs: listing a folder with parsers
// for the many listing dialects real servers send (MLSD, UNIX ls, DOS/IIS,
// VMS, EPLF, OS/400 and a bare-name fallback), changing folders, downloading
// and uploading with resume, renaming, deleting, creating folders, SITE
// CHMOD, binary/ASCII/automatic transfer modes, keepalive NOOPs while the
// panel sits idle, and whole-tree helpers for copying folders.
//
// The package also holds the data model of the connection manager (saved
// connections in folders, passwords sealed with a master password so they
// are never written in plain text, an importer for Total Commander's
// wcx_ftp.ini) and a small connection pool for parallel transfers. It has no
// UI and no knowledge of the panel VFS; the VFS layer wraps a *Client and the
// dialogs edit a *Store. Everything is covered by tests that run against a
// tiny FTP server started on 127.0.0.1 inside the test itself.
package ftp
