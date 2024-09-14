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
	const importedGlobalBegin = 99
	for i, tc := range []struct {
		offset wazevoapi.ModuleContextOffsetData
		m      *wasm.ModuleInstance
	}{
		{
			offset: wazevoapi.ModuleContextOffsetData{
				LocalMemoryBegin:                    10,
				ImportedMemoryBegin:                 -1,
				ImportedFunctionsBegin:              -1,
				TablesBegin:                         -1,
				BeforeListenerTrampolines1stElement: -1,
				AfterListenerTrampolines1stElement:  -1,
				GlobalsBegin:                        -1,
			},
			m: &wasm.ModuleInstance{MemoryInstance: &wasm.MemoryInstance{
				Buffer: make([]byte, 0xff),
			}},
		},
		{
			offset: wazevoapi.ModuleContextOffsetData{
				LocalMemoryBegin:                    -1,
				ImportedMemoryBegin:                 30,
				GlobalsBegin:                        -1,
				TablesBegin:                         -1,
				BeforeListenerTrampolines1stElement: -1,
				AfterListenerTrampolines1stElement:  -1,
				ImportedFunctionsBegin:              -1,
			},
			m: &wasm.ModuleInstance{MemoryInstance: &wasm.MemoryInstance{
				Buffer: make([]byte, 0xff),
			}},
		},
		{
			offset: wazevoapi.ModuleContextOffsetData{
				LocalMemoryBegin:                    -1,
				ImportedMemoryBegin:                 -1,
				ImportedFunctionsBegin:              -1,
				GlobalsBegin:                        30,
				TablesBegin:                         100,
				BeforeListenerTrampolines1stElement: 200,
				AfterListenerTrampolines1stElement:  208,
			},
			m: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{
					{
						Me: &moduleEngine{
							parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{GlobalsBegin: importedGlobalBegin}},
							opaque: make([]byte, 1000),
						},
					},
					{},
					{Val: 1},
					{Val: 1, ValHi: 1230},
				},
				Tables:  []*wasm.TableInstance{{}, {}, {}},
				TypeIDs: make([]wasm.FunctionTypeID, 50),
				Source:  &wasm.Module{ImportGlobalCount: 1},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tc.offset.TotalSize = 1000 // arbitrary large number to ensure we don't panic.
			m := &moduleEngine{
				parent: &compiledModule{
					offsets:                   tc.offset,
					listenerBeforeTrampolines: make([]*byte, 100),
					listenerAfterTrampolines:  make([]*byte, 200),
				},
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
					if i < int(tc.m.Source.ImportGlobalCount) {
						actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[int(tc.offset.GlobalsBegin)+16*i:]))
						imported := g.Me.(*moduleEngine)
						expPtr := uintptr(unsafe.Pointer(&imported.opaque[importedGlobalBegin]))
						require.Equal(t, expPtr, actualPtr)
					} else {
						actual := binary.LittleEndian.Uint64(m.opaque[int(tc.offset.GlobalsBegin)+16*i:])
						actualHi := binary.LittleEndian.Uint64(m.opaque[int(tc.offset.GlobalsBegin)+16*i+8:])
						require.Equal(t, g.Val, actual)
						require.Equal(t, g.ValHi, actualHi)
					}
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
			if tc.offset.BeforeListenerTrampolines1stElement >= 0 {
				actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[int(tc.offset.BeforeListenerTrampolines1stElement):]))
				expPtr := uintptr(unsafe.Pointer(&m.parent.listenerBeforeTrampolines[0]))
				require.Equal(t, expPtr, actualPtr)
			}
			if tc.offset.AfterListenerTrampolines1stElement >= 0 {
				actualPtr := uintptr(binary.LittleEndian.Uint64(m.opaque[int(tc.offset.AfterListenerTrampolines1stElement):]))
				expPtr := uintptr(unsafe.Pointer(&m.parent.listenerAfterTrampolines[0]))
				require.Equal(t, expPtr, actualPtr)
			}
		})
	}
}

func TestModuleEngine_ResolveImportedFunction(t *testing.T) {
	var op1, op2 byte = 0xaa, 0xbb
	importing := &moduleEngine{
		opaquePtr: &op1,
		parent: &compiledModule{
			executables:     &executables{executable: make([]byte, 1000)},
			functionOffsets: []int{1, 5, 10},
		},
		module: &wasm.ModuleInstance{
			TypeIDs: []wasm.FunctionTypeID{0, 0, 0, 888, 111, 222, 333},
			Source:  &wasm.Module{FunctionSection: []wasm.Index{4, 5, 6}},
		},
	}
	imported := &moduleEngine{
		opaquePtr: &op2,
		parent: &compiledModule{
			executables:     &executables{executable: make([]byte, 1000)},
			functionOffsets: []int{50, 4},
		},
		module: &wasm.ModuleInstance{
			TypeIDs: []wasm.FunctionTypeID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 999},
			Source:  &wasm.Module{FunctionSection: []wasm.Index{10}},
		},
	}

	const begin = 5000
	m := &moduleEngine{
		opaque:            make([]byte, 10000),
		importedFunctions: make([]importedFunction, 4),
		parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{
			ImportedFunctionsBegin: begin,
		}},
		module: importing.module,
	}

	m.ResolveImportedFunction(0, 4, 0, importing)
	m.ResolveImportedFunction(1, 3, 0, imported)
	m.ResolveImportedFunction(2, 6, 2, importing)
	m.ResolveImportedFunction(3, 5, 1, importing)

	for i, tc := range []struct {
		index      int
		op         *byte
		executable *byte
		expTypeID  wasm.FunctionTypeID
	}{
		{index: 0, op: &op1, executable: &importing.parent.executable[1], expTypeID: 111},
		{index: 1, op: &op2, executable: &imported.parent.executable[50], expTypeID: 888},
		{index: 2, op: &op1, executable: &importing.parent.executable[10], expTypeID: 333},
		{index: 3, op: &op1, executable: &importing.parent.executable[5], expTypeID: 222},
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

func TestModuleEngine_ResolveImportedFunction_recursive(t *testing.T) {
	var importingOp, importedOp byte = 0xaa, 0xbb
	imported := &moduleEngine{
		opaquePtr: &importedOp,
		parent: &compiledModule{
			executables:     &executables{executable: make([]byte, 50)},
			functionOffsets: []int{10},
		},
		module: &wasm.ModuleInstance{
			TypeIDs: []wasm.FunctionTypeID{111},
			Source:  &wasm.Module{FunctionSection: []wasm.Index{0}},
		},
	}
	importing := &moduleEngine{
		opaquePtr: &importingOp,
		parent: &compiledModule{
			executables:     &executables{executable: make([]byte, 1000)},
			functionOffsets: []int{500},
		},
		importedFunctions: []importedFunction{{me: imported, indexInModule: 0}},
		module: &wasm.ModuleInstance{
			TypeIDs: []wasm.FunctionTypeID{999, 222, 0},
			Source:  &wasm.Module{FunctionSection: []wasm.Index{1}},
		},
	}

	const begin = 5000
	m := &moduleEngine{
		opaque:            make([]byte, 10000),
		importedFunctions: make([]importedFunction, 4),
		parent: &compiledModule{offsets: wazevoapi.ModuleContextOffsetData{
			ImportedFunctionsBegin: begin,
		}},
		module: importing.module,
	}

	m.ResolveImportedFunction(0, 0, 0, importing)
	m.ResolveImportedFunction(1, 1, 1, importing)

	for i, tc := range []struct {
		index      int
		op         *byte
		executable *byte
		expTypeID  wasm.FunctionTypeID
	}{
		{index: 0, op: &importedOp, executable: &imported.parent.executable[10], expTypeID: 999},
		{index: 1, op: &importingOp, executable: &importing.parent.executable[500], expTypeID: 222},
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

func Test_newAlignedOpaque(t *testing.T) {
	for i := 0; i < 100; i++ {
		s := 16 * (i + 10)
		buf := newAlignedOpaque(s)
		require.Equal(t, s, len(buf))
		require.Equal(t, 0, int(uintptr(unsafe.Pointer(&buf[0]))&15))
	}
}
