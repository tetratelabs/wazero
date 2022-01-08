package wat

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero/wasm"
)

// currentField holds the positional state of parser. Values are also useful as they allow you to do a reference search
// for all related code including parsers of that position.
type currentField byte

const (
	// fieldInitial is the first position in the source being parsed.
	fieldInitial currentField = iota
	// fieldModule is at the top-level field named "module" and cannot repeat in the same source.
	fieldModule
	// fieldModuleImport is at the position module.import and can repeat in the same module.
	//
	// At the start of the field, moduleParser.currentValue0 tracks importFunc.module while moduleParser.currentValue1
	// tracks importFunc.name. If a field named "func" is encountered, these names are recorded while
	// fieldModuleImportFunc takes over parsing.
	fieldModuleImport
	// fieldModuleImportFunc is at the position module.import.func and cannot repeat in the same import.
	fieldModuleImportFunc
	// fieldModuleImportFuncParam is at the position module.import.func.param and can repeat in the same function.
	fieldModuleImportFuncParam
	// fieldModuleImportFuncResult is at the position module.import.func.result and cannot in the same function.
	fieldModuleImportFuncResult
	// fieldModuleStart is at the position module.start and cannot repeat in the same module.
	fieldModuleStart
)

type moduleParser struct {
	// source is the entire WebAssembly text format source code being parsed.
	source []byte

	// module holds the fields incrementally parsed from tokens in the source.
	module *module

	// currentField is the parser and error context.
	// This is set after reading a field name, ex "module", or after reaching the end of one, ex ')'.
	currentField currentField

	// tokenParser is called by lex, and changes based on the currentField.
	// The initial tokenParser is ensureLParen because %.wat files must begin with a '(' token (ignoring whitespace).
	tokenParser tokenParser

	// currentValue0 is a currentField-specific place to stash a string when parsing a field.
	// Ex. for fieldModuleImport, this would be Math if (import "Math" "PI" ...)
	currentValue0 []byte

	// currentValue1 is a currentField-specific place to stash a string when parsing a field.
	// Ex. for fieldModuleImport, this would be PI if (import "Math" "PI" ...)
	currentValue1 []byte

	// currentImportIndex allows us to track the relative position of imports regardless of position in the source.
	currentImportIndex uint32

	// currentType allows us to build a type for use when parsing module types or inlined type use. In the case of
	// inlined types, a new entry in module.types is only added if a matching signature doesn't already exist.
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A6
	currentType *typeFunc

	// currentParams allow us to accumulate typeFunc.params across multiple fields, as well support abbreviated
	// anonymous parameters. ex. both (param i32) (param i32) and (param i32 i32) formats.
	// See https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A2
	currentParams []wasm.ValueType

	// currentParamIndex allows us to track the relative position of parameters regardless of position in the source.
	currentParamIndex uint32
}

// parseModule parses the configured source into a module. This function returns when the source is exhausted or an
// error occurs.
//
// Here's a description of the return values:
// * module is the result of parsing or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
func parseModule(source []byte) (*module, error) {
	p := moduleParser{source: source, module: &module{}}

	// A valid source must begin with the token '(', but it could be preceded by whitespace or comments. For this
	// reason, we cannot enforce source[0] == '(', and instead need to start the lexer to check the first token.
	p.tokenParser = p.ensureLParen
	line, col, err := lex(p.parse, p.source)
	if err != nil {
		return nil, &FormatError{line, col, p.errorContext(), err}
	}
	return p.module, nil
}

// parse calls the delegate moduleParser.tokenParser
func (p *moduleParser) parse(tok tokenType, tokenBytes []byte, line, col uint32) error {
	return p.tokenParser(tok, tokenBytes, line, col)
}

func (p *moduleParser) ensureLParen(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	if tok != tokenLParen {
		return fmt.Errorf("expected '(', but found %s: %s", tok, tokenBytes)
	}
	p.tokenParser = p.beginField
	return nil
}

// beginField assigns the correct moduleParser.currentField and moduleParser.parseModule based on the source position
// and fieldName being read.
//
// Once the next parser reaches a tokenRParen, moduleParser.endField must be called. This means that there must be
// parity between the currentField values handled here and those handled in moduleParser.endField
//
// TODO: this design will likely be revisited to introduce a type that handles both begin and end of the current field.
func (p *moduleParser) beginField(tok tokenType, fieldName []byte, _, _ uint32) error {
	if tok != tokenKeyword {
		return fmt.Errorf("expected field, but found %s", tok)
	}

	// We expect p.currentField set according to a potentially nested "($fieldName".
	// Each case must return a tokenParser that consumes the rest of the field up to the ')'.
	// Note: each branch must handle any nesting concerns. Ex. "(module (import" nests further to "(func".
	p.tokenParser = nil
	switch p.currentField {
	case fieldInitial:
		if string(fieldName) == "module" {
			p.currentField = fieldModule
			p.tokenParser = p.parseModule
		}
	case fieldModule:
		switch string(fieldName) {
		// TODO: "types"
		case "import":
			p.currentField = fieldModuleImport
			p.tokenParser = p.parseImport
		case "start":
			if p.module.startFunction != nil {
				return errors.New("redundant start")
			}
			p.currentField = fieldModuleStart
			p.tokenParser = p.parseStart
		}
	case fieldModuleImport:
		// Add the next import func object and ready for parsing it.
		if string(fieldName) == "func" {
			p.module.importFuncs = append(p.module.importFuncs, &importFunc{
				module:      string(p.currentValue0),
				name:        string(p.currentValue1),
				importIndex: p.currentImportIndex,
			})
			p.currentField = fieldModuleImportFunc
			p.tokenParser = p.parseImportFunc
		} // TODO: table, memory or global
	case fieldModuleImportFunc:
		switch string(fieldName) {
		case "param": // can repeat
			p.currentField = fieldModuleImportFuncParam
			p.tokenParser = p.parseImportFuncParam
		case "result": // cannot repeat
			if p.currentType != nil && len(p.currentType.results) == 1 {
				return errors.New("redundant result field") // Wasm 1.0 is single or no results
			}
			p.currentField = fieldModuleImportFuncResult
			p.tokenParser = p.parseImportFuncResult
		case "type": // cannot repeat
			return errors.New("TODO: (module (import (func (type")
		}
	}
	if p.tokenParser == nil {
		return fmt.Errorf("unexpected field: %s", string(fieldName))
	}
	return nil
}

// endField should be called after encountering tokenRParen. It places the current parser at the parent position based
// on fixed knowledge of the text format structure.
//
// Design Note: This is an alternative to using a stack as the structure parsed by moduleParser is fixed depth. For
// example, any function body may be parsed in a more dynamic way.
func (p *moduleParser) endField() {
	switch p.currentField {
	case fieldModuleImportFuncParam, fieldModuleImportFuncResult:
		p.currentField = fieldModuleImportFunc
		p.tokenParser = p.parseImportFunc
	case fieldModuleImportFunc:
		p.currentField = fieldModuleImport
		p.tokenParser = p.parseImport
	case fieldModuleStart, fieldModuleImport:
		p.currentField = fieldModule
		p.tokenParser = p.parseModule
	case fieldModule:
		p.currentField = fieldInitial
		p.tokenParser = p.parseUnexpectedTrailingCharacters // only one module is allowed and nothing else
	default: // currentField is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state: %v", p.currentField))
	}
}

func (p *moduleParser) parseModule(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID:
		name := string(tokenBytes)
		if p.module.name != "" {
			return fmt.Errorf("redundant name: %s", name)
		}
		p.module.name = name
	case tokenLParen:
		p.tokenParser = p.beginField // after this look for a field name
		return nil
	case tokenRParen: // end of module
		p.endField()
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) parseImport(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenString:
		// Note: tokenString is minimum length two on account of quotes. Ex. "" or "foo"
		name := tokenBytes[1 : len(tokenBytes)-1] // unquote
		if p.currentValue0 == nil {               // Ex. (module ""
			p.currentValue0 = name
		} else if p.currentValue1 == nil { // Ex. (module "" ""
			p.currentValue1 = name
		} else { // Ex. (module "" "" ""
			return fmt.Errorf("redundant name: %s", name)
		}
	case tokenLParen: // start fields, ex. (func
		// Err if there's a second description. Ex. (import "" "" (func) (func))
		if uint32(len(p.module.importFuncs)) > p.currentImportIndex {
			return p.unexpectedToken(tok, tokenBytes)
		}
		// Err if there are not enough names when we reach a description. Ex. (import func())
		if err := p.validateImportModuleAndName(); err != nil {
			return err
		}
		p.tokenParser = p.beginField
		return nil
	case tokenRParen: // end of this import
		// Err if we never reached a description...
		if uint32(len(p.module.importFuncs)) == p.currentImportIndex {
			if err := p.validateImportModuleAndName(); err != nil {
				return err // Ex. missing (func) and names: (import) or (import "Math")
			}
			return errors.New("missing description field") // Ex. missing (func): (import "Math" "Pi")
		}

		// Multiple imports are allowed, so advance in case there's a next.
		p.currentImportIndex++

		// Reset parsing state: this is late to help give correct error messages on multiple descriptions.
		p.currentValue0, p.currentValue1 = nil, nil
		p.endField()
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

// validateImportModuleAndName ensures we read both the module and name in the text format, even if they were empty.
func (p *moduleParser) validateImportModuleAndName() error {
	if p.currentValue0 == nil && p.currentValue1 == nil {
		return errors.New("missing module and name")
	} else if p.currentValue1 == nil {
		return errors.New("missing name")
	}
	return nil
}

func (p *moduleParser) parseImportFunc(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID: // Ex. $main
		name := string(tokenBytes)
		fn := p.module.importFuncs[len(p.module.importFuncs)-1]
		if fn.funcName != "" {
			return fmt.Errorf("redundant name: %s", name)
		}
		fn.funcName = name
	case tokenLParen: // start fields, ex. (param or (result
		p.tokenParser = p.beginField
		return nil
	case tokenRParen: // end of this import func
		if p.currentType == nil {
			p.currentType = typeFuncEmpty
		}

		fn := p.module.importFuncs[len(p.module.importFuncs)-1]

		// search for an existing signature that matches the current type
		for i, t := range p.module.types {
			if bytes.Equal(p.currentType.params, t.params) && bytes.Equal(p.currentType.results, t.results) {
				fn.typeIndex = uint32(i)
				p.currentType = nil
				break
			}
		}

		// if we didn't find a match, we need to insert a new type and use it
		if p.currentType != nil { // new type
			fn.typeIndex = uint32(len(p.module.types))
			p.module.types = append(p.module.types, p.currentType)
			p.currentType = nil
		}

		// reset parsing state
		p.currentParamIndex = 0
		p.endField()
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) parseImportFuncParam(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID: // Ex. $len
		return errors.New("TODO param name ex (param $len i32), but not in abbreviation ex (param $x i32 $y i32)")
	case tokenKeyword: // Ex. i32
		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return err
		}
		p.currentParams = append(p.currentParams, vt)
	case tokenRParen: // end of this field
		if p.currentParams == nil {
			return errors.New("expected a type")
		}

		// add the parameters to the current type
		if p.currentType == nil {
			p.currentType = &typeFunc{}
		}
		p.currentType.params = append(p.currentType.params, p.currentParams...)

		// since multiple param fields are valid, ex `(func (param i32) (param i64))`, prepare for any next.
		p.currentParams = nil
		p.currentParamIndex++

		p.endField()
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) parseImportFuncResult(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenKeyword: // Ex. i32
		if p.currentType == nil { // Ex. no params: (func (result i32))
			p.currentType = &typeFunc{}
		}

		if p.currentType.results != nil { // Ex. double result: (func (result i32 i32))
			return errors.New("redundant type")
		}

		vt, err := parseValueType(tokenBytes)
		if err != nil {
			return err
		}

		// add the result to the current type
		p.currentType.results = append(p.currentType.results, vt)
	case tokenRParen: // end of this field
		if p.currentType == nil || p.currentType.results == nil {
			return errors.New("expected a type")
		}
		p.endField()
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func parseValueType(tokenBytes []byte) (wasm.ValueType, error) {
	t := string(tokenBytes)
	var vt wasm.ValueType
	switch t {
	case "i32":
		vt = wasm.ValueTypeI32
	case "i64":
		vt = wasm.ValueTypeI64
	case "f32":
		vt = wasm.ValueTypeF32
	case "f64":
		vt = wasm.ValueTypeF64
	default:
		return 0, fmt.Errorf("unknown type: %s", t)
	}
	return vt, nil
}

func (p *moduleParser) parseStart(tok tokenType, tokenBytes []byte, line, col uint32) error {
	switch tok {
	case tokenUN, tokenID: // Ex. $main or 2
		if p.module.startFunction != nil {
			return errors.New("redundant funcidx")
		}
		p.module.startFunction = &startFunction{string(tokenBytes), line, col}
	case tokenRParen: // end of this start
		if p.module.startFunction == nil {
			return errors.New("missing funcidx")
		}
		p.endField()
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) parseUnexpectedTrailingCharacters(_ tokenType, tokenBytes []byte, _, _ uint32) error {
	return fmt.Errorf("unexpected trailing characters: %s", tokenBytes)
}

func (p *moduleParser) unexpectedToken(tok tokenType, tokenBytes []byte) error {
	if tok == tokenLParen { // unbalanced tokenRParen is caught at the lexer layer
		return errors.New("unexpected '('")
	}
	return fmt.Errorf("unexpected %s: %s", tok, tokenBytes)
}

func (p *moduleParser) errorContext() string {
	switch p.currentField {
	case fieldInitial:
		return ""
	case fieldModule:
		return "module"
	case fieldModuleStart:
		return "module.start"
	case fieldModuleImport, fieldModuleImportFunc, fieldModuleImportFuncParam, fieldModuleImportFuncResult:
		if p.currentField == fieldModuleImportFuncParam {
			return fmt.Sprintf("module.import[%d].func.param[%d]", p.currentImportIndex, p.currentParamIndex)
		}
		if p.currentField == fieldModuleImportFuncResult {
			return fmt.Sprintf("module.import[%d].func.result", p.currentImportIndex)
		}
		if p.currentField == fieldModuleImportFunc {
			return fmt.Sprintf("module.import[%d].func", p.currentImportIndex)
		}
		// TODO: table, memory or global
		return fmt.Sprintf("module.import[%d]", p.currentImportIndex)
	default: // currentField is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state: %v", p.currentField))
	}
}
