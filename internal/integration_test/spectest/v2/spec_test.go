package spectest

import (
	"embed"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/compiler"
	"github.com/tetratelabs/wazero/internal/engine/interpreter"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var testcases embed.FS

const enabledFeatures = wasm.Features20220419

func TestCompiler(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip()
	}

	spectest.Run(t, testcases, compiler.NewEngine, enabledFeatures, func(jsonname string) bool {
		// TODO: remove after SIMD proposal
		if strings.Contains(jsonname, "simd") {
			switch path.Base(jsonname) {
			case "simd_address.json":
			case "simd_const.json":
			case "simd_align.json":
			case "simd_load16_lane.json":
			case "simd_load32_lane.json":
			case "simd_load64_lane.json":
			case "simd_load8_lane.json":
			case "simd_lane.json":
			case "simd_load_extend.json":
			case "simd_load_splat.json":
			case "simd_load_zero.json":
			case "simd_store.json":
			case "simd_store16_lane.json":
			case "simd_store32_lane.json":
			case "simd_store64_lane.json":
			case "simd_store8_lane.json":
			case "simd_bitwise.json":
			case "simd_boolean.json":
			default:
				return false // others not supported, yet!
			}
			return true
		}
		return true
	})
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, testcases, interpreter.NewEngine, enabledFeatures, func(jsonname string) bool {
		// TODO: remove after SIMD proposal
		if strings.Contains(jsonname, "simd") {
			switch path.Base(jsonname) {
			case "simd_address.json":
			case "simd_const.json":
			case "simd_align.json":
			case "simd_load16_lane.json":
			case "simd_load32_lane.json":
			case "simd_load64_lane.json":
			case "simd_load8_lane.json":
			case "simd_lane.json":
			case "simd_load_extend.json":
			case "simd_load_splat.json":
			case "simd_load_zero.json":
			case "simd_store.json":
			case "simd_store16_lane.json":
			case "simd_store32_lane.json":
			case "simd_store64_lane.json":
			case "simd_store8_lane.json":
			case "simd_bitwise.json":
			case "simd_boolean.json":
			default:
				return false // others not supported, yet!
			}
			return true
		}
		return true
	})
}
