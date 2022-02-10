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
	// setBRTargetOnNextInstructions holds branch kind instructions (BR, conditional BR, etc)
	// where we want to set the next coming instruction as the destination of these BR instructions.
	setBRTargetOnNextInstructions []*obj.Prog
	// locationStack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
	// labels maps a label (Ex. ".L1_then") to *labelInfo.
	labels map[string]*labelInfo
	// stackPointerCeil is the greatest stack pointer value (from valueLocationStack) seen during compilation.
	stackPointerCeil uint64
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

	code, err = mmapCodeSegment(c.builder.Assemble())
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
	for _, origin := range c.setBRTargetOnNextInstructions {
		origin.To.SetTarget(inst)
	}
	c.setBRTargetOnNextInstructions = nil
	return
}

func (c *arm64Compiler) addInstruction(inst *obj.Prog) {
	c.builder.AddInstruction(inst)
}

func (c *arm64Compiler) setBRTargetOnNext(progs ...*obj.Prog) {
	c.setBRTargetOnNextInstructions = append(c.setBRTargetOnNextInstructions, progs...)
}

func (c *arm64Compiler) markRegisterUsed(reg int16) {
	c.locationStack.markRegisterUsed(reg)
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
func (c *arm64Compiler) applyMemoryToRegisterInstruction(instruction obj.As, sourceBaseRegister int16, sourceOffsetConst int64, destinationRegister int16) {
	if sourceOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		c.applyConstToRegisterInstruction(arm64.AMOVD, sourceOffsetConst, reservedRegisterForTemporary)
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

// applyRegisterToMemoryInstruction adds an instruction where destination operand is a memory location and source is a register.
// This is the opposite of applyMemoryToRegisterInstruction.
func (c *arm64Compiler) applyRegisterToMemoryInstruction(instruction obj.As, sourceRegister int16, destinationBaseRegister int16, destinationOffsetConst int64) {
	if destinationOffsetConst > math.MaxInt16 {
		// The assembler can take care of offsets larger than 2^15-1 by emitting additional instructions to load such large offset,
		// but it uses "its" temporary register which we cannot track. Therefore, we avoid directly emitting memory load with large offsets,
		// but instead load the constant manually to "our" temporary register, then emit the load with it.
		c.applyConstToRegisterInstruction(arm64.AMOVD, destinationOffsetConst, reservedRegisterForTemporary)
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
	return
}

// applyRegisterToRegisterInstruction adds an instruction where both destination and source operands are registers.
func (c *arm64Compiler) applyRegisterToRegisterInstruction(instruction obj.As, from, to int16) {
	inst := c.newProg()
	inst.As = instruction
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = to
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = from
	c.addInstruction(inst)
}

// applyTwoRegistersToRegisterInstruction adds an instruction which takes two source operands on registers and one destination register operand.
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

// applyTwoRegistersToNoneInstruction adds an instruction which takes two source operands on registers.
func (c *arm64Compiler) applyTwoRegistersToNoneInstruction(instruction obj.As, src1, src2 int16) {
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

func (c *arm64Compiler) emitUnconditionalBRInstruction(targetType obj.AddrType) (jmp *obj.Prog) {
	jmp = c.newProg()
	jmp.As = obj.AJMP
	jmp.To.Type = targetType
	c.addInstruction(jmp)
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

// emitPreamble implements compiler.emitPreamble for the arm64 architecture.
func (c *arm64Compiler) emitPreamble() error {
	// The assembler skips the first instruction so we intentionally add NOP here.
	nop := c.newProg()
	nop.As = obj.ANOP
	c.addInstruction(nop)

	c.pushFunctionParams()

	// Before excuting function body, we must initialize the stack base pointer register
	// so that we can manipulate the memory stack properly.
	return c.initializeReservedStackBasePointerRegister()
}

// returnFunction emits instructions to return from the current function frame.
// If the current frame is the bottom, the code goes back to the Go code with jitCallStatusCodeReturned status.
// Otherwise, we branch into the caller's return address (TODO).
func (c *arm64Compiler) returnFunction() error {
	// TODO: we don't support function calls yet.
	// For now the following code just returns to Go code.

	// Since we return from the function, we need to decrement the callframe stack pointer, and write it back.
	callFramePointerReg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	c.applyMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset, callFramePointerReg)
	c.applyConstToRegisterInstruction(arm64.ASUBS, 1, callFramePointerReg)
	c.applyRegisterToMemoryInstruction(arm64.AMOVD, callFramePointerReg, reservedRegisterForEngine, engineGlobalContextCallFrameStackPointerOffset)
	return c.exit(jitCallStatusCodeReturned)
}

// exit adds instructions to give the control back to engine.exec with the given status code.
func (c *arm64Compiler) exit(status jitCallStatusCode) error {
	// Write the current stack pointer to the engine.stackPointer.
	c.applyConstToRegisterInstruction(arm64.AMOVW, int64(c.locationStack.sp), reservedRegisterForTemporary)
	c.applyRegisterToMemoryInstruction(arm64.AMOVW, reservedRegisterForTemporary, reservedRegisterForEngine,
		engineValueStackContextStackPointerOffset)

	if status != 0 {
		c.applyConstToRegisterInstruction(arm64.AMOVW, int64(status), reservedRegisterForTemporary)
		c.applyRegisterToMemoryInstruction(arm64.AMOVWU, reservedRegisterForTemporary, reservedRegisterForEngine, engineExitContextJITCallStatusCodeOffset)
	} else {
		// If the status == 0, we use zero register to store zero.
		c.applyRegisterToMemoryInstruction(arm64.AMOVWU, zeroRegister, reservedRegisterForEngine, engineExitContextJITCallStatusCodeOffset)
	}

	// The return address to the Go code is stored in archContext.jitReturnAddress which
	// is embedded in engine. We load the value to the tmpRegister, and then
	// invoke RET with that register.
	c.applyMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine, engineArchContextJITCallReturnAddressOffset, reservedRegisterForTemporary)

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
	c.maybeMoveTopConditionalToFreeGeneralPurposeRegister()
	return c.branchInto(o.Target)
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
		if err := c.ensureOnGeneralPurposeRegister(cond); err != nil {
			return err
		}
		// Compare the value with zero register. Note that the value is ensured to be i32 by function validation phase,
		// so we use CMPW (32-bit compare) here.
		c.applyTwoRegistersToNoneInstruction(arm64.ACMPW, cond.register, zeroRegister)
		conditionalBR.As = arm64.ABNE

		c.markRegisterUnused(cond.register)
	}

	c.addInstruction(conditionalBR)

	// Emit the code for branching into else branch.
	// We save and clone the location stack because we might end up modifying it inside of branchInto,
	// and we have to avoid affecting the code generation for Then branch afterwards.
	saved := c.locationStack
	c.setLocationStack(saved.clone())
	if err := c.emitDropRange(o.Else.ToDrop); err != nil {
		return err
	}
	c.branchInto(o.Else.Target)

	// Now ready to emit the code for branching into then branch.
	// Retrieve the original value location stack so that the code below wont'be affected by the Else branch ^^.
	c.setLocationStack(saved)
	// We branch into here from the original conditional BR (conditionalBR).
	c.setBRTargetOnNext(conditionalBR)
	if err := c.emitDropRange(o.Then.ToDrop); err != nil {
		return err
	}
	c.branchInto(o.Then.Target)
	return nil
}

func (c *arm64Compiler) branchInto(target *wazeroir.BranchTarget) error {
	if target.IsReturnTarget() {
		return c.returnFunction()
	} else {
		labelKey := target.String()
		if c.ir.LabelCallers[labelKey] > 1 {
			// We can only re-use register state if when there's a single call-site.
			// Release existing values on registers to the stack if there's multiple ones to have
			// the consistent value location state at the beginning of label.
			if err := c.releaseAllRegistersToStack(); err != nil {
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

		jmp := c.emitUnconditionalBRInstruction(obj.TYPE_BRANCH)
		c.assignBranchTarget(labelKey, jmp)
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
	return c.callFunction(target.Address, target.FunctionType.Type)
}

// compileCall implements compiler.compileCall and compiler.compileCallIndirect (TODO) for the arm64 architecture.
func (c *arm64Compiler) callFunction(addr wasm.FunctionAddress, functype *wasm.FunctionType) error {
	// TODO: the following code can be generalized for CallIndirect.

	// Release all the registers as our calling convention requires the caller-save.
	if err := c.releaseAllRegistersToStack(); err != nil {
		return err
	}

	// Obtain the temporary registers to be used in the followings.
	tmpRegisters, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		// This in theory never happen as all the registers must be free except addrReg.
		return fmt.Errorf("could not find enough free registers")
	}
	c.locationStack.markRegisterUsed(tmpRegisters...)

	// Alias for readability.
	callFrameStackTopAddressRegister := tmpRegisters[0]

	// TODO: Check the callfram stack length, and if necessary, grow the call frame stack before jump into the target.

	c.getCallFrameStackPointerOffsetInBytes(tmpRegisters[1])
	c.applyMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine,
		engineGlobalContextCallFrameStackElement0AddressOffset, tmpRegisters[2])
	// Calculate "callFrameStackTopAddressRegister = tmp[1] + tmp[2]".
	c.applyTwoRegistersToRegisterInstruction(arm64.AADD, tmpRegisters[1], tmpRegisters[2], callFrameStackTopAddressRegister)

	// At this point, we have:
	//
	//      [ra.0, rb.0, rc.0, _, ra.1, rb.1, rc.1, _, ra.next, rb.next, rc.next, ...]  <--- call frame stack's data region (somewhere in the memory)
	//                                               |
	//                              callFrameStackTopAddressRegister
	//               (the absolute address of &callFrame[engine.callFrameStackPointer]])
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
	//   1) Set rb.1 so that we can return back to this function properly.
	//   2) Set engine.valueStackContext.stackBasePointer for the next function.
	//   3) Set rc.next to specify which function is executed on the current call frame (needs to make Go function calls).
	//   4) Set ra.1 so that we can return back to this function properly.

	// 1) Set rb.1 so that we can return back to this function properly.
	c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset,
		tmpRegisters[1])
	c.applyRegisterToMemoryInstruction(arm64.AMOVD,
		tmpRegisters[1],
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset))

	// 2) Set engine.valueStackContext.stackBasePointer for the next function.
	//
	// At this point, tmpRegisters[1] holds the old stack base pointer. We could get the new frame's
	// stack base pointer by "old stack base pointer + old stack pointer - # of function params"
	// See the comments in engine.pushCallFrame which does exactly the same calculation in Go.
	c.applyConstToRegisterInstruction(arm64.AADD,
		int64(c.locationStack.sp)-int64(len(functype.Params)),
		tmpRegisters[1])
	c.applyRegisterToMemoryInstruction(arm64.AMOVD,
		tmpRegisters[1],
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset)

	// Alias for readability.
	compiledFunctionAddressRegister := tmpRegisters[1]

	// 3) Set rc.next to specify which function is executed on the current call frame.
	//
	// First, we read the address of the first item of engine.compiledFunctions slice (= &engine.compiledFunctions[0])
	// into tmpRegisters[1].
	c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextCompiledFunctionsElement0AddressOffset,
		tmpRegisters[1])

	// Next, read the address of the target function (= &engine.compiledFunctions[offset])
	// into compiledFunctionAddressRegister.
	c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		tmpRegisters[1], int64(addr)*8, // * 8 because the size of *compiledFunction equals 8 bytes.
		compiledFunctionAddressRegister)

	// Finally, we are ready to place the address of the target function's *compiledFunction into the new callframe.
	c.applyRegisterToMemoryInstruction(arm64.AMOVD,
		compiledFunctionAddressRegister,
		callFrameStackTopAddressRegister, callFrameCompiledFunctionOffset)

	return nil
}

func (c *arm64Compiler) getCallFrameStackPointerOffsetInBytes(destinationRegister int16) {
	// First we get the callFrameStackPointer in engine.globalContext.
	c.applyMemoryToRegisterInstruction(arm64.AMOVD, reservedRegisterForEngine,
		engineGlobalContextCallFrameStackPointerOffset, destinationRegister)

	// The value is an index to engine.callFrameStack ([]callFrame type), so
	// we shift it by callFrameDataSizeMostSignificantSetBit to get the offset in bytes.
	c.applyConstToRegisterInstruction(arm64.ALSL, callFrameDataSizeMostSignificantSetBit, destinationRegister)
}

func (c *arm64Compiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

// compileDrop implements compiler.compileDrop for the arm64 architecture.
func (c *arm64Compiler) compileDrop(o *wazeroir.OperationDrop) error {
	return c.emitDropRange(o.Range)
}

// emitDropRange is the implementation of compileDrop. See compiler.compileDrop.
func (c *arm64Compiler) emitDropRange(r *wazeroir.InclusiveRange) error {
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
	c.maybeMoveTopConditionalToFreeGeneralPurposeRegister()

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
		if err := c.ensureOnGeneralPurposeRegister(live); err != nil {
			return err
		}
		// Update the runtime memory stack location by pushing onto the location stack.
		c.locationStack.push(live)
	}
	return nil
}

func (c *arm64Compiler) compileSelect() error {
	return fmt.Errorf("TODO: unsupported on arm64")
}

// compilePick implements compiler.compilePick for the arm64 architecture.
func (c *arm64Compiler) compilePick(o *wazeroir.OperationPick) error {
	c.maybeMoveTopConditionalToFreeGeneralPurposeRegister()

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
		c.applyRegisterToRegisterInstruction(inst, pickTarget.register, pickedRegister)
	} else if pickTarget.onStack() {
		// Temporarily assign a register to the pick target, and then load the value.
		pickTarget.setRegister(pickedRegister)
		c.loadValueOnStackToRegister(pickTarget)
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

	c.applyRegisterToRegisterInstruction(inst, x2.register, x1.register)
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

	c.applyTwoRegistersToRegisterInstruction(inst, x2.register, x1.register, destinationReg)
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

	c.applyRegisterToRegisterInstruction(inst, x2.register, x1.register)
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

	c.applyTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

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

	c.applyTwoRegistersToNoneInstruction(inst, zeroRegister, x1.register)

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

	c.applyTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

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

	c.applyTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

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

	c.applyTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

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

	c.applyTwoRegistersToNoneInstruction(inst, x2.register, x1.register)

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
	c.maybeMoveTopConditionalToFreeGeneralPurposeRegister()

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

		c.locationStack.pushValueLocationOnRegister(reg)
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
	c.maybeMoveTopConditionalToFreeGeneralPurposeRegister()

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

	c.locationStack.pushValueLocationOnRegister(reg)
	return nil
}

func (c *arm64Compiler) pushZeroValue() {
	c.locationStack.pushValueLocationOnRegister(zeroRegister)
}

// popTwoValuesOnRegisters pops two values from the location stacks, ensures
// these two values are located on registers, and mark them unused.
func (c *arm64Compiler) popTwoValuesOnRegisters() (x1, x2 *valueLocation, err error) {
	x2, err = c.popValueOnRegister()
	if err != nil {
		return
	}

	x1, err = c.popValueOnRegister()
	return
}

// popValueOnRegister pops one value from the location stack, ensures
// that it is located on a register, and mark it unused.
func (c *arm64Compiler) popValueOnRegister() (v *valueLocation, err error) {
	v = c.locationStack.pop()
	if err = c.ensureOnGeneralPurposeRegister(v); err != nil {
		return
	}

	c.markRegisterUnused(v.register)
	return
}

// ensureOnGeneralPurposeRegister emits instructions to ensure that a value is located on a register.
func (c *arm64Compiler) ensureOnGeneralPurposeRegister(loc *valueLocation) (err error) {
	if loc.onStack() {
		err = c.loadValueOnStackToRegister(loc)
	} else if loc.onConditionalRegister() {
		c.loadConditionalRegisterToGeneralPurposeRegister(loc)
	}
	return
}

// maybeMoveTopConditionalToFreeGeneralPurposeRegister moves the top value on the stack
// if the value is located on a conditional register.
//
// This is usually called at the beginning of arm64Compiler.compile* functions where we possibly
// emit istructions without saving the conditional register value.
// The compile* functions without calling this function is saving the conditional
// value to the stack or register by invoking ensureOnGeneralPurposeRegister for the top.
func (c *arm64Compiler) maybeMoveTopConditionalToFreeGeneralPurposeRegister() {
	if c.locationStack.sp > 0 {
		if loc := c.locationStack.peek(); loc.onConditionalRegister() {
			c.loadConditionalRegisterToGeneralPurposeRegister(loc)
		}
	}
}

// loadConditionalRegisterToGeneralPurposeRegister saves the conditional register value
// to a general purpose register.
//
// We use CSET instruction to set 1 on the register if the condition satisfies:
// https://developer.arm.com/documentation/100076/0100/a64-instruction-set-reference/a64-general-instructions/cset
func (c *arm64Compiler) loadConditionalRegisterToGeneralPurposeRegister(loc *valueLocation) {
	// There must be always at least one free register at this point, as the conditional register located value
	// is always pushed after consuming at least one value (eqz) or two values for most cases (gt, ge, etc.).
	reg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	c.markRegisterUsed(reg)

	c.applyRegisterToRegisterInstruction(arm64.ACSET, int16(loc.conditionalRegister), reg)

	// Record that now the value is located on a general purpose register.
	loc.setRegister(reg)
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

	c.applyMemoryToRegisterInstruction(inst, reservedRegisterForStackBasePointerAddress, int64(loc.stackPointer)*8, reg)

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

// releaseAllRegistersToStack adds instructions to store all the values located on
// either general purpuse or conditional registers onto the memory stack.
// See releaseRegisterToStack.
func (c *arm64Compiler) releaseAllRegistersToStack() error {
	for i := uint64(0); i < c.locationStack.sp; i++ {
		if loc := c.locationStack.stack[i]; loc.onRegister() {
			if err := c.releaseRegisterToStack(loc); err != nil {
				return err
			}
		} else if loc.onConditionalRegister() {
			c.loadConditionalRegisterToGeneralPurposeRegister(loc)
			if err := c.releaseRegisterToStack(loc); err != nil {
				return err
			}
		}
	}
	return nil
}

// releaseRegisterToStack adds an instruction to write the value on a register back to memory stack region.
func (c *arm64Compiler) releaseRegisterToStack(loc *valueLocation) (err error) {
	var inst obj.As = arm64.AMOVD
	if loc.regType == generalPurposeRegisterTypeFloat {
		inst = arm64.AFMOVD
	}

	c.applyRegisterToMemoryInstruction(inst, loc.register, reservedRegisterForStackBasePointerAddress, int64(loc.stackPointer)*8)

	// Mark the register is free.
	c.locationStack.releaseRegister(loc)
	return
}

// initializeReservedStackBasePointerRegister adds intructions to initialize reservedRegisterForStackBasePointerAddress
// so that it points to the absolute address of the stack base for this function.
func (c *arm64Compiler) initializeReservedStackBasePointerRegister() error {
	// First, load the address of the first element in the value stack into reservedRegisterForStackBasePointerAddress temporarily.
	c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineGlobalContextValueStackElement0AddressOffset,
		reservedRegisterForStackBasePointerAddress)

	// Next we move the base pointer (engine.stackBasePointer) to the tmp register.
	c.applyMemoryToRegisterInstruction(arm64.AMOVD,
		reservedRegisterForEngine, engineValueStackContextStackBasePointerOffset,
		reservedRegisterForTemporary)

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
