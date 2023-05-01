package platform

import (
	"io/fs"
	"syscall"
)

var _ File = NoopFile{}

// NoopFile shows the minimal methods a type embedding UnimplementedFile must
// implement.
type NoopFile struct {
	UnimplementedFile
}

// The current design requires the user to consciously implement Close.
// However, we could change UnimplementedFile to return zero.
func (n NoopFile) Close() (errno syscall.Errno) { return }

// Once File.File is removed, it will be possible to implement NoopFile.
func (n NoopFile) File() fs.File { panic("noop") }
