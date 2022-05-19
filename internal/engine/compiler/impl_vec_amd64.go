package compiler

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/asm/amd64"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// compileConstV128 implements compiler.compileConstV128 for amd64 architecture.
func (c *amd64Compiler) compileConstV128(o *wazeroir.OperationConstV128) error {
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

// compileAddV128 implements compiler.compileAddV128 for amd64 architecture.
func (c *amd64Compiler) compileAddV128(o *wazeroir.OperationAddV128) error {
	c.locationStack.pop() // skip higher 64-bits.
	x2 := c.locationStack.pop()
	if err := c.compileEnsureOnGeneralPurposeRegister(x2); err != nil {
		return err
	}

	c.locationStack.pop() // skip higher 64-bits.
	x1 := c.locationStack.pop()
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
