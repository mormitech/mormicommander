// The typed settings model behind the Configuration dialog (section 17 of
// FEATURES.md). Every page of the dialog is one struct, and every option on
// the page is one field whose struct tag says where it lives in the INI file
// (ini:"Section:Key"), its default (def), a label for the dialog and its
// search box (label), and for numbers the allowed range (min, max) and named
// choices (opts). Where Total Commander has an equivalent key we use its
// exact section and key name, so a copied wincmd.ini keeps working; the rest
// follow the same naming style. One reflection-driven loader and saver works
// for all pages: loading starts from the defaults and takes every key found
// in the file (bad values are reported and ignored, numbers are clamped), and
// saving writes a key only if it is already in the file or differs from its
// default, so a hand-written ini doesn't fill up with noise. Fields() exposes
// the metadata so the UI can build the pages, the dialog's search box can
// filter them (F-827), and control mode can read and write any option by its
// "Page.Field" name. Lists that don't fit one key (colours by type, hotlist,
// columns, view modes...) have their own files.

package config

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Settings is the whole Configuration dialog.
type Settings struct {
	Layout          LayoutPage          `page:"Layout"`
	Display         DisplayPage         `page:"Display"`
	Icons           IconsPage           `page:"Icons"`
	Font            FontPage            `page:"Font"`
	Colors          ColorsPage          `page:"Colors"`
	Language        LanguagePage        `page:"Language"`
	Operation       OperationPage       `page:"Operation"`
	ViewerEditor    ViewerEditorPage    `page:"Viewer/Editor"`
	CopyDelete      CopyDeletePage      `page:"Copy/Delete"`
	Refresh         RefreshPage         `page:"Refresh"`
	QuickSearch     QuickSearchPage     `page:"Quick search"`
	FTP             FTPPage             `page:"FTP"`
	Proxy           ProxyPage           `page:"FTP proxy"`
	Plugins         PluginsPage         `page:"Plugins / external tools"`
	Thumbnails      ThumbnailsPage      `page:"Thumbnails"`
	Log             LogPage             `page:"Log file"`
	IgnoreList      IgnoreListPage      `page:"Ignore list"`
	Packer          PackerPage          `page:"Packer"`
	Zip             ZipPage             `page:"Zip packer"`
	Misc            MiscPage            `page:"Misc"`
	FolderTabs      FolderTabsPage      `page:"Folder tabs"`
	CustomColumns   CustomColumnsPage   `page:"Custom columns"`
	FavoriteTabs    FavoriteTabsPage    `page:"Favorite tabs"`
	BackgroundQueue BackgroundQueuePage `page:"Background transfer"`
	Hotlist         HotlistPage         `page:"Directory hotlist"`
	ViewModes       ViewModesPage       `page:"View modes"`
	Tooltips        TooltipsPage        `page:"Tooltips"`
	Lister          ListerPage          `page:"Lister"`
	DragDrop        DragDropPage        `page:"Drag & drop and mouse"`
	Tree            TreePage            `page:"Tree"`
	FindFiles       FindFilesPage       `page:"Find files"`
	AI              AIPage              `page:"AI"`
}

// LayoutPage: which parts of the main window are visible (F-413).
type LayoutPage struct {
	ButtonBar         bool `ini:"Layout:ButtonBar" def:"1" label:"Show button bar"`
	ButtonBarVertical bool `ini:"Layout:ButtonBarVertical" def:"0" label:"Show vertical button bar"`
	DriveBar1         bool `ini:"Layout:DriveBar1" def:"1" label:"Show drive buttons"`
	DriveBar2         bool `ini:"Layout:DriveBar2" def:"1" label:"Two drive button bars"`
	DriveBarFlat      bool `ini:"Layout:DriveBarFlat" def:"1" label:"Flat drive buttons"`
	InterfaceFlat     bool `ini:"Layout:InterfaceFlat" def:"1" label:"Flat interface"`
	DriveCombo        bool `ini:"Layout:DriveCombo" def:"1" label:"Show drive combo box"`
	CurDir            bool `ini:"Layout:CurDir" def:"1" label:"Show current folder line"`
	BreadCrumbBar     bool `ini:"Layout:BreadCrumbBar" def:"1" label:"Breadcrumb path bar"`
	TabHeader         bool `ini:"Layout:TabHeader" def:"1" label:"Show sort header"`
	StatusBar         bool `ini:"Layout:StatusBar" def:"1" label:"Show status bar"`
	CmdLine           bool `ini:"Layout:CmdLine" def:"1" label:"Show command line"`
	KeyButtons        bool `ini:"Layout:KeyButtons" def:"1" label:"Show function key buttons"`
	HistoryHotlist    bool `ini:"Layout:HistoryHotlistButtons" def:"1" label:"Show history and hotlist buttons"`
	FreeSpace         bool `ini:"Layout:FreeSpace" def:"1" label:"Show free space in drive combo"`
	HiddenMarker      bool `ini:"Layout:HiddenMarker" def:"1" label:"Show \"Hidden: shown\" in the status bar while hidden files are shown"`
	MenuBar           bool `ini:"Layout:MenuBar" def:"1" label:"Show main menu bar"`
	Horizontal        bool `ini:"Layout:HorizontalPanels" def:"0" label:"Panels above each other"`
	SeparateTree      int  `ini:"Configuration:SeparateTree" def:"0" min:"0" max:"2" opts:"0=Off|1=One tree for both panels|2=One tree per panel" label:"Separate folder tree"`
}

// DisplayPage: how file lists look (F-414). DirBrackets is on by default in TC; ours is off, the folder icon already
// tells (docs/spec/area-2.md).
type DisplayPage struct {
	ShowHidden         bool   `ini:"Configuration:ShowHiddenSystem" def:"0" label:"Show hidden files"`
	ShowSystem         bool   `ini:"Configuration:ShowSystemFiles" def:"0" label:"Show system files"`
	DirBrackets        bool   `ini:"Configuration:DirBrackets" def:"0" label:"Square brackets around folder names"`
	SeparateExt        bool   `ini:"Configuration:SeparateExt" def:"1" label:"Show extension in its own column"`
	SortDirsByName     bool   `ini:"Configuration:SortDirsByName" def:"1" label:"Always sort folders by name"`
	SortUpper          bool   `ini:"Configuration:SortUpper" def:"0" label:"Case-sensitive sorting"`
	SortNatural        bool   `ini:"Configuration:SortNatural" def:"1" label:"Natural sorting (numbers by value)"`
	SizeStyle          int    `ini:"Configuration:SizeStyle" def:"0" min:"0" max:"4" opts:"0=Bytes|1=kB|2=MB|3=GB|4=Dynamic" label:"File size format"`
	SizeFooter         int    `ini:"Configuration:SizeFooter" def:"4" min:"0" max:"4" opts:"0=Bytes|1=kB|2=MB|3=GB|4=Dynamic" label:"Size format in status bar"`
	DateFormat         string `ini:"Configuration:DateFormat" def:"" label:"Date format (empty = system)"`
	Time24             bool   `ini:"Configuration:Time24" def:"1" label:"24-hour clock"`
	ShowSeconds        bool   `ini:"Configuration:ShowSeconds" def:"0" label:"Show seconds in file times"`
	ShowParentInRoot   bool   `ini:"Configuration:ShowParentDirInRoot" def:"0" label:"Show .. in root folders"`
	FullRowSelect      bool   `ini:"Configuration:FullRowSelect" def:"1" label:"Cursor bar across all columns"`
	HoverHighlight     bool   `ini:"Configuration:HoverHighlight" def:"0" label:"Highlight the row under the mouse"`
	ThousandsSeparator bool   `ini:"Configuration:ThousandsSeparator" def:"1" label:"Thousands separator in sizes"`
	LongNames          bool   `ini:"Configuration:UseLongNames" def:"1" label:"Show long names"`
}

// IconsPage: file icons (F-415).
type IconsPage struct {
	ShowIcons           int  `ini:"Configuration:Showicons" def:"2" min:"0" max:"2" opts:"0=None|1=Standard only|2=All, including file icons" label:"Show icons"`
	IconOverlays        bool `ini:"Configuration:IconOverlays" def:"1" label:"Show overlay icons (links, cloud state)"`
	IconSize            int  `ini:"Configuration:IconSize" def:"16" min:"16" max:"48" opts:"16=16|24=24|32=32|48=48" label:"Icon size in file lists"`
	IconsSpecialFolders int  `ini:"Configuration:IconsSpecialFolders" def:"1" min:"0" max:"3" opts:"0=Never|1=Local only|2=Local and network|3=Always" label:"Folder-specific icons"`
	IconsInMenus        bool `ini:"Configuration:IconsInMenus" def:"1" label:"Icons in menus"`
	IconsOnNet          bool `ini:"Configuration:IconsOnNet" def:"0" label:"Load icons from network folders"`
	ExeIcons            bool `ini:"Configuration:ShowExeLnkIcons" def:"1" label:"Program-specific icons"`
}

// FontPage: fonts (F-416). Sizes are in points.
type FontPage struct {
	PanelFont   string `ini:"Configuration:FontName" def:"" label:"File list font (empty = system)"`
	PanelSize   int    `ini:"Configuration:FontSize" def:"13" min:"6" max:"72" label:"File list font size"`
	PanelBold   bool   `ini:"Configuration:FontWeightBold" def:"0" label:"Bold file list font"`
	WindowFont  string `ini:"Configuration:FontNameWindow" def:"" label:"Main window font"`
	WindowSize  int    `ini:"Configuration:FontSizeWindow" def:"13" min:"6" max:"72" label:"Main window font size"`
	DialogFont  string `ini:"Configuration:FontNameDialog" def:"" label:"Dialog font"`
	DialogSize  int    `ini:"Configuration:FontSizeDialog" def:"13" min:"6" max:"72" label:"Dialog font size"`
	ListerFont  string `ini:"Lister:FontName" def:"" label:"Lister font (empty = monospace)"`
	ListerSize  int    `ini:"Lister:FontSize" def:"12" min:"6" max:"72" label:"Lister font size"`
	ZoomPercent int    `ini:"Configuration:ZoomPercent" def:"100" min:"50" max:"300" label:"Interface zoom (%)"`
}

// ColorsPage: options around the colour scheme (F-417; the colours
// themselves are a ColorScheme in [Colors]).
type ColorsPage struct {
	DarkMode         int    `ini:"Configuration:DarkMode" def:"2" min:"0" max:"2" opts:"0=Never|1=Always|2=Follow system" label:"Dark mode"`
	Scheme           string `ini:"Colors:SchemeName" def:"Default" label:"Colour scheme"`
	AlternateLines   bool   `ini:"Colors:AlternateLines" def:"0" label:"Alternate line background"`
	ColorByType      bool   `ini:"Colors:ColorFilters" def:"1" label:"Colour files by type"`
	InactiveDimmed   bool   `ini:"Colors:InactiveDimmed" def:"1" label:"Dim the inactive panel"`
	CursorFrameOnly  bool   `ini:"Colors:CursorFrameOnly" def:"0" label:"Cursor as a frame only"`
	GradientCursor   bool   `ini:"Colors:GradientCursor" def:"0" label:"Gradient cursor bar"`
	MarkBold         bool   `ini:"Colors:MarkBold" def:"0" label:"Show marked files in bold"`
	SelectionBgColor string `ini:"Colors:MarkBackColor" def:"" label:"Background of marked files (empty = none)"`
}

// LanguagePage (F-418).
type LanguagePage struct {
	LanguageFile string `ini:"Configuration:LanguageIni" def:"" label:"Language file (empty = English)"`
	UILanguage   string `ini:"Configuration:Language" def:"en" label:"Interface language"`
}

// OperationPage: general behaviour (F-419).
type OperationPage struct {
	OnlyOnce          bool `ini:"Configuration:onlyonce" def:"0" label:"Allow only one copy of the program"`
	MouseMode         int  `ini:"Configuration:Mouse" def:"0" min:"0" max:"1" opts:"0=Commander: right button selects|1=Standard: left button selects" label:"Mouse selection mode"`
	MarkDirectories   bool `ini:"Configuration:MarkDirectories" def:"0" label:"Num+ also selects folders"`
	SpaceCalcSize     bool `ini:"Configuration:SpaceCalcSize" def:"1" label:"Space calculates folder size"`
	AlwaysToRoot      bool `ini:"Configuration:AlwaysToRoot" def:"0" label:"Go to root when changing drive"`
	ArchiveEnter      int  `ini:"Configuration:ArchiveEnter" def:"0" min:"0" max:"1" opts:"0=Open like a folder|1=Open with associated program" label:"Enter on an archive"`
	SavePath          bool `ini:"Configuration:Savepath" def:"1" label:"Remember folders on exit"`
	SaveHistory       bool `ini:"Configuration:SaveHistory" def:"1" label:"Remember folder history"`
	SaveCommands      bool `ini:"Configuration:SaveCommands" def:"1" label:"Remember command line history"`
	SavePanels        bool `ini:"Configuration:Savepanels" def:"1" label:"Remember panel layout and views"`
	SaveOnExit        bool `ini:"Configuration:SaveIniOnExit" def:"1" label:"Save settings on exit"`
	MinimizeToTray    bool `ini:"Configuration:MinimizeToTray" def:"0" label:"Minimise to tray / menu bar"`
	DoubleClickParent bool `ini:"Configuration:DblClickParent" def:"0" label:"Double click on empty space goes up"`
	RenameSelectsName bool `ini:"Configuration:RenameSelectName" def:"1" label:"Shift+F6 selects the name without extension"`
	// CursorRepeat: held Up/Down (and Page Up/Down) keys repeat at the system's rate (0, the default) or faster, at
	// mormiCommander's own pace (core/keyrepeat.js).
	CursorRepeat int `ini:"Configuration:CursorRepeatSpeed" def:"0" min:"0" max:"3" opts:"0=System setting|1=Fast|2=Faster|3=Fastest" label:"Held Up/Down key repeat speed"`
}

// ViewerEditorPage (F-420).
type ViewerEditorPage struct {
	Viewer         string `ini:"Configuration:Viewer" def:"" label:"External viewer (empty = Lister)"`
	UseExtViewer   bool   `ini:"Configuration:UseExternalViewer" def:"0" label:"Use external viewer for F3"`
	Editor         string `ini:"Configuration:Editor" def:"" label:"Editor for F4 (empty = system default)"`
	EditorWait     bool   `ini:"Configuration:EditorWait" def:"0" label:"Wait for the editor before reloading"`
	ViewerByType   bool   `ini:"Configuration:ViewerByType" def:"0" label:"Choose viewer by file type"`
	EditTempUpload bool   `ini:"Configuration:EditUploadPrompt" def:"1" label:"Ask before uploading an edited remote file"`
}

// CopyDeletePage (F-421).
type CopyDeletePage struct {
	UseTrash            bool `ini:"Configuration:UseTrash" def:"1" label:"Delete to trash"`
	ConfirmDelete       bool `ini:"Configuration:ConfirmDelete" def:"1" label:"Confirm deleting files"`
	ConfirmDeleteDirs   bool `ini:"Configuration:ConfirmDeleteDirs" def:"1" label:"Confirm deleting non-empty folders"`
	ConfirmDeleteRO     bool `ini:"Configuration:ConfirmDeleteReadOnly" def:"1" label:"Confirm deleting read-only and hidden files"`
	ConfirmOverwrite    bool `ini:"Configuration:ConfirmOverwrite" def:"1" label:"Confirm overwriting files"`
	ConfirmOverwriteRO  bool `ini:"Configuration:ConfirmOverwriteReadOnly" def:"1" label:"Confirm overwriting read-only files"`
	ConfirmCopy         bool `ini:"Configuration:ConfirmCopy" def:"1" label:"Show the copy dialog for F5/F6"`
	ConfirmDragDrop     bool `ini:"Configuration:ConfirmDragDrop" def:"1" label:"Confirm drag and drop"`
	BufferKB            int  `ini:"Configuration:CopyBufferSize" def:"1024" min:"16" max:"65536" label:"Copy buffer size (kB)"`
	Verify              bool `ini:"Configuration:CopyVerify" def:"0" label:"Verify after copying"`
	CopyTimes           bool `ini:"Configuration:CopyTimes" def:"1" label:"Copy file times"`
	CopyAttributes      bool `ini:"Configuration:CopyAttributes" def:"1" label:"Copy attributes"`
	CopyPermissions     bool `ini:"Configuration:CopyPermissions" def:"1" label:"Copy permissions"`
	CopyOwner           bool `ini:"Configuration:CopyOwner" def:"0" label:"Copy owner and group"`
	CopyComments        bool `ini:"Configuration:CopyComments" def:"1" label:"Copy file comments"`
	OverwriteDefault    int  `ini:"Configuration:OverwriteDefault" def:"0" min:"0" max:"5" opts:"0=Ask|1=Overwrite all|2=Skip all|3=Overwrite older|4=Rename new|5=Rename old" label:"When the target exists"`
	BackgroundDefault   bool `ini:"Configuration:CopyBackgroundDefault" def:"0" label:"Copy in the background by default"`
	BigFileMode         int  `ini:"Configuration:BigFileMode" def:"0" min:"0" max:"2" opts:"0=Automatic|1=Always|2=Never" label:"Big file copy mode (no cache)"`
	BigFileThresholdMB  int  `ini:"Configuration:BigFileThreshold" def:"1024" min:"1" max:"1048576" label:"Big file threshold (MB)"`
	FollowLinks         bool `ini:"Configuration:CopyFollowLinks" def:"0" label:"Copy link targets instead of links"`
	CheckFreeSpace      bool `ini:"Configuration:CheckFreeSpace" def:"1" label:"Check free space before copying"`
	MoveDeleteAfterCopy bool `ini:"Configuration:MoveSafe" def:"1" label:"Move across drives: delete only after a full copy"`

	// SkipErrorsBackground (board issue #15): background jobs note failed files and go on, and list them at the end.
	SkipErrorsBackground bool `ini:"Configuration:SkipErrorsInBackground" def:"0" label:"Skip errors in background jobs (list them at the end)"`
}

// RefreshPage (F-422).
type RefreshPage struct {
	AutoRefresh     bool   `ini:"Configuration:AutoRefresh" def:"1" label:"Refresh file lists automatically"`
	OnlyForeground  bool   `ini:"Configuration:RefreshOnlyActive" def:"0" label:"Refresh only when the window is active"`
	RefreshNetwork  bool   `ini:"Configuration:RefreshNetwork" def:"1" label:"Refresh network folders"`
	RefreshRemovabl bool   `ini:"Configuration:RefreshRemovable" def:"1" label:"Refresh removable drives"`
	ExcludeFolders  string `ini:"Configuration:WatchDirsExclude" def:"" label:"Folders not refreshed automatically"`
	IntervalMs      int    `ini:"Configuration:RefreshInterval" def:"500" min:"100" max:"60000" label:"Delay before refreshing (ms)"`
}

// QuickSearchPage (F-423).
type QuickSearchPage struct {
	Mode           int  `ini:"Configuration:Altsearch" def:"2" min:"0" max:"3" opts:"0=Ctrl+Alt+letters|1=Alt+letters|2=Letters only|3=Off" label:"Quick search with"`
	MatchBeginning bool `ini:"Configuration:QuickSearchMatchBeginning" def:"1" label:"Match the beginning of names"`
	MatchEnd       bool `ini:"Configuration:QuickSearchExactMatch" def:"0" label:"Match the end of names"`
	Filter         bool `ini:"Configuration:QuickSearchAutoFilter" def:"0" label:"Show only matching files (filter)"`
	Wildcards      bool `ini:"Configuration:QuickSearchWildcards" def:"1" label:"Allow * and ? wildcards"`
	IgnoreAccents  bool `ini:"Configuration:QuickSearchIgnoreAccents" def:"1" label:"Ignore accents (á matches a)"`
	ShowDropDown   bool `ini:"Configuration:QuickSearchDropDown" def:"0" label:"Show matching names in a list"`
}

// FTPPage (F-424).
type FTPPage struct {
	Passive        bool   `ini:"FTP:PassiveDefault" def:"1" label:"Passive mode by default"`
	ShowHidden     bool   `ini:"FTP:ShowHidden" def:"0" label:"Show hidden files (LIST -a)"`
	TimeoutSec     int    `ini:"FTP:Timeout" def:"30" min:"1" max:"3600" label:"Timeout (seconds)"`
	Retries        int    `ini:"FTP:Retries" def:"3" min:"0" max:"100" label:"Retries"`
	RetryDelaySec  int    `ini:"FTP:RetryDelay" def:"5" min:"0" max:"3600" label:"Delay between retries (seconds)"`
	DownloadDir    string `ini:"FTP:DownloadDir" def:"" label:"Default download folder"`
	AnonymousEmail string `ini:"FTP:AnonymousEmail" def:"anonymous@example.com" label:"E-mail for anonymous login"`
	KeepAliveSec   int    `ini:"FTP:KeepAlive" def:"0" min:"0" max:"3600" label:"Keep-alive interval (seconds, 0 = off)"`
	AsciiMasks     string `ini:"FTP:AsciiMasks" def:"*.txt *.htm *.html *.css *.js" label:"Files sent in text mode"`
	ResumeDefault  bool   `ini:"FTP:Resume" def:"1" label:"Resume interrupted downloads"`
}

// ProxyPage (F-437). Passwords are not kept in the settings file; the UI
// stores them in the platform's credential store.
type ProxyPage struct {
	Type int    `ini:"Proxy:Type" def:"0" min:"0" max:"4" opts:"0=None|1=HTTP CONNECT|2=SOCKS4|3=SOCKS5|4=USER@host" label:"Proxy type"`
	Host string `ini:"Proxy:Host" def:"" label:"Proxy host"`
	Port int    `ini:"Proxy:Port" def:"0" min:"0" max:"65535" label:"Proxy port"`
	User string `ini:"Proxy:User" def:"" label:"Proxy user name"`
}

// PluginsPage: TC plugins are Windows DLLs (F-425 is N/A); this page
// configures built-in handlers and external command-line tools instead.
type PluginsPage struct {
	UseExternalTools bool   `ini:"Plugins:UseExternalTools" def:"1" label:"Use configured external tools"`
	SevenZipPath     string `ini:"Plugins:SevenZip" def:"" label:"7-Zip program (empty = search PATH)"`
	UnrarPath        string `ini:"Plugins:Unrar" def:"" label:"unrar program (empty = search PATH)"`
	ExifToolPath     string `ini:"Plugins:ExifTool" def:"" label:"exiftool program for metadata columns"`
	FFprobePath      string `ini:"Plugins:FFprobe" def:"" label:"ffprobe program for media columns"`
}

// ThumbnailsPage (F-426).
type ThumbnailsPage struct {
	Width       int    `ini:"Thumbnails:Width" def:"96" min:"16" max:"512" label:"Thumbnail width"`
	Height      int    `ini:"Thumbnails:Height" def:"96" min:"16" max:"512" label:"Thumbnail height"`
	Cache       bool   `ini:"Thumbnails:Cache" def:"1" label:"Keep thumbnails in a cache"`
	CacheDir    string `ini:"Thumbnails:CacheDir" def:"" label:"Cache folder (empty = default)"`
	CacheMaxMB  int    `ini:"Thumbnails:CacheMaxMB" def:"500" min:"1" max:"100000" label:"Maximum cache size (MB)"`
	ShowName    bool   `ini:"Thumbnails:ShowName" def:"1" label:"Show file names below thumbnails"`
	FileTypes   string `ini:"Thumbnails:FileTypes" def:"*.jpg *.jpeg *.png *.gif *.bmp *.webp *.heic *.tif *.tiff *.svg *.pdf *.mp4 *.mov" label:"File types with thumbnails"`
	MaxFileMB   int    `ini:"Thumbnails:MaxFileSize" def:"200" min:"1" max:"100000" label:"Skip files larger than (MB)"`
	ShowOnNet   bool   `ini:"Thumbnails:ShowOnNetwork" def:"0" label:"Thumbnails for network folders"`
	FolderThumb bool   `ini:"Thumbnails:FolderPreview" def:"0" label:"Preview images inside folders"`
}

// LogPage (F-427).
type LogPage struct {
	Enabled    bool   `ini:"Configuration:EnableLog" def:"0" label:"Write a log file"`
	File       string `ini:"Configuration:LogFile" def:"%COMMANDER_PATH%/mormicommander.log" label:"Log file"`
	Copy       bool   `ini:"Configuration:LogCopy" def:"1" label:"Log copy and move"`
	Delete     bool   `ini:"Configuration:LogDelete" def:"1" label:"Log delete"`
	Rename     bool   `ini:"Configuration:LogRename" def:"1" label:"Log rename"`
	MkDir      bool   `ini:"Configuration:LogMkDir" def:"1" label:"Log new folders"`
	Pack       bool   `ini:"Configuration:LogPack" def:"1" label:"Log pack and unpack"`
	FTP        bool   `ini:"Configuration:LogFTP" def:"1" label:"Log FTP and network operations"`
	ErrorsOnly bool   `ini:"Configuration:LogErrorsOnly" def:"0" label:"Log errors only"`
	MaxSizeKB  int    `ini:"Configuration:LogMaxSize" def:"0" min:"0" max:"10485760" label:"Start a new log above (kB, 0 = never)"`
}

// IgnoreListPage (F-428).
type IgnoreListPage struct {
	Enabled bool   `ini:"Configuration:IgnoreListFileEnabled" def:"0" label:"Hide files from the ignore list"`
	File    string `ini:"Configuration:IgnoreListFile" def:"%COMMANDER_PATH%/ignore.txt" label:"Ignore list file"`
}

// PackerPage (F-429).
type PackerPage struct {
	DefaultPacker  string `ini:"Packer:DefaultPacker" def:"zip" label:"Default packer (zip, 7z, tar, tgz, tbz, txz)"`
	InternalZip    bool   `ini:"Packer:UseInternalZip" def:"1" label:"Use the internal zip packer"`
	ExternalZip    string `ini:"Packer:ZipProgram" def:"" label:"External zip program"`
	TarOwner       bool   `ini:"Packer:TarStoreOwner" def:"1" label:"Store owner and permissions in tar"`
	MoveToArchive  bool   `ini:"Packer:MoveDefault" def:"0" label:"Move to archive by default"`
	SeparateFiles  bool   `ini:"Packer:SeparateArchives" def:"0" label:"One archive per file by default"`
	ArchiveSubdirs bool   `ini:"Packer:IncludeSubdirs" def:"1" label:"Include subfolders"`
	OpenFolderLike bool   `ini:"Packer:ZipLikeDirectory" def:"1" label:"Treat archives like folders"`
	SearchInside   bool   `ini:"Packer:SearchArchives" def:"0" label:"Find files searches inside archives"`
}

// ZipPage (F-430).
type ZipPage struct {
	Level     int    `ini:"Packer:ZipLevel" def:"6" min:"0" max:"9" label:"Compression level (0 = store, 9 = best)"`
	UTF8Names bool   `ini:"Packer:ZipUTF8Names" def:"1" label:"Store names as UTF-8"`
	SetTimes  bool   `ini:"Packer:ZipSetTimes" def:"1" label:"Keep original file times when unpacking"`
	TempDir   string `ini:"Packer:ZipTempDir" def:"" label:"Temporary folder (empty = system)"`
	AES       bool   `ini:"Packer:ZipAES" def:"1" label:"Use AES-256 when encrypting"`
	Zip64     bool   `ini:"Packer:Zip64" def:"1" label:"Allow ZIP64 for big archives"`
}

// MiscPage (F-431, F-828).
type MiscPage struct {
	WindowTitle    string `ini:"Configuration:TitleBarText" def:"mormiCommander" label:"Window title"`
	ConfirmExit    bool   `ini:"Configuration:ConfirmExit" def:"0" label:"Confirm exit"`
	MainMenuFile   string `ini:"Configuration:Mainmenu" def:"" label:"Custom main menu file (.mnu, empty = default)"`
	SoundOnDone    bool   `ini:"Configuration:SoundOnFinish" def:"0" label:"Play a sound when long operations finish"`
	CheckUpdates   bool   `ini:"Configuration:CheckUpdates" def:"0" label:"Check for updates"`
	BeepOnError    bool   `ini:"Configuration:BeepOnError" def:"1" label:"Beep on errors"`
	CmdLineAutoCmp bool   `ini:"Configuration:CmdLineAutoComplete" def:"1" label:"Command line auto-complete"`
	DialogWindows  bool   `ini:"Configuration:DialogsInWindows" def:"1" label:"Open dialogs in windows of their own"`

	// HideNoEffect (board issue #94): the Options dialog leaves out the options the program does not act on yet
	// (noeffect.go). Off by default, so the dialog shows every option as before, with those marked.
	HideNoEffect bool `ini:"Configuration:HideOptionsWithoutEffect" def:"0" label:"Hide options that have no effect yet"`
}

// FolderTabsPage (F-432).
type FolderTabsPage struct {
	Show            int  `ini:"Tabs:Show" def:"1" min:"0" max:"2" opts:"0=Never|1=When more than one|2=Always" label:"Show tab bar"`
	MultiLine       bool `ini:"Tabs:MultiLine" def:"0" label:"Tabs in several rows"`
	Bottom          bool `ini:"Tabs:Bottom" def:"0" label:"Tabs below the file list"`
	CloseButton     bool `ini:"Tabs:CloseButton" def:"1" label:"Close button on tabs"`
	DoubleClickOpen bool `ini:"Tabs:DblClickClose" def:"1" label:"Double click closes a tab"`
	NewNextToCur    bool `ini:"Tabs:NewNextToCurrent" def:"1" label:"Open new tabs next to the current one"`
	ConfirmCloseAll bool `ini:"Tabs:ConfirmCloseAll" def:"1" label:"Confirm closing all tabs"`
	LockedIcon      bool `ini:"Tabs:ShowLockedIcon" def:"1" label:"Mark locked tabs with a star"`
	MaxTitleChars   int  `ini:"Tabs:MaxTitleChars" def:"32" min:"0" max:"255" label:"Maximum tab title length (0 = unlimited)"`
	CtrlUpNewTab    bool `ini:"Tabs:CtrlUpOpensTab" def:"1" label:"Ctrl+Up opens folder under cursor in a new tab"`
	RestoreTabs     bool `ini:"Tabs:Restore" def:"1" label:"Restore tabs at startup"`
	ActivateNew     bool `ini:"Tabs:ActivateNew" def:"1" label:"Activate newly opened tabs"`
}

// CustomColumnsPage (F-433; the sets themselves are in [CustomFields]).
type CustomColumnsPage struct {
	AutoSwitch       bool `ini:"Configuration:ColumnsAutoSwitch" def:"1" label:"Switch column sets by folder"`
	ShowInShowMenu   bool `ini:"Configuration:ColumnsInMenu" def:"1" label:"List column sets in the Show menu"`
	EqualWidthOnOpen bool `ini:"Configuration:ColumnsEqualWidth" def:"0" label:"Fit columns to the panel width"`
}

// FavoriteTabsPage (F-434).
type FavoriteTabsPage struct {
	RestoreBoth   bool `ini:"Tabs:FavoritesBothPanels" def:"1" label:"Favourite tabs restore both panels"`
	CloseOldTabs  bool `ini:"Tabs:FavoritesCloseOld" def:"1" label:"Close existing tabs when loading favourites"`
	ConfirmReplac bool `ini:"Tabs:FavoritesConfirm" def:"0" label:"Confirm replacing tabs"`
}

// BackgroundQueuePage (F-436).
type BackgroundQueuePage struct {
	MaxParallel  int  `ini:"Configuration:QueueParallel" def:"1" min:"1" max:"16" label:"Jobs running at the same time"`
	SpeedLimitKB int  `ini:"Configuration:QueueSpeedLimit" def:"0" min:"0" max:"10485760" label:"Speed limit (kB/s, 0 = none)"`
	AutoStart    bool `ini:"Configuration:QueueAutoStart" def:"1" label:"Start queued jobs automatically"`
	ShowWindow   bool `ini:"Configuration:QueueShowWindow" def:"1" label:"Show the transfer queue window"`
	KeepFinished bool `ini:"Configuration:QueueKeepFinished" def:"0" label:"Keep finished jobs in the list"`
}

// HotlistPage (F-438, F-662).
type HotlistPage struct {
	ShowPaths     bool `ini:"Configuration:HotlistShowPaths" def:"0" label:"Show folders next to hotlist names"`
	TypeToFilter  bool `ini:"Configuration:HotlistFilter" def:"0" label:"Typing in the hotlist filters it"`
	AddAtTop      bool `ini:"Configuration:HotlistAddTop" def:"0" label:"Add new entries at the top"`
	HistorySize   int  `ini:"Configuration:HistorySize" def:"30" min:"1" max:"500" label:"Folder history size"`
	CmdHistSize   int  `ini:"Configuration:CmdHistorySize" def:"50" min:"1" max:"500" label:"Command line history size"`
	FrequentSize  int  `ini:"Configuration:FrequentSize" def:"20" min:"0" max:"200" label:"Frequently used folders shown"`
	HistoryPerTab bool `ini:"Configuration:HistoryPerTab" def:"1" label:"Separate folder history per tab"`
}

// ViewModesPage (F-820; the modes are in view modes sections).
type ViewModesPage struct {
	AutoSwitch bool `ini:"Configuration:ViewModeAutoSwitch" def:"1" label:"Switch view modes automatically"`
}

// TooltipsPage (F-821).
type TooltipsPage struct {
	Show      int  `ini:"Configuration:Win32TipWindow" def:"2" min:"0" max:"3" opts:"0=Off|1=Only for cut-off names|2=Names and file info|3=File info only" label:"Tooltips in file lists"`
	DelayMs   int  `ini:"Configuration:TooltipDelay" def:"700" min:"0" max:"10000" label:"Tooltip delay (ms)"`
	ByType    bool `ini:"Configuration:TooltipsByType" def:"1" label:"Use per-type tooltip fields"`
	ShowOnNet bool `ini:"Configuration:TooltipsOnNet" def:"0" label:"Tooltips for network files"`
}

// ListerPage (F-822).
type ListerPage struct {
	DefaultMode  int    `ini:"Lister:DefaultMode" def:"0" min:"0" max:"6" opts:"0=Auto|1=Text|2=Binary|3=Hex|4=Unicode|5=UTF-8|6=Image/multimedia" label:"Default mode"`
	TabWidth     int    `ini:"Lister:TabWidth" def:"8" min:"1" max:"32" label:"Tab width"`
	Charset      string `ini:"Lister:Charset" def:"auto" label:"Default character set"`
	AutoDetect   bool   `ini:"Lister:AutoDetect" def:"1" label:"Detect text encoding"`
	Wrap         bool   `ini:"Lister:Wrap" def:"1" label:"Wrap long lines"`
	ImageFit     int    `ini:"Lister:ImageFit" def:"1" min:"0" max:"2" opts:"0=Actual size|1=Shrink large images|2=Fit to window" label:"Image size"`
	UseImages    bool   `ini:"Lister:Images" def:"1" label:"Show images"`
	UseMedia     bool   `ini:"Lister:Multimedia" def:"1" label:"Play audio and video"`
	UseHTML      bool   `ini:"Lister:HTML" def:"1" label:"Show HTML and Markdown formatted"`
	UsePDF       bool   `ini:"Lister:PDF" def:"1" label:"Show PDF files"`
	MaxTextMB    int    `ini:"Lister:MaxTextSize" def:"64" min:"1" max:"100000" label:"Load text files up to (MB)"`
	BinaryWidth  int    `ini:"Lister:BinaryWidth" def:"80" min:"16" max:"1024" label:"Binary mode line width"`
	ExtViewerFor string `ini:"Lister:ExternalFor" def:"" label:"Open these types in the external viewer"`
}

// DragDropPage (F-823).
type DragDropPage struct {
	ConfirmDrop    bool `ini:"Configuration:ConfirmDrop" def:"1" label:"Confirm before dropping"`
	RightDragMenu  bool `ini:"Configuration:RightDragMenu" def:"1" label:"Right-drag shows a menu"`
	SpringLoaded   bool `ini:"Configuration:SpringLoaded" def:"1" label:"Spring-loaded folders"`
	SpringDelayMs  int  `ini:"Configuration:SpringDelay" def:"1000" min:"100" max:"10000" label:"Spring-loaded delay (ms)"`
	MiddleClick    int  `ini:"Configuration:MiddleClick" def:"1" min:"0" max:"2" opts:"0=Nothing|1=Open folder in new tab|2=Open in the other panel" label:"Middle click on a folder"`
	DragToOtherApp bool `ini:"Configuration:DragOut" def:"1" label:"Allow dragging to other programs"`
	NativeDragOut  bool `ini:"Configuration:NativeDragOut" def:"0" label:"Drag files to other programs as real files (macOS)"`
	ThresholdPx    int  `ini:"Configuration:DragThreshold" def:"4" min:"1" max:"100" label:"Drag start distance (pixels)"`
}

// TreePage (F-824).
type TreePage struct {
	FollowFolder bool `ini:"Configuration:TreeFollow" def:"1" label:"Tree follows the current folder"`
	ShowHidden   bool `ini:"Configuration:TreeShowHidden" def:"0" label:"Show hidden folders in the tree"`
	ShowFiles    bool `ini:"Configuration:TreeShowFiles" def:"0" label:"Show files in the tree"`
	AutoExpand   bool `ini:"Configuration:TreeAutoExpand" def:"1" label:"Expand folders on single click"`
	ShowNetwork  bool `ini:"Configuration:TreeNetwork" def:"0" label:"Read network folders in the tree"`
}

// FindFilesPage (F-825).
type FindFilesPage struct {
	Depth           int    `ini:"SearchDefaults:Depth" def:"-1" min:"-1" max:"99" label:"Search depth (-1 = all subfolders)"`
	Archives        bool   `ini:"SearchDefaults:Archives" def:"0" label:"Search in archives"`
	UseIndex        bool   `ini:"SearchDefaults:UseIndex" def:"0" label:"Use the system index (Spotlight, locate, Windows Search)"`
	Encoding        string `ini:"SearchDefaults:Encoding" def:"auto" label:"Text encoding for content search"`
	CaseSensitive   bool   `ini:"SearchDefaults:CaseSensitive" def:"0" label:"Case-sensitive text search"`
	RegEx           bool   `ini:"SearchDefaults:RegEx" def:"0" label:"Regular expressions"`
	Hidden          bool   `ini:"SearchDefaults:Hidden" def:"1" label:"Search hidden files"`
	HiddenFromPanel bool   `ini:"SearchDefaults:HiddenFollowsPanel" def:"0" label:"New searches include hidden files when the panel shows them"`
	FollowLinks     bool   `ini:"SearchDefaults:FollowLinks" def:"0" label:"Follow symbolic links"`
	MaxResults      int    `ini:"SearchDefaults:MaxResults" def:"0" min:"0" max:"10000000" label:"Stop after this many results (0 = none)"`
	// Modeless (board issue #110): the Find files window leaves the main window usable, as TC's does. Off by default,
	// so Find files keeps blocking the main window as it always did; it works with dialogs in windows of their own.
	Modeless bool `ini:"SearchDefaults:Modeless" def:"0" label:"Find files window does not block the main window"`
}

// AIPage: the AI layer (issue #146), where the user writes what to do and an LLM does it with the app's own tools.
// Two providers: the Claude Code CLI (the default; the path and model are empty to let the app find the command and
// the CLI pick its default model), and OpenRouter or any other OpenAI-compatible chat completions API (step 2). The
// OpenRouter model is empty until the user chooses one (Configuration > AI settings lists them), since which models
// exist and what they cost changes too often for a default to stay good. Its API key is not an option: it is kept in
// the system keychain only (feature_ai_openrouter.go).
type AIPage struct {
	Provider        int    `ini:"Configuration:AIProvider" def:"0" min:"0" max:"1" opts:"0=Claude Code (claude command)|1=OpenRouter (or another OpenAI-compatible API)" label:"AI provider"`
	ClaudePath      string `ini:"Configuration:AIClaudePath" def:"" label:"Claude command (empty = find it)"`
	ClaudeModel     string `ini:"Configuration:AIClaudeModel" def:"" label:"Claude model (empty = default)"`
	OpenRouterModel string `ini:"Configuration:AIOpenRouterModel" def:"" label:"OpenRouter model (like anthropic/claude-sonnet-4.5)"`
	OpenRouterURL   string `ini:"Configuration:AIOpenRouterURL" def:"https://openrouter.ai/api/v1" label:"OpenRouter API address (any OpenAI-compatible API)"`
}

// FieldInfo describes one option for dialogs, search and control mode.
type FieldInfo struct {
	Name    string // "Display.ShowHidden"
	Page    string // page title
	PageID  string // "Display"
	Section string
	Key     string
	Label   string
	Type    string // "bool", "int", "string"
	Default string
	Min     *int
	Max     *int
	Options []Option
	// NoEffect: the option is kept in the file but the program does not act on it yet (noeffect.go, board issue #94).
	NoEffect bool
	index    []int
}

// Option is a named choice of an int option.
type Option struct {
	Value int
	Label string
}

// PageInfo is one page of the dialog.
type PageInfo struct {
	ID    string // Go field name, used in option names
	Title string
}

var (
	fieldOnce  sync.Once
	fieldCache []FieldInfo
)

// Fields lists every option in dialog order. The slice is shared; don't
// modify it.
func Fields() []FieldInfo {
	fieldOnce.Do(buildFields)
	return fieldCache
}

func buildFields() {
	st := reflect.TypeOf(Settings{})
	for i := 0; i < st.NumField(); i++ {
		pf := st.Field(i)
		pt := pf.Type
		for j := 0; j < pt.NumField(); j++ {
			ff := pt.Field(j)
			sec, key, ok := strings.Cut(ff.Tag.Get("ini"), ":")
			if !ok {
				panic("config: field without ini tag: " + pf.Name + "." + ff.Name)
			}
			fi := FieldInfo{
				Name: pf.Name + "." + ff.Name, Page: pf.Tag.Get("page"), PageID: pf.Name,
				Section: sec, Key: key, Label: ff.Tag.Get("label"),
				Default: ff.Tag.Get("def"), index: []int{i, j},
			}
			fi.NoEffect = noEffect[fi.Name]
			switch ff.Type.Kind() {
			case reflect.Bool:
				fi.Type = "bool"
			case reflect.Int:
				fi.Type = "int"
			case reflect.String:
				fi.Type = "string"
			default:
				panic("config: unsupported settings type for " + fi.Name)
			}
			if v := ff.Tag.Get("min"); v != "" {
				n, _ := strconv.Atoi(v)
				fi.Min = &n
			}
			if v := ff.Tag.Get("max"); v != "" {
				n, _ := strconv.Atoi(v)
				fi.Max = &n
			}
			if v := ff.Tag.Get("opts"); v != "" {
				for _, o := range strings.Split(v, "|") {
					val, lbl, _ := strings.Cut(o, "=")
					n, _ := strconv.Atoi(val)
					fi.Options = append(fi.Options, Option{n, lbl})
				}
			}
			fieldCache = append(fieldCache, fi)
		}
	}
}

// Pages lists the dialog pages in order.
func Pages() []PageInfo {
	var out []PageInfo
	st := reflect.TypeOf(Settings{})
	for i := 0; i < st.NumField(); i++ {
		out = append(out, PageInfo{ID: st.Field(i).Name, Title: st.Field(i).Tag.Get("page")})
	}
	return out
}

// LookupSetting finds an option by "Page.Field" name (case-insensitive).
func LookupSetting(name string) (FieldInfo, bool) {
	for _, f := range Fields() {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return FieldInfo{}, false
}

// SearchSettings returns the options whose label, name, key or page title
// contains every word of query (F-827), plus the pages they are on.
func SearchSettings(query string) ([]FieldInfo, []string) {
	return SearchSettingsWords(query, false)
}

// SearchSettingsWords is SearchSettings with a choice: with wholeWords, each
// word of query must stand as a whole word in the option's text ("two" finds
// "Two drive button bars" but not "network"); without it, any part of a word matches, as
// the search always has (board issue #109).
func SearchSettingsWords(query string, wholeWords bool) ([]FieldInfo, []string) {
	words := strings.Fields(strings.ToLower(query))
	var out []FieldInfo
	var pages []string
	seen := map[string]bool{}
	for _, f := range Fields() {
		hay := strings.ToLower(f.Label + " " + f.Name + " " + f.Key + " " + f.Page)
		for _, o := range f.Options {
			hay += " " + strings.ToLower(o.Label)
		}
		match := true
		for _, w := range words {
			if !ContainsWord(hay, w, wholeWords) {
				match = false
				break
			}
		}
		if match {
			out = append(out, f)
			if !seen[f.PageID] {
				seen[f.PageID] = true
				pages = append(pages, f.PageID)
			}
		}
	}
	return out, pages
}

// ContainsWord reports whether hay contains word. With whole, the match must
// not have a letter or digit right before or after it, so it is a word of its
// own and not part of a longer one; every place word occurs is tried.
func ContainsWord(hay, word string, whole bool) bool {
	if !whole || word == "" {
		return strings.Contains(hay, word)
	}
	for from := 0; ; {
		i := strings.Index(hay[from:], word)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(word)
		before, _ := utf8.DecodeLastRuneInString(hay[:start])
		after, _ := utf8.DecodeRuneInString(hay[end:])
		if !isWordRune(before) && !isWordRune(after) {
			return true
		}
		_, size := utf8.DecodeRuneInString(hay[start:])
		from = start + size
	}
}

// isWordRune tells the letters and digits that make up words; utf8.RuneError
// (the start or end of the text) is not one.
func isWordRune(r rune) bool {
	return r != utf8.RuneError && (unicode.IsLetter(r) || unicode.IsDigit(r))
}

func (s *Settings) field(fi FieldInfo) reflect.Value {
	return reflect.ValueOf(s).Elem().FieldByIndex(fi.index)
}

// setValue parses text into a field; it clamps ints into [min, max].
func setValue(v reflect.Value, fi FieldInfo, text string) error {
	switch fi.Type {
	case "bool":
		b, ok := parseBool(text)
		if !ok {
			return fmt.Errorf("config: %s: %q is not a yes/no value", fi.Name, text)
		}
		v.SetBool(b)
	case "int":
		n, ok := parseLeadingInt(text)
		if !ok {
			return fmt.Errorf("config: %s: %q is not a number", fi.Name, text)
		}
		if fi.Min != nil && n < *fi.Min {
			n = *fi.Min
		}
		if fi.Max != nil && n > *fi.Max {
			n = *fi.Max
		}
		v.SetInt(int64(n))
	default:
		v.SetString(text)
	}
	return nil
}

func formatValue(v reflect.Value, fi FieldInfo) string {
	switch fi.Type {
	case "bool":
		if v.Bool() {
			return "1"
		}
		return "0"
	case "int":
		return strconv.FormatInt(v.Int(), 10)
	}
	return v.String()
}

// Defaults returns the settings of a fresh install.
func Defaults() Settings {
	var s Settings
	for _, fi := range Fields() {
		if err := setValue(s.field(fi), fi, fi.Default); err != nil {
			panic(err)
		}
	}
	return s
}

// LoadSettings reads every option from f on top of the defaults. Invalid
// values keep the default and are reported.
func LoadSettings(f *File) (Settings, []string) {
	s := Defaults()
	var warnings []string
	for _, fi := range Fields() {
		text, ok := f.Get(fi.Section, fi.Key)
		if !ok {
			continue
		}
		if err := setValue(s.field(fi), fi, text); err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	return s, warnings
}

// Save writes the options into f: keys already in the file are updated,
// others are only added when they differ from the default.
func (s *Settings) Save(f *File) {
	for _, fi := range Fields() {
		val := formatValue(s.field(fi), fi)
		if f.Has(fi.Section, fi.Key) || val != fi.Default {
			f.Set(fi.Section, fi.Key, val)
		}
	}
}

// Get returns an option's value as text ("1"/"0" for booleans).
func (s *Settings) Get(name string) (string, error) {
	fi, ok := LookupSetting(name)
	if !ok {
		return "", fmt.Errorf("config: unknown setting %q", name)
	}
	return formatValue(s.field(fi), fi), nil
}

// Set changes an option from text (clamped like on load).
func (s *Settings) Set(name, value string) error {
	fi, ok := LookupSetting(name)
	if !ok {
		return fmt.Errorf("config: unknown setting %q", name)
	}
	return setValue(s.field(fi), fi, value)
}

// Diff lists the options whose values differ between s and o, sorted, so
// the dialog can apply only what changed (F-412 Apply).
func (s *Settings) Diff(o *Settings) []string {
	var out []string
	for _, fi := range Fields() {
		if formatValue(s.field(fi), fi) != formatValue(o.field(fi), fi) {
			out = append(out, fi.Name)
		}
	}
	sort.Strings(out)
	return out
}
