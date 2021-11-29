//go:build amd64
// +build amd64

package jit

import (
	"fmt"

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
// R13: cacahed stack pointer of engine.stack which should be rewritten into engine.sp
// 		whenever we exit JIT calls.
// R14: cached pointer to the beginning of backing array of the stack.
// R15: pointer to memory space (i.e. *[]byte as uintptr).
const (
	engineInstanceReg     = x86.REG_R12
	cachedStackPointerReg = x86.REG_R13
	cachedStackSliceReg   = x86.REG_R14
	memoryReg             = x86.REG_R15
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
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
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
			// TODO:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
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
}

func (a *amd64Builder) assemble() ([]byte, error) {
	code, err := mmapCodeSegment(a.builder.Assemble())
	return code, err
}

func (a *amd64Builder) addInstruction(prog *obj.Prog) {
	a.builder.AddInstruction(prog)
}

func (a *amd64Builder) newProg() (prog *obj.Prog) {
	return a.builder.NewProg()
}

func (a *amd64Builder) setJITStatus(status jitStatusCodes) *obj.Prog {
	prog := a.builder.NewProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(status)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineJITStatusOffset
	a.builder.AddInstruction(prog)
	return prog
}

func (a *amd64Builder) callHostFunctionFromConstIndex(index uint32) {
	// Set the jit status as jitStatusCallFunction
	a.setJITStatus(jitStatusCallHostFunction)
	// Set the function index.
	a.setFunctionCallIndexFromConst(index)
	// Set the continuation offset on the next instruction.
	a.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	a.initializeReservedRegisters()
}

func (a *amd64Builder) callHostFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitStatusCallFunction
	a.setJITStatus(jitStatusCallHostFunction)
	// Set the function index.
	a.setFunctionCallIndexFromRegister(reg)
	// Set the continuation offset on the next instruction.
	a.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	a.initializeReservedRegisters()
}

func (a *amd64Builder) callFunctionFromConstIndex(index uint32) {
	// Set the jit status as jitStatusCallFunction
	a.setJITStatus(jitStatusCallFunction)
	// Set the function index.
	a.setFunctionCallIndexFromConst(index)
	// Set the continuation offset on the next instruction.
	a.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	a.initializeReservedRegisters()
}

func (a *amd64Builder) callFunctionFromRegisterIndex(reg int16) {
	// Set the jit status as jitStatusCallFunction
	a.setJITStatus(jitStatusCallFunction)
	// Set the function index.
	a.setFunctionCallIndexFromRegister(reg)
	// Set the continuation offset on the next instruction.
	a.setContinuationOffsetAtNextInstructionAndReturn()
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	a.initializeReservedRegisters()
}

// TODO: If this function call is the tail call,
// we don't need to return back to this function.
// Maybe better have another status for that case,
// so that we don't call back again to this function
// and instead just release the call frame.
func (a *amd64Builder) setContinuationOffsetAtNextInstructionAndReturn() {
	// Create the instruction for setting offset.
	prog := a.builder.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(0) // Place holder!
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineContinuationAddressOffset
	a.builder.AddInstruction(prog)
	// Then return temporarily -- giving control to normal Go code.
	a.returnFunction()
	// As we cannot read RIP register directly,
	// we calculate now the offset to the next instruction
	// relative to the beginning of this function body.
	prog.From.Offset = int64(len(a.builder.Assemble()))
}

func (a *amd64Builder) setFunctionCallIndexFromRegister(reg int16) {
	prog := a.builder.NewProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	a.builder.AddInstruction(prog)
}

func (a *amd64Builder) setFunctionCallIndexFromConst(index uint32) {
	prog := a.builder.NewProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(index)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	a.builder.AddInstruction(prog)
}

func (a *amd64Builder) movConstToRegister(val int64, targetRegister int16) *obj.Prog {
	prog := a.builder.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = val
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	a.builder.AddInstruction(prog)
	return prog
}

func (a *amd64Builder) pushRegisterToStack(fromReg int16) {
	// Push value.
	prog := a.builder.NewProg()
	prog.As = x86.AMOVQ
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = cachedStackSliceReg
	prog.To.Index = cachedStackPointerReg
	prog.To.Scale = 8
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = fromReg
	a.builder.AddInstruction(prog)

	// Increment cached stack pointer.
	prog = a.builder.NewProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	a.builder.AddInstruction(prog)
}

func (a *amd64Builder) popFromStackToRegister(toReg int16) {
	// Decrement the cached stack pointer.
	prog := a.builder.NewProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	a.builder.AddInstruction(prog)

	// Pop value to the resgister.
	prog = a.builder.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = cachedStackSliceReg
	prog.From.Index = cachedStackPointerReg
	prog.From.Scale = 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = toReg
	a.builder.AddInstruction(prog)
}

func (a *amd64Builder) returnFunction() {
	// Write back the cached SP to the actual eng.sp.
	prog := a.builder.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = cachedStackPointerReg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineSPOffset
	a.builder.AddInstruction(prog)

	// Return.
	ret := a.builder.NewProg()
	ret.As = obj.ARET
	a.builder.AddInstruction(ret)
}

func (a *amd64Builder) initializeReservedRegisters() {
	// Cache the current stack pointer (engine.sp).
	// movq cachedStackPointerReg [engineInstanceReg+engineSPOffset]
	{
		prog := a.builder.NewProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_MEM
		prog.From.Reg = engineInstanceReg
		prog.From.Offset = engineSPOffset
		// Push to cachedStackPointerReg.
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = cachedStackPointerReg
		a.builder.AddInstruction(prog)
	}
	// Cache the pointer to the current stack backing array.
	// movq cachedStackSliceReg [engineInstanceReg+engineStackOffset]
	{
		prog := a.builder.NewProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_MEM
		prog.From.Reg = engineInstanceReg
		prog.From.Offset = engineStackOffset
		// Push to cachedStackPointerReg.
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = cachedStackSliceReg
		a.builder.AddInstruction(prog)
	}
}
