# Spec area 1: Copy, Move, Delete and the transfer queue

This document describes how Total Commander 11 behaves when copying (F5), moving or renaming (F6) and deleting (F8). It also covers the overwrite prompt, the progress window, the Background Transfer Manager (the queue), verify after copy and the speed limit. It is written so that someone building mormiCommander's dialogs and its file-op job engine can copy the behaviour without having TC installed. Where TC's behaviour is Windows-specific (NTFS permissions, the Recycle Bin, drive letters), the spec gives the macOS/Linux equivalent we should ship instead.

**How sure each part is.** TC's help file is not online in a form we can fetch reliably. Most of this spec comes from long-standing knowledge of TC behaviour, the ghisler.ch wiki's wincmd.ini page, and forum threads where the author answers questions (see Sources at the end). Anything marked **[unverified]** is our best reconstruction (exact label wording, accelerator letters, a default) and should be checked against a real TC 11 install before a pixel-exact clone is attempted. The overall behaviour is not in doubt; the small details may be.

FEATURES.md IDs are given in brackets, for example [F-154].

---

## 1. Commands and hotkeys

| Key | Command | What happens | FEATURES |
|---|---|---|---|
| F5 | cm_Copy | Copy dialog. The target defaults to the other panel's folder | F-154, F-205, F-457 |
| Shift+F5 | cm_CopySamepanel | Copy dialog. The target defaults to the file name under the cursor, in the same folder | F-155, F-215, F-479 |
| F6 | cm_RenMov | Move/rename dialog. The target defaults to the other panel's folder | F-157, F-206, F-503 |
| Shift+F6 | cm_RenameOnly | Inline rename in the list, not a dialog (see area for rename; listed here only to separate it from F6) | F-146, F-158, F-216, F-502 |
| F8 / Delete | cm_Delete | Delete confirmation. Goes to the trash when "Delete to Recycle Bin" is on | F-165, F-208, F-493, F-809 |
| Shift+F8 / Shift+Delete | cm_Delete with the trash bypassed | Permanent delete confirmation | F-166, F-217, F-494 |
| Ctrl+Shift+F5 | cm_CreateShortcut | Create a shortcut or link in the target (outside this area; listed so the key is not reused) | F-178, F-277 |
| Alt+F5 / Alt+Shift+F5 | cm_PackFiles | Pack dialog / pack and move (packer area) | F-156, F-222, F-223 |
| (none) | cm_OpenTransferManager / cm_BackgroundTransferManager | Opens the Background Transfer Manager window (empty if nothing is queued) | F-382, F-470 |
| Ctrl+C / Ctrl+X / Ctrl+V | clipboard copy/cut/paste of files | Paste runs the same copy/move engine with no dialog, but with overwrite prompts | F-237, F-739 |

**F8/Shift+F8 swap rule.** If "Delete to Recycle Bin" (UseTrash) is turned off in Configuration, F8 deletes permanently and Shift+F8 deletes to the trash. The confirmation text always states which of the two will happen [F-810].

**What the command acts on (all of F5, F6, F8).**
1. If one or more items are selected in the source panel, the command acts on the selection. The cursor item is ignored unless it is also selected.
2. If nothing is selected, the command acts on the item under the cursor.
3. If nothing is selected and the cursor is on `..`, nothing happens. TC beeps or does nothing. mormiCommander shows a short status message such as "No files selected" [F-832].
4. In an empty folder (only `..`), the same as 3.

**Multi-key detail.** Holding Shift while clicking the F5/F6/F8 buttons in the bottom function-key bar runs the Shift variant, the same as the keyboard.

---

## 2. Copy dialog (F5)

### 2.1 Layout

The dialog is small and modal. From top to bottom [F-457, F-829, F-830]:

1. **Title bar**: "Total Commander" **[unverified; some builds say "Copy"]**. mormiCommander: "Copy".
2. **Prompt label** (static text), one of:
   - one file: `Copy "name.ext" to:`
   - one folder: `Copy "folder" to:` (TC writes the folder name without a trailing slash)
   - several items: `Copy 3 file(s)/dir(s) to:` **[unverified on exact plural wording]**; recent TC builds give the counts separately, e.g. `Copy 2 file(s) and 1 directory(s) to:`. mormiCommander: `Copy 2 files and 1 folder to:` with correct English plurals.
3. **Target combo box** (editable, with dropdown). Its initial text is the other panel's current path with a trailing separator, for example `D:\Backup\`. The whole text is selected so that typing replaces it. The dropdown lists:
   - the folders of the other panel's tabs, then
   - recent copy/move targets (history, newest first, up to about 20 entries). This history is shared by F5 and F6.
   Typing a path auto-completes folder names [F-782].
4. **"Only files of this type:"** label and an editable combo box, default `*.*`. Next to it a small button opens the saved selection filters (the same filter templates as Ctrl+F12 / "Define" in the file-type filter) [F-457].
5. **Option check boxes** (shown below the filter in TC 9 and later; some are behind "Options" in older builds) **[unverified layout]**:
   - `Copy NTFS permissions (may need administrator rights)` (default off). On macOS/Linux label this `Copy owner and permissions` [F-473].
   - `Verify` (default from Configuration: Copy/Delete page) [F-472].
   - `Copy date/time of directories`? **[unverified that this is in the dialog; it is certainly in Configuration]**.
6. **Button row** (left to right):
   - **OK** (default button, Enter)
   - **F2 Queue** (the label really reads "F2 Queue") [F-468]
   - **Tree** (F10) [F-829]
   - **Options** (opens the extended options described in 2.4) **[unverified that it is a separate button in TC 11; it may be a drop-down arrow on OK]**
   - **Cancel** (Esc)
   - **Help** (F1)

Tab order: target combo, filter combo, filter templates button, check boxes top to bottom, OK, F2 Queue, Tree, Options, Cancel, Help.

### 2.2 Keys inside the dialog

| Key | Effect |
|---|---|
| Enter | OK: start copying in the foreground with the progress window |
| Esc | Cancel. Nothing happens |
| F2 | Same as the "F2 Queue" button: hand the job to the Background Transfer Manager (see 6) |
| F10 | Tree: opens a folder tree of the target drive. Picking a folder writes its path into the target field. Esc in the tree returns without change |
| Alt+Down / F4 | Open the target history dropdown |
| Shift+F5 / Shift+F6 (while the dialog is open) | No effect |
| Ctrl+Backspace | Delete the word before the caret (standard edit control) |
| F1 | Help |

### 2.3 How the target text is interpreted

This is the part that must be exact, because users rely on it for quick renaming [F-458, F-459]:

1. **Ends in a separator** (`D:\Backup\`, `/tmp/x/`): it is a folder. Items keep their names. If the folder does not exist, TC asks `The directory D:\Backup\ does not exist. Create it?` **[unverified wording]** with Yes/No; Yes creates the whole path.
2. **Names an existing folder without a trailing separator**: same as 1.
3. **Does not exist and one file is being copied**: the last part is the new file name. `x\b.txt` copies `a.txt` to `x\b.txt`.
4. **Does not exist and several items are copied**: the last part is treated as a new folder name, which is created, and all items go inside.
5. **Contains wildcards** (`*.bak`, `*_old.*`, `D:\x\*.txt`): the last part is a rename mask applied to each copied file. `*` takes the matching part of the source name; `?` takes one character; a literal text replaces. `*.bak` turns `a.txt` into `a.bak`. `*.*` means "keep the name". Folders keep their own names; the mask applies to files only, inside every folder copied.
6. **Relative path** (`sub\`, `..\other\`): relative to the **source** folder, not the target panel. **[unverified for TC 11; this has always been TC behaviour]**
7. **Empty target**: OK is accepted and copies into the source folder, which then becomes the "copy onto itself" case (see 9).
8. **Archive path** (`D:\x.zip\sub\`): the target is inside an archive; the packer handles it [F-482].
9. **FTP/plugin path** (`\\\Plugin\...`, `ftp://`): handled by the file system plugin [F-484].

### 2.4 Options (extended options)

TC 9+ has an extended part of the dialog with copy options that apply to this one operation **[unverified exact list and labels]**:

- **Overwrite mode** dropdown: `Ask user` (default), `Overwrite all`, `Skip all`, `Overwrite all older`, `Auto-rename copied files`, `Auto-rename target files`, `Copy all larger`, `Copy all smaller` [F-490]. Picking anything but "Ask user" pre-answers the overwrite dialog for the whole job.
- `Copy NTFS permissions` [F-473].
- `Copy symbolic links/junctions as links` (otherwise the link target content is copied) [F-476].
- `Verify` [F-472].
- `Ignore all missing / unreadable files` (do not stop on read errors; log them) **[unverified]**.
- `Use default copy method only` (no big-file mode, no compatibility tricks) [F-478].

mormiCommander adds, under the same heading: `Copy extended attributes and resource forks` [F-475], `Copy file comments (descript.ion)` [F-745].

### 2.5 Only files of this type

When the filter is not `*.*`, only matching files are copied. Folders are always walked recursively; a folder that ends up with no matching files is still created **[unverified; TC creates empty folders when the filter is used on a folder selection]**. Several masks are separated by spaces or `;`. A `|` separates include masks from exclude masks: `*.txt *.md | old*` copies .txt and .md files except those starting with "old". A mask ending in `\` matches folders: `| .git\` skips .git folders. Saved filter names in angle brackets select that filter.

---

## 3. Shift+F5: copy in the same folder

Same dialog, but the target field contains only the **name** of the cursor item (selected without the extension **[unverified]**), and the copy goes into the source folder [F-155, F-479]. With several items selected, the field shows `*.*` and the user must type a mask (such as `*_copy.*`) or a folder. Entering an unchanged name means copying a file onto itself, which TC refuses with `Cannot copy a file to itself!` **[unverified wording]**. mormiCommander may offer the auto-name `name (2).ext` instead [F-464].

---

## 4. Move/rename dialog (F6)

The layout is identical to the copy dialog with these differences [F-157, F-461, F-462, F-488, F-831]:

- Prompt label: `Rename/move "name" to:` or `Rename/move 3 file(s)/dir(s) to:`.
- No `Verify` check box when the move is on the same volume (nothing is copied); it is still shown, and applies, for cross-volume moves **[unverified]**.
- F2 Queue works the same way.

Engine rules:

1. **Same volume**: every top-level item is renamed. No data is copied, so moving a 10 GB folder is instant. If the target already contains an item with the same name, the overwrite prompt appears for files. For folders, the folder is merged: TC moves the content file by file into the existing folder, asking for each clash.
2. **Different volume**: copy, then delete. For each file the source is deleted only after its copy finished with no error (and passed verification when Verify is on). Source folders are removed once they are empty. If a file was skipped or failed, its folder stays, together with the skipped file.
3. **Case-only rename** (`a.txt` to `A.txt`) on a case-insensitive volume works [F-487].
4. **Move a folder into itself or its subfolder** is refused (see 9).
5. **Move onto itself** (unchanged target in the same folder): nothing happens, no error.

---

## 5. Progress window (foreground operation)

Shown while a foreground copy/move/delete runs [F-471]. It is modal to the main window **[TC 11: it is a normal window that can be moved; the panels are not usable while it is open]**.

Contents, top to bottom:

1. Title: percentage and operation, e.g. `37% Copying` **[unverified exact form]**, so the taskbar shows progress. mormiCommander also shows Dock/taskbar progress [F-749].
2. `Copying` and the source path of the current file; `to` and the target path.
3. **Current file** progress bar.
4. **Total** progress bar. It only appears once counting is finished; with CountMarked=0 there is no total bar for folder trees.
5. Speed and time left text, e.g. `12.4 MB/s, 00:01:12 remaining` **[unverified layout; TC shows these in the title area or under the bars]**.
6. Buttons: **Background** (F2) (hands the rest of the job to the Background Transfer Manager), **Pause** (toggles to **Continue**), **Cancel**.

Behaviour:
- **Cancel** asks nothing and stops after the current block. The partly copied target file is deleted. Files already copied stay [F-471].
- **Pause** stops reading/writing; the speed shows 0.
- Esc = Cancel.
- During counting (before the first byte is copied) the window says `Counting files...` **[unverified]** and Cancel stops right away.
- When the operation ends with no errors and no skips, the window closes and both panels re-read. The cursor in the source panel stays where it was; copied items are unselected in the source panel; the target panel puts its cursor on the first copied item **[unverified]**.

---

## 6. Queue and Background Transfer Manager (BTM)

### 6.1 Starting

- **F2 in the copy/move dialog** (or the "F2 Queue" button) adds the job to the BTM and closes the dialog at once. The main window stays usable [F-468, F-469].
- **Background button in the progress window** moves a running foreground job into the BTM.
- **AlwaysCopyInBackground=2** in wincmd.ini makes every F5/F6 OK go to the BTM.
- cm_OpenTransferManager opens the BTM window even when empty.

### 6.2 Queue semantics

- Jobs added with F2 run **one after another** in the order they were added. A newly queued job does not start until the one before it finishes. This is the whole point of the queue: copying two big folders to the same USB stick serially instead of in parallel.
- Foreground jobs started with Enter run at the same time as the queue, not inside it.
- TC can run more than one BTM instance. Each instance processes its own list serially. **[unverified: TC lets the user start a second, parallel queue from the BTM window; details unclear.]** mormiCommander: one BTM window with a "Run in parallel" toggle per job is acceptable.
- Overwrite prompts and error prompts from background jobs appear as normal dialogs, owned by the BTM window. The job waits until answered; later queued jobs also wait.
- Closing the main window while jobs are running asks `Background operations are still running. Abort them?` **[unverified wording]** with Yes/No.

### 6.3 BTM window layout

A small non-modal window, always on top of the main window **[unverified]** [F-470]:

1. **List** of jobs. Each row: operation (Copy/Move/Delete), source → target, state (Running / Waiting / Paused / Done / Error), progress percent, speed.
2. Progress bars for the selected (or running) job: current file, total.
3. Buttons: **Pause** / **Continue**, **Remove** (removes a waiting job; for a running job asks to abort), **Up** / **Down** to reorder waiting jobs **[unverified]**, **Close** (hides the window, jobs keep running).
4. **Speed limit**: a check box `Speed limit` and an edit field with the limit in **kilobytes per second** (bytes, not bits; 1 Mbit/s ≈ 125 KB/s). Turning it on applies to the running job immediately [F-470].

### 6.4 Speed limit details

- The limit applies to the whole BTM queue, measured over about one second windows, so it only has a visible effect on large files or slow links.
- TC had a bug (fixed in 9.20) where the limit stopped working after the first file of a queue. mormiCommander must apply the limit to every file of every job.
- wincmd.ini: TC stores the last limit and whether it is on so the BTM starts with it **[unverified key names, reported on the forum as SpeedLimitEnabled]**.

---

## 7. Overwrite dialog

Appears when a target file already exists and the overwrite mode is "Ask user" [F-463, F-464, F-465, F-466, F-467].

### 7.1 Layout

Title: `Confirm file replace` **[unverified; older builds say "Total Commander"]**.

Two blocks, each with an icon (or thumbnail for images, when previews are on), the full path, size in bytes with thousands separators, and the modification date/time:

```
Overwrite target file:
  D:\Backup\report.pdf
  1,234,567 bytes   2024-03-01 10:12:44
With source file:
  C:\Work\report.pdf
  1,240,001 bytes   2024-03-05 09:00:02
```

The newer of the two dates is highlighted **[unverified]**.

### 7.2 Buttons (TC 9+ layout)

Row 1: **Overwrite**, **Overwrite all**, **Skip**, **Skip all**, **Cancel**
Row 2: **Overwrite all older**, **Rename**, **Compare**, **More options ▾**

"More options" opens a menu: **Auto-rename copied file(s)**, **Auto-rename target file(s)**, **Copy all larger**, **Copy all smaller**, **Overwrite all older or same date** **[unverified exact list]**.

Default button: **Overwrite** **[unverified; may be Skip when the target is newer]**. Esc = Cancel (stops the whole job).

| Button | Effect |
|---|---|
| Overwrite | Replace this one file |
| Overwrite all | Replace this and every later clash in this job without asking |
| Skip | Leave this target as it is; for a move, the source stays too |
| Skip all | Skip this and every later clash |
| Overwrite all older | Replace when the target is older than the source, skip otherwise, for the rest of the job |
| Rename | Asks for a new name for the **copied** file (an edit box prefilled with the name); the target stays [F-465] |
| Compare | Opens Compare by content on the two files; the dialog stays open [F-466] |
| Auto-rename copied | The copied file becomes `name(2).ext`, `name(3).ext`... for all clashes [F-464] |
| Auto-rename target | The existing target is renamed to `name(2).ext` first, then the copy is written |
| Copy all larger / smaller | Replace only when the source is larger / smaller |
| Cancel | Stop the job; files already copied stay |

### 7.3 Read-only, hidden and system targets

If the target is read-only (or hidden/system on Windows), a second prompt follows: `The file X is write protected! Do you really want to overwrite it?` **[unverified wording]** with Yes / All / Skip / Cancel [F-467]. The OverwriteReadonly and OverwriteHidSys keys control whether this prompt appears.

### 7.4 Folder onto file and file onto folder

If the target name exists as a folder but the source is a file (or the other way round), TC shows an error `Cannot create file X: a directory with this name exists` **[unverified wording]** with Skip / Cancel. No overwrite is possible.

---

## 8. Delete (F8)

### 8.1 Confirmation dialog

Title: `Total Commander`. Text **[unverified exact wording]**:

- one file to the trash: `Do you really want to delete "name.ext"?` with the note `(to Recycle Bin)`. mormiCommander: `Move "name.ext" to the Trash?`
- several: `Do you really want to delete the 5 selected files/directories?`
- permanent (Shift+F8, or trash off): the same text plus a clear note that the files will **not** go to the Recycle Bin.
- volume without trash (network share, some USB sticks): the text says files will be deleted permanently [F-833].

Buttons: **Yes** (default) / **No**. Esc = No. Turning off "Confirm delete" in Configuration skips this dialog. [F-493, F-494]

### 8.2 Per-item prompts

- **Non-empty folder**: `The directory "X" is not empty! Do you really want to delete it with all subdirectories?` Buttons: **Yes**, **All**, **Skip**, **Cancel** [F-495]. Controlled by Configuration "Confirm deletion of non-empty directories".
- **Read-only/hidden/system file**: `The file "X" is write protected/hidden/system! Delete it anyway?` Buttons: **Yes**, **All**, **Skip**, **Cancel** [F-496].
- **Cannot move to trash** (file too big for the Recycle Bin, path too long): TC asks whether to delete permanently instead **[unverified wording]**.
- **In use / access denied**: error dialog with **Retry**, **Skip**, **Skip all**, **Cancel**.

### 8.3 Progress

Small deletes show no window. Big deletes show a progress window with the current path, a bar, and Cancel; with the background option they run in the BTM [F-497]. Cancel stops after the current item; already deleted items are gone.

After deleting, the panel re-reads and the cursor goes to the item that followed the first deleted one (or the previous one at the end of the list).

### 8.4 Delete elsewhere

Inside an archive the packer removes the entries [F-498, F-617]. On FTP/SFTP the plugin deletes on the server [F-499]. Neither has a trash; the confirmation says "permanently".

---

## 9. Edge cases and error messages

| Case | TC behaviour | mormiCommander |
|---|---|---|
| Copy a folder into itself or a subfolder of itself | Refused before starting: `Cannot copy a directory into itself!` **[unverified wording]** | Same, message "Can't copy a folder into itself." [F-721] |
| Copy a file onto itself (same source and target path) | `Cannot copy a file to itself!` | Same, or offer auto-name |
| Not enough free space | With SizeCheck=1 TC warns before starting: `Not enough space on target drive! Continue anyway?` Yes/No **[unverified wording]** | Same [F-491] |
| File larger than 4 GB onto FAT32 | Error for that file: target file system cannot hold files ≥4 GB; Skip/Cancel | Same, plain English [F-718] |
| Disk full during copy | Error with Retry / Skip / Cancel; the partial file is deleted | "Can't copy: the disk is full." [F-718] |
| Read error on source | Error with Retry / Skip / Skip all / Cancel | Same; skipped files listed at the end [F-492] |
| Path longer than 260 chars on Windows | Works (TC uses `\\?\`) | Same [F-486] |
| Symlink loop in a tree | TC follows junctions only with an option; loops are detected | Never loop [F-722] |
| Name invalid on target (`:` onto FAT/SMB, trailing dots on Windows) | Error for that file with Skip / Cancel | Same; offer Rename |
| Source disappears before copy | "File not found" with Skip / Cancel | Same |
| Target is a file where a folder was typed | Treated as the overwrite case of a single file copy | Same |
| Drag and drop between panels | Opens the same copy dialog (left drag), move dialog (Shift+drag); confirmation can be turned off (MouseActions) | F-480, F-694 |
| Ctrl+V paste | No dialog; overwrite prompts still appear | F-237 |

Verify failure: when Verify is on and the re-read target differs, TC shows `Verification failed for file X!` **[unverified wording]** with Retry / Skip / Cancel; for a move the source is kept [F-472].

At the end of a job with skipped or failed files, TC leaves those files selected in the source panel, so the user can see what was not copied. mormiCommander keeps this and also shows a summary [F-492].

---

## 10. Verify after copy

- Turned on per job in the dialog, or by default on the Configuration Copy/Delete page [F-421, F-472].
- After each file is written, TC flushes it, drops it from the cache where the OS allows, reads it back and compares it with the source (TC compares content; recent versions compare checksums computed while copying). The progress window shows `Verifying` during the read-back.
- The read-back roughly doubles the time on fast disks; users complain about it on the forum. mormiCommander should hash the source while copying and hash only the target on read-back, so the source is read once.

---

## 11. Configuration page "Copy/Delete" and wincmd.ini keys

The Configuration dialog page Copy/Delete [F-421, F-436, F-719] contains, among others: confirm copy/move, confirm delete, confirm delete of non-empty folders, confirm overwrite, confirm overwrite/delete of read-only/hidden/system files, confirm drag and drop, use Recycle Bin, copy buffer size, big-file copy mode, verify by default, copy date/time of folders, copy NTFS permissions, background by default.

Keys in `[Configuration]` of wincmd.ini (mormiCommander's importer [F-446] should read them):

| Key | Default | Meaning |
|---|---|---|
| AlwaysCopyInBackground | 0 | 0 foreground; 1 background thread; 2 always use the BTM |
| CopyBlockSize | 32 | Copy buffer in KB (same physical disk) |
| CopyBlockSizeOther | 32 | Copy buffer in KB between different physical disks |
| CopyHugeBlockSize | (empty) | Buffer for big files that bypass the cache; only files above this size; min 10 MB |
| CopyHugeBlockSizeOther | (empty) | Same, different disks |
| CopyStreams | 0 | 1 copies NTFS alternate data streams |
| CopyComments | 6 | Bit flags for descript.ion handling; 8 copies comments with files |
| CopyDirTimeStamp | (empty) | 1 copies modification time of folders |
| SizeCheck | 1 | Check free space before copying |
| CountMarked | 1 | Count files in subfolders before copy/delete, for the total bar |
| ExplorerForCopy | 0 | 1 uses Explorer's copy for F5/F6 (not applicable to us) |
| UseTrash | 1 | Delete to the Recycle Bin |
| OverwriteFiles | (confirmation on) | Ask before overwriting files |
| OverwriteReadonly | (confirmation on) | Ask before overwriting/deleting read-only files |
| OverwriteHidSys | (confirmation on) | Ask before overwriting/deleting hidden/system files |
| deleteDirs | (confirmation on) | Ask before deleting non-empty folders |
| MouseActions | (confirmation on) | Ask when files are dropped |
| AppendWarning | 1 | Warn before appending files |
| XPMoveMethod | 1 on XP | 1 applies target folder permissions when moving |

Note: the wiki says 0 means "no prompt" for the Overwrite*/deleteDirs/MouseActions keys but TC's shipped defaults have these confirmations on. The importer should treat a missing key as "on" **[unverified]**.

History of copy targets is stored in a separate section, e.g. `[CopyHistory]` with numbered keys `0=`, `1=` **[unverified section name]**.

---

## 12. Mapping for the job engine (what mormiCommander must implement)

- One job type with kinds Copy, Move, Delete; options: target, rename mask, include/exclude filter, overwrite mode, verify, copy permissions, copy xattrs, links-as-links, foreground/queued, speed limit.
- Jobs report: phase (counting, copying, verifying, deleting), current source/target, bytes done/total for the file and the job, speed, time left.
- Prompts (overwrite, read-only, non-empty folder, errors) are events the job raises and waits on; the answer can carry "for all" scope.
- Queue: serial execution of queued jobs; foreground jobs run outside the queue; pause/resume/cancel/remove/reorder; one global speed limit.
- Cancel deletes the partial target file. Move deletes each source only after its copy (and verify) succeeded.

---

## Sources

- ghisler.ch wiki, wincmd.ini reference: https://www.ghisler.ch/wiki/index.php/Wincmd.ini
- Forum: speed limit in BTM is set in bytes, reached via F5 then F2: https://ghisler.ch/board/viewtopic.php?p=290737 and https://www.ghisler.ch/board/viewtopic.php?p=27366
- Forum: BTM limiter bug after the first file, fixed in 9.20b1: https://www.ghisler.ch/board/viewtopic.php?p=340692 and https://www.ghisler.ch/board/viewtopic.php?p=338908
- Forum: slow verify when copying with verification: https://www.ghisler.ch/board/viewtopic.php?p=433731
- Forum: overwrite prompt during copy vs. at end: https://ghisler.ch/board/viewtopic.php?p=351276
- Forum: suggestion "queue to background": https://ghisler.ch/board/viewtopic.php?p=439972
