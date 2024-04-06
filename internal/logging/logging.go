// Package logging includes utilities used to log function calls. This is in
// an independent package to avoid dependency cycles.
package logging

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/api"
)

// ValueType is an extended form of api.ValueType, used to control logging in
// cases such as bitmasks or strings.
type ValueType = api.ValueType

const (
	ValueTypeI32                 = api.ValueTypeI32
	ValueTypeI64                 = api.ValueTypeI64
	ValueTypeF32                 = api.ValueTypeF32
	ValueTypeF64                 = api.ValueTypeF64
	ValueTypeV128      ValueType = 0x7b // same as wasm.ValueTypeV128
	ValueTypeFuncref   ValueType = 0x70 // same as wasm.ValueTypeFuncref
	ValueTypeExternref           = api.ValueTypeExternref

	// ValueTypeMemI32 is a non-standard type which writes ValueTypeI32 from the memory offset.
	ValueTypeMemI32 = 0xfd
	// ValueTypeMemH64 is a non-standard type which writes 64-bits fixed-width hex from the memory offset.
	ValueTypeMemH64 = 0xfe
	// ValueTypeString is a non-standard type describing an offset/len pair of a string.
	ValueTypeString = 0xff
)

type LogScopes uint64

const (
	LogScopeNone            = LogScopes(0)
	LogScopeClock LogScopes = 1 << iota
	LogScopeProc
	LogScopeFilesystem
	LogScopeMemory
	LogScopePoll
	LogScopeRandom
	LogScopeSock
	LogScopeAll = LogScopes(0xffffffffffffffff)
)

func scopeName(s LogScopes) string {
	switch s {
	case LogScopeClock:
		return "clock"
	case LogScopeProc:
		return "proc"
	case LogScopeFilesystem:
		return "filesystem"
	case LogScopeMemory:
		return "memory"
	case LogScopePoll:
		return "poll"
	case LogScopeRandom:
		return "random"
	case LogScopeSock:
		return "sock"
	default:
		return fmt.Sprintf("<unknown=%d>", s)
	}
}

// IsEnabled returns true if the scope (or group of scopes) is enabled.
func (f LogScopes) IsEnabled(scope LogScopes) bool {
	return f&scope != 0
}

// String implements fmt.Stringer by returning each enabled log scope.
func (f LogScopes) String() string {
	if f == LogScopeAll {
		return "all"
	}
	var builder strings.Builder
	for i := 0; i <= 63; i++ { // cycle through all bits to reduce code and maintenance
		target := LogScopes(1 << i)
		if f.IsEnabled(target) {
			if name := scopeName(target); name != "" {
				if builder.Len() > 0 {
					builder.WriteByte('|')
				}
				builder.WriteString(name)
			}
		}
	}
	return builder.String()
}

type ParamLogger func(ctx context.Context, mod api.Module, w Writer, params []uint64)

type ParamSampler func(ctx context.Context, mod api.Module, params []uint64) bool

type ResultLogger func(ctx context.Context, mod api.Module, w Writer, params, results []uint64)

type Writer interface {
	io.Writer
	io.StringWriter
	io.ByteWriter
}

// ValWriter formats an indexed value. For example, if `vals[i]` is a
// ValueTypeI32, this would format it by default as signed. If a
// ValueTypeString, it would read `vals[i+1]` and write the string from memory.
type ValWriter func(ctx context.Context, mod api.Module, w Writer, i uint32, vals []uint64)

func Config(fnd api.FunctionDefinition) (paramLoggers []ParamLogger, resultLoggers []ResultLogger) {
	types := fnd.ParamTypes()
	names := fnd.ParamNames()
	if paramLen := uint32(len(types)); paramLen > 0 {
		paramLoggers = make([]ParamLogger, paramLen)
		hasParamNames := len(names) > 0
		var offset int64
		for i, t := range types {
			if hasParamNames {
				paramLoggers[i] = NewParamLogger(uint32(offset), names[i], t)
			} else {
				paramLoggers[i] = (&paramLogger{offsetInStack: uint32(offset), valWriter: ValWriterForType(t)}).Log
			}
			offset++
			if t == ValueTypeV128 {
				offset++
			}
		}
	}
	if resultLen := uint32(len(fnd.ResultTypes())); resultLen > 0 {
		resultLoggers = make([]ResultLogger, resultLen)
		hasResultNames := len(fnd.ResultNames()) > 0
		var offset int64
		for i, t := range fnd.ResultTypes() {
			if hasResultNames {
				resultLoggers[i] = NewResultLogger(uint32(offset), fnd.ResultNames()[i], t)
			} else {
				resultLoggers[i] = (&resultLogger{offsetInStack: uint32(offset), valWriter: ValWriterForType(t)}).Log
			}
			offset++
			if t == ValueTypeV128 {
				offset++
			}
		}
	}
	return
}

type paramLogger struct {
	offsetInStack uint32
	valWriter     ValWriter
}

func (n *paramLogger) Log(ctx context.Context, mod api.Module, w Writer, params []uint64) {
	n.valWriter(ctx, mod, w, n.offsetInStack, params)
}

func NewParamLogger(offsetInStack uint32, name string, t ValueType) ParamLogger {
	return (&namedParamLogger{offsetInStack: offsetInStack, name: name, valWriter: ValWriterForType(t)}).Log
}

type namedParamLogger struct {
	offsetInStack uint32
	name          string
	valWriter     ValWriter
}

func (n *namedParamLogger) Log(ctx context.Context, mod api.Module, w Writer, params []uint64) {
	w.WriteString(n.name) //nolint
	w.WriteByte('=')      //nolint
	n.valWriter(ctx, mod, w, n.offsetInStack, params)
}

type resultLogger struct {
	offsetInStack uint32
	valWriter     ValWriter
}

func (n *resultLogger) Log(ctx context.Context, mod api.Module, w Writer, _, results []uint64) {
	n.valWriter(ctx, mod, w, n.offsetInStack, results)
}

func NewResultLogger(idx uint32, name string, t ValueType) ResultLogger {
	return (&namedResultLogger{idx, name, ValWriterForType(t)}).Log
}

type namedResultLogger struct {
	offsetInStack uint32
	name          string
	valWriter     ValWriter
}

func (n *namedResultLogger) Log(ctx context.Context, mod api.Module, w Writer, _, results []uint64) {
	w.WriteString(n.name) //nolint
	w.WriteByte('=')      //nolint
	n.valWriter(ctx, mod, w, n.offsetInStack, results)
}

func ValWriterForType(vt ValueType) ValWriter {
	switch vt {
	case ValueTypeI32:
		return writeI32
	case ValueTypeI64:
		return writeI64
	case ValueTypeF32:
		return writeF32
	case ValueTypeF64:
		return writeF64
	case ValueTypeV128:
		return writeV128
	case ValueTypeExternref, ValueTypeFuncref:
		return writeRef
	case ValueTypeMemI32:
		return writeMemI32
	case ValueTypeMemH64:
		return writeMemH64
	case ValueTypeString:
		return writeString
	default:
		panic(fmt.Errorf("BUG: unsupported type %d", vt))
	}
}

func writeI32(_ context.Context, _ api.Module, w Writer, i uint32, vals []uint64) {
	v := vals[i]
	w.WriteString(strconv.FormatInt(int64(int32(v)), 10)) //nolint
}

func writeI64(_ context.Context, _ api.Module, w Writer, i uint32, vals []uint64) {
	v := vals[i]
	w.WriteString(strconv.FormatInt(int64(v), 10)) //nolint
}

func writeF32(_ context.Context, _ api.Module, w Writer, i uint32, vals []uint64) {
	v := vals[i]
	s := strconv.FormatFloat(float64(api.DecodeF32(v)), 'g', -1, 32)
	w.WriteString(s) //nolint
}

func writeF64(_ context.Context, _ api.Module, w Writer, i uint32, vals []uint64) {
	v := vals[i]
	s := strconv.FormatFloat(api.DecodeF64(v), 'g', -1, 64)
	w.WriteString(s) //nolint
}

// logV128 logs in fixed-width hex
func writeV128(_ context.Context, _ api.Module, w Writer, i uint32, vals []uint64) {
	v1, v2 := vals[i], vals[i+1]
	w.WriteString(fmt.Sprintf("%016x%016x", v1, v2)) //nolint
}

// logRef logs in fixed-width hex
func writeRef(_ context.Context, _ api.Module, w Writer, i uint32, vals []uint64) {
	v := vals[i]
	w.WriteString(fmt.Sprintf("%016x", v)) //nolint
}

func writeMemI32(_ context.Context, mod api.Module, w Writer, i uint32, vals []uint64) {
	offset := uint32(vals[i])
	byteCount := uint32(4)
	if v, ok := mod.Memory().ReadUint32Le(offset); ok {
		w.WriteString(strconv.FormatInt(int64(int32(v)), 10)) //nolint
	} else { // log the positions that were out of memory
		WriteOOM(w, offset, byteCount)
	}
}

func writeMemH64(_ context.Context, mod api.Module, w Writer, i uint32, vals []uint64) {
	offset := uint32(vals[i])
	byteCount := uint32(8)
	if s, ok := mod.Memory().Read(offset, byteCount); ok {
		hex.NewEncoder(w).Write(s) //nolint
	} else { // log the positions that were out of memory
		WriteOOM(w, offset, byteCount)
	}
}

func writeString(_ context.Context, mod api.Module, w Writer, i uint32, vals []uint64) {
	offset, byteCount := uint32(vals[i]), uint32(vals[i+1])
	WriteStringOrOOM(mod.Memory(), w, offset, byteCount)
}

func WriteStringOrOOM(mem api.Memory, w Writer, offset, byteCount uint32) {
	if s, ok := mem.Read(offset, byteCount); ok {
		w.Write(s) //nolint
	} else { // log the positions that were out of memory
		WriteOOM(w, offset, byteCount)
	}
}

func WriteOOM(w Writer, offset uint32, byteCount uint32) {
	w.WriteString("OOM(")                       //nolint
	w.WriteString(strconv.Itoa(int(offset)))    //nolint
	w.WriteByte(',')                            //nolint
	w.WriteString(strconv.Itoa(int(byteCount))) //nolint
	w.WriteByte(')')                            //nolint
}
