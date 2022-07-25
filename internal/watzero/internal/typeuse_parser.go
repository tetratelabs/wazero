package internal

import (
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/internal/wasm"
)

func newTypeUseParser(enabledFeatures wasm.Features, module *wasm.Module, typeNamespace *indexNamespace) *typeUseParser {
	return &typeUseParser{enabledFeatures: enabledFeatures, module: module, typeNamespace: typeNamespace}
}

// onTypeUse is invoked when the grammar "(param)* (result)*" completes.
//
// * typeIdx if unresolved, this is replaced in moduleParser.resolveTypeUses
// * paramNames is nil unless IDs existed on at least one "param" field.
// * pos is the context used to determine which tokenParser to return
//
// Note: this is called when neither a "param" nor a "result" field are parsed, or on any field following a "param"
// that is not a "result": pos clarifies this.
type onTypeUse func(typeIdx wasm.Index, paramNames wasm.NameMap, pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error)

// typeUseParser parses an inlined type from a field such wasm.ExternTypeFuncName and calls onTypeUse.
//
// Ex. `(import "Math" "PI" (func $math.pi (result f32)))`
//
//	starts here --^           ^
//	 onTypeUse resumes here --+
//
// Note: Unlike normal parsers, this is not used for an entire field (enclosed by parens). Rather, this only handles
// "type", "param" and "result" inner fields in the correct order.
// Note: typeUseParser is reusable. The caller resets via begin.
type typeUseParser struct {
	// enabledFeatures should be set to moduleParser.enabledFeatures
	enabledFeatures wasm.Features

	// module during parsing is a read-only pointer to the TypeSection and SectionElementCount
	module *wasm.Module

	typeNamespace *indexNamespace

	section wasm.SectionID

	// inlinedTypes are anonymous types defined by signature, which at the time of definition didn't match a
	// module-defined type. Ex. `(param i32)` in `(import (func (param i32)))`
	//
	// Note: The Text Format requires imports first, not types first. This resolution has to be done later. The impact
	// of this is types here can be ignored if later discovered to be an explicitly defined type.
	//
	// For example, here the `(param i32)` type is doesn't match a module type with the same signature until later:
	//	(module (import (func (param i32))) (type (func (param i32))))`
	inlinedTypes []*wasm.FunctionType

	inlinedTypeIndices []*inlinedTypeIndex

	// onTypeUse is invoked on end
	onTypeUse onTypeUse

	// pos is used to give an appropriate errorContext
	pos parserPosition

	// parsedTypeField is set when there is a "type" field in the current type use.
	parsedTypeField bool

	// currentTypeIndex should be read when parsedTypeField is true
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#type-uses%E2%91%A0
	currentTypeIndex wasm.Index

	// currentTypeIndexUnresolved is set when the currentTypeIndex was not in the api.Module TypeSection
	currentTypeIndexUnresolved *lineCol

	// currentInlinedType is reset on begin and complete onTypeUse
	currentInlinedType *wasm.FunctionType

	// paramIndex enforce the uniqueness constraint on param ID
	paramIndex map[string]struct{}

	// paramNames are the paramIndex formatted for the wasm.NameSection LocalNames
	paramNames wasm.NameMap

	// currentField is a field index and used to give an appropriate errorContext.
	//
	// Note: Due to abbreviation, this may be less than to the length of params or results.
	currentField wasm.Index

	// parsedParamType allows us to check if we parsed a type in a "param" field. This is used to enforce param names
	// can't coexist with abbreviations.
	parsedParamType bool

	// parsedParamID is true when the field at currentField had an ID. Ex. (param $x i32)
	parsedParamID bool
}

// begin should be called after reading any ID in a field that contains a type use. Parsing starts with the returned
// beginTypeParamOrResult and continues until onTypeUse or error. This should be called regardless of the tokenType
// to ensure a valid empty type use is associated with the section index, if needed.
//
// Ex. Given the source `(module (import (func $main (param i32))))`
//
//	beginTypeParamOrResult starts here --^          ^
//	                       onTypeUse resumes here --+
//
// Ex. Given the source `(module (func $main (result i32) (local.get 0))`
//
//	beginTypeParamOrResult starts here --^             ^
//	                          onTypeUse resumes here --+
func (p *typeUseParser) begin(section wasm.SectionID, onTypeUse onTypeUse, tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	pos := callbackPositionUnhandledToken
	p.pos = positionInitial // to ensure errorContext reports properly
	switch tok {
	case tokenLParen:
		p.section = section
		p.onTypeUse = onTypeUse
		return p.beginTypeParamOrResult, nil
	case tokenRParen:
		pos = callbackPositionEndField
	}
	return onTypeUse(p.emptyTypeIndex(section), nil, pos, tok, tokenBytes, line, col)
}

// v_v is a nullary function type (void -> void)
var v_v = &wasm.FunctionType{}

// inlinedTypeIndex searches for any existing empty type to re-use
func (p *typeUseParser) emptyTypeIndex(section wasm.SectionID) wasm.Index {
	for i, t := range p.module.TypeSection {
		if t == v_v {
			return wasm.Index(i)
		}
	}

	foundEmpty := false
	var inlinedIdx wasm.Index
	for i, t := range p.inlinedTypes {
		if t == v_v {
			foundEmpty = true
			inlinedIdx = wasm.Index(i)
		}
	}

	if !foundEmpty {
		inlinedIdx = wasm.Index(len(p.inlinedTypes))
		p.inlinedTypes = append(p.inlinedTypes, v_v)
	}

	// typePos is not needed on empty as there's nothing to verify
	idx := p.module.SectionElementCount(section)
	i := &inlinedTypeIndex{section: section, idx: idx, inlinedIdx: inlinedIdx, typePos: nil}
	p.inlinedTypeIndices = append(p.inlinedTypeIndices, i)
	return 0 // substitute index that will be replaced later
}

// beginTypeParamOrResult decides which tokenParser to use based on its field name: "type", "param" or "result".
func (p *typeUseParser) beginTypeParamOrResult(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	p.currentInlinedType = nil
	if len(p.paramIndex) > 0 {
		p.paramIndex = nil
		p.paramNames = nil
	}
	p.currentField = 0
	p.parsedTypeField = false
	if tok == tokenKeyword && string(tokenBytes) == "type" {
		p.pos = positionType
		p.parsedTypeField = true
		return p.parseType, nil
	}
	p.pos = positionInitial
	return p.beginParamOrResult(tok, tokenBytes, line, col)
}

// parseType parses a type index inside the "type" field. If not yet in the TypeSection, the position is recorded for
// resolution later. Finally, this returns parseTypeEnd to finish the field.
func (p *typeUseParser) parseType(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	idx, resolved, err := p.typeNamespace.parseIndex(p.section, 0, tok, tokenBytes, line, col)
	if err != nil {
		return nil, err
	}

	p.currentTypeIndex = idx
	if !resolved { // save the source position in case there's an inlined use after this field and it doesn't match.
		p.currentTypeIndexUnresolved = &lineCol{line: line, col: col}
	}
	return p.parseTypeEnd, nil
}

// parseTypeEnd is the last parser of a "type" field. As "param" or "result" fields can follow, this returns
// parseMoreParamsOrResult to continue the type use.
func (p *typeUseParser) parseTypeEnd(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenUN, tokenID:
		return nil, errors.New("redundant index")
	case tokenRParen:
		p.pos = positionInitial
		return p.parseMoreParamsOrResult, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// beginParamOrResult decides which tokenParser to use based on its field name: "param" or "result".
func (p *typeUseParser) beginParamOrResult(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	p.parsedParamType = false
	switch string(tokenBytes) {
	case "param":
		p.pos = positionParam
		p.parsedParamID = false
		return p.parseParamID, nil
	case "result":
		p.currentField = 0 // reset
		p.pos = positionResult
		return p.parseResult, nil
	case "type":
		return nil, errors.New("redundant type")
	default:
		return p.end(callbackPositionUnhandledField, tok, tokenBytes, line, col)
	}
}

// parseMoreParamsOrResult looks for a '(', and if present returns beginParamOrResult to continue the type. Otherwise,
// it calls parseEnd.
func (p *typeUseParser) parseMoreParamsOrResult(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenLParen {
		return p.beginParamOrResult, nil
	}
	return p.parseEnd(tok, tokenBytes, line, col)
}

// parseMoreResults looks for a '(', and if present returns beginResult to continue any additional results. Otherwise,
// it calls onType.
func (p *typeUseParser) parseMoreResults(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenLParen {
		p.pos = positionFunc
		return p.beginResult, nil
	}
	return p.parseEnd(tok, tokenBytes, line, col)
}

// beginResult attempts to begin a "result" field.
func (p *typeUseParser) beginResult(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok != tokenKeyword {
		return nil, unexpectedToken(tok, tokenBytes)
	}

	switch string(tokenBytes) {
	case "param":
		return nil, errors.New("param after result")
	case "result":
		// Guard >1.0 feature multi-value
		if err := p.enabledFeatures.Require(wasm.FeatureMultiValue); err != nil {
			err = fmt.Errorf("multiple result types invalid as %v", err)
			return nil, err
		}

		p.pos = positionResult
		return p.parseResult, nil
	case "type":
		return nil, errors.New("type after result")
	default:
		return p.end(callbackPositionUnhandledField, tok, tokenBytes, line, col)
	}
}

// parseParamID sets any ID if present and resumes with parseParam .
//
// Ex. A param ID is present `(param $x i32)`
//
//	                          ^
//	parseParam resumes here --+
//
// Ex. No param ID `(param i32)`
//
//	calls parseParam --^
func (p *typeUseParser) parseParamID(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	if tok == tokenID { // Ex. $len
		if err := p.setParamID(tokenBytes); err != nil {
			return nil, err
		}
		p.parsedParamID = true
		return p.parseParam, nil
	}
	return p.parseParam(tok, tokenBytes, line, col)
}

// setParamID adds the normalized ('$' stripped) parameter ID to the paramIndex and the wasm.NameSection.
func (p *typeUseParser) setParamID(idToken []byte) error {
	// Note: currentField is the index of the param field, but due to mixing and matching of abbreviated params
	// it can be less than the param index. Ex. (param i32 i32) (param $v i32) is param field 2, but the 3rd param.
	var idx wasm.Index
	if p.currentInlinedType != nil {
		idx = wasm.Index(len(p.currentInlinedType.Params))
	}

	id := string(stripDollar(idToken))
	if p.paramIndex == nil {
		p.paramIndex = map[string]struct{}{id: {}}
	} else if _, ok := p.paramIndex[id]; ok {
		return fmt.Errorf("duplicate ID $%s", id)
	} else {
		p.paramIndex[id] = struct{}{}
	}
	p.paramNames = append(p.paramNames, &wasm.NameAssoc{Index: idx, Name: id})
	return nil
}

// parseParam records value type and continues if it is an abbreviated form with multiple value types. When complete,
// this returns parseMoreParamsOrResult.
//
// Ex. One param type is present `(param i32)`
//
//	                      records i32 --^  ^
//	parseMoreParamsOrResult resumes here --+
//
// Ex. Multiple param types are present `(param i32 i64)`
//
//	                  records i32 --^   ^  ^
//	                      records i32 --+  |
//	parseMoreParamsOrResult resumes here --+
func (p *typeUseParser) parseParam(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenID: // Ex. $len
		return nil, fmt.Errorf("redundant ID %s", tokenBytes)
	case tokenKeyword: // Ex. i32
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return nil, err
		}
		if p.parsedParamType && p.parsedParamID {
			return nil, errors.New("cannot assign IDs to parameters in abbreviated form")
		}
		if p.currentInlinedType == nil {
			p.currentInlinedType = &wasm.FunctionType{Params: []wasm.ValueType{vt}}
		} else {
			p.currentInlinedType.Params = append(p.currentInlinedType.Params, vt)
		}
		p.parsedParamType = true
		return p.parseParam, nil
	case tokenRParen: // end of this field
		// since multiple param fields are valid, ex `(func (param i32) (param i64))`, prepare for any next.
		p.currentField++
		p.pos = positionInitial
		return p.parseMoreParamsOrResult, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

// parseResult records value type and continues if it is an abbreviated form with multiple value types. When complete,
// this returns parseMoreResults.
//
// Ex. One result type is present `(result i32)`
//
//	               records i32 --^  ^
//	parseMoreResults resumes here --+
//
// Ex. Multiple result types are present `(result i32 i64)`
//
//	           records i32 --^   ^  ^
//	               records i32 --+  |
//	parseMoreResults resumes here --+
func (p *typeUseParser) parseResult(tok tokenType, tokenBytes []byte, _, _ uint32) (tokenParser, error) {
	switch tok {
	case tokenID: // Ex. $len
		return nil, fmt.Errorf("unexpected ID: %s", tokenBytes)
	case tokenKeyword: // Ex. i32
		if p.currentInlinedType != nil && len(p.currentInlinedType.Results) > 0 { // ex (result i32 i32)
			// Guard >1.0 feature multi-value
			if err := p.enabledFeatures.Require(wasm.FeatureMultiValue); err != nil {
				err = fmt.Errorf("multiple result types invalid as %v", err)
				return nil, err
			}
		}
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return nil, err
		}
		if p.currentInlinedType == nil {
			p.currentInlinedType = &wasm.FunctionType{Results: []wasm.ValueType{vt}}
		} else {
			p.currentInlinedType.Results = append(p.currentInlinedType.Results, vt)
		}
		return p.parseResult, nil
	case tokenRParen: // end of this field
		// since multiple result fields are valid, ex `(func (result i32) (result i64))`, prepare for any next.
		p.currentField++
		p.pos = positionInitial
		return p.parseMoreResults, nil
	default:
		return nil, unexpectedToken(tok, tokenBytes)
	}
}

func (p *typeUseParser) parseEnd(tok tokenType, tokenBytes []byte, line, col uint32) (tokenParser, error) {
	pos := callbackPositionUnhandledToken
	if tok == tokenRParen {
		pos = callbackPositionEndField
	}
	return p.end(pos, tok, tokenBytes, line, col)
}

func (p *typeUseParser) errorContext() string {
	switch p.pos {
	case positionType:
		return ".type"
	case positionParam:
		return fmt.Sprintf(".param[%d]", p.currentField)
	case positionResult:
		return fmt.Sprintf(".result[%d]", p.currentField)
	}
	return ""
}

// lineCol saves source positions in case a FormatError needs to be raised later
type lineCol struct {
	// line is FormatError.Line
	line uint32
	// col is FormatError.Col
	col uint32
}

// end invokes onTypeUse to continue parsing
func (p *typeUseParser) end(pos callbackPosition, tok tokenType, tokenBytes []byte, line, col uint32) (parser tokenParser, err error) {
	// Record the potentially inlined type if needed and invoke onTypeUse with the parsed index
	var typeIdx wasm.Index
	if p.parsedTypeField {
		typeIdx, err = p.typeFieldIndex()
		if err != nil {
			return nil, err
		}
	} else if p.currentInlinedType == nil { // no type was found
		typeIdx = p.emptyTypeIndex(p.section)
	} else { // There was no explicitly defined type, so search for any existing ones to re-use
		typeIdx = p.inlinedTypeIndex()
	}

	// Invoke the onTypeUse hook with the current token
	return p.onTypeUse(typeIdx, p.paramNames, pos, tok, tokenBytes, line, col)
}

// inlinedTypeIndex searches for any existing type to re-use
func (p *typeUseParser) inlinedTypeIndex() wasm.Index {
	it := p.currentInlinedType
	for i, t := range p.module.TypeSection {
		if t.EqualsSignature(it.Params, it.Results) {
			return wasm.Index(i)
		}
	}

	p.maybeAddInlinedType(it)
	return 0 // substitute index that will be replaced later
}

func (p *typeUseParser) typeFieldIndex() (wasm.Index, error) {
	if p.currentInlinedType == nil {
		p.currentTypeIndexUnresolved = nil // no need to resolve a signature
		return p.currentTypeIndex, nil
	}

	typeIdx := p.currentTypeIndex
	if p.currentTypeIndexUnresolved == nil {
		// the type was resolved successfully, validate if needed and return it.
		params, results := p.currentInlinedType.Params, p.currentInlinedType.Results
		if err := requireInlinedMatchesReferencedType(p.module.TypeSection, typeIdx, params, results); err != nil {
			return 0, err
		}
	} else {
		// If we parsed param or results, we need to verify the signature against them once the type is known.
		p.maybeAddInlinedType(p.currentInlinedType)
		p.currentTypeIndexUnresolved = nil
	}
	return typeIdx, nil
}

// maybeAddInlinedType records that the current type use had an inlined declaration. It is added to the inlinedTypes, if
// it didn't already exist.
func (p *typeUseParser) maybeAddInlinedType(it *wasm.FunctionType) {
	for i, t := range p.inlinedTypes {
		if t.EqualsSignature(it.Params, it.Results) {
			p.recordInlinedType(wasm.Index(i))
			return
		}
	}

	// If we didn't find a match, we need to insert an inlined type to use it.
	p.recordInlinedType(wasm.Index(len(p.inlinedTypes)))
	p.inlinedTypes = append(p.inlinedTypes, it)
}

type inlinedTypeIndex struct {
	section    wasm.SectionID
	idx        wasm.Index
	inlinedIdx wasm.Index
	typePos    *lineCol
}

func (p *typeUseParser) recordInlinedType(inlinedIdx wasm.Index) {
	idx := p.module.SectionElementCount(p.section)
	i := &inlinedTypeIndex{section: p.section, idx: idx, inlinedIdx: inlinedIdx, typePos: p.currentTypeIndexUnresolved}
	p.inlinedTypeIndices = append(p.inlinedTypeIndices, i)
}

// requireInlinedMatchesReferencedType satisfies the following rule:
//
//	>> If inline declarations are given, then their types must match the referenced function type.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#type-uses%E2%91%A0
func requireInlinedMatchesReferencedType(typeSection []*wasm.FunctionType, index wasm.Index, params, results []wasm.ValueType) error {
	if !typeSection[index].EqualsSignature(params, results) {
		return fmt.Errorf("inlined type doesn't match module.type[%d].func", index)
	}
	return nil
}
