//go:build windows

package ptyterm

// Windows needs ConPTY (CreatePseudoConsole and a process started with its attribute list), which exec.Cmd cannot do;
// it is left for later, so Start says the terminal is not available and the page tells the user.

// Session is not available on Windows yet.
type Session struct{ Shell string }

// Start reports ErrUnsupported on Windows.
func Start(Options) (*Session, error) { return nil, ErrUnsupported }

func (s *Session) Read([]byte) (int, error)  { return 0, ErrUnsupported }
func (s *Session) Write([]byte) (int, error) { return 0, ErrUnsupported }
func (s *Session) Resize(int, int) error     { return ErrUnsupported }
func (s *Session) Busy() bool                { return true }
func (s *Session) Cd(string) error           { return ErrUnsupported }
func (s *Session) Insert([]string) error     { return ErrUnsupported }
func (s *Session) Done() <-chan struct{}     { c := make(chan struct{}); close(c); return c }
func (s *Session) ExitCode() int             { return -1 }
func (s *Session) Close() error              { return nil }
