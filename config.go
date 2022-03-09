package wazero

import (
	"context"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
)

// NewRuntimeConfigJIT compiles WebAssembly modules into runtime.GOARCH-specific assembly for optimal performance.
//
// Note: This panics at runtime the runtime.GOOS or runtime.GOARCH does not support JIT. Use NewRuntimeConfig to safely
// detect and fallback to NewRuntimeConfigInterpreter if needed.
func NewRuntimeConfigJIT() *RuntimeConfig {
	return &RuntimeConfig{
		engine:          jit.NewEngine(),
		ctx:             context.Background(),
		enabledFeatures: internalwasm.Features20191205,
	}
}

// NewRuntimeConfigInterpreter interprets WebAssembly modules instead of compiling them into assembly.
func NewRuntimeConfigInterpreter() *RuntimeConfig {
	return &RuntimeConfig{
		engine:          interpreter.NewEngine(),
		ctx:             context.Background(),
		enabledFeatures: internalwasm.Features20191205,
	}
}

// RuntimeConfig controls runtime behavior, with the default implementation as NewRuntimeConfig
type RuntimeConfig struct {
	engine          internalwasm.Engine
	ctx             context.Context
	enabledFeatures internalwasm.Features
}

// WithContext sets the default context used to initialize the module. Defaults to context.Background if nil.
//
// Notes:
// * If the Module defines a start function, this is used to invoke it.
// * This is the outer-most ancestor of wasm.Module Context() during wasm.Function invocations.
// * This is the default context of wasm.Function when callers pass nil.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-function%E2%91%A0
func (r *RuntimeConfig) WithContext(ctx context.Context) *RuntimeConfig {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RuntimeConfig{engine: r.engine, ctx: ctx, enabledFeatures: r.enabledFeatures}
}

// WithFeatureMutableGlobal allows globals to be mutable. This defaults to true as the feature was finished in
// WebAssembly 1.0 (20191205).
//
// When false, a wasm.Global can never be cast to a wasm.MutableGlobal, and any source that includes global vars
// will fail to parse.
//
func (r *RuntimeConfig) WithFeatureMutableGlobal(enabled bool) *RuntimeConfig {
	enabledFeatures := r.enabledFeatures.Set(internalwasm.FeatureMutableGlobal, enabled)
	return &RuntimeConfig{engine: r.engine, ctx: r.ctx, enabledFeatures: enabledFeatures}
}

// WithFeatureSignExtensionOps enables sign-extend operations. This defaults to false as the feature was not finished in
// WebAssembly 1.0 (20191205).
//
// See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
func (r *RuntimeConfig) WithFeatureSignExtensionOps(enabled bool) *RuntimeConfig {
	enabledFeatures := r.enabledFeatures.Set(internalwasm.FeatureSignExtensionOps, enabled)
	return &RuntimeConfig{engine: r.engine, ctx: r.ctx, enabledFeatures: enabledFeatures}
}

// Module is a WebAssembly 1.0 (20191205) module to instantiate.
type Module struct {
	name   string
	module *internalwasm.Module
}

// WithName returns a new instance which overrides the name.
func (m *Module) WithName(moduleName string) *Module {
	return &Module{name: moduleName, module: m.module}
}
