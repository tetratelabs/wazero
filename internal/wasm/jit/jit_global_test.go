package jit

import (
	"testing"

	"github.com/heeus/hwazero/internal/testing/require"
	"github.com/heeus/hwazero/internal/wasm"
	"github.com/heeus/hwazero/internal/wazeroir"
)

func TestCompiler_compileGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for _, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(wasm.ValueTypeName(tp), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)

			// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
			globals := []*wasm.GlobalInstance{nil, {Val: globalValue, Type: &wasm.GlobalType{ValType: tp}}}
			env.addGlobals(globals...)

			// Emit the code.
			err := compiler.compilePreamble()
			require.NoError(t, err)
			op := &wazeroir.OperationGlobalGet{Index: 1}
			err = compiler.compileGlobalGet(op)
			require.NoError(t, err)

			// At this point, the top of stack must be the retrieved global on a register.
			global := compiler.valueLocationStack().peek()
			require.True(t, global.onRegister())
			require.Equal(t, 1, len(compiler.valueLocationStack().usedRegisters))
			switch tp {
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				require.True(t, isFloatRegister(global.register))
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				require.True(t, isIntRegister(global.register))
			}
			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run the code assembled above.
			env.exec(code)

			// Since we call global.get, the top of the stack must be the global value.
			require.Equal(t, globalValue, env.stack()[0])
			// Plus as we push the value, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
		})
	}
}

func TestCompiler_compileGlobalSet(t *testing.T) {
	const valueToSet uint64 = 12345
	for _, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64,
		wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(wasm.ValueTypeName(tp), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, nil)

			// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
			env.addGlobals(nil, &wasm.GlobalInstance{Val: 40, Type: &wasm.GlobalType{ValType: tp}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Place the set target value.
			loc := compiler.valueLocationStack().pushValueLocationOnStack()
			switch tp {
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				loc.setRegisterType(generalPurposeRegisterTypeInt)
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				loc.setRegisterType(generalPurposeRegisterTypeFloat)
			}
			env.stack()[loc.stackPointer] = valueToSet

			op := &wazeroir.OperationGlobalSet{Index: 1}
			err = compiler.compileGlobalSet(op)
			require.Equal(t, uint64(0), compiler.valueLocationStack().sp)
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// The global value should be set to valueToSet.
			require.Equal(t, valueToSet, env.getGlobal(op.Index))
			// Plus we consumed the top of the stack, the stack pointer must be decremented.
			require.Equal(t, uint64(0), env.stackPointer())
		})
	}
}
