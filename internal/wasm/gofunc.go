package internalwasm

import (
	"context"
	"fmt"
	"reflect"

	publicwasm "github.com/tetratelabs/wazero/wasm"
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
	// FunctionKindGoModuleContext is a function implemented in Go, with a signature matching FunctionType, except arg zero is
	// a ModuleContext.
	FunctionKindGoModuleContext
)

// GoFunc binds a WebAssembly 1.0 (MVP) Type Use to a Go func signature.
type GoFunc struct {
	wasmFunctionName string
	// functionKind is never FunctionKindWasm
	functionKind FunctionKind
	functionType *FunctionType
	goFunc       *reflect.Value
}

func NewGoFunc(wasmFunctionName string, goFunc interface{}) (hf *GoFunc, err error) {
	hf = &GoFunc{wasmFunctionName: wasmFunctionName}
	fn := reflect.ValueOf(goFunc)
	hf.goFunc = &fn
	hf.functionKind, hf.functionType, _, err = GetFunctionType(hf.wasmFunctionName, hf.goFunc, false)
	return
}

// Below are reflection code to get the interface type used to parse functions and set values.

var moduleContextType = reflect.TypeOf((*publicwasm.ModuleContext)(nil)).Elem()
var goContextType = reflect.TypeOf((*context.Context)(nil)).Elem()
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// GetHostFunctionCallContextValue returns a reflect.Value for a context param[0], or nil if there isn't one.
func GetHostFunctionCallContextValue(fk FunctionKind, ctx *ModuleContext) *reflect.Value {
	switch fk {
	case FunctionKindGoNoContext: // no special param zero
	case FunctionKindGoContext:
		val := reflect.New(goContextType).Elem()
		val.Set(reflect.ValueOf(ctx.Context()))
		return &val
	case FunctionKindGoModuleContext:
		val := reflect.New(moduleContextType).Elem()
		val.Set(reflect.ValueOf(ctx))
		return &val
	}
	return nil
}

// GetFunctionType returns the function type corresponding to the function signature or errs if invalid.
func GetFunctionType(name string, fn *reflect.Value, allowErrorResult bool) (fk FunctionKind, ft *FunctionType, hasErrorResult bool, err error) {
	if fn.Kind() != reflect.Func {
		err = fmt.Errorf("%s is a %s, but should be a Func", name, fn.Kind().String())
		return
	}
	p := fn.Type()

	pOffset := 0
	pCount := p.NumIn()
	fk = FunctionKindGoNoContext
	if pCount > 0 && p.In(0).Kind() == reflect.Interface {
		p0 := p.In(0)
		if p0.Implements(moduleContextType) {
			fk = FunctionKindGoModuleContext
			pOffset = 1
			pCount--
		} else if p0.Implements(goContextType) {
			fk = FunctionKindGoContext
			pOffset = 1
			pCount--
		}
	}

	rCount := p.NumOut()
	if (allowErrorResult && rCount > 2) || (!allowErrorResult && rCount > 1) {
		err = fmt.Errorf("%s has more than one result", name)
		return
	}

	if allowErrorResult && rCount > 0 {
		maybeErrIdx := rCount - 1
		if p.Out(maybeErrIdx).Implements(errorType) {
			hasErrorResult = true
			rCount--
		}
	}

	ft = &FunctionType{Params: make([]ValueType, pCount), Results: make([]ValueType, rCount)}

	for i := 0; i < len(ft.Params); i++ {
		pI := p.In(i + pOffset)
		if t, ok := getTypeOf(pI.Kind()); ok {
			ft.Params[i] = t
			continue
		}

		// Now, we will definitely err, decide which message is best
		var arg0Type reflect.Type
		if hc := pI.Implements(moduleContextType); hc {
			arg0Type = moduleContextType
		} else if gc := pI.Implements(goContextType); gc {
			arg0Type = goContextType
		}

		if arg0Type != nil {
			err = fmt.Errorf("%s param[%d] is a %s, which may be defined only once as param[0]", name, i+pOffset, arg0Type)
		} else {
			err = fmt.Errorf("%s param[%d] is unsupported: %s", name, i+pOffset, pI.Kind())
		}
		return
	}

	if rCount == 0 {
		return
	}

	result := p.Out(0)
	if t, ok := getTypeOf(result.Kind()); ok {
		ft.Results[0] = t
		return
	}

	if result.Implements(errorType) {
		err = fmt.Errorf("%s result[0] is an error, which is unsupported", name)
	} else {
		err = fmt.Errorf("%s result[0] is unsupported: %s", name, result.Kind())
	}
	return
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
	default:
		return 0x00, false
	}
}
