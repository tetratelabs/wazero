package compiler

import (
	"fmt"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// compileV128Const implements compiler.compileV128Const for arm64.
func (c *arm64Compiler) compileV128Const(o *wazeroir.OperationV128Const) error {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()

	result, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	// Moves the lower 64-bits as a scalar float.
	var intReg = arm64ReservedRegisterForTemporary
	if o.Lo == 0 {
		intReg = arm64.RegRZR
	} else {
		c.assembler.CompileConstToRegister(arm64.MOVD, int64(o.Lo), arm64ReservedRegisterForTemporary)
	}
	c.assembler.CompileRegisterToRegister(arm64.FMOVD, intReg, result)

	// Then, insert the higher bits with VMOV (translated as "ins" instruction).
	intReg = arm64ReservedRegisterForTemporary
	if o.Hi == 0 {
		intReg = arm64.RegRZR
	} else {
		c.assembler.CompileConstToRegister(arm64.MOVD, int64(o.Hi), arm64ReservedRegisterForTemporary)
	}
	// "ins Vn.D[1], intReg"
	c.assembler.CompileRegisterToVectorRegister(arm64.VMOV, intReg, result, arm64.VectorArrangementD, 1)

	c.pushVectorRuntimeValueLocationOnRegister(result)
	return nil
}

// compileV128Add implements compiler.compileV128Add for arm64.
func (c *arm64Compiler) compileV128Add(o *wazeroir.OperationV128Add) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	var arr arm64.VectorArrangement
	var inst asm.Instruction
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		inst = arm64.VADD
		arr = arm64.VectorArrangement16B
	case wazeroir.ShapeI16x8:
		inst = arm64.VADD
		arr = arm64.VectorArrangement8H
	case wazeroir.ShapeI32x4:
		inst = arm64.VADD
		arr = arm64.VectorArrangement4S
	case wazeroir.ShapeI64x2:
		inst = arm64.VADD
		arr = arm64.VectorArrangement2D
	case wazeroir.ShapeF32x4:
		inst = arm64.VFADDS
		arr = arm64.VectorArrangement4S
	case wazeroir.ShapeF64x2:
		inst = arm64.VFADDD
		arr = arm64.VectorArrangement2D
	}

	c.assembler.CompileVectorRegisterToVectorRegister(inst, x1.register, x2.register, arr,
		arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.pushVectorRuntimeValueLocationOnRegister(x2.register)
	c.markRegisterUnused(x1.register)
	return nil
}

// compileV128Sub implements compiler.compileV128Sub for arm64.
func (c *arm64Compiler) compileV128Sub(o *wazeroir.OperationV128Sub) (err error) {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	var arr arm64.VectorArrangement
	var inst asm.Instruction
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		inst = arm64.VSUB
		arr = arm64.VectorArrangement16B
	case wazeroir.ShapeI16x8:
		inst = arm64.VSUB
		arr = arm64.VectorArrangement8H
	case wazeroir.ShapeI32x4:
		inst = arm64.VSUB
		arr = arm64.VectorArrangement4S
	case wazeroir.ShapeI64x2:
		inst = arm64.VSUB
		arr = arm64.VectorArrangement2D
	case wazeroir.ShapeF32x4:
		inst = arm64.VFSUBS
		arr = arm64.VectorArrangement4S
	case wazeroir.ShapeF64x2:
		inst = arm64.VFSUBD
		arr = arm64.VectorArrangement2D
	}

	c.assembler.CompileVectorRegisterToVectorRegister(inst, x2.register, x1.register, arr,
		arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	c.markRegisterUnused(x2.register)
	return
}

// compileV128Load implements compiler.compileV128Load for arm64.
func (c *arm64Compiler) compileV128Load(o *wazeroir.OperationV128Load) (err error) {
	c.maybeCompileMoveTopConditionalToFreeGeneralPurposeRegister()
	result, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	switch o.Type {
	case wazeroir.LoadV128Type128:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 16)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementQ,
		)
	case wazeroir.LoadV128Type8x8s:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.SSHLL, result, result,
			arm64.VectorArrangement8B, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.LoadV128Type8x8u:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.USHLL, result, result,
			arm64.VectorArrangement8B, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.LoadV128Type16x4s:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.SSHLL, result, result,
			arm64.VectorArrangement4H, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.LoadV128Type16x4u:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.USHLL, result, result,
			arm64.VectorArrangement4H, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.LoadV128Type32x2s:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.SSHLL, result, result,
			arm64.VectorArrangement2S, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.LoadV128Type32x2u:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.USHLL, result, result,
			arm64.VectorArrangement2S, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.LoadV128Type8Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 1)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement16B)
	case wazeroir.LoadV128Type16Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 2)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement8H)
	case wazeroir.LoadV128Type32Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 4)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement4S)
	case wazeroir.LoadV128Type64Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement2D)
	case wazeroir.LoadV128Type32zero:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 16)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementS,
		)
	case wazeroir.LoadV128Type64zero:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 16)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
	}

	c.pushVectorRuntimeValueLocationOnRegister(result)
	return
}

// compileV128LoadLane implements compiler.compileV128LoadLane for arm64.
func (c *arm64Compiler) compileV128LoadLane(o *wazeroir.OperationV128LoadLane) (err error) {
	targetVector := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(targetVector); err != nil {
		return
	}

	targetSizeInBytes := int64(o.LaneSize / 8)
	source, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	var loadInst asm.Instruction
	var arr arm64.VectorArrangement
	switch o.LaneSize {
	case 8:
		arr = arm64.VectorArrangementB
		loadInst = arm64.MOVB
	case 16:
		arr = arm64.VectorArrangementH
		loadInst = arm64.MOVH
	case 32:
		loadInst = arm64.MOVW
		arr = arm64.VectorArrangementS
	case 64:
		loadInst = arm64.MOVD
		arr = arm64.VectorArrangementD
	}

	c.assembler.CompileMemoryWithRegisterOffsetToRegister(loadInst, arm64ReservedRegisterForMemory, source, source)
	c.assembler.CompileRegisterToVectorRegister(arm64.VMOV, source, targetVector.register, arr, arm64.VectorIndex(o.LaneIndex))

	c.pushVectorRuntimeValueLocationOnRegister(targetVector.register)
	c.locationStack.markRegisterUnused(source)
	return
}

// compileV128Store implements compiler.compileV128Store for arm64.
func (c *arm64Compiler) compileV128Store(o *wazeroir.OperationV128Store) (err error) {
	v := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return
	}

	const targetSizeInBytes = 16
	offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.assembler.CompileVectorRegisterToMemoryWithRegisterOffset(arm64.VMOV,
		v.register, arm64ReservedRegisterForMemory, offset, arm64.VectorArrangementQ)

	c.markRegisterUnused(v.register)
	return
}

// compileV128StoreLane implements compiler.compileV128StoreLane for arm64.
func (c *arm64Compiler) compileV128StoreLane(o *wazeroir.OperationV128StoreLane) (err error) {
	var arr arm64.VectorArrangement
	var storeInst asm.Instruction
	switch o.LaneSize {
	case 8:
		storeInst = arm64.MOVB
		arr = arm64.VectorArrangementB
	case 16:
		storeInst = arm64.MOVH
		arr = arm64.VectorArrangementH
	case 32:
		storeInst = arm64.MOVW
		arr = arm64.VectorArrangementS
	case 64:
		storeInst = arm64.MOVD
		arr = arm64.VectorArrangementD
	}

	v := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return
	}

	targetSizeInBytes := int64(o.LaneSize / 8)
	offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, targetSizeInBytes)
	if err != nil {
		return err
	}

	c.assembler.CompileVectorRegisterToRegister(arm64.VMOV, v.register, arm64ReservedRegisterForTemporary, arr,
		arm64.VectorIndex(o.LaneIndex))

	c.assembler.CompileRegisterToMemoryWithRegisterOffset(storeInst,
		arm64ReservedRegisterForTemporary, arm64ReservedRegisterForMemory, offset)

	c.locationStack.markRegisterUnused(v.register)
	return
}

// compileV128ExtractLane implements compiler.compileV128ExtractLane for arm64.
func (c *arm64Compiler) compileV128ExtractLane(o *wazeroir.OperationV128ExtractLane) (err error) {
	v := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return
	}

	switch o.Shape {
	case wazeroir.ShapeI8x16:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		var inst asm.Instruction
		if o.Signed {
			inst = arm64.SMOV
		} else {
			inst = arm64.VMOV
		}
		c.assembler.CompileVectorRegisterToRegister(inst, v.register, result,
			arm64.VectorArrangementB, arm64.VectorIndex(o.LaneIndex))

		c.locationStack.markRegisterUnused(v.register)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
	case wazeroir.ShapeI16x8:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		var inst asm.Instruction
		if o.Signed {
			inst = arm64.SMOV
		} else {
			inst = arm64.VMOV
		}
		c.assembler.CompileVectorRegisterToRegister(inst, v.register, result,
			arm64.VectorArrangementH, arm64.VectorIndex(o.LaneIndex))

		c.locationStack.markRegisterUnused(v.register)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
	case wazeroir.ShapeI32x4:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileVectorRegisterToRegister(arm64.VMOV, v.register, result,
			arm64.VectorArrangementS, arm64.VectorIndex(o.LaneIndex))

		c.locationStack.markRegisterUnused(v.register)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
	case wazeroir.ShapeI64x2:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileVectorRegisterToRegister(arm64.VMOV, v.register, result,
			arm64.VectorArrangementD, arm64.VectorIndex(o.LaneIndex))

		c.locationStack.markRegisterUnused(v.register)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI64)
	case wazeroir.ShapeF32x4:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VMOV, v.register, v.register,
			arm64.VectorArrangementS, arm64.VectorIndex(o.LaneIndex), 0)
		c.pushRuntimeValueLocationOnRegister(v.register, runtimeValueTypeF32)
	case wazeroir.ShapeF64x2:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VMOV, v.register, v.register,
			arm64.VectorArrangementD, arm64.VectorIndex(o.LaneIndex), 0)
		c.pushRuntimeValueLocationOnRegister(v.register, runtimeValueTypeF64)
	}
	return
}

// compileV128ReplaceLane implements compiler.compileV128ReplaceLane for arm64.
func (c *arm64Compiler) compileV128ReplaceLane(o *wazeroir.OperationV128ReplaceLane) (err error) {
	origin := c.locationStack.pop()
	if err = c.compileEnsureOnGeneralPurposeRegister(origin); err != nil {
		return
	}

	vector := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(vector); err != nil {
		return
	}

	switch o.Shape {
	case wazeroir.ShapeI8x16:
		c.assembler.CompileRegisterToVectorRegister(arm64.VMOV, origin.register, vector.register,
			arm64.VectorArrangementB, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeI16x8:
		c.assembler.CompileRegisterToVectorRegister(arm64.VMOV, origin.register, vector.register,
			arm64.VectorArrangementH, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeI32x4:
		c.assembler.CompileRegisterToVectorRegister(arm64.VMOV, origin.register, vector.register,
			arm64.VectorArrangementS, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeI64x2:
		c.assembler.CompileRegisterToVectorRegister(arm64.VMOV, origin.register, vector.register,
			arm64.VectorArrangementD, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeF32x4:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VMOV, origin.register, vector.register,
			arm64.VectorArrangementS, 0, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeF64x2:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VMOV, origin.register, vector.register,
			arm64.VectorArrangementD, 0, arm64.VectorIndex(o.LaneIndex))
	}

	c.locationStack.markRegisterUnused(origin.register)
	c.pushVectorRuntimeValueLocationOnRegister(vector.register)
	return
}

// compileV128Splat implements compiler.compileV128Splat for arm64.
func (c *arm64Compiler) compileV128Splat(o *wazeroir.OperationV128Splat) (err error) {
	origin := c.locationStack.pop()
	if err = c.compileEnsureOnGeneralPurposeRegister(origin); err != nil {
		return
	}

	var result asm.Register
	switch o.Shape {
	case wazeroir.ShapeI8x16:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUP, origin.register, result,
			arm64.VectorArrangementB, arm64.VectorIndexNone)
	case wazeroir.ShapeI16x8:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUP, origin.register, result,
			arm64.VectorArrangementH, arm64.VectorIndexNone)
	case wazeroir.ShapeI32x4:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUP, origin.register, result,
			arm64.VectorArrangementS, arm64.VectorIndexNone)
	case wazeroir.ShapeI64x2:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUP, origin.register, result,
			arm64.VectorArrangementD, arm64.VectorIndexNone)
	case wazeroir.ShapeF32x4:
		result = origin.register
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.DUP, origin.register, result,
			arm64.VectorArrangementS, 0, arm64.VectorIndexNone)
	case wazeroir.ShapeF64x2:
		result = origin.register
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.DUP, origin.register, result,
			arm64.VectorArrangementD, 0, arm64.VectorIndexNone)
	}

	c.locationStack.markRegisterUnused(origin.register)
	c.pushVectorRuntimeValueLocationOnRegister(result)
	return
}

func (c *arm64Compiler) onValueReleaseRegisterToStack(reg asm.Register) {
	for i := uint64(0); i < c.locationStack.sp; i++ {
		prevValue := c.locationStack.stack[i]
		if prevValue.register == reg {
			c.compileReleaseRegisterToStack(prevValue)
			break
		}
	}
}

// compileV128Shuffle implements compiler.compileV128Shuffle for arm64.
func (c *arm64Compiler) compileV128Shuffle(o *wazeroir.OperationV128Shuffle) (err error) {
	// Shuffle needs two operands (v, w) must be next to each other.
	// For simplicity, we use V29 for v and V30 for w values respectively.
	const vReg, wReg = arm64.RegV29, arm64.RegV30

	// Ensures that w value is placed on wReg.
	w := c.locationStack.popV128()
	if w.register != wReg {
		// If wReg is already in use, save the value onto the stack.
		c.onValueReleaseRegisterToStack(wReg)

		if w.onRegister() {
			c.assembler.CompileVectorRegisterToVectorRegister(arm64.VMOV, w.register, wReg,
				arm64.VectorArrangement16B, arm64.VectorIndexNone, arm64.VectorIndexNone)
			// We no longer use the old register.
			c.markRegisterUnused(w.register)
		} else { // on stack
			w.setRegister(wReg)
			c.compileLoadValueOnStackToRegister(w)
		}
	}

	// Ensures that v value is placed on wReg.
	v := c.locationStack.popV128()
	if v.register != vReg {
		// If vReg is already in use, save the value onto the stack.
		c.onValueReleaseRegisterToStack(vReg)

		if v.onRegister() {
			c.assembler.CompileVectorRegisterToVectorRegister(arm64.VMOV, v.register, vReg,
				arm64.VectorArrangement16B, arm64.VectorIndexNone, arm64.VectorIndexNone)
			// We no longer use the old register.
			c.markRegisterUnused(v.register)
		} else { // on stack
			v.setRegister(vReg)
			c.compileLoadValueOnStackToRegister(v)
		}
	}

	c.locationStack.markRegisterUsed(vReg, wReg)
	result, err := c.allocateRegister(registerTypeVector)
	if err != nil {
		return err
	}

	c.assembler.CompileLoadStaticConstToVectorRegister(arm64.VMOV, o.Lanes[:], result, arm64.VectorArrangementQ)
	c.assembler.CompileVectorRegisterToVectorRegister(arm64.TBL2, vReg, result, arm64.VectorArrangement16B,
		arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.locationStack.markRegisterUnused(vReg, wReg)
	c.pushVectorRuntimeValueLocationOnRegister(result)
	return
}

// compileV128Swizzle implements compiler.compileV128Swizzle for arm64.
func (c *arm64Compiler) compileV128Swizzle(o *wazeroir.OperationV128Swizzle) (err error) {
	indexVec := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(indexVec); err != nil {
		return
	}
	baseVec := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(baseVec); err != nil {
		return
	}

	c.assembler.CompileVectorRegisterToVectorRegister(arm64.TBL1, baseVec.register, indexVec.register,
		arm64.VectorArrangement16B, arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.markRegisterUnused(baseVec.register)
	c.pushVectorRuntimeValueLocationOnRegister(indexVec.register)
	return
}

// compileV128AnyTrue implements compiler.compileV128AnyTrue for arm64.
func (c *arm64Compiler) compileV128AnyTrue(*wazeroir.OperationV128AnyTrue) (err error) {
	vector := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(vector); err != nil {
		return
	}

	v := vector.register
	c.assembler.CompileVectorRegisterToVectorRegister(arm64.UMAXP, v, v,
		arm64.VectorArrangement16B, arm64.VectorIndexNone, arm64.VectorIndexNone)
	c.assembler.CompileVectorRegisterToRegister(arm64.VMOV, v, arm64ReservedRegisterForTemporary,
		arm64.VectorArrangementD, 0)
	c.assembler.CompileTwoRegistersToNone(arm64.CMP, arm64.RegRZR, arm64ReservedRegisterForTemporary)
	c.locationStack.pushRuntimeValueLocationOnConditionalRegister(arm64.CondNE)

	c.locationStack.markRegisterUnused(v)
	return
}

// compileV128AllTrue implements compiler.compileV128AllTrue for arm64.
func (c *arm64Compiler) compileV128AllTrue(o *wazeroir.OperationV128AllTrue) (err error) {
	vector := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(vector); err != nil {
		return
	}

	v := vector.register
	if o.Shape == wazeroir.ShapeI64x2 {
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.CMEQ, arm64.RegRZR, v,
			arm64.VectorArrangementNone, arm64.VectorIndexNone, arm64.VectorIndexNone)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.ADDP, v, v,
			arm64.VectorArrangementD, arm64.VectorIndexNone, arm64.VectorIndexNone)
		c.assembler.CompileTwoRegistersToNone(arm64.FCMPD, v, v)
		c.locationStack.pushRuntimeValueLocationOnConditionalRegister(arm64.CondEQ)
	} else {
		var arr arm64.VectorArrangement
		switch o.Shape {
		case wazeroir.ShapeI8x16:
			arr = arm64.VectorArrangement16B
		case wazeroir.ShapeI16x8:
			arr = arm64.VectorArrangement8H
		case wazeroir.ShapeI32x4:
			arr = arm64.VectorArrangement4S
		}

		c.assembler.CompileVectorRegisterToVectorRegister(arm64.UMINV, v, v,
			arr, arm64.VectorIndexNone, arm64.VectorIndexNone)
		c.assembler.CompileVectorRegisterToRegister(arm64.VMOV, v, arm64ReservedRegisterForTemporary,
			arm64.VectorArrangementD, 0)
		c.assembler.CompileTwoRegistersToNone(arm64.CMP, arm64.RegRZR, arm64ReservedRegisterForTemporary)
		c.locationStack.pushRuntimeValueLocationOnConditionalRegister(arm64.CondNE)
	}
	c.markRegisterUnused(v)
	return
}

// compileV128BitMask implements compiler.compileV128BitMask for arm64.
func (c *arm64Compiler) compileV128BitMask(o *wazeroir.OperationV128BitMask) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128And implements compiler.compileV128And for arm64.
func (c *arm64Compiler) compileV128And(o *wazeroir.OperationV128And) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Not implements compiler.compileV128Not for arm64.
func (c *arm64Compiler) compileV128Not(o *wazeroir.OperationV128Not) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Or implements compiler.compileV128Or for arm64.
func (c *arm64Compiler) compileV128Or(o *wazeroir.OperationV128Or) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Xor implements compiler.compileV128Xor for arm64.
func (c *arm64Compiler) compileV128Xor(o *wazeroir.OperationV128Xor) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Bitselect implements compiler.compileV128Bitselect for arm64.
func (c *arm64Compiler) compileV128Bitselect(o *wazeroir.OperationV128Bitselect) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128AndNot implements compiler.compileV128AndNot for arm64.
func (c *arm64Compiler) compileV128AndNot(o *wazeroir.OperationV128AndNot) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Shr implements compiler.compileV128Shr for arm64.
func (c *arm64Compiler) compileV128Shr(o *wazeroir.OperationV128Shr) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Shl implements compiler.compileV128Shl for arm64.
func (c *arm64Compiler) compileV128Shl(o *wazeroir.OperationV128Shl) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Cmp implements compiler.compileV128Cmp for arm64.
func (c *arm64Compiler) compileV128Cmp(o *wazeroir.OperationV128Cmp) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}
