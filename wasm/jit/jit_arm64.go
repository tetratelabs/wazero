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

func (c *arm64Compiler) String() (ret string) { return }

// emitPreamble implements compiler.emitPreamble for the arm64 architecture.
func (c *arm64Compiler) emitPreamble() error {
	// The assembler skips the first instruction so we intentionally add NOP here.
	nop := c.newProg()
	nop.As = obj.ANOP
	c.addInstruction(nop)

	// Before excuting function body, we must initialize the stack base pointer register
	// so that we can manipulate the memory stack properly.
	c.initializeReservedStackBasePointerRegister()
	return nil
}

func (c *arm64Compiler) returnFunction() {

	// TODO: we don't support function calls yet.
	// For now the following code just simply returns to Go code.

	// Since we return from the function, we need to decrement the callframe stack pointer.
	callFramePointerReg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	loadCurrentCallFramPointer := c.newProg()
	loadCurrentCallFramPointer.As = arm64.AMOVD
	loadCurrentCallFramPointer.To.Type = obj.TYPE_REG
	loadCurrentCallFramPointer.To.Reg = callFramePointerReg
	loadCurrentCallFramPointer.From.Type = obj.TYPE_MEM
	loadCurrentCallFramPointer.From.Reg = reservedRegisterForEngine
	loadCurrentCallFramPointer.From.Offset = engineGlobalContextCallFrameStackPointerOffset
	c.addInstruction(loadCurrentCallFramPointer)

	decCallFrameStackPointer := c.newProg()
	decCallFrameStackPointer.As = arm64.ASUBS
	decCallFrameStackPointer.To.Type = obj.TYPE_REG
	decCallFrameStackPointer.To.Reg = callFramePointerReg
	decCallFrameStackPointer.From.Type = obj.TYPE_CONST
	decCallFrameStackPointer.From.Offset = 1
	c.addInstruction(decCallFrameStackPointer)

	writeDecrementedCallFrameStackPoitner := c.newProg()
	writeDecrementedCallFrameStackPoitner.As = arm64.AMOVD
	writeDecrementedCallFrameStackPoitner.To = loadCurrentCallFramPointer.From
	writeDecrementedCallFrameStackPoitner.From = loadCurrentCallFramPointer.To
	c.addInstruction(writeDecrementedCallFrameStackPoitner)

	c.exit(jitCallStatusCodeReturned)
}

// exit adds instructions to give the control back to engine.exec with the given status code.
func (c *arm64Compiler) exit(status jitCallStatusCode) {
	tmp, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)

	// Write the current stack pointer to the engine.stackPointer.
	loadStackPointerConst := c.newProg()
	loadStackPointerConst.As = arm64.AMOVW
	loadStackPointerConst.To.Type = obj.TYPE_REG
	loadStackPointerConst.To.Reg = tmp
	loadStackPointerConst.From.Type = obj.TYPE_CONST
	loadStackPointerConst.From.Offset = int64(c.locationStack.sp)
	c.addInstruction(loadStackPointerConst)

	writeStackPointerToEngine := c.newProg()
	writeStackPointerToEngine.As = arm64.AMOVW
	writeStackPointerToEngine.From.Type = obj.TYPE_REG
	writeStackPointerToEngine.From.Reg = tmp
	writeStackPointerToEngine.To.Type = obj.TYPE_MEM
	writeStackPointerToEngine.To.Reg = reservedRegisterForEngine
	writeStackPointerToEngine.To.Offset = engineValueStackContextStackPointerOffset
	c.addInstruction(writeStackPointerToEngine)

	if status != 0 {
		loadStatusConst := c.newProg()
		loadStatusConst.As = arm64.AMOVW
		loadStatusConst.To.Type = obj.TYPE_REG
		loadStatusConst.To.Reg = tmp
		loadStatusConst.From.Type = obj.TYPE_CONST
		loadStatusConst.From.Offset = int64(status)
		c.addInstruction(loadStatusConst)

		setJitStatus := c.newProg()
		setJitStatus.As = arm64.AMOVWU
		setJitStatus.From.Type = obj.TYPE_REG
		setJitStatus.From.Reg = tmp
		setJitStatus.To.Type = obj.TYPE_MEM
		setJitStatus.To.Reg = reservedRegisterForEngine
		setJitStatus.To.Offset = engineExitContextJITCallStatusCodeOffset
		c.addInstruction(setJitStatus)
	} else {
		// If the status == 0, we simply use zero register to store zero.
		setJitStatus := c.newProg()
		setJitStatus.As = arm64.AMOVWU
		setJitStatus.From.Type = obj.TYPE_REG
		setJitStatus.From.Reg = zeroRegister
		setJitStatus.To.Type = obj.TYPE_MEM
		setJitStatus.To.Reg = reservedRegisterForEngine
		setJitStatus.To.Offset = engineExitContextJITCallStatusCodeOffset
		c.addInstruction(setJitStatus)
	}

	// The return address to the Go code is stored in archContext.jitReturnAddress which
	// is embedded in engine. We load the value to the tmpRegister, and then
	// invoke RET with that register.
	loadReturnAddress := c.newProg()
	loadReturnAddress.As = arm64.AMOVD
	loadReturnAddress.To.Type = obj.TYPE_REG
	loadReturnAddress.To.Reg = tmp
	loadReturnAddress.From.Type = obj.TYPE_MEM
	loadReturnAddress.From.Reg = reservedRegisterForEngine
	loadReturnAddress.From.Offset = engineArchContextJITCallReturnAddressOffset
	c.addInstruction(loadReturnAddress)

	ret := c.newProg()
	ret.As = obj.ARET
	ret.To.Type = obj.TYPE_REG
	ret.To.Reg = tmp
	c.addInstruction(ret)
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

func (c *arm64Compiler) compileAdd(o *wazeroir.OperationAdd) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileSub(o *wazeroir.OperationSub) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileMul(o *wazeroir.OperationMul) error {
	return fmt.Errorf("TODO: unsupported on arm64")
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

		// Then, load the const.
		loadConst := c.newProg()
		if is32bit {
			loadConst.As = arm64.AMOVW
		} else {
			loadConst.As = arm64.AMOVD
		}
		loadConst.From.Type = obj.TYPE_CONST
		// Note: in raw arm64 assembly, immediates larger than 16-bits
		// are not supported, but the assembler takes care of this and
		// emits corresponding (at most) 4-instructions to load such large constants.
		loadConst.From.Offset = int64(value)
		loadConst.To.Type = obj.TYPE_REG
		loadConst.To.Reg = reg
		c.addInstruction(loadConst)

		loc := c.locationStack.pushValueOnRegister(reg)
		loc.setRegisterType(generalPurposeRegisterTypeInt)
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
		// If the target value is not zero, we have to load the constant
		// temporarily into the integer register since we cannot directly
		// move the const into float register.
		tmpReg, err = c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}

		loadConst := c.newProg()
		if is32bit {
			loadConst.As = arm64.AMOVW
		} else {
			loadConst.As = arm64.AMOVD
		}
		loadConst.From.Type = obj.TYPE_CONST
		// Note: in raw arm64 assembly, immediates larger than 16-bits
		// are not supported, but the assembler takes care of this and
		// emits corresponding (at most) 4-instructions to load such large constants.
		loadConst.From.Offset = int64(value)
		loadConst.To.Type = obj.TYPE_REG
		loadConst.To.Reg = tmpReg
		c.addInstruction(loadConst)
	}

	// Use FMOV instruction to move the value on integer register into the float one.
	mov := c.newProg()
	if is32bit {
		mov.As = arm64.AFMOVS
	} else {
		mov.As = arm64.AFMOVD
	}
	mov.From.Type = obj.TYPE_REG
	mov.From.Reg = tmpReg
	mov.To.Type = obj.TYPE_REG
	mov.To.Reg = reg
	c.addInstruction(mov)

	loc := c.locationStack.pushValueOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)
	return nil
}

func (c *arm64Compiler) pushZeroValue() {
	c.locationStack.pushValueOnRegister(zeroRegister)
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
	c.releaseRegisterToStack(stealTarget)
	return
}

// releaseRegisterToStack adds an instruction to write the value on a register back to memory stack region.
func (c *arm64Compiler) releaseRegisterToStack(loc *valueLocation) {
	// Push value.
	store := c.newProg()
	switch loc.regType {
	case generalPurposeRegisterTypeInt:
		store.As = arm64.AMOVD
	case generalPurposeRegisterTypeFloat:
		store.As = arm64.AFMOVD
	}

	store.To.Type = obj.TYPE_MEM
	store.To.Reg = reservedRegisterForStackBasePointerAddress
	// Note: in raw arm64 assembly, immediates larger than 16-bits
	// are not supported, but the assembler takes care of this and
	// emits corresponding (at most) 4-instructions to load such large constants.
	store.To.Offset = int64(loc.stackPointer) * 8
	store.From.Type = obj.TYPE_REG
	store.From.Reg = loc.register
	c.addInstruction(store)

	nop := c.newProg()
	nop.As = obj.ANOP
	c.addInstruction(nop)

	// Mark the register is free.
	c.locationStack.releaseRegister(loc)
}

// initializeReservedStackBasePointerRegister adds intructions to initialize reservedRegisterForStackBasePointerAddress
// so that it points to the absolute address of the stack base for this function.
func (c *arm64Compiler) initializeReservedStackBasePointerRegister() {
	tmpReg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)

	loadElement0Address := c.newProg()
	loadElement0Address.As = arm64.AMOVD
	loadElement0Address.From.Type = obj.TYPE_MEM
	loadElement0Address.From.Reg = reservedRegisterForEngine
	loadElement0Address.From.Offset = engineGlobalContextValueStackElement0AddressOffset
	loadElement0Address.To.Type = obj.TYPE_REG
	loadElement0Address.To.Reg = reservedRegisterForStackBasePointerAddress
	c.addInstruction(loadElement0Address)

	// Next we move the base pointer (engine.stackBasePointer) to the tmp register.
	getStackBasePointer := c.newProg()
	getStackBasePointer.As = arm64.AMOVD
	getStackBasePointer.From.Type = obj.TYPE_MEM
	getStackBasePointer.From.Reg = reservedRegisterForEngine
	getStackBasePointer.From.Offset = engineValueStackContextStackBasePointerOffset
	getStackBasePointer.To.Type = obj.TYPE_REG
	getStackBasePointer.To.Reg = tmpReg
	c.addInstruction(getStackBasePointer)

	// Finally, we calculate "reservedRegisterForStackBasePointerAddress + tmpReg * 8"
	// where we multiply tmpReg by 8 because stack pointer is an index in the []uint64
	// so as an bytes we must multiply the size of uint64 = 8 bytes.
	calcStackBasePointerAddress := c.newProg()
	calcStackBasePointerAddress.As = arm64.AADD
	calcStackBasePointerAddress.To.Type = obj.TYPE_REG
	calcStackBasePointerAddress.To.Reg = reservedRegisterForStackBasePointerAddress
	// We calculate "tmpReg * 8" as "tmpReg << 3".
	setLeftShiftedRegister(calcStackBasePointerAddress, tmpReg, 3)
	c.addInstruction(calcStackBasePointerAddress)
}

// setShiftedRegister modifies the given *obj.Prog so that .From (source operand)
// becomes the "left shifted register". For example, this is used to emit instruction like
// "add  x1, x2, x3, lsl #3" which means "x1 = x2 + (x3 << 3)".
// See https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L120-L131
func setLeftShiftedRegister(inst *obj.Prog, register int16, shiftNum int64) {
	inst.From.Type = obj.TYPE_SHIFT
	inst.From.Offset = (int64(register)&31)<<16 | 0<<22 | (shiftNum&63)<<10
}
