package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
)

// configArgsEnviron ensures the result data are the same between args and ENV.
var configArgsEnviron = wazero.NewModuleConfig().
	WithArgs("aa=bbbb", "cccccc=dddddddd", "eeeeeeeeee=ffffffffffff").
	WithEnv("aa", "bbbb").
	WithEnv("cccccc", "dddddddd").
	WithEnv("eeeeeeeeee", "ffffffffffff")

func Benchmark_ArgsEnviron(b *testing.B) {
	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	mod, err := instantiateProxyModule(r, configArgsEnviron)
	if err != nil {
		b.Fatal(err)
	}

	for _, n := range []string{
		argsGetName,
		argsSizesGetName,
		environGetName,
		environSizesGetName,
	} {
		n := n
		fn := mod.ExportedFunction(n)
		b.Run(n, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				results, err := fn.Call(testCtx, uint64(0), uint64(4))
				if err != nil {
					b.Fatal(err)
				}
				errno := Errno(results[0])
				if errno != 0 {
					b.Fatal(ErrnoName(errno))
				}
			}
		})
	}
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
