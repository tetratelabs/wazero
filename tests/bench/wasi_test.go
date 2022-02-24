package bench

import (
	"testing"

	"github.com/stretchr/testify/require"

	internalwasi "github.com/tetratelabs/wazero/internal/wasi"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/wasi"
)

var environGetMem = []byte{
	0,                // environBuf is after this
	'a', '=', 'b', 0, // null terminated "a=b",
	'b', '=', 'c', 'd', 0, // null terminated "b=cd"
	0,          // environ is after this
	1, 0, 0, 0, // little endian-encoded offset of "a=b"
	5, 0, 0, 0, // little endian-encoded offset of "b=cd"
	0,
}

func Test_EnvironGet(t *testing.T) {
	envOpt, err := internalwasi.Environ("a=b", "b=cd")
	require.NoError(t, err)

	var mem = &wasm.MemoryInstance{Buffer: make([]byte, 20), Min: 1}
	ctx := (&wasm.ModuleContext{}).WithMemory(mem)
	environGet := internalwasi.NewAPI(envOpt).EnvironGet

	require.Equal(t, wasi.ErrnoSuccess, environGet(ctx, 11, 1))
	require.Equal(t, environGetMem, mem.Buffer)
}

func Benchmark_EnvironGet(b *testing.B) {
	envOpt, err := internalwasi.Environ("a=b", "b=cd")
	if err != nil {
		b.Fatal(err)
	}

	var mem = &wasm.MemoryInstance{Buffer: environGetMem, Min: 1}
	ctx := (&wasm.ModuleContext{}).WithMemory(mem)
	environGet := internalwasi.NewAPI(envOpt).EnvironGet

	b.Run("EnvironGet", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if environGet(ctx, 0, 4) != wasi.ErrnoSuccess {
				b.Fatal()
			}
		}
	})
}
