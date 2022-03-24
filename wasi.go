package wazero

import (
	"fmt"

	internalwasi "github.com/tetratelabs/wazero/internal/wasi"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
)

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

// StartWASICommand instantiates the module and starts its WASI Command function ("_start") if present. The return value
// are all exported functions in the module. This errs if the module doesn't export a memory named "memory", or there
// are any instantiation or function call errors. On success, other modules can import wasi.ModuleSnapshotPreview1.
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
// ## "memory" export
// WASI snapshot-01 requires exporting a memory named "memory", and wazero enforces this as nearly all functions use
// memory to implement multiple returns. StartWASICommand errs if there is no memory exported as "memory".
//
// ## "_start" function export
// WASI snapshot-01 requires exporting a function named "_start", but wazero does not enforce this. If it is defined,
// it is called directly after any module-defined start function, in the runtime context (RuntimeConfig.WithContext).
//
// ## "__indirect_function_table" function export
// WASI snapshot-01 requires exporting a table named "__indirect_function_table", but wazero does not enforce this.
//
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
		err = fmt.Errorf("unsupported Runtime implementation: %s", r)
		return
	}

	if mod, err = internal.store.Instantiate(internal.ctx, module.module, module.name, sys); err != nil {
		return
	}

	start := mod.ExportedFunction(internalwasi.FunctionStart)
	if start == nil {
		return
	}
	if _, err = start.Call(mod.WithContext(internal.ctx)); err != nil {
		err = fmt.Errorf("module[%s] function[%s] failed: %w", module.name, internalwasi.FunctionStart, err)
	}
	return
}
