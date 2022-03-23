package wazero

import (
	"fmt"

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

// WASISnapshotPreview1 are functions importable as the module name wasi.ModuleSnapshotPreview1
func WASISnapshotPreview1() *Module {
	_, fns := internalwasi.SnapshotPreview1Functions()
	m, err := internalwasm.NewHostModule(wasi.ModuleSnapshotPreview1, fns)
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
	return startWASICommandWithSysContext(r, module, internalwasm.DefaultSysContext())
}

// StartWASICommandWithConfig is like StartWASICommand, except you can override configuration based on the importing
// module. For example, you can use this to define different args depending on the importing module.
//
//	r := wazero.NewRuntime()
//	wasi, _ := r.NewHostModule(wazero.WASISnapshotPreview1())
//	mod, _ := r.CompileModule(source)
//
//	// Initialize base configuration:
//	sys := wazero.NewSysConfig().WithStdout(buf)
//
//	// Assign different configuration on each instantiation
//	module, _ := StartWASICommandWithConfig(r, mod.WithName("rotate"), sys.WithArgs("rotate", "angle=90", "dir=cw"))
//
// Note: Config is copied during instantiation: Later changes to config do not affect the instantiated result.
// See StartWASICommand
func StartWASICommandWithConfig(r Runtime, module *Module, config *SysConfig) (mod wasm.Module, err error) {
	var sys *internalwasm.SysContext
	if sys, err = config.toSysContext(); err != nil {
		return
	}
	return startWASICommandWithSysContext(r, module, sys)
}

func startWASICommandWithSysContext(r Runtime, module *Module, sys *internalwasm.SysContext) (mod wasm.Module, err error) {
	if err = internalwasi.ValidateWASICommand(module.module, module.name); err != nil {
		return
	}

	internal, ok := r.(*runtime)
	if !ok {
		return nil, fmt.Errorf("unsupported Runtime implementation: %s", r)
	}

	if mod, err = internal.store.Instantiate(internal.ctx, module.module, module.name, sys); err != nil {
		return
	}

	start := mod.ExportedFunction(internalwasi.FunctionStart)
	if _, err = start.Call(mod.WithContext(internal.ctx)); err != nil {
		return nil, fmt.Errorf("module[%s] function[%s] failed: %w", module.name, internalwasi.FunctionStart, err)
	}
	return mod, nil
}
