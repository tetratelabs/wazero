package compiler

import (
	"fmt"
	"math"
	"math/bits"
	"testing"

	"github.com/tetratelabs/wazero/internal/moremath"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func TestCompiler_compileConsts(t *testing.T) {
	for _, op := range []wazeroir.OperationKind{
		wazeroir.OperationKindConstI32,
		wazeroir.OperationKindConstI64,
		wazeroir.OperationKindConstF32,
		wazeroir.OperationKindConstF64,
		wazeroir.OperationKindV128Const,
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
					env := newCompilerEnvironment()

					// Compile code.
					compiler := env.requireNewCompiler(t, newCompiler, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					switch op {
					case wazeroir.OperationKindConstI32:
						err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(val)})
					case wazeroir.OperationKindConstI64:
						err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: val})
					case wazeroir.OperationKindConstF32:
						err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(val))})
					case wazeroir.OperationKindConstF64:
						err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: math.Float64frombits(val)})
					case wazeroir.OperationKindV128Const:
						err = compiler.compileV128Const(wazeroir.OperationV128Const{Lo: val, Hi: ^val})
					}
					require.NoError(t, err)

					// After compiling const operations, we must see the register allocated value on the top of value.
					loc := compiler.runtimeValueLocationStack().peek()
					require.True(t, loc.onRegister())

					if op == wazeroir.OperationKindV128Const {
						require.Equal(t, runtimeValueTypeV128Hi, loc.valueType)
					}

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate the code under test.
					code, _, err := compiler.compile()
					require.NoError(t, err)

					// Run native code.
					env.exec(code)

					// Compiler status must be returned.
					require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
					if op == wazeroir.OperationKindV128Const {
						require.Equal(t, uint64(2), env.stackPointer()) // a vector value consists of two uint64.
					} else {
						require.Equal(t, uint64(1), env.stackPointer())
					}

					switch op {
					case wazeroir.OperationKindConstI32, wazeroir.OperationKindConstF32:
						require.Equal(t, uint32(val), env.stackTopAsUint32())
					case wazeroir.OperationKindConstI64, wazeroir.OperationKindConstF64:
						require.Equal(t, val, env.stackTopAsUint64())
					case wazeroir.OperationKindV128Const:
						lo, hi := env.stackTopAsV128()
						require.Equal(t, val, lo)
						require.Equal(t, ^val, hi)
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
						{0, 0},
						{1, 1},
						{2, 1},
						{100, 1},
						{1, 0},
						{0, 1},
						{math.MaxInt16, math.MaxInt32},
						{1 << 14, 1 << 21},
						{1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0},
						{0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff},
						{1, 0xffff_ffff_ffff_ffff},
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
							env := newCompilerEnvironment()
							compiler := env.requireNewCompiler(t, newCompiler, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch unsignedType {
								case wazeroir.UnsignedTypeI32:
									err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.UnsignedTypeI64:
									err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: v})
								case wazeroir.UnsignedTypeF32:
									err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.UnsignedTypeF64:
									err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist.
							requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindAdd:
								err = compiler.compileAdd(wazeroir.NewOperationAdd(unsignedType))
							case wazeroir.OperationKindSub:
								err = compiler.compileSub(wazeroir.NewOperationSub(unsignedType))
							case wazeroir.OperationKindMul:
								err = compiler.compileMul(wazeroir.NewOperationMul(unsignedType))
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
							resultLocation := compiler.runtimeValueLocationStack().peek()
							// Plus the result must be located on a register.
							require.True(t, resultLocation.onRegister())
							// Also, the result must have an appropriate register type.
							if unsignedType == wazeroir.UnsignedTypeF32 || unsignedType == wazeroir.UnsignedTypeF64 {
								require.Equal(t, registerTypeVector, resultLocation.getRegisterType())
							} else {
								require.Equal(t, registerTypeGeneralPurpose, resultLocation.getRegisterType())
							}

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, err := compiler.compile()
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
						{0, 0},
						{0, 1},
						{1, 0},
						{1, 1},
						{1 << 31, 1},
						{1, 1 << 31},
						{1 << 31, 1 << 31},
						{1 << 63, 1},
						{1, 1 << 63},
						{1 << 63, 1 << 63},
					} {
						x1, x2 := values[0], values[1]
						for _, x1OnRegister := range []bool{
							true, false,
						} {
							x1OnRegister := x1OnRegister
							t.Run(fmt.Sprintf("x1=0x%x(on_register=%v),x2=0x%x", x1, x1OnRegister, x2), func(t *testing.T) {
								env := newCompilerEnvironment()
								compiler := env.requireNewCompiler(t, newCompiler, nil)
								err := compiler.compilePreamble()
								require.NoError(t, err)

								// Emit consts operands.
								var x1Location *runtimeValueLocation
								switch unsignedInt {
								case wazeroir.UnsignedInt32:
									err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(x1)})
									require.NoError(t, err)
									x1Location = compiler.runtimeValueLocationStack().peek()
									err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: x2})
									require.NoError(t, err)
								case wazeroir.UnsignedInt64:
									err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: x1})
									require.NoError(t, err)
									x1Location = compiler.runtimeValueLocationStack().peek()
									err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: x2})
									require.NoError(t, err)
								}

								if !x1OnRegister {
									compiler.compileReleaseRegisterToStack(x1Location)
								}

								// At this point, two values exist.
								requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)

								// Emit the operation.
								switch kind {
								case wazeroir.OperationKindAnd:
									err = compiler.compileAnd(wazeroir.NewOperationAnd(unsignedInt))
								case wazeroir.OperationKindOr:
									err = compiler.compileOr(wazeroir.NewOperationOr(unsignedInt))
								case wazeroir.OperationKindXor:
									err = compiler.compileXor(wazeroir.NewOperationXor(unsignedInt))
								case wazeroir.OperationKindShl:
									err = compiler.compileShl(wazeroir.NewOperationShl(unsignedInt))
								case wazeroir.OperationKindRotl:
									err = compiler.compileRotl(wazeroir.NewOperationRotl(unsignedInt))
								case wazeroir.OperationKindRotr:
									err = compiler.compileRotr(wazeroir.NewOperationRotr(unsignedInt))
								}
								require.NoError(t, err)

								// We consumed two values, but push the result back.
								requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
								resultLocation := compiler.runtimeValueLocationStack().peek()
								// Also, the result must have an appropriate register type.
								require.Equal(t, registerTypeGeneralPurpose, resultLocation.getRegisterType())

								err = compiler.compileReturnFunction()
								require.NoError(t, err)

								// Compile and execute the code under test.
								code, _, err := compiler.compile()
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
					{0, 0},
					{0, 1},
					{1, 0},
					{1, 1},
					{1 << 31, 1},
					{1, 1 << 31},
					{1 << 31, 1 << 31},
					{1 << 63, 1},
					{1, 1 << 63},
					{1 << 63, 1 << 63},
				} {
					x1, x2 := values[0], values[1]
					t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
						env := newCompilerEnvironment()
						compiler := env.requireNewCompiler(t, newCompiler, nil)
						err := compiler.compilePreamble()
						require.NoError(t, err)

						// Emit consts operands.
						for _, v := range []uint64{x1, x2} {
							switch signedInt {
							case wazeroir.SignedInt32:
								err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(int32(v))})
							case wazeroir.SignedInt64:
								err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: v})
							case wazeroir.SignedUint32:
								err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(v)})
							case wazeroir.SignedUint64:
								err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: v})
							}
							require.NoError(t, err)
						}

						// At this point, two values exist.
						requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)

						// Emit the operation.
						err = compiler.compileShr(wazeroir.NewOperationShr(signedInt))
						require.NoError(t, err)

						// We consumed two values, but push the result back.
						requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
						resultLocation := compiler.runtimeValueLocationStack().peek()
						// Plus the result must be located on a register.
						require.True(t, resultLocation.onRegister())
						// Also, the result must have an appropriate register type.
						require.Equal(t, registerTypeGeneralPurpose, resultLocation.getRegisterType())

						err = compiler.compileReturnFunction()
						require.NoError(t, err)

						// Compile and execute the code under test.
						code, _, err := compiler.compile()
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
						{0, 0},
						{1, 1},
						{2, 1},
						{100, 1},
						{1, 0},
						{0, 1},
						{math.MaxInt16, math.MaxInt32},
						{1 << 14, 1 << 21},
						{1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0},
						{0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff},
						{1, 0xffff_ffff_ffff_ffff},
						{1, math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), 1},
						{0xffff_ffff_ffff_ffff, math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), 0xffff_ffff_ffff_ffff},
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
							return
						}
						t.Run(fmt.Sprintf("x1=0x%x,x2=0x%x", x1, x2), func(t *testing.T) {
							env := newCompilerEnvironment()
							compiler := env.requireNewCompiler(t, newCompiler, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch signedType {
								case wazeroir.SignedTypeUint32:
									err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(v)})
								case wazeroir.SignedTypeInt32:
									err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(int32(v))})
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: v})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							if isEqz {
								// Eqz only needs one value, so pop the top one (x2).
								compiler.runtimeValueLocationStack().pop()
								requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
							} else {
								// At this point, two values exist for comparison.
								requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)
							}

							// Emit the operation.
							switch kind {
							case wazeroir.OperationKindLe:
								err = compiler.compileLe(wazeroir.NewOperationLe(signedType))
							case wazeroir.OperationKindLt:
								err = compiler.compileLt(wazeroir.NewOperationLt(signedType))
							case wazeroir.OperationKindGe:
								err = compiler.compileGe(wazeroir.NewOperationGe(signedType))
							case wazeroir.OperationKindGt:
								err = compiler.compileGt(wazeroir.NewOperationGt(signedType))
							case wazeroir.OperationKindEq:
								// Eq uses UnsignedType instead, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeI32))
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeI64))
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeF32))
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileEq(wazeroir.NewOperationEq(wazeroir.UnsignedTypeF64))
								}
							case wazeroir.OperationKindNe:
								// Ne uses UnsignedType, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileNe(wazeroir.NewOperationNe(wazeroir.UnsignedTypeI32))
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileNe(wazeroir.NewOperationNe(wazeroir.UnsignedTypeI64))
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileNe(wazeroir.NewOperationNe(wazeroir.UnsignedTypeF32))
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileNe(wazeroir.NewOperationNe(wazeroir.UnsignedTypeF64))
								}
							case wazeroir.OperationKindEqz:
								// Eqz uses UnsignedInt, so we translate the signed one.
								switch signedType {
								case wazeroir.SignedTypeUint32, wazeroir.SignedTypeInt32:
									err = compiler.compileEqz(wazeroir.NewOperationEqz(wazeroir.UnsignedInt32))
								case wazeroir.SignedTypeUint64, wazeroir.SignedTypeInt64:
									err = compiler.compileEqz(wazeroir.NewOperationEqz(wazeroir.UnsignedInt64))
								}
							}
							require.NoError(t, err)

							// We consumed two values, but push the result back.
							requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, err := compiler.compile()
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
							env := newCompilerEnvironment()
							compiler := env.requireNewCompiler(t, newCompiler, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							if is32bit {
								err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(v)})
							} else {
								err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: v})
							}
							require.NoError(t, err)

							switch kind {
							case wazeroir.OperationKindClz:
								err = compiler.compileClz(wazeroir.NewOperationClz(tp))
							case wazeroir.OperationKindCtz:
								err = compiler.compileCtz(wazeroir.NewOperationCtz(tp))
							case wazeroir.OperationKindPopcnt:
								err = compiler.compilePopcnt(wazeroir.NewOperationPopcnt(tp))
							}
							require.NoError(t, err)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Generate and run the code under test.
							code, _, err := compiler.compile()
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

func TestCompiler_compile_Min_Max_Copysign(t *testing.T) {
	tests := []struct {
		name       string
		is32bit    bool
		setupFunc  func(t *testing.T, compiler compilerImpl)
		verifyFunc func(t *testing.T, x1, x2 float64, raw uint64)
	}{
		{
			name:    "min-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMin(wazeroir.NewOperationMin(wazeroir.Float32))
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMin32(float32(x1), float32(x2))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, math.Float32bits(exp), math.Float32bits(actual))
				}
			},
		},
		{
			name:    "min-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMin(wazeroir.NewOperationMin(wazeroir.Float64))
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMin64(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, math.Float64bits(exp), math.Float64bits(actual))
				}
			},
		},
		{
			name:    "max-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMax(wazeroir.NewOperationMax(wazeroir.Float32))
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMax32(float32(x1), float32(x2))
				actual := math.Float32frombits(uint32(raw))
				if math.IsNaN(float64(exp)) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(float64(actual)))
				} else {
					require.Equal(t, math.Float32bits(exp), math.Float32bits(actual))
				}
			},
		},
		{
			name:    "max-64-bit",
			is32bit: false,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileMax(wazeroir.NewOperationMax(wazeroir.Float64))
				require.NoError(t, err)
			},
			verifyFunc: func(t *testing.T, x1, x2 float64, raw uint64) {
				exp := moremath.WasmCompatMax64(x1, x2)
				actual := math.Float64frombits(raw)
				if math.IsNaN(exp) { // NaN cannot be compared with themselves, so we have to use IsNaN
					require.True(t, math.IsNaN(actual))
				} else {
					require.Equal(t, math.Float64bits(exp), math.Float64bits(actual))
				}
			},
		},
		{
			name:    "copysign-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileCopysign(wazeroir.NewOperationCopysign(wazeroir.Float32))
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
				err := compiler.compileCopysign(wazeroir.NewOperationCopysign(wazeroir.Float64))
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
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			for _, vs := range [][2]float64{
				{math.Copysign(0, 1), math.Copysign(0, 1)},
				{math.Copysign(0, -1), math.Copysign(0, 1)},
				{math.Copysign(0, 1), math.Copysign(0, -1)},
				{math.Copysign(0, -1), math.Copysign(0, -1)},
				{100, -1.1},
				{100, 0},
				{0, 0},
				{1, 1},
				{-1, 100},
				{100, 200},
				{100.01234124, 100.01234124},
				{100.01234124, -100.01234124},
				{200.12315, 100},
				{6.8719476736e+10 /* = 1 << 36 */, 100},
				{6.8719476736e+10 /* = 1 << 36 */, 1.37438953472e+11 /* = 1 << 37*/},
				{math.Inf(1), 100},
				{math.Inf(1), -100},
				{100, math.Inf(1)},
				{-100, math.Inf(1)},
				{math.Inf(-1), 100},
				{math.Inf(-1), -100},
				{100, math.Inf(-1)},
				{-100, math.Inf(-1)},
				{math.Inf(1), 0},
				{math.Inf(-1), 0},
				{0, math.Inf(1)},
				{0, math.Inf(-1)},
				{math.NaN(), 0},
				{0, math.NaN()},
				{math.NaN(), 12321},
				{12313, math.NaN()},
				{math.NaN(), math.NaN()},
			} {
				x1, x2 := vs[0], vs[1]
				t.Run(fmt.Sprintf("x1=%f_x2=%f", x1, x2), func(t *testing.T) {
					env := newCompilerEnvironment()
					compiler := env.requireNewCompiler(t, newCompiler, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					// Setup the target values.
					if tc.is32bit {
						err := compiler.compileConstF32(wazeroir.OperationConstF32{Value: float32(x1)})
						require.NoError(t, err)
						err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: float32(x2)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(wazeroir.OperationConstF64{Value: x1})
						require.NoError(t, err)
						err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: x2})
						require.NoError(t, err)
					}

					// At this point two values are pushed.
					requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)
					require.Equal(t, 2, len(compiler.runtimeValueLocationStack().usedRegisters))

					tc.setupFunc(t, compiler)

					// We consumed two values, but push one value after operation.
					requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
					require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
					require.Equal(t, uint64(1), env.stackPointer()) // Result must be pushed!

					tc.verifyFunc(t, x1, x2, env.stackTopAsUint64())
				})
			}
		})
	}
}

func TestCompiler_compile_Abs_Neg_Ceil_Floor_Trunc_Nearest_Sqrt(t *testing.T) {
	tests := []struct {
		name       string
		is32bit    bool
		setupFunc  func(t *testing.T, compiler compilerImpl)
		verifyFunc func(t *testing.T, v float64, raw uint64)
	}{
		{
			name:    "abs-32-bit",
			is32bit: true,
			setupFunc: func(t *testing.T, compiler compilerImpl) {
				err := compiler.compileAbs(wazeroir.NewOperationAbs(wazeroir.Float32))
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
				err := compiler.compileAbs(wazeroir.NewOperationAbs(wazeroir.Float64))
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
				err := compiler.compileNeg(wazeroir.NewOperationNeg(wazeroir.Float32))
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
				err := compiler.compileNeg(wazeroir.NewOperationNeg(wazeroir.Float64))
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
				err := compiler.compileCeil(wazeroir.NewOperationCeil(wazeroir.Float32))
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
				err := compiler.compileCeil(wazeroir.NewOperationCeil(wazeroir.Float64))
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
				err := compiler.compileFloor(wazeroir.NewOperationFloor(wazeroir.Float32))
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
				err := compiler.compileFloor(wazeroir.NewOperationFloor(wazeroir.Float64))
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
				err := compiler.compileTrunc(wazeroir.NewOperationTrunc(wazeroir.Float32))
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
				err := compiler.compileTrunc(wazeroir.NewOperationTrunc(wazeroir.Float64))
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
				err := compiler.compileNearest(wazeroir.NewOperationNearest(wazeroir.Float32))
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
				err := compiler.compileNearest(wazeroir.NewOperationNearest(wazeroir.Float64))
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
				err := compiler.compileSqrt(wazeroir.NewOperationSqrt(wazeroir.Float32))
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
				err := compiler.compileSqrt(wazeroir.NewOperationSqrt(wazeroir.Float64))
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
	}

	for _, tt := range tests {
		tc := tt
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
					env := newCompilerEnvironment()
					compiler := env.requireNewCompiler(t, newCompiler, nil)
					err := compiler.compilePreamble()
					require.NoError(t, err)

					if tc.is32bit {
						err := compiler.compileConstF32(wazeroir.OperationConstF32{Value: float32(v)})
						require.NoError(t, err)
					} else {
						err := compiler.compileConstF64(wazeroir.OperationConstF64{Value: v})
						require.NoError(t, err)
					}

					// At this point two values are pushed.
					requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
					require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

					tc.setupFunc(t, compiler)

					// We consumed one value, but push the result after operation.
					requireRuntimeLocationStackPointerEqual(t, uint64(1), compiler)
					require.Equal(t, 1, len(compiler.runtimeValueLocationStack().usedRegisters))

					err = compiler.compileReturnFunction()
					require.NoError(t, err)

					// Generate and run the code under test.
					code, _, err := compiler.compile()
					require.NoError(t, err)
					env.exec(code)

					require.Equal(t, nativeCallStatusCodeReturned, env.compilerStatus())
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
						{0, 0},
						{1, 1},
						{2, 1},
						{100, 1},
						{1, 0},
						{0, 1},
						{math.MaxInt16, math.MaxInt32},
						{1234, 5},
						{5, 1234},
						{4, 2},
						{40, 4},
						{123456, 4},
						{1 << 14, 1 << 21},
						{1 << 14, 1 << 21},
						{0xffff_ffff_ffff_ffff, 0},
						{0xffff_ffff_ffff_ffff, 1},
						{0, 0xffff_ffff_ffff_ffff},
						{1, 0xffff_ffff_ffff_ffff},
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
						{1, math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), 1},
						{0xffff_ffff_ffff_ffff, math.Float64bits(math.NaN())},
						{math.Float64bits(math.NaN()), 0xffff_ffff_ffff_ffff},
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
							env := newCompilerEnvironment()
							compiler := env.requireNewCompiler(t, newCompiler, nil)
							err := compiler.compilePreamble()
							require.NoError(t, err)

							// Emit consts operands.
							for _, v := range []uint64{x1, x2} {
								switch signedType {
								case wazeroir.SignedTypeUint32:
									// In order to test zero value on non-zero register, we directly assign an register.
									loc := compiler.runtimeValueLocationStack().pushRuntimeValueLocationOnStack()
									loc.valueType = runtimeValueTypeI32
									err = compiler.compileEnsureOnRegister(loc)
									require.NoError(t, err)
									env.stack()[loc.stackPointer] = uint64(v)
								case wazeroir.SignedTypeInt32:
									err = compiler.compileConstI32(wazeroir.OperationConstI32{Value: uint32(int32(v))})
								case wazeroir.SignedTypeInt64, wazeroir.SignedTypeUint64:
									err = compiler.compileConstI64(wazeroir.OperationConstI64{Value: v})
								case wazeroir.SignedTypeFloat32:
									err = compiler.compileConstF32(wazeroir.OperationConstF32{Value: math.Float32frombits(uint32(v))})
								case wazeroir.SignedTypeFloat64:
									err = compiler.compileConstF64(wazeroir.OperationConstF64{Value: math.Float64frombits(v)})
								}
								require.NoError(t, err)
							}

							// At this point, two values exist for comparison.
							requireRuntimeLocationStackPointerEqual(t, uint64(2), compiler)

							switch kind {
							case wazeroir.OperationKindDiv:
								err = compiler.compileDiv(wazeroir.NewOperationDiv(signedType))
							case wazeroir.OperationKindRem:
								switch signedType {
								case wazeroir.SignedTypeInt32:
									err = compiler.compileRem(wazeroir.NewOperationRem(wazeroir.SignedInt32))
								case wazeroir.SignedTypeInt64:
									err = compiler.compileRem(wazeroir.NewOperationRem(wazeroir.SignedInt64))
								case wazeroir.SignedTypeUint32:
									err = compiler.compileRem(wazeroir.NewOperationRem(wazeroir.SignedUint32))
								case wazeroir.SignedTypeUint64:
									err = compiler.compileRem(wazeroir.NewOperationRem(wazeroir.SignedUint64))
								case wazeroir.SignedTypeFloat32:
									// Rem undefined for float32.
									return
								case wazeroir.SignedTypeFloat64:
									// Rem undefined for float64.
									return
								}
							}
							require.NoError(t, err)

							err = compiler.compileReturnFunction()
							require.NoError(t, err)

							// Compile and execute the code under test.
							code, _, err := compiler.compile()
							require.NoError(t, err)
							env.exec(code)

							switch kind {
							case wazeroir.OperationKindDiv:
								switch signedType {
								case wazeroir.SignedTypeUint32:
									if uint32(x2) == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else {
										require.Equal(t, uint32(x1)/uint32(x2), env.stackTopAsUint32())
									}
								case wazeroir.SignedTypeInt32:
									v1, v2 := int32(x1), int32(x2)
									if v2 == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else if v1 == math.MinInt32 && v2 == -1 {
										require.Equal(t, nativeCallStatusIntegerOverflow, env.compilerStatus())
									} else {
										require.Equal(t, v1/v2, env.stackTopAsInt32())
									}
								case wazeroir.SignedTypeUint64:
									if x2 == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else {
										require.Equal(t, x1/x2, env.stackTopAsUint64())
									}
								case wazeroir.SignedTypeInt64:
									v1, v2 := int64(x1), int64(x2)
									if v2 == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else if v1 == math.MinInt64 && v2 == -1 {
										require.Equal(t, nativeCallStatusIntegerOverflow, env.compilerStatus())
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
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsInt32())
									}
								case wazeroir.SignedTypeInt64:
									v1, v2 := int64(x1), int64(x2)
									if v2 == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsInt64())
									}
								case wazeroir.SignedTypeUint32:
									v1, v2 := uint32(x1), uint32(x2)
									if v2 == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
									} else {
										require.Equal(t, v1%v2, env.stackTopAsUint32())
									}
								case wazeroir.SignedTypeUint64:
									if x2 == 0 {
										require.Equal(t, nativeCallStatusIntegerDivisionByZero, env.compilerStatus())
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
