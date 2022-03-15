//go:build amd64 || arm64

package jit

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"reflect"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/internal/moremath"
	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

type jitEnv struct {
	me             *moduleEngine
	ce             *callEngine
	moduleInstance *wasm.ModuleInstance
}

func (j *jitEnv) stackTopAsUint32() uint32 {
	return uint32(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) stackTopAsInt32() int32 {
	return int32(j.stack()[j.stackPointer()-1])
}
func (j *jitEnv) stackTopAsUint64() uint64 {
	return j.stack()[j.stackPointer()-1]
}

func (j *jitEnv) stackTopAsInt64() int64 {
	return int64(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) stackTopAsFloat32() float32 {
	return math.Float32frombits(uint32(j.stack()[j.stackPointer()-1]))
}

func (j *jitEnv) stackTopAsFloat64() float64 {
	return math.Float64frombits(j.stack()[j.stackPointer()-1])
}

func (j *jitEnv) memory() []byte {
	return j.moduleInstance.Memory.Buffer
}

func (j *jitEnv) stack() []uint64 {
	return j.ce.valueStack
}

func (j *jitEnv) jitStatus() jitCallStatusCode {
	return j.ce.exitContext.statusCode
}

func (j *jitEnv) builtinFunctionCallAddress() wasm.Index {
	return j.ce.exitContext.builtinFunctionCallIndex
}

func (j *jitEnv) stackPointer() uint64 {
	return j.ce.valueStackContext.stackPointer
}

func (j *jitEnv) stackBasePointer() uint64 {
	return j.ce.valueStackContext.stackBasePointer
}

func (j *jitEnv) setStackPointer(sp uint64) {
	j.ce.valueStackContext.stackPointer = sp
}

func (j *jitEnv) addGlobals(g ...*wasm.GlobalInstance) {
	j.moduleInstance.Globals = append(j.moduleInstance.Globals, g...)
}

func (j *jitEnv) getGlobal(index uint32) uint64 {
	return j.moduleInstance.Globals[index].Val
}

func (j *jitEnv) setTable(table []uintptr) {
	j.moduleInstance.Table = &wasm.TableInstance{Table: table}
}

func (j *jitEnv) callFrameStackPeek() *callFrame {
	return &j.ce.callFrameStack[j.ce.globalContext.callFrameStackPointer-1]
}

func (j *jitEnv) callFrameStackPointer() uint64 {
	return j.ce.globalContext.callFrameStackPointer
}

func (j *jitEnv) setValueStackBasePointer(sp uint64) {
	j.ce.valueStackContext.stackBasePointer = sp
}

func (j *jitEnv) setCallFrameStackPointerLen(l uint64) {
	j.ce.callFrameStackLen = l
}

func (j *jitEnv) module() *wasm.ModuleInstance {
	return j.moduleInstance
}

func (j *jitEnv) moduleEngine() *moduleEngine {
	return j.me
}

func (j *jitEnv) callEngine() *callEngine {
	return j.ce
}

func (j *jitEnv) exec(code []byte) {
	compiledFunction := &compiledFunction{
		codeSegment:        code,
		codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
		source: &wasm.FunctionInstance{
			Kind:   wasm.FunctionKindWasm,
			Type:   &wasm.FunctionType{},
			Module: j.moduleInstance,
		},
	}

	j.ce.pushCallFrame(compiledFunction)

	jitcall(
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(unsafe.Pointer(j.ce)),
	)
}

func (j *jitEnv) requireNewCompiler(t *testing.T, functype *wasm.FunctionType) compilerImpl {
	requireSupportedOSArch(t)
	c, release, err := newCompiler(
		&wasm.FunctionInstance{Module: j.moduleInstance, Kind: wasm.FunctionKindWasm, Type: functype},
		&wazeroir.CompilationResult{LabelCallers: map[string]uint32{}},
	)
	t.Cleanup(release)
	require.NoError(t, err)

	ret, ok := c.(compilerImpl)
	require.True(t, ok)
	return ret
}

// CompilerImpl is the interface used for architecture-independent unit tests in this file.
// This is currently implemented by amd64 and arm64.
type compilerImpl interface {
	compiler
	compileExitFromNativeCode(jitCallStatusCode)
	compileMaybeGrowValueStack() error
	compileReturnFunction() error
	getOnStackPointerCeilDeterminedCallBack() func(uint64)
	setStackPointerCeil(uint64)
	compileReleaseRegisterToStack(loc *valueLocation)
	valueLocationStack() *valueLocationStack
	setValueLocationStack(*valueLocationStack)
	compileEnsureOnGeneralPurposeRegister(loc *valueLocation) error
	compileModuleContextInitialization() error
}

const defaultMemoryPageNumInTest = 1

func newJITEnvironment() *jitEnv {
	me := &moduleEngine{}
	return &jitEnv{
		me: me,
		moduleInstance: &wasm.ModuleInstance{
			Memory:  &wasm.MemoryInstance{Buffer: make([]byte, wasm.MemoryPageSize*defaultMemoryPageNumInTest)},
			Table:   &wasm.TableInstance{},
			Globals: []*wasm.GlobalInstance{},
			Engine:  me,
		},
		ce: me.newCallEngine(),
	}
}

func TestArm64Compiler_compileLabel(t *testing.T) {
	label := &wazeroir.Label{FrameID: 100, Kind: wazeroir.LabelKindContinuation}
	for _, expectSkip := range []bool{false, true} {
		expectSkip := expectSkip
		t.Run(fmt.Sprintf("expect skip=%v", expectSkip), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)

			if expectSkip {
				// If the initial stack is not set, compileLabel must return skip=true.
				actual := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
				require.True(t, actual)
			} else {
				err := compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: label}})
				require.NoError(t, err)
				actual := compiler.compileLabel(&wazeroir.OperationLabel{Label: label})
				require.False(t, actual)
			}
		})
	}
}

func TestCompiler_compileMaybeGrowValueStack(t *testing.T) {
	t.Run("not grow", func(t *testing.T) {
		const stackPointerCeil = 5
		for _, baseOffset := range []uint64{5, 10, 20} {
			t.Run(fmt.Sprintf("%d", baseOffset), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t, nil)

				// The assembler skips the first instruction so we intentionally add const op here, which is ignored.
				// TODO: delete after #233
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 1})
				require.NoError(t, err)
				compiler.valueLocationStack().pop()

				err = compiler.compileMaybeGrowValueStack()
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
		compiler := env.requireNewCompiler(t, nil)

		// The assembler skips the first instruction so we intentionally add const op here, which is ignored.
		// TODO: delete after #233
		err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 1})
		require.NoError(t, err)
		compiler.valueLocationStack().pop()

		err = compiler.compileMaybeGrowValueStack()
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
		jitcall(returnAddress, uintptr(unsafe.Pointer(env.callEngine())))

		// Check the result. This should be "Returned".
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}

func TestCompiler_returnFunction(t *testing.T) {
	t.Run("exit", func(t *testing.T) {
		env := newJITEnvironment()

		// Build code.
		compiler := env.requireNewCompiler(t, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)
		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)

		// JIT status must be returned.
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		// Plus, the call frame stack pointer must be zero after return.
		require.Equal(t, uint64(0), env.callFrameStackPointer())
	})
	t.Run("deep call stack", func(t *testing.T) {
		env := newJITEnvironment()
		moduleEngine := env.moduleEngine()
		ce := env.callEngine()

		// Push the call frames.
		const callFrameNums = 10
		stackPointerToExpectedValue := map[uint64]uint32{}
		for funcIndex := wasm.Index(0); funcIndex < callFrameNums; funcIndex++ {
			// We have to do compilation in a separate subtest since each compilation takes
			// the mutex lock and must release on the cleanup of each subtest.
			// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
			t.Run(fmt.Sprintf("compiling existing callframe %d", funcIndex), func(t *testing.T) {
				// Each function pushes its funcaddr and soon returns.
				compiler := env.requireNewCompiler(t, nil)
				err := compiler.compilePreamble()
				require.NoError(t, err)

				// Push its functionIndex.
				expValue := uint32(funcIndex)
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
				require.NoError(t, err)

				err = compiler.compileReturnFunction()
				require.NoError(t, err)

				code, _, _, err := compiler.compile()
				require.NoError(t, err)

				// Compiles and adds to the engine.
				compiledFunction := &compiledFunction{codeSegment: code, codeInitialAddress: uintptr(unsafe.Pointer(&code[0]))}
				moduleEngine.compiledFunctions = append(moduleEngine.compiledFunctions, compiledFunction)

				// Pushes the frame whose return address equals the beginning of the function just compiled.
				frame := callFrame{
					// Set the return address to the beginning of the function so that we can execute the constI32 above.
					returnAddress: compiledFunction.codeInitialAddress,
					// Note: return stack base pointer is set to funcaddr*5 and this is where the const should be pushed.
					returnStackBasePointer: uint64(funcIndex) * 5,
					compiledFunction:       compiledFunction,
				}
				ce.callFrameStack[ce.globalContext.callFrameStackPointer] = frame
				ce.globalContext.callFrameStackPointer++
				stackPointerToExpectedValue[frame.returnStackBasePointer] = expValue
			})
		}

		require.Equal(t, uint64(callFrameNums), env.callFrameStackPointer())

		// Run code from the top frame.
		env.exec(ce.callFrameTop().compiledFunction.codeSegment)

		// Check the exit status and the values on stack.
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		for pos, exp := range stackPointerToExpectedValue {
			require.Equal(t, exp, uint32(env.stack()[pos]))
		}
	})
}

func TestCompiler_compileConsts(t *testing.T) {
	for _, op := range []wazeroir.OperationKind{
		wazeroir.OperationKindConstI32,
		wazeroir.OperationKindConstI64,
		wazeroir.OperationKindConstF32,
		wazeroir.OperationKindConstF64,
	} {
		op := op
		t.Run(op.String(), func(t *testing.T) {
			for _, val := range []uint64{
				0x0, 0x1, 0x1111000, 1 << 16, 1 << 21, 1 << 27, 1 << 32, 1<<32 + 1, 1 << 53,
				math.Float64bits(math.Inf(1)),
				math.Float64bits(math.Inf(-1)),
				math.Float64bits(math.NaN()),
				math.MaxUint32,
				math.MaxInt32,
				math.MaxUint64,
				math.MaxInt64,
				uint64(math.Float32bits(float32(math.Inf(1)))),
				uint64(math.Float32bits(float32(math.Inf(-1)))),
				uint64(math.Float32bits(float32(math.NaN()))),
			} {
				t.Run(fmt.Sprintf("0x%x", val), func(t *testing.T) {
					env := newJITEnvironment()

					// Build code.
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					switch op {
					case wazeroir.OperationKindConstI32:
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(val)})
					case wazeroir.OperationKindConstI64:
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
					case wazeroir.OperationKindConstF32:
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(val))})
					case wazeroir.OperationKindConstF64:
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
					}
					require.NoError(t, err)

					// After compiling const operations, we must see the register allocated value on the top of value.
					loc := compiler.valueLocationStack().peek()
					require.True(t, loc.onRegister())

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					// Run native code.
					env.exec(code)

					// JIT status must be returned.
					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
					require.Equal(t, uint64(1), env.stackPointer())

					switch op {
					case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstF32:
						require.Equal(t, uint32(val), env.stackTopAsUint32())
					case wazeroir.OperationKindConstI64, wazeroir.OperationKindConstF64:
						require.Equal(t, val, env.stackTopAsUint64())
					}
				})
			}
		})
	}
}

func TestCompiler_compile_Add_Sub_Mul(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindAdd,
		wazeroir.OperationKindSub,
		wazeroir.OperationKindMul,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, unsignedType := range []wazeroir.UnsignedType{
				wazeroir.UnsignedTypeI32,
				wazeroir.UnsignedTypeI64,
				wazeroir.UnsignedTypeF32,
				wazeroir.UnsignedTypeF64,
			} {
				unsignedType := unsignedType
				t.Run(unsignedType.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {1, 1}, {2, 1}, {100, 1}, {1, 0}, {0, 1}, {math.MaxInt16, math.MaxInt32},
						{1 << 14, 1 << 21}, {1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0}, {0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff}, {1, 0xffff_ffff_ffff_ffff},
						{0, math.Float64bits(math.Inf(1))},
						{0, math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), 1},
						{math.Float64bits(math.Inf(-1)), 1},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(1))},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(-1))},
					} {
						x1, x2 := values[0], values[1]
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.UnsignedTypeI64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								case wazeroir.UnsignedTypeF32:
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.UnsignedTypeF64:
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist.
							require.Equal(t, uint64(2), compiler.valueLocationStack().sp)

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindAdd:
								err = compiler.compileAdd(&wazeroir.OperationAdd{Type: unsignedType})
							case wazeroir.OperationKindSub:
								err = compiler.compileSub(&wazeroir.OperationSub{Type: unsignedType})
							case wazeroir.OperationKindMul:
								err = compiler.compileMul(&wazeroir.OperationMul{Type: unsignedType})
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
							resultLocation := compiler.valueLocationStack().peek()
							// Plus the result must be located on a register.
							require.True(t, resultLocation.onRegister())
							// Also, the result must have an appropriate register type.
							if unsignedType == wazeroir.UnsignedTypeF32 || unsignedType == wazeroir.UnsignedTypeF64 {
								require.Equal(t, generalPurposeRegisterTypeFloat, resultLocation.regType)
							} else {
								require.Equal(t, generalPurposeRegisterTypeInt, resultLocation.regType)
							}

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Check the stack.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindAdd:
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									require.Equal(t, uint32(x1)+uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedTypeI64:
									require.Equal(t, x1+x2, env.stackTopAsUint64())
								case wazeroir.UnsignedTypeF32:
									exp := math.Float32frombits(uint32(x1)) + math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) + math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							case wazeroir.OperationKindSub:
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									require.Equal(t, uint32(x1)-uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedTypeI64:
									require.Equal(t, x1-x2, env.stackTopAsUint64())
								case wazeroir.UnsignedTypeF32:
									exp := math.Float32frombits(uint32(x1)) - math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) - math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							case wazeroir.OperationKindMul:
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									require.Equal(t, uint32(x1)*uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedTypeI64:
									require.Equal(t, x1*x2, env.stackTopAsUint64())
								case wazeroir.UnsignedTypeF32:
									exp := math.Float32frombits(uint32(x1)) * math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.UnsignedTypeF64:
									exp := math.Float64frombits(x1) * math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestCompiler_compile_And_Or_Xor_Shl_Rotl_Rotr(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindAnd,
		wazeroir.OperationKindOr,
		wazeroir.OperationKindXor,
		wazeroir.OperationKindShl,
		wazeroir.OperationKindRotl,
		wazeroir.OperationKindRotr,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, unsignedInt := range []wazeroir.UnsignedInt{
				wazeroir.UnsignedInt32,
				wazeroir.UnsignedInt64,
			} {
				unsignedInt := unsignedInt
				t.Run(unsignedInt.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {0, 1}, {1, 0}, {1, 1},
						{1 << 31, 1}, {1, 1 << 31}, {1 << 31, 1 << 31},
						{1 << 63, 1}, {1, 1 << 63}, {1 << 63, 1 << 63},
					} {
						x1, x2 := values[0], values[1]
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.UnsignedInt64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist.
							require.Equal(t, uint64(2), compiler.valueLocationStack().sp)

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindAnd:
								err = compiler.compileAnd(&wazeroir.OperationAnd{Type: unsignedInt})
							case wazeroir.OperationKindOr:
								err = compiler.compileOr(&wazeroir.OperationOr{Type: unsignedInt})
							case wazeroir.OperationKindXor:
								err = compiler.compileXor(&wazeroir.OperationXor{Type: unsignedInt})
							case wazeroir.OperationKindShl:
								err = compiler.compileShl(&wazeroir.OperationShl{Type: unsignedInt})
							case wazeroir.OperationKindRotl:
								err = compiler.compileRotl(&wazeroir.OperationRotl{Type: unsignedInt})
							case wazeroir.OperationKindRotr:
								err = compiler.compileRotr(&wazeroir.OperationRotr{Type: unsignedInt})
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
							resultLocation := compiler.valueLocationStack().peek()
							// Plus the result must be located on a register.
							require.True(t, resultLocation.onRegister())
							// Also, the result must have an appropriate register type.
							require.Equal(t, generalPurposeRegisterTypeInt, resultLocation.regType)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Check the stack.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindAnd:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)&uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1&x2, env.stackTopAsUint64())
								}
							case wazeroir.OperationKindOr:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)|uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1|x2, env.stackTopAsUint64())
								}
							case wazeroir.OperationKindXor:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)^uint32(x2), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1^x2, env.stackTopAsUint64())
								}
							case wazeroir.OperationKindShl:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, uint32(x1)<<uint32(x2%32), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, x1<<(x2%64), env.stackTopAsUint64())
								}
							case wazeroir.OperationKindRotl:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, bits.RotateLeft32(uint32(x1), int(x2)), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, bits.RotateLeft64(x1, int(x2)), env.stackTopAsUint64())
								}
							case wazeroir.OperationKindRotr:
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									require.Equal(t, bits.RotateLeft32(uint32(x1), -int(x2)), env.stackTopAsUint32())
								case wazeroir.UnsignedInt64:
									require.Equal(t, bits.RotateLeft64(x1, -int(x2)), env.stackTopAsUint64())
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestCompiler_compileShr(t *testing.T) {
	kind := wazeroir.OperationKindShr
	t.Run(kind.String(), func(t *testing.T) {
		for _, signedInt := range []wazeroir.SignedInt{
			wazeroir.SignedInt32,
			wazeroir.SignedInt64,
			wazeroir.SignedUint32,
			wazeroir.SignedUint64,
		} {
			signedInt := signedInt
			t.Run(signedInt.String(), func(t *testing.T) {
				for _, values := range [][2]uint64{
					{0, 0}, {0, 1}, {1, 0}, {1, 1},
					{1 << 31, 1}, {1, 1 << 31}, {1 << 31, 1 << 31},
					{1 << 63, 1}, {1, 1 << 63}, {1 << 63, 1 << 63},
				} {
					x1, x2 := values[0], values[1]
					t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
						env := newJITEnvironment()
						compiler := env.requireNewCompiler(t, nil)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Emit consts operands.
						for _, v := range []uint64{x1, x2} {
							switch signedInt {
							case wazeroir.SignedInt32:
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(int32(v))})
							case wazeroir.SignedInt64:
								err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
							case wazeroir.SignedUint32:
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
							case wazeroir.SignedUint64:
								err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
							}
							require.NoError(t, err)
						}

						// At this point, two values exist.
						require.Equal(t, uint64(2), compiler.valueLocationStack().sp)

						// Emit the operation.
						err = compiler.compileShr(&wazeroir.OperationShr{Type: signedInt})
						require.NoError(t, err)

						// We consumed two values, but push the result back.
						require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
						resultLocation := compiler.valueLocationStack().peek()
						// Plus the result must be located on a register.
						require.True(t, resultLocation.onRegister())
						// Also, the result must have an appropriate register type.
						require.Equal(t, generalPurposeRegisterTypeInt, resultLocation.regType)

						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						// Compile and execute the code under test.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)
						env.exec(code)

						// Check the stack.
						require.Equal(t, uint64(1), env.stackPointer())

						switch signedInt {
						case wazeroir.SignedInt32:
							require.Equal(t, int32(x1)>>(uint32(x2)%32), env.stackTopAsInt32())
						case wazeroir.SignedInt64:
							require.Equal(t, int64(x1)>>(x2%64), env.stackTopAsInt64())
						case wazeroir.SignedUint32:
							require.Equal(t, uint32(x1)>>(uint32(x2)%32), env.stackTopAsUint32())
						case wazeroir.SignedUint64:
							require.Equal(t, x1>>(x2%64), env.stackTopAsUint64())
						}
					})
				}
			})
		}
	})
}

func TestCompiler_compile_Le_Lt_Gt_Ge_Eq_Eqz_Ne(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindEq,
		wazeroir.OperationKindEqz,
		wazeroir.OperationKindNe,
		wazeroir.OperationKindLe,
		wazeroir.OperationKindLt,
		wazeroir.OperationKindGe,
		wazeroir.OperationKindGt,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, signedType := range []wazeroir.SignedType{
				wazeroir.SignedTypeUint32,
				wazeroir.SignedTypeUint64,
				wazeroir.SignedTypeInt32,
				wazeroir.SignedTypeInt64,
				wazeroir.SignedTypeFloat32,
				wazeroir.SignedTypeFloat64,
			} {
				signedType := signedType
				t.Run(signedType.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {1, 1}, {2, 1}, {100, 1}, {1, 0}, {0, 1}, {math.MaxInt16, math.MaxInt32},
						{1 << 14, 1 << 21}, {1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0}, {0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff}, {1, 0xffff_ffff_ffff_ffff},
						{1, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 1},
						{0xffff_ffff_ffff_ffff, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 0xffff_ffff_ffff_ffff},
						{math.Float64bits(math.MaxFloat32), 1},
						{math.Float64bits(math.SmallestNonzeroFloat32), 1},
						{math.Float64bits(math.MaxFloat64), 1},
						{math.Float64bits(math.SmallestNonzeroFloat64), 1},
						{0, math.Float64bits(math.Inf(1))},
						{0, math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), 0},
						{math.Float64bits(math.Inf(-1)), 0},
						{math.Float64bits(math.Inf(1)), 1},
						{math.Float64bits(math.Inf(-1)), 1},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(1))},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(-1))},
					} {
						x1, x2 := values[0], values[1]
						isEqz := kind == wazeroir.OperationKindEqz
						if isEqz && (signedType == wazeroir.SignedTypeFloat32 || signedType == wazeroir.SignedTypeFloat64) {
							// Eqz isn't defined for float.
							t.Skip()
						}
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch signedType {
								case wazeroir.SignedTypeUint32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.SignedTypeInt32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(int32(v))})
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							if isEqz {
								// Eqz only needs one value, so pop the top one (x2).
								compiler.valueLocationStack().pop()
								require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
							} else {
								// At this point, two values exist for comparison.
								require.Equal(t, uint64(2), compiler.valueLocationStack().sp)
							}

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindLe:
								err = compiler.compileLe(&wazeroir.OperationLe{Type: signedType})
							case wazeroir.OperationKindLt:
								err = compiler.compileLt(&wazeroir.OperationLt{Type: signedType})
							case wazeroir.OperationKindGe:
								err = compiler.compileGe(&wazeroir.OperationGe{Type: signedType})
							case wazeroir.OperationKindGt:
								err = compiler.compileGt(&wazeroir.OperationGt{Type: signedType})
							case wazeroir.OperationKindEq:
								// Eq uses UnsignedType instead, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI64})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF32})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeF64})
								}
							case wazeroir.OperationKindNe:
								// Ne uses UnsignedType, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI64})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF32})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeF64})
								}
							case wazeroir.OperationKindEqz:
								// Eqz uses UnsignedInt, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt32})
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileEqz(&wazeroir.OperationEqz{Type: wazeroir.UnsignedInt64})
								}
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							require.Equal(t, uint64(1), compiler.valueLocationStack().sp)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// There should only be one value on the stack
							require.Equal(t, uint64(1), env.stackPointer())

							actual := env.stackTopAsUint32() == 1

							switch kind {
							case wazeroir.OperationKindLe:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) <= int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) <= uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) <= int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 <= x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) <= math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) <= math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindLt:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) < int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) < uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) < int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 < x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) < math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) < math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindGe:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) >= int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) >= uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) >= int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 >= x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) >= math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) >= math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindGt:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									require.Equal(t, int32(x1) > int32(x2), actual)
								case wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) > uint32(x2), actual)
								case wazeroir.SignedTypeInt64:
									require.Equal(t, int64(x1) > int64(x2), actual)
								case wazeroir.SignedTypeUint64:
									require.Equal(t, x1 > x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) > math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) > math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindEq:
								switch signedType {
								case wazeroir.SignedTypeInt32, wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) == uint32(x2), actual)
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									require.Equal(t, x1 == x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) == math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) == math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindNe:
								switch signedType {
								case wazeroir.SignedTypeInt32, wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) != uint32(x2), actual)
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									require.Equal(t, x1 != x2, actual)
								case wazeroir.SignedTypeFloat32:
									require.Equal(t, math.Float32frombits(uint32(x1)) != math.Float32frombits(uint32(x2)), actual)
								case wazeroir.SignedTypeFloat64:
									require.Equal(t, math.Float64frombits(x1) != math.Float64frombits(x2), actual)
								}
							case wazeroir.OperationKindEqz:
								switch signedType {
								case wazeroir.SignedTypeInt32, wazeroir.SignedTypeUint32:
									require.Equal(t, uint32(x1) == 0, actual)
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									require.Equal(t, x1 == 0, actual)
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestCompiler_compilePick(t *testing.T) {
	const pickTargetValue uint64 = 12345
	op := &wazeroir.OperationPick{Depth: 1}

	for _, tc := range []struct {
		name                                      string
		pickTargetSetupFunc                       func(compiler compilerImpl, ce *callEngine) error
		isPickTargetFloat, isPickTargetOnRegister bool
	}{
		{
			name: "float on register",
			pickTargetSetupFunc: func(compiler compilerImpl, _ *callEngine) error {
				return compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(pickTargetValue)})
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: true,
		},
		{
			name: "int on register",
			pickTargetSetupFunc: func(compiler compilerImpl, _ *callEngine) error {
				return compiler.compileConstI64(&wazeroir.OperationConstI64{Value: pickTargetValue})
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: true,
		},
		{
			name: "float on stack",
			pickTargetSetupFunc: func(compiler compilerImpl, ce *callEngine) error {
				pickTargetLocation := compiler.valueLocationStack().pushValueLocationOnStack()
				pickTargetLocation.setRegisterType(generalPurposeRegisterTypeFloat)
				ce.valueStack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      true,
			isPickTargetOnRegister: false,
		},
		{
			name: "int on stack",
			pickTargetSetupFunc: func(compiler compilerImpl, ce *callEngine) error {
				pickTargetLocation := compiler.valueLocationStack().pushValueLocationOnStack()
				pickTargetLocation.setRegisterType(generalPurposeRegisterTypeInt)
				ce.valueStack[pickTargetLocation.stackPointer] = pickTargetValue
				return nil
			},
			isPickTargetFloat:      false,
			isPickTargetOnRegister: false,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Set up the stack before picking.
			err = tc.pickTargetSetupFunc(compiler, env.callEngine())
			require.NoError(t, err)
			pickTargetLocation := compiler.valueLocationStack().peek()

			// Push the unused median value.
			_ = compiler.valueLocationStack().pushValueLocationOnStack()
			require.Equal(t, uint64(2), compiler.valueLocationStack().sp)

			// Now ready to compile Pick operation.
			err = compiler.compilePick(op)
			require.NoError(t, err)
			require.Equal(t, uint64(3), compiler.valueLocationStack().sp)

			pickedLocation := compiler.valueLocationStack().peek()
			require.True(t, pickedLocation.onRegister())
			require.Equal(t, pickTargetLocation.registerType(), pickedLocation.registerType())

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Compile and execute the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the returned status and stack pointer.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, uint64(3), env.stackPointer())

			// Verify the top value is the picked one and the pick target's value stays the same.
			if tc.isPickTargetFloat {
				require.Equal(t, math.Float64frombits(pickTargetValue), env.stackTopAsFloat64())
				require.Equal(t, math.Float64frombits(pickTargetValue), math.Float64frombits(env.stack()[pickTargetLocation.stackPointer]))
			} else {
				require.Equal(t, pickTargetValue, env.stackTopAsUint64())
				require.Equal(t, pickTargetValue, env.stack()[pickTargetLocation.stackPointer])
			}
		})
	}
}

func TestCompiler_releaseRegisterToStack(t *testing.T) {
	const val = 10000
	for _, tc := range []struct {
		name         string
		stackPointer uint64
		isFloat      bool
	}{
		{name: "int", stackPointer: 10, isFloat: false},
		{name: "float", stackPointer: 10, isFloat: true},
		{name: "int-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: false},
		{name: "float-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()

			// Build code.
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			s := &valueLocationStack{
				sp:            tc.stackPointer,
				stack:         make([]*valueLocation, tc.stackPointer),
				usedRegisters: map[int16]struct{}{},
			}
			// Peek must be non-nil. Otherwise, compileConst* would fail.
			s.stack[s.sp-1] = &valueLocation{}
			compiler.setValueLocationStack(s)

			if tc.isFloat {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: val})
			}
			require.NoError(t, err)
			// Release the register allocated value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(s.peek())
			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack.
			env.callEngine().builtinFunctionGrowValueStack(tc.stackPointer)
			env.exec(code)

			// JIT status must be returned and stack pointer must end up the specified one.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, tc.stackPointer+1, env.stackPointer())

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val), env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val), env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compileLoadValueOnStackToRegister(t *testing.T) {
	const val = 123
	for _, tc := range []struct {
		name         string
		stackPointer uint64
		isFloat      bool
	}{
		{name: "int", stackPointer: 10, isFloat: false},
		{name: "float", stackPointer: 10, isFloat: true},
		{name: "int-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: false},
		{name: "float-huge-height", stackPointer: math.MaxInt16 + 1, isFloat: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()

			// Build code.
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the location stack so that we push the const on the specified height.
			compiler.valueLocationStack().sp = tc.stackPointer
			compiler.valueLocationStack().stack = make([]*valueLocation, tc.stackPointer)

			// Record that that top value is on top.
			require.Len(t, compiler.valueLocationStack().usedRegisters, 0)
			loc := compiler.valueLocationStack().pushValueLocationOnStack()
			if tc.isFloat {
				loc.setRegisterType(generalPurposeRegisterTypeFloat)
			} else {
				loc.setRegisterType(generalPurposeRegisterTypeInt)
			}
			// At this point the value must be recorded as being on stack.
			require.True(t, loc.onStack())

			// Release the stack-allocated value to register.
			err = compiler.compileEnsureOnGeneralPurposeRegister(loc)
			require.NoError(t, err)
			require.Len(t, compiler.valueLocationStack().usedRegisters, 1)
			require.True(t, loc.onRegister())

			// To verify the behavior, increment the value on the register.
			if tc.isFloat {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: 1})
				require.NoError(t, err)
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: 1})
				require.NoError(t, err)
				err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			}

			// Release the value to the memory stack so that we can see the value after exiting.
			compiler.compileReleaseRegisterToStack(loc)
			require.NoError(t, err)
			compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)
			require.NoError(t, err)

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run native code after growing the value stack, and place the original value.
			env.callEngine().builtinFunctionGrowValueStack(tc.stackPointer)
			env.stack()[tc.stackPointer] = val
			env.exec(code)

			// JIT status must be returned and stack pointer must end up the specified one.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, tc.stackPointer+1, env.stackPointer())

			if tc.isFloat {
				require.Equal(t, math.Float64frombits(val)+1, env.stackTopAsFloat64())
			} else {
				require.Equal(t, uint64(val)+1, env.stackTopAsUint64())
			}
		})
	}
}

func TestCompiler_compileDrop(t *testing.T) {
	t.Run("range nil", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents on stack.
		liveNum := 10
		for i := 0; i < liveNum; i++ {
			compiler.valueLocationStack().pushValueLocationOnStack()
		}
		require.Equal(t, uint64(liveNum), compiler.valueLocationStack().sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Range: nil})
		require.NoError(t, err)

		// After the nil range drop, the stack must remain the same.
		require.Equal(t, uint64(liveNum), compiler.valueLocationStack().sp)

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
	t.Run("start top", func(t *testing.T) {
		r := &wazeroir.InclusiveRange{Start: 0, End: 2}
		dropTargetNum := r.End - r.Start + 1 // +1 as the range is inclusive!
		liveNum := 5

		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents on stack.
		const expectedTopLiveValue = 100
		for i := 0; i < liveNum+dropTargetNum; i++ {
			if i == liveNum-1 {
				err := compiler.compileConstI64(&wazeroir.OperationConstI64{Value: expectedTopLiveValue})
				require.NoError(t, err)
			} else {
				compiler.valueLocationStack().pushValueLocationOnStack()
			}
		}
		require.Equal(t, uint64(liveNum+dropTargetNum), compiler.valueLocationStack().sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Range: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveNum), compiler.valueLocationStack().sp)
		// Plus, the top value must stay on a register.
		top := compiler.valueLocationStack().peek()
		require.True(t, top.onRegister())

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, uint64(5), env.stackPointer())
		require.Equal(t, uint64(expectedTopLiveValue), env.stackTopAsUint64())
	})

	t.Run("start from middle", func(t *testing.T) {
		r := &wazeroir.InclusiveRange{Start: 2, End: 3}
		liveAboveDropStartNum := 3
		dropTargetNum := r.End - r.Start + 1 // +1 as the range is inclusive!
		liveBelowDropEndNum := 5
		total := liveAboveDropStartNum + dropTargetNum + liveBelowDropEndNum
		liveTotal := liveAboveDropStartNum + liveBelowDropEndNum

		env := newJITEnvironment()
		ce := env.callEngine()
		compiler := env.requireNewCompiler(t, nil)

		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Put existing contents except the top on stack
		for i := 0; i < total-1; i++ {
			loc := compiler.valueLocationStack().pushValueLocationOnStack()
			ce.valueStack[loc.stackPointer] = uint64(i) // Put the initial value.
		}

		// Place the top value.
		const expectedTopLiveValue = 100
		err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: expectedTopLiveValue})
		require.NoError(t, err)

		require.Equal(t, uint64(total), compiler.valueLocationStack().sp)

		err = compiler.compileDrop(&wazeroir.OperationDrop{Range: r})
		require.NoError(t, err)

		// After the drop operation, the stack contains only live contents.
		require.Equal(t, uint64(liveTotal), compiler.valueLocationStack().sp)
		// Plus, the top value must stay on a register.
		require.True(t, compiler.valueLocationStack().peek().onRegister())

		err = compiler.compileReturnFunction()
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
		require.Equal(t, uint64(liveTotal), env.stackPointer())

		stack := env.stack()[:env.stackPointer()]
		for i, val := range stack {
			if i <= liveBelowDropEndNum {
				require.Equal(t, uint64(i), val)
			} else if i == liveTotal-1 {
				require.Equal(t, uint64(expectedTopLiveValue), val)
			} else {
				require.Equal(t, uint64(i+dropTargetNum), val)
			}
		}
	})
}

func TestCompiler_compileCall(t *testing.T) {
	for _, growCallFrameStack := range []bool{false, true} {
		growCallFrameStack := growCallFrameStack
		t.Run(fmt.Sprintf("grow=%v", growCallFrameStack), func(t *testing.T) {
			env := newJITEnvironment()
			me := env.moduleEngine()
			expectedValue := uint32(0)

			if growCallFrameStack {
				env.setCallFrameStackPointerLen(1)
			}

			// Emit the call target function.
			const numCalls = 3
			targetFunctionType := &wasm.FunctionType{
				Params:  []wasm.ValueType{wasm.ValueTypeI32},
				Results: []wasm.ValueType{wasm.ValueTypeI32},
			}
			for i := 0; i < numCalls; i++ {
				// Each function takes one arguments, adds the value with 100 + i and returns the result.
				addTargetValue := uint32(100 + i)
				expectedValue += addTargetValue

				// We have to do compilation in a separate subtest since each compilation takes
				// the mutex lock and must release on the cleanup of each subtest.
				// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
				t.Run(fmt.Sprintf("compiling call target %d", i), func(t *testing.T) {
					compiler := env.requireNewCompiler(t, targetFunctionType)

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(addTargetValue)})
					require.NoError(t, err)
					err = compiler.compileAdd(&wazeroir.OperationAdd{Type: wazeroir.UnsignedTypeI32})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					index := wasm.Index(i)
					me.compiledFunctions = append(me.compiledFunctions, &compiledFunction{
						codeSegment:        code,
						codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
					})
					env.module().Functions = append(env.module().Functions,
						&wasm.FunctionInstance{Type: targetFunctionType, Index: index})
				})
			}

			// Now we start building the caller's code.
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			const initialValue = 100
			expectedValue += initialValue
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0}) // Dummy value so the base pointer would be non-trivial for callees.
			require.NoError(t, err)
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: initialValue})
			require.NoError(t, err)

			// Call all the built functions.
			for i := 0; i < numCalls; i++ {
				err = compiler.compileCall(&wazeroir.OperationCall{FunctionIndex: uint32(i)})
				require.NoError(t, err)
			}

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			if growCallFrameStack {
				// If the call frame stack pointer equals the length of call frame stack length,
				// we have to call the builtin function to grow the slice.
				require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
				require.Equal(t, builtinFunctionIndexGrowCallFrameStack, env.builtinFunctionCallAddress())

				// Grow the callFrame stack, and exec again from the return address.
				ce := env.callEngine()
				ce.builtinFunctionGrowCallFrameStack()
				jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(ce)))
			}

			// Check status and returned values.
			require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
			require.Equal(t, uint64(2), env.stackPointer()) // Must be 2 (dummy value + the calculation results)
			require.Equal(t, uint64(0), env.stackBasePointer())
			require.Equal(t, expectedValue, env.stackTopAsUint32())
		})
	}
}

func TestCompiler_compileCallIndirect(t *testing.T) {
	t.Run("out of bounds", func(t *testing.T) {
		env := newJITEnvironment()
		env.setTable(make([]uintptr, 10))
		compiler := env.requireNewCompiler(t, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex.
		env.module().Types = []*wasm.TypeInstance{{Type: &wasm.FunctionType{}}}

		// Place the offset value.
		err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 10})
		require.NoError(t, err)

		err = compiler.compileCallIndirect(targetOperation)
		require.NoError(t, err)

		// We expect to exit from the code in callIndirect so the subsequent code must be unreachable.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)

		// Generate the code under test and run.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeInvalidTableAccess, env.jitStatus())
	})

	t.Run("uninitialized", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		env.module().Types = []*wasm.TypeInstance{{Type: &wasm.FunctionType{}}}

		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]uintptr, 10)
		env.setTable(table)
		table[0] = 0

		// Place the offset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)
		err = compiler.compileCallIndirect(targetOperation)
		require.NoError(t, err)

		// We expect to exit from the code in callIndirect so the subsequent code must be unreachable.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeInvalidTableAccess, env.jitStatus())
	})

	t.Run("type not match", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		targetOperation := &wazeroir.OperationCallIndirect{}
		targetOffset := &wazeroir.OperationConstI32{Value: uint32(0)}
		env.module().Types = []*wasm.TypeInstance{{Type: &wasm.FunctionType{}, TypeID: 1000}}
		// Ensure that the module instance has the type information for targetOperation.TypeIndex,
		// and the typeID doesn't match the table[targetOffset]'s type ID.
		table := make([]uintptr, 10)
		env.setTable(table)

		cf := &compiledFunction{source: &wasm.FunctionInstance{TypeID: 50}}
		table[0] = uintptr(unsafe.Pointer(cf))

		// Place the offset value.
		err = compiler.compileConstI32(targetOffset)
		require.NoError(t, err)

		// Now emit the code.
		require.NoError(t, compiler.compileCallIndirect(targetOperation))

		// We expect to exit from the code in callIndirect so the subsequent code must be unreachable.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		// Generate the code under test and run.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeTypeMismatchOnIndirectCall, env.jitStatus())
	})

	t.Run("ok", func(t *testing.T) {
		for _, growCallFrameStack := range []bool{false} {
			growCallFrameStack := growCallFrameStack
			t.Run(fmt.Sprintf("grow=%v", growCallFrameStack), func(t *testing.T) {
				targetType := &wasm.FunctionType{
					Params:  []wasm.ValueType{},
					Results: []wasm.ValueType{wasm.ValueTypeI32}}
				targetTypeID := wasm.FunctionTypeID(10) // Arbitrary number is fine for testing.
				operation := &wazeroir.OperationCallIndirect{TypeIndex: 0}

				table := make([]uintptr, 10)
				env := newJITEnvironment()
				env.setTable(table)

				// Ensure that the module instance has the type information for targetOperation.TypeIndex,
				// and the typeID  matches the table[targetOffset]'s type ID.
				env.module().Types = make([]*wasm.TypeInstance, 100)
				env.module().Types[operation.TypeIndex] = &wasm.TypeInstance{Type: targetType, TypeID: targetTypeID}
				env.module().Engine = &moduleEngine{compiledFunctions: []*compiledFunction{}}

				me := env.moduleEngine()
				for i := 0; i < len(table); i++ {
					// First we create the call target function with function address = i,
					// and it returns one value.
					expectedReturnValue := uint32(i * 1000)

					// We have to do compilation in a separate subtest since each compilation takes
					// the mutex lock and must release on the cleanup of each subtest.
					// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
					t.Run(fmt.Sprintf("compiling call target for %d", i), func(t *testing.T) {
						compiler := env.requireNewCompiler(t, nil)
						err := compiler.compilePreamble()
						require.NoError(t, err)
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expectedReturnValue})
						require.NoError(t, err)
						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						code, _, _, err := compiler.compile()
						require.NoError(t, err)

						cf := &compiledFunction{
							codeSegment:        code,
							codeInitialAddress: uintptr(unsafe.Pointer(&code[0])),
							source: &wasm.FunctionInstance{
								TypeID: targetTypeID,
							},
						}
						me.compiledFunctions = append(me.compiledFunctions, cf)
						table[i] = uintptr(unsafe.Pointer(cf))
					})
				}

				for i := 1; i < len(table); i++ {
					expectedReturnValue := uint32(i * 1000)
					t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
						if growCallFrameStack {
							env.setCallFrameStackPointerLen(1)
						}

						compiler := env.requireNewCompiler(t, nil)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Place the offset value. Here we try calling a function of functionaddr == table[i].FunctionIndex.
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(i)})
						require.NoError(t, err)

						// At this point, we should have one item (offset value) on the stack.
						require.Equal(t, uint64(1), compiler.valueLocationStack().sp)

						require.NoError(t, compiler.compileCallIndirect(operation))

						// At this point, we consumed the offset value, but the function returns one value,
						// so the stack pointer results in the same.
						require.Equal(t, uint64(1), compiler.valueLocationStack().sp)

						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						// Generate the code under test and run.
						code, _, _, err := compiler.compile()
						require.NoError(t, err)
						env.exec(code)

						if growCallFrameStack {
							// If the call frame stack pointer equals the length of call frame stack length,
							// we have to call the builtin function to grow the slice.
							require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
							require.Equal(t, builtinFunctionIndexGrowCallFrameStack, env.builtinFunctionCallAddress())

							// Grow the callFrame stack, and exec again from the return address.
							ce := env.callEngine()
							ce.builtinFunctionGrowCallFrameStack()
							jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(ce)))
						}

						require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
						require.Equal(t, uint64(1), env.stackPointer())
						require.Equal(t, expectedReturnValue, uint32(env.ce.popValue()))
					})
				}
			})
		}
	})
}

func TestCompiler_compileSelect(t *testing.T) {
	// There are mainly 8 cases we have to test:
	// - [x1 = reg, x2 = reg] select x1
	// - [x1 = reg, x2 = reg] select x2
	// - [x1 = reg, x2 = stack] select x1
	// - [x1 = reg, x2 = stack] select x2
	// - [x1 = stack, x2 = reg] select x1
	// - [x1 = stack, x2 = reg] select x2
	// - [x1 = stack, x2 = stack] select x1
	// - [x1 = stack, x2 = stack] select x2
	// And for each case, we have to test with
	// three conditional value location: stack, gp register, conditional register.
	// So in total we have 24 cases.
	for i, tc := range []struct {
		x1OnRegister, x2OnRegister                                        bool
		selectX1                                                          bool
		condlValueOnStack, condValueOnGPRegister, condValueOnCondRegister bool
	}{
		// Conditional value on stack.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condlValueOnStack: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condlValueOnStack: true},
		// Conditional value on register.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condValueOnGPRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condValueOnGPRegister: true},
		// Conditional value on conditional register.
		{x1OnRegister: true, x2OnRegister: true, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: true, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: true, x2OnRegister: false, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: true, selectX1: false, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: true, condValueOnCondRegister: true},
		{x1OnRegister: false, x2OnRegister: false, selectX1: false, condValueOnCondRegister: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			for _, vals := range [][2]uint64{
				{1, 2}, {0, 1}, {1, 0},
				{math.Float64bits(-1), math.Float64bits(-1)},
				{math.Float64bits(-1), math.Float64bits(1)},
				{math.Float64bits(1), math.Float64bits(-1)},
			} {
				x1Value, x2Value := vals[0], vals[1]
				t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", vals[0], vals[1]), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					x1 := compiler.valueLocationStack().pushValueLocationOnStack()
					env.stack()[x1.stackPointer] = x1Value
					if tc.x1OnRegister {
						err = compiler.compileEnsureOnGeneralPurposeRegister(x1)
						require.NoError(t, err)
					}

					x2 := compiler.valueLocationStack().pushValueLocationOnStack()
					env.stack()[x2.stackPointer] = x2Value
					if tc.x2OnRegister {
						err = compiler.compileEnsureOnGeneralPurposeRegister(x2)
						require.NoError(t, err)
					}

					var c *valueLocation
					if tc.condlValueOnStack {
						c = compiler.valueLocationStack().pushValueLocationOnStack()
						if tc.selectX1 {
							env.stack()[c.stackPointer] = 1
						} else {
							env.stack()[c.stackPointer] = 0
						}
					} else if tc.condValueOnGPRegister {
						c = compiler.valueLocationStack().pushValueLocationOnStack()
						if tc.selectX1 {
							env.stack()[c.stackPointer] = 1
						} else {
							env.stack()[c.stackPointer] = 0
						}
						err = compiler.compileEnsureOnGeneralPurposeRegister(c)
						require.NoError(t, err)
					} else if tc.condValueOnCondRegister {
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
						require.NoError(t, err)
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
						require.NoError(t, err)
						if tc.selectX1 {
							err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
						} else {
							err = compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
						}
						require.NoError(t, err)
					}

					// Now emit code for select.
					err = compiler.compileSelect()
					require.NoError(t, err)

					// x1 should be top of the stack.
					require.Equal(t, x1, compiler.valueLocationStack().peek())

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Run code.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the selected value.
					require.Equal(t, uint64(1), env.stackPointer())
					if tc.selectX1 {
						require.Equal(t, env.stack()[x1.stackPointer], uint64(x1Value))
					} else {
						require.Equal(t, env.stack()[x1.stackPointer], uint64(x2Value))
					}
				})
			}
		})
	}
}

func TestCompiler_compileSwap(t *testing.T) {
	var x1Value, x2Value int64 = 100, 200
	for i, tc := range []struct {
		x1OnConditionalRegister, x1OnRegister, x2OnRegister bool
	}{
		{x1OnRegister: true, x2OnRegister: true},
		{x1OnRegister: true, x2OnRegister: false},
		{x1OnRegister: false, x2OnRegister: true},
		{x1OnRegister: false, x2OnRegister: false},
		// x1 on conditional register
		{x1OnConditionalRegister: true, x2OnRegister: false},
		{x1OnConditionalRegister: true, x2OnRegister: true},
	} {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			x2 := compiler.valueLocationStack().pushValueLocationOnStack()
			env.stack()[x2.stackPointer] = uint64(x2Value)
			if tc.x2OnRegister {
				err = compiler.compileEnsureOnGeneralPurposeRegister(x2)
				require.NoError(t, err)
			}

			_ = compiler.valueLocationStack().pushValueLocationOnStack() // Dummy value!
			if tc.x1OnRegister && !tc.x1OnConditionalRegister {
				x1 := compiler.valueLocationStack().pushValueLocationOnStack()
				env.stack()[x1.stackPointer] = uint64(x1Value)
				err = compiler.compileEnsureOnGeneralPurposeRegister(x1)
				require.NoError(t, err)
			} else if !tc.x1OnConditionalRegister {
				x1 := compiler.valueLocationStack().pushValueLocationOnStack()
				env.stack()[x1.stackPointer] = uint64(x1Value)
			} else {
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 0})
				require.NoError(t, err)
				err = compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
				x1Value = 1
			}

			// Swap x1 and x2.
			err = compiler.compileSwap(&wazeroir.OperationSwap{Depth: 2})
			require.NoError(t, err)

			require.NoError(t, compiler.compileReturnFunction())

			// Generate the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Run code.
			env.exec(code)

			require.Equal(t, uint64(3), env.stackPointer())
			// Check values are swapped.
			require.Equal(t, uint64(x1Value), env.stack()[0])
			require.Equal(t, uint64(x2Value), env.stack()[2])
		})
	}
}

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
				Table:   &wasm.TableInstance{Table: make([]uintptr, 20)},
			},
		},
		{
			name: "globals nil",
			moduleInstance: &wasm.ModuleInstance{
				Memory: &wasm.MemoryInstance{Buffer: make([]byte, 10)},
				Table:  &wasm.TableInstance{Table: make([]uintptr, 20)},
			},
		},
		{
			name: "memory nil",
			moduleInstance: &wasm.ModuleInstance{
				Globals: []*wasm.GlobalInstance{{Val: 100}},
				Table:   &wasm.TableInstance{Table: make([]uintptr, 20)},
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
				Table: &wasm.TableInstance{Table: make([]uintptr, 0)},
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

			compiler := env.requireNewCompiler(t, nil)
			me := &moduleEngine{compiledFunctions: make([]*compiledFunction, 10)}
			tc.moduleInstance.Engine = me

			// The assembler skips the first instruction so we intentionally add const op here, which is ignored.
			// TODO: delete after #233
			err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 1})
			require.NoError(t, err)
			loc := compiler.valueLocationStack().pop()
			compiler.valueLocationStack().markRegisterUnused(loc.register)

			err = compiler.compileModuleContextInitialization()
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

func TestCompiler_compileGlobalGet(t *testing.T) {
	const globalValue uint64 = 12345
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)

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
			require.Len(t, compiler.valueLocationStack().usedRegisters, 1)
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
	for i, tp := range []wasm.ValueType{
		wasm.ValueTypeF32, wasm.ValueTypeF64,
		wasm.ValueTypeI32, wasm.ValueTypeI64,
	} {
		tp := tp
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)

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

func TestCompiler_MemoryOutOfBounds(t *testing.T) {
	bases := []uint32{0, 1 << 5, 1 << 9, 1 << 10, 1 << 15, math.MaxUint32 - 1, math.MaxUint32}
	offsets := []uint32{0,
		1 << 10, 1 << 31,
		defaultMemoryPageNumInTest*wasm.MemoryPageSize - 1, defaultMemoryPageNumInTest * wasm.MemoryPageSize,
		math.MaxInt32 - 1, math.MaxInt32 - 2, math.MaxInt32 - 3, math.MaxInt32 - 4,
		math.MaxInt32 - 5, math.MaxInt32 - 8, math.MaxInt32 - 9, math.MaxInt32, math.MaxUint32,
	}
	targetSizeInBytes := []int64{1, 2, 4, 8}
	for _, base := range bases {
		base := base
		for _, offset := range offsets {
			offset := offset
			for _, targetSizeInByte := range targetSizeInBytes {
				targetSizeInByte := targetSizeInByte
				t.Run(fmt.Sprintf("base=%d,offset=%d,targetSizeInBytes=%d", base, offset, targetSizeInByte), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)

					err := compiler.compilePreamble()
					require.NoError(t, err)

					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: base})
					require.NoError(t, err)

					arg := &wazeroir.MemoryImmediate{Offset: offset}

					switch targetSizeInByte {
					case 1:
						err = compiler.compileLoad8(&wazeroir.OperationLoad8{Type: wazeroir.SignedInt32, Arg: arg})
					case 2:
						err = compiler.compileLoad16(&wazeroir.OperationLoad16{Type: wazeroir.SignedInt32, Arg: arg})
					case 4:
						err = compiler.compileLoad32(&wazeroir.OperationLoad32{Signed: false, Arg: arg})
					case 8:
						err = compiler.compileLoad(&wazeroir.OperationLoad{Type: wazeroir.UnsignedTypeF64, Arg: arg})
					default:
						t.Fail()
					}

					require.NoError(t, err)

					require.NoError(t, compiler.compileReturnFunction())

					// Generate the code under test and run.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					mem := env.memory()
					if ceil := int64(base) + int64(offset) + int64(targetSizeInByte); int64(len(mem)) < ceil {
						// If the targe memory region's ceil exceeds the length of memory, we must exit the function
						// with jitCallStatusCodeMemoryOutOfBounds status code.
						require.Equal(t, jitCallStatusCodeMemoryOutOfBounds, env.jitStatus())
					}
				})
			}
		}
	}
}

func TestCompiler_compileStore(t *testing.T) {
	// For testing. Arbitrary number is fine.
	storeTargetValue := uint64(math.MaxUint64)
	baseOffset := uint32(100)
	arg := &wazeroir.MemoryImmediate{Offset: 361}
	offset := arg.Offset + baseOffset

	for _, tc := range []struct {
		name                string
		isFloatTarget       bool
		targetSizeInBytes   uint32
		operationSetupFn    func(t *testing.T, compiler compilerImpl)
		storedValueVerifyFn func(t *testing.T, mem []byte)
	}{
		{
			name:              "i32.store",
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
		{
			name:              "f32.store",
			isFloatTarget:     true,
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
		{
			name:              "i64.store",
			targetSizeInBytes: 64 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, storeTargetValue, binary.LittleEndian.Uint64(mem[offset:]))
			},
		},
		{
			name:              "f64.store",
			isFloatTarget:     true,
			targetSizeInBytes: 64 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore(&wazeroir.OperationStore{Arg: arg, Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, storeTargetValue, binary.LittleEndian.Uint64(mem[offset:]))
			},
		},
		{
			name:              "store8",
			targetSizeInBytes: 1,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore8(&wazeroir.OperationStore8{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, byte(storeTargetValue), mem[offset])
			},
		},
		{
			name:              "store16",
			targetSizeInBytes: 16 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore16(&wazeroir.OperationStore16{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint16(storeTargetValue), binary.LittleEndian.Uint16(mem[offset:]))
			},
		},
		{
			name:              "store32",
			targetSizeInBytes: 32 / 8,
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileStore32(&wazeroir.OperationStore32{Arg: arg})
				require.NoError(t, err)
			},
			storedValueVerifyFn: func(t *testing.T, mem []byte) {
				require.Equal(t, uint32(storeTargetValue), binary.LittleEndian.Uint32(mem[offset:]))
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)

			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Before store operations, we must push the base offset, and the store target values.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: baseOffset})
			require.NoError(t, err)
			if tc.isFloatTarget {
				err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(storeTargetValue)})
			} else {
				err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: storeTargetValue})
			}
			require.NoError(t, err)

			tc.operationSetupFn(t, compiler)

			// At this point, no registers must be in use, and no values on the stack since we consumed two values.
			require.Len(t, compiler.valueLocationStack().usedRegisters, 0)
			require.Equal(t, uint64(0), compiler.valueLocationStack().sp)

			// Generate the code under test.
			err = compiler.compileReturnFunction()
			require.NoError(t, err)
			code, _, _, err := compiler.compile()
			require.NoError(t, err)

			// Set the value on the left and right neighboring memoryregion,
			// so that we can verify the operation doesn't affect there.
			ceil := offset + tc.targetSizeInBytes
			mem := env.memory()
			expectedNeighbor8Bytes := uint64(0x12_34_56_78_9a_bc_ef_fe)
			binary.LittleEndian.PutUint64(mem[offset-8:offset], expectedNeighbor8Bytes)
			binary.LittleEndian.PutUint64(mem[ceil:ceil+8], expectedNeighbor8Bytes)

			// Run code.
			env.exec(code)

			tc.storedValueVerifyFn(t, mem)

			// The neighboring bytes must be intact.
			require.Equal(t, expectedNeighbor8Bytes, binary.LittleEndian.Uint64(mem[offset-8:offset]))
			require.Equal(t, expectedNeighbor8Bytes, binary.LittleEndian.Uint64(mem[ceil:ceil+8]))
		})
	}
}

func TestCompiler_compileLoad(t *testing.T) {
	// For testing. Arbitrary number is fine.
	loadTargetValue := uint64(0x12_34_56_78_9a_bc_ef_fe)
	baseOffset := uint32(100)
	arg := &wazeroir.MemoryImmediate{Offset: 361}
	offset := baseOffset + arg.Offset

	for _, tc := range []struct {
		name                string
		isFloatTarget       bool
		operationSetupFn    func(t *testing.T, compiler compilerImpl)
		loadedValueVerifyFn func(t *testing.T, loadedValueAsUint64 uint64)
	}{
		{
			name: "i32.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadTargetValue), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeI64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, loadTargetValue, loadedValueAsUint64)
			},
		},
		{
			name: "f32.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeF32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadTargetValue), uint32(loadedValueAsUint64))
			},
			isFloatTarget: true,
		},
		{
			name: "f64.load",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad(&wazeroir.OperationLoad{Arg: arg, Type: wazeroir.UnsignedTypeF64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, loadTargetValue, loadedValueAsUint64)
			},
			isFloatTarget: true,
		},
		{
			name: "i32.load8s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedInt32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int32(int8(loadedValueAsUint64)), int32(uint32(loadedValueAsUint64)))
			},
		},
		{
			name: "i32.load8u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedUint32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(byte(loadedValueAsUint64)), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load8s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedInt64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int8(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load8u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad8(&wazeroir.OperationLoad8{Arg: arg, Type: wazeroir.SignedUint64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(byte(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
		{
			name: "i32.load16s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedInt32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int32(int16(loadedValueAsUint64)), int32(uint32(loadedValueAsUint64)))
			},
		},
		{
			name: "i32.load16u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedUint32})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint32(loadedValueAsUint64), uint32(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load16s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedInt64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int16(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load16u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad16(&wazeroir.OperationLoad16{Arg: arg, Type: wazeroir.SignedUint64})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(uint16(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
		{
			name: "i64.load32s",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad32(&wazeroir.OperationLoad32{Arg: arg, Signed: true})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, int64(int32(loadedValueAsUint64)), int64(loadedValueAsUint64))
			},
		},
		{
			name: "i64.load32u",
			operationSetupFn: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileLoad32(&wazeroir.OperationLoad32{Arg: arg, Signed: false})
				require.NoError(t, err)
			},
			loadedValueVerifyFn: func(t *testing.T, loadedValueAsUint64 uint64) {
				require.Equal(t, uint64(uint32(loadedValueAsUint64)), loadedValueAsUint64)
			},
		},
	} {

		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)

			err := compiler.compilePreamble()
			require.NoError(t, err)

			binary.LittleEndian.PutUint64(env.memory()[offset:], loadTargetValue)

			// Before load operation, we must push the base offset value.
			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: baseOffset})
			require.NoError(t, err)

			tc.operationSetupFn(t, compiler)

			// At this point, the loaded value must be on top of the stack, and placed on a register.
			require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
			require.Len(t, compiler.valueLocationStack().usedRegisters, 1)
			loadedLocation := compiler.valueLocationStack().peek()
			require.True(t, loadedLocation.onRegister())
			if tc.isFloatTarget {
				require.Equal(t, generalPurposeRegisterTypeFloat, loadedLocation.registerType())
			} else {
				require.Equal(t, generalPurposeRegisterTypeInt, loadedLocation.registerType())
			}
			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Verify the loaded value.
			require.Equal(t, uint64(1), env.stackPointer())
			tc.loadedValueVerifyFn(t, env.stackTopAsUint64())
		})
	}
}

func TestCompiler_compileMemorySize(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t, nil)

	err := compiler.compilePreamble()
	require.NoError(t, err)

	// Emit memory.size instructions.
	err = compiler.compileMemorySize()
	require.NoError(t, err)
	// At this point, the size of memory should be pushed onto the stack.
	require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
	require.Equal(t, generalPurposeRegisterTypeInt, compiler.valueLocationStack().peek().registerType())

	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	require.Equal(t, uint32(defaultMemoryPageNumInTest), env.stackTopAsUint32())
}

func TestCompiler_compileMemoryGrow(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t, nil)
	err := compiler.compilePreamble()
	require.NoError(t, err)

	err = compiler.compileMemoryGrow()
	require.NoError(t, err)

	// Emit arbitrary code after MemoryGrow returned so that we can verify
	// that the code can set the return address properly.
	const expValue uint32 = 100
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: expValue})
	require.NoError(t, err)
	err = compiler.compileReturnFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// After the initial exec, the code must exit with builtin function call status and funcaddress for memory grow.
	require.Equal(t, jitCallStatusCodeCallBuiltInFunction, env.jitStatus())
	require.Equal(t, builtinFunctionIndexMemoryGrow, env.builtinFunctionCallAddress())

	// Reenter from the return address.
	jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.callEngine())))

	// Check if the code successfully executed the code after builtin function call.
	require.Equal(t, expValue, env.stackTopAsUint32())
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

func TestCompiler_compileHostFunction(t *testing.T) {
	env := newJITEnvironment()
	compiler := env.requireNewCompiler(t, nil)

	// The assembler skips the first instruction so we intentionally add const op here, which is ignored.
	// TODO: delete after #233
	err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: 1})
	require.NoError(t, err)
	compiler.valueLocationStack().pop()

	err = compiler.compileHostFunction()
	require.NoError(t, err)

	// Generate and run the code under test.
	code, _, _, err := compiler.compile()
	require.NoError(t, err)
	env.exec(code)

	// On the return, the code must exit with the host call status.
	require.Equal(t, jitCallStatusCodeCallHostFunction, env.jitStatus())

	// Re-enter the return address.
	jitcall(env.callFrameStackPeek().returnAddress, uintptr(unsafe.Pointer(env.callEngine())))

	// After that, the code must exit with returned status.
	require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
}

func TestCompiler_compile_Clz_Ctz_Popcnt(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindClz,
		wazeroir.OperationKindCtz,
		wazeroir.OperationKindPopcnt,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, tp := range []wazeroir.UnsignedInt{wazeroir.UnsignedInt32, wazeroir.UnsignedInt64} {
				tp := tp
				is32bit := tp == wazeroir.UnsignedInt32
				t.Run(tp.String(), func(t *testing.T) {
					for _, v := range []uint64{
						0, 1, 1 << 4, 1 << 6, 1 << 31,
						0b11111111110000, 0b010101010, 0b1111111111111, math.MaxUint64,
					} {
						name := fmt.Sprintf("%064b", v)
						if is32bit {
							name = fmt.Sprintf("%032b", v)
						}
						t.Run(name, func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							if is32bit {
								err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
							} else {
								err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
							}
							require.NoError(t, err)

							switch kind {
							case wazeroir.OperationKindClz:
								err = compiler.compileClz(&wazeroir.OperationClz{Type: tp})
							case wazeroir.OperationKindCtz:
								err = compiler.compileCtz(&wazeroir.OperationCtz{Type: tp})
							case wazeroir.OperationKindPopcnt:
								err = compiler.compilePopcnt(&wazeroir.OperationPopcnt{Type: tp})
							}
							require.NoError(t, err)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Generate and run the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// One value must be pushed as a result.
							require.Equal(t, uint64(1), env.stackPointer())

							switch kind {
							case wazeroir.OperationKindClz:
								if is32bit {
									require.Equal(t, bits.LeadingZeros32(uint32(v)), int(env.stackTopAsUint32()))
								} else {
									require.Equal(t, bits.LeadingZeros64(v), int(env.stackTopAsUint32()))
								}
							case wazeroir.OperationKindCtz:
								if is32bit {
									require.Equal(t, bits.TrailingZeros32(uint32(v)), int(env.stackTopAsUint32()))
								} else {
									require.Equal(t, bits.TrailingZeros64(v), int(env.stackTopAsUint32()))
								}
							case wazeroir.OperationKindPopcnt:
								if is32bit {
									require.Equal(t, bits.OnesCount32(uint32(v)), int(env.stackTopAsUint32()))
								} else {
									require.Equal(t, bits.OnesCount64(v), int(env.stackTopAsUint32()))
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestCompiler_compileF32DemoteFromF64(t *testing.T) {
	for _, v := range []float64{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
		6.8719476736e+10,  /* = 1 << 36 */
		1.37438953472e+11, /* = 1 << 37 */
		math.Inf(1), math.Inf(-1), math.NaN(),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the demote target.
			err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
			require.NoError(t, err)

			err = compiler.compileF32DemoteFromF64()
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the result.
			require.Equal(t, uint64(1), env.stackPointer())
			if math.IsNaN(v) {
				require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
			} else {
				exp := float32(v)
				actual := env.stackTopAsFloat32()
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestCompiler_compileF64PromoteFromF32(t *testing.T) {
	for _, v := range []float32{
		0, 100, -100, 1, -1,
		100.01234124, -100.01234124, 200.12315,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()),
	} {
		t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)
			err := compiler.compilePreamble()
			require.NoError(t, err)

			// Setup the promote target.
			err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: v})
			require.NoError(t, err)

			err = compiler.compileF64PromoteFromF32()
			require.NoError(t, err)

			err = compiler.compileReturnFunction()
			require.NoError(t, err)

			// Generate and run the code under test.
			code, _, _, err := compiler.compile()
			require.NoError(t, err)
			env.exec(code)

			// Check the result.
			require.Equal(t, uint64(1), env.stackPointer())
			if math.IsNaN(float64(v)) {
				require.True(t, math.IsNaN(env.stackTopAsFloat64()))
			} else {
				exp := float64(v)
				actual := env.stackTopAsFloat64()
				require.Equal(t, exp, actual)
			}
		})
	}
}

func TestCompiler_compileReinterpret(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindF32ReinterpretFromI32,
		wazeroir.OperationKindF64ReinterpretFromI64,
		wazeroir.OperationKindI32ReinterpretFromF32,
		wazeroir.OperationKindI64ReinterpretFromF64,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, originOnStack := range []bool{false, true} {
				originOnStack := originOnStack
				t.Run(fmt.Sprintf("%v", originOnStack), func(t *testing.T) {
					for _, v := range []uint64{
						0, 1, 1 << 16, 1 << 31, 1 << 32, 1 << 63,
						math.MaxInt32, math.MaxUint32, math.MaxUint64,
					} {
						v := v
						t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							if originOnStack {
								loc := compiler.valueLocationStack().pushValueLocationOnStack()
								env.stack()[loc.stackPointer] = v
								env.setStackPointer(1)
							}

							var is32Bit bool
							switch kind {
							case wazeroir.OperationKindF32ReinterpretFromI32:
								is32Bit = true
								if !originOnStack {
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
									require.NoError(t, err)
								}
								err = compiler.compileF32ReinterpretFromI32()
								require.NoError(t, err)
							case wazeroir.OperationKindF64ReinterpretFromI64:
								if !originOnStack {
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
									require.NoError(t, err)
								}
								err = compiler.compileF64ReinterpretFromI64()
								require.NoError(t, err)
							case wazeroir.OperationKindI32ReinterpretFromF32:
								is32Bit = true
								if !originOnStack {
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
									require.NoError(t, err)
								}
								err = compiler.compileI32ReinterpretFromF32()
								require.NoError(t, err)
							case wazeroir.OperationKindI64ReinterpretFromF64:
								if !originOnStack {
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
									require.NoError(t, err)
								}
								err = compiler.compileI64ReinterpretFromF64()
								require.NoError(t, err)
							default:
								t.Fail()
							}

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Generate and run the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							// Reinterpret must preserve the bit-pattern.
							if is32Bit {
								require.Equal(t, uint32(v), env.stackTopAsUint32())
							} else {
								require.Equal(t, v, env.stackTopAsUint64())
							}
						})
					}
				})
			}
		})
	}
}

func TestCompiler_compileExtend(t *testing.T) {
	for _, signed := range []bool{false, true} {
		signed := signed
		t.Run(fmt.Sprintf("signed=%v", signed), func(t *testing.T) {
			for _, v := range []uint32{
				0, 1, 1 << 14, 1 << 31, math.MaxUint32, 0xFFFFFFFF, math.MaxInt32,
			} {
				v := v
				t.Run(fmt.Sprintf("%v", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the promote target.
					err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: v})
					require.NoError(t, err)

					err = compiler.compileExtend(&wazeroir.OperationExtend{Signed: signed})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, uint64(1), env.stackPointer())
					if signed {
						expected := int64(int32(v))
						require.Equal(t, expected, env.stackTopAsInt64())
					} else {
						expected := uint64(uint32(v))
						require.Equal(t, expected, env.stackTopAsUint64())
					}
				})
			}
		})
	}
}

func TestCompiler_compileSignExtend(t *testing.T) {
	type fromKind byte
	from8, from16, from32 := fromKind(0), fromKind(1), fromKind(2)

	t.Run("32bit", func(t *testing.T) {
		for _, tc := range []struct {
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
		} {
			tc := tc
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t, nil)
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
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt32())
			})
		}
	})
	t.Run("64bit", func(t *testing.T) {
		for _, tc := range []struct {
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
		} {
			tc := tc
			t.Run(fmt.Sprintf("0x%x", tc.in), func(t *testing.T) {
				env := newJITEnvironment()
				compiler := env.requireNewCompiler(t, nil)
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
				code, _, _, err := compiler.compile()
				require.NoError(t, err)
				env.exec(code)

				require.Equal(t, uint64(1), env.stackPointer())
				require.Equal(t, tc.expected, env.stackTopAsInt64())
			})
		}
	})
}

func TestCompiler_compileITruncFromF(t *testing.T) {
	for _, tc := range []struct {
		outputType wazeroir.SignedInt
		inputType  wazeroir.Float
	}{
		{outputType: wazeroir.SignedInt32, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedInt32, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedInt64, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedInt64, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedUint32, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedUint32, inputType: wazeroir.Float64},
		{outputType: wazeroir.SignedUint64, inputType: wazeroir.Float32},
		{outputType: wazeroir.SignedUint64, inputType: wazeroir.Float64},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s from %s", tc.outputType, tc.inputType), func(t *testing.T) {
			for _, v := range []float64{
				1.0, 100, -100, 1, -1, 100.01234124, -100.01234124, 200.12315,
				6.8719476736e+10 /* = 1 << 36 */, -6.8719476736e+10, 1.37438953472e+11, /* = 1 << 37 */
				-1.37438953472e+11, -2147483649.0, 2147483648.0, math.MinInt32,
				math.MaxInt32, math.MaxUint32, math.MinInt64, math.MaxInt64,
				math.MaxUint64, math.MaxFloat32, math.SmallestNonzeroFloat32, math.MaxFloat64,
				math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1), math.NaN(),
			} {
				v := v
				if v == math.MaxInt32 {
					// Note that math.MaxInt32 is rounded up to math.MaxInt32+1 in 32-bit float representation.
					require.Equal(t, float32(2147483648.0) /* = math.MaxInt32+1 */, float32(v))
				} else if v == math.MaxUint32 {
					// Note that math.MaxUint32 is rounded up to math.MaxUint32+1 in 32-bit float representation.
					require.Equal(t, float32(4294967296 /* = math.MaxUint32+1 */), float32(v))
				} else if v == math.MaxInt64 {
					// Note that math.MaxInt64 is rounded up to math.MaxInt64+1 in 32/64-bit float representation.
					require.Equal(t, float32(9223372036854775808.0) /* = math.MaxInt64+1 */, float32(v))
					require.Equal(t, float64(9223372036854775808.0) /* = math.MaxInt64+1 */, float64(v))
				} else if v == math.MaxUint64 {
					// Note that math.MaxUint64 is rounded up to math.MaxUint64+1 in 32/64-bit float representation.
					require.Equal(t, float32(18446744073709551616.0) /* = math.MaxInt64+1 */, float32(v))
					require.Equal(t, float64(18446744073709551616.0) /* = math.MaxInt64+1 */, float64(v))
				}

				t.Run(fmt.Sprintf("%v", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the conversion target.
					if tc.inputType == wazeroir.Float32 {
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(v)})
					} else {
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
					}
					require.NoError(t, err)

					err = compiler.compileITruncFromF(&wazeroir.OperationITruncFromF{
						InputType: tc.inputType, OutputType: tc.outputType,
					})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					expStatus := jitCallStatusCodeReturned
					if math.IsNaN(v) {
						expStatus = jitCallStatusCodeInvalidFloatToIntConversion
					}
					if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedInt32 {
						f32 := float32(v)
						if f32 < math.MinInt32 || f32 >= math.MaxInt32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int32(math.Trunc(float64(f32))), env.stackTopAsInt32())
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedInt64 {
						f32 := float32(v)
						if f32 < math.MinInt64 || f32 >= math.MaxInt64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int64(math.Trunc(float64(f32))), env.stackTopAsInt64())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedInt32 {
						if v < math.MinInt32 || v > math.MaxInt32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int32(math.Trunc(v)), env.stackTopAsInt32())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedInt64 {
						if v < math.MinInt64 || v >= math.MaxInt64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, int64(math.Trunc(v)), env.stackTopAsInt64())
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedUint32 {
						f32 := float32(v)
						if f32 < 0 || f32 >= math.MaxUint32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint32(math.Trunc(float64(f32))), env.stackTopAsUint32())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedUint32 {
						if v < 0 || v > math.MaxUint32 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint32(math.Trunc(v)), env.stackTopAsUint32())
						}
					} else if tc.inputType == wazeroir.Float32 && tc.outputType == wazeroir.SignedUint64 {
						f32 := float32(v)
						if f32 < 0 || f32 >= math.MaxUint64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint64(math.Trunc(float64(f32))), env.stackTopAsUint64())
						}
					} else if tc.inputType == wazeroir.Float64 && tc.outputType == wazeroir.SignedUint64 {
						if v < 0 || v >= math.MaxUint64 {
							expStatus = jitCallStatusIntegerOverflow
						}
						if expStatus == jitCallStatusCodeReturned {
							require.Equal(t, uint64(math.Trunc(v)), env.stackTopAsUint64())
						}
					}
					require.Equal(t, expStatus, env.jitStatus())
				})
			}
		})
	}
}

func TestCompiler_compileFConvertFromI(t *testing.T) {
	for _, tc := range []struct {
		inputType  wazeroir.SignedInt
		outputType wazeroir.Float
	}{
		{inputType: wazeroir.SignedInt32, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedInt32, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedInt64, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedInt64, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedUint32, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedUint32, outputType: wazeroir.Float64},
		{inputType: wazeroir.SignedUint64, outputType: wazeroir.Float32},
		{inputType: wazeroir.SignedUint64, outputType: wazeroir.Float64},
	} {
		tc := tc
		t.Run(fmt.Sprintf("%s from %s", tc.outputType, tc.inputType), func(t *testing.T) {
			for _, v := range []uint64{
				0, 1, 12345, 1 << 31, 1 << 32, 1 << 54, 1 << 63,
				0xffff_ffff_ffff_ffff, 0xffff_ffff,
				0xffff_ffff_ffff_fffe, 0xffff_fffe,
				math.MaxUint32, math.MaxUint64, math.MaxInt32, math.MaxInt64,
			} {
				t.Run(fmt.Sprintf("%d", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the conversion target.
					if tc.inputType == wazeroir.SignedInt32 || tc.inputType == wazeroir.SignedUint32 {
						err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(v)})
					} else {
						err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: uint64(v)})
					}
					require.NoError(t, err)

					err = compiler.compileFConvertFromI(&wazeroir.OperationFConvertFromI{
						InputType: tc.inputType, OutputType: tc.outputType,
					})
					require.NoError(t, err)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					// Check the result.
					require.Equal(t, uint64(1), env.stackPointer())
					actualBits := env.stackTopAsUint64()
					if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedInt32 {
						exp := float32(int32(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedInt64 {
						exp := float32(int64(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedInt32 {
						exp := float64(int32(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedInt64 {
						exp := float64(int64(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedUint32 {
						exp := float32(uint32(v))
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedUint32 {
						exp := float64(uint32(v))
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float32 && tc.inputType == wazeroir.SignedUint64 {
						exp := float32(v)
						actual := math.Float32frombits(uint32(actualBits))
						require.Equal(t, exp, actual)
					} else if tc.outputType == wazeroir.Float64 && tc.inputType == wazeroir.SignedUint64 {
						exp := float64(v)
						actual := math.Float64frombits(actualBits)
						require.Equal(t, exp, actual)
					}
				})
			}
		})
	}
}

func TestCompiler_compile_Min_Max_Copysign(t *testing.T) {
	for _, tc := range []struct {
		name       string
		is32bit    bool
		setupFunc  func(t *testing.T, compiler compilerImpl)
		verifyFunc func(t *testing.T, x1, x2 float64, raw uint64)
	}{
		{
			name:    "min-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMin(&wazeroir.OperationMin{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := float32(moremath.WasmCompatMin(float64(float32(x1)), float64(float32(x2))))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "min-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMin(&wazeroir.OperationMin{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMin(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "max-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMax(&wazeroir.OperationMax{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := float32(moremath.WasmCompatMax(float64(float32(x1)), float64(float32(x2))))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "max-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMax(&wazeroir.OperationMax{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMax(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "max-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileCopysign(&wazeroir.OperationCopysign{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := float32(math.Copysign(float64(float32(x1)), float64(float32(x2))))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "copysign-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileCopysign(&wazeroir.OperationCopysign{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := math.Copysign(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, vs := range [][2]float64{
				{100, -1.1}, {100, 0}, {0, 0}, {1, 1},
				{-1, 100}, {100, 200}, {100.01234124, 100.01234124},
				{100.01234124, -100.01234124}, {200.12315, 100},
				{6.8719476736e+10 /* = 1 << 36 */, 100},
				{6.8719476736e+10 /* = 1 << 36 */, 1.37438953472e+11 /* = 1 << 37*/},
				{math.Inf(1), 100}, {math.Inf(1), -100},
				{100, math.Inf(1)}, {-100, math.Inf(1)},
				{math.Inf(-1), 100}, {math.Inf(-1), -100},
				{100, math.Inf(-1)}, {-100, math.Inf(-1)},
				{math.Inf(1), 0}, {math.Inf(-1), 0},
				{0, math.Inf(1)}, {0, math.Inf(-1)},
				{math.NaN(), 0}, {0, math.NaN()},
				{math.NaN(), 12321}, {12313, math.NaN()},
				{math.NaN(), math.NaN()},
			} {
				x1, x2 := vs[0], vs[1]
				t.Run(fmt.Sprintf("x1=%f_x2=%f", x1, x2), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the target values.
					if tc.is32bit {
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(x1)})
						require.NoError(t, err)
						err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(x2)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: x1})
						require.NoError(t, err)
						err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: x2})
						require.NoError(t, err)
					}

					// At this point two values are pushed.
					require.Equal(t, uint64(2), compiler.valueLocationStack().sp)
					require.Len(t, compiler.valueLocationStack().usedRegisters, 2)

					tc.setupFunc(t, compiler)

					// We consumed two values, but push one value after operation.
					require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
					require.Len(t, compiler.valueLocationStack().usedRegisters, 1)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
					require.Equal(t, uint64(1), env.stackPointer()) // Result must be pushed!

					tc.verifyFunc(t, x1, x2, env.stackTopAsUint64())
				})
			}
		})
	}
}

func TestCompiler_compile_Abs_Neg_Ceil_Floor_Trunc_Nearest_Sqrt(t *testing.T) {
	for _, tc := range []struct {
		name       string
		is32bit    bool
		setupFunc  func(t *testing.T, compiler compilerImpl)
		verifyFunc func(t *testing.T, v float64, raw uint64)
	}{
		{
			name:    "abs-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileAbs(&wazeroir.OperationAbs{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Abs(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "abs-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileAbs(&wazeroir.OperationAbs{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Abs(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "neg-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileNeg(&wazeroir.OperationNeg{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := -float32(v)
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "neg-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileNeg(&wazeroir.OperationNeg{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := -v
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "ceil-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileCeil(&wazeroir.OperationCeil{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Ceil(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "ceil-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileCeil(&wazeroir.OperationCeil{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Ceil(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "floor-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileFloor(&wazeroir.OperationFloor{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Floor(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "floor-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileFloor(&wazeroir.OperationFloor{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Floor(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "trunc-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileTrunc(&wazeroir.OperationTrunc{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Trunc(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "trunc-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileTrunc(&wazeroir.OperationTrunc{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Trunc(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "nearest-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileNearest(&wazeroir.OperationNearest{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := moremath.WasmCompatNearestF32(float32(v))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "nearest-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileNearest(&wazeroir.OperationNearest{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := moremath.WasmCompatNearestF64(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "sqrt-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileSqrt(&wazeroir.OperationSqrt{Type: wazeroir.Float32})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := float32(math.Sqrt(float64(v)))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
		{
			name:    "sqrt-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileSqrt(&wazeroir.OperationSqrt{Type: wazeroir.Float64})
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, v float64, raw uint64) {
				exp := math.Sqrt(v)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, exp, actual)
				}
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, v := range []float64{
				0, 1 << 63, 1<<63 | 12345, 1 << 31,
				1<<31 | 123455, 6.8719476736e+10,
				// This verifies that the impl is Wasm compatible in nearest, rather than being equivalent of math.Round.
				// See moremath.WasmCompatNearestF32 and moremath.WasmCompatNearestF64
				-4.5,
				1.37438953472e+11, -1.3,
				-1231.123, 1.3, 100.3, -100.3, 1231.123,
				math.Inf(1), math.Inf(-1), math.NaN(),
			} {
				v := v
				t.Run(fmt.Sprintf("%f", v), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					if tc.is32bit {
						err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: float32(v)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(&wazeroir.OperationConstF64{Value: v})
						require.NoError(t, err)
					}

					// At this point two values are pushed.
					require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
					require.Len(t, compiler.valueLocationStack().usedRegisters, 1)

					tc.setupFunc(t, compiler)

					// We consumed one value, but push the result after operation.
					require.Equal(t, uint64(1), compiler.valueLocationStack().sp)
					require.Len(t, compiler.valueLocationStack().usedRegisters, 1)

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
					require.Equal(t, uint64(1), env.stackPointer()) // Result must be pushed!

					tc.verifyFunc(t, v, env.stackTopAsUint64())
				})
			}
		})
	}
}

func TestCompiler_compile_Div_Rem(t *testing.T) {
	for _, kind := range []wazeroir.OperationKind{
		wazeroir.OperationKindDiv,
		wazeroir.OperationKindRem,
	} {
		kind := kind
		t.Run(kind.String(), func(t *testing.T) {
			for _, signedType := range []wazeroir.SignedType{
				wazeroir.SignedTypeUint32,
				wazeroir.SignedTypeUint64,
				wazeroir.SignedTypeInt32,
				wazeroir.SignedTypeInt64,
				wazeroir.SignedTypeFloat32,
				wazeroir.SignedTypeFloat64,
			} {
				signedType := signedType
				t.Run(signedType.String(), func(t *testing.T) {
					for _, values := range [][2]uint64{
						{0, 0}, {1, 1}, {2, 1}, {100, 1}, {1, 0}, {0, 1}, {math.MaxInt16, math.MaxInt32},
						{1234, 5}, {5, 1234}, {4, 2}, {40, 4}, {123456, 4},
						{1 << 14, 1 << 21}, {1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0}, {0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff}, {1, 0xffff_ffff_ffff_ffff},
						{0x80000000, 0xffffffff},                 // This is equivalent to (-2^31 / -1) and results in overflow for 32-bit signed div.
						{0x8000000000000000, 0xffffffffffffffff}, // This is equivalent to (-2^63 / -1) and results in overflow for 64-bit signed div.
						{0xffffffff /* -1 in signed 32bit */, 0xfffffffe /* -2 in signed 32bit */},
						{0xffffffffffffffff /* -1 in signed 64bit */, 0xfffffffffffffffe /* -2 in signed 64bit */},
						{1, 0xffff_ffff_ffff_ffff},
						{math.Float64bits(1.11231), math.Float64bits(12312312.12312)},
						{math.Float64bits(1.11231), math.Float64bits(-12312312.12312)},
						{math.Float64bits(-1.11231), math.Float64bits(12312312.12312)},
						{math.Float64bits(-1.11231), math.Float64bits(-12312312.12312)},
						{math.Float64bits(1.11231), math.Float64bits(12312312.12312)},
						{math.Float64bits(-12312312.12312), math.Float64bits(1.11231)},
						{math.Float64bits(12312312.12312), math.Float64bits(-1.11231)},
						{math.Float64bits(-12312312.12312), math.Float64bits(-1.11231)},
						{1, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 1},
						{0xffff_ffff_ffff_ffff, math.Float64bits(math.NaN())}, {math.Float64bits(math.NaN()), 0xffff_ffff_ffff_ffff},
						{math.Float64bits(math.MaxFloat32), 1},
						{math.Float64bits(math.SmallestNonzeroFloat32), 1},
						{math.Float64bits(math.MaxFloat64), 1},
						{math.Float64bits(math.SmallestNonzeroFloat64), 1},
						{0, math.Float64bits(math.Inf(1))},
						{0, math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), 0},
						{math.Float64bits(math.Inf(-1)), 0},
						{math.Float64bits(math.Inf(1)), 1},
						{math.Float64bits(math.Inf(-1)), 1},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(1))},
						{math.Float64bits(1.11231), math.Float64bits(math.Inf(-1))},
						{math.Float64bits(math.Inf(1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(1.11231)},
						{math.Float64bits(math.Inf(1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.Inf(-1)), math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(1))},
						{math.Float64bits(math.NaN()), math.Float64bits(math.Inf(-1))},
					} {
						x1, x2 := values[0], values[1]
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newJITEnvironment()
							compiler := env.requireNewCompiler(t, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch signedType {
								case wazeroir.SignedTypeUint32:
									// In order to test zero value on non-zero register, we directly assign an register.
									loc := compiler.valueLocationStack().pushValueLocationOnStack()
									err = compiler.compileEnsureOnGeneralPurposeRegister(loc)
									require.NoError(t, err)
									env.stack()[loc.stackPointer] = uint64(v)
								case wazeroir.SignedTypeInt32:
									err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(int32(v))})
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									err = compiler.compileConstI64(&wazeroir.OperationConstI64{Value: v})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileConstF64(&wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist for comparison.
							require.Equal(t, uint64(2), compiler.valueLocationStack().sp)

							switch kind {
							case wazeroir.OperationKindDiv:
								err = compiler.compileDiv(&wazeroir.OperationDiv{Type: signedType})
							case wazeroir.OperationKindRem:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt32})
								case wazeroir.SignedTypeInt64:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedInt64})
								case wazeroir.SignedTypeUint32:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint32})
								case wazeroir.SignedTypeUint64:
									err = compiler.compileRem(&wazeroir.OperationRem{Type: wazeroir.SignedUint64})
								case wazeroir.SignedTypeFloat32:
									// Rem undefined for float32.
									t.Skip()
								case wazeroir.SignedTypeFloat64:
									// Rem undefined for float64.
									t.Skip()
								}
							}
							require.NoError(t, err)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							switch kind {
							case wazeroir.OperationKindDiv:
								switch signedType {
								case wazeroir.SignedTypeUint32:
									if uint32(x2) == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, uint32(x1)/uint32(x2), env.stackTopAsUint32())
									}
								case wazeroir.SignedTypeInt32:
									v1, v2 := int32(x1), int32(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else if v1 == math.MinInt32 && v2 == -1 {
										require.Equal(t, jitCallStatusIntegerOverflow, env.jitStatus())
									} else {
										require.Equal(t, v1/v2, env.stackTopAsInt32())
									}
								case wazeroir.SignedTypeUint64:
									if x2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, x1/x2, env.stackTopAsUint64())
									}
								case wazeroir.SignedTypeInt64:
									v1, v2 := int64(x1), int64(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else if v1 == math.MinInt64 && v2 == -1 {
										require.Equal(t, jitCallStatusIntegerOverflow, env.jitStatus())
									} else {
										require.Equal(t, v1/v2, env.stackTopAsInt64())
									}
								case wazeroir.SignedTypeFloat32:
									exp := math.Float32frombits(uint32(x1)) / math.Float32frombits(uint32(x2))
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(float64(exp)) {
										require.True(t, math.IsNaN(float64(env.stackTopAsFloat32())))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat32())
									}
								case wazeroir.SignedTypeFloat64:
									exp := math.Float64frombits(x1) / math.Float64frombits(x2)
									// NaN cannot be compared with themselves, so we have to use IsNaN
									if math.IsNaN(exp) {
										require.True(t, math.IsNaN(env.stackTopAsFloat64()))
									} else {
										require.Equal(t, exp, env.stackTopAsFloat64())
									}
								}
							case wazeroir.OperationKindRem:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									v1, v2 := int32(x1), int32(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsInt32())
									}
								case wazeroir.SignedTypeInt64:
									v1, v2 := int64(x1), int64(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsInt64())
									}
								case wazeroir.SignedTypeUint32:
									v1, v2 := uint32(x1), uint32(x2)
									if v2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsUint32())
									}
								case wazeroir.SignedTypeUint64:
									if x2 == 0 {
										require.Equal(t, jitCallStatusIntegerDivisionByZero, env.jitStatus())
									} else {
										require.Equal(t, x1%x2, env.stackTopAsUint64())
									}

								}
							}
						})
					}
				})
			}
		})
	}
}

func TestCompiler_compileBr(t *testing.T) {
	t.Run("return", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Branch into nil label is interpreted as return. See BranchTarget.IsReturnTarget
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: nil}})
		require.NoError(t, err)

		// Compile and execute the code under test.
		// Note: we don't invoke "compiler.return()" as the code emitted by compilerBr is enough to exit.
		code, _, _, err := compiler.compile()
		require.NoError(t, err)
		env.exec(code)

		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
	t.Run("back-and-forth br", func(t *testing.T) {
		env := newJITEnvironment()
		compiler := env.requireNewCompiler(t, nil)
		err := compiler.compilePreamble()
		require.NoError(t, err)

		// Emit the forward br, meaning that handle Br instruction where the target label hasn't been compiled yet.
		forwardLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 0}
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: forwardLabel}})
		require.NoError(t, err)

		// We must not reach the code after Br, so emit the code exiting with Unreachable status.
		compiler.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
		require.NoError(t, err)

		exitLabel := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: exitLabel}})
		require.NoError(t, err)

		// Emit code for the exitLabel.
		skip := compiler.compileLabel(&wazeroir.OperationLabel{Label: exitLabel})
		require.False(t, skip)
		compiler.compileExitFromNativeCode(jitCallStatusCodeReturned)
		require.NoError(t, err)

		// Emit code for the forwardLabel.
		skip = compiler.compileLabel(&wazeroir.OperationLabel{Label: forwardLabel})
		require.False(t, skip)
		err = compiler.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: exitLabel}})
		require.NoError(t, err)

		code, _, _, err := compiler.compile()
		require.NoError(t, err)

		// The generated code looks like this:
		//
		//    ... code from compilePreamble()
		//    br .forwardLabel
		//    exit jitCallStatusCodeUnreachable  // must not be reached
		//    br .exitLabel                      // must not be reached
		// .exitLabel:
		//    exit jitCallStatusCodeReturned
		// .forwardLabel:
		//    br .exitLabel
		//
		// Therefore, if we start executing from the top, we must end up exiting jitCallStatusCodeReturned.
		env.exec(code)
		require.Equal(t, jitCallStatusCodeReturned, env.jitStatus())
	})
}

func TestCompiler_compileBrTable(t *testing.T) {
	requireRunAndExpectedValueReturned := func(t *testing.T, env *jitEnv, c compilerImpl, expValue uint32) {
		// Emit code for each label which returns the frame ID.
		for returnValue := uint32(0); returnValue < 7; returnValue++ {
			label := &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: returnValue}
			err := c.compileBr(&wazeroir.OperationBr{Target: &wazeroir.BranchTarget{Label: label}})
			require.NoError(t, err)
			_ = c.compileLabel(&wazeroir.OperationLabel{Label: label})
			_ = c.compileConstI32(&wazeroir.OperationConstI32{Value: label.FrameID})
			err = c.compileReturnFunction()
			require.NoError(t, err)
		}

		// Generate the code under test and run.
		code, _, _, err := c.compile()
		require.NoError(t, err)
		env.exec(code)

		// Check the returned value.
		require.Equal(t, uint64(1), env.stackPointer())
		require.Equal(t, expValue, env.stackTopAsUint32())
	}

	getBranchTargetDropFromFrameID := func(frameid uint32) *wazeroir.BranchTargetDrop {
		return &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{
			Label: &wazeroir.Label{FrameID: frameid, Kind: wazeroir.LabelKindHeader}},
		}
	}

	for _, tc := range []struct {
		name          string
		index         int64
		o             *wazeroir.OperationBrTable
		expectedValue uint32
	}{
		{
			name:          "only default with index 0",
			o:             &wazeroir.OperationBrTable{Default: getBranchTargetDropFromFrameID(6)},
			index:         0,
			expectedValue: 6,
		},
		{
			name:          "only default with index 100",
			o:             &wazeroir.OperationBrTable{Default: getBranchTargetDropFromFrameID(6)},
			index:         100,
			expectedValue: 6,
		},
		{
			name: "select default with targets and good index",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(6),
			},
			index:         3,
			expectedValue: 6,
		},
		{
			name: "select default with targets and huge index",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(6),
			},
			index:         100000,
			expectedValue: 6,
		},
		{
			name: "select first with two targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         0,
			expectedValue: 1,
		},
		{
			name: "select last with two targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
				},
				Default: getBranchTargetDropFromFrameID(6),
			},
			index:         1,
			expectedValue: 2,
		},
		{
			name: "select first with five targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
					getBranchTargetDropFromFrameID(3),
					getBranchTargetDropFromFrameID(4),
					getBranchTargetDropFromFrameID(5),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         0,
			expectedValue: 1,
		},
		{
			name: "select middle with five targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
					getBranchTargetDropFromFrameID(3),
					getBranchTargetDropFromFrameID(4),
					getBranchTargetDropFromFrameID(5),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         2,
			expectedValue: 3,
		},
		{
			name: "select last with five targets",
			o: &wazeroir.OperationBrTable{
				Targets: []*wazeroir.BranchTargetDrop{
					getBranchTargetDropFromFrameID(1),
					getBranchTargetDropFromFrameID(2),
					getBranchTargetDropFromFrameID(3),
					getBranchTargetDropFromFrameID(4),
					getBranchTargetDropFromFrameID(5),
				},
				Default: getBranchTargetDropFromFrameID(5),
			},
			index:         4,
			expectedValue: 5,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			env := newJITEnvironment()
			compiler := env.requireNewCompiler(t, nil)

			err := compiler.compilePreamble()
			require.NoError(t, err)

			err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: uint32(tc.index)})
			require.NoError(t, err)

			err = compiler.compileBrTable(tc.o)
			require.NoError(t, err)

			require.Len(t, compiler.valueLocationStack().usedRegisters, 0)

			requireRunAndExpectedValueReturned(t, env, compiler, tc.expectedValue)
		})
	}
}

func requirePushTwoInt32Consts(t *testing.T, x1, x2 uint32, compiler compilerImpl) {
	err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstI32(&wazeroir.OperationConstI32{Value: x2})
	require.NoError(t, err)
}

func requirePushTwoFloat32Consts(t *testing.T, x1, x2 float32, compiler compilerImpl) {
	err := compiler.compileConstF32(&wazeroir.OperationConstF32{Value: x1})
	require.NoError(t, err)
	err = compiler.compileConstF32(&wazeroir.OperationConstF32{Value: x2})
	require.NoError(t, err)
}

func TestCompiler_compileBrIf(t *testing.T) {
	unreachableStatus, thenLabelExitStatus, elseLabelExitStatus :=
		jitCallStatusCodeUnreachable, jitCallStatusCodeUnreachable+1, jitCallStatusCodeUnreachable+2
	thenBranchTarget := &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{Label: &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 1}}}
	elseBranchTarget := &wazeroir.BranchTargetDrop{Target: &wazeroir.BranchTarget{Label: &wazeroir.Label{Kind: wazeroir.LabelKindHeader, FrameID: 2}}}

	for _, tc := range []struct {
		name      string
		setupFunc func(t *testing.T, compiler compilerImpl, shouldGoElse bool)
	}{
		{
			name: "cond on register",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				val := uint32(1)
				if shouldGoElse {
					val = 0
				}
				err := compiler.compileConstI32(&wazeroir.OperationConstI32{Value: val})
				require.NoError(t, err)
			},
		},
		{
			name: "LS",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Le on unsigned integer produces the value on COND_LS register.
				err := compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "LE",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Le on signed integer produces the value on COND_LE register.
				err := compiler.compileLe(&wazeroir.OperationLe{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "HS",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Ge on unsigned integer produces the value on COND_HS register.
				err := compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "GE",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Ge on signed integer produces the value on COND_GE register.
				err := compiler.compileGe(&wazeroir.OperationGe{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "HI",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Gt on unsigned integer produces the value on COND_HI register.
				err := compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "GT",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(2), uint32(1)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Gt on signed integer produces the value on COND_GT register.
				err := compiler.compileGt(&wazeroir.OperationGt{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "LO",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Lt on unsigned integer produces the value on COND_LO register.
				err := compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeUint32})
				require.NoError(t, err)
			},
		},
		{
			name: "LT",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				// Lt on signed integer produces the value on COND_LT register.
				err := compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeInt32})
				require.NoError(t, err)
			},
		},
		{
			name: "MI",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := float32(1), float32(2)
				if shouldGoElse {
					x2, x1 = x1, x2
				}
				requirePushTwoFloat32Consts(t, x1, x2, compiler)
				// Lt on floats produces the value on COND_MI register.
				err := compiler.compileLt(&wazeroir.OperationLt{Type: wazeroir.SignedTypeFloat32})
				require.NoError(t, err)
			},
		},
		{
			name: "EQ",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(1)
				if shouldGoElse {
					x2++
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				err := compiler.compileEq(&wazeroir.OperationEq{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
		},
		{
			name: "NE",
			setupFunc: func(t *testing.T, compiler compilerImpl, shouldGoElse bool) {
				x1, x2 := uint32(1), uint32(2)
				if shouldGoElse {
					x2 = x1
				}
				requirePushTwoInt32Consts(t, x1, x2, compiler)
				err := compiler.compileNe(&wazeroir.OperationNe{Type: wazeroir.UnsignedTypeI32})
				require.NoError(t, err)
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			for _, shouldGoToElse := range []bool{false, true} {
				shouldGoToElse := shouldGoToElse
				t.Run(fmt.Sprintf("should_goto_else=%v", shouldGoToElse), func(t *testing.T) {
					env := newJITEnvironment()
					compiler := env.requireNewCompiler(t, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					tc.setupFunc(t, compiler, shouldGoToElse)
					require.Equal(t, uint64(1), compiler.valueLocationStack().sp)

					err = compiler.compileBrIf(&wazeroir.OperationBrIf{Then: thenBranchTarget, Else: elseBranchTarget})
					require.NoError(t, err)
					compiler.compileExitFromNativeCode(unreachableStatus)

					// Emit code for .then label.
					skip := compiler.compileLabel(&wazeroir.OperationLabel{Label: thenBranchTarget.Target.Label})
					require.False(t, skip)
					compiler.compileExitFromNativeCode(thenLabelExitStatus)

					// Emit code for .else label.
					skip = compiler.compileLabel(&wazeroir.OperationLabel{Label: elseBranchTarget.Target.Label})
					require.False(t, skip)
					compiler.compileExitFromNativeCode(elseLabelExitStatus)

					code, _, _, err := compiler.compile()
					require.NoError(t, err)

					// The generated code looks like this:
					//
					//    ... code from compilePreamble()
					//    ... code from tc.setupFunc()
					//    br_if .then, .else
					//    exit $unreachableStatus
					// .then:
					//    exit $thenLabelExitStatus
					// .else:
					//    exit $elseLabelExitStatus
					//
					// Therefore, if we start executing from the top, we must end up exiting with an appropriate status.
					env.exec(code)
					require.NotEqual(t, unreachableStatus, env.jitStatus())
					if shouldGoToElse {
						require.Equal(t, elseLabelExitStatus, env.jitStatus())
					} else {
						require.Equal(t, thenLabelExitStatus, env.jitStatus())
					}
				})
			}
		})
	}
}
