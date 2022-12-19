package gojs

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"syscall"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

const (
	finalizeRefName        = "syscall/js.finalizeRef"
	stringValName          = "syscall/js.stringVal"
	valueGetName           = "syscall/js.valueGet"
	valueSetName           = "syscall/js.valueSet"
	valueDeleteName        = "syscall/js.valueDelete" // stubbed
	valueIndexName         = "syscall/js.valueIndex"
	valueSetIndexName      = "syscall/js.valueSetIndex" // stubbed
	valueCallName          = "syscall/js.valueCall"
	valueInvokeName        = "syscall/js.valueInvoke" // stubbed
	valueNewName           = "syscall/js.valueNew"
	valueLengthName        = "syscall/js.valueLength"
	valuePrepareStringName = "syscall/js.valuePrepareString"
	valueLoadStringName    = "syscall/js.valueLoadString"
	valueInstanceOfName    = "syscall/js.valueInstanceOf" // stubbed
	copyBytesToGoName      = "syscall/js.copyBytesToGo"
	copyBytesToJSName      = "syscall/js.copyBytesToJS"
)

var le = binary.LittleEndian

// FinalizeRef implements js.finalizeRef, which is used as a
// runtime.SetFinalizer on the given reference.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L61
var FinalizeRef = newSPFunc(finalizeRefName, finalizeRef)

func finalizeRef(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	ref := mustReadUint64Le(mem, "ref", uint32(sp[0]+8))
	id := uint32(ref) // 32-bits of the ref are the ID

	getState(ctx).values.decrement(id)
}

// StringVal implements js.stringVal, which is used to load the string for
// `js.ValueOf(x)`. For example, this is used when setting HTTP headers.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L212
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L305-L308
var StringVal = newSPFunc(stringValName, stringVal)

func stringVal(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 24)

	xAddr := le.Uint32(stack)
	xLen := le.Uint32(stack[8:])

	x := string(mustRead(mem, "x", xAddr, xLen))

	ref := storeRef(ctx, x)

	// Write the results to memory at positions after the parameters.
	le.PutUint64(stack[16:], ref)
}

// ValueGet implements js.valueGet, which is used to load a js.Value property
// by name, e.g. `v.Get("address")`. Notably, this is used by js.handleEvent to
// get the pending event.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L295
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L311-L316
var ValueGet = newSPFunc(valueGetName, valueGet)

func valueGet(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 32)

	vRef := le.Uint64(stack)
	pAddr := le.Uint32(stack[8:])
	pLen := le.Uint32(stack[16:])

	p := string(mustRead(mem, "p", pAddr, pLen))
	v := loadValue(ctx, ref(vRef))

	var result interface{}
	if g, ok := v.(jsGet); ok {
		result = g.get(ctx, p)
	} else if e, ok := v.(error); ok {
		switch p {
		case "message": // js (GOOS=js) error, can be anything.
			result = e.Error()
		case "code": // syscall (GOARCH=wasm) error, must match key in mapJSError in fs_js.go
			result = mapJSError(e).Error()
		default:
			panic(fmt.Errorf("TODO: valueGet(v=%v, p=%s)", v, p))
		}
	} else {
		panic(fmt.Errorf("TODO: valueGet(v=%v, p=%s)", v, p))
	}

	ref := storeRef(ctx, result)

	// Write the results to memory at positions after the parameters.
	le.PutUint64(stack[24:], ref)
}

// ValueSet implements js.valueSet, which is used to store a js.Value property
// by name, e.g. `v.Set("address", a)`. Notably, this is used by js.handleEvent
// set the event result.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L309
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L318-L322
var ValueSet = newSPFunc(valueSetName, valueSet)

func valueSet(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 32)

	vRef := le.Uint64(stack)
	pAddr := le.Uint32(stack[8:])
	pLen := le.Uint32(stack[16:])
	xRef := le.Uint64(stack[24:])

	v := loadValue(ctx, ref(vRef))
	p := string(mustRead(mem, "p", pAddr, pLen))
	x := loadValue(ctx, ref(xRef))
	if v == getState(ctx) {
		switch p {
		case "_pendingEvent":
			if x == nil { // syscall_js.handleEvent
				v.(*state)._pendingEvent = nil
				return
			}
		}
	} else if e, ok := v.(*event); ok { // syscall_js.handleEvent
		switch p {
		case "result":
			e.result = x
			return
		}
	} else if m, ok := v.(*object); ok {
		m.properties[p] = x // e.g. opt.Set("method", req.Method)
		return
	}
	panic(fmt.Errorf("TODO: valueSet(v=%v, p=%s, x=%v)", v, p, x))
}

// ValueDelete is stubbed as it isn't used in Go's main source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L321
var ValueDelete = stubFunction(valueDeleteName)

// ValueIndex implements js.valueIndex, which is used to load a js.Value property
// by index, e.g. `v.Index(0)`. Notably, this is used by js.handleEvent to read
// event arguments
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L334
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L331-L334
var ValueIndex = newSPFunc(valueIndexName, valueIndex)

func valueIndex(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 24)

	vRef := le.Uint64(stack)
	i := le.Uint32(stack[8:])

	v := loadValue(ctx, ref(vRef))
	result := v.(*objectArray).slice[i]

	ref := storeRef(ctx, result)

	// Write the results to memory at positions after the parameters.
	le.PutUint64(stack[16:], ref)
}

// ValueSetIndex is stubbed as it is only used for js.ValueOf when the input is
// []interface{}, which doesn't appear to occur in Go's source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L348
var ValueSetIndex = stubFunction(valueSetIndexName)

// ValueCall implements js.valueCall, which is used to call a js.Value function
// by name, e.g. `document.Call("createElement", "div")`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L394
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L343-L358
var ValueCall = newSPFunc(valueCallName, valueCall)

func valueCall(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read param count * 8 memory starting at SP+8
	params := mustRead(mem, "sp", uint32(sp[0]+8), 40)

	vRef := le.Uint64(params)
	mAddr := le.Uint32(params[8:])
	mLen := le.Uint32(params[16:])
	argsArray := le.Uint32(params[24:])
	argsLen := le.Uint32(params[32:])

	this := ref(vRef)
	v := loadValue(ctx, this)
	m := string(mustRead(mem, "m", mAddr, mLen))
	args := loadArgs(ctx, mod, argsArray, argsLen)

	var xRef uint64
	var ok uint32
	if c, isCall := v.(jsCall); !isCall {
		panic(fmt.Errorf("TODO: valueCall(v=%v, m=%v, args=%v)", v, m, args))
	} else if result, err := c.call(ctx, mod, this, m, args...); err != nil {
		xRef = storeRef(ctx, err)
		ok = 0
	} else {
		xRef = storeRef(ctx, result)
		ok = 1
	}

	// On refresh, start to write results 16 bytes after the last parameter.
	results := mustRead(mem, "sp", refreshSP(mod)+56, 16)

	// Write the results back to the stack
	le.PutUint64(results, xRef)
	le.PutUint32(results[8:], ok)
}

// ValueInvoke is stubbed as it isn't used in Go's main source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L413
var ValueInvoke = stubFunction(valueInvokeName)

// ValueNew implements js.valueNew, which is used to call a js.Value, e.g.
// `array.New(2)`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L432
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L380-L391
var ValueNew = newSPFunc(valueNewName, valueNew)

func valueNew(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read param count * 8 memory starting at SP+8
	params := mustRead(mem, "sp", uint32(sp[0]+8), 24)

	vRef := le.Uint64(params)
	argsArray := le.Uint32(params[8:])
	argsLen := le.Uint32(params[16:])

	args := loadArgs(ctx, mod, argsArray, argsLen)
	ref := ref(vRef)
	v := loadValue(ctx, ref)

	var xRef uint64
	var ok uint32
	switch ref {
	case refArrayConstructor:
		result := &objectArray{}
		xRef = storeRef(ctx, result)
		ok = 1
	case refUint8ArrayConstructor:
		var result *byteArray
		if n, ok := args[0].(float64); ok {
			result = &byteArray{make([]byte, uint32(n))}
		} else if n, ok := args[0].(uint32); ok {
			result = &byteArray{make([]byte, n)}
		} else if b, ok := args[0].(*byteArray); ok {
			// In case of below, in HTTP, return the same ref
			//	uint8arrayWrapper := uint8Array.New(args[0])
			result = b
		} else {
			panic(fmt.Errorf("TODO: valueNew(v=%v, args=%v)", v, args))
		}
		xRef = storeRef(ctx, result)
		ok = 1
	case refObjectConstructor:
		result := &object{properties: map[string]interface{}{}}
		xRef = storeRef(ctx, result)
		ok = 1
	case refHttpHeadersConstructor:
		result := &headers{headers: http.Header{}}
		xRef = storeRef(ctx, result)
		ok = 1
	case refJsDateConstructor:
		xRef = uint64(refJsDate)
		ok = 1
	default:
		panic(fmt.Errorf("TODO: valueNew(v=%v, args=%v)", v, args))
	}

	// On refresh, start to write results 16 bytes after the last parameter.
	results := mustRead(mem, "sp", refreshSP(mod)+40, 16)

	// Write the results back to the stack
	le.PutUint64(results, xRef)
	le.PutUint32(results[8:], ok)
}

// ValueLength implements js.valueLength, which is used to load the length
// property of a value, e.g. `array.length`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L372
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L396-L397
var ValueLength = newSPFunc(valueLengthName, valueLength)

func valueLength(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 16)

	vRef := le.Uint64(stack)

	v := loadValue(ctx, ref(vRef))
	l := uint32(len(v.(*objectArray).slice))

	// Write the results to memory at positions after the parameters.
	le.PutUint32(stack[8:], l)
}

// ValuePrepareString implements js.valuePrepareString, which is used to load
// the string for `o.String()` (via js.jsString) for string, boolean and
// number types. Notably, http.Transport uses this in RoundTrip to coerce the
// URL to a string.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L531
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L402-L405
var ValuePrepareString = newSPFunc(valuePrepareStringName, valuePrepareString)

func valuePrepareString(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 24)

	vRef := le.Uint64(stack)

	v := loadValue(ctx, ref(vRef))
	s := valueString(v)

	sRef := storeRef(ctx, s)
	sLen := uint32(len(s))

	// Write the results to memory at positions after the parameters.
	le.PutUint64(stack[8:], sRef)
	le.PutUint32(stack[16:], sLen)
}

// ValueLoadString implements js.valueLoadString, which is used copy a string
// value for `o.String()`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L533
//
//	https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L410-L412
var ValueLoadString = newSPFunc(valueLoadStringName, valueLoadString)

func valueLoadString(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 24)

	vRef := le.Uint64(stack)
	bAddr := le.Uint32(stack[8:])
	bLen := le.Uint32(stack[16:])

	v := loadValue(ctx, ref(vRef))
	s := valueString(v)
	b := mustRead(mem, "b", bAddr, bLen)
	copy(b, s)
}

// ValueInstanceOf is stubbed as it isn't used in Go's main source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L543
var ValueInstanceOf = stubFunction(valueInstanceOfName)

// CopyBytesToGo copies a JavaScript managed byte array to linear memory.
// For example, this is used to read an HTTP response body.
//
// # Results
//
//   - n is the count of bytes written.
//   - ok is false if the src was not a uint8Array.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L569
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L424-L433
var CopyBytesToGo = newSPFunc(copyBytesToGoName, copyBytesToGo)

func copyBytesToGo(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 48)

	dstAddr := le.Uint32(stack)
	dstLen := le.Uint32(stack[8:])
	/* unknown := le.Uint32(stack[16:]) */
	srcRef := le.Uint64(stack[24:])

	dst := mustRead(mem, "dst", dstAddr, dstLen)
	v := loadValue(ctx, ref(srcRef))

	var n, ok uint32
	if src, isBuf := v.(*byteArray); isBuf {
		n = uint32(copy(dst, src.slice))
		ok = 1
	}

	// Write the results to memory at positions after the parameters.
	le.PutUint32(stack[32:], n)
	le.PutUint32(stack[40:], ok)
}

// CopyBytesToJS copies linear memory to a JavaScript managed byte array.
// For example, this is used to read an HTTP request body.
//
// # Results
//
//   - n is the count of bytes written.
//   - ok is false if the dst was not a uint8Array.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L583
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L438-L448
var CopyBytesToJS = newSPFunc(copyBytesToJSName, copyBytesToJS)

func copyBytesToJS(ctx context.Context, mod api.Module, sp []uint64) {
	mem := mod.Memory()

	// Read (param + result count) * 8 memory starting at SP+8
	stack := mustRead(mem, "sp", uint32(sp[0]+8), 48)

	dstRef := le.Uint64(stack)
	srcAddr := le.Uint32(stack[8:])
	srcLen := le.Uint32(stack[16:])
	/* unknown := le.Uint32(stack[24:]) */

	src := mustRead(mem, "src", srcAddr, srcLen) // nolint
	v := loadValue(ctx, ref(dstRef))

	var n, ok uint32
	if dst, isBuf := v.(*byteArray); isBuf {
		if dst != nil { // empty is possible on EOF
			n = uint32(copy(dst.slice, src))
		}
		ok = 1
	}

	// Write the results to memory at positions after the parameters.
	le.PutUint32(stack[32:], n)
	le.PutUint32(stack[40:], ok)
}

// refreshSP refreshes the stack pointer, which is needed prior to storeValue
// when in an operation that can trigger a Go event handler.
//
// See https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L210-L213
func refreshSP(mod api.Module) uint32 {
	// Cheat by reading global[0] directly instead of through a function proxy.
	// https://github.com/golang/go/blob/go1.19/src/runtime/rt0_js_wasm.s#L87-L90
	return uint32(mod.(*wasm.CallContext).GlobalVal(0))
}

// syscallErr is a (GOARCH=wasm) error, which must match a key in mapJSError.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/tables_js.go#L371-L494
type syscallErr struct {
	s string
}

// Error implements error.
func (e *syscallErr) Error() string {
	return e.s
}

// While usually I/O returns the correct errors, being explicit helps reduce
// chance of problems.
var (
	ebadf   = &syscallErr{"EBADF"}
	einval  = &syscallErr{"EBADF"}
	eexist  = &syscallErr{"EEXIST"}
	enoent  = &syscallErr{"ENOENT"}
	enotdir = &syscallErr{"ENOTDIR"}
)

// mapJSError maps I/O errors as the message must be the code, ex. "EINVAL",
// not the message, ex. "invalid argument".
func mapJSError(err error) *syscallErr {
	if e, ok := err.(*syscallErr); ok {
		return e
	}
	switch {
	case errors.Is(err, syscall.EBADF), errors.Is(err, fs.ErrClosed):
		return ebadf
	case errors.Is(err, syscall.EINVAL), errors.Is(err, fs.ErrInvalid):
		return einval
	case errors.Is(err, syscall.EEXIST), errors.Is(err, fs.ErrExist):
		return eexist
	case errors.Is(err, syscall.ENOENT), errors.Is(err, fs.ErrNotExist):
		return enoent
	case errors.Is(err, syscall.ENOTDIR):
		return enotdir
	default:
		// panic so we can map the error before reaching JavaScript, which
		// can't see the error message as it just prints "object".
		panic(fmt.Errorf("unmapped error: %v", err))
	}
}

// syscallOpen is like syscall.Open
func syscallOpen(mod api.Module, name string, flags, perm uint32) (uint32, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS()
	return fsc.OpenFile(name)
}

// funcWrapper is the result of go's js.FuncOf ("_makeFuncWrapper" here).
//
// This ID is managed on the Go side an increments (possibly rolling over).
type funcWrapper uint32

// jsFn implements jsFn.invoke
func (f funcWrapper) invoke(ctx context.Context, mod api.Module, args ...interface{}) (interface{}, error) {
	e := &event{id: uint32(f), this: args[0].(ref)}

	if len(args) > 1 { // Ensure arguments are hashable.
		e.args = &objectArray{args[1:]}
		for i, v := range e.args.slice {
			if s, ok := v.([]byte); ok {
				args[i] = &byteArray{s}
			} else if s, ok := v.([]interface{}); ok {
				args[i] = &objectArray{s}
			} else if e, ok := v.(error); ok {
				args[i] = e
			}
		}
	}

	getState(ctx)._pendingEvent = e // Note: _pendingEvent reference is cleared during resume!

	if _, err := mod.ExportedFunction("resume").Call(ctx); err != nil {
		if _, ok := err.(*sys.ExitError); ok {
			return nil, nil // allow error-handling to unwind when wasm calls exit due to a panic
		} else {
			return nil, err
		}
	}

	return e.result, nil
}

func newSPFunc(name string, goFunc api.GoModuleFunc) *wasm.HostFunc {
	return &wasm.HostFunc{
		ExportNames: []string{name},
		Name:        name,
		ParamTypes:  []api.ValueType{api.ValueTypeI32},
		ParamNames:  []string{"sp"},
		Code:        &wasm.Code{IsHostFunction: true, GoFunc: goFunc},
	}
}
