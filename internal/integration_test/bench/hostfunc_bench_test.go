package bench

import (
	"context"
	_ "embed"
	"encoding/binary"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"math"
	"testing"
)

const (
	// callGoHostName is the name of exported function which calls the Go-implemented host function.
	callGoHostName = "call_go_host"
	// callWasmHostName is the name of exported function which calls the Wasm-implemented host function.
	callWasmHostName = "call_wasm_host"
)

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
		callGoHost := m.Exports[callGoHostName].Function
		if callGoHost == nil {
			b.Fatal()
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := callGoHost.Call(testCtx, offset)
			if err != nil {
				b.Fatal(err)
			}
			if uint32(res[0]) != math.Float32bits(val) {
				b.Fail()
			}
		}
	})

	b.Run(callWasmHostName, func(b *testing.B) {
		callWasmHost := m.Exports[callWasmHostName].Function
		if callWasmHost == nil {
			b.Fatal()
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			res, err := callWasmHost.Call(testCtx, offset)
			if err != nil {
				b.Fatal(err)
			}
			if uint32(res[0]) != math.Float32bits(val) {
				b.Fail()
			}
		}
	})
}

func TestBenchmarkFunctionCall(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	m := setupHostCallBench(func(err error) {
		require.NoError(t, err)
	})

	callWasmHost := m.Exports[callWasmHostName].Function
	callGoHost := m.Exports[callGoHostName].Function

	require.NotNil(t, callWasmHost)
	require.NotNil(t, callGoHost)

	tests := []struct {
		offset uint32
		val    float32
	}{
		{offset: 0, val: math.Float32frombits(0xffffffff)},
		{offset: 100, val: 1.12314},
		{offset: wasm.MemoryPageSize - 4, val: 1.12314},
	}

	mem := m.Memory.Buffer

	for _, f := range []struct {
		name string
		f    *wasm.FunctionInstance
	}{
		{name: "go", f: callGoHost},
		{name: "wasm", f: callWasmHost},
	} {
		f := f
		t.Run(f.name, func(t *testing.T) {
			for _, tc := range tests {
				binary.LittleEndian.PutUint32(mem[tc.offset:], math.Float32bits(tc.val))
				res, err := f.f.Call(context.Background(), uint64(tc.offset))
				require.NoError(t, err)
				require.Equal(t, math.Float32bits(tc.val), uint32(res[0]))
			}
		})
	}
}

func setupHostCallBench(requireNoError func(error)) *wasm.ModuleInstance {
	eng := compiler.NewEngine(context.Background(), wasm.Features20220419)

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
			{
				IsHostFunction: true,
				Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeI32Load, 0x2, 0x0, // offset = 0
					wasm.OpcodeF32ReinterpretI32,
					wasm.OpcodeEnd,
				},
			},
			wasm.MustParseGoFuncCode(
				func(ctx context.Context, m api.Module, pos uint32) float32 {
					ret, ok := m.Memory().ReadUint32Le(ctx, pos)
					if !ok {
						panic("couldn't read memory")
					}
					return math.Float32frombits(ret)
				},
			),
		},
		ExportSection: []*wasm.Export{
			{Name: "wasm", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "go", Type: wasm.ExternTypeFunc, Index: 1},
		},
		ID: wasm.ModuleID{1, 2, 3, 4, 5},
	}
	hostModule.BuildFunctionDefinitions()

	host := &wasm.ModuleInstance{Name: "host", TypeIDs: []wasm.FunctionTypeID{0}}
	host.Functions = host.BuildFunctions(hostModule, nil)
	host.BuildExports(hostModule.ExportSection)
	goFn, wasnFn := host.Exports["wasm"].Function, host.Exports["go"].Function

	err := eng.CompileModule(testCtx, hostModule)
	requireNoError(err)

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
		// Indicates that this module has a memory so that compilers are able to assembe memory-related initialization.
		MemorySection: &wasm.Memory{Min: 1},
		ID:            wasm.ModuleID{1},
	}

	importingModule.BuildFunctionDefinitions()
	err = eng.CompileModule(testCtx, importingModule)
	requireNoError(err)

	hostME, err := eng.NewModuleEngine(host.Name, hostModule, nil, host.Functions, nil, nil)
	requireNoError(err)
	linkModuleToEngine(host, hostME)

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
