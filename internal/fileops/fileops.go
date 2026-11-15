// Package fileops is the engine behind copy, move, delete, wipe and rename: a queue of jobs that run in the
// background, report progress as events (bytes, files, speed, time left), can be paused, resumed and cancelled, can be
// slowed down by a speed limit, and stop to ask when something needs a decision ("the target exists", "the file is
// read-only", "this file can't be read: retry?"). It has no UI dependencies: the app gives it an Emit function (Wails
// events in the app, a recorder in the tests) and answers the questions through Answer, or through an AskFunc callback
// when it wants to decide synchronously.
//
// The rules follow Total Commander:
//   - a folder is never copied into itself, and links are copied as links by default, so a link loop can't make a
//     job run forever (when links are followed, a loop is detected and reported instead);
//   - a move on the same volume is a rename (instant); across volumes it is copy, then each source file is removed
//     right after its own copy succeeded (and was verified, when asked), and the folders that end up empty go too;
//   - "delete" is permanent, "trash" uses the system's trash and "wipe" overwrites the bytes before deleting; the UI
//     picks which one (F8 / Shift+F8 / the wipe command);
//   - an error on one file doesn't stop the job: by default it is noted and the job goes on, or the user is asked
//     Retry / Skip / Skip all / Cancel when the request says so;
//   - jobs marked Queued run one after another in a single lane (TC's "Queue" button, F2 in the copy dialog); other
//     jobs run at once, side by side (TC's "Background" button).
//
// This file holds the public types that the UI exchanges with the engine as JSON.
package fileops

// Kind is what a job does.
type Kind string

const (
	Copy   Kind = "copy"
	Move   Kind = "move"
	Delete Kind = "delete" // permanent
	Trash  Kind = "trash"  // to the Trash / Recycle Bin
	Wipe   Kind = "wipe"   // overwrite the contents, then delete permanently
)

// Policy says what happens when a target file exists (TC 11's "Overwrite mode" dropdown in the copy dialog).
type Policy string

const (
	Ask              Policy = ""              // ask each time (the default)
	Overwrite        Policy = "overwrite"     // overwrite all
	Skip             Policy = "skip"          // skip all
	Older            Policy = "older"         // overwrite only targets that are older than the source
	AutoRenameSource Policy = "rename"        // the copy gets a free name: "file(2).txt"
	AutoRenameTarget Policy = "rename_target" // the existing target is renamed to "file(2).txt", then the copy is written
	Larger           Policy = "larger"        // overwrite only when the source is larger ("copy all larger")
	Smaller          Policy = "smaller"       // overwrite only when the source is smaller ("copy all smaller")
)

// LinkMode says what a copy does with symbolic links.
type LinkMode string

const (
	LinksCopy   LinkMode = ""       // copy the link itself (the default, and always used by moves)
	LinksFollow LinkMode = "follow" // copy what the link points to; loops are detected and reported
	LinksSkip   LinkMode = "skip"   // leave links out
)

// ErrorPolicy says what happens when one file fails (can't be read, written or removed).
type ErrorPolicy string

const (
	ErrorsSkip ErrorPolicy = ""     // note the error and go on with the next file (the default)
	ErrorsAsk  ErrorPolicy = "ask"  // ask Retry / Skip / Skip all / Cancel
	ErrorsStop ErrorPolicy = "stop" // note the error and cancel the job
)

// Request is a job to start. The zero value of every option is Total Commander's default.
type Request struct {
	Kind    Kind     `json:"kind"`
	Sources []string `json:"sources"` // full paths
	Target  string   `json:"target"`  // the destination folder (copy and move); made when missing
	// Rename is a new name for a single source (copy "a.txt" as "b.txt"), or a mask with * and ? for any number of
	// sources ("*.bak" renames every copy to the .bak extension). "" keeps the names. ParseTarget splits what the user
	// typed in the dialog into Target and Rename.
	Rename    string `json:"rename"`
	Overwrite Policy `json:"overwrite"` // see Policy
	// ReadOnly is what happens when a target that would be overwritten is read-only: Ask (the default, TC's separate
	// prompt), Overwrite or Skip.
	ReadOnly Policy `json:"readOnly"`
	// Filter is TC's "Only files of this type": patterns separated by ";" or spaces, then "|" and patterns to leave
	// out ("*.txt;*.md|old*"); a pattern ending in "/" or "\" after the "|" leaves out folders. "" takes everything.
	Filter string   `json:"filter"`
	Links  LinkMode `json:"links"`

	NoDates bool `json:"noDates"` // don't copy the modification/access/creation dates
	NoPerms bool `json:"noPerms"` // don't copy the permission bits (Unix) / attributes (Windows)
	NoAttrs bool `json:"noAttrs"` // don't copy extended attributes, resource forks (macOS) and alternate data streams (Windows)
	Owner   bool `json:"owner"`   // also copy the owner and group, where the system allows it (Unix, usually root only)
	// TrashReplaced (copy and move, off by default, board #99): a file or link that an overwrite replaces goes to the
	// system trash first instead of being replaced in place. A copy still writes the new contents to a temporary file
	// first, so the old file is trashed only once they are complete; a folder in the way is never trashed.
	TrashReplaced bool `json:"trashReplaced"`
	Verify  bool `json:"verify"`  // re-read every copy and compare its SHA-256 with the source's

	SpeedLimit int64 `json:"speedLimit"` // bytes per second, 0 = no limit; can be changed while running (SetSpeedLimit)
	BufferSize int   `json:"bufferSize"` // copy buffer in bytes; 0 = 1 MiB. Large values plus NoCache are TC's "big file" mode
	NoCache    bool  `json:"noCache"`    // ask the system not to keep the copied bytes in its file cache

	OnError     ErrorPolicy `json:"onError"`     // see ErrorPolicy
	Retries     int         `json:"retries"`     // automatic retries of a failed file before OnError applies
	IgnoreSpace bool        `json:"ignoreSpace"` // don't check the free space of the target before a copy
	Queued      bool        `json:"queued"`      // wait in the queue lane instead of starting at once

	ConfirmDirs     bool `json:"confirmDirs"`     // delete/wipe: ask before removing each non-empty folder (Yes/All/Skip)
	ConfirmReadOnly bool `json:"confirmReadOnly"` // delete/wipe: ask before removing each read-only file
	WipePasses      int  `json:"wipePasses"`      // wipe: how many times the contents are overwritten; 0 = 1

	// NoJournal keeps the job out of the app's undo journal (the jobs an undo itself runs); the engine ignores it.
	NoJournal bool `json:"noJournal,omitempty"`
	// FindTrashed (trash jobs) asks the engine to find where each item went in the trash and report it in the "trash"
	// log entry (Entry.Trashed), so the undo journal can put it back. The app sets it only while it keeps the journal;
	// the page can't, because looking through the trash after every item costs time nobody else needs.
	FindTrashed bool `json:"-"`
}

// AskType tells which question a Conflict is.
type AskType string

const (
	AskExists         AskType = ""                // the target file exists (the classic overwrite dialog)
	AskReadOnly       AskType = "readonly"        // the target that would be overwritten is read-only
	AskError          AskType = "error"           // a file failed: Retry / Skip / Skip all / Cancel
	AskNoSpace        AskType = "no_space"        // the target volume has less free space than the job needs
	AskDeleteDir      AskType = "delete_dir"      // delete a non-empty folder with all its contents?
	AskDeleteReadOnly AskType = "delete_readonly" // delete a read-only file?
)

// Conflict is a question the job asks. The fields used depend on Type; Choices lists the answers that make sense.
type Conflict struct {
	Type    AskType  `json:"type"`
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	SrcSize int64    `json:"srcSize"`
	DstSize int64    `json:"dstSize"`
	SrcTime int64    `json:"srcTime"` // Unix milliseconds
	DstTime int64    `json:"dstTime"`
	DstDir  bool     `json:"dstDir"`         // the target is a folder (a file can't overwrite it)
	Same    *bool    `json:"same,omitempty"` // set after ChoiceCompare: whether the two files have the same content
	Error   string   `json:"error,omitempty"`
	Need    int64    `json:"need,omitempty"` // AskNoSpace: bytes needed
	Free    int64    `json:"free,omitempty"` // AskNoSpace: bytes free
	Choices []Choice `json:"choices"`
}

// Choice is the answer to a Conflict.
type Choice string

const (
	ChoiceOverwrite       Choice = "overwrite"
	ChoiceSkip            Choice = "skip"
	ChoiceOverwriteAll    Choice = "overwrite_all"
	ChoiceSkipAll         Choice = "skip_all"
	ChoiceOlderAll        Choice = "older_all"         // overwrite all older targets, skip the rest
	ChoiceRename          Choice = "rename"            // copy under Answer.NewName instead
	ChoiceAutoRename      Choice = "auto_rename"       // this copy gets a free name: "file(2).txt"
	ChoiceAutoRenameAll   Choice = "auto_rename_all"   // ... and so do all later ones
	ChoiceRenameTarget    Choice = "rename_target"     // the existing file is renamed to a free name, the copy takes its place
	ChoiceRenameTargetAll Choice = "rename_target_all" // ... for all later conflicts too
	ChoiceLargerAll       Choice = "larger_all"        // overwrite when the source is larger, skip otherwise, for all
	ChoiceSmallerAll      Choice = "smaller_all"       // overwrite when the source is smaller, skip otherwise, for all
	ChoiceAppend          Choice = "append"            // add the source's bytes to the end of the target
	ChoiceResume          Choice = "resume"            // the target is a broken-off copy: copy only the missing rest
	ChoiceCompare         Choice = "compare"           // compare the contents; the question comes back with Same set
	ChoiceRetry           Choice = "retry"             // AskError: try the file again
	ChoiceYes             Choice = "yes"               // AskDeleteDir / AskDeleteReadOnly / AskNoSpace: go ahead
	ChoiceAll             Choice = "all"               // ... and don't ask again in this job
	ChoiceDelete          Choice = "delete"            // trash failed: delete this one permanently instead
	ChoiceTrashDirect     Choice = "trash_direct"      // trash through Finder failed (macOS): trash this one without Finder
	ChoiceTrashDirectAll  Choice = "trash_direct_all"  // ... and every later item of the job too
	ChoiceCancel          Choice = "cancel"
)

// Answer is what the user said to a Conflict.
type Answer struct {
	Choice  Choice `json:"choice"`
	NewName string `json:"newName"` // for ChoiceRename: the name (not a path) to use in the target folder
}

// State of a job.
const (
	StateQueued    = "queued" // waiting in the queue lane
	StateRunning   = "running"
	StatePaused    = "paused"
	StateAsking    = "asking"
	StateDone      = "done"
	StateFailed    = "failed" // finished, but some files had errors
	StateCancelled = "cancelled"
)

// Progress is the state of a job as the UI shows it (progress dialog and background transfer manager).
type Progress struct {
	ID          int       `json:"id"`
	Kind        Kind      `json:"kind"`
	State       string    `json:"state"`
	Queued      bool      `json:"queued"` // the job belongs to the queue lane
	Target      string    `json:"target"`
	Files       int       `json:"files"` // files (and links) the job works on, found before it starts
	FilesDone   int       `json:"filesDone"`
	Bytes       int64     `json:"bytes"`
	BytesDone   int64     `json:"bytesDone"`
	Current     string    `json:"current"`     // the file being worked on
	CurrentSize int64     `json:"currentSize"` // its size
	CurrentDone int64     `json:"currentDone"` // bytes of it done
	Speed       int64     `json:"speed"`       // bytes per second over the last few seconds
	ETA         int64     `json:"eta"`         // seconds left, -1 when unknown
	Elapsed     int64     `json:"elapsed"`     // milliseconds of work so far, pauses not counted
	SpeedLimit  int64     `json:"speedLimit"`  // bytes per second, 0 = none
	Errors      []string  `json:"errors"`      // plain sentences, at most maxErrors
	Skipped     int       `json:"skipped"`
	Verified    int       `json:"verified"` // files re-read and found equal (Verify)
	Ask         *Conflict `json:"ask,omitempty"`
	Summary     string    `json:"summary,omitempty"` // a sentence with the statistics, set when the job ends
	// Left lists, for a copy or move that has ended, the sources (as given in the request) that did not get across
	// whole: one of their files was skipped or failed, or the job stopped before reaching them. The panel keeps just
	// these selected, as Total Commander does. It is nil until the job ends and for the other kinds of job.
	Left []string `json:"left"`
	// ErrorCount is the number of errors so far, not capped like Errors, so the progress window can show it.
	ErrorCount int `json:"errorCount"`
	// Failed lists the files and folders that failed, each with its path and the plain-English reason, at most
	// maxFailed of them. The end-of-job error list shows them and "Retry failed" starts a new job from them.
	Failed []Failure `json:"failed"`
}

const maxErrors = 100

// maxFailed caps Progress.Failed, so a job that fails on every one of a million files doesn't fill the memory.
const maxFailed = 10000

// Failure is one file or folder a job could not handle. Source is the path the job worked on ("" when the job as a
// whole failed, e.g. the target folder could not be made); Target is where a copy or move was going ("" for delete,
// trash and wipe, and for a move whose copy worked but whose original could not be removed). Reason is the
// plain-English cause ("permission denied", "the disk is full"); Message is the full sentence shown in Errors.
type Failure struct {
	Source  string `json:"source"`
	Target  string `json:"target"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Event names given to Emit.
const (
	EventProgress = "job"      // Progress, at most every Interval, and at every change of state
	EventAsk      = "job-ask"  // Progress with Ask set
	EventDone     = "job-done" // the final Progress
)
