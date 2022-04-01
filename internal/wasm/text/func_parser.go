package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func newFuncParser(enabledFeatures wasm.Features, typeUseParser *typeUseParser, funcNamespace *indexNamespace, onFunc onFunc) *funcParser {
	return &funcParser{enabledFeatures: enabledFeatures, typeUseParser: typeUseParser, funcNamespace: funcNamespace, onFunc: onFunc}
}

type onFunc func(typeIdx wasm.Index, code *wasm.Code, name string, localNames wasm.NameMap) (tokenParser, error)

// funcParser parses any instructions and dispatches to onFunc.
//
// Ex.  `(module (func (nop)))`
//        begin here --^    ^
//  end calls onFunc here --+
//
// Note: funcParser is reusable. The caller resets via begin.
type funcParser struct {
	// enabledFeatures should be set to moduleParser.enabledFeatures
	enabledFeatures wasm.Features

	// onFunc is called when complete parsing the body. Unless testing, this should be moduleParser.onFuncEnd
	onFunc onFunc

	// typeUseParser is described by moduleParser.typeUseParser
	typeUseParser *typeUseParser

	// funcNamespace is described by moduleParser.funcNamespace
	funcNamespace *indexNamespace

	currentName       string
	currentTypeIdx    wasm.Index
	currentParamNames wasm.NameMap

	// currentBody is the current function body encoded in WebAssembly 1.0 (20191205) binary format
	currentBody []byte
}

// end indicates the end of instructions in this function body
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#expressions%E2%91%A0
var end = []byte{wasm.OpcodeEnd}
var codeEnd = &wasm.Code{Body: end}

// begin should be called after reaching the wasm.ExternTypeFuncName keyword in a module field. Parsing
// continues until onFunc or error.
//
// This stage records the ID of the current function, if present, and resumes with onFunc.
//
// Ex. A func ID is present `(func $main nop)`
//                  records main --^     ^
//              parseFunc resumes here --+
//
// Ex. No func ID `(func nop)`
//     calls parseFunc --^
func (p *funcParser) begin(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $main
		if id, err := p.funcNamespace.setID(tokenBytes); err != nil {
			return nil, err
		} else {
			p.currentName = id
		}
		return p.parseFunc, nil
	}
	p.currentName = ""
	return p.parseFunc(tok, tokenBytes, line, col)
}

// parseFunc passes control to the typeUseParser until any signature is read, then funcParser until and locals or body
// are read. Finally, this finishes via endFunc.
//
// Ex. `(module (func $math.pi (result f32))`
//                begin here --^           ^
//                  endFunc resumes here --+
//
// Ex.    `(module (func $math.pi (result f32) (local i32) )`
//                   begin here --^            ^           ^
//      funcParser.afterTypeUse resumes here --+           |
//                                  endFunc resumes here --+
//
// Ex. If there is no signature `(func)`
//              calls endFunc here ---^
func (p *funcParser) parseFunc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. (func $main $main)
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	}

	return p.typeUseParser.begin(wasm.SectionIDFunction, p.afterTypeUse, tok, tokenBytes, line, col)
}

// afterTypeUse is a tokenParser that starts after a type use.
//
// The onFunc field is invoked once any instructions are written into currentBody.
//
// Ex. Given the source `(module (func nop))`
//          afterTypeUse starts here --^  ^
//                    calls onFunc here --+
func (p *funcParser) afterTypeUse(typeIdx wasm.Index, paramNames wasm.NameMap, pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	switch pos {
	case callbackPositionEndField:
		return p.onFunc(typeIdx, codeEnd, p.currentName, paramNames)
	case callbackPositionUnhandledField:
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
	case "param":
		return nil, errors.New("param after result")
	case "result":
		return nil, moreThanOneInvalid("result")
	case "local":
		return nil, errors.New("TODO: local")
	}
	return nil, fmt.Errorf("TODO: s-expressions are not yet supported: %s", tokenBytes)
}

func (p *funcParser) beginFieldOrInstruction(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
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
func (p *funcParser) beginInstruction(tokenBytes []byte) (next tokenParser, err error) {
	var opCode wasm.Opcode
	switch string(tokenBytes) {
	case "local.get": // See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefsyntax-instr-variablemathsflocalgetx%E2%91%A0
		opCode = wasm.OpcodeLocalGet
		next = p.parseLocalIndex
	case "i32.add": // See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#syntax-instr-numeric
		opCode = wasm.OpcodeI32Add
		next = p.beginFieldOrInstruction
	case "call": // See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#-hrefsyntax-instr-controlmathsfcallx
		opCode = wasm.OpcodeCall
		next = p.parseFuncIndex
	case "i32.extend8_s": // See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
		opCode = wasm.OpcodeI32Extend8S
		next = p.beginFieldOrInstruction
	case "i32.extend16_s": // See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
		opCode = wasm.OpcodeI32Extend16S
		next = p.beginFieldOrInstruction
	case "i64.extend8_s": // See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
		opCode = wasm.OpcodeI64Extend8S
		next = p.beginFieldOrInstruction
	case "i64.extend16_s": // See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
		opCode = wasm.OpcodeI64Extend16S
		next = p.beginFieldOrInstruction
	case "i64.extend32_s": // See https://github.com/WebAssembly/spec/blob/main/proposals/sign-extension-ops/Overview.md
		opCode = wasm.OpcodeI64Extend32S
		next = p.beginFieldOrInstruction
	default:
		return nil, fmt.Errorf("unsupported instruction: %s", tokenBytes)
	}

	// Guard >1.0 feature sign-extension-ops
	if opCode >= wasm.OpcodeI32Extend8S && opCode <= wasm.OpcodeI64Extend32S {
		if err = p.enabledFeatures.Require(wasm.FeatureSignExtensionOps); err != nil {
			return nil, fmt.Errorf("%s invalid as %v", tokenBytes, err)
		}
	}

	p.currentBody = append(p.currentBody, opCode)
	return next, nil
}

// end invokes onFunc to continue parsing
func (p *funcParser) end() (tokenParser, error) {
	var code *wasm.Code
	if p.currentBody == nil {
		code = codeEnd
	} else {
		code = &wasm.Code{Body: append(p.currentBody, wasm.OpcodeEnd)}
	}
	return p.onFunc(p.currentTypeIdx, code, p.currentName, p.currentParamNames)
}

// parseFuncIndex parses an index in the function namespace and appends it to the currentBody. If it was an ID, a
// placeholder byte(0) is added instead and will be resolved later.
func (p *funcParser) parseFuncIndex(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	bodyOffset := uint32(len(p.currentBody))
	idx, resolved, err := p.funcNamespace.parseIndex(wasm.SectionIDCode, bodyOffset, tok, tokenBytes, line, col)
	if err != nil {
		return nil, err
	}
	if !resolved && tok == tokenID {
		p.currentBody = append(p.currentBody, 0) // will be replaced later
	} else {
		p.currentBody = append(p.currentBody, leb128.EncodeUint32(idx)...)
	}
	return p.beginFieldOrInstruction, nil
}

// TODO: port this to the indexNamespace
func (p *funcParser) parseLocalIndex(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenUN: // Ex. 1
		i, overflow := decodeUint32(tokenBytes)
		if overflow {
			return nil, fmt.Errorf("index outside range of uint32: %s", tokenBytes)
		}
		p.currentBody = append(p.currentBody, leb128.EncodeUint32(i)...)

		// TODO: it is possible this is out of range in the index. Local out-of-range is caught in Store.Initialize, but
		// we can provide a better area since we can access the line and col info. However, since the local index starts
		// with parameters and they are on the type, and that type may be defined after the function, it can get hairy.
		// To handle this neatly means doing a quick check to see if the type is already present and immediately
		// validate. If the type isn't yet present, we can save off the context for late validation, noting that we
		// should probably save off the instruction count or at least the current opcode to help with the error message.
		return p.beginFieldOrInstruction, nil
	case tokenID: // Ex $y
		return nil, errors.New("TODO: index variables are not yet supported")
	}
	return nil, unexpectedToken(tok, tokenBytes)
}
