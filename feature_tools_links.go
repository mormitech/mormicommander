package main

// Creating links (F-178, F-179, F-337): symbolic links, hard links, Windows junctions and shortcut files (.url,
// .webloc, .desktop) over internal/links. cm_CreateSymbolicLink and cm_CreateHardLink open TC's "link name / points to"
// dialog for one item; Ctrl+Shift+F5 (cm_CreateShortcut) makes one link per selected item in the target panel.

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"mormicommander/internal/links"
)

// ToolsLinkKind is one kind of link the dialogs offer.
type ToolsLinkKind struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Ext   string `json:"ext"` // the file extension of a shortcut file ("" for links)
}

var toolsLinkLabels = map[links.Kind]string{
	links.KindSymlink: "Symbolic link", links.KindHardLink: "Hard link", links.KindJunction: "Junction (folders only)",
	links.KindURL: "Shortcut file (.url)", links.KindWebloc: "Web location file (.webloc)", links.KindDesktop: "Desktop link file (.desktop)",
}

// LinkKinds lists the kinds of link this system can make, in dialog order.
func (t *Tools) LinkKinds() []ToolsLinkKind {
	out := []ToolsLinkKind{}
	for _, k := range links.SupportedKinds(runtime.GOOS) {
		out = append(out, ToolsLinkKind{Kind: k.String(), Label: toolsLinkLabels[k], Ext: links.ShortcutExt(k)})
	}
	return out
}

func toolsLinkKind(s string) (links.Kind, error) {
	for _, k := range links.SupportedKinds(runtime.GOOS) {
		if k.String() == s {
			return k, nil
		}
	}
	return links.KindNone, errors.New("This kind of link can't be made on this system.")
}

// toolsLinkErr explains why a link could not be made.
func toolsLinkErr(err error) string {
	switch {
	case errors.Is(err, links.ErrPrivilege):
		return "Windows allows symbolic links only with Developer Mode on (Settings > For developers) or for an administrator."
	case errors.Is(err, links.ErrIsDir):
		return "A hard link can't point to a folder. Make a symbolic link instead."
	case errors.Is(err, links.ErrCrossDevice):
		return "A hard link must be on the same drive as its file. Make a symbolic link instead."
	case errors.Is(err, links.ErrSelfLink):
		return "The link would replace the file it points to, so it was not made."
	case errors.Is(err, links.ErrUnsupported):
		return "This kind of link can't be made here."
	}
	return toolsErrText(err)
}

// ToolsLink asks for one link.
type ToolsLink struct {
	Kind      string `json:"kind"`
	Target    string `json:"target"` // the existing file or folder the link points to
	Link      string `json:"link"`   // the new link's full path
	Relative  bool   `json:"relative"`
	Overwrite bool   `json:"overwrite"`
}

// ToolsLinkResult is the outcome for one link.
type ToolsLinkResult struct {
	Source string `json:"source"`
	Link   string `json:"link"`
	Error  string `json:"error"`
	Exists bool   `json:"exists"` // a file is already there under the link's name (nothing was changed)
}

// CreateLink makes one link: Link will point to Target.
func (t *Tools) CreateLink(r ToolsLink) (ToolsLinkResult, error) {
	res := ToolsLinkResult{Source: r.Target, Link: r.Link}
	if strings.TrimSpace(r.Link) == "" {
		return res, errors.New("Type a name for the link.")
	}
	if err := t.toolsLocal(r.Target); err != nil {
		return res, err
	}
	if err := t.toolsLocalDir(filepath.Dir(r.Link)); err != nil {
		return res, err
	}
	k, err := toolsLinkKind(r.Kind)
	if err != nil {
		return res, err
	}
	err = links.Create(k, r.Target, r.Link, links.CreateOptions{Relative: r.Relative, Overwrite: r.Overwrite})
	if errors.Is(err, fs.ErrExist) {
		res.Exists = true
		return res, nil
	}
	if err != nil {
		res.Error = toolsLinkErr(err)
	}
	return res, nil
}

// ToolsLinks asks for one link per source, all in one folder.
type ToolsLinks struct {
	Kind      string   `json:"kind"`
	Sources   []string `json:"sources"`
	DestDir   string   `json:"destDir"`
	Relative  bool     `json:"relative"`
	Overwrite bool     `json:"overwrite"`
}

// CreateLinks makes a link for every source in DestDir, named like the source (plus the shortcut extension).
func (t *Tools) CreateLinks(id string, r ToolsLinks) ([]ToolsLinkResult, error) {
	if err := t.toolsLocal(r.Sources...); err != nil {
		return nil, err
	}
	if err := t.toolsLocalDir(r.DestDir); err != nil {
		return nil, err
	}
	k, err := toolsLinkKind(r.Kind)
	if err != nil {
		return nil, err
	}
	ctx, end := t.toolsBegin(id)
	defer end()
	out := []ToolsLinkResult{}
	for _, res := range links.CreateLinks(ctx, links.BatchRequest{Kind: k, Sources: r.Sources, DestDir: r.DestDir,
		Options: links.CreateOptions{Relative: r.Relative, Overwrite: r.Overwrite}}) {
		lr := ToolsLinkResult{Source: res.Source, Link: res.Link}
		switch {
		case res.Err == nil:
		case errors.Is(res.Err, context.Canceled):
			lr.Error = "Stopped."
		case errors.Is(res.Err, fs.ErrExist):
			lr.Exists = true
		default:
			lr.Error = toolsLinkErr(res.Err)
		}
		out = append(out, lr)
	}
	return out, nil
}
