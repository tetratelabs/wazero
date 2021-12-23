//go:build amd64
// +build amd64

package jit

import (
	"encoding/binary"
	"fmt"
	"math"
	"unsafe"

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

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledWasmFunction, error) {
	ir, err := wazeroir.Compile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to lower to wazeroir: %w", err)
	}

	// We can choose arbitrary number instead of 1024 which indicates the cache size in the builder.
	// TODO: optimize the number.
	b, err := asm.NewBuilder("amd64", 1024)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}
	builder := &amd64Builder{
		eng: e, f: f, builder: b, locationStack: newValueLocationStack(), ir: ir,
		labelInitialInstructions: make(map[string]*obj.Prog),
		onLabelStartCallbacks:    make(map[string][]func(*obj.Prog)),
	}
	// Move the function inputs onto stack, as we assume that
	// all the function inputs (parameters) are already pushed on the stack
	// by the caller.
	builder.pushFunctionInputs()

	// Initialize the reserved registers first of all.
	builder.initializeReservedRegisters()
	// Now move onto the function body to compile each wazeroir operation.
	for _, op := range ir.Operations {
		switch o := op.(type) {
		case *wazeroir.OperationUnreachable:
			builder.handleUnreachable()
		case *wazeroir.OperationLabel:
			if err := builder.handleLabel(o); err != nil {
				return nil, fmt.Errorf("error handling label operation: %w", err)
			}
		case *wazeroir.OperationBr:
			if err := builder.handleBr(o); err != nil {
				return nil, fmt.Errorf("error handling br operation: %w", err)
			}
		case *wazeroir.OperationBrIf:
			if err := builder.handleBrIf(o); err != nil {
				return nil, fmt.Errorf("error handling br_if operation: %w", err)
			}
		case *wazeroir.OperationBrTable:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCall:
			if err := builder.handleCall(o); err != nil {
				return nil, fmt.Errorf("error handling call operation: %w", err)
			}
		case *wazeroir.OperationCallIndirect:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationDrop:
			if err := builder.handleDrop(o); err != nil {
				return nil, fmt.Errorf("error handling drop operation: %w", err)
			}
		case *wazeroir.OperationSelect:
			if err := builder.handleSelect(); err != nil {
				return nil, fmt.Errorf("error handling select operation: %w", err)
			}
		case *wazeroir.OperationPick:
			if err := builder.handlePick(o); err != nil {
				return nil, fmt.Errorf("error handling pick operation: %w", err)
			}
		case *wazeroir.OperationSwap:
			if err := builder.handleSwap(o); err != nil {
				return nil, fmt.Errorf("error handling swap operation: %w", err)
			}
		case *wazeroir.OperationGlobalGet:
			if err := builder.handleGlobalGet(o); err != nil {
				return nil, fmt.Errorf("error handling global.get operation: %w", err)
			}
		case *wazeroir.OperationGlobalSet:
			if err := builder.handleGlobalSet(o); err != nil {
				return nil, fmt.Errorf("error handling global.set operation: %w", err)
			}
		case *wazeroir.OperationLoad:
			if err := builder.handleLoad(o); err != nil {
				return nil, fmt.Errorf("error handling load operation: %w", err)
			}
		case *wazeroir.OperationLoad8:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLoad16:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLoad32:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationStore:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationStore8:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationStore16:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationStore32:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationMemorySize:
			builder.handleMemorySize()
		case *wazeroir.OperationMemoryGrow:
			builder.handleMemoryGrow()
		case *wazeroir.OperationConstI32:
			if err := builder.handleConstI32(o); err != nil {
				return nil, fmt.Errorf("error handling i32.const operation: %w", err)
			}
		case *wazeroir.OperationConstI64:
			if err := builder.handleConstI64(o); err != nil {
				return nil, fmt.Errorf("error handling i64.const operation: %w", err)
			}
		case *wazeroir.OperationConstF32:
			if err := builder.handleConstF32(o); err != nil {
				return nil, fmt.Errorf("error handling f32.const operation: %w", err)
			}
		case *wazeroir.OperationConstF64:
			if err := builder.handleConstF64(o); err != nil {
				return nil, fmt.Errorf("error handling f64.const operation: %w", err)
			}
		case *wazeroir.OperationEq:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationNe:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationEqz:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLt:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationGt:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLe:
			if err := builder.handleLe(o); err != nil {
				return nil, fmt.Errorf("error handling le operation: %w", err)
			}
		case *wazeroir.OperationGe:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationAdd:
			if err := builder.handleAdd(o); err != nil {
				return nil, fmt.Errorf("error handling add operation: %w", err)
			}
		case *wazeroir.OperationSub:
			if err := builder.handleSub(o); err != nil {
				return nil, fmt.Errorf("error handling sub operation: %w", err)
			}
		case *wazeroir.OperationMul:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationClz:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCtz:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationPopcnt:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationDiv:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationRem:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationAnd:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationOr:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationXor:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationShl:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationShr:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationRotl:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationRotr:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationAbs:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationNeg:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCeil:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationFloor:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationTrunc:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationNearest:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationSqrt:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationMin:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationMax:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCopysign:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationI32WrapFromI64:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationITruncFromF:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationFConvertFromI:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationF32DemoteFromF64:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationF64PromoteFromF32:
		case *wazeroir.OperationI32ReinterpretFromF32,
			*wazeroir.OperationI64ReinterpretFromF64,
			*wazeroir.OperationF32ReinterpretFromI32,
			*wazeroir.OperationF64ReinterpretFromI64:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationExtend:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		default:
			return nil, fmt.Errorf("unreachable: a bug in JIT compiler")
		}
	}

	code, err := builder.compile()
	if err != nil {
		return nil, fmt.Errorf("failed to assemble: %w", err)
	}
	return builder.newCompiledWasmFunction(code), nil
}

func (b *amd64Builder) newCompiledWasmFunction(code []byte) *compiledWasmFunction {
	cf := &compiledWasmFunction{
		source:          b.f,
		codeSegment:     code,
		inputs:          uint64(len(b.f.Signature.InputTypes)),
		returns:         uint64(len(b.f.Signature.ReturnTypes)),
		memory:          b.f.ModuleInstance.Memory,
		maxStackPointer: b.locationStack.maxStackPointer,
	}
	if cf.memory != nil && len(cf.memory.Buffer) > 0 {
		cf.memoryAddress = uintptr(unsafe.Pointer(&cf.memory.Buffer[0]))
	}
	if len(b.f.ModuleInstance.Globals) > 0 {
		cf.globalSliceAddress = uintptr(unsafe.Pointer(&b.f.ModuleInstance.Globals[0]))
	}
	cf.codeInitialAddress = uintptr(unsafe.Pointer(&cf.codeSegment[0]))
	return cf
}

func (b *amd64Builder) pushFunctionInputs() {
	for _, t := range b.f.Signature.InputTypes {
		loc := b.locationStack.pushValueOnStack()
		loc.setValueType(wazeroir.WasmValueTypeToSignless(t))
	}
}

type amd64Builder struct {
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

func (b *amd64Builder) compile() ([]byte, error) {
	code, err := mmapCodeSegment(b.builder.Assemble())
	if err != nil {
		return nil, err
	}
	// As we cannot read RIP register directly,
	// we calculate now the offset to the next instruction
	// relative to the beginning of this function body.
	const operandSizeBytes = 8
	for _, obj := range b.requireFunctionCallReturnAddressOffsetResolution {
		// Skip MOV, and the register(rax): "0x49, 0xbd"
		start := obj.Pc + 2
		// obj.Link = setting offset to memory
		// obj.Link.Link = writing back the stack pointer to eng.currentStackPointer.
		// obj.Link.Link.Link = Return instruction.
		// Therefore obj.Link.Link.Link.Link means the next instruction after the return.
		afterReturnInst := obj.Link.Link.Link.Link
		binary.LittleEndian.PutUint64(code[start:start+operandSizeBytes], uint64(afterReturnInst.Pc))
	}
	return code, nil
}

func (b *amd64Builder) addInstruction(prog *obj.Prog) {
	b.builder.AddInstruction(prog)
	if b.setJmpOrigin != nil {
		b.setJmpOrigin.To.SetTarget(prog)
		b.setJmpOrigin = nil
	}
}

func (b *amd64Builder) newProg() (prog *obj.Prog) {
	prog = b.builder.NewProg()
	return
}

func (b *amd64Builder) handleUnreachable() {
	b.releaseAllRegistersToStack()
	b.setJITStatus(jitCallStatusCodeUnreachable)
	b.returnFunction()
}

func (b *amd64Builder) handleSwap(o *wazeroir.OperationSwap) error {
	index := len(b.locationStack.stack) - 1 - o.Depth
	// Note that, in theory, the register types and value types
	// are the same between these swap targets as swap operations
	// are generated from local.set,tee instructions in Wasm.
	x1 := b.locationStack.stack[len(b.locationStack.stack)-1]
	x2 := b.locationStack.stack[index]

	// If x1 is on the conditional register, we must move it to a gp
	// register before swap.
	if x1.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x1); err != nil {
			return err
		}
	}

	if x1.onRegister() && x2.onRegister() {
		x1.register, x2.register = x2.register, x1.register
	} else if x1.onRegister() && x2.onStack() {
		reg := x1.register
		// Save x1's value to the temporary top of the stack.
		tmpStackLocation := b.locationStack.pushValueOnRegister(reg)
		b.releaseRegisterToStack(tmpStackLocation)
		// Then move the x2's value to the x1's register location.
		x2.register = reg
		b.moveStackToRegister(x2)
		// Now move the x1's value to the x1's stack location.
		b.releaseRegisterToStack(x1)
		// Next we move the saved x1's value to the register.
		tmpStackLocation.setRegister(reg)
		b.moveStackToRegister(tmpStackLocation)
		// Finally move the x1's value in the register to the x2's stack location.
		b.locationStack.releaseRegister(x1)
		b.locationStack.releaseRegister(tmpStackLocation)
		x2.setRegister(reg)
		b.locationStack.markRegisterUsed(reg)
		_ = b.locationStack.pop() // Delete tmpStackLocation.
	} else if x1.onStack() && x2.onRegister() {
		reg := x2.register
		// Save x2's value to the temporary top of the stack.
		tmpStackLocation := b.locationStack.pushValueOnRegister(reg)
		b.releaseRegisterToStack(tmpStackLocation)
		// Then move the x1's value to the x2's register location.
		x1.register = reg
		b.moveStackToRegister(x1)
		// Now move the x1's value to the x2's stack location.
		b.releaseRegisterToStack(x2)
		// Next we move the saved x2's value to the register.
		tmpStackLocation.setRegister(reg)
		b.moveStackToRegister(tmpStackLocation)
		// Finally move the x2's value in the register to the x2's stack location.
		b.locationStack.releaseRegister(x2)
		b.locationStack.releaseRegister(tmpStackLocation)
		x1.setRegister(reg)
		b.locationStack.markRegisterUsed(reg)
		_ = b.locationStack.pop() // Delete tmpStackLocation.
	} else if x1.onStack() && x2.onStack() {
		reg, err := b.allocateRegister(x1.registerType())
		if err != nil {
			return err
		}
		// First we move the x2's value to the temp register.
		x2.setRegister(reg)
		b.moveStackToRegister(x2)
		// Save x2's value to the temporary top of the stack.
		tmpStackLocation := b.locationStack.pushValueOnRegister(reg)
		b.releaseRegisterToStack(tmpStackLocation)
		// Then move the x1's value to the x2's register location.
		x1.register = reg
		b.moveStackToRegister(x1)
		// Now move the x1's value to the x2's stack location.
		b.releaseRegisterToStack(x2)
		// Next we move the saved x2's value to the register.
		tmpStackLocation.setRegister(reg)
		b.moveStackToRegister(tmpStackLocation)
		// Finally move the x2's value in the register to the x2's stack location.
		b.locationStack.releaseRegister(x2)
		b.locationStack.releaseRegister(tmpStackLocation)
		x1.setRegister(reg)
		b.locationStack.markRegisterUsed(reg)
		_ = b.locationStack.pop() // Delete tmpStackLocation.
	}
	return nil
}

const globalInstanceValueOffset = 8

func (b *amd64Builder) handleGlobalGet(o *wazeroir.OperationGlobalGet) error {
	intReg, err := b.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, move the pointer to the global slice into the allocated register.
	moveGlobalSlicePointer := b.newProg()
	moveGlobalSlicePointer.As = x86.AMOVQ
	moveGlobalSlicePointer.To.Type = obj.TYPE_REG
	moveGlobalSlicePointer.To.Reg = intReg
	moveGlobalSlicePointer.From.Type = obj.TYPE_MEM
	moveGlobalSlicePointer.From.Reg = reservedRegisterForEngine
	moveGlobalSlicePointer.From.Offset = engineCurrentGlobalSliceAddressOffset
	b.addInstruction(moveGlobalSlicePointer)

	// Then, get the memory location of the target global instance's pointer.
	getGlobalInstanceLocation := b.newProg()
	getGlobalInstanceLocation.As = x86.AADDQ
	getGlobalInstanceLocation.To.Type = obj.TYPE_REG
	getGlobalInstanceLocation.To.Reg = intReg
	getGlobalInstanceLocation.From.Type = obj.TYPE_CONST
	getGlobalInstanceLocation.From.Offset = 8 * int64(o.Index)
	b.addInstruction(getGlobalInstanceLocation)

	// Now, move the location of the global instance into the register.
	getGlobalInstancePointer := b.newProg()
	getGlobalInstancePointer.As = x86.AMOVQ
	getGlobalInstancePointer.To.Type = obj.TYPE_REG
	getGlobalInstancePointer.To.Reg = intReg
	getGlobalInstancePointer.From.Type = obj.TYPE_MEM
	getGlobalInstancePointer.From.Reg = intReg
	b.addInstruction(getGlobalInstancePointer)

	// When an integer, reuse the pointer register for the value. Otherwise, allocate a float register for it.
	valueReg := intReg
	wasmType := b.f.ModuleInstance.Globals[o.Index].Type.ValType
	switch wasmType {
	case wasm.ValueTypeF32, wasm.ValueTypeF64:
		valueReg, err = b.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
	}

	// Using the register holding the pointer to the target instance, move its value into a register.
	moveValue := b.newProg()
	moveValue.As = x86.AMOVQ
	moveValue.To.Type = obj.TYPE_REG
	moveValue.To.Reg = valueReg
	moveValue.From.Type = obj.TYPE_MEM
	moveValue.From.Reg = intReg
	moveValue.From.Offset = globalInstanceValueOffset
	b.addInstruction(moveValue)

	// Record that the retrieved global value on the top of the stack is now in a register.
	loc := b.locationStack.pushValueOnRegister(valueReg)
	loc.setValueType(wazeroir.WasmValueTypeToSignless(wasmType))
	return nil
}

func (b *amd64Builder) handleGlobalSet(o *wazeroir.OperationGlobalSet) error {
	// First, move the value to set into a temporary register.
	val := b.locationStack.pop()
	if val.onStack() {
		if err := b.moveStackToRegisterWithAllocation(val.registerType(), val); err != nil {
			return err
		}
	} else if val.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(val); err != nil {
			return err
		}
	}

	// Allocate a register to hold the memory location of the target global instance.
	intReg, err := b.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}

	// First, move the pointer to the global slice into the allocated register.
	moveGlobalSlicePointer := b.newProg()
	moveGlobalSlicePointer.As = x86.AMOVQ
	moveGlobalSlicePointer.To.Type = obj.TYPE_REG
	moveGlobalSlicePointer.To.Reg = intReg
	moveGlobalSlicePointer.From.Type = obj.TYPE_MEM
	moveGlobalSlicePointer.From.Reg = reservedRegisterForEngine
	moveGlobalSlicePointer.From.Offset = engineCurrentGlobalSliceAddressOffset
	b.addInstruction(moveGlobalSlicePointer)

	// Then, get the memory location of the target global instance's pointer.
	getGlobalInstanceLocation := b.newProg()
	getGlobalInstanceLocation.As = x86.AADDQ
	getGlobalInstanceLocation.To.Type = obj.TYPE_REG
	getGlobalInstanceLocation.To.Reg = intReg
	getGlobalInstanceLocation.From.Type = obj.TYPE_CONST
	getGlobalInstanceLocation.From.Offset = 8 * int64(o.Index)
	b.addInstruction(getGlobalInstanceLocation)

	// Now, move the location of the global instance into the register.
	getGlobalInstancePointer := b.newProg()
	getGlobalInstancePointer.As = x86.AMOVQ
	getGlobalInstancePointer.To.Type = obj.TYPE_REG
	getGlobalInstancePointer.To.Reg = intReg
	getGlobalInstancePointer.From.Type = obj.TYPE_MEM
	getGlobalInstancePointer.From.Reg = intReg
	b.addInstruction(getGlobalInstancePointer)

	// Now ready to write the value to the global instance location.
	moveValue := b.newProg()
	moveValue.As = x86.AMOVQ
	moveValue.From.Type = obj.TYPE_REG
	moveValue.From.Reg = val.register
	moveValue.To.Type = obj.TYPE_MEM
	moveValue.To.Reg = intReg
	moveValue.To.Offset = globalInstanceValueOffset
	b.addInstruction(moveValue)

	// Since the value is now written to memory, release the value register.
	b.locationStack.releaseRegister(val)
	return nil
}

func (b *amd64Builder) handleBr(o *wazeroir.OperationBr) error {
	if o.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		b.releaseAllRegistersToStack()
		b.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		b.returnFunction()
	} else {
		labelKey := o.Target.String()
		targetNumCallers := b.ir.LabelCallers[labelKey]
		if targetNumCallers > 1 {
			// If the number of callers to the target label is larget than one,
			// we have multiple origins to the target branch. In that case,
			// we must have unique register state.
			b.preJumpRegisterAdjustment()
		}
		jmp := b.newProg()
		jmp.As = obj.AJMP
		jmp.To.Type = obj.TYPE_BRANCH
		b.addInstruction(jmp)
		b.assignJumpTarget(labelKey, jmp)
	}
	return nil
}

func (b *amd64Builder) handleBrIf(o *wazeroir.OperationBrIf) error {
	cond := b.locationStack.pop()
	var jmpWithCond *obj.Prog
	if cond.onConditionalRegister() {
		jmpWithCond = b.newProg()
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
			if err := b.moveStackToRegisterWithAllocation(cond.registerType(), cond); err != nil {
				return err
			}
		}
		// Check if the value not equals zero.
		prog := b.newProg()
		prog.As = x86.ACMPQ
		prog.From.Type = obj.TYPE_REG
		prog.From.Reg = cond.register
		prog.To.Type = obj.TYPE_CONST
		prog.To.Offset = 0
		b.addInstruction(prog)
		// Emit jump instruction which jumps when the value does not equals zero.
		jmpWithCond = b.newProg()
		jmpWithCond.As = x86.AJNE
		jmpWithCond.To.Type = obj.TYPE_BRANCH
	}

	// Make sure that the next coming label is the else jump target.
	b.addInstruction(jmpWithCond)
	thenTarget, elseTarget := o.Then, o.Else

	// Here's the diagram of how we organize the instructions necessarly for brif operation.
	//
	// jmp_with_cond -> jmp (.Else) -> Then operations...
	//    |---------(satisfied)------------^^^
	//
	// Note that .Else branch doesn't have ToDrop as .Else is in reality
	// corresponding to either If's Else block or Br_if's else block in Wasm.

	// Emit for else branches
	saved := b.locationStack
	b.locationStack = saved.clone()
	if elseTarget.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		b.releaseAllRegistersToStack()
		b.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		b.returnFunction()
	} else {
		elseLabelKey := elseTarget.Target.Label.String()
		if b.ir.LabelCallers[elseLabelKey] > 1 {
			b.preJumpRegisterAdjustment()
		}
		elseJmp := b.newProg()
		elseJmp.As = obj.AJMP
		elseJmp.To.Type = obj.TYPE_BRANCH
		b.addInstruction(elseJmp)
		b.assignJumpTarget(elseLabelKey, elseJmp)
	}

	// Handle then branch.
	b.setJmpOrigin = jmpWithCond
	b.locationStack = saved
	if err := b.emitDropRange(thenTarget.ToDrop); err != nil {
		return err
	}
	if thenTarget.Target.IsReturnTarget() {
		// Release all the registers as our calling convention requires the callee-save.
		b.releaseAllRegistersToStack()
		b.setJITStatus(jitCallStatusCodeReturned)
		// Then return from this function.
		b.returnFunction()
	} else {
		thenLabelKey := thenTarget.Target.Label.String()
		if b.ir.LabelCallers[thenLabelKey] > 1 {
			b.preJumpRegisterAdjustment()
		}
		thenJmp := b.newProg()
		thenJmp.As = obj.AJMP
		thenJmp.To.Type = obj.TYPE_BRANCH
		b.addInstruction(thenJmp)
		b.assignJumpTarget(thenLabelKey, thenJmp)
	}
	return nil
}

// If a jump target has multiple callesr (origins),
// we must have unique register states, so this function
// must be called before such jump instruction.
func (b *amd64Builder) preJumpRegisterAdjustment() {
	// For now, we just release all registers to memory.
	// But this is obviously inefficient, so we come back here
	// later once we finish the baseline implementation.
	b.releaseAllRegistersToStack()
}

func (b *amd64Builder) assignJumpTarget(labelKey string, jmpInstruction *obj.Prog) {
	jmpTarget, ok := b.labelInitialInstructions[labelKey]
	if ok {
		jmpInstruction.To.SetTarget(jmpTarget)
	} else {
		b.onLabelStartCallbacks[labelKey] = append(b.onLabelStartCallbacks[labelKey], func(jmpTarget *obj.Prog) {
			jmpInstruction.To.SetTarget(jmpTarget)
		})
	}
}

func (b *amd64Builder) handleLabel(o *wazeroir.OperationLabel) error {
	b.locationStack.sp = uint64(o.Label.OriginalStackLen)
	// We use NOP as a beginning of instructions in a label.
	// This should be eventually optimized out by assembler.
	labelKey := o.Label.String()
	labelBegin := b.newProg()
	labelBegin.As = obj.ANOP
	b.addInstruction(labelBegin)
	// Save the instructions so that backward branching
	// instructions can jump to this label.
	b.labelInitialInstructions[labelKey] = labelBegin
	// Invoke callbacks to notify the forward branching
	// instructions can properly jump to this label.
	for _, cb := range b.onLabelStartCallbacks[labelKey] {
		cb(labelBegin)
	}
	// Now we don't need to call the callbacks.
	delete(b.onLabelStartCallbacks, labelKey)
	return nil
}

func (b *amd64Builder) handleCall(o *wazeroir.OperationCall) error {
	target := b.f.ModuleInstance.Functions[o.FunctionIndex]
	if target.HostFunction != nil {
		index := b.eng.compiledHostFunctionIndex[target]
		b.callHostFunctionFromConstIndex(index)
	} else {
		index := b.eng.compiledWasmFunctionIndex[target]
		b.callFunctionFromConstIndex(index)
	}
	return nil
}
func (b *amd64Builder) handleDrop(o *wazeroir.OperationDrop) error {
	return b.emitDropRange(o.Range)
}

func (b *amd64Builder) emitDropRange(r *wazeroir.InclusiveRange) error {
	if r == nil {
		return nil
	} else if r.Start == 0 {
		for i := 0; i < r.End; i++ {
			if loc := b.locationStack.pop(); loc.onRegister() {
				b.locationStack.releaseRegister(loc)
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
		live := b.locationStack.pop()
		if top == nil {
			top = live
			topIsConditional = top.onConditionalRegister()
		}
		liveValues = append(liveValues, live)
	}
	for i := 0; i < r.End-r.Start+1; i++ {
		if loc := b.locationStack.pop(); loc.onRegister() {
			b.locationStack.releaseRegister(loc)
		}
	}
	for i := range liveValues {
		live := liveValues[len(liveValues)-1-i]
		if live.onStack() {
			if topIsConditional {
				// If the top is conditional, and it's not target of drop,
				// we must assign it to the register before we emit any instructions here.
				if err := b.moveConditionalToGPRegister(top); err != nil {
					return err
				}
				topIsConditional = false
			}
			// Write the value in the old stack location to a register
			if err := b.moveStackToRegisterWithAllocation(live.registerType(), live); err != nil {
				return err
			}
			// Modify the location in the stack with new stack pointer.
			b.locationStack.push(live)
		} else if live.onRegister() {
			b.locationStack.push(live)
		}
	}
	return nil
}

// handleSelect uses top three values on the stack:
// Assume we have stack as [..., x1, x2, c], if the value of c
// equals zero, then the stack results in [..., x1]
// otherwise, [..., x2].
// The emitted native code depends on whether the values are on
// the physical registers or memory stack, or maybe conditional register.
func (b *amd64Builder) handleSelect() error {
	c := b.locationStack.pop()
	x2 := b.locationStack.pop()
	// We do not consume x1 here, but modify the value according to
	// the conditional value "c" above.
	peekedX1 := b.locationStack.peek()

	// Ensure the conditional value lives in a gp register.
	if c.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(c); err != nil {
			return err
		}
	} else if c.onStack() {
		if err := b.moveStackToRegisterWithAllocation(c.registerType(), c); err != nil {
			return err
		}
	}

	// Compare the conditional value with zero.
	cmpZero := b.newProg()
	cmpZero.As = x86.ACMPQ
	cmpZero.From.Type = obj.TYPE_REG
	cmpZero.From.Reg = c.register
	cmpZero.To.Type = obj.TYPE_CONST
	cmpZero.To.Offset = 0
	b.addInstruction(cmpZero)

	// Now we can use c.register as temporary location.
	// We alias it here for readability.
	tmpRegister := c.register

	// Set the jump if the top value is not zero.
	jmpIfNotZero := b.newProg()
	jmpIfNotZero.As = x86.AJNE
	jmpIfNotZero.To.Type = obj.TYPE_BRANCH
	b.addInstruction(jmpIfNotZero)

	// If the value is zero, we must place the value of x2 onto the stack position of x1.

	// First we copy the value of x2 to the temporary register if x2 is not currently on a register.
	if x2.onStack() {
		x2.register = tmpRegister
		b.moveStackToRegister(x2)
	}

	//
	// At this point x2's value is always on a register.
	//

	// Then release the value in the x2's register to the x1's stack position.
	if peekedX1.onRegister() {
		movX2ToX1 := b.newProg()
		movX2ToX1.As = x86.AMOVQ
		movX2ToX1.From.Type = obj.TYPE_REG
		movX2ToX1.From.Reg = x2.register
		movX2ToX1.To.Type = obj.TYPE_REG
		movX2ToX1.To.Reg = peekedX1.register
		b.addInstruction(movX2ToX1)
	} else {
		peekedX1.register = x2.register
		b.releaseRegisterToStack(peekedX1) // Note inside we mark the register unused!
	}

	// Else, we don't need to adjust value, just need to jump to the next instruction.
	b.setJmpOrigin = jmpIfNotZero

	// In any case, we don't need x2 and c anymore!
	b.locationStack.releaseRegister(x2)
	b.locationStack.releaseRegister(c)
	return nil
}

func (b *amd64Builder) handlePick(o *wazeroir.OperationPick) error {
	// TODO: if we track the type of values on the stack,
	// we could optimize the instruction according to the bit size of the value.
	// For now, we just move the entire register i.e. as a quad word (8 bytes).
	pickTarget := b.locationStack.stack[b.locationStack.sp-1-uint64(o.Depth)]
	reg, err := b.allocateRegister(pickTarget.registerType())
	if err != nil {
		return err
	}

	if pickTarget.onRegister() {
		prog := b.newProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_REG
		prog.From.Reg = pickTarget.register
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = reg
		b.addInstruction(prog)
	} else if pickTarget.onStack() {
		// Copy the value from the stack.
		prog := b.newProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_MEM
		prog.From.Reg = reservedRegisterForStackBasePointer
		prog.From.Offset = int64(pickTarget.stackPointer) * 8
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = reg
		b.addInstruction(prog)
	} else if pickTarget.onConditionalRegister() {
		panic("TODO")
	}
	// Now we already placed the picked value on the register,
	// so push the location onto the stack.
	loc := b.locationStack.pushValueOnRegister(reg)
	loc.setValueType(pickTarget.valueType)
	return nil
}

func (b *amd64Builder) handleAdd(o *wazeroir.OperationAdd) error {
	// TODO: if the previous instruction is const, then
	// this can be optimized. Same goes for other arithmetic instructions.

	var instruction obj.As
	var tp generalPurposeRegisterType
	switch o.Type {
	case wazeroir.SignLessTypeI32:
		instruction = x86.AADDL
		tp = generalPurposeRegisterTypeInt
		panic("add tests!")
	case wazeroir.SignLessTypeI64:
		instruction = x86.AADDQ
		tp = generalPurposeRegisterTypeInt
	case wazeroir.SignLessTypeF32:
		instruction = x86.AADDSS
		tp = generalPurposeRegisterTypeFloat
		panic("add tests!")
	case wazeroir.SignLessTypeF64:
		instruction = x86.AADDSD
		tp = generalPurposeRegisterTypeFloat
		panic("add tests!")
	}

	x2 := b.locationStack.pop()
	if x2.onStack() {
		if err := b.moveStackToRegisterWithAllocation(tp, x2); err != nil {
			return err
		}
	} else if x2.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x2); err != nil {
			return err
		}
	}

	x1 := b.locationStack.peek() // Note this is peek, pop!
	if x1.onStack() {
		if err := b.moveStackToRegisterWithAllocation(tp, x1); err != nil {
			return err
		}
	} else if x1.onConditionalRegister() {
		// This shouldn't happen as the conditional
		// register must be on top of the stack.
		panic("a bug in jit compiler")
	}

	// x1 += x2.
	prog := b.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x2.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x1.register
	prog.As = instruction
	b.addInstruction(prog)

	// We no longer need x2 register after ADD operation here,
	// so we release it.
	b.locationStack.releaseRegister(x2)
	return nil
}

func (b *amd64Builder) handleSub(o *wazeroir.OperationSub) error {
	// TODO: if the previous instruction is const, then
	// this can be optimized. Same goes for other arithmetic instructions.

	var instruction obj.As
	var tp generalPurposeRegisterType
	switch o.Type {
	case wazeroir.SignLessTypeI32:
		instruction = x86.ASUBL
		tp = generalPurposeRegisterTypeInt
		panic("add tests!")
	case wazeroir.SignLessTypeI64:
		instruction = x86.ASUBQ
		tp = generalPurposeRegisterTypeInt
	case wazeroir.SignLessTypeF32:
		instruction = x86.ASUBSS
		tp = generalPurposeRegisterTypeFloat
		panic("add tests!")
	case wazeroir.SignLessTypeF64:
		instruction = x86.ASUBSD
		tp = generalPurposeRegisterTypeFloat
		panic("add tests!")
	}

	x2 := b.locationStack.pop()
	if x2.onStack() {
		if err := b.moveStackToRegisterWithAllocation(tp, x2); err != nil {
			return err
		}
	} else if x2.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x2); err != nil {
			return err
		}
	}

	x1 := b.locationStack.peek() // Note this is peek, pop!
	if x1.onStack() {
		if err := b.moveStackToRegisterWithAllocation(tp, x1); err != nil {
			return err
		}
	} else if x1.onConditionalRegister() {
		// This shouldn't happen as the conditional
		// register must be on top of the stack.
		panic("a bug in jit compiler")
	}

	// x1 += x2.
	prog := b.newProg()
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x2.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x1.register
	prog.As = instruction
	b.addInstruction(prog)

	// We no longer need x2 register after ADD operation here,
	// so we release it.
	b.locationStack.releaseRegister(x2)
	return nil
}

func (b *amd64Builder) handleLe(o *wazeroir.OperationLe) error {
	var resultConditionState conditionalRegisterState
	var instruction obj.As
	var tp generalPurposeRegisterType
	switch o.Type {
	case wazeroir.SignFulTypeInt32:
		resultConditionState = conditionalRegisterStateLE
		instruction = x86.ACMPL
		tp = generalPurposeRegisterTypeInt
	case wazeroir.SignFulTypeUint32:
		resultConditionState = conditionalRegisterStateBE
		instruction = x86.ACMPL
		tp = generalPurposeRegisterTypeInt
	case wazeroir.SignFulTypeInt64:
		resultConditionState = conditionalRegisterStateLE
		instruction = x86.ACMPQ
		tp = generalPurposeRegisterTypeInt
	case wazeroir.SignFulTypeUint64:
		resultConditionState = conditionalRegisterStateBE
		instruction = x86.ACMPQ
		tp = generalPurposeRegisterTypeInt
	case wazeroir.SignFulTypeFloat32:
		tp = generalPurposeRegisterTypeFloat
		panic("add test!")
	case wazeroir.SignFulTypeFloat64:
		tp = generalPurposeRegisterTypeFloat
		panic("add test!")
	}

	x2 := b.locationStack.pop()
	if x2.onStack() {
		if err := b.moveStackToRegisterWithAllocation(tp, x2); err != nil {
			return err
		}
	} else if x2.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x2); err != nil {
			return err
		}
	}

	x1 := b.locationStack.pop()
	if x1.onStack() {
		if err := b.moveStackToRegisterWithAllocation(tp, x1); err != nil {
			return err
		}
	} else if x1.onConditionalRegister() {
		// This shouldn't happen as the conditional
		// register must be on top of the stack.
		panic("a bug in jit compiler")
	}

	// Compare: set the flag based on x1-x2.
	prog := b.newProg()
	prog.As = instruction
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = x1.register
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = x2.register
	b.addInstruction(prog)

	// We no longer need x1,x2 register after cmp operation here,
	// so we release it.
	b.locationStack.releaseRegister(x1)
	b.locationStack.releaseRegister(x2)

	// Finally we have the result on the conditional register,
	// so record it.
	loc := b.locationStack.pushValueOnConditionalRegister(resultConditionState)
	loc.setValueType(wazeroir.SignLessTypeI32)
	return nil
}

func (b *amd64Builder) handleLoad(o *wazeroir.OperationLoad) error {
	base := b.locationStack.pop()
	if base.onStack() {
		if err := b.moveStackToRegisterWithAllocation(generalPurposeRegisterTypeInt, base); err != nil {
			return err
		}
	} else if base.onConditionalRegister() {
		if err := b.moveConditionalToGeneralPurposeRegister(base); err != nil {
			return err
		}
	}

	// At this point, base's value is on the integer general purpose reg.
	// We reuse the register below, so we alias it here for readability.
	reg := base.register

	// Then we have to calculate the offset on the memory region.
	addOffsetToBase := b.newProg()
	addOffsetToBase.As = x86.AADDL // 32-bit!
	addOffsetToBase.To.Type = obj.TYPE_REG
	addOffsetToBase.To.Reg = reg
	addOffsetToBase.From.Type = obj.TYPE_CONST
	addOffsetToBase.From.Offset = int64(o.Arg.Offest)
	b.addInstruction(addOffsetToBase)

	// TODO: Emit instructions here to check memory out of bounds as
	// potentially it would be an security risk.

	var (
		isIntType bool
		movInst   obj.As
	)
	switch o.Type {
	case wazeroir.SignLessTypeI32:
		isIntType = true
		movInst = x86.AMOVL
	case wazeroir.SignLessTypeI64:
		isIntType = true
		movInst = x86.AMOVQ
	case wazeroir.SignLessTypeF32:
		isIntType = false
		movInst = x86.AMOVL
	case wazeroir.SignLessTypeF64:
		isIntType = false
		movInst = x86.AMOVQ
	}

	if isIntType {
		// For integer types, read the corresponding bytes from the offset to the memory
		// and store the value to the int register.
		moveFromMemory := b.newProg()
		moveFromMemory.As = movInst
		moveFromMemory.To.Type = obj.TYPE_REG
		moveFromMemory.To.Reg = reg
		moveFromMemory.From.Type = obj.TYPE_MEM
		moveFromMemory.From.Reg = reservedRegisterForMemory
		moveFromMemory.From.Index = reg
		moveFromMemory.From.Scale = 1
		b.addInstruction(moveFromMemory)
		top := b.locationStack.pushValueOnRegister(reg)
		top.setValueType(o.Type)
	} else {
		// For float types, we read the value to the float register.
		floatReg, err := b.allocateRegister(generalPurposeRegisterTypeFloat)
		if err != nil {
			return err
		}
		moveFromMemory := b.newProg()
		moveFromMemory.As = movInst
		moveFromMemory.To.Type = obj.TYPE_REG
		moveFromMemory.To.Reg = floatReg
		moveFromMemory.From.Type = obj.TYPE_MEM
		moveFromMemory.From.Reg = reservedRegisterForMemory
		moveFromMemory.From.Index = reg
		moveFromMemory.From.Scale = 1
		b.addInstruction(moveFromMemory)
		top := b.locationStack.pushValueOnRegister(floatReg)
		top.setValueType(o.Type)
		// We no longer need the int register so mark it unused.
		b.locationStack.markRegisterUnused(reg)
	}
	return nil
}

func (b *amd64Builder) handleMemoryGrow() {
	b.callBuiltinFunctionFromConstIndex(builtinFunctionIndexMemoryGrow)
}

func (b *amd64Builder) handleMemorySize() {
	b.callBuiltinFunctionFromConstIndex(builtinFunctionIndexMemorySize)
	loc := b.locationStack.pushValueOnStack() // The size is pushed on the top.
	loc.setValueType(wazeroir.SignLessTypeI32)
}

func (b *amd64Builder) callBuiltinFunctionFromConstIndex(index int64) {
	b.setJITStatus(jitCallStatusCodeCallBuiltInFunction)
	b.setFunctionCallIndexFromConst(index)
	// Release all the registers as our calling convention requires the callee-save.
	b.releaseAllRegistersToStack()
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once we return from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) handleConstI32(o *wazeroir.OperationConstI32) error {
	reg, err := b.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := b.locationStack.pushValueOnRegister(reg)
	loc.setValueType(wazeroir.SignLessTypeI32)

	prog := b.newProg()
	prog.As = x86.AMOVL // Note 32-bit move!
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(o.Value)
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)
	return nil
}

func (b *amd64Builder) handleConstI64(o *wazeroir.OperationConstI64) error {
	reg, err := b.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	loc := b.locationStack.pushValueOnRegister(reg)
	loc.setValueType(wazeroir.SignLessTypeI64)

	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(o.Value)
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)
	return nil
}

func (b *amd64Builder) handleConstF32(o *wazeroir.OperationConstF32) error {
	reg, err := b.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}
	loc := b.locationStack.pushValueOnRegister(reg)
	loc.setValueType(wazeroir.SignLessTypeF32)

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := b.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	moveToTmpReg := b.newProg()
	moveToTmpReg.As = x86.AMOVL // Note 32-bit mov!
	moveToTmpReg.From.Type = obj.TYPE_CONST
	moveToTmpReg.From.Offset = int64(uint64(math.Float32bits(o.Value)))
	moveToTmpReg.To.Type = obj.TYPE_REG
	moveToTmpReg.To.Reg = tmpReg
	b.addInstruction(moveToTmpReg)

	prog := b.newProg()
	prog.As = x86.AMOVL // Note 32-bit mov!
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)
	// We don't need to explicitly release tmpReg here
	// as allocateRegister doesn't mark it used.
	return nil
}

func (b *amd64Builder) handleConstF64(o *wazeroir.OperationConstF64) error {
	reg, err := b.allocateRegister(generalPurposeRegisterTypeFloat)
	if err != nil {
		return err
	}
	loc := b.locationStack.pushValueOnRegister(reg)
	loc.setValueType(wazeroir.SignLessTypeF64)

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := b.allocateRegister(generalPurposeRegisterTypeInt)
	if err != nil {
		return err
	}
	moveToTmpReg := b.newProg()
	moveToTmpReg.As = x86.AMOVQ
	moveToTmpReg.From.Type = obj.TYPE_CONST
	moveToTmpReg.From.Offset = int64(math.Float64bits(o.Value))
	moveToTmpReg.To.Type = obj.TYPE_REG
	moveToTmpReg.To.Reg = tmpReg
	b.addInstruction(moveToTmpReg)

	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)
	// We don't need to explicitly release tmpReg here
	// as allocateRegister doesn't mark it used.
	return nil
}

// TODO: maybe split this function as this is doing too much at once to say at once.
func (b *amd64Builder) moveStackToRegisterWithAllocation(tp generalPurposeRegisterType, loc *valueLocation) error {
	// Allocate the register.
	reg, err := b.allocateRegister(tp)
	if err != nil {
		return err
	}

	// Mark it uses the register.
	loc.setRegister(reg)
	b.locationStack.markRegisterUsed(reg)

	// Now ready to move value.
	b.moveStackToRegister(loc)
	return nil
}

func (b *amd64Builder) moveStackToRegister(loc *valueLocation) {
	// Copy the value from the stack.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForStackBasePointer
	prog.From.Offset = int64(loc.stackPointer) * 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = loc.register
	b.addInstruction(prog)
}

func (b *amd64Builder) moveConditionalToGPRegister(loc *valueLocation) error {
	// Get the free register.
	reg, ok := b.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	if !ok {
		// This in theory should never be reached as moveConditionalToGPRegister
		// is called right after comparison operations, meaning that
		// at least two registers are free at the moment.
		return fmt.Errorf("conditional register mov requires a free register")
	}

	// Set the flag bit to the destination.
	prog := b.newProg()
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
	b.addInstruction(prog)

	// Then we reset the unnecessary bit.
	prog = b.newProg()
	prog.As = x86.AANDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = 0x1
	b.addInstruction(prog)

	// Mark it uses the register.
	loc.setRegister(reg)
	b.locationStack.markRegisterUsed(reg)
	return nil
}

// allocateRegister returns an unused register of the given type. The register will be taken
// either from the free register pool or by stealing an used register.
// Note that resulting registers are NOT marked as used so the call site should
// mark it used if necessary.
func (b *amd64Builder) allocateRegister(t generalPurposeRegisterType) (reg int16, err error) {
	var ok bool
	// Try to get the unused register.
	reg, ok = b.locationStack.takeFreeRegister(t)
	if ok {
		return
	}

	// If not found, we have to steal the register.
	stealTarget, ok := b.locationStack.takeStealTargetFromUsedRegister(t)
	if !ok {
		err = fmt.Errorf("cannot steal register")
		return
	}

	// Release the steal target register value onto stack location.
	reg = stealTarget.register
	b.releaseRegisterToStack(stealTarget)
	return
}

func (b *amd64Builder) setJITStatus(status jitCallStatusCode) {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(status)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineJITCallStatusCodeOffset
	b.addInstruction(prog)
}

func (b *amd64Builder) callHostFunctionFromConstIndex(index int64) {
	// Set the jit status as jitCallStatusCodeCallHostFunction
	b.setJITStatus(jitCallStatusCodeCallHostFunction)
	// Set the function index.
	b.setFunctionCallIndexFromConst(index)
	// Release all the registers as our calling convention requires the callee-save.
	b.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) callHostFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitCallStatusCodeCallHostFunction
	b.setJITStatus(jitCallStatusCodeCallHostFunction)
	// Set the function index.
	b.setFunctionCallIndexFromRegister(reg)
	// Release all the registers as our calling convention requires the callee-save.
	b.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) callFunctionFromConstIndex(index int64) {
	// Set the jit status as jitCallStatusCodeCallWasmFunction
	b.setJITStatus(jitCallStatusCodeCallWasmFunction)
	// Set the function index.
	b.setFunctionCallIndexFromConst(index)
	// Release all the registers as our calling convention requires the callee-save.
	b.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) callFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitCallStatusCodeCallWasmFunction
	b.setJITStatus(jitCallStatusCodeCallWasmFunction)
	// Set the function index.
	b.setFunctionCallIndexFromRegister(reg)
	// Release all the registers as our calling convention requires the callee-save.
	b.releaseAllRegistersToStack()
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) releaseAllRegistersToStack() {
	used := len(b.locationStack.usedRegisters)
	for i := len(b.locationStack.stack) - 1; i >= 0 && used > 0; i-- {
		if loc := b.locationStack.stack[i]; loc.onRegister() {
			b.releaseRegisterToStack(loc)
			used--
		}
	}
}

// TODO: If this function call is the tail call,
// we don't need to return back to this function.
// Maybe better have another status for that case
// so that we don't call back again to this function
// and instead just release the call frame.
func (b *amd64Builder) setContinuationOffsetAtNextInstructionAndReturn() {
	// setContinuationOffsetAtNextInstructionAndReturn is called after releasing
	// all the registers, so at this point we always have free registers.
	tmpReg, _ := b.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)
	// Create the instruction for setting offset.
	// We use tmp register to store the const, not directly movq to memory
	// as it is not valid to move 64-bit const to memory directly.
	// TODO: is it really illegal, though?
	prog := b.newProg()
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
	b.requireFunctionCallReturnAddressOffsetResolution = append(b.requireFunctionCallReturnAddressOffsetResolution, prog)
	b.addInstruction(prog)

	prog = b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpReg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineContinuationAddressOffset
	b.addInstruction(prog)
	// Then return temporarily -- giving control to normal Go code.
	b.returnFunction()
}

func (b *amd64Builder) setFunctionCallIndexFromRegister(reg int16) {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineFunctionCallIndexOffset
	b.addInstruction(prog)
}

func (b *amd64Builder) setFunctionCallIndexFromConst(index int64) {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = index
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineFunctionCallIndexOffset
	b.addInstruction(prog)
}

func (b *amd64Builder) releaseRegisterToStack(loc *valueLocation) {
	// Push value.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForStackBasePointer
	prog.To.Offset = int64(loc.stackPointer) * 8
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = loc.register
	b.addInstruction(prog)

	// Mark the register is free.
	b.locationStack.releaseRegister(loc)
}

func (b *amd64Builder) assignRegisterToValue(loc *valueLocation, reg int16) {
	// Pop value to the resgister.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForStackBasePointer
	prog.From.Offset = int64(loc.stackPointer) * 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)

	// Now the value is on register, so mark as such.
	loc.setRegister(reg)
	b.locationStack.markRegisterUsed(reg)
}

func (b *amd64Builder) returnFunction() {
	// Write back the cached SP to the actual eng.currentStackPointer.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(b.locationStack.sp)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = reservedRegisterForEngine
	prog.To.Offset = engineCurrentStackPointerOffset
	b.addInstruction(prog)

	// Return.
	ret := b.newProg()
	ret.As = obj.ARET
	b.addInstruction(ret)
}

// initializeReservedRegisters must be called at the very beginning and all the
// after-call continuations of JITed functions.
// This caches the actual stack base pointer (engine.currentStackBasePointer*8+[engine.engineStackSliceOffset])
// to cachedStackBasePointerReg
func (b *amd64Builder) initializeReservedRegisters() {
	// At first, make cachedStackBasePointerReg point to the beginning of the slice backing array.
	// movq [engineInstanceReg+engineStackSliceOffset] cachedStackBasePointerReg
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForEngine
	prog.From.Offset = engineStackSliceOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reservedRegisterForStackBasePointer
	b.addInstruction(prog)

	// initializeReservedRegisters is called at the beginning of function calls
	// or right after function returns so at this point we always have free registers.
	reg, _ := b.locationStack.takeFreeRegister(generalPurposeRegisterTypeInt)

	// Next we move the base pointer (engine.currentStackBasePointer) to
	// a temporary register.
	// movq [engineInstanceReg+engineCurrentBaseStackPointerOffset] reg
	prog = b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = reservedRegisterForEngine
	prog.From.Offset = engineCurrentStackBasePointerOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)

	// Multiply reg with 8 via shift left with 3.
	// shlq $3 reg
	prog = b.newProg()
	prog.As = x86.ASHLQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = 3
	b.addInstruction(prog)

	// Finally we add the reg to cachedStackBasePointerReg.
	// addq [reg] cachedStackBasePointerReg
	prog = b.newProg()
	prog.As = x86.AADDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reservedRegisterForStackBasePointer
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	b.addInstruction(prog)
}
