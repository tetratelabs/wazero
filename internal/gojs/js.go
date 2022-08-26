package gojs

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// ref is used to identify a JavaScript value, since the value itself can not be passed to WebAssembly.
//
// The JavaScript value "undefined" is represented by the value 0.
// A JavaScript number (64-bit float, except 0 and NaN) is represented by its IEEE 754 binary representation.
// All other values are represented as an IEEE 754 binary representation of NaN with bits 0-31 used as
// an ID and bits 32-34 used to differentiate between string, symbol, function and object.
type ref uint64

const (
	parameterSp   = "sp"
	functionDebug = "debug"
)

// jsFn is a jsCall.call function, configured via jsVal.addFunction.
type jsFn interface {
	invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error)
}

type jsGet interface {
	get(ctx context.Context, propertyKey string) interface{}
}

// jsCall allows calling a method/function by name.
type jsCall interface {
	call(ctx context.Context, mod api.Module, this ref, method string, args ...interface{}) (interface{}, error)
}

// nanHead are the upper 32 bits of a ref which are set if the value is not encoded as an IEEE 754 number (see above).
const nanHead = 0x7FF80000

type typeFlag byte

const (
	// the type flags need to be in sync with gojs.js
	typeFlagNone typeFlag = iota
	typeFlagObject
	typeFlagString
	typeFlagSymbol // nolint
	typeFlagFunction
)

func valueRef(id uint32, typeFlag typeFlag) ref {
	return (nanHead|ref(typeFlag))<<32 | ref(id)
}

func newJsVal(ref ref, name string) *jsVal {
	return &jsVal{ref: ref, name: name, properties: map[string]interface{}{}, functions: map[string]jsFn{}}
}

// jsVal corresponds to a generic js.Value in go, when `GOOS=js`.
type jsVal struct {
	// ref when is the constant reference used for built-in values, such as
	// objectConstructor.
	ref
	name       string
	properties map[string]interface{}
	functions  map[string]jsFn
}

func (v *jsVal) addProperties(properties map[string]interface{}) *jsVal {
	for k, val := range properties {
		v.properties[k] = val
	}
	return v
}

func (v *jsVal) addFunction(method string, fn jsFn) *jsVal {
	v.functions[method] = fn
	// If fn returns an error, js.Call does a type lookup to verify it is a
	// function.
	// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L389
	v.properties[method] = fn
	return v
}

// get implements jsGet.get
func (v *jsVal) get(_ context.Context, propertyKey string) interface{} {
	if v, ok := v.properties[propertyKey]; ok {
		return v
	}
	panic(fmt.Sprintf("TODO: get %s.%s", v.name, propertyKey))
}

// call implements jsCall.call
func (v *jsVal) call(ctx context.Context, mod api.Module, this ref, method string, args ...interface{}) (interface{}, error) {
	if v, ok := v.functions[method]; ok {
		return v.invoke(ctx, mod, args...)
	}
	panic(fmt.Sprintf("TODO: call %s.%s", v.name, method))
}

// byteArray is a result of uint8ArrayConstructor which temporarily stores
// binary data outside linear memory.
//
// Note: This is a wrapper because a slice is not hashable.
type byteArray struct {
	slice []byte
}

// get implements jsGet.get
func (a *byteArray) get(_ context.Context, propertyKey string) interface{} {
	switch propertyKey {
	case "byteLength":
		return uint32(len(a.slice))
	}
	panic(fmt.Sprintf("TODO: get byteArray.%s", propertyKey))
}

// objectArray is a result of arrayConstructor typically used to pass
// indexed arguments.
//
// Note: This is a wrapper because a slice is not hashable.
type objectArray struct {
	slice []interface{}
}

// object is a result of objectConstructor typically used to pass named
// arguments.
//
// Note: This is a wrapper because a map is not hashable.
type object struct {
	properties map[string]interface{}
}
