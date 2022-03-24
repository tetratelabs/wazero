package wazero

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
)

// NewRuntimeConfigJIT compiles WebAssembly modules into runtime.GOARCH-specific assembly for optimal performance.
//
// Note: This panics at runtime the runtime.GOOS or runtime.GOARCH does not support JIT. Use NewRuntimeConfig to safely
// detect and fallback to NewRuntimeConfigInterpreter if needed.
func NewRuntimeConfigJIT() *RuntimeConfig {
	return &RuntimeConfig{
		engine:          jit.NewEngine(),
		ctx:             context.Background(),
		enabledFeatures: internalwasm.Features20191205,
	}
}

// NewRuntimeConfigInterpreter interprets WebAssembly modules instead of compiling them into assembly.
func NewRuntimeConfigInterpreter() *RuntimeConfig {
	return &RuntimeConfig{
		engine:          interpreter.NewEngine(),
		ctx:             context.Background(),
		enabledFeatures: internalwasm.Features20191205,
	}
}

// RuntimeConfig controls runtime behavior, with the default implementation as NewRuntimeConfig
type RuntimeConfig struct {
	engine          internalwasm.Engine
	ctx             context.Context
	enabledFeatures internalwasm.Features
}

// WithContext sets the default context used to initialize the module. Defaults to context.Background if nil.
//
// Notes:
// * If the Module defines a start function, this is used to invoke it.
// * This is the outer-most ancestor of wasm.Module Context() during wasm.Function invocations.
// * This is the default context of wasm.Function when callers pass nil.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-function%E2%91%A0
func (r *RuntimeConfig) WithContext(ctx context.Context) *RuntimeConfig {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RuntimeConfig{engine: r.engine, ctx: ctx, enabledFeatures: r.enabledFeatures}
}

// WithFeatureMutableGlobal allows globals to be mutable. This defaults to true as the feature was finished in
// WebAssembly 1.0 (20191205).
//
// When false, a wasm.Global can never be cast to a wasm.MutableGlobal, and any source that includes global vars
// will fail to parse.
//
func (r *RuntimeConfig) WithFeatureMutableGlobal(enabled bool) *RuntimeConfig {
	enabledFeatures := r.enabledFeatures.Set(internalwasm.FeatureMutableGlobal, enabled)
	return &RuntimeConfig{engine: r.engine, ctx: r.ctx, enabledFeatures: enabledFeatures}
}

// WithFeatureSignExtensionOps enables sign-extend operations. This defaults to false as the feature was not finished in
// WebAssembly 1.0 (20191205).
//
// See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
func (r *RuntimeConfig) WithFeatureSignExtensionOps(enabled bool) *RuntimeConfig {
	enabledFeatures := r.enabledFeatures.Set(internalwasm.FeatureSignExtensionOps, enabled)
	return &RuntimeConfig{engine: r.engine, ctx: r.ctx, enabledFeatures: enabledFeatures}
}

// Module is a WebAssembly 1.0 (20191205) module to instantiate.
type Module struct {
	name   string
	module *internalwasm.Module
}

// WithName configures the module name. Defaults to what was decoded from the module source.
//
// If the source was in WebAssembly 1.0 (20191205) Binary Format, this defaults to what was decoded from the custom name
// section. Otherwise, if it was decoded from Text Format, this defaults to the module ID stripped of leading '$'.
//
// For example, if the Module was decoded from the text format `(module $math)`, the default name is "math".
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#custom-section%E2%91%A0
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#modules%E2%91%A0%E2%91%A2
func (m *Module) WithName(name string) *Module {
	m.name = name
	return m
}

// SysConfig configures resources needed by functions that have low-level interactions with the host operating system.
// Using this, resources such as STDIN can be isolated (ex via StartWASICommandWithConfig), so that the same module can
// be safely instantiated multiple times.
//
// Note: While wazero supports Windows as a platform, host functions using SysConfig follow a UNIX dialect.
// See RATIONALE.md for design background and relationship to WebAssembly System Interfaces (WASI).
type SysConfig struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
	// environ is pair-indexed to retain order similar to os.Environ.
	environ []string
	// environKeys allow overwriting of existing values.
	environKeys map[string]int

	// preopenFD has the next FD number to use
	preopenFD uint32
	// preopens are keyed on file descriptor and only include the Path and FS fields.
	preopens map[uint32]*internalwasm.FileEntry
	// preopenPaths allow overwriting of existing paths.
	preopenPaths map[string]uint32
}

func NewSysConfig() *SysConfig {
	return &SysConfig{
		environKeys:  map[string]int{},
		preopenFD:    uint32(3), // after stdin/stdout/stderr
		preopens:     map[uint32]*internalwasm.FileEntry{},
		preopenPaths: map[string]uint32{},
	}
}

// WithStdin configures where standard input (file descriptor 0) is read. Defaults to return io.EOF.
//
// This reader is most commonly used by the functions like "fd_read" in wasi.ModuleSnapshotPreview1 although it could be
// used by functions imported from other modules.
//
// Note: The caller is responsible to close any io.Reader they supply: It is not closed on wasm.Module Close.
// Note: This does not default to os.Stdin as that both violates sandboxing and prevents concurrent modules.
// See https://linux.die.net/man/3/stdin
func (c *SysConfig) WithStdin(stdin io.Reader) *SysConfig {
	c.stdin = stdin
	return c
}

// WithStdout configures where standard output (file descriptor 1) is written. Defaults to io.Discard.
//
// This writer is most commonly used by the functions like "fd_write" in wasi.ModuleSnapshotPreview1 although it could
// be used by functions imported from other modules.
//
// Note: The caller is responsible to close any io.Writer they supply: It is not closed on wasm.Module Close.
// Note: This does not default to os.Stdout as that both violates sandboxing and prevents concurrent modules.
// See https://linux.die.net/man/3/stdout
func (c *SysConfig) WithStdout(stdout io.Writer) *SysConfig {
	c.stdout = stdout
	return c
}

// WithStderr configures where standard error (file descriptor 2) is written. Defaults to io.Discard.
//
// This writer is most commonly used by the functions like "fd_write" in wasi.ModuleSnapshotPreview1 although it could
// be used by functions imported from other modules.
//
// Note: The caller is responsible to close any io.Writer they supply: It is not closed on wasm.Module Close.
// Note: This does not default to os.Stderr as that both violates sandboxing and prevents concurrent modules.
// See https://linux.die.net/man/3/stderr
func (c *SysConfig) WithStderr(stderr io.Writer) *SysConfig {
	c.stderr = stderr
	return c
}

// WithArgs assigns command-line arguments visible to an imported function that reads an arg vector (argv). Defaults to
// none.
//
// These values are commonly read by the functions like "args_get" in wasi.ModuleSnapshotPreview1 although they could be
// read by functions imported from other modules.
//
// Similar to os.Args and exec.Cmd Env, many implementations would expect a program name to be argv[0]. However, neither
// WebAssembly nor WebAssembly System Interfaces (WASI) define this. Regardless, you may choose to set the first
// argument to the same value set via WithName.
//
// Note: This does not default to os.Args as that violates sandboxing.
// Note: Runtime.InstantiateModule errs if any value is empty.
// See https://linux.die.net/man/3/argv
// See https://en.wikipedia.org/wiki/Null-terminated_string
func (c *SysConfig) WithArgs(args ...string) *SysConfig {
	c.args = args
	return c
}

// WithEnv sets an environment variable visible to a Module that imports functions. Defaults to none.
//
// Validation is the same as os.Setenv on Linux and replaces any existing value. Unlike exec.Cmd Env, this does not
// default to the current process environment as that would violate sandboxing. This also does not preserve order.
//
// Environment variables are commonly read by the functions like "environ_get" in wasi.ModuleSnapshotPreview1 although
// they could be read by functions imported from other modules.
//
// While similar to process configuration, there are no assumptions that can be made about anything OS-specific. For
// example, neither WebAssembly nor WebAssembly System Interfaces (WASI) define concerns processes have, such as
// case-sensitivity on environment keys. For portability, define entries with case-insensitively unique keys.
//
// Note: Runtime.InstantiateModule errs if the key is empty or contains a NULL(0) or equals("") character.
// See https://linux.die.net/man/3/environ
// See https://en.wikipedia.org/wiki/Null-terminated_string
func (c *SysConfig) WithEnv(key, value string) *SysConfig {
	// Check to see if this key already exists and update it.
	if i, ok := c.environKeys[key]; ok {
		c.environ[i+1] = value // environ is pair-indexed, so the value is 1 after the key.
	} else {
		c.environKeys[key] = len(c.environ)
		c.environ = append(c.environ, key, value)
	}
	return c
}

// WithFS assigns the file system to use for any paths beginning at "/". Defaults to not found.
//
// Ex. This sets a read-only, embedded file-system to serve files under the root ("/") and working (".") directories:
//
//	//go:embed testdata/index.html
//	var testdataIndex embed.FS
//
//	rooted, err := fs.Sub(testdataIndex, "testdata")
//	require.NoError(t, err)
//
//	// "index.html" is accessible as both "/index.html" and "./index.html" because we didn't use WithWorkDirFS.
//	sysConfig := wazero.NewSysConfig().WithFS(rooted)
//
// Note: This sets WithWorkDirFS to the same file-system unless already set.
func (c *SysConfig) WithFS(fs fs.FS) *SysConfig {
	c.setFS("/", fs)
	return c
}

// WithWorkDirFS indicates the file system to use for any paths beginning at "./". Defaults to the same as WithFS.
//
// Ex. This sets a read-only, embedded file-system as the root ("/"), and a mutable one as the working directory ("."):
//
//	//go:embed appA
//	var rootFS embed.FS
//
//	// Files relative to this source under appA are available under "/" and files relative to "/work/appA" under ".".
//	sysConfig := wazero.NewSysConfig().WithFS(rootFS).WithWorkDirFS(os.DirFS("/work/appA"))
//
// Note: os.DirFS documentation includes important notes about isolation, which also applies to fs.Sub. As of Go 1.18,
// the built-in file-systems are not jailed (chroot). See https://github.com/golang/go/issues/42322
func (c *SysConfig) WithWorkDirFS(fs fs.FS) *SysConfig {
	c.setFS(".", fs)
	return c
}

// setFS maps a path to a file-system. This is only used for base paths: "/" and ".".
func (c *SysConfig) setFS(path string, fs fs.FS) {
	// Check to see if this key already exists and update it.
	entry := &internalwasm.FileEntry{Path: path, FS: fs}
	if fd, ok := c.preopenPaths[path]; ok {
		c.preopens[fd] = entry
	} else {
		c.preopens[c.preopenFD] = entry
		c.preopenPaths[path] = c.preopenFD
		c.preopenFD++
	}
}

// toSysContext creates a baseline internalwasm.SysContext configured by SysConfig.
func (c *SysConfig) toSysContext() (sys *internalwasm.SysContext, err error) {
	var environ []string // Intentionally doesn't pre-allocate to reduce logic to default to nil.
	// Same validation as syscall.Setenv for Linux
	for i := 0; i < len(c.environ); i += 2 {
		key, value := c.environ[i], c.environ[i+1]
		if len(key) == 0 {
			err = errors.New("environ invalid: empty key")
			return
		}
		for j := 0; j < len(key); j++ {
			if key[j] == '=' { // NUL enforced in NewSysContext
				err = errors.New("environ invalid: key contains '=' character")
				return
			}
		}
		environ = append(environ, key+"="+value)
	}

	// Ensure no-one set a nil FD. We do this here instead of at the call site to allow chaining as nil is unexpected.
	rootFD := uint32(0) // zero is invalid
	setWorkDirFS := false
	preopens := c.preopens
	for fd, entry := range preopens {
		if entry.FS == nil {
			err = fmt.Errorf("FS for %s is nil", entry.Path)
			return
		} else if entry.Path == "/" {
			rootFD = fd
		} else if entry.Path == "." {
			setWorkDirFS = true
		}
	}

	// Default the working directory to the root FS if it exists.
	if rootFD != 0 && !setWorkDirFS {
		preopens[c.preopenFD] = &internalwasm.FileEntry{Path: ".", FS: preopens[rootFD].FS}
	}

	return internalwasm.NewSysContext(math.MaxUint32, c.args, environ, c.stdin, c.stdout, c.stderr, preopens)
}
