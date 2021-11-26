//go:build amd64
// +build amd64

package jit

import (
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

func setJITStatus(b *asm.Builder, status jitStatusCodes) *obj.Prog {
	prog := b.NewProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(status)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineJITStatusOffset
	b.AddInstruction(prog)
	return prog
}

func callHostFunctionFromConstIndex(b *asm.Builder, index uint32) {
	// Set the jit status as jitStatusCallFunction
	setJITStatus(b, jitStatusCallHostFunction)
	// Set the function index.
	setFunctionCallIndexFromConst(b, index)
	// Set the continuation offset on the next instruction.
	setContinuationOffsetAtNextInstructionAndReturn(b)
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	initializeReservedRegisters(b)
}

func callHostFunctionFromRegisterIndex(b *asm.Builder, reg int16) {
	// Set the jit status as jitStatusCallFunction
	setJITStatus(b, jitStatusCallHostFunction)
	// Set the function index.
	setFunctionCallIndexFromRegister(b, reg)
	// Set the continuation offset on the next instruction.
	setContinuationOffsetAtNextInstructionAndReturn(b)
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	initializeReservedRegisters(b)
}

func callFunctionFromConstIndex(b *asm.Builder, index uint32) {
	// Set the jit status as jitStatusCallFunction
	setJITStatus(b, jitStatusCallFunction)
	// Set the function index.
	setFunctionCallIndexFromConst(b, index)
	// Set the continuation offset on the next instruction.
	setContinuationOffsetAtNextInstructionAndReturn(b)
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	initializeReservedRegisters(b)
}

func callFunctionFromRegisterIndex(b *asm.Builder, reg int16) {
	// Set the jit status as jitStatusCallFunction
	setJITStatus(b, jitStatusCallFunction)
	// Set the function index.
	setFunctionCallIndexFromRegister(b, reg)
	// Set the continuation offset on the next instruction.
	setContinuationOffsetAtNextInstructionAndReturn(b)
	// Once the returns from the function call,
	// we must setup the reserved registers again.
	initializeReservedRegisters(b)
}

func setContinuationOffsetAtNextInstructionAndReturn(b *asm.Builder) {
	// Create the instruction for setting offset.
	prog := b.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(0) // Place holder!
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineContinuationAddressOffset
	b.AddInstruction(prog)
	// Then return temporarily -- giving control to normal Go code.
	returnFunction(b)
	// As we cannot read RIP register directly,
	// we calculate now the offset to the next instruction
	// relative to the beginning of this function body.
	prog.From.Offset = int64(len(b.Assemble()))
}

func setFunctionCallIndexFromRegister(b *asm.Builder, reg int16) {
	prog := b.NewProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = reg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	b.AddInstruction(prog)
}

func setFunctionCallIndexFromConst(b *asm.Builder, index uint32) {
	prog := b.NewProg()
	prog.As = x86.AMOVL
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = int64(index)
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineFunctionCallIndexOffset
	b.AddInstruction(prog)
}

func movConstToRegister(b *asm.Builder, val int64, targetRegister int16) *obj.Prog {
	prog := b.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_CONST
	prog.From.Offset = val
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = targetRegister
	b.AddInstruction(prog)
	return prog
}

func pushRegisterToStack(b *asm.Builder, fromReg int16) {
	// Push value.
	prog := b.NewProg()
	prog.As = x86.AMOVQ
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = cachedStackSliceReg
	prog.To.Index = cachedStackPointerReg
	prog.To.Scale = 8
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = fromReg
	b.AddInstruction(prog)

	// Increment cached stack pointer.
	prog = b.NewProg()
	prog.As = x86.AINCQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	b.AddInstruction(prog)
}

func popFromStackToRegister(b *asm.Builder, toReg int16) {
	// Decrement the cached stack pointer.
	prog := b.NewProg()
	prog.As = x86.ADECQ
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = cachedStackPointerReg
	b.AddInstruction(prog)

	// Pop value to the resgister.
	prog = b.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_MEM
	prog.From.Reg = cachedStackSliceReg
	prog.From.Index = cachedStackPointerReg
	prog.From.Scale = 8
	prog.To.Type = obj.TYPE_REG
	prog.To.Reg = toReg
	b.AddInstruction(prog)
}

func returnFunction(b *asm.Builder) {
	// Write back the cached SP to the actual eng.sp.
	prog := b.NewProg()
	prog.As = x86.AMOVQ
	prog.From.Type = obj.TYPE_REG
	prog.From.Reg = cachedStackPointerReg
	prog.To.Type = obj.TYPE_MEM
	prog.To.Reg = engineInstanceReg
	prog.To.Offset = engineSPOffset
	b.AddInstruction(prog)

	// Return.
	ret := b.NewProg()
	ret.As = obj.ARET
	b.AddInstruction(ret)
}

func initializeReservedRegisters(b *asm.Builder) {
	// Cache the current stack pointer (engine.sp).
	// movq cachedStackPointerReg [engineInstanceReg+engineSPOffset]
	{
		prog := b.NewProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_MEM
		prog.From.Reg = engineInstanceReg
		prog.From.Offset = engineSPOffset
		// Push to cachedStackPointerReg.
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = cachedStackPointerReg
		b.AddInstruction(prog)
	}
	// Cache the pointer to the current stack backing array.
	// movq cachedStackSliceReg [engineInstanceReg+engineStackOffset]
	{
		prog := b.NewProg()
		prog.As = x86.AMOVQ
		prog.From.Type = obj.TYPE_MEM
		prog.From.Reg = engineInstanceReg
		prog.From.Offset = engineStackOffset
		// Push to cachedStackPointerReg.
		prog.To.Type = obj.TYPE_REG
		prog.To.Reg = cachedStackSliceReg
		b.AddInstruction(prog)
	}
}
