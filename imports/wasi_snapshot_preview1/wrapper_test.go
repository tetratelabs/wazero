package wasi_snapshot_preview1

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func Test_proxyResultParams(t *testing.T) {
	tests := []struct {
		name       string
		goFunc     *wasm.HostFunc
		exportName string
		expected   *wasm.ProxyFunc
	}{
		{
			name: "v_i32i32",
			goFunc: &wasm.HostFunc{
				ResultTypes: []wasm.ValueType{i32, i32},
				ResultNames: []string{"r1", "errno"},
				Code: &wasm.Code{IsHostFunction: true, Body: []byte{
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeI32Const, byte(ErrnoSuccess),
					wasm.OpcodeEnd,
				}},
			},
			exportName: "export",
			expected: &wasm.ProxyFunc{
				Proxy: &wasm.HostFunc{
					ExportNames: []string{"export"},
					Name:        "export",
					ParamTypes:  []wasm.ValueType{i32},
					ParamNames:  []string{"result.r1"},
					ResultTypes: []wasm.ValueType{i32},
					ResultNames: []string{"errno"},
					Code: &wasm.Code{
						IsHostFunction: true,
						LocalTypes:     []wasm.ValueType{i32},
						Body: []byte{
							// store memSizeBytes in a temp local and leave it on the stack.
							wasm.OpcodeMemorySize, 0, // size of memory[0]
							wasm.OpcodeI32Const, 0x80, 0x80, 0x04, // leb128 MemoryPageSize
							wasm.OpcodeI32Mul,
							wasm.OpcodeLocalTee, 1,

							// EFAULT if memSizeBytes-4 < result.r1; unsigned
							wasm.OpcodeI32Const, 4, wasm.OpcodeI32Sub,
							wasm.OpcodeLocalGet, 0,
							wasm.OpcodeI32LtU, wasm.OpcodeIf, nilType,
							wasm.OpcodeI32Const, fault,
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// call the proxied goFunc without parameters
							wasm.OpcodeCall, fakeFuncIdx,

							// return the errno if not ESUCCESS
							wasm.OpcodeLocalTee, 1, wasm.OpcodeI32Const, success,
							wasm.OpcodeI32Ne, wasm.OpcodeIf, nilType,
							wasm.OpcodeLocalGet, 1, // errno
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// store r1 at the memory offset in result.r1
							wasm.OpcodeLocalSet, 1,
							wasm.OpcodeLocalGet, 0, // result.r1
							wasm.OpcodeLocalGet, 1, // r1
							wasm.OpcodeI32Store, 0x2, 0x0,

							// return ESUCCESS
							wasm.OpcodeI32Const, success, wasm.OpcodeEnd,
						},
					},
				},
				Proxied: &wasm.HostFunc{
					ResultTypes: []wasm.ValueType{i32, i32},
					ResultNames: []string{"r1", "errno"},
					Code: &wasm.Code{IsHostFunction: true, Body: []byte{
						wasm.OpcodeI32Const, 1,
						wasm.OpcodeI32Const, byte(ErrnoSuccess),
						wasm.OpcodeEnd,
					}},
				},
				CallBodyPos: 22,
			},
		},
		{
			name: "v_i32i64i32",
			goFunc: &wasm.HostFunc{
				ResultTypes: []wasm.ValueType{i32, i64, i32},
				ResultNames: []string{"r1", "r2", "errno"},
				Code: &wasm.Code{IsHostFunction: true, Body: []byte{
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeI64Const, 2,
					wasm.OpcodeI32Const, byte(ErrnoSuccess),
					wasm.OpcodeEnd,
				}},
			},
			exportName: "export",
			expected: &wasm.ProxyFunc{
				Proxy: &wasm.HostFunc{
					ExportNames: []string{"export"},
					Name:        "export",
					ParamTypes:  []wasm.ValueType{i32, i32},
					ParamNames:  []string{"result.r1", "result.r2"},
					ResultTypes: []wasm.ValueType{i32},
					ResultNames: []string{"errno"},
					Code: &wasm.Code{
						IsHostFunction: true,
						LocalTypes:     []wasm.ValueType{i32, i64},
						Body: []byte{
							// store memSizeBytes in a temp local and leave it on the stack.
							wasm.OpcodeMemorySize, 0, // size of memory[0]
							wasm.OpcodeI32Const, 0x80, 0x80, 0x04, // leb128 MemoryPageSize
							wasm.OpcodeI32Mul,
							wasm.OpcodeLocalTee, 2,

							// EFAULT if memSizeBytes-4 < result.r1; unsigned
							wasm.OpcodeI32Const, 4, wasm.OpcodeI32Sub,
							wasm.OpcodeLocalGet, 0,
							wasm.OpcodeI32LtU, wasm.OpcodeIf, nilType,
							wasm.OpcodeI32Const, fault,
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// EFAULT if memSizeBytes-8 < result.r2; unsigned
							wasm.OpcodeLocalGet, 2,
							wasm.OpcodeI32Const, 8, wasm.OpcodeI32Sub,
							wasm.OpcodeLocalGet, 1,
							wasm.OpcodeI32LtU, wasm.OpcodeIf, nilType,
							wasm.OpcodeI32Const, fault,
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// call the proxied goFunc without parameters
							wasm.OpcodeCall, fakeFuncIdx,

							// return the errno if not ESUCCESS
							wasm.OpcodeLocalTee, 2, wasm.OpcodeI32Const, success,
							wasm.OpcodeI32Ne, wasm.OpcodeIf, nilType,
							wasm.OpcodeLocalGet, 2, // errno
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// store r2 at the memory offset in result.r2
							wasm.OpcodeLocalSet, 3,
							wasm.OpcodeLocalGet, 1, // result.r2
							wasm.OpcodeLocalGet, 3, // r2
							wasm.OpcodeI64Store, 0x3, 0x0,

							// store r1 at the memory offset in result.r1
							wasm.OpcodeLocalSet, 2,
							wasm.OpcodeLocalGet, 0, // result.r1
							wasm.OpcodeLocalGet, 2, // r1
							wasm.OpcodeI32Store, 0x2, 0x0,

							// return ESUCCESS
							wasm.OpcodeI32Const, success, wasm.OpcodeEnd,
						},
					},
				},
				Proxied: &wasm.HostFunc{
					ResultTypes: []wasm.ValueType{i32, i64, i32},
					ResultNames: []string{"r1", "r2", "errno"},
					Code: &wasm.Code{IsHostFunction: true, Body: []byte{
						wasm.OpcodeI32Const, 1,
						wasm.OpcodeI64Const, 2,
						wasm.OpcodeI32Const, byte(ErrnoSuccess),
						wasm.OpcodeEnd,
					}},
				},
				CallBodyPos: 36,
			},
		},
		{
			name: "i32_i32i32",
			goFunc: &wasm.HostFunc{
				ParamTypes:  []wasm.ValueType{i32},
				ParamNames:  []string{"p1"},
				ResultTypes: []wasm.ValueType{i32, i32},
				ResultNames: []string{"r1", "errno"},
				Code: &wasm.Code{IsHostFunction: true, Body: []byte{
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeI32Const, byte(ErrnoSuccess),
					wasm.OpcodeEnd,
				}},
			},
			exportName: "export",
			expected: &wasm.ProxyFunc{
				Proxy: &wasm.HostFunc{
					ExportNames: []string{"export"},
					Name:        "export",
					ParamTypes:  []wasm.ValueType{i32, i32},
					ParamNames:  []string{"p1", "result.r1"},
					ResultTypes: []wasm.ValueType{i32},
					ResultNames: []string{"errno"},
					Code: &wasm.Code{
						IsHostFunction: true,
						LocalTypes:     []wasm.ValueType{i32},
						Body: []byte{
							// store memSizeBytes in a temp local and leave it on the stack.
							wasm.OpcodeMemorySize, 0, // size of memory[0]
							wasm.OpcodeI32Const, 0x80, 0x80, 0x04, // leb128 MemoryPageSize
							wasm.OpcodeI32Mul,
							wasm.OpcodeLocalTee, 2,

							// EFAULT if memSizeBytes-4 < result.r1; unsigned
							wasm.OpcodeI32Const, 4, wasm.OpcodeI32Sub,
							wasm.OpcodeLocalGet, 1,
							wasm.OpcodeI32LtU, wasm.OpcodeIf, nilType,
							wasm.OpcodeI32Const, fault,
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// call the proxied goFunc with parameters
							wasm.OpcodeLocalGet, 0, wasm.OpcodeCall, fakeFuncIdx,

							// return the errno if not ESUCCESS
							wasm.OpcodeLocalTee, 2, wasm.OpcodeI32Const, success,
							wasm.OpcodeI32Ne, wasm.OpcodeIf, nilType,
							wasm.OpcodeLocalGet, 2, // errno
							wasm.OpcodeReturn, wasm.OpcodeEnd,

							// store r1 at the memory offset in result.r1
							wasm.OpcodeLocalSet, 2,
							wasm.OpcodeLocalGet, 1, // result.r1
							wasm.OpcodeLocalGet, 2, // r1
							wasm.OpcodeI32Store, 0x2, 0x0,

							// return ESUCCESS
							wasm.OpcodeI32Const, success, wasm.OpcodeEnd,
						},
					},
				},
				Proxied: &wasm.HostFunc{
					ParamTypes:  []wasm.ValueType{i32},
					ParamNames:  []string{"p1"},
					ResultTypes: []wasm.ValueType{i32, i32},
					ResultNames: []string{"r1", "errno"},
					Code: &wasm.Code{IsHostFunction: true, Body: []byte{
						wasm.OpcodeI32Const, 1,
						wasm.OpcodeI32Const, byte(ErrnoSuccess),
						wasm.OpcodeEnd,
					}},
				},
				CallBodyPos: 24,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			proxy := proxyResultParams(tc.goFunc, tc.exportName)
			require.Equal(t, tc.expected, proxy)
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

func i64i32i32i32i32_i64i32_withSP(_ context.Context, stack []uint64) {
	vRef := stack[0]
	mAddr := uint32(stack[1])
	mLen := uint32(stack[2])
	argsArray := uint32(stack[3])
	argsLen := uint32(stack[4])

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

	// set results
	stack[0] = 10
	stack[1] = 20
	stack[2] = 8
}
