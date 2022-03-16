package wazero

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	internalwasi "github.com/tetratelabs/wazero/internal/wasi"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

// WASIDirFS returns a file system for the tree of files rooted at the directory dir.
// It's similar to os.DirFS, except that it also implements wasi.OpenFileFS in addition to fs.FS,
// which allows new file creation and writing to opened files.
func WASIDirFS(dir string) wasi.OpenFileFS {
	return internalwasi.DirFS(dir)
}

// WASIMemFS returns an in-memory file system that implements wasi.OpenFileFS.
func WASIMemFS() wasi.OpenFileFS {
	return &internalwasi.MemFS{}
}

type WASIConfig struct {
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
	args     []string
	environ  map[string]string
	preopens map[string]fs.FS
}

func NewWASIConfig() *WASIConfig {
	return &WASIConfig{}
}

func (c *WASIConfig) WithStdin(stdin io.Reader) *WASIConfig {
	c.stdin = stdin
	return c
}

func (c *WASIConfig) WithStdout(stdout io.Writer) *WASIConfig {
	c.stdout = stdout
	return c
}

func (c *WASIConfig) WithStderr(stderr io.Writer) *WASIConfig {
	c.stderr = stderr
	return c
}

func (c *WASIConfig) WithArgs(args ...string) *WASIConfig {
	c.args = args
	return c
}

func (c *WASIConfig) WithEnviron(environ map[string]string) *WASIConfig {
	c.environ = environ
	return c
}

func (c *WASIConfig) WithPreopens(preopens map[string]fs.FS) *WASIConfig {
	c.preopens = preopens
	return c
}

// WASISnapshotPreview1 are functions importable as the module name wasi.ModuleSnapshotPreview1
func WASISnapshotPreview1() *Module {
	return WASISnapshotPreview1WithConfig(&WASIConfig{})
}

// WASISnapshotPreview1WithConfig are functions importable as the module name wasi.ModuleSnapshotPreview1
func WASISnapshotPreview1WithConfig(c *WASIConfig) *Module {
	cfg := newConfig(c) // safe copy of config
	m, err := internalwasm.NewHostModule(wasi.ModuleSnapshotPreview1, internalwasi.SnapshotPreview1Functions(cfg))
	if err != nil {
		panic(fmt.Errorf("BUG: %w", err))
	}
	return &Module{name: wasi.ModuleSnapshotPreview1, module: m}
}

func newConfig(c *WASIConfig) *internalwasi.Config {
	cfg := internalwasi.NewConfig()
	if c.stdin != nil {
		cfg.Stdin(c.stdin)
	}
	if c.stdout != nil {
		cfg.Stdout(c.stdout)
	}
	if c.stderr != nil {
		cfg.Stderr(c.stderr)
	}
	if len(c.args) > 0 {
		err := cfg.Args(c.args...)
		if err != nil {
			panic(err) // better to panic vs have bother users about unlikely size > uint32
		}
	}
	if len(c.environ) > 0 {
		environ := make([]string, 0, len(c.environ))
		for k, v := range c.environ {
			environ = append(environ, fmt.Sprintf("%s=%s", k, v))
		}
		err := cfg.Environ(environ...)
		if err != nil { // this can't be due to lack of '=' as we did that above.
			panic(err) // better to panic vs have bother users about unlikely size > uint32
		}
	}
	if len(c.preopens) > 0 {
		for k, v := range c.preopens {
			cfg.Preopen(k, v)
		}
	}
	return cfg
}

// StartWASICommandFromSource instantiates a module from the WebAssembly 1.0 (20191205) text or binary source or errs if
// invalid. Once instantiated, this starts its WASI Command function ("_start").
//
// Ex.
//	r := wazero.NewRuntime()
//	wasi, _ := r.NewHostModule(wazero.WASISnapshotPreview1())
//	defer wasi.Close()
//
//	module, _ := StartWASICommandFromSource(r, source)
//	defer module.Close()
//
// Note: This is a convenience utility that chains Runtime.CompileModule with StartWASICommand.
// See StartWASICommandWithConfig
func StartWASICommandFromSource(r Runtime, source []byte) (wasm.Module, error) {
	if decoded, err := r.CompileModule(source); err != nil {
		return nil, err
	} else {
		return StartWASICommand(r, decoded)
	}
}

// StartWASICommand instantiates the module and starts its WASI Command function ("_start"). The return value are all
// exported functions in the module. This errs if the module doesn't comply with prerequisites, or any instantiation
// or function call error.
//
// Ex.
//	r := wazero.NewRuntime()
//	wasi, _ := r.NewHostModule(wazero.WASISnapshotPreview1())
//	defer wasi.Close()
//
//	decoded, _ := r.CompileModule(source)
//	module, _ := StartWASICommand(r, decoded)
//	defer module.Close()
//
// Prerequisites of the "Current Unstable ABI" from wasi.ModuleSnapshotPreview1:
// * "_start" is an exported nullary function and does not export "_initialize"
// * "memory" is an exported memory.
//
// Note: "_start" is invoked in the RuntimeConfig.Context.
// Note: Exporting "__indirect_function_table" is mentioned as required, but not enforced here.
// Note: The wasm.Functions return value does not restrict exports after "_start" as allowed in the specification.
// Note: All TinyGo Wasm are WASI commands. They initialize memory on "_start" and import "fd_write" to implement panic.
// See StartWASICommandWithConfig
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
func StartWASICommand(r Runtime, module *Module) (wasm.Module, error) {
	return StartWASICommandWithConfig(r, module, nil)
}

// StartWASICommandWithConfig is like StartWASICommand, except you can override configuration based on the importing
// module. For example, you can use this to define different args depending on the importing module.
//
//	// Initialize base configuration:
//	r := wazero.NewRuntime()
//	config := wazero.NewWASIConfig().WithStdout(buf)
//	wasi, _ := r.NewHostModule(wazero.WASISnapshotPreview1WithConfig(config))
//	decoded, _ := r.CompileModule(source)
//
//	// Assign configuration only when ready to instantiate.
//	module, _ := StartWASICommandWithConfig(r, decoded, config.WithArgs("rotate", "angle=90", "dir=cw"))
//
// See StartWASICommand
func StartWASICommandWithConfig(r Runtime, module *Module, config *WASIConfig) (wasm.Module, error) {
	if err := internalwasi.ValidateWASICommand(module.module, module.name); err != nil {
		return nil, err
	}

	internal, ok := r.(*runtime)
	if !ok {
		return nil, fmt.Errorf("unsupported Runtime implementation: %s", r)
	}

	// Override the configuration if needed.
	ctx := internal.ctx
	if config != nil {
		ctx = context.WithValue(ctx, internalwasi.ConfigContextKey{}, newConfig(config)) // safe copy of config
	}

	mod, err := internal.store.Instantiate(ctx, module.module, module.name)
	if err != nil {
		return nil, err
	}

	start := mod.ExportedFunction(internalwasi.FunctionStart)
	if _, err = start.Call(mod.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("module[%s] function[%s] failed: %w", module.name, internalwasi.FunctionStart, err)
	}
	return mod, nil
}
