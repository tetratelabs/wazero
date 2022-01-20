package bench

import (
	"os"
	"testing"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm"
	binaryFormat "github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/jit"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

// TestFacIter ensures that the code in BenchmarkFacIter works as expected.
func TestFacIter(t *testing.T) {
	const in = 30
	expValue := uint64(0x865df5dd54000000)
	t.Run("iter", func(t *testing.T) {
		store := newStoreForFacIterBench(jit.NewEngine())
		for i := 0; i < 10000; i++ {
			res, _, err := store.CallFunction("test", "fac-iter", in)
			if err != nil {
				panic(err)
			}
			require.Equal(t, expValue, res[0])
		}
	})

	t.Run("jit", func(t *testing.T) {
		store := newStoreForFacIterBench(jit.NewEngine())
		for i := 0; i < 10000; i++ {
			res, _, err := store.CallFunction("test", "fac-iter", in)
			if err != nil {
				panic(err)
			}
			require.Equal(t, expValue, res[0])
		}
	})
	t.Run("wasmtime-go", func(t *testing.T) {
		store, run := newWasmtimeForFacIterBench()
		for i := 0; i < 10000; i++ {
			res, err := run.Call(store, in)
			if err != nil {
				panic(err)
			}
			require.Equal(t, int64(expValue), res)
		}
	})
}

// Benchmarks on the interative factorial calculation.
func BenchmarkFacIter(b *testing.B) {
	const in = 30
	b.Run("wazeroir", func(b *testing.B) {
		store := newStoreForFacIterBench(wazeroir.NewEngine())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := store.CallFunction("test", "fac-iter", in)
			if err != nil {
				panic(err)
			}
		}
	})
	b.Run("jit", func(b *testing.B) {
		store := newStoreForFacIterBench(jit.NewEngine())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, err := store.CallFunction("test", "fac-iter", in)
			if err != nil {
				panic(err)
			}
		}
	})
	b.Run("wasmtime-go", func(b *testing.B) {
		store, run := newWasmtimeForFacIterBench()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := run.Call(store, in)
			if err != nil {
				panic(err)
			}
		}
	})
}

func newStoreForFacIterBench(engine wasm.Engine) *wasm.Store {
	store := wasm.NewStore(engine)
	buf, err := os.ReadFile("testdata/fac.wasm")
	if err != nil {
		panic(err)
	}
	mod, err := binaryFormat.DecodeModule(buf)
	if err != nil {
		panic(err)
	}
	err = store.Instantiate(mod, "test")
	if err != nil {
		panic(err)
	}
	return store
}

func newWasmtimeForFacIterBench() (*wasmtime.Store, *wasmtime.Func) {
	buf, err := os.ReadFile("testdata/fac.wasm")
	if err != nil {
		panic(err)
	}
	store := wasmtime.NewStore(wasmtime.NewEngine())
	module, err := wasmtime.NewModule(store.Engine, buf)
	if err != nil {
		panic(err)
	}

	instance, err := wasmtime.NewInstance(store, module, nil)
	if err != nil {
		panic(err)
	}

	run := instance.GetFunc(store, "fac-iter")
	if run == nil {
		panic("not a function")
	}
	return store, run
}
