package wazero

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/version"
)

func TestCompilationCacheReadOnlyDirectories(t *testing.T) {
	for _, kind := range []string{"missing root", "missing version", "root file", "version file"} {
		t.Run(kind, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "cache")
			switch kind {
			case "missing version", "version file":
				require.NoError(t, os.Mkdir(root, 0o700))
			case "root file":
				require.NoError(t, os.WriteFile(root, nil, 0o600))
			}
			versionDir := filepath.Join(root, "wazero-"+version.GetWazeroVersion()+"-"+goruntime.GOARCH+"-"+goruntime.GOOS)
			if kind == "version file" {
				require.NoError(t, os.WriteFile(versionDir, nil, 0o600))
			}
			c, err := NewCompilationCacheWithDirReadOnly(root)
			require.Nil(t, c)
			require.True(t, errors.Is(err, ErrCompilationCacheIO))
			if kind == "missing root" {
				_, err = os.Stat(root)
				require.True(t, os.IsNotExist(err))
			}
			if kind == "missing version" {
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Equal(t, 0, len(entries))
			}
		})
	}
}

func TestCompilationCacheReadOnly(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip("persistent compiler cache unsupported")
	}
	ctx := context.Background()
	dir := t.TempDir()
	warm, err := NewCompilationCacheWithDir(dir)
	require.NoError(t, err)
	r := NewRuntimeWithConfig(ctx, NewRuntimeConfigCompiler().WithCompilationCache(warm))
	compiled, err := r.CompileModule(ctx, facWasm)
	require.NoError(t, err)
	require.NoError(t, compiled.Close(ctx))
	require.NoError(t, r.Close(ctx))
	require.NoError(t, warm.Close(ctx))
	before := cacheDirectorySnapshot(t, dir)
	// Each iteration starts with a fresh cache engine, not just a fresh runtime.
	for i := 0; i < 2; i++ {
		c, err := NewCompilationCacheWithDirReadOnly(dir)
		require.NoError(t, err)
		r = NewRuntimeWithConfig(ctx, NewRuntimeConfigCompiler().WithCompilationCache(c))
		m, err := r.Instantiate(ctx, facWasm)
		require.NoError(t, err)
		result, err := m.ExportedFunction("fac-ssa").Call(ctx, 5)
		require.NoError(t, err)
		require.Equal(t, []uint64{120}, result)
		_, err = r.CompileModule(ctx, memGrowWasm)
		require.True(t, errors.Is(err, ErrCompilationCacheMiss))
		_, err = r.NewHostModuleBuilder("host").NewFunctionBuilder().WithFunc(func() {}).Export("f").Instantiate(ctx)
		require.NoError(t, err)
		require.NoError(t, r.Close(ctx))
		require.NoError(t, c.Close(ctx))
		require.Equal(t, before, cacheDirectorySnapshot(t, dir))
	}
	// The default constructor still compiles and writes a miss.
	rw, err := NewCompilationCacheWithDir(dir)
	require.NoError(t, err)
	r = NewRuntimeWithConfig(ctx, NewRuntimeConfigCompiler().WithCompilationCache(rw))
	_, err = r.CompileModule(ctx, memGrowWasm)
	require.NoError(t, err)
	require.NoError(t, r.Close(ctx))
	require.NoError(t, rw.Close(ctx))
	require.True(t, len(cacheDirectorySnapshot(t, dir)) > len(before))
}

func TestCompilationCacheReadOnlyInterpreter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	warm, err := NewCompilationCacheWithDir(dir)
	require.NoError(t, err)
	require.NoError(t, warm.Close(ctx))
	c, err := NewCompilationCacheWithDirReadOnly(dir)
	require.NoError(t, err)
	defer c.Close(ctx)
	configs := []RuntimeConfig{NewRuntimeConfigInterpreter()}
	if !platform.CompilerSupported() {
		configs = append(configs, NewRuntimeConfig())
	}
	for _, config := range configs {
		r := NewRuntimeWithConfig(ctx, config.WithCompilationCache(c))
		_, err = r.CompileModule(ctx, facWasm)
		require.True(t, errors.Is(err, ErrCompilationCacheUnsupported))
		_, err = r.NewHostModuleBuilder("host").NewFunctionBuilder().WithFunc(func() {}).Export("f").Instantiate(ctx)
		require.NoError(t, err)
		require.NoError(t, r.Close(ctx))
	}
}

func cacheDirectorySnapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	require.NoError(t, filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			snapshot[p] = string(data) + info.ModTime().String() + info.Mode().String()
		}
		return nil
	}))
	return snapshot
}
