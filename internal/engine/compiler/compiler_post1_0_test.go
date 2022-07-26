package compiler

import (
	"fmt"
	"strconv"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileSignExtend(t *testing.T) {
	type fromKind byte
	from8, from16, from32 := fromKind(0), fromKind(1), fromKind(2)

	t.Run("32bit", func(t *testing.T) {
		tests := []struct {
			in       int32
			expected int32
			fromKind fromKind
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L270-L276
			{in: 0, expected: 0, fromKind: from8},
			{in: 0x7f, expected: 127, fromKind: from8},
			{in: 0x80, expected: -128, fromKind: from8},
			{in: 0xff, expected: -1, fromKind: from8},
			{in: 0x012345_00, expected: 0, fromKind: from8},
			{in: -19088768 /* = 0xfedcba_80 bit pattern */, expected: -0x80, fromKind: from8},
			{in: -1, expected: -1, fromKind: from8},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i32.wast#L278-L284
			{in: 0, expected: 0, fromKind: from16},
			{in: 0x7fff, expected: 32767, fromKind: from16},
			{in: 0x8000, expected: -32768, fromKind: from16},
			{in: 0xffff, expected: -1, fromKind: from16},
			{in: 0x0123_0000, expected: 0, fromKind: from16},
			{in: -19103744 /* = 0xfedc_8000 bit pattern */, expected: -0x8000, fromKind: from16},
			{in: -1, expected: -1, fromKind: from16},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newCompilerEnvironment()
				compiler := env.requireNewCompiler(t, newCompiler, nil)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the promote target.
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.in)})
				require.NoError(t, err)

				if tc.fromKind == from8 {
					err = compiler.compileSignExtend32From8()
				} else {
					err = compiler.compileSignExtend32From16()
				}
				require.NoError(t, err)

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate and run the code under test.
				code, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		tests := []struct {
			in       int64
			expected int64
			fromKind fromKind
		}{
			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L271-L277
			{in: 0, expected: 0, fromKind: from8},
			{in: 0x7f, expected: 127, fromKind: from8},
			{in: 0x80, expected: -128, fromKind: from8},
			{in: 0xff, expected: -1, fromKind: from8},
			{in: 0x01234567_89abcd_00, expected: 0, fromKind: from8},
			{in: 81985529216486784 /* = 0xfedcba98_765432_80 bit pattern */, expected: -0x80, fromKind: from8},
			{in: -1, expected: -1, fromKind: from8},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L279-L285
			{in: 0, expected: 0, fromKind: from16},
			{in: 0x7fff, expected: 32767, fromKind: from16},
			{in: 0x8000, expected: -32768, fromKind: from16},
			{in: 0xffff, expected: -1, fromKind: from16},
			{in: 0x12345678_9abc_0000, expected: 0, fromKind: from16},
			{in: 81985529216466944 /* = 0xfedcba98_7654_8000 bit pattern */, expected: -0x8000, fromKind: from16},
			{in: -1, expected: -1, fromKind: from16},

			// https://github.com/WebAssembly/spec/blob/ee4a6c40afa22e3e4c58610ce75186aafc22344e/test/core/i64.wast#L287-L296
			{in: 0, expected: 0, fromKind: from32},
			{in: 0x7fff, expected: 32767, fromKind: from32},
			{in: 0x8000, expected: 32768, fromKind: from32},
			{in: 0xffff, expected: 65535, fromKind: from32},
			{in: 0x7fffffff, expected: 0x7fffffff, fromKind: from32},
			{in: 0x80000000, expected: -0x80000000, fromKind: from32},
			{in: 0xffffffff, expected: -1, fromKind: from32},
			{in: 0x01234567_00000000, expected: 0, fromKind: from32},
			{in: -81985529054232576 /* = 0xfedcba98_80000000 bit pattern */, expected: -0x80000000, fromKind: from32},
			{in: -1, expected: -1, fromKind: from32},
		}

		for _, tt := range tests {
			tc := tt
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newCompilerEnvironment()
				compiler := env.requireNewCompiler(t, newCompiler, nil)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Setup the promote target.
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.in)})
				require.NoError(t, err)

				if tc.fromKind == from8 {
					err = compiler.compileSignExtend64From8()
				} else if tc.fromKind == from16 {
					err = compiler.compileSignExtend64From16()
				} else {
					err = compiler.compileSignExtend64From32()
				}
				require.NoError(t, err)

				// To verify the behavior, we release the value
				// to the stack.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				// Generate and run the code under test.
				code, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt64())
			})
		}
	})
}

func TestCompiler_compileMemoryCopy(t *testing.T) {
	const checkCeil = 100
	tests := []struct {
		sourceOffset, destOffset, size uint32
		requireOutOfBoundsError        bool
	}{
		{sourceOffset: 0, destOffset: 0, size: 0},
		{sourceOffset: 10, destOffset: 5, size: 10},
		{sourceOffset: 10, destOffset: 9, size: 1},
		{sourceOffset: 10, destOffset: 9, size: 2},
		{sourceOffset: 0, destOffset: 10, size: 10},
		{sourceOffset: 0, destOffset: 5, size: 10},
		{sourceOffset: 9, destOffset: 10, size: 10},
		{sourceOffset: 11, destOffset: 13, size: 4},
		{sourceOffset: 0, destOffset: 10, size: 5},
		{sourceOffset: 1, destOffset: 10, size: 5},
		{sourceOffset: 0, destOffset: 10, size: 1},
		{sourceOffset: 0, destOffset: 10, size: 0},
		{sourceOffset: 5, destOffset: 10, size: 10},
		{sourceOffset: 5, destOffset: 10, size: 5},
		{sourceOffset: 5, destOffset: 10, size: 1},
		{sourceOffset: 5, destOffset: 10, size: 0},
		{sourceOffset: 10, destOffset: 0, size: 10},
		{sourceOffset: 1, destOffset: 0, size: 2},
		{sourceOffset: 1, destOffset: 0, size: 20},
		{sourceOffset: 10, destOffset: 0, size: 5},
		{sourceOffset: 10, destOffset: 0, size: 1},
		{sourceOffset: 10, destOffset: 0, size: 0},
		{sourceOffset: 0, destOffset: 50, size: 48},
		{sourceOffset: 0, destOffset: 50, size: 49},
		{sourceOffset: 10, destOffset: 20, size: 72},
		{sourceOffset: 20, destOffset: 10, size: 72},
		{sourceOffset: 19, destOffset: 18, size: 79},
		{sourceOffset: 20, destOffset: 19, size: 79},
		{sourceOffset: defaultMemoryPageNumInTest * wasm.MemoryPageSize, destOffset: 0, size: 1, requireOutOfBoundsError: true},
		{sourceOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize + 1, destOffset: 0, size: 0, requireOutOfBoundsError: true},
		{sourceOffset: 0, destOffset: defaultMemoryPageNumInTest * wasm.MemoryPageSize, size: 1, requireOutOfBoundsError: true},
		{sourceOffset: 0, destOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize + 1, size: 0, requireOutOfBoundsError: true},
		{sourceOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize - 99, destOffset: 0, size: 100, requireOutOfBoundsError: true},
		{sourceOffset: 0, destOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize - 99, size: 100, requireOutOfBoundsError: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Compile operands.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.destOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.sourceOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.size})
			require.NoError(t, err)

			err = compiler.compileMemoryCopy()
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Setup the source memory region.
			mem := env.memory()
			for i := 0; i < checkCeil; i++ {
				mem[i] = byte(i)
			}

			// Run code.
			env.exec(code)

			if !tc.requireOutOfBoundsError {
				exp := make([]byte, checkCeil)
				for i := 0; i < checkCeil; i++ {
					exp[i] = byte(i)
				}
				copy(exp[tc.destOffset:],
					exp[tc.sourceOffset:tc.sourceOffset+tc.size])

				// Check the status code and the destination memory region.
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
				require.Equal(t, exp, mem[:checkCeil])
			} else {
				require.Equal(t, nativeCallStatusCodeMemoryOutOfBounds, env.compilerStatus())
			}
		})
	}
}

func TestCompiler_compileMemoryFill(t *testing.T) {
	const checkCeil = 50

	tests := []struct {
		v, destOffset           uint32
		size                    uint32
		requireOutOfBoundsError bool
	}{
		{v: 0, destOffset: 10, size: 10},
		{v: 0, destOffset: 10, size: 5},
		{v: 0, destOffset: 10, size: 1},
		{v: 0, destOffset: 10, size: 0},
		{v: 5, destOffset: 10, size: 10},
		{v: 5, destOffset: 10, size: 5},
		{v: 5, destOffset: 10, size: 1},
		{v: 5, destOffset: 10, size: 0},
		{v: 10, destOffset: 0, size: 10},
		{v: 10, destOffset: 0, size: 5},
		{v: 10, destOffset: 0, size: 1},
		{v: 10, destOffset: 0, size: 0},
		{v: 10, destOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize - 99, size: 100, requireOutOfBoundsError: true},
		{v: 10, destOffset: defaultMemoryPageNumInTest * wasm.MemoryPageSize, size: 5, requireOutOfBoundsError: true},
		{v: 10, destOffset: defaultMemoryPageNumInTest * wasm.MemoryPageSize, size: 1, requireOutOfBoundsError: true},
		{v: 10, destOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize + 1, size: 0, requireOutOfBoundsError: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{HasMemory: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Compile operands.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.destOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.v})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.size})
			require.NoError(t, err)

			err = compiler.compileMemoryFill()
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Setup the memory region.
			mem := env.memory()
			for i := 0; i < checkCeil; i++ {
				mem[i] = byte(i)
			}

			// Run code.
			env.exec(code)

			if !tc.requireOutOfBoundsError {
				exp := make([]byte, checkCeil)
				for i := 0; i < checkCeil; i++ {
					if i >= int(tc.destOffset) && i < int(tc.destOffset+tc.size) {
						exp[i] = byte(tc.v)
					} else {
						exp[i] = byte(i)
					}
				}

				// Check the status code and the destination memory region.
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
				require.Equal(t, exp, mem[:checkCeil])
			} else {
				require.Equal(t, nativeCallStatusCodeMemoryOutOfBounds, env.compilerStatus())
			}
		})
	}
}

func TestCompiler_compileDataDrop(t *testing.T) {
	origins := [][]byte{
		{1}, {2}, {3}, {4}, {5}, {6}, {7}, {8}, {9}, {10},
	}

	for i := 0; i < len(origins); i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()

			env.module().DataInstances = make([][]byte, len(origins))
			copy(env.module().DataInstances, origins)

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				HasDataInstances: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileDataDrop(&wazeroir.OperationDataDrop{
				DataIndex: uint32(i),
			})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())

			// Check if the target data instance is dropped from the dataInstances slice.
			for j := 0; j < len(origins); j++ {
				if i == j {
					require.Nil(t, env.module().DataInstances[j])
				} else {
					require.NotNil(t, env.module().DataInstances[j])
				}
			}
		})
	}
}

func TestCompiler_compileMemoryInit(t *testing.T) {
	dataInstances := []wasm.DataInstance{
		nil, {1, 2, 3, 4, 5},
	}

	tests := []struct {
		sourceOffset, destOffset uint32
		dataIndex                uint32
		copySize                 uint32
		expOutOfBounds           bool
	}{
		{sourceOffset: 0, destOffset: 0, copySize: 0, dataIndex: 0},
		{sourceOffset: 0, destOffset: 0, copySize: 1, dataIndex: 0, expOutOfBounds: true},
		{sourceOffset: 1, destOffset: 0, copySize: 0, dataIndex: 0, expOutOfBounds: true},
		{sourceOffset: 0, destOffset: 0, copySize: 0, dataIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 5, dataIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 1, dataIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 3, dataIndex: 1},
		{sourceOffset: 0, destOffset: 1, copySize: 3, dataIndex: 1},
		{sourceOffset: 0, destOffset: 7, copySize: 4, dataIndex: 1},
		{sourceOffset: 1, destOffset: 7, copySize: 4, dataIndex: 1},
		{sourceOffset: 4, destOffset: 7, copySize: 1, dataIndex: 1},
		{sourceOffset: 5, destOffset: 7, copySize: 0, dataIndex: 1},
		{sourceOffset: 0, destOffset: 7, copySize: 5, dataIndex: 1},
		{sourceOffset: 1, destOffset: 0, copySize: 3, dataIndex: 1},
		{sourceOffset: 0, destOffset: 1, copySize: 4, dataIndex: 1},
		{sourceOffset: 1, destOffset: 1, copySize: 3, dataIndex: 1},
		{sourceOffset: 0, destOffset: 10, copySize: 5, dataIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 6, dataIndex: 1, expOutOfBounds: true},
		{sourceOffset: 0, destOffset: defaultMemoryPageNumInTest * wasm.MemoryPageSize, copySize: 5, dataIndex: 1, expOutOfBounds: true},
		{sourceOffset: 0, destOffset: defaultMemoryPageNumInTest*wasm.MemoryPageSize - 3, copySize: 5, dataIndex: 1, expOutOfBounds: true},
		{sourceOffset: 6, destOffset: 0, copySize: 0, dataIndex: 1, expOutOfBounds: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()
			env.module().DataInstances = dataInstances

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				HasDataInstances: true, HasMemory: true,
				Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Compile operands.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.destOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.sourceOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.copySize})
			require.NoError(t, err)

			err = compiler.compileMemoryInit(&wazeroir.OperationMemoryInit{
				DataIndex: tc.dataIndex,
			})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			if !tc.expOutOfBounds {
				mem := env.memory()
				exp := make([]byte, defaultMemoryPageNumInTest*wasm.MemoryPageSize)
				if dataInst := dataInstances[tc.dataIndex]; dataInst != nil {
					copy(exp[tc.destOffset:], dataInst[tc.sourceOffset:tc.sourceOffset+tc.copySize])
				}
				require.Equal(t, exp[:20], mem[:20])
			} else {
				require.Equal(t, nativeCallStatusCodeMemoryOutOfBounds, env.compilerStatus())
			}
		})
	}
}

func TestCompiler_compileElemDrop(t *testing.T) {
	origins := []wasm.ElementInstance{
		{References: []wasm.Reference{1}},
		{References: []wasm.Reference{2}},
		{References: []wasm.Reference{3}},
		{References: []wasm.Reference{4}},
		{References: []wasm.Reference{5}},
	}

	for i := 0; i < len(origins); i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()

			insts := make([]wasm.ElementInstance, len(origins))
			copy(insts, origins)
			env.module().ElementInstances = insts

			// Verify assumption that before Drop instruction, all the element instances are not empty.
			for _, inst := range insts {
				require.NotEqual(t, 0, len(inst.References))
			}

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				HasElementInstances: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileElemDrop(&wazeroir.OperationElemDrop{
				ElemIndex: uint32(i),
			})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())

			for j := 0; j < len(insts); j++ {
				if i == j {
					require.Zero(t, len(env.module().ElementInstances[j].References))
				} else {
					require.NotEqual(t, 0, len(env.module().ElementInstances[j].References))
				}
			}
		})
	}
}

func TestCompiler_compileTableCopy(t *testing.T) {
	const tableSize = 100
	tests := []struct {
		sourceOffset, destOffset, size uint32
		requireOutOfBoundsError        bool
	}{
		{sourceOffset: 0, destOffset: 0, size: 0},
		{sourceOffset: 10, destOffset: 5, size: 10},
		{sourceOffset: 10, destOffset: 9, size: 1},
		{sourceOffset: 10, destOffset: 9, size: 2},
		{sourceOffset: 0, destOffset: 10, size: 10},
		{sourceOffset: 0, destOffset: 5, size: 10},
		{sourceOffset: 9, destOffset: 10, size: 10},
		{sourceOffset: 11, destOffset: 13, size: 4},
		{sourceOffset: 0, destOffset: 10, size: 5},
		{sourceOffset: 1, destOffset: 10, size: 5},
		{sourceOffset: 0, destOffset: 10, size: 1},
		{sourceOffset: 0, destOffset: 10, size: 0},
		{sourceOffset: 5, destOffset: 10, size: 10},
		{sourceOffset: 5, destOffset: 10, size: 5},
		{sourceOffset: 5, destOffset: 10, size: 1},
		{sourceOffset: 5, destOffset: 10, size: 0},
		{sourceOffset: 10, destOffset: 0, size: 10},
		{sourceOffset: 1, destOffset: 0, size: 2},
		{sourceOffset: 1, destOffset: 0, size: 20},
		{sourceOffset: 10, destOffset: 0, size: 5},
		{sourceOffset: 10, destOffset: 0, size: 1},
		{sourceOffset: 10, destOffset: 0, size: 0},
		{sourceOffset: tableSize, destOffset: 0, size: 1, requireOutOfBoundsError: true},
		{sourceOffset: tableSize + 1, destOffset: 0, size: 0, requireOutOfBoundsError: true},
		{sourceOffset: 0, destOffset: tableSize, size: 1, requireOutOfBoundsError: true},
		{sourceOffset: 0, destOffset: tableSize + 1, size: 0, requireOutOfBoundsError: true},
		{sourceOffset: tableSize - 99, destOffset: 0, size: 100, requireOutOfBoundsError: true},
		{sourceOffset: 0, destOffset: tableSize - 99, size: 100, requireOutOfBoundsError: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{HasTable: true, Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Compile operands.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.destOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.sourceOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.size})
			require.NoError(t, err)

			err = compiler.compileTableCopy(&wazeroir.OperationTableCopy{})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Setup the table.
			table := make([]wasm.Reference, tableSize)
			env.addTable(&wasm.TableInstance{References: table})
			for i := 0; i < tableSize; i++ {
				table[i] = uintptr(i)
			}

			// Run code.
			env.exec(code)

			if !tc.requireOutOfBoundsError {
				exp := make([]wasm.Reference, tableSize)
				for i := 0; i < tableSize; i++ {
					exp[i] = uintptr(i)
				}
				copy(exp[tc.destOffset:],
					exp[tc.sourceOffset:tc.sourceOffset+tc.size])

				// Check the status code and the destination memory region.
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
				require.Equal(t, exp, table)
			} else {
				require.Equal(t, nativeCallStatusCodeInvalidTableAccess, env.compilerStatus())
			}
		})
	}
}

func TestCompiler_compileTableInit(t *testing.T) {
	elementInstances := []wasm.ElementInstance{
		{}, {References: []wasm.Reference{1, 2, 3, 4, 5}},
	}

	const tableSize = 100
	tests := []struct {
		sourceOffset, destOffset uint32
		elemIndex                uint32
		copySize                 uint32
		expOutOfBounds           bool
	}{
		{sourceOffset: 0, destOffset: 0, copySize: 0, elemIndex: 0},
		{sourceOffset: 0, destOffset: 0, copySize: 1, elemIndex: 0, expOutOfBounds: true},
		{sourceOffset: 1, destOffset: 0, copySize: 0, elemIndex: 0, expOutOfBounds: true},
		{sourceOffset: 0, destOffset: 0, copySize: 0, elemIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 5, elemIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 1, elemIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 3, elemIndex: 1},
		{sourceOffset: 0, destOffset: 1, copySize: 3, elemIndex: 1},
		{sourceOffset: 0, destOffset: 7, copySize: 4, elemIndex: 1},
		{sourceOffset: 1, destOffset: 7, copySize: 4, elemIndex: 1},
		{sourceOffset: 4, destOffset: 7, copySize: 1, elemIndex: 1},
		{sourceOffset: 5, destOffset: 7, copySize: 0, elemIndex: 1},
		{sourceOffset: 0, destOffset: 7, copySize: 5, elemIndex: 1},
		{sourceOffset: 1, destOffset: 0, copySize: 3, elemIndex: 1},
		{sourceOffset: 0, destOffset: 1, copySize: 4, elemIndex: 1},
		{sourceOffset: 1, destOffset: 1, copySize: 3, elemIndex: 1},
		{sourceOffset: 0, destOffset: 10, copySize: 5, elemIndex: 1},
		{sourceOffset: 0, destOffset: 0, copySize: 6, elemIndex: 1, expOutOfBounds: true},
		{sourceOffset: 6, destOffset: 0, copySize: 0, elemIndex: 1, expOutOfBounds: true},
	}

	for i, tt := range tests {
		tc := tt
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			env := newCompilerEnvironment()
			env.module().ElementInstances = elementInstances

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				HasElementInstances: true, HasTable: true,
				Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Compile operands.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.destOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.sourceOffset})
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.copySize})
			require.NoError(t, err)

			err = compiler.compileTableInit(&wazeroir.OperationTableInit{
				ElemIndex: tc.elemIndex,
			})
			require.NoError(t, err)

			// Setup the table.
			table := make([]wasm.Reference, tableSize)
			env.addTable(&wasm.TableInstance{References: table})
			for i := 0; i < tableSize; i++ {
				table[i] = uintptr(i)
			}

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			if !tc.expOutOfBounds {
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
				exp := make([]wasm.Reference, tableSize)
				for i := 0; i < tableSize; i++ {
					exp[i] = uintptr(i)
				}
				if inst := elementInstances[tc.elemIndex]; inst.References != nil {
					copy(exp[tc.destOffset:], inst.References[tc.sourceOffset:tc.sourceOffset+tc.copySize])
				}
				require.Equal(t, exp, table)
			} else {
				require.Equal(t, nativeCallStatusCodeInvalidTableAccess, env.compilerStatus())
			}
		})
	}
}

type dog struct{ name string }

func TestCompiler_compileTableSet(t *testing.T) {
	externDog := &dog{name: "sushi"}
	externrefOpaque := uintptr(unsafe.Pointer(externDog))
	funcref := &function{source: &wasm.FunctionInstance{}}
	funcrefOpaque := uintptr(unsafe.Pointer(funcref))

	externTable := &wasm.TableInstance{Type: wasm.RefTypeExternref, References: []wasm.Reference{0, 0, externrefOpaque, 0, 0}}
	funcrefTable := &wasm.TableInstance{Type: wasm.RefTypeFuncref, References: []wasm.Reference{0, 0, 0, 0, funcrefOpaque}}
	tables := []*wasm.TableInstance{externTable, funcrefTable}

	tests := []struct {
		name       string
		tableIndex uint32
		offset     uint32
		in         uintptr
		expExtern  bool
		expError   bool
	}{
		{
			name:       "externref - non nil",
			tableIndex: 0,
			offset:     2,
			in:         externrefOpaque,
			expExtern:  true,
		},
		{
			name:       "externref - nil",
			tableIndex: 0,
			offset:     1,
			in:         0,
			expExtern:  true,
		},
		{
			name:       "externref - out of bounds",
			tableIndex: 0,
			offset:     10,
			in:         0,
			expError:   true,
		},
		{
			name:       "funcref - non nil",
			tableIndex: 1,
			offset:     4,
			in:         funcrefOpaque,
			expExtern:  false,
		},
		{
			name:       "funcref - nil",
			tableIndex: 1,
			offset:     3,
			in:         0,
			expExtern:  false,
		},
		{
			name:       "funcref - out of bounds",
			tableIndex: 1,
			offset:     100000,
			in:         0,
			expError:   true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			for _, table := range tables {
				env.addTable(table)
			}

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				HasTable:  true,
				Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.offset})
			require.NoError(t, err)

			err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(tc.in)})
			require.NoError(t, err)

			err = compiler.compileTableSet(&wazeroir.OperationTableSet{TableIndex: tc.tableIndex})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			if tc.expError {
				require.Equal(t, nativeCallStatusCodeInvalidTableAccess, env.compilerStatus())
			} else {
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
				require.Equal(t, uint64(0), env.stackPointer())

				if tc.expExtern {
					actual := dogFromPtr(externTable.References[tc.offset])
					exp := externDog
					if tc.in == 0 {
						exp = nil
					}
					require.Equal(t, exp, actual)
				} else {
					actual := functionFromPtr(funcrefTable.References[tc.offset])
					exp := funcref
					if tc.in == 0 {
						exp = nil
					}
					require.Equal(t, exp, actual)
				}
			}
		})
	}
}

//go:nocheckptr ignore "pointer arithmetic result points to invalid allocation"
func dogFromPtr(ptr uintptr) *dog {
	if ptr == 0 {
		return nil
	}
	return (*dog)(unsafe.Pointer(ptr))
}

//go:nocheckptr ignore "pointer arithmetic result points to invalid allocation"
func functionFromPtr(ptr uintptr) *function {
	if ptr == 0 {
		return nil
	}
	return (*function)(unsafe.Pointer(ptr))
}

func TestCompiler_compileTableGet(t *testing.T) {

	externDog := &dog{name: "sushi"}
	externrefOpaque := uintptr(unsafe.Pointer(externDog))
	funcref := &function{source: &wasm.FunctionInstance{}}
	funcrefOpaque := uintptr(unsafe.Pointer(funcref))
	tables := []*wasm.TableInstance{
		{Type: wasm.RefTypeExternref, References: []wasm.Reference{0, 0, externrefOpaque, 0, 0}},
		{Type: wasm.RefTypeFuncref, References: []wasm.Reference{0, 0, 0, 0, funcrefOpaque}},
	}

	tests := []struct {
		name       string
		tableIndex uint32
		offset     uint32
		exp        uintptr
		expError   bool
	}{
		{
			name:       "externref - non nil",
			tableIndex: 0,
			offset:     2,
			exp:        externrefOpaque,
		},
		{
			name:       "externref - nil",
			tableIndex: 0,
			offset:     4,
			exp:        0,
		},
		{
			name:       "externref - out of bounds",
			tableIndex: 0,
			offset:     5,
			expError:   true,
		},
		{
			name:       "funcref - non nil",
			tableIndex: 1,
			offset:     4,
			exp:        funcrefOpaque,
		},
		{
			name:       "funcref - nil",
			tableIndex: 1,
			offset:     1,
			exp:        0,
		},
		{
			name:       "funcref - out of bounds",
			tableIndex: 1,
			offset:     1000,
			expError:   true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()

			for _, table := range tables {
				env.addTable(table)
			}

			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				HasTable:  true,
				Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: tc.offset})
			require.NoError(t, err)

			err = compiler.compileTableGet(&wazeroir.OperationTableGet{TableIndex: tc.tableIndex})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			if tc.expError {
				require.Equal(t, nativeCallStatusCodeInvalidTableAccess, env.compilerStatus())
			} else {
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, uint64(tc.exp), env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compileRefFunc(t *testing.T) {
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{Signature: &wasm.FunctionType{}})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	me := env.moduleEngine()
	const numFuncs = 20
	for i := 0; i < numFuncs; i++ {
		me.functions = append(me.functions, &function{source: &wasm.FunctionInstance{}})
	}

	for i := 0; i < numFuncs; i++ {
		i := i
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{Signature: &wasm.FunctionType{}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileRefFunc(&wazeroir.OperationRefFunc{FunctionIndex: uint32(i)})
			require.NoError(t, err)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			require.Equal(t, uint64(1), env.stackPointer())
			require.Equal(t, uintptr(unsafe.Pointer(me.functions[i])), uintptr(env.stackTopAsUint64()))
		})
	}
}
