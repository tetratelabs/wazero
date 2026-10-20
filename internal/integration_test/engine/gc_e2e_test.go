package adhoc

// End-to-end test for wasm-gc using a real compiler output.
//
// hello_world_kt.wasm is a Kotlin/Wasm WASI binary that prints "Hello from
// Kotlin via WASI" and exercises wasm-gc structs, arrays, and casts throughout.
// Source: https://github.com/Kotlin/kotlin-wasm-wasi-template

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/gc_array_copy_get.wasm
var gcArrayCopyGetWasm []byte

//go:embed testdata/gc_typed_select.wasm
var gcTypedSelectWasm []byte

//go:embed testdata/gc_null_ref_subtype.wasm
var gcNullRefSubtypeWasm []byte

//go:embed testdata/gc_global_ref_global.wasm
var gcGlobalRefGlobalWasm []byte

//go:embed testdata/hello_world_kt.wasm
var helloWorldKtWasm []byte

// TestGcArrayCopyGet exercises array.new_default + array.copy + array.get_u
// on packed i16 arrays. This pattern appears in Binaryen-optimized Kotlin/Wasm
// output for string operations.
func TestGcArrayCopyGet(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, gcArrayCopyGetWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	res, err := mod.ExportedFunction("copy_and_get").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(66), api.DecodeI32(res[0]))
}

// TestGcTypedSelect verifies that typed select with a GC concrete ref type
// result compiles correctly. The inline type annotation (ref null $type) is
// multi-byte in the binary encoding, which caused the interpreter's compiler
// to desynchronize its PC and misread subsequent opcodes.
func TestGcTypedSelect(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, gcTypedSelectWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	// Create a point (10, 20), pass it as both args with b non-null.
	// select picks $a (since b is non-null → ref.is_null returns 0).
	// Wait — ref.is_null(b) = 0 (false), so select picks $b (second operand).
	// But $b == $a here, so result is 10 either way.
	fn := mod.ExportedFunction("select_ref")

	// Call with non-null b: allocate via a helper — but we can't create
	// GC structs from the host API yet. Test that the module at least
	// compiles and instantiates without the PC desync crash.
	require.NotNil(t, fn)
}

// TestGcNullRefSubtype verifies that ref.null with an abstract bottom type
// (none) in a global initializer is accepted as a subtype of a nullable
// concrete struct ref. This is the minimal repro for the const expression
// type mismatch that blocked the Kotlin/Wasm e2e test.
func TestGcNullRefSubtype(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, gcNullRefSubtypeWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	res, err := mod.ExportedFunction("set_and_get").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(10), api.DecodeI32(res[0]))
}

// TestGcGlobalRefGlobal verifies that a non-imported global's const
// expression can reference a previously defined global via global.get.
// This is valid under GC / extended-const but was rejected because
// validation only exposed imported globals to const expression evaluation.
func TestGcGlobalRefGlobal(t *testing.T) {
	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 | experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	mod, err := r.InstantiateWithConfig(ctx, gcGlobalRefGlobalWasm,
		wazero.NewModuleConfig().WithStartFunctions())
	require.NoError(t, err)

	res, err := mod.ExportedFunction("get_b_x").Call(ctx)
	require.NoError(t, err)
	require.Equal(t, int32(10), api.DecodeI32(res[0]))
}

func TestGcE2EKotlinWasmInterpreter(t *testing.T) {
	var stdout bytes.Buffer

	ctx := context.Background()
	cfg := wazero.NewRuntimeConfigInterpreter().
		WithCoreFeatures(api.CoreFeaturesV2 |
			experimental.CoreFeaturesExceptionHandling |
			experimental.CoreFeaturesGC)
	r := wazero.NewRuntimeWithConfig(ctx, cfg)
	defer r.Close(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	mod, err := r.InstantiateWithConfig(ctx, helloWorldKtWasm,
		wazero.NewModuleConfig().
			WithStdout(&stdout).
			WithStartFunctions())
	require.NoError(t, err)

	_, err = mod.ExportedFunction("_initialize").Call(ctx)
	require.NoError(t, err)
	require.Contains(t, stdout.String(), "Hello from Kotlin via WASI")
}
