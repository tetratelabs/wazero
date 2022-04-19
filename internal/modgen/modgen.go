package modgen

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
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
	g := &generator{size: len(seed)}
	for i := 0; i < 4; i++ {
		g.rands[i] = rand.New(rand.NewSource(
			int64(binary.LittleEndian.Uint64(checksum[i*8 : (i+1)*8]))))
	}
	return g.gen()
}

type generator struct {
	// rands holds 4 Rand created from the unique sha256 hash value of the seed.
	rands         [4]*rand.Rand
	nextRandIndex int

	// size holds the original size of the seed.
	size int

	// m is the resulting module.
	m *wasm.Module
}

func (g *generator) nextRand() (ret *rand.Rand) {
	ret = g.rands[g.nextRandIndex]
	g.nextRandIndex = (g.nextRandIndex) % 4
	return
}

func (g *generator) gen() *wasm.Module {
	g.m = &wasm.Module{}

	g.typeSection()
	g.importSection()
	return g.m
}

func (g *generator) getSectionSize() int {
	// TODO comment
	return g.nextRand().Intn(g.size&0x00ff_ffff) + 1
}

func (g *generator) typeSection() {
	numTypes := g.getSectionSize()
	fmt.Println(numTypes)
	for i := 0; i < numTypes; i++ {
		ft := g.newFunctionType(g.nextRand().Intn(g.size), g.nextRand().Intn(g.size))
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
	switch g.nextRand().Intn(4) {
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
	numImports := g.getSectionSize()
	var memoryImported, tableImported int
	for i := 0; i < numImports; i++ {
		imp := &wasm.Import{}
		switch g.nextRand().Intn(4 - memoryImported - tableImported) {
		case 0:
			imp.Type = wasm.ExternTypeFunc
			imp.DescFunc = uint32(g.nextRand().Intn(len(g.m.TypeSection)))
		case 1:
			imp.Type = wasm.ExternTypeGlobal
			imp.DescGlobal = &wasm.GlobalType{
				ValType: g.newValueType(),
				Mutable: g.nextRand().Intn(2) == 0,
			}
		case 2:
			imp.Type = wasm.ExternTypeTable
			tableImported = 1
			imp.DescTable = &wasm.Table{}
		case 3:
			imp.Type = wasm.ExternTypeMemory
			imp.DescMem = &wasm.Memory{}
			memoryImported = 1
		default:
			panic("BUG")
		}
		g.m.ImportSection = append(g.m.ImportSection, imp)
	}
}
