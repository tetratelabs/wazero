package guardsig_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/guardsig"
	"github.com/tetratelabs/wazero/internal/testing/binaryencoding"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

// testModule returns a module with a 1-page (max 2) memory exporting:
//   - "load8" (param addr i32) (result i32): i32.load8_u
//   - "load8drop" (param addr i32): i32.load8_u + drop (must still trap)
//   - "grow" (param pages i32) (result i32): memory.grow
func testModule() []byte {
	two := uint32(2)
	m := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
			{Params: []wasm.ValueType{wasm.ValueTypeI32}},
		},
		FunctionSection: []wasm.Index{0, 1, 0},
		MemorySection:   &wasm.Memory{Min: 1, Cap: 1, Max: two, IsMaxEncoded: true},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Load8U, 0, 0, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Load8U, 0, 0, wasm.OpcodeDrop, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeMemoryGrow, 0, wasm.OpcodeEnd}},
		},
		ExportSection: []wasm.Export{
			{Name: "load8", Type: wasm.ExternTypeFunc, Index: 0},
			{Name: "load8drop", Type: wasm.ExternTypeFunc, Index: 1},
			{Name: "grow", Type: wasm.ExternTypeFunc, Index: 2},
		},
	}
	return binaryencoding.EncodeModule(m)
}

func TestGuardPageMemoryTraps(t *testing.T) {
	if !guardsig.Supported() {
		t.Skip("guard-fault handler unavailable")
	}
	ctx := experimental.WithGuardPageMemory(context.Background())
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	defer r.Close(ctx)

	mod, err := r.Instantiate(ctx, testModule())
	require.NoError(t, err)

	load8 := mod.ExportedFunction("load8")

	// In bounds.
	res, err := load8.Call(ctx, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), res[0])
	_, err = load8.Call(ctx, 65535)
	require.NoError(t, err)

	// Out of bounds: first byte past the memory, and far beyond.
	for _, addr := range []uint64{65536, 65537, 100000, 1 << 20, 0xffffffff} {
		_, err = load8.Call(ctx, addr)
		require.True(t, errors.Is(err, wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess), "addr=%d err=%v", addr, err)
	}

	// A load whose result is dropped must still trap.
	_, err = mod.ExportedFunction("load8drop").Call(ctx, 65536)
	require.True(t, errors.Is(err, wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess), "dropped load err=%v", err)

	// Growing commits more of the reservation; the boundary moves.
	res, err = mod.ExportedFunction("grow").Call(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), res[0])
	_, err = load8.Call(ctx, 65536) // now in bounds
	require.NoError(t, err)
	_, err = load8.Call(ctx, 131072) // new boundary
	require.True(t, errors.Is(err, wasmruntime.ErrRuntimeOutOfBoundsMemoryAccess), "after grow err=%v", err)

	// The engine still works for subsequent calls after many traps.
	res, err = load8.Call(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(0), res[0])
}
