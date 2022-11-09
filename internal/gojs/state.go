package gojs

import (
	"context"
	"fmt"
	"math"

	"github.com/tetratelabs/wazero/api"
)

func WithState(ctx context.Context) context.Context {
	s := &state{
		values:      &values{ids: map[interface{}]uint32{}},
		valueGlobal: newJsGlobal(getRoundTripper(ctx)),
		cwd:         "/",
	}
	return context.WithValue(ctx, stateKey{}, s)
}

// stateKey is a context.Context Value key. The value must be a state pointer.
type stateKey struct{}

func getState(ctx context.Context) *state {
	return ctx.Value(stateKey{}).(*state)
}

type event struct {
	// id is the funcWrapper.id
	id     uint32
	this   ref
	args   *objectArray
	result interface{}
}

// get implements jsGet.get
func (e *event) get(_ context.Context, propertyKey string) interface{} {
	switch propertyKey {
	case "id":
		return e.id
	case "this": // ex fs
		return e.this
	case "args":
		return e.args
	}
	panic(fmt.Sprintf("TODO: event.%s", propertyKey))
}

var (
	undefined = struct{ name string }{name: "undefined"}
	NaN       = math.NaN()
)

// loadValue reads up to 8 bytes at the memory offset `addr` to return the
// value written by storeValue.
//
// See https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L122-L133
func loadValue(ctx context.Context, ref ref) interface{} { // nolint
	switch ref {
	case 0:
		return undefined
	case refValueNaN:
		return NaN
	case refValueZero:
		return float64(0)
	case refValueNull:
		return nil
	case refValueTrue:
		return true
	case refValueFalse:
		return false
	case refValueGlobal:
		return getState(ctx).valueGlobal
	case refJsGo:
		return getState(ctx)
	case refObjectConstructor:
		return objectConstructor
	case refArrayConstructor:
		return arrayConstructor
	case refJsProcess:
		return jsProcess
	case refJsfs:
		return jsfs
	case refJsfsConstants:
		return jsfsConstants
	case refUint8ArrayConstructor:
		return uint8ArrayConstructor
	case refJsCrypto:
		return jsCrypto
	case refJsDateConstructor:
		return jsDateConstructor
	case refJsDate:
		return jsDate
	case refHttpHeadersConstructor:
		return headersConstructor
	default:
		if (ref>>32)&nanHead != nanHead { // numbers are passed through as a ref
			return api.DecodeF64(uint64(ref))
		}
		return getState(ctx).values.get(uint32(ref))
	}
}

// loadArgs returns a slice of `len` values at the memory offset `addr`. The
// returned slice is temporary, not stored in state.values.
//
// See https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L191-L199
func loadArgs(ctx context.Context, mod api.Module, sliceAddr, sliceLen uint32) []interface{} { // nolint
	result := make([]interface{}, 0, sliceLen)
	for i := uint32(0); i < sliceLen; i++ { // nolint
		iRef := mustReadUint64Le(ctx, mod.Memory(), "iRef", sliceAddr+i*8)
		result = append(result, loadValue(ctx, ref(iRef)))
	}
	return result
}

// storeRef stores a value prior to returning to wasm from a host function.
// This returns 8 bytes to represent either the value or a reference to it.
// Any side effects besides memory must be cleaned up on wasmExit.
//
// See https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L135-L183
func storeRef(ctx context.Context, v interface{}) uint64 { // nolint
	// allow-list because we control all implementations
	if v == undefined {
		return uint64(refValueUndefined)
	} else if v == nil {
		return uint64(refValueNull)
	} else if r, ok := v.(ref); ok {
		return uint64(r)
	} else if b, ok := v.(bool); ok {
		if b {
			return uint64(refValueTrue)
		} else {
			return uint64(refValueFalse)
		}
	} else if c, ok := v.(*jsVal); ok {
		return uint64(c.ref) // already stored
	} else if _, ok := v.(*event); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagFunction))
	} else if _, ok := v.(funcWrapper); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagFunction))
	} else if _, ok := v.(jsFn); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagFunction))
	} else if _, ok := v.(string); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagString))
	} else if ui, ok := v.(uint32); ok {
		if ui == 0 {
			return uint64(refValueZero)
		}
		return api.EncodeF64(float64(ui)) // numbers are encoded as float and passed through as a ref
	} else if u, ok := v.(uint64); ok {
		return u // float is already encoded as a uint64, doesn't need to be stored.
	} else if f64, ok := v.(float64); ok {
		if f64 == 0 {
			return uint64(refValueZero)
		}
		return api.EncodeF64(f64)
	}
	id := getState(ctx).values.increment(v)
	return uint64(valueRef(id, typeFlagObject))
}

type values struct {
	// Below is needed to avoid exhausting the ID namespace finalizeRef reclaims
	// See https://go-review.googlesource.com/c/go/+/203600

	values      []interface{}          // values indexed by ID, nil
	goRefCounts []uint32               // recount pair-indexed with values
	ids         map[interface{}]uint32 // live values
	idPool      []uint32               // reclaimed IDs (values[i] = nil, goRefCounts[i] nil
}

func (j *values) get(id uint32) interface{} {
	index := id - nextID
	if index >= uint32(len(j.values)) {
		panic(fmt.Errorf("id %d is out of range %d", id, len(j.values)))
	}
	return j.values[index]
}

func (j *values) increment(v interface{}) uint32 {
	id, ok := j.ids[v]
	if !ok {
		if len(j.idPool) == 0 {
			id, j.values, j.goRefCounts = uint32(len(j.values)), append(j.values, v), append(j.goRefCounts, 0)
		} else {
			id, j.idPool = j.idPool[len(j.idPool)-1], j.idPool[:len(j.idPool)-1]
			j.values[id], j.goRefCounts[id] = v, 0
		}
		j.ids[v] = id
	}
	j.goRefCounts[id]++
	return id + nextID
}

func (j *values) decrement(id uint32) {
	// Special IDs are not refcounted.
	if id < nextID {
		return
	}
	id -= nextID
	j.goRefCounts[id]--
	if j.goRefCounts[id] == 0 {
		j.values[id] = nil
		j.idPool = append(j.idPool, id)
	}
}

// state holds state used by the "go" imports used by gojs.
// Note: This is module-scoped.
type state struct {
	values        *values
	_pendingEvent *event

	valueGlobal *jsVal

	// cwd is initially "/"
	cwd string
}

// get implements jsGet.get
func (s *state) get(_ context.Context, propertyKey string) interface{} {
	switch propertyKey {
	case "_pendingEvent":
		return s._pendingEvent
	}
	panic(fmt.Sprintf("TODO: state.%s", propertyKey))
}

// call implements jsCall.call
func (s *state) call(_ context.Context, _ api.Module, this ref, method string, args ...interface{}) (interface{}, error) {
	switch method {
	case "_makeFuncWrapper":
		return funcWrapper(args[0].(float64)), nil
	}
	panic(fmt.Sprintf("TODO: state.%s", method))
}

func (s *state) clear() {
	s.values.values = s.values.values[:0]
	s.values.goRefCounts = s.values.goRefCounts[:0]
	for k := range s.values.ids {
		delete(s.values.ids, k)
	}
	s.values.idPool = s.values.idPool[:0]
	s._pendingEvent = nil
}

func toInt64(arg interface{}) int64 {
	if arg == refValueZero || arg == undefined {
		return 0
	} else if u, ok := arg.(int64); ok {
		return u
	}
	return int64(arg.(float64))
}

func toUint32(arg interface{}) uint32 {
	if arg == refValueZero || arg == undefined {
		return 0
	} else if u, ok := arg.(uint32); ok {
		return u
	}
	return uint32(arg.(float64))
}

// valueString returns the string form of JavaScript string, boolean and number types.
func valueString(v interface{}) string { // nolint
	if s, ok := v.(string); ok {
		return s
	} else {
		return fmt.Sprintf("%v", v)
	}
}
