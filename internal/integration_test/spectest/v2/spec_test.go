package v2

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/experimental/guardsig"
	"github.com/tetratelabs/wazero/internal/integration_test/spectest"
	"github.com/tetratelabs/wazero/internal/platform"
)

const enabledFeatures = api.CoreFeaturesV2

func TestCompiler(t *testing.T) {
	if !platform.CompilerSupported() {
		t.Skip()
	}
	spectest.Run(t, Testcases, context.Background(), wazero.NewRuntimeConfigCompiler().WithCoreFeatures(enabledFeatures))
}

func TestInterpreter(t *testing.T) {
	spectest.Run(t, Testcases, context.Background(), wazero.NewRuntimeConfigInterpreter().WithCoreFeatures(enabledFeatures))
}

// TestCompilerGuardPageMemory runs the whole suite with guard-page backed
// linear memory (no per-access bounds checks in the generated code); every
// out-of-bounds trap assertion must still hold, now via the hardware fault
// path (the guardsig signal handler redirecting the faulting thread to the
// guard-fault exit sequence).
func TestCompilerGuardPageMemory(t *testing.T) {
	if !platform.CompilerSupported() || !platform.GuardPageMemorySupported || !guardsig.Supported() {
		t.Skip()
	}
	ctx := experimental.WithGuardPageMemory(context.Background())
	spectest.Run(t, Testcases, ctx, wazero.NewRuntimeConfigCompiler().WithCoreFeatures(enabledFeatures))
}
