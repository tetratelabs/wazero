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
	"unsafe"

	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

var (
	zero64Bit                                     uint64 = 0
	zero64BitAddress                              uintptr
	float32SignBitMask                            uint32 = 1 << 31
	float32RestBitMask                            uint32 = ^float32SignBitMask
	float32SignBitMaskAddress                     uintptr
	float32RestBitMaskAddress                     uintptr
	float64SignBitMask                            uint64 = 1 << 63
	float64RestBitMask                            uint64 = ^float64SignBitMask
	float64SignBitMaskAddress                     uintptr
	float64RestBitMaskAddress                     uintptr
	float32ForMinimumSigned32bitInteger           float32 = math.Float32frombits(0xCF00_0000)
	float32ForMinimumSigned32bitIntegerAdddress   uintptr
	float64ForMinimumSigned32bitInteger           float64 = math.Float64frombits(0xC1E0_0000_0020_0000)
	float64ForMinimumSigned32bitIntegerAdddress   uintptr
	float32ForMinimumSigned64bitInteger           float32 = math.Float32frombits(0xDF00_0000)
	float32ForMinimumSigned64bitIntegerAdddress   uintptr
	float64ForMinimumSigned64bitInteger           float64 = math.Float64frombits(0xC3E0_0000_0000_0000)
	float64ForMinimumSigned64bitIntegerAdddress   uintptr
	float32ForMaximumSigned32bitIntPlusOne        float32 = math.Float32frombits(0x4F00_0000)
	float32ForMaximumSigned32bitIntPlusOneAddress uintptr
	float64ForMaximumSigned32bitIntPlusOne        float64 = math.Float64frombits(0x41E0_0000_0000_0000)
	float64ForMaximumSigned32bitIntPlusOneAddress uintptr
	float32ForMaximumSigned64bitIntPlusOne        float32 = math.Float32frombits(0x5F00_0000)
	float32ForMaximumSigned64bitIntPlusOneAddress uintptr
	float64ForMaximumSigned64bitIntPlusOne        float64 = math.Float64frombits(0x43E0_0000_0000_0000)
	float64ForMaximumSigned64bitIntPlusOneAddress uintptr
)

func init() {
	zero64BitAddress = uintptr(unsafe.Pointer(&zero64Bit))
	float32SignBitMaskAddress = uintptr(unsafe.Pointer(&float32SignBitMask))
	float32RestBitMaskAddress = uintptr(unsafe.Pointer(&float32RestBitMask))
	float64SignBitMaskAddress = uintptr(unsafe.Pointer(&float64SignBitMask))
	float64RestBitMaskAddress = uintptr(unsafe.Pointer(&float64RestBitMask))
	float32ForMinimumSigned32bitIntegerAdddress = uintptr(unsafe.Pointer(&float32ForMinimumSigned32bitInteger))
	float64ForMinimumSigned32bitIntegerAdddress = uintptr(unsafe.Pointer(&float64ForMinimumSigned32bitInteger))
	float32ForMinimumSigned64bitIntegerAdddress = uintptr(unsafe.Pointer(&float32ForMinimumSigned64bitInteger))
	float64ForMinimumSigned64bitIntegerAdddress = uintptr(unsafe.Pointer(&float64ForMinimumSigned64bitInteger))
	float32ForMaximumSigned32bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float32ForMaximumSigned32bitIntPlusOne))
	float64ForMaximumSigned32bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float64ForMaximumSigned32bitIntPlusOne))
	float32ForMaximumSigned64bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float32ForMaximumSigned64bitIntPlusOne))
	float64ForMaximumSigned64bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float64ForMaximumSigned64bitIntPlusOne))
}

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

	labels := make(map[string]*labelInfo, len(ir.LabelCallers))
	for key, callers := range ir.LabelCallers {
		labels[key] = &labelInfo{callers: callers}
	}
	return &amd64Compiler{
		eng: eng, f: f, builder: b, locationStack: newValueLocationStack(),
		labels: labels,
	}, nil
}

func (c *amd64Compiler) String() string {
	return c.locationStack.String()
}

type amd64Compiler struct {
	builder *asm.Builder
	eng     *engine
	f       *wasm.FunctionInstance
	// setJmpOrigins sets jmp kind instructions where you want to set the next coming
	// instruction as the destination of the jmp instruction.
	setJmpOrigins []*obj.Prog
	// locationStack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
	// labels holds per wazeroir label specific informationsin this function.
	labels                                           map[string]*labelInfo
	requireFunctionCallReturnAddressOffsetResolution []*obj.Prog
	// maxStackPointer tracks the maximum value of stack pointer (from valueLocationStack).
	maxStackPointer uint64
	// currentLabel holds a currently compiled wazeroir label key. For debugging only.
	currentLabel string
}

// replaceLocationStack sets the given valueLocationStack to .locationStack field,
// while allowing us to track valueLocationStack.maxStackPointer across multiple stacks.
// This is called when we branch into different block.
func (c *amd64Compiler) replaceLocationStack(newStack *valueLocationStack) {
	if c.maxStackPointer < c.locationStack.maxStackPointer {
		c.maxStackPointer = c.locationStack.maxStackPointer
	}
	c.locationStack = newStack
}

type labelInfo struct {
	// callers is the number of call sites which may jump into this label.
	callers int
	// initialInstruction is the initial instruction for this label so other block can jump into it.
	initialInstruction *obj.Prog
	// initialStack is the initial value location stack from which we start compiling this label.
	initialStack *valueLocationStack
	// labelBeginningCallbacks holds callbacks should to be called with initialInstruction
	labelBeginningCallbacks []func(*obj.Prog)
}

func (c *amd64Compiler) label(labelKey string) *labelInfo {
	ret, ok := c.labels[labelKey]
	if ok {
		return ret
	}
	c.labels[labelKey] = &labelInfo{}
	return c.labels[labelKey]
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
	// As we cannot read RIP register directly, we calculate now the offset to the next
	// instruction after return instruction for function call.
	const operandSizeBytes = 8
	for _, inst := range c.requireFunctionCallReturnAddressOffsetResolution {
		afterReturnInst := inst
		// Iterate the linked list of instructions until we find the return instruction.
		for ; afterReturnInst != nil; afterReturnInst = afterReturnInst.Link {
			if afterReturnInst.As == obj.ARET {
				// Now we found the return instruction, move forward once again.
				afterReturnInst = afterReturnInst.Link
				break
			}
		}
		if afterReturnInst == nil {
			return nil, 0, fmt.Errorf("invalid function call at %v", inst)
		}
		// Skip MOV, and the register(rax): "0x49, 0xbd"
		start := inst.Pc + 2
		// Set the move target to the program counter (offset to the beginning to this binary).
		binary.LittleEndian.PutUint64(code[start:start+operandSizeBytes], uint64(afterReturnInst.Pc))
	}

	// c.maxStackPointer tracks the maximum stack pointer across all valueLocationStack
	// used for all labels (via replaceLocationStack), excluding the current one.
	// Hense, we check here if the final block's max one exceeds the current c.maxStackPointer.
	maxStackPointer := c.maxStackPointer
	if maxStackPointer < c.locationStack.maxStackPointer {
		maxStackPointer = c.locationStack.maxStackPointer
	}
	return code, maxStackPointer, nil
}

func (c *amd64Compiler) pushFunctionParams() {
	for _, t := range c.f.FunctionType.Type.Params {
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
	for _, origin := range c.setJmpOrigins {
		origin.To.SetTarget(prog)
	}
	c.setJmpOrigins = nil
}

func (c *amd64Compiler) addSetJmpOrigins(progs ...*obj.Prog) {
	c.setJmpOrigins = append(c.setJmpOrigins, progs...)
}

func (c *amd64Compiler) newProg() (prog *obj.Prog) {
	prog = c.builder.NewProg()
	return
}

func (c *amd64Compiler) compileUnreachable() error {
	if err := c.releaseAllRegistersToStack(); err != nil {
		return err
	}
	c.setJITStatus(jitCallStatusCodeUnreachable)
	c.returnFunction()
	return nil
}

func (c *amd64Compiler) compileSwap(o *wazeroir.OperationSwap) error {
	index := int(c.locationStack.sp) - 1 - o.Depth
	// Note that, in theory, the register types and value types
	// are the same between these swap targets as swap operations
	// are generated from local.set,tee instructions in Wasm.
	x1 := c.locationStack.peek()
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
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

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
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

	if o.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		if err := c.releaseAllRegistersToStack(); err != nil {
			return err
		}
		c.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		c.returnFunction()
	} else {
		labelKey := o.Target.String()
		targetLabel := c.label(labelKey)
		if targetLabel.callers > 1 {
			// If the number of callers to the target label is larget than one,
			// we have multiple origins to the target branch. In that case,
			// we must have unique register state.
			if err := c.preJumpRegisterAdjustment(); err != nil {
				return err
			}
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		if targetLabel.initialStack == nil {
			targetLabel.initialStack = c.locationStack.clone()
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
		c.locationStack.markRegisterUnused(cond.register)
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
	c.replaceLocationStack(saved.clone())
	if elseTarget.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		if err := c.releaseAllRegistersToStack(); err != nil {
			return err
		}
		c.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		c.returnFunction()
	} else {
		elseLabelKey := elseTarget.Target.Label.String()
		labelInfo := c.label(elseLabelKey)
		if labelInfo.callers > 1 {
			if err := c.preJumpRegisterAdjustment(); err != nil {
				return err
			}
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		if labelInfo.initialStack == nil {
			labelInfo.initialStack = c.locationStack.clone()
		}
		elseJmp := c.newProg()
		elseJmp.As = obj.AJMP
		elseJmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(elseJmp)
		c.assignJumpTarget(elseLabelKey, elseJmp)
	}

	// Handle then branch.
	c.addSetJmpOrigins(jmpWithCond)
	c.replaceLocationStack(saved)
	if err := c.emitDropRange(thenTarget.ToDrop); err != nil {
		return err
	}
	if thenTarget.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		if err := c.releaseAllRegistersToStack(); err != nil {
			return err
		}
		c.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		c.returnFunction()
	} else {
		thenLabelKey := thenTarget.Target.Label.String()
		labelInfo := c.label(thenLabelKey)
		if c.label(thenLabelKey).callers > 1 {
			if err := c.preJumpRegisterAdjustment(); err != nil {
				return err
			}
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		if labelInfo.initialStack == nil {
			labelInfo.initialStack = c.locationStack.clone()
		}
		thenJmp := c.newProg()
		thenJmp.As = obj.AJMP
		thenJmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(thenJmp)
		c.assignJumpTarget(thenLabelKey, thenJmp)
	}
	return nil
}

func (c *amd64Compiler) compileBrTable(o *wazeroir.OperationBrTable) error {
	panic("TODO implement!")
}

// If a jump target has multiple callesr (origins),
// we must have unique register states, so this function
// must be called before such jump instruction.
func (c *amd64Compiler) preJumpRegisterAdjustment() error {
	// For now, we just release all registers to memory.
	// But this is obviously inefficient, so we come back here
	// later once we finish the baseline implementation.
	if err := c.releaseAllRegistersToStack(); err != nil {
		return err
	}
	return nil
}

func (c *amd64Compiler) assignJumpTarget(labelKey string, jmpInstruction *obj.Prog) {
	jmpTargetLabel := c.label(labelKey)
	if jmpTargetLabel.initialInstruction != nil {
		jmpInstruction.To.SetTarget(jmpTargetLabel.initialInstruction)
	} else {
		jmpTargetLabel.labelBeginningCallbacks = append(jmpTargetLabel.labelBeginningCallbacks, func(labelInitialInstruction *obj.Prog) {
			jmpInstruction.To.SetTarget(labelInitialInstruction)
		})
	}
}

func (c *amd64Compiler) compileLabel(o *wazeroir.OperationLabel) error {
	if buildoptions.IsDebugMode {
		fmt.Printf("[label %s ends]\n%s\n", c.currentLabel, c.locationStack)
	}

	labelKey := o.Label.String()
	// We use NOP as a beginning of instructions in a label.
	// This should be eventually optimized out by assembler.
	labelBegin := c.newProg()
	labelBegin.As = obj.ANOP
	c.addInstruction(labelBegin)

	labelInfo := c.label(labelKey)

	// Save the instructions so that backward branching
	// instructions can jump to this label.
	labelInfo.initialInstruction = labelBegin

	// Set the initial stack.
	if labelInfo.initialStack != nil {
		c.replaceLocationStack(labelInfo.initialStack)
	}

	// Invoke callbacks to notify the forward branching
	// instructions can properly jump to this label.
	for _, cb := range labelInfo.labelBeginningCallbacks {
		cb(labelBegin)
	}

	if buildoptions.IsDebugMode {
		fmt.Printf("[label %s]\n%s\n", labelKey, c.locationStack)
	}
	c.currentLabel = labelKey
	return nil
}

func (c *amd64Compiler) compileCall(o *wazeroir.OperationCall) error {
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

	target := c.f.ModuleInstance.Functions[o.FunctionIndex]
	if err := c.compileFunctionCallFromAddress(jitCallStatusCodeCallFunction, target.Address); err != nil {
		return err
	}

	// We consumed the function parameters from the stack after call.
	for i := 0; i < len(target.FunctionType.Type.Params); i++ {
		c.locationStack.pop()
	}

	// Also, the function results were pushed by the call.
	for _, t := range target.FunctionType.Type.Results {
		loc := c.locationStack.pushValueOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
	}
	return nil
}

const (
	tableElementFunctionAddressOffest = 0
	tableElementTypeIDOffest          = 8
)

// compileCallIndirect adds instructions to perform call_indirect operation.
// This consumes the one value from the top of stack (called "offset"),
// and make a function call against the function whose function address equals "table[offset]".
//
// Note: This is called indirect function call in the sense that the target function is indirectly
// determined by the current state (top value) of the stack.
// Therefore, two checks are performed at runtime before entering the target function:
// 1) If "offset" exceeds the length of table, "out of bounds table access" states (jitCallStatusCodeTableOutOfBounds) is returned.
// 2) If the type of the function table[offset] doesn't match the specified function type, "type mismatch" status (jitCallStatusCodeTypeMismatchOnIndirectCall) is returned.
// Otherwise, we successfully enter the target function.
//
// Note: WebAssembly 1.0 (MVP) supports at most one table, so this doesn't support multiple tables.
func (c *amd64Compiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	offset := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(offset); err != nil {
		return nil
	}

	// First, we need to check if the offset doesn't exceed the length of table.
	cmpLength := c.newProg()
	cmpLength.As = x86.ACMPQ
	cmpLength.To.Type = obj.TYPE_REG
	cmpLength.To.Reg = offset.register
	cmpLength.From.Type = obj.TYPE_MEM
	cmpLength.From.Reg = reservedRegisterForEngine
	cmpLength.From.Offset = engineTableSliceLenOffset
	c.addInstruction(cmpLength)

	notLengthExceedJump := c.newProg()
	notLengthExceedJump.To.Type = obj.TYPE_BRANCH
	notLengthExceedJump.As = x86.AJCC
	c.addInstruction(notLengthExceedJump)

	// If it exceeds, we return the function with jitCallStatusCodeTableOutOfBounds.
	c.setJITStatus(jitCallStatusCodeTableOutOfBounds)
	c.returnFunction()

	// Next we check if the target's type matches the operation's one.
	// In order to get the type instance's address, we have to multiply the offset
	// by 16 as the offset is the "length" of table in Go's "[]wasm.TableElement",
	// and size of wasm.TableInstance equals 128 bit (64-bit wasm.FunctionAddress and 64-bit wasm.TypeID).
	getTypeInstanceAddress := c.newProg()
	notLengthExceedJump.To.SetTarget(getTypeInstanceAddress)
	getTypeInstanceAddress.As = x86.ASHLQ
	getTypeInstanceAddress.To.Type = obj.TYPE_REG
	getTypeInstanceAddress.To.Reg = offset.register
	getTypeInstanceAddress.From.Type = obj.TYPE_CONST
	getTypeInstanceAddress.From.Offset = 4
	c.addInstruction(getTypeInstanceAddress)

	// Adds the address of wasm.TableInstance[0] stored as engine.tableSliceAddress to the offset.
	movTableSliceAddress := c.newProg()
	movTableSliceAddress.As = x86.AADDQ
	movTableSliceAddress.To.Type = obj.TYPE_REG
	movTableSliceAddress.To.Reg = offset.register
	movTableSliceAddress.From.Type = obj.TYPE_MEM
	movTableSliceAddress.From.Reg = reservedRegisterForEngine
	movTableSliceAddress.From.Offset = engineTableSliceAddressOffset
	c.addInstruction(movTableSliceAddress)

	// At this point offset.register holds the address of wasm.TableElement at wasm.TableInstance[offset]
	// So the target type ID lives at offset+tableElementTypeIDOffest, and we compare it
	// with targetFunctionType.TypeID.
	targetFunctionType := c.f.ModuleInstance.Types[o.TypeIndex]
	cmpTypeID := c.newProg()
	cmpTypeID.As = x86.ACMPQ
	cmpTypeID.From.Type = obj.TYPE_MEM
	cmpTypeID.From.Reg = offset.register
	cmpTypeID.From.Offset = tableElementTypeIDOffest
	cmpTypeID.To.Type = obj.TYPE_CONST
	cmpTypeID.To.Offset = int64(targetFunctionType.TypeID)
	c.addInstruction(cmpTypeID)

	// Jump if the type matches.
	typeMatchJump := c.newProg()
	typeMatchJump.To.Type = obj.TYPE_BRANCH
	typeMatchJump.As = x86.AJEQ
	c.addInstruction(typeMatchJump)

	// Otherwise, we return the function with jitCallStatusCodeTypeMismatchOnIndirectCall.
	c.setJITStatus(jitCallStatusCodeTypeMismatchOnIndirectCall)
	c.returnFunction()

	// Now all checks passeed, so we start making function call.
	readValue := c.newProg()
	typeMatchJump.To.SetTarget(readValue)
	readValue.As = x86.AMOVQ
	readValue.To.Type = obj.TYPE_REG
	readValue.To.Reg = offset.register
	readValue.From.Offset = tableElementFunctionAddressOffest
	readValue.From.Type = obj.TYPE_MEM
	readValue.From.Reg = offset.register
	c.addInstruction(readValue)

	if err := c.compileFunctionCallFromRegister(offset.register); err != nil {
		return nil
	}

	// The offset register should be marked as un-used as we consumed in the function call.
	c.locationStack.markRegisterUnused(offset.register)

	// We consumed the function parameters from the stack after call.
	for i := 0; i < len(targetFunctionType.Type.Params); i++ {
		c.locationStack.pop()
	}

	// Also, the function results were pushed by the call.
	for _, t := range targetFunctionType.Type.Results {
		loc := c.locationStack.pushValueOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
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
		for i := 0; i <= r.End; i++ {
			if loc := c.locationStack.pop(); loc.onRegister() {
				c.locationStack.releaseRegister(loc)
			}
		}
		return nil
	}

	var liveValues []*valueLocation
	for i := 0; i < r.Start; i++ {
		live := c.locationStack.pop()
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
			// Write the value in the old stack location to a register
			if err := c.moveStackToRegisterWithAllocation(live.registerType(), live); err != nil {
				return err
			}
		} else if live.onConditionalRegister() {
			// If the live is conditional, and it's not target of drop,
			// we must assign it to the register before we emit any instructions here.
			if err := c.moveConditionalToFreeGeneralPurposeRegister(live); err != nil {
				return err
			}
		}
		// Modify the location in the stack with new stack pointer.
		c.locationStack.push(live)
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
	c.addSetJmpOrigins(jmpIfNotZero)

	// In any case, we don't need x2 and c anymore!
	c.locationStack.releaseRegister(x2)
	c.locationStack.releaseRegister(cv)
	return nil
}

func (c *amd64Compiler) compilePick(o *wazeroir.OperationPick) error {
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

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
		panic("TODO: compilePick for targets on a conditonal register")
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
		c.addSetJmpOrigins(jmpAtEndOfZero)
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
		c.addSetJmpOrigins(jmpAtEndOfZero)
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
// onto the stack. For example, stack [..., 10, 3] results in [..., 3] where
// the remainder is discarded. See compileRem for how to acquire remainder, not quotient.
func (c *amd64Compiler) compileDivForInts(is32Bit bool, signed bool) error {
	if err := c.performDivisionOnInts(false, is32Bit, signed); err != nil {
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
// onto the stack. For example, stack [..., 10, 3] results in [..., 1] where
// the quotient is discarded. See compileDivForInts for how to acquire quotient, not remainder.
func (c *amd64Compiler) compileRem(o *wazeroir.OperationRem) (err error) {
	switch o.Type {
	case wazeroir.SignedInt32:
		err = c.performDivisionOnInts(true, true, true)
	case wazeroir.SignedInt64:
		err = c.performDivisionOnInts(true, false, true)
	case wazeroir.SignedUint32:
		err = c.performDivisionOnInts(true, true, false)
	case wazeroir.SignedUint64:
		err = c.performDivisionOnInts(true, false, false)
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
func (c *amd64Compiler) performDivisionOnInts(isRem, is32Bit, signed bool) error {
	const (
		quotientRegister  = x86.REG_AX
		remainderRegister = x86.REG_DX
	)

	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	// We have to save the existing value on DX as the division instruction
	// place the remainder of the result there.
	if x2.register != remainderRegister {
		c.onValueReleaseRegisterToStack(remainderRegister)
	}

	isSignedRem := isRem && signed
	var signedRemMinusOneDivisorJmp *obj.Prog
	if isSignedRem {
		// If this is for getting remainder of signed division,
		// we have to treat the special case where the divisor equals -1.
		// For example, if this is 32-bit case, the result of (-2^31) / -1 equals (quotient=2^31, remainder=0)
		// where quotient doesn't fit in the 32-bit range whose maximum is 2^31-1.
		// x86 in this case cause floating point exception, but according to the Wasm spec
		// if the divisor equals -1, the result must be zero (not undefined!) as opposed to be "undefined"
		// for divisions on (-2^31) / -1 where we do not need to emit the special branches.
		// For detail, please refer to https://stackoverflow.com/questions/56303282/why-idiv-with-1-causes-floating-point-exception

		// First we compare the division with -1.
		cmpWithMinusOne := c.newProg()
		if is32Bit {
			cmpWithMinusOne.As = x86.ACMPL
		} else {
			cmpWithMinusOne.As = x86.ACMPQ
		}
		cmpWithMinusOne.From.Type = obj.TYPE_REG
		cmpWithMinusOne.From.Reg = x2.register
		cmpWithMinusOne.To.Type = obj.TYPE_CONST
		cmpWithMinusOne.To.Offset = -1
		c.addInstruction(cmpWithMinusOne)

		// If it doesn't equal minus one, we jump to the normal case.
		okJmp := c.newProg()
		okJmp.As = x86.AJNE
		okJmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(okJmp)

		// Otherwise, we store zero into the remainder result register (DX).
		storeZeroToDX := c.newProg()
		if is32Bit {
			storeZeroToDX.As = x86.AXORL
		} else {
			storeZeroToDX.As = x86.AXORQ
		}
		storeZeroToDX.To.Type = obj.TYPE_REG
		storeZeroToDX.To.Reg = remainderRegister
		storeZeroToDX.From.Type = obj.TYPE_REG
		storeZeroToDX.From.Reg = remainderRegister
		c.addInstruction(storeZeroToDX)

		// Emit the exit jump instruction for the divisor -1 case so
		// we skips the normal case.
		signedRemMinusOneDivisorJmp = c.newProg()
		signedRemMinusOneDivisorJmp.As = obj.AJMP
		signedRemMinusOneDivisorJmp.To.Type = obj.TYPE_BRANCH
		c.addInstruction(signedRemMinusOneDivisorJmp)

		// Set the normal case's jump target.
		c.addSetJmpOrigins(okJmp)
	}

	// If x2 is placed in the quotient (AX) register, we just release it to the memory stack
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
	// Since the div instructions takes 2n byte dividend placed in DX:AX registers...
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

	// If this is signed rem instruction, we must set the jump target of
	// the exit jump from division -1 case towards the next instruction.
	if signedRemMinusOneDivisorJmp != nil {
		c.addSetJmpOrigins(signedRemMinusOneDivisorJmp)
	}
	return nil
}

// compileDivForFloats emits the instructions to perform division
// on the top two values of float type on the stack, placing the result back onto the stack.
// For example, stack [..., 1.0, 4.0] results in [..., 0.25].
func (c *amd64Compiler) compileDivForFloats(is32Bit bool) error {
	if is32Bit {
		return c.emitSimpleBinaryOp(x86.ADIVSS)
	} else {
		return c.emitSimpleBinaryOp(x86.ADIVSD)
	}
}

// compileAnd emits instructions to perform an "and" operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileAnd(o *wazeroir.OperationAnd) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.emitSimpleBinaryOp(x86.AANDL)
	case wazeroir.UnsignedInt64:
		err = c.emitSimpleBinaryOp(x86.AANDQ)
	}
	return
}

// compileOr emits instructions to perform an "or" operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileOr(o *wazeroir.OperationOr) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.emitSimpleBinaryOp(x86.AORL)
	case wazeroir.UnsignedInt64:
		err = c.emitSimpleBinaryOp(x86.AORQ)
	}
	return
}

// compileXor emits instructions to perform an xor operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileXor(o *wazeroir.OperationXor) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.emitSimpleBinaryOp(x86.AXORL)
	case wazeroir.UnsignedInt64:
		err = c.emitSimpleBinaryOp(x86.AXORQ)
	}
	return
}

// emitSimpleBinaryOp emits instructions to pop two values from the stack
// and perform the given instruction on these two values and push the result
// onto the stack.
func (c *amd64Compiler) emitSimpleBinaryOp(instruction obj.As) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	inst := c.newProg()
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = x2.register
	inst.To.Type = obj.TYPE_REG
	inst.To.Reg = x1.register
	inst.As = instruction
	c.addInstruction(inst)

	// We consumed x2 register after the operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)

	// We already stored the result in the register used by x1
	// so we record it.
	result := c.locationStack.pushValueOnRegister(x1.register)
	result.setRegisterType(x1.registerType())
	return nil
}

// compileShl emits instructions to perform a shift-left operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileShl(o *wazeroir.OperationShl) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.emitShiftOp(x86.ASHLL, false)
	case wazeroir.UnsignedInt64:
		err = c.emitShiftOp(x86.ASHLQ, true)
	}
	return
}

// compileShr emits instructions to perform a shift-right operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileShr(o *wazeroir.OperationShr) (err error) {
	switch o.Type {
	case wazeroir.SignedInt32:
		err = c.emitShiftOp(x86.ASARL, true)
	case wazeroir.SignedInt64:
		err = c.emitShiftOp(x86.ASARQ, false)
	case wazeroir.SignedUint32:
		err = c.emitShiftOp(x86.ASHRL, true)
	case wazeroir.SignedUint64:
		err = c.emitShiftOp(x86.ASHRQ, false)
	}
	return
}

// compileRotl emits instructions to perform a rotate-left operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileRotl(o *wazeroir.OperationRotl) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.emitShiftOp(x86.AROLL, true)
	case wazeroir.UnsignedInt64:
		err = c.emitShiftOp(x86.AROLQ, false)
	}
	return
}

// compileRotr emits instructions to perform a rotate-right operation on
// top two values on the stack, and pushes the result.
func (c *amd64Compiler) compileRotr(o *wazeroir.OperationRotr) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.emitShiftOp(x86.ARORL, true)
	case wazeroir.UnsignedInt64:
		err = c.emitShiftOp(x86.ARORQ, false)
	}
	return
}

// emitShiftOp adds instructions for shift operations (SHR, SHL, ROTR, ROTL)
// where we have to place the second value (shift counts) on the CX register.
func (c *amd64Compiler) emitShiftOp(instruction obj.As, is32Bit bool) error {
	const shiftCountRegister = x86.REG_CX

	x2 := c.locationStack.pop()
	if x2.onConditionalRegister() {
		if err := c.moveConditionalToFreeGeneralPurposeRegister(x2); err != nil {
			return err
		}
	}

	// Ensures that x2 (holding shift counts) is placed on the CX register.
	if (x2.onRegister() && x2.register != shiftCountRegister) || x2.onStack() {
		// If another value lives on the CX register, we release it to the stack.
		c.onValueReleaseRegisterToStack(shiftCountRegister)

		if x2.onRegister() {
			// If x2 lives on a register, we just move the value to CX.
			movToCX := c.newProg()
			movToCX.From.Type = obj.TYPE_REG
			movToCX.From.Reg = x2.register
			movToCX.To.Type = obj.TYPE_REG
			movToCX.To.Reg = shiftCountRegister
			if is32Bit {
				movToCX.As = x86.AMOVL
			} else {
				movToCX.As = x86.AMOVQ
			}
			c.addInstruction(movToCX)
			// We no longer place any value on the original register, so we record it.
			c.locationStack.markRegisterUnused(x2.register)
			// Instead, we've already placed the value on the CX register.
			x2.setRegister(shiftCountRegister)
		} else {
			// If it is on stack, we just move the memory allocated value to the CX register.
			x2.setRegister(shiftCountRegister)
			c.moveStackToRegister(x2)
		}
		c.locationStack.markRegisterUsed(shiftCountRegister)
	}

	x1 := c.locationStack.peek() // Note this is peek!

	inst := c.newProg()
	inst.As = instruction
	inst.From.Type = obj.TYPE_REG
	inst.From.Reg = x2.register
	if x1.onRegister() {
		inst.To.Type = obj.TYPE_REG
		inst.To.Reg = x1.register
	} else {
		// Shift target can be placed on a memory location.
		inst.To.Type = obj.TYPE_MEM
		inst.To.Reg = reservedRegisterForStackBasePointer
		inst.To.Offset = int64(x1.stackPointer) * 8
	}
	c.addInstruction(inst)

	// We consumed x2 register after the operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileAbs adds instructions to replace the top value of float type on the stack with its absolute value.
// For example, stack [..., -1.123] results in [..., 1.123].
// See the following discussions for how we could take the abs of floats on x86 assembly.
// https://stackoverflow.com/questions/32408665/fastest-way-to-compute-absolute-value-using-sse/32422471#32422471
// https://stackoverflow.com/questions/44630015/how-would-fabsdouble-be-implemented-on-x86-is-it-an-expensive-operation
func (c *amd64Compiler) compileAbs(o *wazeroir.OperationAbs) (err error) {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	// First shift left by one to clear the sign bit.
	shiftLeftByOne := c.newProg()
	if o.Type == wazeroir.Float32 {
		shiftLeftByOne.As = x86.APSLLL
	} else {
		shiftLeftByOne.As = x86.APSLLQ
	}
	shiftLeftByOne.To.Type = obj.TYPE_REG
	shiftLeftByOne.To.Reg = target.register
	shiftLeftByOne.From.Type = obj.TYPE_CONST
	shiftLeftByOne.From.Offset = 1
	c.addInstruction(shiftLeftByOne)

	// Then shift right by one.
	shiftRightByOne := c.newProg()
	if o.Type == wazeroir.Float32 {
		shiftRightByOne.As = x86.APSRLL
	} else {
		shiftRightByOne.As = x86.APSRLQ
	}
	shiftRightByOne.To.Type = obj.TYPE_REG
	shiftRightByOne.To.Reg = target.register
	shiftRightByOne.From.Type = obj.TYPE_CONST
	shiftRightByOne.From.Offset = 1
	c.addInstruction(shiftRightByOne)
	return nil
}

// compileNeg adds instructions to replace the top value of float type on the stack with its negated value.
// For example, stack [..., -1.123] results in [..., 1.123].
func (c *amd64Compiler) compileNeg(o *wazeroir.OperationNeg) (err error) {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	// First we move the sign-bit mask (placed in memory) to the tmp register,
	// since we cannot take XOR directly with float reg and const.
	movToTmp := c.newProg()
	var maskAddr uintptr
	if o.Type == wazeroir.Float32 {
		movToTmp.As = x86.AMOVL
		maskAddr = float32SignBitMaskAddress
	} else {
		movToTmp.As = x86.AMOVQ
		maskAddr = float64SignBitMaskAddress
	}
	movToTmp.From.Type = obj.TYPE_MEM
	movToTmp.From.Offset = int64(maskAddr)
	movToTmp.To.Type = obj.TYPE_REG
	movToTmp.To.Reg = tmpReg
	c.addInstruction(movToTmp)

	// Negate the value by XOR it with the sign-bit mask.
	negate := c.newProg()
	if o.Type == wazeroir.Float32 {
		negate.As = x86.AXORPS
	} else {
		negate.As = x86.AXORPD
	}
	negate.From.Type = obj.TYPE_REG
	negate.From.Reg = tmpReg
	negate.To.Type = obj.TYPE_REG
	negate.To.Reg = target.register
	c.addInstruction(negate)
	return nil
}

// compileCeil adds instructions to replace the top value of float type on the stack with its ceiling value.
// For example, stack [..., 1.123] results in [..., 2.0]. This is equivalent to math.Ceil.
func (c *amd64Compiler) compileCeil(o *wazeroir.OperationCeil) (err error) {
	// Internally, ceil can be performed via ROUND instruction with 0x02 mode.
	// See https://android.googlesource.com/platform/bionic/+/882b8af/libm/x86_64/ceilf.S for example.
	return c.emitRoundInstruction(o.Type == wazeroir.Float32, 0x02)
}

// compileFloor adds instructions to replace the top value of float type on the stack with its floor value.
// For example, stack [..., 1.123] results in [..., 1.0]. This is equivalent to math.Floor.
func (c *amd64Compiler) compileFloor(o *wazeroir.OperationFloor) (err error) {
	// Internally, floor can be performed via ROUND instruction with 0x01 mode.
	// See https://android.googlesource.com/platform/bionic/+/882b8af/libm/x86_64/floorf.S for example.
	return c.emitRoundInstruction(o.Type == wazeroir.Float32, 0x01)
}

// compileTrunc adds instructions to replace the top value of float type on the stack with its truncated value.
// For example, stack [..., 1.9] results in [..., 1.0]. This is equivalent to math.Trunc.
func (c *amd64Compiler) compileTrunc(o *wazeroir.OperationTrunc) error {
	// Internally, trunc can be performed via ROUND instruction with 0x03 mode.
	// See https://android.googlesource.com/platform/bionic/+/882b8af/libm/x86_64/truncf.S for example.
	return c.emitRoundInstruction(o.Type == wazeroir.Float32, 0x03)
}

// compileNearest adds instructions to replace the top value of float type on the stack with its nearest integer value.
// For example, stack [..., 1.9] results in [..., 2.0]. This is NOT equivalent to math.Round and instead has the same
// the semantics of LLVM's rint instrinsic. See https://llvm.org/docs/LangRef.html#llvm-rint-intrinsic.
// For example, math.Round(-4.5) produces -5 while ROUND with 0x00 mode produces -4.
func (c *amd64Compiler) compileNearest(o *wazeroir.OperationNearest) error {
	// Internally, nearest can be performed via ROUND instruction with 0x00 mode.
	// If we compile the following Wat by "wasmtime wasm2obj",
	//
	// (module
	//   (func (export "nearest_f32") (param $x f32) (result f32) (f32.nearest (local.get $x)))
	//   (func (export "nearest_f64") (param $x f64) (result f64) (f64.nearest (local.get $x)))
	// )
	//
	// we see a disassemble of the object via "objdump --disassemble-all" like:
	//
	// 0000000000000000 <_wasm_function_0>:
	// 	0:       55                      push   %rbp
	// 	1:       48 89 e5                mov    %rsp,%rbp
	// 	4:       66 0f 3a 0a c0 00       roundss $0x0,%xmm0,%xmm0
	// 	a:       48 89 ec                mov    %rbp,%rsp
	// 	d:       5d                      pop    %rbp
	// 	e:       c3                      retq
	//
	// 000000000000000f <_wasm_function_1>:
	// 	f:        55                      push   %rbp
	//  10:       48 89 e5                mov    %rsp,%rbp
	//  13:       66 0f 3a 0b c0 00       roundsd $0x0,%xmm0,%xmm0
	//  19:       48 89 ec                mov    %rbp,%rsp
	//  1c:       5d                      pop    %rbp
	//  1d:       c3                      retq
	//
	// Below, we use the same implementation: "rounds{s,d} $0x0,%xmm0,%xmm0" where the mode is set to zero.
	return c.emitRoundInstruction(o.Type == wazeroir.Float32, 0x00)
}

func (c *amd64Compiler) emitRoundInstruction(is32Bit bool, mode int64) error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	round := c.newProg()
	if is32Bit {
		round.As = x86.AROUNDSS
	} else {
		round.As = x86.AROUNDSD
	}
	round.To.Type = obj.TYPE_REG
	round.To.Reg = target.register
	round.From.Type = obj.TYPE_CONST
	round.From.Offset = mode
	round.RestArgs = append(round.RestArgs,
		obj.Addr{Reg: target.register, Type: obj.TYPE_REG})
	c.addInstruction(round)
	return nil
}

// compileMin adds instructions to pop two values from the stack, and push back the maximum of
// these two values onto the stack. For example, stack [..., 100.1, 1.9] results in [..., 1.9].
// For the cases where NaN involves, see the doc of emitMinOrMax below.
func (c *amd64Compiler) compileMin(o *wazeroir.OperationMin) error {
	is32Bit := o.Type == wazeroir.Float32
	if is32Bit {
		return c.emitMinOrMax(is32Bit, x86.AMINSS)
	} else {
		return c.emitMinOrMax(is32Bit, x86.AMINSD)
	}
}

// compileMax adds instructions to pop two values from the stack, and push back the maximum of
// these two values onto the stack. For example, stack [..., 100.1, 1.9] results in [..., 100.1].
// For the cases where NaN involves, see the doc of emitMinOrMax below.
func (c *amd64Compiler) compileMax(o *wazeroir.OperationMax) error {
	is32Bit := o.Type == wazeroir.Float32
	if is32Bit {
		return c.emitMinOrMax(is32Bit, x86.AMAXSS)
	} else {
		return c.emitMinOrMax(is32Bit, x86.AMAXSD)
	}
}

// emitMinOrMax adds instructions to pop two values from the stack, and push back either minimum or
// minimum of these two values onto the stack according to the minOrMaxInstruction argument.
// minOrMaxInstruction must be one of MAXSS, MAXSD, MINSS or MINSD.
// Note: These native min/max instructions are almost compatible with min/max in the Wasm specification,
// but it is slightly different with respect to the NaN handling.
// Native min/max instructions return non-NaN value if exactly one of target values
// is NaN. For example native_{min,max}(5.0, NaN) returns always 5.0, not NaN.
// However, WebAssembly specifies that min/max must always return NaN if one of values is NaN.
// Therefore in this function, we have to add conditional jumps to check if one of values is NaN before
// the native min/max, which is why we cannot simply emit a native min/max instruction here.
//
// For the semantics, see wazeroir.Min and wazeroir.Max for detail.
func (c *amd64Compiler) emitMinOrMax(is32Bit bool, minOrMaxInstruction obj.As) error {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}
	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Check if this is (either x1 or x2 is NaN) or (x1 equals x2) case
	checkNaNOrEquals := c.newProg()
	if is32Bit {
		checkNaNOrEquals.As = x86.AUCOMISS
	} else {
		checkNaNOrEquals.As = x86.AUCOMISD
	}
	checkNaNOrEquals.From.Type = obj.TYPE_REG
	checkNaNOrEquals.From.Reg = x2.register
	checkNaNOrEquals.To.Type = obj.TYPE_REG
	checkNaNOrEquals.To.Reg = x1.register
	c.addInstruction(checkNaNOrEquals)

	// At this point, we have the three cases of conditional flags below
	// (See https://www.felixcloutier.com/x86/ucomiss#operation for detail.)
	//
	// 1) Two values are NaN-free and different: All flags are cleared.
	// 2) Two values are NaN-free and equal: Only ZF flags is set.
	// 3) One of Two values is NaN: ZF, PF and CF flags are set.

	// Jump instruction to handle 1) case by checking the ZF flag
	// as ZF is only set for 2) and 3) cases.
	nanFreeOrDiffJump := c.newProg()
	nanFreeOrDiffJump.As = x86.AJNE
	nanFreeOrDiffJump.To.Type = obj.TYPE_BRANCH
	c.addInstruction(nanFreeOrDiffJump)

	// Start handling 2) and 3).

	// Jump if two values are equal and NaN-free by checking the parity flag (PF).
	// Here we use JPC to do the conditional jump when the parity flag is NOT set,
	// and that is of 2).
	equalExitJmp := c.newProg()
	equalExitJmp.As = x86.AJPC
	equalExitJmp.To.Type = obj.TYPE_BRANCH
	c.addInstruction(equalExitJmp)

	// Start handling 3).

	// We emit the ADD instruction to produce the NaN in x1.
	copyNan := c.newProg()
	if is32Bit {
		copyNan.As = x86.AADDSS
	} else {
		copyNan.As = x86.AADDSD
	}
	copyNan.From.Type = obj.TYPE_REG
	copyNan.From.Reg = x2.register
	copyNan.To.Type = obj.TYPE_REG
	copyNan.To.Reg = x1.register
	c.addInstruction(copyNan)

	// Exit from the NaN case branch.
	nanExitJmp := c.newProg()
	nanExitJmp.As = obj.AJMP
	nanExitJmp.To.Type = obj.TYPE_BRANCH
	c.addInstruction(nanExitJmp)

	// Start handling 1).

	// Now handle the NaN-free and different values case.
	nanFreeOrDiff := c.newProg()
	nanFreeOrDiffJump.To.SetTarget(nanFreeOrDiff)
	nanFreeOrDiff.As = minOrMaxInstruction
	nanFreeOrDiff.From.Type = obj.TYPE_REG
	nanFreeOrDiff.From.Reg = x2.register
	nanFreeOrDiff.To.Type = obj.TYPE_REG
	nanFreeOrDiff.To.Reg = x1.register
	c.addInstruction(nanFreeOrDiff)

	// Set the jump target of 1) and 2) cases to the next instruction after 3) case.
	c.addSetJmpOrigins(nanExitJmp, equalExitJmp)

	// Record that we consumed the x2 and placed the minOrMax result in the x1's register.
	c.locationStack.markRegisterUnused(x2.register)
	c.locationStack.pushValueOnRegister(x1.register)
	return nil
}

// compileCopysign adds instructions to pop two float values from the stack, and copy the signbit of
// the first-popped value to the last one.
// For example, stack [..., 1.213, -5.0] results in [..., -1.213].
func (c *amd64Compiler) compileCopysign(o *wazeroir.OperationCopysign) error {
	is32Bit := o.Type == wazeroir.Float32

	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}
	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	// Move the rest bit mask to the temp register.
	movRestBitMask := c.newProg()
	movRestBitMask.From.Type = obj.TYPE_MEM
	if is32Bit {
		movRestBitMask.As = x86.AMOVL
		movRestBitMask.From.Offset = int64(float32RestBitMaskAddress)
	} else {
		movRestBitMask.As = x86.AMOVQ
		movRestBitMask.From.Offset = int64(float64RestBitMaskAddress)
	}
	movRestBitMask.To.Type = obj.TYPE_REG
	movRestBitMask.To.Reg = tmpReg
	c.addInstruction(movRestBitMask)

	// Clear the sign bit of x1 via AND with the mask.
	clearSignBit := c.newProg()
	clearSignBit.From.Type = obj.TYPE_REG
	clearSignBit.From.Reg = tmpReg
	clearSignBit.To.Type = obj.TYPE_REG
	clearSignBit.To.Reg = x1.register
	if is32Bit {
		clearSignBit.As = x86.AANDPS
	} else {
		clearSignBit.As = x86.AANDPD
	}
	c.addInstruction(clearSignBit)

	// Move the sign bit mask to the temp register.
	movSignBitMask := c.newProg()
	movSignBitMask.From.Type = obj.TYPE_MEM
	if is32Bit {
		movSignBitMask.As = x86.AMOVL
		movSignBitMask.From.Offset = int64(float32SignBitMaskAddress)
	} else {
		movSignBitMask.As = x86.AMOVQ
		movSignBitMask.From.Offset = int64(float64SignBitMaskAddress)
	}
	movSignBitMask.To.Type = obj.TYPE_REG
	movSignBitMask.To.Reg = tmpReg
	c.addInstruction(movSignBitMask)

	// Clear the non-sign bits of x2 via AND with the mask.
	clearNonSignBit := c.newProg()
	clearNonSignBit.From.Type = obj.TYPE_REG
	clearNonSignBit.From.Reg = tmpReg
	clearNonSignBit.To.Type = obj.TYPE_REG
	clearNonSignBit.To.Reg = x2.register
	if is32Bit {
		clearNonSignBit.As = x86.AANDPS
	} else {
		clearNonSignBit.As = x86.AANDPD
	}
	c.addInstruction(clearNonSignBit)

	// Finally, copy the sign bit of x2 to x1.
	copySignBit := c.newProg()
	copySignBit.From.Type = obj.TYPE_REG
	copySignBit.From.Reg = x2.register
	copySignBit.To.Type = obj.TYPE_REG
	copySignBit.To.Reg = x1.register
	if is32Bit {
		copySignBit.As = x86.AORPS
	} else {
		copySignBit.As = x86.AORPD
	}
	c.addInstruction(copySignBit)

	// Record that we consumed the x2 and placed the copysign result in the x1's register.
	c.locationStack.markRegisterUnused(x2.register)
	c.locationStack.pushValueOnRegister(x1.register)
	return nil
}

// compileSqrt adds instructions to replace the top value of float type on the stack with its square root.
// For example, stack [..., 9.0] results in [..., 3.0]. This is equivalent to "math.Sqrt".
func (c *amd64Compiler) compileSqrt(o *wazeroir.OperationSqrt) error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	sqrt := c.newProg()
	if o.Type == wazeroir.Float32 {
		sqrt.As = x86.ASQRTSS
	} else {
		sqrt.As = x86.ASQRTSD
	}
	sqrt.From.Type = obj.TYPE_REG
	sqrt.From.Reg = target.register
	sqrt.To.Type = obj.TYPE_REG
	sqrt.To.Reg = target.register
	c.addInstruction(sqrt)
	return nil
}

// compileI32WrapFromI64 adds instructions to replace the 64-bit int on top of the stack
// with the corresponding 32-bit integer. This is equivalent to uint64(uint32(v)) in Go.
func (c *amd64Compiler) compileI32WrapFromI64() error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	mov := c.newProg()
	mov.As = x86.AMOVL
	mov.From.Type = obj.TYPE_REG
	mov.From.Reg = target.register
	mov.To.Type = obj.TYPE_REG
	mov.To.Reg = target.register
	c.addInstruction(mov)
	return nil
}

// compileITruncFromF adds instructions to replace the top value of float type on the stack with
// the corresponding int value. This is equivalent to int32(math.Trunc(float32(x))), uint32(math.Trunc(float64(x))), etc in Go.
//
// Please refer to [1] and [2] for when we encounter undefined behavior in the WebAssembly specification.
// To summarize, if the source float value is NaN or doesn't fit in the destination range of integers (incl. +=Inf),
// then the runtime behavior is undefined. In wazero, we exit the function in these undefined cases with
// jitCallStatusCodeInvalidFloatToIntConversion status code.
// [1] https://www.w3.org/TR/wasm-core-1/#-hrefop-trunc-umathrmtruncmathsfu_m-n-z for unsigned integers.
// [2] https://www.w3.org/TR/wasm-core-1/#-hrefop-trunc-smathrmtruncmathsfs_m-n-z for signed integers.
//
func (c *amd64Compiler) compileITruncFromF(o *wazeroir.OperationITruncFromF) (err error) {
	// Note: in the follwoing implementations, we use CVTSS2SI and CVTSD2SI to convert floats to signed integers.
	// According to the Intel manual ([1],[2]), if the source float value is either +-Inf or NaN, or it exceeds representative ranges
	// of target signed integer, then the instruction returns "masked" response float32SignBitMask (or float64SignBitMask for 64 bit case).
	// [1] Chapter 11.5.2, SIMD Floating-Point Exception Conditions in "Vol 1, Intel® 64 and IA-32 Architectures Manual"
	//     https://www.intel.com/content/www/us/en/architecture-and-technology/64-ia-32-architectures-software-developer-vol-1-manual.html
	// [2] https://xem.github.io/minix86/manual/intel-x86-and-64-manual-vol1/o_7281d5ea06a5b67a-268.html
	if o.InputType == wazeroir.Float32 && o.OutputType == wazeroir.SignedInt32 {
		err = c.emitSignedI32TruncFromFloat(true)
	} else if o.InputType == wazeroir.Float32 && o.OutputType == wazeroir.SignedInt64 {
		err = c.emitSignedI64TruncFromFloat(true)
	} else if o.InputType == wazeroir.Float64 && o.OutputType == wazeroir.SignedInt32 {
		err = c.emitSignedI32TruncFromFloat(false)
	} else if o.InputType == wazeroir.Float64 && o.OutputType == wazeroir.SignedInt64 {
		err = c.emitSignedI64TruncFromFloat(false)
	} else if o.InputType == wazeroir.Float32 && o.OutputType == wazeroir.SignedUint32 {
		err = c.emitUnsignedI32TruncFromFloat(true)
	} else if o.InputType == wazeroir.Float32 && o.OutputType == wazeroir.SignedUint64 {
		err = c.emitUnsignedI64TruncFromFloat(true)
	} else if o.InputType == wazeroir.Float64 && o.OutputType == wazeroir.SignedUint32 {
		err = c.emitUnsignedI32TruncFromFloat(false)
	} else if o.InputType == wazeroir.Float64 && o.OutputType == wazeroir.SignedUint64 {
		err = c.emitUnsignedI64TruncFromFloat(false)
	}
	return
}

// emitUnsignedI32TruncFromFloat implements compileITruncFromF when the destination type is a 32-bit unsigned integer.
func (c *amd64Compiler) emitUnsignedI32TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we check the source float value is above or equal math.MaxInt32+1.
	cmpWithMaxInt32PlusOne := c.newProg()
	cmpWithMaxInt32PlusOne.From.Type = obj.TYPE_MEM
	if isFloat32Bit {
		cmpWithMaxInt32PlusOne.As = x86.AUCOMISS
		cmpWithMaxInt32PlusOne.From.Offset = int64(float32ForMaximumSigned32bitIntPlusOneAddress)
	} else {
		cmpWithMaxInt32PlusOne.As = x86.AUCOMISD
		cmpWithMaxInt32PlusOne.From.Offset = int64(float64ForMaximumSigned32bitIntPlusOneAddress)
	}
	cmpWithMaxInt32PlusOne.To.Type = obj.TYPE_REG
	cmpWithMaxInt32PlusOne.To.Reg = source.register
	c.addInstruction(cmpWithMaxInt32PlusOne)

	// Jump if the source float values is above or equal math.MaxInt32+1.
	jmpAboveOrEqualMaxIn32PlusOne := c.newProg()
	jmpAboveOrEqualMaxIn32PlusOne.As = x86.AJCC
	jmpAboveOrEqualMaxIn32PlusOne.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpAboveOrEqualMaxIn32PlusOne)

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNaN := c.newProg()
	jmpIfNaN.As = x86.AJPS // jump if parity is set.
	jmpIfNaN.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNaN)

	// Next we conver the value as a signed integer.
	convert := c.newProg()
	if isFloat32Bit {
		convert.As = x86.ACVTTSS2SL
	} else {
		convert.As = x86.ACVTTSD2SL
	}
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = source.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = result
	c.addInstruction(convert)

	// Then if the result is minus, it is invalid conversion from minus float (incl. -Inf).
	testIfMinusOrMinusInf := c.newProg()
	testIfMinusOrMinusInf.As = x86.ATESTL
	testIfMinusOrMinusInf.From.Type = obj.TYPE_REG
	testIfMinusOrMinusInf.From.Reg = result
	testIfMinusOrMinusInf.To.Type = obj.TYPE_REG
	testIfMinusOrMinusInf.To.Reg = result
	c.addInstruction(testIfMinusOrMinusInf)

	jmpIfMinusOrMinusInf := c.newProg()
	jmpIfMinusOrMinusInf.As = x86.AJMI
	jmpIfMinusOrMinusInf.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfMinusOrMinusInf)

	// Otherwise, the valus is valid.
	okJmpForLessThanMaxInt32PlusOne := c.newProg()
	okJmpForLessThanMaxInt32PlusOne.As = obj.AJMP
	okJmpForLessThanMaxInt32PlusOne.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmpForLessThanMaxInt32PlusOne)

	// Now, start handling the case where the original float value is above or equal math.MaxInt32+1.
	//
	// First, we subtract the math.MaxInt32+1 from the original value so it can fit in signed 32-bit integer.
	subMaxInt32PlusOne := c.newProg()
	jmpAboveOrEqualMaxIn32PlusOne.To.SetTarget(subMaxInt32PlusOne)
	subMaxInt32PlusOne.From.Type = obj.TYPE_MEM
	if isFloat32Bit {
		subMaxInt32PlusOne.As = x86.ASUBSS
		subMaxInt32PlusOne.From.Offset = int64(float32ForMaximumSigned32bitIntPlusOneAddress)
	} else {
		subMaxInt32PlusOne.As = x86.ASUBSD
		subMaxInt32PlusOne.From.Offset = int64(float64ForMaximumSigned32bitIntPlusOneAddress)
	}
	subMaxInt32PlusOne.To.Type = obj.TYPE_REG
	subMaxInt32PlusOne.To.Reg = source.register
	c.addInstruction(subMaxInt32PlusOne)

	// Then, convert the subtracted value as a signed 32-bit integer.
	convertAfterSub := c.newProg()
	if isFloat32Bit {
		convertAfterSub.As = x86.ACVTTSS2SL
	} else {
		convertAfterSub.As = x86.ACVTTSD2SL
	}
	convertAfterSub.From.Type = obj.TYPE_REG
	convertAfterSub.From.Reg = source.register
	convertAfterSub.To.Type = obj.TYPE_REG
	convertAfterSub.To.Reg = result
	c.addInstruction(convertAfterSub)

	// Next, we have to check if the value is from NaN, +Inf.
	// NaN or +Inf cases result in 0x8000_0000 according to the semantics of conversion,
	// This means we check if the result int value is minus or not.
	testIfMinus := c.newProg()
	testIfMinus.As = x86.ATESTL
	testIfMinus.From.Type = obj.TYPE_REG
	testIfMinus.From.Reg = result
	testIfMinus.To.Type = obj.TYPE_REG
	testIfMinus.To.Reg = result
	c.addInstruction(testIfMinus)

	// If the result is minus, the conversion is invalid (from NaN or +Inf)
	jmpIfNaNOrPlusInf := c.newProg()
	jmpIfNaNOrPlusInf.As = x86.AJMI
	jmpIfNaNOrPlusInf.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNaNOrPlusInf)

	// Otherwise, we successfully converted the the source float minus (math.MaxInt32+1) to int.
	// So, we retrieve the original source float value by adding the sign mask.
	add := c.newProg()
	add.As = x86.AADDL
	add.From.Type = obj.TYPE_MEM
	add.From.Offset = int64(float32SignBitMaskAddress)
	add.To.Type = obj.TYPE_REG
	add.To.Reg = result
	c.addInstruction(add)

	okJmpForAboveOrEqualMaxInt32PlusOne := c.newProg()
	okJmpForAboveOrEqualMaxInt32PlusOne.As = obj.AJMP
	okJmpForAboveOrEqualMaxInt32PlusOne.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmpForAboveOrEqualMaxInt32PlusOne)

	// Start emitting the error handling. These invalid float conversions are unrecoverable,
	// so we must exit from the function with the dedicated status code.
	c.addSetJmpOrigins(jmpIfMinusOrMinusInf, jmpIfNaN, jmpIfNaNOrPlusInf)
	c.setJITStatus(jitCallStatusCodeInvalidFloatToIntConversion)
	c.returnFunction()

	// We jump to the next instructions for valid cases.
	c.addSetJmpOrigins(okJmpForLessThanMaxInt32PlusOne, okJmpForAboveOrEqualMaxInt32PlusOne)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.locationStack.pushValueOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// emitUnsignedI32TruncFromFloat implements compileITruncFromF when the destination type is a 64-bit unsigned integer.
func (c *amd64Compiler) emitUnsignedI64TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we check the source float value is above or equal math.MaxInt64+1.
	cmpWithMaxInt32PlusOne := c.newProg()
	cmpWithMaxInt32PlusOne.From.Type = obj.TYPE_MEM
	if isFloat32Bit {
		cmpWithMaxInt32PlusOne.As = x86.AUCOMISS
		cmpWithMaxInt32PlusOne.From.Offset = int64(float32ForMaximumSigned64bitIntPlusOneAddress)
	} else {
		cmpWithMaxInt32PlusOne.As = x86.AUCOMISD
		cmpWithMaxInt32PlusOne.From.Offset = int64(float64ForMaximumSigned64bitIntPlusOneAddress)
	}
	cmpWithMaxInt32PlusOne.To.Type = obj.TYPE_REG
	cmpWithMaxInt32PlusOne.To.Reg = source.register
	c.addInstruction(cmpWithMaxInt32PlusOne)

	// Jump if the source float values is above or equal math.MaxInt64+1.
	jmpAboveOrEqualMaxIn32PlusOne := c.newProg()
	jmpAboveOrEqualMaxIn32PlusOne.As = x86.AJCC
	jmpAboveOrEqualMaxIn32PlusOne.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpAboveOrEqualMaxIn32PlusOne)

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNaN := c.newProg()
	jmpIfNaN.As = x86.AJPS // jump if parity is set.
	jmpIfNaN.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNaN)

	// Next we convert the value as a signed integer.
	convert := c.newProg()
	if isFloat32Bit {
		convert.As = x86.ACVTTSS2SQ
	} else {
		convert.As = x86.ACVTTSD2SQ
	}
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = source.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = result
	c.addInstruction(convert)

	// Then if the result is minus, it is invalid conversion from minus float (incl. -Inf).
	testIfMinusOrMinusInf := c.newProg()
	testIfMinusOrMinusInf.As = x86.ATESTQ
	testIfMinusOrMinusInf.From.Type = obj.TYPE_REG
	testIfMinusOrMinusInf.From.Reg = result
	testIfMinusOrMinusInf.To.Type = obj.TYPE_REG
	testIfMinusOrMinusInf.To.Reg = result
	c.addInstruction(testIfMinusOrMinusInf)

	jmpIfMinusOrMinusInf := c.newProg()
	jmpIfMinusOrMinusInf.As = x86.AJMI
	jmpIfMinusOrMinusInf.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfMinusOrMinusInf)

	// Otherwise, the valus is valid.
	okJmpForLessThanMaxInt64PlusOne := c.newProg()
	okJmpForLessThanMaxInt64PlusOne.As = obj.AJMP
	okJmpForLessThanMaxInt64PlusOne.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmpForLessThanMaxInt64PlusOne)

	// Now, start handling the case where the original float value is above or equal math.MaxInt64+1.
	//
	// First, we subtract the math.MaxInt64+1 from the original value so it can fit in signed 64-bit integer.
	subMaxInt64PlusOne := c.newProg()
	jmpAboveOrEqualMaxIn32PlusOne.To.SetTarget(subMaxInt64PlusOne)
	subMaxInt64PlusOne.From.Type = obj.TYPE_MEM
	if isFloat32Bit {
		subMaxInt64PlusOne.As = x86.ASUBSS
		subMaxInt64PlusOne.From.Offset = int64(float32ForMaximumSigned64bitIntPlusOneAddress)
	} else {
		subMaxInt64PlusOne.As = x86.ASUBSD
		subMaxInt64PlusOne.From.Offset = int64(float64ForMaximumSigned64bitIntPlusOneAddress)
	}
	subMaxInt64PlusOne.To.Type = obj.TYPE_REG
	subMaxInt64PlusOne.To.Reg = source.register
	c.addInstruction(subMaxInt64PlusOne)

	// Then, convert the subtracted value as a signed 64-bit integer.
	convertAfterSub := c.newProg()
	if isFloat32Bit {
		convertAfterSub.As = x86.ACVTTSS2SQ
	} else {
		convertAfterSub.As = x86.ACVTTSD2SQ
	}
	convertAfterSub.From.Type = obj.TYPE_REG
	convertAfterSub.From.Reg = source.register
	convertAfterSub.To.Type = obj.TYPE_REG
	convertAfterSub.To.Reg = result
	c.addInstruction(convertAfterSub)

	// Next, we have to check if the value is from NaN, +Inf.
	// NaN or +Inf cases result in 0x8000_0000 according to the semantics of conversion,
	// This means we check if the result int value is minus or not.
	testIfMinus := c.newProg()
	testIfMinus.As = x86.ATESTQ
	testIfMinus.From.Type = obj.TYPE_REG
	testIfMinus.From.Reg = result
	testIfMinus.To.Type = obj.TYPE_REG
	testIfMinus.To.Reg = result
	c.addInstruction(testIfMinus)

	// If the result is minus, the conversion is invalid (from NaN or +Inf)
	jmpIfNaNOrPlusInf := c.newProg()
	jmpIfNaNOrPlusInf.As = x86.AJMI
	jmpIfNaNOrPlusInf.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNaNOrPlusInf)

	// Otherwise, we successfully converted the the source float minus (math.MaxInt64+1) to int.
	// So, we retrieve the original source float value by adding the sign mask.
	add := c.newProg()
	add.As = x86.AADDQ
	add.From.Type = obj.TYPE_MEM
	add.From.Offset = int64(float64SignBitMaskAddress)
	add.To.Type = obj.TYPE_REG
	add.To.Reg = result
	c.addInstruction(add)

	okJmpForAboveOrEqualMaxInt64PlusOne := c.newProg()
	okJmpForAboveOrEqualMaxInt64PlusOne.As = obj.AJMP
	okJmpForAboveOrEqualMaxInt64PlusOne.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmpForAboveOrEqualMaxInt64PlusOne)

	// Start emitting the error handling. These invalid flaot conversions are unrecoverable,
	// so we must exit from the function with the dedicated status code.
	c.addSetJmpOrigins(jmpIfMinusOrMinusInf, jmpIfNaN, jmpIfNaNOrPlusInf)
	c.setJITStatus(jitCallStatusCodeInvalidFloatToIntConversion)
	c.returnFunction()

	// We jump to the next instructions for valid cases.
	c.addSetJmpOrigins(okJmpForLessThanMaxInt64PlusOne, okJmpForAboveOrEqualMaxInt64PlusOne)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.locationStack.pushValueOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// emitSignedI32TruncFromFloat implements compileITruncFromF when the destination type is a 32-bit signed integer.
func (c *amd64Compiler) emitSignedI32TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First we unconditionally convert source to integer via CVTTSS2SI (CVTTSD2SI for 64bit float).
	convert := c.newProg()
	if isFloat32Bit {
		convert.As = x86.ACVTTSS2SL
	} else {
		convert.As = x86.ACVTTSD2SL
	}
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = source.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = result
	c.addInstruction(convert)

	// We compare the conversion result with the sign bit mask to check if it is either
	// 1) the source float value is either +-Inf or NaN, or it exceeds representative ranges of 32bit signed integer, or
	// 2) the source equals the minimum signed 32-bit (=-2147483648.000000) whose bit pattern is float32ForMinimumSigned32bitIntegerAdddress for 32 bit flaot
	// 	  or float64ForMinimumSigned32bitIntegerAdddress for 64bit float.
	cmpResult := c.newProg()
	cmpResult.As = x86.ACMPL
	cmpResult.From.Type = obj.TYPE_MEM
	cmpResult.From.Offset = int64(float32SignBitMaskAddress)
	cmpResult.To.Type = obj.TYPE_REG
	cmpResult.To.Reg = result
	c.addInstruction(cmpResult)

	// Otherwise, jump to exit as the result is valid.
	okJmp := c.newProg()
	okJmp.As = x86.AJNE
	okJmp.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmp)

	// Start handling the case of 1) and 2).
	// First, check if the value is NaN.
	checkIfNaN := c.newProg()
	if isFloat32Bit {
		checkIfNaN.As = x86.AUCOMISS
	} else {
		checkIfNaN.As = x86.AUCOMISD
	}
	checkIfNaN.From.Type = obj.TYPE_REG
	checkIfNaN.From.Reg = source.register
	checkIfNaN.To.Type = obj.TYPE_REG
	checkIfNaN.To.Reg = source.register
	c.addInstruction(checkIfNaN)

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNaN := c.newProg()
	jmpIfNaN.As = x86.AJPS // jump if parity is set.
	jmpIfNaN.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNaN)

	// Check if the value is larger than or equal the minimum 32-bit integer value,
	// meaning that the value exceeds the lower bound of 32-bit signed integer range.
	checkIfExceedsLowerBound := c.newProg()
	if isFloat32Bit {
		checkIfExceedsLowerBound.As = x86.AUCOMISS
		checkIfExceedsLowerBound.From.Offset = int64(float32ForMinimumSigned32bitIntegerAdddress)
	} else {
		checkIfExceedsLowerBound.As = x86.AUCOMISD
		checkIfExceedsLowerBound.From.Offset = int64(float64ForMinimumSigned32bitIntegerAdddress)
	}
	checkIfExceedsLowerBound.From.Type = obj.TYPE_MEM
	checkIfExceedsLowerBound.To.Type = obj.TYPE_REG
	checkIfExceedsLowerBound.To.Reg = source.register
	c.addInstruction(checkIfExceedsLowerBound)

	// Jump if the value is -Inf.
	jmpIfExceedsLowerBound := c.newProg()
	jmpIfExceedsLowerBound.As = x86.AJCS
	jmpIfExceedsLowerBound.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfExceedsLowerBound)

	// At this point, the value is the minimum signed 32-bit int (=-2147483648.000000) or larger than 32-bit maximum.
	// So, check if the value equals the minimum signed 32-bit int.
	checkIfMinimumSignedInt := c.newProg()
	if isFloat32Bit {
		checkIfMinimumSignedInt.As = x86.AUCOMISS
	} else {
		checkIfMinimumSignedInt.As = x86.AUCOMISD
	}
	checkIfMinimumSignedInt.From.Type = obj.TYPE_MEM
	checkIfMinimumSignedInt.From.Offset = int64(zero64BitAddress)
	checkIfMinimumSignedInt.To.Type = obj.TYPE_REG
	checkIfMinimumSignedInt.To.Reg = source.register
	c.addInstruction(checkIfMinimumSignedInt)

	jmpIfMinimumSignedInt := c.newProg()
	if isFloat32Bit {
		jmpIfExceedsLowerBound.As = x86.AJCS
	} else {
		jmpIfExceedsLowerBound.As = x86.AJLS
	}
	jmpIfMinimumSignedInt.As = x86.AJCS // jump if the value is minus (= the minimum signed 32-bit int).
	jmpIfMinimumSignedInt.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfMinimumSignedInt)

	// Start emitting the error handling. These invalid flaot conversions are unrecoverable,
	// so we must exit from the function with the dedicated status code.
	// These existing code are jumped from cases where the original float value
	// is either +-Inf, NaN or not in representative ranges of 32bit signed integer
	c.addSetJmpOrigins(jmpIfExceedsLowerBound, jmpIfNaN)
	c.setJITStatus(jitCallStatusCodeInvalidFloatToIntConversion)
	c.returnFunction()

	// We jump to the next instructions for valid cases.
	c.addSetJmpOrigins(okJmp, jmpIfMinimumSignedInt)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.locationStack.pushValueOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// emitSignedI64TruncFromFloat implements compileITruncFromF when the destination type is a 64-bit signed integer.
func (c *amd64Compiler) emitSignedI64TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First we unconditionally convert source to integer via CVTTSS2SI (CVTTSD2SI for 64bit float).
	convert := c.newProg()
	if isFloat32Bit {
		convert.As = x86.ACVTTSS2SQ
	} else {
		convert.As = x86.ACVTTSD2SQ
	}
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = source.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = result
	c.addInstruction(convert)

	// We compare the conversion result with the sign bit mask to check if it is either
	// 1) the source float value is either +-Inf or NaN, or it exceeds representative ranges of 32bit signed integer, or
	// 2) the source equals the minimum signed 32-bit (=-9223372036854775808.0) whose bit pattern is float32ForMinimumSigned64bitIntegerAdddress for 32 bit flaot
	// 	  or float64ForMinimumSigned64bitIntegerAdddress for 64bit float.
	cmpResult := c.newProg()
	cmpResult.As = x86.ACMPQ
	cmpResult.From.Type = obj.TYPE_MEM
	cmpResult.From.Offset = int64(float64SignBitMaskAddress)
	cmpResult.To.Type = obj.TYPE_REG
	cmpResult.To.Reg = result
	c.addInstruction(cmpResult)

	// Otherwise, we simply jump to exit as the result is valid.
	okJmp := c.newProg()
	okJmp.As = x86.AJNE
	okJmp.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmp)

	// Start handling the case of 1) and 2).
	// First, check if the value is NaN.
	checkIfNaN := c.newProg()
	if isFloat32Bit {
		checkIfNaN.As = x86.AUCOMISS
	} else {
		checkIfNaN.As = x86.AUCOMISD
	}
	checkIfNaN.From.Type = obj.TYPE_REG
	checkIfNaN.From.Reg = source.register
	checkIfNaN.To.Type = obj.TYPE_REG
	checkIfNaN.To.Reg = source.register
	c.addInstruction(checkIfNaN)

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNaN := c.newProg()
	jmpIfNaN.As = x86.AJPS // jump if parity is set.
	jmpIfNaN.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfNaN)

	// Check if the value is larger than or equal the minimum 64-bit integer value,
	// meaning that the value exceeds the lower bound of 64-bit signed integer range.
	checkIfExceedsLowerBound := c.newProg()
	if isFloat32Bit {
		checkIfExceedsLowerBound.As = x86.AUCOMISS
		checkIfExceedsLowerBound.From.Offset = int64(float32ForMinimumSigned64bitIntegerAdddress)
	} else {
		checkIfExceedsLowerBound.As = x86.AUCOMISD
		checkIfExceedsLowerBound.From.Offset = int64(float64ForMinimumSigned64bitIntegerAdddress)
	}
	checkIfExceedsLowerBound.From.Type = obj.TYPE_MEM
	checkIfExceedsLowerBound.To.Type = obj.TYPE_REG
	checkIfExceedsLowerBound.To.Reg = source.register
	c.addInstruction(checkIfExceedsLowerBound)

	// Jump if the value is -Inf.
	jmpIfExceedsLowerBound := c.newProg()
	jmpIfExceedsLowerBound.As = x86.AJCS
	jmpIfExceedsLowerBound.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfExceedsLowerBound)

	// At this point, the value is the minimum signed 64-bit int (=-9223372036854775808.0) or larger than 64-bit maximum.
	// So, check if the value equals the minimum signed 64-bit int.
	checkIfMinimumSignedInt := c.newProg()
	if isFloat32Bit {
		checkIfMinimumSignedInt.As = x86.AUCOMISS
	} else {
		checkIfMinimumSignedInt.As = x86.AUCOMISD
	}
	checkIfMinimumSignedInt.From.Type = obj.TYPE_MEM
	checkIfMinimumSignedInt.From.Offset = int64(zero64BitAddress)
	checkIfMinimumSignedInt.To.Type = obj.TYPE_REG
	checkIfMinimumSignedInt.To.Reg = source.register
	c.addInstruction(checkIfMinimumSignedInt)

	jmpIfMinimumSignedInt := c.newProg()
	jmpIfMinimumSignedInt.As = x86.AJCS // jump if the value is minus (= the minimum signed 64-bit int).
	jmpIfMinimumSignedInt.To.Type = obj.TYPE_BRANCH
	c.addInstruction(jmpIfMinimumSignedInt)

	// Start emitting the error handling. These invalid flaot conversions are unrecoverable,
	// so we must exit from the function with the dedicated status code.
	// These existing code are jumped from cases where the original float value
	// is either +-Inf, NaN or not in representative ranges of 64 bit signed integer
	c.addSetJmpOrigins(jmpIfExceedsLowerBound, jmpIfNaN)
	c.setJITStatus(jitCallStatusCodeInvalidFloatToIntConversion)
	c.returnFunction()

	// We jump to the next instructions for valid cases.
	c.addSetJmpOrigins(okJmp, jmpIfMinimumSignedInt)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.locationStack.pushValueOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileFConvertFromI adds instructions to replace the top value of int type on the stack with
// the corresponding float value. This is equivalent to float32(uint32(x)), float32(int32(x)), etc in Go.
func (c *amd64Compiler) compileFConvertFromI(o *wazeroir.OperationFConvertFromI) (err error) {
	if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedInt32 {
		err = c.emitSimpleConversion(x86.ACVTSL2SS, generalPurposeRegisterTypeFloat) // = CVTSI2SS for 32bit int
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedInt64 {
		err = c.emitSimpleConversion(x86.ACVTSQ2SS, generalPurposeRegisterTypeFloat) // = CVTSI2SS for 64bit int
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedInt32 {
		err = c.emitSimpleConversion(x86.ACVTSL2SD, generalPurposeRegisterTypeFloat) // = CVTSI2SD for 32bit int
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedInt64 {
		err = c.emitSimpleConversion(x86.ACVTSQ2SD, generalPurposeRegisterTypeFloat) // = CVTSI2SD for 64bit int
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedUint32 {
		// See the following link for why we use 64bit conversion for unsigned 32bit integer sources:
		// https://stackoverflow.com/questions/41495498/fpu-operations-generated-by-gcc-during-casting-integer-to-float.
		//
		// Here's the summary:
		// >> CVTSI2SS is indeed designed for converting a signed integer to a scalar single-precision float,
		// >> not an unsigned integer like you have here. So what gives? Well, a 64-bit processor has 64-bit wide
		// >> registers available, so the unsigned 32-bit input values can be stored as signed 64-bit intermediate values,
		// >> which allows CVTSI2SS to be used after all.
		err = c.emitSimpleConversion(x86.ACVTSQ2SS, generalPurposeRegisterTypeFloat) // = CVTSI2SS for 64bit int.
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedUint32 {
		// For the same reason above, we use 64bit conversion for unsigned 32bit.
		err = c.emitSimpleConversion(x86.ACVTSQ2SD, generalPurposeRegisterTypeFloat) // = CVTSI2SD for 64bit int.
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedUint64 {
		err = c.emitUnsignedInt64ToFloatConversion(true)
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedUint64 {
		err = c.emitUnsignedInt64ToFloatConversion(false)
	}
	return
}

// emitUnsignedInt64ToFloatConversion is handling the case of unsigned 64-bit integer
// in compileFConvertFromI.
func (c *amd64Compiler) emitUnsignedInt64ToFloatConversion(isFloat32bit bool) error {
	// The logic here is exactly the same as GCC emits for the following code:
	//
	// float convert(int num) {
	//     float foo;
	//     uint64_t ptr1 = 100;
	//     foo = (float)(ptr1);
	//     return foo;
	// }
	//
	// which is compiled by GCC as
	//
	// convert:
	// 	   push    rbp
	// 	   mov     rbp, rsp
	// 	   mov     DWORD PTR [rbp-20], edi
	// 	   mov     DWORD PTR [rbp-4], 100
	// 	   mov     eax, DWORD PTR [rbp-4]
	// 	   test    rax, rax
	// 	   js      .handle_sign_bit_case
	// 	   cvtsi2ss        xmm0, rax
	// 	   jmp     .exit
	// .handle_sign_bit_case:
	// 	   mov     rdx, rax
	// 	   shr     rdx
	// 	   and     eax, 1
	// 	   or      rdx, rax
	// 	   cvtsi2ss        xmm0, rdx
	// 	   addsd   xmm0, xmm0
	// .exit: ...
	//
	// tl;dr is that we have a branch depending on whether or not sign bit is set.

	origin := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(origin); err != nil {
		return err
	}

	dest, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	// Check if the most significant bit (sign bit) is set.
	test := c.newProg()
	test.As = x86.ATESTQ
	test.From.Type = obj.TYPE_REG
	test.From.Reg = origin.register
	test.To.Type = obj.TYPE_REG
	test.To.Reg = origin.register
	c.addInstruction(test)

	// Jump if the sign bit is set.
	jmpIfSignbitSet := c.newProg()
	jmpIfSignbitSet.To.Type = obj.TYPE_BRANCH
	jmpIfSignbitSet.As = x86.AJMI
	c.addInstruction(jmpIfSignbitSet)

	// Otherwise, we could fit the unsigned int into float32.
	// So, we convert it to float32 and emit jump instruction to exit from this branch.
	convert := c.newProg()
	if isFloat32bit {
		convert.As = x86.ACVTSQ2SS
	} else {
		convert.As = x86.ACVTSQ2SD
	}
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = origin.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = dest
	c.addInstruction(convert)

	exitFromSignbitUnSet := c.newProg()
	exitFromSignbitUnSet.As = obj.AJMP
	exitFromSignbitUnSet.To.Type = obj.TYPE_BRANCH
	c.addInstruction(exitFromSignbitUnSet)

	// Now handling the case where sign-bit is set.
	// We emit the following sequences:
	// 	   mov     tmpReg, origin
	// 	   shr     tmpReg, 1
	// 	   and     origin, 1
	// 	   or      tmpReg, origin
	// 	   cvtsi2ss        xmm0, tmpReg
	// 	   addsd   xmm0, xmm0
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	movToTmp := c.newProg()
	jmpIfSignbitSet.To.SetTarget(movToTmp)
	movToTmp.As = x86.AMOVQ
	movToTmp.From.Type = obj.TYPE_REG
	movToTmp.From.Reg = origin.register
	movToTmp.To.Type = obj.TYPE_REG
	movToTmp.To.Reg = tmpReg
	c.addInstruction(movToTmp)

	divideBy2 := c.newProg()
	divideBy2.As = x86.ASHRQ
	divideBy2.From.Type = obj.TYPE_CONST
	divideBy2.From.Offset = 1
	divideBy2.To.Type = obj.TYPE_REG
	divideBy2.To.Reg = tmpReg
	c.addInstruction(divideBy2)

	rescueLeastSignificantBit := c.newProg()
	rescueLeastSignificantBit.As = x86.AANDQ
	rescueLeastSignificantBit.From.Type = obj.TYPE_CONST
	rescueLeastSignificantBit.From.Offset = 0x1
	rescueLeastSignificantBit.To.Type = obj.TYPE_REG
	rescueLeastSignificantBit.To.Reg = origin.register
	c.addInstruction(rescueLeastSignificantBit)

	addRescuedBit := c.newProg()
	addRescuedBit.As = x86.AORQ
	addRescuedBit.From.Type = obj.TYPE_REG
	addRescuedBit.From.Reg = origin.register
	addRescuedBit.To.Type = obj.TYPE_REG
	addRescuedBit.To.Reg = tmpReg
	c.addInstruction(addRescuedBit)

	convertDividedBy2Value := c.newProg()
	if isFloat32bit {
		convertDividedBy2Value.As = x86.ACVTSQ2SS
	} else {
		convertDividedBy2Value.As = x86.ACVTSQ2SD
	}
	convertDividedBy2Value.From.Type = obj.TYPE_REG
	convertDividedBy2Value.From.Reg = tmpReg
	convertDividedBy2Value.To.Type = obj.TYPE_REG
	convertDividedBy2Value.To.Reg = dest
	c.addInstruction(convertDividedBy2Value)

	multiplyBy2 := c.newProg()
	if isFloat32bit {
		multiplyBy2.As = x86.AADDSS
	} else {
		multiplyBy2.As = x86.AADDSD
	}
	multiplyBy2.From.Type = obj.TYPE_REG
	multiplyBy2.From.Reg = dest
	multiplyBy2.To.Type = obj.TYPE_REG
	multiplyBy2.To.Reg = dest
	c.addInstruction(multiplyBy2)

	// Now, we finished the sign-bit set branch.
	// We have to make the exit jump target of sign-bit unset branch
	// towards the next instruction.
	c.addSetJmpOrigins(exitFromSignbitUnSet)

	// We consumed the origin's register and placed the conversion result
	// in the dest register.
	c.locationStack.markRegisterUnused(origin.register)
	loc := c.locationStack.pushValueOnRegister(dest)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)
	return nil
}

// emitSimpleConversion pops a value type from the stack, and applies the
// given instruction on it, and push the result onto a register of the given type.
func (c *amd64Compiler) emitSimpleConversion(convInstruction obj.As, destinationRegisterType generalPurposeRegisterType) error {
	// TODO: it is not necessary to place the origin on a register (i.e. ok to directly move the memory allocated value).
	origin := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(origin); err != nil {
		return err
	}

	dest, err := c.allocateRegister(destinationRegisterType)
	if err != nil {
		return err
	}

	convert := c.newProg()
	convert.As = convInstruction
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = origin.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = dest
	c.addInstruction(convert)

	c.locationStack.markRegisterUnused(origin.register)
	loc := c.locationStack.pushValueOnRegister(dest)
	loc.setRegisterType(destinationRegisterType)
	return nil
}

// compileF32DemoteFromF64 adds instructions to replace the 64-bit float on top of the stack
// with the corresponding 32-bit float. This is equivalent to float32(float64(v)) in Go.
func (c *amd64Compiler) compileF32DemoteFromF64() error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	convert := c.newProg()
	convert.As = x86.ACVTSD2SS
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = target.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = target.register
	c.addInstruction(convert)
	return nil
}

// compileF64PromoteFromF32 adds instructions to replace the 32-bit float on top of the stack
// with the corresponding 64-bit float. This is equivalent to float64(float32(v)) in Go.
func (c *amd64Compiler) compileF64PromoteFromF32() error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	convert := c.newProg()
	convert.As = x86.ACVTSS2SD
	convert.From.Type = obj.TYPE_REG
	convert.From.Reg = target.register
	convert.To.Type = obj.TYPE_REG
	convert.To.Reg = target.register
	c.addInstruction(convert)
	return nil
}

// compileI32ReinterpretFromF32 adds instructions to reinterpret the 32-bit float on top of the stack
// as a 32-bit integer by preserving the bit representation. If the value is on the stack,
// this is no-op as there is nothing to do for converting type.
func (c *amd64Compiler) compileI32ReinterpretFromF32() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		peek.setRegisterType(generalPurposeRegisterTypeInt)
		return nil
	}
	return c.emitSimpleConversion(x86.AMOVL, generalPurposeRegisterTypeInt)
}

// compileI64ReinterpretFromF64 adds instructions to reinterpret the 64-bit float on top of the stack
// as a 64-bit integer by preserving the bit representation. If the value is on the stack,
// this is no-op as there is nothing to do for converting type.
func (c *amd64Compiler) compileI64ReinterpretFromF64() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		peek.setRegisterType(generalPurposeRegisterTypeInt)
		return nil
	}
	return c.emitSimpleConversion(x86.AMOVQ, generalPurposeRegisterTypeInt)
}

// compileF32ReinterpretFromI32 adds instructions to reinterpret the 32-bit int on top of the stack
// as a 32-bit float by preserving the bit representation. If the value is on the stack,
// this is no-op as there is nothing to do for converting type.
func (c *amd64Compiler) compileF32ReinterpretFromI32() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		peek.setRegisterType(generalPurposeRegisterTypeFloat)
		return nil
	}
	return c.emitSimpleConversion(x86.AMOVL, generalPurposeRegisterTypeFloat)
}

// compileF64ReinterpretFromI64 adds instructions to reinterpret the 64-bit int on top of the stack
// as a 64-bit float by preserving the bit representation. If the value is on the stack,
// this is no-op as there is nothing to do for converting type.
func (c *amd64Compiler) compileF64ReinterpretFromI64() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		peek.setRegisterType(generalPurposeRegisterTypeFloat)
		return nil
	}
	return c.emitSimpleConversion(x86.AMOVQ, generalPurposeRegisterTypeFloat)
}

// compileExtend adds instructions to extend the 32-bit signed or unsigned int on top of the stack
// as a 64-bit integer of coressponding signedness. For unsigned case, this is just reinterpreting the
// underlying bit pattern as 64-bit integer. For signed case, this is sign-extension which preserves the
// original integer's sign.
func (c *amd64Compiler) compileExtend(o *wazeroir.OperationExtend) error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.ensureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	extend := c.newProg()
	if o.Signed {
		extend.As = x86.AMOVLQSX // = MOVSXD https://www.felixcloutier.com/x86/movsx:movsxd
	} else {
		extend.As = x86.AMOVQ
	}
	extend.From.Type = obj.TYPE_REG
	extend.From.Reg = target.register
	extend.To.Type = obj.TYPE_REG
	extend.To.Reg = target.register
	c.addInstruction(extend)
	return nil
}

func (c *amd64Compiler) compileEq(o *wazeroir.OperationEq) error {
	return c.emitEqOrNe(o.Type, true)
}

func (c *amd64Compiler) compileNe(o *wazeroir.OperationNe) error {
	return c.emitEqOrNe(o.Type, false)
}

func (c *amd64Compiler) emitEqOrNe(t wazeroir.UnsignedType, shouldEqual bool) (err error) {
	x2 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	switch t {
	case wazeroir.UnsignedTypeI32:
		err = c.emitEqOrNeForInts(x1.register, x2.register, x86.ACMPL, shouldEqual)
	case wazeroir.UnsignedTypeI64:
		err = c.emitEqOrNeForInts(x1.register, x2.register, x86.ACMPQ, shouldEqual)
	case wazeroir.UnsignedTypeF32:
		err = c.emitEqOrNeForFloats(x1.register, x2.register, x86.AUCOMISS, shouldEqual)
	case wazeroir.UnsignedTypeF64:
		err = c.emitEqOrNeForFloats(x1.register, x2.register, x86.AUCOMISD, shouldEqual)
	}
	if err != nil {
		return
	}

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)
	return
}

func (c *amd64Compiler) emitEqOrNeForInts(x1Reg, x2Reg int16, cmpInstruction obj.As, shouldEqual bool) error {
	cmp := c.newProg()
	cmp.As = cmpInstruction
	cmp.From.Type = obj.TYPE_REG
	cmp.From.Reg = x2Reg
	cmp.To.Type = obj.TYPE_REG
	cmp.To.Reg = x1Reg
	c.addInstruction(cmp)

	// Record that the result is on the conditional register.
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

// For float EQ and NE, we have to take NaN values into account.
// Notably, Wasm specification states that if one of targets is NaN,
// the result must be zero for EQ or one for NE.
func (c *amd64Compiler) emitEqOrNeForFloats(x1Reg, x2Reg int16, cmpInstruction obj.As, shouldEqual bool) error {
	// Before we allocate the result, we have to reserve two int registers.
	cmpResultReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	c.locationStack.markRegisterUsed(cmpResultReg)
	nanFragReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// Then, execute the comparison.
	cmp := c.newProg()
	cmp.As = cmpInstruction
	cmp.From.Type = obj.TYPE_REG
	cmp.From.Reg = x2Reg
	cmp.To.Type = obj.TYPE_REG
	cmp.To.Reg = x1Reg
	c.addInstruction(cmp)

	// First, we get the parity flag which indicates whether one of values was NaN.
	getNaNFlag := c.newProg()
	if shouldEqual {
		getNaNFlag.As = x86.ASETPC // Set 1 if two values are NOT NaN.
	} else {
		getNaNFlag.As = x86.ASETPS // Set 1 if one of values is NaN.
	}
	getNaNFlag.To.Type = obj.TYPE_REG
	getNaNFlag.To.Reg = nanFragReg
	c.addInstruction(getNaNFlag)

	// Next, we get the usual comparison flag.
	getCmpResult := c.newProg()
	if shouldEqual {
		getCmpResult.As = x86.ASETEQ // Set 1 if equal.
	} else {
		getCmpResult.As = x86.ASETNE // Set 1 if NOT equal.
	}
	getCmpResult.To.Type = obj.TYPE_REG
	getCmpResult.To.Reg = cmpResultReg
	c.addInstruction(getCmpResult)

	// Do "and" operations on these two flags to get the actual result.
	andCmpResultWithNaNFlag := c.newProg()
	if shouldEqual {
		andCmpResultWithNaNFlag.As = x86.AANDL
	} else {
		andCmpResultWithNaNFlag.As = x86.AORL
	}
	andCmpResultWithNaNFlag.To.Type = obj.TYPE_REG
	andCmpResultWithNaNFlag.To.Reg = cmpResultReg
	andCmpResultWithNaNFlag.From.Type = obj.TYPE_REG
	andCmpResultWithNaNFlag.From.Reg = nanFragReg
	c.addInstruction(andCmpResultWithNaNFlag)

	// Clear the unnecessary bits by zero extending the first byte.
	// This is necessary the upper bits (5 to 32 bits) of SET* instruction result is undefined.
	clearHigherBits := c.newProg()
	clearHigherBits.As = x86.AMOVBLZX
	clearHigherBits.To.Type = obj.TYPE_REG
	clearHigherBits.To.Reg = cmpResultReg
	clearHigherBits.From.Type = obj.TYPE_REG
	clearHigherBits.From.Reg = cmpResultReg
	c.addInstruction(clearHigherBits)

	// Now we have the result in cmpResultReg register, so we record it.
	loc := c.locationStack.pushValueOnRegister(cmpResultReg)
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
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.ACOMISS
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.ACOMISD
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	}
	c.addInstruction(prog)

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
		prog.As = x86.AUCOMISS
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateA
		prog.As = x86.AUCOMISD
		prog.From.Reg = x2.register
		prog.To.Reg = x1.register
	}
	c.addInstruction(prog)

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
		resultConditionState = conditionalRegisterStateAE
		prog.As = x86.AUCOMISS
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	case wazeroir.SignedTypeFloat64:
		resultConditionState = conditionalRegisterStateAE
		prog.As = x86.AUCOMISD
		prog.From.Reg = x1.register
		prog.To.Reg = x2.register
	}
	c.addInstruction(prog)

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

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileLoad(o *wazeroir.OperationLoad) error {
	var (
		isIntType        bool
		movInst          obj.As
		targetSizeInByte int64
	)
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		isIntType = true
		movInst = x86.AMOVL
		targetSizeInByte = 32 / 8
	case wazeroir.UnsignedTypeI64:
		isIntType = true
		movInst = x86.AMOVQ
		targetSizeInByte = 64 / 8
	case wazeroir.UnsignedTypeF32:
		isIntType = false
		movInst = x86.AMOVL
		targetSizeInByte = 32 / 8
	case wazeroir.UnsignedTypeF64:
		isIntType = false
		movInst = x86.AMOVQ
		targetSizeInByte = 64 / 8
	}

	reg, err := c.setupMemoryOffset(o.Arg.Offest, targetSizeInByte)
	if err != nil {
		return err
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
	reg, err := c.setupMemoryOffset(o.Arg.Offest, 1)
	if err != nil {
		return err
	}

	// Then move a byte at the offset to the register.
	// Note that Load8 is only for integer types.
	moveFromMemory := c.newProg()
	switch o.Type {
	case wazeroir.SignedInt32:
		moveFromMemory.As = x86.AMOVBLSX
	case wazeroir.SignedUint32:
		moveFromMemory.As = x86.AMOVBLZX
	case wazeroir.SignedInt64:
		moveFromMemory.As = x86.AMOVBQSX
	case wazeroir.SignedUint64:
		moveFromMemory.As = x86.AMOVBQZX
	}
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
	reg, err := c.setupMemoryOffset(o.Arg.Offest, 16/8)
	if err != nil {
		return err
	}

	// Then move 2 bytes at the offset to the register.
	// Note that Load16 is only for integer types.
	moveFromMemory := c.newProg()
	switch o.Type {
	case wazeroir.SignedInt32:
		moveFromMemory.As = x86.AMOVWLSX
	case wazeroir.SignedInt64:
		moveFromMemory.As = x86.AMOVWQSX
	case wazeroir.SignedUint32:
		moveFromMemory.As = x86.AMOVWLZX
	case wazeroir.SignedUint64:
		moveFromMemory.As = x86.AMOVWQZX
	}
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
	reg, err := c.setupMemoryOffset(o.Arg.Offest, 32/8)
	if err != nil {
		return err
	}

	// Then move 4 bytes at the offset to the register.
	moveFromMemory := c.newProg()
	if o.Signed {
		moveFromMemory.As = x86.AMOVLQSX
	} else {
		moveFromMemory.As = x86.AMOVLQZX
	}
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

// setupMemoryOffset pops the top value from the stack (called "base"), and returns the result of addition with
// base and offsetArg, which we call "offset". The returned offsetRegister is the register number with the offset calculation value.
// targetSizeInByte is the original memory operation's target size in byte. For example, 4 = 32 / 8 for Load32 operation.
// This is used for all Store* and Load* instructions.
//
// Note that this also emits the instructions to check the out of bounds memory access. That means
// if the base+offsetArg+targetSizeInByte exceeds the memory size, we exit this function with
// jitCallStatusCodeMemoryOutOfBounds status code since we read memory as [base+offsetArg: base+offsetArg+targetSizeInByte].
func (c *amd64Compiler) setupMemoryOffset(offsetArg uint32, targetSizeInByte int64) (offsetRegister int16, err error) {
	base := c.locationStack.pop()
	if err = c.ensureOnGeneralPurposeRegister(base); err != nil {
		return 0, err
	}

	// First, we calculate the offset on the memory region.
	addOffsetToBase := c.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = base.register
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(offsetArg)
	c.addInstruction(addOffsetToBase)

	// If the base+offset already overflows from uint32 range, we exit with the out of boundary status.
	overflowJmp := c.newProg()
	overflowJmp.As = x86.AJCS
	overflowJmp.To.Type = obj.TYPE_BRANCH
	c.addInstruction(overflowJmp)

	// Otherwise, we calculate base+offset+targetSizeInByte and check if it is within memory boundary.
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return 0, err
	}

	// Copy the 32-bit base+offset as to the temporary register as 64-bit integer.
	copyOffset := c.newProg()
	copyOffset.As = x86.AMOVLQZX // Zero extend
	copyOffset.To.Type = obj.TYPE_REG
	copyOffset.To.Reg = tmpReg
	copyOffset.From.Type = obj.TYPE_REG
	copyOffset.From.Reg = base.register
	c.addInstruction(copyOffset)

	// Adds targetSizeInByte to base+offset stored in the temporary register.
	addTargetSize := c.newProg()
	addTargetSize.As = x86.AADDQ
	addTargetSize.To.Type = obj.TYPE_REG
	addTargetSize.To.Reg = tmpReg
	addTargetSize.From.Type = obj.TYPE_CONST
	addTargetSize.From.Offset = targetSizeInByte
	c.addInstruction(addTargetSize)

	// Now we compare the value with the memory length which is held by engine.
	cmp := c.newProg()
	cmp.As = x86.ACMPQ
	cmp.To.Type = obj.TYPE_REG
	cmp.To.Reg = tmpReg
	cmp.From.Type = obj.TYPE_MEM
	cmp.From.Reg = reservedRegisterForEngine
	cmp.From.Offset = engineMemorySliceLenOffset
	c.addInstruction(cmp)

	// Jump if the value is within the memory length.
	okJmp := c.newProg()
	okJmp.As = x86.AJCC
	okJmp.To.Type = obj.TYPE_BRANCH
	c.addInstruction(okJmp)

	// Otherwise, we exit the function with out of bounds status code.
	c.addSetJmpOrigins(overflowJmp)
	c.setJITStatus(jitCallStatusCodeMemoryOutOfBounds)
	c.returnFunction()

	c.addSetJmpOrigins(okJmp)
	return base.register, nil
}

func (c *amd64Compiler) compileStore(o *wazeroir.OperationStore) error {
	var movInst obj.As
	var targetSizeInByte int64
	switch o.Type {
	case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
		movInst = x86.AMOVL
		targetSizeInByte = 32 / 8
	case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
		movInst = x86.AMOVQ
		targetSizeInByte = 64 / 8
	}
	return c.moveToMemory(o.Arg.Offest, movInst, targetSizeInByte)
}

func (c *amd64Compiler) compileStore8(o *wazeroir.OperationStore8) error {
	return c.moveToMemory(o.Arg.Offest, x86.AMOVB, 1)
}

func (c *amd64Compiler) compileStore16(o *wazeroir.OperationStore16) error {
	return c.moveToMemory(o.Arg.Offest, x86.AMOVW, 16/8)
}

func (c *amd64Compiler) compileStore32(o *wazeroir.OperationStore32) error {
	return c.moveToMemory(o.Arg.Offest, x86.AMOVL, 32/8)
}

func (c *amd64Compiler) moveToMemory(offsetConst uint32, moveInstruction obj.As, targetSizeInByte int64) error {
	val := c.locationStack.pop()
	if err := c.ensureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	reg, err := c.setupMemoryOffset(offsetConst, targetSizeInByte)
	if err != nil {
		return nil
	}

	moveToMemory := c.newProg()
	moveToMemory.As = moveInstruction
	moveToMemory.From.Type = obj.TYPE_REG
	moveToMemory.From.Reg = val.register
	moveToMemory.To.Type = obj.TYPE_MEM
	moveToMemory.To.Reg = reservedRegisterForMemory
	moveToMemory.To.Index = reg
	moveToMemory.To.Scale = 1
	c.addInstruction(moveToMemory)

	// We no longer need both the value and base registers.
	c.locationStack.releaseRegister(val)
	c.locationStack.markRegisterUnused(reg)
	return nil
}

func (c *amd64Compiler) compileMemoryGrow() error {
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}
	return c.compileFunctionCallFromAddress(jitCallStatusCodeCallBuiltInFunction, builtinFunctionAddressMemoryGrow)
}

func (c *amd64Compiler) compileMemorySize() error {
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}
	if err := c.compileFunctionCallFromAddress(jitCallStatusCodeCallBuiltInFunction, builtinFunctionAddressMemorySize); err != nil {
		return err
	}
	loc := c.locationStack.pushValueOnStack() // The size is pushed on the top.
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileConstI32(o *wazeroir.OperationConstI32) error {
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

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
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

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
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

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
	// If the top value is conditional one, we must save it before executing the following instructions
	// as they clear the conditional flag, meaning that the conditional value might change.
	if err := c.maybeMoveTopConditionalToFreeGeneralPurposeRegister(); err != nil {
		return err
	}

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

// maybeMoveTopConditionalToFreeGeneralPurposeRegister moves the top value on the stack
// if the value is located on a conditional register. This is usually called at the beginning of
// amd64Compiler.compile* functions where we possibly emit istructions without saving the conditional
// register value. The compile* functions without calling this function is saving the conditional
// value to the stack or register by invoking ensureOnGeneralPurposeRegister for the top.
func (c *amd64Compiler) maybeMoveTopConditionalToFreeGeneralPurposeRegister() (err error) {
	if c.locationStack.sp > 0 {
		if loc := c.locationStack.peek(); loc.onConditionalRegister() {
			err = c.moveConditionalToFreeGeneralPurposeRegister(loc)
		}
	}
	return
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
	loc.setRegisterType(generalPurposeRegisterTypeInt)
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

// compileFunctionCallFromAddress adds instructions to call a function whose address equals the addr parameter.
// jitStatus is set before making call, and it should either jitCallStatusCodeCallBuiltInFunction or
// jitCallStatusCodeCallFunction.
func (c *amd64Compiler) compileFunctionCallFromAddress(jitStatus jitCallStatusCode, addr wasm.FunctionAddress) error {
	c.setJITStatus(jitStatus)

	prog := c.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(addr)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineFunctionCallAddressOffset
	c.addInstruction(prog)

	// Release all the registers as our calling convention requires the callee-save.
	if err := c.releaseAllRegistersToStack(); err != nil {
		return err
	}
	// Set the continuation offset on the next instruction.
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the function call returns, we must re-initialize the reserved registers.
	c.initializeReservedRegisters()
	return nil
}

// compileFunctionCallFromRegister adds instructions to call a function whose address equals the value on
// the functionCallAddressRegister.
func (c *amd64Compiler) compileFunctionCallFromRegister(functionCallAddressRegister int16) error {
	c.setJITStatus(jitCallStatusCodeCallFunction)

	setFunctionAddressFromReg := c.newProg()
	setFunctionAddressFromReg.As = x86.AMOVQ
	setFunctionAddressFromReg.From.Type = obj.TYPE_REG
	setFunctionAddressFromReg.From.Reg = functionCallAddressRegister
	setFunctionAddressFromReg.To.Type = obj.TYPE_MEM
	setFunctionAddressFromReg.To.Reg = reservedRegisterForEngine
	setFunctionAddressFromReg.To.Offset = engineFunctionCallAddressOffset
	c.addInstruction(setFunctionAddressFromReg)

	// Release all the registers as our calling convention requires the callee-save.
	if err := c.releaseAllRegistersToStack(); err != nil {
		return err
	}
	// Set the continuation offset on the next instruction.
	c.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the function call returns, we must re-initialize the reserved registers.
	c.initializeReservedRegisters()
	return nil
}

func (c *amd64Compiler) releaseAllRegistersToStack() error {
	used := len(c.locationStack.usedRegisters)
	for i := uint64(0); i < c.locationStack.sp && used > 0; i++ {
		if loc := c.locationStack.stack[i]; loc.onRegister() {
			c.releaseRegisterToStack(loc)
			used--
		} else if loc.onConditionalRegister() {
			if err := c.moveConditionalToFreeGeneralPurposeRegister(loc); err != nil {
				return err
			}
			c.releaseRegisterToStack(loc)
			used--
		}
	}
	return nil
}

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
	prog.To.Offset = engineFunctionCallAddressOffset
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
