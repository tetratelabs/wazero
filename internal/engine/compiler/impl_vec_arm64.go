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
	c.assembler.CompileRegisterToVectorRegister(arm64.INSGEN, intReg, result, arm64.VectorArrangementD, 1)

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
	case wazeroir.V128LoadType128:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 16)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementQ,
		)
	case wazeroir.V128LoadType8x8s:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.SSHLLIMM, result, result,
			arm64.VectorArrangement8B, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.V128LoadType8x8u:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.USHLLIMM, result, result,
			arm64.VectorArrangement8B, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.V128LoadType16x4s:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.SSHLLIMM, result, result,
			arm64.VectorArrangement4H, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.V128LoadType16x4u:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.USHLLIMM, result, result,
			arm64.VectorArrangement4H, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.V128LoadType32x2s:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.SSHLLIMM, result, result,
			arm64.VectorArrangement2S, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.V128LoadType32x2u:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementD,
		)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.USHLLIMM, result, result,
			arm64.VectorArrangement2S, arm64.VectorIndexNone, arm64.VectorIndexNone)
	case wazeroir.V128LoadType8Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 1)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement16B)
	case wazeroir.V128LoadType16Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 2)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement8H)
	case wazeroir.V128LoadType32Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 4)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement4S)
	case wazeroir.V128LoadType64Splat:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 8)
		if err != nil {
			return err
		}
		c.assembler.CompileRegisterToRegister(arm64.ADD, arm64ReservedRegisterForMemory, offset)
		c.assembler.CompileMemoryToVectorRegister(arm64.LD1R, offset, 0, result, arm64.VectorArrangement2D)
	case wazeroir.V128LoadType32zero:
		offset, err := c.compileMemoryAccessOffsetSetup(o.Arg.Offset, 16)
		if err != nil {
			return err
		}
		c.assembler.CompileMemoryWithRegisterOffsetToVectorRegister(arm64.VMOV,
			arm64ReservedRegisterForMemory, offset, result, arm64.VectorArrangementS,
		)
	case wazeroir.V128LoadType64zero:
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
	c.assembler.CompileRegisterToVectorRegister(arm64.INSGEN, source, targetVector.register, arr, arm64.VectorIndex(o.LaneIndex))

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

	c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v.register, arm64ReservedRegisterForTemporary, arr,
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
			inst = arm64.SMOV32
		} else {
			inst = arm64.UMOV
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
			inst = arm64.SMOV32
		} else {
			inst = arm64.UMOV
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
		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v.register, result,
			arm64.VectorArrangementS, arm64.VectorIndex(o.LaneIndex))

		c.locationStack.markRegisterUnused(v.register)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
	case wazeroir.ShapeI64x2:
		result, err := c.allocateRegister(registerTypeGeneralPurpose)
		if err != nil {
			return err
		}
		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v.register, result,
			arm64.VectorArrangementD, arm64.VectorIndex(o.LaneIndex))

		c.locationStack.markRegisterUnused(v.register)
		c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI64)
	case wazeroir.ShapeF32x4:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.INSELEM, v.register, v.register,
			arm64.VectorArrangementS, arm64.VectorIndex(o.LaneIndex), 0)
		c.pushRuntimeValueLocationOnRegister(v.register, runtimeValueTypeF32)
	case wazeroir.ShapeF64x2:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.INSELEM, v.register, v.register,
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
		c.assembler.CompileRegisterToVectorRegister(arm64.INSGEN, origin.register, vector.register,
			arm64.VectorArrangementB, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeI16x8:
		c.assembler.CompileRegisterToVectorRegister(arm64.INSGEN, origin.register, vector.register,
			arm64.VectorArrangementH, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeI32x4:
		c.assembler.CompileRegisterToVectorRegister(arm64.INSGEN, origin.register, vector.register,
			arm64.VectorArrangementS, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeI64x2:
		c.assembler.CompileRegisterToVectorRegister(arm64.INSGEN, origin.register, vector.register,
			arm64.VectorArrangementD, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeF32x4:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.INSELEM, origin.register, vector.register,
			arm64.VectorArrangementS, 0, arm64.VectorIndex(o.LaneIndex))
	case wazeroir.ShapeF64x2:
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.INSELEM, origin.register, vector.register,
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
		c.assembler.CompileRegisterToVectorRegister(arm64.DUPGEN, origin.register, result,
			arm64.VectorArrangement16B, arm64.VectorIndexNone)
	case wazeroir.ShapeI16x8:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUPGEN, origin.register, result,
			arm64.VectorArrangement8H, arm64.VectorIndexNone)
	case wazeroir.ShapeI32x4:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUPGEN, origin.register, result,
			arm64.VectorArrangement4S, arm64.VectorIndexNone)
	case wazeroir.ShapeI64x2:
		result, err = c.allocateRegister(registerTypeVector)
		if err != nil {
			return
		}
		c.assembler.CompileRegisterToVectorRegister(arm64.DUPGEN, origin.register, result,
			arm64.VectorArrangement2D, arm64.VectorIndexNone)
	case wazeroir.ShapeF32x4:
		result = origin.register
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.DUPELEM, origin.register, result,
			arm64.VectorArrangementS, 0, arm64.VectorIndexNone)
	case wazeroir.ShapeF64x2:
		result = origin.register
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.DUPELEM, origin.register, result,
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
			c.assembler.CompileTwoVectorRegistersToVectorRegister(arm64.VORR,
				w.register, w.register, wReg, arm64.VectorArrangement16B)
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
			c.assembler.CompileTwoVectorRegistersToVectorRegister(arm64.VORR,
				v.register, v.register, vReg, arm64.VectorArrangement16B)
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
	c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, arm64ReservedRegisterForTemporary,
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
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.CMEQZERO, arm64.RegRZR, v,
			arm64.VectorArrangement2D, arm64.VectorIndexNone, arm64.VectorIndexNone)
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.ADDP, v, v,
			arm64.VectorArrangementNone, arm64.VectorIndexNone, arm64.VectorIndexNone)
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
		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, arm64ReservedRegisterForTemporary,
			arm64.VectorArrangementD, 0)
		c.assembler.CompileTwoRegistersToNone(arm64.CMP, arm64.RegRZR, arm64ReservedRegisterForTemporary)
		c.locationStack.pushRuntimeValueLocationOnConditionalRegister(arm64.CondNE)
	}
	c.markRegisterUnused(v)
	return
}

var (
	i8x16BitmaskConst = [16]byte{
		0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80,
		0x01, 0x02, 0x04, 0x08, 0x10, 0x20, 0x40, 0x80,
	}
	i16x8BitmaskConst = [16]byte{
		0x01, 0x00, 0x02, 0x00, 0x04, 0x00, 0x08, 0x00,
		0x10, 0x00, 0x20, 0x00, 0x40, 0x00, 0x80, 0x00,
	}
	i32x4BitmaskConst = [16]byte{
		0x01, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00,
		0x04, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00,
	}
)

// compileV128BitMask implements compiler.compileV128BitMask for arm64.
func (c *arm64Compiler) compileV128BitMask(o *wazeroir.OperationV128BitMask) (err error) {
	vector := c.locationStack.popV128()
	if err = c.compileEnsureOnGeneralPurposeRegister(vector); err != nil {
		return
	}

	v := vector.register

	result, err := c.allocateRegister(registerTypeGeneralPurpose)
	if err != nil {
		return err
	}

	switch o.Shape {
	case wazeroir.ShapeI8x16:
		vecTmp, err := c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		// Right arithmetic shift on the original vector and store the result into vecTmp. So we have:
		// v[i] = 0xff if vi<0, 0 otherwise.
		c.assembler.CompileVectorRegisterToVectorRegisterWithConst(arm64.SSHR, v, v, arm64.VectorArrangement16B, 7)

		// Load the bit mask into vecTmp.
		c.assembler.CompileLoadStaticConstToVectorRegister(arm64.VMOV, i8x16BitmaskConst[:], vecTmp, arm64.VectorArrangementQ)

		// Lane-wise logical AND with i8x16BitmaskConst, meaning that we have
		// v[i] = (1 << i) if vi<0, 0 otherwise.
		//
		// Below, we use the following notation:
		// wi := (1 << i) if vi<0, 0 otherwise.
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VAND, vecTmp, v, arm64.VectorArrangement16B,
			arm64.VectorIndexNone, arm64.VectorIndexNone)

		// Swap the lower and higher 8 byte elements, and write it into vecTmp, meaning that we have
		// vecTmp[i] = w(i+8) if i < 8, w(i-8) otherwise.
		//
		c.assembler.CompileTwoVectorRegistersToVectorRegisterWithConst(arm64.EXT, v, v, vecTmp, arm64.VectorArrangement16B, 0x8)

		// v = [w0, w8, ..., w7, w15]
		c.assembler.CompileTwoVectorRegistersToVectorRegister(arm64.ZIP1, vecTmp, v, v, arm64.VectorArrangement16B)

		// v.h[0] = w0 + ... + w15
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.ADDV, v, v,
			arm64.VectorArrangement8H, arm64.VectorIndexNone, arm64.VectorIndexNone)

		// Extract the v.h[0] as the result.
		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, result, arm64.VectorArrangementH, 0)
	case wazeroir.ShapeI16x8:
		vecTmp, err := c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}
		// Right arithmetic shift on the original vector and store the result into vecTmp. So we have:
		// v[i] = 0xffff if vi<0, 0 otherwise.
		c.assembler.CompileVectorRegisterToVectorRegisterWithConst(arm64.SSHR, v, v, arm64.VectorArrangement8H, 15)

		// Load the bit mask into vecTmp.
		c.assembler.CompileLoadStaticConstToVectorRegister(arm64.VMOV, i16x8BitmaskConst[:], vecTmp, arm64.VectorArrangementQ)

		// Lane-wise logical AND with i16x8BitmaskConst, meaning that we have
		// v[i] = (1 << i)     if vi<0, 0 otherwise for i=0..3
		//      = (1 << (i+4)) if vi<0, 0 otherwise for i=3..7
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VAND, vecTmp, v, arm64.VectorArrangement16B,
			arm64.VectorIndexNone, arm64.VectorIndexNone)

		c.assembler.CompileVectorRegisterToVectorRegister(arm64.ADDV, v, v,
			arm64.VectorArrangement8H, arm64.VectorIndexNone, arm64.VectorIndexNone)

		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, result, arm64.VectorArrangementH, 0)
	case wazeroir.ShapeI32x4:
		vecTmp, err := c.allocateRegister(registerTypeVector)
		if err != nil {
			return err
		}

		// Right arithmetic shift on the original vector and store the result into vecTmp. So we have:
		// v[i] = 0xffffffff if vi<0, 0 otherwise.
		c.assembler.CompileVectorRegisterToVectorRegisterWithConst(arm64.SSHR, v, v, arm64.VectorArrangement4S, 32)

		// Load the bit mask into vecTmp.
		c.assembler.CompileLoadStaticConstToVectorRegister(arm64.VMOV, i32x4BitmaskConst[:], vecTmp, arm64.VectorArrangementQ)

		// Lane-wise logical AND with i16x8BitmaskConst, meaning that we have
		// v[i] = (1 << i)     if vi<0, 0 otherwise for i in [0, 1]
		//      = (1 << (i+4)) if vi<0, 0 otherwise for i in [2, 3]
		c.assembler.CompileVectorRegisterToVectorRegister(arm64.VAND, vecTmp, v, arm64.VectorArrangement16B,
			arm64.VectorIndexNone, arm64.VectorIndexNone)

		c.assembler.CompileVectorRegisterToVectorRegister(arm64.ADDV, v, v,
			arm64.VectorArrangement4S, arm64.VectorIndexNone, arm64.VectorIndexNone)

		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, result, arm64.VectorArrangementS, 0)
	case wazeroir.ShapeI64x2:
		// Move the lower 64-bit int into result,
		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, result,
			arm64.VectorArrangementD, 0)
		// Move the higher 64-bit int into arm64ReservedRegisterForTemporary.
		c.assembler.CompileVectorRegisterToRegister(arm64.UMOV, v, arm64ReservedRegisterForTemporary,
			arm64.VectorArrangementD, 1)

		// Move the sign bit into the least significant bit.
		c.assembler.CompileConstToRegister(arm64.LSR, 63, result)
		c.assembler.CompileConstToRegister(arm64.LSR, 63, arm64ReservedRegisterForTemporary)

		// result = (arm64ReservedRegisterForTemporary<<1) | result
		c.assembler.CompileLeftShiftedRegisterToRegister(arm64.ADD,
			arm64ReservedRegisterForTemporary, 1, result, result)
	}

	c.markRegisterUnused(v)
	c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeI32)
	return
}

// compileV128And implements compiler.compileV128And for arm64.
func (c *arm64Compiler) compileV128And(*wazeroir.OperationV128And) error {
	return c.compileV128x2BinOp(arm64.VAND, arm64.VectorArrangement16B)
}

// compileV128Not implements compiler.compileV128Not for arm64.
func (c *arm64Compiler) compileV128Not(*wazeroir.OperationV128Not) error {
	return c.compileV128UniOp(arm64.NOT, arm64.VectorArrangement16B)
}

// compileV128Or implements compiler.compileV128Or for arm64.
func (c *arm64Compiler) compileV128Or(*wazeroir.OperationV128Or) error {
	return c.compileV128x2BinOp(arm64.VORR, arm64.VectorArrangement16B)
}

// compileV128Xor implements compiler.compileV128Xor for arm64.
func (c *arm64Compiler) compileV128Xor(*wazeroir.OperationV128Xor) error {
	return c.compileV128x2BinOp(arm64.EOR, arm64.VectorArrangement16B)
}

// compileV128Bitselect implements compiler.compileV128Bitselect for arm64.
func (c *arm64Compiler) compileV128Bitselect(*wazeroir.OperationV128Bitselect) error {
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

	c.assembler.CompileTwoVectorRegistersToVectorRegister(arm64.BSL,
		x2.register, x1.register, selector.register, arm64.VectorArrangement16B)

	c.markRegisterUnused(x1.register, x2.register)
	c.pushVectorRuntimeValueLocationOnRegister(selector.register)
	return nil
}

// compileV128AndNot implements compiler.compileV128AndNot for arm64.
func (c *arm64Compiler) compileV128AndNot(*wazeroir.OperationV128AndNot) error {
	return c.compileV128x2BinOp(arm64.BIC, arm64.VectorArrangement16B)
}

func (c *arm64Compiler) compileV128UniOp(inst asm.Instruction, arr arm64.VectorArrangement) error {
	v := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(v); err != nil {
		return err
	}

	c.assembler.CompileVectorRegisterToVectorRegister(inst, v.register, v.register, arr, arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.pushVectorRuntimeValueLocationOnRegister(v.register)
	return nil
}

func (c *arm64Compiler) compileV128x2BinOp(inst asm.Instruction, arr arm64.VectorArrangement) error {
	x2 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	x1 := c.locationStack.popV128()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1); err != nil {
		return err
	}

	c.assembler.CompileVectorRegisterToVectorRegister(inst, x2.register, x1.register, arr, arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.markRegisterUnused(x2.register)
	c.pushVectorRuntimeValueLocationOnRegister(x1.register)
	return nil
}

// compileV128Shr implements compiler.compileV128Shr for arm64.
func (c *arm64Compiler) compileV128Shr(o *wazeroir.OperationV128Shr) error {
	var inst asm.Instruction
	if o.Signed {
		inst = arm64.SSHL
	} else {
		inst = arm64.USHL
	}
	return c.compileV128ShiftImpl(o.Shape, inst, true)
}

// compileV128Shl implements compiler.compileV128Shl for arm64.
func (c *arm64Compiler) compileV128Shl(o *wazeroir.OperationV128Shl) error {
	return c.compileV128ShiftImpl(o.Shape, arm64.SSHL, false)
}

func (c *arm64Compiler) compileV128ShiftImpl(shape wazeroir.Shape, ins asm.Instruction, rightShift bool) error {
	s := c.locationStack.pop()
	if s.register == arm64.RegRZR {
		// If the shift amount is zero register, nothing to do here.
		return nil
	}

	var modulo asm.ConstantValue
	var arr arm64.VectorArrangement
	switch shape {
	case wazeroir.ShapeI8x16:
		modulo = 0x7 // modulo 8.
		arr = arm64.VectorArrangement16B
	case wazeroir.ShapeI16x8:
		modulo = 0xf // modulo 16.
		arr = arm64.VectorArrangement8H
	case wazeroir.ShapeI32x4:
		modulo = 0x1f // modulo 32.
		arr = arm64.VectorArrangement4S
	case wazeroir.ShapeI64x2:
		modulo = 0x3f // modulo 64.
		arr = arm64.VectorArrangement2D
	}

	if err := c.compileEnsureOnGeneralPurposeRegister(s); err != nil {
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

	c.assembler.CompileConstToRegister(arm64.ANDIMM32, modulo, s.register)

	if rightShift {
		// Negate the amount to make this as right shift.
		c.assembler.CompileRegisterToRegister(arm64.NEG, s.register, s.register)
	}

	// Copy the shift amount into a vector register as SSHL requires it to be there.
	c.assembler.CompileRegisterToVectorRegister(arm64.DUPGEN, s.register, tmp,
		arr, arm64.VectorIndexNone)

	c.assembler.CompileVectorRegisterToVectorRegister(ins, tmp, v.register, arr,
		arm64.VectorIndexNone, arm64.VectorIndexNone)

	c.markRegisterUnused(s.register)
	c.pushVectorRuntimeValueLocationOnRegister(v.register)
	return nil
}

// compileV128Cmp implements compiler.compileV128Cmp for arm64.
func (c *arm64Compiler) compileV128Cmp(o *wazeroir.OperationV128Cmp) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128AddSat implements compiler.compileV128AddSat for arm64.
func (c *arm64Compiler) compileV128AddSat(o *wazeroir.OperationV128AddSat) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128SubSat implements compiler.compileV128SubSat for arm64.
func (c *arm64Compiler) compileV128SubSat(o *wazeroir.OperationV128SubSat) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Mul implements compiler.compileV128Mul for arm64.
func (c *arm64Compiler) compileV128Mul(o *wazeroir.OperationV128Mul) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Div implements compiler.compileV128Div for arm64.
func (c *arm64Compiler) compileV128Div(o *wazeroir.OperationV128Div) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Neg implements compiler.compileV128Neg for arm64.
func (c *arm64Compiler) compileV128Neg(o *wazeroir.OperationV128Neg) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Sqrt implements compiler.compileV128Sqrt for arm64.
func (c *arm64Compiler) compileV128Sqrt(o *wazeroir.OperationV128Sqrt) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Abs implements compiler.compileV128Abs for arm64.
func (c *arm64Compiler) compileV128Abs(o *wazeroir.OperationV128Abs) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Popcnt implements compiler.compileV128Popcnt for arm64.
func (c *arm64Compiler) compileV128Popcnt(o *wazeroir.OperationV128Popcnt) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Min implements compiler.compileV128Min for arm64.
func (c *arm64Compiler) compileV128Min(o *wazeroir.OperationV128Min) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Max implements compiler.compileV128Max for arm64.
func (c *arm64Compiler) compileV128Max(o *wazeroir.OperationV128Max) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128AvgrU implements compiler.compileV128AvgrU for arm64.
func (c *arm64Compiler) compileV128AvgrU(o *wazeroir.OperationV128AvgrU) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Pmin implements compiler.compileV128Pmin for arm64.
func (c *arm64Compiler) compileV128Pmin(o *wazeroir.OperationV128Pmin) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Pmax implements compiler.compileV128Pmax for arm64.
func (c *arm64Compiler) compileV128Pmax(o *wazeroir.OperationV128Pmax) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Ceil implements compiler.compileV128Ceil for arm64.
func (c *arm64Compiler) compileV128Ceil(o *wazeroir.OperationV128Ceil) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Floor implements compiler.compileV128Floor for arm64.
func (c *arm64Compiler) compileV128Floor(o *wazeroir.OperationV128Floor) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Trunc implements compiler.compileV128Trunc for arm64.
func (c *arm64Compiler) compileV128Trunc(o *wazeroir.OperationV128Trunc) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Nearest implements compiler.compileV128Nearest for arm64.
func (c *arm64Compiler) compileV128Nearest(o *wazeroir.OperationV128Nearest) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Extend implements compiler.compileV128Extend for arm64.
func (c *arm64Compiler) compileV128Extend(o *wazeroir.OperationV128Extend) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128ExtMul implements compiler.compileV128ExtMul for arm64.
func (c *arm64Compiler) compileV128ExtMul(o *wazeroir.OperationV128ExtMul) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Q15mulrSatS implements compiler.compileV128Q15mulrSatS for arm64.
func (c *arm64Compiler) compileV128Q15mulrSatS(o *wazeroir.OperationV128Q15mulrSatS) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128ExtAddPairwise implements compiler.compileV128ExtAddPairwise for arm64.
func (c *arm64Compiler) compileV128ExtAddPairwise(o *wazeroir.OperationV128ExtAddPairwise) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128FloatPromote implements compiler.compileV128FloatPromote for arm64.
func (c *arm64Compiler) compileV128FloatPromote(o *wazeroir.OperationV128FloatPromote) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128FloatDemote implements compiler.compileV128FloatDemote for arm64.
func (c *arm64Compiler) compileV128FloatDemote(o *wazeroir.OperationV128FloatDemote) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128FConvertFromI implements compiler.compileV128FConvertFromI for arm64.
func (c *arm64Compiler) compileV128FConvertFromI(o *wazeroir.OperationV128FConvertFromI) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Dot implements compiler.compileV128Dot for arm64.
func (c *arm64Compiler) compileV128Dot(o *wazeroir.OperationV128Dot) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128Narrow implements compiler.compileV128Narrow for arm64.
func (c *arm64Compiler) compileV128Narrow(o *wazeroir.OperationV128Narrow) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}

// compileV128ITruncSatFromF implements compiler.compileV128ITruncSatFromF for arm64.
func (c *arm64Compiler) compileV128ITruncSatFromF(o *wazeroir.OperationV128ITruncSatFromF) error {
	return fmt.Errorf("TODO: %s is not implemented yet on arm64 compiler", o.Kind())
}
