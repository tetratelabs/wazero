package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func newMemoryParser(memoryMaxPages uint32, memoryNamespace *indexNamespace, onMemory onMemory) *memoryParser {
	return &memoryParser{memoryMaxPages: memoryMaxPages, memoryNamespace: memoryNamespace, onMemory: onMemory}
}

type onMemory func(min, max uint32) tokenParser

// memoryParser parses a api.Memory from and dispatches to onMemory.
//
// Ex. `(module (memory 0 1024))`
//        starts here --^     ^
//    onMemory resumes here --+
//
// Note: memoryParser is reusable. The caller resets via begin.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memories%E2%91%A7
type memoryParser struct {
	// memoryMaxPages is the limit of pages (not bytes) for each wasm.Memory.
	memoryMaxPages uint32

	memoryNamespace *indexNamespace

	// onMemory is invoked on end
	onMemory onMemory

	// currentMin is reset on begin and read onMemory
	currentMin uint32
	// currentMax is reset on begin and read onMemory
	currentMax uint32
}

// begin should be called after reaching the wasm.ExternTypeMemoryName keyword in a module field. Parsing
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
	p.currentMax = p.memoryMaxPages
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
		if i, overflow := decodeUint32(tokenBytes); overflow || i > p.memoryMaxPages {
			return nil, fmt.Errorf("min %d pages (%s) outside range of %d pages (%s)", i, wasm.PagesToUnitOfBytes(i), p.memoryMaxPages, wasm.PagesToUnitOfBytes(p.memoryMaxPages))
		} else {
			p.currentMin = i
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
		if overflow || i > p.memoryMaxPages {
			return nil, fmt.Errorf("max %d pages (%s) outside range of %d pages (%s)", i, wasm.PagesToUnitOfBytes(i), p.memoryMaxPages, wasm.PagesToUnitOfBytes(p.memoryMaxPages))
		} else if i < p.currentMin {
			return nil, fmt.Errorf("min %d pages (%s) > max %d pages (%s)", p.currentMin, wasm.PagesToUnitOfBytes(p.currentMin), i, wasm.PagesToUnitOfBytes(i))
		}
		p.currentMax = i
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
