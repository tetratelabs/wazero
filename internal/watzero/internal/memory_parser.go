package internal

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func newMemoryParser(
	memorySizer func(minPages uint32, maxPages *uint32) (min, capacity, max uint32),
	memoryNamespace *indexNamespace,
	onMemory onMemory,
) *memoryParser {
	return &memoryParser{memorySizer: memorySizer, memoryNamespace: memoryNamespace, onMemory: onMemory}
}

type onMemory func(*wasm.Memory) tokenParser

// memoryParser parses an api.Memory from and dispatches to onMemory.
//
// Ex. `(module (memory 0 1024))`
//
//	    starts here --^     ^
//	onMemory resumes here --+
//
// Note: memoryParser is reusable. The caller resets via begin.
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#memories%E2%91%A7
type memoryParser struct {
	memorySizer func(minPages uint32, maxPages *uint32) (min, capacity, max uint32)

	memoryNamespace *indexNamespace

	// onMemory is invoked on end
	onMemory onMemory

	// currentMin is reset on begin and read onMemory
	currentMemory *wasm.Memory
}

// begin should be called after reaching the wasm.ExternTypeMemoryName keyword in a module field. Parsing
// continues until onMemory or error.
//
// This stage records the ID of the current memory, if present, and resumes with beginMin.
//
// Ex. A memory ID is present `(memory $mem 0)`
//
//	     records mem --^    ^
//	beginMin resumes here --+
//
// Ex. No memory ID `(memory 0)`
//
//	calls beginMin --^
func (p *memoryParser) begin(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	p.currentMemory = &wasm.Memory{}
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
		mem := p.currentMemory
		if min, err := decodePages("min", tokenBytes); err != nil {
			return nil, err
		} else {
			mem.Min, mem.Cap, mem.Max = p.memorySizer(min, nil)
			if err = mem.Validate(); err != nil {
				return nil, err
			}
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
		mem := p.currentMemory
		if max, err := decodePages("max", tokenBytes); err != nil {
			return nil, err
		} else {
			mem.Min, mem.Cap, mem.Max = p.memorySizer(p.currentMemory.Min, &max)
			mem.IsMaxEncoded = true
			if err = mem.Validate(); err != nil {
				return nil, err
			}
		}
		return p.end, nil
	case tokenRParen:
		return p.end(tok, tokenBytes, line, col)
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

func decodePages(fieldName string, tokenBytes []byte) (uint32, error) {
	i, overflow := decodeUint32(tokenBytes)
	if overflow {
		return 0, fmt.Errorf("%s %d pages (%s) over limit of %d pages (%s)", fieldName,
			i, wasm.PagesToUnitOfBytes(i), wasm.MemoryLimitPages, wasm.PagesToUnitOfBytes(wasm.MemoryLimitPages))
	}
	return i, nil
}

// end increments the memory namespace and calls onMemory with the current limits
func (p *memoryParser) end(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}
	p.memoryNamespace.count++
	return p.onMemory(p.currentMemory), nil
}
