package wasm

import (
	"context"
	"fmt"
	"math"
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

// getGoFuncCallContextValue returns a reflect.Value for a context param[0], or nil if there isn't one.
func getGoFuncCallContextValue(fk FunctionKind, ctx *CallContext) *reflect.Value {
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

// PopGoFuncParams pops the correct number of parameters off the stack into a parameter slice for use in CallGoFunc
//
// For example, if the host function F requires the (x1 uint32, x2 float32) parameters, and
// the stack is [..., A, B], then the function is called as F(A, B) where A and B are interpreted
// as uint32 and float32 respectively.
func PopGoFuncParams(f *FunctionInstance, popParam func() uint64) []uint64 {
	// First, determine how many values we need to pop
	paramCount := f.GoFunc.Type().NumIn()
	if f.Kind != FunctionKindGoNoContext {
		paramCount--
	}

	return PopValues(paramCount, popParam)
}

// PopValues pops api.ValueType values from the stack and returns them in reverse order.
//
// Note: the popper intentionally doesn't return bool or error because the caller's stack depth is trusted.
func PopValues(count int, popper func() uint64) []uint64 {
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
func CallGoFunc(callCtx *CallContext, f *FunctionInstance, params []uint64) []uint64 {
	tp := f.GoFunc.Type()

	var in []reflect.Value
	if tp.NumIn() != 0 {
		in = make([]reflect.Value, tp.NumIn())

		wasmParamOffset := 0
		if f.Kind != FunctionKindGoNoContext {
			wasmParamOffset = 1
		}

		for i, raw := range params {
			inI := i + wasmParamOffset
			val := reflect.New(tp.In(inI)).Elem()
			switch tp.In(inI).Kind() {
			case reflect.Float32:
				val.SetFloat(float64(math.Float32frombits(uint32(raw))))
			case reflect.Float64:
				val.SetFloat(math.Float64frombits(raw))
			case reflect.Uint32, reflect.Uint64:
				val.SetUint(raw)
			case reflect.Int32, reflect.Int64:
				val.SetInt(int64(raw))
			}
			in[inI] = val
		}

		// Handle any special parameter zero
		if val := getGoFuncCallContextValue(f.Kind, callCtx); val != nil {
			in[0] = *val
		}
	}

	// Execute the host function and push back the call result onto the stack.
	var results []uint64
	if tp.NumOut() > 0 {
		results = make([]uint64, 0, tp.NumOut())
	}
	for _, ret := range f.GoFunc.Call(in) {
		switch ret.Kind() {
		case reflect.Float32:
			results = append(results, uint64(math.Float32bits(float32(ret.Float()))))
		case reflect.Float64:
			results = append(results, math.Float64bits(ret.Float()))
		case reflect.Uint32, reflect.Uint64:
			results = append(results, ret.Uint())
		case reflect.Int32, reflect.Int64:
			results = append(results, uint64(ret.Int()))
		default:
			panic("BUG: invalid return type")
		}
	}
	return results
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
