package wazevo

import (
	"context"
	"fmt"
	"reflect"
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
	getCM := func(addr uintptr) *compiledModule {
		var buf []byte
		{
			// TODO: use unsafe.Slice after floor version is set to Go 1.20.
			hdr := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
			hdr.Data = addr
			hdr.Len = 4
			hdr.Cap = 4
		}
		cm := &compiledModule{executables: &executables{executable: buf}}
		return cm
	}

	requireEqualExisting := func(t *testing.T, e *engine, expected []uintptr) {
		actual := make([]uintptr, 0)
		for _, cm := range e.sortedCompiledModules {
			actual = append(actual, uintptr(unsafe.Pointer(&cm.executable[0])))
		}
		require.Equal(t, expected, actual)
	}

	m1 := getCM(1)
	m100 := getCM(100)
	m5 := getCM(5)
	m10 := getCM(10)

	t.Run("add", func(t *testing.T) {
		e := &engine{}
		e.addCompiledModuleToSortedList(m1)
		e.addCompiledModuleToSortedList(m100)
		e.addCompiledModuleToSortedList(m5)
		e.addCompiledModuleToSortedList(m10)
		requireEqualExisting(t, e, []uintptr{1, 5, 10, 100})
	})
	t.Run("delete", func(t *testing.T) {
		e := &engine{}
		e.addCompiledModuleToSortedList(m1)
		e.addCompiledModuleToSortedList(m100)
		e.addCompiledModuleToSortedList(m5)
		e.addCompiledModuleToSortedList(m10)
		e.deleteCompiledModuleFromSortedList(m100)
		require.Equal(t, 3, len(e.sortedCompiledModules))
		requireEqualExisting(t, e, []uintptr{1, 5, 10})
		e.deleteCompiledModuleFromSortedList(m1)
		requireEqualExisting(t, e, []uintptr{5, 10})
		e.deleteCompiledModuleFromSortedList(m10)
		requireEqualExisting(t, e, []uintptr{5})
		e.deleteCompiledModuleFromSortedList(m5)
		requireEqualExisting(t, e, []uintptr{})
	})

	t.Run("OfAddr", func(t *testing.T) {
		e := &engine{}
		e.addCompiledModuleToSortedList(m1)
		e.addCompiledModuleToSortedList(m100)
		e.addCompiledModuleToSortedList(m5)
		e.addCompiledModuleToSortedList(m10)

		require.Equal(t, nil, e.compiledModuleOfAddr(0))
		require.Equal(t, unsafe.Pointer(m1), unsafe.Pointer(e.compiledModuleOfAddr(1)))
		require.Equal(t, unsafe.Pointer(m1), unsafe.Pointer(e.compiledModuleOfAddr(4)))
		require.Equal(t, unsafe.Pointer(m5), unsafe.Pointer(e.compiledModuleOfAddr(5)))
		require.Equal(t, unsafe.Pointer(m5), unsafe.Pointer(e.compiledModuleOfAddr(8)))
		require.Equal(t, unsafe.Pointer(m10), unsafe.Pointer(e.compiledModuleOfAddr(10)))
		require.Equal(t, unsafe.Pointer(m10), unsafe.Pointer(e.compiledModuleOfAddr(11)))
		require.Equal(t, unsafe.Pointer(m10), unsafe.Pointer(e.compiledModuleOfAddr(12)))
		require.Equal(t, unsafe.Pointer(m100), unsafe.Pointer(e.compiledModuleOfAddr(100)))
		require.Equal(t, unsafe.Pointer(m100), unsafe.Pointer(e.compiledModuleOfAddr(103)))
		e.deleteCompiledModuleFromSortedList(m1)
		require.Equal(t, nil, e.compiledModuleOfAddr(1))
		require.Equal(t, nil, e.compiledModuleOfAddr(2))
		require.Equal(t, nil, e.compiledModuleOfAddr(4))
		e.deleteCompiledModuleFromSortedList(m100)
		require.Equal(t, nil, e.compiledModuleOfAddr(100))
		require.Equal(t, nil, e.compiledModuleOfAddr(103))
	})
}

func TestCompiledModule_functionIndexOf(t *testing.T) {
	const executableAddr = 0xaaaa
	var executable []byte
	{
		// TODO: use unsafe.Slice after floor version is set to Go 1.20.
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
