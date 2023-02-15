package wazero

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engineext"
	"github.com/tetratelabs/wazero/internal/filecache"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// This file only for internal usage only, and doesn't contain any exported symbols.

func wrapNewEngineExt(f newEngineExt) newEngine {
	return func(ctx context.Context, features api.CoreFeatures, fc filecache.Cache) wasm.Engine {
		e := f(ctx, features, fc)
		return &engineExtWrap{e}
	}
}

type engineExtWrap struct{ engineext.EngineExt }

func (ee *engineExtWrap) Close() (err error) {
	return ee.CloseFn()
}

func (ee *engineExtWrap) CompileModule(ctx context.Context, module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) error {
	return ee.CompileModuleFn(ctx, module, listeners, ensureTermination)
}

func (ee *engineExtWrap) CompiledModuleCount() uint32 {
	return ee.CompiledModuleCountFn()
}

func (ee *engineExtWrap) DeleteCompiledModule(module *wasm.Module) {
	ee.DeleteCompiledModuleFn(module)
}

func (ee *engineExtWrap) NewModuleEngine(name string, module *wasm.Module, functions []wasm.FunctionInstance) (wasm.ModuleEngine, error) {
	imported := module.ImportFuncCount()
	var moduleInst any
	for i := range functions[imported:] {
		moduleInst = functions[i].Module
	}
	base, err := ee.NewModuleEngineFn(name, module, moduleInst)
	return &moduleEngineExtWrap{base}, err
}

type moduleEngineExtWrap struct{ engineext.ModuleEngineExt }

func (m *moduleEngineExtWrap) Name() string {
	return m.NameFn()
}

func (m *moduleEngineExtWrap) NewCallEngine(callCtx *wasm.CallContext, f *wasm.FunctionInstance) (wasm.CallEngine, error) {
	callExtFn, err := m.NewCallEngineFn(callCtx, f)
	if err != nil {
		return nil, err
	}
	return callEngineExtWrap{callExtFn}, nil
}

func (m *moduleEngineExtWrap) LookupFunction(t *wasm.TableInstance, typeId wasm.FunctionTypeID, tableOffset wasm.Index) (wasm.Index, error) {
	return m.LookupFunctionFn(t, uint32(typeId), tableOffset)
}

func (m *moduleEngineExtWrap) CreateFuncElementInstance(indexes []*wasm.Index) *wasm.ElementInstance {
	refs := m.GetFunctionReferencesFn(indexes)
	return &wasm.ElementInstance{
		References: refs,
		Type:       wasm.ExternTypeFunc,
	}
}

func (m *moduleEngineExtWrap) FunctionInstanceReference(funcIndex wasm.Index) wasm.Reference {
	return m.FunctionInstanceReferenceFn(funcIndex)
}

type callEngineExtWrap struct {
	engineext.CallEngineExt
}

func (ce callEngineExtWrap) Call(ctx context.Context, m *wasm.CallContext, params []uint64) (results []uint64, err error) {
	return ce.CallEngineExt(ctx, m, params)
}
