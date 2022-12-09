package wasi_snapshot_preview1

import (
	"embed"
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
	"github.com/tetratelabs/wazero/internal/wasm"
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

type money struct{}

// Read implements io.Reader by returning endless '$'.
func (money) Read(b []byte) (n int, err error) {
	for i := range b {
		b[i] = '$'
	}

	return len(b), nil
}

func Benchmark_fdRead(b *testing.B) {
	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().WithStdin(money{}))
	if err != nil {
		b.Fatal(err)
	}
	fn := mod.ExportedFunction(fdReadName)

	mod.Memory().Write(testCtx, 0, []byte{
		32, 0, 0, 0, // = iovs[0].offset
		8, 0, 0, 0, // = iovs[0].length
		40, 0, 0, 0, // = iovs[1].offset
		8, 0, 0, 0, // = iovs[1].length
		48, 0, 0, 0, // = iovs[2].offset
		16, 0, 0, 0, // = iovs[2].length
		64, 0, 0, 0, // = iovs[3].offset
		16, 0, 0, 0, // = iovs[3].length
	})

	benches := []struct {
		name      string
		iovs      uint32
		iovsCount uint32
	}{
		{
			name:      "1x8",
			iovs:      0,
			iovsCount: 1,
		},
		{
			name:      "2x16",
			iovs:      16,
			iovsCount: 2,
		},
	}

	for _, bb := range benches {
		bc := bb

		b.ReportAllocs()
		b.Run(bc.name, func(b *testing.B) {
			resultNread := uint32(128) // arbitrary offset

			for i := 0; i < b.N; i++ {
				results, err := fn.Call(testCtx, uint64(0), uint64(bc.iovs), uint64(bc.iovsCount), uint64(resultNread))
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

//go:embed testdata
var testdata embed.FS

func Benchmark_fdReaddir(b *testing.B) {
	embedFS, err := fs.Sub(testdata, "testdata")
	if err != nil {
		b.Fatal(err)
	}

	benches := []struct {
		name string
		fs   fs.FS
	}{
		{
			name: "embed.FS",
			fs:   embedFS,
		},
		{
			name: "os.DirFS",
			fs:   os.DirFS("testdata"),
		},
	}

	for _, bb := range benches {
		bc := bb

		b.ReportAllocs()
		b.Run(bc.name, func(b *testing.B) {
			r := wazero.NewRuntime(testCtx)
			defer r.Close(testCtx)

			mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().WithFS(bc.fs))
			if err != nil {
				b.Fatal(err)
			}

			fn := mod.ExportedFunction(fdReaddirName)

			// Open the root directory as a file-descriptor.
			fsc := mod.(*wasm.CallContext).Sys.FS(testCtx)
			fd, err := fsc.OpenFile(testCtx, ".")
			if err != nil {
				b.Fatal(err)
			}
			f, ok := fsc.OpenedFile(testCtx, fd)
			if !ok {
				b.Fatal("couldn't open fd ", fd)
			}
			defer fsc.CloseFile(testCtx, fd)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				// Recreate the file under the file-descriptor
				if err = f.File.Close(); err != nil {
					b.Fatal(err)
				}
				if f.File, err = bc.fs.Open("."); err != nil {
					b.Fatal(err)
				}
				f.ReadDir = nil
				b.StartTimer()

				// Execute the benchmark, allowing up to 8KB buffer usage
				results, err := fn.Call(testCtx, uint64(fd), uint64(0), uint64(8096), uint64(0), uint64(16192))
				if err != nil {
					b.Fatal(err)
				}

				if errno := Errno(results[0]); errno != 0 {
					b.Fatal(ErrnoName(errno))
				}
			}
		})
	}
}

type writerFunc func(p []byte) (n int, err error)

// Write implements io.Writer by calling writerFunc.
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
