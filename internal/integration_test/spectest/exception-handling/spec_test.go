package spectest

import (
	"context"
	"embed"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/platform"
)

//go:embed testdata
var testcases embed.FS

const enabledFeatures = api.CoreFeaturesV2 | experimental.CoreFeaturesExceptionHandling | experimental.CoreFeaturesTailCall | experimental.CoreFeaturesTypedFunctionReferences

func TestCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	ctx := context.Background()
	config := wazero.NewRuntimeConfigCompiler().WithCoreFeatures(enabledFeatures)
	runCases(t, ctx, config)
}

func TestInterpreter(t *testing.T) {
	ctx := context.Background()
	config := wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(enabledFeatures)
	runCases(t, ctx, config)
}

func runCases(t *testing.T, ctx context.Context, config wazero.RuntimeConfig) {
	spectest.RunCase(t, testcases, "throw", ctx, config, -1, 0, math.MaxInt)
	spectest.RunCase(t, testcases, "throw_ref", ctx, config, -1, 0, math.MaxInt)
	spectest.RunCase(t, testcases, "tag", ctx, config, -1, 0, math.MaxInt)
	spectest.RunCase(t, testcases, "try_table", ctx, config, -1, 0, math.MaxInt)
}
