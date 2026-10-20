package adhoc

import (
	"context"
	_ "embed"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/gc_nested_ref.wasm
var gcNestedRefWasm []byte

//go:embed testdata/gc_alloc_many.wasm
var gcAllocManyWasm []byte

// TestGcNestedRefSurvivesGC validates that a struct reachable only through
// another struct's field survives Go's garbage collector.
//
// After "setup", the GC sweep keeps only $outer in GCRoots (from the
// global). $inner is NOT in GCRoots — it's only reachable through
// $outer's field. If fields store Go pointers, Go traces the graph and
// keeps $inner alive. If fields store uint64, $inner is collected.
//
// We detect collection via runtime.SetFinalizer: if the finalizer fires,
// $inner was collected prematurely.
func TestGcNestedRefSurvivesGC(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, gcNestedRefWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	_, err = mod.ExportedFunction("setup").Call(ctx)
	require.NoError(t, err)

	// After setup + sweep at Call() return, GCRoots has only $outer
	// (from the global). $inner is removed from GCRoots.
	// Fish out the inner struct through the global → outer → field.
	mi := mod.(*wasm.ModuleInstance)
	outerTagged := mi.Globals[0].Val
	outer := (*wasm.WasmStruct)(wasm.UntagGCPointer(outerTagged))

	// The field stores inner — either as *WasmStruct (Go pointer, after fix)
	// or as uint64 (tagged pointer, before fix). Extract the *WasmStruct
	// either way so we can set a finalizer.
	var inner *wasm.WasmStruct
	switch v := outer.Fields[0].(type) {
	case *wasm.WasmStruct:
		inner = v
	case uint64:
		inner = (*wasm.WasmStruct)(wasm.UntagGCPointer(v))
	}

	var finalized atomic.Bool
	runtime.SetFinalizer(inner, func(*wasm.WasmStruct) {
		finalized.Store(true)
	})
	inner = nil // clear local ref; only the field (if Go-traceable) keeps it alive

	for i := 0; i < 10; i++ {
		runtime.GC()
		runtime.Gosched()
	}

	if finalized.Load() {
		t.Fatal("inner struct was collected by Go's GC despite being " +
			"reachable through outer's field — fields must store Go pointers")
	}

	// Also verify the value is readable through wasm.
	res, err := mod.ExportedFunction("read_inner").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(42), api.DecodeI32(res[0]))
}

// TestGcDeadObjectsCollected validates that the GC sweep removes dead objects
// from GCRoots. Without the sweep, 100K structs accumulate; with it, periodic
// sweeps during execution keep only stack-live objects.
func TestGcDeadObjectsCollected(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, gcAllocManyWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	_, err = mod.ExportedFunction("allocate_many").Call(ctx)
	require.NoError(t, err)

	mi := mod.(*wasm.ModuleInstance)
	gcRootsLen := len(mi.GCRoots)

	if gcRootsLen > 100 {
		t.Errorf("GCRoots has %d entries after allocate_many, expected ≤ 100 "+
			"(sweep should have removed dead objects)", gcRootsLen)
	}
}
