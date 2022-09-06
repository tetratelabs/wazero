package spfunc

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func Test_callFromSP(t *testing.T) {
	tests := []struct {
		name                string
		expectSP            bool
		inputMem, outputMem []byte
		proxied             *wasm.HostFunc
		expected            *wasm.ProxyFunc
		expectedErr         string
	}{
		{
			name: "i32_v",
			proxied: &wasm.HostFunc{
				ExportNames: []string{"ex"},
				Name:        "n",
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
				ParamNames:  []string{"x"},
				Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeEnd}},
			},
			expected: &wasm.ProxyFunc{
				Proxy: &wasm.HostFunc{
					ExportNames: []string{"ex"},
					Name:        "n.proxy",
					ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
					ParamNames:  []string{"sp"},
					Code: &wasm.Code{
						IsHostFunction: true,
						LocalTypes:     nil, // because there are no results
						Body: []byte{
							wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 8, wasm.OpcodeI32Add,
							wasm.OpcodeI32Load, 0x2, 0x0,
							wasm.OpcodeCall, 0,
							wasm.OpcodeEnd,
						},
					},
				},
				Proxied: &wasm.HostFunc{
					Name:       "n",
					ParamTypes: []wasm.ValueType{wasm.ValueTypeI32},
					ParamNames: []string{"x"},
					Code:       &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeEnd}},
				},
				CallBodyPos: 9,
			},
		},
		{
			name: "i32_i32",
			proxied: &wasm.HostFunc{
				ExportNames: []string{"ex"},
				Name:        "n",
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
				ParamNames:  []string{"x"},
				ResultTypes: []wasm.ValueType{wasm.ValueTypeI32},
				Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeEnd}},
			},
			expected: &wasm.ProxyFunc{
				Proxy: &wasm.HostFunc{
					ExportNames: []string{"ex"},
					Name:        "n.proxy",
					ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
					ParamNames:  []string{"sp"},
					Code: &wasm.Code{
						IsHostFunction: true,
						LocalTypes:     []api.ValueType{api.ValueTypeI32, api.ValueTypeI64},
						Body: []byte{
							wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 8, wasm.OpcodeI32Add,
							wasm.OpcodeI32Load, 0x2, 0x0,
							wasm.OpcodeCall, 0,
							wasm.OpcodeLocalSet, 1,
							wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 16, wasm.OpcodeI32Add,
							wasm.OpcodeLocalGet, 1,
							wasm.OpcodeI32Store, 0x2, 0x0,
							wasm.OpcodeEnd,
						},
					},
				},
				Proxied: &wasm.HostFunc{
					Name:        "n",
					ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
					ParamNames:  []string{"x"},
					ResultTypes: []wasm.ValueType{wasm.ValueTypeI32},
					Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeEnd}},
				},
				CallBodyPos: 9,
			},
		},
		{
			name: "i32i32_i64",
			proxied: &wasm.HostFunc{
				ExportNames: []string{"ex"},
				Name:        "n",
				ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
				ParamNames:  []string{"x", "y"},
				ResultTypes: []wasm.ValueType{wasm.ValueTypeI64},
				Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeI64Const, 1, wasm.OpcodeEnd}},
			},
			expected: &wasm.ProxyFunc{
				Proxy: &wasm.HostFunc{
					ExportNames: []string{"ex"},
					Name:        "n.proxy",
					ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
					ParamNames:  []string{"sp"},
					Code: &wasm.Code{
						IsHostFunction: true,
						LocalTypes:     []api.ValueType{api.ValueTypeI32, api.ValueTypeI64},
						Body: []byte{
							wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 8, wasm.OpcodeI32Add,
							wasm.OpcodeI32Load, 0x2, 0x0,
							wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 16, wasm.OpcodeI32Add,
							wasm.OpcodeI32Load, 0x2, 0x0,
							wasm.OpcodeCall, 0,
							wasm.OpcodeLocalSet, 2,
							wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 24, wasm.OpcodeI32Add,
							wasm.OpcodeLocalGet, 2,
							wasm.OpcodeI64Store, 0x3, 0x0,
							wasm.OpcodeEnd,
						},
					},
				},
				Proxied: &wasm.HostFunc{
					Name:        "n",
					ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
					ParamNames:  []string{"x", "y"},
					ResultTypes: []wasm.ValueType{wasm.ValueTypeI64},
					Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeI64Const, 1, wasm.OpcodeEnd}},
				},
				CallBodyPos: 17,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			proxy, err := callFromSP(tc.expectSP, tc.proxied)
			if tc.expectedErr != "" {
				require.EqualError(t, err, tc.expectedErr)
			} else {
				require.Equal(t, tc.expected, proxy)
			}
		})
	}
}

var spMem = []byte{
	0, 0, 0, 0, 0, 0, 0, 0,
	1, 0, 0, 0, 0, 0, 0, 0,
	2, 0, 0, 0, 0, 0, 0, 0,
	3, 0, 0, 0, 0, 0, 0, 0,
	4, 0, 0, 0, 0, 0, 0, 0,
	5, 0, 0, 0, 0, 0, 0, 0,
	6, 0, 0, 0, 0, 0, 0, 0,
	7, 0, 0, 0, 0, 0, 0, 0,
	8, 0, 0, 0, 0, 0, 0, 0,
	9, 0, 0, 0, 0, 0, 0, 0,
	10, 0, 0, 0, 0, 0, 0, 0,
}

func i64i32i32i32i32_i64i32_withSP(vRef uint64, mAddr, mLen, argsArray, argsLen uint32) (xRef uint64, ok uint32, sp uint32) {
	if vRef != 1 {
		panic("vRef")
	}
	if mAddr != 2 {
		panic("mAddr")
	}
	if mLen != 3 {
		panic("mLen")
	}
	if argsArray != 4 {
		panic("argsArray")
	}
	if argsLen != 5 {
		panic("argsLen")
	}
	return 10, 20, 8
}

func TestMustCallFromSP(t *testing.T) {
	r := wazero.NewRuntimeWithConfig(testCtx, wazero.NewRuntimeConfigInterpreter())
	defer r.Close(testCtx)

	funcName := "i64i32i32i32i32_i64i32_withSP"
	im, err := r.NewModuleBuilder("go").
		ExportFunction(funcName, MustCallFromSP(true, wasm.NewGoFunc(
			funcName, funcName,
			[]string{"v", "mAddr", "mLen", "argsArray", "argsLen"},
			i64i32i32i32i32_i64i32_withSP))).
		Instantiate(testCtx, r)
	require.NoError(t, err)

	callDef := im.ExportedFunction(funcName).Definition()

	bin := binary.EncodeModule(&wasm.Module{
		TypeSection: []*wasm.FunctionType{{
			Params:  callDef.ParamTypes(),
			Results: callDef.ResultTypes(),
		}},
		ImportSection:   []*wasm.Import{{Module: "go", Name: funcName}},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: 1},
		FunctionSection: []wasm.Index{0},
		CodeSection: []*wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, 0, wasm.OpcodeEnd}},
		},
		ExportSection: []*wasm.Export{{Name: funcName, Index: 1}},
	})
	require.NoError(t, err)

	mod, err := r.InstantiateModuleFromBinary(testCtx, bin)
	require.NoError(t, err)

	memView, ok := mod.Memory().Read(testCtx, 0, uint32(len(spMem)))
	require.True(t, ok)
	copy(memView, spMem)

	_, err = mod.ExportedFunction(funcName).Call(testCtx, 0) // SP
	require.NoError(t, err)
	require.Equal(t, []byte{
		7, 0, 0, 0, 0, 0, 0, 0, // 7 left alone as SP was refreshed to 8
		10, 0, 0, 0, 0, 0, 0, 0,
		20, 0, 0, 0, 0, 0, 0, 0,
		10, 0, 0, 0, 0, 0, 0, 0,
	}, memView[56:])
}
