//go:build arm64
// +build arm64

package jit

import (
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
)

// jitcall is implemented in jit_arm64.s as a Go Assembler function.
// This is used by engine.exec and the entrypoint to enter the JITed native code.
// codeSegment is the pointer to the initial instruction of the compiled native code.
// engine is the pointer to the "*engine" as uintptr.
func jitcall(codeSegment, engine uintptr)

// newCompiler returns a new compiler interface which can be used to compile the given function instance.
// Note: ir param can be nil for host functions.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	return &arm64Copmiler{}, nil
}

type arm64Copmiler struct{}

func (c *arm64Copmiler) String() (ret string) { return }

func (c *arm64Copmiler) emitPreamble() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) generate() (code []byte, staticData compiledFunctionStaticData, maxStackPointer uint64, err error) {
	return nil, nil, 0, fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileHostFunction(address wasm.FunctionAddress) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLabel(o *wazeroir.OperationLabel) (skipThisLabel bool) {
	return false
}

func (c *arm64Copmiler) compileUnreachable() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileSwap(o *wazeroir.OperationSwap) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileGlobalGet(o *wazeroir.OperationGlobalGet) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileGlobalSet(o *wazeroir.OperationGlobalSet) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileBr(o *wazeroir.OperationBr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileBrIf(o *wazeroir.OperationBrIf) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileBrTable(o *wazeroir.OperationBrTable) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileCall(o *wazeroir.OperationCall) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileDrop(o *wazeroir.OperationDrop) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileSelect() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compilePick(o *wazeroir.OperationPick) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileAdd(o *wazeroir.OperationAdd) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileSub(o *wazeroir.OperationSub) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileMul(o *wazeroir.OperationMul) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileClz(o *wazeroir.OperationClz) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileCtz(o *wazeroir.OperationCtz) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compilePopcnt(o *wazeroir.OperationPopcnt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileDiv(o *wazeroir.OperationDiv) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileRem(o *wazeroir.OperationRem) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileAnd(o *wazeroir.OperationAnd) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileOr(o *wazeroir.OperationOr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileXor(o *wazeroir.OperationXor) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileShl(o *wazeroir.OperationShl) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileShr(o *wazeroir.OperationShr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileRotl(o *wazeroir.OperationRotl) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileRotr(o *wazeroir.OperationRotr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileAbs(o *wazeroir.OperationAbs) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileNeg(o *wazeroir.OperationNeg) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileCeil(o *wazeroir.OperationCeil) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileFloor(o *wazeroir.OperationFloor) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileTrunc(o *wazeroir.OperationTrunc) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileNearest(o *wazeroir.OperationNearest) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileSqrt(o *wazeroir.OperationSqrt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileMin(o *wazeroir.OperationMin) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileMax(o *wazeroir.OperationMax) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileCopysign(o *wazeroir.OperationCopysign) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileI32WrapFromI64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileITruncFromF(o *wazeroir.OperationITruncFromF) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileFConvertFromI(o *wazeroir.OperationFConvertFromI) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileF32DemoteFromF64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileF64PromoteFromF32() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileI32ReinterpretFromF32() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileI64ReinterpretFromF64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileF32ReinterpretFromI32() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileF64ReinterpretFromI64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileExtend(o *wazeroir.OperationExtend) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileEq(o *wazeroir.OperationEq) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileNe(o *wazeroir.OperationNe) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileEqz(o *wazeroir.OperationEqz) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLt(o *wazeroir.OperationLt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileGt(o *wazeroir.OperationGt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLe(o *wazeroir.OperationLe) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileGe(o *wazeroir.OperationGe) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLoad(o *wazeroir.OperationLoad) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLoad8(o *wazeroir.OperationLoad8) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLoad16(o *wazeroir.OperationLoad16) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileLoad32(o *wazeroir.OperationLoad32) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileStore(o *wazeroir.OperationStore) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileStore8(o *wazeroir.OperationStore8) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileStore16(o *wazeroir.OperationStore16) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileStore32(o *wazeroir.OperationStore32) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileMemoryGrow() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileMemorySize() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileConstI32(o *wazeroir.OperationConstI32) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileConstI64(o *wazeroir.OperationConstI64) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileConstF32(o *wazeroir.OperationConstF32) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Copmiler) compileConstF64(o *wazeroir.OperationConstF64) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}
