package compiler

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// compileMemoryAccessBaseSetup pops the top value from the stack (called "base"), stores "memoryBufferStart + base + offsetArg"
// into a register, and returns the stored register. We call the result "base" because it refers to "base addressing" as
// per arm docs, which are reads from addresses without offsets. The result is equivalent to &memory.Buffer[offset].
//
// Note: this also emits the instructions to check the out of bounds memory access.
// In other words, if the offset+targetSizeInBytes exceeds the memory size, the code exits with nativeCallStatusCodeMemoryOutOfBounds status.
func (c *arm64Compiler) compileMemoryAccessBaseSetup(offsetArg uint32, targetSizeInBytes int64) (baseRegister asm.Register, err error) {
	offsetReg, err := c.compileMemoryAccessOffsetSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return
	}

	c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offsetReg)
	baseRegister = offsetReg
	return
}

func (c *arm64Compiler) compileMemoryAlignmentCheck(baseRegister asm.Register, targetSizeInBytes int64) {
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
	c.assembler.CompileRegisterAndConstToRegister(arm64.ANDS, baseRegister, checkBits, arm64.RegRZR)
	c.compileMaybeExitFromNativeCode(arm64.BCONDEQ, nativeCallStatusUnalignedAtomic)
}

func (c *arm64Compiler) compileAtomicLoad(o *wazeroir.UnionOperation) error {
	var (
		loadInst          asm.Instruction
		targetSizeInBytes int64
		vt                runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		loadInst = arm64.LDARW
		targetSizeInBytes = 32 / 8
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		loadInst = arm64.LDARD
		targetSizeInBytes = 64 / 8
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicLoadImpl(offset, loadInst, targetSizeInBytes, vt)
}

// compileAtomicLoad8 implements compiler.compileAtomicLoad8 for the arm64 architecture.
func (c *arm64Compiler) compileAtomicLoad8(o *wazeroir.UnionOperation) error {
	var vt runtimeValueType

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicLoadImpl(offset, arm64.LDARB, 1, vt)
}

// compileAtomicLoad16 implements compiler.compileAtomicLoad16 for the arm64 architecture.
func (c *arm64Compiler) compileAtomicLoad16(o *wazeroir.UnionOperation) error {
	var vt runtimeValueType

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicLoadImpl(offset, arm64.LDARH, 16/8, vt)
}

func (c *arm64Compiler) compileAtomicLoadImpl(offsetArg uint32, loadInst asm.Instruction,
	targetSizeInBytes int64, resultRuntimeValueType runtimeValueType,
) error {
	baseReg, err := c.compileMemoryAccessBaseSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.compileMemoryAlignmentCheck(baseReg, targetSizeInBytes)

	resultRegister := baseReg
	c.assembler.CompileMemoryWithRegisterSourceToRegister(loadInst, baseReg, resultRegister)

	c.pushRuntimeValueLocationOnRegister(resultRegister, resultRuntimeValueType)
	return nil
}

func (c *arm64Compiler) compileAtomicStore(o *wazeroir.UnionOperation) error {
	var (
		storeInst         asm.Instruction
		targetSizeInBytes int64
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		storeInst = arm64.STLRW
		targetSizeInBytes = 32 / 8
	case wazeroir.UnsignedTypeI64:
		storeInst = arm64.STLRD
		targetSizeInBytes = 64 / 8
	}
	return c.compileAtomicStoreImpl(offset, storeInst, targetSizeInBytes)
}

// compileAtomicStore8 implements compiler.compileAtomiStore8 for the arm64 architecture.
func (c *arm64Compiler) compileAtomicStore8(o *wazeroir.UnionOperation) error {
	offset := uint32(o.U2)
	return c.compileAtomicStoreImpl(offset, arm64.STLRB, 1)
}

// compileAtomicStore16 implements compiler.compileAtomicStore16 for the arm64 architecture.
func (c *arm64Compiler) compileAtomicStore16(o *wazeroir.UnionOperation) error {
	offset := uint32(o.U2)
	return c.compileAtomicStoreImpl(offset, arm64.STLRH, 16/8)
}

func (c *arm64Compiler) compileAtomicStoreImpl(offsetArg uint32, storeInst asm.Instruction, targetSizeInBytes int64) error {
	val, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	// Mark temporarily used as compileMemoryAccessOffsetSetup might try allocating register.
	c.markRegisterUsed(val.register)

	baseReg, err := c.compileMemoryAccessBaseSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.compileMemoryAlignmentCheck(baseReg, targetSizeInBytes)

	c.assembler.CompileRegisterToMemoryWithRegisterDest(
		storeInst,
		val.register,
		baseReg,
	)

	c.markRegisterUnused(val.register)
	return nil
}

func (c *arm64Compiler) compileAtomicRMW(o *wazeroir.UnionOperation) error {
	var (
		inst              asm.Instruction
		targetSizeInBytes int64
		vt                runtimeValueType
		negateArg         bool
		flipArg           bool
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
			inst = arm64.LDADDALW
		case wazeroir.AtomicArithmeticOpSub:
			inst = arm64.LDADDALW
			negateArg = true
		case wazeroir.AtomicArithmeticOpAnd:
			inst = arm64.LDCLRALW
			flipArg = true
		case wazeroir.AtomicArithmeticOpOr:
			inst = arm64.LDSETALW
		case wazeroir.AtomicArithmeticOpXor:
			inst = arm64.LDEORALW
		case wazeroir.AtomicArithmeticOpNop:
			inst = arm64.SWPALW
		}
	case wazeroir.UnsignedTypeI64:
		targetSizeInBytes = 64 / 8
		vt = runtimeValueTypeI64
		switch op {
		case wazeroir.AtomicArithmeticOpAdd:
			inst = arm64.LDADDALD
		case wazeroir.AtomicArithmeticOpSub:
			inst = arm64.LDADDALD
			negateArg = true
		case wazeroir.AtomicArithmeticOpAnd:
			inst = arm64.LDCLRALD
			flipArg = true
		case wazeroir.AtomicArithmeticOpOr:
			inst = arm64.LDSETALD
		case wazeroir.AtomicArithmeticOpXor:
			inst = arm64.LDEORALD
		case wazeroir.AtomicArithmeticOpNop:
			inst = arm64.SWPALD
		}
	}
	return c.compileAtomicRMWImpl(inst, offset, negateArg, flipArg, targetSizeInBytes, vt)
}

func (c *arm64Compiler) compileAtomicRMW8(o *wazeroir.UnionOperation) error {
	var (
		inst      asm.Instruction
		vt        runtimeValueType
		negateArg bool
		flipArg   bool
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	op := wazeroir.AtomicArithmeticOp(o.B2)
	offset := uint32(o.U2)

	switch op {
	case wazeroir.AtomicArithmeticOpAdd:
		inst = arm64.LDADDALB
	case wazeroir.AtomicArithmeticOpSub:
		inst = arm64.LDADDALB
		negateArg = true
	case wazeroir.AtomicArithmeticOpAnd:
		inst = arm64.LDCLRALB
		flipArg = true
	case wazeroir.AtomicArithmeticOpOr:
		inst = arm64.LDSETALB
	case wazeroir.AtomicArithmeticOpXor:
		inst = arm64.LDEORALB
	case wazeroir.AtomicArithmeticOpNop:
		inst = arm64.SWPALB
	}

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWImpl(inst, offset, negateArg, flipArg, 1, vt)
}

func (c *arm64Compiler) compileAtomicRMW16(o *wazeroir.UnionOperation) error {
	var (
		inst      asm.Instruction
		vt        runtimeValueType
		negateArg bool
		flipArg   bool
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	op := wazeroir.AtomicArithmeticOp(o.B2)
	offset := uint32(o.U2)

	switch op {
	case wazeroir.AtomicArithmeticOpAdd:
		inst = arm64.LDADDALH
	case wazeroir.AtomicArithmeticOpSub:
		inst = arm64.LDADDALH
		negateArg = true
	case wazeroir.AtomicArithmeticOpAnd:
		inst = arm64.LDCLRALH
		flipArg = true
	case wazeroir.AtomicArithmeticOpOr:
		inst = arm64.LDSETALH
	case wazeroir.AtomicArithmeticOpXor:
		inst = arm64.LDEORALH
	case wazeroir.AtomicArithmeticOpNop:
		inst = arm64.SWPALH
	}

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWImpl(inst, offset, negateArg, flipArg, 16/8, vt)
}

func (c *arm64Compiler) compileAtomicRMWImpl(inst asm.Instruction, offsetArg uint32, negateArg bool, flipArg bool,
	targetSizeInBytes int64, resultRuntimeValueType runtimeValueType,
) error {
	val, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	// Mark temporarily used as compileMemoryAccessOffsetSetup might try allocating register.
	c.markRegisterUsed(val.register)

	if negateArg {
		switch resultRuntimeValueType {
		case runtimeValueTypeI32:
			c.assembler.CompileRegisterToRegister(arm64.NEGW, val.register, val.register)
		case runtimeValueTypeI64:
			c.assembler.CompileRegisterToRegister(arm64.NEG, val.register, val.register)
		}
	}

	if flipArg {
		switch resultRuntimeValueType {
		case runtimeValueTypeI32:
			c.assembler.CompileTwoRegistersToRegister(arm64.ORNW, val.register, arm64.RegRZR, val.register)
		case runtimeValueTypeI64:
			c.assembler.CompileTwoRegistersToRegister(arm64.ORN, val.register, arm64.RegRZR, val.register)
		}
	}

	addrReg, err := c.compileMemoryAccessBaseSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.compileMemoryAlignmentCheck(addrReg, targetSizeInBytes)

	resultRegister := addrReg
	c.assembler.CompileTwoRegistersToRegister(inst, val.register, addrReg, resultRegister)

	c.markRegisterUnused(val.register)

	c.pushRuntimeValueLocationOnRegister(resultRegister, resultRuntimeValueType)
	return nil
}

func (c *arm64Compiler) compileAtomicRMWCmpxchg(o *wazeroir.UnionOperation) error {
	var (
		casInst           asm.Instruction
		targetSizeInBytes int64
		vt                runtimeValueType
	)

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		casInst = arm64.CASALW
		targetSizeInBytes = 32 / 8
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		casInst = arm64.CASALD
		targetSizeInBytes = 64 / 8
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWCmpxchgImpl(casInst, offset, targetSizeInBytes, vt)
}

func (c *arm64Compiler) compileAtomicRMW8Cmpxchg(o *wazeroir.UnionOperation) error {
	var vt runtimeValueType

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWCmpxchgImpl(arm64.CASALB, offset, 1, vt)
}

func (c *arm64Compiler) compileAtomicRMW16Cmpxchg(o *wazeroir.UnionOperation) error {
	var vt runtimeValueType

	unsignedType := wazeroir.UnsignedType(o.B1)
	offset := uint32(o.U2)

	switch unsignedType {
	case wazeroir.UnsignedTypeI32:
		vt = runtimeValueTypeI32
	case wazeroir.UnsignedTypeI64:
		vt = runtimeValueTypeI64
	}
	return c.compileAtomicRMWCmpxchgImpl(arm64.CASALH, offset, 16/8, vt)
}

func (c *arm64Compiler) compileAtomicRMWCmpxchgImpl(inst asm.Instruction, offsetArg uint32, targetSizeInBytes int64, resultRuntimeValueType runtimeValueType) error {
	repl, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	c.markRegisterUsed(repl.register)
	// CAS instruction loads the old value into the register with the comparison value.
	exp, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	if isZeroRegister(exp.register) {
		// exp is also used to load, so if it's set to the zero register we need to move to a
		// loadable register.
		reg, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.MOVD, arm64.RegRZR, reg)
		exp.register = reg
	}
	// Mark temporarily used as compileMemoryAccessOffsetSetup might try allocating register.
	c.markRegisterUsed(exp.register)

	addrReg, err := c.compileMemoryAccessBaseSetup(offsetArg, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.compileMemoryAlignmentCheck(addrReg, targetSizeInBytes)

	c.assembler.CompileTwoRegistersToRegister(inst, exp.register, addrReg, repl.register)

	c.markRegisterUnused(repl.register)
	c.pushRuntimeValueLocationOnRegister(exp.register, resultRuntimeValueType)
	return nil
}

func (c *arm64Compiler) compileAtomicMemoryWait(o *wazeroir.UnionOperation) error {
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

	timeout, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	c.markRegisterUsed(timeout.register)
	exp, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	c.markRegisterUsed(exp.register)

	baseReg, err := c.compileMemoryAccessBaseSetup(offset, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.markRegisterUsed(baseReg)
	c.compileMemoryAlignmentCheck(baseReg, targetSizeInBytes)

	// Push address, values, and timeout back to read in Go
	c.pushRuntimeValueLocationOnRegister(baseReg, runtimeValueTypeI64)
	c.pushRuntimeValueLocationOnRegister(exp.register, vt)
	c.pushRuntimeValueLocationOnRegister(timeout.register, runtimeValueTypeI64)
	if err := c.compileCallGoFunction(nativeCallStatusCodeCallBuiltInFunction, waitFunc); err != nil {
		return err
	}
	// Address, values and timeout consumed in Go
	c.locationStack.pop()
	c.locationStack.pop()
	c.locationStack.pop()

	// Then, the result was pushed.
	v := c.locationStack.pushRuntimeValueLocationOnStack()
	v.valueType = runtimeValueTypeI32

	c.markRegisterUnused(baseReg)
	c.markRegisterUnused(exp.register)
	c.markRegisterUnused(timeout.register)

	// After return, we re-initialize reserved registers just like preamble of functions.
	c.compileReservedStackBasePointerRegisterInitialization()
	c.compileReservedMemoryRegisterInitialization()

	return nil
}

func (c *arm64Compiler) compileAtomicMemoryNotify(o *wazeroir.UnionOperation) error {
	offset := uint32(o.U2)

	if err := c.maybeCompileMoveTopConditionalToGeneralPurposeRegister(); err != nil {
		return err
	}

	count, err := c.popValueOnRegister()
	if err != nil {
		return err
	}
	c.markRegisterUsed(count.register)

	baseReg, err := c.compileMemoryAccessBaseSetup(offset, 4)
	if err != nil {
		return err
	}
	c.compileMemoryAlignmentCheck(baseReg, 4)

	// Push address and count back to read in Go
	c.pushRuntimeValueLocationOnRegister(baseReg, runtimeValueTypeI64)
	c.pushRuntimeValueLocationOnRegister(count.register, runtimeValueTypeI32)
	if err := c.compileCallGoFunction(nativeCallStatusCodeCallBuiltInFunction, builtinFunctionMemoryNotify); err != nil {
		return err
	}

	// Address and count consumed by Go
	c.locationStack.pop()
	c.locationStack.pop()

	// Then, the result was pushed.
	v := c.locationStack.pushRuntimeValueLocationOnStack()
	v.valueType = runtimeValueTypeI32

	c.markRegisterUnused(count.register)

	// After return, we re-initialize reserved registers just like preamble of functions.
	c.compileReservedStackBasePointerRegisterInitialization()
	c.compileReservedMemoryRegisterInitialization()
	return nil
}

func (c *arm64Compiler) compileAtomicFence(_ *wazeroir.UnionOperation) error {
	c.assembler.CompileStandAlone(arm64.DMB)
	return nil
}
