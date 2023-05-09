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
				before: func(context.Context, api.Module, api.FunctionDefinition, []uint64, experimental.StackIterator) context.Context {
					return context.Background()
				},
				after: func(context.Context, api.Module, api.FunctionDefinition, error, []uint64) {
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
		ctx:          context.Background(),
		stack:        []uint64{0, 1, 2, 3, 4, 0, 0, 0},
		stackContext: stackContext{stackBasePointerInBytes: 16},
	}

	module := new(wasm.ModuleInstance)

	for i := 0; i < b.N; i++ {
		ce.builtinFunctionFunctionListenerBefore(ce.ctx, module, f)
		ce.builtinFunctionFunctionListenerAfter(ce.ctx, module, f)
	}
}
