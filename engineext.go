package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engineext"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// This file is only for internal usage, and doesn't contain any exported symbols.

// wrapNewEngineExt wraps an newEngineExt as newEngine.
func wrapNewEngineExt(f newEngineExt) newEngine {
	return func(ctx context.Context, features api.CoreFeatures, fc filecache.Cache) wasm.Engine {
		e := f(ctx, features, fc)
		return &engineExtWrap{e}
	}
}

// engineExtWrap wraps engineext.EngineExt and implements wasm.Engine.
type engineExtWrap struct{ engineext.EngineExt }

// Close implements wasm.Engine CompileModule.
func (ee *engineExtWrap) Close() (err error) {
	return ee.CloseFn()
}

// CompileModule implements wasm.Engine CompileModule.
func (ee *engineExtWrap) CompileModule(ctx context.Context, module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) error {
	return ee.CompileModuleFn(ctx, module, listeners, ensureTermination)
}

// CompiledModuleCount implements wasm.Engine CompiledModuleCount.
func (ee *engineExtWrap) CompiledModuleCount() uint32 {
	return ee.CompiledModuleCountFn()
}

// DeleteCompiledModule implements wasm.Engine DeleteCompiledModule.
func (ee *engineExtWrap) DeleteCompiledModule(module *wasm.Module) {
	ee.DeleteCompiledModuleFn(module)
}

// NewModuleEngine implements wasm.Engine NewModuleEngine.
func (ee *engineExtWrap) NewModuleEngine(name string, module *wasm.Module, functions []wasm.FunctionInstance) (wasm.ModuleEngine, error) {
	imported := module.ImportFuncCount()
	var moduleInst any
	// TODO: refactor wasm.Engine NewModuleEngine, and pass the module instance directly.
	for i := range functions[imported:] {
		moduleInst = functions[i].Module
		break // Take the first one.
	}
	base, err := ee.NewModuleEngineFn(name, module, moduleInst)
	return &moduleEngineExtWrap{base}, err
}

// engineExtWrap wraps engineext.ModuleEngineExt and implements wasm.ModuleEngine.
type moduleEngineExtWrap struct{ engineext.ModuleEngineExt }

// Name implements wasm.ModuleEngine Name.
func (m *moduleEngineExtWrap) Name() string {
	return m.NameFn()
}

// NewCallEngine implements wasm.ModuleEngine NewCallEngine.
func (m *moduleEngineExtWrap) NewCallEngine(callCtx *wasm.CallContext, f *wasm.FunctionInstance) (wasm.CallEngine, error) {
	callExtFn, err := m.NewCallEngineFn(callCtx, f)
	if err != nil {
		return nil, err
	}
	return callEngineExtWrap{callExtFn}, nil
}

// LookupFunction implements wasm.ModuleEngine LookupFunction.
func (m *moduleEngineExtWrap) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (wasm.Index, error) {
	return m.LookupFunctionFn(t, uint32(typeId), tableOffset)
}

// CreateFuncElementInstance implements wasm.ModuleEngine CreateFuncElementInstance.
func (m *moduleEngineExtWrap) CreateFuncElementInstance(indexes []*wasm.Index) *wasm.ElementInstance {
	refs := m.GetFunctionReferencesFn(indexes)
	return &wasm.ElementInstance{
		References: refs,
		Type:       wasm.ExternTypeFunc,
	}
}

// FunctionInstanceReference implements wasm.ModuleEngine FunctionInstanceReference.
func (m *moduleEngineExtWrap) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference {
	return m.FunctionInstanceReferenceFn(funcIndex)
}

// callEngineExtWrap wraps engineext.CallEngineExt and implements wasm.CallEngine.
type callEngineExtWrap struct {
	c engineext.CallEngineExt
}

// Call implements wasm.CallEngine Call.
func (ce callEngineExtWrap) Call(ctx context.Context, m *wasm.CallContext, params []uint64) (results []uint64, err error) {
	return ce.c(ctx, m, params)
}
