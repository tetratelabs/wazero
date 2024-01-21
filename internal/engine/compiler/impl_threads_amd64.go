package compiler

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func (c *amd64Compiler) compileAtomicLoad(o *wazeroir.UnionOperation) error {
	var (
		inst              asm.Instruction
		targetSizeInBytes int64
		vt                runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		inst = amd64.MOVL
		targetSizeInBytes = 32 / 8
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		inst = amd64.MOVQ
		targetSizeInBytes = 64 / 8
		vt = runtimeValueTypeI64
	}

	return c.compileAtomicLoadImpl(inst, offset, targetSizeInBytes, vt)
}

func (c *amd64Compiler) compileAtomicLoad8(o *wazeroir.UnionOperation) error {
	var (
		inst asm.Instruction
		vt   runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		inst = amd64.MOVBLZX
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		inst = amd64.MOVBQZX
		vt = runtimeValueTypeI64
	}

	return c.compileAtomicLoadImpl(inst, offset, 1, vt)
}

func (c *amd64Compiler) compileAtomicLoad16(o *wazeroir.UnionOperation) error {
	var (
		inst asm.Instruction
		vt   runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		inst = amd64.MOVWLZX
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		inst = amd64.MOVWQZX
		vt = runtimeValueTypeI64
	}

	return c.compileAtomicLoadImpl(inst, offset, 16/8, vt)
}

func (c *amd64Compiler) compileAtomicLoadImpl(
	inst asm.Instruction, offset uint32, targetSizeInBytes int64, resultRuntimeValueType runtimeValueType,
) error {
	reg, err := c.compileMemoryAccessCeilSetup(offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)

	c.assembler.CompileMemoryWithIndexToRegister(inst,
		// we access memory as memory.Buffer[ceil-targetSizeInBytes: ceil].
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
		reg)
	c.pushRuntimeValueLocationOnRegister(reg, resultRuntimeValueType)

	return nil
}

func (c *amd64Compiler) compileAtomicStore(o *wazeroir.UnionOperation) error {
	var inst asm.Instruction
	var targetSizeInByte int64
	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)
	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		inst = amd64.XCHGL
		targetSizeInByte = 32 / 8
	case wazeroir.UnsignedTypeI64:
		inst = amd64.XCHGQ
		targetSizeInByte = 64 / 8
	}
	return c.compileAtomicStoreImpl(inst, offset, targetSizeInByte)
}

func (c *amd64Compiler) compileAtomicStore8(o *wazeroir.UnionOperation) error {
	return c.compileAtomicStoreImpl(amd64.XCHGB, uint32(o.U2), 1)
}

func (c *amd64Compiler) compileAtomicStore16(o *wazeroir.UnionOperation) error {
	return c.compileAtomicStoreImpl(amd64.XCHGW, uint32(o.U2), 16/8)
}

func (c *amd64Compiler) compileAtomicStoreImpl(
	inst asm.Instruction, offset uint32, targetSizeInBytes int64,
) error {
	val := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(val); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)

	c.assembler.CompileRegisterToMemoryWithIndex(
		inst, val.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
	)

	// We no longer need both the value and base registers.
	c.locationStack.releaseRegister(val)
	c.locationStack.markRegisterUnused(reg)
	return nil
}

func (c *amd64Compiler) compileAtomicRMW(o *wazeroir.UnionOperation) error {
	var (
		inst              asm.Instruction
		targetSizeInBytes int64
		vt                runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	op := wazeroir.AtomicArithmeticOp(o.B2)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		targetSizeInBytes = 32 / 8
		vt = runtimeValueTypeI32
		switch op {
		case wazeroir.AtomicArithmeticOpAdd:
			return c.compileAtomicAddImpl(amd64.XADDL, offset, false, targetSizeInBytes, vt)
		case wazeroir.AtomicArithmeticOpSub:
			return c.compileAtomicAddImpl(amd64.XADDL, offset, true, targetSizeInBytes, vt)
		case wazeroir.AtomicArithmeticOpAnd:
			inst = amd64.ANDL
		case wazeroir.AtomicArithmeticOpOr:
			inst = amd64.ORL
		case wazeroir.AtomicArithmeticOpXor:
			inst = amd64.XORL
		case wazeroir.AtomicArithmeticOpNop:
			return c.compileAtomicXchgImpl(amd64.XCHGL, offset, targetSizeInBytes, vt)
		}
	case wazeroir.UnsignedTypeI64:
		targetSizeInBytes = 64 / 8
		vt = runtimeValueTypeI64
		switch op {
		case wazeroir.AtomicArithmeticOpAdd:
			return c.compileAtomicAddImpl(amd64.XADDQ, offset, false, targetSizeInBytes, vt)
		case wazeroir.AtomicArithmeticOpSub:
			return c.compileAtomicAddImpl(amd64.XADDQ, offset, true, targetSizeInBytes, vt)
		case wazeroir.AtomicArithmeticOpAnd:
			inst = amd64.ANDQ
		case wazeroir.AtomicArithmeticOpOr:
			inst = amd64.ORQ
		case wazeroir.AtomicArithmeticOpXor:
			inst = amd64.XORQ
		case wazeroir.AtomicArithmeticOpNop:
			return c.compileAtomicXchgImpl(amd64.XCHGQ, offset, targetSizeInBytes, vt)
		}
	}

	return c.compileAtomicRMWCASLoopImpl(inst, offset, targetSizeInBytes, vt)
}

func (c *amd64Compiler) compileAtomicRMW8(o *wazeroir.UnionOperation) error {
	var (
		inst asm.Instruction
		vt   runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	op := wazeroir.AtomicArithmeticOp(o.B2)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}

	switch op {
	case wazeroir.AtomicArithmeticOpAdd:
		return c.compileAtomicAddImpl(amd64.XADDB, offset, false, 1, vt)
	case wazeroir.AtomicArithmeticOpSub:
		return c.compileAtomicAddImpl(amd64.XADDB, offset, true, 1, vt)
	case wazeroir.AtomicArithmeticOpAnd:
		inst = amd64.ANDL
	case wazeroir.AtomicArithmeticOpOr:
		inst = amd64.ORL
	case wazeroir.AtomicArithmeticOpXor:
		inst = amd64.XORL
	case wazeroir.AtomicArithmeticOpNop:
		return c.compileAtomicXchgImpl(amd64.XCHGB, offset, 1, vt)
	}

	return c.compileAtomicRMWCASLoopImpl(inst, offset, 1, vt)
}

func (c *amd64Compiler) compileAtomicRMW16(o *wazeroir.UnionOperation) error {
	var (
		inst asm.Instruction
		vt   runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	op := wazeroir.AtomicArithmeticOp(o.B2)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}

	switch op {
	case wazeroir.AtomicArithmeticOpAdd:
		return c.compileAtomicAddImpl(amd64.XADDW, offset, false, 16/8, vt)
	case wazeroir.AtomicArithmeticOpSub:
		return c.compileAtomicAddImpl(amd64.XADDW, offset, true, 16/8, vt)
	case wazeroir.AtomicArithmeticOpAnd:
		inst = amd64.ANDL
	case wazeroir.AtomicArithmeticOpOr:
		inst = amd64.ORL
	case wazeroir.AtomicArithmeticOpXor:
		inst = amd64.XORL
	case wazeroir.AtomicArithmeticOpNop:
		return c.compileAtomicXchgImpl(amd64.XCHGW, offset, 16/8, vt)
	}

	return c.compileAtomicRMWCASLoopImpl(inst, offset, 16/8, vt)
}

func (c *amd64Compiler) compileAtomicAddImpl(inst asm.Instruction, offsetConst uint32, negateArg bool, targetSizeInBytes int64, resultRuntimeValueType runtimeValueType) error {
	val := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(val); err != nil {
		return err
	}

	if negateArg {
		var negArg asm.Instruction
		switch targetSizeInBytes {
		case 1:
			negArg = amd64.NEGB
		case 2:
			negArg = amd64.NEGW
		case 4:
			negArg = amd64.NEGL
		case 8:
			negArg = amd64.NEGQ
		}
		c.assembler.CompileNoneToRegister(negArg, val.register)
	}

	reg, err := c.compileMemoryAccessCeilSetup(offsetConst, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)

	c.assembler.CompileRegisterToMemoryWithIndexAndLock(
		inst, val.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
	)

	if targetSizeInBytes < 4 {
		mask := (1 << (8 * targetSizeInBytes)) - 1
		c.assembler.CompileConstToRegister(amd64.ANDQ, int64(mask), val.register)
	}

	c.locationStack.markRegisterUnused(reg)
	c.locationStack.pushRuntimeValueLocationOnRegister(val.register, resultRuntimeValueType)

	return nil
}

func (c *amd64Compiler) compileAtomicXchgImpl(inst asm.Instruction, offsetConst uint32, targetSizeInBytes int64, resultRuntimeValueType runtimeValueType) error {
	val := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(val); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offsetConst, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)

	c.assembler.CompileRegisterToMemoryWithIndex(
		inst, val.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
	)

	if targetSizeInBytes < 4 {
		mask := (1 << (8 * targetSizeInBytes)) - 1
		c.assembler.CompileConstToRegister(amd64.ANDQ, int64(mask), val.register)
	}

	c.locationStack.markRegisterUnused(reg)
	c.locationStack.pushRuntimeValueLocationOnRegister(val.register, resultRuntimeValueType)

	return nil
}

func (c *amd64Compiler) compileAtomicRMWCASLoopImpl(rmwInst asm.Instruction,
	offsetConst uint32, targetSizeInBytes int64, resultRuntimeValueType runtimeValueType,
) error {
	const resultRegister = amd64.RegAX

	var copyInst asm.Instruction
	var loadInst asm.Instruction
	var cmpXchgInst asm.Instruction

	switch targetSizeInBytes {
	case 8:
		copyInst = amd64.MOVQ
		loadInst = amd64.MOVQ
		cmpXchgInst = amd64.CMPXCHGQ
	case 4:
		copyInst = amd64.MOVL
		loadInst = amd64.MOVL
		cmpXchgInst = amd64.CMPXCHGL
	case 2:
		copyInst = amd64.MOVL
		loadInst = amd64.MOVWLZX
		cmpXchgInst = amd64.CMPXCHGW
	case 1:
		copyInst = amd64.MOVL
		loadInst = amd64.MOVBLZX
		cmpXchgInst = amd64.CMPXCHGB
	}

	c.onValueReleaseRegisterToStack(resultRegister)
	c.locationStack.markRegisterUsed(resultRegister)

	tmp, err := c.allocateRegister(registerTypeGeneralPurpose)
	if err != nil {
		return err
	}
	c.locationStack.markRegisterUsed(tmp)

	val := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(val); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offsetConst, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)

	if targetSizeInBytes < 32 {
		mask := (1 << (8 * targetSizeInBytes)) - 1
		c.assembler.CompileConstToRegister(amd64.ANDQ, int64(mask), val.register)
	}

	beginLoop := c.assembler.CompileStandAlone(amd64.NOP)
	c.assembler.CompileRegisterToRegister(copyInst, val.register, tmp)
	c.assembler.CompileMemoryWithIndexToRegister(
		loadInst, amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1, resultRegister)
	if targetSizeInBytes < 32 {
		mask := (1 << (8 * targetSizeInBytes)) - 1
		c.assembler.CompileConstToRegister(amd64.ANDQ, int64(mask), resultRegister)
	}
	c.assembler.CompileRegisterToRegister(rmwInst, resultRegister, tmp)
	c.assembler.CompileRegisterToMemoryWithIndexAndLock(
		cmpXchgInst, tmp,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
	)
	c.assembler.CompileJump(amd64.JNE).AssignJumpTarget(beginLoop)

	if targetSizeInBytes < 32 {
		mask := (1 << (8 * targetSizeInBytes)) - 1
		c.assembler.CompileConstToRegister(amd64.ANDQ, int64(mask), resultRegister)
	}

	c.locationStack.markRegisterUnused(reg)
	c.locationStack.markRegisterUnused(tmp)
	c.locationStack.markRegisterUnused(val.register)
	c.locationStack.pushRuntimeValueLocationOnRegister(resultRegister, resultRuntimeValueType)

	return nil
}

func (c *amd64Compiler) compileAtomicRMWCmpxchg(o *wazeroir.UnionOperation) error {
	var (
		casInst           asm.Instruction
		targetSizeInBytes int64
		vt                runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		casInst = amd64.CMPXCHGL
		targetSizeInBytes = 32 / 8
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		casInst = amd64.CMPXCHGQ
		targetSizeInBytes = 64 / 8
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWCmpxchgImpl(casInst, offset, targetSizeInBytes, vt)
}

func (c *amd64Compiler) compileAtomicRMW8Cmpxchg(o *wazeroir.UnionOperation) error {
	var vt runtimeValueType

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWCmpxchgImpl(amd64.CMPXCHGB, offset, 1, vt)
}

func (c *amd64Compiler) compileAtomicRMW16Cmpxchg(o *wazeroir.UnionOperation) error {
	var vt runtimeValueType

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWCmpxchgImpl(amd64.CMPXCHGW, offset, 16/8, vt)
}

func (c *amd64Compiler) compileAtomicRMWCmpxchgImpl(inst asm.Instruction, offsetArg uint32, targetSizeInBytes int64, resultRuntimeValueType runtimeValueType) error {
	const resultRegister = amd64.RegAX

	repl := c.locationStack.pop()
	exp := c.locationStack.pop()

	// expected value must be in accumulator register, which will also hold the loaded result.
	if exp.register != resultRegister {
		c.onValueReleaseRegisterToStack(resultRegister)
		if exp.onConditionalRegister() {
			c.compileMoveConditionalToGeneralPurposeRegister(exp, resultRegister)
		} else if exp.onStack() {
			exp.setRegister(resultRegister)
			c.compileLoadValueOnStackToRegister(exp)
			c.locationStack.markRegisterUnused(resultRegister)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.MOVQ, exp.register, resultRegister)
			c.locationStack.releaseRegister(exp)
			exp.setRegister(resultRegister)
			c.locationStack.markRegisterUsed(resultRegister)
		}
	}

	if err := c.compileEnsureOnRegister(repl); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)

	c.assembler.CompileRegisterToMemoryWithIndexAndLock(
		inst, repl.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, reg, 1,
	)

	if targetSizeInBytes < 4 {
		mask := (1 << (8 * targetSizeInBytes)) - 1
		c.assembler.CompileConstToRegister(amd64.ANDQ, int64(mask), resultRegister)
	}

	c.locationStack.markRegisterUnused(reg)
	c.locationStack.markRegisterUnused(repl.register)
	c.locationStack.pushRuntimeValueLocationOnRegister(resultRegister, resultRuntimeValueType)

	return nil
}

func (c *amd64Compiler) compileAtomicMemoryWait(o *wazeroir.UnionOperation) error {
	if err := c.maybeCompileMoveTopConditionalToGeneralPurposeRegister(); err != nil {
		return err
	}

	var (
		vt                runtimeValueType
		targetSizeInBytes int64
		waitFunc          wasm.Index
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
		targetSizeInBytes = 32 / 8
		waitFunc = builtinFunctionMemoryWait32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
		targetSizeInBytes = 64 / 8
		waitFunc = builtinFunctionMemoryWait64
	}

	timeout := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(timeout); err != nil {
		return err
	}
	exp := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(exp); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offset, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.locationStack.markRegisterUsed(reg)
	c.compileMemoryAlignmentCheck(reg, targetSizeInBytes)
	c.assembler.CompileRegisterToRegister(amd64.ADDQ, amd64ReservedRegisterForMemory, reg)
	c.assembler.CompileConstToRegister(amd64.ADDQ, -targetSizeInBytes, reg)

	// Push address, values, and timeout back to read in Go
	c.pushRuntimeValueLocationOnRegister(reg, runtimeValueTypeI64)
	c.pushRuntimeValueLocationOnRegister(exp.register, vt)
	c.pushRuntimeValueLocationOnRegister(timeout.register, runtimeValueTypeI64)
	if err := c.compileCallBuiltinFunction(waitFunc); err != nil {
		return err
	}
	// Address, values and timeout consumed in Go
	c.locationStack.pop()
	c.locationStack.pop()
	c.locationStack.pop()

	// Then, the result was pushed.
	v := c.locationStack.pushRuntimeValueLocationOnStack()
	v.valueType = runtimeValueTypeI32

	c.locationStack.markRegisterUnused(reg)
	c.locationStack.releaseRegister(exp)
	c.locationStack.releaseRegister(timeout)

	// After return, we re-initialize reserved registers just like preamble of functions.
	c.compileReservedStackBasePointerInitialization()
	c.compileReservedMemoryPointerInitialization()

	return nil
}

func (c *amd64Compiler) compileAtomicMemoryNotify(o *wazeroir.UnionOperation) error {
	offset := uint32(o.U2)

	if err := c.maybeCompileMoveTopConditionalToGeneralPurposeRegister(); err != nil {
		return err
	}

	count := c.locationStack.pop()
	if err := c.compileEnsureOnRegister(count); err != nil {
		return err
	}

	reg, err := c.compileMemoryAccessCeilSetup(offset, 4)
	if err != nil {
		return err
	}
	c.compileMemoryAlignmentCheck(reg, 4)
	c.assembler.CompileRegisterToRegister(amd64.ADDQ, amd64ReservedRegisterForMemory, reg)
	c.assembler.CompileConstToRegister(amd64.ADDQ, -4, reg)

	// Push address and count back to read in Go
	c.pushRuntimeValueLocationOnRegister(reg, runtimeValueTypeI64)
	c.pushRuntimeValueLocationOnRegister(count.register, runtimeValueTypeI32)
	if err := c.compileCallBuiltinFunction(builtinFunctionMemoryNotify); err != nil {
		return err
	}

	// Address and count consumed by Go
	c.locationStack.pop()
	c.locationStack.pop()

	// Then, the result was pushed.
	v := c.locationStack.pushRuntimeValueLocationOnStack()
	v.valueType = runtimeValueTypeI32

	// After return, we re-initialize reserved registers just like preamble of functions.
	c.compileReservedStackBasePointerInitialization()
	c.compileReservedMemoryPointerInitialization()
	return nil
}

func (c *amd64Compiler) compileAtomicFence(_ *wazeroir.UnionOperation) error {
	c.assembler.CompileStandAlone(amd64.MFENCE)
	return nil
}

func (c *amd64Compiler) compileMemoryAlignmentCheck(baseRegister asm.Register, targetSizeInBytes int64) {
	if targetSizeInBytes == 1 {
		return // No alignment restrictions when accessing a byte
	}
	var checkBits asm.ConstantValue
	switch targetSizeInBytes {
	case 2:
		checkBits = 0b1
	case 4:
		checkBits = 0b11
	case 8:
		checkBits = 0b111
	}
	c.assembler.CompileConstToRegister(amd64.TESTQ, checkBits, baseRegister)
	aligned := c.assembler.CompileJump(amd64.JEQ)

	c.compileExitFromNativeCode(nativeCallStatusUnalignedAtomic)
	c.assembler.SetJumpTargetOnNext(aligned)
}
