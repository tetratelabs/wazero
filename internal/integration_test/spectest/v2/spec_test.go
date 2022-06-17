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
		case "simd_i8x16_cmp.json", "simd_i16x8_cmp.json", "simd_i32x4_cmp.json", "simd_i64x2_cmp.json",
			"simd_f32x4_cmp.json", "simd_f64x2_cmp.json", "simd_f32x4_arith.json", "simd_f64x2_arith.json",
			"simd_i16x8_arith.json", "simd_i64x2_arith.json", "simd_i32x4_arith.json", "simd_i8x16_arith.json",
			"simd_i16x8_sat_arith.json", "simd_i8x16_sat_arith.json",
			"simd_i16x8_arith2.json", "simd_i8x16_arith2.json", "simd_i32x4_arith2.json", "simd_i64x2_arith2.json",
			"simd_f64x2.json", "simd_f32x4.json", "simd_f32x4_rounding.json", "simd_f64x2_rounding.json",
			"simd_f64x2_pmin_pmax.json", "simd_f32x4_pmin_pmax.json", "simd_int_to_int_extend.json",
			"simd_i64x2_extmul_i32x4.json", "simd_i32x4_extmul_i16x8.json", "simd_i16x8_extmul_i8x16.json",
			"simd_i16x8_q15mulr_sat_s.json", "simd_i16x8_extadd_pairwise_i8x16.json", "simd_i32x4_extadd_pairwise_i16x8.json",
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
