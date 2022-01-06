//go:build amd64
// +build amd64

package jit

// This file implements the compiler for amd64/x86_64 target.
// Please refer to https://www.felixcloutier.com/x86/index.html
// if unfamiliar with amd64 instructions used here.
// Note that x86 pkg used here prefixes all the instructions with "A"
// e.g. MOVQ will be given as x86.AMOVQ.

import (
	"encoding/binary"
	"fmt"
	"math"
	"runtime"

	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

// jitcall is implemented in jit_amd64.s as a Go Assembler function.
// This is used by engine.exec and the entrypoint to enter the JITed native code.
// codeSegment is the pointer to the initial instruction of the compiled native code.
// engine is the pointer to the "*engine" as uintptr.
// memory is the pointer to the first byte of memoryInstance.Buffer slice to be used by the target function.
func jitcall(codeSegment, engine, memory uintptr)

func newCompiler(eng *engine, f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	// We can choose arbitrary number instead of 1024 which indicates the cache size in the compiler.
	// TODO: optimize the number.
	b, err := asm.NewBuilder("amd64", 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}

	return &amd64Compiler{
		eng: eng, f: f, builder: b, locationStack: newValueLocationStack(), ir: ir,
		labelInitialInstructions: make(map[string]*obj.Prog),
		onLabelStartCallbacks:    make(map[string][]func(*obj.Prog)),
	}, nil
}

type amd64Compiler struct {
	eng *engine
	f   *wasm.FunctionInstance
	ir  *wazeroir.CompilationResult
	// Set a jmp kind instruction where you want to set the next coming
	// instruction as the destination of the jmp instruction.
	setJmpOrigin *obj.Prog
	builder      *asm.Builder
	// location stack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
	// Label resolvers.
	onLabelStartCallbacks map[string][]func(*obj.Prog)
	// Store the initial instructions for each label so
	// other block can jump into it.
	labelInitialInstructions                         map[string]*obj.Prog
	requireFunctionCallReturnAddressOffsetResolution []*obj.Prog
}

func (c *amd64Compiler) emitPreamble() {
	// We assume all function parameters are already pushed onto the stack by
	// the caller.
	c.pushFunctionParams()
	// Initialize the reserved registers first of all.
	c.initializeReservedRegisters()
}

func (c *amd64Compiler) generate() ([]byte, uint64, error) {
	code, err := mmapCodeSegment(c.builder.Assemble())
	if err != nil {
		return nil, 0, err
	}
	// As we cannot read RIP register directly,
	// we calculate now the offset to the next instruction
	// relative to the beginning of this function body.
	const operandSizeBytes = 8
	for _, obj := range c.requireFunctionCallReturnAddressOffsetResolution {
		// Skip MOV, and the register(rax): "0x49, 0xbd"
		start := obj.Pc + 2
		// obj.Link = setting offset to memory
		// obj.Link.Link = writing back the stack pointer to eng.stackPointer.
		// obj.Link.Link.Link = Return instruction.
		// Therefore obj.Link.Link.Link.Link means the next instruction after the return.
		afterReturnInst := obj.Link.Link.Link.Link
		binary.LittleEndian.PutUint64(code[start:start+operandSizeBytes], uint64(afterReturnInst.Pc))
	}
	return code, c.locationStack.maxStackPointer, nil
}

func (c *amd64Compiler) pushFunctionParams() {
	for _, t := range c.f.Signature.Params {
		loc := c.locationStack.pushValueOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
	}
}

func (c *amd64Compiler) addInstruction(prog *obj.Prog) {
	c.builder.AddInstruction(prog)
	if c.setJmpOrigin != nil {
		c.setJmpOrigin.To.SetTarget(prog)
		c.setJmpOrigin = nil
	}
}

func (c *amd64Compiler) newProg() (prog *obj.Prog) {
	prog = c.builder.NewProg()
	return
}

func (c *amd64Compiler) compileUnreachable() {
	c.releaseAllRegistersToStack()
	c.setJITStatus(jitCallStatusCodeUnreachable)
	c.returnFunction()
}

func (c *amd64Compiler) compileSwap(o *wazeroir.OperationSwap) error {
	index := len(c.locationStack.stack) - 1 - o.Depth
	// Note that, in theory, the register types and value types
	// are the same between these swap targets as swap operations
	// are generated from local.set,tee instructions in Wasm.
	x1 := c.locationStack.stack[len(c.locationStack.stack)-1]
	x2 := c.locationStack.stack[index]

	// If x1 is on the conditional register, we must move it to a gp
	// register before swap.
	if x1.onConditionalRegister() {
		if err := c.moveConditionalToFreeGeneralPurposeRegister(x1); err != nil {
			return err
		}
	}

	if x1.onRegister() && x2.onRegister() {
		x1.register, x2.register = x2.register, x1.register
	} else if x1.onRegister() && x2.onStack() {
		reg := x1.register
		// Save x1's value to the temporary top of the stack.
		tmpStackLocation := c.locationStack.pushValueOnRegister(reg)
		c.releaseRegisterToStack(tmpStackLocation)
		// Then move the x2's value to the x1's register location.
		x2.register = reg
		c.moveStackToRegister(x2)
		// Now move the x1's value to the x1's stack location.
		c.releaseRegisterToStack(x1)
		// Next we move the saved x1's value to the register.
		tmpStackLocation.setRegister(reg)
		c.moveStackToRegister(tmpStackLocation)
		// Finally move the x1's value in the register to the x2's stack location.
		c.locationStack.releaseRegister(x1)
		c.locationStack.releaseRegister(tmpStackLocation)
		x2.setRegister(reg)
		c.locationStack.markRegisterUsed(reg)
		_ = c.locationStack.pop() // Delete tmpStackLocation.
	} else if x1.onStack() && x2.onRegister() {
		reg := x2.register
		// Save x2's value to the temporary top of the stack.
		tmpStackLocation := c.locationStack.pushValueOnRegister(reg)
		c.releaseRegisterToStack(tmpStackLocation)
		// Then move the x1's value to the x2's register location.
		x1.register = reg
		c.moveStackToRegister(x1)
		// Now move the x1's value to the x2's stack location.
		c.releaseRegisterToStack(x2)
		// Next we move the saved x2's value to the register.
		tmpStackLocation.setRegister(reg)
		c.moveStackToRegister(tmpStackLocation)
		// Finally move the x2's value in the register to the x2's stack location.
		c.locationStack.releaseRegister(x2)
		c.locationStack.releaseRegister(tmpStackLocation)
		x1.setRegister(reg)
		c.locationStack.markRegisterUsed(reg)
		_ = c.locationStack.pop() // Delete tmpStackLocation.
	} else if x1.onStack() && x2.onStack() {
		reg, err := c.allocateRegister(x1.registerType())
		if err != nil {
			return err
		}
		// First we move the x2's value to the temp register.
		x2.setRegister(reg)
		c.moveStackToRegister(x2)
		// Save x2's value to the temporary top of the stack.
		tmpStackLocation := c.locationStack.pushValueOnRegister(reg)
		c.releaseRegisterToStack(tmpStackLocation)
		// Then move the x1's value to the x2's register location.
		x1.register = reg
		c.moveStackToRegister(x1)
		// Now move the x1's value to the x2's stack location.
		c.releaseRegisterToStack(x2)
		// Next we move the saved x2's value to the register.
		tmpStackLocation.setRegister(reg)
		c.moveStackToRegister(tmpStackLocation)
		// Finally move the x2's value in the register to the x2's stack location.
		c.locationStack.releaseRegister(x2)
		c.locationStack.releaseRegister(tmpStackLocation)
		x1.setRegister(reg)
		c.locationStack.markRegisterUsed(reg)
		_ = c.locationStack.pop() // Delete tmpStackLocation.
	}
	return nil
}

const globalInstanceValueOffset = 8

func (c *amd64Compiler) compileGlobalGet(o *wazeroir.OperationGlobalGet) error {
	intReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, move the pointer to the global slice into the allocated register.
	moveGlobalSlicePointer := c.newProg()
	moveGlobalSlicePointer.As = x86.AMOVQ
	moveGlobalSlicePointer.To.Type = obj.TYPE_REG
	moveGlobalSlicePointer.To.Reg = intReg
	moveGlobalSlicePointer.From.Type = obj.TYPE_MEM
	moveGlobalSlicePointer.From.Reg = reservedRegisterForEngine
	moveGlobalSlicePointer.From.Offset = engineglobalSliceAddressOffset
	c.addInstruction(moveGlobalSlicePointer)

	// Then, get the memory location of the target global instance's pointer.
	getGlobalInstanceLocation := c.newProg()
	getGlobalInstanceLocation.As = x86.AADDQ
	getGlobalInstanceLocation.To.Type = obj.TYPE_REG
	getGlobalInstanceLocation.To.Reg = intReg
	getGlobalInstanceLocation.From.Type = obj.TYPE_CONST
	getGlobalInstanceLocation.From.Offset = 8 * int64(o.Index)
	c.addInstruction(getGlobalInstanceLocation)

	// Now, move the location of the global instance into the register.
	getGlobalInstancePointer := c.newProg()
	getGlobalInstancePointer.As = x86.AMOVQ
	getGlobalInstancePointer.To.Type = obj.TYPE_REG
	getGlobalInstancePointer.To.Reg = intReg
	getGlobalInstancePointer.From.Type = obj.TYPE_MEM
	getGlobalInstancePointer.From.Reg = intReg
	c.addInstruction(getGlobalInstancePointer)

	// When an integer, reuse the pointer register for the value. Otherwise, allocate a float register for it.
	valueReg := intReg
	wasmType := c.f.ModuleInstance.Globals[o.Index].Type.ValType
	switch wasmType {
	case wasm.ValueTypeF32, wasm.ValueTypeF64:
		valueReg, err = c.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
	}

	// Using the register holding the pointer to the target instance, move its value into a register.
	moveValue := c.newProg()
	moveValue.As = x86.AMOVQ
	moveValue.To.Type = obj.TYPE_REG
	moveValue.To.Reg = valueReg
	moveValue.From.Type = obj.TYPE_MEM
	moveValue.From.Reg = intReg
	moveValue.From.Offset = globalInstanceValueOffset
	c.addInstruction(moveValue)

	// Record that the retrieved global value on the top of the stack is now in a register.
	loc := c.locationStack.pushValueOnRegister(valueReg)
	switch wasmType {
	case wasm.ValueTypeI32, wasm.ValueTypeI64:
		loc.setRegisterType(generalPurposeRegisterTypeInt)
	case wasm.ValueTypeF32, wasm.ValueTypeF64:
		loc.setRegisterType(generalPurposeRegisterTypeFloat)
	}
	return nil
}

func (c *amd64Compiler) compileGlobalSet(o *wazeroir.OperationGlobalSet) error {
	// First, move the value to set into a temporary register.
	val := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	// Allocate a register to hold the memory location of the target global instance.
	intReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, move the pointer to the global slice into the allocated register.
	moveGlobalSlicePointer := c.newProg()
	moveGlobalSlicePointer.As = x86.AMOVQ
	moveGlobalSlicePointer.To.Type = obj.TYPE_REG
	moveGlobalSlicePointer.To.Reg = intReg
	moveGlobalSlicePointer.From.Type = obj.TYPE_MEM
	moveGlobalSlicePointer.From.Reg = reservedRegisterForEngine
	moveGlobalSlicePointer.From.Offset = engineglobalSliceAddressOffset
	c.addInstruction(moveGlobalSlicePointer)

	// Then, get the memory location of the target global instance's pointer.
	getGlobalInstanceLocation := c.newProg()
	getGlobalInstanceLocation.As = x86.AADDQ
	getGlobalInstanceLocation.To.Type = obj.TYPE_REG
	getGlobalInstanceLocation.To.Reg = intReg
	getGlobalInstanceLocation.From.Type = obj.TYPE_CONST
	getGlobalInstanceLocation.From.Offset = 8 * int64(o.Index)
	c.addInstruction(getGlobalInstanceLocation)

	// Now, move the location of the global instance into the register.
	getGlobalInstancePointer := c.newProg()
	getGlobalInstancePointer.As = x86.AMOVQ
	getGlobalInstancePointer.To.Type = obj.TYPE_REG
	getGlobalInstancePointer.To.Reg = intReg
	getGlobalInstancePointer.From.Type = obj.TYPE_MEM
	getGlobalInstancePointer.From.Reg = intReg
	c.addInstruction(getGlobalInstancePointer)

	// Now ready to write the value to the global instance location.
	moveValue := c.newProg()
	moveValue.As = x86.AMOVQ
	moveValue.From.Type = obj.TYPE_REG
	moveValue.From.Reg = val.register
	moveValue.To.Type = obj.TYPE_MEM
	moveValue.To.Reg = intReg
	moveValue.To.Offset = globalInstanceValueOffset
	c.addInstruction(moveValue)

	// Since the value is now written to memory, release the value register.
	c.locationStack.releaseRegister(val)
	return nil
}

func (c *amd64Compiler) compileBr(o *wazeroir.OperationBr) error {
	if o.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		c.releaseAllRegistersToStack()
		c.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		c.returnFunction()
	} else {
		labelKey := o.Target.String()
		targetNumCallers := c.ir.LabelCallers[labelKey]
		if targetNumCallers > 1 {
			// If the number of callers to the target label is larget than one,
			// we have multiple origins to the target branch. In that case,
			// we must have unique register state.
			c.preJumpRegisterAdjustment()
		}
		jmp := c.newProg()
		jmp.As = obj.AJMP
		jmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(jmp)
		c.assignJumpTarget(labelKey, jmp)
	}
	return nil
}

func (c *amd64Compiler) compileBrIf(o *wazeroir.OperationBrIf) error {
	cond := c.locationStack.pop()
	var jmpWithCond *obj.Prog
	if cond.onConditionalRegister() {
		jmpWithCond = c.newProg()
		jmpWithCond.To.Type = obj.TYPE_BRANCH
		switch cond.conditionalRegister {
		case conditionalRegisterStateE:
			jmpWithCond.As = x86.AJEQ
		case conditionalRegisterStateNE:
			jmpWithCond.As = x86.AJNE
		case conditionalRegisterStateS:
			jmpWithCond.As = x86.AJMI
		case conditionalRegisterStateNS:
			jmpWithCond.As = x86.AJPL
		case conditionalRegisterStateG:
			jmpWithCond.As = x86.AJGT
		case conditionalRegisterStateGE:
			jmpWithCond.As = x86.AJGE
		case conditionalRegisterStateL:
			jmpWithCond.As = x86.AJLT
		case conditionalRegisterStateLE:
			jmpWithCond.As = x86.AJLE
		case conditionalRegisterStateA:
			jmpWithCond.As = x86.AJHI
		case conditionalRegisterStateAE:
			jmpWithCond.As = x86.AJCC
		case conditionalRegisterStateB:
			jmpWithCond.As = x86.AJCS
		case conditionalRegisterStateBE:
			jmpWithCond.As = x86.AJLS
		}
	} else {
		// Usually the comparison operand for br_if is on the conditional register,
		// but in some cases, they are on the stack or register.
		// For example, the following code
		// 		i64.const 1
		//      local.get 1
		//      i64.add
		//      br_if ....
		// will try to use the result of i64.add, which resides on the (virtual) stack,
		// as the operand for br_if instruction.
		if cond.onStack() {
			// This case even worse, the operand is not on a allocated register, but
			// actually in the stack memory, so we have to assign a register to it
			// before we judge if we should jump to the Then branch or Else.
			if err := c.moveStackToRegisterWithAllocation(cond.registerType(), cond); err != nil {
				return err
			}
		}
		// Check if the value not equals zero.
		prog := c.newProg()
		prog.As = x86.ACMPQ
		prog.From.Type = obj.TYPE_REG
		prog.From.Reg = cond.register
		prog.To.Type = obj.TYPE_CONST
		prog.To.Offset = 0
		c.addInstruction(prog)
		// Emit jump instruction which jumps when the value does not equals zero.
		jmpWithCond = c.newProg()
		jmpWithCond.As = x86.AJNE
		jmpWithCond.To.Type = obj.TYPE_BRANCH
	}

	// Make sure that the next coming label is the else jump target.
	c.addInstruction(jmpWithCond)
	thenTarget, elseTarget := o.Then, o.Else

	// Here's the diagram of how we organize the instructions necessarly for brif operation.
	//
	// jmp_with_cond -> jmp (.Else) -> Then operations...
	//    |---------(satisfied)------------^^^
	//
	// Note that .Else branch doesn't have ToDrop as .Else is in reality
	// corresponding to either If's Else block or Br_if's else block in Wasm.

	// Emit for else branches
	saved := c.locationStack
	c.locationStack = saved.clone()
	if elseTarget.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		c.releaseAllRegistersToStack()
		c.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		c.returnFunction()
	} else {
		elseLabelKey := elseTarget.Target.Label.String()
		if c.ir.LabelCallers[elseLabelKey] > 1 {
			c.preJumpRegisterAdjustment()
		}
		elseJmp := c.newProg()
		elseJmp.As = obj.AJMP
		elseJmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(elseJmp)
		c.assignJumpTarget(elseLabelKey, elseJmp)
	}

	// Handle then branch.
	c.setJmpOrigin = jmpWithCond
	c.locationStack = saved
	if err := c.emitDropRange(thenTarget.ToDrop); err != nil {
		return err
	}
	if thenTarget.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		c.releaseAllRegistersToStack()
		c.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		c.returnFunction()
	} else {
		thenLabelKey := thenTarget.Target.Label.String()
		if c.ir.LabelCallers[thenLabelKey] > 1 {
			c.preJumpRegisterAdjustment()
		}
		thenJmp := c.newProg()
		thenJmp.As = obj.AJMP
		thenJmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(thenJmp)
		c.assignJumpTarget(thenLabelKey, thenJmp)
	}
	return nil
}

// If a jump target has multiple callesr (origins),
// we must have unique register states, so this function
// must be called before such jump instruction.
func (c *amd64Compiler) preJumpRegisterAdjustment() {
	// For now, we just release all registers to memory.
	// But this is obviously inefficient, so we come back here
	// later once we finish the baseline implementation.
	c.releaseAllRegistersToStack()
}

func (c *amd64Compiler) assignJumpTarget(labelKey string, jmpInstruction *obj.Prog) {
	jmpTarget, ok := c.labelInitialInstructions[labelKey]
	if ok {
		jmpInstruction.To.SetTarget(jmpTarget)
	} else {
		c.onLabelStartCallbacks[labelKey] = append(c.onLabelStartCallbacks[labelKey], func(jmpTarget *obj.Prog) {
			jmpInstruction.To.SetTarget(jmpTarget)
		})
	}
}

func (c *amd64Compiler) compileLabel(o *wazeroir.OperationLabel) error {
	c.locationStack.sp = uint64(o.Label.OriginalStackLen)
	// We use NOP as a beginning of instructions in a label.
	// This should be eventually optimized out by assembler.
	labelKey := o.Label.String()
	labelBegin := c.newProg()
	labelBegin.As = obj.ANOP
	c.addInstruction(labelBegin)
	// Save the instructions so that backward branching
	// instructions can jump to this label.
	c.labelInitialInstructions[labelKey] = labelBegin
	// Invoke callbacks to notify the forward branching
	// instructions can properly jump to this label.
	for _, cb := range c.onLabelStartCallbacks[labelKey] {
		cb(labelBegin)
	}
	// Now we don't need to call the callbacks.
	delete(c.onLabelStartCallbacks, labelKey)
	return nil
}

func (c *amd64Compiler) compileCall(o *wazeroir.OperationCall) error {
	target := c.f.ModuleInstance.Functions[o.FunctionIndex]
	if target.HostFunction != nil {
		index := c.eng.compiledHostFunctionIndex[target]
		c.callHostFunctionFromConstIndex(index)
	} else {
		index := c.eng.compiledWasmFunctionIndex[target]
		c.callFunctionFromConstIndex(index)
	}
	return nil
}
func (c *amd64Compiler) compileDrop(o *wazeroir.OperationDrop) error {
	return c.emitDropRange(o.Range)
}

func (c *amd64Compiler) emitDropRange(r *wazeroir.InclusiveRange) error {
	if r == nil {
		return nil
	} else if r.Start == 0 {
		for i := 0; i < r.End; i++ {
			if loc := c.locationStack.pop(); loc.onRegister() {
				c.locationStack.releaseRegister(loc)
			}
		}
		return nil
	}

	var (
		top              *valueLocation
		topIsConditional bool
		liveValues       []*valueLocation
	)
	for i := 0; i < r.Start; i++ {
		live := c.locationStack.pop()
		if top == nil {
			top = live
			topIsConditional = top.onConditionalRegister()
		}
		liveValues = append(liveValues, live)
	}
	for i := 0; i < r.End-r.Start+1; i++ {
		if loc := c.locationStack.pop(); loc.onRegister() {
			c.locationStack.releaseRegister(loc)
		}
	}
	for i := range liveValues {
		live := liveValues[len(liveValues)-1-i]
		if live.onStack() {
			if topIsConditional {
				// If the top is conditional, and it's not target of drop,
				// we must assign it to the register before we emit any instructions here.
				if err := c.moveConditionalToFreeGeneralPurposeRegister(top); err != nil {
					return err
				}
				topIsConditional = false
			}
			// Write the value in the old stack location to a register
			if err := c.moveStackToRegisterWithAllocation(live.registerType(), live); err != nil {
				return err
			}
			// Modify the location in the stack with new stack pointer.
			c.locationStack.push(live)
		} else if live.onRegister() {
			c.locationStack.push(live)
		}
	}
	return nil
}

// compileSelect uses top three values on the stack:
// Assume we have stack as [..., x1, x2, c], if the value of c
// equals zero, then the stack results in [..., x1]
// otherwise, [..., x2].
// The emitted native code depends on whether the values are on
// the physical registers or memory stack, or maybe conditional register.
func (c *amd64Compiler) compileSelect() error {
	cv := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(cv); err != nil {
		return err
	}

	x2 := c.locationStack.pop()
	// We do not consume x1 here, but modify the value according to
	// the conditional value "c" above.
	peekedX1 := c.locationStack.peek()

	// Compare the conditional value with zero.
	cmpZero := c.newProg()
	cmpZero.As = x86.ACMPQ
	cmpZero.From.Type = obj.TYPE_REG
	cmpZero.From.Reg = cv.register
	cmpZero.To.Type = obj.TYPE_CONST
	cmpZero.To.Offset = 0
	c.addInstruction(cmpZero)

	// Now we can use c.register as temporary location.
	// We alias it here for readability.
	tmpRegister := cv.register

	// Set the jump if the top value is not zero.
	jmpIfNotZero := c.newProg()
	jmpIfNotZero.As = x86.AJNE
	jmpIfNotZero.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNotZero)

	// If the value is zero, we must place the value of x2 onto the stack position of x1.

	// First we copy the value of x2 to the temporary register if x2 is not currently on a register.
	if x2.onStack() {
		x2.register = tmpRegister
		c.moveStackToRegister(x2)
	}

	//
	// At this point x2's value is always on a register.
	//

	// Then release the value in the x2's register to the x1's stack position.
	if peekedX1.onRegister() {
		movX2ToX1 := c.newProg()
		movX2ToX1.As = x86.AMOVQ
		movX2ToX1.From.Type = obj.TYPE_REG
		movX2ToX1.From.Reg = x2.register
		movX2ToX1.To.Type = obj.TYPE_REG
		movX2ToX1.To.Reg = peekedX1.register
		c.addInstruction(movX2ToX1)
	} else {
		peekedX1.register = x2.register
		c.releaseRegisterToStack(peekedX1) // Note inside we mark the register unused!
	}

	// Else, we don't need to adjust value, just need to jump to the next instruction.
	c.setJmpOrigin = jmpIfNotZero

	// In any case, we don't need x2 and c anymore!
	c.locationStack.releaseRegister(x2)
	c.locationStack.releaseRegister(cv)
	return nil
}

func (c *amd64Compiler) compilePick(o *wazeroir.OperationPick) error {
	// TODO: if we track the type of values on the stack,
	// we could optimize the instruction according to the bit size of the value.
	// For now, we just move the entire register i.e. as a quad word (8 bytes).
	pickTarget := c.locationStack.stack[c.locationStack.sp-1-uint64(o.Depth)]
	reg, err := c.allocateRegister(pickTarget.registerType())
	if err != nil {
		return err
	}

	if pickTarget.onRegister() {
		prog := c.newProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_REG
		prog.From.Reg = pickTarget.register
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = reg
		c.addInstruction(prog)
	} else if pickTarget.onStack() {
		// Copy the value from the stack.
		prog := c.newProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_MEM
		prog.From.Reg = reservedRegisterForStackBasePointer
		prog.From.Offset = int64(pickTarget.stackPointer) * 8
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = reg
		c.addInstruction(prog)
	} else if pickTarget.onConditionalRegister() {
		panic("TODO")
	}
	// Now we already placed the picked value on the register,
	// so push the location onto the stack.
	loc := c.locationStack.pushValueOnRegister(reg)
	loc.setRegisterType(pickTarget.registerType())
	return nil
}

func (c *amd64Compiler) compileAdd(o *wazeroir.OperationAdd) error {
	// TODO: if the previous instruction is const, then
	// this can be optimized. Same goes for other arithmetic instructions.

	var instruction obj.As
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		instruction = x86.AADDL
	case wazeroir.UnsignedTypeI64:
		instruction = x86.AADDQ
	case wazeroir.UnsignedTypeF32:
		instruction = x86.AADDSS
	case wazeroir.UnsignedTypeF64:
		instruction = x86.AADDSD
	}

	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek, pop!
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// x1 += x2.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x2.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x1.register
	prog.As = instruction
	c.addInstruction(prog)

	// We no longer need x2 register after ADD operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

func (c *amd64Compiler) compileSub(o *wazeroir.OperationSub) error {
	// TODO: if the previous instruction is const, then
	// this can be optimized. Same goes for other arithmetic instructions.

	var instruction obj.As
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		instruction = x86.ASUBL
	case wazeroir.UnsignedTypeI64:
		instruction = x86.ASUBQ
	case wazeroir.UnsignedTypeF32:
		instruction = x86.ASUBSS
	case wazeroir.UnsignedTypeF64:
		instruction = x86.ASUBSD
	}

	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek, pop!
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// x1 -= x2.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x2.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x1.register
	prog.As = instruction
	c.addInstruction(prog)

	// We no longer need x2 register after ADD operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileMul adds instructions to multiply two operands which may be on the stack or registers.
// After execution, the result of multiplication pushed onto the stack.
func (c *amd64Compiler) compileMul(o *wazeroir.OperationMul) (err error) {
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		err = c.compileMulForInts(true, x86.AMULL)
	case wazeroir.UnsignedTypeI64:
		err = c.compileMulForInts(false, x86.AMULQ)
	case wazeroir.UnsignedTypeF32:
		err = c.compileMulForFloats(x86.AMULSS)
	case wazeroir.UnsignedTypeF64:
		err = c.compileMulForFloats(x86.AMULSD)
	}
	return
}

// compileMulForInts emits instructions to perform integer multiplication for
// top two values on the stack. If unfamiliar with the convention for integer
// multiplication on x86, see https://www.felixcloutier.com/x86/mul.
//
// In summary, one of the values must be on the AX register,
// and the mul instruction stores the overflow info in DX register which we don't use.
// Here, we mean "the overflow info" by 65 bit or higher part of the result for 64 bit case.
//
// So, we have to ensure that
// 1) Previously located value on DX must be saved to memory stack. That is because
//    the existing value will be overriden after the mul execution.
// 2) One of the operands (x1 or x2) must be on AX register.
// See https://www.felixcloutier.com/x86/mul#description for detail semantics.
func (c *amd64Compiler) compileMulForInts(is32Bit bool, mulInstruction obj.As) error {
	const (
		resultRegister   = x86.REG_AX
		reservedRegister = x86.REG_DX
	)

	x2 := c.locationStack.pop()
	x1 := c.locationStack.pop()

	var valueOnAX *valueLocation
	if x1.register == resultRegister {
		valueOnAX = x1
	} else if x2.register == resultRegister {
		valueOnAX = x2
	} else {
		valueOnAX = x2
		// This case we  move x2 to AX register.
		c.onValueReleaseRegisterToStack(resultRegister)
		if x2.onConditionalRegister() {
			c.moveConditionalToGeneralPurposeRegister(x2, resultRegister)
		} else if x2.onStack() {
			x2.setRegister(resultRegister)
			c.moveStackToRegister(x2)
			c.locationStack.markRegisterUsed(resultRegister)
		} else {
			moveX2ToAX := c.newProg()
			if is32Bit {
				moveX2ToAX.As = x86.AMOVL
			} else {
				moveX2ToAX.As = x86.AMOVQ
			}
			moveX2ToAX.To.Reg = resultRegister
			moveX2ToAX.To.Type = obj.TYPE_REG
			moveX2ToAX.From.Reg = x2.register
			moveX2ToAX.From.Type = obj.TYPE_REG
			c.addInstruction(moveX2ToAX)
			// We no longer uses the prev register of x2.
			c.locationStack.releaseRegister(x2)
			x2.setRegister(resultRegister)
			c.locationStack.markRegisterUsed(resultRegister)
		}
	}

	// We have to make sure that at this point the operands must be on registers.
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// We have to save the existing value on DX.
	// If the DX register is used by either x1 or x2, we don't need to
	// save the value because it is consumed by mul anyway.
	if x1.register != reservedRegister && x2.register != reservedRegister {
		c.onValueReleaseRegisterToStack(reservedRegister)
	}

	// Now ready to emit the mul instruction.
	mul := c.newProg()
	mul.As = mulInstruction
	mul.To.Type = obj.TYPE_NONE
	mul.From.Type = obj.TYPE_REG
	if x1 == valueOnAX {
		mul.From.Reg = x2.register
		c.locationStack.markRegisterUnused(x2.register)
	} else {
		mul.From.Reg = x1.register
		c.locationStack.markRegisterUnused(x1.register)
	}
	c.addInstruction(mul)

	// Now we have the result in the AX register,
	// so we record it.
	result := c.locationStack.pushValueOnRegister(resultRegister)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileMulForFloats(instruction obj.As) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// x1 *= x2.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x2.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x1.register
	prog.As = instruction
	c.addInstruction(prog)

	// We no longer need x2 register after MUL operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileClz emits instructions to count up the leading zeros in the
// current top of the stack, and push the count result.
// For example, stack of [..., 0x00_ff_ff_ff] results in [..., 8].
func (c *amd64Compiler) compileClz(o *wazeroir.OperationClz) error {
	target := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		countZeros := c.newProg()
		countZeros.From.Type = obj.TYPE_REG
		countZeros.From.Reg = target.register
		countZeros.To.Type = obj.TYPE_REG
		countZeros.To.Reg = target.register
		if o.Type == wazeroir.UnsignedInt32 {
			countZeros.As = x86.ALZCNTL
		} else {
			countZeros.As = x86.ALZCNTQ
		}
		c.addInstruction(countZeros)
	} else {
		// On x86 mac, we cannot use LZCNT as it always results in zero.
		// Instead we combine BSR (calculating most significant set bit)
		// with XOR. This logic is described in
		// "Replace Raw Assembly Code with Builtin Intrinsics" section in:
		// https://developer.apple.com/documentation/apple-silicon/addressing-architectural-differences-in-your-macos-code.

		// First, we have to check if the target is non-zero as BSR is undefined
		// on zero. See https://www.felixcloutier.com/x86/bsr.
		cmpZero := c.newProg()
		cmpZero.As = x86.ACMPQ
		cmpZero.From.Type = obj.TYPE_REG
		cmpZero.From.Reg = target.register
		cmpZero.To.Type = obj.TYPE_CONST
		cmpZero.To.Offset = 0
		c.addInstruction(cmpZero)

		jmpIfNonZero := c.newProg()
		jmpIfNonZero.As = x86.AJNE
		jmpIfNonZero.To.Type = obj.TYPE_BRANCH
		c.addInstruction(jmpIfNonZero)

		// If the value is zero, we just push the const value.
		if o.Type == wazeroir.UnsignedInt32 {
			c.emitConstI32(32, target.register)
		} else {
			c.emitConstI64(64, target.register)
		}

		// Emit the jmp instruction to jump to the position right after
		// the non-zero case.
		jmpAtEndOfZero := c.newProg()
		jmpAtEndOfZero.As = obj.AJMP
		jmpAtEndOfZero.To.Type = obj.TYPE_BRANCH
		c.addInstruction(jmpAtEndOfZero)

		// Start emitting non-zero case.
		// First, we calculate the most significant set bit.
		mostSignificantSetBit := c.newProg()
		// Set the jump target of the first non-zero check.
		jmpIfNonZero.To.SetTarget(mostSignificantSetBit)
		if o.Type == wazeroir.UnsignedInt32 {
			mostSignificantSetBit.As = x86.ABSRL
		} else {
			mostSignificantSetBit.As = x86.ABSRQ
		}
		mostSignificantSetBit.From.Type = obj.TYPE_REG
		mostSignificantSetBit.From.Reg = target.register
		mostSignificantSetBit.To.Type = obj.TYPE_REG
		mostSignificantSetBit.To.Reg = target.register
		c.addInstruction(mostSignificantSetBit)

		// Now we XOR the value with the bit length minus one.
		xorWithBitLength := c.newProg()
		xorWithBitLength.To.Type = obj.TYPE_REG
		xorWithBitLength.To.Reg = target.register
		xorWithBitLength.From.Type = obj.TYPE_CONST
		if o.Type == wazeroir.UnsignedInt32 {
			xorWithBitLength.As = x86.AXORL
			xorWithBitLength.From.Offset = 31
		} else {
			xorWithBitLength.As = x86.AXORQ
			xorWithBitLength.From.Offset = 63
		}
		c.addInstruction(xorWithBitLength)

		// Finally the end jump instruction of zero case must target towards
		// the next instruction.
		c.setJmpOrigin = jmpAtEndOfZero
	}

	// We reused the same register of target for the result.
	result := c.locationStack.pushValueOnRegister(target.register)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileCtz emits instructions to count up the trailing zeros in the
// current top of the stack, and push the count result.
// For example, stack of [..., 0xff_ff_ff_00] results in [..., 8].
func (c *amd64Compiler) compileCtz(o *wazeroir.OperationCtz) error {
	target := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		countZeros := c.newProg()
		countZeros.From.Type = obj.TYPE_REG
		countZeros.From.Reg = target.register
		countZeros.To.Type = obj.TYPE_REG
		countZeros.To.Reg = target.register
		if o.Type == wazeroir.UnsignedInt32 {
			countZeros.As = x86.ATZCNTL
		} else {
			countZeros.As = x86.ATZCNTQ
		}
		c.addInstruction(countZeros)
	} else {
		// Somehow, if the target value is zero, TZCNT always returns zero: this is wrong.
		// Meanwhile, we need branches for non-zero and zero cases on macos.
		// TODO: find the reference to this behavior and put the link here.

		// First we compare the target with zero.
		cmpZero := c.newProg()
		cmpZero.As = x86.ACMPQ
		cmpZero.From.Type = obj.TYPE_REG
		cmpZero.From.Reg = target.register
		cmpZero.To.Type = obj.TYPE_CONST
		cmpZero.To.Offset = 0
		c.addInstruction(cmpZero)

		jmpIfNonZero := c.newProg()
		jmpIfNonZero.As = x86.AJNE
		jmpIfNonZero.To.Type = obj.TYPE_BRANCH
		c.addInstruction(jmpIfNonZero)

		// If the value is zero, we just push the const value.
		if o.Type == wazeroir.UnsignedInt32 {
			c.emitConstI32(32, target.register)
		} else {
			c.emitConstI64(64, target.register)
		}

		// Emit the jmp instruction to jump to the position right after
		// the non-zero case.
		jmpAtEndOfZero := c.newProg()
		jmpAtEndOfZero.As = obj.AJMP
		jmpAtEndOfZero.To.Type = obj.TYPE_BRANCH
		c.addInstruction(jmpAtEndOfZero)

		// Otherwise, emit the TZCNT.
		countZeros := c.newProg()
		jmpIfNonZero.To.SetTarget(countZeros)
		countZeros.From.Type = obj.TYPE_REG
		countZeros.From.Reg = target.register
		countZeros.To.Type = obj.TYPE_REG
		countZeros.To.Reg = target.register
		if o.Type == wazeroir.UnsignedInt32 {
			countZeros.As = x86.ATZCNTL
		} else {
			countZeros.As = x86.ATZCNTQ
		}
		c.addInstruction(countZeros)

		// Finally the end jump instruction of zero case must target towards
		// the next instruction.
		c.setJmpOrigin = jmpAtEndOfZero
	}

	// We reused the same register of target for the result.
	result := c.locationStack.pushValueOnRegister(target.register)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compilePopcnt emits instructions to count up the number of set bits in the
// current top of the stack, and push the count result.
// For example, stack of [..., 0b00_00_00_11] results in [..., 2].
func (c *amd64Compiler) compilePopcnt(o *wazeroir.OperationPopcnt) error {
	target := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	countBits := c.newProg()
	countBits.From.Type = obj.TYPE_REG
	countBits.From.Reg = target.register
	countBits.To.Type = obj.TYPE_REG
	countBits.To.Reg = target.register
	if o.Type == wazeroir.UnsignedInt32 {
		countBits.As = x86.APOPCNTL
	} else {
		countBits.As = x86.APOPCNTQ
	}
	c.addInstruction(countBits)

	// We reused the same register of target for the result.
	result := c.locationStack.pushValueOnRegister(target.register)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileDiv emits the instructions to perform division on the top two values on the stack.
func (c *amd64Compiler) compileDiv(o *wazeroir.OperationDiv) (err error) {
	switch o.Type {
	case wazeroir.SignedTypeUint32:
		err = c.compileDivForInts(true, false)
	case wazeroir.SignedTypeUint64:
		err = c.compileDivForInts(false, false)
	case wazeroir.SignedTypeInt32:
		err = c.compileDivForInts(true, true)
	case wazeroir.SignedTypeInt64:
		err = c.compileDivForInts(false, true)
	case wazeroir.SignedTypeFloat32:
		err = c.compileDivForFloats(true)
	case wazeroir.SignedTypeFloat64:
		err = c.compileDivForFloats(false)
	}
	return
}

// compileDivForInts emits the instructions to perform division on the top
// two values of integer type on the stack and puts the quotient of the result
// onto the stack. For example, stack [..., 4, 3] results in [..., 1] where
// the remainder is discarded. See compileRem for how to acquire remainder, not quotient.
func (c *amd64Compiler) compileDivForInts(is32Bit bool, signed bool) error {
	if err := c.performDivisionOnInts(is32Bit, signed); err != nil {
		return err
	}
	// Now we have the quotient of the division result in the AX register,
	// so we record it.
	result := c.locationStack.pushValueOnRegister(x86.REG_AX)
	result.setRegisterType(generalPurposeRegisterTypeInt)

	// Make sure that the DX register (holding remainder of the division) is marked unused.
	c.locationStack.markRegisterUnused(x86.REG_DX)
	return nil
}

// compileRem emits the instructions to perform division on the top
// two values of integer type on the stack and puts the remainder of the result
// onto the stack. For example, stack [..., 8, 3] results in [..., 2] where
// the quotient is discarded. See compileDivForInts for how to acquire quotient, not remainder.
func (c *amd64Compiler) compileRem(o *wazeroir.OperationRem) (err error) {
	switch o.Type {
	case wazeroir.SignedInt32:
		err = c.performDivisionOnInts(true, true)
	case wazeroir.SignedInt64:
		err = c.performDivisionOnInts(false, true)
	case wazeroir.SignedUint32:
		err = c.performDivisionOnInts(true, false)
	case wazeroir.SignedUint64:
		err = c.performDivisionOnInts(false, false)
	}
	if err != nil {
		return err
	}

	// Now we have the remainder of the division result in the DX register,
	// so we record it.
	result := c.locationStack.pushValueOnRegister(x86.REG_DX)
	result.setRegisterType(generalPurposeRegisterTypeInt)

	// Make sure that the AX register (holding quotient of the division) is marked unused.
	c.locationStack.markRegisterUnused(x86.REG_AX)
	return
}

// performDivisionOnInts emits the instructions to do divisions on top two integers on the stack
// via DIV (unsigned div) and IDIV (signed div) instructions.
// See the following explanation of these instructions' semantics from https://www.lri.fr/~filliatr/ens/compil/x86-64.pdf
//
// >> Division requires special arrangements: idiv (signed) and div (unsigned) operate on a 2n-byte dividend and
// >> an n-byte divisor to produce an n-byte quotient and n-byte remainder. The dividend always lives in a fixed pair of
// >> registers (%edx and %eax for the 32-bit case; %rdx and %rax for the 64-bit case); the divisor is specified as the
// >> source operand in the instruction. The quotient goes in %eax (resp. %rax); the remainder in %edx (resp. %rdx). For
// >> signed division, the cltd (resp. ctqo) instruction is used to prepare %edx (resp. %rdx) with the sign extension of
// >> %eax (resp. %rax). For example, if a,b, c are memory locations holding quad words, then we could set c = a/b
// >> using the sequence: movq a(%rip), %rax; ctqo; idivq b(%rip); movq %rax, c(%rip).
//
// tl;dr is that the division result is placed in AX and DX registers after instructions emitted by this function
// where AX holds the quotient while DX the remainder of the division result.
func (c *amd64Compiler) performDivisionOnInts(is32Bit bool, signed bool) error {

	const (
		quotientRegister  = x86.REG_AX
		remainderRegister = x86.REG_DX
	)

	x2 := c.locationStack.pop()
	if x2.onConditionalRegister() {
		if err := c.moveConditionalToFreeGeneralPurposeRegister(x2); err != nil {
			return err
		}
	}
	// If x2 is placed in the AX, we just release it to the memory stack
	// as AX must be set to the x1's value below.
	if x2.register == quotientRegister {
		c.releaseRegisterToStack(x2)
	}

	x1 := c.locationStack.pop()
	// Ensure that x1 is placed on the quotient (AX) register.
	if x1.register != quotientRegister {
		c.onValueReleaseRegisterToStack(quotientRegister)
		if x2.onConditionalRegister() {
			c.moveConditionalToGeneralPurposeRegister(x1, quotientRegister)
		} else if x1.onStack() {
			x1.setRegister(quotientRegister)
			c.moveStackToRegister(x1)
			c.locationStack.markRegisterUsed(quotientRegister)
		} else {
			moveX1ToQuotientRegister := c.newProg()
			if is32Bit {
				moveX1ToQuotientRegister.As = x86.AMOVL
			} else {
				moveX1ToQuotientRegister.As = x86.AMOVQ
			}
			moveX1ToQuotientRegister.To.Reg = quotientRegister
			moveX1ToQuotientRegister.To.Type = obj.TYPE_REG
			moveX1ToQuotientRegister.From.Reg = x1.register
			moveX1ToQuotientRegister.From.Type = obj.TYPE_REG
			c.addInstruction(moveX1ToQuotientRegister)
			// We no longer uses the prev register of x1.
			c.locationStack.releaseRegister(x1)
			x1.setRegister(quotientRegister)
			c.locationStack.markRegisterUsed(quotientRegister)
		}
	}

	// We have to save the existing value on DX as the division instruction
	// place the remainder of the result there.
	c.onValueReleaseRegisterToStack(remainderRegister)

	// Now ready to emit the div instruction.
	div := c.newProg()
	div.To.Type = obj.TYPE_NONE
	// Since the div instructions takes 2n byte dividend placed in DX:AX registers,
	// * signed case - we need to sign-extend the dividend into DX register via CDQ (32 bit) or CQO (64 bit).
	// * unsigned case - we need to zero DX register via "XOR DX DX"
	if is32Bit && signed {
		div.As = x86.AIDIVL // Signed 32-bit
		// Emit sign-extension to have 64 bit dividend over DX and AX registers.
		extIntoDX := c.newProg()
		extIntoDX.As = x86.ACDQ
		c.addInstruction(extIntoDX)
	} else if is32Bit && !signed {
		div.As = x86.ADIVL // Unsigned 32-bit
		// Zeros DX register to have 64 bit dividend over DX and AX registers.
		zerosDX := c.newProg()
		zerosDX.As = x86.AXORQ
		zerosDX.From.Type = obj.TYPE_REG
		zerosDX.From.Reg = x86.REG_DX
		zerosDX.To.Type = obj.TYPE_REG
		zerosDX.To.Reg = x86.REG_DX
		c.addInstruction(zerosDX)
	} else if !is32Bit && signed {
		div.As = x86.AIDIVQ // Signed 64-bit
		// Emits sign-extension to have 128 bit dividend over DX and AX registers.
		extIntoDX := c.newProg()
		extIntoDX.As = x86.ACQO
		c.addInstruction(extIntoDX)
	} else if !is32Bit && !signed {
		div.As = x86.ADIVQ // Unsigned 64-bit
		// Zeros DX register to have 128 bit dividend over DX and AX registers.
		zerosDX := c.newProg()
		zerosDX.As = x86.AXORQ
		zerosDX.From.Type = obj.TYPE_REG
		zerosDX.From.Reg = x86.REG_DX
		zerosDX.To.Type = obj.TYPE_REG
		zerosDX.To.Reg = x86.REG_DX
		c.addInstruction(zerosDX)
	}

	if x2.onRegister() {
		div.From.Reg = x2.register
		div.From.Type = obj.TYPE_REG
		c.locationStack.markRegisterUnused(x2.register)
	} else {
		// On stack case.
		div.From.Type = obj.TYPE_MEM
		div.From.Reg = reservedRegisterForStackBasePointer
		div.From.Offset = int64(x2.stackPointer) * 8
	}
	c.addInstruction(div)
	return nil
}

// compileDivForFloats emits the instructions to perform division
// on the top two values of float type on the stack, placing the result back onto the stack.
// For example, stack [..., 1.0, 4.0] results in [..., 0.25].
func (c *amd64Compiler) compileDivForFloats(is32Bit bool) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	div := c.newProg()
	div.From.Type = obj.TYPE_REG
	div.From.Reg = x2.register
	div.To.Type = obj.TYPE_REG
	div.To.Reg = x1.register
	if is32Bit {
		div.As = x86.ADIVSS
	} else {
		div.As = x86.ADIVSD
	}
	c.addInstruction(div)

	// We consumed x2 register after DIV operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

func (c *amd64Compiler) compileEq(o *wazeroir.OperationEq) error {
	return c.emitEqOrNe(o.Type, true)
}

func (c *amd64Compiler) compileNe(o *wazeroir.OperationNe) error {
	return c.emitEqOrNe(o.Type, false)
}

func (c *amd64Compiler) emitEqOrNe(t wazeroir.UnsignedType, shouldEqual bool) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x1.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x2.register
	switch t {
	case wazeroir.UnsignedTypeI32:
		prog.As = x86.ACMPL
	case wazeroir.UnsignedTypeI64:
		prog.As = x86.ACMPQ
	case wazeroir.UnsignedTypeF32:
		prog.As = x86.ACOMISS
	case wazeroir.UnsignedTypeF64:
		prog.As = x86.ACOMISD
	}
	c.addInstruction(prog)

	// TODO: emit NaN value handings for floats.

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	var condReg conditionalRegisterState
	if shouldEqual {
		condReg = conditionalRegisterStateE
	} else {
		condReg = conditionalRegisterStateNE
	}
	loc := c.locationStack.pushValueOnConditionalRegister(condReg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileEqz(o *wazeroir.OperationEqz) error {
	v := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	// Take the temporary register for holding the zero value.
	zeroRegister, err := c.allocateRegister(v.registerType())
	if err != nil {
		return err
	}

	// First, we have to clear the register so the value becomes zero via XOR on itself.
	xorZero := c.newProg()
	xorZero.As = x86.AXORQ
	xorZero.From.Type = obj.TYPE_REG
	xorZero.From.Reg = zeroRegister
	xorZero.To.Type = obj.TYPE_REG
	xorZero.To.Reg = zeroRegister
	c.addInstruction(xorZero)

	// Emit the compare instruction.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = zeroRegister
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = v.register
	switch o.Type {
	case wazeroir.UnsignedInt32:
		prog.As = x86.ACMPL
	case wazeroir.UnsignedInt64:
		prog.As = x86.ACMPQ
	}
	c.addInstruction(prog)

	// v is consumed by the cmp operation so release it.
	c.locationStack.releaseRegister(v)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueOnConditionalRegister(conditionalRegisterStateE)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileLt(o *wazeroir.OperationLt) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.To.Type = obj.TYPE_REG
	var resultConditionState conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = conditionalRegisterStateL
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint32:
		resultConditionState = conditionalRegisterStateB
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeInt64:
		resultConditionState = conditionalRegisterStateL
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint64:
		resultConditionState = conditionalRegisterStateB
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeFloat32:
		resultConditionState = conditionalRegisterStateB
		prog.As = x86.ACOMISS
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateB
		prog.As = x86.ACOMISD
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	}
	c.addInstruction(prog)

	// TODO: emit NaN value handings for floats.

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileGt(o *wazeroir.OperationGt) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.To.Type = obj.TYPE_REG
	var resultConditionState conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = conditionalRegisterStateG
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint32:
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeInt64:
		resultConditionState = conditionalRegisterStateG
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint64:
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeFloat32:
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.ACOMISS
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.ACOMISD
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	}
	c.addInstruction(prog)

	// TODO: emit NaN value handings for floats.

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileLe(o *wazeroir.OperationLe) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.To.Type = obj.TYPE_REG
	var resultConditionState conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = conditionalRegisterStateLE
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint32:
		resultConditionState = conditionalRegisterStateBE
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeInt64:
		resultConditionState = conditionalRegisterStateLE
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint64:
		resultConditionState = conditionalRegisterStateBE
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeFloat32:
		resultConditionState = conditionalRegisterStateBE
		prog.As = x86.ACOMISS
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateBE
		prog.As = x86.ACOMISD
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	}
	c.addInstruction(prog)

	// TODO: emit NaN value handings for floats.

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileGe(o *wazeroir.OperationGe) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	prog := c.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.To.Type = obj.TYPE_REG
	var resultConditionState conditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = conditionalRegisterStateGE
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint32:
		resultConditionState = conditionalRegisterStateAE
		prog.As = x86.ACMPL
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeInt64:
		resultConditionState = conditionalRegisterStateGE
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeUint64:
		resultConditionState = conditionalRegisterStateAE
		prog.As = x86.ACMPQ
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeFloat32:
		resultConditionState = conditionalRegisterStateAE
		prog.As = x86.ACOMISS
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateAE
		prog.As = x86.ACOMISD
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	}
	c.addInstruction(prog)

	// TODO: emit NaN value handings for floats.

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileLoad(o *wazeroir.OperationLoad) error {
	base := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(base); err != nil {
		return err
	}

	// At this point, base's value is on the integer general purpose reg.
	// We reuse the register below, so we alias it here for readability.
	reg := base.register

	// Then we have to calculate the offset on the memory region.
	addOffsetToBase := c.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = reg
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(o.Arg.Offest)
	c.addInstruction(addOffsetToBase)

	// TODO: Emit instructions here to check memory out of bounds as
	// potentially it would be an security risk.

	var (
		isIntType bool
		movInst   obj.As
	)
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		isIntType = true
		movInst = x86.AMOVL
	case wazeroir.UnsignedTypeI64:
		isIntType = true
		movInst = x86.AMOVQ
	case wazeroir.UnsignedTypeF32:
		isIntType = false
		movInst = x86.AMOVL
	case wazeroir.UnsignedTypeF64:
		isIntType = false
		movInst = x86.AMOVQ
	}

	if isIntType {
		// For integer types, read the corresponding bytes from the offset to the memory
		// and store the value to the int register.
		moveFromMemory := c.newProg()
		moveFromMemory.As = movInst
		moveFromMemory.To.Type = obj.TYPE_REG
		moveFromMemory.To.Reg = reg
		moveFromMemory.From.Type = obj.TYPE_MEM
		moveFromMemory.From.Reg = reservedRegisterForMemory
		moveFromMemory.From.Index = reg
		moveFromMemory.From.Scale = 1
		c.addInstruction(moveFromMemory)
		top := c.locationStack.pushValueOnRegister(reg)
		top.setRegisterType(generalPurposeRegisterTypeInt)
	} else {
		// For float types, we read the value to the float register.
		floatReg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
		moveFromMemory := c.newProg()
		moveFromMemory.As = movInst
		moveFromMemory.To.Type = obj.TYPE_REG
		moveFromMemory.To.Reg = floatReg
		moveFromMemory.From.Type = obj.TYPE_MEM
		moveFromMemory.From.Reg = reservedRegisterForMemory
		moveFromMemory.From.Index = reg
		moveFromMemory.From.Scale = 1
		c.addInstruction(moveFromMemory)
		top := c.locationStack.pushValueOnRegister(floatReg)
		top.setRegisterType(generalPurposeRegisterTypeFloat)
		// We no longer need the int register so mark it unused.
		c.locationStack.markRegisterUnused(reg)
	}
	return nil
}

func (c *amd64Compiler) compileLoad8(o *wazeroir.OperationLoad8) error {
	base := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(base); err != nil {
		return err
	}

	// At this point, base's value is on the integer general purpose reg.
	// We reuse the register below, so we alias it here for readability.
	reg := base.register

	// We have to calculate the offset on the memory region.
	addOffsetToBase := c.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = reg
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(o.Arg.Offest)
	c.addInstruction(addOffsetToBase)

	// Then move a byte at the offset to the register.
	// Note that Load8 is only for integer types.
	moveFromMemory := c.newProg()
	moveFromMemory.As = x86.AMOVB
	moveFromMemory.To.Type = obj.TYPE_REG
	moveFromMemory.To.Reg = reg
	moveFromMemory.From.Type = obj.TYPE_MEM
	moveFromMemory.From.Reg = reservedRegisterForMemory
	moveFromMemory.From.Index = reg
	moveFromMemory.From.Scale = 1
	c.addInstruction(moveFromMemory)
	top := c.locationStack.pushValueOnRegister(reg)

	// The result of load8 is always int type.
	top.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileLoad16(o *wazeroir.OperationLoad16) error {
	base := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(base); err != nil {
		return err
	}

	// At this point, base's value is on the integer general purpose reg.
	// We reuse the register below, so we alias it here for readability.
	reg := base.register

	// We have to calculate the offset on the memory region.
	addOffsetToBase := c.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = reg
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(o.Arg.Offest)
	c.addInstruction(addOffsetToBase)

	// Then move 2 bytes at the offset to the register.
	// Note that Load16 is only for integer types.
	moveFromMemory := c.newProg()
	moveFromMemory.As = x86.AMOVW
	moveFromMemory.To.Type = obj.TYPE_REG
	moveFromMemory.To.Reg = reg
	moveFromMemory.From.Type = obj.TYPE_MEM
	moveFromMemory.From.Reg = reservedRegisterForMemory
	moveFromMemory.From.Index = reg
	moveFromMemory.From.Scale = 1
	c.addInstruction(moveFromMemory)
	top := c.locationStack.pushValueOnRegister(reg)

	// The result of load16 is always int type.
	top.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileLoad32(o *wazeroir.OperationLoad32) error {
	base := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(base); err != nil {
		return err
	}

	// At this point, base's value is on the integer general purpose reg.
	// We reuse the register below, so we alias it here for readability.
	reg := base.register

	// We have to calculate the offset on the memory region.
	addOffsetToBase := c.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = reg
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(o.Arg.Offest)
	c.addInstruction(addOffsetToBase)

	// Then move 4 bytes at the offset to the register.
	moveFromMemory := c.newProg()
	moveFromMemory.As = x86.AMOVL
	moveFromMemory.To.Type = obj.TYPE_REG
	moveFromMemory.To.Reg = reg
	moveFromMemory.From.Type = obj.TYPE_MEM
	moveFromMemory.From.Reg = reservedRegisterForMemory
	moveFromMemory.From.Index = reg
	moveFromMemory.From.Scale = 1
	c.addInstruction(moveFromMemory)
	top := c.locationStack.pushValueOnRegister(reg)

	// The result of load32 is always int type.
	top.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileStore(o *wazeroir.OperationStore) error {
	var movInst obj.As
	switch o.Type {
	case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
		movInst = x86.AMOVL
	case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
		movInst = x86.AMOVQ
	}
	return c.moveToMemory(o.Arg.Offest, movInst)
}

func (c *amd64Compiler) compileStore8(o *wazeroir.OperationStore8) error {
	return c.moveToMemory(o.Arg.Offest, x86.AMOVB)
}

func (c *amd64Compiler) compileStore16(o *wazeroir.OperationStore16) error {
	return c.moveToMemory(o.Arg.Offest, x86.AMOVW)
}

func (c *amd64Compiler) compileStore32(o *wazeroir.OperationStore32) error {
	return c.moveToMemory(o.Arg.Offest, x86.AMOVL)
}

func (c *amd64Compiler) moveToMemory(offsetConst uint32, moveInstruction obj.As) error {
	val := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	base := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(base); err != nil {
		return err
	}

	// Then we have to calculate the offset on the memory region.
	addOffsetToBase := c.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = base.register
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(offsetConst)
	c.addInstruction(addOffsetToBase)

	// TODO: Emit instructions here to check memory out of bounds as
	// potentially it would be an security risk.

	moveToMemory := c.newProg()
	moveToMemory.As = moveInstruction
	moveToMemory.From.Type = obj.TYPE_REG
	moveToMemory.From.Reg = val.register
	moveToMemory.To.Type = obj.TYPE_MEM
	moveToMemory.To.Reg = reservedRegisterForMemory
	moveToMemory.To.Index = base.register
	moveToMemory.To.Scale = 1
	c.addInstruction(moveToMemory)

	// We no longer need both the value and base registers.
	c.locationStack.releaseRegister(val)
	c.locationStack.releaseRegister(base)
	return nil
}

func (c *amd64Compiler) compileMemoryGrow() {
	c.callBuiltinFunctionFromConstIndex(builtinFunctionIndexMemoryGrow)
}

func (c *amd64Compiler) compileMemorySize() {
	c.callBuiltinFunctionFromConstIndex(builtinFunctionIndexMemorySize)
	loc := c.locationStack.pushValueOnStack() // The size is pushed on the top.
	loc.setRegisterType(generalPurposeRegisterTypeInt)
}

func (c *amd64Compiler) callBuiltinFunctionFromConstIndex(index int64) {
	c.setJITStatus(jitCallStatusCodeCallBuiltInFunction)
	c.setFunctionCallIndexFromConst(index)
	// Release all the registers as our calling convention requires the callee-save.
	c.releaseAllRegistersToStack()
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once we return from the function call,
	// we must setup the reserved registers again.
	c.initializeReservedRegisters()
}

func (c *amd64Compiler) compileConstI32(o *wazeroir.OperationConstI32) error {
	reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := c.locationStack.pushValueOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)

	c.emitConstI32(o.Value, reg)
	return nil
}

func (c *amd64Compiler) emitConstI32(val uint32, register int16) {
	prog := c.newProg()
	prog.As = x86.AMOVL // Note 32-bit move!
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(val)
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = register
	c.addInstruction(prog)
}

func (c *amd64Compiler) compileConstI64(o *wazeroir.OperationConstI64) error {
	reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := c.locationStack.pushValueOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)

	c.emitConstI64(o.Value, reg)
	return nil
}

func (c *amd64Compiler) emitConstI64(val uint64, register int16) {
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(val)
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = register
	c.addInstruction(prog)
}

func (c *amd64Compiler) compileConstF32(o *wazeroir.OperationConstF32) error {
	reg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}
	loc := c.locationStack.pushValueOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	moveToTmpReg := c.newProg()
	moveToTmpReg.As = x86.AMOVL // Note 32-bit mov!
	moveToTmpReg.From.Type = obj.TYPE_CONST
	moveToTmpReg.From.Offset = int64(uint64(math.Float32bits(o.Value)))
	moveToTmpReg.To.Type = obj.TYPE_REG
	moveToTmpReg.To.Reg = tmpReg
	c.addInstruction(moveToTmpReg)

	prog := c.newProg()
	prog.As = x86.AMOVL // Note 32-bit mov!
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	c.addInstruction(prog)
	// We don't need to explicitly release tmpReg here
	// as allocateRegister doesn't mark it used.
	return nil
}

func (c *amd64Compiler) compileConstF64(o *wazeroir.OperationConstF64) error {
	reg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}
	loc := c.locationStack.pushValueOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	moveToTmpReg := c.newProg()
	moveToTmpReg.As = x86.AMOVQ
	moveToTmpReg.From.Type = obj.TYPE_CONST
	moveToTmpReg.From.Offset = int64(math.Float64bits(o.Value))
	moveToTmpReg.To.Type = obj.TYPE_REG
	moveToTmpReg.To.Reg = tmpReg
	c.addInstruction(moveToTmpReg)

	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	c.addInstruction(prog)
	// We don't need to explicitly release tmpReg here
	// as allocateRegister doesn't mark it used.
	return nil
}

// TODO: maybe split this function as this is doing too much at once to say at once.
func (c *amd64Compiler) moveStackToRegisterWithAllocation(tp generalPurposeRegisterType, loc *valueLocation) error {
	// Allocate the register.
	reg, err := c.allocateRegister(tp)
	if err != nil {
		return err
	}

	// Mark it uses the register.
	loc.setRegister(reg)
	c.locationStack.markRegisterUsed(reg)

	// Now ready to move value.
	c.moveStackToRegister(loc)
	return nil
}

func (c *amd64Compiler) moveStackToRegister(loc *valueLocation) {
	// Copy the value from the stack.
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForStackBasePointer
	prog.From.Offset = int64(loc.stackPointer) * 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = loc.register
	c.addInstruction(prog)
}

func (c *amd64Compiler) moveConditionalToFreeGeneralPurposeRegister(loc *valueLocation) error {
	// Get the free register.
	reg, ok := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	if !ok {
		// This in theory should never be reached as moveConditionalToGeneralPurposeRegister
		// is called right after comparison operations, meaning that
		// at least two registers are free at the moment.
		return fmt.Errorf("conditional register mov requires a free register")
	}

	c.moveConditionalToGeneralPurposeRegister(loc, reg)
	return nil
}

func (c *amd64Compiler) moveConditionalToGeneralPurposeRegister(loc *valueLocation, reg int16) {
	// Set the flag bit to the destination.
	prog := c.newProg()
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg

	// See
	// - https://c9x.me/x86/html/file_module_x86_id_288.html
	// - https://github.com/golang/go/blob/master/src/cmd/internal/obj/x86/asm6.go#L1453-L1468
	// to translate conditionalRegisterState* to x86.ASET*
	switch loc.conditionalRegister {
	case conditionalRegisterStateE:
		prog.As = x86.ASETEQ
	case conditionalRegisterStateNE:
		prog.As = x86.ASETNE
	case conditionalRegisterStateS:
		prog.As = x86.ASETMI
	case conditionalRegisterStateNS:
		prog.As = x86.ASETPL
	case conditionalRegisterStateG:
		prog.As = x86.ASETGT
	case conditionalRegisterStateGE:
		prog.As = x86.ASETGE
	case conditionalRegisterStateL:
		prog.As = x86.ASETLT
	case conditionalRegisterStateLE:
		prog.As = x86.ASETLE
	case conditionalRegisterStateA:
		prog.As = x86.ASETHI
	case conditionalRegisterStateAE:
		prog.As = x86.ASETCC
	case conditionalRegisterStateB:
		prog.As = x86.ASETCS
	case conditionalRegisterStateBE:
		prog.As = x86.ASETLS
	}
	c.addInstruction(prog)

	// Then we reset the unnecessary bit.
	prog = c.newProg()
	prog.As = x86.AANDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = 0x1
	c.addInstruction(prog)

	// Mark it uses the register.
	loc.setRegister(reg)
	c.locationStack.markRegisterUsed(reg)
}

// allocateRegister returns an unused register of the given type. The register will be taken
// either from the free register pool or by stealing an used register.
// Note that resulting registers are NOT marked as used so the call site should
// mark it used if necessary.
func (c *amd64Compiler) allocateRegister(t generalPurposeRegisterType) (reg int16, err error) {
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

func (c *amd64Compiler) setJITStatus(status jitCallStatusCode) {
	prog := c.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(status)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineJITCallStatusCodeOffset
	c.addInstruction(prog)
}

func (c *amd64Compiler) callHostFunctionFromConstIndex(index int64) {
	// Set the jit status as jitCallStatusCodeCallHostFunction
	c.setJITStatus(jitCallStatusCodeCallHostFunction)
	// Set the function index.
	c.setFunctionCallIndexFromConst(index)
	// Release all the registers as our calling convention requires the callee-save.
	c.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the function call returns, we must re-initialize the reserved registers.
	c.initializeReservedRegisters()
}

func (c *amd64Compiler) callHostFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitCallStatusCodeCallHostFunction
	c.setJITStatus(jitCallStatusCodeCallHostFunction)
	// Set the function index.
	c.setFunctionCallIndexFromRegister(reg)
	// Release all the registers as our calling convention requires the callee-save.
	c.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the function call returns, we must re-initialize the reserved registers..
	c.initializeReservedRegisters()
}

func (c *amd64Compiler) callFunctionFromConstIndex(index int64) {
	// Set the jit status as jitCallStatusCodeCallWasmFunction
	c.setJITStatus(jitCallStatusCodeCallWasmFunction)
	// Set the function index.
	c.setFunctionCallIndexFromConst(index)
	// Release all the registers as our calling convention requires the callee-save.
	c.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the function call returns, we must re-initialize the reserved registers.
	c.initializeReservedRegisters()
}

func (c *amd64Compiler) callFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitCallStatusCodeCallWasmFunction
	c.setJITStatus(jitCallStatusCodeCallWasmFunction)
	// Set the function index.
	c.setFunctionCallIndexFromRegister(reg)
	// Release all the registers as our calling convention requires the callee-save.
	c.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the function call returns, we must re-initialize the reserved registers.
	c.initializeReservedRegisters()
}

func (c *amd64Compiler) releaseAllRegistersToStack() {
	used := len(c.locationStack.usedRegisters)
	for i := uint64(0); i < c.locationStack.sp && used > 0; i++ {
		if loc := c.locationStack.stack[i]; loc.onRegister() {
			c.releaseRegisterToStack(loc)
			used--
		}
	}
}

// TODO: If this function call is the tail call,
// we don't need to return back to this function.
// Maybe better have another status for that case
// so that we don't call back again to this function
// and instead just release the call frame.
func (c *amd64Compiler) setContinuationOffsetAtNextInstructionAndReturn() {
	// setContinuationOffsetAtNextInstructionAndReturn is called after releasing
	// all the registers, so at this point we always have free registers.
	tmpReg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	// Create the instruction for setting offset.
	// We use tmp register to store the const, not directly movq to memory
	// as it is not valid to move 64-bit const to memory directly.
	// TODO: is it really illegal, though?
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = tmpReg
	// We calculate the return address offset later, as at this point of compilation
	// we don't yet know addresses of instructions.
	// We intentionally use 1 << 33 to let the assembler to emit the instructions for
	// 64-bit mov, instead of 32-bit mov.
	prog.From.Offset = int64(1 << 33)
	// Append this instruction so we can later resolve the actual offset of the next instruction after return below.
	c.requireFunctionCallReturnAddressOffsetResolution = append(c.requireFunctionCallReturnAddressOffsetResolution, prog)
	c.addInstruction(prog)

	prog = c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineContinuationAddressOffset
	c.addInstruction(prog)
	// Then return temporarily -- giving control to normal Go code.
	c.returnFunction()
}

func (c *amd64Compiler) setFunctionCallIndexFromRegister(reg int16) {
	prog := c.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineFunctionCallIndexOffset
	c.addInstruction(prog)
}

func (c *amd64Compiler) setFunctionCallIndexFromConst(index int64) {
	prog := c.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = index
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineFunctionCallIndexOffset
	c.addInstruction(prog)
}

func (c *amd64Compiler) onValueReleaseRegisterToStack(reg int16) {
	prevValue := c.locationStack.findValueForRegister(reg)
	if prevValue == nil {
		// This case the target register is not used by any value.
		return
	}
	c.releaseRegisterToStack(prevValue)
}

func (c *amd64Compiler) releaseRegisterToStack(loc *valueLocation) {
	// Push value.
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForStackBasePointer
	prog.To.Offset = int64(loc.stackPointer) * 8
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = loc.register
	c.addInstruction(prog)

	// Mark the register is free.
	c.locationStack.releaseRegister(loc)
}

func (c *amd64Compiler) assignRegisterToValue(loc *valueLocation, reg int16) {
	// Pop value to the resgister.
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForStackBasePointer
	prog.From.Offset = int64(loc.stackPointer) * 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	c.addInstruction(prog)

	// Now the value is on register, so mark as such.
	loc.setRegister(reg)
	c.locationStack.markRegisterUsed(reg)
}

func (c *amd64Compiler) returnFunction() {
	// Write back the cached SP to the actual eng.stackPointer.
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(c.locationStack.sp)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = enginestackPointerOffset
	c.addInstruction(prog)

	// Return.
	ret := c.newProg()
	ret.As = obj.ARET
	c.addInstruction(ret)
}

// initializeReservedRegisters must be called at the very beginning and all the
// after-call continuations of JITed functions.
// This caches the actual stack base pointer (engine.stackBasePointer*8+[engine.engineStackSliceOffset])
// to cachedStackBasePointerReg
func (c *amd64Compiler) initializeReservedRegisters() {
	// At first, make cachedStackBasePointerReg point to the beginning of the slice backing array.
	// movq [engineInstanceReg+engineStackSliceOffset] cachedStackBasePointerReg
	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForEngine
	prog.From.Offset = engineStackSliceOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reservedRegisterForStackBasePointer
	c.addInstruction(prog)

	// Since initializeReservedRegisters is called at the beginning of function
	// calls (or right after they return), we have free registers at this point.
	reg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)

	// Next we move the base pointer (engine.stackBasePointer) to
	// a temporary register.
	// movq [engineInstanceReg+engineCurrentstackBasePointerOffset] reg
	prog = c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForEngine
	prog.From.Offset = enginestackBasePointerOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	c.addInstruction(prog)

	// Multiply reg with 8 via shift left with 3.
	// shlq $3 reg
	prog = c.newProg()
	prog.As = x86.ASHLQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = 3
	c.addInstruction(prog)

	// Finally we add the reg to cachedStackBasePointerReg.
	// addq [reg] cachedStackBasePointerReg
	prog = c.newProg()
	prog.As = x86.AADDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reservedRegisterForStackBasePointer
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	c.addInstruction(prog)
}

// ensureOnGeneralPurposeRegister ensures that the given value is located on a
// general purpose register of an appropriate type.
func (c *amd64Compiler) ensureOnGeneralPurposeRegister(loc *valueLocation) error {
	if loc.onStack() {
		if err := c.moveStackToRegisterWithAllocation(loc.registerType(), loc); err != nil {
			return err
		}
	} else if loc.onConditionalRegister() {
		if err := c.moveConditionalToFreeGeneralPurposeRegister(loc); err != nil {
			return err
		}
	}
	return nil
}
