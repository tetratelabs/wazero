package wazevo_test

import (
	"context"
	"crypto/rand"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/opt"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var configs = []struct {
	name   string
	config wazero.RuntimeConfig
}{
	{
		name:   "baseline",
		config: wazero.NewRuntimeConfigCompiler(),
	},
	{
		name:   "optimizing",
		config: opt.NewRuntimeConfigOptimizingCompiler(),
	},
}

func BenchmarkStdlibs(b *testing.B) {
	b.Run("zig", benchmarkZig)
	b.Run("tinygo", benchmarkTinyGo)
	b.Run("gowasip1", benchmarkGoWasip1)
}

func benchmarkZig(b *testing.B) {
	dir := "testdata/zig/"
	ctx := context.Background()

	modCfg := defaultModuleConfig().
		WithFSConfig(wazero.NewFSConfig().WithDirMount(".", "/")).
		WithArgs("test.wasm")

	files, err := os.ReadDir(dir)
	require.NoError(b, err)
	for _, f := range files {
		bin, err := os.ReadFile(dir + f.Name())
		require.NoError(b, err)
		b.Run(f.Name(), func(b *testing.B) {
			for _, cfg := range configs {
				r := wazero.NewRuntimeWithConfig(ctx, cfg.config)
				wasi_snapshot_preview1.MustInstantiate(ctx, r)
				b.Cleanup(func() { r.Close(ctx) })

				m, err := r.CompileModule(ctx, bin)
				require.NoError(b, err)

				b.Run(cfg.name, func(b *testing.B) {
					b.Run("Compile", func(b *testing.B) {
						_, err := r.CompileModule(ctx, bin)
						require.NoError(b, err)
					})
					im, err := r.InstantiateModule(ctx, m, modCfg)
					require.NoError(b, err)
					b.Run("Run", func(b *testing.B) {
						_, err := im.ExportedFunction("_start").Call(ctx)
						requireZeroExitCode(b, err)
					})
				})
			}
		})
	}
}

func benchmarkTinyGo(b *testing.B) {
	dir := "testdata/tinygo/"
	ctx := context.Background()

	files, err := os.ReadDir(dir)
	require.NoError(b, err)
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".test") {
			continue
		}
		bin, err := os.ReadFile(dir + f.Name())
		fsconfig := wazero.NewFSConfig().
			WithDirMount(".", "/").
			WithDirMount("/tmp", "/tmp")
		modCfg := defaultModuleConfig().
			WithFSConfig(fsconfig).
			WithArgs(f.Name(), "-test.v")

		require.NoError(b, err)
		b.Run(f.Name(), func(b *testing.B) {
			for _, cfg := range configs {
				r := wazero.NewRuntimeWithConfig(ctx, cfg.config)
				wasi_snapshot_preview1.MustInstantiate(ctx, r)
				b.Cleanup(func() { r.Close(ctx) })

				m, err := r.CompileModule(ctx, bin)
				require.NoError(b, err)

				b.Run(cfg.name, func(b *testing.B) {
					b.Run("Compile", func(b *testing.B) {
						_, err := r.CompileModule(ctx, bin)
						require.NoError(b, err)
					})
					im, err := r.InstantiateModule(ctx, m, modCfg)
					require.NoError(b, err)
					b.Run("Run", func(b *testing.B) {
						_, err := im.ExportedFunction("_start").Call(ctx)
						requireZeroExitCode(b, err)
					})
				})
			}
		})
	}
}

func benchmarkGoWasip1(b *testing.B) {
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	dir := "testdata/go/"
	ctx := context.Background()

	files, err := os.ReadDir(dir)
	require.NoError(b, err)
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".test") {
			continue
		}
		os.Chdir(cwd)
		fpath := filepath.Join(cwd, dir, f.Name())
		bin, err := os.ReadFile(fpath)
		fsuffixstripped := strings.ReplaceAll(f.Name(), ".test", "")
		inferredpath := strings.ReplaceAll(fsuffixstripped, "_", "/")
		testdir := filepath.Join(runtime.GOROOT(), inferredpath)

		os.Chdir(testdir)
		modCfg := defaultModuleConfig().
			WithFSConfig(
				wazero.NewFSConfig().
					WithDirMount("/", "/")).
			WithEnv("PWD", testdir).
			WithArgs(f.Name(), "-test.short", "-test.v")

		require.NoError(b, err)
		b.Run(f.Name(), func(b *testing.B) {
			for _, cfg := range configs {
				r := wazero.NewRuntimeWithConfig(ctx, cfg.config)
				wasi_snapshot_preview1.MustInstantiate(ctx, r)
				b.Cleanup(func() { r.Close(ctx) })

				m, err := r.CompileModule(ctx, bin)
				require.NoError(b, err)

				b.Run(cfg.name, func(b *testing.B) {
					b.Run("Compile", func(b *testing.B) {
						_, err := r.CompileModule(ctx, bin)
						require.NoError(b, err)
					})
					im, err := r.InstantiateModule(ctx, m, modCfg)
					require.NoError(b, err)
					b.Run("Run", func(b *testing.B) {
						_, err := im.ExportedFunction("_start").Call(ctx)
						requireZeroExitCode(b, err)
					})
				})
			}
		})
	}
}

func requireZeroExitCode(b *testing.B, err error) {
	b.Helper()
	if se, ok := err.(*sys.ExitError); ok {
		if se.ExitCode() != 0 { // Don't err on success.
			require.NoError(b, err)
		}
	}
}

func defaultModuleConfig() wazero.ModuleConfig {
	return wazero.NewModuleConfig().
		WithSysNanosleep().
		WithSysNanotime().
		WithSysWalltime().
		WithRandSource(rand.Reader).
		// Some tests require Stdout and Stderr to be present.
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithStartFunctions()
}
