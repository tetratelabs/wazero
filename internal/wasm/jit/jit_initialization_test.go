package jit

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func TestCompiler_compileModuleContextInitialization(t *testing.T) {
	for _, tc := range []struct {
		name           string
		moduleInstance *wasm.ModuleInstance
	}{
		{
			name: "no nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{{Val: 100}},
				Memory:  &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Table:   &wasm.TableInstance{Table: make([]interface{}, 20)},
			},
		},
		{
			name: "globals nil",
			moduleInstance: &wasm.ModuleInstance{
				Memory: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Table:  &wasm.TableInstance{Table: make([]interface{}, 20)},
			},
		},
		{
			name: "memory nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{{Val: 100}},
				Table:   &wasm.TableInstance{Table: make([]interface{}, 20)},
			},
		},
		{
			name: "table nil",
			moduleInstance: &wasm.ModuleInstance{
				Memory: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Table:  &wasm.TableInstance{Table: nil},
			},
		},
		{
			name: "table empty",
			moduleInstance: &wasm.ModuleInstance{
				Table: &wasm.TableInstance{Table: make([]interface{}, 0)},
			},
		},
		{
			name: "memory zero length",
			moduleInstance: &wasm.ModuleInstance{
				Memory: &wasm.MemoryInstance{Buffer: make([]byte, 0)},
			},
		},
		{
			name:           "all nil except mod engine",
			moduleInstance: &wasm.ModuleInstance{},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			env.moduleInstance = tc.moduleInstance
			ce := env.callEngine()

			compiler := env.requireNewCompiler(t, newCompiler, nil)
			me := &moduleEngine{compiledFunctions: make([]*compiledFunction, 10)}
			tc.moduleInstance.Engine = me

			// The golang-asm assembler skips the first instruction, so we emit NOP here which is ignored.
			// TODO: delete after #233
			compiler.compileNOP()

			err := compiler.compileModuleContextInitialization()
			require.NoError(t, err)
			require.Empty(t, compiler.valueLocationStack().usedRegisters)

			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			env.exec(code)

			// Check the exit status.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())

			// Check if the fields of callEngine.moduleContext are updated.
			bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Globals))
			require.Equal(t, bufSliceHeader.Data, ce.moduleContext.globalElement0Address)

			if tc.moduleInstance.Memory != nil {
				bufSliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Memory.Buffer))
				require.Equal(t, uint64(bufSliceHeader.Len), ce.moduleContext.memorySliceLen)
				require.Equal(t, bufSliceHeader.Data, ce.moduleContext.memoryElement0Address)
			}

			if tc.moduleInstance.Table != nil {
				tableHeader := (*reflect.SliceHeader)(unsafe.Pointer(&tc.moduleInstance.Table.Table))
				require.Equal(t, uint64(tableHeader.Len), ce.moduleContext.tableSliceLen)
				require.Equal(t, tableHeader.Data, ce.moduleContext.tableElement0Address)
			}

			require.Equal(t, uintptr(unsafe.Pointer(&me.compiledFunctions[0])), ce.moduleContext.compiledFunctionsElement0Address)
		})
	}
}

func TestCompiler_compileMaybeGrowValueStack(t *testing.T) {
	t.Run("not grow", func(t *testing.T) {
		const stackPointerCeil = 5
		for _, baseOffset := range []uint64{5, 10, 20} {
			t.Run(fmt.Sprintf("%d", baseOffset), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t, newCompiler, nil)

				// The golang-asm assembler skips the first instruction, so we emit NOP here which is ignored.
				// TODO: delete after #233
				compiler.compileNOP()

				err := compiler.compileMaybeGrowValueStack()
				require.NoError(t, err)
				require.NotNil(t, compiler.getOnStackPointerCeilDeterminedCallBack())

				valueStackLen := uint64(len(env.stack()))
				stackBasePointer := valueStackLen - baseOffset // Ceil <= valueStackLen - stackBasePointer = no need to grow!
				compiler.getOnStackPointerCeilDeterminedCallBack()(stackPointerCeil)
				env.setValueStackBasePointer(stackBasePointer)

				compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

				// Generate and run the code under test.
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				// The status code must be "Returned", not "BuiltinFunctionCall".
				require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			})
		}
	})
	t.Run("grow", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, newCompiler, nil)

		// The golang-asm assembler skips the first instruction, so we emit NOP here which is ignored.
		// TODO: delete after #233
		compiler.compileNOP()

		err := compiler.compileMaybeGrowValueStack()
		require.NoError(t, err)

		// On the return from grow value stack, we simply return.
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		stackPointerCeil := uint64(6)
		compiler.setStackPointerCeil(stackPointerCeil)
		valueStackLen := uint64(len(env.stack()))
		stackBasePointer := valueStackLen - 5 // Ceil > valueStackLen - stackBasePointer = need to grow!
		env.setValueStackBasePointer(stackBasePointer)

		// Generate and run the code under test.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		// Check if the call exits with builtin function call status.
		require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())

		// Reenter from the return address.
		returnAddress := env.callFrameStackPeek().returnAddress
		require.NotZero(t, returnAddress)
		jitcall(
			returnAddress, uintptr(unsafe.Pointer(env.callEngine())),
			uintptr(unsafe.Pointer(env.module())),
		)

		// Check the result. This should be "Returned".
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}
