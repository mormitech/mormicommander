package main

// Editing a file on a server (F4 on an FTP, SFTP or plugin file system entry; FEATURES.md F-593, TC's "download, edit,
// upload"). The file is downloaded to a temporary copy, the copy opens in the editor, and the copy is then watched by
// the same watcher as archive entries (feature_archive_edit.go): each save sends "archive-edit-changed" with Remote
// set, the page asks whether to upload the new version (Options > Viewer/Editor > "Ask before uploading an edited
// remote file"), and ListerEdit.AnswerArchiveEdit uploads it over the file on the server. The server is only contacted
// because the user pressed F4 and answered the question (zero-network rule).
//
// Before this F4 on a server file said that editing on servers was not ready yet, so nothing that worked before
// behaves differently.

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"mormicommander/internal/archive"
	"mormicommander/internal/launch"
	"mormicommander/internal/vfs"
)

// vfsServerKind says whether the panel path p is a file on a server (FTP, SFTP or a plugin file system), not in an
// archive and not on the local disk.
func vfsServerKind(r *vfs.Router, p string) bool {
	if r == nil {
		return false
	}
	switch r.Kind(p) {
	case vfs.KindFTP, vfs.KindSFTP, vfs.KindOther:
		return true
	}
	return false
}

// serverEditPrepare downloads the server file path to a temporary copy for the editor; folder is the server folder
// it is in (for the question and for reading the panels again).
func serverEditPrepare(r *vfs.Router, path string) (local, folder string, err error) {
	path = r.Clean(path)
	e, err := r.Stat(path)
	if err != nil {
		return "", "", errors.New(listError(path, err))
	}
	if e.Flags&vfs.FlagDir != 0 {
		return "", "", fmt.Errorf("%s is a folder, not a file.", e.Name)
	}
	folder = r.Parent(path)
	dir, err := vfsTempDir("edit")
	if err != nil {
		return "", "", err
	}
	name := e.Name
	if name == "" {
		name = vfsBase(path)
	}
	local = filepath.Join(dir, archive.SanitizeName(name))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := r.Download(ctx, path, local, vfs.TransferOptions{PreserveTime: true}); err != nil {
		return "", "", fmt.Errorf("Can't download %s: %s", name, vfsErrText(err))
	}
	return local, folder, nil
}

// editOnServer downloads a server file, opens the copy in the editor and watches it, so each save asks whether to
// upload it.
func (e *ListerEdit) editOnServer(r *vfs.Router, path, tool string, ctx launch.ParamContext) (ListerEditResult, error) {
	local, folder, err := serverEditPrepare(r, path)
	if err != nil {
		return ListerEditResult{}, err
	}
	w := arcEditWatcherOf(e.app, r)
	id, err := w.addRemote(r.Clean(path), folder, local)
	if err != nil {
		return ListerEditResult{}, fmt.Errorf("Can't watch the copy of %s for changes: %v", filepath.Base(local), err)
	}
	if err := e.launch.OpenInTool(launch.ParseTool(tool), local, ctx); err != nil {
		w.remove(id)
		return ListerEditResult{}, listerLaunchError("the editor", filepath.Base(local), err)
	}
	w.start()
	return ListerEditResult{Watching: true, Remote: true}, nil
}
