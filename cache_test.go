package wazero

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed internal/integration_test/vs/testdata/fac.wasm
var facWasm []byte

func TestCache(t *testing.T) {
	ctx := context.Background()
	// Ensures the normal Wasm module compilation cache works.
	t.Run("non-host module", func(t *testing.T) {
		foo, bar := getCacheSharedRuntimes(ctx, t)
		cacheInst := foo.cache

		// Try compiling.
		compiled, err := foo.CompileModule(ctx, facWasm)
		require.NoError(t, err)
		// Also check it is actually cached.
		require.Equal(t, uint32(1), cacheInst.eng.CompiledModuleCount())
		barCompiled, err := bar.CompileModule(ctx, facWasm)
		require.NoError(t, err)

		// Ensures compiled modules are the same.
		require.Equal(t, compiled, barCompiled)

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
		require.Equal(t, uint32(1), cacheInst.eng.CompiledModuleCount())

		// Close the cache, and ensure the engine is closed.
		err = cacheInst.Close(ctx)
		require.NoError(t, err)
		require.Equal(t, uint32(0), cacheInst.eng.CompiledModuleCount())
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
}

func getCacheSharedRuntimes(ctx context.Context, t *testing.T) (foo, bar *runtime) {
	// Creates new cache instance and pass it to the config.
	c := NewCache()
	config := NewRuntimeConfig().WithCache(c)

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
