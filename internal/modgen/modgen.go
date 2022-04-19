package modgen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// Gen generates a pseudo random compilable module based on `seed`.
// "Pseudo" here means the determinism of the generated results,
// e.g. giving same seed returns exactly the same module for
// the same code base in Gen.
//
// Note: this is only used for testing wazero runtime.
func Gen(seed []byte) *wasm.Module {
	if len(seed) == 0 {
		return &wasm.Module{}
	}

	checksum := sha256.Sum256(seed)
	// Use 4 randoms created from the unique sha256 hash value of the seed.
	g := &generator{size: len(seed), rands: make([]random, 4)}
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
}

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

func (g *generator) gen() *wasm.Module {
	g.m = &wasm.Module{}
	g.typeSection()
	g.importSection()
	g.functionSection()
	g.tableSection()
	g.memorySection()
	g.globalSection()
	g.exportSection()
	g.startSection()
	g.elementSection()
	g.codeSection()
	g.dataSection()
	return g.m
}

func (g *generator) typeSection() {
	numTypes := g.nextRandom().Intn(g.size)
	for i := 0; i < numTypes; i++ {
		ft := g.newFunctionType(g.nextRandom().Intn(g.size), g.nextRandom().Intn(g.size))
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

func (g *generator) importSection() {
	numImports := g.nextRandom().Intn(g.size)
	var memoryImported, tableImported int
	for i := 0; i < numImports; i++ {
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
			max := g.nextRandom().Intn(int(wasm.MemoryMaxPages)-min) + min

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
			max := uint32(g.nextRandom().Intn(int(math.MaxInt32)-min) + min)

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

func (g *generator) functionSection() {
	numTypes := len(g.m.TypeSection)
	if numTypes == 0 {
		return
	}
	numFunctions := g.nextRandom().Intn(g.size)
	for i := 0; i < numFunctions; i++ {
		typeIndex := g.nextRandom().Intn(numTypes)
		g.m.FunctionSection = append(g.m.FunctionSection, uint32(typeIndex))
	}
}

func (g *generator) tableSection() {
	if g.m.ImportTableCount() != 0 {
		return
	}

	min := g.nextRandom().Intn(4) // Min in reality is relatively small like 4.
	max := uint32(g.nextRandom().Intn(int(math.MaxInt32)-min) + min)
	g.m.TableSection = &wasm.Table{Min: uint32(min), Max: &max}
}

func (g *generator) memorySection() {
	if g.m.ImportMemoryCount() != 0 {
		return
	}
	min := g.nextRandom().Intn(4) // Min in reality is relatively small like 4.
	max := g.nextRandom().Intn(int(wasm.MemoryMaxPages)-min) + min
	g.m.MemorySection = &wasm.Memory{Min: uint32(min), Max: uint32(max), IsMaxEncoded: true}
}

func (g *generator) globalSection() {

}

func (g *generator) exportSection() {

}

func (g *generator) startSection() {

}

func (g *generator) elementSection() {

}

func (g *generator) codeSection() {

}

func (g *generator) dataSection() {

}
