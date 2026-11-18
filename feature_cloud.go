package main

// Cloud placeholder awareness (board #30, F-873): iCloud Drive and OneDrive (and other File Provider / Cloud Files)
// files whose contents are only in the cloud. internal/cloudfile reads the system's own mark from the stat data a
// listing already has (macOS SF_DATALESS, Windows RECALL_ON_DATA_ACCESS / RECALL_ON_OPEN / OFFLINE), so knowing it
// never downloads anything. What this adds, all opt-in:
//   - the entry flag vfs.FlagCloud the page shows as a mark after the name (Display.CloudMark, off by default) and the
//     custom column field [=mc.cloud] ("online-only");
//   - Operation.CloudNoDownload (off by default): content search, duplicate search by content (find.Options.SkipCloud),
//     thumbnails and the media column fields (image size, EXIF date, audio tags) leave such files unread, so they
//     are not downloaded by the hundred. Folder sizes never read file contents, so they need no option.
// Opening or copying such a file is left to the system, which downloads it on the first read as before.

import (
	"mormicommander/internal/cloudfile"
	lst "mormicommander/internal/listing"
)

func init() {
	lst.RegisterField("mc", lst.Field{Name: "cloud", Kind: lst.KindText, Help: "\"online-only\" for an iCloud Drive / OneDrive file that is not downloaded",
		Get: func(it *lst.Item, _ string, _ *lst.Format) lst.Value {
			if it.IsUp() || it.Attr&lst.AttrCloud == 0 {
				return lst.TextValue("")
			}
			return lst.TextValue("online-only")
		}})
	lst.SkipCloudContent = cloudNoDownload
}

// cloudNoDownload reports whether Options > Operation asks not to download online-only cloud files for searches,
// thumbnails and column fields.
func cloudNoDownload() bool {
	s, ok := currentSettings()
	return ok && s.Operation.CloudNoDownload
}

// cloudKeepDownloaded drops the online-only cloud placeholders from paths when skip is on (thumbnails).
func cloudKeepDownloaded(paths []string, skip bool) []string {
	if !skip {
		return paths
	}
	out := paths[:0:0]
	for _, p := range paths {
		if !cloudfile.PathPlaceholder(p) {
			out = append(out, p)
		}
	}
	return out
}
