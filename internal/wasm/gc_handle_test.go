package wasm

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestGCTaggedPointer_roundTrip(t *testing.T) {
	s := newStore()
	m := &ModuleInstance{s: s}

	st := &WasmStruct{TypeID: 1}
	ar := &WasmArray{TypeID: 2}
	h1 := m.GCRegister(st)
	h2 := m.GCRegister(ar)

	require.True(t, IsGCRef(h1))
	require.True(t, IsGCRef(h2))
	require.True(t, IsGCStructRef(h1))
	require.False(t, IsGCArrayRef(h1))
	require.True(t, IsGCArrayRef(h2))
	require.False(t, IsGCStructRef(h2))

	// Untag and recover the original pointers.
	got1 := (*WasmStruct)(UntagGCPointer(h1))
	got2 := (*WasmArray)(UntagGCPointer(h2))
	require.Equal(t, st, got1)
	require.Equal(t, ar, got2)

	// Objects are rooted.
	require.Equal(t, 2, len(m.GCRoots))
}

func TestTagGCPointer_nullIsNotGCRef(t *testing.T) {
	require.False(t, IsGCRef(0))
}

func TestTagGCPointer_highBitsClear(t *testing.T) {
	st := &WasmStruct{TypeID: 42}
	ptr := uint64(uintptr(unsafe.Pointer(st)))
	require.Equal(t, uint64(0), ptr&tagMask)
}
