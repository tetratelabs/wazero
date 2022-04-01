package text

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func newTypeParser(typeNamespace *indexNamespace, onType onType) *typeParser {
	return &typeParser{typeNamespace: typeNamespace, onType: onType}
}

type onType func(ft *wasm.FunctionType) tokenParser

// typeParser parses a wasm.Type from and dispatches to onType.
//
// Ex. `(module (type (func (param i32) (result i64)))`
//      starts here --^                             ^
//                         onType resumes here --+
//
// Note: typeParser is reusable. The caller resets via begin.
type typeParser struct {
	typeNamespace *indexNamespace

	// onType is invoked on end
	onType onType

	// pos is used to give an appropriate errorContext
	pos parserPosition

	// currentType is reset on begin and complete onType
	currentType *wasm.FunctionType

	// currentParamField is a field index and used to give an appropriate errorContext. Due to abbreviation it may be
	// unrelated to the length of currentParams
	currentParamField wasm.Index

	// parsedParam allows us to check if we parsed a type in a "param" field. We can't use currentParamField because
	// when parameters are abbreviated, ex. (param i32 i32), the currentParamField will be less than the type count.
	parsedParam bool

	// parsedParamID is true when the field at currentParamField had an ID. Ex. (param $x i32)
	//
	// Note: param IDs are allowed to be present on module types, but they serve no purpose. parsedParamID is only used
	// to validate the grammar rules: ID validation is not necessary.
	//
	// See https://github.com/WebAssembly/spec/issues/1411
	parsedParamID bool
}

// begin should be called after reaching the "type" keyword in a module field. Parsing continues until onType or error.
//
// This stage records the ID of the current type, if present, and resumes with tryBeginFunc.
//
// Ex. A type ID is present `(type $t0 (func (result i32)))`
//                    records t0 --^   ^
//         tryBeginFunc resumes here --+
//
// Ex. No type ID `(type (func (result i32)))`
//  calls tryBeginFunc --^
func (p *typeParser) begin(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	p.currentType = &wasm.FunctionType{}
	if tok == tokenID { // Ex. $v_v
		if _, err := p.typeNamespace.setID(tokenBytes); err != nil {
			return nil, err
		}
		return p.tryBeginFunc, nil
	}
	return p.tryBeginFunc(tok, tokenBytes, line, col)
}

// tryBeginFunc begins a field on '(' by returning beginFunc, or errs on any other token.
func (p *typeParser) tryBeginFunc(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenID: // Ex.(type $rf32 $rf32
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	case tokenLParen:
		return p.beginFunc, nil
	case tokenRParen: // end of this type
		return nil, errors.New("missing func field") // Ex. (type)
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// beginFunc returns a parser according to the type field name (tokenKeyword), or errs if invalid.
func (p *typeParser) beginFunc(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, expectedField(tok)
	}

	if string(tokenBytes) != wasm.ExternTypeFuncName {
		return nil, unexpectedFieldName(tokenBytes)
	}

	p.pos = positionFunc
	return p.parseFunc, nil
}

// parseFunc passes control to the typeParser until any signature is read, then returns parseFuncEnd.
//
// Ex. `(module (type $rf32 (func (result f32))))`
//            starts here --^                 ^
//                parseFuncEnd resumes here --+
//
// Ex. If there is no signature `(module (type $rf32 ))`
//                    calls parseFuncEnd here ---^
func (p *typeParser) parseFunc(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	switch tok {
	case tokenLParen:
		return p.beginParamOrResult, nil // start fields, ex. (param or (result
	case tokenRParen: //empty
		return p.parseFuncEnd(tok, tokenBytes, line, col)
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseFuncEnd completes the wasm.ExternTypeFuncName field and returns end
func (p *typeParser) parseFuncEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}
	p.pos = positionInitial
	return p.end, nil
}

// end increments the type namespace and calls onType with the current type
func (p *typeParser) end(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenRParen {
		return nil, unexpectedToken(tok, tokenBytes)
	}
	p.typeNamespace.count++
	return p.onType(p.currentType), nil
}

// beginParamOrResult decides which tokenParser to use based on its field name: "param" or "result".
func (p *typeParser) beginParamOrResult(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	switch string(tokenBytes) {
	case "param":
		p.pos = positionParam
		p.parsedParam, p.parsedParamID = false, false
		return p.parseParamID, nil
	case "result":
		p.pos = positionResult
		return p.parseResult, nil
	default:
		return nil, unexpectedFieldName(tokenBytes)
	}
}

// parseMoreParamsOrResult looks for a '(', and if present returns beginParamOrResult to continue the type. Otherwise,
// it calls onType.
func (p *typeParser) parseMoreParamsOrResult(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenLParen {
		p.pos = positionFunc
		return p.beginParamOrResult, nil
	}
	return p.parseFuncEnd(tok, tokenBytes, line, col) // end of params, but no result. Ex. (func (param i32)) or (func)
}

// parseParamID ignores any ID if present and resumes with parseParam .
//
// Ex. A param ID is present `(param $x i32)`
//                                      ^
//            parseParam resumes here --+
//
// Ex. No param ID `(param i32)`
//      calls parseParam --^
func (p *typeParser) parseParamID(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $len
		p.parsedParamID = true
		return p.parseParam, nil
	}
	return p.parseParam(tok, tokenBytes, line, col)
}

// parseParam records value type and continues if it is an abbreviated form with multiple value types. When complete,
// this returns parseMoreParamsOrResult.
//
// Ex. One param type is present `(param i32)`
//                         records i32 --^  ^
//   parseMoreParamsOrResult resumes here --+
//
// Ex. One param type is present `(param i32)`
//                         records i32 --^  ^
//   parseMoreParamsOrResult resumes here --+
//
// Ex. type is missing `(param)`
//                errs here --^
func (p *typeParser) parseParam(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenID: // Ex. $len
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	case tokenKeyword: // Ex. i32
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return nil, err
		}
		if p.parsedParam && p.parsedParamID {
			return nil, errors.New("cannot assign IDs to parameters in abbreviated form")
		}
		p.currentType.Params = append(p.currentType.Params, vt)
		p.parsedParam = true
		return p.parseParam, nil
	case tokenRParen: // end of this field
		if !p.parsedParam {
			return nil, errors.New("expected a type")
		}
		// since multiple param fields are valid, ex `(func (param i32) (param i64))`, prepare for any next.
		p.currentParamField++
		p.pos = positionFunc
		return p.parseMoreParamsOrResult, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseResult parses the api.ValueType in the "result" field and returns onType to finish the type.
func (p *typeParser) parseResult(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenKeyword: // Ex. i32
		if p.currentType.Results != nil {
			return nil, errors.New("redundant type")
		}

		var err error
		p.currentType.Results, err = parseResultType(tokenBytes)
		return p.parseResult, err
	case tokenRParen: // end of this field
		if p.currentType.Results == nil {
			return nil, errors.New("expected a type")
		}
		p.pos = positionFunc
		return p.parseFuncEnd, nil // end of result, and only one is allowed
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

func parseResultType(tokenBytes []byte) ([]wasm.ValueType, error) {
	vt, err := parseValueType(tokenBytes)
	if err != nil {
		return nil, err
	}
	return leb128.EncodeUint32(uint32(vt)), nil // reuse cache
}

func (p *typeParser) errorContext() string {
	switch p.pos {
	case positionFunc:
		return ".func"
	case positionParam:
		return fmt.Sprintf(".func.param[%d]", p.currentParamField)
	case positionResult:
		return ".func.result"
	}
	return ""
}

func parseValueType(tokenBytes []byte) (wasm.ValueType, error) {
	t := string(tokenBytes)
	switch t {
	case "i32":
		return wasm.ValueTypeI32, nil
	case "i64":
		return wasm.ValueTypeI64, nil
	case "f32":
		return wasm.ValueTypeF32, nil
	case "f64":
		return wasm.ValueTypeF64, nil
	default:
		return 0, fmt.Errorf("unknown type: %s", t)
	}
}
