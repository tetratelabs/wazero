package wasm

import (
	"context"
	"fmt"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
)

// LookupFunction looks up the table by the given index, and returns the api.Function implementation if found,
// otherwise this panics according to the same semantics as call_indirect instruction.
// Currently, this is only used by emscripten which needs to do call_indirect-like operation in the host function.
func (m *ModuleInstance) LookupFunction(t *TableInstance, typeId FunctionTypeID, tableOffset Index) api.Function {
	fm, index := m.Engine.LookupFunction(t, typeId, tableOffset)
	if source := fm.Source; source.IsHostModule {
		// This case, the function is a host function stored in the table.
		// At the top leve, engines are only responsible for calling Wasm-defined functions,
		// so we need to wrap the host function as a special case.
		def := &source.FunctionDefinitionSection[index]
		goF := source.CodeSection[index].GoFunc
		switch typed := goF.(type) {
		case api.GoFunction:
			return &lookedUpGoFunction{def: def, lookedUpModule: m, g: typed}
		case api.GoModuleFunction:
			return &lookedUpGoModuleFunction{def: def, lookedUpModule: m, g: typed}
		default:
			panic(fmt.Sprintf("unexpected GoFunc type: %T", goF))
		}
	} else {
		return fm.Engine.NewFunction(index)
	}
}

type (
	// lookedUpGoFunction implements api.Function for an api.GoFunction.
	lookedUpGoFunction lookedUpGoFunctionBase[api.GoFunction]
	// lookedUpGoModuleFunction implements api.Function for an api.GoModuleFunction.
	lookedUpGoModuleFunction lookedUpGoFunctionBase[api.GoModuleFunction]
	// lookedUpGoFunctionBase is a base type for lookedUpGoFunction and lookedUpGoModuleFunction.
	lookedUpGoFunctionBase[T any] struct {
		internalapi.WazeroOnly
		def            *FunctionDefinition
		lookedUpModule *ModuleInstance
		g              T
	}
)

// Definition implements api.Function.
func (l *lookedUpGoModuleFunction) Definition() api.FunctionDefinition { return l.def }

// Call implements api.Function.
func (l *lookedUpGoModuleFunction) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	typ := l.def.Functype
	stackSize := typ.ParamNumInUint64
	rn := typ.ResultNumInUint64
	if rn > stackSize {
		stackSize = rn
	}
	stack := make([]uint64, stackSize)
	copy(stack, params)
	l.g.Call(ctx, l.lookedUpModule, stack)
	return stack[:rn], nil
}

// CallWithStack implements api.Function.
func (l *lookedUpGoModuleFunction) CallWithStack(ctx context.Context, stack []uint64) error {
	l.g.Call(ctx, l.lookedUpModule, stack)
	return nil
}

// Definition implements api.Function.
func (l *lookedUpGoFunction) Definition() api.FunctionDefinition { return l.def }

// Call implements api.Function.
func (l *lookedUpGoFunction) Call(ctx context.Context, params ...uint64) ([]uint64, error) {
	typ := l.def.Functype
	stackSize := typ.ParamNumInUint64
	rn := typ.ResultNumInUint64
	if rn > stackSize {
		stackSize = rn
	}
	stack := make([]uint64, stackSize)
	copy(stack, params)
	l.g.Call(ctx, stack)
	return stack[:rn], nil
}

// CallWithStack implements api.Function.
func (l *lookedUpGoFunction) CallWithStack(ctx context.Context, stack []uint64) error {
	l.g.Call(ctx, stack)
	return nil
}
