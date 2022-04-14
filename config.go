package wazero

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"strings"

	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
)

// RuntimeConfig controls runtime behavior, with the default implementation as NewRuntimeConfig
type RuntimeConfig struct {
	enabledFeatures wasm.Features
	newEngine       func(wasm.Features) wasm.Engine
	ctx             context.Context
	memoryMaxPages  uint32
}

// engineLessConfig helps avoid copy/pasting the wrong defaults.
var engineLessConfig = &RuntimeConfig{
	enabledFeatures: wasm.Features20191205,
	ctx:             context.Background(),
	memoryMaxPages:  wasm.MemoryMaxPages,
}

// clone ensures all fields are coped even if nil.
func (c *RuntimeConfig) clone() *RuntimeConfig {
	return &RuntimeConfig{
		enabledFeatures: c.enabledFeatures,
		newEngine:       c.newEngine,
		ctx:             c.ctx,
		memoryMaxPages:  c.memoryMaxPages,
	}
}

// NewRuntimeConfigJIT compiles WebAssembly modules into runtime.GOARCH-specific assembly for optimal performance.
//
// Note: This panics at runtime the runtime.GOOS or runtime.GOARCH does not support JIT. Use NewRuntimeConfig to safely
// detect and fallback to NewRuntimeConfigInterpreter if needed.
func NewRuntimeConfigJIT() *RuntimeConfig {
	ret := engineLessConfig.clone()
	ret.newEngine = jit.NewEngine
	return ret
}

// NewRuntimeConfigInterpreter interprets WebAssembly modules instead of compiling them into assembly.
func NewRuntimeConfigInterpreter() *RuntimeConfig {
	ret := engineLessConfig.clone()
	ret.newEngine = interpreter.NewEngine
	return ret
}

// WithContext sets the default context used to initialize the module. Defaults to context.Background if nil.
//
// Notes:
// * If the Module defines a start function, this is used to invoke it.
// * This is the outer-most ancestor of api.Module Context() during api.Function invocations.
// * This is the default context of api.Function when callers pass nil.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-function%E2%91%A0
func (c *RuntimeConfig) WithContext(ctx context.Context) *RuntimeConfig {
	if ctx == nil {
		ctx = context.Background()
	}
	ret := c.clone()
	ret.ctx = ctx
	return ret
}

// WithMemoryMaxPages reduces the maximum number of pages a module can define from 65536 pages (4GiB) to a lower value.
//
// Notes:
// * If a module defines no memory max limit, Runtime.CompileModule sets max to this value.
// * If a module defines a memory max larger than this amount, it will fail to compile (Runtime.CompileModule).
// * Any "memory.grow" instruction that results in a larger value than this results in an error at runtime.
// * Zero is a valid value and results in a crash if any module uses memory.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#grow-mem
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memory-types%E2%91%A0
func (c *RuntimeConfig) WithMemoryMaxPages(memoryMaxPages uint32) *RuntimeConfig {
	ret := c.clone()
	ret.memoryMaxPages = memoryMaxPages
	return ret
}

// WithFinishedFeatures enables currently supported "finished" feature proposals. Use this to improve compatibility with
// tools that enable all features by default.
//
// Note: The features implied can vary and can lead to unpredictable behavior during updates.
// Note: This only includes "finished" features, but "finished" is not an official W3C term: it is possible that
// "finished" features do not make the next W3C recommended WebAssembly core specification.
// See https://github.com/WebAssembly/spec/tree/main/proposals
func (c *RuntimeConfig) WithFinishedFeatures() *RuntimeConfig {
	ret := c.clone()
	ret.enabledFeatures = wasm.FeaturesFinished
	return ret
}

// WithFeatureMutableGlobal allows globals to be mutable. This defaults to true as the feature was finished in
// WebAssembly 1.0 (20191205).
//
// When false, an api.Global can never be cast to an api.MutableGlobal, and any source that includes global vars
// will fail to parse.
func (c *RuntimeConfig) WithFeatureMutableGlobal(enabled bool) *RuntimeConfig {
	ret := c.clone()
	ret.enabledFeatures = ret.enabledFeatures.Set(wasm.FeatureMutableGlobal, enabled)
	return ret
}

// WithFeatureSignExtensionOps enables sign extension instructions ("sign-extension-ops"). This defaults to false as the
// feature was not finished in WebAssembly 1.0 (20191205).
//
// This has the following effects:
// * Adds instructions `i32.extend8_s`, `i32.extend16_s`, `i64.extend8_s`, `i64.extend16_s` and `i64.extend32_s`
//
// See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
func (c *RuntimeConfig) WithFeatureSignExtensionOps(enabled bool) *RuntimeConfig {
	ret := c.clone()
	ret.enabledFeatures = ret.enabledFeatures.Set(wasm.FeatureSignExtensionOps, enabled)
	return ret
}

// WithFeatureMultiValue enables multiple values ("multi-value"). This defaults to false as the feature was not finished
// in WebAssembly 1.0 (20191205).
//
// This has the following effects:
// * Function (`func`) types allow more than one result
// * Block types (`block`, `loop` and `if`) can be arbitrary function types
//
// See https://github.com/WebAssembly/spec/blob/main/proposals/multi-value/Overview.md
func (c *RuntimeConfig) WithFeatureMultiValue(enabled bool) *RuntimeConfig {
	ret := c.clone()
	ret.enabledFeatures = ret.enabledFeatures.Set(wasm.FeatureMultiValue, enabled)
	return ret
}

// CompiledCode is a WebAssembly 1.0 (20191205) module ready to be instantiated (Runtime.InstantiateModule) as an\
// api.Module.
//
// Note: In WebAssembly language, this is a decoded, validated, and possibly also compiled module. wazero avoids using
// the name "Module" for both before and after instantiation as the name conflation has caused confusion.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#semantic-phases%E2%91%A0
type CompiledCode struct {
	module *wasm.Module
	// cachedEngines maps wasm.Engine to []*wasm.Module which originate from this .module.
	// This is necessary to track which engine caches *Module where latter might be different
	// from .module via import replacement config (ModuleConfig.WithImport).
	cachedEngines map[wasm.Engine]map[*wasm.Module]struct{}
}

// Close releases all the allocated resources for this CompiledCode.
func (c *CompiledCode) Close() {
	for engine, modules := range c.cachedEngines {
		for module := range modules {
			engine.ReleaseCompilationCache(module)
		}
	}
}

func (c *CompiledCode) addCacheEntry(module *wasm.Module, engine wasm.Engine) {
	if c.cachedEngines == nil {
		c.cachedEngines = map[wasm.Engine]map[*wasm.Module]struct{}{}
	}

	cache, ok := c.cachedEngines[engine]
	if !ok {
		cache = map[*wasm.Module]struct{}{}
		c.cachedEngines[engine] = cache
	}

	cache[module] = struct{}{}
}

// ModuleConfig configures resources needed by functions that have low-level interactions with the host operating system.
// Using this, resources such as STDIN can be isolated (ex via StartWASICommandWithConfig), so that the same module can
// be safely instantiated multiple times.
//
// Note: While wazero supports Windows as a platform, host functions using ModuleConfig follow a UNIX dialect.
// See RATIONALE.md for design background and relationship to WebAssembly System Interfaces (WASI).
type ModuleConfig struct {
	name           string
	startFunctions []string
	stdin          io.Reader
	stdout         io.Writer
	stderr         io.Writer
	args           []string
	// environ is pair-indexed to retain order similar to os.Environ.
	environ []string
	// environKeys allow overwriting of existing values.
	environKeys map[string]int

	// preopenFD has the next FD number to use
	preopenFD uint32
	// preopens are keyed on file descriptor and only include the Path and FS fields.
	preopens map[uint32]*wasm.FileEntry
	// preopenPaths allow overwriting of existing paths.
	preopenPaths map[string]uint32
	// replacedImports holds the latest state of WithImport
	// Note: Key is NUL delimited as import module and name can both include any UTF-8 characters.
	replacedImports map[string][2]string
	// replacedImportModules holds the latest state of WithImportModule
	replacedImportModules map[string]string
}

func NewModuleConfig() *ModuleConfig {
	return &ModuleConfig{
		startFunctions: []string{"_start"},
		environKeys:    map[string]int{},
		preopenFD:      uint32(3), // after stdin/stdout/stderr
		preopens:       map[uint32]*wasm.FileEntry{},
		preopenPaths:   map[string]uint32{},
	}
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
func (c *ModuleConfig) WithName(name string) *ModuleConfig {
	c.name = name
	return c
}

// WithImport replaces a specific import module and name with a new one. This allows you to break up a monolithic
// module imports, such as "env". This can also help reduce cyclic dependencies.
//
// For example, if a module was compiled with one module owning all imports:
//	(import "js" "tbl" (table $tbl 4 funcref))
//	(import "js" "increment" (func $increment (result i32)))
//	(import "js" "decrement" (func $decrement (result i32)))
//	(import "js" "wasm_increment" (func $wasm_increment (result i32)))
//	(import "js" "wasm_decrement" (func $wasm_decrement (result i32)))
//
// Use this function to import "increment" and "decrement" from the module "go" and other imports from "wasm":
//	config.WithImportModule("js", "wasm")
//	config.WithImport("wasm", "increment", "go", "increment")
//	config.WithImport("wasm", "decrement", "go", "decrement")
//
// Upon instantiation, imports resolve as if they were compiled like so:
//	(import "wasm" "tbl" (table $tbl 4 funcref))
//	(import "go" "increment" (func $increment (result i32)))
//	(import "go" "decrement" (func $decrement (result i32)))
//	(import "wasm" "wasm_increment" (func $wasm_increment (result i32)))
//	(import "wasm" "wasm_decrement" (func $wasm_decrement (result i32)))
//
// Note: Any WithImport instructions happen in order, after any WithImportModule instructions.
func (c *ModuleConfig) WithImport(oldModule, oldName, newModule, newName string) *ModuleConfig {
	if c.replacedImports == nil {
		c.replacedImports = map[string][2]string{}
	}
	var builder strings.Builder
	builder.WriteString(oldModule)
	builder.WriteByte(0) // delimit with NUL as module and name can be any UTF-8 characters.
	builder.WriteString(oldName)
	c.replacedImports[builder.String()] = [2]string{newModule, newName}
	return c
}

// WithImportModule replaces every import with oldModule with newModule. This is helpful for modules who have
// transitioned to a stable status since the underlying wasm was compiled.
//
// For example, if a module was compiled like below, with an old module for WASI:
//	(import "wasi_unstable" "args_get" (func (param i32, i32) (result i32)))
//
// Use this function to update it to the current version:
//	config.WithImportModule("wasi_unstable", wasi.ModuleSnapshotPreview1)
//
// See WithImport for a comprehensive example.
// Note: Any WithImportModule instructions happen in order, before any WithImport instructions.
func (c *ModuleConfig) WithImportModule(oldModule, newModule string) *ModuleConfig {
	if c.replacedImportModules == nil {
		c.replacedImportModules = map[string]string{}
	}
	c.replacedImportModules[oldModule] = newModule
	return c
}

// WithStartFunctions configures the functions to call after the module is instantiated. Defaults to "_start".
//
// Note: If any function doesn't exist, it is skipped. However, all functions that do exist are called in order.
func (c *ModuleConfig) WithStartFunctions(startFunctions ...string) *ModuleConfig {
	c.startFunctions = startFunctions
	return c
}

// WithStdin configures where standard input (file descriptor 0) is read. Defaults to return io.EOF.
//
// This reader is most commonly used by the functions like "fd_read" in "wasi_snapshot_preview1" although it could be
// used by functions imported from other modules.
//
// Note: The caller is responsible to close any io.Reader they supply: It is not closed on api.Module Close.
// Note: This does not default to os.Stdin as that both violates sandboxing and prevents concurrent modules.
// See https://linux.die.net/man/3/stdin
func (c *ModuleConfig) WithStdin(stdin io.Reader) *ModuleConfig {
	c.stdin = stdin
	return c
}

// WithStdout configures where standard output (file descriptor 1) is written. Defaults to io.Discard.
//
// This writer is most commonly used by the functions like "fd_write" in "wasi_snapshot_preview1" although it could
// be used by functions imported from other modules.
//
// Note: The caller is responsible to close any io.Writer they supply: It is not closed on api.Module Close.
// Note: This does not default to os.Stdout as that both violates sandboxing and prevents concurrent modules.
// See https://linux.die.net/man/3/stdout
func (c *ModuleConfig) WithStdout(stdout io.Writer) *ModuleConfig {
	c.stdout = stdout
	return c
}

// WithStderr configures where standard error (file descriptor 2) is written. Defaults to io.Discard.
//
// This writer is most commonly used by the functions like "fd_write" in "wasi_snapshot_preview1" although it could
// be used by functions imported from other modules.
//
// Note: The caller is responsible to close any io.Writer they supply: It is not closed on api.Module Close.
// Note: This does not default to os.Stderr as that both violates sandboxing and prevents concurrent modules.
// See https://linux.die.net/man/3/stderr
func (c *ModuleConfig) WithStderr(stderr io.Writer) *ModuleConfig {
	c.stderr = stderr
	return c
}

// WithArgs assigns command-line arguments visible to an imported function that reads an arg vector (argv). Defaults to
// none.
//
// These values are commonly read by the functions like "args_get" in "wasi_snapshot_preview1" although they could be
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
func (c *ModuleConfig) WithArgs(args ...string) *ModuleConfig {
	c.args = args
	return c
}

// WithEnv sets an environment variable visible to a Module that imports functions. Defaults to none.
//
// Validation is the same as os.Setenv on Linux and replaces any existing value. Unlike exec.Cmd Env, this does not
// default to the current process environment as that would violate sandboxing. This also does not preserve order.
//
// Environment variables are commonly read by the functions like "environ_get" in "wasi_snapshot_preview1" although
// they could be read by functions imported from other modules.
//
// While similar to process configuration, there are no assumptions that can be made about anything OS-specific. For
// example, neither WebAssembly nor WebAssembly System Interfaces (WASI) define concerns processes have, such as
// case-sensitivity on environment keys. For portability, define entries with case-insensitively unique keys.
//
// Note: Runtime.InstantiateModule errs if the key is empty or contains a NULL(0) or equals("") character.
// See https://linux.die.net/man/3/environ
// See https://en.wikipedia.org/wiki/Null-terminated_string
func (c *ModuleConfig) WithEnv(key, value string) *ModuleConfig {
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
//	config := wazero.NewModuleConfig().WithFS(rooted)
//
// Note: This sets WithWorkDirFS to the same file-system unless already set.
func (c *ModuleConfig) WithFS(fs fs.FS) *ModuleConfig {
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
//	config := wazero.NewModuleConfig().WithFS(rootFS).WithWorkDirFS(os.DirFS("/work/appA"))
//
// Note: os.DirFS documentation includes important notes about isolation, which also applies to fs.Sub. As of Go 1.18,
// the built-in file-systems are not jailed (chroot). See https://github.com/golang/go/issues/42322
func (c *ModuleConfig) WithWorkDirFS(fs fs.FS) *ModuleConfig {
	c.setFS(".", fs)
	return c
}

// setFS maps a path to a file-system. This is only used for base paths: "/" and ".".
func (c *ModuleConfig) setFS(path string, fs fs.FS) {
	// Check to see if this key already exists and update it.
	entry := &wasm.FileEntry{Path: path, FS: fs}
	if fd, ok := c.preopenPaths[path]; ok {
		c.preopens[fd] = entry
	} else {
		c.preopens[c.preopenFD] = entry
		c.preopenPaths[path] = c.preopenFD
		c.preopenFD++
	}
}

// toSysContext creates a baseline wasm.SysContext configured by ModuleConfig.
func (c *ModuleConfig) toSysContext() (sys *wasm.SysContext, err error) {
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
		preopens[c.preopenFD] = &wasm.FileEntry{Path: ".", FS: preopens[rootFD].FS}
	}

	return wasm.NewSysContext(math.MaxUint32, c.args, environ, c.stdin, c.stdout, c.stderr, preopens)
}

func (c *ModuleConfig) replaceImports(module *wasm.Module) *wasm.Module {
	if (c.replacedImportModules == nil && c.replacedImports == nil) || module.ImportSection == nil {
		return module
	}

	changed := false

	ret := *module // shallow copy
	replacedImports := make([]*wasm.Import, len(module.ImportSection))
	copy(replacedImports, module.ImportSection)

	// First, replace any import.Module
	for oldModule, newModule := range c.replacedImportModules {
		for i, imp := range replacedImports {
			if imp.Module == oldModule {
				changed = true
				cp := *imp // shallow copy
				cp.Module = newModule
				replacedImports[i] = &cp
			} else {
				replacedImports[i] = imp
			}
		}
	}

	// Now, replace any import.Module+import.Name
	for oldImport, newImport := range c.replacedImports {
		for i, imp := range replacedImports {
			nulIdx := strings.IndexByte(oldImport, 0)
			oldModule := oldImport[0:nulIdx]
			oldName := oldImport[nulIdx+1:]
			if imp.Module == oldModule && imp.Name == oldName {
				changed = true
				cp := *imp // shallow copy
				cp.Module = newImport[0]
				cp.Name = newImport[1]
				replacedImports[i] = &cp
			} else {
				replacedImports[i] = imp
			}
		}
	}

	if !changed {
		return module
	}
	ret.ImportSection = replacedImports
	return &ret
}
