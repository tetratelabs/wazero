package bench

import (
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/wasi"
	"github.com/tetratelabs/wazero/wasm"
	binaryFormat "github.com/tetratelabs/wazero/wasm/binary"
	"github.com/tetratelabs/wazero/wasm/interpreter"
	"github.com/tetratelabs/wazero/wasm/jit"
)

//go:embed testdata/case.wasm
var caseWasm []byte

func BenchmarkEngines(b *testing.B) {
	b.Run("wazeroir", func(b *testing.B) {
		store := newStore(interpreter.NewEngine())
		setUpStore(store)
		runAllBenches(b, store)
	})
	if runtime.GOARCH == "amd64" {
		b.Run("jit", func(b *testing.B) {
			store := newStore(jit.NewEngine())
			setUpStore(store)
			runAllBenches(b, store)
		})
	}
}

func setUpStore(store *wasm.Store) {
	mod, err := binaryFormat.DecodeModule(caseWasm)
	if err != nil {
		panic(err)
	}
	err = store.Instantiate(mod, "test")
	if err != nil {
		panic(err)
	}

	// We assume that TinyGo binary expose "_start" symbol
	// to initialize the memory state.
	// Meaning that TinyGo binary is "WASI command":
	// https://github.com/WebAssembly/WASI/blob/main/design/application-abi.md
	_, _, err = store.CallFunction(context.Background(), "test", "_start")
	if err != nil {
		panic(err)
	}
}

func runAllBenches(b *testing.B, store *wasm.Store) {
	runBase64Benches(b, store)
	runFibBenches(b, store)
	runStringsManipulationBenches(b, store)
	runReverseArrayBenches(b, store)
	runRandomMatMul(b, store)
}

func runBase64Benches(b *testing.B, store *wasm.Store) {
	ctx := context.Background()
	for _, numPerExec := range []int{5, 100, 10000} {
		numPerExec := numPerExec
		b.ResetTimer()
		b.Run(fmt.Sprintf("base64_%d_per_exec", numPerExec), func(b *testing.B) {
			_, _, err := store.CallFunction(ctx, "test", "base64", uint64(numPerExec))
			if err != nil {
				panic(err)
			}
		})
	}
}

func runFibBenches(b *testing.B, store *wasm.Store) {
	ctx := context.Background()
	for _, num := range []int{5, 10, 20, 30} {
		num := num
		b.ResetTimer()
		b.Run(fmt.Sprintf("fib_for_%d", num), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction(ctx, "test", "fibonacci", uint64(num))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func runStringsManipulationBenches(b *testing.B, store *wasm.Store) {
	ctx := context.Background()
	for _, initialSize := range []int{50, 100, 1000} {
		initialSize := initialSize
		b.ResetTimer()
		b.Run(fmt.Sprintf("string_manipulation_size_%d", initialSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction(ctx, "test", "string_manipulation", uint64(initialSize))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func runReverseArrayBenches(b *testing.B, store *wasm.Store) {
	ctx := context.Background()
	for _, arraySize := range []int{500, 1000, 10000} {
		arraySize := arraySize
		b.ResetTimer()
		b.Run(fmt.Sprintf("reverse_array_size_%d", arraySize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction(ctx, "test", "reverse_array", uint64(arraySize))
				if err != nil {
					panic(err)
				}
			}
		})
	}
}

func runRandomMatMul(b *testing.B, store *wasm.Store) {
	ctx := context.Background()
	for _, matrixSize := range []int{5, 10, 20} {
		matrixSize := matrixSize
		b.ResetTimer()
		b.Run(fmt.Sprintf("random_mat_mul_size_%d", matrixSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, err := store.CallFunction(ctx, "test", "random_mat_mul", uint64(matrixSize))
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
		ret, _, _ := store.CallFunction(ctx.Context(), "test", "allocate_buffer", 10)
		bufAddr := ret[0]
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[retBufPtr:], uint32(bufAddr))
		binary.LittleEndian.PutUint32(ctx.Memory.Buffer[retBufSize:], 10)
		_, _ = rand.Read(ctx.Memory.Buffer[bufAddr : bufAddr+10])
	}

	_ = store.AddHostFunction("env", "get_random_string", reflect.ValueOf(getRandomString))
	_ = wasi.NewEnvironment().Register(store)
	return store
}
