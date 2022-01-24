package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
)

// funcParser parses any instructions and dispatches to onBodyEnd.
//
// Ex.  `(module (func (nop)))`
//       starts here --^    ^
// onBodyEnd resumes here --+
//
// Note: funcParser is reusable. The caller resets when reaching the appropriate tokenRParen via beginBody.
type funcParser struct {
	// m is used as a function pointer to moduleParser.tokenParser. This updates based on state changes.
	m *moduleParser

	// onBodyEnd is called when complete parsing the body. Unless testing, this should be moduleParser.parseFuncEnd
	onBodyEnd tokenParser

	currentInstruction wasm.Opcode
	// currentParameters are the current parameters to currentInstruction in WebAssembly 1.0 (MVP) binary format
	currentParameters []byte

	// currentCode is the current function body encoded in WebAssembly 1.0 (MVP) binary format
	currentCode []byte
}

// end indicates the end of instructions in this function body
// See https://www.w3.org/TR/wasm-core-1/#expressions%E2%91%A0
var end = []byte{wasm.OpcodeEnd}

func (p *funcParser) getBody() []byte {
	if p.currentCode == nil {
		return end
	}
	return append(p.currentCode, wasm.OpcodeEnd)
}

// beginLocalsOrBody returns a parser that consumes a function body
//
// The onBodyEnd field is invoked once any instructions are written into currentCode.
//
// Ex. Given the source `(module (func nop))`
//             beginBody starts here --^  ^
//               onBodyEnd resumes here --+
//
//
// NOTE: An empty function is valid and will not reach a tokenLParen! Ex. `(module (func))`
func (p *funcParser) beginBody() tokenParser {
	p.currentCode = nil
	p.m.tokenParser = p.parseBody
	return p.m.parse
}

// beginBodyField returns a parser that starts inside the first field of a function that isn't a type use.
//
// The onBodyEnd field is invoked once any instructions are written into currentCode.
//
// Ex. Given the source `(module (func $main (param i32) (nop)))`
//                          beginBodyField starts here --^    ^
//                                   onBodyEnd resumes here --+
//
//
// NOTE: An empty function is valid and will not reach a tokenLParen! Ex. `(module (func))`
func (p *funcParser) beginBodyField() tokenParser {
	p.currentCode = nil
	p.m.tokenParser = p.parseBody
	return p.m.parse
}

func sExpressionsUnsupported(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	if tok != tokenKeyword {
		return unexpectedToken(tok, tokenBytes)
	}
	fieldName := string(tokenBytes)
	switch fieldName {
	case "result", "param": // TODO: local
		return fmt.Errorf("%s declared out of order", fieldName)
	}
	return fmt.Errorf("TODO: s-expressions are not yet supported: %s", fieldName)
}

func (p *funcParser) parseBody(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenLParen {
		p.m.tokenParser = sExpressionsUnsupported
		return nil
	}
	return p.beginInstruction(tok, tokenBytes, line, col)
}

// beginInstruction is a tokenParser called after a tokenLParen and accepts an instruction field.
func (p *funcParser) beginInstruction(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenKeyword {
		switch string(tokenBytes) {
		case "local.get":
			p.currentInstruction = wasm.OpcodeLocalGet
			p.m.tokenParser = p.parseIndex
			return nil
		case "i32.add":
			p.currentInstruction = wasm.OpcodeI32Add
			return p.endInstruction()
		}
		return fmt.Errorf("unsupported instruction: %s", tokenBytes)
	}
	return p.onBodyEnd(tok, tokenBytes, line, col)
}

func (p *funcParser) parseIndex(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenUN: // Ex. 1
		i, err := decodeUint32(tokenBytes)
		if err != nil {
			return fmt.Errorf("malformed i32 %s: %w", tokenBytes, errors.Unwrap(err))
		}
		p.currentParameters = leb128.EncodeUint32(i)
		// TODO: it is possible this is out of range in the index. Local out-of-range is caught in Store.Initialize, but
		// we can provide a better area since we can access the line and col info. However, since the local index starts
		// with parameters and they are on the type, and that type may be defined after the function, it can get hairy.
		// To handle this neatly means doing a quick check to see if the type is already present and immediately
		// validate. If the type isn't yet present, we can save off the context for late validation, noting that we
		// should probably save off the instruction count or at least the current opcode to help with the error message.
		return p.endInstruction()
	case tokenID: // Ex $y
		return errors.New("TODO: index variables are not yet supported")
	}
	return unexpectedToken(tok, tokenBytes)
}

func (p *funcParser) endInstruction() error {
	p.currentCode = append(p.currentCode, p.currentInstruction)
	p.currentCode = append(p.currentCode, p.currentParameters...)
	p.currentParameters = nil
	p.m.tokenParser = p.parseBody
	return nil
}

func (p *funcParser) errorContext() string {
	return "" // TODO: add locals etc
}
