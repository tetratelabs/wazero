package wasm

import (
	"context"
	"fmt"
	"reflect"

	"github.com/tetratelabs/wazero/api"
)

// FunctionKind identifies the type of function that can be called.
type FunctionKind byte

const (
	// FunctionKindWasm is not a Go function: it is implemented in Wasm.
	FunctionKindWasm FunctionKind = iota
	// FunctionKindGoNoContext is a function implemented in Go, with a signature matching FunctionType.
	FunctionKindGoNoContext
	// FunctionKindGoContext is a function implemented in Go, with a signature matching FunctionType, except arg zero is
	// a context.Context.
	FunctionKindGoContext
	// FunctionKindGoModule is a function implemented in Go, with a signature matching FunctionType, except arg
	// zero is an api.Module.
	FunctionKindGoModule
	// FunctionKindGoContextModule is a function implemented in Go, with a signature matching FunctionType, except arg
	// zero is a context.Context and arg one is an api.Module.
	FunctionKindGoContextModule
)

// Below are reflection code to get the interface type used to parse functions and set values.

var moduleType = reflect.TypeOf((*api.Module)(nil)).Elem()
var goContextType = reflect.TypeOf((*context.Context)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// PopGoFuncParams pops the correct number of parameters off the stack into a parameter slice for use in CallGoFunc
//
// For example, if the host function F requires the (x1 uint32, x2 float32) parameters, and
// the stack is [..., A, B], then the function is called as F(A, B) where A and B are interpreted
// as uint32 and float32 respectively.
func PopGoFuncParams(f *FunctionInstance, popParam func() uint64) []uint64 {
	// First, determine how many values we need to pop
	paramCount := len(f.GoFunc.NumIn) - 2
	return PopValues(paramCount, popParam)
}

// PopValues pops api.ValueType values from the stack and returns them in reverse order.
//
// Note: the popper intentionally doesn't return bool or error because the caller's stack depth is trusted.
func PopValues(count int, popper func() uint64) []uint64 {
	if count == 0 {
		return nil
	}
	params := make([]uint64, count)
	for i := count - 1; i >= 0; i-- {
		params[i] = popper()
	}
	return params
}

// CallGoFunc executes the FunctionInstance.GoFunc by converting params to Go types. The results of the function call
// are converted back to api.ValueType.
//
// * callCtx is passed to the host function as a first argument.
//
// Note: ctx must use the caller's memory, which might be different from the defining module on an imported function.
func CallGoFunc(ctx context.Context, callCtx *CallContext, f *FunctionInstance, params []uint64) []uint64 {
	results, err := f.GoFunc.Fn(ctx, callCtx, params...)
	if err != nil {
		// todo: add error return
		panic(err)
	}
	return results
}

func newContextVal(ctx context.Context) reflect.Value {
	val := reflect.New(goContextType).Elem()
	val.Set(reflect.ValueOf(ctx))
	return val
}

func newModuleVal(m api.Module) reflect.Value {
	val := reflect.New(moduleType).Elem()
	val.Set(reflect.ValueOf(m))
	return val
}

// MustParseGoFuncCode parses Code from the go function or panics.
//
// Exposing this simplifies FunctionDefinition of host functions in built-in host
// modules and tests.
func MustParseGoFuncCode(fn interface{}) *Code {
	_, _, code, err := parseGoFunc(fn)
	if err != nil {
		panic(err)
	}
	return code
}

func parseGoFunc(fn interface{}) (params, results []ValueType, code *Code, err error) {
	fnV, ok := fn.(*api.HostFuncSignature)
	if !ok {
		err = fmt.Errorf("host function type mistake")
		return
	}

	code = &Code{IsHostFunction: true, Kind: FunctionKindGoContextModule, GoFunc: fnV}
	params = fnV.NumIn
	results = fnV.NumOut
	return
}

func kind(p reflect.Type) FunctionKind {
	pCount := p.NumIn()
	if pCount > 0 && p.In(0).Kind() == reflect.Interface {
		p0 := p.In(0)
		if p0.Implements(moduleType) {
			return FunctionKindGoModule
		} else if p0.Implements(goContextType) {
			if pCount >= 2 && p.In(1).Implements(moduleType) {
				return FunctionKindGoContextModule
			}
			return FunctionKindGoContext
		}
	}
	return FunctionKindGoNoContext
}

func getTypeOf(kind reflect.Kind) (ValueType, bool) {
	switch kind {
	case reflect.Float64:
		return ValueTypeF64, true
	case reflect.Float32:
		return ValueTypeF32, true
	case reflect.Int32, reflect.Uint32:
		return ValueTypeI32, true
	case reflect.Int64, reflect.Uint64:
		return ValueTypeI64, true
	case reflect.Uintptr:
		return ValueTypeExternref, true
	default:
		return 0x00, false
	}
}
