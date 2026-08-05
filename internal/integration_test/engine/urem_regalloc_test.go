package adhoc

import (
	"context"
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/urem_regalloc.wasm
var uremRegallocWasm []byte

// TestUremRegalloc is a regression test for an ARM64 compiler bug where the
// register allocator inserts a spill/reload between the udiv and msub
// instructions that implement i32.rem_u, clobbering the quotient register.
// This causes the remainder to be computed with a stale value, producing
// a wrong result that triggers a spurious bounds-check unreachable trap.
func TestUremRegalloc(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig())
	defer r.Close(ctx)

	_, err := r.NewHostModuleBuilder("repro").
		NewFunctionBuilder().WithFunc(func(context.Context, uint32) {}).
		Export("update_nonce").
		Instantiate(ctx)
	require.NoError(t, err)

	mod, err := r.Instantiate(ctx, uremRegallocWasm)
	require.NoError(t, err)
	defer mod.Close(ctx)

	_, ok := mod.Memory().Grow(300)
	require.True(t, ok)

	self := make([]byte, 32)
	binary.LittleEndian.PutUint32(self[8:], 8)
	binary.LittleEndian.PutUint32(self[12:], 2)
	binary.LittleEndian.PutUint32(self[16:], 1)
	ok = mod.Memory().Write(0x100000, self)
	require.True(t, ok)

	mod.ExportedGlobal("__stack_pointer").(api.MutableGlobal).Set(0xFFF00)

	result, err := mod.ExportedFunction("fill_blocks").Call(ctx, 0x100000, 0x120000, 8, 0xFFF00)
	require.NoError(t, err)
	require.Equal(t, uint64(18), result[0])
}
