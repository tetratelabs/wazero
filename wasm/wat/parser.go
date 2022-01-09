package wat

import (
	"errors"
	"fmt"
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
	// fieldModuleStart is at the position module.start and cannot repeat in the same module.
	fieldModuleStart
)

type moduleParser struct {
	// tokenParser primarily supports dispatching parse to a different tokenParser depending on the position in the file
	// The initial value is ensureLParen because %.wat files must begin with a '(' token (ignoring whitespace).
	//
	// Design Note: This is an alternative to using a stack as the structure defined in "module" is fixed depth, except
	// function bodies. Any function body may be parsed in a more dynamic way.
	tokenParser tokenParser

	// source is the entire WebAssembly text format source code being parsed.
	source []byte

	// module holds the fields incrementally parsed from tokens in the source.
	module *module

	// currentField is the parser and error context.
	// This is set after reading a field name, ex "module", or after reaching the end of one, ex ')'.
	currentField currentField

	// currentValue0 is a currentField-specific place to stash a string when parsing a field.
	// Ex. for fieldModuleImport, this would be Math if (import "Math" "PI" ...)
	currentValue0 []byte

	// currentValue1 is a currentField-specific place to stash a string when parsing a field.
	// Ex. for fieldModuleImport, this would be PI if (import "Math" "PI" ...)
	currentValue1 []byte

	// currentImportIndex allows us to track the relative position of imports regardless of position in the source.
	currentImportIndex uint32

	typeParser *typeParser
}

// parse has the same signature as tokenParser and called by lex on each token.
//
// The tokenParser this dispatches to should be updated when reading a new field name, and restored to the prior
// value or a different parser on endField.
func (p *moduleParser) parse(tok tokenType, tokenBytes []byte, line, col uint32) error {
	return p.tokenParser(tok, tokenBytes, line, col)
}

// parseModule parses the configured source into a module. This function returns when the source is exhausted or an
// error occurs.
//
// Here's a description of the return values:
// * module is the result of parsing or nil on error
// * err is a FormatError invoking the parser, dangling block comments or unexpected characters.
func parseModule(source []byte) (*module, error) {
	p := moduleParser{source: source, module: &module{}}
	p.typeParser = &typeParser{m: &p}

	// A valid source must begin with the token '(', but it could be preceded by whitespace or comments. For this
	// reason, we cannot enforce source[0] == '(', and instead need to start the lexer to check the first token.
	p.tokenParser = p.ensureLParen
	line, col, err := lex(p.parse, p.source)
	if err != nil {
		return nil, &FormatError{line, col, p.errorContext(), err}
	}
	return p.module, nil
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
			p.tokenParser = p.parseModuleName
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
			p.tokenParser = p.parseImportFuncName
		} // TODO: table, memory or global
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

func (p *moduleParser) parseModuleName(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenID { // Ex. $Math
		name := string(tokenBytes)
		p.module.name = name
		p.tokenParser = p.parseModule
		return nil
	}
	return p.parseModule(tok, tokenBytes, line, col)
}

func (p *moduleParser) parseModule(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenID:
		return fmt.Errorf("redundant name: %s", string(tokenBytes))
	case tokenLParen:
		p.tokenParser = p.beginField // after this look for a field name
		return nil
	case tokenRParen: // end of module
		p.endField()
	default:
		return unexpectedToken(tok, tokenBytes)
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
			return unexpectedToken(tok, tokenBytes)
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
		return unexpectedToken(tok, tokenBytes)
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

func (p *moduleParser) parseImportFuncName(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenID { // Ex. $main
		name := string(tokenBytes)
		fn := p.module.importFuncs[len(p.module.importFuncs)-1]
		fn.funcName = name
		p.tokenParser = p.parseImportFunc
		return nil
	}
	return p.parseImportFunc(tok, tokenBytes, line, col)
}

func (p *moduleParser) parseImportFunc(tok tokenType, tokenBytes []byte, line, col uint32) error {
	switch tok {
	case tokenID: // Ex. (func $main $main)
		return fmt.Errorf("redundant name: %s", string(tokenBytes))
	case tokenLParen: // start fields, ex. (param or (result
		p.typeParser.beginParsingParamsOrResult(p.parseImportFuncAfterType)
		return nil
	}
	return p.parseImportFuncAfterType(tok, tokenBytes, line, col)
}

// parseImportFuncAfterType handles tokens after any type signature was read. The only valid token is tokenRParen
// because no fields are defined except those in the function type.
func (p *moduleParser) parseImportFuncAfterType(tok tokenType, tokenBytes []byte, line, col uint32) error {
	if tok == tokenRParen {
		p.endImportFunc()
		return nil
	}
	return unexpectedToken(tok, tokenBytes)
}

// endImportFunc adds the module.types index for this function regardless of whether one was defined inline or not
func (p *moduleParser) endImportFunc() {
	fn := p.module.importFuncs[len(p.module.importFuncs)-1]
	fn.typeIndex = p.typeParser.findOrAddType(p.module)
	p.currentField = fieldModuleImport
	p.tokenParser = p.parseImport
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
		return unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) parseUnexpectedTrailingCharacters(_ tokenType, tokenBytes []byte, _, _ uint32) error {
	return fmt.Errorf("unexpected trailing characters: %s", tokenBytes)
}

func unexpectedToken(tok tokenType, tokenBytes []byte) error {
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
	case fieldModuleImport:
		return fmt.Sprintf("module.import[%d]", p.currentImportIndex)
	case fieldModuleImportFunc: // TODO: table, memory or global
		return fmt.Sprintf("module.import[%d].func%s", p.currentImportIndex, p.typeParser.errorContext())
	default: // currentField is an enum, we expect to have handled all cases above. panic if we didn't
		panic(fmt.Errorf("BUG: unhandled parsing state: %v", p.currentField))
	}
}
