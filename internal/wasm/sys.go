package internalwasm

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/tetratelabs/wazero/internal/cstring"
	"github.com/tetratelabs/wazero/wasi"
)

// FileEntry temporarily uses wasi types until #394.
//
// Note: This does not introduce cycles because the types here are in the package "wasi" not "internalwasi".
type FileEntry struct {
	Path string
	FS   wasi.FS
	File wasi.File
}

// SystemContext holds module-scoped system resources currently only used by internalwasi.
//
// TODO: Most fields are mutable so that WASI config can overwrite fields when starting a command module. This can be
// fixed in #394 by replacing wazero.WASIConfig with wazero.SystemConfig, defaulted by wazero.RuntimeConfig and
// overridable with InstantiateModule.
type SystemContext struct {
	// WithArgs hold a possibly empty (cstring.EmptyNullTerminatedStrings) list of arguments similar to os.Args.
	//
	// TODO: document this better in #396
	Args *cstring.NullTerminatedStrings

	// WithArgs hold a possibly empty (cstring.EmptyNullTerminatedStrings) list of arguments key/value pairs, similar to
	// os.Environ.
	//
	// TODO: document this better in #396
	Environ *cstring.NullTerminatedStrings

	// WithStdin defaults to os.Stdin.
	//
	// TODO: change default to read os.DevNull in #396
	Stdin io.Reader

	// WithStdout defaults to os.Stdout.
	//
	// TODO: change default to io.Discard in #396
	Stdout io.Writer

	// WithStderr defaults to os.Stderr.
	//
	// TODO: change default to io.Discard in #396
	Stderr io.Writer

	// OpenedFiles are a map of file descriptor numbers (starting at 3) to open files (or directories).
	// TODO: This is unguarded, so not goroutine-safe!
	OpenedFiles map[uint32]*FileEntry
}

func NewSystemContext() (*SystemContext, error) {
	return &SystemContext{
		Args:        cstring.EmptyNullTerminatedStrings,
		Environ:     cstring.EmptyNullTerminatedStrings,
		Stdin:       os.Stdin, // TODO: stop this default #396
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		OpenedFiles: map[uint32]*FileEntry{},
	}, nil // TODO: once we open stdin from os.DevNull, this may raise an error (albeit unlikely).
}

// WithStdin the same as wazero.WASIConfig WithStdin
func (c *SystemContext) WithStdin(reader io.Reader) {
	c.Stdin = reader
}

// WithStdout the same as wazero.WASIConfig WithStdout
func (c *SystemContext) WithStdout(writer io.Writer) {
	c.Stdout = writer
}

// WithStderr the same as wazero.WASIConfig WithStderr
func (c *SystemContext) WithStderr(writer io.Writer) {
	c.Stderr = writer
}

// WithArgs is the same as wazero.WASIConfig WithArgs
func (c *SystemContext) WithArgs(args ...string) error {
	wasiStrings, err := cstring.NewNullTerminatedStrings(math.MaxUint32, "arg", args...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return err
	}
	c.Args = wasiStrings
	return nil
}

// WithEnviron is the same as wazero.WASIConfig WithEnviron
func (c *SystemContext) WithEnviron(environ ...string) error {
	for i, env := range environ {
		if !strings.Contains(env, "=") {
			return fmt.Errorf("environ[%d] is not joined with '='", i)
		}
	}
	wasiStrings, err := cstring.NewNullTerminatedStrings(math.MaxUint32, "environ", environ...) // TODO: this is crazy high even if spec allows it
	if err != nil {
		return err
	}
	c.Environ = wasiStrings
	return nil
}

// WithPreopen adds one element in  wazero.WASIConfig WithPreopens
func (c *SystemContext) WithPreopen(dir string, fileSys wasi.FS) {
	c.OpenedFiles[uint32(len(c.OpenedFiles))+3] = &FileEntry{Path: dir, FS: fileSys}
}

// Close implements io.Closer
func (c *SystemContext) Close() (err error) {
	// Note: WithStdin, WithStdout and WithStderr are not closed as we didn't open them.
	// TODO: In #394, close open files
	return
}
