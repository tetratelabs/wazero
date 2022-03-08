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
// When false, a wasm.Global can never be cast to a wasm.MutableGlobal, and any source that includes mutable globals
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

// DecodedModule is a WebAssembly 1.0 (20191205) text or binary encoded module to instantiate.
type DecodedModule struct {
	name   string
	module *internalwasm.Module
}

// WithName returns a new instance which overrides the name.
func (m *DecodedModule) WithName(moduleName string) *DecodedModule {
	return &DecodedModule{name: moduleName, module: m.module}
}

// HostModuleConfig are WebAssembly 1.0 (20191205) exports from the host bound to a module name used by InstantiateHostModule.
type HostModuleConfig struct {
	// Name is the module name that these exports can be imported with. Ex. wasi.ModuleSnapshotPreview1
	Name string

	// Functions adds functions written in Go, which a WebAssembly Module can import.
	//
	// The key is the name to export and the value is the func. Ex. WASISnapshotPreview1
	//
	// Noting a context exception described later, all parameters or result types must match WebAssembly 1.0 (20191205) value
	// types. This means uint32, uint64, float32 or float64. Up to one result can be returned.
	//
	// Ex. This is a valid host function:
	//
	//	addInts := func(x uint32, uint32) uint32 {
	//		return x + y
	//	}
	//
	// Host functions may also have an initial parameter (param[0]) of type context.Context or wasm.Module.
	//
	// Ex. This uses a Go Context:
	//
	//	addInts := func(ctx context.Context, x uint32, uint32) uint32 {
	//		// add a little extra if we put some in the context!
	//		return x + y + ctx.Value(extraKey).(uint32)
	//	}
	//
	// The most sophisticated context is wasm.Module, which allows access to the Go context, but also
	// allows writing to memory. This is important because there are only numeric types in Wasm. The only way to share other
	// data is via writing memory and sharing offsets.
	//
	// Ex. This reads the parameters from!
	//
	//	addInts := func(ctx wasm.Module, offset uint32) uint32 {
	//		x, _ := ctx.Memory().ReadUint32Le(offset)
	//		y, _ := ctx.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
	//		return x + y
	//	}
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#host-functions%E2%91%A2
	Functions map[string]interface{}
}
