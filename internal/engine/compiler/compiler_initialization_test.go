package compiler

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileModuleContextInitialization(t *testing.T) {
	tests := []struct {
		name           string
		moduleInstance *wasm.ModuleInstance
		memoryType     wazeroir.MemoryType
	}{
		{
			name: "no nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals:        []*wasm.GlobalInstance{{Val: 100}},
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables: []*wasm.TableInstance{
					{References: make([]wasm.Reference, 20)},
					{References: make([]wasm.Reference, 10)},
				},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    make([][]byte, 10),
				ElementInstances: make([]wasm.ElementInstance, 10),
			},
			memoryType: wazeroir.MemoryTypeStandard,
		},
		{
			name: "element instances nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals:          []*wasm.GlobalInstance{{Val: 100}},
				MemoryInstance:   &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:           []*wasm.TableInstance{{References: make([]wasm.Reference, 20)}},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    make([][]byte, 10),
				ElementInstances: nil,
			},
			memoryType: wazeroir.MemoryTypeStandard,
		},
		{
			name: "data instances nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals:          []*wasm.GlobalInstance{{Val: 100}},
				MemoryInstance:   &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:           []*wasm.TableInstance{{References: make([]wasm.Reference, 20)}},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    nil,
				ElementInstances: make([]wasm.ElementInstance, 10),
			},
			memoryType: wazeroir.MemoryTypeStandard,
		},
		{
			name: "globals nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance:   &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:           []*wasm.TableInstance{{References: make([]wasm.Reference, 20)}},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    make([][]byte, 10),
				ElementInstances: make([]wasm.ElementInstance, 10),
			},
			memoryType: wazeroir.MemoryTypeStandard,
		},
		{
			name: "memory nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals:          []*wasm.GlobalInstance{{Val: 100}},
				Tables:           []*wasm.TableInstance{{References: make([]wasm.Reference, 20)}},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    make([][]byte, 10),
				ElementInstances: make([]wasm.ElementInstance, 10),
			},
		},
		{
			name: "table nil",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance:   &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Tables:           []*wasm.TableInstance{{References: nil}},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    make([][]byte, 10),
				ElementInstances: make([]wasm.ElementInstance, 10),
			},
			memoryType: wazeroir.MemoryTypeStandard,
		},
		{
			name: "table empty",
			moduleInstance: &wasm.ModuleInstance{
				Tables:           []*wasm.TableInstance{{References: make([]wasm.Reference, 20)}},
				TypeIDs:          make([]wasm.FunctionTypeID, 10),
				DataInstances:    make([][]byte, 10),
				ElementInstances: make([]wasm.ElementInstance, 10),
			},
		},
		{
			name: "memory zero length",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 0)},
			},
			memoryType: wazeroir.MemoryTypeStandard,
		},
		{
			name: "memory shared",
			moduleInstance: &wasm.ModuleInstance{
				MemoryInstance: &wasm.MemoryInstance{Buffer: make([]byte, 10), Shared: true},
			},
			memoryType: wazeroir.MemoryTypeShared,
		},
		{
			name:           "all nil except mod engine",
			moduleInstance: &wasm.ModuleInstance{},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			env := newCompilerEnvironment()
			env.moduleInstance = tc.moduleInstance
			ce := env.callEngine()

			ir := &wazeroir.CompilationResult{
				Memory:              tc.memoryType,
				HasTable:            len(tc.moduleInstance.Tables) > 0,
				HasDataInstances:    len(tc.moduleInstance.DataInstances) > 0,
				HasElementInstances: len(tc.moduleInstance.ElementInstances) > 0,
			}
			for _, g := range tc.moduleInstance.Globals {
				ir.Globals = append(ir.Globals, g.Type)
			}
			compiler := env.requireNewCompiler(t, &wasm.FunctionType{}, newCompiler, ir)
			me := &moduleEngine{functions: make([]function, 10)}
			tc.moduleInstance.Engine = me

			err := compiler.compileModuleContextInitialization()
			require.NoError(t, err)
			require.Zero(t, len(compiler.runtimeValueLocationStack().usedRegisters.list()), "expected no usedRegisters")

			compiler.compileExitFromNativeCode(nativeCallStatusCodeReturned)

			code := asm.CodeSegment{}
			defer func() { require.NoError(t, code.Unmap()) }()

			// Generate the code under test.
			_, err = compiler.compile(code.NextCodeSection())
			require.NoError(t, err)

			env.exec(code.Bytes())

			// Check the exit status.
			require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())

			// Check if the fields of callEngine.moduleContext are updated.
			bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Globals))
			require.Equal(t, bufSliceHeader.Data, ce.moduleContext.globalElement0Address)

			if tc.moduleInstance.MemoryInstance != nil {
				bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.MemoryInstance.Buffer))
				require.Equal(t, uint64(bufSliceHeader.Len), ce.moduleContext.memorySliceLen)
				require.Equal(t, bufSliceHeader.Data, ce.moduleContext.memoryElement0Address)
				require.Equal(t, tc.moduleInstance.MemoryInstance, ce.moduleContext.memoryInstance)
			}

			if len(tc.moduleInstance.Tables) > 0 {
				tableHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Tables))
				require.Equal(t, tableHeader.Data, ce.moduleContext.tablesElement0Address)
				require.Equal(t, uintptr(unsafe.Pointer(&tc.moduleInstance.TypeIDs[0])), ce.moduleContext.typeIDsElement0Address)
				require.Equal(t, uintptr(unsafe.Pointer(&tc.moduleInstance.Tables[0])), ce.moduleContext.tablesElement0Address)
			}

			if len(tc.moduleInstance.DataInstances) > 0 {
				dataInstancesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.DataInstances))
				require.Equal(t, dataInstancesHeader.Data, ce.moduleContext.dataInstancesElement0Address)
				require.Equal(t, uintptr(unsafe.Pointer(&tc.moduleInstance.DataInstances[0])), ce.moduleContext.dataInstancesElement0Address)
			}

			if len(tc.moduleInstance.ElementInstances) > 0 {
				elementInstancesHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.ElementInstances))
				require.Equal(t, elementInstancesHeader.Data, ce.moduleContext.elementInstancesElement0Address)
				require.Equal(t, uintptr(unsafe.Pointer(&tc.moduleInstance.ElementInstances[0])), ce.moduleContext.elementInstancesElement0Address)
			}

			require.Equal(t, uintptr(unsafe.Pointer(&me.functions[0])), ce.moduleContext.functionsElement0Address)
		})
	}
}

func TestCompiler_compileMaybeGrowStack(t *testing.T) {
	t.Run("not grow", func(t *testing.T) {
		const stackPointerCeil = 5
		for _, baseOffset := range []uint64{5, 10, 20} {
			t.Run(fmt.Sprintf("%d", baseOffset), func(t *testing.T) {
				env := newCompilerEnvironment()
				compiler := env.requireNewCompiler(t, &wasm.FunctionType{}, newCompiler, nil)

				err := compiler.compilePreamble()
				require.NoError(t, err)

				stackLen := uint64(len(env.stack()))
				stackBasePointer := stackLen - baseOffset // Ceil <= stackLen - stackBasePointer = no need to grow!
				compiler.assignStackPointerCeil(stackPointerCeil)
				env.setStackBasePointer(stackBasePointer)

				compiler.compileExitFromNativeCode(nativeCallStatusCodeReturned)

				code := asm.CodeSegment{}
				defer func() { require.NoError(t, code.Unmap()) }()

				// Generate and run the code under test.
				_, err = compiler.compile(code.NextCodeSection())
				require.NoError(t, err)
				env.exec(code.Bytes())

				// The status code must be "Returned", not "BuiltinFunctionCall".
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			})
		}
	})

	defaultStackLen := uint64(initialStackSize)
	t.Run("grow", func(t *testing.T) {
		tests := []struct {
			name             string
			stackPointerCeil uint64
			stackBasePointer uint64
		}{
			{
				name:             "ceil=6/sbp=len-5",
				stackPointerCeil: 6,
				stackBasePointer: defaultStackLen - 5,
			},
			{
				name:             "ceil=10000/sbp=0",
				stackPointerCeil: 10000,
				stackBasePointer: 0,
			},
		}

		for _, tc := range tests {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				env := newCompilerEnvironment()
				compiler := env.requireNewCompiler(t, &wasm.FunctionType{}, newCompiler, nil)

				err := compiler.compilePreamble()
				require.NoError(t, err)

				// On the return from grow value stack, we simply return.
				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				code := asm.CodeSegment{}
				defer func() { require.NoError(t, code.Unmap()) }()

				// Generate code under test with the given stackPointerCeil.
				compiler.setStackPointerCeil(tc.stackPointerCeil)
				_, err = compiler.compile(code.NextCodeSection())
				require.NoError(t, err)

				// And run the code with the specified stackBasePointer.
				env.setStackBasePointer(tc.stackBasePointer)
				env.exec(code.Bytes())

				// Check if the call exits with builtin function call status.
				require.Equal(t, nativeCallStatusCodeCallBuiltInFunction, env.compilerStatus())

				// Reenter from the return address.
				returnAddress := env.ce.returnAddress
				require.True(t, returnAddress != 0, "returnAddress was zero %d", returnAddress)
				nativecall(returnAddress, env.callEngine(), env.module())

				// Check the result. This should be "Returned".
				require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
			})
		}
	})
}
