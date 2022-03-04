package internalwasm

import (
	"fmt"

	publicwasm "github.com/tetratelabs/wazero/wasm"
)

type mutableGlobal struct {
	g *GlobalInstance
}

// compile-time check to ensure mutableGlobal is a wasm.Global
var _ publicwasm.Global = &mutableGlobal{}

// Type implements wasm.Global Type
func (g *mutableGlobal) Type() publicwasm.ValueType {
	return g.g.Type.ValType
}

// Get implements wasm.Global Get
func (g *mutableGlobal) Get() uint64 {
	return g.g.Val
}

// Set implements wasm.MutableGlobal Set
func (g *mutableGlobal) Set(v uint64) {
	g.g.Val = v
}

// String implements fmt.Stringer
func (g *mutableGlobal) String() string {
	switch g.Type() {
	case ValueTypeI32, ValueTypeI64:
		return fmt.Sprintf("global(%d)", g.Get())
	case ValueTypeF32:
		return fmt.Sprintf("global(%f)", publicwasm.DecodeF32(g.Get()))
	case ValueTypeF64:
		return fmt.Sprintf("global(%f)", publicwasm.DecodeF64(g.Get()))
	default:
		panic(fmt.Errorf("BUG: unknown value type %X", g.Type()))
	}
}

type globalI32 uint64

// compile-time check to ensure globalI32 is a wasm.Global
var _ publicwasm.Global = globalI32(0)

// Type implements wasm.Global Type
func (g globalI32) Type() publicwasm.ValueType {
	return ValueTypeI32
}

// Get implements wasm.Global Get
func (g globalI32) Get() uint64 {
	return uint64(g)
}

// String implements fmt.Stringer
func (g globalI32) String() string {
	return fmt.Sprintf("global(%d)", g)
}

type globalI64 uint64

// compile-time check to ensure globalI64 is a publicwasm.Global
var _ publicwasm.Global = globalI64(0)

// Type implements wasm.Global Type
func (g globalI64) Type() publicwasm.ValueType {
	return ValueTypeI64
}

// Get implements wasm.Global Get
func (g globalI64) Get() uint64 {
	return uint64(g)
}

// String implements fmt.Stringer
func (g globalI64) String() string {
	return fmt.Sprintf("global(%d)", g)
}

type globalF32 uint64

// compile-time check to ensure globalF32 is a publicwasm.Global
var _ publicwasm.Global = globalF32(0)

// Type implements wasm.Global Type
func (g globalF32) Type() publicwasm.ValueType {
	return ValueTypeF32
}

// Get implements wasm.Global Get
func (g globalF32) Get() uint64 {
	return uint64(g)
}

// String implements fmt.Stringer
func (g globalF32) String() string {
	return fmt.Sprintf("global(%f)", publicwasm.DecodeF32(g.Get()))
}

type globalF64 uint64

// compile-time check to ensure globalF64 is a publicwasm.Global
var _ publicwasm.Global = globalF64(0)

// Type implements wasm.Global Type
func (g globalF64) Type() publicwasm.ValueType {
	return ValueTypeF64
}

// Get implements wasm.Global Get
func (g globalF64) Get() uint64 {
	return uint64(g)
}

// String implements fmt.Stringer
func (g globalF64) String() string {
	return fmt.Sprintf("global(%f)", publicwasm.DecodeF64(g.Get()))
}

// Global implements wasm.Module Global
func (m *PublicModule) Global(name string) publicwasm.Global {
	exp, err := m.instance.getExport(name, ExternTypeGlobal)
	if err != nil {
		return nil
	}
	if exp.Global.Type.Mutable {
		return &mutableGlobal{exp.Global}
	}
	valType := exp.Global.Type.ValType
	switch valType {
	case ValueTypeI32:
		return globalI32(exp.Global.Val)
	case ValueTypeI64:
		return globalI64(exp.Global.Val)
	case ValueTypeF32:
		return globalF32(exp.Global.Val)
	case ValueTypeF64:
		return globalF64(exp.Global.Val)
	default:
		panic(fmt.Errorf("BUG: unknown value type %X", valType))
	}
}
