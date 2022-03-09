package wazero

import (
	"fmt"
	"io"

	internalwasi "github.com/tetratelabs/wazero/internal/wasi"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

// WASIDirFS returns a file system (a wasi.FS) for the tree of files rooted at
// the directory dir. It's similar to os.DirFS, except that it implements
// wasi.FS instead of the fs.FS interface.
func WASIDirFS(dir string) wasi.FS {
	return internalwasi.DirFS(dir)
}

func WASIMemFS() wasi.FS {
	return &internalwasi.MemFS{
		Files: map[string][]byte{},
	}
}

type WASIConfig struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Args     []string
	Environ  map[string]string
	Preopens map[string]wasi.FS
}

// WASISnapshotPreview1 are functions importable as the module name wasi.ModuleSnapshotPreview1
func WASISnapshotPreview1() *Module {
	return WASISnapshotPreview1WithConfig(&WASIConfig{})
}

// WASISnapshotPreview1WithConfig are functions importable as the module name wasi.ModuleSnapshotPreview1
func WASISnapshotPreview1WithConfig(c *WASIConfig) *Module {
	// TODO: delete the internalwasi.Option types as they are not accessible as they are internal!
	var opts []internalwasi.Option
	if c.Stdin != nil {
		opts = append(opts, internalwasi.Stdin(c.Stdin))
	}
	if c.Stdout != nil {
		opts = append(opts, internalwasi.Stdout(c.Stdout))
	}
	if c.Stderr != nil {
		opts = append(opts, internalwasi.Stderr(c.Stderr))
	}
	if len(c.Args) > 0 {
		opt, err := internalwasi.Args(c.Args...)
		if err != nil {
			panic(err) // better to panic vs have bother users about unlikely size > uint32
		}
		opts = append(opts, opt)
	}
	if len(c.Environ) > 0 {
		environ := make([]string, 0, len(c.Environ))
		for k, v := range c.Environ {
			environ = append(environ, fmt.Sprintf("%s=%s", k, v))
		}
		opt, err := internalwasi.Environ(environ...)
		if err != nil { // this can't be due to lack of '=' as we did that above.
			panic(err) // better to panic vs have bother users about unlikely size > uint32
		}
		opts = append(opts, opt)
	}
	if len(c.Preopens) > 0 {
		for k, v := range c.Preopens {
			opts = append(opts, internalwasi.Preopen(k, v))
		}
	}
	m, err := internalwasm.NewHostModule(wasi.ModuleSnapshotPreview1, internalwasi.SnapshotPreview1Functions(opts...))
	if err != nil {
		panic(fmt.Errorf("BUG: %w", err))
	}
	return &Module{name: wasi.ModuleSnapshotPreview1, module: m}
}

// StartWASICommandFromSource instantiates a module from the WebAssembly 1.0 (20191205) text or binary source or errs if
// invalid. Once instantiated, this starts its WASI Command function ("_start").
//
// Ex.
//	r := wazero.NewRuntime()
//	wasi, _ := r.NewHostModule(wazero.WASISnapshotPreview1())
//	module, _ := StartWASICommandFromSource(r, source)
//
// Note: This is a convenience utility that chains Runtime.DecodeModule with StartWASICommand.
func StartWASICommandFromSource(r Runtime, source []byte) (wasm.Module, error) {
	if decoded, err := r.DecodeModule(source); err != nil {
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
//	decoded, _ := r.DecodeModule(source)
//	module, _ := StartWASICommand(r, decoded)
//
// Prerequisites of the "Current Unstable ABI" from wasi.ModuleSnapshotPreview1:
// * "_start" is an exported nullary function and does not export "_initialize"
// * "memory" is an exported memory.
//
// Note: "_start" is invoked in the RuntimeConfig.Context.
// Note: Exporting "__indirect_function_table" is mentioned as required, but not enforced here.
// Note: The wasm.Functions return value does not restrict exports after "_start" as allowed in the specification.
// Note: All TinyGo Wasm are WASI commands. They initialize memory on "_start" and import "fd_write" to implement panic.
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/design/application-abi.md#current-unstable-abi
func StartWASICommand(r Runtime, module *Module) (wasm.Module, error) {
	if err := internalwasi.ValidateWASICommand(module.module, module.name); err != nil {
		return nil, err
	}

	internal, ok := r.(*runtime)
	if !ok {
		return nil, fmt.Errorf("unsupported Runtime implementation: %s", r)
	}

	mod, err := internal.store.Instantiate(module.module, module.name)
	if err != nil {
		return nil, err
	}

	start := mod.ExportedFunction(internalwasi.FunctionStart)
	if _, err = start.Call(nil); err != nil {
		return nil, fmt.Errorf("module[%s] function[%s] failed: %w", module.name, internalwasi.FunctionStart, err)
	}
	return mod, nil
}
