//go:build amd64
// +build amd64

package jit

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	asm "github.com/twitchyliquid64/golang-asm"
	"github.com/twitchyliquid64/golang-asm/obj"
	"github.com/twitchyliquid64/golang-asm/obj/x86"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/wazeroir"
)

func jitcall(codeSegment, engine, memory uintptr)

// Reserved registers.
// Note that we don't use "call" instruction (See wasm/jit/RATIONALE.md#general-limitations-on-pure-go-jit-engines)
const (
	// engineInstanceReg R12: pointer to engine instance (i.e. *engine as uintptr)
	engineInstanceReg = x86.REG_R12
	// cachedStackBasePointerReg R14: cached stack base pointer (engine.currentStackBase) in the current function call.
	cachedStackBasePointerReg = x86.REG_R14
	// TODO: we use memoryReg later when we support the store/load memory operations.
	// memoryReg R15: pointer to memory space (i.e. *[]byte as uintptr).
	// memoryReg                 = x86.REG_R15
)

func (e *engine) compileWasmFunction(f *wasm.FunctionInstance) (*compiledWasmFunction, error) {
	ir, err := wazeroir.Compile(f)
	if err != nil {
		return nil, fmt.Errorf("failed to lower to wazeroir: %w", err)
	}

	// TODO: delete
	fmt.Printf("compilation target wazeroir:\n%s\n%v\n", wazeroir.Format(ir.Operations), ir.LabelCallers)

	// We can chose arbitrary number instead of 1024 which indicates the cache size in the builder.
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
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationLabel:
			if err := builder.handleLabel(o); err != nil {
				return nil, fmt.Errorf("error handling label operation %s: %w", o, err)
			}
		case *wazeroir.OperationBr:
			if err := builder.handleBr(o); err != nil {
				return nil, fmt.Errorf("error handling br operation %v: %w", o, err)
			}
		case *wazeroir.OperationBrIf:
			if err := builder.handleBrIf(o); err != nil {
				return nil, fmt.Errorf("error handling br_if operation %v: %w", o, err)
			}
		case *wazeroir.OperationBrTable:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationCall:
			if err := builder.handleCall(o); err != nil {
				return nil, fmt.Errorf("error handling call operation %v: %w", o, err)
			}
		case *wazeroir.OperationCallIndirect:
		case *wazeroir.OperationDrop:
			if err := builder.handleDrop(o); err != nil {
				return nil, fmt.Errorf("error handling drop operation %v: %w", o, err)
			}
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
			if err := builder.handleConstI64(o); err != nil {
				return nil, fmt.Errorf("error handling i64.const operation %v: %w", o, err)
			}
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
			if err := builder.handleLe(o); err != nil {
				return nil, fmt.Errorf("error handling le operation %v: %w", o, err)
			}
		case *wazeroir.OperationGe:
			return nil, fmt.Errorf("unsupported operation in JIT compiler: %v", o)
		case *wazeroir.OperationAdd:
			if err := builder.handleAdd(o); err != nil {
				return nil, fmt.Errorf("error handling add operation %v: %w", o, err)
			}
		case *wazeroir.OperationSub:
			if err := builder.handleSub(o); err != nil {
				return nil, fmt.Errorf("error handling sub operation %v: %w", o, err)
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

	code, err := builder.assemble()
	if err != nil {
		return nil, fmt.Errorf("failed to assemble: %w", err)
	}
	cf := &compiledWasmFunction{
		codeSegment: code,
		inputs:      uint64(len(f.Signature.InputTypes)),
		returns:     uint64(len(f.Signature.ReturnTypes)),
	}
	cf.memory = f.ModuleInstance.Memory
	if cf.memory != nil {
		cf.memoryAddress = uintptr(unsafe.Pointer(&cf.memory.Buffer[0]))
	}
	cf.codeInitialAddress = uintptr(unsafe.Pointer(&cf.codeSegment[0]))

	return cf, nil
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

func (b *amd64Builder) assemble() ([]byte, error) {
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
	// If the next instruction is jmp and , we can omit this instruction
	// as in anycase we can jump to the next next instruction.
	if prog.As == obj.AJMP && b.setJmpOrigin != nil && b.setJmpOrigin.As == obj.AJMP {
		return
	}
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
		if cond.onStack() {
			if err := b.moveStackToRegister(cond.registerType(), cond); err != nil {
				return err
			}
		}
		// Check if the value equals zero
		prog := b.newProg()
		prog.As = x86.ACMPQ
		prog.From.Type = obj.TYPE_REG
		prog.From.Reg = cond.register
		prog.To.Type = obj.TYPE_CONST
		prog.To.Offset = 0
		b.addInstruction(prog)
		// Then emit jump instruction.
		jmpWithCond = b.newProg()
		jmpWithCond.As = x86.AJEQ
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
		index := b.eng.hostFunctionIndex[target]
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
			if err := b.moveStackToRegister(live.registerType(), live); err != nil {
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
		prog.From.Reg = cachedStackBasePointerReg
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
		tp = gpTypeInt
		panic("add tests!")
	case wazeroir.SignLessTypeI64:
		instruction = x86.AADDQ
		tp = gpTypeInt
	case wazeroir.SignLessTypeF32:
		instruction = x86.AADDSS
		tp = gpTypeFloat
		panic("add tests!")
	case wazeroir.SignLessTypeF64:
		instruction = x86.AADDSD
		tp = gpTypeFloat
		panic("add tests!")
	}

	x2 := b.locationStack.pop()
	if x2.onStack() {
		if err := b.moveStackToRegister(tp, x2); err != nil {
			return err
		}
	} else if x2.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x2); err != nil {
			return err
		}
	}

	x1 := b.locationStack.peek() // Note this is peek, pop!
	if x1.onStack() {
		if err := b.moveStackToRegister(tp, x1); err != nil {
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
		tp = gpTypeInt
		panic("add tests!")
	case wazeroir.SignLessTypeI64:
		instruction = x86.ASUBQ
		tp = gpTypeInt
	case wazeroir.SignLessTypeF32:
		instruction = x86.ASUBSS
		tp = gpTypeFloat
		panic("add tests!")
	case wazeroir.SignLessTypeF64:
		instruction = x86.ASUBSD
		tp = gpTypeFloat
		panic("add tests!")
	}

	x2 := b.locationStack.pop()
	if x2.onStack() {
		if err := b.moveStackToRegister(tp, x2); err != nil {
			return err
		}
	} else if x2.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x2); err != nil {
			return err
		}
	}

	x1 := b.locationStack.peek() // Note this is peek, pop!
	if x1.onStack() {
		if err := b.moveStackToRegister(tp, x1); err != nil {
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
		tp = gpTypeInt
	case wazeroir.SignFulTypeUint32:
		resultConditionState = conditionalRegisterStateBE
		instruction = x86.ACMPL
		tp = gpTypeInt
	case wazeroir.SignFulTypeInt64:
		resultConditionState = conditionalRegisterStateLE
		instruction = x86.ACMPQ
		tp = gpTypeInt
	case wazeroir.SignFulTypeUint64:
		resultConditionState = conditionalRegisterStateBE
		instruction = x86.ACMPQ
		tp = gpTypeInt
	case wazeroir.SignFulTypeFloat32:
		tp = gpTypeFloat
		panic("add test!")
	case wazeroir.SignFulTypeFloat64:
		tp = gpTypeFloat
		panic("add test!")
	}

	x2 := b.locationStack.pop()
	if x2.onStack() {
		if err := b.moveStackToRegister(tp, x2); err != nil {
			return err
		}
	} else if x2.onConditionalRegister() {
		if err := b.moveConditionalToGPRegister(x2); err != nil {
			return err
		}
	}

	x1 := b.locationStack.pop()
	if x1.onStack() {
		if err := b.moveStackToRegister(tp, x1); err != nil {
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

func (b *amd64Builder) handleConstI64(o *wazeroir.OperationConstI64) error {
	reg, err := b.allocateRegister(gpTypeInt)
	if err != nil {
		return err
	}
	loc := b.locationStack.pushValueOnRegister(reg)
	loc.setValueType(wazeroir.SignLessTypeI64)
	b.movConstToRegister(int64(o.Value), reg)
	return nil
}

func (b *amd64Builder) moveStackToRegister(tp generalPurposeRegisterType, loc *valueLocation) error {
	// Allocate the register.
	reg, err := b.allocateRegister(tp)
	if err != nil {
		return err
	}

	// Then copy the value from the stack.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = cachedStackBasePointerReg
	prog.From.Offset = int64(loc.stackPointer) * 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = reg
	b.addInstruction(prog)

	// Mark it uses the register.
	loc.setRegister(reg)
	b.locationStack.markRegisterUsed(reg)
	return nil
}

func (b *amd64Builder) moveConditionalToGPRegister(loc *valueLocation) error {
	// Get the free register.
	reg, ok := b.locationStack.takeFreeRegister(gpTypeInt)
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
	b.releaseRegister(stealTarget)
	return
}

func (b *amd64Builder) setJITStatus(status jitCallStatusCode) *obj.Prog {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(status)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineJITCallStatusCodeOffset
	b.addInstruction(prog)
	return prog
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

func (b *amd64Builder) callFunctionFromConstIndex(index int64) (last *obj.Prog) {
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
	last = b.initializeReservedRegisters()
	return
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
			b.releaseRegister(loc)
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
	tmpReg, _ := b.locationStack.takeFreeRegister(gpTypeInt)
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
	prog.To.Reg = engineInstanceReg
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
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	b.addInstruction(prog)
}

func (b *amd64Builder) setFunctionCallIndexFromConst(index int64) {
	prog := b.newProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = index
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

func (b *amd64Builder) releaseRegister(loc *valueLocation) {
	// Push value.
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = cachedStackBasePointerReg
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
	prog.From.Reg = cachedStackBasePointerReg
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
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineCurrentStackPointerOffset
	b.addInstruction(prog)

	// Return.
	ret := b.newProg()
	ret.As = obj.ARET
	b.addInstruction(ret)
}

// initializeReservedRegisters must be called at the very beginning and all the
// after-call continuations of JITed functions.
// This caches the actual stack base pointer (engine.currentBaseStackPointer*8+[engine.engineStackSliceOffset])
// to cachedStackBasePointerReg
func (b *amd64Builder) initializeReservedRegisters() *obj.Prog {
	// At first, make cachedStackBasePointerReg point to the beginning of the slice backing array.
	// movq [engineInstanceReg+engineStackSliceOffset] cachedStackBasePointerReg
	prog := b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = engineInstanceReg
	prog.From.Offset = engineStackSliceOffset
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackBasePointerReg
	b.addInstruction(prog)

	// initializeReservedRegisters is called at the beginning of function calls
	// or right after function returns so at this point we always have free registers.
	reg, _ := b.locationStack.takeFreeRegister(gpTypeInt)

	// Next we move the base pointer (engine.currentBaseStackPointer) to
	// a temporary register.
	// movq [engineInstanceReg+engineCurrentBaseStackPointerOffset] reg
	prog = b.newProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = engineInstanceReg
	prog.From.Offset = engineCurrentBaseStackPointerOffset
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
	prog.To.Reg = cachedStackBasePointerReg
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	b.addInstruction(prog)
	return prog
}
