package trash

// On Windows items go to the Recycle Bin through the shell's own file operation, reached from PowerShell via
// Microsoft.VisualBasic's FileSystem class (it sets the "allow undo" flag, which is what makes a delete a recycle).
// The path travels in an environment variable, never inside the script text. The shell operation recycles a
// symbolic link or junction as the link itself. MoveAll calls PowerShell once per item for now.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
)

const supported = true

const directSupported = false

func moveDirect(ctx context.Context, path string) error { return moveOne(ctx, path) }

const script = `Add-Type -AssemblyName Microsoft.VisualBasic
$p = $env:MORMICOMMANDER_PATH
if (Test-Path -PathType Container -LiteralPath $p) {
  [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
} else {
  [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
}`

func moveOne(ctx context.Context, path string) error {
	cmd := command(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "MORMICOMMANDER_PATH="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := runCommand(cmd); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s: %s", path, strings.TrimSpace(string(out)))
	}
	return nil
}

func moveMany(ctx context.Context, paths []string) error { return moveEach(ctx, paths) }
