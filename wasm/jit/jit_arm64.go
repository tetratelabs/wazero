//go:build arm64
// +build arm64

// This file implements the compiler for arm64 target.
// Please refer to https://developer.arm.com/documentation/102374/latest/
// if unfamiliar with arm64 instructions and semantics.
//
// Note: we use arm64 pkg as the assembler (github.com/twitchyliquid64/golang-asm/obj/arm64)
// which has different notation from the original arm64 assembly. For example,
// 64-bit variant ldr, str, stur are all corresponding to arm64.AMOVD.
// Please refer to https://pkg.go.dev/cmd/internal/obj/arm64.

package jit

import (
	"errors"
	"fmt"
	"math"

	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/wazeroir"
)

// archContext is embedded in Engine in order to store architecture-specific data.
type archContext struct {
	// jitCallReturnAddress holds the absolute return address for jitcall.
	// The value is set whenever jitcall is executed and done in jit_arm64.s
	// Native code can return back to the engine.exec's main loop back by
	// executing "ret" instruction with this value. See arm64Compiler.exit.
	jitCallReturnAddress uint64
}

// engineArchContextJITCallReturnAddressOffset is the offset of archContext.jitCallReturnAddress
const engineArchContextJITCallReturnAddressOffset = 136

// jitcall is implemented in jit_arm64.s as a Go Assembler function.
// This is used by engine.exec and the entrypoint to enter the JITed native code.
// codeSegment is the pointer to the initial instruction of the compiled native code.
// engine is the pointer to the "*engine" as uintptr.
func jitcall(codeSegment, engine uintptr)

// newCompiler returns a new compiler interface which can be used to compile the given function instance.
// Note: ir param can be nil for host functions.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	// We can choose arbitrary number instead of 1024 which indicates the cache size in the compiler.
	// TODO: optimize the number.
	b, err := asm.NewBuilder("arm64", 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}

	compiler := &arm64Compiler{
		f:             f,
		builder:       b,
		locationStack: newValueLocationStack(),
	}

	return compiler, nil
}

type arm64Compiler struct {
	builder *asm.Builder
	f       *wasm.FunctionInstance
	// locationStack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
}

// compile implements compiler.compile for the arm64 architecture.
func (c *arm64Compiler) compile() (code []byte, staticData compiledFunctionStaticData, maxStackPointer uint64, err error) {
	code, err = mmapCodeSegment(c.builder.Assemble())
	if err != nil {
		return
	}
	return
}

func (c *arm64Compiler) newProg() (inst *obj.Prog) {
	inst = c.builder.NewProg()
	return
}

func (c *arm64Compiler) addInstruction(inst *obj.Prog) {
	c.builder.AddInstruction(inst)
}

func (c *arm64Compiler) markRegisterUnused(reg int16) {
	if !isZeroRegister(reg) {
		c.locationStack.markRegisterUnused(reg)
	}
}

// applyConstToRegisterInstruction adds an instruction where source operand is a constant and destination is a register.
func (c *arm64Compiler) applyConstToRegisterInstruction(instruction obj.As, constValue int64, destinationRegister int16) {
	applyConst := c.newProg()
	applyConst.As = instruction
	applyConst.From.Type = obj.TYPE_CONST
	// Note: in raw arm64 assembly, immediates larger than 16-bits
	// are not supported, but the assembler takes care of this and
	// emits corresponding (at most) 4-instructions to load such large constants.
	applyConst.From.Offset = constValue
	applyConst.To.Type = obj.TYPE_REG
	applyConst.To.Reg = destinationRegister
	c.addInstruction(applyConst)
}

// applyMemoryToRegisterInstruction adds an instruction where source operand is a memory location and destination is a register.
// baseRegister is the base absolute address in the memory, and offset is the offset from the absolute address in baseRegister.
func (c *arm64Compiler) applyMemoryToRegisterInstruction(instruction obj.As, baseRegister int16, offset int64, destinationRegister int16) (err error) {
	if offset > math.MaxInt16 {
		// This is a bug in JIT copmiler: caller must check the offset at compilation time, and avoid such a large offset
		// by loading the const to the register beforehand and then using applyRegisterOffsetMemoryToRegisterInstruction instead.
		err = fmt.Errorf("memory offset must be smaller than or equal %d, but got %d", math.MaxInt16, offset)
		return
	}
	inst := c.newProg()
	inst.As = instruction
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = baseRegister
	inst.From.Offset = offset
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destinationRegister
	c.addInstruction(inst)
	return
}

// applyRegisterOffsetMemoryToRegisterInstruction adds an instruction where source operand is a memory location and destination is a register.
// The difference from applyMemoryToRegisterInstruction is that here we specify the offset by a register rather than offset constant.
func (c *arm64Compiler) applyRegisterOffsetMemoryToRegisterInstruction(instruction obj.As, baseRegister, offsetRegister, destinationRegister int16) (err error) {
	inst := c.newProg()
	inst.As = instruction
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = baseRegister
	inst.From.Index = offsetRegister
	inst.From.Scale = 1
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destinationRegister
	c.addInstruction(inst)
	return nil
}

// applyRegisterToMemoryInstruction adds an instruction where destination operand is a memory location and source is a register.
// This is the opposite of applyMemoryToRegisterInstruction.
func (c *arm64Compiler) applyRegisterToMemoryInstruction(instruction obj.As, baseRegister int16, offset int64, source int16) (err error) {
	if offset > math.MaxInt16 {
		// This is a bug in JIT copmiler: caller must check the offset at compilation time, and avoid such a large offset
		// by loading the const to the register beforehand and then using applyRegisterToRegisterOffsetMemoryInstruction instead.
		err = fmt.Errorf("memory offset must be smaller than or equal %d, but got %d", math.MaxInt16, offset)
		return
	}
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = baseRegister
	inst.To.Offset = offset
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = source
	c.addInstruction(inst)
	return
}

// applyRegisterToRegisterOffsetMemoryInstruction adds an instruction where destination operand is a memory location and source is a register.
// The difference from applyRegisterToMemoryInstruction is that here we specify the offset by a register rather than offset constant.
func (c *arm64Compiler) applyRegisterToRegisterOffsetMemoryInstruction(instruction obj.As, baseRegister, offsetRegister, source int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = baseRegister
	inst.To.Index = offsetRegister
	inst.To.Scale = 1
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = source
	c.addInstruction(inst)
}

// applyRegisterToRegisterOffsetMemoryInstruction adds an instruction where both destination and source operands are registers.
func (c *arm64Compiler) applyRegisterToRegisterInstruction(instruction obj.As, from, to int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = to
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = from
	c.addInstruction(inst)
}

// applyRegisterToRegisterOffsetMemoryInstruction adds an instruction which takes two source operands on registers and one destination register operand.
func (c *arm64Compiler) applyTwoRegistersToRegisterInstruction(instruction obj.As, src1, src2, destination int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destination
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = src1
	inst.Reg = src2
	c.addInstruction(inst)
}

func (c *arm64Compiler) String() (ret string) { return }

// emitPreamble implements compiler.emitPreamble for the arm64 architecture.
func (c *arm64Compiler) emitPreamble() error {
	// The assembler skips the first instruction so we intentionally add NOP here.
	nop := c.newProg()
	nop.As = obj.ANOP
	c.addInstruction(nop)

	// TODO: Push function parameter constants.

	// Before excuting function body, we must initialize the stack base pointer register
	// so that we can manipulate the memory stack properly.
	return c.initializeReservedStackBasePointerRegister()
}

// returnFunction emits instructions to return from the current function frame.
// If the current frame is the bottom, the code goes back to the Go code with jitCallStatusCodeReturned status.
// Otherwise, we jump into the caller's return address (TODO).
func (c *arm64Compiler) returnFunction() error {
	// TODO: we don't support function calls yet.
	// For now the following code just simply returns to Go code.

	// Since we return from the function, we need to decrement the callframe stack pointer, and write it back.
	callFramePointerReg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	if err := c.applyMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine,
		engineGlobalContextCallFrameStackPointerOffset, callFramePointerReg); err != nil {
		return err
	}
	c.applyConstToRegisterInstruction(arm64.ASUBS, 1, callFramePointerReg)
	if err := c.applyRegisterToMemoryInstruction(arm64.AMOVD, reservedRegisterForEngine,
		engineGlobalContextCallFrameStackPointerOffset, callFramePointerReg); err != nil {
		return err
	}

	return c.exit(jitCallStatusCodeReturned)
}

// exit adds instructions to give the control back to engine.exec with the given status code.
func (c *arm64Compiler) exit(status jitCallStatusCode) error {
	// Write the current stack pointer to the engine.stackPointer.
	c.applyConstToRegisterInstruction(arm64.AMOVW, int64(c.locationStack.sp), reservedRegisterForTemporary)
	if err := c.applyRegisterToMemoryInstruction(arm64.AMOVW, reservedRegisterForEngine,
		engineValueStackContextStackPointerOffset, reservedRegisterForTemporary); err != nil {
		return err
	}

	if status != 0 {
		c.applyConstToRegisterInstruction(arm64.AMOVW, int64(status), reservedRegisterForTemporary)
		if err := c.applyRegisterToMemoryInstruction(arm64.AMOVWU, reservedRegisterForEngine,
			engineExitContextJITCallStatusCodeOffset, reservedRegisterForTemporary); err != nil {
			return err
		}
	} else {
		// If the status == 0, we simply use zero register to store zero.
		if err := c.applyRegisterToMemoryInstruction(arm64.AMOVWU, reservedRegisterForEngine,
			engineExitContextJITCallStatusCodeOffset, zeroRegister); err != nil {
			return err
		}
	}

	// The return address to the Go code is stored in archContext.jitReturnAddress which
	// is embedded in engine. We load the value to the tmpRegister, and then
	// invoke RET with that register.
	if err := c.applyMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine,
		engineArchContextJITCallReturnAddressOffset, reservedRegisterForTemporary); err != nil {
		return err
	}

	ret := c.newProg()
	ret.As = obj.ARET
	ret.To.Type = obj.TYPE_REG
	ret.To.Reg = reservedRegisterForTemporary
	c.addInstruction(ret)
	return nil
}

func (c *arm64Compiler) compileHostFunction(address wasm.FunctionAddress) error {
	return errors.New("TODO: implement compileHostFunction on arm64")
}

func (c *arm64Compiler) compileLabel(o *wazeroir.OperationLabel) (skipThisLabel bool) {
	return false
}

func (c *arm64Compiler) compileUnreachable() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileSwap(o *wazeroir.OperationSwap) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileGlobalGet(o *wazeroir.OperationGlobalGet) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileGlobalSet(o *wazeroir.OperationGlobalSet) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

// compileBr implements compiler.compileBr for the arm64 architecture.
func (c *arm64Compiler) compileBr(o *wazeroir.OperationBr) error {
	if o.Target.IsReturnTarget() {
		c.returnFunction()
		return nil
	} else {
		return fmt.Errorf("TODO: only return target is available on arm64")
	}
}

func (c *arm64Compiler) compileBrIf(o *wazeroir.OperationBrIf) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileBrTable(o *wazeroir.OperationBrTable) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileCall(o *wazeroir.OperationCall) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileDrop(o *wazeroir.OperationDrop) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileSelect() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compilePick(o *wazeroir.OperationPick) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

// compileAdd implements compiler.compileAdd for the arm64 architecture.
func (c *arm64Compiler) compileAdd(o *wazeroir.OperationAdd) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// Additon can be nop if one of operands is zero.
	if isZeroRegister(x1.register) {
		c.locationStack.pushValueOnRegister(x2.register)
		return nil
	} else if isZeroRegister(x2.register) {
		c.locationStack.pushValueOnRegister(x1.register)
		return nil
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		inst = arm64.AADDW
	case wazeroir.UnsignedTypeI64:
		inst = arm64.AADD
	case wazeroir.UnsignedTypeF32:
		inst = arm64.AFADDS
	case wazeroir.UnsignedTypeF64:
		inst = arm64.AFADDD
	}

	c.applyRegisterToRegisterInstruction(inst, x2.register, x1.register)
	// The result is placed on a register for x1, so record it.
	c.locationStack.pushValueOnRegister(x1.register)
	return nil
}

// compileSub implements compiler.compileSub for the arm64 architecture.
func (c *arm64Compiler) compileSub(o *wazeroir.OperationSub) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// If both of registers are zeros, this can be nop and simply push the zero register.
	if isZeroRegister(x1.register) && isZeroRegister(x2.register) {
		c.locationStack.pushValueOnRegister(zeroRegister)
		return nil
	}

	// At this point, at least one of x1 or x2 registers is non zero.
	// Choose the non-zero register as destination.
	var destinationReg int16 = x1.register
	if isZeroRegister(x1.register) {
		destinationReg = x2.register
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		inst = arm64.ASUBW
	case wazeroir.UnsignedTypeI64:
		inst = arm64.ASUB
	case wazeroir.UnsignedTypeF32:
		inst = arm64.AFSUBS
	case wazeroir.UnsignedTypeF64:
		inst = arm64.AFSUBD
	}

	c.applyTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, destinationReg)
	c.locationStack.pushValueOnRegister(destinationReg)
	return nil
}

// compileMul implements compiler.compileMul for the arm64 architecture.
func (c *arm64Compiler) compileMul(o *wazeroir.OperationMul) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// Multiplcation can be done by putting a zero register if one of operands is zero.
	if isZeroRegister(x1.register) || isZeroRegister(x2.register) {
		c.locationStack.pushValueOnRegister(zeroRegister)
		return nil
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		inst = arm64.AMULW
	case wazeroir.UnsignedTypeI64:
		inst = arm64.AMUL
	case wazeroir.UnsignedTypeF32:
		inst = arm64.AFMULS
	case wazeroir.UnsignedTypeF64:
		inst = arm64.AFMULD
	}

	c.applyRegisterToRegisterInstruction(inst, x2.register, x1.register)
	// The result is placed on a register for x1, so record it.
	c.locationStack.pushValueOnRegister(x1.register)
	return nil
}

func (c *arm64Compiler) compileClz(o *wazeroir.OperationClz) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileCtz(o *wazeroir.OperationCtz) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compilePopcnt(o *wazeroir.OperationPopcnt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileDiv(o *wazeroir.OperationDiv) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileRem(o *wazeroir.OperationRem) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileAnd(o *wazeroir.OperationAnd) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileOr(o *wazeroir.OperationOr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileXor(o *wazeroir.OperationXor) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileShl(o *wazeroir.OperationShl) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileShr(o *wazeroir.OperationShr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileRotl(o *wazeroir.OperationRotl) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileRotr(o *wazeroir.OperationRotr) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileAbs(o *wazeroir.OperationAbs) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileNeg(o *wazeroir.OperationNeg) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileCeil(o *wazeroir.OperationCeil) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileFloor(o *wazeroir.OperationFloor) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileTrunc(o *wazeroir.OperationTrunc) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileNearest(o *wazeroir.OperationNearest) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileSqrt(o *wazeroir.OperationSqrt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileMin(o *wazeroir.OperationMin) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileMax(o *wazeroir.OperationMax) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileCopysign(o *wazeroir.OperationCopysign) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileI32WrapFromI64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileITruncFromF(o *wazeroir.OperationITruncFromF) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileFConvertFromI(o *wazeroir.OperationFConvertFromI) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileF32DemoteFromF64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileF64PromoteFromF32() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileI32ReinterpretFromF32() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileI64ReinterpretFromF64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileF32ReinterpretFromI32() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileF64ReinterpretFromI64() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileExtend(o *wazeroir.OperationExtend) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileEq(o *wazeroir.OperationEq) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileNe(o *wazeroir.OperationNe) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileEqz(o *wazeroir.OperationEqz) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileLt(o *wazeroir.OperationLt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileGt(o *wazeroir.OperationGt) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileLe(o *wazeroir.OperationLe) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileGe(o *wazeroir.OperationGe) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileLoad(o *wazeroir.OperationLoad) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileLoad8(o *wazeroir.OperationLoad8) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileLoad16(o *wazeroir.OperationLoad16) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileLoad32(o *wazeroir.OperationLoad32) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileStore(o *wazeroir.OperationStore) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileStore8(o *wazeroir.OperationStore8) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileStore16(o *wazeroir.OperationStore16) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileStore32(o *wazeroir.OperationStore32) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileMemoryGrow() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileMemorySize() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

// compileConstI32 implements compiler.compileConstI32 for the arm64 architecture.
func (c *arm64Compiler) compileConstI32(o *wazeroir.OperationConstI32) error {
	return c.emitIntConstant(true, uint64(o.Value))
}

// compileConstI64 implements compiler.compileConstI64 for the arm64 architecture.
func (c *arm64Compiler) compileConstI64(o *wazeroir.OperationConstI64) error {
	return c.emitIntConstant(false, o.Value)
}

// emitIntConstant adds instructions to load an integer constant.
// is32bit is true if the target value is originally 32-bit const, false otherwise.
// value holds the (zero-extended for 32-bit case) load target constant.
func (c *arm64Compiler) emitIntConstant(is32bit bool, value uint64) error {
	if value == 0 {
		c.pushZeroValue()
	} else {
		// Take a register to load the value.
		reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}

		var inst obj.As
		if is32bit {
			inst = arm64.AMOVW
		} else {
			inst = arm64.AMOVD
		}
		c.applyConstToRegisterInstruction(inst, int64(value), reg)

		c.locationStack.pushValueOnRegister(reg)
	}
	return nil
}

// compileConstF32 implements compiler.compileConstF32 for the arm64 architecture.
func (c *arm64Compiler) compileConstF32(o *wazeroir.OperationConstF32) error {
	return c.emitFloatConstant(true, uint64(math.Float32bits(o.Value)))
}

// compileConstF64 implements compiler.compileConstF64 for the arm64 architecture.
func (c *arm64Compiler) compileConstF64(o *wazeroir.OperationConstF64) error {
	return c.emitFloatConstant(false, math.Float64bits(o.Value))
}

// emitFloatConstant adds instructions to load a float constant.
// is32bit is true if the target value is originally 32-bit const, false otherwise.
// value holds the (zero-extended for 32-bit case) bit representation of load target float constant.
func (c *arm64Compiler) emitFloatConstant(is32bit bool, value uint64) error {
	// Take a register to load the value.
	reg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	tmpReg := zeroRegister
	if value != 0 {
		tmpReg = reservedRegisterForTemporary
		var inst obj.As
		if is32bit {
			inst = arm64.AMOVW
		} else {
			inst = arm64.AMOVD
		}
		c.applyConstToRegisterInstruction(inst, int64(value), tmpReg)
	}

	// Use FMOV instruction to move the value on integer register into the float one.
	var inst obj.As
	if is32bit {
		inst = arm64.AFMOVS
	} else {
		inst = arm64.AFMOVD
	}
	c.applyRegisterToRegisterInstruction(inst, tmpReg, reg)

	c.locationStack.pushValueOnRegister(reg)
	return nil
}

func (c *arm64Compiler) pushZeroValue() {
	c.locationStack.pushValueOnRegister(zeroRegister)
}

// popTwoValuesOnRegisters pops two values from the location stacks, ensures
// these two values are located on registers, and mark them unused.
func (c *arm64Compiler) popTwoValuesOnRegisters() (x1, x2 *valueLocation, err error) {
	x2 = c.locationStack.pop()
	if err = c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return
	}

	x1 = c.locationStack.pop()
	if err = c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return
	}

	c.markRegisterUnused(x1.register)
	c.markRegisterUnused(x2.register)
	return
}

// ensureOnGeneralPurposeRegister emits instructions to ensure that a value is located on a register.
func (c *arm64Compiler) ensureOnGeneralPurposeRegister(loc *valueLocation) (err error) {
	if loc.onStack() {
		err = c.loadValueOnStackToRegister(loc)
	} else if loc.onConditionalRegister() {
		err = fmt.Errorf("TODO: support moving conditional to general purpose register")
	}
	return
}

// loadValueOnStackToRegister emits instructions to load the value located on the stack to a register.
func (c *arm64Compiler) loadValueOnStackToRegister(loc *valueLocation) (err error) {
	var inst obj.As
	var reg int16
	switch loc.regType {
	case generalPurposeRegisterTypeInt:
		inst = arm64.AMOVD
		reg, err = c.allocateRegister(generalPurposeRegisterTypeInt)
	case generalPurposeRegisterTypeFloat:
		inst = arm64.AFMOVD
		reg, err = c.allocateRegister(generalPurposeRegisterTypeFloat)
	}

	if err != nil {
		return
	}

	if offset := int64(loc.stackPointer) * 8; offset > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		c.applyConstToRegisterInstruction(arm64.AMOVD, offset, reservedRegisterForTemporary)
		c.applyRegisterOffsetMemoryToRegisterInstruction(inst, reservedRegisterForStackBasePointerAddress, reservedRegisterForTemporary, reg)
	} else {
		c.applyMemoryToRegisterInstruction(inst, reservedRegisterForStackBasePointerAddress, offset, reg)
	}

	// Record that the value holds the register and the register is marked used.
	loc.setRegister(reg)
	c.locationStack.markRegisterUsed(reg)
	return
}

// allocateRegister returns an unused register of the given type. The register will be taken
// either from the free register pool or by spilling an used register. If we allocate an used register,
// this adds an instruction to write the value on a register back to memory stack region.
// Note: resulting registers are NOT marked as used so the call site should mark it used if necessary.
func (c *arm64Compiler) allocateRegister(t generalPurposeRegisterType) (reg int16, err error) {
	var ok bool
	// Try to get the unused register.
	reg, ok = c.locationStack.takeFreeRegister(t)
	if ok {
		return
	}

	// If not found, we have to steal the register.
	stealTarget, ok := c.locationStack.takeStealTargetFromUsedRegister(t)
	if !ok {
		err = fmt.Errorf("cannot steal register")
		return
	}

	// Release the steal target register value onto stack location.
	reg = stealTarget.register
	err = c.releaseRegisterToStack(stealTarget)
	return
}

// releaseRegisterToStack adds an instruction to write the value on a register back to memory stack region.
func (c *arm64Compiler) releaseRegisterToStack(loc *valueLocation) (err error) {
	var inst obj.As = arm64.AMOVD
	if loc.regType == generalPurposeRegisterTypeFloat {
		inst = arm64.AFMOVD
	}

	if offset := int64(loc.stackPointer) * 8; offset > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		c.applyConstToRegisterInstruction(arm64.AMOVD, offset, reservedRegisterForTemporary)
		c.applyRegisterToRegisterOffsetMemoryInstruction(inst, reservedRegisterForStackBasePointerAddress, reservedRegisterForTemporary, loc.register)
	} else {
		if err = c.applyRegisterToMemoryInstruction(inst, reservedRegisterForStackBasePointerAddress, offset, loc.register); err != nil {
			return
		}
	}

	// Mark the register is free.
	c.locationStack.releaseRegister(loc)
	return
}

// initializeReservedStackBasePointerRegister adds intructions to initialize reservedRegisterForStackBasePointerAddress
// so that it points to the absolute address of the stack base for this function.
func (c *arm64Compiler) initializeReservedStackBasePointerRegister() error {
	// First, load the address of the first element in the value stack into reservedRegisterForStackBasePointerAddress temporarily.
	if err := c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextValueStackElement0AddressOffset,
		reservedRegisterForStackBasePointerAddress); err != nil {
		return err
	}

	// Next we move the base pointer (engine.stackBasePointer) to the tmp register.
	if err := c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset,
		reservedRegisterForTemporary); err != nil {
		return err
	}

	// Finally, we calculate "reservedRegisterForStackBasePointerAddress + tmpReg * 8"
	// where we multiply tmpReg by 8 because stack pointer is an index in the []uint64
	// so as an bytes we must multiply the size of uint64 = 8 bytes.
	calcStackBasePointerAddress := c.newProg()
	calcStackBasePointerAddress.As = arm64.AADD
	calcStackBasePointerAddress.To.Type = obj.TYPE_REG
	calcStackBasePointerAddress.To.Reg = reservedRegisterForStackBasePointerAddress
	// We calculate "tmpReg * 8" as "tmpReg << 3".
	setLeftShiftedRegister(calcStackBasePointerAddress, reservedRegisterForTemporary, 3)
	c.addInstruction(calcStackBasePointerAddress)
	return nil
}

// setShiftedRegister modifies the given *obj.Prog so that .From (source operand)
// becomes the "left shifted register". For example, this is used to emit instruction like
// "add  x1, x2, x3, lsl #3" which means "x1 = x2 + (x3 << 3)".
// See https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L120-L131
func setLeftShiftedRegister(inst *obj.Prog, register int16, shiftNum int64) {
	inst.From.Type = obj.TYPE_SHIFT
	inst.From.Offset = (int64(register)&31)<<16 | 0<<22 | (shiftNum&63)<<10
}
