package wazevo

import (
	"encoding/binary"
	"runtime"
	"strconv"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestModuleEngine_setupOpaque(t *testing.T) {
	for i, tc := range []struct {
		offset wazevoapi.ModuleContextOffsetData
		m      *wasm.ModuleInstance
	}{
		{
			offset: wazevoapi.ModuleContextOffsetData{
				LocalMemoryBegin:       10,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           -1,
			},
			m: &wasm.ModuleInstance{MemoryInstance: &wasm.MemoryInstance{
				Buffer: make([]byte, 0xff),
			}},
		},
		{
			offset: wazevoapi.ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    30,
				GlobalsBegin:           -1,
				ImportedFunctionsBegin: -1,
			},
			m: &wasm.ModuleInstance{MemoryInstance: &wasm.MemoryInstance{
				Buffer: make([]byte, 0xff),
			}},
		},
		{
			offset: wazevoapi.ModuleContextOffsetData{
				LocalMemoryBegin:       -1,
				ImportedMemoryBegin:    -1,
				ImportedFunctionsBegin: -1,
				GlobalsBegin:           30,
			},
			m: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{{}, {}, {}, {}, {}, {}},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tc.offset.TotalSize = 1000 // arbitrary large number to ensure we don't panic.
			m := &moduleEngine{
				parent: &compiledModule{offsets: tc.offset},
				module: tc.m,
				opaque: make([]byte, tc.offset.TotalSize),
			}
			m.setupOpaque()

			if tc.offset.LocalMemoryBegin >= 0 {
				actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[tc.offset.LocalMemoryBegin:]))
				expPtr := uintptr(unsafe.Pointer(&tc.m.MemoryInstance.Buffer[0]))
				require.Equal(t, expPtr, actualPtr)
				actualLen := int(binary.LittleEndian.Uint64(m.opaque[tc.offset.LocalMemoryBegin+8:]))
				expLen := len(tc.m.MemoryInstance.Buffer)
				require.Equal(t, expLen, actualLen)
			}
			if tc.offset.ImportedMemoryBegin >= 0 {
				imported := &moduleEngine{
					opaque: []byte{1, 2, 3}, module: &wasm.ModuleInstance{MemoryInstance: tc.m.MemoryInstance},
					parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{ImportedMemoryBegin: -1}},
				}
				imported.opaquePtr = &imported.opaque[0]
				m.ResolveImportedMemory(imported)

				actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[tc.offset.ImportedMemoryBegin:]))
				expPtr := uintptr(unsafe.Pointer(tc.m.MemoryInstance))
				require.Equal(t, expPtr, actualPtr)

				actualOpaquePtr := uintptr(binary.LittleEndian.Uint64(m.opaque[tc.offset.ImportedMemoryBegin+8:]))
				require.Equal(t, uintptr(unsafe.Pointer(imported.opaquePtr)), actualOpaquePtr)
				runtime.KeepAlive(imported)
			}
			if tc.offset.GlobalsBegin >= 0 {
				for i, g := range tc.m.Globals {
					actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[int(tc.offset.GlobalsBegin)+8*i:]))
					expPtr := uintptr(unsafe.Pointer(g))
					require.Equal(t, expPtr, actualPtr)
				}
			}
		})
	}
}

func TestModuleEngine_ResolveImportedFunction(t *testing.T) {
	const begin = 5000
	m := &moduleEngine{opaque: make([]byte, 10000), parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{
		ImportedFunctionsBegin: begin,
	}}}

	var op1, op2 byte = 0xaa, 0xbb
	im1 := &moduleEngine{
		opaquePtr: &op1,
		parent: &compiledModule{
			executable:      make([]byte, 1000),
			functionOffsets: []compiledFunctionOffset{{offset: 1, goPreambleSize: 4}, {offset: 5, goPreambleSize: 4}, {offset: 10, goPreambleSize: 4}},
		},
	}
	im2 := &moduleEngine{
		opaquePtr: &op2,
		parent: &compiledModule{
			executable:      make([]byte, 1000),
			functionOffsets: []compiledFunctionOffset{{offset: 50, goPreambleSize: 4}},
		},
	}

	m.ResolveImportedFunction(0, 0, im1)
	m.ResolveImportedFunction(1, 0, im2)
	m.ResolveImportedFunction(2, 2, im1)
	m.ResolveImportedFunction(3, 1, im1)

	for _, tc := range []struct {
		index      int
		op         *byte
		executable *byte
	}{
		{index: 0, op: &op1, executable: &im1.parent.executable[1+4]},
		{index: 1, op: &op2, executable: &im2.parent.executable[50+4]},
		{index: 2, op: &op1, executable: &im1.parent.executable[10+4]},
		{index: 3, op: &op1, executable: &im1.parent.executable[5+4]},
	} {
		buf := m.opaque[begin+16*tc.index:]
		actualExecutable := binary.LittleEndian.Uint64(buf)
		actualOpaquePtr := binary.LittleEndian.Uint64(buf[8:])
		expExecutable := uint64(uintptr(unsafe.Pointer(tc.executable)))
		expOpaquePtr := uint64(uintptr(unsafe.Pointer(tc.op)))
		require.Equal(t, expExecutable, actualExecutable)
		require.Equal(t, expOpaquePtr, actualOpaquePtr)
	}
}
