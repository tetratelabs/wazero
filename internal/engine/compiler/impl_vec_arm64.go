package compiler

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/arm64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

func (c *arm64Compiler) compileConstV128(o *wazeroir.OperationConstV128) error {
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

	c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeV128Lo)
	c.pushRuntimeValueLocationOnRegister(result, runtimeValueTypeV128Hi)
	return nil
}

func (c *arm64Compiler) compileAddV128(o *wazeroir.OperationAddV128) error {
	c.locationStack.pop() // skip higher 64-bits.
	x1Low := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x1Low); err != nil {
		return err
	}
	c.locationStack.pop() // skip higher 64-bits.
	x2Low := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2Low); err != nil {
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

	c.assembler.CompileVectorRegisterToVectorRegister(inst, x1Low.register, x2Low.register, arr)

	resultReg := x2Low.register
	c.pushRuntimeValueLocationOnRegister(resultReg, runtimeValueTypeV128Lo)
	c.pushRuntimeValueLocationOnRegister(resultReg, runtimeValueTypeV128Hi)

	c.markRegisterUnused(x1Low.register)
	return nil
}
