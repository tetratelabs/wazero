// Package v1_0 is a user-level API only supporting features that exist in Wasm 1.0
// See https://www.w3.org/TR/wasm-core-1/
package v1_0

import (
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// ValueType represents numeric types a Function can accept as an argument or return as a result.
// See https://www.w3.org/TR/wasm-core-1/#value-types%E2%91%A0
type ValueType byte

const (
	I32 ValueType = iota
	I64
	F32
	F64
)

// String returns the name of k.
func (v ValueType) String() string {
	switch v {
	case I32:
		return "i32"
	case I64:
		return "i64"
	case F32:
		return "f32"
	case F64:
		return "f64"
	}
	return fmt.Sprintf("valueType(%d)", v)
}

// Module is a WebAssembly module allowing access to exported functions.
type Module interface {
	// Name returns the case-sensitive name of the Module.
	Name() string

	// FunctionByName returns a function of the given name or false for any failure including it not being exported.
	FunctionByName(name string) (f Function, ok bool)
}

// Function is a WebAssembly function. This can be called multiple times, though not in different goroutines.
//
// As this package is scoped to only spec version 1.0, only one or no result is returned.
// See https://www.w3.org/TR/wasm-core-1/#function-types%E2%91%A0
type Function interface {
	// Name returns the case-sensitive name of the Function.
	Name() string

	// ParameterTypes returns a possibly empty slice of parameter types of the Call signature.
	ParameterTypes() []ValueType

	// ResultType returns the result type of the Call signature or false if there is none.
	ResultType() (resultType ValueType, hasResult bool)

	// Call invokes the function with the given parameters and returns the result or an error on runtime exception.
	//
	// Parameters will coerce according to types indicated in ParameterTypes and the result to ResultType.
	//
	// Call never panics, as doing so would leak runtime implementation details which could create a security problem.
	// You must check the runtime error on each invocation and handle accordingly. Otherwise, you can misinterpret a
	// zero result as a success.
	Call(parameters ...uint64) (uint64, error)
}

// NewModule returns a WebAssembly v1.0 view of the given wasm.Store or false if the module doesn't exist or isn't 1.0.
func NewModule(s *wasm.Store, moduleName string) (Module, bool) {
	if _, ok := s.ModuleInstances[moduleName]; !ok {
		return nil, false
	} else {
		// TODO check the Wasm version
		return &module{s, moduleName}, true
	}
}

type module struct {
	s *wasm.Store
	n string
}

func (m *module) Name() string {
	return m.n
}

func (m *module) FunctionByName(name string) (Function, bool) {
	if f, ok := m.s.FunctionByName(m.n, name); !ok {
		return nil, false
	} else {
		return &function{m, f, name}, true
	}
}

type function struct {
	m *module
	f *wasm.FunctionInstance
	n string
}

func (f *function) Name() string {
	return f.n
}

func (f *function) ParameterTypes() []ValueType {
	var result []ValueType
	for _, t := range f.f.Signature.InputTypes {
		result = append(result, f.convertValueType(t))
	}
	return result
}

func (f *function) convertValueType(t wasm.ValueType) ValueType {
	var r ValueType
	switch t {
	case wasm.ValueTypeI32:
		r = I32
	case wasm.ValueTypeI64:
		r = I64
	case wasm.ValueTypeF32:
		r = F32
	case wasm.ValueTypeF64:
		r = F64
	default:
		panic(fmt.Sprintf("invalid function %s/%s: value type %d is unexpected in Wasm is unexpected in Wasm 1.0", f.m.n, f.n, t))
	}
	return r
}

func (f *function) ResultType() (resultType ValueType, hasResult bool) {
	switch len(f.f.Signature.ReturnTypes) {
	case 0:
		return 0, false
	case 1:
		return f.convertValueType(f.f.Signature.ReturnTypes[0]), true
	default:
		panic(fmt.Sprintf("invalid function %s/%s: more than one value type is unexpected in Wasm 1.0", f.m.n, f.n))
	}
}

func (f *function) Call(parameters ...uint64) (uint64, error) {
	ret, _, err := f.m.s.CallFunction(f.m.n, f.n, parameters...)
	if len(ret) == 0 {
		return 0, err
	}
	return ret[0], err
}
