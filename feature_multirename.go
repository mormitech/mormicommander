package main

// The Multi-Rename tool (Ctrl+M, FEATURES.md section 22) as the page sees it: a thin layer over internal/multirename.
// The tool is modal, so one session at a time lives here: the files as they are on disk ("real"), the names they would
// have after the chained steps of F5 ("virt", the old names the masks work on), new names loaded from a file, the
// clipboard or the editor, and the last preview, which is what Start! executes. The page sends the dialog's settings
// on every keystroke and draws the rows it gets back; sorting and reordering happen here so the counter follows the
// list order. The undo log of the last batch is kept in the settings folder, so Undo still works after a restart.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"mormicommander/internal/launch"
	"mormicommander/internal/multirename"
)

// MultiRename is what the page calls for the Multi-Rename tool (window.go.main.MultiRename).
type MultiRename struct {
	app      *App
	mu       sync.Mutex
	real     []multirename.Item // the files on disk, in list order
	virt     []multirename.Item // the same files with the names of the chained steps (F5)
	names    []string           // new names loaded from a file, the clipboard or the editor; nil = none
	last     *multirename.Result
	lastLog  *multirename.UndoLog // the last batch of this session, for the result list
	editFile string               // the temporary file of "Edit names"
	media    mrMediaCache         // EXIF, image size and audio tag facts for [=img.*] and [=audio.*]
}

func init() {
	registerFeature("multirename", func(a *App) []exposed { return []exposed{expose(&MultiRename{app: a})} })
}

// mrRow is one line of the list for the page.
type mrRow struct {
	Old      string                `json:"old"`
	New      string                `json:"new"`
	Dir      string                `json:"dir"`
	IsDir    bool                  `json:"isDir"`
	Size     int64                 `json:"size"`
	Time     int64                 `json:"time"` // modification time, Unix milliseconds
	Problems []multirename.Problem `json:"problems,omitempty"`
	Warnings []string              `json:"warnings,omitempty"`
}

// mrPreview is the answer to every change in the dialog.
type mrPreview struct {
	Rows       []mrRow  `json:"rows"`
	Warnings   []string `json:"warnings"`
	Errors     int      `json:"errors"`
	Changes    int      `json:"changes"`
	Field      string   `json:"field"`      // the input field with a mistake: nameMask, extMask or search
	FieldError string   `json:"fieldError"` // what is wrong with it
	Names      int      `json:"names"`      // number of loaded names, -1 when none
	Steps      bool     `json:"steps"`      // true when F5 steps are chained
}

// mrUndoInfo tells whether the last batch can be undone.
type mrUndoInfo struct {
	Available bool   `json:"available"`
	Count     int    `json:"count"`
	Time      string `json:"time"`
}

// mrStart is what the page gets when the tool opens.
type mrStart struct {
	Rows     []mrRow              `json:"rows"`
	Defaults multirename.Settings `json:"defaults"`
	Fields   []string             `json:"fields"`
	Media    []string             `json:"media"` // the media fields ([=img.exifdate], [=audio.artist], ...)
	Presets  []string             `json:"presets"`
	Undo     mrUndoInfo           `json:"undo"`
}

// mrExecResult is the outcome of Start!.
type mrExecResult struct {
	Renamed   int    `json:"renamed"`
	UndoSaved bool   `json:"undoSaved"`
	Note      string `json:"note"`
}

func mrUndoPath() string {
	if d := configDir(); d != "" {
		return filepath.Join(d, "multirename-undo.json")
	}
	return ""
}

func mrPresetDir() string {
	if d := configDir(); d != "" {
		return filepath.Join(d, "multirename-presets")
	}
	return ""
}

// Start opens a session for the given entries of a folder (names may hold sub-paths, as in a branch view).
func (m *MultiRename) Start(dir string, names []string) (mrStart, error) {
	if m.app.mock != nil {
		return mrStart{}, errors.New("This is the demo: its files are not real, so they can't be renamed.")
	}
	if strings.Contains(dir, "://") {
		return mrStart{}, errors.New("Files in archives and on servers can't be renamed with the Multi-Rename tool.")
	}
	if len(names) == 0 {
		return mrStart{}, errors.New("Select the files to rename first.")
	}
	items := make([]multirename.Item, 0, len(names))
	for _, n := range names {
		full := filepath.Clean(filepath.Join(dir, n))
		fi, err := os.Lstat(full)
		if err != nil {
			return mrStart{}, fmt.Errorf("%s is not there any more.", full)
		}
		items = append(items, multirename.Item{Dir: filepath.Dir(full), Name: filepath.Base(full), IsDir: fi.IsDir(), Size: fi.Size(), ModTime: fi.ModTime()})
	}
	m.mu.Lock()
	m.cleanupLocked()
	m.real = items
	m.virt = append([]multirename.Item(nil), items...)
	m.names, m.last, m.lastLog = nil, nil, nil
	m.media = mrMediaCache{}
	rows := m.plainRowsLocked()
	m.mu.Unlock()
	presets, _ := m.Presets()
	return mrStart{Rows: rows, Defaults: multirename.DefaultSettings(), Fields: multirename.FieldNames(), Media: mrMediaFieldNames, Presets: presets, Undo: m.UndoInfo()}, nil
}

// Close ends the session (the dialog was closed).
func (m *MultiRename) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked()
	m.real, m.virt, m.names, m.last, m.lastLog = nil, nil, nil, nil, nil
	m.media = mrMediaCache{}
}

func (m *MultiRename) cleanupLocked() {
	if m.editFile != "" {
		os.Remove(m.editFile)
		m.editFile = ""
	}
}

func mrRowOf(it multirename.Item, oldName string) mrRow {
	return mrRow{Old: oldName, New: oldName, Dir: it.Dir, IsDir: it.IsDir, Size: it.Size, Time: it.ModTime.UnixMilli()}
}

func (m *MultiRename) plainRowsLocked() []mrRow {
	rows := make([]mrRow, len(m.real))
	for i, it := range m.real {
		rows[i] = mrRowOf(it, m.virt[i].Name)
	}
	return rows
}

// computeLocked builds the preview for s: the masks work on the chained names, loaded names win, and the conflict
// check runs against the real files.
func (m *MultiRename) computeLocked(s multirename.Settings) (*multirename.Result, error) {
	r, err := multirename.Compile(s)
	if err != nil {
		return nil, err
	}
	// Media fields are read from the files on disk (m.real), whatever names the chained F5 steps gave them.
	pos := make(map[*multirename.Item]int, len(m.virt))
	for i := range m.virt {
		pos[&m.virt[i]] = i
	}
	r.SetFieldSource(func(it *multirename.Item, name string) (multirename.FieldValue, bool) {
		if i, ok := pos[it]; ok {
			return m.media.source(&m.real[i], name)
		}
		return m.media.source(it, name)
	})
	names, warns := r.NewNames(m.virt)
	var extra []string
	if m.names != nil {
		if len(m.names) != len(names) {
			extra = append(extra, fmt.Sprintf("%d names loaded for %d files.", len(m.names), len(names)))
		}
		for i := range names {
			if i < len(m.names) {
				names[i] = m.names[i]
			}
		}
	}
	res := r.Preview(m.real, multirename.PreviewOptions{Names: names})
	for i := range res.Entries {
		res.Entries[i].Warnings = warns[i]
	}
	res.Warnings = append(extra, res.Warnings...)
	return res, nil
}

func mrFieldError(err error) (string, string) {
	var se *multirename.SettingsError
	if errors.As(err, &se) {
		return se.Field, mrSentence(se.Err.Error())
	}
	return "", mrSentence(err.Error())
}

// mrSentence turns an engine message into a sentence for the dialog.
func mrSentence(s string) string {
	s = strings.TrimPrefix(s, "multirename: ")
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// Preview computes the new names for the dialog's settings.
func (m *MultiRename) Preview(s multirename.Settings) mrPreview {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := mrPreview{Names: -1, Steps: m.stepsLocked(), Warnings: []string{}}
	if m.names != nil {
		p.Names = len(m.names)
	}
	res, err := m.computeLocked(s)
	if err != nil {
		m.last = nil
		p.Field, p.FieldError = mrFieldError(err)
		p.Rows = m.plainRowsLocked()
		for i := range p.Rows {
			p.Rows[i].New = "<Error!>"
		}
		p.Errors = len(p.Rows)
		return p
	}
	m.last = res
	p.Rows = make([]mrRow, len(res.Entries))
	for i, e := range res.Entries {
		row := mrRowOf(e.Item, m.virt[i].Name)
		row.New = e.NewName
		for _, pr := range e.Problems {
			pr.Detail = mrSentence(pr.Detail)
			row.Problems = append(row.Problems, pr)
		}
		row.Warnings = e.Warnings
		p.Rows[i] = row
	}
	for _, w := range res.Warnings {
		p.Warnings = append(p.Warnings, mrSentence(w))
	}
	p.Errors, p.Changes = res.Errors, res.Changes
	return p
}

func (m *MultiRename) stepsLocked() bool {
	for i := range m.real {
		if m.real[i].Name != m.virt[i].Name {
			return true
		}
	}
	return false
}

// Sort sorts the list by a column: name, ext, size, date or path; the counter follows the new order.
func (m *MultiRename) Sort(key string, descending bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	const idx = "\x00mr-index"
	tmp := make([]multirename.Item, len(m.virt))
	for i, it := range m.virt {
		f := map[string]string{idx: strconv.Itoa(i)}
		for k, v := range it.Fields {
			f[k] = v
		}
		it.Fields = f
		tmp[i] = it
	}
	multirename.SortItems(tmp, multirename.SortKey(key), descending)
	order := make([]int, len(tmp))
	for i, it := range tmp {
		order[i], _ = strconv.Atoi(it.Fields[idx])
	}
	m.reorderLocked(order)
}

// Reorder puts the rows in a new order: order[i] is the old position of the row that goes to position i.
func (m *MultiRename) Reorder(order []int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(order) != len(m.real) {
		return errors.New("The new order does not match the list.")
	}
	seen := make([]bool, len(order))
	for _, o := range order {
		if o < 0 || o >= len(order) || seen[o] {
			return errors.New("The new order does not match the list.")
		}
		seen[o] = true
	}
	m.reorderLocked(order)
	return nil
}

func (m *MultiRename) reorderLocked(order []int) {
	real := make([]multirename.Item, len(order))
	virt := make([]multirename.Item, len(order))
	for i, o := range order {
		real[i], virt[i] = m.real[o], m.virt[o]
	}
	m.real, m.virt = real, virt
}

// NextStep (F5) takes the current new names as the old names of a further step; nothing is renamed yet.
func (m *MultiRename) NextStep(s multirename.Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.computeLocked(s)
	if err != nil {
		_, msg := mrFieldError(err)
		return errors.New(msg)
	}
	for i, e := range res.Entries {
		m.virt[i].Name = e.NewName
	}
	m.names = nil
	return nil
}

// Reset forgets the chained steps and the loaded names (the "Default" item).
func (m *MultiRename) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.virt = append([]multirename.Item(nil), m.real...)
	m.names = nil
}

// ClearNames forgets the loaded names.
func (m *MultiRename) ClearNames() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.names = nil
}

func (m *MultiRename) setNames(data []byte) int {
	names := multirename.ParseNames(data)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.names = names
	if m.names == nil {
		m.names = []string{}
	}
	return len(m.names)
}

// LoadNamesFile asks for a text file and takes its lines as the new names; -1 when the user cancelled.
func (m *MultiRename) LoadNamesFile() (int, error) {
	if m.app.ctx == nil {
		return -1, errors.New("There is no window to ask in.")
	}
	path, err := m.app.openFileDialog("Load names from file", "",
		[]fileFilter{{DisplayName: "Text files", Pattern: "*.txt;*.lst;*.csv"}, {DisplayName: "All files", Pattern: "*"}})
	if err != nil {
		return -1, err
	}
	if path == "" {
		return -1, nil
	}
	return m.LoadNamesFrom(path)
}

// LoadNamesFrom takes the lines of the text file at path as the new names.
func (m *MultiRename) LoadNamesFrom(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1, fmt.Errorf("Could not read %s.", path)
	}
	return m.setNames(data), nil
}

// LoadNamesClipboard takes the lines of the text on the clipboard as the new names.
func (m *MultiRename) LoadNamesClipboard() (int, error) {
	if m.app.ctx == nil {
		return -1, errors.New("The clipboard can't be read here.")
	}
	text, err := m.app.clipboardText()
	if err != nil {
		return -1, errors.New("The clipboard can't be read.")
	}
	if strings.TrimSpace(text) == "" {
		return -1, errors.New("The clipboard holds no text.")
	}
	return m.setNames([]byte(text)), nil
}

// EditNames writes the current new names to a text file and opens it in the editor; ReadEditedNames loads it back.
func (m *MultiRename) EditNames(s multirename.Settings) (string, error) {
	m.mu.Lock()
	var names []string
	if res, err := m.computeLocked(s); err == nil {
		names = multirename.NewNamesOf(res)
	} else {
		for _, it := range m.virt {
			names = append(names, it.Name)
		}
	}
	m.cleanupLocked()
	f, err := os.CreateTemp("", "mormicommander-names-*.txt")
	if err != nil {
		m.mu.Unlock()
		return "", errors.New("Could not make a temporary file for the names.")
	}
	_, werr := f.Write(multirename.FormatNames(names, runtime.GOOS == "windows"))
	f.Close()
	if werr != nil {
		os.Remove(f.Name())
		m.mu.Unlock()
		return "", errors.New("Could not write the names to a temporary file.")
	}
	m.editFile = f.Name()
	path := m.editFile
	m.mu.Unlock()
	if err := mrOpenEditor(m.app, path); err != nil {
		return path, fmt.Errorf("Could not open the editor: %v", err)
	}
	return path, nil
}

// mrOpenEditor opens path in the editor of the setting "editor" (program and parameters), or the system's.
func mrOpenEditor(a *App, path string) error {
	if os.Getenv("MORMICOMMANDER_NO_EDITOR") != "" { // tests and QA runs: the file is edited by the test itself
		return nil
	}
	l := launch.New()
	if a.settings != nil {
		if s, ok := a.settings.Get("editor").(string); ok && strings.TrimSpace(s) != "" {
			return l.OpenInTool(launch.ParseTool(s), path, launch.ParamContext{File: path})
		}
	}
	c, err := l.DefaultEditorCommand(path)
	if err != nil {
		return err
	}
	return l.Start(c)
}

// ReadEditedNames loads the names back from the file of EditNames.
func (m *MultiRename) ReadEditedNames() (int, error) {
	m.mu.Lock()
	path := m.editFile
	m.mu.Unlock()
	if path == "" {
		return -1, errors.New("No names are being edited.")
	}
	return m.LoadNamesFrom(path)
}

// Execute renames the files of the last preview, saves the undo log and makes the new names the list's old names.
func (m *MultiRename) Execute() (mrExecResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := m.last
	if res == nil {
		return mrExecResult{}, errors.New("The masks have a mistake; fix it first.")
	}
	if res.HasErrors() {
		return mrExecResult{}, fmt.Errorf("%d new names have problems (shown in red); fix them first.", res.Errors)
	}
	if res.Changes == 0 {
		return mrExecResult{Note: "No name changes."}, nil
	}
	total := res.Changes
	log, err := multirename.Execute(res, multirename.ExecOptions{Progress: func(done, all int) {
		m.app.emit("multirename-progress", map[string]int{"done": done, "total": all})
	}})
	if err != nil {
		return mrExecResult{}, mrExecErr(err)
	}
	out := mrExecResult{Renamed: total}
	if p := mrUndoPath(); p != "" {
		if os.MkdirAll(filepath.Dir(p), 0o755) == nil && multirename.SaveUndoLog(p, log) == nil {
			out.UndoSaved = true
		}
	}
	if !out.UndoSaved {
		out.Note = "The undo record could not be saved."
	}
	// The renamed files are the list's files now; folders renamed in the batch move the files inside them too.
	moved := map[string]string{}
	for _, e := range res.Entries {
		if e.Changed() {
			moved[e.Item.Path()] = e.NewPath()
		}
	}
	notesInstance.movedMany(moved) // notes and colour labels go along (feature_notes.go)
	moveComments(moved)            // and so do the descript.ion comments (F-745)
	for i, it := range m.real {
		np := mrMapPath(it.Path(), moved)
		m.real[i].Dir, m.real[i].Name = filepath.Dir(np), filepath.Base(np)
	}
	m.virt = append([]multirename.Item(nil), m.real...)
	m.names, m.last, m.lastLog = nil, nil, log
	return out, nil
}

func mrExecErr(err error) error {
	if errors.Is(err, multirename.ErrHasConflicts) {
		return errors.New("Some new names have problems (shown in red); fix them first.")
	}
	var ee *multirename.ExecError
	if errors.As(err, &ee) {
		why := ee.Err.Error()
		if errors.Is(ee.Err, multirename.ErrTargetExists) {
			why = "a file with that name appeared in the meantime"
		} else {
			var pe *os.LinkError
			if errors.As(ee.Err, &pe) {
				why = pe.Err.Error()
			}
		}
		msg := fmt.Sprintf("Could not rename %s to %s: %s.", filepath.Base(ee.From), filepath.Base(ee.To), why)
		if len(ee.RollbackErrs) > 0 {
			msg += fmt.Sprintf(" %d of the finished renames could not be taken back; check the folder.", len(ee.RollbackErrs))
		} else {
			msg += " Nothing was renamed: the finished renames were taken back."
		}
		return errors.New(msg)
	}
	return errors.New(mrSentence(err.Error()))
}

// mrMapPath gives the path p after the renames in moved (old full path → new full path). It walks from the deepest
// part up, so a file inside a renamed folder that was renamed itself gets both changes. Each rename stays inside its
// folder, so only the last part of a matching prefix changes.
func mrMapPath(p string, moved map[string]string) string {
	sep := string(filepath.Separator)
	comps := strings.Split(p, sep)
	for i := len(comps); i >= 1; i-- {
		if to, ok := moved[strings.Join(comps[:i], sep)]; ok {
			comps[i-1] = filepath.Base(to)
		}
	}
	return strings.Join(comps, sep)
}

// UndoInfo tells whether a saved batch can be undone.
func (m *MultiRename) UndoInfo() mrUndoInfo {
	p := mrUndoPath()
	if p == "" {
		return mrUndoInfo{}
	}
	log, err := multirename.LoadUndoLog(p)
	if err != nil || log == nil || len(log.Ops) == 0 {
		return mrUndoInfo{}
	}
	return mrUndoInfo{Available: true, Count: len(log.Ops), Time: log.Time.Local().Format("2006-01-02 15:04")}
}

// Undo gives the files of the last batch their old names back and forgets the batch (one level of undo).
func (m *MultiRename) Undo() (int, error) {
	p := mrUndoPath()
	if p == "" {
		return 0, errors.New("There is nothing to undo.")
	}
	log, err := multirename.LoadUndoLog(p)
	if err != nil || log == nil || len(log.Ops) == 0 {
		return 0, errors.New("There is nothing to undo.")
	}
	if err := multirename.Undo(log, multirename.ExecOptions{}); err != nil {
		return 0, mrExecErr(err)
	}
	os.Remove(p)
	back := map[string]string{}
	for _, o := range log.Ops {
		back[filepath.Join(o.Dir, o.NewName)] = filepath.Join(o.Dir, o.OldName)
	}
	m.mu.Lock()
	for i, it := range m.real {
		np := mrMapPath(it.Path(), back)
		m.real[i].Dir, m.real[i].Name = filepath.Dir(np), filepath.Base(np)
	}
	m.virt = append([]multirename.Item(nil), m.real...)
	m.names, m.last = nil, nil
	m.mu.Unlock()
	return len(log.Ops), nil
}

// ResultText lists "old → new" for the last batch of this session, or for the changes of the current preview.
func (m *MultiRename) ResultText() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	if m.lastLog != nil {
		fmt.Fprintf(&b, "Renamed %s:\n", m.lastLog.Time.Local().Format("2006-01-02 15:04:05"))
		for _, o := range m.lastLog.Ops {
			fmt.Fprintf(&b, "%s → %s\n", filepath.Join(o.Dir, o.OldName), o.NewName)
		}
		return b.String()
	}
	if m.last != nil {
		b.WriteString("Not renamed yet:\n")
		for _, e := range m.last.Entries {
			if e.Changed() {
				fmt.Fprintf(&b, "%s → %s\n", e.Item.Path(), e.NewName)
			}
		}
	}
	return b.String()
}

// Presets lists the saved settings by name.
func (m *MultiRename) Presets() ([]string, error) {
	d := mrPresetDir()
	if d == "" {
		return []string{}, nil
	}
	list, err := multirename.ListPresets(d)
	if err != nil {
		return []string{}, nil
	}
	sort.SliceStable(list, func(i, j int) bool { return multirename.NaturalCompare(list[i], list[j]) < 0 })
	if list == nil {
		list = []string{}
	}
	return list, nil
}

// SavePreset saves the settings under a name, replacing a preset of that name.
func (m *MultiRename) SavePreset(name string, s multirename.Settings) error {
	d := mrPresetDir()
	if d == "" {
		return errors.New("There is no settings folder to save in.")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("Give the settings a name.")
	}
	if len(name) > 252 {
		return errors.New("The name is too long (at most 252 characters).")
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return errors.New("Could not make the folder for saved settings.")
	}
	if err := multirename.SavePreset(d, name, s); err != nil {
		return fmt.Errorf("Could not save the settings: %v", err)
	}
	return nil
}

// LoadPreset reads the settings saved under a name.
func (m *MultiRename) LoadPreset(name string) (multirename.Settings, error) {
	s, err := multirename.LoadPreset(mrPresetDir(), name)
	if err != nil {
		return s, fmt.Errorf("Could not load the settings %q.", name)
	}
	return s, nil
}

// DeletePreset removes the settings saved under a name.
func (m *MultiRename) DeletePreset(name string) error {
	if err := multirename.DeletePreset(mrPresetDir(), name); err != nil {
		return fmt.Errorf("Could not delete the settings %q.", name)
	}
	return nil
}
