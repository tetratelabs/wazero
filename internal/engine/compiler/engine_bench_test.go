package compiler

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func BenchmarkCallEngine_builtinFunctionFunctionListener(b *testing.B) {
	f := &function{
		funcType: &wasm.FunctionType{ParamNumInUint64: 3},
		parent: &compiledFunction{
			listener: mockListener{
				before: func(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator) {
				},
				after: func(context.Context, api.Module, api.FunctionDefinition, []uint64) {
				},
			},
			index: 0,
			parent: &compiledModule{
				source: &wasm.Module{
					FunctionDefinitionSection: []wasm.FunctionDefinition{{}},
				},
			},
		},
	}

	ce := &callEngine{
		stack:        []uint64{0, 1, 2, 3, 4, 0, 0, 0},
		stackContext: stackContext{stackBasePointerInBytes: 16},
	}

	mod := new(wasm.ModuleInstance)
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		ce.builtinFunctionFunctionListenerBefore(ctx, mod, f)
		ce.builtinFunctionFunctionListenerAfter(ctx, mod, f)
	}
}
