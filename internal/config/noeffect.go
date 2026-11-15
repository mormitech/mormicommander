// The options of the Options dialog that are kept in the settings file but that the program does not act on yet
// (board issue #94). Most come from Total Commander's dialog, so a wincmd.ini copied over keeps its values, but the
// feature behind them is not there yet, or works without the option. The dialog marks these options as having no
// effect yet, and leaves them out with Misc.HideNoEffect. Their values are still loaded and saved as before.
//
// The list is checked by options_audit_test.go in the main package: an option the page or the Go code reads must not
// be here, and an option nobody reads must be. So when an option gets wired up, take it off this list; when a new
// option is added before its feature, put it on.

package config

// noEffect holds the "Page.Field" names of the options with no effect yet.
var noEffect = map[string]bool{
	"Layout.DriveBar2":     true,
	"Layout.DriveCombo":    true,
	"Layout.BreadCrumbBar": true,
	"Layout.MenuBar":       true,
	"Layout.SeparateTree":  true,

	"Display.ShowSystem":       true,
	"Display.SizeFooter":       true,
	"Display.ShowParentInRoot": true,
	"Display.FullRowSelect":    true,
	"Display.LongNames":        true,

	"Icons.ShowIcons":           true,
	"Icons.IconOverlays":        true,
	"Icons.IconSize":            true,
	"Icons.IconsSpecialFolders": true,
	"Icons.IconsInMenus":        true,
	"Icons.IconsOnNet":          true,
	"Icons.ExeIcons":            true,

	"Font.WindowFont":  true,
	"Font.WindowSize":  true,
	"Font.ListerFont":  true,
	"Font.ZoomPercent": true,

	"Colors.Scheme": true,

	"Language.LanguageFile": true,
	"Language.UILanguage":   true,

	"Operation.OnlyOnce":       true,
	"Operation.MouseMode":      true,
	"Operation.SavePath":       true,
	"Operation.SaveHistory":    true,
	"Operation.SaveCommands":   true,
	"Operation.SavePanels":     true,
	"Operation.SaveOnExit":     true,
	"Operation.MinimizeToTray": true,

	"ViewerEditor.UseExtViewer":   true,
	"ViewerEditor.EditorWait":     true,
	"ViewerEditor.ViewerByType":   true,
	"ViewerEditor.EditTempUpload": true,

	"CopyDelete.CopyComments":        true,
	"CopyDelete.BigFileThresholdMB":  true,
	"CopyDelete.MoveDeleteAfterCopy": true,

	"Refresh.RefreshRemovabl": true,
	"Refresh.IntervalMs":      true,

	"QuickSearch.ShowDropDown": true,

	"FTP.Passive":        true,
	"FTP.ShowHidden":     true,
	"FTP.TimeoutSec":     true,
	"FTP.Retries":        true,
	"FTP.RetryDelaySec":  true,
	"FTP.DownloadDir":    true,
	"FTP.AnonymousEmail": true,
	"FTP.KeepAliveSec":   true,
	"FTP.AsciiMasks":     true,
	"FTP.ResumeDefault":  true,

	"Proxy.Type": true,
	"Proxy.Host": true,
	"Proxy.Port": true,
	"Proxy.User": true,

	"Plugins.UseExternalTools": true,
	"Plugins.SevenZipPath":     true,
	"Plugins.UnrarPath":        true,
	"Plugins.ExifToolPath":     true,
	"Plugins.FFprobePath":      true,

	"Thumbnails.Height":      true,
	"Thumbnails.Cache":       true,
	"Thumbnails.CacheDir":    true,
	"Thumbnails.CacheMaxMB":  true,
	"Thumbnails.ShowName":    true,
	"Thumbnails.FileTypes":   true,
	"Thumbnails.MaxFileMB":   true,
	"Thumbnails.ShowOnNet":   true,
	"Thumbnails.FolderThumb": true,

	"Log.Copy":       true,
	"Log.Delete":     true,
	"Log.Rename":     true,
	"Log.MkDir":      true,
	"Log.Pack":       true,
	"Log.FTP":        true,
	"Log.ErrorsOnly": true,

	"IgnoreList.Enabled": true,
	"IgnoreList.File":    true,

	"Packer.InternalZip":    true,
	"Packer.ExternalZip":    true,
	"Packer.TarOwner":       true,
	"Packer.MoveToArchive":  true,
	"Packer.SeparateFiles":  true,
	"Packer.ArchiveSubdirs": true,
	"Packer.OpenFolderLike": true,
	"Packer.SearchInside":   true,

	"Zip.Level":     true,
	"Zip.UTF8Names": true,
	"Zip.SetTimes":  true,
	"Zip.TempDir":   true,
	"Zip.AES":       true,
	"Zip.Zip64":     true,

	"Misc.MainMenuFile": true,
	"Misc.SoundOnDone":  true,
	"Misc.CheckUpdates": true,
	"Misc.BeepOnError":  true,

	"FolderTabs.CtrlUpNewTab": true,

	"CustomColumns.ShowInShowMenu":   true,
	"CustomColumns.EqualWidthOnOpen": true,

	"BackgroundQueue.MaxParallel":  true,
	"BackgroundQueue.AutoStart":    true,
	"BackgroundQueue.ShowWindow":   true,
	"BackgroundQueue.KeepFinished": true,

	"Hotlist.AddAtTop":      true,
	"Hotlist.HistoryPerTab": true,

	"ViewModes.AutoSwitch": true,

	"Tooltips.Show":      true,
	"Tooltips.DelayMs":   true,
	"Tooltips.ByType":    true,
	"Tooltips.ShowOnNet": true,

	"Lister.AutoDetect":   true,
	"Lister.UseImages":    true,
	"Lister.UsePDF":       true,
	"Lister.MaxTextMB":    true,
	"Lister.ExtViewerFor": true,

	"DragDrop.RightDragMenu": true,
	"DragDrop.MiddleClick":   true,
	"DragDrop.ThresholdPx":   true,

	"Tree.FollowFolder": true,
	"Tree.ShowHidden":   true,
	"Tree.ShowFiles":    true,
	"Tree.AutoExpand":   true,
	"Tree.ShowNetwork":  true,

	"FindFiles.UseIndex": true,
	"FindFiles.Encoding": true,
}

// HasNoEffect says whether the program does not act on the option yet.
func HasNoEffect(name string) bool {
	f, ok := LookupSetting(name)
	return ok && f.NoEffect
}
