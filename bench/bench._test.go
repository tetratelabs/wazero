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
	mod, _ := wasm.DecodeModule((buf))
	b.Run("naivevm", func(b *testing.B) {
		store := newStore(naivevm.NewEngine())
		_ = store.Instantiate(mod, "test")
		runBase64Benches(b, store)
		runFibBenches(b, store)
	})
	b.Run("wazeroir_interpreter", func(b *testing.B) {
		store := newStore(wazeroir.NewEngine())
		_ = store.Instantiate(mod, "test")
		runBase64Benches(b, store)
		runFibBenches(b, store)
	})
}

func runBase64Benches(b *testing.B, store *wasm.Store) {
	for _, numPerExec := range []int{5, 100, 10000} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("base64_%d_per_exec", numPerExec), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, _ = store.CallFunction("test", "base64", uint64(numPerExec))
			}
		})
	}
}

func runFibBenches(b *testing.B, store *wasm.Store) {
	for _, num := range []int{5, 10} {
		b.ResetTimer()
		b.Run(fmt.Sprintf("fibo_for_%d", num), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _, _ = store.CallFunction("test", "fibonacci", uint64(num))
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
