package wazevo_test

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/opt"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func BenchmarkZig(b *testing.B) {
	if runtime.GOARCH == "arm64" {
		b.Run("optimizing", func(b *testing.B) {
			c := opt.NewRuntimeConfigOptimizingCompiler()
			runtBenches(b, context.Background(), c, zigTestCase)
		})
	}
	b.Run("baseline", func(b *testing.B) {
		c := wazero.NewRuntimeConfigCompiler()
		runtBenches(b, context.Background(), c, zigTestCase)
	})
}

func BenchmarkTinyGo(b *testing.B) {
	if runtime.GOARCH == "arm64" {
		b.Run("optimizing", func(b *testing.B) {
			c := opt.NewRuntimeConfigOptimizingCompiler()
			runtBenches(b, context.Background(), c, tinyGoTestCase)
		})
	}
	b.Run("baseline", func(b *testing.B) {
		c := wazero.NewRuntimeConfigCompiler()
		runtBenches(b, context.Background(), c, tinyGoTestCase)
	})
}

func BenchmarkWasip1(b *testing.B) {
	if runtime.GOARCH == "arm64" {
		b.Run("optimizing", func(b *testing.B) {
			c := opt.NewRuntimeConfigOptimizingCompiler()
			runtBenches(b, context.Background(), c, wasip1TestCase)
		})
	}
	b.Run("baseline", func(b *testing.B) {
		c := wazero.NewRuntimeConfigCompiler()
		runtBenches(b, context.Background(), c, wasip1TestCase)
	})
}

type testCase struct {
	name, dir    string
	readTestCase func(fpath string, fname string) ([]byte, wazero.ModuleConfig, error)
}

var (
	zigTestCase = testCase{
		name: "zig",
		dir:  "testdata/zig/",
		readTestCase: func(fpath string, fname string) ([]byte, wazero.ModuleConfig, error) {
			bin, err := os.ReadFile(fpath)
			modCfg := defaultModuleConfig().
				WithFSConfig(wazero.NewFSConfig().WithDirMount(".", "/")).
				WithArgs("test.wasm")

			return bin, modCfg, err
		},
	}
	tinyGoTestCase = testCase{
		name: "tinygo",
		dir:  "testdata/tinygo/",
		readTestCase: func(fpath string, fname string) ([]byte, wazero.ModuleConfig, error) {
			if !strings.HasSuffix(fname, ".test") {
				return nil, nil, nil
			}
			bin, err := os.ReadFile(fpath)
			fsconfig := wazero.NewFSConfig().
				WithDirMount(".", "/").
				WithDirMount(os.TempDir(), "/tmp")
			modCfg := defaultModuleConfig().
				WithFSConfig(fsconfig).
				WithArgs(fname, "-test.v")

			return bin, modCfg, err
		},
	}
	wasip1TestCase = testCase{
		name: "wasip1",
		dir:  "testdata/go/",
		readTestCase: func(fpath string, fname string) ([]byte, wazero.ModuleConfig, error) {
			if !strings.HasSuffix(fname, ".test") {
				return nil, nil, nil
			}
			bin, err := os.ReadFile(fpath)
			if err != nil {
				return nil, nil, err
			}
			fsuffixstripped := strings.ReplaceAll(fname, ".test", "")
			inferredpath := strings.ReplaceAll(fsuffixstripped, "_", "/")
			testdir := filepath.Join(runtime.GOROOT(), inferredpath)
			err = os.Chdir(testdir)

			sysroot := filepath.VolumeName(testdir) + string(os.PathSeparator)
			normalizedTestdir := normalizeOsPath(testdir)

			modCfg := defaultModuleConfig().
				WithFSConfig(
					wazero.NewFSConfig().
						WithDirMount(sysroot, "/").
						WithDirMount(os.TempDir(), "/tmp")).
				WithEnv("PWD", normalizedTestdir)

			args := []string{fname, "-test.short", "-test.v"}

			// Skip tests that are fragile on Windows.
			if runtime.GOOS == "windows" {
				modCfg = modCfg.
					WithEnv("GOROOT", normalizeOsPath(runtime.GOROOT()))

				args = append(args,
					"-test.skip=TestRenameCaseDifference/dir|"+
						"TestDirFSPathsValid|TestDirFS|TestDevNullFile|"+
						"TestOpenError|TestSymlinkWithTrailingSlash")
			}
			modCfg = modCfg.WithArgs(args...)

			return bin, modCfg, err
		},
	}
)

func runtBenches(b *testing.B, ctx context.Context, rc wazero.RuntimeConfig, tc testCase) {
	cwd, _ := os.Getwd()
	files, err := os.ReadDir(tc.dir)
	require.NoError(b, err)
	for _, f := range files {
		fname := f.Name()
		// Ensure we are on root dir.
		err = os.Chdir(cwd)
		require.NoError(b, err)

		fpath := filepath.Join(cwd, tc.dir, fname)
		bin, modCfg, err := tc.readTestCase(fpath, fname)
		require.NoError(b, err)
		if bin == nil {
			continue
		}

		for _, compile := range []bool{false, true} {
			if compile {
				b.Run("Compile/"+fname, func(b *testing.B) {
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						r := wazero.NewRuntimeWithConfig(ctx, rc)
						_, err := r.CompileModule(ctx, bin)
						require.NoError(b, err)
						require.NoError(b, r.Close(ctx))
					}
				})
			} else {
				r := wazero.NewRuntimeWithConfig(ctx, rc)
				wasi_snapshot_preview1.MustInstantiate(ctx, r)
				b.Cleanup(func() { r.Close(ctx) })

				cm, err := r.CompileModule(ctx, bin)
				require.NoError(b, err)
				b.Run("Run/"+fname, func(b *testing.B) {
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						// Instantiate in the loop as _start cannot be called multiple times.
						m, err := r.InstantiateModule(ctx, cm, modCfg)
						requireZeroExitCode(b, err)
						require.NoError(b, m.Close(ctx))
					}
				})
			}
		}
	}
}

// Normalize an absolute path to a Unix-style path, regardless if it is a Windows path.
func normalizeOsPath(path string) string {
	// Remove volume name. This is '/' on *Nix and 'C:' (with C being any letter identifier).
	root := filepath.VolumeName(path)
	testdirnoprefix := path[len(root):]
	// Normalizes all the path separators to a Unix separator.
	testdirnormalized := strings.ReplaceAll(testdirnoprefix, string(os.PathSeparator), "/")
	return testdirnormalized
}

func defaultModuleConfig() wazero.ModuleConfig {
	return wazero.NewModuleConfig().
		WithSysNanosleep().
		WithSysNanotime().
		WithSysWalltime().
		WithRandSource(rand.Reader).
		// Some tests require Stdout and Stderr to be present.
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)
}

func requireZeroExitCode(b *testing.B, err error) {
	b.Helper()
	if se, ok := err.(*sys.ExitError); ok {
		if se.ExitCode() != 0 { // Don't err on success.
			require.NoError(b, err)
		}
	}
}
