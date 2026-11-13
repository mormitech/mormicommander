//go:build !darwin && !linux && !windows

// This file covers platforms without clipboard support here (the BSDs and others). Every call returns
// ErrUnsupported so the UI can show a plain message; the pure formatters in the package still work.

package clipfiles

type noBackend struct{}

func newSystemBackend() backend { return noBackend{} }

func (noBackend) writeFiles([]string, Op) error    { return ErrUnsupported }
func (noBackend) readFiles() ([]string, Op, error) { return nil, Copy, ErrUnsupported }
func (noBackend) writeText(string) error           { return ErrUnsupported }
func (noBackend) readText() (string, error)        { return "", ErrUnsupported }
func (noBackend) writeImage([]byte) error          { return ErrUnsupported }
func (noBackend) readImage() ([]byte, error)       { return nil, ErrUnsupported }
func (noBackend) content() (Content, error)        { return Content{}, ErrUnsupported }
func (noBackend) clear() error                     { return ErrUnsupported }
