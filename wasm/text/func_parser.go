package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
)

func newFuncParser(onFunc onFunc) *funcParser {
	return &funcParser{onFunc: onFunc}
}

type onFunc func(typeIdx wasm.Index, code *wasm.Code, localNames wasm.NameMap) (tokenParser, error)

// funcParser parses any instructions and dispatches to onFunc.
//
// Ex.  `(module (func (nop)))`
//       starts here --^    ^
//      calls onFunc here --+
//
// Note: funcParser is reusable. The caller resets via begin.
type funcParser struct {
	// onFunc is called when complete parsing the body. Unless testing, this should be moduleParser.onFuncEnd
	onFunc onFunc

	currentTypeIdx    wasm.Index
	currentParamNames wasm.NameMap

	// currentOpcode is the opcode parsed from an instruction. Ex wasm.OpcodeLocalGet if "local.get 3"
	currentOpcode wasm.Opcode

	// currentParameters are the parameters to the currentOpcode in WebAssembly 1.0 (MVP) binary format
	currentParameters []byte

	// currentBody is the current function body encoded in WebAssembly 1.0 (MVP) binary format
	currentBody []byte
}

// end indicates the end of instructions in this function body
// See https://www.w3.org/TR/wasm-core-1/#expressions%E2%91%A0
var end = []byte{wasm.OpcodeEnd}
var codeEnd = &wasm.Code{Body: end}

// begin is a tokenParser that starts after a type use.
//
// The onFunc field is invoked once any instructions are written into currentBody.
//
// Ex. Given the source `(module (func nop))`
//                 begin starts here --^  ^
//                    calls onFunc here --+
func (p *funcParser) begin(typeIdx wasm.Index, paramNames wasm.NameMap, pos onTypeUsePosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	switch pos {
	case onTypeUseEndField:
		return p.onFunc(typeIdx, codeEnd, paramNames)
	case onTypeUseUnhandledField:
		return sExpressionsUnsupported(tok, tokenBytes, line, col)
	}

	p.currentBody = nil
	p.currentTypeIdx = typeIdx
	p.currentParamNames = paramNames
	return p.beginFieldOrInstruction(tok, tokenBytes, line, col)
}

func sExpressionsUnsupported(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, unexpectedToken(tok, tokenBytes)
	}
	switch string(tokenBytes) {
	case "result", "param": // TODO: local
		return nil, fmt.Errorf("%s declared out of order", tokenBytes)
	}
	return nil, fmt.Errorf("TODO: s-expressions are not yet supported: %s", tokenBytes)
}

func (p *funcParser) beginFieldOrInstruction(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	switch tok {
	case tokenLParen:
		return sExpressionsUnsupported, nil
	case tokenRParen:
		return p.end()
	case tokenKeyword:
		return p.beginInstruction(tokenBytes)
	}
	return nil, unexpectedToken(tok, tokenBytes)
}

// beginInstruction parses the token into an opcode and dispatches accordingly. If there are none, this calls onFunc.
func (p *funcParser) beginInstruction(tokenBytes []byte) (tokenParser, error) {
	switch string(tokenBytes) {
	case "local.get":
		p.currentOpcode = wasm.OpcodeLocalGet
		return p.parseInstructionIndex, nil
	case "i32.add":
		p.currentOpcode = wasm.OpcodeI32Add
		return p.endInstruction()
	}
	return nil, fmt.Errorf("unsupported instruction: %s", tokenBytes)
}

// end invokes onFunc to continue parsing
func (p *funcParser) end() (tokenParser, error) {
	var code *wasm.Code
	if p.currentBody == nil {
		code = codeEnd
	} else {
		code = &wasm.Code{Body: append(p.currentBody, wasm.OpcodeEnd)}
	}
	return p.onFunc(p.currentTypeIdx, code, p.currentParamNames)
}

// TODO: port this to the indexNamespace
func (p *funcParser) parseInstructionIndex(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenUN: // Ex. 1
		i, overflow := decodeUint32(tokenBytes)
		if overflow {
			return nil, fmt.Errorf("index outside range of uint32: %s", tokenBytes)
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
		return nil, errors.New("TODO: index variables are not yet supported")
	}
	return nil, unexpectedToken(tok, tokenBytes)
}

func (p *funcParser) endInstruction() (tokenParser, error) {
	p.currentBody = append(p.currentBody, p.currentOpcode)
	p.currentBody = append(p.currentBody, p.currentParameters...)
	p.currentParameters = nil
	return p.beginFieldOrInstruction, nil
}
