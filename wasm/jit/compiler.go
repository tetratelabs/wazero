package jit

import "github.com/tetratelabs/wazero/wasm/wazeroir"

// compiler is the interface of architecture-specific native code compiler,
// and this is responsible for compiling native code for all wazeroir operations.
type compiler interface {
	String() string
	// emitPreamble is called before compiling any wazeroir operation.
	// This is used, for example, to initilize the reserved registers, etc.
	emitPreamble()
	// Generates the byte slice of native codes.
	// maxStackPointer is the max stack pointer that the target function would reach.
	generate() (code []byte, maxStackPointer uint64, err error)
	// Followings are resinposible for compiling each wazeroir operation.
	compileUnreachable()
	compileSwap(o *wazeroir.OperationSwap) error
	compileGlobalGet(o *wazeroir.OperationGlobalGet) error
	compileGlobalSet(o *wazeroir.OperationGlobalSet) error
	compileBr(o *wazeroir.OperationBr) error
	compileBrIf(o *wazeroir.OperationBrIf) error
	compileLabel(o *wazeroir.OperationLabel) error
	compileCall(o *wazeroir.OperationCall) error
	compileDrop(o *wazeroir.OperationDrop) error
	compileSelect() error
	compilePick(o *wazeroir.OperationPick) error
	compileAdd(o *wazeroir.OperationAdd) error
	compileSub(o *wazeroir.OperationSub) error
	compileMul(o *wazeroir.OperationMul) error
	compileClz(o *wazeroir.OperationClz) error
	compileCtz(o *wazeroir.OperationCtz) error
	compilePopcnt(o *wazeroir.OperationPopcnt) error
	compileDiv(o *wazeroir.OperationDiv) error
	compileRem(o *wazeroir.OperationRem) error
	compileAnd(o *wazeroir.OperationAnd) error
	compileOr(o *wazeroir.OperationOr) error
	compileXor(o *wazeroir.OperationXor) error
	compileShl(o *wazeroir.OperationShl) error
	compileShr(o *wazeroir.OperationShr) error
	compileRotl(o *wazeroir.OperationRotl) error
	compileRotr(o *wazeroir.OperationRotr) error
	compileAbs(o *wazeroir.OperationAbs) error
	compileNeg(o *wazeroir.OperationNeg) error
	compileCeil(o *wazeroir.OperationCeil) error
	compileFloor(o *wazeroir.OperationFloor) error
	compileTrunc(o *wazeroir.OperationTrunc) error
	compileNearest(o *wazeroir.OperationNearest) error
	compileSqrt(o *wazeroir.OperationSqrt) error
	compileMin(o *wazeroir.OperationMin) error
	compileMax(o *wazeroir.OperationMax) error
	compileCopysign(o *wazeroir.OperationCopysign) error
	compileI32WrapFromI64() error
	compileITruncFromF(o *wazeroir.OperationITruncFromF) error
	compileFConvertFromI(o *wazeroir.OperationFConvertFromI) error
	compileF32DemoteFromF64() error
	compileF64PromoteFromF32() error
	compileI32ReinterpretFromF32() error
	compileI64ReinterpretFromF64() error
	compileF32ReinterpretFromI32() error
	compileF64ReinterpretFromI64() error
	compileExtend(o *wazeroir.OperationExtend) error
	compileEq(o *wazeroir.OperationEq) error
	compileNe(o *wazeroir.OperationNe) error
	compileEqz(o *wazeroir.OperationEqz) error
	compileLt(o *wazeroir.OperationLt) error
	compileGt(o *wazeroir.OperationGt) error
	compileLe(o *wazeroir.OperationLe) error
	compileGe(o *wazeroir.OperationGe) error
	compileLoad(o *wazeroir.OperationLoad) error
	compileLoad8(o *wazeroir.OperationLoad8) error
	compileLoad16(o *wazeroir.OperationLoad16) error
	compileLoad32(o *wazeroir.OperationLoad32) error
	compileStore(o *wazeroir.OperationStore) error
	compileStore8(o *wazeroir.OperationStore8) error
	compileStore16(o *wazeroir.OperationStore16) error
	compileStore32(o *wazeroir.OperationStore32) error
	compileMemoryGrow() error
	compileMemorySize() error
	compileConstI32(o *wazeroir.OperationConstI32) error
	compileConstI64(o *wazeroir.OperationConstI64) error
	compileConstF32(o *wazeroir.OperationConstF32) error
	compileConstF64(o *wazeroir.OperationConstF64) error
}
