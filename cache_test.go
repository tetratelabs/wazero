package wazero

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path"
	goruntime "runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/fac.wasm
var facWasm []byte

//go:embed testdata/mem_grow.wasm
var memGrowWasm []byte

func TestCompilationCache(t *testing.T) {
	ctx := context.Background()
	// Ensures the normal Wasm module compilation cache works.
	t.Run("non-host module", func(t *testing.T) {
		foo, bar := getCacheSharedRuntimes(ctx, t)
		cacheInst := foo.cache

		// Create a different type id on the bar's store so that we can emulate that bar instantiated the module before facWasm.
		_, err := bar.store.GetFunctionTypeIDs(
			// Arbitrary one is fine as long as it is not used in facWasm.
			[]wasm.FunctionType{{Params: []wasm.ValueType{
				wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32,
				wasm.ValueTypeI32, wasm.ValueTypeV128, wasm.ValueTypeI32, wasm.ValueTypeV128, wasm.ValueTypeI32, wasm.ValueTypeI32,
				wasm.ValueTypeI32, wasm.ValueTypeV128, wasm.ValueTypeI32, wasm.ValueTypeV128, wasm.ValueTypeI32, wasm.ValueTypeI32,
				wasm.ValueTypeI32, wasm.ValueTypeV128, wasm.ValueTypeI32, wasm.ValueTypeV128, wasm.ValueTypeI32, wasm.ValueTypeI32,
				wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32, wasm.ValueTypeI32,
			}}})
		require.NoError(t, err)

		// add interpreter first, to ensure compiler support isn't order dependent
		eng := foo.cache.engs[engineKindInterpreter]
		if platform.CompilerSupported() {
			eng = foo.cache.engs[engineKindCompiler]
		}

		// Try compiling.
		compiled, err := foo.CompileModule(ctx, facWasm)
		require.NoError(t, err)
		// Also check it is actually cached.
		require.Equal(t, uint32(1), eng.CompiledModuleCount())
		barCompiled, err := bar.CompileModule(ctx, facWasm)
		require.NoError(t, err)

		// Ensures compiled modules are the same modulo type IDs, which is unique per store.
		require.Equal(t, compiled.(*compiledModule).module, barCompiled.(*compiledModule).module)
		require.Equal(t, compiled.(*compiledModule).closeWithModule, barCompiled.(*compiledModule).closeWithModule)
		require.Equal(t, compiled.(*compiledModule).compiledEngine, barCompiled.(*compiledModule).compiledEngine)
		// TypeIDs must be different as we create a different type ID on bar beforehand.
		require.NotEqual(t, compiled.(*compiledModule).typeIDs, barCompiled.(*compiledModule).typeIDs)

		// Two runtimes are completely separate except the compilation cache,
		// therefore it should be ok to instantiate the same name module for each of them.
		fooInst, err := foo.InstantiateModule(ctx, compiled, NewModuleConfig().WithName("same_name"))
		require.NoError(t, err)
		barInst, err := bar.InstantiateModule(ctx, compiled, NewModuleConfig().WithName("same_name"))
		require.NoError(t, err)
		// Two instances are not equal.
		require.NotEqual(t, fooInst, barInst)

		// Closing two runtimes shouldn't clear the cache as cache.Close must be explicitly called to clear the cache.
		err = foo.Close(ctx)
		require.NoError(t, err)
		err = bar.Close(ctx)
		require.NoError(t, err)
		require.Equal(t, uint32(1), eng.CompiledModuleCount())

		// Close the cache, and ensure the engine is closed.
		err = cacheInst.Close(ctx)
		require.NoError(t, err)
		require.Equal(t, uint32(0), eng.CompiledModuleCount())
	})

	// Even when cache is configured, compiled host modules must be different as that's the way
	// to provide per-runtime isolation on Go functions.
	t.Run("host module", func(t *testing.T) {
		foo, bar := getCacheSharedRuntimes(ctx, t)

		goFn := func() (dummy uint32) { return }
		fooCompiled, err := foo.NewHostModuleBuilder("env").
			NewFunctionBuilder().WithFunc(goFn).Export("go_fn").
			Compile(testCtx)
		require.NoError(t, err)
		barCompiled, err := bar.NewHostModuleBuilder("env").
			NewFunctionBuilder().WithFunc(goFn).Export("go_fn").
			Compile(testCtx)
		require.NoError(t, err)

		// Ensures they are different.
		require.NotEqual(t, fooCompiled, barCompiled)
	})

	t.Run("memory limit should not affect caches", func(t *testing.T) {
		// Creates new cache instance and pass it to the config.
		c := NewCompilationCache()
		config := NewRuntimeConfig().WithCompilationCache(c)

		// create two different runtimes with separate memory limits
		rt0 := NewRuntimeWithConfig(ctx, config)
		rt1 := NewRuntimeWithConfig(ctx, config.WithMemoryLimitPages(2))
		rt2 := NewRuntimeWithConfig(ctx, config.WithMemoryLimitPages(4))

		// the compiled module is not equal because the memory limits are applied to the Memory instance
		module0, _ := rt0.CompileModule(ctx, memGrowWasm)
		module1, _ := rt1.CompileModule(ctx, memGrowWasm)
		module2, _ := rt2.CompileModule(ctx, memGrowWasm)

		max0, _ := module0.ExportedMemories()["memory"].Max()
		max1, _ := module1.ExportedMemories()["memory"].Max()
		max2, _ := module2.ExportedMemories()["memory"].Max()
		require.Equal(t, uint32(5), max0)
		require.Equal(t, uint32(2), max1)
		require.Equal(t, uint32(4), max2)

		compiledModule0 := module0.(*compiledModule)
		compiledModule1 := module1.(*compiledModule)
		compiledModule2 := module2.(*compiledModule)

		// compare the compiled engine which contains the underlying "codes"
		require.Equal(t, compiledModule0.compiledEngine, compiledModule1.compiledEngine)
		require.Equal(t, compiledModule1.compiledEngine, compiledModule2.compiledEngine)
	})
}

func getCacheSharedRuntimes(ctx context.Context, t *testing.T) (foo, bar *runtime) {
	// Creates new cache instance and pass it to the config.
	c := NewCompilationCache()
	config := NewRuntimeConfig().WithCompilationCache(c)

	_foo := NewRuntimeWithConfig(ctx, config)
	_bar := NewRuntimeWithConfig(ctx, config)

	var ok bool
	foo, ok = _foo.(*runtime)
	require.True(t, ok)
	bar, ok = _bar.(*runtime)
	require.True(t, ok)

	// Make sure that two runtimes share the same cache instance.
	require.Equal(t, foo.cache, bar.cache)
	return
}

func TestCache_ensuresFileCache(t *testing.T) {
	const version = "dev"
	// We expect to create a version-specific subdirectory.
	expectedSubdir := fmt.Sprintf("wazero-dev-%s-%s", goruntime.GOARCH, goruntime.GOOS)

	t.Run("ok", func(t *testing.T) {
		dir := t.TempDir()
		c := &cache{}
		err := c.ensuresFileCache(dir, version)
		require.NoError(t, err)
	})
	t.Run("create dir", func(t *testing.T) {
		tmpDir := path.Join(t.TempDir(), "1", "2", "3")
		dir := path.Join(tmpDir, "foo") // Non-existent directory.

		c := &cache{}
		err := c.ensuresFileCache(dir, version)
		require.NoError(t, err)

		requireContainsDir(t, tmpDir, "foo")
	})
	t.Run("create relative dir", func(t *testing.T) {
		tmpDir, oldwd := requireChdirToTemp(t)
		defer os.Chdir(oldwd) //nolint
		dir := "foo"

		c := &cache{}
		err := c.ensuresFileCache(dir, version)
		require.NoError(t, err)

		requireContainsDir(t, tmpDir, dir)
	})
	t.Run("basedir is not a dir", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "nondir")
		require.NoError(t, err)
		defer f.Close()

		c := &cache{}
		err = c.ensuresFileCache(f.Name(), version)
		require.Contains(t, err.Error(), "is not dir")
	})
	t.Run("versiondir is not a dir", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(path.Join(dir, expectedSubdir), []byte{}, 0o600))
		c := &cache{}
		err := c.ensuresFileCache(dir, version)
		require.Contains(t, err.Error(), "is not dir")
	})
}

// requireContainsDir ensures the directory was created in the correct path,
// as file.Abs can return slightly different answers for a temp directory. For
// example, /var/folders/... vs /private/var/folders/...
func requireContainsDir(t *testing.T, parent, dir string) {
	entries, err := os.ReadDir(parent)
	require.NoError(t, err)
	require.Equal(t, 1, len(entries))
	require.Equal(t, dir, entries[0].Name())
	require.True(t, entries[0].IsDir())
}

func requireChdirToTemp(t *testing.T) (string, string) {
	tmpDir := t.TempDir()
	oldwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmpDir))
	return tmpDir, oldwd
}

func TestCache_Close(t *testing.T) {
	t.Run("all engines", func(t *testing.T) {
		c := &cache{engs: [engineKindCount]wasm.Engine{&mockEngine{}, &mockEngine{}}}
		err := c.Close(testCtx)
		require.NoError(t, err)
		for i := engineKind(0); i < engineKindCount; i++ {
			require.True(t, c.engs[i].(*mockEngine).closed)
		}
	})
	t.Run("only interp", func(t *testing.T) {
		c := &cache{engs: [engineKindCount]wasm.Engine{nil, &mockEngine{}}}
		err := c.Close(testCtx)
		require.NoError(t, err)
		require.True(t, c.engs[engineKindInterpreter].(*mockEngine).closed)
	})
	t.Run("only compiler", func(t *testing.T) {
		c := &cache{engs: [engineKindCount]wasm.Engine{&mockEngine{}, nil}}
		err := c.Close(testCtx)
		require.NoError(t, err)
		require.True(t, c.engs[engineKindCompiler].(*mockEngine).closed)
	})
}
