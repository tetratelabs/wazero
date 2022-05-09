package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	experimentalapi "github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/sys"
)

// Runtime allows embedding of WebAssembly modules.
//
// Ex.
//	ctx := context.Background()
//	r := wazero.NewRuntime()
//	module, _ := r.InstantiateModuleFromCode(ctx, source)
//	defer module.Close()
type Runtime interface {
	// NewModuleBuilder lets you create modules out of functions defined in Go.
	//
	// Ex. Below defines and instantiates a module named "env" with one function:
	//
	//	ctx := context.Background()
	//	hello := func() {
	//		fmt.Fprintln(stdout, "hello!")
	//	}
	//	_, err := r.NewModuleBuilder("env").ExportFunction("hello", hello).Instantiate(ctx)
	NewModuleBuilder(moduleName string) ModuleBuilder

	// Module returns exports from an instantiated module or nil if there aren't any.
	Module(moduleName string) api.Module

	// CompileModule decodes the WebAssembly text or binary source or errs if invalid.
	// Any pre-compilation done after decoding the source is dependent on RuntimeConfig or CompileConfig.
	//
	// There are two main reasons to use CompileModule instead of InstantiateModuleFromCode:
	//  * Improve performance when the same module is instantiated multiple times under different names
	//  * Reduce the amount of errors that can occur during InstantiateModule.
	//
	// Note: When the context is nil, it defaults to context.Background.
	// Note: The resulting module name defaults to what was binary from the custom name section.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	CompileModule(ctx context.Context, source []byte, config CompileConfig) (CompiledModule, error)

	// InstantiateModuleFromCode instantiates a module from the WebAssembly text or binary source or errs if invalid.
	//
	// Ex.
	//	ctx := context.Background()
	//	module, _ := wazero.NewRuntime().InstantiateModuleFromCode(ctx, source)
	//	defer module.Close()
	//
	// Note: When the context is nil, it defaults to context.Background.
	// Note: This is a convenience utility that chains CompileModule with InstantiateModule. To instantiate the same
	// source multiple times, use CompileModule as InstantiateModule avoids redundant decoding and/or compilation.
	// Note: To avoid using configuration defaults, use InstantiateModule instead.
	InstantiateModuleFromCode(ctx context.Context, source []byte) (api.Module, error)

	// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
	//
	// Ex.
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	compiled, _ := r.CompileModule(ctx, source, wazero.NewCompileConfig())
	//	defer compiled.Close()
	//	module, _ := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName("prod"))
	//	defer module.Close()
	//
	// While CompiledModule is pre-validated, there are a few situations which can cause an error:
	//  * The module name is already in use.
	//  * The module has a table element initializer that resolves to an index outside the Table minimum size.
	//  * The module has a start function, and it failed to execute.
	//
	// Configuration can also define different args depending on the importing module.
	//
	//	ctx := context.Background()
	//	r := wazero.NewRuntime()
	//	wasi, _ := wasi.InstantiateSnapshotPreview1(r)
	//	compiled, _ := r.CompileModule(ctx, source, wazero.NewCompileConfig())
	//
	//	// Initialize base configuration:
	//	config := wazero.NewModuleConfig().WithStdout(buf)
	//
	//	// Assign different configuration on each instantiation
	//	module, _ := r.InstantiateModule(ctx, compiled, config.WithName("rotate").WithArgs("rotate", "angle=90", "dir=cw"))
	//
	// Note: Config is copied during instantiation: Later changes to config do not affect the instantiated result.
	// Note: When the context is nil, it defaults to context.Background.
	InstantiateModule(ctx context.Context, compiled CompiledModule, config ModuleConfig) (api.Module, error)
}

func NewRuntime() Runtime {
	return NewRuntimeWithConfig(NewRuntimeConfig())
}

// NewRuntimeWithConfig returns a runtime with the given configuration.
func NewRuntimeWithConfig(rConfig RuntimeConfig) Runtime {
	config, ok := rConfig.(*runtimeConfig)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.RuntimeConfig implementation: %#v", rConfig))
	}
	return &runtime{
		store:           wasm.NewStore(config.enabledFeatures, config.newEngine(config.enabledFeatures)),
		enabledFeatures: config.enabledFeatures,
	}
}

// runtime allows decoupling of public interfaces from internal representation.
type runtime struct {
	store           *wasm.Store
	enabledFeatures wasm.Features
}

// Module implements Runtime.Module
func (r *runtime) Module(moduleName string) api.Module {
	return r.store.Module(moduleName)
}

// CompileModule implements Runtime.CompileModule
func (r *runtime) CompileModule(ctx context.Context, source []byte, cConfig CompileConfig) (CompiledModule, error) {
	if source == nil {
		return nil, errors.New("source == nil")
	}

	config, ok := cConfig.(*compileConfig)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.CompileConfig implementation: %#v", cConfig))
	}

	if len(source) < 8 { // Ex. less than magic+version in binary or '(module)' in text
		return nil, errors.New("invalid source")
	}

	// Peek to see if this is a binary or text format
	var decoder wasm.DecodeModule
	if bytes.Equal(source[0:4], binary.Magic) {
		decoder = binary.DecodeModule
	} else {
		decoder = text.DecodeModule
	}

	internal, err := decoder(source, r.enabledFeatures, config.memorySizer)
	if err != nil {
		return nil, err
	} else if err = internal.Validate(r.enabledFeatures); err != nil {
		// TODO: decoders should validate before returning, as that allows
		// them to err with the correct source position.
		return nil, err
	}

	// Replace imports if any configuration exists to do so.
	if importRenamer := config.importRenamer; importRenamer != nil {
		for _, i := range internal.ImportSection {
			i.Module, i.Name = importRenamer(i.Type, i.Module, i.Name)
		}
	}

	internal.AssignModuleID(source)

	if err = r.store.Engine.CompileModule(ctx, internal); err != nil {
		return nil, err
	}

	return &compiledCode{module: internal, compiledEngine: r.store.Engine}, nil
}

//
//func (c *compileConfig) replaceImports(compile *wasm.Compile) *wasm.Compile {
//	if (c.replacedImportCompiles == nil && c.replacedImports == nil) || compile.ImportSection == nil {
//		return compile
//	}
//
//	changed := false
//
//	ret := *compile // shallow copy
//	replacedImports := make([]*wasm.Import, len(compile.ImportSection))
//	copy(replacedImports, compile.ImportSection)
//
//	// First, replace any import.Compile
//	for oldCompile, newCompile := range c.replacedImportCompiles {
//		for i, imp := range replacedImports {
//			if imp.Compile == oldCompile {
//				changed = true
//				cp := *imp // shallow copy
//				cp.Compile = newCompile
//				replacedImports[i] = &cp
//			} else {
//				replacedImports[i] = imp
//			}
//		}
//	}
//
//	// Now, replace any import.Compile+import.Name
//	for oldImport, newImport := range c.replacedImports {
//		for i, imp := range replacedImports {
//			nulIdx := strings.IndexByte(oldImport, 0)
//			oldCompile := oldImport[0:nulIdx]
//			oldName := oldImport[nulIdx+1:]
//			if imp.Compile == oldCompile && imp.Name == oldName {
//				changed = true
//				cp := *imp // shallow copy
//				cp.Compile = newImport[0]
//				cp.Name = newImport[1]
//				replacedImports[i] = &cp
//			} else {
//				replacedImports[i] = imp
//			}
//		}
//	}
//
//	if !changed {
//		return compile
//	}
//	ret.ImportSection = replacedImports
//	return &ret
//}

// InstantiateModuleFromCode implements Runtime.InstantiateModuleFromCode
func (r *runtime) InstantiateModuleFromCode(ctx context.Context, source []byte) (api.Module, error) {
	if compiled, err := r.CompileModule(ctx, source, NewCompileConfig()); err != nil {
		return nil, err
	} else {
		// *wasm.ModuleInstance for the source cannot be tracked, so we release the cache inside this function.
		defer compiled.Close(ctx)
		return r.InstantiateModule(ctx, compiled, NewModuleConfig())
	}
}

// InstantiateModule implements Runtime.InstantiateModule
func (r *runtime) InstantiateModule(ctx context.Context, compiled CompiledModule, mConfig ModuleConfig) (mod api.Module, err error) {
	code, ok := compiled.(*compiledCode)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.CompiledModule implementation: %#v", compiled))
	}

	config, ok := mConfig.(*moduleConfig)
	if !ok {
		panic(fmt.Errorf("unsupported wazero.ModuleConfig implementation: %#v", mConfig))
	}

	var sysCtx *wasm.SysContext
	if sysCtx, err = config.toSysContext(); err != nil {
		return
	}

	name := config.name
	if name == "" && code.module.NameSection != nil && code.module.NameSection.ModuleName != "" {
		name = code.module.NameSection.ModuleName
	}

	var functionListenerFactory experimentalapi.FunctionListenerFactory
	if ctx != nil { // Test to see if internal code are using an experimental feature.
		if fnlf := ctx.Value(experimentalapi.FunctionListenerFactoryKey{}); fnlf != nil {
			functionListenerFactory = fnlf.(experimentalapi.FunctionListenerFactory)
		}
	}

	mod, err = r.store.Instantiate(ctx, code.module, name, sysCtx, functionListenerFactory)
	if err != nil {
		return
	}

	for _, fn := range config.startFunctions {
		start := mod.ExportedFunction(fn)
		if start == nil {
			continue
		}
		if _, err = start.Call(ctx); err != nil {
			if _, ok := err.(*sys.ExitError); ok {
				return
			}
			err = fmt.Errorf("module[%s] function[%s] failed: %w", name, fn, err)
			return
		}
	}
	return
}
