package makefunc

import (
	"context"
	"fmt"
	"reflect"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	publicwasm "github.com/tetratelabs/wazero/wasm"
)

var uint64Zero = reflect.Zero(reflect.TypeOf(uint64(0)))
var errorType = reflect.TypeOf((*error)(nil)).Elem()

// MakeWasmFunc implements the goFuncPtr to call the provided wasmFunction.
//
// See reflect.MakeFunc
// TODO: Maybe we can optimize our own variant of reflect.MakeFunc in JIT and add this to the Engine interface.
func MakeWasmFunc(ctx *wasm.ModuleContext, name string, impl *wasm.FunctionInstance, goFuncPtr interface{}) error {
	// goFuncPtr should point to nil if used properly, but we don't care as we clobber it.
	fn := reflect.ValueOf(goFuncPtr).Elem()

	// Ensure the signature is correct and if it has an error result or not.
	fk, ft, hasErrorResult, err := wasm.GetFunctionType(name, &fn, true)
	if err != nil {
		return err
	}

	cf := &wasmFunc{
		ctx:                  ctx,
		impl:                 impl,
		goFuncKind:           fk,
		goFuncResultCount:    uint32(len(ft.Results)),
		goFuncHasErrorResult: hasErrorResult,
	}

	if cf.goFuncResultCount == 1 {
		cf.goFuncResultKind = fn.Type().Out(0).Kind()
	}
	if cf.goFuncHasErrorResult {
		cf.goFuncResultCount++
	}

	// Now that we know the type is compatible with Wasm, make a reflective invoker.
	v := reflect.MakeFunc(fn.Type(), cf.makeFunc)

	// Set the pointer value to the implementation that calls the Wasm function.
	fn.Set(v)
	return nil
}

type wasmFunc struct {
	ctx                  *wasm.ModuleContext
	impl                 *wasm.FunctionInstance
	goFuncKind           wasm.FunctionKind
	goFuncResultKind     reflect.Kind
	goFuncResultCount    uint32
	goFuncHasErrorResult bool
}

func (f *wasmFunc) makeFunc(args []reflect.Value) (results []reflect.Value) {
	wasmParamOffset := 1
	ctx := f.ctx
	switch f.goFuncKind {
	case wasm.FunctionKindGoNoContext: // no special param zero
		wasmParamOffset = 0
	case wasm.FunctionKindGoContext:
		ctx = ctx.WithContext(args[0].Interface().(context.Context))
	case wasm.FunctionKindGoModuleContext:
		if ctxImpl, ok := args[0].Interface().(*wasm.ModuleContext); ok {
			ctx = ctxImpl
		} else {
			return f.error(fmt.Errorf("unsupported ModuleContext implementation: %s", args[0].Interface()))
		}
	default:
		panic(fmt.Errorf("BUG: unhandled FunctionKind: %d", f.goFuncKind))
	}

	wasmParams := make([]uint64, len(args)-wasmParamOffset)

	for i := 0; i < len(wasmParams); i++ {
		arg := args[i+wasmParamOffset]
		switch arg.Kind() {
		case reflect.Float32:
			wasmParams[i] = publicwasm.EncodeF32(float32(arg.Float()))
		case reflect.Float64:
			wasmParams[i] = publicwasm.EncodeF64(arg.Float())
		case reflect.Uint32, reflect.Uint64:
			wasmParams[i] = arg.Uint()
		case reflect.Int32, reflect.Int64:
			wasmParams[i] = uint64(arg.Int())
		default:
			panic(fmt.Errorf("invalid arg[%d]: %s", i+wasmParamOffset, arg))
		}
	}
	wasmResults, err := ctx.Engine.Call(ctx, f.impl, wasmParams...)
	if err != nil {
		return f.error(err)
	}

	// Note: Each result element cannot be nil, though it may be reflect.Zero
	results = make([]reflect.Value, f.goFuncResultCount)

	// There's no error at this point, so backfill a zero value for it.
	if f.goFuncHasErrorResult {
		results[f.goFuncResultCount-1] = reflect.Zero(errorType)
	}

	// Here, we know there is no error. If the signature expects no result, return early.
	if f.goFuncResultKind == 0 {
		return
	}

	// In wazero, all value types are uint64. Particularly floats need special handling.
	wasmResult := wasmResults[0]
	var goResult reflect.Value
	switch f.goFuncResultKind {
	case reflect.Invalid: // Void or only-error return
	case reflect.Float32:
		goResult = reflect.ValueOf(publicwasm.DecodeF32(wasmResult))
	case reflect.Float64:
		goResult = reflect.ValueOf(publicwasm.DecodeF64(wasmResult))
	case reflect.Uint32:
		goResult = reflect.ValueOf(uint32(wasmResult))
	case reflect.Uint64:
		goResult = reflect.ValueOf(wasmResult)
	case reflect.Int32, reflect.Int64:
		goResult = reflect.ValueOf(int64(wasmResult))
	default:
		panic(fmt.Errorf("BUG: unsupported result kind: %s", f.goFuncResultKind))
	}
	results[0] = goResult
	return
}

func (f *wasmFunc) error(err error) []reflect.Value {
	// If the user didn't define an error return on their goFunc, our only choice is panic.
	if !f.goFuncHasErrorResult {
		panic(err)
	}

	reflectErr := reflect.ValueOf(err)
	// If there is a Go return type, we have to populate with a zero value it even on error.
	if f.goFuncResultKind != 0 {
		return []reflect.Value{uint64Zero, reflectErr}
	}
	return []reflect.Value{reflectErr}
}
