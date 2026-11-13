package trash

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

const supported = true

const script = `Add-Type -AssemblyName Microsoft.VisualBasic
$p = $env:MORMICOMMANDER_PATH
if (Test-Path -PathType Container -LiteralPath $p) {
  [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
} else {
  [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile($p, 'OnlyErrorDialogs', 'SendToRecycleBin')
}`

// Move sends path to the Recycle Bin. The path travels in an environment variable, never inside the script text.
func Move(path string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "MORMICOMMANDER_PATH="+path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %s", path, out)
	}
	return nil
}
