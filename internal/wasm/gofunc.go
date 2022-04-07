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
	// zero is a Module.
	FunctionKindGoModule
)

// Below are reflection code to get the interface type used to parse functions and set values.

var moduleType = reflect.TypeOf((*api.Module)(nil)).Elem()
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
	case FunctionKindGoModule:
		val := reflect.New(moduleType).Elem()
		val.Set(reflect.ValueOf(ctx))
		return &val
	}
	return nil
}

// getFunctionType returns the function type corresponding to the function signature or errs if invalid.
func getFunctionType(fn *reflect.Value, enabledFeatures Features) (fk FunctionKind, ft *FunctionType, err error) {
	p := fn.Type()

	if fn.Kind() != reflect.Func {
		err = fmt.Errorf("kind != func: %s", fn.Kind().String())
		return
	}

	pOffset := 0
	if fk = kind(p); fk != FunctionKindGoNoContext {
		pOffset = 1
	}

	rCount := p.NumOut()

	if rCount > 1 {
		// Guard >1.0 feature multi-value
		if err = enabledFeatures.Require(FeatureMultiValue); err != nil {
			err = fmt.Errorf("multiple result types invalid as %v", err)
			return
		}
	}

	ft = &FunctionType{Params: make([]ValueType, p.NumIn()-pOffset), Results: make([]ValueType, rCount)}

	for i := 0; i < len(ft.Params); i++ {
		pI := p.In(i + pOffset)
		if t, ok := getTypeOf(pI.Kind()); ok {
			ft.Params[i] = t
			continue
		}

		// Now, we will definitely err, decide which message is best
		var arg0Type reflect.Type
		if hc := pI.Implements(moduleType); hc {
			arg0Type = moduleType
		} else if gc := pI.Implements(goContextType); gc {
			arg0Type = goContextType
		}

		if arg0Type != nil {
			err = fmt.Errorf("param[%d] is a %s, which may be defined only once as param[0]", i+pOffset, arg0Type)
		} else {
			err = fmt.Errorf("param[%d] is unsupported: %s", i+pOffset, pI.Kind())
		}
		return
	}

	for i := 0; i < len(ft.Results); i++ {
		rI := p.Out(i)
		if t, ok := getTypeOf(rI.Kind()); ok {
			ft.Results[i] = t
			continue
		}

		// Now, we will definitely err, decide which message is best
		if rI.Implements(errorType) {
			err = fmt.Errorf("result[%d] is an error, which is unsupported", i)
		} else {
			err = fmt.Errorf("result[%d] is unsupported: %s", i, rI.Kind())
		}
		return
	}
	return
}

func kind(p reflect.Type) FunctionKind {
	pCount := p.NumIn()
	if pCount > 0 && p.In(0).Kind() == reflect.Interface {
		p0 := p.In(0)
		if p0.Implements(moduleType) {
			return FunctionKindGoModule
		} else if p0.Implements(goContextType) {
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
	default:
		return 0x00, false
	}
}
