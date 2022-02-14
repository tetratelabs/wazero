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
		ir:            ir,
		labels:        map[string]*labelInfo{},
	}
	return compiler, nil
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
func (c *arm64Compiler) compileConstToRegisterInstruction(instruction obj.As, constValue int64, destinationRegister int16) {
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

// compileMemoryToRegisterInstruction adds an instruction where source operand is a memory location and destination is a register.
// baseRegister is the base absolute address in the memory, and offset is the offset from the absolute address in baseRegister.
func (c *arm64Compiler) compileMemoryToRegisterInstruction(instruction obj.As, sourceBaseRegister int16, sourceOffsetConst int64, destinationRegister int16) {
	if sourceOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		c.compileConstToRegisterInstruction(arm64.AMOVD, sourceOffsetConst, reservedRegisterForTemporary)
		inst := c.newProg()
		inst.As = instruction
		inst.From.Type = obj.TYPE_MEM
		inst.From.Reg = sourceBaseRegister
		inst.From.Index = reservedRegisterForTemporary
		inst.From.Scale = 1
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = destinationRegister
		c.addInstruction(inst)
	} else {
		inst := c.newProg()
		inst.As = instruction
		inst.From.Type = obj.TYPE_MEM
		inst.From.Reg = sourceBaseRegister
		inst.From.Offset = sourceOffsetConst
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = destinationRegister
		c.addInstruction(inst)
	}
}

// compileRegisterToMemoryInstruction adds an instruction where destination operand is a memory location and source is a register.
// This is the opposite of compileMemoryToRegisterInstruction.
func (c *arm64Compiler) compileRegisterToMemoryInstruction(instruction obj.As, sourceRegister int16, destinationBaseRegister int16, destinationOffsetConst int64) {
	if destinationOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but we cannot track its temporary register. Therefore, we avoid directly emitting memory load with large offsets:
		// load the constant manually to "our" temporary register, then emit the load with it.
		c.compileConstToRegisterInstruction(arm64.AMOVD, destinationOffsetConst, reservedRegisterForTemporary)
		inst := c.newProg()
		inst.As = instruction
		inst.To.Type = obj.TYPE_MEM
		inst.To.Reg = destinationBaseRegister
		inst.To.Index = reservedRegisterForTemporary
		inst.To.Scale = 1
		inst.From.Type = obj.TYPE_REG
		inst.From.Reg = sourceRegister
		c.addInstruction(inst)
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

func (c *arm64Compiler) compileUnconditionalBranchInstruction() (br *obj.Prog) {
	br = c.newProg()
	br.As = obj.AJMP
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
	nop := c.newProg()
	nop.As = obj.ANOP
	c.addInstruction(nop)

	c.pushFunctionParams()

	// Before excuting function body, we must initialize the stack base pointer register
	// so that we can manipulate the memory stack properly.
	return c.compileInitializeReservedStackBasePointerRegister()
}

// returnFunction emits instructions to return from the current function frame.
// If the current frame is the bottom, the code goes back to the Go code with jitCallStatusCodeReturned status.
// Otherwise, we branch into the caller's return address.
func (c *arm64Compiler) compileReturnFunction() error {
	// Release all the registers as our calling convention requires the caller-save.
	if err := c.compileReleaseAllRegistersToStack(); err != nil {
		return err
	}

	// Since we return from the function, we need to decrement the callframe stack pointer, and write it back.
	tmpRegs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		return fmt.Errorf("BUG: all the registers should be free at this point")
	}

	// Alias for readability.
	callFramePointerReg, callFrameStackTopAddressRegister, tmpReg := tmpRegs[0], tmpRegs[1], tmpRegs[2]

	// First we decrement the callframe stack pointer.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset, callFramePointerReg)
	c.compileConstToRegisterInstruction(arm64.ASUBS, 1, callFramePointerReg)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD, callFramePointerReg, reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset)

	// Next we compare the decremented call frame stack pointer with the engine.precviousCallFrameStackPointer.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextPreviouscallFrameStackPointer,
		tmpReg,
	)
	c.compileTwoRegistersToNoneInstruction(arm64.ACMP, callFramePointerReg, tmpReg)

	// If the values are identical, we return back to the Go code with returned status.
	brIfNotEqual := c.newProg()
	brIfNotEqual.As = arm64.ABNE
	brIfNotEqual.To.Type = obj.TYPE_BRANCH
	c.addInstruction(brIfNotEqual)
	c.exit(jitCallStatusCodeReturned)

	// Otherwise, we have to jump to the caller's return address.
	c.setBranchTargetOnNext(brIfNotEqual)

	// First, we have to calculate the caller callFrame's absolute address to aquire the return address.
	//
	// "tmpReg = &engine.callFrameStack[0]"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextCallFrameStackElement0AddressOffset,
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
	//                   (absolute address of &callFrameStack[engine.callFrameStackPointer])
	//
	// where:
	//      ra.* = callFrame.returnAddress
	//      rb.* = callFrame.returnStackBasePointer
	//      rc.* = callFrame.compiledFunction
	//      _  = callFrame's padding (see comment on callFrame._ field.)
	//
	// What we have to do in the following is that
	//   1) Set engine.valueStackContext.stackBasePointer to the value on "rb.caller".
	//   2) Jump into the address of "ra.caller".

	// 1) Set engine.valueStackContext.stackBasePointer to the value on "rb.caller".
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		// "rb.caller" is below the top address.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset),
		tmpReg)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		tmpReg,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset)

	// 2) Branch into the address of "ra.caller".
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		// "rb.caller" is below the top address.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnAddressOffset),
		tmpReg)
	c.compileUnconditionalBranchToAddressOnRegister(tmpReg)

	c.locationStack.markRegisterUnused(tmpRegs...)
	return nil
}

// exit adds instructions to give the control back to engine.exec with the given status code.
func (c *arm64Compiler) exit(status jitCallStatusCode) error {
	// Write the current stack pointer to the engine.stackPointer.
	c.compileConstToRegisterInstruction(arm64.AMOVW, int64(c.locationStack.sp), reservedRegisterForTemporary)
	c.compileRegisterToMemoryInstruction(arm64.AMOVW, reservedRegisterForTemporary, reservedRegisterForEngine,
		engineValueStackContextStackPointerOffset)

	if status != 0 {
		c.compileConstToRegisterInstruction(arm64.AMOVW, int64(status), reservedRegisterForTemporary)
		c.compileRegisterToMemoryInstruction(arm64.AMOVWU, reservedRegisterForTemporary, reservedRegisterForEngine, engineExitContextJITCallStatusCodeOffset)
	} else {
		// If the status == 0, we use zero register to store zero.
		c.compileRegisterToMemoryInstruction(arm64.AMOVWU, zeroRegister, reservedRegisterForEngine, engineExitContextJITCallStatusCodeOffset)
	}

	// The return address to the Go code is stored in archContext.jitReturnAddress which
	// is embedded in engine. We load the value to the tmpRegister, and then
	// invoke RET with that register.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine, engineArchContextJITCallReturnAddressOffset, reservedRegisterForTemporary)

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
	labelBegin := c.newProg()
	labelBegin.As = obj.ANOP
	c.addInstruction(labelBegin)

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
	return c.exit(jitCallStatusCodeUnreachable)
}

// compileSwap implements compiler.compileSwap for the arm64 architecture.
func (c *arm64Compiler) compileSwap(o *wazeroir.OperationSwap) error {
	x := c.locationStack.peek()
	y := c.locationStack.stack[int(c.locationStack.sp)-1-o.Depth]

	if err := c.maybeCompileEnsureOnGeneralPurposeRegister(x); err != nil {
		return err
	}
	if err := c.maybeCompileEnsureOnGeneralPurposeRegister(y); err != nil {
		return err
	}

	x.register, y.register = y.register, x.register
	return nil
}

func (c *arm64Compiler) compileGlobalGet(o *wazeroir.OperationGlobalGet) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

func (c *arm64Compiler) compileGlobalSet(o *wazeroir.OperationGlobalSet) error {
	return fmt.Errorf("TODO: unsupported on arm64")
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
		if err := c.maybeCompileEnsureOnGeneralPurposeRegister(cond); err != nil {
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
	c.compileBranchInto(o.Else.Target)

	// Now ready to emit the code for branching into then branch.
	// Retrieve the original value location stack so that the code below wont'be affected by the Else branch ^^.
	c.setLocationStack(saved)
	// We branch into here from the original conditional BR (conditionalBR).
	c.setBranchTargetOnNext(conditionalBR)
	if err := c.compileDropRange(o.Then.ToDrop); err != nil {
		return err
	}
	c.compileBranchInto(o.Then.Target)
	return nil
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

		br := c.compileUnconditionalBranchInstruction()
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

func (c *arm64Compiler) compileBrTable(o *wazeroir.OperationBrTable) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

// compileCall implements compiler.compileCall for the arm64 architecture.
func (c *arm64Compiler) compileCall(o *wazeroir.OperationCall) error {
	target := c.f.ModuleInstance.Functions[o.FunctionIndex]

	if err := c.compileCallFunction(target.Address, target.FunctionType.Type); err != nil {
		return err
	}
	return nil
}

// compileCall implements compiler.compileCall and compiler.compileCallIndirect (TODO) for the arm64 architecture.
func (c *arm64Compiler) compileCallFunction(addr wasm.FunctionAddress, functype *wasm.FunctionType) error {
	// TODO: the following code can be generalized for CallIndirect.

	// Release all the registers as our calling convention requires the caller-save.
	if err := c.compileReleaseAllRegistersToStack(); err != nil {
		return err
	}

	// Obtain the free registers to be used in the followings.
	freeRegisters, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 5)
	if !found {
		return fmt.Errorf("BUG: all registers except addrReg should be free at this point")
	}
	c.locationStack.markRegisterUsed(freeRegisters...)

	// Alias for readability.
	callFrameStackTopAddressRegister, compiledFunctionAddressRegister, oldStackBasePointer,
		tmp := freeRegisters[0], freeRegisters[1], freeRegisters[2], freeRegisters[3]

	// TODO: Check the callframe stack length, and if necessary, grow the call frame stack before jump into the target.

	// "tmp = engine.callFrameStackPointer"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset,
		tmp)
	// "callFrameStackTopAddressRegister = &engine.callFrameStack[0]"
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextCallFrameStackElement0AddressOffset,
		callFrameStackTopAddressRegister)
	// "callFrameStackTopAddressRegister += tmp << $callFrameDataSizeMostSignificantSetBit"
	c.compileAddInstructionWithLeftShiftedRegister(
		tmp, callFrameDataSizeMostSignificantSetBit,
		callFrameStackTopAddressRegister,
		callFrameStackTopAddressRegister,
	)

	// At this point, we have:
	//
	//    [..., ra.current, rb.current, rc.current, _, ra.next, rb.next, rc.next, ...]  <- call frame stack's data region (somewhere in the memory)
	//                                               ^
	//                              callFrameStackTopAddressRegister
	//                (absolute address of &callFrame[engine.callFrameStackPointer]])
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
	//   2) Set engine.valueStackContext.stackBasePointer for the next function.
	//   3) Set rc.next to specify which function is executed on the current call frame (needs to make Go function calls).
	//   4) Set ra.current so that we can return back to this function properly.

	// 1) Set rb.current so that we can return back to this function properly.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset,
		oldStackBasePointer)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		oldStackBasePointer,
		// "rb.current" is BELOW the top address. See the above example for detail.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset))

	// 2) Set engine.valueStackContext.stackBasePointer for the next function.
	//
	// At this point, oldStackBasePointer holds the old stack base pointer. We could get the new frame's
	// stack base pointer by "old stack base pointer + old stack pointer - # of function params"
	// See the comments in engine.pushCallFrame which does exactly the same calculation in Go.
	c.compileConstToRegisterInstruction(arm64.AADD,
		int64(c.locationStack.sp)-int64(len(functype.Params)),
		oldStackBasePointer)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		oldStackBasePointer,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset)

	// 3) Set rc.next to specify which function is executed on the current call frame.
	//
	// First, we read the address of the first item of engine.compiledFunctions slice (= &engine.compiledFunctions[0])
	// into tmp.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextCompiledFunctionsElement0AddressOffset,
		tmp)

	// Next, read the address of the target function (= &engine.compiledFunctions[offset])
	// into compiledFunctionAddressRegister.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		tmp, int64(addr)*8, // * 8 because the size of *compiledFunction equals 8 bytes.
		compiledFunctionAddressRegister)

	// Finally, we are ready to write the address of the target function's *compiledFunction into the new callframe.
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		compiledFunctionAddressRegister,
		callFrameStackTopAddressRegister, callFrameCompiledFunctionOffset)

	// 4) Set ra.current so that we can return back to this function properly.
	//
	// First, Get the return address into the tmp.
	c.readInstructionAddress(obj.AJMP, tmp)
	// Then write the address into the callframe.
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		tmp,
		// "ra.current" is BELOW the top address. See the above example for detail.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnAddressOffset),
	)

	// Everthing is done to make function call now.
	// We increment the callframe stack pointer.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset,
		tmp)
	c.compileConstToRegisterInstruction(arm64.AADD, 1, tmp)
	c.compileRegisterToMemoryInstruction(arm64.AMOVD,
		tmp,
		reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset)

	// Then, br into the target function's initial address.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		compiledFunctionAddressRegister, compiledFunctionCodeInitialAddressOffset,
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
	c.compileInitializeReservedStackBasePointerRegister()

	// TODO: initialize module context, and memory pointer.
	return nil
}

// readInstructionAddress adds an ADR instruction to set the absolute address of "target instruction"
// into destinationRegister. "target instruction" is specified by beforeTargetInst argument and
// the target is determined by "the instruction right after beforeTargetInst type".
//
// For example, if beforeTargetInst == RET and we have the instruction sequence like
// ADR -> X -> Y -> ... -> RET -> MOV, then the ADR instruction emitted by this function set the absolute
// address of MOV instruction into the destination register.
func (c *arm64Compiler) readInstructionAddress(beforeTargetInst obj.As, destinationRegister int16) {
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

func (c *arm64Compiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	return fmt.Errorf("TODO: unsupported on arm64")
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
	liveValues := c.locationStack.stack[c.locationStack.sp-uint64(r.Start):]
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
		if err := c.maybeCompileEnsureOnGeneralPurposeRegister(live); err != nil {
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

	x1, x2, err := c.popTwoValuesOnRegisters()
	if err != nil {
		return err
	}

	if isZeroRegister(x1.register) && isZeroRegister(x2.register) {
		c.locationStack.pushValueLocationOnRegister(zeroRegister)
		c.markRegisterUnused(cv.register)
		return nil
	} else if isZeroRegister(x1.register) {
		c.markRegisterUsed(x2.register, cv.register)
		x1Reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
		if err != nil {
			return err
		}
		x1.setRegister(x1Reg)
		c.compileRegisterToRegisterInstruction(arm64.AMOVD, zeroRegister, x1Reg)
	}

	c.compileTwoRegistersToNoneInstruction(arm64.ACMPW, zeroRegister, cv.register)
	brIfNotZero := c.newProg()
	brIfNotZero.As = arm64.ABNE
	brIfNotZero.To.Type = obj.TYPE_BRANCH
	c.addInstruction(brIfNotZero)

	// If cv == 0, we choose the value of x2 to the x1.register.
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
		c.compileLoadValueOnStackToRegister(pickTarget)
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
	if err = c.maybeCompileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return
	}

	x1 = c.locationStack.pop()
	if err = c.maybeCompileEnsureOnGeneralPurposeRegister(x1); err != nil {
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
	if err = c.maybeCompileEnsureOnGeneralPurposeRegister(v); err != nil {
		return
	}

	c.markRegisterUnused(v.register)
	return
}

// maybeCompileEnsureOnGeneralPurposeRegister emits instructions to ensure that a value is located on a register.
func (c *arm64Compiler) maybeCompileEnsureOnGeneralPurposeRegister(loc *valueLocation) (err error) {
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

// compilaleInitializeReservedStackBasePointerRegister adds intructions to initialize reservedRegisterForStackBasePointerAddress
// so that it points to the absolute address of the stack base for this function.
func (c *arm64Compiler) compileInitializeReservedStackBasePointerRegister() error {
	// First, load the address of the first element in the value stack into reservedRegisterForStackBasePointerAddress temporarily.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextValueStackElement0AddressOffset,
		reservedRegisterForStackBasePointerAddress)

	// Next we move the base pointer (engine.stackBasePointer) to reservedRegisterForTemporary.
	c.compileMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset,
		reservedRegisterForTemporary)

	// Finally, we calculate "reservedRegisterForStackBasePointerAddress + reservedRegisterForTemporary << 3"
	// where we shift tmpReg by 3 because stack pointer is an index in the []uint64
	// so we must multiply the value by the size of uint64 = 8 bytes.
	c.compileAddInstructionWithLeftShiftedRegister(
		reservedRegisterForTemporary, 3, reservedRegisterForStackBasePointerAddress,
		reservedRegisterForStackBasePointerAddress)
	return nil
}

// emitAddInstructionWithLeftShiftedRegister emits an ADD instruction to perform "destinationReg = srcReg + (shiftedSourceReg << shiftNum)".
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
