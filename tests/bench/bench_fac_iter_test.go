//go:build amd64
// +build amd64

// Wasmtime cannot be used non-amd64 platform.
package bench

import (
	_ "embed"
	"errors"
	"testing"

	"github.com/bytecodealliance/wasmtime-go"
	"github.com/stretchr/testify/require"
	"github.com/wasmerio/wasmer-go/wasmer"

	"github.com/tetratelabs/wazero/wasm"
	binaryFormat "github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/jit"
)

// facWasm is compiled from testdata/fac.wat
//go:embed testdata/fac.wasm
var facWasm []byte

// TestFacIter ensures that the code in BenchmarkFacIter works as expected.
func TestFacIter(t *testing.T) {
	const in = 30
	expValue := uint64(0x865df5dd54000000)
	t.Run("iter", func(t *testing.T) {
		store, err := newStoreForFacIterBench(jit.NewEngine())
		require.NoError(t, err)

		for i := 0; i < 10000; i++ {
			res, _, err := store.CallFunction("test", "fac-iter", in)
			require.NoError(t, err)
			require.Equal(t, expValue, res[0])
		}
	})

	t.Run("jit", func(t *testing.T) {
		store, err := newStoreForFacIterBench(jit.NewEngine())
		require.NoError(t, err)

		for i := 0; i < 10000; i++ {
			res, _, err := store.CallFunction("test", "fac-iter", in)
			require.NoError(t, err)
			require.Equal(t, expValue, res[0])
		}
	})

	t.Run("wasmer-go", func(t *testing.T) {
		store, instance, fn, err := newWasmerForFacIterBench()
		require.NoError(t, err)
		defer store.Close()
		defer instance.Close()

		for i := 0; i < 10000; i++ {
			res, err := fn(in)
			require.NoError(t, err)
			require.Equal(t, int64(expValue), res)
		}
	})

	t.Run("wasmtime-go", func(t *testing.T) {
		store, run, err := newWasmtimeForFacIterBench()
		require.NoError(t, err)
		for i := 0; i < 10000; i++ {
			res, err := run.Call(store, in)
			if err != nil {
				panic(err)
			}
			require.Equal(t, int64(expValue), res)
		}
	})
}

// BenchmarkFacIter_Init tracks the time spent readying a function for use
func BenchmarkFacIter_Init(b *testing.B) {
	b.Run("interpreter", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := newStoreForFacIterBench(interpreter.NewEngine()); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("jit", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := newStoreForFacIterBench(jit.NewEngine()); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("wasmer-go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			store, instance, _, err := newWasmerForFacIterBench()
			if err != nil {
				b.Fatal(err)
			}
			store.Close()
			instance.Close()
		}
	})
	b.Run("wasmtime-go", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, _, err := newWasmtimeForFacIterBench(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkFacIter_Invoke benchmarks the time spent invoking a factorial calculation.
func BenchmarkFacIter_Invoke(b *testing.B) {
	const in = 30
	b.Run("interpreter", func(b *testing.B) {
		store, err := newStoreForFacIterBench(interpreter.NewEngine())
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, _, err = store.CallFunction("test", "fac-iter", in); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("jit", func(b *testing.B) {
		store, err := newStoreForFacIterBench(jit.NewEngine())
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, _, err = store.CallFunction("test", "fac-iter", in); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("wasmer-go", func(b *testing.B) {
		store, instance, fn, err := newWasmerForFacIterBench()
		if err != nil {
			b.Fatal(err)
		}
		defer store.Close()
		defer instance.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err = fn(in); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("wasmtime-go", func(b *testing.B) {
		store, run, err := newWasmtimeForFacIterBench()
		if err != nil {
			b.Fatal(err)
		}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err = run.Call(store, in); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func newStoreForFacIterBench(engine wasm.Engine) (*wasm.Store, error) {
	store := wasm.NewStore(engine)
	mod, err := binaryFormat.DecodeModule(facWasm)
	if err != nil {
		return nil, err
	}
	err = store.Instantiate(mod, "test")
	if err != nil {
		return nil, err
	}
	return store, nil
}

// newWasmerForFacIterBench returns the store and instance that scope the factorial function.
// Note: these should be closed
func newWasmerForFacIterBench() (*wasmer.Store, *wasmer.Instance, wasmer.NativeFunction, error) {
	store := wasmer.NewStore(wasmer.NewEngine())
	importObject := wasmer.NewImportObject()
	module, err := wasmer.NewModule(store, facWasm)
	if err != nil {
		return nil, nil, nil, err
	}
	instance, err := wasmer.NewInstance(module, importObject)
	if err != nil {
		return nil, nil, nil, err
	}
	f, err := instance.Exports.GetFunction("fac-iter")
	if err != nil {
		return nil, nil, nil, err
	}
	if f == nil {
		return nil, nil, nil, errors.New("not a function")
	}
	return store, instance, f, nil
}

func newWasmtimeForFacIterBench() (*wasmtime.Store, *wasmtime.Func, error) {
	store := wasmtime.NewStore(wasmtime.NewEngine())
	module, err := wasmtime.NewModule(store.Engine, facWasm)
	if err != nil {
		return nil, nil, err
	}

	instance, err := wasmtime.NewInstance(store, module, nil)
	if err != nil {
		return nil, nil, err
	}

	run := instance.GetFunc(store, "fac-iter")
	if run == nil {
		return nil, nil, errors.New("not a function")
	}
	return store, run, nil
}
