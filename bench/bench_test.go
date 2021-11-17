package bench

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/naivevm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func BenchmarkEngines(b *testing.B) {
	buf, _ := os.ReadFile("case/case.wasm")
	b.Run("naivevm", func(b *testing.B) {
		store := newStore(naivevm.NewEngine())
		mod, err := wasm.DecodeModule((buf))
		if err != nil {
			panic(err)
		}
		err = store.Instantiate(mod, "test")
		if err != nil {
			panic(err)
		}
		runBase64Benches(b, store)
		runFibBenches(b, store)
		runStringsManipulationBenches(b, store)
		runReverseArrayBenches(b, store)
		runRandomMatMul(b, store)
	})
	b.Run("wazeroir", func(b *testing.B) {
		store := newStore(wazeroir.NewEngine())
		mod, err := wasm.DecodeModule((buf))
		if err != nil {
			panic(err)
		}
		err = store.Instantiate(mod, "test")
		if err != nil {
			panic(err)
		}
		runBase64Benches(b, store)
		runFibBenches(b, store)
		runStringsManipulationBenches(b, store)
		runReverseArrayBenches(b, store)
		runRandomMatMul(b, store)
	})
}

func runBase64Benches(b *testing.B, store *wasm.Store) {
	for _, numPerExec := range []int{5, 100, 10000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("base64_%d_per_exec", numPerExec), func(b *testing.B) {
			_, _, err := store.CallFunction("test", "base64", uint64(numPerExec))
			if err != nil {
				panic(err)
			}
		})
	}
}

func runFibBenches(b *testing.B, store *wasm.Store) {
	for _, num := range []int{5, 10, 20} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("fibofor_%d", num), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction("test", "fibonacci", uint64(num))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func runStringsManipulationBenches(b *testing.B, store *wasm.Store) {
	for _, initialSize := range []int{50, 100, 1000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("string_manipulation_size_%d", initialSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction("test", "string_manipulation", uint64(initialSize))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func runReverseArrayBenches(b *testing.B, store *wasm.Store) {
	for _, arraySize := range []int{500, 1000, 10000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("reverse_array_size_%d", arraySize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction("test", "reverse_array", uint64(arraySize))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func runRandomMatMul(b *testing.B, store *wasm.Store) {
	for _, matrixSize := range []int{5, 10, 100} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("random_mat_mul_size_%d", matrixSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction("test", "random_mat_mul", uint64(matrixSize))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func newStore(engine wasm.Engine) *wasm.Store {
	store := wasm.NewStore(engine)
	getRandomString := func(ctx *wasm.HostFunctionCallContext, retBufPtr uint32, retBufSize uint32) {
		ret, _, _ := store.CallFunction("test", "allocate_buffer", 10)
		bufAddr := ret[0]
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[retBufPtr:], uint32(bufAddr))
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[retBufSize:], 10)
		_, _ = rand.Read(ctx.Memory.Buffer[bufAddr : bufAddr+10])
	}

	_ = store.AddHostFunction("env", "get_random_string", reflect.ValueOf(getRandomString))
	_ = wasi.NewEnvironment().Register(store)
	return store
}
