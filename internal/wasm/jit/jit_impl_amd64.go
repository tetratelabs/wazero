package jit

// This file implements the compiler for amd64/x86_64 target.
// Please refer to https://www.felixcloutier.com/x86/index.html
// if unfamiliar with amd64 instructions used here.
// Note that x86 pkg used here prefixes all the instructions with "A"
// e.g. MOVQ will be given as amd64.MOVQ.

import (
	"fmt"
	"math"
	"runtime"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/asm"
	amd64 "github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/buildoptions"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

var (
	zero64Bit                                     uint64 = 0
	zero64BitAddress                              uintptr
	minimum32BitSignedInt                         int32 = math.MinInt32
	minimum32BitSignedIntAddress                  uintptr
	minimum64BitSignedInt                         int64 = math.MinInt64
	minimum64BitSignedIntAddress                  uintptr
	float32SignBitMask                            uint32 = 1 << 31
	float32RestBitMask                            uint32 = ^float32SignBitMask
	float32SignBitMaskAddress                     uintptr
	float32RestBitMaskAddress                     uintptr
	float64SignBitMask                            uint64 = 1 << 63
	float64RestBitMask                            uint64 = ^float64SignBitMask
	float64SignBitMaskAddress                     uintptr
	float64RestBitMaskAddress                     uintptr
	float32ForMinimumSigned32bitInteger           float32 = math.Float32frombits(0xCF00_0000)
	float32ForMinimumSigned32bitIntegerAddress    uintptr
	float64ForMinimumSigned32bitInteger           float64 = math.Float64frombits(0xC1E0_0000_0020_0000)
	float64ForMinimumSigned32bitIntegerAddress    uintptr
	float32ForMinimumSigned64bitInteger           float32 = math.Float32frombits(0xDF00_0000)
	float32ForMinimumSigned64bitIntegerAddress    uintptr
	float64ForMinimumSigned64bitInteger           float64 = math.Float64frombits(0xC3E0_0000_0000_0000)
	float64ForMinimumSigned64bitIntegerAddress    uintptr
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
	// TODO: what if these address exceed 32-bit address space?  Even though AMD says 2GB memory space
	// should be enough for everyone, we might end up in these circum stances. We access these variables
	// via 32-bit displacement which cannot accomodate 64-bit addresses.
	// https://stackoverflow.com/questions/31853189/x86-64-assembly-why-displacement-not-64-bits
	zero64BitAddress = uintptr(unsafe.Pointer(&zero64Bit))
	minimum32BitSignedIntAddress = uintptr(unsafe.Pointer(&minimum32BitSignedInt))
	minimum64BitSignedIntAddress = uintptr(unsafe.Pointer(&minimum64BitSignedInt))
	float32SignBitMaskAddress = uintptr(unsafe.Pointer(&float32SignBitMask))
	float32RestBitMaskAddress = uintptr(unsafe.Pointer(&float32RestBitMask))
	float64SignBitMaskAddress = uintptr(unsafe.Pointer(&float64SignBitMask))
	float64RestBitMaskAddress = uintptr(unsafe.Pointer(&float64RestBitMask))
	float32ForMinimumSigned32bitIntegerAddress = uintptr(unsafe.Pointer(&float32ForMinimumSigned32bitInteger))
	float64ForMinimumSigned32bitIntegerAddress = uintptr(unsafe.Pointer(&float64ForMinimumSigned32bitInteger))
	float32ForMinimumSigned64bitIntegerAddress = uintptr(unsafe.Pointer(&float32ForMinimumSigned64bitInteger))
	float64ForMinimumSigned64bitIntegerAddress = uintptr(unsafe.Pointer(&float64ForMinimumSigned64bitInteger))
	float32ForMaximumSigned32bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float32ForMaximumSigned32bitIntPlusOne))
	float64ForMaximumSigned32bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float64ForMaximumSigned32bitIntPlusOne))
	float32ForMaximumSigned64bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float32ForMaximumSigned64bitIntPlusOne))
	float64ForMaximumSigned64bitIntPlusOneAddress = uintptr(unsafe.Pointer(&float64ForMaximumSigned64bitIntPlusOne))
}

var (
	// amd64ReservedRegisterForCallEngine: pointer to callEngine (i.e. *callEngine as uintptr)
	amd64ReservedRegisterForCallEngine = amd64.REG_R13
	// amd64ReservedRegisterForStackBasePointerAddress: stack base pointer's address (callEngine.stackBasePointer) in the current function call.
	amd64ReservedRegisterForStackBasePointerAddress = amd64.REG_R14
	// amd64ReservedRegisterForMemory: pointer to the memory slice's data (i.e. &memory.Buffer[0] as uintptr).
	amd64ReservedRegisterForMemory = amd64.REG_R15
)

var (
	amd64UnreservedGeneralPurposeFloatRegisters = []asm.Register{ // nolint
		amd64.REG_X0, amd64.REG_X1, amd64.REG_X2, amd64.REG_X3,
		amd64.REG_X4, amd64.REG_X5, amd64.REG_X6, amd64.REG_X7,
		amd64.REG_X8, amd64.REG_X9, amd64.REG_X10, amd64.REG_X11,
		amd64.REG_X12, amd64.REG_X13, amd64.REG_X14, amd64.REG_X15,
	}
	// Note that we never invoke "call" instruction,
	// so we don't need to care about the calling convention.
	// TODO: Maybe it is safe just save rbp, rsp somewhere
	// in Go-allocated variables, and reuse these registers
	// in JITed functions and write them back before returns.
	amd64UnreservedGeneralPurposeIntRegisters = []asm.Register{ // nolint
		amd64.REG_AX, amd64.REG_CX, amd64.REG_DX, amd64.REG_BX,
		amd64.REG_SI, amd64.REG_DI, amd64.REG_R8, amd64.REG_R9,
		amd64.REG_R10, amd64.REG_R11, amd64.REG_R12,
	}
)

func (c *amd64Compiler) String() string {
	return c.locationStack.String()
}

type amd64Compiler struct {
	assembler amd64.Assembler
	f         *wasm.FunctionInstance
	ir        *wazeroir.CompilationResult
	// locationStack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
	// labels hold per wazeroir label specific information in this function.
	labels map[string]*amd64LabelInfo
	// stackPointerCeil is the greatest stack pointer value (from valueLocationStack) seen during compilation.
	stackPointerCeil uint64
	// currentLabel holds a currently compiled wazeroir label key. For debugging only.
	currentLabel string
	// onStackPointerCeilDeterminedCallBack hold a callback which are called when the max stack pointer is determined BEFORE generating native code.
	onStackPointerCeilDeterminedCallBack func(stackPointerCeil uint64)
	staticData                           compiledFunctionStaticData
}

func newAmd64Compiler(f *wasm.FunctionInstance, ir *wazeroir.CompilationResult) (compiler, error) {
	c := &amd64Compiler{
		f:             f,
		assembler:     amd64.NewAssemblerImpl(),
		locationStack: newValueLocationStack(),
		currentLabel:  wazeroir.EntrypointLabel,
		ir:            ir,
		labels:        map[string]*amd64LabelInfo{},
	}
	return c, nil
}

// setLocationStack sets the given valueLocationStack to .locationStack field,
// while allowing us to track valueLocationStack.stackPointerCeil across multiple stacks.
// This is called when we branch into different block.
func (c *amd64Compiler) setLocationStack(newStack *valueLocationStack) {
	if c.stackPointerCeil < c.locationStack.stackPointerCeil {
		c.stackPointerCeil = c.locationStack.stackPointerCeil
	}
	c.locationStack = newStack
}

func (c *amd64Compiler) addStaticData(d []byte) {
	c.staticData = append(c.staticData, d)
}

func (c *amd64Compiler) pushValueLocationOnRegister(reg asm.Register) (ret *valueLocation) {
	ret = c.locationStack.pushValueLocationOnRegister(reg)
	c.locationStack.markRegisterUsed(reg)
	return
}

type amd64LabelInfo struct {
	// initialInstruction is the initial instruction for this label so other block can jump into it.
	initialInstruction asm.Node
	// initialStack is the initial value location stack from which we start compiling this label.
	initialStack *valueLocationStack
	// labelBeginningCallbacks holds callbacks should to be called with initialInstruction
	labelBeginningCallbacks []func(asm.Node)
}

func (c *amd64Compiler) label(labelKey string) *amd64LabelInfo {
	ret, ok := c.labels[labelKey]
	if ok {
		return ret
	}
	c.labels[labelKey] = &amd64LabelInfo{}
	return c.labels[labelKey]
}

// compileHostFunction constructs the entire code to enter the host function implementation,
// and return back to the caller.
func (c *amd64Compiler) compileHostFunction() error {
	// First we must update the location stack to reflect the number of host function inputs.
	c.pushFunctionParams()

	if err := c.compileCallHostFunction(); err != nil {
		return err
	}

	return c.compileReturnFunction()
}

// compile implements compiler.compile for the amd64 architecture.
func (c *amd64Compiler) compile() (code []byte, staticData compiledFunctionStaticData, stackPointerCeil uint64, err error) {
	// c.stackPointerCeil tracks the stack pointer ceiling (max seen) value across all valueLocationStack(s)
	// used for all labels (via setLocationStack), excluding the current one.
	// Hence, we check here if the final block's max one exceeds the current c.stackPointerCeil.
	stackPointerCeil = c.stackPointerCeil
	if stackPointerCeil < c.locationStack.stackPointerCeil {
		stackPointerCeil = c.locationStack.stackPointerCeil
	}

	// Now that the max stack pointer is determined, we are invoking the callback.
	// Note this MUST be called before Assemble() below.
	if c.onStackPointerCeilDeterminedCallBack != nil {
		c.onStackPointerCeilDeterminedCallBack(stackPointerCeil)
		c.onStackPointerCeilDeterminedCallBack = nil
	}

	code, err = c.assembler.Assemble()
	if err != nil {
		return
	}

	code, err = mmapCodeSegment(code)
	if err != nil {
		return
	}

	staticData = c.staticData
	return
}

func (c *amd64Compiler) pushFunctionParams() {
	if c.f != nil && c.f.Type != nil {
		for _, t := range c.f.Type.Params {
			loc := c.locationStack.pushValueLocationOnStack()
			switch t {
			case wasm.ValueTypeI32, wasm.ValueTypeI64:
				loc.setRegisterType(generalPurposeRegisterTypeInt)
			case wasm.ValueTypeF32, wasm.ValueTypeF64:
				loc.setRegisterType(generalPurposeRegisterTypeFloat)
			}
		}
	}
}

// compileUnreachable implements compiler.compileUnreachable for the amd64 architecture.
func (c *amd64Compiler) compileUnreachable() error {
	c.compileExitFromNativeCode(jitCallStatusCodeUnreachable)
	return nil
}

// compileUnreachable implements compiler.compileUnreachable for the amd64 architecture.
func (c *amd64Compiler) compileSwap(o *wazeroir.OperationSwap) error {
	index := int(c.locationStack.sp) - 1 - o.Depth
	// Note that, in theory, the register types and value types
	// are the same between these swap targets as swap operations
	// are generated from local.set,tee instructions in Wasm.
	x1 := c.locationStack.peek()
	x2 := c.locationStack.stack[index]

	// If x1 is on the conditional register, we must move it to a gp
	// register before swap.
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	if x1.onRegister() && x2.onRegister() {
		x1.register, x2.register = x2.register, x1.register
	} else if x1.onRegister() && x2.onStack() {
		reg := x1.register
		c.locationStack.markRegisterUnused(reg)
		// Save x1's value to the temporary top of the stack.
		tmpStackLocation := c.pushValueLocationOnRegister(reg)
		c.compileReleaseRegisterToStack(tmpStackLocation)
		// Then move the x2's value to the x1's register location.
		x2.register = reg
		c.compileLoadValueOnStackToRegister(x2)
		// Now move the x1's value to the x1's stack location.
		c.compileReleaseRegisterToStack(x1)
		// Next we move the saved x1's value to the register.
		tmpStackLocation.setRegister(reg)
		c.compileLoadValueOnStackToRegister(tmpStackLocation)
		// Finally move the x1's value in the register to the x2's stack location.
		c.locationStack.releaseRegister(x1)
		c.locationStack.releaseRegister(tmpStackLocation)
		x2.setRegister(reg)
		c.locationStack.markRegisterUsed(reg)
		_ = c.locationStack.pop() // Delete tmpStackLocation.
	} else if x1.onStack() && x2.onRegister() {
		reg := x2.register
		c.locationStack.markRegisterUnused(reg)
		// Save x2's value to the temporary top of the stack.
		tmpStackLocation := c.pushValueLocationOnRegister(reg)
		c.compileReleaseRegisterToStack(tmpStackLocation)
		// Then move the x1's value to the x2's register location.
		x1.register = reg
		c.compileLoadValueOnStackToRegister(x1)
		// Now move the x1's value to the x2's stack location.
		c.compileReleaseRegisterToStack(x2)
		// Next we move the saved x2's value to the register.
		tmpStackLocation.setRegister(reg)
		c.compileLoadValueOnStackToRegister(tmpStackLocation)
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
		c.compileLoadValueOnStackToRegister(x2)
		// Save x2's value to the temporary top of the stack.
		tmpStackLocation := c.pushValueLocationOnRegister(reg)
		c.compileReleaseRegisterToStack(tmpStackLocation)
		// Then move the x1's value to the x2's register location.
		x1.register = reg
		c.compileLoadValueOnStackToRegister(x1)
		// Now move the x1's value to the x2's stack location.
		c.compileReleaseRegisterToStack(x2)
		// Next we move the saved x2's value to the register.
		tmpStackLocation.setRegister(reg)
		c.compileLoadValueOnStackToRegister(tmpStackLocation)
		// Finally move the x2's value in the register to the x2's stack location.
		c.locationStack.releaseRegister(x2)
		c.locationStack.releaseRegister(tmpStackLocation)
		x1.setRegister(reg)
		c.locationStack.markRegisterUsed(reg)
		_ = c.locationStack.pop() // Delete tmpStackLocation.
	}
	return nil
}

// compileGlobalGet implements compiler.compileGlobalGet for the amd64 architecture.
func (c *amd64Compiler) compileGlobalGet(o *wazeroir.OperationGlobalGet) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	intReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, move the pointer to the global slice into the allocated register.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForCallEngine, callEngineModuleContextGlobalElement0AddressOffset, intReg)

	// Then, get the memory location of the target global instance's pointer.
	c.assembler.CompileConstToRegister(amd64.ADDQ, 8*int64(o.Index), intReg)

	// Now, move the location of the global instance into the register.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, intReg, 0, intReg)

	// When an integer, reuse the pointer register for the value. Otherwise, allocate a float register for it.
	valueReg := intReg
	wasmType := c.f.Module.Globals[o.Index].Type.ValType
	switch wasmType {
	case wasm.ValueTypeF32, wasm.ValueTypeF64:
		valueReg, err = c.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
	}

	// Using the register holding the pointer to the target instance, move its value into a register.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, intReg, globalInstanceValueOffset, valueReg)

	// Record that the retrieved global value on the top of the stack is now in a register.
	loc := c.pushValueLocationOnRegister(valueReg)
	switch wasmType {
	case wasm.ValueTypeI32, wasm.ValueTypeI64:
		loc.setRegisterType(generalPurposeRegisterTypeInt)
	case wasm.ValueTypeF32, wasm.ValueTypeF64:
		loc.setRegisterType(generalPurposeRegisterTypeFloat)
	}
	return nil
}

// compileGlobalSet implements compiler.compileGlobalSet for the amd64 architecture.
func (c *amd64Compiler) compileGlobalSet(o *wazeroir.OperationGlobalSet) error {
	// First, move the value to set into a temporary register.
	val := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	// Allocate a register to hold the memory location of the target global instance.
	intReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, move the pointer to the global slice into the allocated register.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForCallEngine, callEngineModuleContextGlobalElement0AddressOffset, intReg)

	// Then, get the memory location of the target global instance's pointer.
	c.assembler.CompileConstToRegister(amd64.ADDQ, 8*int64(o.Index), intReg)

	// Now, move the location of the global instance into the register.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, intReg, 0, intReg)

	// Now ready to write the value to the global instance location.
	c.assembler.CompileRegisterToMemory(amd64.MOVQ, val.register, intReg, globalInstanceValueOffset)

	// Since the value is now written to memory, release the value register.
	c.locationStack.releaseRegister(val)
	return nil
}

// compileBr implements compiler.compileBr for the amd64 architecture.
func (c *amd64Compiler) compileBr(o *wazeroir.OperationBr) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()
	return c.branchInto(o.Target)
}

// branchInto adds instruction necessary to jump into the given branch target.
func (c *amd64Compiler) branchInto(target *wazeroir.BranchTarget) error {
	if target.IsReturnTarget() {
		return c.compileReturnFunction()
	} else {
		labelKey := target.String()
		if c.ir.LabelCallers[labelKey] > 1 {
			// We can only re-use register state if when there's a single call-site.
			// Release existing values on registers to the stack if there's multiple ones to have
			// the consistent value location state at the beginning of label.
			c.compileReleaseAllRegistersToStack()
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		targetLabel := c.label(labelKey)
		if targetLabel.initialStack == nil {
			// It seems unnecessary to clone as branchInto is always the tail of the current block.
			// TODO: verify ^^.
			targetLabel.initialStack = c.locationStack.clone()
		}
		jmp := c.assembler.CompileJump(amd64.JMP)
		c.assignJumpTarget(labelKey, jmp)
	}
	return nil
}

// compileBrIf implements compiler.compileBrIf for the amd64 architecture.
func (c *amd64Compiler) compileBrIf(o *wazeroir.OperationBrIf) error {
	cond := c.locationStack.pop()
	var jmpWithCond asm.Node
	if cond.onConditionalRegister() {
		var inst asm.Instruction
		switch cond.conditionalRegister {
		case amd64.ConditionalRegisterStateE:
			inst = amd64.JEQ
		case amd64.ConditionalRegisterStateNE:
			inst = amd64.JNE
		case amd64.ConditionalRegisterStateS:
			inst = amd64.JMI
		case amd64.ConditionalRegisterStateNS:
			inst = amd64.JPL
		case amd64.ConditionalRegisterStateG:
			inst = amd64.JGT
		case amd64.ConditionalRegisterStateGE:
			inst = amd64.JGE
		case amd64.ConditionalRegisterStateL:
			inst = amd64.JLT
		case amd64.ConditionalRegisterStateLE:
			inst = amd64.JLE
		case amd64.ConditionalRegisterStateA:
			inst = amd64.JHI
		case amd64.ConditionalRegisterStateAE:
			inst = amd64.JCC
		case amd64.ConditionalRegisterStateB:
			inst = amd64.JCS
		case amd64.ConditionalRegisterStateBE:
			inst = amd64.JLS
		}
		jmpWithCond = c.assembler.CompileJump(inst)
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
		if err := c.compileEnsureOnGeneralPurposeRegister(cond); err != nil {
			return err
		}
		// Check if the value not equals zero.
		c.assembler.CompileRegisterToConst(amd64.CMPQ, cond.register, 0)

		// Emit jump instruction which jumps when the value does not equals zero.
		jmpWithCond = c.assembler.CompileJump(amd64.JNE)
		c.locationStack.markRegisterUnused(cond.register)
	}

	// Make sure that the next coming label is the else jump target.
	thenTarget, elseTarget := o.Then, o.Else

	// Here's the diagram of how we organize the instructions necessarily for brif operation.
	//
	// jmp_with_cond -> jmp (.Else) -> Then operations...
	//    |---------(satisfied)------------^^^
	//
	// Note that .Else branch doesn't have ToDrop as .Else is in reality
	// corresponding to either If's Else block or Br_if's else block in Wasm.

	// Emit for else branches
	saved := c.locationStack
	c.setLocationStack(saved.clone())
	if elseTarget.Target.IsReturnTarget() {
		if err := c.compileReturnFunction(); err != nil {
			return err
		}
	} else {
		elseLabelKey := elseTarget.Target.Label.String()
		if c.ir.LabelCallers[elseLabelKey] > 1 {
			// We can only re-use register state if when there's a single call-site.
			// Release existing values on registers to the stack if there's multiple ones to have
			// the consistent value location state at the beginning of label.
			c.compileReleaseAllRegistersToStack()
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		amd64LabelInfo := c.label(elseLabelKey)
		if amd64LabelInfo.initialStack == nil {
			amd64LabelInfo.initialStack = c.locationStack
		}

		elseJmp := c.assembler.CompileJump(amd64.JMP)
		c.assignJumpTarget(elseLabelKey, elseJmp)
	}

	// Handle then branch.
	c.assembler.SetJumpTargetOnNext(jmpWithCond)
	c.setLocationStack(saved)
	if err := c.emitDropRange(thenTarget.ToDrop); err != nil {
		return err
	}
	if thenTarget.Target.IsReturnTarget() {
		return c.compileReturnFunction()
	} else {
		thenLabelKey := thenTarget.Target.Label.String()
		if c.ir.LabelCallers[thenLabelKey] > 1 {
			// We can only re-use register state if when there's a single call-site.
			// Release existing values on registers to the stack if there's multiple ones to have
			// the consistent value location state at the beginning of label.
			c.compileReleaseAllRegistersToStack()
		}
		// Set the initial stack of the target label, so we can start compiling the label
		// with the appropriate value locations. Note we clone the stack here as we maybe
		// manipulate the stack before compiler reaches the label.
		amd64LabelInfo := c.label(thenLabelKey)
		if amd64LabelInfo.initialStack == nil {
			amd64LabelInfo.initialStack = c.locationStack
		}
		thenJmp := c.assembler.CompileJump(amd64.JMP)
		c.assignJumpTarget(thenLabelKey, thenJmp)
		return nil
	}
}

// compileBrTable implements compiler.compileBrTable for the amd64 architecture.
func (c *amd64Compiler) compileBrTable(o *wazeroir.OperationBrTable) error {
	index := c.locationStack.pop()

	// If the operation only consists of the default target, we branch into it and return early.
	if len(o.Targets) == 0 {
		c.locationStack.releaseRegister(index)
		if err := c.emitDropRange(o.Default.ToDrop); err != nil {
			return err
		}
		return c.branchInto(o.Default.Target)
	}

	// Otherwise, we jump into the selected branch.
	if err := c.compileEnsureOnGeneralPurposeRegister(index); err != nil {
		return err
	}

	tmp, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we move the length of target list into the tmp register.
	c.assembler.CompileConstToRegister(amd64.MOVQ, int64(len(o.Targets)), tmp)

	// Then, we compare the value with the length of targets.
	c.assembler.CompileRegisterToRegister(amd64.CMPL, tmp, index.register)

	// If the value is larger than the length,
	// we round the index to the length as the spec states that
	// if the index is larger than or equal the length of list,
	// branch into the default branch.
	c.assembler.CompileRegisterToRegister(amd64.CMOVQCS, tmp, index.register)

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
	// Note: We store each offset of 32-bite unsigned integer as 4 consecutive bytes. So more precisely,
	// the above example's offsetData would be [0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x0, 0x0, 0x8, 0x0, 0x0, 0x0].
	//
	// Note: this is similar to how GCC implements Switch statements in C.
	offsetData := make([]byte, 4*(len(o.Targets)+1))
	c.addStaticData(offsetData)

	c.assembler.CompileConstToRegister(amd64.MOVQ, int64(uintptr(unsafe.Pointer(&offsetData[0]))), tmp)

	// Now we have the address of first byte of offsetData in tmp register.
	// So the target offset's first byte is at tmp+index*4 as we store
	// the offset as 4 bytes for a 32-byte integer.
	// Here, we store the offset into the index.register.
	c.assembler.CompileMemoryWithIndexToRegister(amd64.MOVL, tmp, 0, index.register, 4, index.register)

	// Now we read the address of the beginning of the jump table.
	// In the above example, this corresponds to reading the address of 0x123001.
	c.assembler.CompileReadInstructionAddress(tmp, amd64.JMP)

	// Now we have the address of L0 in tmp register, and the offset to the target label in the index.register.
	// So we could achieve the br_table jump by adding them and jump into the resulting address.
	c.assembler.CompileRegisterToRegister(amd64.ADDQ, index.register, tmp)

	c.assembler.CompileJumpToRegister(amd64.JMP, tmp)

	// We no longer need the index's register, so mark it unused.
	c.locationStack.markRegisterUnused(index.register)

	// [Emit the code for each targets and default branch]
	labelInitialInstructions := make([]asm.Node, len(o.Targets)+1)
	saved := c.locationStack
	for i := range labelInitialInstructions {
		// Emit the initial instruction of each target.
		// We use NOP as we don't yet know the next instruction in each label.
		// Assembler would optimize out this NOP during code generation, so this is harmless.
		labelInitialInstructions[i] = c.assembler.CompileStandAlone(amd64.NOP)

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
		if err := c.emitDropRange(target.ToDrop); err != nil {
			return err
		}
		if err := c.branchInto(target.Target); err != nil {
			return err
		}
	}

	c.assembler.BuildJumpTable(offsetData, labelInitialInstructions)
	return nil
}

func (c *amd64Compiler) assignJumpTarget(labelKey string, jmpInstruction asm.Node) {
	jmpTargetLabel := c.label(labelKey)
	if jmpTargetLabel.initialInstruction != nil {
		jmpInstruction.AssignJumpTarget(jmpTargetLabel.initialInstruction)
	} else {
		jmpTargetLabel.labelBeginningCallbacks = append(jmpTargetLabel.labelBeginningCallbacks, func(labelInitialInstruction asm.Node) {
			jmpInstruction.AssignJumpTarget(labelInitialInstruction)
		})
	}
}

// compileLabel implements compiler.compileLabel for the amd64 architecture.
func (c *amd64Compiler) compileLabel(o *wazeroir.OperationLabel) (skipLabel bool) {
	if buildoptions.IsDebugMode {
		fmt.Printf("[label %s ends]\n\n", c.currentLabel)
	}

	labelKey := o.Label.String()
	amd64LabelInfo := c.label(labelKey)

	// If initialStack is not set, that means this label has never been reached.
	if amd64LabelInfo.initialStack == nil {
		skipLabel = true
		c.currentLabel = ""
		return
	}

	// We use NOP as a beginning of instructions in a label.
	labelBegin := c.assembler.CompileStandAlone(amd64.NOP)

	// Save the instructions so that backward branching
	// instructions can jump to this label.
	amd64LabelInfo.initialInstruction = labelBegin

	// Set the initial stack.
	c.setLocationStack(amd64LabelInfo.initialStack)

	// Invoke callbacks to notify the forward branching
	// instructions can properly jump to this label.
	for _, cb := range amd64LabelInfo.labelBeginningCallbacks {
		cb(labelBegin)
	}

	// Clear for debugging purpose. See the comment in "len(amd64LabelInfo.labelBeginningCallbacks) > 0" block above.
	amd64LabelInfo.labelBeginningCallbacks = nil

	if buildoptions.IsDebugMode {
		fmt.Printf("[label %s (num callers=%d)]\n%s\n", labelKey, c.ir.LabelCallers[labelKey], c.locationStack)
	}
	c.currentLabel = labelKey
	return
}

// compileCall implements compiler.compileCall for the amd64 architecture.
func (c *amd64Compiler) compileCall(o *wazeroir.OperationCall) error {
	target := c.f.Module.Functions[o.FunctionIndex]
	if err := c.compileCallFunctionImpl(o.FunctionIndex, asm.NilRegister, target.Type); err != nil {
		return err
	}

	// We consumed the function parameters from the stack after call.
	for i := 0; i < len(target.Type.Params); i++ {
		c.locationStack.pop()
	}

	// Also, the function results were pushed by the call.
	for _, t := range target.Type.Results {
		loc := c.locationStack.pushValueLocationOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
	}
	return nil
}

// compileCallIndirect implements compiler.compileCallIndirect for the amd64 architecture.
func (c *amd64Compiler) compileCallIndirect(o *wazeroir.OperationCallIndirect) error {
	offset := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(offset); err != nil {
		return nil
	}

	tmp, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we need to check if the offset doesn't exceed the length of table.
	c.assembler.CompileMemoryToRegister(amd64.CMPQ, amd64ReservedRegisterForCallEngine, callEngineModuleContextTableSliceLenOffset, offset.register)
	notLengthExceedJump := c.assembler.CompileJump(amd64.JHI)

	// If it exceeds, we return the function with jitCallStatusCodeInvalidTableAccess.
	c.compileExitFromNativeCode(jitCallStatusCodeInvalidTableAccess)
	c.assembler.SetJumpTargetOnNext(notLengthExceedJump)

	// Next we check if the target's type matches the operation's one.
	// In order to get the type instance's address, we have to multiply the offset
	// by 16 as the offset is the "length" of table in Go's "[]interface{}",
	// and size of interface{} equals 16 bytes == (2^4).
	c.assembler.CompileConstToRegister(amd64.SHLQ, 4, offset.register)

	// Adds the address of wasm.Table[0] stored as callEngine.tableElement0Address to the offset.
	c.assembler.CompileMemoryToRegister(amd64.ADDQ,
		amd64ReservedRegisterForCallEngine, callEngineModuleContextTableElement0AddressOffset, offset.register)

	// "offset = (*offset) + interfaceDataOffset (== table[offset] + interfaceDataOffset == *compiledFunction type)"
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, offset.register, interfaceDataOffset, offset.register)

	// At this point offset.register holds the address of *compiledFunction (as uintptr) at wasm.Table[offset].
	//
	// Check if the value of table[offset] equals zero, meaning that the target is uninitialized.
	c.assembler.CompileRegisterToConst(amd64.CMPQ, offset.register, 0)

	// Jump if the target is initialized element.
	jumpIfInitialized := c.assembler.CompileJump(amd64.JNE)

	// If not initialized, we return the function with jitCallStatusCodeInvalidTableAccess.
	c.compileExitFromNativeCode(jitCallStatusCodeInvalidTableAccess)

	c.assembler.SetJumpTargetOnNext(jumpIfInitialized)

	// Next we need to check the type matches, i.e. table[offset].source.TypeID == targetFunctionType.
	//
	// "tmp = table[offset].source ( == *FunctionInstance type)"
	c.assembler.CompileMemoryToRegister(amd64.MOVQ, offset.register, compiledFunctionSourceOffset, tmp)

	ti := c.f.Module.Types[o.TypeIndex]
	targetFunctionType := ti.Type
	c.assembler.CompileMemoryToConst(amd64.CMPL, tmp, functionInstanceTypeIDOffset, int64(ti.TypeID))

	// Jump if the type matches.
	jumpIfTypeMatch := c.assembler.CompileJump(amd64.JEQ)

	// Otherwise, exit with type mismatch status.
	c.compileExitFromNativeCode(jitCallStatusCodeTypeMismatchOnIndirectCall)

	c.assembler.SetJumpTargetOnNext(jumpIfTypeMatch)
	if err = c.compileCallFunctionImpl(0, offset.register, targetFunctionType); err != nil {
		return nil
	}

	// The offset register should be marked as un-used as we consumed in the function call.
	c.locationStack.markRegisterUnused(offset.register, tmp)

	// We consumed the function parameters from the stack after call.
	for i := 0; i < len(targetFunctionType.Params); i++ {
		c.locationStack.pop()
	}

	// Also, the function results were pushed by the call.
	for _, t := range targetFunctionType.Results {
		loc := c.locationStack.pushValueLocationOnStack()
		switch t {
		case wasm.ValueTypeI32, wasm.ValueTypeI64:
			loc.setRegisterType(generalPurposeRegisterTypeInt)
		case wasm.ValueTypeF32, wasm.ValueTypeF64:
			loc.setRegisterType(generalPurposeRegisterTypeFloat)
		}
	}
	return nil
}

// compileDrop implements compiler.compileDrop for the amd64 architecture.
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

		// If the value is on a memory, we have to move it to a register,
		// otherwise the memory location is overridden by other values
		// after this drop instruction.
		if err := c.compileEnsureOnGeneralPurposeRegister(live); err != nil {

			return err
		}

		// Modify the location in the stack with new stack pointer.
		c.locationStack.push(live)
	}
	return nil
}

// compileSelect implements compiler.compileSelect for the amd64 architecture.
//
// The emitted native code depends on whether the values are on
// the physical registers or memory stack, or maybe conditional register.
func (c *amd64Compiler) compileSelect() error {
	cv := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(cv); err != nil {
		return err
	}

	x2 := c.locationStack.pop()
	// We do not consume x1 here, but modify the value according to
	// the conditional value "c" above.
	peekedX1 := c.locationStack.peek()

	// Compare the conditional value with zero.
	c.assembler.CompileRegisterToConst(amd64.CMPQ, cv.register, 0)

	// Now we can use c.register as temporary location.
	// We alias it here for readability.
	tmpRegister := cv.register

	// Set the jump if the top value is not zero.
	jmpIfNotZero := c.assembler.CompileJump(amd64.JNE)

	// If the value is zero, we must place the value of x2 onto the stack position of x1.

	// First we copy the value of x2 to the temporary register if x2 is not currently on a register.
	if x2.onStack() {
		x2.register = tmpRegister
		c.compileLoadValueOnStackToRegister(x2)
	}

	//
	// At this point x2's value is always on a register.
	//

	// Then release the value in the x2's register to the x1's stack position.
	if peekedX1.onRegister() {
		c.assembler.CompileRegisterToRegister(amd64.MOVQ, x2.register, peekedX1.register)
	} else {
		peekedX1.register = x2.register
		c.compileReleaseRegisterToStack(peekedX1) // Note inside we mark the register unused!
	}

	// Else, we don't need to adjust value, just need to jump to the next instruction.
	c.assembler.SetJumpTargetOnNext(jmpIfNotZero)

	// In any case, we don't need x2 and c anymore!
	c.locationStack.releaseRegister(x2)
	c.locationStack.releaseRegister(cv)
	return nil
}

// compilePick implements compiler.compilePick for the amd64 architecture.
func (c *amd64Compiler) compilePick(o *wazeroir.OperationPick) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	// TODO: if we track the type of values on the stack,
	// we could optimize the instruction according to the bit size of the value.
	// For now, we just move the entire register i.e. as a quad word (8 bytes).
	pickTarget := c.locationStack.stack[c.locationStack.sp-1-uint64(o.Depth)]
	reg, err := c.allocateRegister(pickTarget.registerType())
	if err != nil {
		return err
	}

	if pickTarget.onRegister() {
		c.assembler.CompileRegisterToRegister(amd64.MOVQ, pickTarget.register, reg)
	} else if pickTarget.onStack() {
		// Copy the value from the stack.
		// Note: stack pointers are ensured not to exceed 2^27 so this offset never exceeds 32-bit range.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForStackBasePointerAddress, int64(pickTarget.stackPointer)*8, reg)
	}
	// Now we already placed the picked value on the register,
	// so push the location onto the stack.
	loc := c.pushValueLocationOnRegister(reg)
	loc.setRegisterType(pickTarget.registerType())
	return nil
}

// compileAdd implements compiler.compileAdd for the amd64 architecture.
func (c *amd64Compiler) compileAdd(o *wazeroir.OperationAdd) error {
	// TODO: if the previous instruction is const, then
	// this can be optimized. Same goes for other arithmetic instructions.

	var instruction asm.Instruction
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		instruction = amd64.ADDL
	case wazeroir.UnsignedTypeI64:
		instruction = amd64.ADDQ
	case wazeroir.UnsignedTypeF32:
		instruction = amd64.ADDSS
	case wazeroir.UnsignedTypeF64:
		instruction = amd64.ADDSD
	}

	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek, pop!
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// x1 += x2.
	c.assembler.CompileRegisterToRegister(instruction, x2.register, x1.register)

	// We no longer need x2 register after ADD operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileSub implements compiler.compileSub for the amd64 architecture.
func (c *amd64Compiler) compileSub(o *wazeroir.OperationSub) error {
	// TODO: if the previous instruction is const, then
	// this can be optimized. Same goes for other arithmetic instructions.

	var instruction asm.Instruction
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		instruction = amd64.SUBL
	case wazeroir.UnsignedTypeI64:
		instruction = amd64.SUBQ
	case wazeroir.UnsignedTypeF32:
		instruction = amd64.SUBSS
	case wazeroir.UnsignedTypeF64:
		instruction = amd64.SUBSD
	}

	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek, pop!
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// x1 -= x2.
	c.assembler.CompileRegisterToRegister(instruction, x2.register, x1.register)

	// We no longer need x2 register after ADD operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileMul implements compiler.compileMul for the amd64 architecture.
func (c *amd64Compiler) compileMul(o *wazeroir.OperationMul) (err error) {
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		err = c.compileMulForInts(true, amd64.MULL)
	case wazeroir.UnsignedTypeI64:
		err = c.compileMulForInts(false, amd64.MULQ)
	case wazeroir.UnsignedTypeF32:
		err = c.compileMulForFloats(amd64.MULSS)
	case wazeroir.UnsignedTypeF64:
		err = c.compileMulForFloats(amd64.MULSD)
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
//    the existing value will be overridden after the mul execution.
// 2) One of the operands (x1 or x2) must be on AX register.
// See https://www.felixcloutier.com/x86/mul#description for detail semantics.
func (c *amd64Compiler) compileMulForInts(is32Bit bool, mulInstruction asm.Instruction) error {
	const (
		resultRegister   = amd64.REG_AX
		reservedRegister = amd64.REG_DX
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
			c.compileMoveConditionalToGeneralPurposeRegister(x2, resultRegister)
		} else if x2.onStack() {
			x2.setRegister(resultRegister)
			c.compileLoadValueOnStackToRegister(x2)
			c.locationStack.markRegisterUsed(resultRegister)
		} else {
			var inst asm.Instruction
			if is32Bit {
				inst = amd64.MOVL
			} else {
				inst = amd64.MOVQ
			}
			c.assembler.CompileRegisterToRegister(inst, x2.register, resultRegister)

			// We no longer uses the prev register of x2.
			c.locationStack.releaseRegister(x2)
			x2.setRegister(resultRegister)
			c.locationStack.markRegisterUsed(resultRegister)
		}
	}

	// We have to make sure that at this point the operands must be on registers.
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// We have to save the existing value on DX.
	// If the DX register is used by either x1 or x2, we don't need to
	// save the value because it is consumed by mul anyway.
	if x1.register != reservedRegister && x2.register != reservedRegister {
		c.onValueReleaseRegisterToStack(reservedRegister)
	}

	// Now ready to emit the mul instruction.
	if x1 == valueOnAX {
		c.assembler.CompileRegisterToNone(mulInstruction, x2.register)
	} else {
		c.assembler.CompileRegisterToNone(mulInstruction, x1.register)
	}

	c.locationStack.markRegisterUnused(x2.register)
	c.locationStack.markRegisterUnused(x1.register)

	// Now we have the result in the AX register,
	// so we record it.
	result := c.pushValueLocationOnRegister(resultRegister)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

func (c *amd64Compiler) compileMulForFloats(instruction asm.Instruction) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// x1 *= x2.
	c.assembler.CompileRegisterToRegister(instruction, x2.register, x1.register)

	// We no longer need x2 register after MUL operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileClz implements compiler.compileClz for the amd64 architecture.
func (c *amd64Compiler) compileClz(o *wazeroir.OperationClz) error {
	target := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileRegisterToRegister(amd64.LZCNTL, target.register, target.register)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.LZCNTQ, target.register, target.register)
		}
	} else {
		// On x86 mac, we cannot use LZCNT as it always results in zero.
		// Instead we combine BSR (calculating most significant set bit)
		// with XOR. This logic is described in
		// "Replace Raw Assembly Code with Builtin Intrinsics" section in:
		// https://developer.apple.com/documentation/apple-silicon/addressing-architectural-differences-in-your-macos-code.

		// First, we have to check if the target is non-zero as BSR is undefined
		// on zero. See https://www.felixcloutier.com/x86/bsr.
		c.assembler.CompileRegisterToConst(amd64.CMPQ, target.register, 0)
		jmpIfNonZero := c.assembler.CompileJump(amd64.JNE)

		// If the value is zero, we just push the const value.
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileConstToRegister(amd64.MOVL, int64(32), target.register)
		} else {
			c.assembler.CompileConstToRegister(amd64.MOVL, int64(64), target.register)
		}

		// Emit the jmp instruction to jump to the position right after
		// the non-zero case.
		jmpAtEndOfZero := c.assembler.CompileJump(amd64.JMP)

		// Start emitting non-zero case.
		c.assembler.SetJumpTargetOnNext(jmpIfNonZero)
		// First, we calculate the most significant set bit.
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileRegisterToRegister(amd64.BSRL, target.register, target.register)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.BSRQ, target.register, target.register)
		}

		// Now we XOR the value with the bit length minus one.
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileConstToRegister(amd64.XORL, 31, target.register)
		} else {
			c.assembler.CompileConstToRegister(amd64.XORQ, 63, target.register)
		}

		// Finally the end jump instruction of zero case must target towards
		// the next instruction.
		c.assembler.SetJumpTargetOnNext(jmpAtEndOfZero)
	}

	// We reused the same register of target for the result.
	c.locationStack.markRegisterUnused(target.register)
	result := c.pushValueLocationOnRegister(target.register)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileCtz implements compiler.compileCtz for the amd64 architecture.
func (c *amd64Compiler) compileCtz(o *wazeroir.OperationCtz) error {
	target := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileRegisterToRegister(amd64.TZCNTL, target.register, target.register)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.TZCNTQ, target.register, target.register)
		}
	} else {
		// Somehow, if the target value is zero, TZCNT always returns zero: this is wrong.
		// Meanwhile, we need branches for non-zero and zero cases on macos.
		// TODO: find the reference to this behavior and put the link here.

		// First we compare the target with zero.
		c.assembler.CompileRegisterToConst(amd64.CMPQ, target.register, 0)
		jmpIfNonZero := c.assembler.CompileJump(amd64.JNE)

		// If the value is zero, we just push the const value.
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileConstToRegister(amd64.MOVL, int64(32), target.register)
		} else {
			c.assembler.CompileConstToRegister(amd64.MOVL, int64(64), target.register)
		}

		// Emit the jmp instruction to jump to the position right after
		// the non-zero case.
		jmpAtEndOfZero := c.assembler.CompileJump(amd64.JMP)

		// Otherwise, emit the TZCNT.
		c.assembler.SetJumpTargetOnNext(jmpIfNonZero)
		if o.Type == wazeroir.UnsignedInt32 {
			c.assembler.CompileRegisterToRegister(amd64.TZCNTL, target.register, target.register)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.TZCNTQ, target.register, target.register)
		}

		// Finally the end jump instruction of zero case must target towards
		// the next instruction.
		c.assembler.SetJumpTargetOnNext(jmpAtEndOfZero)
	}

	// We reused the same register of target for the result.
	c.locationStack.markRegisterUnused(target.register)
	result := c.pushValueLocationOnRegister(target.register)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compilePopcnt implements compiler.compilePopcnt for the amd64 architecture.
func (c *amd64Compiler) compilePopcnt(o *wazeroir.OperationPopcnt) error {
	target := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	if o.Type == wazeroir.UnsignedInt32 {
		c.assembler.CompileRegisterToRegister(amd64.POPCNTL, target.register, target.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.POPCNTQ, target.register, target.register)
	}

	// We reused the same register of target for the result.
	c.locationStack.markRegisterUnused(target.register)
	result := c.pushValueLocationOnRegister(target.register)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileDiv implements compiler.compileDiv for the amd64 architecture.
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
// the remainder is discarded.
func (c *amd64Compiler) compileDivForInts(is32Bit bool, signed bool) error {
	if err := c.performDivisionOnInts(false, is32Bit, signed); err != nil {
		return err
	}
	// Now we have the quotient of the division result in the AX register,
	// so we record it.
	result := c.pushValueLocationOnRegister(amd64.REG_AX)
	result.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileRem implements compiler.compileRem for the amd64 architecture.
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
	result := c.pushValueLocationOnRegister(amd64.REG_DX)
	result.setRegisterType(generalPurposeRegisterTypeInt)
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
		quotientRegister  = amd64.REG_AX
		remainderRegister = amd64.REG_DX
	)

	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	// Ensures that previous values on these registers are saved to memory.
	c.onValueReleaseRegisterToStack(quotientRegister)
	c.onValueReleaseRegisterToStack(remainderRegister)

	// In order to ensure x2 is placed on a temporary register for x2 value other than AX and DX,
	// we mark them as used here.
	c.locationStack.markRegisterUsed(quotientRegister)
	c.locationStack.markRegisterUsed(remainderRegister)

	// Ensure that x2 is placed on a register which is not either AX or DX.
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	// Now we successfully place x2 on a temp register, so we no longer need to
	// mark these registers used.
	c.locationStack.markRegisterUnused(quotientRegister)
	c.locationStack.markRegisterUnused(remainderRegister)

	// Check if the x2 equals zero.
	if is32Bit {
		c.assembler.CompileRegisterToConst(amd64.CMPL, x2.register, 0)
	} else {
		c.assembler.CompileRegisterToConst(amd64.CMPQ, x2.register, 0)
	}

	// Jump if the divisor is not zero.
	jmpIfNotZero := c.assembler.CompileJump(amd64.JNE)

	// Otherwise, we return with jitCallStatusIntegerDivisionByZero status.
	c.compileExitFromNativeCode(jitCallStatusIntegerDivisionByZero)

	c.assembler.SetJumpTargetOnNext(jmpIfNotZero)

	// Next, we ensure that x1 is placed on AX.
	x1 := c.locationStack.pop()
	if x1.onRegister() && x1.register != quotientRegister {
		// Move x1 to quotientRegister.
		if is32Bit {
			c.assembler.CompileRegisterToRegister(amd64.MOVL, x1.register, quotientRegister)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.MOVQ, x1.register, quotientRegister)
		}
		c.locationStack.markRegisterUnused(x1.register)
		x1.setRegister(quotientRegister)
	} else if x1.onStack() {
		x1.setRegister(quotientRegister)
		c.compileLoadValueOnStackToRegister(x1)
	}

	// Note: at this point, x1 is placed on AX, x2 is on a register which is not AX or DX.

	isSignedRem := isRem && signed
	isSignedDiv := !isRem && signed
	var signedRemMinusOneDivisorJmp asm.Node
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
		if is32Bit {
			c.assembler.CompileRegisterToConst(amd64.CMPL, x2.register, -1)
		} else {
			c.assembler.CompileRegisterToConst(amd64.CMPQ, x2.register, -1)
		}

		// If it doesn't equal minus one, we jump to the normal case.
		okJmp := c.assembler.CompileJump(amd64.JNE)

		// Otherwise, we store zero into the remainder result register (DX).
		if is32Bit {
			c.assembler.CompileRegisterToRegister(amd64.XORL, remainderRegister, remainderRegister)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.XORQ, remainderRegister, remainderRegister)
		}

		// Emit the exit jump instruction for the divisor -1 case so
		// we skips the normal case.
		signedRemMinusOneDivisorJmp = c.assembler.CompileJump(amd64.JMP)

		// Set the normal case's jump target.
		c.assembler.SetJumpTargetOnNext(okJmp)
	} else if isSignedDiv {
		// For signed division, we have to have branches for "math.MinInt{32,64} / -1"
		// case which results in the floating point exception via division error as
		// the resulting value exceeds the maximum of signed int.

		// First we compare the division with -1.
		if is32Bit {
			c.assembler.CompileRegisterToConst(amd64.CMPL, x2.register, -1)
		} else {
			c.assembler.CompileRegisterToConst(amd64.CMPQ, x2.register, -1)
		}

		// If it doesn't equal minus one, we jump to the normal case.
		nonMinusOneDivisorJmp := c.assembler.CompileJump(amd64.JNE)

		// Next we check if the quotient is the most negative value for the signed integer.
		// That means whether or not we try to do (math.MaxInt32 / -1) or (math.Math.Int64 / -1) respectively.
		if is32Bit {
			c.assembler.CompileRegisterToMemory(amd64.CMPL, x1.register, asm.NilRegister, int64(minimum32BitSignedIntAddress))
		} else {
			c.assembler.CompileRegisterToMemory(amd64.CMPQ, x1.register, asm.NilRegister, int64(minimum64BitSignedIntAddress))
		}

		// If it doesn't equal, we jump to the normal case.
		jmpOK := c.assembler.CompileJump(amd64.JNE)

		// Otherwise, we are trying to do (math.MaxInt32 / -1) or (math.Math.Int64 / -1),
		// and that is the overflow in division as the result becomes 2^31 which is larger than
		// the maximum of signed 32-bit int (2^31-1).
		c.compileExitFromNativeCode(jitCallStatusIntegerOverflow)

		// Set the normal case's jump target.
		c.assembler.SetJumpTargetOnNext(nonMinusOneDivisorJmp, jmpOK)
	}

	// Now ready to emit the div instruction.
	// Since the div instructions takes 2n byte dividend placed in DX:AX registers...
	// * signed case - we need to sign-extend the dividend into DX register via CDQ (32 bit) or CQO (64 bit).
	// * unsigned case - we need to zero DX register via "XOR DX DX"
	if is32Bit && signed {
		// Emit sign-extension to have 64 bit dividend over DX and AX registers.
		c.assembler.CompileStandAlone(amd64.CDQ)
		c.assembler.CompileRegisterToNone(amd64.IDIVL, x2.register)
	} else if is32Bit && !signed {
		// Zeros DX register to have 64 bit dividend over DX and AX registers.
		c.assembler.CompileRegisterToRegister(amd64.XORQ, amd64.REG_DX, amd64.REG_DX)
		c.assembler.CompileRegisterToNone(amd64.DIVL, x2.register)
	} else if !is32Bit && signed {
		// Emits sign-extension to have 128 bit dividend over DX and AX registers.
		c.assembler.CompileStandAlone(amd64.CQO)
		c.assembler.CompileRegisterToNone(amd64.IDIVQ, x2.register)
	} else if !is32Bit && !signed {
		// Zeros DX register to have 128 bit dividend over DX and AX registers.
		c.assembler.CompileRegisterToRegister(amd64.XORQ, amd64.REG_DX, amd64.REG_DX)
		c.assembler.CompileRegisterToNone(amd64.DIVQ, x2.register)
	}

	// If this is signed rem instruction, we must set the jump target of
	// the exit jump from division -1 case towards the next instruction.
	if signedRemMinusOneDivisorJmp != nil {
		c.assembler.SetJumpTargetOnNext(signedRemMinusOneDivisorJmp)
	}

	// We mark them as unused so that we can push one of them onto the location stack at call sites.
	c.locationStack.markRegisterUnused(remainderRegister)
	c.locationStack.markRegisterUnused(quotientRegister)
	c.locationStack.markRegisterUnused(x2.register)
	return nil
}

// compileDivForFloats emits the instructions to perform division
// on the top two values of float type on the stack, placing the result back onto the stack.
// For example, stack [..., 1.0, 4.0] results in [..., 0.25].
func (c *amd64Compiler) compileDivForFloats(is32Bit bool) error {
	if is32Bit {
		return c.compileSimpleBinaryOp(amd64.DIVSS)
	} else {
		return c.compileSimpleBinaryOp(amd64.DIVSD)
	}
}

// compileAnd implements compiler.compileAnd for the amd64 architecture.
func (c *amd64Compiler) compileAnd(o *wazeroir.OperationAnd) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.compileSimpleBinaryOp(amd64.ANDL)
	case wazeroir.UnsignedInt64:
		err = c.compileSimpleBinaryOp(amd64.ANDQ)
	}
	return
}

// compileOr implements compiler.compileOr for the amd64 architecture.
func (c *amd64Compiler) compileOr(o *wazeroir.OperationOr) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.compileSimpleBinaryOp(amd64.ORL)
	case wazeroir.UnsignedInt64:
		err = c.compileSimpleBinaryOp(amd64.ORQ)
	}
	return
}

// compileXor implements compiler.compileXor for the amd64 architecture.
func (c *amd64Compiler) compileXor(o *wazeroir.OperationXor) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.compileSimpleBinaryOp(amd64.XORL)
	case wazeroir.UnsignedInt64:
		err = c.compileSimpleBinaryOp(amd64.XORQ)
	}
	return
}

// compileSimpleBinaryOp emits instructions to pop two values from the stack
// and perform the given instruction on these two values and push the result
// onto the stack.
func (c *amd64Compiler) compileSimpleBinaryOp(instruction asm.Instruction) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(instruction, x2.register, x1.register)

	// We consumed x2 register after the operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)

	// We already stored the result in the register used by x1
	// so we record it.
	c.locationStack.markRegisterUnused(x1.register)
	result := c.pushValueLocationOnRegister(x1.register)
	result.setRegisterType(x1.registerType())
	return nil
}

// compileShl implements compiler.compileShl for the amd64 architecture.
func (c *amd64Compiler) compileShl(o *wazeroir.OperationShl) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.compileShiftOp(amd64.SHLL, false)
	case wazeroir.UnsignedInt64:
		err = c.compileShiftOp(amd64.SHLQ, true)
	}
	return
}

// compileShr implements compiler.compileShr for the amd64 architecture.
func (c *amd64Compiler) compileShr(o *wazeroir.OperationShr) (err error) {
	switch o.Type {
	case wazeroir.SignedInt32:
		err = c.compileShiftOp(amd64.SARL, true)
	case wazeroir.SignedInt64:
		err = c.compileShiftOp(amd64.SARQ, false)
	case wazeroir.SignedUint32:
		err = c.compileShiftOp(amd64.SHRL, true)
	case wazeroir.SignedUint64:
		err = c.compileShiftOp(amd64.SHRQ, false)
	}
	return
}

// compileRotl implements compiler.compileRotl for the amd64 architecture.
func (c *amd64Compiler) compileRotl(o *wazeroir.OperationRotl) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.compileShiftOp(amd64.ROLL, true)
	case wazeroir.UnsignedInt64:
		err = c.compileShiftOp(amd64.ROLQ, false)
	}
	return
}

// compileRotr implements compiler.compileRotr for the amd64 architecture.
func (c *amd64Compiler) compileRotr(o *wazeroir.OperationRotr) (err error) {
	switch o.Type {
	case wazeroir.UnsignedInt32:
		err = c.compileShiftOp(amd64.RORL, true)
	case wazeroir.UnsignedInt64:
		err = c.compileShiftOp(amd64.RORQ, false)
	}
	return
}

// compileShiftOp adds instructions for shift operations (SHR, SHL, ROTR, ROTL)
// where we have to place the second value (shift counts) on the CX register.
func (c *amd64Compiler) compileShiftOp(instruction asm.Instruction, is32Bit bool) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	x2 := c.locationStack.pop()

	// Ensures that x2 (holding shift counts) is placed on the CX register.
	const shiftCountRegister = amd64.REG_CX
	if (x2.onRegister() && x2.register != shiftCountRegister) || x2.onStack() {
		// If another value lives on the CX register, we release it to the stack.
		c.onValueReleaseRegisterToStack(shiftCountRegister)

		if x2.onRegister() {
			// If x2 lives on a register, we move the value to CX.
			if is32Bit {
				c.assembler.CompileRegisterToRegister(amd64.MOVL, x2.register, shiftCountRegister)
			} else {
				c.assembler.CompileRegisterToRegister(amd64.MOVQ, x2.register, shiftCountRegister)
			}
			// We no longer place any value on the original register, so we record it.
			c.locationStack.markRegisterUnused(x2.register)
			// Instead, we've already placed the value on the CX register.
			x2.setRegister(shiftCountRegister)
		} else {
			// If it is on stack, we just move the memory allocated value to the CX register.
			x2.setRegister(shiftCountRegister)
			c.compileLoadValueOnStackToRegister(x2)
		}
		c.locationStack.markRegisterUsed(shiftCountRegister)
	}

	x1 := c.locationStack.peek() // Note this is peek!

	if x1.onRegister() {
		c.assembler.CompileRegisterToRegister(instruction, x2.register, x1.register)
	} else {
		// Shift target can be placed on a memory location.
		// Note: stack pointers are ensured not to exceed 2^27 so this offset never exceeds 32-bit range.
		c.assembler.CompileRegisterToMemory(instruction, x2.register, amd64ReservedRegisterForStackBasePointerAddress, int64(x1.stackPointer)*8)
	}

	// We consumed x2 register after the operation here,
	// so we release it.
	c.locationStack.releaseRegister(x2)
	return nil
}

// compileAbs implements compiler.compileAbs for the amd64 architecture.
//
// See the following discussions for how we could take the abs of floats on x86 assembly.
// https://stackoverflow.com/questions/32408665/fastest-way-to-compute-absolute-value-using-sse/32422471#32422471
// https://stackoverflow.com/questions/44630015/how-would-fabsdouble-be-implemented-on-x86-is-it-an-expensive-operation
func (c *amd64Compiler) compileAbs(o *wazeroir.OperationAbs) (err error) {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	// First shift left by one to clear the sign bit, and then shift right by one.
	if o.Type == wazeroir.Float32 {
		c.assembler.CompileConstToRegister(amd64.PSLLL, 1, target.register)
		c.assembler.CompileConstToRegister(amd64.PSRLL, 1, target.register)
	} else {
		c.assembler.CompileConstToRegister(amd64.PSLLQ, 1, target.register)
		c.assembler.CompileConstToRegister(amd64.PSRLQ, 1, target.register)
	}
	return nil
}

// compileNeg implements compiler.compileNeg for the amd64 architecture.
func (c *amd64Compiler) compileNeg(o *wazeroir.OperationNeg) (err error) {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	// First we move the sign-bit mask (placed in memory) to the tmp register,
	// since we cannot take XOR directly with float reg and const.
	// And then negate the value by XOR it with the sign-bit mask.
	if o.Type == wazeroir.Float32 {
		c.assembler.CompileMemoryToRegister(amd64.MOVL, asm.NilRegister, int64(float32SignBitMaskAddress), tmpReg)
		c.assembler.CompileRegisterToRegister(amd64.XORPS, tmpReg, target.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, asm.NilRegister, int64(float64SignBitMaskAddress), tmpReg)
		c.assembler.CompileRegisterToRegister(amd64.XORPD, tmpReg, target.register)
	}
	return nil
}

// compileCeil implements compiler.compileCeil for the amd64 architecture.
func (c *amd64Compiler) compileCeil(o *wazeroir.OperationCeil) (err error) {
	// Internally, ceil can be performed via ROUND instruction with 0x02 mode.
	// See https://android.googlesource.com/platform/bionic/+/882b8af/libm/x86_64/ceilf.S for example.
	return c.compileRoundInstruction(o.Type == wazeroir.Float32, 0x02)
}

// compileFloor implements compiler.compileFloor for the amd64 architecture.
func (c *amd64Compiler) compileFloor(o *wazeroir.OperationFloor) (err error) {
	// Internally, floor can be performed via ROUND instruction with 0x01 mode.
	// See https://android.googlesource.com/platform/bionic/+/882b8af/libm/x86_64/floorf.S for example.
	return c.compileRoundInstruction(o.Type == wazeroir.Float32, 0x01)
}

// compileTrunc implements compiler.compileTrunc for the amd64 architecture.
func (c *amd64Compiler) compileTrunc(o *wazeroir.OperationTrunc) error {
	// Internally, trunc can be performed via ROUND instruction with 0x03 mode.
	// See https://android.googlesource.com/platform/bionic/+/882b8af/libm/x86_64/truncf.S for example.
	return c.compileRoundInstruction(o.Type == wazeroir.Float32, 0x03)
}

// compileNearest implements compiler.compileNearest for the amd64 architecture.
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
	return c.compileRoundInstruction(o.Type == wazeroir.Float32, 0x00)
}

func (c *amd64Compiler) compileRoundInstruction(is32Bit bool, mode int64) error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	if is32Bit {
		c.assembler.CompileRegisterToRegisterWithMode(amd64.ROUNDSS, target.register, target.register, byte(mode))
	} else {
		c.assembler.CompileRegisterToRegisterWithMode(amd64.ROUNDSD, target.register, target.register, byte(mode))
	}
	return nil
}

// compileMin implements compiler.compileMin for the amd64 architecture.
func (c *amd64Compiler) compileMin(o *wazeroir.OperationMin) error {
	is32Bit := o.Type == wazeroir.Float32
	if is32Bit {
		return c.compileMinOrMax(is32Bit, amd64.MINSS)
	} else {
		return c.compileMinOrMax(is32Bit, amd64.MINSD)
	}
}

// compileMax implements compiler.compileMax for the amd64 architecture.
func (c *amd64Compiler) compileMax(o *wazeroir.OperationMax) error {
	is32Bit := o.Type == wazeroir.Float32
	if is32Bit {
		return c.compileMinOrMax(is32Bit, amd64.MAXSS)
	} else {
		return c.compileMinOrMax(is32Bit, amd64.MAXSD)
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
func (c *amd64Compiler) compileMinOrMax(is32Bit bool, minOrMaxInstruction asm.Instruction) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}
	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Check if this is (either x1 or x2 is NaN) or (x1 equals x2) case
	if is32Bit {
		c.assembler.CompileRegisterToRegister(amd64.UCOMISS, x2.register, x1.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.UCOMISD, x2.register, x1.register)
	}

	// At this point, we have the three cases of conditional flags below
	// (See https://www.felixcloutier.com/x86/ucomiss#operation for detail.)
	//
	// 1) Two values are NaN-free and different: All flags are cleared.
	// 2) Two values are NaN-free and equal: Only ZF flags is set.
	// 3) One of Two values is NaN: ZF, PF and CF flags are set.

	// Jump instruction to handle 1) case by checking the ZF flag
	// as ZF is only set for 2) and 3) cases.
	nanFreeOrDiffJump := c.assembler.CompileJump(amd64.JNE)

	// Start handling 2) and 3).

	// Jump if two values are equal and NaN-free by checking the parity flag (PF).
	// Here we use JPC to do the conditional jump when the parity flag is NOT set,
	// and that is of 2).
	equalExitJmp := c.assembler.CompileJump(amd64.JPC)

	// Start handling 3).

	// We emit the ADD instruction to produce the NaN in x1.
	if is32Bit {
		c.assembler.CompileRegisterToRegister(amd64.ADDSS, x2.register, x1.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.ADDSD, x2.register, x1.register)
	}

	// Exit from the NaN case branch.
	nanExitJmp := c.assembler.CompileJump(amd64.JMP)

	// Start handling 1).
	c.assembler.SetJumpTargetOnNext(nanFreeOrDiffJump)

	// Now handle the NaN-free and different values case.
	c.assembler.CompileRegisterToRegister(minOrMaxInstruction, x2.register, x1.register)

	// Set the jump target of 1) and 2) cases to the next instruction after 3) case.
	c.assembler.SetJumpTargetOnNext(nanExitJmp, equalExitJmp)

	// Record that we consumed the x2 and placed the minOrMax result in the x1's register.
	c.locationStack.markRegisterUnused(x2.register)
	c.locationStack.markRegisterUnused(x1.register)
	c.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileCopysign implements compiler.compileCopysign for the amd64 architecture.
func (c *amd64Compiler) compileCopysign(o *wazeroir.OperationCopysign) error {
	is32Bit := o.Type == wazeroir.Float32

	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}
	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	// Move the rest bit mask to the temp register.
	if is32Bit {
		c.assembler.CompileMemoryToRegister(amd64.MOVL, asm.NilRegister, int64(float32RestBitMaskAddress), tmpReg)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, asm.NilRegister, int64(float64RestBitMaskAddress), tmpReg)
	}

	// Clear the sign bit of x1 via AND with the mask.
	if is32Bit {
		c.assembler.CompileRegisterToRegister(amd64.ANDPS, tmpReg, x1.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.ANDPD, tmpReg, x1.register)
	}

	// Move the sign bit mask to the temp register.
	if is32Bit {
		c.assembler.CompileMemoryToRegister(amd64.MOVL, asm.NilRegister, int64(float32SignBitMaskAddress), tmpReg)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, asm.NilRegister, int64(float64SignBitMaskAddress), tmpReg)
	}

	// Clear the non-sign bits of x2 via AND with the mask.
	if is32Bit {
		c.assembler.CompileRegisterToRegister(amd64.ANDPS, tmpReg, x2.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.ANDPD, tmpReg, x2.register)
	}

	// Finally, copy the sign bit of x2 to x1.
	if is32Bit {
		c.assembler.CompileRegisterToRegister(amd64.ORPS, x2.register, x1.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.ORPD, x2.register, x1.register)
	}

	// Record that we consumed the x2 and placed the copysign result in the x1's register.
	c.locationStack.markRegisterUnused(x2.register)
	c.locationStack.markRegisterUnused(x1.register)
	c.pushValueLocationOnRegister(x1.register)
	return nil
}

// compileSqrt implements compiler.compileSqrt for the amd64 architecture.
func (c *amd64Compiler) compileSqrt(o *wazeroir.OperationSqrt) error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}
	if o.Type == wazeroir.Float32 {
		c.assembler.CompileRegisterToRegister(amd64.SQRTSS, target.register, target.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.SQRTSD, target.register, target.register)
	}
	return nil
}

// compileI32WrapFromI64 implements compiler.compileI32WrapFromI64 for the amd64 architecture.
func (c *amd64Compiler) compileI32WrapFromI64() error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}
	c.assembler.CompileRegisterToRegister(amd64.MOVL, target.register, target.register)
	return nil
}

// compileITruncFromF implements compiler.compileITruncFromF for the amd64 architecture.
//
// Note: in the following implementation, we use CVTSS2SI and CVTSD2SI to convert floats to signed integers.
// According to the Intel manual ([1],[2]), if the source float value is either +-Inf or NaN, or it exceeds representative ranges
// of target signed integer, then the instruction returns "masked" response float32SignBitMask (or float64SignBitMask for 64 bit case).
// [1] Chapter 11.5.2, SIMD Floating-Point Exception Conditions in "Vol 1, Intel® 64 and IA-32 Architectures Manual"
//     https://www.intel.com/content/www/us/en/architecture-and-technology/64-ia-32-architectures-software-developer-vol-1-manual.html
// [2] https://xem.github.io/minix86/manual/intel-x86-and-64-manual-vol1/o_7281d5ea06a5b67a-268.html
func (c *amd64Compiler) compileITruncFromF(o *wazeroir.OperationITruncFromF) (err error) {
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
	if err := c.compileEnsureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we check the source float value is above or equal math.MaxInt32+1.
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISS, asm.NilRegister, int64(float32ForMaximumSigned32bitIntPlusOneAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISD, asm.NilRegister, int64(float64ForMaximumSigned32bitIntPlusOneAddress), source.register)
	}

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNaN := c.assembler.CompileJump(amd64.JPS) // jump if parity is set.

	// Jump if the source float value is above or equal math.MaxInt32+1.
	jmpAboveOrEqualMaxIn32PlusOne := c.assembler.CompileJump(amd64.JCC)

	// Next we convert the value as a signed integer.
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSS2SL, source.register, result)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSD2SL, source.register, result)
	}

	// Then if the result is minus, it is invalid conversion from minus float (incl. -Inf).
	c.assembler.CompileRegisterToRegister(amd64.TESTL, result, result)

	jmpIfMinusOrMinusInf := c.assembler.CompileJump(amd64.JMI)

	// Otherwise, the values is valid.
	okJmpForLessThanMaxInt32PlusOne := c.assembler.CompileJump(amd64.JMP)

	// Now, start handling the case where the original float value is above or equal math.MaxInt32+1.
	//
	// First, we subtract the math.MaxInt32+1 from the original value so it can fit in signed 32-bit integer.
	c.assembler.SetJumpTargetOnNext(jmpAboveOrEqualMaxIn32PlusOne)
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.SUBSS, asm.NilRegister, int64(float32ForMaximumSigned32bitIntPlusOneAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.SUBSD, asm.NilRegister, int64(float64ForMaximumSigned32bitIntPlusOneAddress), source.register)
	}

	// Then, convert the subtracted value as a signed 32-bit integer.
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSS2SL, source.register, result)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSD2SL, source.register, result)
	}

	// Next, we have to check if the value is from NaN, +Inf.
	// NaN or +Inf cases result in 0x8000_0000 according to the semantics of conversion,
	// This means we check if the result int value is minus or not.
	c.assembler.CompileRegisterToRegister(amd64.TESTL, result, result)

	// If the result is minus, the conversion is invalid (from NaN or +Inf)
	jmpIfPlusInf := c.assembler.CompileJump(amd64.JMI)

	// Otherwise, we successfully converted the the source float minus (math.MaxInt32+1) to int.
	// So, we retrieve the original source float value by adding the sign mask.
	c.assembler.CompileMemoryToRegister(amd64.ADDL, asm.NilRegister, int64(float32SignBitMaskAddress), result)

	okJmpForAboveOrEqualMaxInt32PlusOne := c.assembler.CompileJump(amd64.JMP)

	c.assembler.SetJumpTargetOnNext(jmpIfNaN)
	c.compileExitFromNativeCode(jitCallStatusCodeInvalidFloatToIntConversion)

	c.assembler.SetJumpTargetOnNext(jmpIfMinusOrMinusInf, jmpIfPlusInf)
	c.compileExitFromNativeCode(jitCallStatusIntegerOverflow)

	// We jump to the next instructions for valid cases.
	c.assembler.SetJumpTargetOnNext(okJmpForLessThanMaxInt32PlusOne, okJmpForAboveOrEqualMaxInt32PlusOne)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.pushValueLocationOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// emitUnsignedI32TruncFromFloat implements compileITruncFromF when the destination type is a 64-bit unsigned integer.
func (c *amd64Compiler) emitUnsignedI64TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, we check the source float value is above or equal math.MaxInt64+1.
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISS, asm.NilRegister, int64(float32ForMaximumSigned64bitIntPlusOneAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISD, asm.NilRegister, int64(float64ForMaximumSigned64bitIntPlusOneAddress), source.register)
	}

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNaN := c.assembler.CompileJump(amd64.JPS) // jump if parity is set.

	// Jump if the source float values is above or equal math.MaxInt64+1.
	jmpAboveOrEqualMaxIn32PlusOne := c.assembler.CompileJump(amd64.JCC)

	// Next we convert the value as a signed integer.
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSS2SQ, source.register, result)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSD2SQ, source.register, result)
	}

	// Then if the result is minus, it is invalid conversion from minus float (incl. -Inf).
	c.assembler.CompileRegisterToRegister(amd64.TESTQ, result, result)
	jmpIfMinusOrMinusInf := c.assembler.CompileJump(amd64.JMI)

	// Otherwise, the values is valid.
	okJmpForLessThanMaxInt64PlusOne := c.assembler.CompileJump(amd64.JMP)

	// Now, start handling the case where the original float value is above or equal math.MaxInt64+1.
	//
	// First, we subtract the math.MaxInt64+1 from the original value so it can fit in signed 64-bit integer.
	c.assembler.SetJumpTargetOnNext(jmpAboveOrEqualMaxIn32PlusOne)
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.SUBSS, asm.NilRegister, int64(float32ForMaximumSigned64bitIntPlusOneAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.SUBSD, asm.NilRegister, int64(float64ForMaximumSigned64bitIntPlusOneAddress), source.register)
	}

	// Then, convert the subtracted value as a signed 64-bit integer.
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSS2SQ, source.register, result)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSD2SQ, source.register, result)
	}

	// Next, we have to check if the value is from NaN, +Inf.
	// NaN or +Inf cases result in 0x8000_0000 according to the semantics of conversion,
	// This means we check if the result int value is minus or not.
	c.assembler.CompileRegisterToRegister(amd64.TESTQ, result, result)

	// If the result is minus, the conversion is invalid (from NaN or +Inf)
	jmpIfPlusInf := c.assembler.CompileJump(amd64.JMI)

	// Otherwise, we successfully converted the the source float minus (math.MaxInt64+1) to int.
	// So, we retrieve the original source float value by adding the sign mask.
	c.assembler.CompileMemoryToRegister(amd64.ADDQ, asm.NilRegister, int64(float64SignBitMaskAddress), result)

	okJmpForAboveOrEqualMaxInt64PlusOne := c.assembler.CompileJump(amd64.JMP)

	c.assembler.SetJumpTargetOnNext(jmpIfNaN)
	c.compileExitFromNativeCode(jitCallStatusCodeInvalidFloatToIntConversion)

	c.assembler.SetJumpTargetOnNext(jmpIfMinusOrMinusInf, jmpIfPlusInf)
	c.compileExitFromNativeCode(jitCallStatusIntegerOverflow)

	// We jump to the next instructions for valid cases.
	c.assembler.SetJumpTargetOnNext(okJmpForLessThanMaxInt64PlusOne, okJmpForAboveOrEqualMaxInt64PlusOne)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.pushValueLocationOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// emitSignedI32TruncFromFloat implements compileITruncFromF when the destination type is a 32-bit signed integer.
func (c *amd64Compiler) emitSignedI32TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First we unconditionally convert source to integer via CVTTSS2SI (CVTTSD2SI for 64bit float).
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSS2SL, source.register, result)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSD2SL, source.register, result)
	}

	// We compare the conversion result with the sign bit mask to check if it is either
	// 1) the source float value is either +-Inf or NaN, or it exceeds representative ranges of 32bit signed integer, or
	// 2) the source equals the minimum signed 32-bit (=-2147483648.000000) whose bit pattern is float32ForMinimumSigned32bitIntegerAddress for 32 bit float
	// 	  or float64ForMinimumSigned32bitIntegerAddress for 64bit float.
	c.assembler.CompileMemoryToRegister(amd64.CMPL, asm.NilRegister, int64(float32SignBitMaskAddress), result)

	// Otherwise, jump to exit as the result is valid.
	okJmp := c.assembler.CompileJump(amd64.JNE)

	// Start handling the case of 1) and 2).
	// First, check if the value is NaN.
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.UCOMISS, source.register, source.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.UCOMISD, source.register, source.register)
	}

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNotNaN := c.assembler.CompileJump(amd64.JPC) // jump if parity is not set.

	// If the value is NaN, we return the function with jitCallStatusCodeInvalidFloatToIntConversion.
	c.compileExitFromNativeCode(jitCallStatusCodeInvalidFloatToIntConversion)

	// Check if the value is larger than or equal the minimum 32-bit integer value,
	// meaning that the value exceeds the lower bound of 32-bit signed integer range.
	c.assembler.SetJumpTargetOnNext(jmpIfNotNaN)
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISS, asm.NilRegister, int64(float32ForMinimumSigned32bitIntegerAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISD, asm.NilRegister, int64(float64ForMinimumSigned32bitIntegerAddress), source.register)
	}

	// Jump if the value exceeds the lower bound.
	var jmpIfExceedsLowerBound asm.Node
	if isFloat32Bit {
		jmpIfExceedsLowerBound = c.assembler.CompileJump(amd64.JCS)
	} else {
		jmpIfExceedsLowerBound = c.assembler.CompileJump(amd64.JLS)
	}

	// At this point, the value is the minimum signed 32-bit int (=-2147483648.000000) or larger than 32-bit maximum.
	// So, check if the value equals the minimum signed 32-bit int.
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISS, asm.NilRegister, int64(zero64BitAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISD, asm.NilRegister, int64(zero64BitAddress), source.register)
	}

	jmpIfMinimumSignedInt := c.assembler.CompileJump(amd64.JCS) // jump if the value is minus (= the minimum signed 32-bit int).

	c.assembler.SetJumpTargetOnNext(jmpIfExceedsLowerBound)
	c.compileExitFromNativeCode(jitCallStatusIntegerOverflow)

	// We jump to the next instructions for valid cases.
	c.assembler.SetJumpTargetOnNext(okJmp, jmpIfMinimumSignedInt)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.pushValueLocationOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// emitSignedI64TruncFromFloat implements compileITruncFromF when the destination type is a 64-bit signed integer.
func (c *amd64Compiler) emitSignedI64TruncFromFloat(isFloat32Bit bool) error {
	source := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(source); err != nil {
		return err
	}

	result, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First we unconditionally convert source to integer via CVTTSS2SI (CVTTSD2SI for 64bit float).
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSS2SQ, source.register, result)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTTSD2SQ, source.register, result)
	}

	// We compare the conversion result with the sign bit mask to check if it is either
	// 1) the source float value is either +-Inf or NaN, or it exceeds representative ranges of 32bit signed integer, or
	// 2) the source equals the minimum signed 32-bit (=-9223372036854775808.0) whose bit pattern is float32ForMinimumSigned64bitIntegerAddress for 32 bit float
	// 	  or float64ForMinimumSigned64bitIntegerAddress for 64bit float.
	c.assembler.CompileMemoryToRegister(amd64.CMPQ, asm.NilRegister, int64(float64SignBitMaskAddress), result)

	// Otherwise, we simply jump to exit as the result is valid.
	okJmp := c.assembler.CompileJump(amd64.JNE)

	// Start handling the case of 1) and 2).
	// First, check if the value is NaN.
	if isFloat32Bit {
		c.assembler.CompileRegisterToRegister(amd64.UCOMISS, source.register, source.register)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.UCOMISD, source.register, source.register)
	}

	// Check the parity flag (set when the value is NaN), and if it is set, we should raise an exception.
	jmpIfNotNaN := c.assembler.CompileJump(amd64.JPC) // jump if parity is not set.

	c.compileExitFromNativeCode(jitCallStatusCodeInvalidFloatToIntConversion)

	// Check if the value is larger than or equal the minimum 64-bit integer value,
	// meaning that the value exceeds the lower bound of 64-bit signed integer range.
	c.assembler.SetJumpTargetOnNext(jmpIfNotNaN)
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISS, asm.NilRegister, int64(float32ForMinimumSigned64bitIntegerAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISD, asm.NilRegister, int64(float64ForMinimumSigned64bitIntegerAddress), source.register)
	}

	// Jump if the value is -Inf.
	jmpIfExceedsLowerBound := c.assembler.CompileJump(amd64.JCS)

	// At this point, the value is the minimum signed 64-bit int (=-9223372036854775808.0) or larger than 64-bit maximum.
	// So, check if the value equals the minimum signed 64-bit int.
	if isFloat32Bit {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISS, asm.NilRegister, int64(zero64BitAddress), source.register)
	} else {
		c.assembler.CompileMemoryToRegister(amd64.UCOMISD, asm.NilRegister, int64(zero64BitAddress), source.register)
	}

	jmpIfMinimumSignedInt := c.assembler.CompileJump(amd64.JCS) // jump if the value is minus (= the minimum signed 64-bit int).

	c.assembler.SetJumpTargetOnNext(jmpIfExceedsLowerBound)
	c.compileExitFromNativeCode(jitCallStatusIntegerOverflow)

	// We jump to the next instructions for valid cases.
	c.assembler.SetJumpTargetOnNext(okJmp, jmpIfMinimumSignedInt)

	// We consumed the source's register and placed the conversion result
	// in the result register.
	c.locationStack.markRegisterUnused(source.register)
	loc := c.pushValueLocationOnRegister(result)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileFConvertFromI implements compiler.compileFConvertFromI for the amd64 architecture.
func (c *amd64Compiler) compileFConvertFromI(o *wazeroir.OperationFConvertFromI) (err error) {
	if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedInt32 {
		err = c.compileSimpleConversion(amd64.CVTSL2SS, generalPurposeRegisterTypeFloat) // = CVTSI2SS for 32bit int
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedInt64 {
		err = c.compileSimpleConversion(amd64.CVTSQ2SS, generalPurposeRegisterTypeFloat) // = CVTSI2SS for 64bit int
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedInt32 {
		err = c.compileSimpleConversion(amd64.CVTSL2SD, generalPurposeRegisterTypeFloat) // = CVTSI2SD for 32bit int
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedInt64 {
		err = c.compileSimpleConversion(amd64.CVTSQ2SD, generalPurposeRegisterTypeFloat) // = CVTSI2SD for 64bit int
	} else if o.OutputType == wazeroir.Float32 && o.InputType == wazeroir.SignedUint32 {
		// See the following link for why we use 64bit conversion for unsigned 32bit integer sources:
		// https://stackoverflow.com/questions/41495498/fpu-operations-generated-by-gcc-during-casting-integer-to-float.
		//
		// Here's the summary:
		// >> CVTSI2SS is indeed designed for converting a signed integer to a scalar single-precision float,
		// >> not an unsigned integer like you have here. So what gives? Well, a 64-bit processor has 64-bit wide
		// >> registers available, so the unsigned 32-bit input values can be stored as signed 64-bit intermediate values,
		// >> which allows CVTSI2SS to be used after all.
		err = c.compileSimpleConversion(amd64.CVTSQ2SS, generalPurposeRegisterTypeFloat) // = CVTSI2SS for 64bit int.
	} else if o.OutputType == wazeroir.Float64 && o.InputType == wazeroir.SignedUint32 {
		// For the same reason above, we use 64bit conversion for unsigned 32bit.
		err = c.compileSimpleConversion(amd64.CVTSQ2SD, generalPurposeRegisterTypeFloat) // = CVTSI2SD for 64bit int.
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
	if err := c.compileEnsureOnGeneralPurposeRegister(origin); err != nil {
		return err
	}

	dest, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}

	c.locationStack.markRegisterUsed(dest)

	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// Check if the most significant bit (sign bit) is set.
	c.assembler.CompileRegisterToRegister(amd64.TESTQ, origin.register, origin.register)

	// Jump if the sign bit is set.
	jmpIfSignbitSet := c.assembler.CompileJump(amd64.JMI)

	// Otherwise, we could fit the unsigned int into float32.
	// So, we convert it to float32 and emit jump instruction to exit from this branch.
	if isFloat32bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTSQ2SS, origin.register, dest)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTSQ2SD, origin.register, dest)
	}
	exitFromSignbitUnSet := c.assembler.CompileJump(amd64.JMP)

	// Now handling the case where sign-bit is set.
	// We emit the following sequences:
	// 	   mov     tmpReg, origin
	// 	   shr     tmpReg, 1
	// 	   and     origin, 1
	// 	   or      tmpReg, origin
	// 	   cvtsi2ss        xmm0, tmpReg
	// 	   addsd   xmm0, xmm0

	c.assembler.SetJumpTargetOnNext(jmpIfSignbitSet)
	c.assembler.CompileRegisterToRegister(amd64.MOVQ, origin.register, tmpReg)
	c.assembler.CompileConstToRegister(amd64.SHRQ, 1, tmpReg)
	c.assembler.CompileConstToRegister(amd64.ANDQ, 1, origin.register)
	c.assembler.CompileRegisterToRegister(amd64.ORQ, origin.register, tmpReg)
	if isFloat32bit {
		c.assembler.CompileRegisterToRegister(amd64.CVTSQ2SS, tmpReg, dest)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.CVTSQ2SD, tmpReg, dest)
	}
	if isFloat32bit {
		c.assembler.CompileRegisterToRegister(amd64.ADDSS, dest, dest)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.ADDSD, dest, dest)
	}

	// Now, we finished the sign-bit set branch.
	// We have to make the exit jump target of sign-bit unset branch
	// towards the next instruction.
	c.assembler.SetJumpTargetOnNext(exitFromSignbitUnSet)

	// We consumed the origin's register and placed the conversion result
	// in the dest register.
	c.locationStack.markRegisterUnused(origin.register)
	loc := c.pushValueLocationOnRegister(dest)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)
	return nil
}

// compileSimpleConversion pops a value type from the stack, and applies the
// given instruction on it, and push the result onto a register of the given type.
func (c *amd64Compiler) compileSimpleConversion(convInstruction asm.Instruction, destinationRegisterType generalPurposeRegisterType) error {
	origin := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(origin); err != nil {
		return err
	}

	dest, err := c.allocateRegister(destinationRegisterType)
	if err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(convInstruction, origin.register, dest)

	c.locationStack.markRegisterUnused(origin.register)
	loc := c.pushValueLocationOnRegister(dest)
	loc.setRegisterType(destinationRegisterType)
	return nil
}

// compileF32DemoteFromF64 implements compiler.compileF32DemoteFromF64 for the amd64 architecture.
func (c *amd64Compiler) compileF32DemoteFromF64() error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.CVTSD2SS, target.register, target.register)
	return nil
}

// compileF64PromoteFromF32 implements compiler.compileF64PromoteFromF32 for the amd64 architecture.
func (c *amd64Compiler) compileF64PromoteFromF32() error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.CVTSS2SD, target.register, target.register)
	return nil
}

// compileI32ReinterpretFromF32 implements compiler.compileI32ReinterpretFromF32 for the amd64 architecture.
func (c *amd64Compiler) compileI32ReinterpretFromF32() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeInt)
		return nil
	}
	return c.compileSimpleConversion(amd64.MOVL, generalPurposeRegisterTypeInt)
}

// compileI64ReinterpretFromF64 implements compiler.compileI64ReinterpretFromF64 for the amd64 architecture.
func (c *amd64Compiler) compileI64ReinterpretFromF64() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeInt)
		return nil
	}
	return c.compileSimpleConversion(amd64.MOVQ, generalPurposeRegisterTypeInt)
}

// compileF32ReinterpretFromI32 implements compiler.compileF32ReinterpretFromI32 for the amd64 architecture.
func (c *amd64Compiler) compileF32ReinterpretFromI32() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeFloat)
		return nil
	}
	return c.compileSimpleConversion(amd64.MOVL, generalPurposeRegisterTypeFloat)
}

// compileF64ReinterpretFromI64 implements compiler.compileF64ReinterpretFromI64 for the amd64 architecture.
func (c *amd64Compiler) compileF64ReinterpretFromI64() error {
	if peek := c.locationStack.peek(); peek.onStack() {
		// If the value is on the stack, this is no-op as there is nothing to do for converting type.
		peek.setRegisterType(generalPurposeRegisterTypeFloat)
		return nil
	}
	return c.compileSimpleConversion(amd64.MOVQ, generalPurposeRegisterTypeFloat)
}

// compileExtend implements compiler.compileExtend for the amd64 architecture.
func (c *amd64Compiler) compileExtend(o *wazeroir.OperationExtend) error {
	var inst asm.Instruction
	if o.Signed {
		inst = amd64.MOVLQSX // = MOVSXD https://www.felixcloutier.com/x86/movsx:movsxd
	} else {
		inst = amd64.MOVQ
	}
	return c.compileExtendImpl(inst)
}

// compileSignExtend32From8 implements compiler.compileSignExtend32From8 for the amd64 architecture.
func (c *amd64Compiler) compileSignExtend32From8() error {
	return c.compileExtendImpl(amd64.MOVBLSX)
}

// compileSignExtend32From16 implements compiler.compileSignExtend32From16 for the amd64 architecture.
func (c *amd64Compiler) compileSignExtend32From16() error {
	return c.compileExtendImpl(amd64.MOVWLSX)
}

// compileSignExtend64From8 implements compiler.compileSignExtend64From8 for the amd64 architecture.
func (c *amd64Compiler) compileSignExtend64From8() error {
	return c.compileExtendImpl(amd64.MOVBQSX)
}

// compileSignExtend64From16 implements compiler.compileSignExtend64From16 for the amd64 architecture.
func (c *amd64Compiler) compileSignExtend64From16() error {
	return c.compileExtendImpl(amd64.MOVWQSX)
}

// compileSignExtend64From32 implements compiler.compileSignExtend64From32 for the amd64 architecture.
func (c *amd64Compiler) compileSignExtend64From32() error {
	return c.compileExtendImpl(amd64.MOVLQSX)
}

func (c *amd64Compiler) compileExtendImpl(inst asm.Instruction) error {
	target := c.locationStack.peek() // Note this is peek!
	if err := c.compileEnsureOnGeneralPurposeRegister(target); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(inst, target.register, target.register)
	return nil
}

// compileEq implements compiler.compileEq for the amd64 architecture.
func (c *amd64Compiler) compileEq(o *wazeroir.OperationEq) error {
	return c.compileEqOrNe(o.Type, true)
}

// compileNe implements compiler.compileNe for the amd64 architecture.
func (c *amd64Compiler) compileNe(o *wazeroir.OperationNe) error {
	return c.compileEqOrNe(o.Type, false)
}

func (c *amd64Compiler) compileEqOrNe(t wazeroir.UnsignedType, shouldEqual bool) (err error) {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	switch t {
	case wazeroir.UnsignedTypeI32:
		err = c.compileEqOrNeForInts(x1.register, x2.register, amd64.CMPL, shouldEqual)
	case wazeroir.UnsignedTypeI64:
		err = c.compileEqOrNeForInts(x1.register, x2.register, amd64.CMPQ, shouldEqual)
	case wazeroir.UnsignedTypeF32:
		err = c.compileEqOrNeForFloats(x1.register, x2.register, amd64.UCOMISS, shouldEqual)
	case wazeroir.UnsignedTypeF64:
		err = c.compileEqOrNeForFloats(x1.register, x2.register, amd64.UCOMISD, shouldEqual)
	}
	if err != nil {
		return
	}

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)
	return
}

func (c *amd64Compiler) compileEqOrNeForInts(x1Reg, x2Reg asm.Register, cmpInstruction asm.Instruction, shouldEqual bool) error {
	c.assembler.CompileRegisterToRegister(cmpInstruction, x2Reg, x1Reg)

	// Record that the result is on the conditional register.
	var condReg asm.ConditionalRegisterState
	if shouldEqual {
		condReg = amd64.ConditionalRegisterStateE
	} else {
		condReg = amd64.ConditionalRegisterStateNE
	}
	loc := c.locationStack.pushValueLocationOnConditionalRegister(condReg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// For float EQ and NE, we have to take NaN values into account.
// Notably, Wasm specification states that if one of targets is NaN,
// the result must be zero for EQ or one for NE.
func (c *amd64Compiler) compileEqOrNeForFloats(x1Reg, x2Reg asm.Register, cmpInstruction asm.Instruction, shouldEqual bool) error {
	// Before we allocate the result, we have to reserve two int registers.
	nanFragReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	c.locationStack.markRegisterUsed(nanFragReg)
	cmpResultReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// Then, execute the comparison.
	c.assembler.CompileRegisterToRegister(cmpInstruction, x2Reg, x1Reg)

	// First, we get the parity flag which indicates whether one of values was NaN.
	if shouldEqual {
		// Set 1 if two values are NOT NaN.
		c.assembler.CompileNoneToRegister(amd64.SETPC, nanFragReg)
	} else {
		// Set 1 if one of values is NaN.
		c.assembler.CompileNoneToRegister(amd64.SETPS, nanFragReg)
	}

	// Next, we get the usual comparison flag.
	if shouldEqual {
		// Set 1 if equal.
		c.assembler.CompileNoneToRegister(amd64.SETEQ, cmpResultReg)
	} else {
		// Set 1 if not equal.
		c.assembler.CompileNoneToRegister(amd64.SETNE, cmpResultReg)
	}

	// Do "and" or "or" operations on these two flags to get the actual result.
	if shouldEqual {
		c.assembler.CompileRegisterToRegister(amd64.ANDL, nanFragReg, cmpResultReg)
	} else {
		c.assembler.CompileRegisterToRegister(amd64.ORL, nanFragReg, cmpResultReg)
	}

	// Clear the unnecessary bits by zero extending the first byte.
	// This is necessary the upper bits (5 to 32 bits) of SET* instruction result is undefined.
	c.assembler.CompileRegisterToRegister(amd64.MOVBLZX, cmpResultReg, cmpResultReg)

	// Now we have the result in cmpResultReg register, so we record it.
	loc := c.pushValueLocationOnRegister(cmpResultReg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	// Also, we no longer need nanFragRegister.
	c.locationStack.markRegisterUnused(nanFragReg)
	return nil
}

// compileEqz implements compiler.compileEqz for the amd64 architecture.
func (c *amd64Compiler) compileEqz(o *wazeroir.OperationEqz) error {
	v := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	switch o.Type {
	case wazeroir.UnsignedInt32:
		c.assembler.CompileMemoryToRegister(amd64.CMPL, asm.NilRegister, int64(zero64BitAddress), v.register)
	case wazeroir.UnsignedInt64:
		c.assembler.CompileMemoryToRegister(amd64.CMPQ, asm.NilRegister, int64(zero64BitAddress), v.register)
	}

	// v is consumed by the cmp operation so release it.
	c.locationStack.releaseRegister(v)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueLocationOnConditionalRegister(amd64.ConditionalRegisterStateE)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileLt implements compiler.compileLt for the amd64 architecture.
func (c *amd64Compiler) compileLt(o *wazeroir.OperationLt) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	var resultConditionState asm.ConditionalRegisterState
	var inst asm.Instruction
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = amd64.ConditionalRegisterStateL
		inst = amd64.CMPL
	case wazeroir.SignedTypeUint32:
		resultConditionState = amd64.ConditionalRegisterStateB
		inst = amd64.CMPL
	case wazeroir.SignedTypeInt64:
		inst = amd64.CMPQ
		resultConditionState = amd64.ConditionalRegisterStateL
	case wazeroir.SignedTypeUint64:
		resultConditionState = amd64.ConditionalRegisterStateB
		inst = amd64.CMPQ
	case wazeroir.SignedTypeFloat32:
		resultConditionState = amd64.ConditionalRegisterStateA
		inst = amd64.COMISS
	case wazeroir.SignedTypeFloat64:
		resultConditionState = amd64.ConditionalRegisterStateA
		inst = amd64.COMISD
	}
	c.assembler.CompileRegisterToRegister(inst, x1.register, x2.register)

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueLocationOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileGt implements compiler.compileGt for the amd64 architecture.
func (c *amd64Compiler) compileGt(o *wazeroir.OperationGt) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	var resultConditionState asm.ConditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = amd64.ConditionalRegisterStateG
		c.assembler.CompileRegisterToRegister(amd64.CMPL, x1.register, x2.register)
	case wazeroir.SignedTypeUint32:
		c.assembler.CompileRegisterToRegister(amd64.CMPL, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateA
	case wazeroir.SignedTypeInt64:
		c.assembler.CompileRegisterToRegister(amd64.CMPQ, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateG
	case wazeroir.SignedTypeUint64:
		c.assembler.CompileRegisterToRegister(amd64.CMPQ, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateA
	case wazeroir.SignedTypeFloat32:
		c.assembler.CompileRegisterToRegister(amd64.UCOMISS, x2.register, x1.register)
		resultConditionState = amd64.ConditionalRegisterStateA
	case wazeroir.SignedTypeFloat64:
		c.assembler.CompileRegisterToRegister(amd64.UCOMISD, x2.register, x1.register)
		resultConditionState = amd64.ConditionalRegisterStateA
	}

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueLocationOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileLe implements compiler.compileLe for the amd64 architecture.
func (c *amd64Compiler) compileLe(o *wazeroir.OperationLe) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	var inst asm.Instruction
	var resultConditionState asm.ConditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		resultConditionState = amd64.ConditionalRegisterStateLE
		inst = amd64.CMPL
	case wazeroir.SignedTypeUint32:
		resultConditionState = amd64.ConditionalRegisterStateBE
		inst = amd64.CMPL
	case wazeroir.SignedTypeInt64:
		resultConditionState = amd64.ConditionalRegisterStateLE
		inst = amd64.CMPQ
	case wazeroir.SignedTypeUint64:
		resultConditionState = amd64.ConditionalRegisterStateBE
		inst = amd64.CMPQ
	case wazeroir.SignedTypeFloat32:
		resultConditionState = amd64.ConditionalRegisterStateAE
		inst = amd64.UCOMISS
	case wazeroir.SignedTypeFloat64:
		resultConditionState = amd64.ConditionalRegisterStateAE
		inst = amd64.UCOMISD
	}
	c.assembler.CompileRegisterToRegister(inst, x1.register, x2.register)

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueLocationOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileGe implements compiler.compileGe for the amd64 architecture.
func (c *amd64Compiler) compileGe(o *wazeroir.OperationGe) error {
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// Emit the compare instruction.
	var resultConditionState asm.ConditionalRegisterState
	switch o.Type {
	case wazeroir.SignedTypeInt32:
		c.assembler.CompileRegisterToRegister(amd64.CMPL, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateGE
	case wazeroir.SignedTypeUint32:
		c.assembler.CompileRegisterToRegister(amd64.CMPL, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateAE
	case wazeroir.SignedTypeInt64:
		c.assembler.CompileRegisterToRegister(amd64.CMPQ, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateGE
	case wazeroir.SignedTypeUint64:
		c.assembler.CompileRegisterToRegister(amd64.CMPQ, x1.register, x2.register)
		resultConditionState = amd64.ConditionalRegisterStateAE
	case wazeroir.SignedTypeFloat32:
		c.assembler.CompileRegisterToRegister(amd64.COMISS, x2.register, x1.register)
		resultConditionState = amd64.ConditionalRegisterStateAE
	case wazeroir.SignedTypeFloat64:
		c.assembler.CompileRegisterToRegister(amd64.COMISD, x2.register, x1.register)
		resultConditionState = amd64.ConditionalRegisterStateAE
	}

	// x1 and x2 are temporary registers only used for the cmp operation. Release them.
	c.locationStack.releaseRegister(x1)
	c.locationStack.releaseRegister(x2)

	// Finally, record that the result is on the conditional register.
	loc := c.locationStack.pushValueLocationOnConditionalRegister(resultConditionState)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileLoad implements compiler.compileLoad for the amd64 architecture.
func (c *amd64Compiler) compileLoad(o *wazeroir.OperationLoad) error {
	var (
		isIntType         bool
		movInst           asm.Instruction
		targetSizeInBytes int64
	)
	switch o.Type {
	case wazeroir.UnsignedTypeI32:
		isIntType = true
		movInst = amd64.MOVL
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeI64:
		isIntType = true
		movInst = amd64.MOVQ
		targetSizeInBytes = 64 / 8
	case wazeroir.UnsignedTypeF32:
		isIntType = false
		movInst = amd64.MOVL
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeF64:
		isIntType = false
		movInst = amd64.MOVQ
		targetSizeInBytes = 64 / 8
	}

	reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	if isIntType {
		// For integer types, read the corresponding bytes from the offset to the memory
		// and store the value to the int register.
		c.assembler.CompileMemoryWithIndexToRegister(movInst,
			// we access memory as memory.Buffer[ceil-targetSizeInBytes: ceil].
			amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
			reg)
		top := c.pushValueLocationOnRegister(reg)
		top.setRegisterType(generalPurposeRegisterTypeInt)
	} else {
		// For float types, we read the value to the float register.
		floatReg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithIndexToRegister(movInst,
			// we access memory as memory.Buffer[ceil-targetSizeInBytes: ceil].
			amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
			floatReg)
		top := c.pushValueLocationOnRegister(floatReg)
		top.setRegisterType(generalPurposeRegisterTypeFloat)
		// We no longer need the int register so mark it unused.
		c.locationStack.markRegisterUnused(reg)
	}
	return nil
}

// compileLoad8 implements compiler.compileLoad8 for the amd64 architecture.
func (c *amd64Compiler) compileLoad8(o *wazeroir.OperationLoad8) error {
	const targetSizeInBytes = 1
	reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	// Then move a byte at the offset to the register.
	// Note that Load8 is only for integer types.
	var inst asm.Instruction
	switch o.Type {
	case wazeroir.SignedInt32:
		inst = amd64.MOVBLSX
	case wazeroir.SignedUint32:
		inst = amd64.MOVBLZX
	case wazeroir.SignedInt64:
		inst = amd64.MOVBQSX
	case wazeroir.SignedUint64:
		inst = amd64.MOVBQZX
	}

	c.assembler.CompileMemoryWithIndexToRegister(inst,
		// we access memory as memory.Buffer[ceil-targetSizeInBytes: ceil].
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
		reg)

	top := c.pushValueLocationOnRegister(reg)

	// The result of load8 is always int type.
	top.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileLoad16 implements compiler.compileLoad16 for the amd64 architecture.
func (c *amd64Compiler) compileLoad16(o *wazeroir.OperationLoad16) error {
	const targetSizeInBytes = 16 / 8
	reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	// Then move 2 bytes at the offset to the register.
	// Note that Load16 is only for integer types.
	var inst asm.Instruction
	switch o.Type {
	case wazeroir.SignedInt32:
		inst = amd64.MOVWLSX
	case wazeroir.SignedInt64:
		inst = amd64.MOVWQSX
	case wazeroir.SignedUint32:
		inst = amd64.MOVWLZX
	case wazeroir.SignedUint64:
		inst = amd64.MOVWQZX
	}
	c.assembler.CompileMemoryWithIndexToRegister(inst,
		// we access memory as memory.Buffer[ceil-targetSizeInBytes: ceil].
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
		reg)

	top := c.pushValueLocationOnRegister(reg)
	// The result of load16 is always int type.
	top.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileLoad32 implements compiler.compileLoad32 for the amd64 architecture.
func (c *amd64Compiler) compileLoad32(o *wazeroir.OperationLoad32) error {
	const targetSizeInBytes = 32 / 8
	reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	// Then move 4 bytes at the offset to the register.
	var inst asm.Instruction
	if o.Signed {
		inst = amd64.MOVLQSX
	} else {
		inst = amd64.MOVLQZX
	}
	c.assembler.CompileMemoryWithIndexToRegister(inst,
		// We access memory as memory.Buffer[ceil-targetSizeInBytes: ceil].
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
		reg)
	top := c.pushValueLocationOnRegister(reg)

	// The result of load32 is always int type.
	top.setRegisterType(generalPurposeRegisterTypeInt)
	return nil
}

// compileMemoryAccessCeilSetup pops the top value from the stack (called "base"), stores "base + offsetArg + targetSizeInBytes"
// into a register, and returns the stored register. We call the result "ceil" because we access the memory
// as memory.Buffer[ceil-targetSizeInBytes: ceil].
//
// Note: this also emits the instructions to check the out of bounds memory access.
// In other words, if the ceil exceeds the memory size, the code exits with jitCallStatusCodeMemoryOutOfBounds status.
func (c *amd64Compiler) compileMemoryAccessCeilSetup(offsetArg uint32, targetSizeInBytes int64) (asm.Register, error) {
	base := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(base); err != nil {
		return 0, err
	}

	result := base.register
	if offsetConst := int64(offsetArg) + targetSizeInBytes; offsetConst <= math.MaxUint32 {
		c.assembler.CompileConstToRegister(amd64.ADDQ, offsetConst, result)
	} else {
		// If the offset const is too large, we exit with jitCallStatusCodeMemoryOutOfBounds.
		c.compileExitFromNativeCode(jitCallStatusCodeMemoryOutOfBounds)
		return result, nil
	}

	// Now we compare the value with the memory length which is held by callEngine.
	c.assembler.CompileMemoryToRegister(amd64.CMPQ, amd64ReservedRegisterForCallEngine, callEngineModuleContextMemorySliceLenOffset, result)

	// Jump if the value is within the memory length.
	okJmp := c.assembler.CompileJump(amd64.JCC)

	// Otherwise, we exit the function with out of bounds status code.
	c.compileExitFromNativeCode(jitCallStatusCodeMemoryOutOfBounds)

	c.assembler.SetJumpTargetOnNext(okJmp)

	c.locationStack.markRegisterUnused(result)
	return result, nil
}

// compileStore implements compiler.compileStore for the amd64 architecture.
func (c *amd64Compiler) compileStore(o *wazeroir.OperationStore) error {
	var movInst asm.Instruction
	var targetSizeInByte int64
	switch o.Type {
	case wazeroir.UnsignedTypeI32, wazeroir.UnsignedTypeF32:
		movInst = amd64.MOVL
		targetSizeInByte = 32 / 8
	case wazeroir.UnsignedTypeI64, wazeroir.UnsignedTypeF64:
		movInst = amd64.MOVQ
		targetSizeInByte = 64 / 8
	}
	return c.compileStoreImpl(o.Arg.Offset, movInst, targetSizeInByte)
}

// compileStore8 implements compiler.compileStore8 for the amd64 architecture.
func (c *amd64Compiler) compileStore8(o *wazeroir.OperationStore8) error {
	return c.compileStoreImpl(o.Arg.Offset, amd64.MOVB, 1)
}

// compileStore32 implements compiler.compileStore32 for the amd64 architecture.
func (c *amd64Compiler) compileStore16(o *wazeroir.OperationStore16) error {
	return c.compileStoreImpl(o.Arg.Offset, amd64.MOVW, 16/8)
}

// compileStore32 implements compiler.compileStore32 for the amd64 architecture.
func (c *amd64Compiler) compileStore32(o *wazeroir.OperationStore32) error {
	return c.compileStoreImpl(o.Arg.Offset, amd64.MOVL, 32/8)
}

func (c *amd64Compiler) compileStoreImpl(offsetConst uint32, inst asm.Instruction, targetSizeInBytes int64) error {
	val := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offsetConst, targetSizeInBytes)
	if err != nil {
		return nil
	}

	c.assembler.CompileRegisterToMemoryWithIndex(
		inst, val.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
	)

	// We no longer need both the value and base registers.
	c.locationStack.releaseRegister(val)
	c.locationStack.markRegisterUnused(reg)
	return nil
}

// compileMemoryGrow implements compiler.compileMemoryGrow for the amd64 architecture.
func (c *amd64Compiler) compileMemoryGrow() error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	if err := c.compileCallBuiltinFunction(builtinFunctionIndexMemoryGrow); err != nil {
		return err
	}

	// After the function call, we have to initialize the stack base pointer and memory reserved registers.
	c.compileReservedStackBasePointerInitialization()
	c.compileReservedMemoryPointerInitialization()

	return nil
}

// compileMemorySize implements compiler.compileMemorySize for the amd64 architecture.
func (c *amd64Compiler) compileMemorySize() error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := c.pushValueLocationOnRegister(reg)

	c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForCallEngine, callEngineModuleContextMemorySliceLenOffset, loc.register)

	// WebAssembly's memory.size returns the page size (65536) of memory region.
	// That is equivalent to divide the len of memory slice by 65536 and
	// that can be calculated as SHR by 16 bits as 65536 = 2^16.
	c.assembler.CompileConstToRegister(amd64.SHRQ, wasm.MemoryPageSizeInBits, loc.register)
	return nil
}

// compileConstI32 implements compiler.compileConstI32 for the amd64 architecture.
func (c *amd64Compiler) compileConstI32(o *wazeroir.OperationConstI32) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := c.pushValueLocationOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)

	c.assembler.CompileConstToRegister(amd64.MOVL, int64(o.Value), reg)
	return nil
}

// compileConstI64 implements compiler.compileConstI64 for the amd64 architecture.
func (c *amd64Compiler) compileConstI64(o *wazeroir.OperationConstI64) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	reg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := c.pushValueLocationOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)

	c.assembler.CompileConstToRegister(amd64.MOVQ, int64(o.Value), reg)
	return nil
}

// compileConstF32 implements compiler.compileConstF32 for the amd64 architecture.
func (c *amd64Compiler) compileConstF32(o *wazeroir.OperationConstF32) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	reg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}
	loc := c.pushValueLocationOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	c.assembler.CompileConstToRegister(amd64.MOVL, int64(uint64(math.Float32bits(o.Value))), tmpReg)
	c.assembler.CompileRegisterToRegister(amd64.MOVL, tmpReg, reg)
	return nil
}

// compileConstF64 implements compiler.compileConstF64 for the amd64 architecture.
func (c *amd64Compiler) compileConstF64(o *wazeroir.OperationConstF64) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	reg, err := c.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}
	loc := c.pushValueLocationOnRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeFloat)

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := c.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	c.assembler.CompileConstToRegister(amd64.MOVQ, int64(math.Float64bits(o.Value)), tmpReg)
	c.assembler.CompileRegisterToRegister(amd64.MOVQ, tmpReg, reg)
	return nil
}

func (c *amd64Compiler) compileLoadValueOnStackToRegister(loc *valueLocation) {
	// Copy the value from the stack.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		// Note: stack pointers are ensured not to exceed 2^27 so this offset never exceeds 32-bit range.
		amd64ReservedRegisterForStackBasePointerAddress, int64(loc.stackPointer)*8,
		loc.register)
}

// maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister moves the top value on the stack
// if the value is located on a conditional register.
//
// This is usually called at the beginning of methods on compiler interface where we possibly
// compile instructions without saving the conditional register value.
// The compile* functions without calling this function is saving the conditional
// value to the stack or register by invoking compileEnsureOnGeneralPurposeRegister for the top.
func (c *amd64Compiler) maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister() {
	if c.locationStack.sp > 0 {
		if loc := c.locationStack.peek(); loc.onConditionalRegister() {
			c.compileLoadConditionalRegisterToGeneralPurposeRegister(loc)
		}
	}
}

// loadConditionalRegisterToGeneralPurposeRegister saves the conditional register value
// to a general purpose register.
func (c *amd64Compiler) compileLoadConditionalRegisterToGeneralPurposeRegister(loc *valueLocation) {
	// Get the free register.
	reg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	c.compileMoveConditionalToGeneralPurposeRegister(loc, reg)
}

func (c *amd64Compiler) compileMoveConditionalToGeneralPurposeRegister(loc *valueLocation, reg asm.Register) {
	// Set the flag bit to the destination. See
	// - https://c9x.me/x86/html/file_module_x86_id_288.html
	// - https://github.com/golang/go/blob/master/src/cmd/internal/obj/x86/asm6.go#L1453-L1468
	// to translate conditionalRegisterState* to amd64.SET*
	var inst asm.Instruction
	switch loc.conditionalRegister {
	case amd64.ConditionalRegisterStateE:
		inst = amd64.SETEQ
	case amd64.ConditionalRegisterStateNE:
		inst = amd64.SETNE
	case amd64.ConditionalRegisterStateS:
		inst = amd64.SETMI
	case amd64.ConditionalRegisterStateNS:
		inst = amd64.SETPL
	case amd64.ConditionalRegisterStateG:
		inst = amd64.SETGT
	case amd64.ConditionalRegisterStateGE:
		inst = amd64.SETGE
	case amd64.ConditionalRegisterStateL:
		inst = amd64.SETLT
	case amd64.ConditionalRegisterStateLE:
		inst = amd64.SETLE
	case amd64.ConditionalRegisterStateA:
		inst = amd64.SETHI
	case amd64.ConditionalRegisterStateAE:
		inst = amd64.SETCC
	case amd64.ConditionalRegisterStateB:
		inst = amd64.SETCS
	case amd64.ConditionalRegisterStateBE:
		inst = amd64.SETLS
	}

	c.assembler.CompileNoneToRegister(inst, reg)

	// Then we reset the unnecessary bit.
	c.assembler.CompileConstToRegister(amd64.ANDQ, 0x1, reg)

	// Mark it uses the register.
	loc.setRegister(reg)
	loc.setRegisterType(generalPurposeRegisterTypeInt)
	c.locationStack.markRegisterUsed(reg)
}

// allocateRegister returns an unused register of the given type. The register will be taken
// either from the free register pool or by stealing an used register.
// Note that resulting registers are NOT marked as used so the call site should
// mark it used if necessary.
func (c *amd64Compiler) allocateRegister(t generalPurposeRegisterType) (reg asm.Register, err error) {
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
	c.compileReleaseRegisterToStack(stealTarget)
	return
}

// callFunction adds instructions to call a function whose address equals either addr parameter or the value on indexReg.
// Pass indexReg == asm.NilRegister to indicate that use addr argument as the source of target function's address.
// Otherwise, the added code tries to read the function address from the register for indexReg argument.
//
// Note: this is the counter part for returnFunction, and see the comments there as well
// to understand how the function calls are achieved.
func (c *amd64Compiler) compileCallFunctionImpl(index wasm.Index, compiledFunctionAddressRegister asm.Register, functype *wasm.FunctionType) error {
	// Release all the registers as our calling convention requires the caller-save.
	c.compileReleaseAllRegistersToStack()

	// First, we have to make sure that
	if !isNilRegister(compiledFunctionAddressRegister) {
		c.locationStack.markRegisterUsed(compiledFunctionAddressRegister)
	}

	// Obtain the temporary registers to be used in the followings.
	freeRegs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 4)
	if !found {
		// This in theory never happen as all the registers must be free except compiledFunctionAddressRegister.
		return fmt.Errorf("could not find enough free registers")
	}
	c.locationStack.markRegisterUsed(freeRegs...)

	// Alias these free tmp registers for readability.
	callFrameStackPointerRegister, tmpRegister, targetAddressRegister,
		callFrameStackTopAddressRegister := freeRegs[0], freeRegs[1], freeRegs[2], freeRegs[3]

	// First, we read the current call frame stack pointer.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		callFrameStackPointerRegister)

	// And compare it with the underlying slice length.
	c.assembler.CompileMemoryToRegister(amd64.CMPQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackLenOffset, callFrameStackPointerRegister)

	// If they do not equal, then we don't have to grow the call frame stack.
	jmpIfNotCallFrameStackNeedsGrow := c.assembler.CompileJump(amd64.JNE)

	// Otherwise, we have to make the builtin function call to grow the call frame stack.
	if !isNilRegister(compiledFunctionAddressRegister) {
		// If we need to get the target funcaddr from register (call_indirect case), we must save it before growing the
		// call-frame stack, as the register is not saved across function calls.
		savedOffsetLocation := c.pushValueLocationOnRegister(compiledFunctionAddressRegister)
		c.compileReleaseRegisterToStack(savedOffsetLocation)
	}

	// Grow the stack.
	if err := c.compileCallBuiltinFunction(builtinFunctionIndexGrowCallFrameStack); err != nil {
		return err
	}

	// For call_indirect, we need to push the value back to the register.
	if !isNilRegister(compiledFunctionAddressRegister) {
		// Since this is right after callGoFunction, we have to initialize the stack base pointer
		// to properly load the value on memory stack.
		c.compileReservedStackBasePointerInitialization()

		savedOffsetLocation := c.locationStack.pop()
		savedOffsetLocation.setRegister(compiledFunctionAddressRegister)
		c.compileLoadValueOnStackToRegister(savedOffsetLocation)
	}

	// Also we have to re-read the call frame stack pointer into callFrameStackPointerRegister.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		callFrameStackPointerRegister)

	// Now that call-frame stack is enough length, we are ready to create a new call frame
	// for the function call we are about to make.
	c.assembler.SetJumpTargetOnNext(jmpIfNotCallFrameStackNeedsGrow)
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackElement0AddressOffset,
		tmpRegister)

	// Since call frame stack pointer is the index for callEngine.callFrameStack slice,
	// here we get the actual offset in bytes via shifting callFrameStackPointerRegister by callFrameDataSizeMostSignificantSetBit.
	// That is valid because the size of callFrame struct is a power of 2 (see TestVerifyOffsetValue), which means
	// multiplying withe the size of struct equals shifting by its most significant bit.
	c.assembler.CompileConstToRegister(amd64.SHLQ, int64(callFrameDataSizeMostSignificantSetBit), callFrameStackPointerRegister)

	// At this point, callFrameStackPointerRegister holds the offset in call frame slice in bytes,
	// and tmpRegister holds the absolute address of the first item of call frame slice.
	// To illustrate the situation:
	//
	//  tmpRegister (holding the absolute address of &callFrame[0])
	//      |
	//      [ra.0, rb.0, rc.0, _, ra.1, rb.1, rc.1, _, ra.next, rb.next, rc.next, ...]  <--- call frame stack's data region (somewhere in the memory)
	//      |                                        |
	//      |---------------------------------------->
	//          callFrameStackPointerRegister (holding the offset from &callFrame[0] in bytes.)
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
	//   2) Set callEngine.valueStackContext.stackBasePointer for the next function.
	//   3) Set rc.next to specify which function is executed on the current call frame (needs to make builtin function calls).
	//   4) Set ra.1 so that we can return back to this function properly.

	// First, read the address corresponding to tmpRegister+callFrameStackPointerRegister
	// by LEA instruction which equals the address of call frame stack top.
	c.assembler.CompileMemoryWithIndexToRegister(amd64.LEAQ,
		tmpRegister, 0, callFrameStackPointerRegister, 1,
		callFrameStackTopAddressRegister)

	// 1) Set rb.1 so that we can return back to this function properly.
	{
		// We must save the current stack base pointer (which lives on callEngine.valueStackContext.stackPointer)
		// to the call frame stack. In the example, this is equivalent to writing the value into "rb.1".
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset, tmpRegister)

		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister,
			// "rb.1" is BELOW the top address. See the above example for detail.
			callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset),
		)
	}

	// 2) Set callEngine.valueStackContext.stackBasePointer for the next function.
	if offset := int64(c.locationStack.sp) - int64(len(functype.Params)); offset > 0 {
		// At this point, tmpRegister holds the old stack base pointer. We could get the new frame's
		// stack base pointer by "old stack base pointer + old stack pointer - # of function params"
		// See the comments in callEngine.pushCallFrame which does exactly the same calculation in Go.
		c.assembler.CompileConstToRegister(amd64.ADDQ, offset, tmpRegister)

		// Write the calculated value to callEngine.valueStackContext.stackBasePointer.
		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister, amd64ReservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset)
	}

	// 3) Set rc.next to specify which function is executed on the current call frame (needs to make builtin function calls).
	{
		if isNilRegister(compiledFunctionAddressRegister) {
			// We must set the target function's address(pointer) of *compiledFunction into the next call-frame stack.
			// In the example, this is equivalent to writing the value into "rc.next".
			//
			// First, we read the address of the first item of callEngine.compiledFunctions slice (= &callEngine.compiledFunctions[0])
			// into tmpRegister.
			c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForCallEngine, callEngineModuleContextCompiledFunctionsElement0AddressOffset, tmpRegister)

			// Next, read the address of the target function (= &callEngine.compiledFunctions[offset])
			// into targetAddressRegister.
			c.assembler.CompileMemoryToRegister(amd64.MOVQ,
				// Note: FunctionIndex is limited up to 2^27 so this offset never exceeds 32-bit integer.
				// *8 because the size of *compiledFunction equals 8 bytes.
				tmpRegister, int64(index)*8,
				targetAddressRegister,
			)
		} else {
			targetAddressRegister = compiledFunctionAddressRegister
		}
		// Finally, we are ready to place the address of the target function's *compiledFunction into the new call-frame.
		// In the example, this is equivalent to set "rc.next".
		c.assembler.CompileRegisterToMemory(amd64.MOVQ, targetAddressRegister, callFrameStackTopAddressRegister, callFrameCompiledFunctionOffset)
	}

	// 4) Set ra.1 so that we can return back to this function properly.
	//
	// We have to set the return address for the current call frame (which is "ra.1" in the example).
	// First, Get the return address into the tmpRegister.
	c.assembler.CompileReadInstructionAddress(tmpRegister, amd64.JMP)

	// Now we are ready to set the return address to the current call frame.
	// This is equivalent to set "ra.1" in the example.
	c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister,
		callFrameStackTopAddressRegister,
		// "ra.1" is BELOW the top address. See the above example for detail.
		-(callFrameDataSize - callFrameReturnAddressOffset),
	)

	// Every preparation (1 to 5 in the description above) is done to enter into the target function.
	// So we increment the call frame stack pointer.
	c.assembler.CompileNoneToMemory(amd64.INCQ, amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset)

	// And jump into the initial address of the target function.
	c.assembler.CompileJumpToMemory(amd64.JMP, targetAddressRegister, compiledFunctionCodeInitialAddressOffset)

	// All the registers used are temporary so we mark them unused.
	c.locationStack.markRegisterUnused(freeRegs...)

	// On the function return, we have to initialize the state.
	// This could be reached after returnFunction(), so callEngine.valueStackContext.stackBasePointer
	// and callEngine.moduleContext.moduleInstanceAddress are changed (See comments in returnFunction()).
	// Therefore we have to initialize the state according to these changes.
	//
	// Due to the change to callEngine.valueStackContext.stackBasePointer.
	c.compileReservedStackBasePointerInitialization()
	// Due to the change to callEngine.moduleContext.moduleInstanceAddress.
	if err := c.compileModuleContextInitialization(); err != nil {
		return err
	}
	// Due to the change to callEngine.moduleContext.moduleInstanceAddress as that might result in
	// the memory instance manipulation.
	c.compileReservedMemoryPointerInitialization()
	return nil
}

// returnFunction adds instructions to return from the current callframe back to the caller's frame.
// If this is the current one is the origin, we return back to the callEngine.execWasmFunction with the Returned status.
// Otherwise, we jump into the callers' return address stored in callFrame.returnAddress while setting
// up all the necessary change on the callEngine's state.
//
// Note: this is the counter part for callFunction, and see the comments there as well
// to understand how the function calls are achieved.
func (c *amd64Compiler) compileReturnFunction() error {
	// Release all the registers as our calling convention requires the caller-save.
	c.compileReleaseAllRegistersToStack()

	// Obtain the temporary registers to be used in the followings.
	regs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		return fmt.Errorf("BUG: all the registers should be free at this point")
	}
	c.locationStack.markRegisterUsed(regs...)

	// Alias these free tmp registers for readability.
	decrementedCallFrameStackPointerRegister, callFrameStackTopAddressRegister, tmpRegister := regs[0], regs[1], regs[2]

	// Since we return from the function, we need to decement the callframe stack pointer.
	c.assembler.CompileNoneToMemory(amd64.DECQ, amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset)

	// Next, get the decremented callframe stack pointer into decrementedCallFrameStackPointerRegister.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset,
		decrementedCallFrameStackPointerRegister)

	// We have to exit if the decremented stack pointer equals zero.
	c.assembler.CompileRegisterToRegister(amd64.TESTQ, decrementedCallFrameStackPointerRegister, decrementedCallFrameStackPointerRegister)

	jmpIfNotCallStackPointerZero := c.assembler.CompileJump(amd64.JNE)

	// If the callframe stack pointer equals the previous one,
	// we exit the JIT call with returned status.
	c.compileExitFromNativeCode(jitCallStatusCodeReturned)

	// Otherwise, we return back to the top call frame.
	//
	// Since call frame stack pointer is the index for callEngine.callFrameStack slice,
	// here we get the actual offset in bytes via shifting decrementedCallFrameStackPointerRegister by callFrameDataSizeMostSignificantSetBit.
	// That is valid because the size of callFrame struct is a power of 2 (see TestVerifyOffsetValue), which means
	// multiplying withe the size of struct equals shifting by its most significant bit.
	c.assembler.SetJumpTargetOnNext(jmpIfNotCallStackPointerZero)
	c.assembler.CompileConstToRegister(amd64.SHLQ, int64(callFrameDataSizeMostSignificantSetBit), decrementedCallFrameStackPointerRegister)

	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackElement0AddressOffset, tmpRegister)

	c.assembler.CompileMemoryWithIndexToRegister(amd64.LEAQ,
		tmpRegister, 0, decrementedCallFrameStackPointerRegister, 1,
		callFrameStackTopAddressRegister)

	// At this point, decrementedCallFrameStackPointerRegister holds the offset in call frame slice in bytes,
	// and tmpRegister holds the absolute address of the first item of call frame slice.
	// To illustrate the situation:
	//
	//  tmpRegister (holding the absolute address of &callFrame[0])
	//      |                              callFrameStackTopAddressRegister (absolute address of tmpRegister+decrementedCallFrameStackPointerRegister)
	//      |                                           |
	//      [......., ra.caller, rb.caller, rc.caller, _, ra.current, rb.current, rc.current, _, ...]  <--- call frame stack's data region (somewhere in the memory)
	//      |                                           |
	//      |------------------------------------------->
	//           decrementedCallFrameStackPointerRegister (holding the offset from &callFrame[0] in bytes.)
	//
	// where:
	//      ra.* = callFrame.returnAddress
	//      rb.* = callFrame.returnStackBasePointer
	//      rc.* = callFrame.compiledFunction
	//      _  = callFrame's padding (see comment on callFrame._ field.)
	//
	// What we have to do in the following is that
	//   1) Set callEngine.valueStackContext.stackBasePointer to the value on "rb.caller".
	//   2) Jump into the address of "ra.caller".

	// 1) Set callEngine.valueStackContext.stackBasePointer to the value on "rb.caller"
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		// "rb.caller" is BELOW the top address. See the above example for detail.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnStackBasePointerOffset),
		tmpRegister,
	)
	c.assembler.CompileRegisterToMemory(amd64.MOVQ,
		tmpRegister, amd64ReservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset)

	// 2) Jump into the address of "ra.caller".
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		// "ra.caller" is BELOW the top address. See the above example for detail.
		callFrameStackTopAddressRegister, -(callFrameDataSize - callFrameReturnAddressOffset),
		tmpRegister,
	)

	c.assembler.CompileJumpToRegister(amd64.JMP, tmpRegister)

	// They were temporarily used, so we mark them unused.
	c.locationStack.markRegisterUnused(regs...)
	return nil
}

func (c *amd64Compiler) compileCallHostFunction() error {
	return c.compileCallGoFunction(jitCallStatusCodeCallHostFunction)
}

func (c *amd64Compiler) compileCallBuiltinFunction(index wasm.Index) error {
	// Set the functionAddress to the callEngine.exitContext functionCallAddress.
	c.assembler.CompileConstToMemory(amd64.MOVL, int64(index), amd64ReservedRegisterForCallEngine, callEngineExitContextBuiltinFunctionCallAddressOffset)
	return c.compileCallGoFunction(jitCallStatusCodeCallBuiltInFunction)
}

func (c *amd64Compiler) compileCallGoFunction(jitStatus jitCallStatusCode) error {
	// Release all the registers as our calling convention requires the caller-save.
	c.compileReleaseAllRegistersToStack()

	// Obtain the temporary registers to be used in the followings.
	regs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		// This in theory never happen as all the registers must be free except indexReg.
		return fmt.Errorf("could not find enough free registers")
	}
	c.locationStack.markRegisterUsed(regs...)

	// Alias these free tmp registers for readability.
	instructionAddressRegister, currentCallFrameAddressRegister, tmpRegister := regs[0], regs[1], regs[2]

	// We need to store the address of the current callFrame's return address.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackPointerOffset, currentCallFrameAddressRegister)

	// Next we shift the stack pointer so we get the actual offset from the address of stack's initial item.
	c.assembler.CompileConstToRegister(amd64.SHLQ, int64(callFrameDataSizeMostSignificantSetBit), currentCallFrameAddressRegister)

	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextCallFrameStackElement0AddressOffset, tmpRegister)

	// Now we can get the current call frame's address, which is equivalent to get &callEngine.callFrameStack[callEngine.callStackFramePointer-1].returnAddress.
	c.assembler.CompileMemoryWithIndexToRegister(
		amd64.LEAQ,
		tmpRegister, -(callFrameDataSize - callFrameReturnAddressOffset), currentCallFrameAddressRegister, 1,
		currentCallFrameAddressRegister,
	)

	c.assembler.CompileReadInstructionAddress(instructionAddressRegister, amd64.RET)

	// We are ready to store the return address (in instructionAddressRegister) to callEngine.callFrameStack[callEngine.callStackFramePointer-1].
	c.assembler.CompileRegisterToMemory(amd64.MOVQ, instructionAddressRegister, currentCallFrameAddressRegister, callFrameReturnAddressOffset)

	c.compileExitFromNativeCode(jitStatus)

	// They were temporarily used, so we mark them unused.
	c.locationStack.markRegisterUnused(regs...)
	return nil
}

// compileReleaseAllRegistersToStack add the instructions to release all the LIVE value
// in the value location stack at this point into the stack memory location.
func (c *amd64Compiler) compileReleaseAllRegistersToStack() {
	for i := uint64(0); i < c.locationStack.sp; i++ {
		if loc := c.locationStack.stack[i]; loc.onRegister() {
			c.compileReleaseRegisterToStack(loc)
		} else if loc.onConditionalRegister() {
			c.compileLoadConditionalRegisterToGeneralPurposeRegister(loc)
			c.compileReleaseRegisterToStack(loc)
		}
	}
}

func (c *amd64Compiler) onValueReleaseRegisterToStack(reg asm.Register) {
	for i := uint64(0); i < c.locationStack.sp; i++ {
		prevValue := c.locationStack.stack[i]
		if prevValue.register == reg {
			c.compileReleaseRegisterToStack(prevValue)
			break
		}
	}
}

func (c *amd64Compiler) compileReleaseRegisterToStack(loc *valueLocation) {
	// Push value.
	c.assembler.CompileRegisterToMemory(amd64.MOVQ, loc.register,
		// Note: stack pointers are ensured not to exceed 2^27 so this offset never exceeds 32-bit range.
		amd64ReservedRegisterForStackBasePointerAddress, int64(loc.stackPointer)*8)

	// Mark the register is free.
	c.locationStack.releaseRegister(loc)
}

func (c *amd64Compiler) compileExitFromNativeCode(status jitCallStatusCode) {
	c.assembler.CompileConstToMemory(amd64.MOVB, int64(status), amd64ReservedRegisterForCallEngine, callEngineExitContextJITCallStatusCodeOffset)

	// Write back the cached SP to the actual eng.stackPointer.
	c.assembler.CompileConstToMemory(amd64.MOVQ, int64(c.locationStack.sp), amd64ReservedRegisterForCallEngine, callEngineValueStackContextStackPointerOffset)

	c.assembler.CompileStandAlone(amd64.RET)
}

func (c *amd64Compiler) compilePreamble() (err error) {
	// We assume all function parameters are already pushed onto the stack by
	// the caller.
	c.pushFunctionParams()

	// Check if it's necessary to grow the value stack by using max stack pointer.
	if err = c.compileMaybeGrowValueStack(); err != nil {
		return err
	}

	c.compileReservedStackBasePointerInitialization()

	// Once the stack base pointer is initialized and the size of stack is ok,
	// initialize the module context next.
	if err := c.compileModuleContextInitialization(); err != nil {
		return err
	}

	// Finally, we initialize the reserved memory register based on the module context.
	c.compileReservedMemoryPointerInitialization()
	return
}

func (c *amd64Compiler) compileReservedStackBasePointerInitialization() {
	// First, make reservedRegisterForStackBasePointer point to the beginning of the slice backing array.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineGlobalContextValueStackElement0AddressOffset,
		amd64ReservedRegisterForStackBasePointerAddress)

	// Since initializeReservedRegisters is called at the beginning of function
	// calls (or right after they return), we have free registers at this point.
	tmpReg, _ := c.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)

	// Next we move the base pointer (callEngine.stackBasePointer) to the tmp register.
	c.assembler.CompileMemoryToRegister(amd64.MOVQ,
		amd64ReservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset,
		tmpReg,
	)

	c.assembler.CompileMemoryWithIndexToRegister(
		amd64.LEAQ,
		amd64ReservedRegisterForStackBasePointerAddress, 0, tmpReg, 8,
		amd64ReservedRegisterForStackBasePointerAddress,
	)
}

func (c *amd64Compiler) compileReservedMemoryPointerInitialization() {
	if c.f.Module.Memory != nil {
		c.assembler.CompileMemoryToRegister(amd64.MOVQ,
			amd64ReservedRegisterForCallEngine, callEngineModuleContextMemoryElement0AddressOffset,
			amd64ReservedRegisterForMemory,
		)
	}
}

// compileMaybeGrowValueStack adds instructions to check the necessity to grow the value stack,
// and if so, make the builtin function call to do so. These instructions are called in the function's
// preamble.
func (c *amd64Compiler) compileMaybeGrowValueStack() error {
	tmpRegister, _ := c.allocateRegister(generalPurposeRegisterTypeInt)

	c.assembler.CompileMemoryToRegister(amd64.MOVQ, amd64ReservedRegisterForCallEngine, callEngineGlobalContextValueStackLenOffset, tmpRegister)
	c.assembler.CompileMemoryToRegister(amd64.SUBQ, amd64ReservedRegisterForCallEngine, callEngineValueStackContextStackBasePointerOffset, tmpRegister)

	// If stack base pointer + max stack pointer > valueStackLen, we need to grow the stack.
	cmpWithStackPointerCeil := c.assembler.CompileRegisterToConst(amd64.CMPQ, tmpRegister, 0)
	c.onStackPointerCeilDeterminedCallBack = func(stackPointerCeil uint64) {
		cmpWithStackPointerCeil.AssignDestinationConstant(int64(stackPointerCeil))
	}

	// Jump if we have no need to grow.
	jmpIfNoNeedToGrowStack := c.assembler.CompileJump(amd64.JCC)

	// Otherwise, we have to make the builtin function call to grow the call stack.
	if err := c.compileCallBuiltinFunction(builtinFunctionIndexGrowValueStack); err != nil {
		return err
	}

	c.assembler.SetJumpTargetOnNext(jmpIfNoNeedToGrowStack)
	return nil
}

// compileModuleContextInitialization adds instructions to initialize callEngine.ModuleContext's fields based on
// callEngine.ModuleContext.ModuleInstanceAddress.
// This is called in two cases: in function preamble, and on the return from (non-Go) function calls.
func (c *amd64Compiler) compileModuleContextInitialization() error {

	// Obtain the temporary registers to be used in the followings.
	regs, found := c.locationStack.takeFreeRegisters(generalPurposeRegisterTypeInt, 3)
	if !found {
		// This in theory never happen as all the registers must be free except indexReg.
		return fmt.Errorf("could not find enough free registers")
	}
	c.locationStack.markRegisterUsed(regs...)

	// Alias these free tmp registers for readability.
	moduleInstanceAddressRegister, tmpRegister, tmpRegister2 := regs[0], regs[1], regs[2]

	c.assembler.CompileConstToRegister(amd64.MOVQ, int64(uintptr(unsafe.Pointer(c.f.Module))), moduleInstanceAddressRegister)

	// If the module instance address stays the same, we could skip the entire code below.
	// The rationale/idea for this is that, in almost all use cases, users instantiate a single
	// Wasm binary and run the functions from it, rather than doing import/export on multiple
	// binaries. As a result, this cmp and jmp instruction sequence below must be easy for
	// x64 CPU to do branch prediction since almost 100% jump happens across function calls.
	c.assembler.CompileMemoryToRegister(amd64.CMPQ,
		amd64ReservedRegisterForCallEngine, callEngineModuleContextModuleInstanceAddressOffset, moduleInstanceAddressRegister)
	jmpIfModuleNotChange := c.assembler.CompileJump(amd64.JEQ)

	// Otherwise, we need to update fields.
	// First, save the read module instance address to callEngine.moduleInstanceAddress
	c.assembler.CompileRegisterToMemory(amd64.MOVQ, moduleInstanceAddressRegister,
		amd64ReservedRegisterForCallEngine, callEngineModuleContextModuleInstanceAddressOffset)

	// Otherwise, we have to update the following fields:
	// * callEngine.moduleContext.globalElement0Address
	// * callEngine.moduleContext.tableElement0Address
	// * callEngine.moduleContext.tableSliceLen
	// * callEngine.moduleContext.memoryElement0Address
	// * callEngine.moduleContext.memorySliceLen
	// * callEngine.moduleContext.compiledFunctionsElement0Address

	// Update globalElement0Address.
	//
	// Note: if there's global.get or set instruction in the function, the existence of the globals
	// is ensured by function validation at module instantiation phase, and that's why it is ok to
	// skip the initialization if the module's globals slice is empty.
	if len(c.f.Module.Globals) > 0 {
		// Since ModuleInstance.Globals is []*globalInstance, internally
		// the address of the first item in the underlying array lies exactly on the globals offset.
		// See https://go.dev/blog/slices-intro if unfamiliar.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, moduleInstanceAddressRegister, moduleInstanceGlobalsOffset, tmpRegister)

		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister, amd64ReservedRegisterForCallEngine, callEngineModuleContextGlobalElement0AddressOffset)
	}

	// Update tableElement0Address and tableSliceLen.
	//
	// Note: if there's table instruction in the function, the existence of the table
	// is ensured by function validation at module instantiation phase, and that's
	// why it is ok to skip the initialization if the module's table doesn't exist.
	if c.f.Module.Table != nil {
		// First, we need to read the *wasm.Table.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, moduleInstanceAddressRegister, moduleInstanceTableOffset, tmpRegister)

		// At this point, tmpRegister holds the address of ModuleInstance.Table.
		// So we are ready to read and put the first item's address stored in Table.Table.
		// Here we read the value into tmpRegister2.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, tmpRegister, tableInstanceTableOffset, tmpRegister2)

		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister2,
			amd64ReservedRegisterForCallEngine, callEngineModuleContextTableElement0AddressOffset)

		// Finally, read the length of table and update tableSliceLen accordingly.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, tmpRegister, tableInstanceTableLenOffset, tmpRegister2)

		// And put the length into tableSliceLen.

		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister2,
			amd64ReservedRegisterForCallEngine, callEngineModuleContextTableSliceLenOffset)
	}

	// Update memoryElement0Address and memorySliceLen.
	//
	// Note: if there's memory instruction in the function, memory instance must be non-nil.
	// That is ensured by function validation at module instantiation phase, and that's
	// why it is ok to skip the initialization if the module's memory instance is nil.
	if c.f.Module.Memory != nil {
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, moduleInstanceAddressRegister, moduleInstanceMemoryOffset, tmpRegister)

		// Set length.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, tmpRegister, memoryInstanceBufferLenOffset, tmpRegister2)
		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister2,
			amd64ReservedRegisterForCallEngine, callEngineModuleContextMemorySliceLenOffset)

		// Set elemnt zero address.
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, tmpRegister, memoryInstanceBufferOffset, tmpRegister2)
		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister2,
			amd64ReservedRegisterForCallEngine, callEngineModuleContextMemoryElement0AddressOffset)
	}

	// Update moduleContext.compiledFunctionsElement0Address
	{
		// "tmpRegister = [moduleInstanceAddressRegister + moduleInstanceEngineOffset + interfaceDataOffset] (== *moduleEngine)"
		//
		// Go's interface is laid out on memory as two quad words as struct {tab, data uintptr}
		// where tab points to the interface table, and the latter points to the actual
		// implementation of interface. This case, we extract "data" pointer as *moduleEngine.
		// See the following references for detail:
		// * https://research.swtch.com/interfaces
		// * https://github.com/golang/go/blob/release-branch.go1.17/src/runtime/runtime2.go#L207-L210
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, moduleInstanceAddressRegister, moduleInstanceEngineOffset+interfaceDataOffset, tmpRegister)

		// "tmpRegister = [tmpRegister + moduleEngineCompiledFunctionsOffset] (== &moduleEngine.compiledFunctions[0])"
		c.assembler.CompileMemoryToRegister(amd64.MOVQ, tmpRegister, moduleEngineCompiledFunctionsOffset, tmpRegister)

		// "callEngine.moduleContext.compiledFunctionsElement0Address = tmpRegister".
		c.assembler.CompileRegisterToMemory(amd64.MOVQ, tmpRegister, amd64ReservedRegisterForCallEngine, callEngineModuleContextCompiledFunctionsElement0AddressOffset)
	}

	c.locationStack.markRegisterUnused(regs...)

	// Set the jump target towards the next instruction for the case where module instance address hasn't changed.
	c.assembler.SetJumpTargetOnNext(jmpIfModuleNotChange)
	return nil
}

// compileEnsureOnGeneralPurposeRegister ensures that the given value is located on a
// general purpose register of an appropriate type.
func (c *amd64Compiler) compileEnsureOnGeneralPurposeRegister(loc *valueLocation) error {
	if loc.onStack() {
		// Allocate the register.
		reg, err := c.allocateRegister(loc.registerType())
		if err != nil {
			return err
		}

		// Mark it uses the register.
		loc.setRegister(reg)
		c.locationStack.markRegisterUsed(reg)

		c.compileLoadValueOnStackToRegister(loc)
	} else if loc.onConditionalRegister() {
		c.compileLoadConditionalRegisterToGeneralPurposeRegister(loc)
	}
	return nil
}
