// Package sysfs includes a low-level filesystem interface and utilities needed
// for WebAssembly host functions (ABI) such as WASI and runtime.GOOS=js.
//
// The name sysfs was chosen because wazero's public API has a "sys" package,
// which was named after https://github.com/golang/sys.
//
// This tracked in https://github.com/tetratelabs/wazero/issues/1013
package sysfs

import (
	"io/fs"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
)

// Adapt adapts the input to sys.FS unless it is already one. Use NewDirFS
// instead of os.DirFS as it handles interop issues such as windows support.
//
// Note: This performs no flag verification on OpenFile. fs.FS cannot read
// flags as there is no parameter to pass them through with. Moreover, fs.FS
// documentation does not require the file to be present. In summary, we can't
// enforce flag behavior.
func Adapt(fs fs.FS) experimentalsys.FS {
	return sysfs.Adapt(fs)
}

// NewReadFS is used to mask an existing sys.FS for reads. Notably, this allows
// the CLI to do read-only mounts of directories the host user can write, but
// doesn't want the guest wasm to. For example, Python libraries shouldn't be
// written to at runtime by the python wasm file.
func NewReadFS(fs experimentalsys.FS) experimentalsys.FS {
	return sysfs.NewReadFS(fs)
}

// NewDirFS is like os.DirFS except it returns sys.FS, which has more features.
func NewDirFS(dir string) experimentalsys.FS {
	return sysfs.NewDirFS(dir)
}
