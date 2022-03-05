//go:build arm64

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
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"unsafe"

	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/arm64"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func newArchContext() archContext {
	return archContext{
		minimum32BitSignedInt: math.MinInt32,
		minimum64BitSignedInt: math.MinInt64,
	}
}

// archContext is embedded in callEngine in order to store architecture-specific data.
type archContext struct {
	// jitCallReturnAddress holds the absolute return address for jitcall.
	// The value is set whenever jitcall is executed and done in jit_arm64.s
	// Native code can return back to the ce.execWasmFunction's main loop back by
	// executing "ret" instruction with this value. See arm64Compiler.exit.
	// Note: this is only used by JIT code so mark this as nolint.
	jitCallReturnAddress uint64 //nolint

	// Loading large constants in arm64 is a bit costly so we place the following
	// consts on callEngine struct so that we can quickly access them during various operations.

	// minimum32BitSignedInt is used for overflow check for 32-bit signed division.
	// Note: this can be obtained by moving $1 and doing left-shift with 31, but it is
	// slower than directly loading fron this location.
	minimum32BitSignedInt int32
	// Note: this can be obtained by moving $1 and doing left-shift with 63, but it is
	// slower than directly loading fron this location.
	// minimum64BitSignedInt is used for overflow check for 64-bit signed division.
	minimum64BitSignedInt int64
}

const (
	// callEngineArchContextJITCallReturnAddressOffset is the offset of archContext.jitCallReturnAddress in callEngine.
	callEngineArchContextJITCallReturnAddressOffset = 128
	// callEngineArchContextMinimum32BitSignedIntOffset is the offset of archContext.minimum32BitSignedIntAddress in callEngine.
	callEngineArchContextMinimum32BitSignedIntOffset = 136
	// callEngineArchContextMinimum64BitSignedIntOffset is the offset of archContext.minimum64BitSignedIntAddress in callEngine.
	callEngineArchContextMinimum64BitSignedIntOffset = 144
)

// jitcall is implemented in jit_arm64.s as a Go Assembler function.
// This is used by callEngine.execWasmFunction and the entrypoint to enter the JITed native code.
// codeSegment is the pointer to the initial instruction of the compiled native code.
// ce is "*callEngine" as uintptr.
func jitcall(codeSegment, ce uintptr)

// golang-asm is not goroutine-safe so we take lock until we complete the compilation.
// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
var assemblerMutex = &sync.Mutex{}

func unlockAssembler() {
	assemblerMutex.Unlock()
}

// newCompiler returns a new compiler interface which can be used to compile the given function instance.
// The function returned must be invoked when finished compiling, so use `defer` to ensure this.
// Note: ir param can be nil for host functions.
func newCompiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (c compiler, done func(), err error) {
	// golang-asm is not goroutine-safe so we take lock until we complete the compilation.
	// TODO: delete after https://github.com/tetratelabs/wazero/issues/233
	assemblerMutex.Lock()

	// We can choose arbitrary number instead of 1024 which indicates the cache size in the compiler.
	// TODO: optimize the number.
	b, err := asm.NewBuilder("arm64", 1024)
	if err != nil {
		return nil, unlockAssembler, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}

	compiler := &arm64Compiler{
		f:             f,
		builder:       b,
		locationStack: newValueLocationStack(),
		ir:            ir,
		labels:        map[string]*labelInfo{},
	}
	return compiler, unlockAssembler, nil
}

type arm64Compiler struct {
	builder *asm.Builder
	f       *wasm.FunctionInstance
	ir      *wazeroir.CompilationResult
	// setBranchTargetOnNextInstructions holds branch kind instructions (BR, conditional BR, etc)
	// where we want to set the next coming instruction as the destination of these BR instructions.
	setBranchTargetOnNextInstructions []*obj.Prog
	// locationStack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
	// labels maps a label (Ex. ".L1_then") to *labelInfo.
	labels map[string]*labelInfo
	// stackPointerCeil is the greatest stack pointer value (from valueLocationStack) seen during compilation.
	stackPointerCeil uint64
	// afterAssembleCallback hold the callbacks which are called after assembling native code.
	afterAssembleCallback []func(code []byte) error
	// onStackPointerCeilDeterminedCallBack hold a callback which are called when the ceil of stack pointer is determined before generating native code.
	onStackPointerCeilDeterminedCallBack func(stackPointerCeil uint64)
	// compiledFunctionStaticData holds br_table offset tables.
	// See compiledFunctionStaticData and arm64Compiler.compileBrTable.
	staticData compiledFunctionStaticData
}

func (c *arm64Compiler) addStaticData(d []byte) {
	c.staticData = append(c.staticData, d)
}

// compile implements compiler.compile for the arm64 architecture.
func (c *arm64Compiler) compile() (code []byte, staticData compiledFunctionStaticData, stackPointerCeil uint64, err error) {
	// c.stackPointerCeil tracks the stack pointer ceiling (max seen) value across all valueLocationStack(s)
	// used for all labels (via setLocationStack), excluding the current one.
	// Hence, we check here if the final block's max one exceeds the current c.stackPointerCeil.
	stackPointerCeil = c.stackPointerCeil
	if stackPointerCeil < c.locationStack.stackPointerCeil {
		stackPointerCeil = c.locationStack.stackPointerCeil
	}

	// Now that the ceil of stack pointer is determined, we are invoking the callback.
	// Note: this must be called before Assemble() befolow.
	if c.onStackPointerCeilDeterminedCallBack != nil {
		c.onStackPointerCeilDeterminedCallBack(stackPointerCeil)
	}

	original := c.builder.Assemble()

	for _, cb := range c.afterAssembleCallback {
		if err = cb(original); err != nil {
			return
		}
	}

	code, err = mmapCodeSegment(original)
	if err != nil {
		return
	}

	staticData = c.staticData
	return
}

// labelInfo holds a wazeroir label specific information in this function.
type labelInfo struct {
	// initialInstruction is the initial instruction for this label so other block can branch into it.
	initialInstruction *obj.Prog
	// initialStack is the initial value location stack from which we start compiling this label.
	initialStack *valueLocationStack
	// labelBeginningCallbacks holds callbacks should to be called with initialInstruction
	labelBeginningCallbacks []func(*obj.Prog)
}

func (c *arm64Compiler) label(labelKey string) *labelInfo {
	ret, ok := c.labels[labelKey]
	if ok {
		return ret
	}
	c.labels[labelKey] = &labelInfo{}
	return c.labels[labelKey]
}

func (c *arm64Compiler) newProg() (inst *obj.Prog) {
	inst = c.builder.NewProg()
	for _, origin := range c.setBranchTargetOnNextInstructions {
		origin.To.SetTarget(inst)
	}
	c.setBranchTargetOnNextInstructions = nil
	return
}

func (c *arm64Compiler) addInstruction(inst *obj.Prog) {
	c.builder.AddInstruction(inst)
}

func (c *arm64Compiler) setBranchTargetOnNext(progs ...*obj.Prog) {
	c.setBranchTargetOnNextInstructions = append(c.setBranchTargetOnNextInstructions, progs...)
}

func (c *arm64Compiler) markRegisterUsed(regs ...int16) {
	for _, reg := range regs {
		if !isZeroRegister(reg) {
			c.locationStack.markRegisterUsed(reg)
		}
	}
}

func (c *arm64Compiler) markRegisterUnused(regs ...int16) {
	for _, reg := range regs {
		if !isZeroRegister(reg) {
			c.locationStack.markRegisterUnused(reg)
		}
	}
}

// compileConstToRegisterInstruction adds an instruction where source operand is a constant and destination is a register.
func (c *arm64Compiler) compileConstToRegisterInstruction(instruction obj.As, constValue int64, destinationRegister int16) (inst *obj.Prog) {
	inst = c.newProg()
	inst.As = instruction
	inst.From.Type = obj.TYPE_CONST
	// Note: in raw arm64 assembly, immediates larger than 16-bits
	// are not supported, but the assembler takes care of this and
	// emits corresponding (at most) 4-instructions to load such large constants.
	inst.From.Offset = constValue
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destinationRegister
	c.addInstruction(inst)
	return
}

// compileMemoryToRegisterInstruction adds an instruction where source operand points a memory location and destination is a register.
// sourceBaseReg is the base absolute address in the memory, and sourceOffsetConst is the offset from the absolute address in sourceBaseReg.
// This is the opposite of compileRegisterToMemoryInstruction.
func (c *arm64Compiler) compileMemoryToRegisterInstruction(instruction obj.As, sourceBaseReg int16, sourceOffsetConst int64, destinationReg int16) {
	if sourceOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		c.compileConstToRegisterInstruction(arm64.AMOVD, sourceOffsetConst, reservedRegisterForTemporary)
		c.compileMemoryWithRegisterOffsetToRegisterInstruction(instruction, sourceBaseReg, reservedRegisterForTemporary, destinationReg)
	} else {
		inst := c.newProg()
		inst.As = instruction
		inst.From.Type = obj.TYPE_MEM
		inst.From.Reg = sourceBaseReg
		inst.From.Offset = sourceOffsetConst
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = destinationReg
		c.addInstruction(inst)
	}
}

func (c *arm64Compiler) compileMemoryWithRegisterOffsetToRegisterInstruction(instruction obj.As, sourceBaseReg, sourceOffsetReg, destinationReg int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.From.Type = obj.TYPE_MEM
	inst.From.Reg = sourceBaseReg
	inst.From.Index = sourceOffsetReg
	inst.From.Scale = 1
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destinationReg
	c.addInstruction(inst)
}

// compileRegisterToMemoryInstruction adds an instruction where destination operand points a memory location and source is a register.
// This is the opposite of compileMemoryToRegisterInstruction.
func (c *arm64Compiler) compileRegisterToMemoryInstruction(instruction obj.As, sourceRegister int16, destinationBaseRegister int16, destinationOffsetConst int64) {
	if destinationOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but we cannot track its temporary register. Therefore, we avoid directly emitting memory load with large offsets:
		// load the constant manually to "our" temporary register, then emit the load with it.
		c.compileConstToRegisterInstruction(arm64.AMOVD, destinationOffsetConst, reservedRegisterForTemporary)
		c.compileRegisterToMemoryWithRegisterOffsetInstruction(instruction, sourceRegister, destinationBaseRegister, reservedRegisterForTemporary)
	} else {
		inst := c.newProg()
		inst.As = instruction
		inst.To.Type = obj.TYPE_MEM
		inst.To.Reg = destinationBaseRegister
		inst.To.Offset = destinationOffsetConst
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = sourceRegister
		c.addInstruction(inst)
	}
}

func (c *arm64Compiler) compileRegisterToMemoryWithRegisterOffsetInstruction(instruction obj.As, sourceRegister, destinationBaseRegister, destinationOffsetRegister int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_MEM
	inst.To.Reg = destinationBaseRegister
	inst.To.Index = destinationOffsetRegister
	inst.To.Scale = 1
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = sourceRegister
	c.addInstruction(inst)
}

// compileRegisterToRegisterInstruction adds an instruction where both destination and source operands are registers.
func (c *arm64Compiler) compileRegisterToRegisterInstruction(instruction obj.As, from, to int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = to
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = from
	c.addInstruction(inst)
}

// compileTwoRegistersToRegisterInstruction adds an instruction which takes two source operands on registers and one destination register operand.
func (c *arm64Compiler) compileTwoRegistersToRegisterInstruction(instruction obj.As, src1, src2, destination int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destination
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = src1
	inst.Reg = src2
	c.addInstruction(inst)
}

// compileTwoRegistersToRegisterInstruction adds an instruction which takes two source and destination register operands.
func (c *arm64Compiler) compileTwoRegistersInstruction(instruction obj.As, src1, src2, dst1, dst2 int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = dst1
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = src1
	inst.Reg = src2
	inst.RestArgs = append(inst.RestArgs, obj.Addr{Type: obj.TYPE_REG, Reg: dst2})
	c.addInstruction(inst)
}

// compileTwoRegistersToNoneInstruction adds an instruction which takes two source operands on registers.
func (c *arm64Compiler) compileTwoRegistersToNoneInstruction(instruction obj.As, src1, src2 int16) {
	inst := c.newProg()
	inst.As = instruction
	// TYPE_NONE indicates that this instruction doesn't have a destination.
	// Note: this line is deletable as the value equals zero in anyway.
	inst.To.Type = obj.TYPE_NONE
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = src1
	inst.Reg = src2
	c.addInstruction(inst)
}

func (c *arm64Compiler) compileRegisterAndConstSourceToNoneInstruction(instruction obj.As, src int16, srcConst int64) {
	inst := c.newProg()
	inst.As = instruction
	// TYPE_NONE indicates that this instruction doesn't have a destination.
	// Note: this line is deletable as the value equals zero in anyway.
	inst.To.Type = obj.TYPE_NONE
	inst.From.Type = obj.TYPE_CONST
	inst.From.Offset = srcConst
	inst.Reg = src
	c.addInstruction(inst)
}

func (c *arm64Compiler) compilelBranchInstruction(inst obj.As) (br *obj.Prog) {
	br = c.newProg()
	br.As = inst
	br.To.Type = obj.TYPE_BRANCH
	c.addInstruction(br)
	return
}

func (c *arm64Compiler) compileUnconditionalBranchToAddressOnRegister(addressRegister int16) {
	br := c.newProg()
	br.As = obj.AJMP
	br.To.Type = obj.TYPE_MEM
	br.To.Reg = addressRegister
	c.addInstruction(br)
}

// compileAddInstructionWithLeftShiftedRegister emits an ADD instruction to perform "destinationReg = srcReg + (shiftedSourceReg << shiftNum)".
func (c *arm64Compiler) compileAddInstructionWithLeftShiftedRegister(shiftedSourceReg int16, shiftNum int64, srcReg, destinationReg int16) {
	inst := c.newProg()
	inst.As = arm64.AADD
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = destinationReg
	// See https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L120-L131
	inst.From.Type = obj.TYPE_SHIFT
	inst.From.Offset = (int64(shiftedSourceReg)&31)<<16 | 0<<22 | (shiftNum&63)<<10
	inst.Reg = srcReg
	c.addInstruction(inst)
}

func (c *arm64Compiler) compileNOP() (nop *obj.Prog) {
	nop = c.newProg()
	nop.As = obj.ANOP
	c.addInstruction(nop)
	return
}

func (c *arm64Compiler) String() (ret string) { return }

// pushFunctionParams pushes any function parameters onto the stack, setting appropriate register types.
func (c *arm64Compiler) pushFunctionParams() {
	if c.f == nil || c.f.FunctionType == nil {
		return
	}
	for _, t := range c.f.FunctionType.Type.Params {
		loc := c.locationStack.pushValueLocationOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
	}
}

// compilePreamble implements compiler.compilePreamble for the arm64 architecture.
func (c *arm64Compiler) compilePreamble() error {
	// The assembler skips the first instruction so we intentionally add NOP here.
	// TODO: delete after #233
	c.compileNOP()

	c.pushFunctionParams()

	// Check if it's necessary to grow the value stack before entering function body.
	if err := c.compileMaybeGrowValueStack(); err != nil {
		return err
	}

	// We must initialize the stack base pointer register so that we can manipulate the stack properly.
	c.compileReservedStackBasePointerRegisterInitialization()

	if err := c.compileModuleContextInitialization(); err != nil {
		return err
	}

	c.compileReservedMemoryRegisterInitialization()
	return nil
}

// compileMaybeGrowValueStack adds instructions to check the necessity to grow the value stack,
// and if so, make the builtin function call to do so. These instructions are called in the function's
// preamble.
func (c *arm64Compiler) compileMaybeGrowValueStack() error {
	tmpRegs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 2)
	if !found {
		return fmt.Errorf("BUG: all the registers should be free at this point")
	}
	tmpX, tmpY := tmpRegs[0], tmpRegs[1]

	// "tmpX = len(ce.valueStack)"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextValueStackLenOffset,
		tmpX,
	)

	// "tmpY = ce.stackBasePointer"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset,
		tmpY,
	)

	// "tmpX = tmpX - tmpY", in other words "tmpX = len(ce.valueStack) - ce.stackBasePointer"
	c.compileRegisterToRegisterInstruction(
		arm64.ASUB,
		tmpY,
		tmpX,
	)

	// "tmpY = stackPointerCeil"
	loadStackPointerCeil := c.compileConstToRegisterInstruction(
		arm64.AMOVD,
		math.MaxInt32,
		tmpY,
	)
	// At this point of compilation, we don't know the value of stack pointe ceil,
	// so we layzily resolve the value later.
	c.onStackPointerCeilDeterminedCallBack = func(stackPointerCeil uint64) { loadStackPointerCeil.From.Offset = int64(stackPointerCeil) }

	// Compare tmpX (len(ce.valueStack) - ce.stackBasePointer) and tmpY (ce.stackPointerCeil)
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, tmpX, tmpY)

	// If ceil > valueStackLen - stack base pointer, we need to grow the stack by calling builtin Go function.
	brIfValueStackOK := c.compilelBranchInstruction(arm64.ABLS)
	if err := c.compileCallGoFunction(jitCallStatusCodeCallBuiltInFunction, builtinFunctionIndexGrowValueStack); err != nil {
		return err
	}

	// Otherwise, skip calling it.
	c.setBranchTargetOnNext(brIfValueStackOK)

	c.locationStack.markRegisterUnused(tmpRegs...)
	return nil
}

// returnFunction emits instructions to return from the current function frame.
// If the current frame is the bottom, the code goes back to the Go code with jitCallStatusCodeReturned status.
// Otherwise, we branch into the caller's return address.
func (c *arm64Compiler) compileReturnFunction() error {
	// Release all the registers as our calling convention requires the caller-save.
	if err := c.compileReleaseAllRegistersToStack(); err != nil {
		return err
	}

	tmpRegs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		return fmt.Errorf("BUG: all the registers should be free at this point")
	}

	// Alias for readability.
	callFramePointerReg, callFrameStackTopAddressRegister, tmpReg := tmpRegs[0], tmpRegs[1], tmpRegs[2]

	// First we decrement the callframe stack pointer.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset, callFramePointerReg)
	c.compileConstToRegisterInstruction(arm64.ASUBS, 1, callFramePointerReg)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD, callFramePointerReg, reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset)

	// Next we compare the decremented call frame stack pointer with zero.
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, callFramePointerReg, zeroRegister)

	// If the values are identical, we return back to the Go code with returned status.
	brIfNotEqual := c.compilelBranchInstruction(arm64.ABNE)
	if err := c.compileExitFromNativeCode(jitCallStatusCodeReturned); err != nil {
		return err
	}

	// Otherwise, we have to jump to the caller's return address.
	c.setBranchTargetOnNext(brIfNotEqual)

	// First, we have to calculate the caller callFrame's absolute address to aquire the return address.
	//
	// "tmpReg = &ce.callFrameStack[0]"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackElement0AddressOffset,
		tmpReg,
	)
	// "callFrameStackTopAddressRegister = tmpReg + callFramePointerReg << ${callFrameDataSizeMostSignificantSetBit}"
	c.compileAddInstructionWithLeftShiftedRegister(
		callFramePointerReg, callFrameDataSizeMostSignificantSetBit,
		tmpReg,
		callFrameStackTopAddressRegister,
	)

	// At this point, we have
	//
	//      [......., ra.caller, rb.caller, rc.caller, _, ra.current, rb.current, rc.current, _, ...]  <- call frame stack's data region (somewhere in the memory)
	//                                                  ^
	//                               callFrameStackTopAddressRegister
	//                   (absolute address of &callFrameStack[ce.callFrameStackPointer])
	//
	// where:
	//      ra.* = callFrame.returnAddress
	//      rb.* = callFrame.returnStackBasePointer
	//      rc.* = callFrame.compiledFunction
	//      _  = callFrame's padding (see comment on callFrame._ field.)
	//
	// What we have to do in the following is that
	//   1) Set ce.valueStackContext.stackBasePointer to the value on "rb.caller".
	//   2) Jump into the address of "ra.caller".

	// 1) Set ce.valueStackContext.stackBasePointer to the value on "rb.caller".
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		// "rb.caller" is below the top address.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset),
		tmpReg)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		tmpReg,
		reservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset)

	// 2) Branch into the address of "ra.caller".
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		// "rb.caller" is below the top address.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnAddressOffset),
		tmpReg)
	c.compileUnconditionalBranchToAddressOnRegister(tmpReg)

	c.locationStack.markRegisterUnused(tmpRegs...)
	return nil
}

// compileExitFromNativeCode adds instructions to give the control back to ce.exec with the given status code.
func (c *arm64Compiler) compileExitFromNativeCode(status jitCallStatusCode) error {
	// Write the current stack pointer to the ce.stackPointer.
	c.compileConstToRegisterInstruction(arm64.AMOVD, int64(c.locationStack.sp), reservedRegisterForTemporary)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD, reservedRegisterForTemporary, reservedRegisterForCallEngine,
		callEngineValueStackContextStackPointerOffset)

	if status != 0 {
		c.compileConstToRegisterInstruction(arm64.AMOVW, int64(status), reservedRegisterForTemporary)
		c.compileRegisterToMemoryInstruction(arm64.AMOVW, reservedRegisterForTemporary, reservedRegisterForCallEngine, callEngineExitContextJITCallStatusCodeOffset)
	} else {
		// If the status == 0, we use zero register to store zero.
		c.compileRegisterToMemoryInstruction(arm64.AMOVW, zeroRegister, reservedRegisterForCallEngine, callEngineExitContextJITCallStatusCodeOffset)
	}

	// The return address to the Go code is stored in archContext.jitReturnAddress which
	// is embedded in ce. We load the value to the tmpRegister, and then
	// invoke RET with that register.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForCallEngine, callEngineArchContextJITCallReturnAddressOffset, reservedRegisterForTemporary)

	ret := c.newProg()
	ret.As = obj.ARET
	ret.To.Type = obj.TYPE_REG
	ret.To.Reg = reservedRegisterForTemporary
	c.addInstruction(ret)
	return nil
}

// compileHostFunction implements compiler.compileHostFunction for the arm64 architecture.
func (c *arm64Compiler) compileHostFunction(address wasm.FunctionIndex) error {
	// The assembler skips the first instruction so we intentionally add NOP here.
	// TODO: delete after #233
	c.compileNOP()

	// First we must update the location stack to reflect the number of host function inputs.
	c.pushFunctionParams()

	if err := c.compileCallGoFunction(jitCallStatusCodeCallHostFunction, address); err != nil {
		return err
	}
	return c.compileReturnFunction()
}

// setLocationStack sets the given valueLocationStack to .locationStack field,
// while allowing us to track valueLocationStack.stackPointerCeil across multiple stacks.
// This is called when we branch into different block.
func (c *arm64Compiler) setLocationStack(newStack *valueLocationStack) {
	if c.stackPointerCeil < c.locationStack.stackPointerCeil {
		c.stackPointerCeil = c.locationStack.stackPointerCeil
	}
	c.locationStack = newStack
}

// arm64Compiler implements compiler.arm64Compiler for the arm64 architecture.
func (c *arm64Compiler) compileLabel(o *wazeroir.OperationLabel) (skipThisLabel bool) {
	labelKey := o.Label.String()
	labelInfo := c.label(labelKey)

	// If initialStack is not set, that means this label has never been reached.
	if labelInfo.initialStack == nil {
		skipThisLabel = true
		return
	}

	// We use NOP as a beginning of instructions in a label.
	// This should be eventually optimized out by assembler.
	labelBegin := c.compileNOP()

	// Save the instructions so that backward branching
	// instructions can branch to this label.
	labelInfo.initialInstruction = labelBegin

	// Set the initial stack.
	c.setLocationStack(labelInfo.initialStack)

	// Invoke callbacks to notify the forward branching
	// instructions can properly branch to this label.
	for _, cb := range labelInfo.labelBeginningCallbacks {
		cb(labelBegin)
	}
	return false
}

// compileUnreachable implements compiler.compileUnreachable for the arm64 architecture.
func (c *arm64Compiler) compileUnreachable() error {
	return c.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
}

// compileSwap implements compiler.compileSwap for the arm64 architecture.
func (c *arm64Compiler) compileSwap(o *wazeroir.OperationSwap) error {
	x := c.locationStack.peek()
	y := c.locationStack.stack[int(c.locationStack.sp)-1-o.Depth] // Depth is relative to the last stack value

	if err := c.compileEnsureOnGeneralPurposeRegister(x); err != nil {
		return err
	}
	if err := c.compileEnsureOnGeneralPurposeRegister(y); err != nil {
		return err
	}

	x.register, y.register = y.register, x.register
	return nil
}

// Only used in test, but define this in the main file as sometimes
// we need to call this from the main code when debugging.
//nolint:unused
func (c *arm64Compiler) undefined() {
	ud := c.newProg()
	ud.As = obj.AUNDEF
	c.addInstruction(ud)
}

// compileGlobalGet implements compiler.compileGlobalGet for the arm64 architecture.
func (c *arm64Compiler) compileGlobalGet(o *wazeroir.OperationGlobalGet) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	// Get the address of globals[index] into intReg.
	intReg, err := c.compileReadGlobalAddress(o.Index)
	if err != nil {
		return err
	}

	var intMov, floatMov obj.As = obj.ANOP, obj.ANOP
	switch c.f.ModuleInstance.Globals[o.Index].Type.ValType {
	case wasm.ValueTypeI32:
		intMov = arm64.AMOVWU
	case wasm.ValueTypeI64:
		intMov = arm64.AMOVD
	case wasm.ValueTypeF32:
		intMov = arm64.AMOVWU
		floatMov = arm64.AFMOVS
	case wasm.ValueTypeF64:
		intMov = arm64.AMOVD
		floatMov = arm64.AFMOVD
	}

	// "intReg = [intReg + globalInstanceValueOffset] (== globals[index].Val)"
	c.compileMemoryToRegisterInstruction(
		intMov,
		intReg, globalInstanceValueOffset,
		intReg,
	)

	// If the value type is float32 or float64, we have to move the value
	// further into the float register.
	resultReg := intReg
	if floatMov != obj.ANOP {
		resultReg, err = c.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
		c.compileRegisterToRegisterInstruction(floatMov, intReg, resultReg)
	}

	c.locationStack.pushValueLocationOnRegister(resultReg)
	return nil
}

// compileGlobalSet implements compiler.compileGlobalSet for the arm64 architecture.
func (c *arm64Compiler) compileGlobalSet(o *wazeroir.OperationGlobalSet) error {
	val := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	globalInstanceAddressRegister, err := c.compileReadGlobalAddress(o.Index)
	if err != nil {
		return err
	}

	var mov obj.As
	switch c.f.ModuleInstance.Globals[o.Index].Type.ValType {
	case wasm.ValueTypeI32:
		mov = arm64.AMOVWU
	case wasm.ValueTypeI64:
		mov = arm64.AMOVD
	case wasm.ValueTypeF32:
		mov = arm64.AFMOVS
	case wasm.ValueTypeF64:
		mov = arm64.AFMOVD
	}

	// At this point "globalInstanceAddressRegister = globals[index]".
	// Therefore, this means "globals[index].Val = val.register"
	c.compileRegisterToMemoryInstruction(
		mov,
		val.register,
		globalInstanceAddressRegister, globalInstanceValueOffset,
	)

	c.markRegisterUnused(val.register)
	return nil
}

// compileReadGlobalAddress adds instructions to store the absolute address of the global instance at globalIndex into a register
func (c *arm64Compiler) compileReadGlobalAddress(globalIndex uint32) (destinationRegister int16, err error) {
	// TODO: rethink about the type used in store `globals []*GlobalInstance`.
	// If we use `[]GlobalInstance` instead, we could reduce one MOV instruction here.

	destinationRegister, err = c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return
	}

	// "destinationRegister = globalIndex * 8"
	c.compileConstToRegisterInstruction(
		// globalIndex is an index to []*GlobalInstance, therefore
		// we have to multiply it by the size of *GlobalInstance == the pointer size == 8.
		arm64.AMOVD, int64(globalIndex)*8, destinationRegister,
	)

	// "reservedRegisterForTemporary = &globals[0]"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineModuleContextGlobalElement0AddressOffset,
		reservedRegisterForTemporary,
	)

	// "destinationRegister = [reservedRegisterForTemporary + destinationRegister] (== globals[globalIndex])".
	c.compileMemoryWithRegisterOffsetToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForTemporary, destinationRegister,
		destinationRegister,
	)
	return
}

// compileBr implements compiler.compileBr for the arm64 architecture.
func (c *arm64Compiler) compileBr(o *wazeroir.OperationBr) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()
	return c.compileBranchInto(o.Target)
}

// compileBrIf implements compiler.compileBrIf for the arm64 architecture.
func (c *arm64Compiler) compileBrIf(o *wazeroir.OperationBrIf) error {
	cond := c.locationStack.pop()

	conditionalBR := c.newProg()
	conditionalBR.To.Type = obj.TYPE_BRANCH
	if cond.onConditionalRegister() {
		// If the cond is on a conditional register, it corresponds to one of "conditonal codes"
		// https://developer.arm.com/documentation/dui0801/a/Condition-Codes/Condition-code-suffixes
		// Here we represent the conditional codes by using arm64.COND_** registers, and that means the
		// conditional jump can be performed if we use arm64.AB**.
		// For example, if we have arm64.COND_EQ on cond, that means we performed compileEq right before
		// this compileBrIf and BrIf can be achieved by arm64.ABEQ.
		switch cond.conditionalRegister {
		case arm64.COND_EQ:
			conditionalBR.As = arm64.ABEQ
		case arm64.COND_NE:
			conditionalBR.As = arm64.ABNE
		case arm64.COND_HS:
			conditionalBR.As = arm64.ABHS
		case arm64.COND_LO:
			conditionalBR.As = arm64.ABLO
		case arm64.COND_MI:
			conditionalBR.As = arm64.ABMI
		case arm64.COND_HI:
			conditionalBR.As = arm64.ABHI
		case arm64.COND_LS:
			conditionalBR.As = arm64.ABLS
		case arm64.COND_GE:
			conditionalBR.As = arm64.ABGE
		case arm64.COND_LT:
			conditionalBR.As = arm64.ABLT
		case arm64.COND_GT:
			conditionalBR.As = arm64.ABGT
		case arm64.COND_LE:
			conditionalBR.As = arm64.ABLE
		default:
			// BUG: This means that we use the cond.conditionalRegister somewhere in this file,
			// but not covered in switch ^. That shouldn't happen.
			return fmt.Errorf("unsupported condition for br_if: %v", cond.conditionalRegister)
		}
	} else {
		// If the value is not on the conditional register, we compare the value with the zero register,
		// and then do the conditional BR if the value does't equal zero.
		if err := c.compileEnsureOnGeneralPurposeRegister(cond); err != nil {
			return err
		}
		// Compare the value with zero register. Note that the value is ensured to be i32 by function validation phase,
		// so we use CMPW (32-bit compare) here.
		c.compileTwoRegistersToNoneInstruction(arm64.ACMPW, cond.register, zeroRegister)
		conditionalBR.As = arm64.ABNE

		c.markRegisterUnused(cond.register)
	}

	c.addInstruction(conditionalBR)

	// Emit the code for branching into else branch.
	// We save and clone the location stack because we might end up modifying it inside of branchInto,
	// and we have to avoid affecting the code generation for Then branch afterwards.
	saved := c.locationStack
	c.setLocationStack(saved.clone())
	if err := c.compileDropRange(o.Else.ToDrop); err != nil {
		return err
	}
	if err := c.compileBranchInto(o.Else.Target); err != nil {
		return err
	}

	// Now ready to emit the code for branching into then branch.
	// Retrieve the original value location stack so that the code below wont'be affected by the Else branch ^^.
	c.setLocationStack(saved)
	// We branch into here from the original conditional BR (conditionalBR).
	c.setBranchTargetOnNext(conditionalBR)
	if err := c.compileDropRange(o.Then.ToDrop); err != nil {
		return err
	}
	return c.compileBranchInto(o.Then.Target)
}

func (c *arm64Compiler) compileBranchInto(target *wazeroir.BranchTarget) error {
	if target.IsReturnTarget() {
		return c.compileReturnFunction()
	} else {
		labelKey := target.String()
		if c.ir.LabelCallers[labelKey] > 1 {
			// We can only re-use register state if when there's a single call-site.
			// Release existing values on registers to the stack if there's multiple ones to have
			// the consistent value location state at the beginning of label.
			if err := c.compileReleaseAllRegistersToStack(); err != nil {
				return err
			}
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		targetLabel := c.label(labelKey)
		if targetLabel.initialStack == nil {
			targetLabel.initialStack = c.locationStack.clone()
		}

		br := c.compilelBranchInstruction(obj.AJMP)
		c.assignBranchTarget(labelKey, br)
		return nil
	}
}

// assignBranchTarget assigns the given label's initial instruction to the destination of br.
func (c *arm64Compiler) assignBranchTarget(labelKey string, br *obj.Prog) {
	target := c.label(labelKey)
	if target.initialInstruction != nil {
		br.To.SetTarget(target.initialInstruction)
	} else {
		// This case, the target label hasn't been compiled yet, so we append the callback and assign
		// the target instruction when compileLabel is called for the label.
		target.labelBeginningCallbacks = append(target.labelBeginningCallbacks, func(labelInitialInstruction *obj.Prog) {
			br.To.SetTarget(labelInitialInstruction)
		})
	}
}

// compileBrTable implements compiler.compileBrTable for the arm64 architecture.
func (c *arm64Compiler) compileBrTable(o *wazeroir.OperationBrTable) error {
	// If the operation only consists of the default target, we branch into it and return early.
	if len(o.Targets) == 0 {
		loc := c.locationStack.pop()
		if loc.onRegister() {
			c.markRegisterUnused(loc.register)
		}
		if err := c.compileDropRange(o.Default.ToDrop); err != nil {
			return err
		}
		return c.compileBranchInto(o.Default.Target)
	}

	index := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(index); err != nil {
		return err
	}

	if isZeroRegister(index.register) {
		reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}
		index.setRegister(reg)
		c.markRegisterUsed(reg)

		// Zero the value on a picked register.
		c.compileRegisterToRegisterInstruction(arm64.AMOVD, zeroRegister, reg)
	}

	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// Load the branch table's length.
	// "tmpReg = len(o.Targets)"
	c.compileConstToRegisterInstruction(arm64.AMOVW, int64(len(o.Targets)), tmpReg)
	// Compare the length with offest.
	c.compileTwoRegistersToNoneInstruction(arm64.ACMPW, tmpReg, index.register)
	// If the value exceeds the length, we will branch into the default target (corresponding to len(o.Targets) index).
	brDefaultIndex := c.compilelBranchInstruction(arm64.ABLO)
	c.compileRegisterToRegisterInstruction(arm64.AMOVW, tmpReg, index.register)
	c.setBranchTargetOnNext(brDefaultIndex)

	// We prepare the static data which holds the offset of
	// each target's first instruction (incl. default)
	// relative to the beginning of label tables.
	//
	// For example, if we have targets=[L0, L1] and default=L_DEFAULT,
	// we emit the the code like this at [Emit the code for each targets and default branch] below.
	//
	// L0:
	//  0x123001: XXXX, ...
	//  .....
	// L1:
	//  0x123005: YYY, ...
	//  .....
	// L_DEFAULT:
	//  0x123009: ZZZ, ...
	//
	// then offsetData becomes like [0x0, 0x5, 0x8].
	// By using this offset list, we could jump into the label for the index by
	// "jmp offsetData[index]+0x123001" and "0x123001" can be acquired by "LEA"
	// instruction.
	//
	// Note: We store each offset of 32-bit unsigned integer as 4 consecutive bytes. So more precisely,
	// the above example's offsetData would be [0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0].
	//
	// Note: this is similar to how GCC implements Switch statements in C.
	offsetData := make([]byte, 4*(len(o.Targets)+1))
	c.addStaticData(offsetData)

	// "tmpReg = &offsetData[0]"
	c.compileConstToRegisterInstruction(
		arm64.AMOVD,
		// Note: this should be modified to support Clone() functionality per #179.
		int64(uintptr(unsafe.Pointer(&offsetData[0]))),
		tmpReg,
	)

	// "index.register = tmpReg + (index.register << 2) (== &offsetData[offset])"
	c.compileAddInstructionWithLeftShiftedRegister(index.register, 2, tmpReg, index.register)

	// "index.regsetr = *index.reigier (== offsetData[offset])"
	c.compileMemoryToRegisterInstruction(arm64.AMOVW, index.register, 0, index.register)

	// Now we read the address of the beginning of the jump table.
	// In the above example, this corresponds to reading the address of 0x123001.
	c.compileReadInstructionAddress(obj.AJMP, tmpReg)

	// Now we have the address of L0 in tmp register, and the offset to the target label in the index.register.
	// So we could achieve the br_table jump by adding them and jump into the resulting address.
	c.compileRegisterToRegisterInstruction(arm64.AADD, tmpReg, index.register)

	c.compileUnconditionalBranchToAddressOnRegister(index.register)

	// We no longer need the index's register, so mark it unused.
	c.markRegisterUnused(index.register)

	// [Emit the code for each targets and default branch]
	labelInitialInstructions := make([]*obj.Prog, len(o.Targets)+1)
	saved := c.locationStack
	for i := range labelInitialInstructions {
		// Emit the initial instruction of each target where
		// we use NOP as we don't yet know the next instruction in each label.
		init := c.compileNOP()
		labelInitialInstructions[i] = init

		var locationStack *valueLocationStack
		var target *wazeroir.BranchTargetDrop
		if i < len(o.Targets) {
			target = o.Targets[i]
			// Clone the location stack so the branch-specific code doesn't
			// affect others.
			locationStack = saved.clone()
		} else {
			target = o.Default
			// If this is the default branch, we use the original one
			// as this is the last code in this block.
			locationStack = saved
		}
		c.setLocationStack(locationStack)
		if err := c.compileDropRange(target.ToDrop); err != nil {
			return err
		}
		if err := c.compileBranchInto(target.Target); err != nil {
			return err
		}
	}

	c.afterAssembleCallback = append(c.afterAssembleCallback, func(code []byte) error {
		// Build the offset table for each target including default one.
		base := labelInitialInstructions[0].Pc // This corresponds to the L0's address in the example.
		for i, nop := range labelInitialInstructions {
			if uint64(nop.Pc)-uint64(base) >= math.MaxUint32 {
				// TODO: this happens when users try loading an extremely large webassembly binary
				// which contains a br_table statement with approximately 4294967296 (2^32) targets.
				// We would like to support that binary, but realistically speaking, that kind of binary
				// could result in more than ten giga bytes of native JITed code where we have to care about
				// huge stacks whose height might exceed 32-bit range, and such huge stack doesn't work with the
				// current implementation.
				return fmt.Errorf("too large br_table")
			}
			// We store the offset from the beiggning of the L0's initial instruction.
			binary.LittleEndian.PutUint32(offsetData[i*4:(i+1)*4], uint32(nop.Pc)-uint32(base))
		}
		return nil
	})
	return nil
}

// compileCall implements compiler.compileCall for the arm64 architecture.
func (c *arm64Compiler) compileCall(o *wazeroir.OperationCall) error {
	target := c.f.ModuleInstance.Functions[o.FunctionIndex]
	return c.compileCallImpl(target.Index, nilRegister, target.FunctionType.Type)
}

// compileCallImpl implements compiler.compileCall and compiler.compileCallIndirect for the arm64 architecture.
func (c *arm64Compiler) compileCallImpl(addr wasm.FunctionIndex, indexRegister int16, functype *wasm.FunctionType) error {
	// Release all the registers as our calling convention requires the caller-save.
	if err := c.compileReleaseAllRegistersToStack(); err != nil {
		return err
	}

	freeRegisters, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 5)
	if !found {
		return fmt.Errorf("BUG: all registers except indexReg should be free at this point")
	}
	c.locationStack.markRegisterUsed(freeRegisters...)

	// Alias for readability.
	callFrameStackPointerRegister, callFrameStackTopAddressRegister, compiledFunctionIndexRegister, oldStackBasePointer,
		tmp := freeRegisters[0], freeRegisters[1], freeRegisters[2], freeRegisters[3], freeRegisters[4]

	// First, we have to check if we need to grow the callFrame stack.
	//
	// "callFrameStackPointerRegister = ce.callFrameStackPointer"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		callFrameStackPointerRegister)
	// "tmp = len(ce.callFrameStack)"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackLenOffset,
		tmp,
	)
	// Compare tmp(len(ce.callFrameStack)) with callFrameStackPointerRegister(ce.callFrameStackPointer).
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, tmp, callFrameStackPointerRegister)
	brIfCallFrameStackOK := c.compilelBranchInstruction(arm64.ABNE)

	// If these values equal, we need to grow the callFrame stack.
	// For call_indirect, we need to push the value back to the register.
	if !isNilRegister(indexRegister) {
		// If we need to get the target funcaddr from register (call_indirect case), we must save it before growing callframe stack,
		// as the register is not saved across function calls.
		savedOffsetLocation := c.locationStack.pushValueLocationOnRegister(indexRegister)
		if err := c.compileReleaseRegisterToStack(savedOffsetLocation); err != nil {
			return err
		}
	}

	if err := c.compileCallGoFunction(jitCallStatusCodeCallBuiltInFunction, builtinFunctionIndexGrowCallFrameStack); err != nil {
		return err
	}

	// For call_indirect, we need to push the value back to the register.
	if !isNilRegister(indexRegister) {
		// Since this is right after callGoFunction, we have to initialize the stack base pointer
		// to properly load the value on memory stack.
		c.compileReservedStackBasePointerRegisterInitialization()

		savedOffsetLocation := c.locationStack.pop()
		savedOffsetLocation.setRegister(indexRegister)
		if err := c.compileLoadValueOnStackToRegister(savedOffsetLocation); err != nil {
			return err
		}
	}

	// On the function return, we again have to set ce.callFrameStackPointer into callFrameStackPointerRegister.
	// "callFrameStackPointerRegister = ce.callFrameStackPointer"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		callFrameStackPointerRegister)

	// Now that we ensured callFrameStack length is enough.
	c.setBranchTargetOnNext(brIfCallFrameStackOK)
	c.compileCalcCallFrameStackTopAddress(callFrameStackPointerRegister, callFrameStackTopAddressRegister)

	// At this point, we have:
	//
	//    [..., ra.current, rb.current, rc.current, _, ra.next, rb.next, rc.next, ...]  <- call frame stack's data region (somewhere in the memory)
	//                                               ^
	//                              callFrameStackTopAddressRegister
	//                (absolute address of &callFrame[ce.callFrameStackPointer]])
	//
	// where:
	//      ra.* = callFrame.returnAddress
	//      rb.* = callFrame.returnStackBasePointer
	//      rc.* = callFrame.compiledFunction
	//      _  = callFrame's padding (see comment on callFrame._ field.)
	//
	// In the following comment, we use the notations in the above example.
	//
	// What we have to do in the following is that
	//   1) Set rb.current so that we can return back to this function properly.
	//   2) Set ce.valueStackContext.stackBasePointer for the next function.
	//   3) Set rc.next to specify which function is executed on the current call frame (needs to make Go function calls).
	//   4) Set ra.current so that we can return back to this function properly.

	// 1) Set rb.current so that we can return back to this function properly.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset,
		oldStackBasePointer)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		oldStackBasePointer,
		// "rb.current" is BELOW the top address. See the above example for detail.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset))

	// 2) Set ce.valueStackContext.stackBasePointer for the next function.
	//
	// At this point, oldStackBasePointer holds the old stack base pointer. We could get the new frame's
	// stack base pointer by "old stack base pointer + old stack pointer - # of function params"
	// See the comments in ce.pushCallFrame which does exactly the same calculation in Go.
	if offset := int64(c.locationStack.sp) - int64(len(functype.Params)); offset > 0 {
		c.compileConstToRegisterInstruction(arm64.AADD, offset, oldStackBasePointer)
		c.compileRegisterToMemoryInstruction(arm64.AMOVD,
			oldStackBasePointer,
			reservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset)
	}

	// 3) Set rc.next to specify which function is executed on the current call frame.
	//
	// First, we read the address of the first item of ce.compiledFunctions slice (= &ce.compiledFunctions[0])
	// into tmp.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCompiledFunctionsElement0AddressOffset,
		tmp)

	// Next, read the index of the target function (= &ce.compiledFunctions[offset])
	// into compiledFunctionIndexRegister.
	if isNilRegister(indexRegister) {
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			tmp, int64(addr)*8, // * 8 because the size of *compiledFunction equals 8 bytes.
			compiledFunctionIndexRegister)
	} else {
		// Shift indexRegister by 3 because the size of *compiledFunction equals 8 bytes.
		c.compileConstToRegisterInstruction(arm64.ALSLW, 3, indexRegister)
		c.compileMemoryWithRegisterOffsetToRegisterInstruction(
			arm64.AMOVD,
			tmp, indexRegister,
			compiledFunctionIndexRegister,
		)
	}

	// Finally, we are ready to write the address of the target function's *compiledFunction into the new callframe.
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		compiledFunctionIndexRegister,
		callFrameStackTopAddressRegister, callFrameCompiledFunctionOffset)

	// 4) Set ra.current so that we can return back to this function properly.
	//
	// First, Get the return address into the tmp.
	c.compileReadInstructionAddress(obj.AJMP, tmp)
	// Then write the address into the callframe.
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		tmp,
		// "ra.current" is BELOW the top address. See the above example for detail.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnAddressOffset),
	)

	// Everthing is done to make function call now.
	// We increment the callframe stack pointer.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		tmp)
	c.compileConstToRegisterInstruction(arm64.AADD, 1, tmp)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		tmp,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset)

	// Then, br into the target function's initial address.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		compiledFunctionIndexRegister, compiledFunctionCodeInitialAddressOffset,
		tmp)

	c.compileUnconditionalBranchToAddressOnRegister(tmp)

	// All the registers used are temporary so we mark them unused.
	c.markRegisterUnused(freeRegisters...)

	// We consumed the function parameters from the stack after call.
	for i := 0; i < len(functype.Params); i++ {
		c.locationStack.pop()
	}

	// Also, the function results were pushed by the call.
	for _, t := range functype.Results {
		loc := c.locationStack.pushValueLocationOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
	}

	// On the function return, we initialize the state for this function.
	c.compileReservedStackBasePointerRegisterInitialization()

	if err := c.compileModuleContextInitialization(); err != nil {
		return err
	}

	c.compileReservedMemoryRegisterInitialization()
	return nil
}

// compileCalcCallFrameStackTopAddress adds instructions to set the absolute address of
// ce.callFrameStack[callFrameStackPointerRegister] into destinationRegister.
func (c *arm64Compiler) compileCalcCallFrameStackTopAddress(callFrameStackPointerRegister, destinationRegister int16) {
	// "destinationRegister = &ce.callFrameStack[0]"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackElement0AddressOffset,
		destinationRegister)
	// "destinationRegister += callFrameStackPointerRegister << $callFrameDataSizeMostSignificantSetBit"
	c.compileAddInstructionWithLeftShiftedRegister(
		callFrameStackPointerRegister, callFrameDataSizeMostSignificantSetBit,
		destinationRegister,
		destinationRegister,
	)
}

// compileReadInstructionAddress adds an ADR instruction to set the absolute address of "target instruction"
// into destinationRegister. "target instruction" is specified by beforeTargetInst argument and
// the target is determined by "the instruction right after beforeTargetInst type".
//
// For example, if beforeTargetInst == RET and we have the instruction sequence like
// ADR -> X -> Y -> ... -> RET -> MOV, then the ADR instruction emitted by this function set the absolute
// address of MOV instruction into the destination register.
func (c *arm64Compiler) compileReadInstructionAddress(beforeTargetInst obj.As, destinationRegister int16) {
	// Emit ADR instruction to read the specified instruction's absolute address.
	// Note: we cannot emit the "ADR REG, $(target's offset from here)" due to the
	// incapability of the assembler. Instead, we emit "ADR REG, ." meaning that
	// "reading the current program counter" = "reading the absolute address of this ADR instruction".
	// And then, after compilation phase, we directly edit the native code slice so that
	// it can properly read the target instruction's absolute address.
	readAddress := c.newProg()
	readAddress.As = arm64.AADR
	readAddress.From.Type = obj.TYPE_BRANCH
	readAddress.To.Type = obj.TYPE_REG
	readAddress.To.Reg = destinationRegister
	c.addInstruction(readAddress)

	// Setup the callback to modify the instruction bytes after compilation.
	// Note: this is the closure over readAddress (*obj.Prog).
	c.afterAssembleCallback = append(c.afterAssembleCallback, func(code []byte) error {
		// Find the target instruction.
		target := readAddress
		for target != nil {
			if target.As == beforeTargetInst {
				// At this point, target is the instruction right before the target instruction.
				// Thus, advance one more time to make target the target instruction.
				target = target.Link
				break
			}
			target = target.Link
		}

		if target == nil {
			return fmt.Errorf("BUG: target instruction not found for read instruction address")
		}

		offset := target.Pc - readAddress.Pc
		if offset > math.MaxUint8 {
			// We could support up to 20-bit integer, but byte should be enough for our impl.
			// If the necessity comes up, we could fix the below to support larger offsets.
			return fmt.Errorf("BUG: too large offset for read")
		}

		// Now ready to write an offset byte.
		v := byte(offset)
		// arm64 has 4-bytes = 32-bit fixed-length instruction.
		adrInstructionBytes := code[readAddress.Pc : readAddress.Pc+4]
		// According to the binary format of ADR instruction in arm64:
		// https://developer.arm.com/documentation/ddi0596/2021-12/Base-Instructions/ADR--Form-PC-relative-address-?lang=en
		//
		// The 0 to 1 bits live on 29 to 30 bits of the instruction.
		adrInstructionBytes[3] |= (v & 0b00000011) << 5
		// The 2 to 4 bits live on 5 to 7 bits of the instruction.
		adrInstructionBytes[0] |= (v & 0b00011100) << 3
		// The 5 to 7 bits live on 8 to 10 bits of the instruction.
		adrInstructionBytes[1] |= (v & 0b11100000) >> 5
		return nil
	})
}

// compileCallIndirect implements compiler.compileCallIndirect for the arm64 architecture.
func (c *arm64Compiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	offset := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(offset); err != nil {
		return err
	}

	if isZeroRegister(offset.register) {
		reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}
		offset.setRegister(reg)
		c.markRegisterUsed(reg)

		// Zero the value on a picked register.
		c.compileRegisterToRegisterInstruction(arm64.AMOVD, zeroRegister, reg)
	}

	tmp, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we need to check if the offset doesn't exceed the length of table.
	// "tmp = len(table)"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineModuleContextTableSliceLenOffset,
		tmp,
	)
	// "cmp tmp, offset"
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, tmp, offset.register)

	// If it exceeds len(table), we exit the execution.
	brIfOffsetOK := c.compilelBranchInstruction(arm64.ABLO)
	if err := c.compileExitFromNativeCode(jitCallStatusCodeInvalidTableAccess); err != nil {
		return err
	}

	// Otherwise, we proceed to do function type check.
	c.setBranchTargetOnNext(brIfOffsetOK)

	// We need to obtains the absolute address of table element.
	// "tmp = &table[0]"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineModuleContextTableElement0AddressOffset,
		tmp,
	)
	// "offset = tmp + (offset << 4) (== &table[offset])"
	c.compileAddInstructionWithLeftShiftedRegister(
		offset.register, 4,
		tmp,
		offset.register,
	)

	// Check if table[offset].TypeID == targetFunctionType.
	targetFunctionType := c.f.ModuleInstance.Types[o.TypeIndex]
	// "tmp = table[offset].TypeID"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD, offset.register, tableElementFunctionTypeIDOffset,
		tmp,
	)
	// "reservedRegisterForTemporary = targetFunctionType.TypeID"
	c.compileConstToRegisterInstruction(arm64.AMOVD, int64(targetFunctionType.TypeID), reservedRegisterForTemporary)
	// Compare these two values, and if they equal, we are ready to make function call.
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, tmp, reservedRegisterForTemporary)
	brIfTypeMatched := c.compilelBranchInstruction(arm64.ABEQ)

	// Otherwise, we have to exit the execution with either jitCallStatusCodeTypeMismatchOnIndirectCall or jitCallStatusCodeInvalidTableAccess.
	{
		// We exit with jitCallStatusCodeInvalidTableAccess if the targetFunctionType.TypeID equals the uninitialized one (wasm.UninitializedTableElementTypeID).
		c.compileConstToRegisterInstruction(arm64.AMOVD, int64(wasm.UninitializedTableElementTypeID), reservedRegisterForTemporary)
		c.compileTwoRegistersToNoneInstruction(arm64.ACMPW, tmp, reservedRegisterForTemporary)

		brIfInitizlied := c.compilelBranchInstruction(arm64.ABNE)
		if err := c.compileExitFromNativeCode(jitCallStatusCodeInvalidTableAccess); err != nil {
			return err
		}

		// Otherwise exit with jitCallStatusCodeTypeMismatchOnIndirectCall.
		c.setBranchTargetOnNext(brIfInitizlied)
		if err := c.compileExitFromNativeCode(jitCallStatusCodeTypeMismatchOnIndirectCall); err != nil {
			return err
		}
	}

	c.setBranchTargetOnNext(brIfTypeMatched)

	// Now all checks passed, so read the target's function address, and make call.
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVW,
		offset.register, tableElementFunctionIndexOffset,
		offset.register,
	)

	if err := c.compileCallImpl(0, offset.register, targetFunctionType.Type); err != nil {
		return err
	}

	// The offset register should be marked as un-used as we consumed in the function call.
	c.markRegisterUnused(offset.register)
	return nil
}

// compileDrop implements compiler.compileDrop for the arm64 architecture.
func (c *arm64Compiler) compileDrop(o *wazeroir.OperationDrop) error {
	return c.compileDropRange(o.Range)
}

// compileDropRange is the implementation of compileDrop. See compiler.compileDrop.
func (c *arm64Compiler) compileDropRange(r *wazeroir.InclusiveRange) error {
	if r == nil {
		return nil
	} else if r.Start == 0 {
		// When the drop starts from the top of the stack, mark all registers unused.
		for i := 0; i <= r.End; i++ {
			if loc := c.locationStack.pop(); loc.onRegister() {
				c.markRegisterUnused(loc.register)
			}
		}
		return nil
	}

	// Below, we might end up moving a non-top value first which
	// might result in changing the flag value.
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	// Save the live values because we pop and release values in drop range below.
	liveValues := c.locationStack.stack[c.locationStack.sp-uint64(r.Start) : c.locationStack.sp]
	c.locationStack.sp -= uint64(r.Start)

	// Note: drop target range is inclusive.
	dropNum := r.End - r.Start + 1

	// Then mark all registers used by drop tragets unused.
	for i := 0; i < dropNum; i++ {
		if loc := c.locationStack.pop(); loc.onRegister() {
			c.markRegisterUnused(loc.register)
		}
	}

	for _, live := range liveValues {
		// If the value is on a memory, we have to move it to a register,
		// otherwise the memory location is overriden by other values
		// after this drop instructin.
		if err := c.compileEnsureOnGeneralPurposeRegister(live); err != nil {
			return err
		}
		// Update the runtime memory stack location by pushing onto the location stack.
		c.locationStack.push(live)
	}
	return nil
}

// compileSelect implements compiler.compileSelect for the arm64 architecture.
func (c *arm64Compiler) compileSelect() error {
	cv, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	c.markRegisterUsed(cv.register)

	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) && isZeroRegister(x2.register) {
		// If both values are zero, the result is always zero.
		c.locationStack.pushValueLocationOnRegister(zeroRegister)
		c.markRegisterUnused(cv.register)
		return nil
	}

	// In the following, we emit the code so that x1's register contains the chosen value
	// no matter which of oroginal x1 or x2 is selected.
	//
	// If x1 is currently on zero register, we cannot place the result because
	// "MOV zeroRegister x2.register" results in zeroRegister regardless of the value.
	// So we explicitly assign a general purpuse register to x1 here.
	if isZeroRegister(x1.register) {
		// Mark x2 and cv's regiseters are used so they won't be chosen.
		c.markRegisterUsed(x2.register)
		// Pick the non-zero register for x1.
		x1Reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}
		x1.setRegister(x1Reg)
		// And zero our the picked register.
		c.compileRegisterToRegisterInstruction(arm64.AMOVD, zeroRegister, x1Reg)
	}

	// At this point, x1 is non-zero register, and x2 is either general purpuse or zero register.

	c.compileTwoRegistersToNoneInstruction(arm64.ACMPW, zeroRegister, cv.register)
	brIfNotZero := c.compilelBranchInstruction(arm64.ABNE)
	c.addInstruction(brIfNotZero)

	// If cv == 0, we move the value of x2 to the x1.register.
	if x1.registerType() == generalPurposeRegisterTypeInt {
		c.compileRegisterToRegisterInstruction(arm64.AMOVD, x2.register, x1.register)
	} else {
		c.compileRegisterToRegisterInstruction(arm64.AFMOVD, x2.register, x1.register)
	}
	c.locationStack.pushValueLocationOnRegister(x1.register)

	// Otherwise, nothing to do for select.
	c.setBranchTargetOnNext(brIfNotZero)

	// Only x1.register is reused.
	c.markRegisterUnused(cv.register, x2.register)
	return nil
}

// compilePick implements compiler.compilePick for the arm64 architecture.
func (c *arm64Compiler) compilePick(o *wazeroir.OperationPick) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	pickTarget := c.locationStack.stack[c.locationStack.sp-1-uint64(o.Depth)]
	pickedRegister, err := c.allocateRegister(pickTarget.registerType())
	if err != nil {
		return err
	}

	if pickTarget.onRegister() { // Copy the value to the pickedRegister.
		var inst obj.As
		switch pickTarget.registerType() {
		case generalPurposeRegisterTypeInt:
			inst = arm64.AMOVD
		case generalPurposeRegisterTypeFloat:
			inst = arm64.AFMOVD
		}
		c.compileRegisterToRegisterInstruction(inst, pickTarget.register, pickedRegister)
	} else if pickTarget.onStack() {
		// Temporarily assign a register to the pick target, and then load the value.
		pickTarget.setRegister(pickedRegister)
		if err := c.compileLoadValueOnStackToRegister(pickTarget); err != nil {
			return err
		}
		// After the load, we revert the register assignment to the pick target.
		pickTarget.setRegister(nilRegister)
	}

	// Now we have the value of the target on the pickedRegister,
	// so push the location.
	c.locationStack.pushValueLocationOnRegister(pickedRegister)
	return nil
}

// compileAdd implements compiler.compileAdd for the arm64 architecture.
func (c *arm64Compiler) compileAdd(o *wazeroir.OperationAdd) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// Additon can be nop if one of operands is zero.
	if isZeroRegister(x1.register) {
		c.locationStack.pushValueLocationOnRegister(x2.register)
		return nil
	} else if isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(x1.register)
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

	c.compileRegisterToRegisterInstruction(inst, x2.register, x1.register)
	// The result is placed on a register for x1, so record it.
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileSub implements compiler.compileSub for the arm64 architecture.
func (c *arm64Compiler) compileSub(o *wazeroir.OperationSub) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// If both of registers are zeros, this can be nop and push the zero register.
	if isZeroRegister(x1.register) && isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(zeroRegister)
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

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, destinationReg)
	c.locationStack.pushValueLocationOnRegister(destinationReg)
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
		c.locationStack.pushValueLocationOnRegister(zeroRegister)
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

	c.compileRegisterToRegisterInstruction(inst, x2.register, x1.register)
	// The result is placed on a register for x1, so record it.
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileClz implements compiler.compileClz for the arm64 architecture.
func (c *arm64Compiler) compileClz(o *wazeroir.OperationClz) error {
	v, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	if isZeroRegister(v.register) {
		// If the target is zero register, the result is always 32 (or 64 for 64-bits),
		// so we allocate a register and put the const on it.
		reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}
		if o.Type == wazeroir.UnsignedInt32 {
			c.compileConstToRegisterInstruction(arm64.AMOVW, 32, reg)
		} else {
			c.compileConstToRegisterInstruction(arm64.AMOVD, 64, reg)
		}
		c.locationStack.pushValueLocationOnRegister(reg)
		return nil
	}

	reg := v.register
	if o.Type == wazeroir.UnsignedInt32 {
		c.compileRegisterToRegisterInstruction(arm64.ACLZW, reg, reg)
	} else {
		c.compileRegisterToRegisterInstruction(arm64.ACLZ, reg, reg)
	}
	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

// compileCtz implements compiler.compileCtz for the arm64 architecture.
func (c *arm64Compiler) compileCtz(o *wazeroir.OperationCtz) error {
	v, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	reg := v.register
	if isZeroRegister(reg) {
		// If the target is zero register, the result is always 32 (or 64 for 64-bits),
		// so we allocate a register and put the const on it.
		reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}
		if o.Type == wazeroir.UnsignedInt32 {
			c.compileConstToRegisterInstruction(arm64.AMOVW, 32, reg)
		} else {
			c.compileConstToRegisterInstruction(arm64.AMOVD, 64, reg)
		}
		c.locationStack.pushValueLocationOnRegister(reg)
		return nil
	}

	// Since arm64 doesn't have an instruction directly counting trailing zeros,
	// we reverse the bits first, and then do CLZ, which is exactly the same as
	// gcc implements __builtin_ctz for arm64.
	if o.Type == wazeroir.UnsignedInt32 {
		c.compileRegisterToRegisterInstruction(arm64.ARBITW, reg, reg)
		c.compileRegisterToRegisterInstruction(arm64.ACLZW, reg, reg)
	} else {
		c.compileRegisterToRegisterInstruction(arm64.ARBIT, reg, reg)
		c.compileRegisterToRegisterInstruction(arm64.ACLZ, reg, reg)
	}
	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

// compilePopcnt implements compiler.compilePopcnt for the arm64 architecture.
func (c *arm64Compiler) compilePopcnt(o *wazeroir.OperationPopcnt) error {
	v, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	reg := v.register
	if isZeroRegister(reg) {
		c.locationStack.pushValueLocationOnRegister(reg)
		return nil
	}

	freg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	// arm64 doesn't have an instruction for population count on scalar register,
	// so we use the vector one (VCNT).
	// This exactly what the official Go implements bits.OneCount.
	// For example, "func () int { return bits.OneCount(10) }" is compiled as
	//
	//    MOVD    $10, R0
	//    FMOVD   R0, F0
	//    VCNT    V0.B8, V0.B8
	//    VUADDLV V0.B8, V0
	//
	c.compileRegisterToRegisterInstruction(arm64.AFMOVD, reg, freg)
	vreg := simdRegisterForScalarFloatRegister(freg)
	// For how to specify "V0.B8" (SIMD register arrangement), see
	// * https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L172-L177
	// * https://github.com/golang/go/blob/739328c694d5e608faa66d17192f0a59f6e01d04/src/cmd/compile/internal/arm64/ssa.go#L972
	c.compileRegisterToRegisterInstruction(arm64.AVCNT, vreg&31+arm64.REG_ARNG+(arm64.ARNG_8B&15)<<5, vreg&31+arm64.REG_ARNG+(arm64.ARNG_8B&15)<<5)
	c.compileRegisterToRegisterInstruction(arm64.AVUADDLV, vreg&31+arm64.REG_ARNG+(arm64.ARNG_8B&15)<<5, vreg)
	c.compileRegisterToRegisterInstruction(arm64.AFMOVD, freg, reg)

	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

// compileDiv implements compiler.compileDiv for the arm64 architecture.
func (c *arm64Compiler) compileDiv(o *wazeroir.OperationDiv) error {
	dividend, divisor, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// If the divisor is on the zero register, exit from the function deterministically.
	if isZeroRegister(divisor.register) {
		// Push any value so that the subsequent instruction can have a consistent location stack state.
		c.locationStack.pushValueLocationOnStack()
		return c.compileExitFromNativeCode(jitCallStatusIntegerDivisionByZero)
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.SignedTypeUint32:
		inst = arm64.AUDIVW
		if err := c.compileIntegerDivPrecheck(true, false, dividend.register, divisor.register); err != nil {
			return err
		}
	case wazeroir.SignedTypeUint64:
		if err := c.compileIntegerDivPrecheck(false, false, dividend.register, divisor.register); err != nil {
			return err
		}
		inst = arm64.AUDIV
	case wazeroir.SignedTypeInt32:
		if err := c.compileIntegerDivPrecheck(true, true, dividend.register, divisor.register); err != nil {
			return err
		}
		inst = arm64.ASDIVW
	case wazeroir.SignedTypeInt64:
		if err := c.compileIntegerDivPrecheck(false, true, dividend.register, divisor.register); err != nil {
			return err
		}
		inst = arm64.ASDIV
	case wazeroir.SignedTypeFloat32:
		inst = arm64.AFDIVS
	case wazeroir.SignedTypeFloat64:
		inst = arm64.AFDIVD
	}

	c.compileRegisterToRegisterInstruction(inst, divisor.register, dividend.register)

	c.locationStack.pushValueLocationOnRegister(dividend.register)
	return nil
}

// compileIntegerDivPrecheck adds instructions to check if the divisor and dividend are sound for division operation.
// First, this adds instrucitons to check if the divisor equals zero, and if so, exits the function.
// Plus, for signed divisions, check if the result might result in overflow or not.
func (c *arm64Compiler) compileIntegerDivPrecheck(is32Bit, isSigned bool, dividend, divisor int16) error {
	// We check the divisor value equals zero.
	var cmpInst, movInst obj.As
	var minValueOffsetInVM int64
	if is32Bit {
		cmpInst = arm64.ACMPW
		movInst = arm64.AMOVW
		minValueOffsetInVM = callEngineArchContextMinimum32BitSignedIntOffset
	} else {
		cmpInst = arm64.ACMP
		movInst = arm64.AMOVD
		minValueOffsetInVM = callEngineArchContextMinimum64BitSignedIntOffset
	}
	c.compileTwoRegistersToNoneInstruction(cmpInst, zeroRegister, divisor)

	// If it is zero, we exit with jitCallStatusIntegerDivisionByZero.
	brIfDivisorNonZero := c.compilelBranchInstruction(arm64.ABNE)
	if err := c.compileExitFromNativeCode(jitCallStatusIntegerDivisionByZero); err != nil {
		return err
	}

	// Otherwise, we proceed.
	c.setBranchTargetOnNext(brIfDivisorNonZero)

	// If the operation is a signed integer div, we have to do an additional check on overflow.
	if isSigned {
		// For sigined division, we have to have branches for "math.MinInt{32,64} / -1"
		// case which results in the overflow.

		// First, we compare the divisor with -1.
		c.compileConstToRegisterInstruction(movInst, -1, reservedRegisterForTemporary)
		c.compileTwoRegistersToNoneInstruction(cmpInst, reservedRegisterForTemporary, divisor)

		// If they not equal, we skip the following check.
		brIfDivisorNonMinusOne := c.compilelBranchInstruction(arm64.ABNE)

		// Otherwise, we further check if the dividend equals math.MinInt32 or MinInt64.
		c.compileMemoryToRegisterInstruction(
			movInst,
			reservedRegisterForCallEngine, minValueOffsetInVM,
			reservedRegisterForTemporary,
		)
		c.compileTwoRegistersToNoneInstruction(cmpInst, reservedRegisterForTemporary, dividend)

		// If they not equal, we are safe to execute the division.
		brIfDividendNotMinInt := c.compilelBranchInstruction(arm64.ABNE)

		// Otherwise, we raise overflow error.
		if err := c.compileExitFromNativeCode(jitCallStatusIntegerOverflow); err != nil {
			return err
		}

		c.setBranchTargetOnNext(brIfDivisorNonMinusOne, brIfDividendNotMinInt)
	}
	return nil
}

// compileRem implements compiler.compileRem for the arm64 architecture.
func (c *arm64Compiler) compileRem(o *wazeroir.OperationRem) error {
	dividend, divisor, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	dividendReg := dividend.register
	divisorReg := divisor.register

	// If the divisor is on the zero register, exit from the function deterministically.
	if isZeroRegister(divisor.register) {
		// Push any value so that the subsequent instruction can have a consistent location stack state.
		c.locationStack.pushValueLocationOnStack()
		return c.compileExitFromNativeCode(jitCallStatusIntegerDivisionByZero)
	}

	var divInst, msubInst, cmpInst obj.As
	switch o.Type {
	case wazeroir.SignedUint32:
		divInst = arm64.AUDIVW
		msubInst = arm64.AMSUBW
		cmpInst = arm64.ACMPW
	case wazeroir.SignedUint64:
		divInst = arm64.AUDIV
		msubInst = arm64.AMSUB
		cmpInst = arm64.ACMP
	case wazeroir.SignedInt32:
		divInst = arm64.ASDIVW
		msubInst = arm64.AMSUBW
		cmpInst = arm64.ACMPW
	case wazeroir.SignedInt64:
		divInst = arm64.ASDIV
		msubInst = arm64.AMSUB
		cmpInst = arm64.ACMP
	}

	// We check the divisor value equals zero.
	c.compileTwoRegistersToNoneInstruction(cmpInst, zeroRegister, divisorReg)

	// If it is zero, we exit with jitCallStatusIntegerDivisionByZero.
	brIfDivisorNonZero := c.compilelBranchInstruction(arm64.ABNE)
	if err := c.compileExitFromNativeCode(jitCallStatusIntegerDivisionByZero); err != nil {
		return err
	}

	// Othrewise, we proceed.
	c.setBranchTargetOnNext(brIfDivisorNonZero)

	// Temporarily mark them used to allocate a result register while keeping these values.
	c.markRegisterUsed(dividend.register, divisor.register)

	resultReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// arm64 doesn't have an instruction for rem, we use calculate it by two instructions: UDIV (SDIV for signed) and MSUB.
	// This exactly the same code that Clang emits.
	// [input: x0=dividend, x1=divisor]
	// >> UDIV x2, x0, x1
	// >> MSUB x3, x2, x1, x0
	// [result: x2=quotient, x3=remainder]
	//
	c.compileTwoRegistersToRegisterInstruction(divInst, divisorReg, dividendReg, resultReg)
	c.compileTwoRegistersInstruction(msubInst, divisorReg, dividendReg, resultReg, resultReg)

	c.markRegisterUnused(dividend.register, divisor.register)
	c.locationStack.pushValueLocationOnRegister(resultReg)
	return nil
}

// compileAnd implements compiler.compileAnd for the arm64 architecture.
func (c *arm64Compiler) compileAnd(o *wazeroir.OperationAnd) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// If either of the registers x1 or x2 is zero,
	// the result will always be zero.
	if isZeroRegister(x1.register) || isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(zeroRegister)
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
	case wazeroir.UnsignedInt32:
		inst = arm64.AANDW
	case wazeroir.UnsignedInt64:
		inst = arm64.AAND
	}

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, destinationReg)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileOr implements compiler.compileOr for the arm64 architecture.
func (c *arm64Compiler) compileOr(o *wazeroir.OperationOr) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) {
		c.locationStack.pushValueLocationOnRegister(x2.register)
		return nil
	}
	if isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(x1.register)
		return nil
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedInt32:
		inst = arm64.AORRW
	case wazeroir.UnsignedInt64:
		inst = arm64.AORR
	}

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, x1.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileXor implements compiler.compileXor for the arm64 architecture.
func (c *arm64Compiler) compileXor(o *wazeroir.OperationXor) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	// At this point, at least one of x1 or x2 registers is non zero.
	// Choose the non-zero register as destination.
	var destinationReg int16 = x1.register
	if isZeroRegister(x1.register) {
		destinationReg = x2.register
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedInt32:
		inst = arm64.AEORW
	case wazeroir.UnsignedInt64:
		inst = arm64.AEOR
	}

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, destinationReg)
	c.locationStack.pushValueLocationOnRegister(destinationReg)
	return nil
}

// compileShl implements compiler.compileShl for the arm64 architecture.
func (c *arm64Compiler) compileShl(o *wazeroir.OperationShl) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) || isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(x1.register)
		return nil
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedInt32:
		inst = arm64.ALSLW
	case wazeroir.UnsignedInt64:
		inst = arm64.ALSL
	}

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, x1.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileShr implements compiler.compileShr for the arm64 architecture.
func (c *arm64Compiler) compileShr(o *wazeroir.OperationShr) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) || isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(x1.register)
		return nil
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.SignedInt32:
		inst = arm64.AASRW
	case wazeroir.SignedInt64:
		inst = arm64.AASR
	case wazeroir.SignedUint32:
		inst = arm64.ALSRW
	case wazeroir.SignedUint64:
		inst = arm64.ALSR
	}

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, x1.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileRotl implements compiler.compileRotl for the arm64 architecture.
func (c *arm64Compiler) compileRotl(o *wazeroir.OperationRotl) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) || isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(x1.register)
		return nil
	}

	var (
		inst    obj.As
		neginst obj.As
	)

	switch o.Type {
	case wazeroir.UnsignedInt32:
		inst = arm64.ARORW
		neginst = arm64.ANEGW
	case wazeroir.UnsignedInt64:
		inst = arm64.AROR
		neginst = arm64.ANEG
	}

	// Arm64 doesn't have rotate left instruction.
	// The shift amount needs to be converted to a negative number, similar to assembly output of bits.RotateLeft.
	c.compileRegisterToRegisterInstruction(neginst, x2.register, x2.register)

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, x1.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileRotr implements compiler.compileRotr for the arm64 architecture.
func (c *arm64Compiler) compileRotr(o *wazeroir.OperationRotr) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) || isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(x1.register)
		return nil
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedInt32:
		inst = arm64.ARORW
	case wazeroir.UnsignedInt64:
		inst = arm64.AROR
	}

	c.compileTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, x1.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileAbs implements compiler.compileAbs for the arm64 architecture.
func (c *arm64Compiler) compileAbs(o *wazeroir.OperationAbs) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFABSS)
	} else {
		return c.compileSimpleUnop(arm64.AFABSD)
	}
}

// compileNeg implements compiler.compileNeg for the arm64 architecture.
func (c *arm64Compiler) compileNeg(o *wazeroir.OperationNeg) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFNEGS)
	} else {
		return c.compileSimpleUnop(arm64.AFNEGD)
	}
}

// compileCeil implements compiler.compileCeil for the arm64 architecture.
func (c *arm64Compiler) compileCeil(o *wazeroir.OperationCeil) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFRINTPS)
	} else {
		return c.compileSimpleUnop(arm64.AFRINTPD)
	}
}

// compileFloor implements compiler.compileFloor for the arm64 architecture.
func (c *arm64Compiler) compileFloor(o *wazeroir.OperationFloor) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFRINTMS)
	} else {
		return c.compileSimpleUnop(arm64.AFRINTMD)
	}
}

// compileTrunc implements compiler.compileTrunc for the arm64 architecture.
func (c *arm64Compiler) compileTrunc(o *wazeroir.OperationTrunc) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFRINTZS)
	} else {
		return c.compileSimpleUnop(arm64.AFRINTZD)
	}
}

// compileNearest implements compiler.compileNearest for the arm64 architecture.
func (c *arm64Compiler) compileNearest(o *wazeroir.OperationNearest) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFRINTNS)
	} else {
		return c.compileSimpleUnop(arm64.AFRINTND)
	}
}

// compileSqrt implements compiler.compileSqrt for the arm64 architecture.
func (c *arm64Compiler) compileSqrt(o *wazeroir.OperationSqrt) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleUnop(arm64.AFSQRTS)
	} else {
		return c.compileSimpleUnop(arm64.AFSQRTD)
	}
}

// compileMin implements compiler.compileMin for the arm64 architecture.
func (c *arm64Compiler) compileMin(o *wazeroir.OperationMin) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleFloatBinop(arm64.AFMINS)
	} else {
		return c.compileSimpleFloatBinop(arm64.AFMIND)
	}
}

// compileMax implements compiler.compileMax for the arm64 architecture.
func (c *arm64Compiler) compileMax(o *wazeroir.OperationMax) error {
	if o.Type == wazeroir.Float32 {
		return c.compileSimpleFloatBinop(arm64.AFMAXS)
	} else {
		return c.compileSimpleFloatBinop(arm64.AFMAXD)
	}
}

func (c *arm64Compiler) compileSimpleFloatBinop(inst obj.As) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}
	c.compileRegisterToRegisterInstruction(inst, x2.register, x1.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileCopysign implements compiler.compileCopysign for the arm64 architecture.
func (c *arm64Compiler) compileCopysign(o *wazeroir.OperationCopysign) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	var fmov obj.As
	var minValueOffsetInVM int64
	if o.Type == wazeroir.Float32 {
		fmov = arm64.AFMOVS
		minValueOffsetInVM = callEngineArchContextMinimum32BitSignedIntOffset
	} else {
		fmov = arm64.AFMOVD
		minValueOffsetInVM = callEngineArchContextMinimum64BitSignedIntOffset
	}

	c.markRegisterUsed(x1.register, x2.register)
	freg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	x1vreg := simdRegisterForScalarFloatRegister(x1.register)
	x2vreg := simdRegisterForScalarFloatRegister(x2.register)
	vreg := simdRegisterForScalarFloatRegister(freg)

	// This is exactly the same code emitted by GCC for "__builtin_copysign":
	//
	//    mov     x0, -9223372036854775808
	//    fmov    d2, x0
	//    vbit     v0.8b, v1.8b, v2.8b
	//
	// "mov freg, -9223372036854775808 (stored at ce.minimum64BitSignedInt)"
	c.compileMemoryToRegisterInstruction(
		fmov,
		reservedRegisterForCallEngine, minValueOffsetInVM,
		freg,
	)

	// VBIT inserts each bit from the first operand into the destination if the corresponding bit of the second operand is 1,
	// otherwise it leaves the destination bit unchanged.
	// See https://developer.arm.com/documentation/dui0801/g/Advanced-SIMD-Instructions--32-bit-/VBIT
	//
	// For how to specify "V0.B8" (SIMD register arrangement), see
	// * https://github.com/twitchyliquid64/golang-asm/blob/v0.15.1/obj/link.go#L172-L177
	// * https://github.com/golang/go/blob/739328c694d5e608faa66d17192f0a59f6e01d04/src/cmd/compile/internal/arm64/ssa.go#L972
	//
	// "vbit vreg.8b, x2vreg.8b, x1vreg.8b" == "inserting 64th bit of x2 into x1".
	c.compileTwoRegistersToRegisterInstruction(arm64.AVBIT,
		vreg&31+arm64.REG_ARNG+(arm64.ARNG_8B&15)<<5,
		x2vreg&31+arm64.REG_ARNG+(arm64.ARNG_8B&15)<<5,
		x1vreg&31+arm64.REG_ARNG+(arm64.ARNG_8B&15)<<5,
	)

	c.markRegisterUnused(x2.register)
	c.locationStack.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileI32WrapFromI64 implements compiler.compileI32WrapFromI64 for the arm64 architecture.
func (c *arm64Compiler) compileI32WrapFromI64() error {
	return c.compileSimpleUnop(arm64.AMOVW)
}

// compileITruncFromF implements compiler.compileITruncFromF for the arm64 architecture.
func (c *arm64Compiler) compileITruncFromF(o *wazeroir.OperationITruncFromF) error {
	// Clear the floating point status register (FPSR).
	c.compileRegisterToRegisterInstruction(arm64.AMSR, zeroRegister, arm64.REG_FPSR)

	var convinst obj.As
	var is32bitFloat = o.InputType == wazeroir.Float32
	if is32bitFloat && o.OutputType == wazeroir.SignedInt32 {
		convinst = arm64.AFCVTZSSW
	} else if is32bitFloat && o.OutputType == wazeroir.SignedInt64 {
		convinst = arm64.AFCVTZSS
	} else if !is32bitFloat && o.OutputType == wazeroir.SignedInt32 {
		convinst = arm64.AFCVTZSDW
	} else if !is32bitFloat && o.OutputType == wazeroir.SignedInt64 {
		convinst = arm64.AFCVTZSD
	} else if is32bitFloat && o.OutputType == wazeroir.SignedUint32 {
		convinst = arm64.AFCVTZUSW
	} else if is32bitFloat && o.OutputType == wazeroir.SignedUint64 {
		convinst = arm64.AFCVTZUS
	} else if !is32bitFloat && o.OutputType == wazeroir.SignedUint32 {
		convinst = arm64.AFCVTZUDW
	} else if !is32bitFloat && o.OutputType == wazeroir.SignedUint64 {
		convinst = arm64.AFCVTZUD
	}

	source, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	destinationReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	c.compileRegisterToRegisterInstruction(convinst, source.register, destinationReg)
	c.locationStack.pushValueLocationOnRegister(destinationReg)

	// Obtain the floating point status register value into the general purpose register,
	// so that we can check if the conversion resulted in undefined behavior.
	c.compileRegisterToRegisterInstruction(arm64.AMRS, arm64.REG_FPSR, reservedRegisterForTemporary)
	// Check if the conversion was undefined by comparing the status with 1.
	// See https://developer.arm.com/documentation/ddi0595/2020-12/AArch64-Registers/FPSR--Floating-point-Status-Register
	c.compileRegisterAndConstSourceToNoneInstruction(arm64.ACMP, reservedRegisterForTemporary, 1)

	brOK := c.compilelBranchInstruction(arm64.ABNE)

	// If so, exit the execution with errors depending on whether or not the source value is NaN.
	{
		var floatcmp obj.As
		if is32bitFloat {
			floatcmp = arm64.AFCMPS
		} else {
			floatcmp = arm64.AFCMPD
		}
		c.compileTwoRegistersToNoneInstruction(floatcmp, source.register, source.register)
		// VS flag is set if at least one of values for FCMP is NaN.
		// https://developer.arm.com/documentation/dui0801/g/Condition-Codes/Comparison-of-condition-code-meanings-in-integer-and-floating-point-code
		brIfSourceNaN := c.compilelBranchInstruction(arm64.ABVS)

		// If the source value is not NaN, the operation was overflow.
		if err := c.compileExitFromNativeCode(jitCallStatusIntegerOverflow); err != nil {
			return err
		}
		// Otherwise, the operation was invalid as this is trying to convert NaN to integer.
		c.setBranchTargetOnNext(brIfSourceNaN)
		if err := c.compileExitFromNativeCode(jitCallStatusCodeInvalidFloatToIntConversion); err != nil {
			return err
		}
	}

	// Otherwise, we branch into the next instruction.
	c.setBranchTargetOnNext(brOK)
	return nil
}

// compileFConvertFromI implements compiler.compileFConvertFromI for the arm64 architecture.
func (c *arm64Compiler) compileFConvertFromI(o *wazeroir.OperationFConvertFromI) error {
	var convinst obj.As
	if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedInt32 {
		convinst = arm64.ASCVTFWS
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedInt64 {
		convinst = arm64.ASCVTFS
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedInt32 {
		convinst = arm64.ASCVTFWD
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedInt64 {
		convinst = arm64.ASCVTFD
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedUint32 {
		convinst = arm64.AUCVTFWS
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedUint64 {
		convinst = arm64.AUCVTFS
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedUint32 {
		convinst = arm64.AUCVTFWD
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedUint64 {
		convinst = arm64.AUCVTFD
	}
	return c.compileSimpleConversion(convinst, generalPurposeRegisterTypeFloat)
}

// compileF32DemoteFromF64 implements compiler.compileF32DemoteFromF64 for the arm64 architecture.
func (c *arm64Compiler) compileF32DemoteFromF64() error {
	return c.compileSimpleUnop(arm64.AFCVTDS)
}

// compileF64PromoteFromF32 implements compiler.compileF64PromoteFromF32 for the arm64 architecture.
func (c *arm64Compiler) compileF64PromoteFromF32() error {
	return c.compileSimpleUnop(arm64.AFCVTSD)
}

// compileI32ReinterpretFromF32 implements compiler.compileI32ReinterpretFromF32 for the arm64 architecture.
func (c *arm64Compiler) compileI32ReinterpretFromF32() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeInt)
		return nil
	}
	return c.compileSimpleConversion(arm64.AFMOVS, generalPurposeRegisterTypeInt)
}

// compileI64ReinterpretFromF64 implements compiler.compileI64ReinterpretFromF64 for the arm64 architecture.
func (c *arm64Compiler) compileI64ReinterpretFromF64() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeInt)
		return nil
	}
	return c.compileSimpleConversion(arm64.AFMOVD, generalPurposeRegisterTypeInt)
}

// compileF32ReinterpretFromI32 implements compiler.compileF32ReinterpretFromI32 for the arm64 architecture.
func (c *arm64Compiler) compileF32ReinterpretFromI32() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeFloat)
		return nil
	}
	return c.compileSimpleConversion(arm64.AFMOVS, generalPurposeRegisterTypeFloat)
}

// compileF64ReinterpretFromI64 implements compiler.compileF64ReinterpretFromI64 for the arm64 architecture.
func (c *arm64Compiler) compileF64ReinterpretFromI64() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeFloat)
		return nil
	}
	return c.compileSimpleConversion(arm64.AFMOVD, generalPurposeRegisterTypeFloat)
}

func (c *arm64Compiler) compileSimpleConversion(inst obj.As, destinationRegType generalPurposeRegisterType) error {
	source, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	destinationReg, err := c.allocateRegister(destinationRegType)
	if err != nil {
		return err
	}

	c.compileRegisterToRegisterInstruction(inst, source.register, destinationReg)
	c.locationStack.pushValueLocationOnRegister(destinationReg)
	return nil
}

// compileExtend implements compiler.compileExtend for the arm64 architecture.
func (c *arm64Compiler) compileExtend(o *wazeroir.OperationExtend) error {
	if o.Signed {
		return c.compileSimpleUnop(arm64.ASXTW)
	} else {
		return c.compileSimpleUnop(arm64.AUXTW)
	}
}

func (c *arm64Compiler) compileSimpleUnop(inst obj.As) error {
	v, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	reg := v.register
	c.compileRegisterToRegisterInstruction(inst, reg, reg)
	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

// compileEq implements compiler.compileEq for the arm64 architecture.
func (c *arm64Compiler) compileEq(o *wazeroir.OperationEq) error {
	return c.emitEqOrNe(true, o.Type)
}

// compileNe implements compiler.compileNe for the arm64 architecture.
func (c *arm64Compiler) compileNe(o *wazeroir.OperationNe) error {
	return c.emitEqOrNe(false, o.Type)
}

// emitEqOrNe implements compiler.compileEq and compiler.compileNe for the arm64 architecture.
func (c *arm64Compiler) emitEqOrNe(isEq bool, unsignedType wazeroir.UnsignedType) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	var inst obj.As
	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		inst = arm64.ACMPW
	case wazeroir.UnsignedTypeI64:
		inst = arm64.ACMP
	case wazeroir.UnsignedTypeF32:
		inst = arm64.AFCMPS
	case wazeroir.UnsignedTypeF64:
		inst = arm64.AFCMPD
	}

	c.compileTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

	// Push the comparison result as a conditional register value.
	cond := conditionalRegisterState(arm64.COND_NE)
	if isEq {
		cond = arm64.COND_EQ
	}
	c.locationStack.pushValueLocationOnConditionalRegister(cond)
	return nil
}

// compileEqz implements compiler.compileEqz for the arm64 architecture.
func (c *arm64Compiler) compileEqz(o *wazeroir.OperationEqz) error {
	x1, err := c.popValueOnRegister()
	if err != nil {
		return err
	}

	var inst obj.As
	switch o.Type {
	case wazeroir.UnsignedInt32:
		inst = arm64.ACMPW
	case wazeroir.UnsignedInt64:
		inst = arm64.ACMP
	}

	c.compileTwoRegistersToNoneInstruction(inst, zeroRegister, x1.register)

	// Push the comparison result as a conditional register value.
	cond := conditionalRegisterState(arm64.COND_EQ)
	c.locationStack.pushValueLocationOnConditionalRegister(cond)
	return nil
}

// compileLt implements compiler.compileLt for the arm64 architecture.
func (c *arm64Compiler) compileLt(o *wazeroir.OperationLt) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	var inst obj.As
	var conditionalRegister conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeUint32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_LO
	case wazeroir.SignedTypeUint64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_LO
	case wazeroir.SignedTypeInt32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_LT
	case wazeroir.SignedTypeInt64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_LT
	case wazeroir.SignedTypeFloat32:
		inst = arm64.AFCMPS
		conditionalRegister = arm64.COND_MI
	case wazeroir.SignedTypeFloat64:
		inst = arm64.AFCMPD
		conditionalRegister = arm64.COND_MI
	}

	c.compileTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

	// Push the comparison result as a conditional register value.
	c.locationStack.pushValueLocationOnConditionalRegister(conditionalRegister)
	return nil
}

// compileGt implements compiler.compileGt for the arm64 architecture.
func (c *arm64Compiler) compileGt(o *wazeroir.OperationGt) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	var inst obj.As
	var conditionalRegister conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeUint32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_HI
	case wazeroir.SignedTypeUint64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_HI
	case wazeroir.SignedTypeInt32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_GT
	case wazeroir.SignedTypeInt64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_GT
	case wazeroir.SignedTypeFloat32:
		inst = arm64.AFCMPS
		conditionalRegister = arm64.COND_GT
	case wazeroir.SignedTypeFloat64:
		inst = arm64.AFCMPD
		conditionalRegister = arm64.COND_GT
	}

	c.compileTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

	// Push the comparison result as a conditional register value.
	c.locationStack.pushValueLocationOnConditionalRegister(conditionalRegister)
	return nil
}

// compileLe implements compiler.compileLe for the arm64 architecture.
func (c *arm64Compiler) compileLe(o *wazeroir.OperationLe) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	var inst obj.As
	var conditionalRegister conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeUint32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_LS
	case wazeroir.SignedTypeUint64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_LS
	case wazeroir.SignedTypeInt32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_LE
	case wazeroir.SignedTypeInt64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_LE
	case wazeroir.SignedTypeFloat32:
		inst = arm64.AFCMPS
		conditionalRegister = arm64.COND_LS
	case wazeroir.SignedTypeFloat64:
		inst = arm64.AFCMPD
		conditionalRegister = arm64.COND_LS
	}

	c.compileTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

	// Push the comparison result as a conditional register value.
	c.locationStack.pushValueLocationOnConditionalRegister(conditionalRegister)
	return nil
}

// compileGe implements compiler.compileGe for the arm64 architecture.
func (c *arm64Compiler) compileGe(o *wazeroir.OperationGe) error {
	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	var inst obj.As
	var conditionalRegister conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeUint32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_HS
	case wazeroir.SignedTypeUint64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_HS
	case wazeroir.SignedTypeInt32:
		inst = arm64.ACMPW
		conditionalRegister = arm64.COND_GE
	case wazeroir.SignedTypeInt64:
		inst = arm64.ACMP
		conditionalRegister = arm64.COND_GE
	case wazeroir.SignedTypeFloat32:
		inst = arm64.AFCMPS
		conditionalRegister = arm64.COND_GE
	case wazeroir.SignedTypeFloat64:
		inst = arm64.AFCMPD
		conditionalRegister = arm64.COND_GE
	}

	c.compileTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

	// Push the comparison result as a conditional register value.
	c.locationStack.pushValueLocationOnConditionalRegister(conditionalRegister)
	return nil
}

// compileLoad implements compiler.compileLoad for the arm64 architecture.
func (c *arm64Compiler) compileLoad(o *wazeroir.OperationLoad) error {
	var (
		isFloat           bool
		loadInst          obj.As
		targetSizeInBytes int64
	)

	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		loadInst = arm64.AMOVWU
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeI64:
		loadInst = arm64.AMOVD
		targetSizeInBytes = 64 / 8
	case wazeroir.UnsignedTypeF32:
		loadInst = arm64.AFMOVS
		isFloat = true
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeF64:
		loadInst = arm64.AFMOVD
		isFloat = true
		targetSizeInBytes = 64 / 8
	}
	return c.compileLoadImpl(o.Arg.Offset, loadInst, targetSizeInBytes, isFloat)
}

// compileLoad8 implements compiler.compileLoad8 for the arm64 architecture.
func (c *arm64Compiler) compileLoad8(o *wazeroir.OperationLoad8) error {
	var loadInst obj.As
	switch o.Type {
	case wazeroir.SignedInt32, wazeroir.SignedInt64:
		// TODO: looks like the assembler cannot emit 32-bit variant of LDRSB.
		// Differentiate 32-bit vs 64-bit after #233.
		loadInst = arm64.AMOVB
	case wazeroir.SignedUint32, wazeroir.SignedUint64:
		loadInst = arm64.AMOVBU
	}
	return c.compileLoadImpl(o.Arg.Offset, loadInst, 1, false)
}

// compileLoad16 implements compiler.compileLoad16 for the arm64 architecture.
func (c *arm64Compiler) compileLoad16(o *wazeroir.OperationLoad16) error {
	var loadInst obj.As
	switch o.Type {
	case wazeroir.SignedInt32, wazeroir.SignedInt64:
		// TODO: looks like the assembler cannot emit 32-bit variant of LDRSH.
		// Differentiate 32-bit vs 64-bit after #233.
		loadInst = arm64.AMOVH
	case wazeroir.SignedUint32, wazeroir.SignedUint64:
		loadInst = arm64.AMOVHU
	}
	return c.compileLoadImpl(o.Arg.Offset, loadInst, 16/8, false)
}

// compileLoad32 implements compiler.compileLoad32 for the arm64 architecture.
func (c *arm64Compiler) compileLoad32(o *wazeroir.OperationLoad32) error {
	var loadInst obj.As
	if o.Signed {
		loadInst = arm64.AMOVW
	} else {
		loadInst = arm64.AMOVWU
	}
	return c.compileLoadImpl(o.Arg.Offset, loadInst, 32/8, false)
}

// compileLoadImpl implements compileLoadImpl* variants for arm64 architecture.
func (c *arm64Compiler) compileLoadImpl(offsetArg uint32, loadInst obj.As, targetSizeInBytes int64, isFloat bool) error {
	offsetReg, err := c.compileMemoryAccessOffsetSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}

	resultRegister := offsetReg
	if isFloat {
		resultRegister, err = c.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
	}

	// "resultRegister = [reservedRegisterForMemory + offsetReg]"
	// In other words, "resultRegister = memory.Buffer[offset: offset+targetSizeInBytes]"
	c.compileMemoryWithRegisterOffsetToRegisterInstruction(
		loadInst,
		reservedRegisterForMemory, offsetReg,
		resultRegister,
	)

	c.locationStack.pushValueLocationOnRegister(resultRegister)
	return nil
}

// compileStore implements compiler.compileStore for the arm64 architecture.
func (c *arm64Compiler) compileStore(o *wazeroir.OperationStore) error {
	var movInst obj.As
	var targetSizeInBytes int64
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		movInst = arm64.AMOVW
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeI64:
		movInst = arm64.AMOVD
		targetSizeInBytes = 64 / 8
	case wazeroir.UnsignedTypeF32:
		movInst = arm64.AFMOVS
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeF64:
		movInst = arm64.AFMOVD
		targetSizeInBytes = 64 / 8
	}
	return c.compileStoreImpl(o.Arg.Offset, movInst, targetSizeInBytes)
}

// compileStore8 implements compiler.compileStore8 for the arm64 architecture.
func (c *arm64Compiler) compileStore8(o *wazeroir.OperationStore8) error {
	return c.compileStoreImpl(o.Arg.Offset, arm64.AMOVB, 1)
}

// compileStore16 implements compiler.compileStore16 for the arm64 architecture.
func (c *arm64Compiler) compileStore16(o *wazeroir.OperationStore16) error {
	return c.compileStoreImpl(o.Arg.Offset, arm64.AMOVH, 16/8)
}

// compileStore32 implements compiler.compileStore32 for the arm64 architecture.
func (c *arm64Compiler) compileStore32(o *wazeroir.OperationStore32) error {
	return c.compileStoreImpl(o.Arg.Offset, arm64.AMOVW, 32/8)
}

// compileStoreImpl implements compleStore* variants for arm64 architecture.
func (c *arm64Compiler) compileStoreImpl(offsetArg uint32, storeInst obj.As, targetSizeInBytes int64) error {
	val, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	// Mark temporarily used as compileMemoryAccessOffsetSetup might try allocating register.
	c.markRegisterUsed(val.register)

	offsetReg, err := c.compileMemoryAccessOffsetSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}

	// "[reservedRegisterForMemory + offsetReg] = val.register"
	// In other words, "memory.Buffer[offset: offset+targetSizeInBytes] = val.register"
	c.compileRegisterToMemoryWithRegisterOffsetInstruction(
		storeInst, val.register,
		reservedRegisterForMemory, offsetReg,
	)

	c.markRegisterUnused(val.register)
	return nil
}

// compileMemoryAccessOffsetSetup pops the top value from the stack (called "base"), stores "base + offsetArg + targetSizeInBytes"
// into a register, and returns the stored register. We call the result "offset" because we access the memory
// as memory.Buffer[offset: offset+targetSizeInBytes].
//
// Note: this also emits the instructions to check the out of bounds memory access.
// In other words, if the offset+targetSizeInBytes exceeds the memory size, the code exits with jitCallStatusCodeMemoryOutOfBounds status.
func (c *arm64Compiler) compileMemoryAccessOffsetSetup(offsetArg uint32, targetSizeInBytes int64) (offsetRegister int16, err error) {
	base, err := c.popValueOnRegister()
	if err != nil {
		return 0, err
	}

	offsetRegister = base.register
	if isZeroRegister(base.register) {
		offsetRegister, err = c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return
		}
		c.compileRegisterToRegisterInstruction(arm64.AMOVD, zeroRegister, offsetRegister)
	}

	if offsetConst := int64(offsetArg) + targetSizeInBytes; offsetConst <= math.MaxUint32 {
		// "offsetRegister = base + offsetArg + targetSizeInBytes"
		c.compileConstToRegisterInstruction(arm64.AADD, offsetConst, offsetRegister)
	} else {
		// If the offset const is too large, we exit with jitCallStatusCodeMemoryOutOfBounds.
		err = c.compileExitFromNativeCode(jitCallStatusCodeMemoryOutOfBounds)
		return
	}

	// "reservedRegisterForTemporary = len(memory.Buffer)"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineModuleContextMemorySliceLenOffset,
		reservedRegisterForTemporary)

	// Check if offsetRegister(= base+offsetArg+targetSizeInBytes) > len(memory.Buffer).
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, reservedRegisterForTemporary, offsetRegister)
	boundsOK := c.compilelBranchInstruction(arm64.ABLS)

	// If offsetRegister(= base+offsetArg+targetSizeInBytes) exceeds the memory length,
	//  we exit the function with jitCallStatusCodeMemoryOutOfBounds.
	if err = c.compileExitFromNativeCode(jitCallStatusCodeMemoryOutOfBounds); err != nil {
		return
	}

	// Otherwise, we subtract targetSizeInBytes from offsetRegister.
	c.setBranchTargetOnNext(boundsOK)
	c.compileConstToRegisterInstruction(arm64.ASUB, targetSizeInBytes, offsetRegister)
	return offsetRegister, nil
}

// compileMemoryGrow implements compileMemoryGrow variants for arm64 architecture.
func (c *arm64Compiler) compileMemoryGrow() error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	if err := c.compileCallGoFunction(jitCallStatusCodeCallBuiltInFunction, builtinFunctionIndexMemoryGrow); err != nil {
		return err
	}

	// After return, we re-initialize reserved registers just like preamble of functions.
	c.compileReservedStackBasePointerRegisterInitialization()
	c.compileReservedMemoryRegisterInitialization()
	return nil
}

// compileMemorySize implements compileMemorySize variants for arm64 architecture.
func (c *arm64Compiler) compileMemorySize() error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// "reg = len(memory.Buffer)"
	c.compileMemoryToRegisterInstruction(
		arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineModuleContextMemorySliceLenOffset,
		reg,
	)

	// memory.size loads the page size of memory, so we have to divide by the page size.
	// "reg = reg >> wasm.MemoryPageSizeInBits (== reg / wasm.MemoryPageSize) "
	c.compileConstToRegisterInstruction(
		arm64.ALSR,
		wasm.MemoryPageSizeInBits,
		reg,
	)

	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

// compileCallGoFunction adds instructions to call a Go function whose address equals the addr parameter.
// jitStatus is set before making call, and it should be either jitCallStatusCodeCallBuiltInFunction or
// jitCallStatusCodeCallHostFunction.
func (c *arm64Compiler) compileCallGoFunction(jitStatus jitCallStatusCode, addr wasm.FunctionIndex) error {
	// Release all the registers as our calling convention requires the caller-save.
	if err := c.compileReleaseAllRegistersToStack(); err != nil {
		return err
	}

	freeRegs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 4)
	if !found {
		return fmt.Errorf("BUG: all registers except indexReg should be free at this point")
	}
	c.locationStack.markRegisterUsed(freeRegs...)

	// Alias these free tmp registers for readability.
	tmp, currentCallFrameStackPointerRegister, currentCallFrameTopAddressRegister, returnAddressRegister :=
		freeRegs[0], freeRegs[1], freeRegs[2], freeRegs[3]

	// Set the target function address to ce.functionCallAddress
	// "tmp = $addr"
	c.compileConstToRegisterInstruction(
		arm64.AMOVD,
		int64(addr),
		tmp,
	)
	// "[reservedRegisterForCallEngine + callEngineExitContextFunctionCallAddressOffset] = tmp"
	// In other words, "ce.functionCallAddress = tmp (== $addr)"
	c.compileRegisterToMemoryInstruction(
		arm64.AMOVD,
		tmp,
		reservedRegisterForCallEngine, callEngineExitContextFunctionCallAddressOffset,
	)

	// Next, we have to set the return address into callFrameStack[ce.callFrameStackPointer-1].returnAddress.
	//
	// "currentCallFrameStackPointerRegister = ce.callFrameStackPointer"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		currentCallFrameStackPointerRegister)

	// Set the address of callFrameStack[ce.callFrameStackPointer] into currentCallFrameTopAddressRegister.
	c.compileCalcCallFrameStackTopAddress(currentCallFrameStackPointerRegister, currentCallFrameTopAddressRegister)

	// Set the return address (after RET in c.exit below) into returnAddressRegister.
	c.compileReadInstructionAddress(obj.ARET, returnAddressRegister)

	// Write returnAddressRegister into callFrameStack[ce.callFrameStackPointer-1].returnAddress.
	c.compileRegisterToMemoryInstruction(
		arm64.AMOVD,
		returnAddressRegister,
		// callFrameStack[ce.callFrameStackPointer-1] is below of currentCallFrameTopAddressRegister.
		currentCallFrameTopAddressRegister, -(callFrameDataSize - callFrameReturnAddressOffset),
	)

	c.markRegisterUnused(freeRegs...)
	return c.compileExitFromNativeCode(jitStatus)
}

// compileConstI32 implements compiler.compileConstI32 for the arm64 architecture.
func (c *arm64Compiler) compileConstI32(o *wazeroir.OperationConstI32) error {
	return c.compileIntConstant(true, uint64(o.Value))
}

// compileConstI64 implements compiler.compileConstI64 for the arm64 architecture.
func (c *arm64Compiler) compileConstI64(o *wazeroir.OperationConstI64) error {
	return c.compileIntConstant(false, o.Value)
}

// compileIntConstant adds instructions to load an integer constant.
// is32bit is true if the target value is originally 32-bit const, false otherwise.
// value holds the (zero-extended for 32-bit case) load target constant.
func (c *arm64Compiler) compileIntConstant(is32bit bool, value uint64) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

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
		c.compileConstToRegisterInstruction(inst, int64(value), reg)

		c.locationStack.pushValueLocationOnRegister(reg)
	}
	return nil
}

// compileConstF32 implements compiler.compileConstF32 for the arm64 architecture.
func (c *arm64Compiler) compileConstF32(o *wazeroir.OperationConstF32) error {
	return c.compileFloatConstant(true, uint64(math.Float32bits(o.Value)))
}

// compileConstF64 implements compiler.compileConstF64 for the arm64 architecture.
func (c *arm64Compiler) compileConstF64(o *wazeroir.OperationConstF64) error {
	return c.compileFloatConstant(false, math.Float64bits(o.Value))
}

// compileFloatConstant adds instructions to load a float constant.
// is32bit is true if the target value is originally 32-bit const, false otherwise.
// value holds the (zero-extended for 32-bit case) bit representation of load target float constant.
func (c *arm64Compiler) compileFloatConstant(is32bit bool, value uint64) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

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
		c.compileConstToRegisterInstruction(inst, int64(value), tmpReg)
	}

	// Use FMOV instruction to move the value on integer register into the float one.
	var inst obj.As
	if is32bit {
		inst = arm64.AFMOVS
	} else {
		inst = arm64.AFMOVD
	}
	c.compileRegisterToRegisterInstruction(inst, tmpReg, reg)

	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

func (c *arm64Compiler) pushZeroValue() {
	c.locationStack.pushValueLocationOnRegister(zeroRegister)
}

// popTwoValuesOnRegisters pops two values from the location stacks, ensures
// these two values are located on registers, and mark them unused.
//
// TODO: we’d usually prefix this with compileXXX as this might end up emitting instructions,
// but the name seems awkward.
func (c *arm64Compiler) popTwoValuesOnRegisters() (x1, x2 *valueLocation, err error) {
	x2 = c.locationStack.pop()
	if err = c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return
	}

	x1 = c.locationStack.pop()
	if err = c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return
	}

	c.markRegisterUnused(x2.register)
	c.markRegisterUnused(x1.register)
	return
}

// popValueOnRegister pops one value from the location stack, ensures
// that it is located on a register, and mark it unused.
//
// TODO: we’d usually prefix this with compileXXX as this might end up emitting instructions,
// but the name seems awkward.
func (c *arm64Compiler) popValueOnRegister() (v *valueLocation, err error) {
	v = c.locationStack.pop()
	if err = c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return
	}

	c.markRegisterUnused(v.register)
	return
}

// compileEnsureOnGeneralPurposeRegister emits instructions to ensure that a value is located on a register.
func (c *arm64Compiler) compileEnsureOnGeneralPurposeRegister(loc *valueLocation) (err error) {
	if loc.onStack() {
		err = c.compileLoadValueOnStackToRegister(loc)
	} else if loc.onConditionalRegister() {
		c.compileLoadConditionalRegisterToGeneralPurposeRegister(loc)
	}
	return
}

// maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister moves the top value on the stack
// if the value is located on a conditional register.
//
// This is usually called at the beginning of methods on compiler interface where we possibly
// compile istructions without saving the conditional register value.
// The compile* functions without calling this function is saving the conditional
// value to the stack or register by invoking ensureOnGeneralPurposeRegister for the top.
func (c *arm64Compiler) maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister() {
	if c.locationStack.sp > 0 {
		if loc := c.locationStack.peek(); loc.onConditionalRegister() {
			c.compileLoadConditionalRegisterToGeneralPurposeRegister(loc)
		}
	}
}

// loadConditionalRegisterToGeneralPurposeRegister saves the conditional register value
// to a general purpose register.
//
// We use CSET instruction to set 1 on the register if the condition satisfies:
// https://developer.arm.com/documentation/100076/0100/a64-instruction-set-reference/a64-general-instructions/cset
func (c *arm64Compiler) compileLoadConditionalRegisterToGeneralPurposeRegister(loc *valueLocation) {
	// There must be always at least one free register at this point, as the conditional register located value
	// is always pushed after consuming at least one value (eqz) or two values for most cases (gt, ge, etc.).
	reg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	c.markRegisterUsed(reg)

	c.compileRegisterToRegisterInstruction(arm64.ACSET, int16(loc.conditionalRegister), reg)

	// Record that now the value is located on a general purpose register.
	loc.setRegister(reg)
}

// compileLoadValueOnStackToRegister emits instructions to load the value located on the stack to a register.
func (c *arm64Compiler) compileLoadValueOnStackToRegister(loc *valueLocation) (err error) {
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

	c.compileMemoryToRegisterInstruction(inst, reservedRegisterForStackBasePointerAddress, int64(loc.stackPointer)*8, reg)

	// Record that the value holds the register and the register is marked used.
	loc.setRegister(reg)
	c.locationStack.markRegisterUsed(reg)
	return
}

// allocateRegister returns an unused register of the given type. The register will be taken
// either from the free register pool or by spilling an used register. If we allocate an used register,
// this adds an instruction to write the value on a register back to memory stack region.
// Note: resulting registers are NOT marked as used so the call site should mark it used if necessary.
//
// TODO: we’d usually prefix this with compileXXX as this might end up emitting instructions,
// but the name seems awkward.
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
	err = c.compileReleaseRegisterToStack(stealTarget)
	return
}

// compileReleaseAllRegistersToStack adds instructions to store all the values located on
// either general purpuse or conditional registers onto the memory stack.
// See releaseRegisterToStack.
func (c *arm64Compiler) compileReleaseAllRegistersToStack() error {
	for i := uint64(0); i < c.locationStack.sp; i++ {
		if loc := c.locationStack.stack[i]; loc.onRegister() {
			if err := c.compileReleaseRegisterToStack(loc); err != nil {
				return err
			}
		} else if loc.onConditionalRegister() {
			c.compileLoadConditionalRegisterToGeneralPurposeRegister(loc)
			if err := c.compileReleaseRegisterToStack(loc); err != nil {
				return err
			}
		}
	}
	return nil
}

// releaseRegisterToStack adds an instruction to write the value on a register back to memory stack region.
func (c *arm64Compiler) compileReleaseRegisterToStack(loc *valueLocation) (err error) {
	var inst obj.As = arm64.AMOVD
	if loc.regType == generalPurposeRegisterTypeFloat {
		inst = arm64.AFMOVD
	}

	c.compileRegisterToMemoryInstruction(inst, loc.register, reservedRegisterForStackBasePointerAddress, int64(loc.stackPointer)*8)

	// Mark the register is free.
	c.locationStack.releaseRegister(loc)
	return
}

// compileReservedStackBasePointerRegisterInitialization adds intructions to initialize reservedRegisterForStackBasePointerAddress
// so that it points to the absolute address of the stack base for this function.
func (c *arm64Compiler) compileReservedStackBasePointerRegisterInitialization() {
	// First, load the address of the first element in the value stack into reservedRegisterForStackBasePointerAddress temporarily.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineGlobalContextValueStackElement0AddressOffset,
		reservedRegisterForStackBasePointerAddress)

	// Next we move the base pointer (ce.stackBasePointer) to reservedRegisterForTemporary.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset,
		reservedRegisterForTemporary)

	// Finally, we calculate "reservedRegisterForStackBasePointerAddress + reservedRegisterForTemporary << 3"
	// where we shift tmpReg by 3 because stack pointer is an index in the []uint64
	// so we must multiply the value by the size of uint64 = 8 bytes.
	c.compileAddInstructionWithLeftShiftedRegister(
		reservedRegisterForTemporary, 3, reservedRegisterForStackBasePointerAddress,
		reservedRegisterForStackBasePointerAddress)
}

func (c *arm64Compiler) compileReservedMemoryRegisterInitialization() {
	if c.f.ModuleInstance.MemoryInstance != nil {
		// "reservedRegisterForMemory = ce.MemoryElement0Address"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			reservedRegisterForCallEngine, callEngineModuleContextMemoryElement0AddressOffset,
			reservedRegisterForMemory,
		)
	}
}

// compileModuleContextInitialization adds instructions to initialize ce.ModuleContext's fields based on
// ce.ModuleContext.ModuleInstanceAddress.
// This is called in two cases: in function preamble, and on the return from (non-Go) function calls.
func (c *arm64Compiler) compileModuleContextInitialization() error {
	regs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		return fmt.Errorf("BUG: all the registers should be free at this point")
	}
	c.locationStack.markRegisterUsed(regs...)

	// Alias these free registers for readability.
	moduleInstanceAddressRegister, tmpX, tmpY := regs[0], regs[1], regs[2]

	// Load the absolute address of the current function's module instance.
	// Note: this should be modified to support Clone() functionality per #179.
	c.compileConstToRegisterInstruction(arm64.AMOVD, int64(uintptr(unsafe.Pointer(c.f.ModuleInstance))), moduleInstanceAddressRegister)

	// "tmpX = ce.ModuleInstanceAddress"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForCallEngine, callEngineModuleContextModuleInstanceAddressOffset, tmpX)

	// If the module instance address stays the same, we could skip the entire code below.
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, moduleInstanceAddressRegister, tmpX)
	brIfModuleUnchanged := c.compilelBranchInstruction(arm64.ABEQ)
	c.addInstruction(brIfModuleUnchanged)

	// Otherwise, we have to update the following fields:
	// * ce.moduleContext.globalElement0Address
	// * ce.moduleContext.memoryElement0Address
	// * ce.moduleContext.memorySliceLen
	// * ce.moduleContext.tableElement0Address
	// * ce.moduleContext.tableSliceLen

	// Update globalElement0Address.
	//
	// Note: if there's global.get or set instruction in the function, the existence of the globals
	// is ensured by function validation at module instantiation phase, and that's why it is ok to
	// skip the initialization if the module's globals slice is empty.
	if len(c.f.ModuleInstance.Globals) > 0 {
		// "tmpX = &moduleInstance.Globals[0]"
		c.compileMemoryToRegisterInstruction(arm64.AMOVD,
			moduleInstanceAddressRegister, moduleInstanceGlobalsOffset,
			tmpX,
		)

		// "ce.GlobalElement0Address = tmpX (== &moduleInstance.Globals[0])"
		c.compileRegisterToMemoryInstruction(
			arm64.AMOVD, tmpX,
			reservedRegisterForCallEngine, callEngineModuleContextGlobalElement0AddressOffset,
		)
	}

	// Update memoryElement0Address and memorySliceLen.
	//
	// Note: if there's memory instruction in the function, memory instance must be non-nil.
	// That is ensured by function validation at module instantiation phase, and that's
	// why it is ok to skip the initialization if the module's memory instance is nil.
	if c.f.ModuleInstance.MemoryInstance != nil {
		// "tmpX = moduleInstance.Memory"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			moduleInstanceAddressRegister, moduleInstanceMemoryOffset,
			tmpX,
		)

		// First, we write the memory length into ce.MemorySliceLen.
		//
		// "tmpY = [tmpX + memoryInstanceBufferLenOffset] (== len(memory.Buffer))"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			tmpX, memoryInstanceBufferLenOffset,
			tmpY,
		)
		// "ce.MemorySliceLen = tmpY".
		c.compileRegisterToMemoryInstruction(
			arm64.AMOVD,
			tmpY,
			reservedRegisterForCallEngine, callEngineModuleContextMemorySliceLenOffset,
		)

		// Next write ce.memoryElement0Address.
		//
		// "tmpY = *tmpX (== &memory.Buffer[0])"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			tmpX, memoryInstanceBufferOffset,
			tmpY,
		)
		// "ce.memoryElement0Address = tmpY".
		c.compileRegisterToMemoryInstruction(
			arm64.AMOVD,
			tmpY,
			reservedRegisterForCallEngine, callEngineModuleContextMemoryElement0AddressOffset,
		)
	}

	// Update tableElement0Address and tableSliceLen.
	//
	// Note: if there's table instruction in the function, the existence of the table
	// is ensured by function validation at module instantiation phase, and that's
	// why it is ok to skip the initialization if the module's table doesn't exist.
	if c.f.ModuleInstance.TableInstance != nil {
		// "tmpX = &tables[0] (type of **wasm.TableInstance)"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			moduleInstanceAddressRegister, moduleInstanceTableOffset,
			tmpX,
		)

		// Update ce.tableElement0Address.
		// "tmpY = &tables[0].Table[0]"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			tmpX, tableInstanceTableOffset,
			tmpY,
		)
		// "ce.tableElement0Address = tmpY".
		c.compileRegisterToMemoryInstruction(
			arm64.AMOVD,
			tmpY,
			reservedRegisterForCallEngine, callEngineModuleContextTableElement0AddressOffset,
		)

		// Update ce.tableSliceLen.
		// "tmpY = len(tables[0].Table)"
		c.compileMemoryToRegisterInstruction(
			arm64.AMOVD,
			tmpX, tableInstanceTableLenOffset,
			tmpY,
		)
		// "ce.tableSliceLen = tmpY".
		c.compileRegisterToMemoryInstruction(
			arm64.AMOVD,
			tmpY,
			reservedRegisterForCallEngine, callEngineModuleContextTableSliceLenOffset,
		)
	}

	c.setBranchTargetOnNext(brIfModuleUnchanged)
	c.locationStack.markRegisterUnused(regs...)
	return nil
}
