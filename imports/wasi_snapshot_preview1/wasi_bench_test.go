package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/sys"
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

type writerFunc func(p []byte) (n int, err error)

func (f writerFunc) Write(p []byte) (n int, err error) {
	return f(p)
}

func Benchmark_fdWrite(b *testing.B) {
	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().
		WithStdout(writerFunc(func(p []byte) (n int, err error) { return len(p), nil })),
	)
	if err != nil {
		b.Fatal(err)
	}
	fn := mod.ExportedFunction(fdWriteName)

	iovs := uint32(1) // arbitrary offset
	mod.Memory().Write(testCtx, 0, []byte{
		'?',         // `iovs` is after this
		18, 0, 0, 0, // = iovs[0].offset
		4, 0, 0, 0, // = iovs[0].length
		23, 0, 0, 0, // = iovs[1].offset
		2, 0, 0, 0, // = iovs[1].length
		'?',                // iovs[0].offset is after this
		'w', 'a', 'z', 'e', // iovs[0].length bytes
		'?',      // iovs[1].offset is after this
		'r', 'o', // iovs[1].length bytes
		'?',
	})

	iovsCount := uint32(2)       // The count of iovs
	resultNwritten := uint32(26) // arbitrary offset

	benches := []struct {
		name string
		fd   uint32
	}{
		{
			name: "io.Writer",
			fd:   sys.FdStdout,
		},
		{
			name: "io.Discard",
			fd:   sys.FdStderr,
		},
	}

	for _, bb := range benches {
		bc := bb

		b.ReportAllocs()
		b.Run(bc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				results, err := fn.Call(testCtx, uint64(bc.fd), uint64(iovs), uint64(iovsCount), uint64(resultNwritten))
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
