// Package frontend implements the translation of WebAssembly to SSA IR using the ssa package.
package frontend

import (
	"bytes"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Compiler is in charge of lowering Wasm to SSA IR, and does the optimization
// on top of it in architecture-independent way.
type Compiler struct {
	// Per-module data that is used across all functions.

	m *wasm.Module
	// ssaBuilder is a ssa.Builder used by this frontend.
	ssaBuilder ssa.Builder
	signatures map[*wasm.FunctionType]*ssa.Signature

	// Followings are reset by per function.

	// wasmLocalToVariable maps the index (considered as wasm.Index of locals)
	// to the corresponding ssa.Variable.
	wasmLocalToVariable    map[wasm.Index]ssa.Variable
	wasmLocalFunctionIndex wasm.Index
	wasmFunctionTyp        *wasm.FunctionType
	wasmFunctionLocalTypes []wasm.ValueType
	wasmFunctionBody       []byte
	// br is reused during lowering.
	br *bytes.Reader
	// trapBlocks maps wazevoapi.ExitCode to the corresponding BasicBlock which
	// exits the execution with the code.
	trapBlocks    [wazevoapi.ExitCodeCount]ssa.BasicBlock
	loweringState loweringState

	execCtxPtrValue, moduleCtxPtrValue ssa.Value
}

// NewFrontendCompiler returns a frontend Compiler.
func NewFrontendCompiler(m *wasm.Module, ssaBuilder ssa.Builder) *Compiler {
	c := &Compiler{
		m:                   m,
		ssaBuilder:          ssaBuilder,
		br:                  bytes.NewReader(nil),
		wasmLocalToVariable: make(map[wasm.Index]ssa.Variable),
	}

	c.signatures = make(map[*wasm.FunctionType]*ssa.Signature, len(m.TypeSection))
	for i := range m.TypeSection {
		wasmSig := &m.TypeSection[i]
		sig := &ssa.Signature{
			ID: ssa.SignatureID(i),
			// +2 to pass moduleContextPtr and executionContextPtr. See the inline comment LowerToSSA.
			Params:  make([]ssa.Type, len(wasmSig.Params)+2),
			Results: make([]ssa.Type, len(wasmSig.Results)),
		}
		sig.Params[0] = executionContextPtrTyp
		sig.Params[1] = moduleContextPtrTyp
		for j, typ := range wasmSig.Params {
			sig.Params[j+2] = wasmToSSA(typ)
		}
		for j, typ := range wasmSig.Results {
			sig.Results[j] = wasmToSSA(typ)
		}
		c.signatures[wasmSig] = sig
		c.ssaBuilder.DeclareSignature(sig)
	}
	return c
}

// Init initializes the state of frontendCompiler and make it ready for a next function.
func (c *Compiler) Init(idx wasm.Index, typ *wasm.FunctionType, localTypes []wasm.ValueType, body []byte) {
	c.ssaBuilder.Init(c.signatures[typ])
	c.loweringState.reset()
	c.trapBlocks = [wazevoapi.ExitCodeCount]ssa.BasicBlock{}

	c.wasmLocalFunctionIndex = idx
	c.wasmFunctionTyp = typ
	c.wasmFunctionLocalTypes = localTypes
	c.wasmFunctionBody = body
}

// Note: this assumes 64-bit platform (I believe we won't have 32-bit backend ;)).
const executionContextPtrTyp, moduleContextPtrTyp = ssa.TypeI64, ssa.TypeI64

// LowerToSSA lowers the current function to SSA function which will be held by ssaBuilder.
// After calling this, the caller will be able to access the SSA info in *Compiler.ssaBuilder.
//
// Note that this only does the naive lowering, and do not do any optimization, instead the caller is expected to do so.
func (c *Compiler) LowerToSSA() error {
	builder := c.ssaBuilder

	// Set up the entry block.
	entryBlock := builder.AllocateBasicBlock()
	builder.SetCurrentBlock(entryBlock)

	// Functions always take two parameters in addition to Wasm-level parameters:
	//
	//  1. executionContextPtr: pointer to the *executionContext in wazevo package.
	//    This will be used to exit the execution in the face of trap, plus used for host function calls.
	//
	// 	2. moduleContextPtr: pointer to the *moduleContextOpaque in wazevo package.
	//	  This will be used to access memory, etc. Also, this will be used during host function calls.
	//
	// Note: it's clear that sometimes a function won't need them. For example,
	//  if the function doesn't trap and doesn't make function call, then
	// 	we might be able to eliminate the parameter. However, if that function
	//	can be called via call_indirect, then we cannot eliminate because the
	//  signature won't match with the expected one.
	// TODO: maybe there's some way to do this optimization without glitches, but so far I have no clue about the feasibility.
	//
	// Note: In Wasmtime or many other runtimes, moduleContextPtr is called "vmContext". Also note that `moduleContextPtr`
	//  is wazero-specific since other runtimes can naturally use the OS-level signal to do this job thanks to the fact that
	//  they can use native stack vs wazero cannot use Go-routine stack and have to use Go-runtime allocated []byte as a stack.
	c.execCtxPtrValue = entryBlock.AddParam(builder, executionContextPtrTyp)
	c.moduleCtxPtrValue = entryBlock.AddParam(builder, moduleContextPtrTyp)
	builder.AnnotateValue(c.execCtxPtrValue, "exec_ctx")
	builder.AnnotateValue(c.moduleCtxPtrValue, "module_ctx")

	for i, typ := range c.wasmFunctionTyp.Params {
		st := wasmToSSA(typ)
		variable := builder.DeclareVariable(st)
		value := entryBlock.AddParam(builder, st)
		builder.DefineVariable(variable, value, entryBlock)
		c.wasmLocalToVariable[wasm.Index(i)] = variable
	}
	c.declareWasmLocals(entryBlock)

	c.lowerBody(entryBlock)
	c.emitTrapBlocks()
	return nil
}

// localVariable returns the SSA variable for the given Wasm local index.
func (c *Compiler) localVariable(index wasm.Index) ssa.Variable {
	return c.wasmLocalToVariable[index]
}

// declareWasmLocals declares the SSA variables for the Wasm locals.
func (c *Compiler) declareWasmLocals(entry ssa.BasicBlock) {
	localCount := wasm.Index(len(c.wasmFunctionTyp.Params))
	for i, typ := range c.wasmFunctionLocalTypes {
		st := wasmToSSA(typ)
		variable := c.ssaBuilder.DeclareVariable(st)
		c.wasmLocalToVariable[wasm.Index(i)+localCount] = variable

		zeroInst := c.ssaBuilder.AllocateInstruction()
		switch st {
		case ssa.TypeI32:
			zeroInst.AsIconst32(0)
		case ssa.TypeI64:
			zeroInst.AsIconst64(0)
		case ssa.TypeF32:
			zeroInst.AsF32const(0)
		case ssa.TypeF64:
			zeroInst.AsF64const(0)
		default:
			panic("TODO: " + wasm.ValueTypeName(typ))
		}

		c.ssaBuilder.InsertInstruction(zeroInst)
		value := zeroInst.Return()
		c.ssaBuilder.DefineVariable(variable, value, entry)
	}
}

// wasmToSSA converts wasm.ValueType to ssa.Type.
func wasmToSSA(vt wasm.ValueType) ssa.Type {
	switch vt {
	case wasm.ValueTypeI32:
		return ssa.TypeI32
	case wasm.ValueTypeI64:
		return ssa.TypeI64
	case wasm.ValueTypeF32:
		return ssa.TypeF32
	case wasm.ValueTypeF64:
		return ssa.TypeF64
	default:
		panic("TODO: " + wasm.ValueTypeName(vt))
	}
}

// addBlockParamsFromWasmTypes adds the block parameters to the given block.
func (c *Compiler) addBlockParamsFromWasmTypes(tps []wasm.ValueType, blk ssa.BasicBlock) {
	for _, typ := range tps {
		st := wasmToSSA(typ)
		blk.AddParam(c.ssaBuilder, st)
	}
}

// formatBuilder outputs the constructed SSA function as a string with a source information.
func (c *Compiler) formatBuilder() string {
	// TODO: use source position to add the Wasm-level source info.
	return c.ssaBuilder.Format()
}

// getOrCreateTrapBlock returns the trap block for the given trap code.
func (c *Compiler) getOrCreateTrapBlock(code wazevoapi.ExitCode) ssa.BasicBlock {
	blk := c.trapBlocks[code]
	if blk == nil {
		blk = c.ssaBuilder.AllocateBasicBlock()
		c.trapBlocks[code] = blk
	}
	return blk
}

// emitTrapBlocks emits the trap blocks.
func (c *Compiler) emitTrapBlocks() {
	builder := c.ssaBuilder
	for exitCode := wazevoapi.ExitCode(0); exitCode < wazevoapi.ExitCodeCount; exitCode++ {
		blk := c.trapBlocks[exitCode]
		if blk == nil {
			continue
		}
		builder.SetCurrentBlock(blk)

		exitCodeInstr := builder.AllocateInstruction()
		exitCodeInstr.AsIconst32(uint32(exitCode))
		builder.InsertInstruction(exitCodeInstr)
		exitCodeVal := exitCodeInstr.Return()

		execCtx := c.execCtxPtrValue
		store := builder.AllocateInstruction()
		store.AsStore(exitCodeVal, execCtx, wazevoapi.ExecutionContextOffsets.ExitCodeOffset.U32())
		builder.InsertInstruction(store)

		trap := builder.AllocateInstruction()
		trap.AsTrap(c.execCtxPtrValue)
		builder.InsertInstruction(trap)
	}
}
