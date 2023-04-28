package wasm

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
)

// constantGlobal wraps GlobalInstance to implement api.Global.
type constantGlobal struct {
	g *GlobalInstance
}

// Type implements api.Global.
func (g constantGlobal) Type() api.ValueType {
	return g.g.Type.ValType
}

// Get implements api.Global.
func (g constantGlobal) Get() uint64 {
	return g.g.Val
}

// String implements api.Global.
func (g constantGlobal) String() string {
	switch g.Type() {
	case ValueTypeI32, ValueTypeI64:
		return fmt.Sprintf("global(%d)", g.Get())
	case ValueTypeF32:
		return fmt.Sprintf("global(%f)", api.DecodeF32(g.Get()))
	case ValueTypeF64:
		return fmt.Sprintf("global(%f)", api.DecodeF64(g.Get()))
	default:
		panic(fmt.Errorf("BUG: unknown value type %X", g.Type()))
	}
}

// mutableGlobal extends constantGlobal to allow updates.
type mutableGlobal struct {
	constantGlobal
	internalapi.WazeroOnlyType
}

// compile-time check to ensure mutableGlobal is a api.Global.
var _ api.Global = mutableGlobal{}

// Set implements the same method as documented on api.MutableGlobal.
func (g *mutableGlobal) Set(v uint64) {
	g.g.Val = v
}
