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
type HostFunctions struct {
	nameToHostFunction map[string]*internalwasm.HostFunction
}

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
