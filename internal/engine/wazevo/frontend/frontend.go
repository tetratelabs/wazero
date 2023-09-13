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

	m      *wasm.Module
	offset *wazevoapi.ModuleContextOffsetData
	// ssaBuilder is a ssa.Builder used by this frontend.
	ssaBuilder    ssa.Builder
	signatures    map[*wasm.FunctionType]*ssa.Signature
	memoryGrowSig ssa.Signature

	// Followings are reset by per function.

	// wasmLocalToVariable maps the index (considered as wasm.Index of locals)
	// to the corresponding ssa.Variable.
	wasmLocalToVariable                   map[wasm.Index]ssa.Variable
	wasmLocalFunctionIndex                wasm.Index
	wasmFunctionTyp                       *wasm.FunctionType
	wasmFunctionLocalTypes                []wasm.ValueType
	wasmFunctionBody                      []byte
	memoryBaseVariable, memoryLenVariable ssa.Variable
	needMemory                            bool
	globalVariables                       []ssa.Variable
	globalVariablesTypes                  []ssa.Type
	mutableGlobalVariablesIndexes         []wasm.Index // index to ^.
	// br is reused during lowering.
	br            *bytes.Reader
	loweringState loweringState

	execCtxPtrValue, moduleCtxPtrValue ssa.Value
}

// NewFrontendCompiler returns a frontend Compiler.
func NewFrontendCompiler(m *wasm.Module, ssaBuilder ssa.Builder, offset *wazevoapi.ModuleContextOffsetData) *Compiler {
	c := &Compiler{
		m:                   m,
		ssaBuilder:          ssaBuilder,
		br:                  bytes.NewReader(nil),
		wasmLocalToVariable: make(map[wasm.Index]ssa.Variable),
		offset:              offset,
	}

	c.signatures = make(map[*wasm.FunctionType]*ssa.Signature, len(m.TypeSection)+1)
	for i := range m.TypeSection {
		wasmSig := &m.TypeSection[i]
		sig := SignatureForWasmFunctionType(wasmSig)
		sig.ID = ssa.SignatureID(i)
		c.signatures[wasmSig] = &sig
		c.ssaBuilder.DeclareSignature(&sig)
	}

	c.memoryGrowSig = ssa.Signature{
		ID: ssa.SignatureID(len(m.TypeSection)),
		// Takes execution context and the page size to grow.
		Params: []ssa.Type{ssa.TypeI64, ssa.TypeI32},
		// Returns the new size.
		Results: []ssa.Type{ssa.TypeI32},
	}
	c.ssaBuilder.DeclareSignature(&c.memoryGrowSig)

	return c
}

func SignatureForWasmFunctionType(typ *wasm.FunctionType) ssa.Signature {
	sig := ssa.Signature{
		// +2 to pass moduleContextPtr and executionContextPtr. See the inline comment LowerToSSA.
		Params:  make([]ssa.Type, len(typ.Params)+2),
		Results: make([]ssa.Type, len(typ.Results)),
	}
	sig.Params[0] = executionContextPtrTyp
	sig.Params[1] = moduleContextPtrTyp
	for j, typ := range typ.Params {
		sig.Params[j+2] = WasmTypeToSSAType(typ)
	}
	for j, typ := range typ.Results {
		sig.Results[j] = WasmTypeToSSAType(typ)
	}
	return sig
}

// Init initializes the state of frontendCompiler and make it ready for a next function.
func (c *Compiler) Init(idx wasm.Index, typ *wasm.FunctionType, localTypes []wasm.ValueType, body []byte) {
	c.ssaBuilder.Init(c.signatures[typ])
	c.loweringState.reset()

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
func (c *Compiler) LowerToSSA() {
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
		st := WasmTypeToSSAType(typ)
		variable := builder.DeclareVariable(st)
		value := entryBlock.AddParam(builder, st)
		builder.DefineVariable(variable, value, entryBlock)
		c.wasmLocalToVariable[wasm.Index(i)] = variable
	}
	c.declareWasmLocals(entryBlock)
	c.declareNecessaryVariables()

	c.lowerBody(entryBlock)
}

// localVariable returns the SSA variable for the given Wasm local index.
func (c *Compiler) localVariable(index wasm.Index) ssa.Variable {
	return c.wasmLocalToVariable[index]
}

// declareWasmLocals declares the SSA variables for the Wasm locals.
func (c *Compiler) declareWasmLocals(entry ssa.BasicBlock) {
	localCount := wasm.Index(len(c.wasmFunctionTyp.Params))
	for i, typ := range c.wasmFunctionLocalTypes {
		st := WasmTypeToSSAType(typ)
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
		case ssa.TypeV128:
			zeroInst.AsVconst(0, 0)
		default:
			panic("TODO: " + wasm.ValueTypeName(typ))
		}

		c.ssaBuilder.InsertInstruction(zeroInst)
		value := zeroInst.Return()
		c.ssaBuilder.DefineVariable(variable, value, entry)
	}
}

func (c *Compiler) declareNecessaryVariables() {
	c.needMemory = c.m.ImportMemoryCount > 0 || c.m.MemorySection != nil
	if c.needMemory {
		c.memoryBaseVariable = c.ssaBuilder.DeclareVariable(ssa.TypeI64)
		c.memoryLenVariable = c.ssaBuilder.DeclareVariable(ssa.TypeI64)
	}

	c.globalVariables = c.globalVariables[:0]
	c.mutableGlobalVariablesIndexes = c.mutableGlobalVariablesIndexes[:0]
	c.globalVariablesTypes = c.globalVariablesTypes[:0]
	for _, imp := range c.m.ImportSection {
		if imp.Type == wasm.ExternTypeGlobal {
			desc := imp.DescGlobal
			c.declareWasmGlobal(desc.ValType, desc.Mutable)
		}
	}
	for _, g := range c.m.GlobalSection {
		desc := g.Type
		c.declareWasmGlobal(desc.ValType, desc.Mutable)
	}

	// TODO: add tables.
}

func (c *Compiler) declareWasmGlobal(typ wasm.ValueType, mutable bool) {
	var st ssa.Type
	switch typ {
	case wasm.ValueTypeI32:
		st = ssa.TypeI32
	case wasm.ValueTypeI64:
		st = ssa.TypeI64
	case wasm.ValueTypeF32:
		st = ssa.TypeF32
	case wasm.ValueTypeF64:
		st = ssa.TypeF64
	case wasm.ValueTypeV128:
		st = ssa.TypeV128
	default:
		panic("TODO: " + wasm.ValueTypeName(typ))
	}
	v := c.ssaBuilder.DeclareVariable(st)
	index := wasm.Index(len(c.globalVariables))
	c.globalVariables = append(c.globalVariables, v)
	c.globalVariablesTypes = append(c.globalVariablesTypes, st)
	if mutable {
		c.mutableGlobalVariablesIndexes = append(c.mutableGlobalVariablesIndexes, index)
	}
}

// WasmTypeToSSAType converts wasm.ValueType to ssa.Type.
func WasmTypeToSSAType(vt wasm.ValueType) ssa.Type {
	switch vt {
	case wasm.ValueTypeI32:
		return ssa.TypeI32
	case wasm.ValueTypeI64:
		return ssa.TypeI64
	case wasm.ValueTypeF32:
		return ssa.TypeF32
	case wasm.ValueTypeF64:
		return ssa.TypeF64
	case wasm.ValueTypeV128:
		return ssa.TypeV128
	default:
		panic("TODO: " + wasm.ValueTypeName(vt))
	}
}

// addBlockParamsFromWasmTypes adds the block parameters to the given block.
func (c *Compiler) addBlockParamsFromWasmTypes(tps []wasm.ValueType, blk ssa.BasicBlock) {
	for _, typ := range tps {
		st := WasmTypeToSSAType(typ)
		blk.AddParam(c.ssaBuilder, st)
	}
}

// formatBuilder outputs the constructed SSA function as a string with a source information.
func (c *Compiler) formatBuilder() string {
	// TODO: use source position to add the Wasm-level source info.
	return c.ssaBuilder.Format()
}
