package modgen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strconv"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// Gen generates a pseudo random compilable module based on `seed`.
// The size of each section is controlled by the corresponding params.
// For example, `numImports` controls the number of segment in the import section.
//
// Note: "pseudo" here means the determinism of the generated results,
// e.g. giving same seed returns exactly the same module for
// the same code base in Gen.
//
// Note: this is only used for testing wazero runtime.
func Gen(seed []byte, enabledFeature wasm.Features,
	numTypes, numFunctions, numImports, numExports, numGlobals, numElements, numData uint32,
	needStartSection bool,
) *wasm.Module {
	if len(seed) == 0 {
		return &wasm.Module{}
	}

	checksum := sha256.Sum256(seed)
	g := &generator{
		// Use 4 randoms created from the unique sha256 hash value of the seed.
		size: len(seed), rands: make([]random, 4), enabledFeature: enabledFeature,
		numTypes:         numTypes,
		numFunctions:     numFunctions,
		numImports:       numImports,
		numExports:       numExports,
		numGlobals:       numGlobals,
		numElements:      numElements,
		numData:          numData,
		needStartSection: needStartSection,
	}
	for i := 0; i < 4; i++ {
		g.rands[i] = rand.New(rand.NewSource(
			int64(binary.LittleEndian.Uint64(checksum[i*8 : (i+1)*8]))))
	}
	return g.gen()
}

type generator struct {
	// rands holds random sources for generating a module.
	rands         []random
	nextRandIndex int

	// size holds the original size of the seed.
	size int

	// m is the resulting module.
	m *wasm.Module

	enabledFeature wasm.Features
	numTypes, numFunctions, numImports, numExports,
	numGlobals, numElements, numData uint32
	needStartSection bool
}

// random is the interface over methods of rand.Rand which are used by our generator.
// This is only for testing the generator implmenetation.
type random interface {
	// See rand.Intn.
	Intn(n int) int

	// See rand.Read
	Read(p []byte) (n int, err error)
}

func (g *generator) nextRandom() (ret random) {
	ret = g.rands[g.nextRandIndex]
	g.nextRandIndex = (g.nextRandIndex + 1) % len(g.rands)
	return
}

// gen generates a random Wasm module.
func (g *generator) gen() *wasm.Module {
	g.m = &wasm.Module{}
	g.genTypeSection()
	g.genImportSection()
	g.genFunctionSection()
	g.genTableSection()
	g.genMemorySection()
	g.genGlobalSection()
	g.genExportSection()
	g.genStartSection()
	g.genElementSection()
	g.genCodeSection()
	g.genDataSection()
	return g.m
}

// genTypeSection creates random types each with a random number of parameters and results.
func (g *generator) genTypeSection() {
	for i := uint32(0); i < g.numTypes; i++ {
		var resultNumCeil = g.size
		if !g.enabledFeature.Get(wasm.FeatureMultiValue) {
			resultNumCeil = 2
		}
		ft := g.newFunctionType(g.nextRandom().Intn(g.size), g.nextRandom().Intn(resultNumCeil))
		g.m.TypeSection = append(g.m.TypeSection, ft)
	}
}

func (g *generator) newFunctionType(params, results int) *wasm.FunctionType {
	ret := &wasm.FunctionType{}
	for i := 0; i < params; i++ {
		ret.Params = append(ret.Params, g.newValueType())
	}
	for i := 0; i < results; i++ {
		ret.Results = append(ret.Results, g.newValueType())
	}
	return ret
}

func (g *generator) newValueType() (ret wasm.ValueType) {
	switch g.nextRandom().Intn(4) {
	case 0:
		ret = wasm.ValueTypeI32
	case 1:
		ret = wasm.ValueTypeI64
	case 2:
		ret = wasm.ValueTypeF32
	case 3:
		ret = wasm.ValueTypeF64
	default:
		panic("BUG")
	}
	return
}

// genImportSection creates random import descriptions, including memory and table.
func (g *generator) genImportSection() {
	var memoryImported, tableImported int
	for i := uint32(0); i < g.numImports; i++ {
		imp := &wasm.Import{
			Name:   fmt.Sprintf("%d", i),
			Module: fmt.Sprintf("module-%d", i),
		}
		g.m.ImportSection = append(g.m.ImportSection, imp)

		r := g.nextRandom().Intn(4 - memoryImported - tableImported)
		if r == 0 && len(g.m.TypeSection) > 0 {
			imp.Type = wasm.ExternTypeFunc
			imp.DescFunc = uint32(g.nextRandom().Intn(len(g.m.TypeSection)))
			continue
		}

		if r == 0 || r == 1 {
			imp.Type = wasm.ExternTypeGlobal
			imp.DescGlobal = &wasm.GlobalType{
				ValType: g.newValueType(),
				Mutable: g.nextRandom().Intn(2) == 0,
			}
			continue
		}

		if memoryImported == 0 {
			min := g.nextRandom().Intn(4) // Min in reality is relatively small like 4.
			max := g.nextRandom().Intn(int(wasm.MemoryLimitPages)-min) + min

			imp.Type = wasm.ExternTypeMemory
			imp.DescMem = &wasm.Memory{
				Min:          uint32(min),
				Max:          uint32(max),
				IsMaxEncoded: true,
			}
			memoryImported = 1
			continue
		}

		if tableImported == 0 {
			min := g.nextRandom().Intn(4) // Min in reality is relatively small like 4.
			max := uint32(g.nextRandom().Intn(int(wasm.MemoryLimitPages)-min) + min)

			imp.Type = wasm.ExternTypeTable
			tableImported = 1
			imp.DescTable = &wasm.Table{
				Min: uint32(min),
				Max: &max,
			}
			continue
		}

		panic("BUG")
	}
}

// genFunctionSection generates random function declarations whose type is randomly chosen
// from already generated type section.
func (g *generator) genFunctionSection() {
	numTypes := len(g.m.TypeSection)
	if numTypes == 0 {
		return
	}
	for i := uint32(0); i < g.numFunctions; i++ {
		typeIndex := g.nextRandom().Intn(numTypes)
		g.m.FunctionSection = append(g.m.FunctionSection, uint32(typeIndex))
	}
}

// genTableSection generates random table definition if there's no import fot table.
func (g *generator) genTableSection() {
	if g.m.ImportTableCount() != 0 {
		return
	}

	min := g.nextRandom().Intn(4) // Min in reality is relatively small like 4.
	max := uint32(g.nextRandom().Intn(int(wasm.MemoryLimitPages)-min) + min)
	g.m.TableSection = []*wasm.Table{{Min: uint32(min), Max: &max}}
}

// genTableSection generates random memory definition if there's no import fot table.
func (g *generator) genMemorySection() {
	if g.m.ImportMemoryCount() != 0 {
		return
	}
	min := g.nextRandom().Intn(4) // Min in reality is relatively small like 4.
	max := g.nextRandom().Intn(int(wasm.MemoryLimitPages)-min) + min
	g.m.MemorySection = &wasm.Memory{Min: uint32(min), Max: uint32(max), IsMaxEncoded: true}
}

// genTableSection generates random globals.
func (g *generator) genGlobalSection() {
	for i := uint32(0); i < g.numGlobals; i++ {
		expr, t := g.newConstExpr()
		mutable := g.nextRandom().Intn(2) == 0
		global := &wasm.Global{
			Type: &wasm.GlobalType{ValType: t, Mutable: mutable},
			Init: expr,
		}
		g.m.GlobalSection = append(g.m.GlobalSection, global)
	}
}

func (g *generator) newConstExpr() (*wasm.ConstantExpression, wasm.ValueType) {
	importedGlobalCount := g.m.ImportGlobalCount()
	importedGlobalsNotExist := 1
	if importedGlobalCount > 0 {
		importedGlobalsNotExist = 0
	}
	var opcode wasm.Opcode
	var data []byte
	var valueType wasm.ValueType
	switch g.nextRandom().Intn(5 - importedGlobalsNotExist) {
	case 0:
		opcode = wasm.OpcodeI32Const
		v := g.nextRandom().Intn(math.MaxInt32)
		if g.nextRandom().Intn(2) == 0 {
			v = -v
		}
		data = leb128.EncodeInt32(int32(v))
		valueType = wasm.ValueTypeI32
	case 1:
		opcode = wasm.OpcodeI64Const
		v := g.nextRandom().Intn(math.MaxInt64)
		if g.nextRandom().Intn(2) == 0 {
			v = -v
		}
		data = leb128.EncodeInt64(int64(v))
		valueType = wasm.ValueTypeI64
	case 2:
		opcode = wasm.OpcodeF32Const
		data = make([]byte, 4)
		_, err := g.nextRandom().Read(data)
		if err != nil {
			panic(err)
		}
		valueType = wasm.ValueTypeF32
	case 3:
		opcode = wasm.OpcodeF64Const
		data = make([]byte, 8)
		_, err := g.nextRandom().Read(data)
		if err != nil {
			panic(err)
		}
		valueType = wasm.ValueTypeF64
	case 4:
		opcode = wasm.OpcodeGlobalGet
		// Constexpr can only reference imported globals.
		globalIndex := g.nextRandom().Intn(int(importedGlobalCount))
		data = leb128.EncodeUint32(uint32(globalIndex))
		// Find the value type of the imported global.
		var cnt int
		for _, imp := range g.m.ImportSection {
			if imp.Type == wasm.ExternTypeGlobal {
				if cnt == globalIndex {
					valueType = imp.DescGlobal.ValType
					break
				} else {
					cnt++
				}
			}
		}
	default:
		panic("BUG")
	}
	return &wasm.ConstantExpression{Opcode: opcode, Data: data}, valueType
}

// genTableSection generates random export descriptions from previously generated functions, globals, table and memory declarations.
func (g *generator) genExportSection() {
	funcs, globals, table, memory, err := g.m.AllDeclarations()
	if err != nil {
		panic("BUG:" + err.Error())
	}

	var possibleExports []wasm.Export
	for i := range funcs {
		possibleExports = append(possibleExports, wasm.Export{Type: wasm.ExternTypeFunc, Index: uint32(i)})
	}
	for i := range globals {
		possibleExports = append(possibleExports, wasm.Export{Type: wasm.ExternTypeGlobal, Index: uint32(i)})
	}
	if table != nil {
		possibleExports = append(possibleExports, wasm.Export{Type: wasm.ExternTypeTable, Index: 0})
	}
	if memory != nil {
		possibleExports = append(possibleExports, wasm.Export{Type: wasm.ExternTypeMemory, Index: 0})
	}

	for i := uint32(0); i < g.numExports; i++ {
		target := possibleExports[g.nextRandom().Intn(len(possibleExports))]

		g.m.ExportSection = append(g.m.ExportSection, &wasm.Export{
			Type:  target.Type,
			Index: target.Index,
			Name:  strconv.Itoa(int(i)),
		})
	}
}

// genStartSection generates start section whose function is randomly chosen from previously declared function.
func (g *generator) genStartSection() {
	if !g.needStartSection {
		return
	}
	funcs, _, _, _, err := g.m.AllDeclarations()
	if err != nil {
		panic("BUG:" + err.Error())
	}

	var candidates []wasm.Index
	for funcIndex, typeIndex := range funcs {
		sig := g.m.TypeSection[typeIndex]
		// Start function must have the empty signature.
		if sig.EqualsSignature(nil, nil) {
			candidates = append(candidates, wasm.Index(funcIndex))
		}
	}

	if len(candidates) > 0 {
		g.m.StartSection = &candidates[g.nextRandom().Intn(len(candidates))]
		return
	}
}

// genStartSection generates random element section if table and functions exist.
func (g *generator) genElementSection() {
	funcs, _, _, tables, err := g.m.AllDeclarations()
	if err != nil {
		panic("BUG:" + err.Error())
	}

	numFuncs := len(funcs)
	if tables == nil || numFuncs == 0 {
		return
	}

	min := tables[0].Min
	for i := uint32(0); i < g.numElements; i++ {
		// Elements can't exceed min of table.
		indexes := make([]*wasm.Index, g.nextRandom().Intn(int(min)+1))
		for i := range indexes {
			v := uint32(g.nextRandom().Intn(numFuncs))
			indexes[i] = &v
		}

		offset := g.nextRandom().Intn(int(min) - len(indexes) + 1)
		elem := &wasm.ElementSegment{
			OffsetExpr: &wasm.ConstantExpression{
				// TODO: support global.get expression.
				Opcode: wasm.OpcodeI32Const,
				Data:   leb128.EncodeInt32(int32(offset)),
			},
			Init: indexes,
			Type: wasm.RefTypeFuncref,
		}
		g.m.ElementSection = append(g.m.ElementSection, elem)
	}
}

// genCodeSection generates random code section for functions defined in this module.
func (g *generator) genCodeSection() {
	codeSectionSize := len(g.m.FunctionSection)
	for i := 0; i < codeSectionSize; i++ {
		g.m.CodeSection = append(g.m.CodeSection, g.newCode())
	}
}

func (g *generator) newCode() *wasm.Code {
	// TODO: generate random body.
	return &wasm.Code{Body: []byte{wasm.OpcodeUnreachable, // With unreachable allows us to make this body valid for any signature.
		wasm.OpcodeEnd}}
}

// genDataSection generates random data section if memory is declared and its min is not zero.
func (g *generator) genDataSection() {
	_, _, mem, _, err := g.m.AllDeclarations()
	if err != nil {
		panic("BUG:" + err.Error())
	}

	if mem == nil || mem.Min == 0 || g.numData == 0 {
		return
	}

	min := int(mem.Min * wasm.MemoryPageSize)
	for i := uint32(0); i < g.numData; i++ {
		offset := g.nextRandom().Intn(min)
		expr := &wasm.ConstantExpression{
			Opcode: wasm.OpcodeI32Const,
			Data:   leb128.EncodeInt32(int32(offset)),
		}

		init := make([]byte, g.nextRandom().Intn(min-offset+1))
		if len(init) == 0 {
			continue
		}
		_, err := g.nextRandom().Read(init)
		if err != nil {
			panic(err)
		}

		g.m.DataSection = append(g.m.DataSection, &wasm.DataSegment{
			OffsetExpression: expr,
			Init:             init,
		})
	}
}
