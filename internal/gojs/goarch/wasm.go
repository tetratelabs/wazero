// Package goarch isolates code from runtime.GOARCH=wasm in a way that avoids
// cyclic dependencies when re-used from other packages.
package goarch

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs/util"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// StubFunction stubs functions not used in Go's main source tree.
// This traps (unreachable opcode) to ensure the function is never called.
func StubFunction(name string) *wasm.HostFunc {
	return &wasm.HostFunc{
		ExportNames: []string{name},
		Name:        name,
		ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
		ParamNames:  []string{"sp"},
		Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeUnreachable, wasm.OpcodeEnd}},
	}
}

func NoopFunction(name string) *wasm.HostFunc {
	return &wasm.HostFunc{
		ExportNames: []string{name},
		Name:        name,
		ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
		ParamNames:  []string{"sp"},
		Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeEnd}},
	}
}

var le = binary.LittleEndian

type Stack interface {
	Param(i int) uint64
	ParamName(i int) string

	// ParamBytes reads a byte slice, given its memory offset and length (stack
	// positions i, i+1)
	ParamBytes(mem api.Memory, i int) []byte

	// ParamString reads a string, given its memory offset and length (stack
	// positions i, i+1)
	ParamString(mem api.Memory, i int) string

	ParamUint32(i int) uint32

	// Refresh the stack from the current stack pointer (SP).
	//
	// Note: This is needed prior to storing a value when in an operation that
	// can trigger a Go event handler.
	Refresh(api.Module)

	ResultName(i int) string

	SetResult(i int, v uint64)

	SetResultBool(i int, v bool)

	SetResultI32(i int, v int32)

	SetResultI64(i int, v int64)

	SetResultUint32(i int, v uint32)
}

func NewStack(mem api.Memory, sp uint32, paramNames, resultNames []string) Stack {
	s := &stack{paramNames: paramNames, resultNames: resultNames}
	s.refresh(mem, sp)
	return s
}

type stack struct {
	paramNames, resultNames []string
	buf                     []byte
}

// Param implements Stack.Param
func (s *stack) Param(i int) (res uint64) {
	pos := i << 3
	res = le.Uint64(s.buf[pos:])
	return
}

// ParamName implements Stack.ParamName
func (s *stack) ParamName(i int) string {
	return s.paramNames[i]
}

// ParamBytes implements Stack.ParamBytes
func (s *stack) ParamBytes(mem api.Memory, i int) (res []byte) {
	offset := s.ParamUint32(i)
	byteCount := s.ParamUint32(i + 1)
	return mustRead(mem, s.paramNames[i], offset, byteCount)
}

// ParamString implements Stack.ParamString
func (s *stack) ParamString(mem api.Memory, i int) string {
	return string(s.ParamBytes(mem, i))
}

// ParamUint32 implements Stack.ParamUint32
func (s *stack) ParamUint32(i int) uint32 {
	return uint32(s.Param(i))
}

// Refresh implements Stack.Refresh
func (s *stack) Refresh(mod api.Module) {
	s.refresh(mod.Memory(), getSP(mod))
}

func (s *stack) refresh(mem api.Memory, sp uint32) {
	count := uint32(len(s.paramNames) + len(s.resultNames))
	s.buf = mustRead(mem, "sp", sp+8, count<<3)
}

// SetResult implements Stack.SetResult
func (s *stack) SetResult(i int, v uint64) {
	pos := (len(s.paramNames) + i) << 3
	le.PutUint64(s.buf[pos:], v)
}

// ResultName implements Stack.ResultName
func (s *stack) ResultName(i int) string {
	return s.resultNames[i]
}

// SetResultBool implements Stack.SetResultBool
func (s *stack) SetResultBool(i int, v bool) {
	if v {
		s.SetResultUint32(i, 1)
	} else {
		s.SetResultUint32(i, 0)
	}
}

// SetResultI32 implements Stack.SetResultI32
func (s *stack) SetResultI32(i int, v int32) {
	s.SetResult(i, uint64(v))
}

// SetResultI64 implements Stack.SetResultI64
func (s *stack) SetResultI64(i int, v int64) {
	s.SetResult(i, uint64(v))
}

// SetResultUint32 implements Stack.SetResultUint32
func (s *stack) SetResultUint32(i int, v uint32) {
	s.SetResult(i, uint64(v))
}

// getSP gets the stack pointer, which is needed prior to storing a value when
// in an operation that can trigger a Go event handler.
//
// See https://github.com/golang/go/blob/go1.19/misc/wasm/wasm_exec.js#L210-L213
func getSP(mod api.Module) uint32 {
	// Cheat by reading global[0] directly instead of through a function proxy.
	// https://github.com/golang/go/blob/go1.19/src/runtime/rt0_js_wasm.s#L87-L90
	return uint32(mod.(*wasm.CallContext).GlobalVal(0))
}

// mustRead is like api.Memory except that it panics if the offset and
// byteCount are out of range.
func mustRead(mem api.Memory, fieldName string, offset, byteCount uint32) []byte {
	buf, ok := mem.Read(offset, byteCount)
	if !ok {
		panic(fmt.Errorf("out of memory reading %s", fieldName))
	}
	return buf
}

func NewFunc(name string, goFunc Func, paramNames, resultNames []string) *wasm.HostFunc {
	return util.NewFunc(name, (&stackFunc{f: goFunc, paramNames: paramNames, resultNames: resultNames}).Call)
}

type Func func(context.Context, api.Module, Stack)

type stackFunc struct {
	f                       Func
	paramNames, resultNames []string
}

// Call implements the same method as defined on api.GoModuleFunction.
func (f *stackFunc) Call(ctx context.Context, mod api.Module, wasmStack []uint64) {
	s := NewStack(mod.Memory(), uint32(wasmStack[0]), f.paramNames, f.resultNames)
	f.f(ctx, mod, s)
}
