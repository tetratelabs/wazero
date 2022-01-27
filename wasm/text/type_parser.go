package text

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/internal/leb128"
)

// typeParsingState is used to give an appropriate typeParser.errorContext
type typeParsingState byte

const (
	parsingTypeUse typeParsingState = iota
	parsingType
	parsingParamOrResult
	parsingParam
	parsingResult
	parsingComplete
)

// typeParser parses an inlined type from a field such as "type" or "func" and dispatches to onTypeEnd.
//
// Ex. `(import "Math" "PI" (func $math.pi (result f32))`
//                           starts here --^           ^
//                            onTypeEnd resumes here --+
//
// Ex. `(type (func (param i32) (result i64)))`
//    starts here --^                       ^
//                 onTypeEnd resumes here --+
//
// Ex. `(module (import "" "" (func $main)))`
//                calls onTypeEnd here --^
//
// Note: Unlike normal parsers, this is not used for an entire field (enclosed by parens). Rather, this only handles
// "param" and "result" inner fields in the correct order.
// Note: typeParser is reusable. The caller resets when reaching the appropriate tokenRParen via reset.
type typeParser struct {
	// m is used as a function pointer to moduleParser.tokenParser. This updates based on state changes.
	m *moduleParser

	// onTypeEnd is invoked when the grammar "(param)* (result)?" completes.
	//
	// Note: this is called when neither a "param" nor a "result" field are found, or on any field following a "param"
	// that is not a "result".
	onTypeEnd tokenParser

	// onUnknownField is invoked when the grammar "(param)* (result)?" completes with a field name (tokenID)
	onUnknownField tokenParser

	// state is initially parsingParamOrResult and updated alongside tokenParser
	state typeParsingState

	// inlinedTypes are a collection of types currently known to be inlined.
	// Ex. `(param i32)` in `(import (func (param i32)))`
	//
	// Note: The Text Format requires imports first, not types first. This resolution has to be done later. The impact
	// of this is types here can be removed if later discovered to be an explicitly defined type.
	//
	// For example, here the `(param i32)` type is initially considered inlined until the module type with the same
	// signature is read later: (module (import (func (param i32))) (type (func (param i32))))`
	inlinedTypes []*wasm.FunctionType

	// currentTypeIndex is set when there's a "type" field in a type use
	// See https://www.w3.org/TR/wasm-core-1/#type-uses%E2%91%A0
	currentTypeIndex *index

	// currentParams allow us to accumulate wasm.FunctionType Params across multiple fields, as well support abbreviated
	// anonymous parameters. ex. both (param i32) (param i32) and (param i32 i32) formats.
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A2
	currentParams []wasm.ValueType

	// paramIDContext accumulates any symbolic identifier to numeric index mappings for the currentParams.
	// Ex. x is the symbolic ID, and name, of the parameter (param $x i32)
	//
	// Note: IDs can be missing because they were never assigned, ex. (param i32), or due to abbreviated format which
	// does not support it. Ex. (param i32 i32)
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A2
	paramIDContext idContext

	// paramNames are the paramIDContext formatted for the wasm.NameSection LocalNames
	paramNames wasm.NameMap

	// currentParamField is a field index and used to give an appropriate errorContext. Due to abbreviation it may be
	// unrelated to the length of currentParams
	currentParamField wasm.Index

	// onParamID is ignoreParamID when beginType and setParamID when beginTypeUse
	onParamID onParamID

	// foundParam allows us to check if we found a type in a "param" field. We can't use currentParamField because when
	// parameters are abbreviated, ex. (param i32 i32), the currentParamField will be less than the type count.
	foundParam bool

	// foundID is true when the field at currentParamField had an ID. Ex. (param $x i32)
	foundID bool

	// currentResults is empty until set, and only set once as WebAssembly 1.0 only supports up to one result.
	currentResults []wasm.ValueType

	// currentTypeUseStartLine tracks the start column of a type use in case there's an error later
	currentTypeUseStartLine uint32

	// currentTypeUseStartCol tracks the start column of a type use in case there's an error later
	currentTypeUseStartCol uint32
}

// beginTypeUse sets the next parser to beginTypeParamOrResult. reset must be called prior to this.
// This should only be called when reaching the first tokenLParen after the optional field name (tokenID).
//
// Ex. Given the source `(module (import (func $main (param i32))))`
//              beginTypeParamOrResult starts here --^          ^
//                                     onTypeEnd resumes here --+
//
// Ex. Given the source `(module (func $main (result i32) (local.get 0))`
//      beginTypeParamOrResult starts here --^             ^
//                           onUnknownField resumes here --+
//
// The onTypeEnd parameter is invoked once all fields have been consumed.
// The onUnknownField parameter is invoked on tokenKeyword after any "type" "param" or "result" fields.
//
// NOTE: An empty function is valid and will not reach a tokenLParen! Ex. `(module (import (func)))`
func (p *typeParser) beginTypeUse(onTypeEnd, onUnknownField tokenParser) {
	p.onParamID = setParamID
	p.onTypeEnd = onTypeEnd
	p.onUnknownField = onUnknownField
	p.state = parsingTypeUse
	p.m.tokenParser = p.beginTypeParamOrResult
}

// beginTypeParamOrResult is a tokenParser called after a tokenLParen and accepts a "type", "param" or a "result" field
// (tokenKeyword).
func (p *typeParser) beginTypeParamOrResult(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenKeyword && string(tokenBytes) == "type" {
		// If we see a "type" field, there's a chance there's an inlined type following it. We record the position, as
		// we can't verify the signature until all types are read. If there's a signature mismatch later, we need to
		// know where in the source it was wrong!
		p.currentTypeUseStartLine = line
		p.currentTypeUseStartCol = col
		p.state = parsingType
		p.m.tokenParser = p.m.indexParser.beginParsingIndex(p.parseTypeIndexEnd)
		return nil
	}
	p.state = parsingParamOrResult
	return p.beginParamOrResult(tok, tokenBytes, line, col)
}

// beginType sets the next parser to parseMoreParamsOrResult. reset must be called prior to this.
//
// Ex. Given the source `(module (type (func (param i32))))`
//        parsingParamOrResult starts here --^          ^
//                             onTypeEnd resumes here --+
//
// The onTypeEnd parameter is invoked once any "param" and "result" fields have been consumed.
//
// NOTE: An empty function is valid and will not reach a tokenLParen! Ex. `(module (type (func)))`
func (p *typeParser) beginType(onTypeEnd tokenParser) {
	p.onParamID = ignoreParamID
	p.onTypeEnd = onTypeEnd
	p.onUnknownField = onTypeEnd
	p.state = parsingParamOrResult
	p.m.tokenParser = p.beginParamOrResult
}

func (p *typeParser) reset() {
	p.currentTypeIndex = nil
	p.currentParams = nil
	if len(p.paramIDContext) > 0 {
		p.paramIDContext = idContext{}
		p.paramNames = nil
	}
	p.currentParamField = 0
	p.currentResults = nil
}

func (p *typeParser) parseTypeIndexEnd(index *index) {
	p.currentTypeIndex = index
	p.state = parsingParamOrResult // because a type field can be followed by its signature
	p.m.tokenParser = p.parseMoreParamsOrResult
}

// beginParamOrResult is a tokenParser called after a tokenLParen and accepts either a "param" or a "result" field
// (tokenKeyword).
func (p *typeParser) beginParamOrResult(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenKeyword {
		switch string(tokenBytes) {
		case "param":
			p.state = parsingParam
			p.foundParam, p.foundID = false, false
			p.m.tokenParser = p.parseParamID
			return nil
		case "result":
			p.state = parsingResult
			p.m.tokenParser = p.parseResult
			return nil
		case "type": // cannot repeat
			return errors.New("redundant type")
		default:
			return p.onUnknownField(tok, tokenBytes, line, col)
		}
	}
	// If we reach here, it is a syntax error, so punt it to onTypeEnd. Ex. (func ($param i32))
	return p.onTypeEnd(tok, tokenBytes, line, col)
}

func (p *typeParser) parseMoreParamsOrResult(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenLParen {
		p.state = parsingParamOrResult
		p.m.tokenParser = p.beginParamOrResult
		return nil
	}
	// If we reached this point, we have one or more parameters, but no result. Ex. (func (param i32)) or (func)
	return p.onTypeEnd(tok, tokenBytes, line, col)
}

// parseParamID is the first parser inside a param field. This calls onParamID with the ID if present or parseParam if
// not found.
//
// Ex. A param ID is present `(param $x i32)`
//                 calls onParamID --^  ^
//            parseParam resumes here --+
//
// Ex. No param ID `(param i32)`
//      calls parseParam --^
func (p *typeParser) parseParamID(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenID { // Ex. $len
		p.foundID = true
		p.m.tokenParser = p.parseParam
		return p.onParamID(p, tokenBytes)
	}
	return p.parseParam(tok, tokenBytes, line, col)
}

// onParamID handles a tokenID in a param field
type onParamID func(p *typeParser, idToken []byte) error

// ignoreParamID is used for module.types, where parameter IDs are allowed to be present, but needn't be validated, and
// serve no purpose.
//
// See https://github.com/WebAssembly/spec/issues/1411
func ignoreParamID(_ *typeParser, _ []byte) error {
	return nil
}

// setParamID adds the normalized ('$' stripped) parameter ID to the paramIDContext and the wasm.NameSection.
func setParamID(p *typeParser, idToken []byte) error {
	// Note: currentParamField is the index of the param field, but due to mixing and matching of abbreviated params
	// it can be less than the param index. Ex. (param i32 i32) (param $v i32) is param field 2, but the 3rd param.
	idx := wasm.Index(len(p.currentParams))
	id, err := p.paramIDContext.setID(idToken, idx)
	if err != nil {
		return err
	}
	p.paramNames = append(p.paramNames, &wasm.NameAssoc{Index: idx, Name: id})
	return nil
}

// parseParam is the last parser inside the param field. This records value type and continues if it is an abbreviated
// form with multiple value types. When complete, this sets the next parser to parseMoreParamsOrResult.
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
func (p *typeParser) parseParam(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID: // Ex. $len
		return fmt.Errorf("redundant ID %s", tokenBytes)
	case tokenKeyword: // Ex. i32
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return err
		}
		if p.foundParam && p.foundID {
			return errors.New("cannot assign IDs to parameters in abbreviated form")
		}
		p.currentParams = append(p.currentParams, vt)
		p.foundParam = true
	case tokenRParen: // end of this field
		if !p.foundParam {
			return errors.New("expected a type")
		}
		// since multiple param fields are valid, ex `(func (param i32) (param i64))`, prepare for any next.
		p.currentParamField++
		p.state = parsingParamOrResult
		p.m.tokenParser = p.parseMoreParamsOrResult
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

// parseResult is a tokenParser inside a "result" field (tokenKeyword). When this field completes (tokenRParen), control
// transitions to parseComplete.
func (p *typeParser) parseResult(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenKeyword: // Ex. i32
		if p.currentResults != nil {
			return errors.New("redundant type")
		}
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return err
		}
		p.currentResults = leb128.EncodeUint32(uint32(vt)) // reuse cache
	case tokenRParen: // end of this field
		if p.currentResults == nil {
			return errors.New("expected a type")
		}
		p.m.tokenParser = p.onTypeEnd
		p.state = parsingComplete
	default:
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *typeParser) errorContext() string {
	switch p.state {
	case parsingParam:
		return fmt.Sprintf(".param[%d]", p.currentParamField)
	case parsingResult:
		return ".result"
	case parsingType:
		return ".type"
	}
	return ""
}

var typeFuncEmpty = &wasm.FunctionType{}

// getTypeUse finalizes any current params or result and returns the current typeIndex and/or type. localNames are only
// returned if defined inline.
func (p *typeParser) getTypeUse() (ty *typeUse, paramIDs map[string]wasm.Index, paramNames wasm.NameMap) {
	ty = &typeUse{typeIndex: p.currentTypeIndex}
	if len(p.paramIDContext) > 0 {
		paramIDs = p.paramIDContext
		paramNames = p.paramNames
	}

	// Don't conflate lack of verification type with nullary
	if ty.typeIndex != nil && funcTypeEquals(typeFuncEmpty, p.currentParams, p.currentResults) {
		return
	}

	// Search for an existing signature that matches the current type in the module types.
	for _, t := range p.m.module.types {
		if funcTypeEquals(t, p.currentParams, p.currentResults) {
			ty.typeInlined = &inlinedTypeFunc{t, p.currentTypeUseStartLine, p.currentTypeUseStartCol}
			return
		}
	}

	// Search for an existing signature that matches the current type in the pending inlined types
	for _, t := range p.inlinedTypes {
		if funcTypeEquals(t, p.currentParams, p.currentResults) {
			ty.typeInlined = &inlinedTypeFunc{t, p.currentTypeUseStartLine, p.currentTypeUseStartCol}
			return
		}
	}

	ty.typeInlined = &inlinedTypeFunc{
		typeFunc: &wasm.FunctionType{Params: p.currentParams, Results: p.currentResults},
		line:     p.currentTypeUseStartLine,
		col:      p.currentTypeUseStartCol,
	}

	// If we didn't find a match, we need to insert an inlined type to use it. We don't do this when there is a type
	// index because in this case, the signature is only used for verification on an existing type.
	if ty.typeIndex == nil {
		p.inlinedTypes = append(p.inlinedTypes, ty.typeInlined.typeFunc)
	}
	return
}

func funcTypeEquals(f *wasm.FunctionType, params []wasm.ValueType, results []wasm.ValueType) bool {
	return bytes.Equal(f.Params, params) && bytes.Equal(f.Results, results)
}

// getType finalizes any current params or result and returns the current type and any paramNames for it.
//
// If the current type is in typeParser.inlinedTypes, it is removed prior to returning.
func (p *typeParser) getType() (sig *wasm.FunctionType) {
	// Search inlined types in case a matching type was found after its type use.
	for i, t := range p.inlinedTypes {
		if funcTypeEquals(t, p.currentParams, p.currentResults) {
			// If we got here, we found a type field after a type use. This means it wasn't an inlined type, rather an
			// out-of-order type. Hence, remove it from the inlined types and add it to the module types.
			p.inlinedTypes = append(p.inlinedTypes[:i], p.inlinedTypes[i+1:]...)
			sig = t
			break
		}
	}

	// While inlined types are supposed to re-use an existing type index, there's no no unique constraint on explicitly
	// defined module types. This means a duplicate type is not a bug: we don't check module.types first.
	if sig == nil {
		sig = &wasm.FunctionType{Params: p.currentParams, Results: p.currentResults}
	}
	return
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
