//go:build amd64
// +build amd64

package jit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"
)

func jitcall(codeSegment, engine, memory uintptr)

// Reserved registers:
// Note that we don't use "call" instruction.
// R12: pointer to engine instance (i.e. *engine as uintptr)
// R13: cacahed stack pointer of engine.stack which should be rewritten into engine.currentStackPointer
// 		whenever we exit JIT calls.
// R14: cached stack base pointer (engine.currentStackBase) in the current function call.
// R15: pointer to memory space (i.e. *[]byte as uintptr).
const (
	engineInstanceReg         = x86.REG_R12
	cachedStackPointerReg     = x86.REG_R13
	cachedStackBasePointerReg = x86.REG_R14
	memoryReg                 = x86.REG_R15
)

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledWasmFunction, error) {
	ir, err := wazeroir.Compile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to lower to wazeroir: %w", err)
	}

	// TODO: delete
	fmt.Printf("compilation target wazeroir:\n%s\n", wazeroir.Format(ir))

	b, err := asm.NewBuilder("amd64", 128)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new assembly builder: %w", err)
	}
	builder := &amd64Builder{eng: e, f: f, builder: b, locationStack: newValueLocationStack()}
	// Move the signature locals onto stack, as we assume that
	// all the function parameters (signature locals) are already pushed on the stack
	// by the caller.
	builder.pushSignatureLocals()

	// Initialize the reserved registers first of all.
	builder.initializeReservedRegisters()
	// Now move onto the function body to compile each wazeroir operation.
	for _, op := range ir {
		switch o := op.(type) {
		case *wazeroir.OperationUnreachable:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLabel:
			if err := builder.handleLabel(o); err != nil {
				return nil, fmt.Errorf("error handling label operation %s: %w", o, err)
			}
		case *wazeroir.OperationBr:
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationBrIf:
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationBrTable:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCall:
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCallIndirect:
		case *wazeroir.OperationDrop:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationSelect:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationPick:
			if err := builder.handlePick(o); err != nil {
				return nil, fmt.Errorf("error handling pick operation %v: %w", o, err)
			}
		case *wazeroir.OperationSwap:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationGlobalGet:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationGlobalSet:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLoad:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
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
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationMemoryGrow:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationConstI32:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationConstI64:
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationConstF32:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationConstF64:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
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
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationGe:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationAdd:
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationSub:
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
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

	if len(builder.onLabelStartCallbacks) > 0 {
		keys := make([]string, 0, len(builder.onLabelStartCallbacks))
		for key := range builder.onLabelStartCallbacks {
			keys = append(keys, key)
		}
		return nil, fmt.Errorf("labels are not defined: %s", strings.Join(keys, ","))
	}

	code, err := builder.assemble()
	if err != nil {
		return nil, fmt.Errorf("failed to assemble: %w", err)
	}
	return &compiledWasmFunction{codeSegment: code, memoryInst: nil}, nil
}

func (b *amd64Builder) pushSignatureLocals() {
	for ; b.memoryStackPointer < uint64(len(b.f.Signature.InputTypes)); b.memoryStackPointer++ {
		t := b.f.Signature.InputTypes[b.memoryStackPointer]
		sp := uint64(b.memoryStackPointer)
		loc := &valueLocation{valueType: wazeroir.WasmValueTypeToSignless(t), stackPointer: &sp}
		b.locationStack.push(loc)
	}
}

type amd64Builder struct {
	eng     *engine
	f       *wasm.FunctionInstance
	builder *asm.Builder
	// location stack holds the state of wazeroir virtual stack.
	// and each item is either placed in register or the actual memory stack.
	locationStack *valueLocationStack
	// Stack pointer which begins with signature locals.
	// TODO: get the maximum height.
	memoryStackPointer uint64
	// Label resolvers.
	onLabelStartCallbacks map[string][]func(*obj.Prog)
	labelProgs            map[string]*obj.Prog
}

func (b *amd64Builder) assemble() ([]byte, error) {
	code, err := mmapCodeSegment(b.builder.Assemble())
	return code, err
}

func (b *amd64Builder) addInstruction(prog *obj.Prog) {
	b.builder.AddInstruction(prog)
}

func (b *amd64Builder) newProg() (prog *obj.Prog) {
	return b.builder.NewProg()
}

func (b *amd64Builder) handleLabel(o *wazeroir.OperationLabel) error {
	// We use NOP as a beginning of instructions in a label.
	// This should be eventually optimized out by assembler.
	labelKey := o.Label.String()
	labelBegin := b.newProg()
	labelBegin.As = obj.ANOP
	b.addInstruction(labelBegin)
	// Save the instructions so that backward branching
	// instructions can jump to this label.
	b.labelProgs[labelKey] = labelBegin
	// Invoke callbacks to notify the forward branching
	// instructions can properly jump to this label.
	for _, cb := range b.onLabelStartCallbacks[labelKey] {
		cb(labelBegin)
	}
	// Now we don't need to call the callbacks.
	delete(b.onLabelStartCallbacks, labelKey)
	return nil
}

func (b *amd64Builder) handlePick(o *wazeroir.OperationPick) error {
	// TODO: if we track the type of values on the stack,
	// we could optimize the instruction according to the bit size of the value.
	// For now, we just move the entire register i.e. as a quad word (8 bytes).
	pickTarget := b.locationStack.stack[len(b.locationStack.stack)-1-o.Depth]
	if reg, err := b.locationStack.takeFreeRegister(gpTypeInt); err == nil {
		if pickTarget.onRegister() {
			prog := b.newProg()
			prog.As = x86.AMOVQ
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = *pickTarget.register
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = reg
			b.addInstruction(prog)
		} else if pickTarget.onStack() {
			// Place the stack pointer at first.
			prog := b.newProg()
			prog.As = x86.AMOVQ
			prog.From.Type = obj.TYPE_CONST
			prog.From.Offset = int64(*pickTarget.stackPointer)
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = reg
			b.addInstruction(prog)

			// Then Copy the value from the stack.
			prog = b.newProg()
			prog.As = x86.AMOVQ
			prog.From.Type = obj.TYPE_MEM
			prog.From.Reg = cachedStackBasePointerReg
			prog.From.Index = reg
			prog.From.Scale = 8
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = reg
			b.addInstruction(prog)
		} else if pickTarget.onConditionalRegister() {
			panic("TODO")
		}
		// Now we already placed the picked value on the register,
		// so push the location onto the stack.
		loc := &valueLocation{register: &reg}
		b.locationStack.push(loc)
		return nil
	} else if errors.Is(err, errFreeRegisterNotFound) {
		stealTarget, ok := b.locationStack.takeStealTargetFromUsedRegister(gpTypeInt)
		if !ok {
			return fmt.Errorf("cannot steal register")
		}
		// First we copy the value in the target register onto stack.
		evictedValueStackPointer := b.memoryStackPointer
		b.pushRegisterToStack(*stealTarget.register)
		reg := *stealTarget.register
		stealTarget.setStackPointer(evictedValueStackPointer)

		// This case, pick target is the steal target, meaning that
		// we don't need to move the value. Instead copy the
		// register value onto memory stack, and swap the locations.
		if stealTarget == pickTarget {
			loc := &valueLocation{register: &reg}
			b.locationStack.push(loc)
			return nil
		}

		if pickTarget.onRegister() {
			// Copy the value of pickTarget into stealTarget.
			prog := b.newProg()
			prog.As = x86.AMOVQ
			prog.From.Type = obj.TYPE_REG
			prog.From.Reg = *pickTarget.register
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = reg
			b.addInstruction(prog)
		} else if pickTarget.onStack() {
			// Place the stack pointer at first.
			prog := b.newProg()
			prog.As = x86.AMOVQ
			prog.From.Type = obj.TYPE_CONST
			prog.From.Offset = int64(*pickTarget.stackPointer)
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = reg
			b.addInstruction(prog)
			// Then Copy the value from the stack.
			prog = b.newProg()
			prog.As = x86.AMOVQ
			prog.From.Type = obj.TYPE_MEM
			prog.From.Reg = cachedStackBasePointerReg
			prog.From.Index = reg
			prog.From.Scale = 8
			prog.To.Type = obj.TYPE_REG
			prog.To.Reg = reg
			b.addInstruction(prog)
		} else if pickTarget.onConditionalRegister() {
			panic("TODO!")
		}
		return nil
	} else {
		return fmt.Errorf("cannot take free register: %w", err)
	}
}

func (b *amd64Builder) setJITStatus(status jitStatusCodes) *obj.Prog {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(status)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineJITStatusOffset
	b.addInstruction(prog)
	return prog
}

func (b *amd64Builder) callHostFunctionFromConstIndex(index uint32) {
	// Set the jit status as jitStatusCallFunction
	b.setJITStatus(jitStatusCallHostFunction)
	// Set the function index.
	b.setFunctionCallIndexFromConst(index)
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) callHostFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitStatusCallFunction
	b.setJITStatus(jitStatusCallHostFunction)
	// Set the function index.
	b.setFunctionCallIndexFromRegister(reg)
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

func (b *amd64Builder) callFunctionFromConstIndex(index uint32) (last *obj.Prog) {
	// Set the jit status as jitStatusCallFunction
	b.setJITStatus(jitStatusCallFunction)
	// Set the function index.
	b.setFunctionCallIndexFromConst(index)
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	last = b.initializeReservedRegisters()
	return
}

func (b *amd64Builder) callFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitStatusCallFunction
	b.setJITStatus(jitStatusCallFunction)
	// Set the function index.
	b.setFunctionCallIndexFromRegister(reg)
	// Set the continuation offset on the next instruction.
	b.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	b.initializeReservedRegisters()
}

// TODO: If this function call is the tail call,
// we don't need to return back to this function.
// Maybe better have another status for that case,
// so that we don't call back again to this function
// and instead just release the call frame.
func (b *amd64Builder) setContinuationOffsetAtNextInstructionAndReturn() {
	// Create the instruction for setting offset.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(0) // Place holder!
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineContinuationAddressOffset
	b.addInstruction(prog)
	// Then return temporarily -- giving control to normal Go code.
	b.returnFunction()
	// As we cannot read RIP register directly,
	// we calculate now the offset to the next instruction
	// relative to the beginning of this function body.
	prog.From.Offset = int64(len(b.builder.Assemble()))
}

func (b *amd64Builder) setFunctionCallIndexFromRegister(reg int16) {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	b.addInstruction(prog)
}

func (b *amd64Builder) setFunctionCallIndexFromConst(index uint32) {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(index)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	b.addInstruction(prog)
}

func (b *amd64Builder) movConstToRegister(val int64, targetRegister int16) *obj.Prog {
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = val
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	b.addInstruction(prog)
	return prog
}

func (b *amd64Builder) pushRegisterToStack(fromReg int16) {
	// Push value.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = cachedStackBasePointerReg
	prog.To.Index = cachedStackPointerReg
	prog.To.Scale = 8
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = fromReg
	b.addInstruction(prog)

	// Increment cached stack pointer.
	prog = b.newProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	b.addInstruction(prog)
	b.memoryStackPointer++
}

func (b *amd64Builder) popFromStackToRegister(toReg int16) {
	// Decrement the cached stack pointer.
	prog := b.newProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	b.addInstruction(prog)

	// Pop value to the resgister.
	prog = b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = cachedStackBasePointerReg
	prog.From.Index = cachedStackPointerReg
	prog.From.Scale = 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = toReg
	b.addInstruction(prog)
}

func (b *amd64Builder) returnFunction() {
	// Write back the cached SP to the actual eng.sp.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = cachedStackPointerReg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineCurrentStackPointerOffset
	b.addInstruction(prog)

	// Return.
	ret := b.newProg()
	ret.As = obj.ARET
	b.addInstruction(ret)
}

// initializeReservedRegisters must be called at the very beginning and all the
// after-call continuations of JITed functions,
func (b *amd64Builder) initializeReservedRegisters() *obj.Prog {
	// Cache the current stack pointer (engine.currentStackPointer).
	// movq cachedStackPointerReg [engineInstanceReg+engineCurrentStackPointerOffset]
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = engineInstanceReg
	prog.From.Offset = engineCurrentStackPointerOffset
	// Push to cachedStackPointerReg.
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	b.addInstruction(prog)

	// Cache the actual stack base pointer (engine.currentBaseStackPointer*8+[engine.engineStackSliceOffset])
	// to cachedStackBasePointerReg
	// At first, make cachedStackBasePointerReg point to the beginning of the slice backing array.
	// movq [engineInstanceReg+engineStackSliceOffset] cachedStackBasePointerReg
	prog = b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = engineInstanceReg
	prog.From.Offset = engineStackSliceOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackBasePointerReg
	b.addInstruction(prog)

	// Next we move the base pointer (engine.currentBaseStackPointer) to
	// a temporary register. Here we use tmpRegister=rax but anything un-reserved is fine.
	// movq [engineInstanceReg+engineCurrentBaseStackPointerOffset] tmpRegister
	const tmpRegister = x86.REG_AX
	prog = b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = engineInstanceReg
	prog.From.Offset = engineCurrentBaseStackPointerOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = tmpRegister
	b.addInstruction(prog)

	// Multiply tmpRegister with 8 via shift left with 3.
	// shlq $3 tmpRegister
	prog = b.newProg()
	prog.As = x86.ASHLQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = tmpRegister
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = 3
	b.addInstruction(prog)

	// Finally we add the tmpRegister to cachedStackBasePointerReg.
	// addq [tmpRegister] cachedStackBasePointerReg
	prog = b.newProg()
	prog.As = x86.AADDQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackBasePointerReg
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = tmpRegister
	b.addInstruction(prog)
	return prog
}
