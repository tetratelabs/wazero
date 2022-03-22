package wazero

import (
	"context"
	"fmt"
	"io"
	"math"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
	"github.com/tetratelabs/wazero/wasi"
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
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
	args     []string
	environ  map[string]string
	preopens map[string]wasi.FS
}

func NewSysConfig() *SysConfig {
	return &SysConfig{}
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
	c.environ[key] = value
	return c
}

// WithPreopens is intentionally undocumented as it is being removed in #394
func (c *SysConfig) WithPreopens(preopens map[string]wasi.FS) *SysConfig {
	c.preopens = preopens
	return c
}

// buildSysContext creates a baseline internalwasm.SysContext configured by SysConfig.
func buildSysContext(c *SysConfig) (sys *internalwasm.SysContext, err error) {
	environ := make([]string, 0, len(c.environ))
	// Same validation as syscall.Setenv for Linux
	for key, value := range c.environ {
		if len(key) == 0 {
			err = fmt.Errorf("empty environ key")
			return
		}
		for i := 0; i < len(key); i++ {
			if key[i] == '=' || key[i] == 0 {
				err = fmt.Errorf("environ key contained a NUL or '=' character: %s", key)
				return
			}
		}
		environ = append(environ, key+"="+value)
	}

	openedFiles := map[uint32]*internalwasm.FileEntry{}
	i := uint32(3) // after stdin/stdout/stderr
	for dir, fs := range c.preopens {
		openedFiles[i] = &internalwasm.FileEntry{Path: dir, FS: fs}
		i++
	}

	return internalwasm.NewSystemContext(math.MaxUint32, c.args, environ, c.stdin, c.stdout, c.stderr, openedFiles)
}
