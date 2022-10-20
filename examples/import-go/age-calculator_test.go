package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/wasm"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

const (
	// callGoHostName is the name of exported function which calls the Go-implemented host function.
	callGoHostName = "call_go_host"
	// callWasmHostName is the name of exported function which calls the Wasm-implemented host function.
	callWasmHostName = "call_wasm_host"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

// Test_main ensures the following will work:
//
//	go run age-calculator.go 2000
func Test_main(t *testing.T) {
	// Set ENV to ensure this test doesn't need maintenance every year.
	t.Setenv("CURRENT_YEAR", "2021")

	stdout, _ := maintester.TestMain(t, main, "age-calculator", "2000")
	require.Equal(t, `println >> 21
log_i32 >> 21
`, stdout)
}

// BenchmarkHostFunctionCall measures the cost of host function calls whose target functions are either
// Go-implemented or Wasm-implemented, and compare the results between them.
func BenchmarkHostFunctionCall(b *testing.B) {
	if !platform.CompilerSupported() {
		b.Skip()
	}

	m := setupHostCallBench(func(err error) {
		if err != nil {
			b.Fatal(err)
		}
	})

	const offset = 100
	const val = float32(1.1234)

	binary.LittleEndian.PutUint32(m.Memory.Buffer[offset:], math.Float32bits(val))

	b.Run(callGoHostName, func(b *testing.B) {
		ce, err := getCallEngine(m, callGoHostName)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := ce.Call(testCtx, m.CallCtx, offset)
			if err != nil {
				b.Fatal(err)
			}
			if uint32(res[0]) != math.Float32bits(val) {
				b.Fail()
			}
		}
	})

	b.Run(callWasmHostName, func(b *testing.B) {
		ce, err := getCallEngine(m, callWasmHostName)
		if err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := ce.Call(testCtx, m.CallCtx, offset)
			if err != nil {
				b.Fatal(err)
			}
			if uint32(res[0]) != math.Float32bits(val) {
				b.Fail()
			}
		}
	})
}

func getCallEngine(m *wasm.ModuleInstance, name string) (ce wasm.CallEngine, err error) {
	f := m.Exports[name].Function
	if f == nil {
		err = fmt.Errorf("%s not found", name)
		return
	}

	ce, err = m.Engine.NewCallEngine(m.CallCtx, f)
	return
}

func setupHostCallBench(requireNoError func(error)) *wasm.ModuleInstance {
	eng := compiler.NewEngine(context.Background(), api.CoreFeaturesV2)

	ft := &wasm.FunctionType{
		Params:           []wasm.ValueType{wasm.ValueTypeI32},
		Results:          []wasm.ValueType{wasm.ValueTypeF32},
		ParamNumInUint64: 1, ResultNumInUint64: 1,
	}

	// Build the host module.
	hostModule := &wasm.Module{
		TypeSection:     []*wasm.FunctionType{ft},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []*wasm.Code{
			wasm.MustParseGoFuncCode(
				&api.HostFuncSignature{
					Fn: func(ctx context.Context, m api.Module, params ...uint64) ([]uint64, error) {
						ret, ok := m.Memory().ReadUint32Le(ctx, uint32(params[0]))
						if !ok {
							panic("couldn't read memory")
						}
						return []uint64{uint64(ret)}, nil
					},
					NumIn:  []api.ValueType{api.ValueTypeI32},
					NumOut: []api.ValueType{api.ValueTypeF32},
				},
			),
			{
				IsHostFunction: true,
				Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeI32Load, 0x2, 0x0, // offset = 0
					wasm.OpcodeF32ReinterpretI32,
					wasm.OpcodeEnd,
				},
			},
		},
		ExportSection: []*wasm.Export{
			{Name: "go", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "wasm", Type: wasm.ExternTypeFunc, Index: 1},
		},
		ID: wasm.ModuleID{1, 2, 3, 4, 5},
	}
	hostModule.BuildFunctionDefinitions()

	host := &wasm.ModuleInstance{Name: "host", TypeIDs: []wasm.FunctionTypeID{0}}
	host.Functions = host.BuildFunctions(hostModule, nil)
	host.BuildExports(hostModule.ExportSection)
	goFn, wasnFn := host.Exports["go"].Function, host.Exports["wasm"].Function

	err := eng.CompileModule(testCtx, hostModule)
	requireNoError(err)

	hostME, err := eng.NewModuleEngine(host.Name, hostModule, nil, host.Functions, nil, nil)
	requireNoError(err)
	linkModuleToEngine(host, hostME)

	// Build the importing module.
	importingModule := &wasm.Module{
		TypeSection: []*wasm.FunctionType{ft},
		ImportSection: []*wasm.Import{
			// Placeholders for imports from hostModule.
			{Type: wasm.ExternTypeFunc},
			{Type: wasm.ExternTypeFunc},
		},
		FunctionSection: []wasm.Index{0, 0},
		ExportSection: []*wasm.Export{
			{Name: callGoHostName, Type: wasm.ExternTypeFunc, Index: 2},
			{Name: callWasmHostName, Type: wasm.ExternTypeFunc, Index: 3},
		},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Calling the index 0 = host.go.
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 1, wasm.OpcodeEnd}}, // Calling the index 1 = host.wasm.
		},
		// Indicates that this module has a memory so that compilers are able to assemble memory-related initialization.
		MemorySection: &wasm.Memory{Min: 1},
		ID:            wasm.ModuleID{1},
	}

	importingModule.BuildFunctionDefinitions()
	err = eng.CompileModule(testCtx, importingModule)
	requireNoError(err)

	importing := &wasm.ModuleInstance{TypeIDs: []wasm.FunctionTypeID{0}}
	importingFunctions := importing.BuildFunctions(importingModule, nil)
	importing.Functions = append([]*wasm.FunctionInstance{goFn, wasnFn}, importingFunctions...)
	importing.BuildExports(importingModule.ExportSection)

	importingMe, err := eng.NewModuleEngine(importing.Name, importingModule, []*wasm.FunctionInstance{goFn, wasnFn}, importingFunctions, nil, nil)
	requireNoError(err)
	linkModuleToEngine(importing, importingMe)

	importing.Memory = &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize), Min: 1, Cap: 1, Max: 1}
	return importing
}

func linkModuleToEngine(module *wasm.ModuleInstance, me wasm.ModuleEngine) {
	module.Engine = me
	module.CallCtx = wasm.NewCallContext(nil, module, nil)
}
