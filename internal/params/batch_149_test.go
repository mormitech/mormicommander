package params

import "testing"

// A batch file is read by cmd.exe, where an unquoted "a&b.txt" ends the command and starts "b.txt". For .bat and .cmd
// programs names with cmd's operators are quoted; other programs keep Total Commander's rule (quote for blanks only).
func TestBatchButtonQuotesCmdOperators(t *testing.T) {
	ctx := winCtx(t)
	ctx.Source = Panel{Path: `C:\d`, Cursor: Entry{Name: "a&b.txt"}}
	l, err := PrepareButton(Button{Command: `C:\tools\run.BAT`, Params: "%N"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if l.Line != `"a&b.txt"` {
		t.Errorf("batch file: line %s, want the name quoted", l.Line)
	}
	l, err = PrepareButton(Button{Command: `C:\tools\run.exe`, Params: "%N"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if l.Line != `a&b.txt` {
		t.Errorf("program: line %s, want it as Total Commander passes it", l.Line)
	}
}
