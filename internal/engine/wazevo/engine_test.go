package wazevo

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_sharedFunctionsFinalizer(t *testing.T) {
	sf := &sharedFunctions{}

	b1, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b2, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b3, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b6, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b7, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b8, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b9, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)
	b10, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)

	sf.memoryGrowExecutable = b1
	sf.stackGrowExecutable = b2
	sf.checkModuleExitCode = b3
	sf.tableGrowExecutable = b6
	sf.refFuncExecutable = b7
	sf.memoryWait32Executable = b8
	sf.memoryWait64Executable = b9
	sf.memoryNotifyExecutable = b10

	sharedFunctionsFinalizer(sf)
	require.Nil(t, sf.memoryGrowExecutable)
	require.Nil(t, sf.stackGrowExecutable)
	require.Nil(t, sf.checkModuleExitCode)
	require.Nil(t, sf.tableGrowExecutable)
	require.Nil(t, sf.refFuncExecutable)
	require.Nil(t, sf.memoryWait32Executable)
	require.Nil(t, sf.memoryWait64Executable)
	require.Nil(t, sf.memoryNotifyExecutable)
}

func Test_executablesFinalizer(t *testing.T) {
	b, err := platform.MmapCodeSegment(100)
	require.NoError(t, err)

	exec := &executables{}
	exec.executable = b
	executablesFinalizer(exec)
	require.Nil(t, exec.executable)
}

type fakeFinalizer map[*executables]func(module *executables)

func (f fakeFinalizer) setFinalizer(obj interface{}, finalizer interface{}) {
	cf := obj.(*executables)
	if _, ok := f[cf]; ok { // easier than adding a field for testing.T
		panic(fmt.Sprintf("BUG: %v already had its finalizer set", cf))
	}
	f[cf] = finalizer.(func(*executables))
}

func TestEngine_CompileModule(t *testing.T) {
	ctx := context.Background()
	e := NewEngine(ctx, 0, nil).(*engine)
	ff := fakeFinalizer{}
	e.setFinalizer = ff.setFinalizer

	okModule := &wasm.Module{
		TypeSection:     []wasm.FunctionType{{}},
		FunctionSection: []wasm.Index{0, 0, 0, 0},
		CodeSection: []wasm.Code{
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
			{Body: []byte{wasm.OpcodeEnd}},
		},
		ID: wasm.ModuleID{},
	}

	err := e.CompileModule(ctx, okModule, nil, false)
	require.NoError(t, err)

	// Compiling same module shouldn't be compiled again, but instead should be cached.
	err = e.CompileModule(ctx, okModule, nil, false)
	require.NoError(t, err)

	// Pretend the finalizer executed, by invoking them one-by-one.
	for k, v := range ff {
		v(k)
	}
}

func TestEngine_sortedCompiledModules(t *testing.T) {
	requireEqualExisting := func(t *testing.T, e *engine, expected []uintptr) {
		actual := make([]uintptr, 0)
		for _, cm := range e.sortedCompiledModules {
			actual = append(actual, uintptr(unsafe.Pointer(&cm.executable[0])))
		}
		require.Equal(t, expected, actual)
	}

	var cms []struct {
		cm   *compiledModule
		addr uintptr
	}
	for i := 0; i < 4; i++ {
		cm := &compiledModule{executables: &executables{executable: make([]byte, 100)}}
		cms = append(cms, struct {
			cm   *compiledModule
			addr uintptr
		}{cm, uintptr(unsafe.Pointer(&cm.executables.executable[0]))})
	}

	sort.Slice(cms, func(i, j int) bool {
		return cms[i].addr < cms[j].addr
	})
	cm1, addr1 := cms[0].cm, cms[0].addr
	cm2, addr2 := cms[1].cm, cms[1].addr
	cm3, addr3 := cms[2].cm, cms[2].addr
	cm4, addr4 := cms[3].cm, cms[3].addr

	t.Run("add", func(t *testing.T) {
		e := &engine{}
		e.addCompiledModuleToSortedList(cm1)
		e.addCompiledModuleToSortedList(cm4)
		e.addCompiledModuleToSortedList(cm2)
		e.addCompiledModuleToSortedList(cm3)
		requireEqualExisting(t, e, []uintptr{addr1, addr2, addr3, addr4})
	})
	t.Run("delete", func(t *testing.T) {
		e := &engine{}
		e.addCompiledModuleToSortedList(cm1)
		e.addCompiledModuleToSortedList(cm4)
		e.addCompiledModuleToSortedList(cm2)
		e.addCompiledModuleToSortedList(cm3)
		e.deleteCompiledModuleFromSortedList(cm4)
		require.Equal(t, 3, len(e.sortedCompiledModules))
		requireEqualExisting(t, e, []uintptr{addr1, addr2, addr3})
		e.deleteCompiledModuleFromSortedList(cm2)
		requireEqualExisting(t, e, []uintptr{addr1, addr3})
		e.deleteCompiledModuleFromSortedList(cm1)
		requireEqualExisting(t, e, []uintptr{addr3})
		e.deleteCompiledModuleFromSortedList(cm3)
		requireEqualExisting(t, e, []uintptr{})
	})

	t.Run("OfAddr", func(t *testing.T) {
		e := &engine{}
		e.addCompiledModuleToSortedList(cm1)
		e.addCompiledModuleToSortedList(cm4)
		e.addCompiledModuleToSortedList(cm2)
		e.addCompiledModuleToSortedList(cm3)

		require.Equal(t, nil, e.compiledModuleOfAddr(0))
		require.Equal(t, unsafe.Pointer(cm1), unsafe.Pointer(e.compiledModuleOfAddr(addr1)))
		require.Equal(t, unsafe.Pointer(cm1), unsafe.Pointer(e.compiledModuleOfAddr(addr1+10)))
		require.Equal(t, unsafe.Pointer(cm2), unsafe.Pointer(e.compiledModuleOfAddr(addr2)))
		require.Equal(t, unsafe.Pointer(cm2), unsafe.Pointer(e.compiledModuleOfAddr(addr2+50)))
		require.Equal(t, unsafe.Pointer(cm3), unsafe.Pointer(e.compiledModuleOfAddr(addr3)))
		require.Equal(t, unsafe.Pointer(cm3), unsafe.Pointer(e.compiledModuleOfAddr(addr3+1)))
		require.Equal(t, unsafe.Pointer(cm3), unsafe.Pointer(e.compiledModuleOfAddr(addr3+2)))
		require.Equal(t, unsafe.Pointer(cm4), unsafe.Pointer(e.compiledModuleOfAddr(addr4+1)))
		require.Equal(t, unsafe.Pointer(cm4), unsafe.Pointer(e.compiledModuleOfAddr(addr4+10)))
		e.deleteCompiledModuleFromSortedList(cm1)
		require.Equal(t, nil, e.compiledModuleOfAddr(addr1))
		require.Equal(t, nil, e.compiledModuleOfAddr(addr1+1))
		require.Equal(t, nil, e.compiledModuleOfAddr(addr1+99))
		e.deleteCompiledModuleFromSortedList(cm4)
		require.Equal(t, nil, e.compiledModuleOfAddr(addr4))
		require.Equal(t, nil, e.compiledModuleOfAddr(addr4+10))
	})
}

func TestCompiledModule_functionIndexOf(t *testing.T) {
	const executableAddr = 0xaaaa
	var executable []byte
	{
		//nolint:staticcheck
		hdr := (*reflect.SliceHeader)(unsafe.Pointer(&executable))
		hdr.Data = executableAddr
		hdr.Len = 0xffff
		hdr.Cap = 0xffff
	}

	cm := &compiledModule{
		executables:     &executables{executable: executable},
		functionOffsets: []int{0, 500, 1000, 2000},
	}
	require.Equal(t, wasm.Index(0), cm.functionIndexOf(executableAddr))
	require.Equal(t, wasm.Index(0), cm.functionIndexOf(executableAddr+499))
	require.Equal(t, wasm.Index(1), cm.functionIndexOf(executableAddr+500))
	require.Equal(t, wasm.Index(1), cm.functionIndexOf(executableAddr+999))
	require.Equal(t, wasm.Index(2), cm.functionIndexOf(executableAddr+1000))
	require.Equal(t, wasm.Index(2), cm.functionIndexOf(executableAddr+1500))
	require.Equal(t, wasm.Index(2), cm.functionIndexOf(executableAddr+1999))
	require.Equal(t, wasm.Index(3), cm.functionIndexOf(executableAddr+2000))
}

func Test_checkAddrInBytes(t *testing.T) {
	bytes := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	begin := uintptr(unsafe.Pointer(&bytes[0]))
	end := uintptr(unsafe.Pointer(&bytes[len(bytes)-1]))

	require.True(t, checkAddrInBytes(begin, bytes))
	require.True(t, checkAddrInBytes(end, bytes))
	require.False(t, checkAddrInBytes(begin-1, bytes))
	require.False(t, checkAddrInBytes(end+1, bytes))
}
