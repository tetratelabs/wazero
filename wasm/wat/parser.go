package wat

import (
	"errors"
	"fmt"
)

type currentField byte

const (
	// fieldInitial is the first position in the source being parsed.
	fieldInitial currentField = iota
	fieldModule
	fieldModuleImport
	fieldModuleImportFunc
	fieldModuleStart
)

type moduleParser struct {
	source []byte
	module *module
	// currentStringCount allows us to unquote the _import.module and _import.name fields, differentiating empty
	// from never set, without making _import.module and _import.name pointers.
	currentStringCount int
	// currentField is the parser and error context.
	// This is set after reading a field name, ex "module", or after reaching the end of one, ex ')'.
	currentField
	// tokenParser is called by lex, and changes based on the currentField.
	// The initial tokenParser is ensureLParen because %.wat files must begin with a '(' token (ignoring whitespace).
	tokenParser
	// currentImportIndex allows us to track the relative position of imports regardless of what they describe
	currentImportIndex int
	// currentImportModule is the first string of the current import. Ex. "Math" if (import "Math" "PI" (func))
	currentImportModule string
	// currentImportName is the second string of the current import. Ex. "PI" if (import "Math" "PI" (func))
	currentImportName string
	// currentImportHasDesc tracks if we found a description (Ex. (func)) of the current import.
	currentImportHasDesc bool
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

func (p *moduleParser) ensureLParen(tok tokenType, _ []byte, _, _ uint32) error {
	if tok != tokenLParen {
		return fmt.Errorf("expected '(', but found %s", tok)
	}
	p.tokenParser = p.startField
	return nil
}

func (p *moduleParser) startField(tok tokenType, tokenBytes []byte, _, _ uint32) (err error) {
	if tok != tokenKeyword {
		return fmt.Errorf("expected field, but found %s", tok)
	}

	// We expect p.currentField set according to a potentially nested "($fieldName".
	// Each case must return a tokenParser that consumes the rest of the field up to the ')'.
	// Note: each branch must handle any nesting concerns. Ex. "(module (import" nests further to "(func".
	switch p.currentField {
	case fieldInitial:
		p.tokenParser, err = p.initialFieldHandler(tokenBytes)
	case fieldModule:
		p.tokenParser, err = p.moduleFieldHandler(tokenBytes)
	case fieldModuleImport:
		p.tokenParser, err = p.importFieldHandler(tokenBytes)
	default:
		return fmt.Errorf("unexpected current field %d", p.currentField)
	}
	return
}

// initialFieldHandler returns a tokenParser for the top-level fields in the WebAssembly source.
func (p *moduleParser) initialFieldHandler(fieldName []byte) (tokenParser, error) {
	if string(fieldName) == "module" {
		p.currentField = fieldModule
		return p.parseModule, nil
	} else {
		return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
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
		p.tokenParser = p.startField // after this look for a field name
		return nil
	case tokenRParen: // end of module
		p.currentField = fieldInitial
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) moduleFieldHandler(fieldName []byte) (tokenParser, error) {
	switch string(fieldName) {
	// TODO: "types"
	case "import":
		p.currentField = fieldModuleImport
		return p.parseImport, nil
	case "start":
		if p.module.startFunction != nil {
			return nil, errors.New("redundant start")
		}
		p.currentField = fieldModuleStart
		return p.parseStart, nil
	}
	return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
}

func (p *moduleParser) parseImport(tok tokenType, tokenBytes []byte, _, _ uint32) error {
	switch tok {
	case tokenString:
		// Note: tokenString is minimum length two on account of quotes. Ex. "" or "foo"
		name := string(tokenBytes[1 : len(tokenBytes)-1]) // unquote
		if p.currentStringCount == 0 {
			p.currentImportModule = name
		} else if p.currentImportName != "" {
			return fmt.Errorf("redundant name: %s", name)
		} else {
			p.currentImportName = name
		}
		p.currentStringCount = p.currentStringCount + 1
	case tokenLParen: // start fields, ex. (func
		p.tokenParser = p.startField
		return nil
	case tokenRParen: // end of this import
		switch p.currentStringCount { // names precede desc
		case 0:
			return errors.New("missing module and name")
		case 1:
			return errors.New("missing name")
		}
		if !p.currentImportHasDesc {
			return errors.New("missing description")
		}
		p.currentField = fieldModule
		p.currentImportIndex = p.currentImportIndex + 1
		p.currentStringCount = 0
		p.currentImportModule = ""
		p.currentImportName = ""
		p.currentImportHasDesc = false
		p.tokenParser = p.parseModule
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) importFieldHandler(fieldName []byte) (tokenParser, error) {
	if p.currentImportHasDesc {
		return nil, fmt.Errorf("redundant field: %s", string(fieldName))
	}
	switch string(fieldName) {
	case "func":
		p.currentImportHasDesc = true
		p.currentField = fieldModuleImportFunc
		desc := &importFunc{module: p.currentImportModule, name: p.currentImportName, importIndex: p.currentImportIndex}
		p.module.importFuncs = append(p.module.importFuncs, desc)
		return p.parseImportFunc, nil
	} // TODO: table, memory or global
	return nil, fmt.Errorf("unexpected field: %s", string(fieldName))
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
	case tokenLParen:
		return errors.New("TODO: handle (type..) and inlined type (param..) (result..)")
		// * typeuse https://www.w3.org/TR/wasm-core-1/#text-typeuse
		// * inlined type https://www.w3.org/TR/wasm-core-1/#abbreviations%E2%91%A6
	case tokenRParen: // end of this import func
		// TODO: once we handle import types, this won't always be empty
		if len(p.module.types) == 0 {
			p.module.types = append(p.module.types, typeFuncEmpty)
		}
		p.module.importFuncs[len(p.module.importFuncs)-1].typeIndex = 0
		p.currentField = fieldModuleImport
		p.tokenParser = p.parseImport
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
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
		p.currentField = fieldModule
		p.tokenParser = p.parseModule
	default:
		return p.unexpectedToken(tok, tokenBytes)
	}
	return nil
}

func (p *moduleParser) unexpectedToken(tok tokenType, tokenBytes []byte) error {
	if tok == tokenLParen || tok == tokenRParen {
		return fmt.Errorf("unexpected %s", tok)
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
	case fieldModuleImport, fieldModuleImportFunc:
		if p.currentField == fieldModuleImportFunc {
			return fmt.Sprintf("module.import[%d].func", p.currentImportIndex)
		}
		// TODO: table, memory or global
		return fmt.Sprintf("module.import[%d]", p.currentImportIndex)
	}
	return ""
}
