package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

var testMem = []byte{
	0,                // environBuf is after this
	'a', '=', 'b', 0, // null terminated "a=b",
	'b', '=', 'c', 'd', 0, // null terminated "b=cd"
	0,          // environ is after this
	1, 0, 0, 0, // little endian-encoded offset of "a=b"
	5, 0, 0, 0, // little endian-encoded offset of "b=cd"
	0,
}

func Test_Benchmark_EnvironGet(t *testing.T) {
	mod, r, log := requireProxyModule(t, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	defer r.Close(testCtx)

	// Invoke environGet and check the memory side effects.
	requireErrno(t, ErrnoSuccess, mod, functionEnvironGet, uint64(11), uint64(1))
	require.Equal(t, `
--> proxy.environ_get(environ=11,environ_buf=1)
	==> wasi_snapshot_preview1.environ_get(environ=11,environ_buf=1)
	<== ESUCCESS
<-- (0)
`, "\n"+log.String())

	mem, ok := mod.Memory().Read(testCtx, 0, uint32(len(testMem)))
	require.True(t, ok)
	require.Equal(t, testMem, mem)
}

func Benchmark_EnvironGet(b *testing.B) {
	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().
		WithEnv("a", "b").WithEnv("b", "cd"))
	if err != nil {
		b.Fatal(err)
	}

	b.Run("environGet", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			results, err := mod.ExportedFunction(functionEnvironGet).Call(testCtx, uint64(0), uint64(4))
			if err != nil {
				b.Fatal(err)
			}
			errno := Errno(results[0])
			if errno != ErrnoSuccess {
				b.Fatal(ErrnoName(errno))
			}
		}
	})
}

// instantiateProxyModule instantiates a guest that re-exports WASI functions.
func instantiateProxyModule(r wazero.Runtime, config wazero.ModuleConfig) (api.Module, error) {
	wasiModuleCompiled, err := (&builder{r}).hostModuleBuilder().Compile(testCtx)
	if err != nil {
		return nil, err
	}

	if _, err = r.InstantiateModule(testCtx, wasiModuleCompiled, wazero.NewModuleConfig()); err != nil {
		return nil, err
	}

	proxyBin := proxy.GetProxyModuleBinary(ModuleName, wasiModuleCompiled)

	proxyCompiled, err := r.CompileModule(testCtx, proxyBin)
	if err != nil {
		return nil, err
	}

	return r.InstantiateModule(testCtx, proxyCompiled, config)
}
