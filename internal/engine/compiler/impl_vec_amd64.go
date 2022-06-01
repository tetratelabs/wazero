package compiler

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// compileV128Const implements compiler.compileV128Const for amd64 architecture.
func (c *amd64Compiler) compileV128Const(o *wazeroir.OperationV128Const) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	result, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	// We cannot directly load the value from memory to float regs,
	// so we move it to int reg temporarily.
	tmpReg, err := c.allocateRegister(registerTypeGeneralPurpose)
	if err != nil {
		return err
	}

	// Move the lower 64-bits.
	if o.Lo == 0 {
		c.assembler.CompileRegisterToRegister(amd64.XORQ, tmpReg, tmpReg)
	} else {
		c.assembler.CompileConstToRegister(amd64.MOVQ, int64(o.Lo), tmpReg)
	}
	c.assembler.CompileRegisterToRegister(amd64.MOVQ, tmpReg, result)

	if o.Lo != 0 && o.Hi == 0 {
		c.assembler.CompileRegisterToRegister(amd64.XORQ, tmpReg, tmpReg)
	} else if o.Hi != 0 {
		c.assembler.CompileConstToRegister(amd64.MOVQ, int64(o.Hi), tmpReg)
	}
	// Move the higher 64-bits with PINSRQ at the second element of 64x2 vector.
	c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRQ, tmpReg, result, 1)

	c.pushVectorRuntimeValueLocationOnRegister(result)
	return nil
}

// compileV128Add implements compiler.compileV128Add for amd64 architecture.
func (c *amd64Compiler) compileV128Add(o *wazeroir.OperationV128Add) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}
	var inst asm.Instruction
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		inst = amd64.PADDB
	case wazeroir.ShapeI16x8:
		inst = amd64.PADDW
	case wazeroir.ShapeI32x4:
		inst = amd64.PADDL
	case wazeroir.ShapeI64x2:
		inst = amd64.PADDQ
	case wazeroir.ShapeF32x4:
		inst = amd64.ADDPS
	case wazeroir.ShapeF64x2:
		inst = amd64.ADDPD
	}
	c.assembler.CompileRegisterToRegister(inst, x2.register, x1.register)

	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	c.locationStack.markRegisterUnused(x2.register)
	return nil
}

// compileV128Sub implements compiler.compileV128Sub for amd64 architecture.
func (c *amd64Compiler) compileV128Sub(o *wazeroir.OperationV128Sub) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}
	var inst asm.Instruction
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		inst = amd64.PSUBB
	case wazeroir.ShapeI16x8:
		inst = amd64.PSUBW
	case wazeroir.ShapeI32x4:
		inst = amd64.PSUBL
	case wazeroir.ShapeI64x2:
		inst = amd64.PSUBQ
	case wazeroir.ShapeF32x4:
		inst = amd64.SUBPS
	case wazeroir.ShapeF64x2:
		inst = amd64.SUBPD
	}
	c.assembler.CompileRegisterToRegister(inst, x2.register, x1.register)

	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	c.locationStack.markRegisterUnused(x2.register)
	return nil
}

// compileV128Load implements compiler.compileV128Load for amd64 architecture.
func (c *amd64Compiler) compileV128Load(o *wazeroir.OperationV128Load) error {
	result, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	switch o.Type {
	case wazeroir.LoadV128Type128:
		err = c.compileV128LoadImpl(amd64.MOVDQU, o.Arg.Offset, 16, result)
	case wazeroir.LoadV128Type8x8s:
		err = c.compileV128LoadImpl(amd64.PMOVSXBW, o.Arg.Offset, 8, result)
	case wazeroir.LoadV128Type8x8u:
		err = c.compileV128LoadImpl(amd64.PMOVZXBW, o.Arg.Offset, 8, result)
	case wazeroir.LoadV128Type16x4s:
		err = c.compileV128LoadImpl(amd64.PMOVSXWD, o.Arg.Offset, 8, result)
	case wazeroir.LoadV128Type16x4u:
		err = c.compileV128LoadImpl(amd64.PMOVZXWD, o.Arg.Offset, 8, result)
	case wazeroir.LoadV128Type32x2s:
		err = c.compileV128LoadImpl(amd64.PMOVSXDQ, o.Arg.Offset, 8, result)
	case wazeroir.LoadV128Type32x2u:
		err = c.compileV128LoadImpl(amd64.PMOVZXDQ, o.Arg.Offset, 8, result)
	case wazeroir.LoadV128Type8Splat:
		reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, 1)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithIndexToRegister(amd64.MOVBQZX, amd64ReservedRegisterForMemory, -1,
			reg, 1, reg)
		// pinsrb   $0, reg, result
		// pxor	    tmpVReg, tmpVReg
		// pshufb   tmpVReg, result
		c.locationStack.markRegisterUsed(result)
		tmpVReg, err := c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRB, reg, result, 0)
		c.assembler.CompileRegisterToRegister(amd64.PXOR, tmpVReg, tmpVReg)
		c.assembler.CompileRegisterToRegister(amd64.PSHUFB, tmpVReg, result)
	case wazeroir.LoadV128Type16Splat:
		reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, 2)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithIndexToRegister(amd64.MOVWQZX, amd64ReservedRegisterForMemory, -2,
			reg, 1, reg)
		// pinsrw $0, reg, result
		// pinsrw $1, reg, result
		// pshufd $0, result, result (result = result[0,0,0,0])
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRW, reg, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRW, reg, result, 1)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, result, result, 0)
	case wazeroir.LoadV128Type32Splat:
		reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, 4)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithIndexToRegister(amd64.MOVLQZX, amd64ReservedRegisterForMemory, -4,
			reg, 1, reg)
		// pinsrd $0, reg, result
		// pshufd $0, result, result (result = result[0,0,0,0])
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRD, reg, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, result, result, 0)
	case wazeroir.LoadV128Type64Splat:
		reg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithIndexToRegister(amd64.MOVQ, amd64ReservedRegisterForMemory, -8,
			reg, 1, reg)
		// pinsrq $0, reg, result
		// pinsrq $1, reg, result
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRQ, reg, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRQ, reg, result, 1)
	case wazeroir.LoadV128Type32zero:
		err = c.compileV128LoadImpl(amd64.MOVL, o.Arg.Offset, 4, result)
	case wazeroir.LoadV128Type64zero:
		err = c.compileV128LoadImpl(amd64.MOVQ, o.Arg.Offset, 8, result)
	}

	if err != nil {
		return err
	}

	c.pushVectorRuntimeValueLocationOnRegister(result)
	return nil
}

func (c *amd64Compiler) compileV128LoadImpl(inst asm.Instruction, offset uint32, targetSizeInBytes int64, dst asm.Register) error {
	offsetReg, err := c.compileMemoryAccessCeilSetup(offset, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.assembler.CompileMemoryWithIndexToRegister(inst, amd64ReservedRegisterForMemory, -targetSizeInBytes,
		offsetReg, 1, dst)
	return nil
}

// compileV128LoadLane implements compiler.compileV128LoadLane for amd64.
func (c *amd64Compiler) compileV128LoadLane(o *wazeroir.OperationV128LoadLane) error {
	targetVector := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(targetVector); err != nil {
		return err
	}

	var insertInst asm.Instruction
	switch o.LaneSize {
	case 8:
		insertInst = amd64.PINSRB
	case 16:
		insertInst = amd64.PINSRW
	case 32:
		insertInst = amd64.PINSRD
	case 64:
		insertInst = amd64.PINSRQ
	}

	targetSizeInBytes := int64(o.LaneSize / 8)
	offsetReg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}
	c.assembler.CompileMemoryWithIndexAndArgToRegister(insertInst, amd64ReservedRegisterForMemory, -targetSizeInBytes,
		offsetReg, 1, targetVector.register, o.LaneIndex)

	c.pushVectorRuntimeValueLocationOnRegister(targetVector.register)
	return nil
}

// compileV128Store implements compiler.compileV128Store for amd64.
func (c *amd64Compiler) compileV128Store(o *wazeroir.OperationV128Store) error {
	val := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	const targetSizeInBytes = 16
	offsetReg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.assembler.CompileRegisterToMemoryWithIndex(amd64.MOVDQU, val.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, offsetReg, 1)

	c.locationStack.markRegisterUnused(val.register, offsetReg)
	return nil
}

// compileV128StoreLane implements compiler.compileV128StoreLane for amd64.
func (c *amd64Compiler) compileV128StoreLane(o *wazeroir.OperationV128StoreLane) error {
	var storeInst asm.Instruction
	switch o.LaneSize {
	case 8:
		storeInst = amd64.PEXTRB
	case 16:
		storeInst = amd64.PEXTRW
	case 32:
		storeInst = amd64.PEXTRD
	case 64:
		storeInst = amd64.PEXTRQ
	}

	val := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(val); err != nil {
		return err
	}

	targetSizeInBytes := int64(o.LaneSize / 8)
	offsetReg, err := c.compileMemoryAccessCeilSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.assembler.CompileRegisterToMemoryWithIndexAndArg(storeInst, val.register,
		amd64ReservedRegisterForMemory, -targetSizeInBytes, offsetReg, 1, o.LaneIndex)

	c.locationStack.markRegisterUnused(val.register, offsetReg)
	return nil
}

// compileV128ExtractLane implements compiler.compileV128ExtractLane for amd64.
func (c *amd64Compiler) compileV128ExtractLane(o *wazeroir.OperationV128ExtractLane) error {
	val := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(val); err != nil {
		return err
	}
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PEXTRB, val.register, result, o.LaneIndex)
		if o.Signed {
			c.assembler.CompileRegisterToRegister(amd64.MOVBQSX, result, result)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.MOVBLZX, result, result)
		}
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
		c.locationStack.markRegisterUnused(val.register)
	case wazeroir.ShapeI16x8:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PEXTRW, val.register, result, o.LaneIndex)
		if o.Signed {
			c.assembler.CompileRegisterToRegister(amd64.MOVWLSX, result, result)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.MOVWLZX, result, result)
		}
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
		c.locationStack.markRegisterUnused(val.register)
	case wazeroir.ShapeI32x4:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PEXTRD, val.register, result, o.LaneIndex)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
		c.locationStack.markRegisterUnused(val.register)
	case wazeroir.ShapeI64x2:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PEXTRQ, val.register, result, o.LaneIndex)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI64)
		c.locationStack.markRegisterUnused(val.register)
	case wazeroir.ShapeF32x4:
		if o.LaneIndex != 0 {
			c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, val.register, val.register, o.LaneIndex)
		}
		c.pushRuntimeValueLocationOnRegister(val.register, runtimeValueTypeF32)
	case wazeroir.ShapeF64x2:
		if o.LaneIndex != 0 {
			// This case we can assume LaneIndex == 1.
			// We have to modify the val.register as, for example:
			//    0b11 0b10 0b01 0b00
			//     |    |    |    |
			//   [x3,  x2,  x1,  x0] -> [x0,  x0,  x3,  x2]
			// where val.register = [x3, x2, x1, x0] and each xN = 32bits.
			// Then, we interpret the register as float64, therefore, the float64 value is obtained as [x3, x2].
			arg := byte(0b00_00_11_10)
			c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, val.register, val.register, arg)
		}
		c.pushRuntimeValueLocationOnRegister(val.register, runtimeValueTypeF64)
	}

	return nil
}

// compileV128ReplaceLane implements compiler.compileV128ReplaceLane for amd64.
func (c *amd64Compiler) compileV128ReplaceLane(o *wazeroir.OperationV128ReplaceLane) error {
	origin := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(origin); err != nil {
		return err
	}

	vector := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(vector); err != nil {
		return err
	}

	switch o.Shape {
	case wazeroir.ShapeI8x16:
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRB, origin.register, vector.register, o.LaneIndex)
	case wazeroir.ShapeI16x8:
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRW, origin.register, vector.register, o.LaneIndex)
	case wazeroir.ShapeI32x4:
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRD, origin.register, vector.register, o.LaneIndex)
	case wazeroir.ShapeI64x2:
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRQ, origin.register, vector.register, o.LaneIndex)
	case wazeroir.ShapeF32x4:
		c.assembler.CompileRegisterToRegisterWithArg(amd64.INSERTPS, origin.register, vector.register,
			// In INSERTPS instruction, the destination index is encoded at 4 and 5 bits of the argument.
			// See https://www.felixcloutier.com/x86/insertps
			o.LaneIndex<<4,
		)
	case wazeroir.ShapeF64x2:
		if o.LaneIndex == 0 {
			c.assembler.CompileRegisterToRegister(amd64.MOVSD, origin.register, vector.register)
		} else {
			c.assembler.CompileRegisterToRegister(amd64.MOVLHPS, origin.register, vector.register)
		}
	}

	c.pushVectorRuntimeValueLocationOnRegister(vector.register)
	c.locationStack.markRegisterUnused(origin.register)
	return nil
}

// compileV128Splat implements compiler.compileV128Splat for amd64.
func (c *amd64Compiler) compileV128Splat(o *wazeroir.OperationV128Splat) (err error) {
	origin := c.locationStack.pop()
	if err = c.compileEnsureOnGeneralPurposeRegister(origin); err != nil {
		return
	}

	var result asm.Register
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		c.locationStack.markRegisterUsed(result)

		tmp, err := c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRB, origin.register, result, 0)
		c.assembler.CompileRegisterToRegister(amd64.PXOR, tmp, tmp)
		c.assembler.CompileRegisterToRegister(amd64.PSHUFB, tmp, result)
	case wazeroir.ShapeI16x8:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		c.locationStack.markRegisterUsed(result)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRW, origin.register, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRW, origin.register, result, 1)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, result, result, 0)
	case wazeroir.ShapeI32x4:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		c.locationStack.markRegisterUsed(result)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRD, origin.register, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, result, result, 0)
	case wazeroir.ShapeI64x2:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		c.locationStack.markRegisterUsed(result)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRQ, origin.register, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PINSRQ, origin.register, result, 1)
	case wazeroir.ShapeF32x4:
		result = origin.register
		c.assembler.CompileRegisterToRegisterWithArg(amd64.INSERTPS, origin.register, result, 0)
		c.assembler.CompileRegisterToRegisterWithArg(amd64.PSHUFD, result, result, 0)
	case wazeroir.ShapeF64x2:
		result = origin.register
		c.assembler.CompileRegisterToRegister(amd64.MOVQ, origin.register, result)
		c.assembler.CompileRegisterToRegister(amd64.MOVLHPS, origin.register, result)
	}

	c.locationStack.markRegisterUnused(origin.register)
	c.pushVectorRuntimeValueLocationOnRegister(result)
	return nil
}

// compileV128Shuffle implements compiler.compileV128Shuffle for amd64.
func (c *amd64Compiler) compileV128Shuffle(o *wazeroir.OperationV128Shuffle) error {
	w := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(w); err != nil {
		return err
	}

	v := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	tmp, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	consts := [32]byte{}
	for i, lane := range o.Lanes {
		if lane < 16 {
			consts[i+16] = 0x80
			consts[i] = lane
		} else {
			consts[i+16] = lane - 16
			consts[i] = 0x80
		}
	}

	err = c.assembler.CompileLoadStaticConstToRegister(amd64.MOVDQU, consts[:16], tmp)
	if err != nil {
		return err
	}
	c.assembler.CompileRegisterToRegister(amd64.PSHUFB, tmp, v.register)
	err = c.assembler.CompileLoadStaticConstToRegister(amd64.MOVDQU, consts[16:], tmp)
	if err != nil {
		return err
	}
	c.assembler.CompileRegisterToRegister(amd64.PSHUFB, tmp, w.register)
	c.assembler.CompileRegisterToRegister(amd64.ORPS, v.register, w.register)

	c.pushVectorRuntimeValueLocationOnRegister(w.register)
	c.locationStack.markRegisterUnused(v.register)
	return nil
}

var swizzleConst = [16]byte{
	0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70,
	0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70, 0x70,
}

// compileV128Swizzle implements compiler.compileV128Swizzle for amd64.
func (c *amd64Compiler) compileV128Swizzle(*wazeroir.OperationV128Swizzle) error {
	indexVec := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(indexVec); err != nil {
		return err
	}

	baseVec := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(baseVec); err != nil {
		return err
	}

	tmp, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	err = c.assembler.CompileLoadStaticConstToRegister(amd64.MOVDQU, swizzleConst[:], tmp)
	if err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.PADDUSB, tmp, indexVec.register)
	c.assembler.CompileRegisterToRegister(amd64.PSHUFB, indexVec.register, baseVec.register)

	c.pushVectorRuntimeValueLocationOnRegister(baseVec.register)
	c.locationStack.markRegisterUnused(indexVec.register)
	return nil
}

// compileV128AnyTrue implements compiler.compileV128AnyTrue for amd64.
func (c *amd64Compiler) compileV128AnyTrue(*wazeroir.OperationV128AnyTrue) error {
	v := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.PTEST, v.register, v.register)

	c.locationStack.pushRuntimeValueLocationOnConditionalRegister(amd64.ConditionalRegisterStateNE)
	c.locationStack.markRegisterUnused(v.register)
	return nil
}

// compileV128AllTrue implements compiler.compileV128AllTrue for amd64.
func (c *amd64Compiler) compileV128AllTrue(o *wazeroir.OperationV128AllTrue) error {
	v := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	tmp, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	var cmpInst asm.Instruction
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		cmpInst = amd64.PCMPEQB
	case wazeroir.ShapeI16x8:
		cmpInst = amd64.PCMPEQW
	case wazeroir.ShapeI32x4:
		cmpInst = amd64.PCMPEQD
	case wazeroir.ShapeI64x2:
		cmpInst = amd64.PCMPEQQ
	}

	c.assembler.CompileRegisterToRegister(amd64.PXOR, tmp, tmp)
	c.assembler.CompileRegisterToRegister(cmpInst, v.register, tmp)
	c.assembler.CompileRegisterToRegister(amd64.PTEST, tmp, tmp)
	c.locationStack.markRegisterUnused(v.register, tmp)
	c.locationStack.pushRuntimeValueLocationOnConditionalRegister(amd64.ConditionalRegisterStateE)
	return nil
}

// compileV128BitMask implements compiler.compileV128BitMask for amd64.
func (c *amd64Compiler) compileV128BitMask(o *wazeroir.OperationV128BitMask) error {
	v := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	result, err := c.allocateRegister(registerTypeGeneralPurpose)
	if err != nil {
		return err
	}

	switch o.Shape {
	case wazeroir.ShapeI8x16:
		c.assembler.CompileRegisterToRegister(amd64.PMOVMSKB, v.register, result)
	case wazeroir.ShapeI16x8:
		// When we have:
		// 	R1 = [R1(w1), R1(w2), R1(w3), R1(w4), R1(w5), R1(w6), R1(w7), R1(v8)]
		// 	R2 = [R2(w1), R2(w2), R2(w3), R2(v4), R2(w5), R2(w6), R2(w7), R2(v8)]
		//	where RX(wn) is n-th signed word (16-bit) of RX register,
		//
		// "PACKSSWB R1, R2" produces
		//  R1 = [
		// 		byte_sat(R1(w1)), byte_sat(R1(w2)), byte_sat(R1(w3)), byte_sat(R1(w4)),
		// 		byte_sat(R1(w5)), byte_sat(R1(w6)), byte_sat(R1(w7)), byte_sat(R1(w8)),
		// 		byte_sat(R2(w1)), byte_sat(R2(w2)), byte_sat(R2(w3)), byte_sat(R2(w4)),
		// 		byte_sat(R2(w5)), byte_sat(R2(w6)), byte_sat(R2(w7)), byte_sat(R2(w8)),
		//  ]
		//  where R1 is the destination register, and
		// 	byte_sat(w) = w if w fits as signed 8-bit,
		//                0x80 if w is less than 0x80
		//                0x7F if w is greater than 0x7f
		//
		// See https://www.felixcloutier.com/x86/packsswb:packssdw for detail.
		//
		// Therefore, v.register ends up having i-th and (i+8)-th bit set if i-th lane is negative (for i in 0..8).
		c.assembler.CompileRegisterToRegister(amd64.PACKSSWB, v.register, v.register)
		c.assembler.CompileRegisterToRegister(amd64.PMOVMSKB, v.register, result)
		// Clear the higher bits than 8.
		c.assembler.CompileConstToRegister(amd64.SHRQ, 8, result)
	case wazeroir.ShapeI32x4:
		c.assembler.CompileRegisterToRegister(amd64.MOVMSKPS, v.register, result)
	case wazeroir.ShapeI64x2:
		c.assembler.CompileRegisterToRegister(amd64.MOVMSKPD, v.register, result)
	}

	c.locationStack.markRegisterUnused(v.register)
	c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
	return nil
}

// compileV128And implements compiler.compileV128And for amd64.
func (c *amd64Compiler) compileV128And(*wazeroir.OperationV128And) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.PAND, x2.register, x1.register)

	c.locationStack.markRegisterUnused(x2.register)
	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	return nil
}

// compileV128Not implements compiler.compileV128Not for amd64.
func (c *amd64Compiler) compileV128Not(*wazeroir.OperationV128Not) error {
	v := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	tmp, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	// Set all bits on tmp register.
	c.assembler.CompileRegisterToRegister(amd64.PCMPEQD, tmp, tmp)
	// Then XOR with tmp to reverse all bits on v.register.
	c.assembler.CompileRegisterToRegister(amd64.PXOR, tmp, v.register)
	c.pushVectorRuntimeValueLocationOnRegister(v.register)
	return nil
}

// compileV128Or implements compiler.compileV128Or for amd64.
func (c *amd64Compiler) compileV128Or(*wazeroir.OperationV128Or) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.POR, x2.register, x1.register)

	c.locationStack.markRegisterUnused(x2.register)
	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	return nil
}

// compileV128Xor implements compiler.compileV128Xor for amd64.
func (c *amd64Compiler) compileV128Xor(*wazeroir.OperationV128Xor) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.PXOR, x2.register, x1.register)

	c.locationStack.markRegisterUnused(x2.register)
	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	return nil
}

// compileV128Bitselect implements compiler.compileV128Bitselect for amd64.
func (c *amd64Compiler) compileV128Bitselect(*wazeroir.OperationV128Bitselect) error {
	selector := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(selector); err != nil {
		return err
	}

	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	// The following logic is equivalent to v128.or(v128.and(v1, selector), v128.and(v2, v128.not(selector)))
	// See https://github.com/WebAssembly/spec/blob/main/proposals/simd/SIMD.md#bitwise-select
	c.assembler.CompileRegisterToRegister(amd64.PAND, selector.register, x1.register)
	c.assembler.CompileRegisterToRegister(amd64.PANDN, x2.register, selector.register)
	c.assembler.CompileRegisterToRegister(amd64.POR, selector.register, x1.register)

	c.locationStack.markRegisterUnused(x2.register, selector.register)
	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	return nil
}

// compileV128AndNot implements compiler.compileV128AndNot for amd64.
func (c *amd64Compiler) compileV128AndNot(*wazeroir.OperationV128AndNot) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	c.assembler.CompileRegisterToRegister(amd64.PANDN, x1.register, x2.register)

	c.locationStack.markRegisterUnused(x1.register)
	c.pushVectorRuntimeValueLocationOnRegister(x2.register)
	return nil
}
