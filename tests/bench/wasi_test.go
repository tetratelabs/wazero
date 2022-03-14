package bench

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	internalwasi "github.com/tetratelabs/wazero/internal/wasi"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
)

var ctx = wasm.NewModuleContext(context.Background(), nil, &wasm.ModuleInstance{
	Memory: &wasm.MemoryInstance{
		Min: 1,
		Buffer: []byte{
			0,                // environBuf is after this
			'a', '=', 'b', 0, // null terminated "a=b",
			'b', '=', 'c', 'd', 0, // null terminated "b=cd"
			0,          // environ is after this
			1, 0, 0, 0, // little endian-encoded offset of "a=b"
			5, 0, 0, 0, // little endian-encoded offset of "b=cd"
			0,
		},
	},
})

func Test_EnvironGet(t *testing.T) {
	config := internalwasi.NewConfig()
	err := config.Environ("a=b", "b=cd")
	require.NoError(t, err)

	testCtx := newCtx(make([]byte, 20))
	environGet := internalwasi.NewAPI(config).EnvironGet

	require.Equal(t, wasi.ErrnoSuccess, environGet(testCtx, 11, 1))
	require.Equal(t, testCtx.Memory(), ctx.Memory())
}

func Benchmark_EnvironGet(b *testing.B) {
	config := internalwasi.NewConfig()
	err := config.Environ("a=b", "b=cd")
	if err != nil {
		b.Fatal(err)
	}

	environGet := internalwasi.NewAPI(config).EnvironGet
	b.Run("EnvironGet", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if environGet(ctx, 0, 4) != wasi.ErrnoSuccess {
				b.Fatal()
			}
		}
	})
}

func newCtx(buf []byte) *wasm.ModuleContext {
	return wasm.NewModuleContext(context.Background(), nil, &wasm.ModuleInstance{
		Memory: &wasm.MemoryInstance{Min: 1, Buffer: buf},
	})
}
