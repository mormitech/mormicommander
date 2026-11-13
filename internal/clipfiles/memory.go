package clipfiles

// A clipboard held in memory, for tests of the code that uses this package: it behaves like a system clipboard that
// holds one thing at a time (files with their copy or cut mark, text, or an image) and never touches the real one.

// memoryBackend is the in-memory clipboard behind NewMemory.
type memoryBackend struct {
	paths []string
	op    Op
	text  *string
	png   []byte
}

// NewMemory returns a clipboard held in memory, empty at first.
func NewMemory() *Clipboard { return &Clipboard{b: &memoryBackend{}} }

func (m *memoryBackend) writeFiles(p []string, op Op) error {
	*m = memoryBackend{paths: append([]string(nil), p...), op: op}
	return nil
}

func (m *memoryBackend) readFiles() ([]string, Op, error) {
	if len(m.paths) == 0 {
		return nil, Copy, ErrEmpty
	}
	return append([]string(nil), m.paths...), m.op, nil
}

func (m *memoryBackend) writeText(s string) error { *m = memoryBackend{text: &s}; return nil }

func (m *memoryBackend) readText() (string, error) {
	if m.text == nil {
		return "", ErrEmpty
	}
	return *m.text, nil
}

func (m *memoryBackend) writeImage(b []byte) error { *m = memoryBackend{png: b}; return nil }

func (m *memoryBackend) readImage() ([]byte, error) {
	if m.png == nil {
		return nil, ErrEmpty
	}
	return m.png, nil
}

func (m *memoryBackend) content() (Content, error) {
	return Content{Files: len(m.paths) > 0, Text: m.text != nil, Image: m.png != nil}, nil
}

func (m *memoryBackend) clear() error { *m = memoryBackend{}; return nil }
