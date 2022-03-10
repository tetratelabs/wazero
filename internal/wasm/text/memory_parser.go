package text

import (
	"errors"
	"fmt"

	wasm "github.com/tetratelabs/wazero/internal/wasm"
)

func newMemoryParser(memoryNamespace *indexNamespace, onMemory onMemory) *memoryParser {
	return &memoryParser{memoryNamespace: memoryNamespace, onMemory: onMemory}
}

type onMemory func(min uint32, max *uint32) tokenParser

// memoryParser parses a wasm.Memory from and dispatches to onMemory.
//
// Ex. `(module (memory 0 1024))`
//        starts here --^     ^
//    onMemory resumes here --+
//
// Note: memoryParser is reusable. The caller resets via begin.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memories%E2%91%A7
type memoryParser struct {
	memoryNamespace *indexNamespace

	// onMemory is invoked on end
	onMemory onMemory

	// currentMin is reset on begin and read onMemory
	currentMin uint32
	// currentMax is reset on begin and read onMemory
	currentMax *uint32
}

// begin should be called after reaching the internalwasm.ExternTypeMemoryName keyword in a module field. Parsing
// continues until onMemory or error.
//
// This stage records the ID of the current memory, if present, and resumes with beginMin.
//
// Ex. A memory ID is present `(memory $mem 0)`
//                       records mem --^    ^
//                  beginMin resumes here --+
//
// Ex. No memory ID `(memory 0)`
//          calls beginMin --^
func (p *memoryParser) begin(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	p.currentMin = 0
	p.currentMax = nil
	if tok == tokenID { // Ex. $mem
		if _, err := p.memoryNamespace.setID(tokenBytes); err != nil {
			return nil, err
		}
		return p.beginMin, nil
	}
	return p.beginMin(tok, tokenBytes, line, col)
}

// beginMin looks for the minimum memory size and proceeds with beginMax, or errs on any other token.
func (p *memoryParser) beginMin(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenID: // Ex.(memory $rf32 $rf32
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	case tokenUN:
		var overflow bool
		if p.currentMin, overflow = decodeUint32(tokenBytes); overflow || p.currentMin > wasm.MemoryPageSize {
			return nil, fmt.Errorf("min outside range of %d: %s", wasm.MemoryPageSize, tokenBytes)
		}
		return p.beginMax, nil
	case tokenRParen:
		return nil, errors.New("missing min")
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// beginMax looks for the max memory size and returns end. If this is an ')' end completes the memory. Otherwise, this
// errs on any other token.
func (p *memoryParser) beginMax(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	switch tok {
	case tokenUN:
		i, overflow := decodeUint32(tokenBytes)
		if overflow || i > wasm.MemoryPageSize {
			return nil, fmt.Errorf("min outside range of %d: %s", wasm.MemoryMaxPages, tokenBytes)
		} else if i < p.currentMin {
			return nil, fmt.Errorf("max %d < min %d", p.currentMax, p.currentMin)
		}
		p.currentMax = &i
		return p.end, nil
	case tokenRParen:
		return p.end(tok, tokenBytes, line, col)
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// end increments the memory namespace and calls onMemory with the current limits
func (p *memoryParser) end(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}
	p.memoryNamespace.count++
	return p.onMemory(p.currentMin, p.currentMax), nil
}
