package compiler

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for _, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64, wasm.ValueTypeExternref, wasm.ValueTypeFuncref,
	} {
		tp := tp
		t.Run(wasm.ValueTypeName(tp), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				Signature: &wasm.FunctionType{},
				Globals:   []wasm.GlobalType{{}, {ValType: tp}},
			})

			// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
			globals := []*wasm.GlobalInstance{nil, {Val: globalValue, Type: wasm.GlobalType{ValType: tp}}}
			env.addGlobals(globals...)

			// Emit the code.
			err := compiler.compilePreamble()
			require.NoError(t, err)
			// op := wazeroir.OperationGlobalGet{Index: 1}
			op := wazeroir.NewOperationGlobalGet(1)
			err = compiler.compileGlobalGet(op)
			require.NoError(t, err)

			// At this point, the top of stack must be the retrieved global on a register.
			global := compiler.runtimeValueLocationStack().peek()
			require.True(t, global.onRegister())
			require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
			switch tp {
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				require.True(t, isVectorRegister(global.register))
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				require.True(t, isGeneralPurposeRegister(global.register))
			}
			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)

			// Run the code assembled above.
			env.exec(code)

			// Since we call global.get, the top of the stack must be the global value.
			require.Equal(t, globalValue, env.stackTopAsUint64())
			// Plus as we push the value, the stack pointer must be incremented.
			require.Equal(t, uint64(1), env.stackPointer())
		})
	}
}

func TestCompiler_compileGlobalGet_v128(t *testing.T) {
	const v128Type = wasm.ValueTypeV128
	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Globals:   []wasm.GlobalType{{}, {ValType: v128Type}},
	})

	// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
	globals := []*wasm.GlobalInstance{nil, {Val: 12345, ValHi: 6789, Type: wasm.GlobalType{ValType: v128Type}}}
	env.addGlobals(globals...)

	// Emit the code.
	err := compiler.compilePreamble()
	require.NoError(t, err)
	// op := wazeroir.OperationGlobalGet{Index: 1}
	op := wazeroir.NewOperationGlobalGet(1)
	err = compiler.compileGlobalGet(op)
	require.NoError(t, err)

	// At this point, the top of stack must be the retrieved global on a register.
	global := compiler.runtimeValueLocationStack().peek()
	require.True(t, global.onRegister())
	require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))
	require.True(t, isVectorRegister(global.register))
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)

	// Run the code assembled above.
	env.exec(code)

	require.Equal(t, uint64(2), env.stackPointer())
	require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

	// Since we call global.get, the top of the stack must be the global value.
	actual := globals[1]
	sp := env.ce.stackContext.stackPointer
	stack := env.stack()
	require.Equal(t, actual.Val, stack[sp-2])
	require.Equal(t, actual.ValHi, stack[sp-1])
}

func TestCompiler_compileGlobalSet(t *testing.T) {
	const valueToSet uint64 = 12345
	for _, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64,
		wasm.ValueTypeI32, wasm.ValueTypeI64,
		wasm.ValueTypeExternref, wasm.ValueTypeFuncref,
	} {
		tp := tp
		t.Run(wasm.ValueTypeName(tp), func(t *testing.T) {
			env := newCompilerEnvironment()
			compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
				Signature: &wasm.FunctionType{},
				Globals:   []wasm.GlobalType{{}, {ValType: tp}},
			})

			// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
			env.addGlobals(nil, &wasm.GlobalInstance{Val: 40, Type: wasm.GlobalType{ValType: tp}})

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Place the set target value.
			loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
			switch tp {
			case wasm.ValueTypeI32:
				loc.valueType = runtimeValueTypeI32
			case wasm.ValueTypeI64, wasm.ValueTypeExternref, wasm.ValueTypeFuncref:
				loc.valueType = runtimeValueTypeI64
			case wasm.ValueTypeF32:
				loc.valueType = runtimeValueTypeF32
			case wasm.ValueTypeF64:
				loc.valueType = runtimeValueTypeF64
			}
			env.stack()[loc.stackPointer] = valueToSet

			index := uint32(1)
			op := wazeroir.NewOperationGlobalSet(index)
			err = compiler.compileGlobalSet(op)
			requireRuntimeLocationStackPointerEqual(t, 0, compiler)

			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate the code under test.
			code, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// The global value should be set to valueToSet.
			actual := env.globals()[index]
			require.Equal(t, valueToSet, actual.Val)
			// Plus we consumed the top of the stack, the stack pointer must be decremented.
			require.Equal(t, uint64(0), env.stackPointer())
		})
	}
}

func TestCompiler_compileGlobalSet_v128(t *testing.T) {
	const v128Type = wasm.ValueTypeV128
	const valueToSetLo, valueToSetHi uint64 = 0xffffff, 1

	env := newCompilerEnvironment()
	compiler := env.requireNewCompiler(t, newCompiler, &wazeroir.CompilationResult{
		Signature: &wasm.FunctionType{},
		Globals:   []wasm.GlobalType{{}, {ValType: v128Type}},
	})

	// Setup the global. (Start with nil as a dummy so that global index can be non-trivial.)
	env.addGlobals(nil, &wasm.GlobalInstance{Val: 0, ValHi: 0, Type: wasm.GlobalType{ValType: v128Type}})

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Place the set target value.
	lo := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
	lo.valueType = runtimeValueTypeV128Lo
	env.stack()[lo.stackPointer] = valueToSetLo
	hi := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
	hi.valueType = runtimeValueTypeV128Hi
	env.stack()[hi.stackPointer] = valueToSetHi

	index := uint32(1)
	op := wazeroir.NewOperationGlobalSet(index)
	err = compiler.compileGlobalSet(op)
	requireRuntimeLocationStackPointerEqual(t, 0, compiler)
	require.NoError(t, err)

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate the code under test.
	code, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, uint64(0), env.stackPointer())
	require.Equal(t, nativeCallStatusCodeReturned, env.callEngine().statusCode)

	// The global value should be set to valueToSet.
	actual := env.globals()[index]
	require.Equal(t, valueToSetLo, actual.Val)
	require.Equal(t, valueToSetHi, actual.ValHi)
}
