package fsapi

import (
	"fmt"
	"io/fs"

	"github.com/tetratelabs/wazero/experimental/sys"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

func Adapt(f experimentalsys.File) File {
	if f, ok := f.(File); ok {
		return f
	}
	fmt.Printf("unimplmented %T\n", f)
	return unimplementedFile{f}
}

type unimplementedFile struct{ experimentalsys.File }

// IsNonblock implements File.IsNonblock
func (unimplementedFile) IsNonblock() bool {
	return false
}

// SetNonblock implements File.SetNonblock
func (unimplementedFile) SetNonblock(bool) experimentalsys.Errno {
	return experimentalsys.ENOSYS
}

// Poll implements File.Poll
func (unimplementedFile) Poll(Pflag, int32) (ready bool, errno experimentalsys.Errno) {
	return false, experimentalsys.ENOSYS
}

// OpenAt implements File.OpenAt
func (unimplementedFile) OpenAt(
	fs experimentalsys.FS,
	path string,
	flag experimentalsys.Oflag,
	mode fs.FileMode,
) (sys.File, experimentalsys.Errno) {
	return nil, experimentalsys.ENOSYS
}
