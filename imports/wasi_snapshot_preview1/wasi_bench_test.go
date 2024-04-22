package wasi_snapshot_preview1_test

import (
	"embed"
	"io/fs"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/sys"
	"github.com/tetratelabs/wazero/internal/testing/proxy"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasip1"
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
		wasip1.ArgsGetName,
		wasip1.ArgsSizesGetName,
		wasip1.EnvironGetName,
		wasip1.EnvironSizesGetName,
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
				requireESuccess(b, results)
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
	fn := mod.ExportedFunction(wasip1.FdReadName)

	mod.Memory().Write(0, []byte{
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
				requireESuccess(b, results)
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
		// dirMount ensures direct use of sys.FS
		dirMount string
		// twoCalls tests performance of reading a directory in two calls.
		twoCalls bool
	}{
		{
			name: "embed.FS",
			fs:   embedFS,
		},
		{
			name:     "embed.FS - two calls",
			fs:       embedFS,
			twoCalls: true,
		},
		{
			name: "os.DirFS",
			fs:   os.DirFS("testdata"),
		},
		{
			name:     "os.DirFS - two calls",
			fs:       os.DirFS("testdata"),
			twoCalls: true,
		},
		{
			name:     "sysfs.DirFS",
			dirMount: "testdata",
		},
		{
			name:     "sysfs.DirFS - two calls",
			dirMount: "testdata",
			twoCalls: true,
		},
	}

	for _, bb := range benches {
		bc := bb

		b.Run(bc.name, func(b *testing.B) {
			r := wazero.NewRuntime(testCtx)
			defer r.Close(testCtx)

			fsConfig := wazero.NewFSConfig()
			if bc.fs != nil {
				fsConfig = fsConfig.WithFSMount(bc.fs, "")
			} else {
				fsConfig = fsConfig.WithDirMount(bc.dirMount, "")
			}

			mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().WithFSConfig(fsConfig))
			if err != nil {
				b.Fatal(err)
			}

			fn := mod.ExportedFunction(wasip1.FdReaddirName)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				cookie := 0        // where to begin (last read d_next)
				resultBufused := 0 // where to write the amount used out of bufLen
				buf := 8           // where to start the dirents
				bufLen := 8096     // allow up to 8KB buffer usage

				// Open the root directory as a file-descriptor.
				fsc := mod.(*wasm.ModuleInstance).Sys.FS()
				preopenFile, ok := fsc.LookupFile(sys.FdPreopen)
				require.True(b, ok)
				preopen := preopenFile.FS
				fd, errno := fsc.OpenFile(preopen, ".", experimentalsys.O_RDONLY, 0)
				if errno != 0 {
					b.Fatal(errno)
				}

				// Time the call to write the dirents
				b.StartTimer()

				if bc.twoCalls {
					// Read the dot entries
					bufLen := wasip1.DirentSize + 1 // size of "."
					bufLen += wasip1.DirentSize + 2 // size of ".."

					results, err := fn.Call(testCtx, uint64(fd), uint64(buf), uint64(bufLen), uint64(cookie), uint64(resultBufused))
					if err != nil {
						b.Fatal(err)
					}
					requireESuccess(b, results)
					cookie = 2 // d_next of "..", the real file we couldn't read.
				}

				results, err := fn.Call(testCtx, uint64(fd), uint64(buf), uint64(bufLen), uint64(cookie), uint64(resultBufused))
				if err != nil {
					b.Fatal(err)
				}
				b.StopTimer()

				requireESuccess(b, results)
				if errno = fsc.CloseFile(fd); errno != 0 {
					b.Fatal(errno)
				}
			}
		})
	}
}

func Benchmark_pathFilestat(b *testing.B) {
	embedFS, err := fs.Sub(testdata, "testdata")
	if err != nil {
		b.Fatal(err)
	}

	benches := []struct {
		name string
		fs   fs.FS
		// dirMount ensures direct use of sys.FS
		dirMount string
		path     string
		fd       int32
	}{
		{
			name: "embed.FS fd=root",
			fs:   embedFS,
			path: "zig",
			fd:   sys.FdPreopen,
		},
		{
			name: "embed.FS fd=directory",
			fs:   embedFS,
			path:/* zig */ "wasi.zig",
		},
		{
			name: "os.DirFS fd=root",
			fs:   os.DirFS("testdata"),
			path: "zig",
			fd:   sys.FdPreopen,
		},
		{
			name: "os.DirFS fd=directory",
			fs:   os.DirFS("testdata"),
			path:/* zig */ "wasi.zig",
		},
		{
			name:     "sysfs.DirFS fd=root",
			dirMount: "testdata",
			path:     "zig",
			fd:       sys.FdPreopen,
		},
		{
			name:     "sysfs.DirFS fd=directory",
			dirMount: "testdata",
			path:/* zig */ "wasi.zig",
		},
	}

	for _, bb := range benches {
		bc := bb

		b.Run(bc.name, func(b *testing.B) {
			r := wazero.NewRuntime(testCtx)
			defer r.Close(testCtx)

			fsConfig := wazero.NewFSConfig()
			if bc.fs != nil {
				fsConfig = fsConfig.WithFSMount(bc.fs, "")
			} else {
				fsConfig = fsConfig.WithDirMount(bc.dirMount, "")
			}

			mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().WithFSConfig(fsConfig))
			if err != nil {
				b.Fatal(err)
			}

			// If the benchmark's file descriptor isn't root, open the file
			// under a pre-determined directory: zig
			fd := sys.FdPreopen
			if bc.fd != sys.FdPreopen {
				fsc := mod.(*wasm.ModuleInstance).Sys.FS()
				preopenFile, ok := fsc.LookupFile(sys.FdPreopen)
				require.True(b, ok)
				preopen := preopenFile.FS
				fd, errno := fsc.OpenFile(preopen, "zig", experimentalsys.O_RDONLY, 0)
				if errno != 0 {
					b.Fatal(errno)
				}
				defer fsc.CloseFile(fd) //nolint
			}

			fn := mod.ExportedFunction(wasip1.PathFilestatGetName)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()

				flags := uint32(0)
				path := uint32(0)
				pathLen := len(bc.path)
				resultFilestat := 1024 // where to write the stat

				if !mod.Memory().WriteString(path, bc.path) {
					b.Fatal("could not write path")
				}

				// Time the call to write the stat
				b.StartTimer()
				results, err := fn.Call(testCtx, uint64(fd), uint64(flags), uint64(path), uint64(pathLen), uint64(resultFilestat))
				if err != nil {
					b.Fatal(err)
				}
				b.StopTimer()

				requireESuccess(b, results)
			}
		})
	}
}

func requireESuccess(b *testing.B, results []uint64) {
	b.Helper()
	if errno := wasip1.Errno(results[0]); errno != 0 {
		b.Fatal(wasip1.ErrnoName(errno))
	}
}

type writerFunc func(buf []byte) (n int, err error)

// Write implements io.Writer by calling writerFunc.
func (f writerFunc) Write(buf []byte) (n int, err error) {
	return f(buf)
}

func Benchmark_fdWrite(b *testing.B) {
	r := wazero.NewRuntime(testCtx)
	defer r.Close(testCtx)

	mod, err := instantiateProxyModule(r, wazero.NewModuleConfig().
		WithStdout(writerFunc(func(buf []byte) (n int, err error) { return len(buf), nil })),
	)
	if err != nil {
		b.Fatal(err)
	}
	fn := mod.ExportedFunction(wasip1.FdWriteName)

	iovs := uint32(1) // arbitrary offset
	mod.Memory().Write(0, []byte{
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
		fd   int32
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
				requireESuccess(b, results)
			}
		})
	}
}

// instantiateProxyModule instantiates a guest that re-exports WASI functions.
func instantiateProxyModule(r wazero.Runtime, config wazero.ModuleConfig) (api.Module, error) {
	wasiModuleCompiled, err := wasi_snapshot_preview1.NewBuilder(r).Compile(testCtx)
	if err != nil {
		return nil, err
	}

	if _, err = r.InstantiateModule(testCtx, wasiModuleCompiled, wazero.NewModuleConfig()); err != nil {
		return nil, err
	}

	proxyBin := proxy.NewModuleBinary(wasi_snapshot_preview1.ModuleName, wasiModuleCompiled)

	proxyCompiled, err := r.CompileModule(testCtx, proxyBin)
	if err != nil {
		return nil, err
	}

	return r.InstantiateModule(testCtx, proxyCompiled, config)
}
