package adhoc

// Exception handling integration tests for the interpreter engine.
//
// Background: these tests cover the Emscripten/pdfium-style EH pattern where
// exceptions propagate across multiple function call frames, each of which may
// have its own try_table handler (catch_all_ref + throw_ref cleanup pattern).
//
// The interpreter bug fixed here: when an inner callWithUnwind (e.g., in a
// "child" function) recovered a *thrownException whose matching try_table
// handler belonged to an outer (grandparent) callNativeFunc invocation,
// doRestore incorrectly restored grandparent's frame while still inside
// child's callNativeFunc.  The child then started executing grandparent's
// body, eventually calling popTryHandler on an already-empty slice and
// panicking with "slice bounds out of range [:-1]".
//
// The fix: callWithUnwind only handles handlers whose savedFrames length is >=
// the caller's frame count (i.e., handlers set up at the current or deeper
// call depth).  Handlers from outer invocations are re-panicked so that the
// correct outer callWithUnwind catches them.

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

//go:embed testdata/eh_cross_callnative.wasm
var ehCrossCallnativeWasm []byte

//go:embed testdata/eh_pdfium.wasm
var ehPdfiumWasm []byte

//go:embed testdata/eh_throw_ref_null.wasm
var ehThrowRefNullWasm []byte

//go:embed testdata/eh_br_orphan.wasm
var ehBrOrphanWasm []byte

//go:embed testdata/eh_br_stale_handler.wasm
var ehBrStaleHandlerWasm []byte

//go:embed testdata/eh_locals_corrupted.wasm
var ehLocalsCorruptedWasm []byte

//go:embed testdata/eh_locals_nested_nocatch.wasm
var ehLocalsNestedNocatchWasm []byte

//go:embed testdata/eh_locals_nested_catch.wasm
var ehLocalsNestedCatchWasm []byte

//go:embed testdata/eh_locals_cross_func.wasm
var ehLocalsCrossFuncWasm []byte

//go:embed testdata/eh_br_own_label.wasm
var ehBrOwnLabelWasm []byte

//go:embed testdata/eh_catch_outside.wasm
var ehCatchOutsideWasm []byte

// TestExceptionHandlingInterpreter runs EH tests only for the interpreter.
func TestExceptionHandlingInterpreter(t *testing.T) {
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
	runEHTests(t, cfg)
}

// TestExceptionHandlingCompiler runs EH tests for the compiler where supported.
func TestExceptionHandlingCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	cfg := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling)
	runEHTests(t, cfg)
}

func runEHTests(t *testing.T, cfg wazero.RuntimeConfig) {
	t.Run("cross_frame_catch", func(t *testing.T) {
		testEHCrossFrameCatch(t, cfg)
	})
	t.Run("pdfium_rethrow_pattern", func(t *testing.T) {
		testEHPdfiumRethrow(t, cfg)
	})
	t.Run("throw_ref_null", func(t *testing.T) {
		testThrowRefNull(t, cfg)
	})
	t.Run("br_exits_try_table", func(t *testing.T) {
		testBrExitsTryTable(t, cfg)
	})
	t.Run("br_stale_handler", func(t *testing.T) {
		testBrStaleHandler(t, cfg)
	})
	t.Run("locals_corrupted", func(t *testing.T) {
		testEHLocalsCorrupted(t, cfg)
	})
	t.Run("locals_nested_nocatch", func(t *testing.T) {
		testEHLocalsNestedNocatch(t, cfg)
	})
	t.Run("locals_nested_catch", func(t *testing.T) {
		testEHLocalsNestedCatch(t, cfg)
	})
	t.Run("locals_cross_func", func(t *testing.T) {
		testEHLocalsCrossFunc(t, cfg)
	})
	t.Run("br_own_label", func(t *testing.T) {
		testEHBrOwnLabel(t, cfg)
	})
	t.Run("catch_outside", func(t *testing.T) {
		testEHCatchOutside(t, cfg)
	})
}

// testEHCrossFrameCatch is the core reproducer for the interpreter bug:
// try_table in grandparent, exception thrown in grandchild,
// propagating through child which has no handler of its own.
// The grandparent's handler must catch it correctly.
func testEHCrossFrameCatch(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehCrossCallnativeWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// grandparent has a try_table, calls child, child calls grandchild which throws.
	// Grandparent's handler must catch via cross-frame propagation.
	res, err := mod.ExportedFunction("test_cross_frame_catch").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	// Rethrow pattern: child has catch_all_ref + throw_ref, grandparent catches the rethrow.
	res, err = mod.ExportedFunction("test_rethrow_cross_frame").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(2), api.DecodeI32(res[0]))
}

// testEHPdfiumRethrow tests the Emscripten destructor-cleanup pattern:
// catch_all_ref captures the exnref, runs cleanup, then rethrows via throw_ref.
// This pattern appears in pdfium.wasm for C++ exception handling.
func testEHPdfiumRethrow(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehPdfiumWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// One-level: leaf throws, level2 catches + rethrows, outer catches.
	res, err := mod.ExportedFunction("test_one_level_rethrow").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	// Two-level: throw → catch_all_ref + throw_ref → catch_all_ref + throw_ref → catch.
	res, err = mod.ExportedFunction("test_two_level_rethrow").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// testThrowRefNull verifies that throw_ref on a null exnref traps with
// "null reference" (not "unreachable"). This was a bug where the interpreter
// used ErrRuntimeUnreachable instead of ErrRuntimeNullReference.
func testThrowRefNull(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehThrowRefNullWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// Call with null exnref (0) — should trap as "null reference".
	_, err = mod.ExportedFunction("throw_ref_null").Call(ctx, 0)
	require.ErrorIs(t, err, wasmruntime.ErrRuntimeNullReference)
}

// testBrExitsTryTable verifies that br/br_if that exits a try_table block
// correctly pops the try handler. Without the fix, orphaned handlers would
// cause a popTryHandler underflow panic ("slice bounds out of range [:-1]").
func testBrExitsTryTable(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehBrOrphanWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// The function calls loop_with_try (which exits try_table via br_if),
	// then catches a throw in its own try_table. Should return 1.
	res, err := mod.ExportedFunction("test").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// testBrStaleHandler verifies that br exiting a try_table pops the handler
// so it doesn't interfere with later exception dispatch. Without the fix,
// the stale handler from try_table A incorrectly catches a throw meant for
// the outer handler, returning 99 instead of 1.
func testBrStaleHandler(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehBrStaleHandlerWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// Without fix: stale handler A catches $tag1 -> wrong checkpoint restore.
	// With fix: outer handler catches $tag1 -> returns 1.
	res, err := mod.ExportedFunction("test").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// testEHLocalsCorrupted verifies that locals mutated inside a try_table body
// retain their throw-time values when an exception is caught (issue #2503).
func testEHLocalsCorrupted(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehLocalsCorruptedWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// f() sets flag=1, enters try_table, sets flag=0, then throws.
	// The handler reads flag — must be 0 (throw-time), not 1 (entry-time).
	res, err := mod.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(0), api.DecodeI32(res[0]))
}

// testEHLocalsNestedNocatch verifies that a try_table with no catch clauses
// nested inside one with catch clauses doesn't break locals tracking.
func testEHLocalsNestedNocatch(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehLocalsNestedNocatchWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// The inner try_table (no catch) must not break the outer try body's
	// locals save-area tracking. flag must be 0 at throw time.
	res, err := mod.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(0), api.DecodeI32(res[0]))
}

// testEHLocalsNestedCatch verifies that nested try_tables in the same function
// share the locals save area so an outer handler sees throw-time local values
// modified inside the inner try body.
func testEHLocalsNestedCatch(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehLocalsNestedCatchWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// Inner try catches $t2, but thrower throws $t1 → outer catches.
	// flag was set to 0 inside the inner body; outer must see 0.
	res, err := mod.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(0), api.DecodeI32(res[0]))
}

// testEHLocalsCrossFunc verifies cross-function try_table nesting: function A
// has a try_table and calls B which also has a try_table. B throws an exception
// caught by A. A's handler must see A's locals, not B's.
func testEHLocalsCrossFunc(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehLocalsCrossFuncWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// A sets flag=0 inside its try body, then calls B which throws.
	// A's handler must see flag=0 (A's throw-time value).
	res, err := mod.ExportedFunction("f").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(0), api.DecodeI32(res[0]))
}

// testEHBrOwnLabel verifies that br to a try_table's own label pops its handler.
func testEHBrOwnLabel(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehBrOwnLabelWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	res, err := mod.ExportedFunction("run").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// testEHCatchOutside verifies that a catch clause jumping outside an enclosing
// try_table pops that try_table's handler.
func testEHCatchOutside(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, ehCatchOutsideWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	res, err := mod.ExportedFunction("run").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// TestExceptionHandlingCompilationCache verifies that
// the compilation cache round-trips the catchClauseTable correctly.
func TestExceptionHandlingCompilationCache(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	cacheDir := t.TempDir()

	for _, tc := range []struct {
		name string
		wasm []byte
		fn   string
		args []uint64
		want int32
	}{
		{"cross_frame_catch", ehCrossCallnativeWasm, "test_cross_frame_catch", nil, 1},
		{"pdfium_rethrow", ehPdfiumWasm, "test_one_level_rethrow", nil, 1},
		{"br_exits_try_table", ehBrOrphanWasm, "test", nil, 1},
		{"br_stale_handler", ehBrStaleHandlerWasm, "test", nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()

			// First: compile fresh and populate the file cache.
			cache1, err := wazero.NewCompilationCacheWithDir(cacheDir)
			require.NoError(t, err)
			cfg1 := wazero.NewRuntimeConfigCompiler().
				WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
				WithCompilationCache(cache1)
			r1 := wazero.NewRuntimeWithConfig(ctx, cfg1)
			mod1, err := r1.InstantiateWithConfig(ctx, tc.wasm,
				wazero.NewModuleConfig().WithStartFunctions())
			require.NoError(t, err)
			res, err := mod1.ExportedFunction(tc.fn).Call(ctx, tc.args...)
			require.NoError(t, err)
			require.Equal(t, tc.want, api.DecodeI32(res[0]))
			r1.Close(ctx)
			cache1.Close(ctx)

			// Second: new runtime loading from file cache.
			// Without the fix, catchClauseTable is empty → panic.
			cache2, err := wazero.NewCompilationCacheWithDir(cacheDir)
			require.NoError(t, err)
			cfg2 := wazero.NewRuntimeConfigCompiler().
				WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling).
				WithCompilationCache(cache2)
			r2 := wazero.NewRuntimeWithConfig(ctx, cfg2)
			mod2, err := r2.InstantiateWithConfig(ctx, tc.wasm,
				wazero.NewModuleConfig().WithStartFunctions())
			require.NoError(t, err)
			res, err = mod2.ExportedFunction(tc.fn).Call(ctx, tc.args...)
			require.NoError(t, err)
			require.Equal(t, tc.want, api.DecodeI32(res[0]))
			r2.Close(ctx)
			cache2.Close(ctx)
		})
	}
}
