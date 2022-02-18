package wazero

import (
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/interpreter"
	"github.com/tetratelabs/wazero/internal/wasm/jit"
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

// HostFunctions are functions written in Go, which a WebAssembly Module can import.
//
// Noting a context exception described later, all parameters or result types must match WebAssembly 1.0 (MVP) value
// types. This means uint32, uint64, float32 or float64. Up to one result can be returned.
//
// Ex. This is a valid host function:
//
//	addInts := func(x uint32, uint32) uint32 {
//		return x + y
//	}
//
// Host functions may also have an initial parameter (param[0]) of type context.Context or wasm.HostFunctionCallContext.
//
// Ex. This uses a Go Context:
//
//	addInts := func(ctx context.Context, x uint32, uint32) uint32 {
//		// add a little extra if we put some in the context!
//		return x + y + ctx.Value(extraKey).(uint32)
//	}
//
// The most sophisticated context is wasm.HostFunctionCallContext, which allows access to the Go context, but also
// allows writing to memory. This is important because there are only numeric types in Wasm. The only way to share other
// data is via writing memory and sharing offsets.
//
// Ex. This reads the parameters from!
//
//	addInts := func(ctx wasm.HostFunctionCallContext, offset uint32) uint32 {
//		x, _ := ctx.Memory().ReadUint32Le(offset)
//		y, _ := ctx.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
//		return x + y
//	}
//
// See https://www.w3.org/TR/wasm-core-1/#value-types%E2%91%A0
type HostFunctions struct {
	nameToHostFunction map[string]*internalwasm.HostFunction
}

// NewHostFunctions returns host functions to export. The map key is the name to export and the value is the function.
// See HostFunctions documentation for notes on writing host functions.
func NewHostFunctions(nameToGoFunc map[string]interface{}) (ret *HostFunctions, err error) {
	ret = &HostFunctions{make(map[string]*internalwasm.HostFunction, len(nameToGoFunc))}
	for name, goFunc := range nameToGoFunc {
		if ret.nameToHostFunction[name], err = internalwasm.NewHostFunction(name, goFunc); err != nil {
			return nil, err
		}
	}
	return
}

// StoreConfig allows customization of a Store via NewStoreWithConfig
type StoreConfig struct {
	// Engine defaults to NewEngineInterpreter
	Engine *Engine
	// ModuleToHostFunctions include any module to host function exports
	ModuleToHostFunctions map[string]*HostFunctions
}

func NewStore() *Store {
	return &Store{s: internalwasm.NewStore(interpreter.NewEngine())}
}

// NewStoreWithConfig returns a store with the given configuration or errs if there was a problem registering a host
// function (such as WASI).
func NewStoreWithConfig(config *StoreConfig) (*Store, error) {
	engine := config.Engine
	if engine == nil {
		engine = NewEngineInterpreter()
	}
	s := internalwasm.NewStore(engine.e)
	if config.ModuleToHostFunctions != nil {
		for m, hfs := range config.ModuleToHostFunctions {
			for _, hf := range hfs.nameToHostFunction {
				if err := s.AddHostFunction(m, hf); err != nil {
					return nil, err
				}
			}
		}
	}
	return &Store{s: s}, nil
}

type Store struct {
	s *internalwasm.Store
}

func (s *Store) Instantiate(module *Module) (wasm.ModuleFunctions, error) {
	return s.s.Instantiate(module.m, module.name)
}
