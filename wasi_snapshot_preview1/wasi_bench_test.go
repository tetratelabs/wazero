package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var testMem = &wasm.MemoryInstance{
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
}

func Test_Benchmark_EnvironGet(t *testing.T) {
	sysCtx, err := newSysContext(nil, []string{"a=b", "b=cd"}, nil)
	require.NoError(t, err)

	mod := newModule(make([]byte, 20), sysCtx)
	environGet := (&wasi{}).EnvironGet

	require.Equal(t, ErrnoSuccess, environGet(testCtx, mod, 11, 1))
	require.Equal(t, mod.Memory(), testMem)
}

func Benchmark_EnvironGet(b *testing.B) {
	sysCtx, err := newSysContext(nil, []string{"a=b", "b=cd"}, nil)
	if err != nil {
		b.Fatal(err)
	}

	mod := newModule([]byte{
		0,                // environBuf is after this
		'a', '=', 'b', 0, // null terminated "a=b",
		'b', '=', 'c', 'd', 0, // null terminated "b=cd"
		0,          // environ is after this
		1, 0, 0, 0, // little endian-encoded offset of "a=b"
		5, 0, 0, 0, // little endian-encoded offset of "b=cd"
		0,
	}, sysCtx)

	environGet := (&wasi{}).EnvironGet
	b.Run("EnvironGet", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if environGet(testCtx, mod, 0, 4) != ErrnoSuccess {
				b.Fatal()
			}
		}
	})
}

func newModule(buf []byte, sys *sys.Context) *wasm.CallContext {
	return wasm.NewCallContext(nil, &wasm.ModuleInstance{
		Memory: &wasm.MemoryInstance{Min: 1, Buffer: buf},
	}, sys)
}
