package wazero

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
	"github.com/tetratelabs/wazero/internal/wasm/text"
	"github.com/tetratelabs/wazero/wasm"
)

type Engine struct {
	e internalwasm.Engine
}

func NewEngineInterpreter() *Engine {
	return &Engine{e: interpreter.NewEngine()}
}

func NewEngineJIT() *Engine { // TODO: compiler?
	return &Engine{e: jit.NewEngine()}
}

// StoreConfig allows customization of a Store via NewStoreWithConfig
type StoreConfig struct {
	// Context is the default context used to initialize the module. Defaults to context.Background.
	//
	// Notes:
	// * If the Module defines a start function, this is used to invoke it.
	// * This is the outer-most ancestor of wasm.ModuleContext Context() during wasm.HostFunction invocations.
	// * This is the default context of wasm.Function when callers pass nil.
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#start-function%E2%91%A0
	Context context.Context
	// Engine defaults to NewEngineInterpreter
	Engine *Engine
}

func NewStore() wasm.Store {
	return internalwasm.NewStore(context.Background(), interpreter.NewEngine())
}

// NewStoreWithConfig returns a store with the given configuration.
func NewStoreWithConfig(config *StoreConfig) wasm.Store {
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}
	engine := config.Engine
	if engine == nil {
		engine = NewEngineInterpreter()
	}
	return internalwasm.NewStore(ctx, engine.e)
}

// ModuleConfig defines the WebAssembly 1.0 (20191205) module to instantiate.
type ModuleConfig struct {
	// Name defaults to what's decoded from the custom name section and can be overridden WithName.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#name-section%E2%91%A0
	Name string
	// Source is the WebAssembly 1.0 (20191205) text or binary encoding of the module.
	Source []byte

	validatedSource []byte
	decodedModule   *internalwasm.Module
}

// Validate eagerly decodes the Source and errs if it is invalid.
//
// This is used to pre-flight check and cache the module for later instantiation.
func (m *ModuleConfig) Validate() (err error) {
	mod, err := decodeModule(m)
	if err != nil {
		return
	}
	// TODO: decoders should validate before returning, as that allows
	// them to err with the correct source position.
	err = mod.Validate()
	return
}

// WithName returns a new instance which overrides the Name, but keeps any internal cache made by Validate.
func (m *ModuleConfig) WithName(moduleName string) *ModuleConfig {
	return &ModuleConfig{
		Name:            moduleName,
		Source:          m.Source,
		validatedSource: m.validatedSource,
		decodedModule:   m.decodedModule,
	}
}

// InstantiateModule instantiates the module namespace or errs if the configuration was invalid.
//
// Ex.
//	exports, _ := wazero.InstantiateModule(wazero.NewStore(), &wazero.ModuleConfig{Source: wasm})
//
// Note: StoreConfig.Context is used for any WebAssembly 1.0 (20191205) Start Function.
func InstantiateModule(store wasm.Store, module *ModuleConfig) (wasm.ModuleExports, error) {
	internal, ok := store.(*internalwasm.Store)
	if !ok {
		return nil, fmt.Errorf("unsupported Store implementation: %s", store)
	}
	m, err := decodeModule(module)
	if err != nil {
		return nil, err
	}

	if err = m.Validate(); err != nil {
		return nil, err
	}
	return internal.Instantiate(m, getModuleName(module.Name, m))
}

// getModuleName returns the ModuleName from the internalwasm.NameSection if the input name was empty.
func getModuleName(name string, m *internalwasm.Module) string {
	if name == "" && m.NameSection != nil {
		return m.NameSection.ModuleName
	}
	return name
}

func decodeModule(module *ModuleConfig) (m *internalwasm.Module, err error) {
	if module.Source == nil {
		err = errors.New("source == nil")
		return
	}

	if len(module.Source) < 8 { // Ex. less than magic+version in binary or '(module)' in text
		err = errors.New("invalid source")
		return
	}

	// Check if this source was already decoded
	if bytes.Equal(module.Source, module.validatedSource) {
		m = module.decodedModule
		return
	}

	// Peek to see if this is a binary or text format
	if bytes.Equal(module.Source[0:4], binary.Magic) {
		m, err = binary.DecodeModule(module.Source)
	} else {
		m, err = text.DecodeModule(module.Source)
	}
	if err != nil {
		return
	}

	// Cache as tools like wapc-go re-instantiate the same module many times.
	module.validatedSource = module.Source
	module.decodedModule = m
	return
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
	// Host functions may also have an initial parameter (param[0]) of type context.Context or wasm.ModuleContext.
	//
	// Ex. This uses a Go Context:
	//
	//	addInts := func(ctx context.Context, x uint32, uint32) uint32 {
	//		// add a little extra if we put some in the context!
	//		return x + y + ctx.Value(extraKey).(uint32)
	//	}
	//
	// The most sophisticated context is wasm.ModuleContext, which allows access to the Go context, but also
	// allows writing to memory. This is important because there are only numeric types in Wasm. The only way to share other
	// data is via writing memory and sharing offsets.
	//
	// Ex. This reads the parameters from!
	//
	//	addInts := func(ctx wasm.ModuleContext, offset uint32) uint32 {
	//		x, _ := ctx.Memory().ReadUint32Le(offset)
	//		y, _ := ctx.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
	//		return x + y
	//	}
	//
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#host-functions%E2%91%A2
	Functions map[string]interface{}

	// TODO
	Globals map[string]uint64
}

// InstantiateHostModule instantiates the module namespace from the host or errs if the configuration was invalid.
//
// Ex.
//	store := wazero.NewStore()
//	wasiExports, _ := wazero.InstantiateHostModule(store, wazero.WASISnapshotPreview1())
//
func InstantiateHostModule(store wasm.Store, hostModule *HostModuleConfig) (wasm.HostExports, error) {
	internal, ok := store.(*internalwasm.Store)
	if !ok {
		return nil, fmt.Errorf("unsupported Store implementation: %s", store)
	}
	return internal.ExportHostFunctions(hostModule.Name, hostModule.Functions)
}
