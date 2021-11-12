package wazeroir

import (
	"github.com/tetratelabs/wazero/wasm"
)

func NewEngine() wasm.Engine {
	// TODO: add option to use JIT instead of interpreter
	return &interpreter{
		functions:                 map[*wasm.FunctionInstance]*interpreterFunction{},
		functionTypeIDs:           map[string]uint64{},
		onComilationDoneCallbacks: map[*wasm.FunctionInstance][]func(*interpreterFunction){},
	}
}
