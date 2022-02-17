package wasi

// TODO: rename these according to other naming conventions
const (
	// WASI open flags
	O_CREATE = 1 << iota
	O_DIR
	O_EXCL
	O_TRUNC

	// WASI fs rights
	R_FD_READ = 1 << iota
	R_FD_SEEK
	R_FD_FDSTAT_SET_FLAGS
	R_FD_SYNC
	R_FD_TELL
	R_FD_WRITE
)

type File interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
}

// FS is an interface for a preopened directory.
type FS interface {
	// OpenWASI is a general method to open a file, similar to
	// os.OpenFile, but with WASI flags and rights instead of POSIX.
	OpenWASI(dirFlags uint32, path string, oFlags uint32, fsRights, fsRightsInheriting uint64, fdFlags uint32) (File, error)
}
