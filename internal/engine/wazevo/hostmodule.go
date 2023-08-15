package wazevo

import (
	"encoding/binary"
	"reflect"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func buildHostModuleOpaque(m *wasm.Module) moduleContextOpaque {
	size := len(m.CodeSection) * 16
	ret := make(moduleContextOpaque, size)

	var offset int
	for i := range m.CodeSection {
		goFn := m.CodeSection[i].GoFunc
		writeIface(goFn, ret[offset:])
		offset += 16
	}
	return ret
}

func hostModuleGoFuncFromOpaque[T any](index int, opaqueBegin uintptr) T {
	offset := uintptr(index * 16)
	ptr := opaqueBegin + offset

	var opaqueViewOverFunction []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&opaqueViewOverFunction))
	sh.Data = ptr
	sh.Len = 16
	sh.Cap = 16
	return readIface(opaqueViewOverFunction).(T)
}

func writeIface(goFn interface{}, buf []byte) {
	goFnIface := *(*[2]uint64)(unsafe.Pointer(&goFn))
	binary.LittleEndian.PutUint64(buf, goFnIface[0])
	binary.LittleEndian.PutUint64(buf[8:], goFnIface[1])
}

func readIface(buf []byte) interface{} {
	b := binary.LittleEndian.Uint64(buf)
	s := binary.LittleEndian.Uint64(buf[8:])
	return *(*interface{})(unsafe.Pointer(&[2]uint64{b, s}))
}
