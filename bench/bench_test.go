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
	"github.com/tetratelabs/wazero/wasm/v1_0"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func BenchmarkEngines(b *testing.B) {
	b.Run("naivevm", func(b *testing.B) {
		store := newStore(naivevm.NewEngine())
		setUpStore(store)
		if m, ok := v1_0.NewModule(store, "test"); !ok {
			b.Fatal("couldn't find module test in nativevm")
		} else {
			runAllBenches(b, m)
		}
	})
	b.Run("wazeroir", func(b *testing.B) {
		store := newStore(wazeroir.NewEngine())
		setUpStore(store)
		if m, ok := v1_0.NewModule(store, "test"); !ok {
			b.Fatal("couldn't find module test in wazeroir")
		} else {
			runAllBenches(b, m)
		}
	})
}

func setUpStore(store *wasm.Store) {
	buf, err := os.ReadFile("case/case.wasm")
	if err != nil {
		panic(err)
	}
	mod, err := wasm.DecodeModule(buf)
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
	_, _, err = store.CallFunction("test", "_start")
	if err != nil {
		panic(err)
	}
}

func runAllBenches(b *testing.B, module v1_0.Module) {
	runBase64Benches(b, module)
	runFibBenches(b, module)
	runStringsManipulationBenches(b, module)
	runReverseArrayBenches(b, module)
	runRandomMatMul(b, module)
}

func runBase64Benches(b *testing.B, module v1_0.Module) {
	functionName := "base64"
	f, ok := module.FunctionByName(functionName)
	if !ok {
		b.Fatalf("couldn't find function %s in module %s", functionName, module.Name())
	}
	for _, numPerExec := range []int{5, 100, 10000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("base64_%d_per_exec", numPerExec), func(b *testing.B) {
			if _, err := f.Call(uint64(numPerExec)); err != nil {
				panic(err)
			}
		})
	}
}

func runFibBenches(b *testing.B, module v1_0.Module) {
	functionName := "fibonacci"
	f, ok := module.FunctionByName(functionName)
	if !ok {
		b.Fatalf("couldn't find function %s in module %s", functionName, module.Name())
	}
	for _, num := range []int{5, 10, 20} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("fib_for_%d", num), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := f.Call(uint64(num)); err != nil {
					panic(err)
				}
			}
		})
	}
}

func runStringsManipulationBenches(b *testing.B, module v1_0.Module) {
	functionName := "string_manipulation"
	f, ok := module.FunctionByName(functionName)
	if !ok {
		b.Fatalf("couldn't find function %s in module %s", functionName, module.Name())
	}
	for _, initialSize := range []int{50, 100, 1000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("string_manipulation_size_%d", initialSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := f.Call(uint64(initialSize)); err != nil {
					panic(err)
				}
			}
		})
	}
}

func runReverseArrayBenches(b *testing.B, module v1_0.Module) {
	functionName := "reverse_array"
	f, ok := module.FunctionByName(functionName)
	if !ok {
		b.Fatalf("couldn't find function %s in module %s", functionName, module.Name())
	}
	for _, arraySize := range []int{500, 1000, 10000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("reverse_array_size_%d", arraySize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := f.Call(uint64(arraySize)); err != nil {
					panic(err)
				}
			}
		})
	}
}

func runRandomMatMul(b *testing.B, module v1_0.Module) {
	functionName := "random_mat_mul"
	f, ok := module.FunctionByName(functionName)
	if !ok {
		b.Fatalf("couldn't find function %s in module %s", functionName, module.Name())
	}
	for _, matrixSize := range []int{5, 10, 100} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("random_mat_mul_size_%d", matrixSize), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := f.Call(uint64(matrixSize)); err != nil {
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
