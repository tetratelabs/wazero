package gojs

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Debug has unknown use, so stubbed.
//
// See https://github.com/golang/go/blob/go1.19/src/cmd/link/internal/wasm/asm.go#L133-L138
var Debug = stubFunction(functionDebug)

// stubFunction stubs functions not used in Go's main source tree.
func stubFunction(name string) *wasm.HostFunc {
	return &wasm.HostFunc{
		ExportNames: []string{name},
		Name:        name,
		ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
		ParamNames:  []string{parameterSp},
		Code:        &wasm.Code{IsHostFunction: true, Body: []byte{wasm.OpcodeUnreachable, wasm.OpcodeEnd}},
	}
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

// mustReadUint32Le is like api.Memory except that it panics if the offset
// is out of range.
func mustReadUint32Le(mem api.Memory, fieldName string, offset uint32) uint32 {
	result, ok := mem.ReadUint32Le(offset)
	if !ok {
		panic(fmt.Errorf("out of memory reading %s", fieldName))
	}
	return result
}

// mustReadUint64Le is like api.Memory except that it panics if the offset
// is out of range.
func mustReadUint64Le(mem api.Memory, fieldName string, offset uint32) uint64 {
	result, ok := mem.ReadUint64Le(offset)
	if !ok {
		panic(fmt.Errorf("out of memory reading %s", fieldName))
	}
	return result
}

// mustWrite is like api.Memory except that it panics if the offset
// is out of range.
func mustWrite(mem api.Memory, fieldName string, offset uint32, val []byte) {
	if ok := mem.Write(offset, val); !ok {
		panic(fmt.Errorf("out of memory writing %s", fieldName))
	}
}

// mustWriteUint64Le is like api.Memory except that it panics if the offset
// is out of range.
func mustWriteUint64Le(mem api.Memory, fieldName string, offset uint32, val uint64) {
	if ok := mem.WriteUint64Le(offset, val); !ok {
		panic(fmt.Errorf("out of memory writing %s", fieldName))
	}
}
