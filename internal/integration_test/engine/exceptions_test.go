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
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
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
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling | experimental.CoreFeaturesTailCall)
	runEHTests(t, cfg)
}

// TestExceptionHandlingCompiler runs EH tests for the compiler where supported.
func TestExceptionHandlingCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	cfg := wazero.NewRuntimeConfigCompiler().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling | experimental.CoreFeaturesTailCall)
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
	t.Run("nested_dispatch", func(t *testing.T) {
		testEHNestedDispatch(t, cfg)
	})
	t.Run("call_with_stack_param_count", func(t *testing.T) {
		testEHCallWithStackParamCount(t, cfg)
	})
	t.Run("propagate_entry_block_call", func(t *testing.T) {
		testEHPropagateEntryBlockCall(t, cfg)
	})
	t.Run("propagate_tail_call", func(t *testing.T) {
		testEHPropagateTailCall(t, cfg)
	})
	t.Run("uncaught_stack_trace", func(t *testing.T) {
		testEHUncaughtStackTrace(t, cfg)
	})
	t.Run("exnref_survives_call", func(t *testing.T) {
		testEHExnrefSurvivesCall(t, cfg)
	})
	t.Run("exnref_released_on_overwrite", func(t *testing.T) {
		testEHExnrefReleasedOnOverwrite(t, cfg)
	})
	t.Run("exnref_read_then_overwritten", func(t *testing.T) {
		testEHExnrefReadThenOverwritten(t, cfg)
	})
	t.Run("exnref_param_survives_call", func(t *testing.T) {
		testEHExnrefParamSurvivesCall(t, cfg)
	})
	t.Run("exnref_table_bulk_ops", func(t *testing.T) {
		testEHExnrefTableBulkOps(t, cfg)
	})
	t.Run("exnref_concurrent_slot_access", func(t *testing.T) {
		testEHExnrefConcurrentSlotAccess(t, cfg)
	})
	t.Run("exnref_across_modules", func(t *testing.T) {
		testEHExnrefAcrossModules(t, cfg)
	})
	t.Run("exnref_local_live_across_catch", func(t *testing.T) {
		testEHExnrefLocalLiveAcrossCatch(t, cfg)
	})
	t.Run("exnref_closing_own_slot_keeps_other_holders", func(t *testing.T) {
		testEHClosingOwnSlotKeepsOtherHolders(t, cfg)
	})
	t.Run("plain_catch_holds_nothing_per_throw", func(t *testing.T) {
		testEHPlainCatchHoldsNothingPerThrow(t, cfg)
	})
	t.Run("rethrow_stack_trace", func(t *testing.T) {
		testEHRethrowStackTrace(t, cfg)
	})
	t.Run("uncaught_stack_trace_through_try_table", func(t *testing.T) {
		testEHUncaughtStackTraceThroughTryTable(t, cfg)
	})
	t.Run("listeners_abort_unwound_frames", func(t *testing.T) {
		testEHListenersAbortUnwoundFrames(t, cfg)
	})
	t.Run("listeners_abort_deep_unwind", func(t *testing.T) {
		testEHListenersAbortDeepUnwind(t, cfg)
	})
	t.Run("listener_abort_module_identity", func(t *testing.T) {
		testEHListenerAbortModuleIdentity(t, cfg)
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

	// The export makes the null itself and passes it on, the host not being able to: see
	// wasm.ExnrefInSignature. Either way throw_ref should trap as "null reference".
	_, err = mod.ExportedFunction("throw_ref_null").Call(ctx)
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

// testEHNestedDispatch checks nested try_table propagation: an exception whose tag
// is not matched by the inner try_table propagates to the enclosing try_table that
// does match it, and the handler observes locals set before the throw. Here $t1 is
// thrown inside an inner try_table that only catches $t0; the outer try_table
// catches $t1 and returns a local set to 30 before the throw.
func testEHNestedDispatch(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, buildNestedDispatchModule(),
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	res, err := mod.ExportedFunction("run").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(30), api.DecodeI32(res[0]))
}

// testEHCallWithStackParamCount checks that CallWithStack accepts a stack sized to
// the function's wasm param/result count when exception handling is enabled: an
// (i32)->(i32) function invoked with a 1-slot stack runs and returns its result
// rather than failing the param-count check.
func testEHCallWithStackParamCount(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	// Minimal EH module: exported (i32)->(i32) identity, no try_table.
	var buf []byte
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)
	buf = appendSection(buf, 1, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		s = append(s, 0x60, 1, byte(wasm.ValueTypeI32), 1, byte(wasm.ValueTypeI32))
		return s
	})
	buf = appendSection(buf, 3, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		s = append(s, 0)
		return s
	})
	buf = appendSection(buf, 7, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		s = appendUleb128(s, 2)
		s = append(s, "id"...)
		s = append(s, wasm.ExternTypeFunc)
		s = appendUleb128(s, 0)
		return s
	})
	buf = appendSection(buf, 10, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		body := []byte{0x00, wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}
		s = appendUleb128(s, uint32(len(body)))
		s = append(s, body...)
		return s
	})

	mod, err := r.Instantiate(ctx, buf)
	require.NoError(t, err)

	// CallWithStack with a wasm-sized (1-slot) stack: param in, result out.
	stack := []uint64{42}
	require.NoError(t, mod.ExportedFunction("id").CallWithStack(ctx, stack))
	require.Equal(t, int32(42), api.DecodeI32(stack[0]))
}

func buildNestedDispatchModule() []byte {
	// $thrower: () -> () { throw $t1 }
	thrower := []byte{0x00 /* no locals */, wasm.OpcodeThrow, 0x01, wasm.OpcodeEnd}

	// run: () -> (i32), 1 i32 local ($x).
	//   x=10
	//   block $done (result i32) {
	//     block $ocatch {
	//       try_table (catch $t1 -> $ocatch) {     ;; outer
	//         x=20
	//         try_table (catch $t0 -> $ocatch) {   ;; inner (does NOT catch $t1)
	//           x=30; call $thrower                 ;; throws $t1 -> propagates to outer
	//         }
	//         br $done (i32.const 0)                ;; inner normal (unreached)
	//       }
	//       br $done (i32.const 0)                  ;; outer normal (unreached)
	//     } ;; $ocatch end == where the caught exception resumes:
	//     br $done (local.get $x)                   ;; reads $x (=30)
	//   }
	runBody := []byte{
		wasm.OpcodeI32Const, 10, wasm.OpcodeLocalSet, 0,
		wasm.OpcodeBlock, 0x7f, // block $done (result i32)
		wasm.OpcodeBlock, 0x40, // block $ocatch (void)
		// outer try_table: void, 1 catch {kind=catch, tag=1, label=0 ($ocatch)}
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
		wasm.OpcodeI32Const, 20, wasm.OpcodeLocalSet, 0,
		// inner try_table: void, 1 catch {kind=catch, tag=0, label=1 ($ocatch)}
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x01,
		wasm.OpcodeI32Const, 30, wasm.OpcodeLocalSet, 0,
		wasm.OpcodeCall, 0x00, // call $thrower
		wasm.OpcodeEnd,                              // end inner try_table
		wasm.OpcodeI32Const, 0, wasm.OpcodeBr, 0x02, // inner-normal: br $done
		wasm.OpcodeEnd,                              // end outer try_table
		wasm.OpcodeI32Const, 0, wasm.OpcodeBr, 0x01, // outer-normal: br $done
		wasm.OpcodeEnd,                              // end $ocatch  (caught exception resumes here)
		wasm.OpcodeLocalGet, 0, wasm.OpcodeBr, 0x00, // br $done (local.get $x)
		wasm.OpcodeEnd, // end $done
		wasm.OpcodeEnd, // end func
	}
	// run locals: 1 group of 1 i32.
	run := append([]byte{0x01, 0x01, byte(wasm.ValueTypeI32)}, runBody...)

	var buf []byte
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)
	// Type section: type0 ()->(), type1 ()->(i32).
	buf = appendSection(buf, 1, func(s []byte) []byte {
		s = appendUleb128(s, 2)
		s = append(s, 0x60, 0, 0)
		s = append(s, 0x60, 0, 1, byte(wasm.ValueTypeI32))
		return s
	})
	// Function section: func0=type0, func1=type1.
	buf = appendSection(buf, 3, func(s []byte) []byte {
		s = appendUleb128(s, 2)
		s = append(s, 0, 1)
		return s
	})
	// Memory section: 1 page, so the catch path must restore memory state after the
	// throwing call.
	buf = appendSection(buf, 5, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		s = append(s, 0x00)
		s = appendUleb128(s, 1)
		return s
	})
	// Tag section (id 13): 2 tags, both attribute 0 + type 0.
	buf = appendSection(buf, 13, func(s []byte) []byte {
		s = appendUleb128(s, 2)
		s = append(s, 0x00, 0x00) // tag $t0: attr=0, type=0
		s = append(s, 0x00, 0x00) // tag $t1: attr=0, type=0
		return s
	})
	// Export "run" = func1.
	buf = appendSection(buf, 7, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		s = appendUleb128(s, 3)
		s = append(s, "run"...)
		s = append(s, wasm.ExternTypeFunc)
		s = appendUleb128(s, 1)
		return s
	})
	// Code section.
	buf = appendSection(buf, 10, func(s []byte) []byte {
		s = appendUleb128(s, 2)
		s = appendUleb128(s, uint32(len(thrower)))
		s = append(s, thrower...)
		s = appendUleb128(s, uint32(len(run)))
		s = append(s, run...)
		return s
	})
	return buf
}

// testEHPropagateEntryBlockCall covers an exception propagating through a frame whose
// throwing call sits in its entry block: $mid's call to $b is in block 0; $b throws,
// $mid has no try (so it propagates), $a catches and returns 42.
func testEHPropagateEntryBlockCall(t *testing.T, cfg wazero.RuntimeConfig) {
	requireEHCatchReturns42(t, cfg, buildEHPropagateModule(false))
}

// testEHPropagateTailCall covers an exception propagating out of a tail-callee. $mid
// return_call's $b, so $b runs in $mid's reused frame; when $b throws, $mid is already
// gone, and the throw must land in $a (which called $mid). $a catches and returns 42.
func testEHPropagateTailCall(t *testing.T, cfg wazero.RuntimeConfig) {
	requireEHCatchReturns42(t, cfg, buildEHPropagateModule(true))
}

// testEHUncaughtStackTrace pins the wasm stack trace an uncaught exception carries: the
// stack of the throw that was propagating, captured as the raise started. Asserting the
// exact text is what keeps the two engines from drifting apart here.
func testEHUncaughtStackTrace(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	// $c throws, $b calls $c, $a calls $b, and nothing catches.
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 0, 0},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "a", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd}},
		},
	}
	mod, err := r.Instantiate(ctx, encodeModule(m))
	require.NoError(t, err)

	_, err = mod.ExportedFunction("a").Call(ctx)
	require.Error(t, err)
	require.Equal(t, `wasm error: uncaught exception
wasm stack trace:
	.$0()
	.$1()
	.$2()`, err.Error())
}

// testEHExnrefLocalLiveAcrossCatch keeps an exnref local live across a catch in the same
// frame, which is where the two halves of the reference bookkeeping for a local have to meet.
//
// A catch rewinds the reference counts to the try_table's entry, but the locals a handler
// sees come from the save area, not from the rewound stack. So the runtime takes a reference
// for the handle the save area names and the handler releases the one the rewind restored --
// the same handle here, and the two must cancel. Getting either half wrong is loud: the call
// returns holding a reference it never let go of, or releases one it does not hold.
func testEHExnrefLocalLiveAcrossCatch(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Results: i32}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "run", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
				Body: []byte{
					// Catch by reference into local 0.
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeCall, 0x00,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeLocalSet, 0x00,
					// A second catch, with local 0 live across it, so its handler reloads a
					// local that already holds a reference.
					wasm.OpcodeBlock, 0x40,
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
					wasm.OpcodeCall, 0x00,
					wasm.OpcodeEnd, wasm.OpcodeEnd,
					// Let go of it, so the call returns holding nothing.
					wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref),
					wasm.OpcodeLocalSet, 0x00,
					wasm.OpcodeI32Const, 7, wasm.OpcodeEnd,
				},
			},
		},
	}
	mod, err := r.Instantiate(ctx, encodeModule(m))
	require.NoError(t, err)

	// Repeated, since a count that drifts by one per call only shows up on a later one.
	for i := 0; i < 3; i++ {
		res, err := mod.ExportedFunction("run").Call(ctx)
		require.NoError(t, err)
		require.Equal(t, int32(7), api.DecodeI32(res[0]))
	}
}

// testEHExnrefSurvivesCall parks an exnref in a global (and in a table), returns, and uses
// it from a later call. The slot holding it is what keeps it alive.
func testEHExnrefSurvivesCall(t *testing.T, cfg wazero.RuntimeConfig) {
	// The tag section sits between memory and global, so a table section has to precede
	// it and a global section has to follow it.
	for _, store := range []struct {
		name              string
		set, get          []byte
		beforeTagSection  bool
		buildStoreSection func([]byte) []byte
	}{
		{
			name: "global",
			set:  []byte{wasm.OpcodeGlobalSet, 0x00},
			get:  []byte{wasm.OpcodeGlobalGet, 0x00},
			buildStoreSection: func(buf []byte) []byte {
				return appendSection(buf, 6, func(s []byte) []byte {
					s = appendUleb128(s, 1)
					s = append(s, byte(wasm.ValueTypeExnref), 0x01)
					return append(s, wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref), wasm.OpcodeEnd)
				})
			},
		},
		{
			name:             "table",
			beforeTagSection: true,
			set:              []byte{wasm.OpcodeTableSet, 0x00},
			get:              []byte{wasm.OpcodeTableGet, 0x00},
			buildStoreSection: func(buf []byte) []byte {
				return appendSection(buf, 4, func(s []byte) []byte {
					s = appendUleb128(s, 1)
					s = append(s, byte(wasm.ValueTypeExnref), 0x00)
					return appendUleb128(s, 1)
				})
			},
		},
	} {
		t.Run(store.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, cfg)
			defer r.Close(ctx)
			mod, err := r.Instantiate(ctx, buildEHExnrefLifetimeModule(store.set, store.get, store.beforeTagSection, store.buildStoreSection))
			require.NoError(t, err)

			// Same call: capture, stash, reload and rethrow -- still valid.
			res, err := mod.ExportedFunction("round_trip").Call(ctx)
			require.NoError(t, err)
			require.Equal(t, int32(1), api.DecodeI32(res[0]))

			// A later call reads the same reference back out and throws it again.
			res, err = mod.ExportedFunction("use_stashed").Call(ctx)
			require.NoError(t, err)
			require.Equal(t, int32(1), api.DecodeI32(res[0]))

			// Still valid however many calls later, since the slot still names it.
			for i := 0; i < 3; i++ {
				res, err = mod.ExportedFunction("use_stashed").Call(ctx)
				require.NoError(t, err)
				require.Equal(t, int32(1), api.DecodeI32(res[0]))
			}
		})
	}
}

// buildEHExnrefLifetimeModule builds a module with one tag, a thrower, somewhere to stash
// an exnref, and two exported functions:
//
//	"round_trip":  catch by ref -> stash -> reload -> throw_ref -> caught  => 1
//	"use_stashed": reload whatever "round_trip" left behind -> throw_ref
func buildEHExnrefLifetimeModule(set, get []byte, beforeTagSection bool, storeSection func([]byte) []byte) []byte {
	var buf []byte
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)
	buf = appendSection(buf, 1, func(s []byte) []byte {
		s = appendUleb128(s, 2)
		s = append(s, 0x60, 0, 0)
		return append(s, 0x60, 0, 1, byte(wasm.ValueTypeI32))
	})
	buf = appendSection(buf, 3, func(s []byte) []byte {
		s = appendUleb128(s, 3)
		return append(s, 0, 1, 1)
	})
	if beforeTagSection {
		buf = storeSection(buf)
	}
	buf = appendSection(buf, 13, func(s []byte) []byte {
		s = appendUleb128(s, 1)
		return append(s, 0x00, 0x00)
	})
	if !beforeTagSection {
		buf = storeSection(buf)
	}
	buf = appendSection(buf, 7, func(s []byte) []byte {
		s = appendUleb128(s, 2)
		for i, n := range []string{"round_trip", "use_stashed"} {
			s = appendUleb128(s, uint32(len(n)))
			s = append(s, n...)
			s = append(s, wasm.ExternTypeFunc)
			s = appendUleb128(s, uint32(1+i))
		}
		return s
	})
	buf = appendSection(buf, 10, func(s []byte) []byte {
		s = appendUleb128(s, 3)
		add := func(b []byte) { s = appendUleb128(s, uint32(len(b))); s = append(s, b...) }
		add([]byte{0x00, wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd})

		// A table.set needs the index pushed before the value; a global.set does not.
		var idx []byte
		if get[0] == wasm.OpcodeTableGet {
			idx = []byte{wasm.OpcodeI32Const, 0x00}
		}

		// rethrow: <get> the stashed ref and throw_ref it inside a try_table catching the
		// tag, yielding 1 when it is still live.
		rethrow := []byte{wasm.OpcodeBlock, 0x40, wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00}
		rethrow = append(rethrow, idx...)
		rethrow = append(rethrow, get...)
		rethrow = append(rethrow, wasm.OpcodeThrowRef, wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x00, wasm.OpcodeReturn, wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd)

		capture := []byte{0x00}
		capture = append(capture, idx...)
		capture = append(capture,
			wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
			wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
			wasm.OpcodeCall, 0x00,
			wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd)
		capture = append(capture, set...)
		add(append(capture, rethrow...))      // round_trip
		add(append([]byte{0x00}, rethrow...)) // use_stashed
		return s
	})
	return buf
}

// ehRecordingListener records the calls it receives, so a test can assert that every entry
// a listener is told about is matched by an exit.
type ehRecordingListener struct{ events *[]string }

func (l ehRecordingListener) Before(_ context.Context, _ api.Module, def api.FunctionDefinition, _ []uint64, _ experimental.StackIterator) {
	*l.events = append(*l.events, "before "+def.DebugName())
}

func (l ehRecordingListener) After(_ context.Context, _ api.Module, def api.FunctionDefinition, _ []uint64) {
	*l.events = append(*l.events, "after "+def.DebugName())
}

func (l ehRecordingListener) Abort(_ context.Context, _ api.Module, def api.FunctionDefinition, err error) {
	*l.events = append(*l.events, fmt.Sprintf("abort %s (%v)", def.DebugName(), err))
}

// testEHListenersAbortUnwoundFrames covers what a listener is told when an exception takes
// frames off the stack. Each is aborted as it is unwound, including when a handler above
// catches the exception and the call as a whole succeeds.
func testEHListenersAbortUnwoundFrames(t *testing.T, cfg wazero.RuntimeConfig) {
	// $0 throws, $1 calls $0, and $2 is the exported entry, whose body varies per case.
	unwound := "(" + experimental.ErrUnwoundByException.Error() + ")"
	for _, tc := range []struct {
		name     string
		locals   []wasm.ValueType
		body     []byte
		expected []string
	}{
		{
			// Caught: the call succeeds, but $1 and $0 are gone before it does.
			name: "caught",
			body: []byte{
				wasm.OpcodeBlock, 0x40,
				wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
				wasm.OpcodeCall, 0x01,
				wasm.OpcodeEnd, wasm.OpcodeEnd, wasm.OpcodeEnd,
			},
			expected: []string{
				"before .$2", "before .$1", "before .$0",
				"abort .$0 " + unwound, "abort .$1 " + unwound,
				"after .$2",
			},
		},
		{
			// Uncaught: every frame is unwound, $2 included.
			name: "uncaught",
			body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd},
			expected: []string{
				"before .$2", "before .$1", "before .$0",
				"abort .$0 " + unwound, "abort .$1 " + unwound, "abort .$2 " + unwound,
			},
		},
		{
			// Caught by reference and thrown again: $2 is unwound by the second raise,
			// so it is aborted too, after the frames the first raise took.
			name:   "rethrown",
			locals: []wasm.ValueType{wasm.ValueTypeExnref},
			body: []byte{
				wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
				wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
				wasm.OpcodeCall, 0x01,
				wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
				wasm.OpcodeThrowRef, wasm.OpcodeEnd,
			},
			expected: []string{
				"before .$2", "before .$1", "before .$0",
				"abort .$0 " + unwound, "abort .$1 " + unwound, "abort .$2 " + unwound,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			ctx := experimental.WithFunctionListenerFactory(context.Background(),
				experimental.FunctionListenerFactoryFunc(func(api.FunctionDefinition) experimental.FunctionListener {
					return ehRecordingListener{&events}
				}))
			r := wazero.NewRuntimeWithConfig(ctx, cfg)
			defer r.Close(ctx)

			m := &wasm.Module{
				TypeSection:     []wasm.FunctionType{{}},
				TagSection:      []wasm.Tag{{Type: 0}},
				FunctionSection: []wasm.Index{0, 0, 0},
				ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "a", Index: 2}},
				CodeSection: []wasm.Code{
					{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
					{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}},
					{LocalTypes: tc.locals, Body: tc.body},
				},
			}
			mod, err := r.Instantiate(ctx, encodeModule(m))
			require.NoError(t, err)

			_, err = mod.ExportedFunction("a").Call(ctx)
			if tc.name == "caught" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, tc.expected, events)
		})
	}
}

// testEHListenersAbortDeepUnwind unwinds more frames than a stack trace holds
// (wasmdebug.MaxFrames). Truncating the trace is fine, but every frame still left the
// stack, so every one has to be aborted.
func testEHListenersAbortDeepUnwind(t *testing.T, cfg wazero.RuntimeConfig) {
	const depth = wasmdebug.MaxFrames + 10

	var events []string
	ctx := experimental.WithFunctionListenerFactory(context.Background(),
		experimental.FunctionListenerFactoryFunc(func(api.FunctionDefinition) experimental.FunctionListener {
			return ehRecordingListener{&events}
		}))
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	// $0 catches, $1..$depth-2 each call the next, and the last one throws.
	m := &wasm.Module{
		TypeSection:   []wasm.FunctionType{{}},
		TagSection:    []wasm.Tag{{Type: 0}},
		ExportSection: []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "a", Index: 0}},
	}
	for i := 0; i < depth; i++ {
		m.FunctionSection = append(m.FunctionSection, 0)
		var body []byte
		switch i {
		case 0:
			body = []byte{
				wasm.OpcodeBlock, 0x40,
				wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
				wasm.OpcodeCall, 0x01,
				wasm.OpcodeEnd, wasm.OpcodeEnd, wasm.OpcodeEnd,
			}
		case depth - 1:
			body = []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}
		default:
			body = append([]byte{wasm.OpcodeCall}, appendUleb128(nil, uint32(i+1))...)
			body = append(body, wasm.OpcodeEnd)
		}
		m.CodeSection = append(m.CodeSection, wasm.Code{Body: body})
	}
	mod, err := r.Instantiate(ctx, encodeModule(m))
	require.NoError(t, err)

	_, err = mod.ExportedFunction("a").Call(ctx)
	require.NoError(t, err) // $0 catches it, so the call itself succeeds.

	// Everything below $0 was unwound, innermost first, and $0 alone returned.
	var expected []string
	for i := 0; i < depth; i++ {
		expected = append(expected, fmt.Sprintf("before .$%d", i))
	}
	for i := depth - 1; i > 0; i-- {
		expected = append(expected, fmt.Sprintf("abort .$%d (%v)", i, experimental.ErrUnwoundByException))
	}
	expected = append(expected, "after .$0")
	require.Equal(t, expected, events)
}

// testEHPlainCatchHoldsNothingPerThrow covers what a `catch` clause has to keep alive per
// throw, which is nothing: it hands guest code param values, never a reference, so once the
// handler has loaded them the exception is unreachable.
//
// The params are read by compiled code through a raw pointer to Go memory, so something must
// root that buffer for the length of those loads. What roots it has to be O(1) in the number
// of throws -- a field holding the latest, not an entry per exception.
//
// The measurement has to be taken from *inside* the call: whatever a call holds is dropped
// when it returns, so sampling afterwards cannot tell the two behaviours apart. So the guest
// calls a host function after its last throw, and that host function collects and reports the
// live heap while the call is still on the stack.
func testEHPlainCatchHoldsNothingPerThrow(t *testing.T, cfg wazero.RuntimeConfig) {
	const throws = 200_000

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	var liveDuringCall uint64
	_, err := r.NewHostModuleBuilder("host").
		NewFunctionBuilder().WithFunc(func() {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		liveDuringCall = m.HeapAlloc
	}).Export("sample").
		Instantiate(ctx)
	require.NoError(t, err)

	mod, err := r.Instantiate(ctx, encodeModule(ehPlainCatchLoopModule()))
	require.NoError(t, err)
	loop := mod.ExportedFunction("throw_n")

	// Warm up, so the measurement covers steady state rather than first-call setup, and take
	// the baseline the same way the sample is taken.
	_, err = loop.Call(ctx, 1)
	require.NoError(t, err)
	baseline := liveDuringCall

	res, err := loop.Call(ctx, throws)
	require.NoError(t, err)
	require.Equal(t, int32(throws), api.DecodeI32(res[0]), "the loop ran to completion")

	growth := int64(liveDuringCall) - int64(baseline)
	// One held exception costs well over 32 bytes -- the object, its params, its trace -- so
	// a budget of 32 bytes per throw cannot be met while holding even a fraction of them,
	// and still leaves room for unrelated heap noise.
	require.True(t, growth < throws*32,
		"live heap grew by %d bytes over %d throws in one call: a plain catch should hold no exception per throw",
		growth, throws)
}

// ehPlainCatchLoopModule builds a module exporting "throw_n": (param i32) (result i32),
// which throws and catches n times in one call, by value, then calls the imported
// host.sample before returning how many it caught.
//
//	loop { block (result i32) { try_table (catch $t -> block) { call $thrower } unreachable }
//	       drop; i++; br_if loop (i < n) }
//	call $sample; i
func ehPlainCatchLoopModule() *wasm.Module {
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	return &wasm.Module{
		// $t carries an i32, so the handler has params to read through the buffer.
		TypeSection:         []wasm.FunctionType{{Params: i32}, {}, {Params: i32, Results: i32}},
		ImportSection:       []wasm.Import{{Module: "host", Name: "sample", Type: wasm.ExternTypeFunc, DescFunc: 1}},
		ImportFunctionCount: 1,
		TagSection:          []wasm.Tag{{Type: 0}},
		FunctionSection:     []wasm.Index{1, 2}, // func 1: thrower, func 2: throw_n
		ExportSection:       []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "throw_n", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeI32Const, 0x07, wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32}, // local 1: the count.
				Body: []byte{
					wasm.OpcodeLoop, 0x40,
					wasm.OpcodeBlock, byte(wasm.ValueTypeI32),
					// A catch clause's label is resolved outside the try_table, so 0x00 is the block.
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00,
					wasm.OpcodeCall, 0x01, // $thrower
					wasm.OpcodeEnd, // try_table: its body always throws, so this is unreachable.
					wasm.OpcodeUnreachable,
					wasm.OpcodeEnd,  // block: caught, with the tag's i32 on the stack.
					wasm.OpcodeDrop, // the caught param value.
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeI32Const, 0x01, wasm.OpcodeI32Add,
					wasm.OpcodeLocalSet, 0x01,
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeI32LtS, wasm.OpcodeBrIf, 0x00,
					wasm.OpcodeEnd,        // loop.
					wasm.OpcodeCall, 0x00, // host.sample, while this call is still on the stack.
					wasm.OpcodeLocalGet, 0x01,
					wasm.OpcodeEnd,
				},
			},
		},
	}
}

// TestExceptionHandlingCompilerRefCounting covers what the compiler has to release per
// iteration for a loop that catches by reference. A catch_all_ref hands guest code a handle,
// which is a reference the call has to hold -- but only for as long as guest code has it, so a
// loop that catches and lets go must not accumulate one exception per iteration.
//
// Compiler-only: the interpreter still holds every reference for the length of the call, so it
// grows here by design rather than by mistake.
func TestExceptionHandlingCompilerRefCounting(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	const iters = 200_000
	for _, tc := range []struct {
		name   string
		module func() *wasm.Module
	}{
		{"discarded from the stack", func() *wasm.Module { return ehCatchRefLoopModule(false) }},
		// A round trip through a local exercises the other three adjustments: the copy
		// local.get leaves on the stack, the move local.set makes, and the release of what
		// the local held from the previous iteration.
		{"round-tripped through a local", func() *wasm.Module { return ehCatchRefLoopModule(true) }},
		// A throw whose tag carries an exnref: the raise takes over the reference the param
		// was popped from, and hands it to the exception or drops it at the match.
		{"passed as an exnref tag param", ehThrowExnrefParamLoopModule},
		// A return_call passing an exnref: the frame is replaced, so the reference has to
		// travel into the callee's parameter rather than wait for a caller to release it.
		{"passed to a return_call", ehTailCallExnrefArgLoopModule},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
				WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling|
					experimental.CoreFeaturesTailCall))
			defer r.Close(ctx)

			// Sampled from inside the call: whatever a call holds is dropped when it returns,
			// so measuring afterwards cannot tell holding from releasing apart.
			var liveDuringCall uint64
			_, err := r.NewHostModuleBuilder("host").
				NewFunctionBuilder().WithFunc(func() {
				runtime.GC()
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				liveDuringCall = m.HeapAlloc
			}).Export("sample").
				Instantiate(ctx)
			require.NoError(t, err)

			mod, err := r.Instantiate(ctx, encodeModule(tc.module()))
			require.NoError(t, err)
			loop := mod.ExportedFunction("loop")

			_, err = loop.Call(ctx, 1)
			require.NoError(t, err)
			baseline := liveDuringCall

			res, err := loop.Call(ctx, iters)
			require.NoError(t, err)
			require.Equal(t, int32(iters), api.DecodeI32(res[0]), "the loop ran to completion")

			growth := int64(liveDuringCall) - int64(baseline)
			// A held exception runs to well over 64 bytes -- the object, its params, its
			// trace -- so this budget cannot be met while holding even a fraction of them.
			//
			// The floor is not zero: entering a try_table clones the stack 16 bytes larger
			// than the one it came from, and a catch keeps the clone, so a loop that catches
			// grows the heap by that much per try_table per iteration whatever it holds.
			require.True(t, growth < iters*64,
				"live heap grew by %d bytes over %d catch_all_ref iterations in one call: "+
					"a reference that guest code has let go of should not be held",
				growth, iters)
		})
	}
}

// ehCatchRefLoopModule builds a module exporting "loop": (param i32) (result i32), which
// throws and catches by reference n times in one call, discards the exnref each time, then
// calls the imported host.sample before returning how many it caught.
func ehCatchRefLoopModule(useLocal bool) *wasm.Module {
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	locals := []wasm.ValueType{wasm.ValueTypeI32} // local 1: the count.
	body := []byte{
		wasm.OpcodeLoop, 0x40,
		wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
		// A catch clause's label is resolved outside the try_table, so 0x00 is the block.
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
		wasm.OpcodeCall, 0x01, // $thrower
		wasm.OpcodeEnd, // try_table: its body always throws, so this is unreachable.
		wasm.OpcodeUnreachable,
		wasm.OpcodeEnd, // block: caught, with the exnref on the stack.
	}
	if useLocal {
		locals = append(locals, wasm.ValueTypeExnref) // local 2.
		body = append(body, wasm.OpcodeLocalSet, 0x02, wasm.OpcodeLocalGet, 0x02)
	}
	body = append(body,
		wasm.OpcodeDrop,
		wasm.OpcodeLocalGet, 0x01, wasm.OpcodeI32Const, 0x01, wasm.OpcodeI32Add, wasm.OpcodeLocalSet, 0x01,
		wasm.OpcodeLocalGet, 0x01, wasm.OpcodeLocalGet, 0x00, wasm.OpcodeI32LtS, wasm.OpcodeBrIf, 0x00,
		wasm.OpcodeEnd,        // loop.
		wasm.OpcodeCall, 0x00, // host.sample, while this call is still on the stack.
		wasm.OpcodeLocalGet, 0x01, wasm.OpcodeEnd,
	)
	return &wasm.Module{
		TypeSection:         []wasm.FunctionType{{}, {Params: i32, Results: i32}},
		ImportSection:       []wasm.Import{{Module: "host", Name: "sample", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		ImportFunctionCount: 1,
		TagSection:          []wasm.Tag{{Type: 0}},
		FunctionSection:     []wasm.Index{0, 1}, // func 1: thrower, func 2: loop
		ExportSection:       []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "loop", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{LocalTypes: locals, Body: body},
		},
	}
}

// ehThrowExnrefParamLoopModule is ehCatchRefLoopModule's shape, except the exnref the loop
// catches is then thrown again as the parameter of a second tag and caught by value. That makes
// the raise, rather than any stack slot or local, the thing holding the inner exception between
// the throw and the match.
func ehThrowExnrefParamLoopModule() *wasm.Module {
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	exnref := []wasm.ValueType{wasm.ValueTypeExnref}
	return &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{},                          // 0: () -> (), $inner and tag 0
			{Params: exnref},            // 1: (exnref) -> (), tag 1's type
			{Params: i32, Results: i32}, // 2: loop
		},
		ImportSection:       []wasm.Import{{Module: "host", Name: "sample", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		ImportFunctionCount: 1,
		TagSection:          []wasm.Tag{{Type: 0}, {Type: 1}},
		FunctionSection:     []wasm.Index{0, 2}, // func 1: $inner, func 2: loop
		ExportSection:       []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "loop", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32}, // local 1: the count.
				Body: []byte{
					wasm.OpcodeLoop, 0x40,
					// Catch tag 1 by value, which hands the handler the exnref it carries.
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
					// Catch tag 0 by reference and throw it again as tag 1's parameter.
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeCall, 0x01, // $inner
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeThrow, 0x01,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeDrop, // the exnref tag 1 carried.
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeI32Const, 0x01, wasm.OpcodeI32Add,
					wasm.OpcodeLocalSet, 0x01,
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeI32LtS, wasm.OpcodeBrIf, 0x00,
					wasm.OpcodeEnd,
					wasm.OpcodeCall, 0x00, // host.sample
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeEnd,
				},
			},
		},
	}
}

// ehTailCallExnrefArgLoopModule is ehCatchRefLoopModule's shape, except each iteration hands the
// exnref it caught to a return_call. The frame is replaced there, so the reference has to travel
// into the callee's parameter -- there is no later point in the caller to release it at.
func ehTailCallExnrefArgLoopModule() *wasm.Module {
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	exnref := []wasm.ValueType{wasm.ValueTypeExnref}
	return &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{},                             // 0: () -> (), $thrower and the tag
			{Params: exnref, Results: i32}, // 1: (exnref) -> i32, the tail-call target
			{Params: i32, Results: i32},    // 2: loop
			{Params: exnref, Results: i32}, // 3: $tailer, same shape as 1
		},
		ImportSection:       []wasm.Import{{Module: "host", Name: "sample", Type: wasm.ExternTypeFunc, DescFunc: 0}},
		ImportFunctionCount: 1,
		TagSection:          []wasm.Tag{{Type: 0}},
		// func 1: $thrower, func 2: $sink (the tail-call target), func 3: $tailer, func 4: loop
		FunctionSection: []wasm.Index{0, 1, 3, 2},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "loop", Index: 4}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			// $sink takes the exnref and returns 0. Its parameter is the reference's new owner.
			{Body: []byte{wasm.OpcodeI32Const, 0x00, wasm.OpcodeEnd}},
			// $tailer hands its own parameter straight on by return_call, so the reference
			// moves twice per iteration.
			{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeTailCallReturnCall, 0x02, wasm.OpcodeEnd}},
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeI32}, // local 1: the count.
				Body: []byte{
					wasm.OpcodeLoop, 0x40,
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeCall, 0x01, // $thrower
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeCall, 0x03, // $tailer, which return_calls $sink with it
					wasm.OpcodeDrop,
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeI32Const, 0x01, wasm.OpcodeI32Add,
					wasm.OpcodeLocalSet, 0x01,
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeI32LtS, wasm.OpcodeBrIf, 0x00,
					wasm.OpcodeEnd,
					wasm.OpcodeCall, 0x00, // host.sample
					wasm.OpcodeLocalGet, 0x01, wasm.OpcodeEnd,
				},
			},
		},
	}
}

// ehFrameEvent is one listener callback, including which module it was told the frame is in.
type ehFrameEvent struct{ kind, module, fn string }

type ehModuleRecordingListener struct{ events *[]ehFrameEvent }

func (l ehModuleRecordingListener) Before(_ context.Context, mod api.Module, def api.FunctionDefinition, _ []uint64, _ experimental.StackIterator) {
	*l.events = append(*l.events, ehFrameEvent{"before", mod.Name(), def.DebugName()})
}

func (l ehModuleRecordingListener) After(_ context.Context, mod api.Module, def api.FunctionDefinition, _ []uint64) {
	*l.events = append(*l.events, ehFrameEvent{"after", mod.Name(), def.DebugName()})
}

func (l ehModuleRecordingListener) Abort(_ context.Context, mod api.Module, def api.FunctionDefinition, _ error) {
	*l.events = append(*l.events, ehFrameEvent{"abort", mod.Name(), def.DebugName()})
}

// testEHListenerAbortModuleIdentity covers which module a listener is told a frame is in
// when an exception unwinds it across a module boundary.
//
// The abort for a frame comes from that frame's own propagate path, so it has to name the
// module the frame is running in -- the same one the frame's Before named. Naming whichever
// module last called into the runtime instead gets the callee's whenever the exception came
// from one, which is the usual case.
//
// The two engines disagree about which module a frame belongs to (the interpreter reports
// the module of the call the listener was entered through), so this checks the two
// callbacks agree with each other rather than pinning either answer.
func testEHListenerAbortModuleIdentity(t *testing.T, cfg wazero.RuntimeConfig) {
	var events []ehFrameEvent
	ctx := experimental.WithFunctionListenerFactory(context.Background(),
		experimental.FunctionListenerFactoryFunc(func(api.FunctionDefinition) experimental.FunctionListener {
			return ehModuleRecordingListener{&events}
		}))
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	// The inner module throws one of its own tag. Nothing catches it, so both frames unwind.
	inner := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "boom", Index: 0}},
		CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}}},
	}
	_, err := r.InstantiateWithConfig(ctx, encodeModule(inner),
		wazero.NewModuleConfig().WithName("inner"))
	require.NoError(t, err)

	// The outer module only calls it, so it has nothing to do with the raise but the frame
	// the exception passes through on its way out.
	outer := &wasm.Module{
		TypeSection: []wasm.FunctionType{{}},
		ImportSection: []wasm.Import{{
			Module: "inner", Name: "boom", Type: wasm.ExternTypeFunc, DescFunc: 0,
		}},
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "run", Index: 1}},
		CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}}},
	}
	outerMod, err := r.InstantiateWithConfig(ctx, encodeModule(outer),
		wazero.NewModuleConfig().WithName("outer"))
	require.NoError(t, err)

	_, err = outerMod.ExportedFunction("run").Call(ctx)
	require.Error(t, err) // uncaught.

	// ".$0" is the inner module's only function, ".$1" the outer module's, which follows the
	// function it imports.
	before := map[string]string{}
	var aborted []string
	for _, e := range events {
		switch e.kind {
		case "before":
			before[e.fn] = e.module
		case "abort":
			aborted = append(aborted, e.fn)
			require.Equal(t, before[e.fn], e.module,
				"%s was aborted as being in module %q, but entered as being in %q",
				e.fn, e.module, before[e.fn])
		}
	}
	require.Equal(t, []string{".$0", ".$1"}, aborted)
}

// testEHUncaughtStackTraceThroughTryTable is testEHUncaughtStackTrace with a non-matching
// try_table mid-stack. Looking for a handler is what unwinds frames, so a try_table between
// the throw and the entry is what can cost an engine the frames below it.
func testEHUncaughtStackTraceThroughTryTable(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	// $0 throws tag 0, $1 calls it while catching tag 1 only, $2 calls $1 and catches nothing.
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		TagSection:      []wasm.Tag{{Type: 0}, {Type: 0}},
		FunctionSection: []wasm.Index{0, 0, 0},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "a", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{
				wasm.OpcodeBlock, 0x40,
				wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
				wasm.OpcodeCall, 0x00,
				wasm.OpcodeEnd, wasm.OpcodeEnd, wasm.OpcodeEnd,
			}},
			{Body: []byte{wasm.OpcodeCall, 0x01, wasm.OpcodeEnd}},
		},
	}
	mod, err := r.Instantiate(ctx, encodeModule(m))
	require.NoError(t, err)

	_, err = mod.ExportedFunction("a").Call(ctx)
	require.Error(t, err)
	require.Equal(t, `wasm error: uncaught exception
wasm stack trace:
	.$0()
	.$1()
	.$2()`, err.Error())
}

// testEHRethrowStackTrace covers an uncaught exception that is not on its first
// propagation: A is thrown deep and caught by reference, B is thrown and caught inside that
// handler, and only then is A thrown again and left uncaught. The main trace describes
// where A is propagating from now; where it came from follows under its own header.
func testEHRethrowStackTrace(t *testing.T, cfg wazero.RuntimeConfig) {
	for _, tc := range []struct {
		name string
		// rethrow replaces the tail of $3, which holds the caught exnref in local 0.
		rethrow []byte
		// funcs are appended after $3, and expected is the whole error message.
		funcs    []wasm.Code
		expected string
	}{
		{
			// The rethrow is in the frame that caught it, so only that frame is live.
			name:    "same_frame",
			rethrow: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeThrowRef},
			expected: `wasm error: uncaught exception
wasm stack trace:
	.$3()
originally thrown at:
	.$0()
	.$2()
	.$3()`,
		},
		{
			// Handing the exnref to a callee that rethrows puts a frame under the raise,
			// so the trace has to reach the throw_ref rather than stop at $3.
			name:    "deeper_frame",
			rethrow: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeCall, 0x04},
			funcs: []wasm.Code{
				{Body: []byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeThrowRef, wasm.OpcodeEnd}},
			},
			// $4 takes the exnref, which api.ValueTypeName has no name for, as with funcref.
			expected: `wasm error: uncaught exception
wasm stack trace:
	.$4(unknown)
	.$3()
originally thrown at:
	.$0()
	.$2()
	.$3()`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, cfg)
			defer r.Close(ctx)

			// $0 throws A, $1 throws B, $2 calls $0, and $3 drives the whole sequence.
			m := &wasm.Module{
				TypeSection:     []wasm.FunctionType{{}, {Params: []wasm.ValueType{wasm.ValueTypeExnref}}},
				TagSection:      []wasm.Tag{{Type: 0}, {Type: 0}},
				FunctionSection: []wasm.Index{0, 0, 0, 0},
				ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "run", Index: 3}},
				CodeSection: []wasm.Code{
					{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
					{Body: []byte{wasm.OpcodeThrow, 0x01, wasm.OpcodeEnd}},
					{Body: []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}},
					{
						LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
						Body: append([]byte{
							// Catch A by reference out of $2 -> $0 and park it in local 0.
							wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
							wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
							wasm.OpcodeCall, 0x02,
							wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
							wasm.OpcodeLocalSet, 0x00,
							// Throw and catch B, entirely inside A's handler.
							wasm.OpcodeBlock, 0x40,
							wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
							wasm.OpcodeCall, 0x01,
							wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
							// Throw A again, with nothing left to catch it.
						}, append(tc.rethrow, wasm.OpcodeEnd)...),
					},
				},
			}
			for range tc.funcs {
				m.FunctionSection = append(m.FunctionSection, 1) // (exnref) -> ()
			}
			m.CodeSection = append(m.CodeSection, tc.funcs...)

			mod, err := r.Instantiate(ctx, encodeModule(m))
			require.NoError(t, err)

			_, err = mod.ExportedFunction("run").Call(ctx)
			require.Error(t, err)
			require.Equal(t, tc.expected, err.Error())
		})
	}
}

func requireEHCatchReturns42(t *testing.T, cfg wazero.RuntimeConfig, bin []byte) {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.Instantiate(ctx, bin)
	require.NoError(t, err)

	res, err := mod.ExportedFunction("a").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(42), api.DecodeI32(res[0]))
}

// buildEHPropagateModule builds:
//
//	$b   () -> ()      { throw $t }
//	$mid () -> ()      { call $b }   (or return_call $b when tailCall) -- no try, so it propagates
//	$a   () -> (i32)   { block { try_table(catch_all -> block){ call $mid }; i32.const 99; return }
//	                     i32.const 42 }
//
// $a returns 99 on the (unreached) normal path and 42 when it catches.
func buildEHPropagateModule(tailCall bool) []byte {
	m := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{}, // type0: () -> ()    ($b, $mid, tag)
			{Results: []wasm.ValueType{wasm.ValueTypeI32}}, // type1: () -> (i32)  ($a)
		},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 0, 1}, // $b=type0, $mid=type0, $a=type1
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "a", Index: 2}},
	}

	// $b: throw $t.
	bBody := []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}

	// $mid: (return_)call $b, in the entry block, with no try -> it propagates.
	midBody := []byte{wasm.OpcodeCall, 0x00, wasm.OpcodeEnd}
	if tailCall {
		midBody = []byte{wasm.OpcodeTailCallReturnCall, 0x00, wasm.OpcodeEnd}
	}

	// $a: try_table{ call $mid } catch_all -> returns 42.
	aBody := []byte{
		wasm.OpcodeBlock, 0x40, // block void ($caught lands after it)
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
		wasm.OpcodeCall, 0x01, // call $mid
		wasm.OpcodeEnd,                               // end try_table
		wasm.OpcodeI32Const, 0x63, wasm.OpcodeReturn, // i32.const 99; return (unreached)
		wasm.OpcodeEnd,            // end block (catch_all lands here)
		wasm.OpcodeI32Const, 0x2a, // i32.const 42
		wasm.OpcodeEnd,
	}

	m.CodeSection = []wasm.Code{{Body: bBody}, {Body: midBody}, {Body: aBody}}
	return encodeModule(m)
}

func ehLifetimeModule() []byte {
	const (
		fnThrow = iota // () -> ()          throws tag 0
		fnPark
		fnUse
		fnClear
		fnReadThenClear
		fnToTable
		fnFromTable
		fnFill
		fnCopy
		fnGrow
		fnNullTable
	)
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	// Catch tag 0 around a throw_ref of whatever is on the stack, yielding 1 when the
	// reference was still live enough to throw.
	rethrow := func(load []byte) []byte {
		body := []byte{wasm.OpcodeBlock, 0x40, wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00}
		body = append(body, load...)
		return append(body, wasm.OpcodeThrowRef, wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x00, wasm.OpcodeReturn, wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd)
	}
	// Catch by reference out of $throw, leaving the exnref on the stack.
	catchRef := []byte{
		wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
		wasm.OpcodeCall, fnThrow,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
	}
	refNull := []byte{wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref)}
	globalGet := []byte{wasm.OpcodeGlobalGet, 0x00}

	max := uint32(8)
	m := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{},                          // 0: () -> ()
			{Results: i32},              // 1: () -> i32
			{Params: i32},               // 2: (i32) -> ()
			{Params: i32, Results: i32}, // 3: (i32) -> i32
		},
		TableSection: []wasm.Table{{Min: 4, Max: &max, Type: wasm.ValueTypeExnref}},
		TagSection:   []wasm.Tag{{Type: 0}},
		GlobalSection: []wasm.Global{{
			Type: wasm.GlobalType{ValType: wasm.ValueTypeExnref, Mutable: true},
			Init: wasm.ConstantExpression{Data: refNull},
		}},
		FunctionSection: []wasm.Index{0, 1, 1, 0, 1, 2, 3, 0, 0, 1, 2},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			// park: catch by reference and stash it, reporting 1.
			{Body: append(append([]byte{}, catchRef...),
				wasm.OpcodeGlobalSet, 0x00, wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd)},
			// use: read it back and throw it again.
			{Body: rethrow(globalGet)},
			// clear.
			{Body: append(append([]byte{}, refNull...), wasm.OpcodeGlobalSet, 0x00, wasm.OpcodeEnd)},
			// read_then_clear: read into a local, clear the slot, then throw what was read.
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
				Body: append(append(append([]byte{},
					wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeLocalSet, 0x00),
					append(append([]byte{}, refNull...), wasm.OpcodeGlobalSet, 0x00)...),
					rethrow([]byte{wasm.OpcodeLocalGet, 0x00})...),
			},
			// to_table: table[param] = global 0.
			{Body: []byte{
				wasm.OpcodeLocalGet, 0x00, wasm.OpcodeGlobalGet, 0x00,
				wasm.OpcodeTableSet, 0x00, wasm.OpcodeEnd,
			}},
			// from_table: throw table[param] and catch it.
			{Body: rethrow([]byte{wasm.OpcodeLocalGet, 0x00, wasm.OpcodeTableGet, 0x00})},
			// fill: table[0..4) = global 0.
			{Body: []byte{
				wasm.OpcodeI32Const, 0x00, wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeI32Const, 0x04,
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableFill, 0x00, wasm.OpcodeEnd,
			}},
			// copy: seed table[1] as well, then table[2..4) = table[0..2).
			{Body: []byte{
				wasm.OpcodeI32Const, 0x01, wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeTableSet, 0x00,
				wasm.OpcodeI32Const, 0x02, wasm.OpcodeI32Const, 0x00, wasm.OpcodeI32Const, 0x02,
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableCopy, 0x00, 0x00, wasm.OpcodeEnd,
			}},
			// grow: two more slots holding global 0's reference.
			{Body: []byte{
				wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeI32Const, 0x02,
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableGrow, 0x00, wasm.OpcodeEnd,
			}},
			// null_table: table[param] = ref.null.
			{Body: []byte{
				wasm.OpcodeLocalGet, 0x00, wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref),
				wasm.OpcodeTableSet, 0x00, wasm.OpcodeEnd,
			}},
		},
	}
	for _, e := range []struct {
		name  string
		index wasm.Index
	}{
		{"park", fnPark},
		{"use", fnUse},
		{"clear", fnClear},
		{"read_then_clear", fnReadThenClear},
		{"to_table", fnToTable},
		{"from_table", fnFromTable},
		{"fill", fnFill},
		{"copy", fnCopy},
		{"grow", fnGrow},
		{"null_table", fnNullTable},
	} {
		m.ExportSection = append(m.ExportSection,
			wasm.Export{Type: wasm.ExternTypeFunc, Name: e.name, Index: e.index})
	}
	return encodeModule(m)
}

// ehLifetimeInstance instantiates ehLifetimeModule and returns a helper for calling its
// exports, which take at most one i32 and return at most one.
func ehLifetimeInstance(t *testing.T, cfg wazero.RuntimeConfig) (context.Context, func(name string, args ...uint64) (int32, error)) {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	t.Cleanup(func() { r.Close(ctx) })
	mod, err := r.Instantiate(ctx, ehLifetimeModule())
	require.NoError(t, err)
	return ctx, func(name string, args ...uint64) (int32, error) {
		res, err := mod.ExportedFunction(name).Call(ctx, args...)
		if err != nil || len(res) == 0 {
			return 0, err
		}
		return api.DecodeI32(res[0]), nil
	}
}

// testEHExnrefReleasedOnOverwrite is the other half: overwriting the slot with ref.null is
// what lets the exception go.
func testEHExnrefReleasedOnOverwrite(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx, call := ehLifetimeInstance(t, cfg)
	_ = ctx

	got, err := call("park")
	require.NoError(t, err)
	require.Equal(t, int32(1), got)

	got, err = call("use")
	require.NoError(t, err)
	require.Equal(t, int32(1), got)

	_, err = call("clear")
	require.NoError(t, err)

	// The global holds ref.null now, so throwing it is a null reference rather than a
	// stale one -- the exception itself is unreachable and gone.
	_, err = call("use")
	require.Error(t, err)
	require.Contains(t, err.Error(), "null reference")
}

// testEHExnrefReadThenOverwritten covers the window the read barrier closes: a handle read
// out of a slot stays usable for the rest of the call even if the slot is then cleared.
func testEHExnrefReadThenOverwritten(t *testing.T, cfg wazero.RuntimeConfig) {
	_, call := ehLifetimeInstance(t, cfg)

	_, err := call("park")
	require.NoError(t, err)

	got, err := call("read_then_clear")
	require.NoError(t, err)
	require.Equal(t, int32(1), got)
}

// testEHExnrefTableBulkOps covers the table operations that move references without going
// through table.set: each destination slot becomes a holder of what it now names.
//
// Every reference that got there by a path other than the operation under test is dropped
// before the check, so the surviving hold can only be the one that operation made. Without
// that the test passes on an engine whose bulk operations count nothing, since the
// table.set and the global still hold it.
func testEHExnrefTableBulkOps(t *testing.T, cfg wazero.RuntimeConfig) {
	for _, op := range []string{"fill", "copy", "grow"} {
		t.Run(op, func(t *testing.T) {
			_, call := ehLifetimeInstance(t, cfg)

			// Park it in the global and in table slot 0, the two barriered paths.
			_, err := call("park")
			require.NoError(t, err)
			_, err = call("to_table", 0)
			require.NoError(t, err)

			// Spread it with the operation under test: fill covers slots 0..3, copy takes
			// slots 0..1 to 2..3, and grow appends two more holding it.
			_, err = call(op)
			require.NoError(t, err)

			// Drop both barriered holds. If the operation counted nothing, the exception
			// is now unreachable and the slots it wrote name a released handle.
			_, err = call("clear")
			require.NoError(t, err)
			_, err = call("null_table", 0)
			require.NoError(t, err)

			slots := []uint64{1, 2, 3}
			if op == "grow" {
				slots = []uint64{4, 5} // the appended ones
			}
			for _, slot := range slots {
				got, err := call("from_table", slot)
				require.NoError(t, err, "slot %d", slot)
				require.Equal(t, int32(1), got, "slot %d", slot)
			}
		})
	}
}

// testEHExnrefConcurrentSlotAccess hammers one exnref global from several goroutines.
// Globals are shared across a store, which wazero allows concurrent calls into, so a
// barrier has to hold up against another one on the same slot. Run under -race.
func testEHExnrefConcurrentSlotAccess(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)
	mod, err := r.Instantiate(ctx, ehLifetimeModule())
	require.NoError(t, err)

	const goroutines, iterations = 4, 200
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine needs its own api.Function: Call is not goroutine-safe, but
			// the globals and tables underneath them are shared.
			park := mod.ExportedFunction("park")
			use := mod.ExportedFunction("use")
			clear := mod.ExportedFunction("clear")
			for i := 0; i < iterations; i++ {
				if _, err := park.Call(ctx); err != nil {
					t.Error(err)
					return
				}
				// Another goroutine may have cleared the global in between, so a null
				// reference is a legitimate outcome here; anything else is not.
				if _, err := use.Call(ctx); err != nil &&
					!strings.Contains(err.Error(), "null reference") {
					t.Error(err)
					return
				}
				if _, err := clear.Call(ctx); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// testEHExnrefParamSurvivesCall covers param edges: an exception carrying another as an
// exnref param is parked in a global, so only the param edge keeps the inner one reachable.
// A later call unpacks it and throws it.
func testEHExnrefParamSurvivesCall(t *testing.T, cfg wazero.RuntimeConfig) {
	const (
		fnThrowInner = iota // () -> ()      throws tag 0
		fnPark              // () -> i32     catch inner by ref, wrap it in tag 1, park that
		fnUnwrap            // () -> i32     read the wrapper back, unpack and throw the inner
		fnSameCall          // () -> i32     wrap and unpack within one call
	)
	refNull := []byte{wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref)}
	exnref := []wasm.ValueType{wasm.ValueTypeExnref}

	m := &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{},               // 0: () -> ()      tag 0's type
			{Params: exnref}, // 1: (exnref) -> () tag 1's type
			{Results: []wasm.ValueType{wasm.ValueTypeI32}}, // 2: () -> i32
		},
		TagSection:      []wasm.Tag{{Type: 0}, {Type: 1}},
		FunctionSection: []wasm.Index{0, 2, 2, 2},
		GlobalSection: []wasm.Global{{
			Type: wasm.GlobalType{ValType: wasm.ValueTypeExnref, Mutable: true},
			Init: wasm.ConstantExpression{Data: refNull},
		}},
		ExportSection: []wasm.Export{
			{Type: wasm.ExternTypeFunc, Name: "park", Index: fnPark},
			{Type: wasm.ExternTypeFunc, Name: "unwrap", Index: fnUnwrap},
			{Type: wasm.ExternTypeFunc, Name: "same_call", Index: fnSameCall},
		},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			// park: catch the inner exception by reference, throw tag 1 carrying it, catch
			// *that* by reference, and stash the wrapper. The inner one is then named only
			// by the wrapper's params. A block cannot see the operand stack outside it, so
			// the reference travels between them through a local.
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
				Body: []byte{
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeCall, fnThrowInner,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeLocalSet, 0x00,
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeThrow, 0x01, // throw tag 1, carrying the inner reference
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeGlobalSet, 0x00,
					wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
				},
			},
			// unwrap: read the wrapper back and throw it, catching it by tag so its param
			// comes out as a value, then throw that inner reference and catch it by its
			// own tag. Reaching the second throw at all means the param outlived the call
			// that parked the wrapper.
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
				Body: []byte{
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
					wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeThrowRef,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeLocalSet, 0x00,
					wasm.OpcodeBlock, 0x40,
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00,
					wasm.OpcodeLocalGet, 0x00, wasm.OpcodeThrowRef,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
				},
			},
			// same_call: wrap and unpack without the exception ever leaving the call. The
			// wrapper is caught by tag rather than by reference, so nothing ever names it;
			// its param still has to come out throwable.
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
				Body: []byte{
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeCall, fnThrowInner,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeLocalSet, 0x00,
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
					wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeThrow, 0x01, // throw tag 1, carrying the inner reference
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeLocalSet, 0x00,
					wasm.OpcodeBlock, 0x40,
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00,
					wasm.OpcodeLocalGet, 0x00, wasm.OpcodeThrowRef,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
				},
			},
		},
	}

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)
	mod, err := r.Instantiate(ctx, encodeModule(m))
	require.NoError(t, err)

	res, err := mod.ExportedFunction("park").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	// A later call: the inner exception is reachable only through the wrapper's params.
	res, err = mod.ExportedFunction("unwrap").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	res, err = mod.ExportedFunction("same_call").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// testEHExnrefAcrossModules covers an exception outliving the module that threw it. One
// module owns an exnref global, a second throws and parks a reference in it, and a third
// reads it back and throws it again -- after the thrower has been closed.
func testEHExnrefAcrossModules(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	refNull := []byte{wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref)}
	importSlot := []wasm.Import{{
		Module: "owner", Name: "slot", Type: wasm.ExternTypeGlobal,
		DescGlobal: wasm.GlobalType{ValType: wasm.ValueTypeExnref, Mutable: true},
	}}
	i32 := []wasm.ValueType{wasm.ValueTypeI32}

	// The owner exports a mutable exnref global for others to park references in.
	owner := &wasm.Module{
		TypeSection: []wasm.FunctionType{{}},
		GlobalSection: []wasm.Global{{
			Type: wasm.GlobalType{ValType: wasm.ValueTypeExnref, Mutable: true},
			Init: wasm.ConstantExpression{Data: refNull},
		}},
		ExportSection: []wasm.Export{{Type: wasm.ExternTypeGlobal, Name: "slot", Index: 0}},
	}
	_, err := r.InstantiateWithConfig(ctx, encodeModule(owner),
		wazero.NewModuleConfig().WithName("owner"))
	require.NoError(t, err)

	// The thrower throws one of its own tag and parks the reference in that global.
	thrower := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Results: i32}},
		ImportSection:   importSlot,
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "park", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{
				wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
				wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
				wasm.OpcodeCall, 0x00,
				wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
				wasm.OpcodeGlobalSet, 0x00,
				wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
			}},
		},
	}
	throwerMod, err := r.InstantiateWithConfig(ctx, encodeModule(thrower),
		wazero.NewModuleConfig().WithName("thrower"))
	require.NoError(t, err)
	res, err := throwerMod.ExportedFunction("park").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	// The reader never saw the throw and cannot name the thrower's tag, so it catches
	// everything.
	reader := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Results: i32}},
		ImportSection:   importSlot,
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "use", Index: 0}},
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeBlock, 0x40,
			wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
			wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeThrowRef,
			wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x00, wasm.OpcodeReturn, wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
		}}},
	}
	readerMod, err := r.InstantiateWithConfig(ctx, encodeModule(reader),
		wazero.NewModuleConfig().WithName("reader"))
	require.NoError(t, err)

	// The module that threw it goes away; the global still names the exception.
	require.NoError(t, throwerMod.Close(ctx))

	res, err = readerMod.ExportedFunction("use").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// testEHClosingOwnSlotKeepsOtherHolders closes a module whose own exnref global names an
// exception that a global elsewhere names too. Closing has to drop exactly the one naming
// it was responsible for: dropping neither leaks the exception for the life of the engine,
// and dropping both takes it out from under the holder that is still live, which then sees
// a handle it can no longer use.
//
// This is the case testEHExnrefAcrossModules does not reach, since there the closing module
// parked the reference only in an imported global -- a slot the module that defined it is
// responsible for, and so one closing skips.
func testEHClosingOwnSlotKeepsOtherHolders(t *testing.T, cfg wazero.RuntimeConfig) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	refNull := []byte{wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref)}
	exnrefGlobal := wasm.GlobalType{ValType: wasm.ValueTypeExnref, Mutable: true}
	importSlot := []wasm.Import{{
		Module: "owner", Name: "slot", Type: wasm.ExternTypeGlobal, DescGlobal: exnrefGlobal,
	}}
	i32 := []wasm.ValueType{wasm.ValueTypeI32}

	// The owner outlives everyone and keeps naming the exception throughout.
	owner := &wasm.Module{
		TypeSection: []wasm.FunctionType{{}},
		GlobalSection: []wasm.Global{{
			Type: exnrefGlobal, Init: wasm.ConstantExpression{Data: refNull},
		}},
		ExportSection: []wasm.Export{{Type: wasm.ExternTypeGlobal, Name: "slot", Index: 0}},
	}
	_, err := r.InstantiateWithConfig(ctx, encodeModule(owner),
		wazero.NewModuleConfig().WithName("owner"))
	require.NoError(t, err)

	// The holder throws, then parks the same reference twice: in the owner's global, and in
	// one of its own. Its own is the slot closing it is responsible for.
	holder := &wasm.Module{
		TypeSection:   []wasm.FunctionType{{}, {Results: i32}},
		ImportSection: importSlot,
		GlobalSection: []wasm.Global{{
			Type: exnrefGlobal, Init: wasm.ConstantExpression{Data: refNull},
		}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "park", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{
				LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref},
				Body: []byte{
					wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
					wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
					wasm.OpcodeCall, 0x00,
					wasm.OpcodeEnd, wasm.OpcodeUnreachable, wasm.OpcodeEnd,
					wasm.OpcodeLocalSet, 0x00,
					wasm.OpcodeLocalGet, 0x00, wasm.OpcodeGlobalSet, 0x00, // the owner's slot.
					wasm.OpcodeLocalGet, 0x00, wasm.OpcodeGlobalSet, 0x01, // its own.
					wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
				},
			},
		},
	}
	holderMod, err := r.InstantiateWithConfig(ctx, encodeModule(holder),
		wazero.NewModuleConfig().WithName("holder"))
	require.NoError(t, err)
	res, err := holderMod.ExportedFunction("park").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	// The reader only reaches the exception through the owner's global. It never saw the
	// throw and cannot name the holder's tag, so it catches everything.
	reader := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Results: i32}},
		ImportSection:   importSlot,
		FunctionSection: []wasm.Index{0},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "use", Index: 0}},
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeBlock, 0x40,
			wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
			wasm.OpcodeGlobalGet, 0x00, wasm.OpcodeThrowRef,
			wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x00, wasm.OpcodeReturn, wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0x01, wasm.OpcodeEnd,
		}}},
	}
	readerMod, err := r.InstantiateWithConfig(ctx, encodeModule(reader),
		wazero.NewModuleConfig().WithName("reader"))
	require.NoError(t, err)

	// Base case: while the holder is alive, two slots name the exception and the reader can
	// rethrow what the owner's global holds.
	res, err = readerMod.ExportedFunction("use").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	require.NoError(t, holderMod.Close(ctx))

	// The owner's global is untouched by that, so the reader must still be able to rethrow.
	// Over-releasing shows up here as ErrRuntimeExpiredExceptionRef.
	res, err = readerMod.ExportedFunction("use").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))

	// Closing the holder twice must not drop a second naming either.
	require.NoError(t, holderMod.Close(ctx))
	res, err = readerMod.ExportedFunction("use").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(1), api.DecodeI32(res[0]))
}

// TestExceptionHandlingCompilerReleasesOnEveryPath covers the operations that make an exnref
// stack slot cease to exist without being an ordinary drop or branch. Each one is a place the
// compiler has to emit a release, and each shape here leaked one reference per execution before
// it did.
//
// The check is that the call returns at all: the runtime asserts that a call holds no
// references once it is over, so a missed release fails here as "exnref reference(s) still held
// when the call returned" rather than as a number that has to be interpreted.
//
// Compiler-only, as with the other reference-counting tests: the interpreter holds every
// reference for the length of the call by design, so none of these can fail there.
// ehUnwindModule assembles a module exporting "main": (result i32), running body. It carries
// everything the unwind cases below need:
//
//	tag 0 () -- caught by reference by ehCaughtExnrefPrologue, which is what makes the exnrefs
//	tag 1 () -- thrown to escape a clause that catches tag 0, so the raise re-raises
//	tag 2 (exnref exnref), tag 3 (exnref) -- carry exnrefs as tag params
//	tag 4 (i32) -- carries a value a catch clause hands to its label
//	func 0: throw tag 0   func 1: throw tag 1
//	func 2: (param exnref) throw tag 0   func 3: throw tag 4 carrying 42
func ehUnwindModule(locals []wasm.ValueType, body []byte) *wasm.Module {
	exnref := []wasm.ValueType{wasm.ValueTypeExnref}
	twoExnref := []wasm.ValueType{wasm.ValueTypeExnref, wasm.ValueTypeExnref}
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	return &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{},
			{Results: i32},
			{Params: exnref},
			{Params: twoExnref},
			{Results: twoExnref},
			{Params: i32},
		},
		TagSection:      []wasm.Tag{{Type: 0}, {Type: 0}, {Type: 3}, {Type: 2}, {Type: 5}},
		FunctionSection: []wasm.Index{0, 0, 2, 0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 4}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeThrow, 0x01, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeI32Const, 42, wasm.OpcodeThrow, 0x04, wasm.OpcodeEnd}},
			{LocalTypes: locals, Body: body},
		},
	}
}

// TestExceptionHandlingUnwindReleasesEveryPath covers the operand stack slots a raise unwinds
// past, which no single point in the lowering sees all of: the landing pad releases what sat
// above the try_table, and the dispatch releases the rest -- down to the matched clause's
// label, or down to the enclosing raise target when nothing matched. A slot missed by both is
// a reference the call never lets go of; a slot released by both is one it lets go of twice.
func TestExceptionHandlingUnwindReleasesEveryPath(t *testing.T) {
	// A raise passing through a try_table whose clause does not match, on its way to one that
	// does. The exnref between the two try_tables' heights is above the outer and below the
	// inner, so it belongs to neither end of the unwind.
	reRaise := func(escape []byte) []byte {
		b := []byte{wasm.OpcodeBlock, 0x40} // $b, the outer catch's target
		b = append(b, wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00)
		b = append(b, ehCaughtExnrefPrologue...)
		b = append(b, wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00)
		b = append(b, escape...)
		return append(b,
			wasm.OpcodeEnd, wasm.OpcodeUnreachable,
			wasm.OpcodeEnd, wasm.OpcodeUnreachable,
			wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)
	}

	// The same, one level deeper, so the releases have to telescope.
	threeDeep := []byte{wasm.OpcodeBlock, 0x40}
	threeDeep = append(threeDeep, wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00)
	threeDeep = append(threeDeep, ehCaughtExnrefPrologue...)
	threeDeep = append(threeDeep, wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00)
	threeDeep = append(threeDeep, ehCaughtExnrefPrologue...)
	threeDeep = append(threeDeep,
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x00, 0x00,
		wasm.OpcodeCall, 0x01,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)

	// A catch whose label is a loop header, so it unwinds to the loop's height rather than a
	// block's. The counter makes the second time round leave.
	loopTarget := []byte{
		wasm.OpcodeBlock, byte(wasm.ValueTypeI32), // $done
		wasm.OpcodeLoop, 0x40, // $l
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeIf, 0x40,
		wasm.OpcodeI32Const, 42, wasm.OpcodeBr, 0x02,
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 0x01, wasm.OpcodeLocalSet, 0x00,
	}
	loopTarget = append(loopTarget, ehCaughtExnrefPrologue...)
	loopTarget = append(loopTarget,
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00, // -> $l
		wasm.OpcodeCall, 0x00,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable, // loop
		wasm.OpcodeEnd, // $done
		wasm.OpcodeEnd)

	// An exnref taken as the try_table's own block parameter, still on the stack at the raise.
	blockParam := []byte{wasm.OpcodeBlock, 0x40}
	blockParam = append(blockParam, ehCaughtExnrefPrologue...)
	blockParam = append(blockParam,
		wasm.OpcodeTryTable, 0x02, 0x01, wasm.CatchKindCatchAll, 0x00, // blocktype (param exnref)
		wasm.OpcodeCall, 0x01,
		wasm.OpcodeUnreachable,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)

	// One exception named twice by a tag's params, caught by value: the raise takes both
	// operands and the handler pushes both back.
	tagParamsTwice := []byte{wasm.OpcodeBlock, 0x04} // $b, results (exnref exnref)
	tagParamsTwice = append(tagParamsTwice, ehCaughtExnrefPrologue...)
	tagParamsTwice = append(tagParamsTwice,
		wasm.OpcodeLocalSet, 0x00,
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x02, 0x00,
		wasm.OpcodeLocalGet, 0x00, wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeThrow, 0x02,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeDrop, wasm.OpcodeDrop,
		wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)

	// catch_all hands nothing over, so the exnref the raise carried is dropped instead.
	tagParamDiscarded := []byte{wasm.OpcodeBlock, 0x40}
	tagParamDiscarded = append(tagParamDiscarded, ehCaughtExnrefPrologue...)
	tagParamDiscarded = append(tagParamDiscarded,
		wasm.OpcodeLocalSet, 0x00,
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
		wasm.OpcodeLocalGet, 0x00,
		wasm.OpcodeThrow, 0x03,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)

	// The reference moved into a callee's parameter, so the callee's propagate path owns it.
	paramToThrowingCallee := []byte{
		wasm.OpcodeBlock, 0x40,
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
	}
	paramToThrowingCallee = append(paramToThrowingCallee, ehCaughtExnrefPrologue...)
	paramToThrowingCallee = append(paramToThrowingCallee,
		wasm.OpcodeCall, 0x02,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)

	// A branch out of a try_table body discards slots on both sides of the try's height.
	branchOut := []byte{wasm.OpcodeBlock, 0x40}
	branchOut = append(branchOut, ehCaughtExnrefPrologue...)
	branchOut = append(branchOut, wasm.OpcodeTryTable, 0x40, 0x00)
	branchOut = append(branchOut, ehCaughtExnrefPrologue...)
	branchOut = append(branchOut,
		wasm.OpcodeBr, 0x01,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 42, wasm.OpcodeEnd)

	// A return out of a try_table body leaves the frame, so its locals go too.
	returnOut := append([]byte{}, ehCaughtExnrefPrologue...)
	returnOut = append(returnOut, wasm.OpcodeTryTable, 0x40, 0x00)
	returnOut = append(returnOut, ehCaughtExnrefPrologue...)
	returnOut = append(returnOut,
		wasm.OpcodeI32Const, 42, wasm.OpcodeReturn,
		wasm.OpcodeEnd, wasm.OpcodeUnreachable,
		wasm.OpcodeEnd)

	exnrefLocal := []wasm.ValueType{wasm.ValueTypeExnref}
	for _, tc := range []struct {
		name   string
		locals []wasm.ValueType
		body   []byte
	}{
		// The raise reaches the dispatch through a landing pad.
		{"re-raise past a slot", nil, reRaise([]byte{wasm.OpcodeCall, 0x01})},
		// ... and by a compiled jump, which is the other way in.
		{"re-raise past a slot from a direct throw", nil, reRaise([]byte{wasm.OpcodeThrow, 0x01})},
		{"re-raise through three levels", nil, threeDeep},
		{"catch unwinds to a loop header", []wasm.ValueType{wasm.ValueTypeI32}, loopTarget},
		{"try_table takes an exnref parameter", nil, blockParam},
		{"one exception as two tag params", exnrefLocal, tagParamsTwice},
		{"tag param discarded by catch_all", exnrefLocal, tagParamDiscarded},
		{"param handed to a callee that throws", nil, paramToThrowingCallee},
		{"branch out of a try body", nil, branchOut},
		{"return out of a try body", nil, returnOut},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := ehUnwindModule(tc.locals, tc.body)
			for _, e := range []struct {
				name string
				cfg  func() wazero.RuntimeConfig
			}{
				{"compiler", func() wazero.RuntimeConfig { return wazero.NewRuntimeConfigCompiler() }},
				{"interpreter", func() wazero.RuntimeConfig { return wazero.NewRuntimeConfigInterpreter() }},
			} {
				t.Run(e.name, func(t *testing.T) {
					if e.name == "compiler" && !platform.CompilerSupported() {
						t.Skip()
					}
					ctx := context.Background()
					r := wazero.NewRuntimeWithConfig(ctx, e.cfg().
						WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling))
					defer r.Close(ctx)
					mod, err := r.Instantiate(ctx, encodeModule(m))
					require.NoError(t, err)
					res, err := mod.ExportedFunction("main").Call(ctx)
					require.NoError(t, err)
					require.Equal(t, int32(42), api.DecodeI32(res[0]))
				})
			}
		})
	}
}

// TestExceptionHandlingUnwindEdgeCases covers three shapes the unwind reasoning covers only by
// argument: a br_table whose targets are a mix of ordinary labels and the function's own (each
// releases through a different path), an exnref held across a stack grow, and an overlapping
// table.copy between exnref slots.
func TestExceptionHandlingUnwindEdgeCases(t *testing.T) {
	i32 := []wasm.ValueType{wasm.ValueTypeI32}

	// br_table target 0 is a block, the default is the function label. Both discard an exnref,
	// but the block target goes through branchRange and the function target through the frame
	// exit, so a mistake in either shows up on only one arm.
	mixedBrTable := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Params: i32, Results: i32}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: func() []byte {
				b := []byte{wasm.OpcodeBlock, byte(wasm.ValueTypeI32)} // $b
				b = append(b, ehCaughtExnrefPrologue...)               // the exnref both arms discard
				return append(b,
					wasm.OpcodeI32Const, 42, // the value both labels carry
					wasm.OpcodeLocalGet, 0x00, // the selector
					wasm.OpcodeBrTable, 0x01, 0x00, 0x01, // [$b], default = function label
					wasm.OpcodeEnd, // $b
					wasm.OpcodeEnd)
			}()},
		},
	}

	// Each frame catches one by reference into a local and recurses, so several hundred live
	// references span the stack growth, and every frame releases its own on the way back.
	acrossStackGrow := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Params: i32, Results: i32}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref}, Body: func() []byte {
				b := append([]byte{}, ehCaughtExnrefPrologue...)
				return append(b,
					wasm.OpcodeLocalSet, 0x01, // held across the recursive call
					wasm.OpcodeLocalGet, 0x00,
					wasm.OpcodeIf, byte(wasm.ValueTypeI32),
					wasm.OpcodeLocalGet, 0x00, wasm.OpcodeI32Const, 0x01, wasm.OpcodeI32Sub,
					wasm.OpcodeCall, 0x01,
					wasm.OpcodeElse,
					wasm.OpcodeI32Const, 42,
					wasm.OpcodeEnd,
					wasm.OpcodeEnd)
			}()},
		},
	}

	// table.fill then an overlapping table.copy, both through the runtime's barriers.
	overlappingCopy := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Params: i32, Results: i32}},
		TagSection:      []wasm.Tag{{Type: 0}},
		TableSection:    []wasm.Table{{Min: 8, Type: wasm.ValueTypeExnref}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: func() []byte {
				b := []byte{wasm.OpcodeI32Const, 0x00} // fill offset
				b = append(b, ehCaughtExnrefPrologue...)
				return append(b,
					wasm.OpcodeI32Const, 0x04, // fill count: t[0..4) = X
					wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableFill, 0x00,
					// t[2..6) = t[0..4): the runs overlap, so the source has to be read first.
					wasm.OpcodeI32Const, 0x02, wasm.OpcodeI32Const, 0x00, wasm.OpcodeI32Const, 0x04,
					wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableCopy, 0x00, 0x00,
					// A slot the copy wrote must still name the exception.
					wasm.OpcodeI32Const, 0x05,
					wasm.OpcodeTableGet, 0x00,
					wasm.OpcodeRefIsNull,
					wasm.OpcodeIf, byte(wasm.ValueTypeI32),
					wasm.OpcodeI32Const, 0x00,
					wasm.OpcodeElse,
					wasm.OpcodeI32Const, 42,
					wasm.OpcodeEnd,
					wasm.OpcodeEnd)
			}()},
		},
	}

	for _, tc := range []struct {
		name string
		m    *wasm.Module
		arg  uint64
	}{
		{"br_table to a block label", mixedBrTable, 0},
		{"br_table to the function label", mixedBrTable, 1},
		{"exnrefs held across a stack grow", acrossStackGrow, 400},
		{"overlapping table.copy between exnref slots", overlappingCopy, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, e := range []struct {
				name string
				cfg  func() wazero.RuntimeConfig
			}{
				{"compiler", func() wazero.RuntimeConfig { return wazero.NewRuntimeConfigCompiler() }},
				{"interpreter", func() wazero.RuntimeConfig { return wazero.NewRuntimeConfigInterpreter() }},
			} {
				t.Run(e.name, func(t *testing.T) {
					if e.name == "compiler" && !platform.CompilerSupported() {
						t.Skip()
					}
					ctx := context.Background()
					r := wazero.NewRuntimeWithConfig(ctx, e.cfg().
						WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling))
					defer r.Close(ctx)
					mod, err := r.Instantiate(ctx, encodeModule(tc.m))
					require.NoError(t, err)
					res, err := mod.ExportedFunction("main").Call(ctx, tc.arg)
					require.NoError(t, err)
					require.Equal(t, int32(42), api.DecodeI32(res[0]))
				})
			}
		})
	}
}

// TestExceptionHandlingCompilerSnapshotRewindsExnrefs covers Snapshot/Restore across exnref
// activity. A restore rewinds the stack the references live in, so the reference table has to
// be rewound with it: the slots that took a reference are gone, and the releases that would
// have balanced them are on paths that no longer run.
//
// Compiler only. The interpreter's restore does not resume after the snapshot point at all,
// with or without exception handling, which is a separate matter.
func TestExceptionHandlingCompilerSnapshotRewindsExnrefs(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	// Catch one by reference and put it in local 0. Function 2 is the thrower.
	catchIntoLocal := []byte{
		wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
		wasm.OpcodeCall, 0x02,
		wasm.OpcodeEnd,
		wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
		wasm.OpcodeLocalSet, 0x00,
	}
	// Function 0 is host.snapshot, returning 0 the first time and 1 after the restore;
	// function 1 is host.restore, which does not return.
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{
			// The rewind takes away the local the reference went into, so the count it
			// added has to go too, or the call returns still holding it.
			"rewind discards a held reference",
			append(append([]byte{
				wasm.OpcodeCall, 0x00,
				wasm.OpcodeIf, 0x40,
				wasm.OpcodeI32Const, 42, wasm.OpcodeReturn,
				wasm.OpcodeEnd,
			}, catchIntoLocal...),
				wasm.OpcodeCall, 0x01,
				wasm.OpcodeI32Const, 0, wasm.OpcodeEnd),
		},
		{
			// The mirror image: the reference is let go of after the snapshot, so the
			// rewind brings it back and the local has to be usable again.
			"rewind revives a released reference",
			append(append([]byte{}, catchIntoLocal...),
				wasm.OpcodeCall, 0x00,
				wasm.OpcodeIf, 0x40,
				wasm.OpcodeLocalGet, 0x00, wasm.OpcodeDrop,
				wasm.OpcodeI32Const, 42, wasm.OpcodeReturn,
				wasm.OpcodeEnd,
				// Let go of it: the copy on the stack, then the local itself.
				wasm.OpcodeLocalGet, 0x00, wasm.OpcodeDrop,
				wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref), wasm.OpcodeLocalSet, 0x00,
				wasm.OpcodeCall, 0x01,
				wasm.OpcodeI32Const, 0, wasm.OpcodeEnd),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &wasm.Module{
				TypeSection: []wasm.FunctionType{{}, {Results: []wasm.ValueType{wasm.ValueTypeI32}}},
				ImportSection: []wasm.Import{
					{Module: "host", Name: "snapshot", Type: wasm.ExternTypeFunc, DescFunc: 1},
					{Module: "host", Name: "restore", Type: wasm.ExternTypeFunc, DescFunc: 0},
				},
				ImportFunctionCount: 2,
				TagSection:          []wasm.Tag{{Type: 0}},
				FunctionSection:     []wasm.Index{0, 1}, // 2: thrower, 3: run
				ExportSection:       []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "run", Index: 3}},
				CodeSection: []wasm.Code{
					{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
					{LocalTypes: []wasm.ValueType{wasm.ValueTypeExnref}, Body: tc.body},
				},
			}

			ctx := experimental.WithSnapshotter(context.Background())
			r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
				WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling))
			defer r.Close(ctx)

			var snap experimental.Snapshot
			_, err := r.NewHostModuleBuilder("host").
				NewFunctionBuilder().WithFunc(func(ctx context.Context) int32 {
				snap = experimental.GetSnapshotter(ctx).Snapshot()
				return 0
			}).Export("snapshot").
				NewFunctionBuilder().WithFunc(func(context.Context) {
				snap.Restore([]uint64{1})
			}).Export("restore").
				Instantiate(ctx)
			require.NoError(t, err)

			mod, err := r.Instantiate(ctx, encodeModule(m))
			require.NoError(t, err)

			res, err := mod.ExportedFunction("run").Call(ctx)
			require.NoError(t, err)
			require.Equal(t, int32(42), api.DecodeI32(res[0]))
		})
	}
}

func TestExceptionHandlingExnrefIsNotCallableFromHost(t *testing.T) {
	exnref := []wasm.ValueType{wasm.ValueTypeExnref}
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	// takes:    (param exnref) -> ()   -- not callable
	// gives:    () -> (exnref)         -- not callable
	// reachable: () -> (i32)           -- callable, and calls both of the above
	m := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: exnref}, {Results: exnref}, {Results: i32}},
		FunctionSection: []wasm.Index{0, 1, 2},
		ExportSection: []wasm.Export{
			{Type: wasm.ExternTypeFunc, Name: "takes", Index: 0},
			{Type: wasm.ExternTypeFunc, Name: "gives", Index: 1},
			{Type: wasm.ExternTypeFunc, Name: "reachable", Index: 2},
		},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref), wasm.OpcodeEnd}},
			{Body: []byte{
				wasm.OpcodeCall, 0x01, // gives
				wasm.OpcodeCall, 0x00, // takes
				wasm.OpcodeI32Const, 42, wasm.OpcodeEnd,
			}},
		},
	}

	for _, tc := range []struct {
		name string
		cfg  func() wazero.RuntimeConfig
	}{
		{"compiler", func() wazero.RuntimeConfig { return wazero.NewRuntimeConfigCompiler() }},
		{"interpreter", func() wazero.RuntimeConfig { return wazero.NewRuntimeConfigInterpreter() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "compiler" && !platform.CompilerSupported() {
				t.Skip()
			}
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, tc.cfg().
				WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling))
			defer r.Close(ctx)

			mod, err := r.Instantiate(ctx, encodeModule(m))
			require.NoError(t, err)

			// The definitions are still there to introspect: it is calling that is refused,
			// not the export.
			for _, c := range []struct {
				name string
				args []uint64
			}{
				{"takes", []uint64{0}}, // even `ref.null exn`, which is the only one the host could mean
				{"gives", nil},
			} {
				fn := mod.ExportedFunction(c.name)
				require.NotNil(t, fn, c.name)
				_, err = fn.Call(ctx, c.args...)
				require.Error(t, err, c.name)
				require.Contains(t, err.Error(), "exnref in its signature", c.name)
			}

			res, err := mod.ExportedFunction("reachable").Call(ctx)
			require.NoError(t, err)
			require.Equal(t, int32(42), api.DecodeI32(res[0]))
		})
	}
}

// TestExceptionHandlingHostFunctionRejectsExnref covers the same rule on the way in: a host
// function is entered from wasm and left back into it, so an exnref in its signature would be
// a handle handed to Go, which has no way to resolve one.
func TestExceptionHandlingHostFunctionRejectsExnref(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling))
	defer r.Close(ctx)

	for _, tc := range []struct {
		name            string
		params, results []api.ValueType
	}{
		{"parameter", []api.ValueType{byte(wasm.ValueTypeExnref)}, nil},
		{"result", nil, []api.ValueType{byte(wasm.ValueTypeExnref)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := r.NewHostModuleBuilder("host_"+tc.name).
				NewFunctionBuilder().
				WithGoFunction(api.GoFunc(func(context.Context, []uint64) {}), tc.params, tc.results).
				Export("f").
				Instantiate(ctx)
			require.Error(t, err)
			require.Contains(t, err.Error(), "exnref in its signature")
		})
	}
}

func TestExceptionHandlingCompilerReleasesOnEveryPath(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	for _, tc := range []struct {
		name   string
		module *wasm.Module
		want   int32
	}{
		// ref.is_null pops the exnref and pushes an i32, so the reference has nothing
		// downstream to live in and has to go at the test itself.
		{"ref.is_null consumes the operand", ehRefIsNullModule(), 0},
		// A branch unwinds the operand stack to its label's height. br_on_null is a branch
		// like br_if, so the slots above the label die on the taken edge.
		{"br_on_null discards a slot", ehBrOnNullDiscardModule(), 0},
		// br_on_non_null delivers one of the label's values from the operand it popped rather
		// than from the stack, so it discards one slot more than its label's arity suggests.
		{"br_on_non_null discards a slot", ehBrOnNonNullDiscardModule(), 0},
		// Leaving the frame releases its exnref locals, parameters included -- a parameter is
		// owned, its reference having moved out of the caller's stack slot at the call.
		{"br_if returns with an exnref parameter", ehBrIfReturnExnrefParamModule(), 7},
		// A br_table whose label vector is empty is lowered as a plain jump, which still
		// discards everything above its one target's height.
		{"empty br_table discards a slot", ehEmptyBrTableDiscardModule(), 0},
		// A catch clause branches like any other, so it discards the slots between its
		// label's height and the try_table's. Those sat under the try_table, so the raise
		// never abandoned them and the landing pad did not release them either.
		{"catch unwinds past a slot", ehCatchUnwindsStackSlotModule(), 0},
		// A catch label at the function's own depth leaves the frame, so it releases what
		// the frame holds -- here an exnref in a local and another on the operand stack --
		// and the tag's params become the function's results.
		{"catch unwinds the frame", ehCatchToFunctionLabelModule(), 42},
		// The same, with nothing on the operand stack where the raise happens: the frame
		// exit has fewer live slots than the function has results, which is only not a
		// contradiction because a catch's results do not come off the stack.
		{"catch unwinds an empty frame", ehCatchToFunctionLabelEmptyStackModule(), 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
				WithCoreFeatures(api.CoreFeaturesV2|experimental.CoreFeaturesExceptionHandling|
					experimental.CoreFeaturesTypedFunctionReferences))
			defer r.Close(ctx)

			mod, err := r.Instantiate(ctx, encodeModule(tc.module))
			require.NoError(t, err)
			res, err := mod.ExportedFunction("main").Call(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.want, api.DecodeI32(res[0]))
		})
	}
}

// ehCatchUnwindsStackSlotModule catches into a label below the try_table's own operand stack
// height, so taking it discards a caught exnref that was on the stack before the try_table was
// entered.
func ehCatchUnwindsStackSlotModule() *wasm.Module {
	body := []byte{wasm.OpcodeBlock, 0x40} // the catch target, taking no values
	body = append(body, ehCaughtExnrefPrologue...)
	return ehReleasePathModule(nil, append(body,
		// The catch label is resolved outside the try_table, so 0 is the block above, and
		// taking it unwinds past the exnref the prologue left on the stack.
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAll, 0x00,
		wasm.OpcodeCall, 0x00,
		wasm.OpcodeEnd, // try_table: the body always throws, so this is unreachable.
		wasm.OpcodeUnreachable,
		wasm.OpcodeEnd, // block
		wasm.OpcodeI32Const, 0x00, wasm.OpcodeEnd,
	))
}

// ehCatchTagParamModule assembles a module whose exported "main" runs body and returns an i32.
// Function 0 throws tag 0, which carries nothing; function 1 throws tag 1, which carries the
// i32 a catch clause hands to its label.
func ehCatchTagParamModule(locals []wasm.ValueType, body []byte) *wasm.Module {
	i32 := []wasm.ValueType{wasm.ValueTypeI32}
	return &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Params: i32}, {Results: i32}},
		TagSection:      []wasm.Tag{{Type: 0}, {Type: 1}},
		FunctionSection: []wasm.Index{0, 0, 2}, // 0: thrower, 1: i32 thrower, 2: main
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeI32Const, 42, wasm.OpcodeThrow, 0x01, wasm.OpcodeEnd}},
			{LocalTypes: locals, Body: body},
		},
	}
}

func ehCatchToFunctionLabelModule() *wasm.Module {
	body := append([]byte{}, ehCaughtExnrefPrologue...)
	return ehCatchTagParamModule([]wasm.ValueType{wasm.ValueTypeExnref}, append(body,
		wasm.OpcodeLocalTee, 0x00, // the local and the stack slot each hold a reference
		// Raise again. This catch's label is the function frame -- catch labels resolve
		// outside the try_table -- so taking it releases both of those and returns 42.
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
		wasm.OpcodeCall, 0x01,
		wasm.OpcodeEnd, // try_table: the body always throws, so this is unreachable.
		wasm.OpcodeUnreachable,
		wasm.OpcodeEnd,
	))
}

func ehCatchToFunctionLabelEmptyStackModule() *wasm.Module {
	return ehCatchTagParamModule(nil, []byte{
		wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatch, 0x01, 0x00,
		wasm.OpcodeCall, 0x01, // the raise point, with an empty operand stack
		wasm.OpcodeEnd,
		wasm.OpcodeI32Const, 0x00,
		wasm.OpcodeEnd,
	})
}

// ehCaughtExnrefPrologue leaves the exnref of a freshly caught exception on the operand stack.
// Function 0 is the thrower it calls.
var ehCaughtExnrefPrologue = []byte{
	wasm.OpcodeBlock, byte(wasm.ValueTypeExnref),
	wasm.OpcodeTryTable, 0x40, 0x01, wasm.CatchKindCatchAllRef, 0x00,
	wasm.OpcodeCall, 0x00,
	wasm.OpcodeEnd, // try_table: the body always throws, so this is unreachable.
	wasm.OpcodeUnreachable,
	wasm.OpcodeEnd, // block: caught, with the exnref on the stack.
}

// ehReleasePathModule assembles a module whose exported "main" runs body, with function 0 a
// thrower for ehCaughtExnrefPrologue to catch.
func ehReleasePathModule(locals []wasm.ValueType, body []byte) *wasm.Module {
	return &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}, {Results: []wasm.ValueType{wasm.ValueTypeI32}}},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 1}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{LocalTypes: locals, Body: body},
		},
	}
}

func ehRefIsNullModule() *wasm.Module {
	body := append([]byte{}, ehCaughtExnrefPrologue...)
	return ehReleasePathModule(nil, append(body,
		wasm.OpcodeRefIsNull, // consumes the exnref, yields an i32
		wasm.OpcodeEnd,
	))
}

func ehBrOnNullDiscardModule() *wasm.Module {
	body := []byte{wasm.OpcodeBlock, 0x40} // the branch target, taking no values
	body = append(body, ehCaughtExnrefPrologue...)
	return ehReleasePathModule(nil, append(body,
		wasm.OpcodeRefNull, byte(wasm.ValueTypeExnref),
		wasm.OpcodeBrOnNull, 0x00, // taken: unwinds past the caught exnref, discarding it
		wasm.OpcodeDrop, wasm.OpcodeDrop, // fall-through only
		wasm.OpcodeEnd, // block
		wasm.OpcodeI32Const, 0x00, wasm.OpcodeEnd,
	))
}

func ehBrOnNonNullDiscardModule() *wasm.Module {
	body := append([]byte{}, ehCaughtExnrefPrologue...)
	return ehReleasePathModule([]wasm.ValueType{wasm.ValueTypeExnref}, append(body,
		wasm.OpcodeLocalSet, 0x00,
		wasm.OpcodeBlock, byte(wasm.ValueTypeExnref), // the branch target, taking the ref
		wasm.OpcodeLocalGet, 0x00, // discarded when the branch is taken
		wasm.OpcodeLocalGet, 0x00, // delivered to the label
		wasm.OpcodeBrOnNonNull, 0x00,
		wasm.OpcodeEnd, // block: yields the discarded copy on the fall-through
		wasm.OpcodeDrop,
		wasm.OpcodeI32Const, 0x00, wasm.OpcodeEnd,
	))
}

func ehEmptyBrTableDiscardModule() *wasm.Module {
	body := []byte{wasm.OpcodeBlock, 0x40}
	body = append(body, ehCaughtExnrefPrologue...)
	return ehReleasePathModule(nil, append(body,
		wasm.OpcodeI32Const, 0x00,
		wasm.OpcodeBrTable, 0x00, 0x00, // empty label vector, default label 0
		wasm.OpcodeEnd, // block
		wasm.OpcodeI32Const, 0x00, wasm.OpcodeEnd,
	))
}

// ehBrIfReturnExnrefParamModule passes a caught exnref to a callee that returns by branching to
// its own function label. The callee owns its parameter, so that branch is what has to release
// it -- and nothing else in the callee mentions an exnref, which is what made it the case the
// compiler overlooked.
func ehBrIfReturnExnrefParamModule() *wasm.Module {
	body := append([]byte{}, ehCaughtExnrefPrologue...)
	return &wasm.Module{
		TypeSection: []wasm.FunctionType{
			{}, // 0: the thrower
			{Params: []wasm.ValueType{wasm.ValueTypeExnref}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
			{Results: []wasm.ValueType{wasm.ValueTypeI32}},
		},
		TagSection:      []wasm.Tag{{Type: 0}},
		FunctionSection: []wasm.Index{0, 1, 2},
		ExportSection:   []wasm.Export{{Type: wasm.ExternTypeFunc, Name: "main", Index: 2}},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeThrow, 0x00, wasm.OpcodeEnd}},
			{Body: []byte{
				wasm.OpcodeI32Const, 0x07, // the result the branch carries
				wasm.OpcodeI32Const, 0x01, // always taken
				wasm.OpcodeBrIf, 0x00,
				wasm.OpcodeEnd,
			}},
			{Body: append(body, wasm.OpcodeCall, 0x01, wasm.OpcodeEnd)},
		},
	}
}
