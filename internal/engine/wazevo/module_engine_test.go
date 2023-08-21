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
				TablesBegin:            -1,
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
				TablesBegin:            -1,
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
				TablesBegin:            100,
			},
			m: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{{}, {}, {}, {}, {}, {}},
				Tables:  []*wasm.TableInstance{{}, {}, {}},
				TypeIDs: make([]wasm.FunctionTypeID, 50),
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
			if tc.offset.TablesBegin >= 0 {
				typeIDsPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[int(tc.offset.TypeIDs1stElement):]))
				expPtr := uintptr(unsafe.Pointer(&tc.m.TypeIDs[0]))
				require.Equal(t, expPtr, typeIDsPtr)

				for i, table := range tc.m.Tables {
					actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[int(tc.offset.TablesBegin)+8*i:]))
					expPtr := uintptr(unsafe.Pointer(table))
					require.Equal(t, expPtr, actualPtr)
				}
			}
		})
	}
}

func TestModuleEngine_ResolveImportedFunction(t *testing.T) {
	const begin = 5000
	m := &moduleEngine{
		opaque:            make([]byte, 10000),
		importedFunctions: make([]importedFunction, 4),
		parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{
			ImportedFunctionsBegin: begin,
		}},
	}

	var op1, op2 byte = 0xaa, 0xbb
	im1 := &moduleEngine{
		opaquePtr: &op1,
		parent: &compiledModule{
			executable:      make([]byte, 1000),
			functionOffsets: []compiledFunctionOffset{{offset: 1, goPreambleSize: 4}, {offset: 5, goPreambleSize: 4}, {offset: 10, goPreambleSize: 4}},
		},
		module: &wasm.ModuleInstance{
			TypeIDs: []wasm.FunctionTypeID{0, 0, 0, 0, 111, 222, 333},
			Source:  &wasm.Module{FunctionSection: []wasm.Index{4, 5, 6}},
		},
	}
	im2 := &moduleEngine{
		opaquePtr: &op2,
		parent: &compiledModule{
			executable:      make([]byte, 1000),
			functionOffsets: []compiledFunctionOffset{{offset: 50, goPreambleSize: 4}},
		},
		module: &wasm.ModuleInstance{
			TypeIDs: []wasm.FunctionTypeID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 999},
			Source:  &wasm.Module{FunctionSection: []wasm.Index{10}},
		},
	}

	m.ResolveImportedFunction(0, 0, im1)
	m.ResolveImportedFunction(1, 0, im2)
	m.ResolveImportedFunction(2, 2, im1)
	m.ResolveImportedFunction(3, 1, im1)

	for i, tc := range []struct {
		index      int
		op         *byte
		executable *byte
		expTypeID  wasm.FunctionTypeID
	}{
		{index: 0, op: &op1, executable: &im1.parent.executable[1+4], expTypeID: 111},
		{index: 1, op: &op2, executable: &im2.parent.executable[50+4], expTypeID: 999},
		{index: 2, op: &op1, executable: &im1.parent.executable[10+4], expTypeID: 333},
		{index: 3, op: &op1, executable: &im1.parent.executable[5+4], expTypeID: 222},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			buf := m.opaque[begin+wazevoapi.FunctionInstanceSize*tc.index:]
			actualExecutable := binary.LittleEndian.Uint64(buf)
			actualOpaquePtr := binary.LittleEndian.Uint64(buf[8:])
			actualTypeID := binary.LittleEndian.Uint64(buf[16:])
			expExecutable := uint64(uintptr(unsafe.Pointer(tc.executable)))
			expOpaquePtr := uint64(uintptr(unsafe.Pointer(tc.op)))
			require.Equal(t, expExecutable, actualExecutable)
			require.Equal(t, expOpaquePtr, actualOpaquePtr)
			require.Equal(t, uint64(tc.expTypeID), actualTypeID)
		})
	}
}

func TestModuleEngine_ResolveImportedMemory_reexported(t *testing.T) {
	m := &moduleEngine{
		parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{
			ImportedMemoryBegin: 50,
		}},
		opaque: make([]byte, 100),
	}

	importedME := &moduleEngine{
		parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{
			ImportedMemoryBegin: 1000,
		}},
		opaque: make([]byte, 2000),
	}
	binary.LittleEndian.PutUint64(importedME.opaque[1000:], 0x1234567890abcdef)
	binary.LittleEndian.PutUint64(importedME.opaque[1000+8:], 0xabcdef1234567890)

	m.ResolveImportedMemory(importedME)
	require.Equal(t, uint64(0x1234567890abcdef), binary.LittleEndian.Uint64(m.opaque[50:]))
	require.Equal(t, uint64(0xabcdef1234567890), binary.LittleEndian.Uint64(m.opaque[50+8:]))
}

func Test_functionInstance_offsets(t *testing.T) {
	var fi functionInstance
	require.Equal(t, wazevoapi.FunctionInstanceSize, int(unsafe.Sizeof(fi)))
	require.Equal(t, wazevoapi.FunctionInstanceExecutableOffset, int(unsafe.Offsetof(fi.executable)))
	require.Equal(t, wazevoapi.FunctionInstanceModuleContextOpaquePtrOffset, int(unsafe.Offsetof(fi.moduleContextOpaquePtr)))
	require.Equal(t, wazevoapi.FunctionInstanceTypeIDOffset, int(unsafe.Offsetof(fi.typeID)))

	m := wazevoapi.ModuleContextOffsetData{ImportedFunctionsBegin: 100}
	ptr, moduleCtx, typeID := m.ImportedFunctionOffset(10)
	require.Equal(t, 100+10*wazevoapi.FunctionInstanceSize, int(ptr))
	require.Equal(t, moduleCtx, ptr+8)
	require.Equal(t, typeID, ptr+16)
}

func Test_getTypeIDOf(t *testing.T) {
	m := &wasm.ModuleInstance{
		TypeIDs: []wasm.FunctionTypeID{111, 222, 333, 444},
		Source: &wasm.Module{
			ImportFunctionCount: 1,
			ImportSection: []wasm.Import{
				{Type: wasm.ExternTypeMemory},
				{Type: wasm.ExternTypeTable},
				{Type: wasm.ExternTypeFunc, DescFunc: 3},
			},
			FunctionSection: []wasm.Index{2, 1, 0},
		},
	}

	require.Equal(t, wasm.FunctionTypeID(444), getTypeIDOf(0, m))
	require.Equal(t, wasm.FunctionTypeID(333), getTypeIDOf(1, m))
	require.Equal(t, wasm.FunctionTypeID(222), getTypeIDOf(2, m))
	require.Equal(t, wasm.FunctionTypeID(111), getTypeIDOf(3, m))
}
