package bench

import (
	"context"
	_ "embed"
	"encoding/binary"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	// callGoHostName is the name of exported function which calls the
	// Go-implemented host function.
	callGoHostName = "call_go_host"
	// callGoReflectHostName is the name of exported function which calls the
	// Go-implemented host function defined in reflection.
	callGoReflectHostName = "call_go_reflect_host"
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

	const offset = uint64(100)
	const val = float32(1.1234)

	binary.LittleEndian.PutUint32(m.MemoryInstance.Buffer[offset:], math.Float32bits(val))

	for _, fn := range []string{callGoReflectHostName, callGoHostName} {
		fn := fn

		b.Run(fn, func(b *testing.B) {
			ce := getCallEngine(m, fn)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := ce.Call(testCtx, offset)
				if err != nil {
					b.Fatal(err)
				}
				if uint32(res[0]) != math.Float32bits(val) {
					b.Fail()
				}
			}
		})

		b.Run(fn+"_with_stack", func(b *testing.B) {
			ce := getCallEngine(m, fn)

			b.ResetTimer()
			stack := make([]uint64, 1)
			for i := 0; i < b.N; i++ {
				stack[0] = offset
				err := ce.CallWithStack(testCtx, stack)
				if err != nil {
					b.Fatal(err)
				}
				if uint32(stack[0]) != math.Float32bits(val) {
					b.Fail()
				}
			}
		})
	}
}

func TestBenchmarkFunctionCall(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	m := setupHostCallBench(func(err error) {
		require.NoError(t, err)
	})

	callGoHost := getCallEngine(m, callGoHostName)
	callGoReflectHost := getCallEngine(m, callGoReflectHostName)

	require.NotNil(t, callGoHost)
	require.NotNil(t, callGoReflectHost)

	tests := []struct {
		offset uint32
		val    float32
	}{
		{offset: 0, val: math.Float32frombits(0xffffffff)},
		{offset: 100, val: 1.12314},
		{offset: wasm.MemoryPageSize - 4, val: 1.12314},
	}

	mem := m.MemoryInstance.Buffer

	for _, f := range []struct {
		name string
		ce   api.Function
	}{
		{name: "go", ce: callGoHost},
		{name: "go-reflect", ce: callGoReflectHost},
	} {
		f := f
		t.Run(f.name, func(t *testing.T) {
			for _, tc := range tests {
				binary.LittleEndian.PutUint32(mem[tc.offset:], math.Float32bits(tc.val))
				res, err := f.ce.Call(context.Background(), uint64(tc.offset))
				require.NoError(t, err)
				require.Equal(t, math.Float32bits(tc.val), uint32(res[0]))
			}
		})
	}
}

func getCallEngine(m *wasm.ModuleInstance, name string) (ce api.Function) {
	exp := m.Exports[name]
	ce = m.Engine.NewFunction(exp.Index)
	return
}

func setupHostCallBench(requireNoError func(error)) *wasm.ModuleInstance {
	eng := compiler.NewEngine(context.Background(), api.CoreFeaturesV2, nil)

	ft := wasm.FunctionType{
		Params:           []wasm.ValueType{wasm.ValueTypeI32},
		Results:          []wasm.ValueType{wasm.ValueTypeF32},
		ParamNumInUint64: 1, ResultNumInUint64: 1,
	}

	// Build the host module.
	hostModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{ft},
		FunctionSection: []wasm.Index{0, 0},
		CodeSection: []wasm.Code{
			{
				GoFunc: api.GoModuleFunc(func(_ context.Context, mod api.Module, stack []uint64) {
					ret, ok := mod.Memory().ReadUint32Le(uint32(stack[0]))
					if !ok {
						panic("couldn't read memory")
					}
					stack[0] = uint64(ret)
				}),
			},
			wasm.MustParseGoReflectFuncCode(
				func(_ context.Context, m api.Module, pos uint32) float32 {
					ret, ok := m.Memory().ReadUint32Le(pos)
					if !ok {
						panic("couldn't read memory")
					}
					return math.Float32frombits(ret)
				},
			),
		},
		ExportSection: []wasm.Export{
			{Name: "go", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "go-reflect", Type: wasm.ExternTypeFunc, Index: 1},
		},
		Exports: map[string]*wasm.Export{
			"go":         {Name: "go", Type: wasm.ExternTypeFunc, Index: 0},
			"go-reflect": {Name: "go-reflect", Type: wasm.ExternTypeFunc, Index: 1},
		},
		ID: wasm.ModuleID{1, 2, 3, 4, 5},
	}
	hostModule.BuildFunctionDefinitions()

	host := &wasm.ModuleInstance{ModuleName: "host", TypeIDs: []wasm.FunctionTypeID{0}}
	host.Exports = hostModule.Exports

	err := eng.CompileModule(testCtx, hostModule, nil, false)
	requireNoError(err)

	hostMe, err := eng.NewModuleEngine(hostModule, host)
	requireNoError(err)
	linkModuleToEngine(host, hostMe)

	// Build the importing module.
	importingModule := &wasm.Module{
		ImportFunctionCount: 2,
		TypeSection:         []wasm.FunctionType{ft},
		ImportSection: []wasm.Import{
			// Placeholders for imports from hostModule.
			{Type: wasm.ExternTypeFunc},
			{Type: wasm.ExternTypeFunc},
		},
		FunctionSection: []wasm.Index{0, 0},
		ExportSection: []wasm.Export{
			{Name: callGoHostName, Type: wasm.ExternTypeFunc, Index: 2},
			{Name: callGoReflectHostName, Type: wasm.ExternTypeFunc, Index: 3},
		},
		Exports: map[string]*wasm.Export{
			callGoHostName:        {Name: callGoHostName, Type: wasm.ExternTypeFunc, Index: 2},
			callGoReflectHostName: {Name: callGoReflectHostName, Type: wasm.ExternTypeFunc, Index: 3},
		},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}}, // Calling the index 0 = host.go.
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 1, wasm.OpcodeEnd}}, // Calling the index 1 = host.go-reflect.
		},
		// Indicates that this module has a memory so that compilers are able to assemble memory-related initialization.
		MemorySection: &wasm.Memory{Min: 1},
		ID:            wasm.ModuleID{1},
	}

	importingModule.BuildFunctionDefinitions()
	err = eng.CompileModule(testCtx, importingModule, nil, false)
	requireNoError(err)

	importing := &wasm.ModuleInstance{TypeIDs: []wasm.FunctionTypeID{0}}
	importing.Exports = importingModule.Exports

	importingMe, err := eng.NewModuleEngine(importingModule, importing)
	requireNoError(err)
	linkModuleToEngine(importing, importingMe)
	importingMe.ResolveImportedFunction(0, 0, hostMe)
	importingMe.ResolveImportedFunction(1, 1, hostMe)

	importing.MemoryInstance = &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize), Min: 1, Cap: 1, Max: 1}
	return importing
}

func linkModuleToEngine(module *wasm.ModuleInstance, me wasm.ModuleEngine) {
	module.Engine = me
}
