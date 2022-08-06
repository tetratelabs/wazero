package gojs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"syscall"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/spfunc"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

const (
	functionFinalizeRef        = "syscall/js.finalizeRef"
	functionStringVal          = "syscall/js.stringVal"
	functionValueGet           = "syscall/js.valueGet"
	functionValueSet           = "syscall/js.valueSet"
	functionValueDelete        = "syscall/js.valueDelete" // stubbed
	functionValueIndex         = "syscall/js.valueIndex"
	functionValueSetIndex      = "syscall/js.valueSetIndex" // stubbed
	functionValueCall          = "syscall/js.valueCall"
	functionValueInvoke        = "syscall/js.valueInvoke" // stubbed
	functionValueNew           = "syscall/js.valueNew"
	functionValueLength        = "syscall/js.valueLength"
	functionValuePrepareString = "syscall/js.valuePrepareString"
	functionValueLoadString    = "syscall/js.valueLoadString"
	functionValueInstanceOf    = "syscall/js.valueInstanceOf" // stubbed
	functionCopyBytesToGo      = "syscall/js.copyBytesToGo"
	functionCopyBytesToJS      = "syscall/js.copyBytesToJS"
)

// FinalizeRef implements js.finalizeRef, which is used as a
// runtime.SetFinalizer on the given reference.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L61
var FinalizeRef = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionFinalizeRef, functionFinalizeRef,
	[]string{"r"},
	func(ctx context.Context, mod api.Module, id uint32) { // 32-bits of the ref are the ID
		getState(ctx).values.decrement(id)
	},
))

// StringVal implements js.stringVal, which is used to load the string for
// `js.ValueOf(x)`. For example, this is used when setting HTTP headers.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L212
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L305-L308
var StringVal = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionStringVal, functionStringVal,
	[]string{"xAddr", "xLen"},
	func(ctx context.Context, mod api.Module, xAddr, xLen uint32) uint64 {
		x := string(mustRead(ctx, mod.Memory(), "x", xAddr, xLen))
		return storeRef(ctx, x)
	},
))

// ValueGet implements js.valueGet, which is used to load a js.Value property
// by name, ex. `v.Get("address")`. Notably, this is used by js.handleEvent to
// get the pending event.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L295
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L311-L316
var ValueGet = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionValueGet, functionValueGet,
	[]string{"v", "pAddr", "pLen"},
	func(ctx context.Context, mod api.Module, vRef uint64, pAddr, pLen uint32) uint64 {
		p := string(mustRead(ctx, mod.Memory(), "p", pAddr, pLen))
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

		xRef := storeRef(ctx, result)
		return xRef
	},
))

// ValueSet implements js.valueSet, which is used to store a js.Value property
// by name, ex. `v.Set("address", a)`. Notably, this is used by js.handleEvent
// set the event result.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L309
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L318-L322
var ValueSet = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionValueSet, functionValueSet,
	[]string{"v", "pAddr", "pLen", "x"},
	func(ctx context.Context, mod api.Module, vRef uint64, pAddr, pLen uint32, xRef uint64) {
		v := loadValue(ctx, ref(vRef))
		p := string(mustRead(ctx, mod.Memory(), "p", pAddr, pLen))
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
			m.properties[p] = x // Ex. opt.Set("method", req.Method)
			return
		}
		panic(fmt.Errorf("TODO: valueSet(v=%v, p=%s, x=%v)", v, p, x))
	},
))

// ValueDelete is stubbed as it isn't used in Go's main source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L321
var ValueDelete = stubFunction(functionValueDelete)

// ValueIndex implements js.valueIndex, which is used to load a js.Value property
// by index, ex. `v.Index(0)`. Notably, this is used by js.handleEvent to read
// event arguments
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L334
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L331-L334
var ValueIndex = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionValueIndex, functionValueIndex,
	[]string{"v", "i"},
	func(ctx context.Context, mod api.Module, vRef uint64, i uint32) (xRef uint64) {
		v := loadValue(ctx, ref(vRef))
		result := v.(*objectArray).slice[i]
		xRef = storeRef(ctx, result)
		return
	},
))

// ValueSetIndex is stubbed as it is only used for js.ValueOf when the input is
// []interface{}, which doesn't appear to occur in Go's source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L348
var ValueSetIndex = stubFunction(functionValueSetIndex)

// ValueCall implements js.valueCall, which is used to call a js.Value function
// by name, ex. `document.Call("createElement", "div")`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L394
//
//	https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L343-L358
var ValueCall = spfunc.MustCallFromSP(true, wasm.NewGoFunc(
	functionValueCall, functionValueCall,
	[]string{"v", "mAddr", "mLen", "argsArray", "argsLen"},
	func(ctx context.Context, mod api.Module, vRef uint64, mAddr, mLen, argsArray, argsLen uint32) (xRef uint64, ok uint32, sp uint32) {
		this := ref(vRef)
		v := loadValue(ctx, this)
		m := string(mustRead(ctx, mod.Memory(), "m", mAddr, mLen))
		args := loadArgs(ctx, mod, argsArray, argsLen)

		if c, isCall := v.(jsCall); !isCall {
			panic(fmt.Errorf("TODO: valueCall(v=%v, m=%v, args=%v)", v, m, args))
		} else if result, err := c.call(ctx, mod, this, m, args...); err != nil {
			xRef = storeRef(ctx, err)
			ok = 0
		} else {
			xRef = storeRef(ctx, result)
			ok = 1
		}

		sp = refreshSP(mod)
		return
	},
))

// ValueInvoke is stubbed as it isn't used in Go's main source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L413
var ValueInvoke = stubFunction(functionValueInvoke)

// ValueNew implements js.valueNew, which is used to call a js.Value, ex.
// `array.New(2)`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L432
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L380-L391
var ValueNew = spfunc.MustCallFromSP(true, wasm.NewGoFunc(
	functionValueNew, functionValueNew,
	[]string{"v", "argsArray", "argsLen"},
	func(ctx context.Context, mod api.Module, vRef uint64, argsArray, argsLen uint32) (xRef uint64, ok uint32, sp uint32) {
		args := loadArgs(ctx, mod, argsArray, argsLen)
		ref := ref(vRef)
		v := loadValue(ctx, ref)

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

		sp = refreshSP(mod)
		return
	},
))

// ValueLength implements js.valueLength, which is used to load the length
// property of a value, ex. `array.length`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L372
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L396-L397
var ValueLength = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionValueLength, functionValueLength,
	[]string{"v"},
	func(ctx context.Context, mod api.Module, vRef uint64) uint32 {
		v := loadValue(ctx, ref(vRef))
		return uint32(len(v.(*objectArray).slice))
	},
))

// ValuePrepareString implements js.valuePrepareString, which is used to load
// the string for `o.String()` (via js.jsString) for string, boolean and
// number types. Notably, http.Transport uses this in RoundTrip to coerce the
// URL to a string.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L531
// and https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L402-L405
var ValuePrepareString = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionValuePrepareString, functionValuePrepareString,
	[]string{"v"},
	func(ctx context.Context, mod api.Module, vRef uint64) (sRef uint64, sLen uint32) {
		v := loadValue(ctx, ref(vRef))
		s := valueString(v)
		sRef = storeRef(ctx, s)
		sLen = uint32(len(s))
		return
	},
))

// ValueLoadString implements js.valueLoadString, which is used copy a string
// value for `o.String()`.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L533
//
//	https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L410-L412
var ValueLoadString = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionValueLoadString, functionValueLoadString,
	[]string{"v", "bAddr", "bLen"},
	func(ctx context.Context, mod api.Module, vRef uint64, bAddr, bLen uint32) {
		v := loadValue(ctx, ref(vRef))
		s := valueString(v)
		b := mustRead(ctx, mod.Memory(), "b", bAddr, bLen)
		copy(b, s)
	},
))

// ValueInstanceOf is stubbed as it isn't used in Go's main source tree.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L543
var ValueInstanceOf = stubFunction(functionValueInstanceOf)

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
var CopyBytesToGo = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionCopyBytesToGo, functionCopyBytesToGo,
	[]string{"dstAddr", "dstLen", "src"},
	func(ctx context.Context, mod api.Module, dstAddr, dstLen, _ uint32, srcRef uint64) (n, ok uint32) {
		dst := mustRead(ctx, mod.Memory(), "dst", dstAddr, dstLen) // nolint
		v := loadValue(ctx, ref(srcRef))
		if src, isBuf := v.(*byteArray); isBuf {
			n = uint32(copy(dst, src.slice))
			ok = 1
		}
		return
	},
))

// CopyBytesToJS copies linear memory to a JavaScript managed byte array.
// For example, this is used to read an HTTP request body.
//
// # Results
//
//   - n is the count of bytes written.
//   - ok is false if the dst was not a uint8Array.
//
// See https://github.com/golang/go/blob/go1.19/src/syscall/js/js.go#L583
//
//	https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L438-L448
var CopyBytesToJS = spfunc.MustCallFromSP(false, wasm.NewGoFunc(
	functionCopyBytesToJS, functionCopyBytesToJS,
	[]string{"dst", "srcAddr", "srcLen"},
	func(ctx context.Context, mod api.Module, dstRef uint64, srcAddr, srcLen, _ uint32) (n, ok uint32) {
		src := mustRead(ctx, mod.Memory(), "src", srcAddr, srcLen) // nolint
		v := loadValue(ctx, ref(dstRef))
		if dst, isBuf := v.(*byteArray); isBuf {
			if dst != nil { // empty is possible on EOF
				n = uint32(copy(dst.slice, src))
			}
			ok = 1
		}
		return
	},
))

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
func syscallOpen(ctx context.Context, mod api.Module, name string, flags, perm uint32) (uint32, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS(ctx)
	return fsc.OpenFile(ctx, name)
}

const (
	fdStdin = iota
	fdStdout
	fdStderr
)

// fdReader returns a valid reader for the given file descriptor or nil if ErrnoBadf.
func fdReader(ctx context.Context, mod api.Module, fd uint32) io.Reader {
	sysCtx := mod.(*wasm.CallContext).Sys
	if fd == fdStdin {
		return sysCtx.Stdin()
	} else if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
		return nil
	} else {
		return f.File
	}
}

// fdWriter returns a valid writer for the given file descriptor or nil if ErrnoBadf.
func fdWriter(ctx context.Context, mod api.Module, fd uint32) io.Writer {
	sysCtx := mod.(*wasm.CallContext).Sys
	switch fd {
	case fdStdout:
		return sysCtx.Stdout()
	case fdStderr:
		return sysCtx.Stderr()
	default:
		// Check to see if the file descriptor is available
		if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok || f.File == nil {
			return nil
			// fs.FS doesn't declare io.Writer, but implementations such as
			// os.File implement it.
		} else if writer, ok := f.File.(io.Writer); !ok {
			return nil
		} else {
			return writer
		}
	}
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
