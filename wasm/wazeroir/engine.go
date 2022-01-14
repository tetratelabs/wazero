package wazeroir

import (
	"github.com/tetratelabs/wazero/wasm"
)

func NewEngine() wasm.Engine {
	return &interpreter{
		functions:                  map[wasm.FunctionAddress]*interpreterFunction{},
		functionTypeIDs:            map[string]uint64{},
		onCompilationDoneCallbacks: map[wasm.FunctionAddress][]func(*interpreterFunction){},
	}
}
