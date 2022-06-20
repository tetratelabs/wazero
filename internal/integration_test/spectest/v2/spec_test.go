package spectest

import (
	"embed"
	"path"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/engine/interpreter"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var testcases embed.FS

const enabledFeatures = wasm.Features20220419

func TestCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}

	spectest.Run(t, testcases, compiler.NewEngine, enabledFeatures, func(jsonname string) bool {
		switch path.Base(jsonname) {
		case "simd_f64x2_pmin_pmax.json", "simd_f32x4_pmin_pmax.json",
			"simd_i32x4_dot_i16x8.json", "simd_i32x4_trunc_sat_f32x4.json",
			"simd_splat.json", "simd_load.json", "simd_i32x4_trunc_sat_f64x2.json",
			"simd_conversions.json":
			// TODO: implement on arm64.
			return runtime.GOARCH == "amd64"
		default:
			return true
		}
	})
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, testcases, interpreter.NewEngine, enabledFeatures, func(string) bool { return true })
}
