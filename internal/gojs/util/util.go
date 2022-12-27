package util

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// MustWrite is like api.Memory except that it panics if the offset
// is out of range.
func MustWrite(mem api.Memory, fieldName string, offset uint32, val []byte) {
	if ok := mem.Write(offset, val); !ok {
		panic(fmt.Errorf("out of memory writing %s", fieldName))
	}
}

// MustRead is like api.Memory except that it panics if the offset and
// byteCount are out of range.
func MustRead(mem api.Memory, funcName string, paramIdx int, offset, byteCount uint32) []byte {
	buf, ok := mem.Read(offset, byteCount)
	if ok {
		return buf
	}
	var paramName string
	if names, ok := custom.NameSection[funcName]; ok {
		if paramIdx < len(names.ParamNames) {
			paramName = names.ParamNames[paramIdx]
		}
	}
	if paramName == "" {
		paramName = fmt.Sprintf("%s param[%d]", funcName, paramIdx)
	}
	panic(fmt.Errorf("out of memory reading %s", paramName))
}

func NewFunc(name string, goFunc api.GoModuleFunc) *wasm.HostFunc {
	return &wasm.HostFunc{
		ExportNames: []string{name},
		Name:        name,
		ParamTypes:  []api.ValueType{api.ValueTypeI32},
		ParamNames:  []string{"sp"},
		Code:        &wasm.Code{IsHostFunction: true, GoFunc: goFunc},
	}
}
