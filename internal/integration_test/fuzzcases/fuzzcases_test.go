package fuzzcases

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

var ctx = context.Background()

var (
	//go:embed testdata/695.wasm
	case695 []byte
	//go:embed testdata/696.wasm
	case696 []byte
	//go:embed testdata/699.wasm
	case699 []byte
	//go:embed testdata/701.wasm
	case701 []byte
	//go:embed testdata/704.wasm
	case704 []byte
)

func newRuntimeCompiler() wazero.Runtime {
	return wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigCompiler().WithWasmCore2())
}

func newRuntimeInterpreter() wazero.Runtime {
	return wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter().WithWasmCore2())
}

// Test695 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test695(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	for _, tc := range []struct {
		name string
		r    wazero.Runtime
	}{
		{name: "compiler", r: newRuntimeCompiler()},
		{name: "interpreter", r: newRuntimeInterpreter()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer tc.r.Close(ctx)
			module, err := tc.r.InstantiateModuleFromBinary(ctx, case695)
			require.NoError(t, err)

			_, err = module.ExportedFunction("i8x16s").Call(ctx)
			require.Contains(t, err.Error(), "out of bounds memory access")

			_, err = module.ExportedFunction("i16x8s").Call(ctx)
			require.Contains(t, err.Error(), "out of bounds memory access")
		})
	}
}

func Test696(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	functionNames := [4]string{
		"select with 0 / after calling dummy",
		"select with 0",
		"typed select with 1 / after calling dummy",
		"typed select with 1",
	}

	for _, tc := range []struct {
		name string
		r    wazero.Runtime
	}{
		{name: "compiler", r: newRuntimeCompiler()},
		{name: "interpreter", r: newRuntimeInterpreter()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer tc.r.Close(ctx)
			module, err := tc.r.InstantiateModuleFromBinary(ctx, case696)
			require.NoError(t, err)

			for _, name := range functionNames {
				_, err := module.ExportedFunction(name).Call(ctx)
				require.NoError(t, err)
			}
		})
	}
}

// Test699 ensures that accessing element instances and data instances works
// without crash even when the access happens in the nested function call.
func Test699(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	for _, tc := range []struct {
		name string
		r    wazero.Runtime
	}{
		{name: "compiler", r: newRuntimeCompiler()},
		{name: "interpreter", r: newRuntimeInterpreter()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer tc.r.Close(ctx)
			_, err := tc.r.InstantiateModuleFromBinary(ctx, case699)
			require.NoError(t, err)
		})
	}
}

// Test701 requires two functions to exit with "out of bounds memory access" consistently across the implementations.
func Test701(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	for _, tc := range []struct {
		name string
		r    wazero.Runtime
	}{
		{name: "compiler", r: newRuntimeCompiler()},
		{name: "interpreter", r: newRuntimeInterpreter()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer tc.r.Close(ctx)
			module, err := tc.r.InstantiateModuleFromBinary(ctx, case701)
			require.NoError(t, err)

			_, err = module.ExportedFunction("i32.extend16_s").Call(ctx)
			require.Contains(t, err.Error(), "out of bounds memory access")

			_, err = module.ExportedFunction("i32.extend8_s").Call(ctx)
			require.Contains(t, err.Error(), "out of bounds memory access")
		})
	}
}

func Test704(t *testing.T) {
	if !platform.CompilerSupported() {
		return
	}

	for _, tc := range []struct {
		name string
		r    wazero.Runtime
	}{
		{name: "compiler", r: newRuntimeCompiler()},
		{name: "interpreter", r: newRuntimeInterpreter()},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			defer tc.r.Close(ctx)
			_, err := tc.r.InstantiateModuleFromBinary(ctx, case704)
			require.NoError(t, err)
		})
	}
}
