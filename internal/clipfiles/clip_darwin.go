// This file is the macOS clipboard backend. It talks to NSPasteboard through osascript running JavaScript for
// Automation, which reaches AppKit through the Objective-C bridge without cgo. Files are written as NSURL objects,
// which gives exactly the types Finder writes (public.file-url and the legacy NSFilenamesPboardType), so Cmd+V in
// Finder pastes them and files copied in Finder paste into a panel. Finder itself has no "cut" on the pasteboard (it
// moves with Cmd+Option+V instead), so a cut is marked with a private pasteboard type that only mormiCommander reads;
// Finder sees an ordinary copy, which is the safe reading. Requests and replies travel as JSON on stdin and stdout, so
// paths and text never become part of the script, and non-ASCII output is \u-escaped so the console encoding cannot
// garble it. A named pasteboard can be used instead of the general one, which lets the tests run the real script
// without touching the user's clipboard.

package clipfiles

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// cutType is the private pasteboard type that marks a cut.
const cutType = "org.mormicommander.drop-effect"

const jxaScript = `ObjC.import('AppKit');
function isNil(x) { return x === undefined || x === null || (typeof x.isNil === 'function' && x.isNil()); }
function str(x) { return isNil(x) ? null : ObjC.unwrap(x); }
function run() {
  var d = $.NSFileHandle.fileHandleWithStandardInput.readDataToEndOfFile;
  var q = JSON.parse(ObjC.unwrap($.NSString.alloc.initWithDataEncoding(d, $.NSUTF8StringEncoding)));
  var pb = q.board ? $.NSPasteboard.pasteboardWithName($(q.board)) : $.NSPasteboard.generalPasteboard;
  var r = {};
  switch (q.op) {
  case 'writeFiles':
    pb.clearContents;
    var urls = q.paths.map(function (p) { return $.NSURL.fileURLWithPath($(p)); });
    if (!pb.writeObjects($(urls))) throw new Error('the pasteboard refused the file list');
    pb.setStringForType($(q.names), $.NSPasteboardTypeString);
    pb.setStringForType($(q.effect), $('` + cutType + `'));
    break;
  case 'readFiles':
    var opts = $.NSDictionary.dictionaryWithObjectForKey($.NSNumber.numberWithBool(true),
      $('NSPasteboardURLReadingFileURLsOnlyKey'));
    var arr = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), opts);
    r.paths = [];
    if (!isNil(arr)) {
      for (var i = 0; i < arr.count; i++) {
        var u = arr.objectAtIndex(i);
        var fp = u.filePathURL;
        r.paths.push(ObjC.unwrap((isNil(fp) ? u : fp).path));
      }
    }
    r.effect = str(pb.stringForType($('` + cutType + `'))) || 'copy';
    break;
  case 'writeText':
    pb.clearContents;
    if (!pb.setStringForType($(q.text), $.NSPasteboardTypeString)) throw new Error('the pasteboard refused the text');
    break;
  case 'readText':
    r.text = str(pb.stringForType($.NSPasteboardTypeString));
    break;
  case 'writeImage':
    pb.clearContents;
    var data = $.NSData.alloc.initWithBase64EncodedStringOptions($(q.png), 0);
    if (isNil(data) || !pb.setDataForType(data, $('public.png'))) throw new Error('the pasteboard refused the image');
    break;
  case 'readImage':
    var png = pb.dataForType($('public.png'));
    if (isNil(png)) {
      var tiff = pb.dataForType($('public.tiff'));
      if (!isNil(tiff)) {
        var rep = $.NSBitmapImageRep.imageRepWithData(tiff);
        if (!isNil(rep)) png = rep.representationUsingTypeProperties(4, $.NSDictionary.dictionary);
      }
    }
    r.png = isNil(png) ? null : ObjC.unwrap(png.base64EncodedStringWithOptions(0));
    break;
  case 'types':
    r.types = isNil(pb.types) ? [] : ObjC.deepUnwrap(pb.types);
    break;
  case 'clear':
    pb.clearContents;
    break;
  case 'release':
    pb.releaseGlobally;
    break;
  default:
    throw new Error('unknown request ' + q.op);
  }
  return JSON.stringify(r).replace(/[\u007f-￿]/g, function (c) {
    return '\\u' + ('0000' + c.charCodeAt(0).toString(16)).slice(-4);
  });
}`

// jxaRequest is one call into the script.
type jxaRequest struct {
	Op     string   `json:"op"`
	Board  string   `json:"board,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Names  string   `json:"names,omitempty"`
	Effect string   `json:"effect,omitempty"`
	Text   string   `json:"text"`
	PNG    string   `json:"png,omitempty"`
}

// jxaReply is the script's answer. Pointers tell "absent" from "empty".
type jxaReply struct {
	Paths  []string `json:"paths"`
	Effect string   `json:"effect"`
	Text   *string  `json:"text"`
	PNG    *string  `json:"png"`
	Types  []string `json:"types"`
}

// jxaBackend reaches NSPasteboard through osascript. board names a private pasteboard; empty means the general one.
type jxaBackend struct {
	r     runner
	board string
}

func newSystemBackend() backend { return &jxaBackend{r: execRunner{}} }

func (j *jxaBackend) call(q jxaRequest) (jxaReply, error) {
	q.Board = j.board
	in, err := json.Marshal(q)
	if err != nil {
		return jxaReply{}, err
	}
	out, err := j.r.output(in, "osascript", "-l", "JavaScript", "-e", jxaScript)
	if err != nil {
		return jxaReply{}, err
	}
	var r jxaReply
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &r); err != nil {
		return jxaReply{}, fmt.Errorf("osascript: unexpected reply %q", out)
	}
	return r, nil
}

func (j *jxaBackend) writeFiles(paths []string, op Op) error {
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p) // Finder also adds the bare names as text, for pasting into an editor
	}
	_, err := j.call(jxaRequest{Op: "writeFiles", Paths: paths, Names: strings.Join(names, "\n"), Effect: op.String()})
	return err
}

func (j *jxaBackend) readFiles() ([]string, Op, error) {
	r, err := j.call(jxaRequest{Op: "readFiles"})
	if err != nil {
		return nil, Copy, err
	}
	if len(r.Paths) == 0 {
		return nil, Copy, ErrEmpty
	}
	op := Copy
	if r.Effect == "cut" {
		op = Cut
	}
	return r.Paths, op, nil
}

func (j *jxaBackend) writeText(s string) error {
	_, err := j.call(jxaRequest{Op: "writeText", Text: s})
	return err
}

func (j *jxaBackend) readText() (string, error) {
	r, err := j.call(jxaRequest{Op: "readText"})
	if err != nil {
		return "", err
	}
	if r.Text == nil {
		return "", ErrEmpty
	}
	return *r.Text, nil
}

func (j *jxaBackend) writeImage(png []byte) error {
	_, err := j.call(jxaRequest{Op: "writeImage", PNG: base64.StdEncoding.EncodeToString(png)})
	return err
}

func (j *jxaBackend) readImage() ([]byte, error) {
	r, err := j.call(jxaRequest{Op: "readImage"})
	if err != nil {
		return nil, err
	}
	if r.PNG == nil || *r.PNG == "" {
		return nil, ErrEmpty
	}
	data, err := base64.StdEncoding.DecodeString(*r.PNG)
	if err != nil {
		return nil, errors.New("osascript: bad image data")
	}
	return data, nil
}

func (j *jxaBackend) content() (Content, error) {
	r, err := j.call(jxaRequest{Op: "types"})
	if err != nil {
		return Content{}, err
	}
	return Content{
		Files: slices.Contains(r.Types, "public.file-url"),
		Text:  slices.Contains(r.Types, "public.utf8-plain-text"),
		Image: slices.Contains(r.Types, "public.png") || slices.Contains(r.Types, "public.tiff"),
		Types: r.Types,
	}, nil
}

func (j *jxaBackend) clear() error {
	_, err := j.call(jxaRequest{Op: "clear"})
	return err
}

// release frees a named pasteboard. Tests call it so they leave nothing behind.
func (j *jxaBackend) release() error {
	_, err := j.call(jxaRequest{Op: "release"})
	return err
}
