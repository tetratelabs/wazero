package wasm

import (
	"context"
	"fmt"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
)

// LookupFunction implements api.ModuleInstance.
func (m *ModuleInstance) LookupFunction(t *TableInstance, typeId FunctionTypeID, tableOffset Index) api.Function {
	fm, index := m.Engine.LookupFunction(t, typeId, tableOffset)
	if source := fm.Source; source.IsHostModule {
		def := source.FunctionDefinition(index)
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
	lookedUpGoFunction struct {
		internalapi.WazeroOnly
		def            *FunctionDefinition
		lookedUpModule *ModuleInstance
		g              api.GoFunction
	}
	lookedUpGoModuleFunction struct {
		internalapi.WazeroOnly
		def            *FunctionDefinition
		lookedUpModule *ModuleInstance
		g              api.GoModuleFunction
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
